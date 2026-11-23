package prs

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// recorder is a gh that remembers what it was asked and answers out.
type recorder struct {
	calls [][]string
	out   string
	err   error
}

func (r *recorder) run(_ context.Context, args ...string) ([]byte, error) {
	r.calls = append(r.calls, args)
	return []byte(r.out), r.err
}

func TestSubmitReviewAsksGHForEachKindOfReview(t *testing.T) {
	for _, tc := range []struct {
		event ReviewEvent
		body  string
		want  []string
	}{
		{Approve, "", []string{"pr", "review", "7", "--repo", "acme/api", "--approve"}},
		{Approve, "looks good", []string{"pr", "review", "7", "--repo", "acme/api", "--approve", "--body", "looks good"}},
		{RequestChanges, "add a test", []string{"pr", "review", "7", "--repo", "acme/api", "--request-changes", "--body", "add a test"}},
		{Note, "thanks!", []string{"pr", "review", "7", "--repo", "acme/api", "--comment", "--body", "thanks!"}},
	} {
		r := &recorder{}
		if err := SubmitReview(context.Background(), r.run, "acme/api", 7, tc.event, tc.body); err != nil {
			t.Errorf("%v: %v", tc.event, err)
			continue
		}
		if len(r.calls) != 1 || !reflect.DeepEqual(r.calls[0], tc.want) {
			t.Errorf("%v: gh asked %q, want %q", tc.event, r.calls, tc.want)
		}
	}
}

func TestSubmitReviewRefusesWhatGitHubWouldRefuseWithoutAsking(t *testing.T) {
	for _, ev := range []ReviewEvent{RequestChanges, Note} {
		r := &recorder{}
		if err := SubmitReview(context.Background(), r.run, "acme/api", 7, ev, "  "); err == nil {
			t.Errorf("%v with no text was sent", ev)
		}
		if len(r.calls) != 0 {
			t.Errorf("%v with no text still asked gh: %q", ev, r.calls)
		}
	}
}

func TestSubmitReviewReportsAGHFailure(t *testing.T) {
	r := &recorder{err: errors.New("boom")}
	if err := SubmitReview(context.Background(), r.run, "acme/api", 7, Approve, ""); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want it to carry the gh failure", err)
	}
}

func TestPostLineCommentSendsTheLineOnTheRightSide(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    LineComment
		want []string
	}{
		{"a new line", LineComment{"acme/api", 7, "abc123", "app.go", 2, Right, "needs a test"},
			[]string{"api", "repos/acme/api/pulls/7/comments", "-f", "body=needs a test", "-f", "commit_id=abc123", "-f", "path=app.go", "-F", "line=2", "-f", "side=RIGHT"}},
		{"a removed line", LineComment{"acme/api", 7, "abc123", "old.go", 9, Left, "why drop it?"},
			[]string{"api", "repos/acme/api/pulls/7/comments", "-f", "body=why drop it?", "-f", "commit_id=abc123", "-f", "path=old.go", "-F", "line=9", "-f", "side=LEFT"}},
	} {
		r := &recorder{}
		if err := PostLineComment(context.Background(), r.run, tc.c); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if len(r.calls) != 1 || !reflect.DeepEqual(r.calls[0], tc.want) {
			t.Errorf("%s: gh asked %q, want %q", tc.name, r.calls, tc.want)
		}
	}
}

func TestAnAtSignInACommentIsTextNotAFile(t *testing.T) {
	r := &recorder{}
	c := LineComment{"acme/api", 7, "abc123", "app.go", 2, Right, "@/etc/passwd"}
	if err := PostLineComment(context.Background(), r.run, c); err != nil {
		t.Fatal(err)
	}
	for i, a := range r.calls[0] {
		if a == "body=@/etc/passwd" && r.calls[0][i-1] != "-f" {
			t.Errorf("the body goes as %q, which gh would read as a file", r.calls[0][i-1])
		}
	}
}

func TestPostLineCommentRefusesAnIncompleteComment(t *testing.T) {
	ok := LineComment{"acme/api", 7, "abc123", "app.go", 2, Right, "hi"}
	for name, c := range map[string]LineComment{
		"no text":   {ok.Repo, ok.Number, ok.Commit, ok.Path, ok.Line, ok.Side, " "},
		"no commit": {ok.Repo, ok.Number, "", ok.Path, ok.Line, ok.Side, ok.Body},
		"no path":   {ok.Repo, ok.Number, ok.Commit, "", ok.Line, ok.Side, ok.Body},
		"no line":   {ok.Repo, ok.Number, ok.Commit, ok.Path, 0, ok.Side, ok.Body},
	} {
		r := &recorder{}
		if err := PostLineComment(context.Background(), r.run, c); err == nil {
			t.Errorf("%s: sent", name)
		}
		if len(r.calls) != 0 {
			t.Errorf("%s: asked gh %q", name, r.calls)
		}
	}
}

func TestHeadSHAReadsTheCommitTheDiffIsAt(t *testing.T) {
	r := &recorder{out: "abc123def\n"}
	sha, err := HeadSHA(context.Background(), r.run, "acme/api", 7)
	if err != nil || sha != "abc123def" {
		t.Fatalf("sha = %q, %v", sha, err)
	}
	want := []string{"pr", "view", "7", "--repo", "acme/api", "--json", "headRefOid", "-q", ".headRefOid"}
	if !reflect.DeepEqual(r.calls[0], want) {
		t.Errorf("gh asked %q, want %q", r.calls[0], want)
	}
	r = &recorder{out: "\n"}
	if _, err := HeadSHA(context.Background(), r.run, "acme/api", 7); err == nil {
		t.Error("an empty answer was taken for a commit")
	}
}
