package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/claude"
)

// stripRows are the strip's first and last rows: number and agent, then
// state (the branch row sits between them).
func stripRows(m Model) (string, string) {
	lines := strings.Split(view(m), "\n")
	return lines[2], lines[4]
}

func TestTheStripListsStationsWithAgentAndState(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	names, states := stripRows(m)
	for _, want := range []string{"1 claude · portal-front", "2 claude · portal-service"} {
		if !strings.Contains(names, want) {
			t.Errorf("strip names %q should contain %q", names, want)
		}
	}
	if strings.Count(states, "○ idle") != 2 {
		t.Errorf("strip states %q should show each station's state", states)
	}
	if v := view(m); strings.Contains(v, "Today") || strings.Contains(v, "Week") || strings.Contains(v, "-daily-limit") {
		t.Errorf("the token totals should be gone from the header:\n%s", v)
	}
	// The floor still starts on the same row, so mouse hits are unchanged.
	if lines := strings.Split(view(m), "\n"); !strings.HasPrefix(strings.TrimSpace(lines[officeTop-1]), "┌─ office") {
		t.Errorf("row %d should be the office wall, got %q", officeTop-1, lines[officeTop-1])
	}
}

func TestTheStripNamesTheAgentProgramAndMarksTheOpenStation(t *testing.T) {
	rec := &termRecorder{}
	m := New(nil).WithAgentCmd([]string{`C:\tools\codex.exe`, "--yolo"})
	m.startTerm = rec.start
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = update(m, snapshotMsg(claude.Snapshot{}))
	m, _ = m.startLocal(t.TempDir())
	names, states := stripRows(m)
	if !strings.Contains(names, "1 codex") {
		t.Errorf("a local agent started with codex should read \"1 codex\": %q", names)
	}
	if !strings.Contains(states, "○ idle") {
		t.Errorf("states %q", states)
	}
	raw := strings.Split(m.render(), "\n")[2]
	if !strings.Contains(raw, "\x1b[7m") {
		t.Errorf("the open station should be marked: %q", raw)
	}
}

func TestAgentKindFromItsCommand(t *testing.T) {
	cases := map[string][]string{
		"claude": {"claude", "--session-id", "x"},
		"codex":  {`C:\Users\me\bin\Codex.EXE`},
		"gemini": {"/usr/local/bin/gemini"},
		"":       nil,
	}
	for want, argv := range cases {
		if got := agentKind(argv); got != want {
			t.Errorf("agentKind(%v) = %q, want %q", argv, got, want)
		}
	}
}

func TestTheTitleCountsTheStationsOnTheFloor(t *testing.T) {
	m, _, _ := twoAgentOffice(t) // two running agents and one session that runs nowhere
	if title := strings.Split(view(m), "\n")[0]; !strings.Contains(title, "2 stations · 0 working") {
		t.Errorf("title = %q, want the stations on the floor", title)
	}
}

func TestTheStripShowsEachStationsBranchWithAheadAndBehind(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = update(m, gitMsg{
		folderKey(frontDir):   {branch: "develop", ahead: 2, behind: 1},
		folderKey(serviceDir): {branch: "main"},
	})
	branches := strings.Split(view(m), "\n")[3]
	for _, want := range []string{"develop ↑2↓1", "main"} {
		if !strings.Contains(branches, want) {
			t.Errorf("branch row %q should contain %q", branches, want)
		}
	}
	raw := strings.Split(m.render(), "\n")[3]
	if !strings.Contains(raw, "152;195;121m↑2") || !strings.Contains(raw, "224;108;117m↓1") {
		t.Errorf("ahead should be green and behind red: %q", raw)
	}
}
