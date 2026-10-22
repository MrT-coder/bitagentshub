package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/claude"
)

// frontIn returns the two-agent office's snapshot with front in state.
func frontIn(state claude.State) snapshotMsg {
	return snapshotMsg(claude.Snapshot{Agents: []claude.Agent{
		{ID: "front", Group: "acme-company", Project: "portal-front", Cwd: frontDir, State: state, LastActivity: time.Now()},
		{ID: "service", Group: "acme-company", Project: "portal-service", Cwd: serviceDir, State: claude.StateWaiting, LastActivity: time.Now().Add(-10 * time.Minute)},
	}})
}

func TestAgentsWaitingWhenTheOfficeOpensAreIdle(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	_, states := stripRows(m)
	if strings.Count(states, "○ idle") != 2 || strings.Contains(states, "done") {
		t.Errorf("states %q: both agents should be idle", states)
	}
}

func TestAnAgentThatFinishesUnseenIsDoneUntilItsWindowOpens(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = update(m, frontIn(claude.StateThinking))
	if _, states := stripRows(m); !strings.Contains(states, "● working") {
		t.Fatalf("states %q: front should be working", states)
	}
	m = update(m, frontIn(claude.StateWaiting))
	if _, states := stripRows(m); !strings.Contains(states, "✓ done") {
		t.Fatalf("states %q: front finished unseen and should be done", states)
	}
	m, _ = m.openWindow("front")
	m = update(m, frontIn(claude.StateWaiting))
	if _, states := stripRows(m); strings.Contains(states, "done") {
		t.Errorf("states %q: opening its window should make front idle", states)
	}
}

func TestAnAgentThatFinishesInItsOpenWindowIsIdle(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m, _ = m.openWindow("front")
	m = update(m, frontIn(claude.StateTool))
	m = update(m, frontIn(claude.StateWaiting))
	if _, states := stripRows(m); strings.Contains(states, "done") {
		t.Errorf("states %q: front finished while watched and should be idle", states)
	}
}
