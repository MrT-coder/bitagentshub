package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/prs"
)

// recGH is a gh that answers reads and records every write, so no test can
// reach GitHub.
type recGH struct {
	list      prs.Runner
	sha       string // the head commit; "" makes reading it fail
	sent      [][]string
	errReview error
	errAPI    error
}

func (g *recGH) run(ctx context.Context, args ...string) ([]byte, error) {
	switch {
	case args[0] == "pr" && args[1] == "diff":
		return []byte(sampleUIDiff), nil
	case args[0] == "pr" && args[1] == "view":
		if g.sha == "" {
			return nil, errors.New("no head commit")
		}
		return []byte(g.sha + "\n"), nil
	case args[0] == "pr" && args[1] == "review":
		g.sent = append(g.sent, args)
		return nil, g.errReview
	case args[0] == "api" && args[1] != "graphql":
		g.sent = append(g.sent, args)
		return nil, g.errAPI
	}
	return g.list(ctx, args...)
}

// reviewOffice has the diff of pull request 7 open.
func reviewOffice(t *testing.T) (Model, *recGH) {
	t.Helper()
	list, _ := ghAnswering(prNode("acme/api", 7, "Fix login"))
	g := &recGH{list: list, sha: "abc123"}
	m := press(prsOffice(t, prsOn, g.run), key("r"))
	return press(m, key("enter")), g
}

// sendKey presses k and plays what it starts: the command's answer. It
// returns the model after the answer and the command that answer asks for.
func sendKey(m Model, k string) (Model, tea.Cmd) {
	next, cmd := m.Update(key(k))
	m = next.(Model)
	if cmd == nil {
		return m, nil
	}
	next, cmd = m.Update(cmd())
	return next.(Model), cmd
}

func TestApprovingNeedsAYBeforeAnythingIsSent(t *testing.T) {
	m, g := reviewOffice(t)
	m = press(m, key("a"))
	if !strings.Contains(view(m), "Approve this pull request?") {
		t.Fatalf("no confirmation asked:\n%s", view(m))
	}
	for _, k := range []string{"enter", "n", " "} {
		m = press(m, key(k))
	}
	if len(g.sent) != 0 {
		t.Fatalf("something was sent without a y: %q", g.sent)
	}
	m, refresh := sendKey(m, "y")
	want := [][]string{{"pr", "review", "7", "--repo", "acme/api", "--approve"}}
	if !reflect.DeepEqual(g.sent, want) {
		t.Errorf("sent %q, want %q", g.sent, want)
	}
	if !strings.Contains(view(m), "Approved.") {
		t.Errorf("no word of the approval:\n%s", view(m))
	}
	if refresh == nil {
		t.Error("an approval does not read the inbox again, so the star would stay")
	}
}

func TestEscCancelsAConfirmationAndSendsNothing(t *testing.T) {
	m, g := reviewOffice(t)
	m = press(press(m, key("a")), key("esc"))
	if len(g.sent) != 0 || strings.Contains(view(m), "Approve this pull request?") || !m.hubOpen || m.pulls.diff == nil {
		t.Errorf("esc did not cancel and stay in the diff (sent %q):\n%s", g.sent, view(m))
	}
}

func TestRequestingChangesNeedsTextThenAConfirmation(t *testing.T) {
	m, g := reviewOffice(t)
	m = press(press(m, key("x")), key("enter"))
	if !strings.Contains(view(m), "A comment needs some text.") || len(g.sent) != 0 {
		t.Fatalf("an empty request went on (sent %q):\n%s", g.sent, view(m))
	}
	m = press(typeText(m, "add a test"), key("enter"))
	if !strings.Contains(view(m), "(y/esc) Request changes: add a test") || len(g.sent) != 0 {
		t.Fatalf("no confirmation with the text (sent %q):\n%s", g.sent, view(m))
	}
	m, _ = sendKey(m, "y")
	want := [][]string{{"pr", "review", "7", "--repo", "acme/api", "--request-changes", "--body", "add a test"}}
	if !reflect.DeepEqual(g.sent, want) {
		t.Errorf("sent %q, want %q", g.sent, want)
	}
	if !strings.Contains(view(m), "Changes requested.") {
		t.Errorf("view:\n%s", view(m))
	}
}

