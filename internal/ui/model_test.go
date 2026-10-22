package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/herdr"
)

func plain(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			inEsc = false
		case !inEsc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestView_SaysLoadingBeforeFirstSnapshot(t *testing.T) {
	m := update(New(nil), tea.WindowSizeMsg{Width: 60, Height: 30})
	if !strings.Contains(plain(m.render()), "Reading transcripts") {
		t.Errorf("view = %q, want loading message", plain(m.render()))
	}
}

func TestView_NamesTheEmptyStateWhenNoAgentIsActive(t *testing.T) {
	m := update(New(nil), tea.WindowSizeMsg{Width: 60, Height: 30})
	m = update(m, snapshotMsg(claude.Snapshot{Files: 3}))
	if !strings.Contains(plain(m.render()), "No agents running. Press n to start one.") {
		t.Errorf("view = %q, want empty-state message", plain(m.render()))
	}
}

// runningInHerdr links agent "a" to a herdr pane, so the office shows it.
func runningInHerdr(m Model) Model {
	return update(m, herdrMsg{panes: []herdr.Pane{{PaneID: "w1:p1", Agent: "claude", SessionID: "a"}}})
}

func TestView_ShowsPerAgentSpend(t *testing.T) {
	m := update(New(nil), tea.WindowSizeMsg{Width: 60, Height: 30})
	m = update(m, snapshotMsg(claude.Snapshot{Agents: []claude.Agent{{
		ID: "a", Group: "acme-company", Project: "shopping-portal", State: claude.StateTool,
		LastActivity:    time.Now(),
		Today:           claude.Usage{Output: 320_000},
		Session:         claude.Usage{Output: 1_100_000},
		ActiveSubagents: 2,
	}}}))
	m = runningInHerdr(m)
	v := plain(m.render())
	for _, want := range []string{"shopping-portal", "320K ● working +2"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestView_NoLineExceedsTheTerminalWidth(t *testing.T) {
	m := update(New(nil), tea.WindowSizeMsg{Width: 44, Height: 30})
	m = update(m, snapshotMsg(claude.Snapshot{Agents: []claude.Agent{{
		ID: "a", Project: strings.Repeat("very-long-project-name-", 4), State: claude.StateThinking,
		LastActivity: time.Now(),
	}}}))
	m = runningInHerdr(m)
	for _, line := range strings.Split(plain(m.render()), "\n") {
		if n := len([]rune(line)); n > 44 {
			t.Errorf("line has %d cells > 44: %q", n, line)
		}
	}
}
