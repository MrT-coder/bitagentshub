package prs

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

// fakePR renders one pull request the way gh api graphql returns it.
func fakePR(repo string, n int, draft bool, decision, checks, requested string) string {
	state := "null"
	if checks != "" {
		state = fmt.Sprintf(`{"state":%q}`, checks)
	}
	reviewers := ""
	switch {
	case strings.HasPrefix(requested, "user:"):
		reviewers = fmt.Sprintf(`{"requestedReviewer":{"login":%q}}`, strings.TrimPrefix(requested, "user:"))
	case strings.HasPrefix(requested, "team:"):
		reviewers = fmt.Sprintf(`{"requestedReviewer":{"slug":%q}}`, strings.TrimPrefix(requested, "team:"))
	}
	return fmt.Sprintf(`{"number":%d,"title":"PR %d","url":"https://github.com/%s/pull/%d","isDraft":%t,
"updatedAt":"2026-11-20T10:00:00Z","reviewDecision":%q,"repository":{"nameWithOwner":%q},
"author":{"login":"ana"},"reviewRequests":{"nodes":[%s]},
"commits":{"nodes":[{"commit":{"statusCheckRollup":%s}}]}}`, n, n, repo, n, draft, decision, repo, reviewers, state)
}

func response(viewer string, nodes ...string) []byte {
	return []byte(fmt.Sprintf(`{"data":{"viewer":{"login":%q},"search":{"nodes":[%s]}}}`, viewer, strings.Join(nodes, ",")))
}

// fakeGH answers a search by its qualifier and remembers the searches asked.
type fakeGH struct {
	requested, authored, others []string
	asked                       []string
	err                         error
}

func (f *fakeGH) run(_ context.Context, args ...string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	var search string
	for _, a := range args {
		if s, ok := strings.CutPrefix(a, "q="); ok {
			search = s
		}
	}
	f.asked = append(f.asked, search)
	if strings.Contains(search, "review-requested:@me") {
		return response("me", f.requested...), nil
	}
	if strings.Contains(search, "author:@me") {
		return response("me", f.authored...), nil
	}
	return response("me", f.others...), nil
}

func TestTheInboxSortsPullRequestsIntoGitHubsGroups(t *testing.T) {
	gh := &fakeGH{
		requested: []string{
			fakePR("acme/api", 1, false, "", "SUCCESS", "user:me"),
			fakePR("acme/api", 2, false, "", "SUCCESS", "team:core"),
		},
		authored: []string{
			fakePR("acme/web", 3, true, "", "", ""),
			fakePR("acme/web", 4, false, "CHANGES_REQUESTED", "SUCCESS", ""),
			fakePR("acme/web", 5, false, "", "FAILURE", ""),
			fakePR("acme/web", 6, false, "APPROVED", "SUCCESS", ""),
			fakePR("acme/web", 7, false, "REVIEW_REQUIRED", "PENDING", ""),
		},
	}
	in, err := Load(context.Background(), gh.run, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		group Group
		want  []int
	}{
		{NeedsReview, []int{1}},
		{NeedsTeamReview, []int{2}},
		{Drafts, []int{3}},
		{NeedsAction, []int{4, 5}},
		{ReadyToMerge, []int{6}},
		{Waiting, []int{7}},
	} {
		var got []int
		for _, p := range in.Group(tc.group) {
			got = append(got, p.Number)
		}
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%v = %v, want %v", tc.group, got, tc.want)
		}
	}
	if in.Viewer != "me" {
		t.Errorf("viewer = %q, want me", in.Viewer)
	}
}

