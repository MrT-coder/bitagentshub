package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/prs"
)

var sampleUIDiff = strings.Join([]string{
	"diff --git a/app.go b/app.go",
	"--- a/app.go",
	"+++ b/app.go",
	"@@ -1,3 +1,3 @@",
	" package main",
	"-var a = 1",
	"+var a = 2",
	" // end",
	"diff --git a/logo.png b/logo.png",
	"Binary files a/logo.png and b/logo.png differ",
	"",
}, "\n")

// ghWithDiff answers pull request reads like ghAnswering and pr diff with
// diff, or with err.
func ghWithDiff(diff string, err error, nodes ...string) prs.Runner {
	list, _ := ghAnswering(nodes...)
	return func(ctx context.Context, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "pr" {
			return []byte(diff), err
		}
		return list(ctx, args...)
	}
}

// diffOffice opens the first pull request's diff.
func diffOffice(t *testing.T, diff string, err error) Model {
	t.Helper()
	m := press(prsOffice(t, prsOn, ghWithDiff(diff, err, prNode("acme/api", 7, "Fix login"), prNode("acme/api", 8, "Add cache"))), key("r"))
	return press(m, key("enter"))
}

func TestEnterOpensTheSelectedPullRequestsDiff(t *testing.T) {
	m := diffOffice(t, sampleUIDiff, nil)
	for _, want := range []string{"acme/api#7", "Fix login", "app.go", "@@ -1,3 +1,3 @@", "package main", "var a = 1", "var a = 2", "logo.png", "binary file"} {
		if !strings.Contains(view(m), want) {
			t.Errorf("view missing %q:\n%s", want, view(m))
		}
	}
	if strings.Contains(view(m), "Needs your review") {
		t.Errorf("the list still shows under the diff:\n%s", view(m))
	}
}

func TestTheDiffOfTheSecondPullRequestIsAskedForWhenItIsSelected(t *testing.T) {
	var asked []string
	list, _ := ghAnswering(prNode("acme/api", 7, "Fix login"), prNode("acme/api", 8, "Add cache"))
	run := func(ctx context.Context, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "pr" {
			asked = append(asked, strings.Join(args, " "))
			return []byte(sampleUIDiff), nil
		}
		return list(ctx, args...)
	}
	m := press(prsOffice(t, prsOn, run), key("r"))
	m = press(press(m, key("down")), key("enter"))
	if len(asked) != 1 || asked[0] != "pr diff 8 --repo acme/api --color=never" {
		t.Errorf("gh was asked %q", asked)
	}
	_ = m
}

func TestTheDiffShowsMarkersAndColorsForAddedAndRemovedLines(t *testing.T) {
	m := diffOffice(t, sampleUIDiff, nil)
	for _, want := range []string{"-var a = 1", "+var a = 2", " package main"} {
		if !strings.Contains(view(m), want) {
			t.Errorf("view missing %q:\n%s", want, view(m))
		}
	}
	raw := m.render()
	for name, bg := range map[string]string{"added": "48;2;32;53;40", "removed": "48;2;58;32;36"} {
		if !strings.Contains(raw, bg) {
			t.Errorf("no %s-line background (%s) in the screen", name, bg)
		}
	}
}

func TestTheDiffShowsLineNumbersWhenThereIsRoomAndNotOnAPhone(t *testing.T) {
	m := diffOffice(t, sampleUIDiff, nil)
	if !strings.Contains(view(m), "   2    2  // end") && !strings.Contains(view(m), "   3    3  // end") {
		t.Errorf("a wide diff has no line numbers:\n%s", view(m))
	}
	m = update(m, tea.WindowSizeMsg{Width: 44, Height: 24})
	v := view(m)
	if !strings.Contains(v, "+var a = 2") || strings.Contains(v, "   3    3 ") {
		t.Errorf("a 44-column diff keeps its gutter or loses its lines:\n%s", v)
	}
	for i, line := range strings.Split(v, "\n") {
		if n := len([]rune(line)); n > 44 {
			t.Errorf("row %d is %d columns wide: %q", i, n, line)
		}
	}
}

func TestTheCursorWalksTheDiffAndFileKeysJumpBetweenFiles(t *testing.T) {
	m := diffOffice(t, sampleUIDiff, nil)
	cur := func() int { return m.pulls.diff.cur }
	if cur() != 0 {
		t.Fatalf("the cursor starts at %d, want the first row", cur())
	}
	m = press(press(m, key("down")), key("j"))
	if cur() != 2 {
		t.Errorf("down, j: cursor %d, want 2", cur())
	}
	m = press(m, key("k"))
	m = press(m, key("up"))
	if cur() != 0 {
		t.Errorf("k, up: cursor %d, want 0", cur())
	}
	m = press(m, key("]"))
	if row := m.pulls.diff.rows[cur()]; row.kind != rowFile || row.text != "logo.png" {
		t.Errorf("] landed on %+v, want the logo.png file row", row)
	}
	m = press(m, key("["))
	if row := m.pulls.diff.rows[cur()]; row.kind != rowFile || row.text != "app.go" {
		t.Errorf("[ landed on %+v, want the app.go file row", row)
	}
	m = press(m, key("G"))
	if cur() != len(m.pulls.diff.rows)-1 {
		t.Errorf("G: cursor %d, want the last row %d", cur(), len(m.pulls.diff.rows)-1)
	}
	m = press(m, key("g"))
	if cur() != 0 {
		t.Errorf("g: cursor %d, want the first row", cur())
	}
	for range 40 {
		m = press(m, key("up"))
	}
	if cur() != 0 {
		t.Errorf("up at the top: cursor %d", cur())
	}
}

