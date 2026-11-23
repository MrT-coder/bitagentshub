// Package prs reads the pull requests that wait on you, or that you wait
// on, through the gh command line, and sorts them into the groups GitHub
// shows in its pull request inbox.
package prs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"time"
)

// Runner runs gh with args and returns its output. The office passes
// ExecGH; tests pass a fake, so nothing here needs the network.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// ExecGH runs the gh program.
func ExecGH(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "gh", args...).Output()
}

// Status is what gh needs before the window can show anything.
type Status int

const (
	Ready        Status = iota // gh is installed and logged in
	NotInstalled               // gh is not on PATH
	NotLoggedIn                // gh exists but has no usable login
)

// Setup reports whether gh is installed and logged in.
func Setup(ctx context.Context, run Runner) Status {
	_, err := run(ctx, "auth", "status")
	switch {
	case err == nil:
		return Ready
	case errors.Is(err, exec.ErrNotFound):
		return NotInstalled
	default:
		return NotLoggedIn
	}
}

// Filter narrows the search. Both lists empty means every pull request.
type Filter struct {
	Orgs  []string // "acme"
	Repos []string // "owner/name"
}

func (f Filter) scopes() []string {
	var out []string
	for _, o := range f.Orgs {
		out = append(out, "org:"+o)
	}
	for _, r := range f.Repos {
		out = append(out, "repo:"+r)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// PR is one open pull request.
type PR struct {
	Repo    string // "owner/name"
	Number  int
	Title   string
	Author  string
	URL     string
	Draft   bool
	Updated time.Time
	// Decision is GitHub's review decision: APPROVED, CHANGES_REQUESTED,
	// REVIEW_REQUIRED, or empty.
	Decision string
	// Checks is the state of the latest commit's checks: SUCCESS, FAILURE,
	// ERROR, PENDING, EXPECTED, or empty when there are none.
	Checks string
	// State is OPEN, MERGED or CLOSED; a list read holds open ones only, but
	// one that merged since still shows until the next reading.
	State string
	// Approvers and ChangesBy are the logins whose latest review approves
	// or asks for changes.
	Approvers, ChangesBy []string
	// Direct is true when the review is asked of you, Team when only of a
	// team of yours.
	Direct, Team bool
}

// Group is a section of the inbox.
type Group int

const (
	NeedsReview     Group = iota // your review is asked
	NeedsTeamReview              // a team of yours is asked
	Drafts                       // your drafts
	Waiting                      // yours, waiting on review or checks
	NeedsAction                  // yours, with changes requested or failing checks
	ReadyToMerge                 // yours, approved and green
	OtherOpen                    // every other open pull request, never asked of you
	groupCount
)

func (g Group) String() string {
	return [...]string{"Needs your review", "Needs your teams' review", "Your drafts",
		"Waiting for review or checks", "Needs action", "Ready to merge", "Other open pull requests"}[g]
}

// Groups lists the groups in the order the inbox shows them.
func Groups() []Group {
	out := make([]Group, groupCount)
	for i := range out {
		out[i] = Group(i)
	}
	return out
}

// Inbox is the pull requests that involve you, sorted into groups.
type Inbox struct {
	Viewer string // your GitHub login
	groups [groupCount][]PR
}

// Group is the pull requests of g, most recently updated first.
func (in Inbox) Group(g Group) []PR { return in.groups[g] }

// Pending counts the pull requests whose review is asked of you or of a
// team of yours: the ones the star in the office stands for.
func (in Inbox) Pending() int {
	return len(in.groups[NeedsReview]) + len(in.groups[NeedsTeamReview])
}

const query = `query($q:String!){viewer{login} search(query:$q,type:ISSUE,first:50){nodes{... on PullRequest{
number title url isDraft state updatedAt reviewDecision
latestReviews(first:20){nodes{state author{login}}}
repository{nameWithOwner} author{login}
reviewRequests(first:30){nodes{requestedReviewer{... on User{login} ... on Team{slug}}}}
commits(last:1){nodes{commit{statusCheckRollup{state}}}}}}}}`

type node struct {
	Number        int    `json:"number"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	IsDraft       bool   `json:"isDraft"`
	State         string `json:"state"`
	LatestReviews struct {
		Nodes []reviewNode `json:"nodes"`
	} `json:"latestReviews"`
	UpdatedAt      time.Time `json:"updatedAt"`
	ReviewDecision string    `json:"reviewDecision"`
	Repository     struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Login string `json:"login"`
				Slug  string `json:"slug"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// reviewNode is the latest review of one reviewer.
type reviewNode struct {
	State  string `json:"state"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
}

// sortReviews splits the latest reviews into who approved and who asked
// for changes.
func sortReviews(nodes []reviewNode) (approvers, changes []string) {
	for _, r := range nodes {
		switch {
		case r.Author.Login == "":
		case r.State == "APPROVED":
			approvers = append(approvers, r.Author.Login)
		case r.State == "CHANGES_REQUESTED":
			changes = append(changes, r.Author.Login)
		}
	}
	return approvers, changes
}

// search runs one search and returns the viewer and the pull requests.
func search(ctx context.Context, run Runner, q string) (string, []PR, error) {
	out, err := run(ctx, "api", "graphql", "-f", "query="+query, "-f", "q="+q)
	if err != nil {
		return "", nil, fmt.Errorf("gh search %q: %w", q, err)
	}
	var resp struct {
		Data struct {
			Viewer struct {
				Login string `json:"login"`
			} `json:"viewer"`
			Search struct {
				Nodes []node `json:"nodes"`
			} `json:"search"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", nil, fmt.Errorf("gh search %q: unreadable answer: %w", q, err)
	}
	viewer := resp.Data.Viewer.Login
	var prs []PR
	for _, n := range resp.Data.Search.Nodes {
		if n.Number == 0 { // a search hit that is not a pull request
			continue
		}
		p := PR{
			Repo: n.Repository.NameWithOwner, Number: n.Number, Title: n.Title, Author: n.Author.Login,
			URL: n.URL, Draft: n.IsDraft, Updated: n.UpdatedAt, Decision: n.ReviewDecision, State: n.State,
		}
		p.Approvers, p.ChangesBy = sortReviews(n.LatestReviews.Nodes)
		if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
			p.Checks = n.Commits.Nodes[0].Commit.StatusCheckRollup.State
		}
		for _, r := range n.ReviewRequests.Nodes {
			switch {
			case r.RequestedReviewer.Login != "" && r.RequestedReviewer.Login == viewer:
				p.Direct = true
			case r.RequestedReviewer.Slug != "":
				p.Team = true
			}
		}
		prs = append(prs, p)
	}
	return viewer, prs, nil
}

// Load asks gh for the pull requests that wait on you and those you wrote,
// within filter, and sorts them into the inbox groups.
func Load(ctx context.Context, run Runner, f Filter) (Inbox, error) {
	var in Inbox
	seen := map[string]bool{}
	// The last search is every other open pull request: the whole of each
	// org and repo of the filter, or, with no filter, those that involve you
	// (a search of all GitHub would mean nothing).
	for _, qualifier := range []string{"review-requested:@me", "author:@me", ""} {
		for _, scope := range f.scopes() {
			q := "is:pr is:open " + qualifier
			switch {
			case qualifier == "" && scope == "":
				q = "is:pr is:open sort:updated-desc involves:@me"
			case qualifier == "":
				q = "is:pr is:open sort:updated-desc " + scope
			case scope != "":
				q += " " + scope
			}
			viewer, found, err := search(ctx, run, q)
			if err != nil {
				return Inbox{}, err
			}
			if viewer != "" {
				in.Viewer = viewer
			}
			for _, p := range found {
				key := fmt.Sprintf("%s#%d", p.Repo, p.Number)
				if seen[key] {
					continue
				}
				seen[key] = true
				switch qualifier {
				case "":
					in.groups[OtherOpen] = append(in.groups[OtherOpen], p)
				default:
					in.add(p, qualifier == "author:@me")
				}
			}
		}
	}
	for g := range in.groups {
		list := in.groups[g]
		sort.SliceStable(list, func(i, j int) bool { return list[i].Updated.After(list[j].Updated) })
	}
	return in, nil
}

func (in *Inbox) add(p PR, mine bool) {
	var g Group
	switch {
	case !mine && p.Direct:
		g = NeedsReview
	case !mine:
		g = NeedsTeamReview
	case p.Draft:
		g = Drafts
	case p.Decision == "CHANGES_REQUESTED" || p.Checks == "FAILURE" || p.Checks == "ERROR":
		g = NeedsAction
	case p.Decision == "APPROVED" && (p.Checks == "SUCCESS" || p.Checks == ""):
		g = ReadyToMerge
	default:
		g = Waiting
	}
	in.groups[g] = append(in.groups[g], p)
}
