package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/claude"
)

func TestHeader_SaysANewerReleaseIsAvailable(t *testing.T) {
	checked := 0
	m := New(nil).WithUpdateCheck(func() string { checked++; return "v0.4.0" })
	m = update(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = update(m, snapshotMsg(claude.Snapshot{Files: 1, Agents: []claude.Agent{{ID: "a", Project: "p", Group: "g", State: claude.StateWaiting}}}))
	if strings.Contains(plain(m.render()), "available") {
		t.Fatalf("the header speaks of a release before the check answers")
	}

	cmd := m.checkUpdate()
	if cmd == nil {
		t.Fatal("no check runs as the office opens")
	}
	m = update(m, cmd())
	if checked != 1 {
		t.Errorf("checked %d times, want 1", checked)
	}
	if got := plain(m.render()); !strings.Contains(got, "v0.4.0 available: bah upgrade") {
		t.Errorf("header = %q, want it to name the release and bah upgrade", strings.SplitN(got, "\n", 2)[0])
	}
}

func TestHeader_SaysNothingWithoutACheckOrANewerRelease(t *testing.T) {
	if cmd := New(nil).checkUpdate(); cmd != nil {
		t.Error("a check runs although updates are not looked for")
	}
	m := New(nil).WithUpdateCheck(func() string { return "" })
	m = update(m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = update(m, snapshotMsg(claude.Snapshot{Files: 1, Agents: []claude.Agent{{ID: "a", Project: "p", Group: "g", State: claude.StateWaiting}}}))
	m = update(m, m.checkUpdate()())
	if strings.Contains(plain(m.render()), "available") {
		t.Errorf("the header speaks of a release although there is none")
	}
}