func TestOnlyReviewsAskedOfYouAreWaitingOnYou(t *testing.T) {
	gh := &fakeGH{
		requested: []string{
			fakePR("acme/api", 1, false, "", "", "user:me"),
			fakePR("acme/api", 2, false, "", "", "team:core"),
		},
		authored: []string{fakePR("acme/web", 3, false, "", "", "")},
	}
	in, err := Load(context.Background(), gh.run, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if got := in.Pending(); got != 2 {
		t.Errorf("pending = %d, want 2: the direct and the team request, not your own PR", got)
	}
}

func TestTheFilterSearchesEachOrgAndRepoAndKeepsOneCopy(t *testing.T) {
	gh := &fakeGH{requested: []string{fakePR("acme/api", 1, false, "", "", "user:me")}}
	in, err := Load(context.Background(), gh.run, Filter{Orgs: []string{"acme", "tools"}, Repos: []string{"me/side"}})
	if err != nil {
		t.Fatal(err)
	}
	var scopes []string
	for _, s := range gh.asked {
		if strings.Contains(s, "review-requested:@me") {
			scopes = append(scopes, s)
		}
	}
	want := []string{
		"is:pr is:open review-requested:@me org:acme",
		"is:pr is:open review-requested:@me org:tools",
		"is:pr is:open review-requested:@me repo:me/side",
	}
	if fmt.Sprint(scopes) != fmt.Sprint(want) {
		t.Errorf("searches = %q, want %q", scopes, want)
	}
	if n := len(in.Group(NeedsReview)); n != 1 {
		t.Errorf("the same PR came back from three searches and shows %d times, want 1", n)
	}
}

func TestNoFilterSearchesEverywhere(t *testing.T) {
	gh := &fakeGH{}
	if _, err := Load(context.Background(), gh.run, Filter{}); err != nil {
		t.Fatal(err)
	}
	for _, s := range gh.asked {
		if strings.Contains(s, "org:") || strings.Contains(s, "repo:") {
			t.Errorf("search %q is scoped though no filter is set", s)
		}
	}
}

func TestEveryOpenPRShowsEvenWhenNoReviewIsAskedOfYou(t *testing.T) {
	gh := &fakeGH{others: []string{
		fakePR("acme/api", 9, false, "", "SUCCESS", ""),
		fakePR("acme/api", 1, false, "", "SUCCESS", "user:me"), // also found by the review search
	}, requested: []string{fakePR("acme/api", 1, false, "", "SUCCESS", "user:me")}}
	in, err := Load(context.Background(), gh.run, Filter{Orgs: []string{"acme"}})
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, p := range in.Group(OtherOpen) {
		got = append(got, p.Number)
	}
	if fmt.Sprint(got) != "[9]" {
		t.Errorf("other open PRs = %v, want [9]", got)
	}
	if n := len(in.Group(NeedsReview)); n != 1 {
		t.Errorf("the review asked of you shows %d times, want 1", n)
	}
	if got := in.Pending(); got != 1 {
		t.Errorf("pending = %d, want 1: only the review asked of you raises the star", got)
	}
}

func TestWithoutAFilterTheOtherPRsAreThoseThatInvolveYou(t *testing.T) {
	gh := &fakeGH{}
	if _, err := Load(context.Background(), gh.run, Filter{}); err != nil {
		t.Fatal(err)
	}
	want := "is:pr is:open sort:updated-desc involves:@me"
	for _, s := range gh.asked {
		if s == want {
			return
		}
	}
	t.Errorf("searches = %q, want one of %q: an unscoped search would read all of GitHub", gh.asked, want)
}

func TestALoadFailureIsReportedNotHidden(t *testing.T) {
	gh := &fakeGH{err: errors.New("boom")}
	if _, err := Load(context.Background(), gh.run, Filter{}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want it to carry the gh failure", err)
	}
}

func TestSetupTellsWhatGHNeeds(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want Status
	}{
		{nil, Ready},
		{exec.ErrNotFound, NotInstalled},
		{errors.New("exit status 1"), NotLoggedIn},
	} {
		run := func(context.Context, ...string) ([]byte, error) { return nil, tc.err }
		if got := Setup(context.Background(), run); got != tc.want {
			t.Errorf("err %v: status = %v, want %v", tc.err, got, tc.want)
		}
	}
}

func TestTheInboxKeepsWhoApprovedAndWhoAskedForChanges(t *testing.T) {
	node := `{"number":3,"title":"t","state":"OPEN","isDraft":true,"repository":{"nameWithOwner":"a/b"},"author":{"login":"ana"},
"latestReviews":{"nodes":[{"state":"APPROVED","author":{"login":"bob"}},{"state":"CHANGES_REQUESTED","author":{"login":"cy"}},{"state":"DISMISSED","author":{"login":"dee"}}]}}`
	run := func(context.Context, ...string) ([]byte, error) {
		return []byte(`{"data":{"viewer":{"login":"me"},"search":{"nodes":[` + node + `]}}}`), nil
	}
	in, err := Load(context.Background(), run, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	p := in.Group(NeedsTeamReview)[0]
	if p.State != "OPEN" || !reflect.DeepEqual(p.Approvers, []string{"bob"}) || !reflect.DeepEqual(p.ChangesBy, []string{"cy"}) {
		t.Errorf("state %q approvers %v changes %v", p.State, p.Approvers, p.ChangesBy)
	}
}

func TestReadInfoSaysIfThePullRequestIsStillOpen(t *testing.T) {
	var got []string
	run := func(_ context.Context, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"state":"MERGED","isDraft":false,"headRefOid":"abc","latestReviews":[{"state":"APPROVED","author":{"login":"bob"}}]}`), nil
	}
	in, err := ReadInfo(context.Background(), run, "a/b", 9)
	if err != nil || in.State != "MERGED" || in.SHA != "abc" || !reflect.DeepEqual(in.Approvers, []string{"bob"}) {
		t.Errorf("info %+v, err %v", in, err)
	}
	if want := []string{"pr", "view", "9", "--repo", "a/b", "--json", "state,isDraft,headRefOid,latestReviews"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ran %q, want %q", got, want)
	}
}
