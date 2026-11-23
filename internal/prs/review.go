package prs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ReviewEvent is what a review does to a pull request.
type ReviewEvent int

const (
	Approve ReviewEvent = iota
	RequestChanges
	Note // a comment on the whole pull request
)

func (e ReviewEvent) String() string {
	return [...]string{"approve", "request changes", "note"}[e]
}

// SubmitReview sends a review of pull request n of repo. Asking for changes
// and leaving a note need some text; an approval may carry it. Nothing is
// sent for an incomplete one.
func SubmitReview(ctx context.Context, run Runner, repo string, n int, ev ReviewEvent, body string) error {
	body = strings.TrimSpace(body)
	if body == "" && ev != Approve {
		return fmt.Errorf("a review that %s needs some text", ev)
	}
	args := []string{"pr", "review", strconv.Itoa(n), "--repo", repo}
	switch ev {
	case Approve:
		args = append(args, "--approve")
	case RequestChanges:
		args = append(args, "--request-changes")
	default:
		args = append(args, "--comment")
	}
	if body != "" {
		args = append(args, "--body", body)
	}
	if _, err := run(ctx, args...); err != nil {
		return fmt.Errorf("gh pr review %s#%d: %w", repo, n, err)
	}
	return nil
}

// Side is the side of the diff a comment is on.
type Side int

const (
	Right Side = iota // the new file: added and unchanged lines
	Left              // the old file: removed lines
)

func (s Side) String() string {
	if s == Left {
		return "LEFT"
	}
	return "RIGHT"
}

// LineComment is a comment on one line of a pull request's diff. Commit is
// the head commit the diff was read at; the line is the number on Side.
type LineComment struct {
	Repo   string
	Number int
	Commit string
	Path   string
	Line   int
	Side   Side
	Body   string
}

// PostLineComment puts the comment on its line in GitHub. The text goes as a
// raw field, so a leading @ stays text and is never read as a file.
func PostLineComment(ctx context.Context, run Runner, c LineComment) error {
	body := strings.TrimSpace(c.Body)
	switch {
	case body == "":
		return errors.New("a comment needs some text")
	case c.Commit == "":
		return errors.New("the head commit of the pull request is unknown")
	case c.Path == "" || c.Line <= 0:
		return errors.New("a comment needs a file and a line")
	}
	args := []string{"api", fmt.Sprintf("repos/%s/pulls/%d/comments", c.Repo, c.Number),
		"-f", "body=" + body, "-f", "commit_id=" + c.Commit, "-f", "path=" + c.Path,
		"-F", "line=" + strconv.Itoa(c.Line), "-f", "side=" + c.Side.String()}
	if _, err := run(ctx, args...); err != nil {
		return fmt.Errorf("gh comment on %s#%d: %w", c.Repo, c.Number, err)
	}
	return nil
}

// HeadSHA is the head commit of pull request n of repo, which a line
// comment is anchored to.
func HeadSHA(ctx context.Context, run Runner, repo string, n int) (string, error) {
	out, err := run(ctx, "pr", "view", strconv.Itoa(n), "--repo", repo, "--json", "headRefOid", "-q", ".headRefOid")
	if err != nil {
		return "", fmt.Errorf("gh pr view %s#%d: %w", repo, n, err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("gh pr view %s#%d: no head commit", repo, n)
	}
	return sha, nil
}

// Info is what a pull request is right now: whether it is still open, a
// draft, who approved it and the commit its diff is of.
type Info struct {
	State     string // OPEN, MERGED or CLOSED
	Draft     bool
	Approvers []string
	ChangesBy []string
	SHA       string
}

// ReadInfo reads the current state of pull request n of repo.
func ReadInfo(ctx context.Context, run Runner, repo string, n int) (Info, error) {
	out, err := run(ctx, "pr", "view", strconv.Itoa(n), "--repo", repo, "--json", "state,isDraft,headRefOid,latestReviews")
	if err != nil {
		return Info{}, fmt.Errorf("gh pr view %s#%d: %w", repo, n, err)
	}
	var v struct {
		State         string       `json:"state"`
		IsDraft       bool         `json:"isDraft"`
		HeadRefOid    string       `json:"headRefOid"`
		LatestReviews []reviewNode `json:"latestReviews"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return Info{}, fmt.Errorf("gh pr view %s#%d: unreadable answer: %w", repo, n, err)
	}
	in := Info{State: v.State, Draft: v.IsDraft, SHA: v.HeadRefOid}
	in.Approvers, in.ChangesBy = sortReviews(v.LatestReviews)
	return in, nil
}