func TestANoteOnTheWholePullRequest(t *testing.T) {
	m, g := reviewOffice(t)
	m = press(typeText(press(m, key("m")), "thanks!"), key("enter"))
	m, _ = sendKey(m, "y")
	want := [][]string{{"pr", "review", "7", "--repo", "acme/api", "--comment", "--body", "thanks!"}}
	if !reflect.DeepEqual(g.sent, want) || !strings.Contains(view(m), "Note posted.") {
		t.Errorf("sent %q:\n%s", g.sent, view(m))
	}
}

func TestALineCommentGoesToTheLineUnderTheCursor(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  int // in sampleUIDiff: 3 is "-var a = 1", 4 is "+var a = 2"
		want []string
	}{
		{"an added line", 4, []string{"api", "repos/acme/api/pulls/7/comments", "-f", "body=needs a test", "-f", "commit_id=abc123", "-f", "path=app.go", "-F", "line=2", "-f", "side=RIGHT"}},
		{"a removed line", 3, []string{"api", "repos/acme/api/pulls/7/comments", "-f", "body=needs a test", "-f", "commit_id=abc123", "-f", "path=app.go", "-F", "line=2", "-f", "side=LEFT"}},
	} {
		m, g := reviewOffice(t)
		m = m.diffMove(tc.row)
		m = press(typeText(press(m, key("c")), "needs a test"), key("enter"))
		if !strings.Contains(view(m), "app.go:2") || len(g.sent) != 0 {
			t.Fatalf("%s: no confirmation naming the line (sent %q):\n%s", tc.name, g.sent, view(m))
		}
		m, refresh := sendKey(m, "y")
		if len(g.sent) != 1 || !reflect.DeepEqual(g.sent[0], tc.want) {
			t.Errorf("%s: sent %q, want %q", tc.name, g.sent, tc.want)
		}
		if !strings.Contains(view(m), "Comment posted on app.go:2.") {
			t.Errorf("%s: view:\n%s", tc.name, view(m))
		}
		if refresh != nil {
			t.Errorf("%s: a line comment asks for a refresh of the inbox", tc.name)
		}
	}
}

func TestOnlyACodeLineCanBeCommented(t *testing.T) {
	m, g := reviewOffice(t)
	for _, row := range []int{0, 1, 7} { // a file, a hunk header, a note
		m = press(m.diffMove(row), key("c"))
		if !strings.Contains(view(m), "Move to a code line to comment.") {
			t.Errorf("row %d: no hint:\n%s", row, view(m))
		}
	}
	if len(g.sent) != 0 || m.pulls.diff.mode != modeBrowse {
		t.Errorf("a comment started on a row that is not code (sent %q)", g.sent)
	}
}

func TestAFailedSendSaysWhyAndCanBeTriedAgain(t *testing.T) {
	m, g := reviewOffice(t)
	g.errAPI = errors.New("boom")
	m = m.diffMove(4)
	m = press(typeText(press(m, key("c")), "hi"), key("enter"))
	m, _ = sendKey(m, "y")
	if !strings.Contains(view(m), "Could not send: ") || !strings.Contains(view(m), "boom") {
		t.Fatalf("the failure is not shown:\n%s", view(m))
	}
	g.errAPI = nil
	m = press(typeText(press(m, key("c")), "hi again"), key("enter"))
	m, _ = sendKey(m, "y")
	if !strings.Contains(view(m), "Comment posted on app.go:2.") {
		t.Errorf("the second try did not go through:\n%s", view(m))
	}
}