func TestALongDiffScrollsToKeepTheCursorInView(t *testing.T) {
	lines := []string{"diff --git a/big.go b/big.go", "--- a/big.go", "+++ b/big.go", "@@ -1,60 +1,60 @@"}
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf(" line number %02d", i))
	}
	m := diffOffice(t, strings.Join(lines, "\n")+"\n", nil)
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 24})
	if !strings.Contains(view(m), "line number 01") || strings.Contains(view(m), "line number 60") {
		t.Fatalf("the top of a long diff:\n%s", view(m))
	}
	m = press(m, key("G"))
	if !strings.Contains(view(m), "line number 60") || strings.Contains(view(m), "line number 01") {
		t.Errorf("G did not scroll to the end:\n%s", view(m))
	}
	m = press(m, key("g"))
	if !strings.Contains(view(m), "line number 01") {
		t.Errorf("g did not scroll back to the top:\n%s", view(m))
	}
	for range 30 {
		m = press(m, key("down"))
	}
	if !strings.Contains(view(m), fmt.Sprintf("line number %02d", 30-3)) && !strings.Contains(view(m), "line number 27") {
		t.Errorf("moving the cursor 30 rows down left it out of view:\n%s", view(m))
	}
	m = press(m, key("pgdown"))
	if m.pulls.diff.cur <= 30 {
		t.Errorf("page down did not move the cursor (at %d)", m.pulls.diff.cur)
	}
}

func TestEscLeavesTheDiffForTheListAndAgainClosesTheHub(t *testing.T) {
	m := diffOffice(t, sampleUIDiff, nil)
	m = press(m, key("esc"))
	if !m.hubOpen || !strings.Contains(view(m), "Needs your review (2)") || m.pulls.diff != nil {
		t.Fatalf("esc did not come back to the list:\n%s", view(m))
	}
	if m.pulls.sel != 0 {
		t.Errorf("the list lost its selection: %d", m.pulls.sel)
	}
	m = press(m, key("esc"))
	if m.hubOpen {
		t.Error("the second esc left the hub open")
	}
}

func TestAnUnreadableDiffSaysSo(t *testing.T) {
	m := diffOffice(t, "", errors.New("boom"))
	if !strings.Contains(view(m), "Could not read the diff: ") || !strings.Contains(view(m), "boom") {
		t.Errorf("view:\n%s", view(m))
	}
	m = press(m, key("esc"))
	if m.pulls.diff != nil {
		t.Error("esc did not leave the failed diff")
	}
}

func TestAnAnswerForAPullRequestYouLeftIsIgnored(t *testing.T) {
	m := diffOffice(t, sampleUIDiff, nil)
	m = press(m, key("esc"))
	m = update(m, prsDiffMsg{repo: "acme/api", number: 7, files: prs.ParseDiff(sampleUIDiff)})
	if m.pulls.diff != nil || !strings.Contains(view(m), "Needs your review") {
		t.Errorf("a late diff reopened the view:\n%s", view(m))
	}
}

func TestADiffIsTextNeverControlSequencesAndKeepsItsIndentation(t *testing.T) {
	evil := strings.Join([]string{
		"diff --git a/x.go b/x.go", "--- a/x.go", "+++ b/x.go", "@@ -1 +1,2 @@",
		" ok",
		"+\treturn \x1b]0;pwned\x07 x",
		"+\u202eevil",
		"",
	}, "\n")
	m := diffOffice(t, evil, nil)
	raw := m.render()
	if strings.Contains(raw, "\x1b]0;") || strings.Contains(raw, "\x07") {
		t.Errorf("the screen carries a control sequence from the diff: %q", raw)
	}
	if !strings.Contains(view(m), "+    return") {
		t.Errorf("a tab was lost instead of becoming spaces:\n%s", view(m))
	}
}

func TestClickingADiffRowMovesTheCursorThere(t *testing.T) {
	m := diffOffice(t, sampleUIDiff, nil)
	x, y, _, _ := m.hubRect()
	m = press(m, click(1+x+4, officeY+y+2+3))
	if m.pulls.diff.cur != 3 {
		t.Errorf("cursor %d, want 3", m.pulls.diff.cur)
	}
}

func TestTheWheelMovesTheDiffCursor(t *testing.T) {
	m := diffOffice(t, sampleUIDiff, nil)
	m = press(m, wheel(false))
	if m.pulls.diff.cur != wheelStep {
		t.Errorf("wheel down: cursor %d, want %d", m.pulls.diff.cur, wheelStep)
	}
	m = press(m, wheel(true))
	if m.pulls.diff.cur != 0 {
		t.Errorf("wheel up: cursor %d, want 0", m.pulls.diff.cur)
	}
}

func TestEnterOnAnEmptyListDoesNothing(t *testing.T) {
	run, _ := ghAnswering()
	m := press(prsOffice(t, prsOn, run), key("r"))
	m = press(m, key("enter"))
	if m.pulls.diff != nil {
		t.Error("enter opened a diff with no pull request selected")
	}
}

func TestAPullRequestTitleCannotInjectControlSequences(t *testing.T) {
	run, _ := ghAnswering(prNode("acme/api", 7, "Fix \x1b]0;pwned\x07 login"))
	m := press(prsOffice(t, prsOn, run), key("r"))
	if raw := m.render(); strings.Contains(raw, "\x1b]0;") || strings.Contains(raw, "\x07") {
		t.Errorf("the list carries a control sequence from a title: %q", raw)
	}
}

func TestABinaryFileShowsNoLineCounts(t *testing.T) {
	m := diffOffice(t, sampleUIDiff, nil)
	for _, l := range strings.Split(view(m), "\n") {
		if strings.Contains(l, "logo.png") && strings.Contains(l, "+0") {
			t.Errorf("a binary file row shows counts: %q", l)
		}
	}
}