func TestWithoutTheHeadCommitALineCannotBeCommentedAndNothingIsSent(t *testing.T) {
	list, _ := ghAnswering(prNode("acme/api", 7, "Fix login"))
	g := &recGH{list: list} // no sha: reading it fails
	m := press(press(prsOffice(t, prsOn, g.run), key("r")), key("enter"))
	m = m.diffMove(4)
	m = press(typeText(press(m, key("c")), "hi"), key("enter"))
	m, _ = sendKey(m, "y")
	if len(g.sent) != 0 || !strings.Contains(view(m), "head commit") {
		t.Errorf("sent %q:\n%s", g.sent, view(m))
	}
	// The diff itself still reads fine without it.
	if !strings.Contains(view(m), "var a = 2") {
		t.Errorf("the diff is gone:\n%s", view(m))
	}
}

func TestWhileTypingACommentEveryKeyIsTextAndEscCancels(t *testing.T) {
	m, g := reviewOffice(t)
	m = typeText(press(m.diffMove(4), key("c")), "yaxmq g]")
	if !m.hubOpen || m.pulls.diff == nil || !strings.Contains(view(m), "yaxmq g]") {
		t.Fatalf("keys acted instead of typing:\n%s", view(m))
	}
	for range 3 {
		m = press(m, key("backspace"))
	}
	if !strings.Contains(view(m), "yaxmq") || strings.Contains(view(m), "yaxmq g]") {
		t.Errorf("backspace did not delete:\n%s", view(m))
	}
	m = press(m, key("esc"))
	if len(g.sent) != 0 || m.pulls.diff == nil || strings.Contains(view(m), "yaxmq") {
		t.Errorf("esc did not give up the comment and stay in the diff (sent %q):\n%s", g.sent, view(m))
	}
}

func TestAComingConfirmationMovesNothingElse(t *testing.T) {
	m, g := reviewOffice(t)
	m = press(typeText(press(m.diffMove(4), key("c")), "hi"), key("enter"))
	at := m.pulls.diff.cur
	m = press(press(m, key("down")), key("g"))
	if m.pulls.diff.cur != at || len(g.sent) != 0 {
		t.Errorf("a key other than y or esc acted while asking (cursor %d→%d, sent %q)", at, m.pulls.diff.cur, g.sent)
	}
}

func TestTheDiffOffersItsReviewKeysAndTheyFitAPhone(t *testing.T) {
	m, _ := reviewOffice(t)
	if !strings.Contains(view(m), "c comment · a approve · x request changes · m note") {
		t.Errorf("the keys are not offered:\n%s", view(m))
	}
	m = update(m, tea.WindowSizeMsg{Width: 44, Height: 24})
	m = press(m, key("a"))
	for i, line := range strings.Split(view(m), "\n") {
		if n := len([]rune(line)); n > 44 {
			t.Errorf("row %d is %d columns wide: %q", i, n, line)
		}
	}
	if !strings.Contains(view(m), "(y/esc) Approve this pull request?") {
		t.Errorf("the question does not show on a phone:\n%s", view(m))
	}
}

func TestACommentIsTextEvenWhenItLooksLikeASequence(t *testing.T) {
	m, _ := reviewOffice(t)
	m = typeText(press(m.diffMove(4), key("c")), "x")
	m = update(m, tea.PasteMsg{Content: "line1\nline2\x1b]0;pwned\x07"})
	if raw := m.render(); strings.Contains(raw, "\x1b]0;") || strings.Contains(raw, "\x07") {
		t.Errorf("a pasted control sequence reached the screen: %q", raw)
	}
}

func TestOnAPhoneTheQuestionAlwaysSaysHowToAnswer(t *testing.T) {
	m, _ := reviewOffice(t)
	m = update(m, tea.WindowSizeMsg{Width: 44, Height: 24})
	m = typeText(press(m.diffMove(4), key("c")), "a rather long comment that will not fit in the row")
	m = press(m, key("enter"))
	if !strings.Contains(view(m), "(y/esc) Post on app.go:2") {
		t.Errorf("a 44-column question hides how to answer it:\n%s", view(m))
	}
}
