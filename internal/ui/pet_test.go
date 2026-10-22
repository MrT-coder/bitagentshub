package ui

import (
	"testing"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/board"
	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/usage"
)

func TestThePetSleepsUntilAnAgentWorks(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	if got := m.petState().Mood; got != pet.Sleeping {
		t.Fatalf("both agents wait: mood %v, want sleeping", got)
	}
	snap := *m.snap
	snap.Agents = append([]claude.Agent(nil), snap.Agents...)
	snap.Agents[0].State = claude.StateTool
	m = update(m, snapshotMsg(snap))
	if got := m.petState().Mood; got != pet.Busy {
		t.Errorf("an agent works: mood %v, want busy", got)
	}
}

func TestThePetEatsAStarCheckedOffWhileTheOfficeIsOpen(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = m.WithBoard(config.Board{Dir: t.TempDir()})
	m = update(m, boardMsg{tasks: map[string]board.Count{}, stars: board.Stars{Total: 4}})
	if got := m.petState().Mood; got != pet.Hungry {
		t.Fatalf("stars from earlier days only: mood %v, want hungry", got)
	}
	m = update(m, boardMsg{tasks: map[string]board.Count{}, stars: board.Stars{Total: 5, Today: 1}})
	if got := m.petState().Mood; got != pet.Eating {
		t.Errorf("a star just checked off: mood %v, want eating", got)
	}
	m.eatUntil = time.Now().Add(-time.Second)
	if got := m.petState().Mood; got != pet.Sleeping {
		t.Errorf("after eating, fed and nobody working: mood %v, want sleeping", got)
	}
}

func TestThePetReadsTheLimitsEveryHalfMinute(t *testing.T) {
	src := &fakeSource{ok: true, r: usage.Report{Tool: "codex", Today: -1, Windows: []usage.Window{
		{Name: "5h", UsedPercent: 93, ResetsAt: time.Now().Add(time.Hour)},
	}}}
	m := spendingOffice(t, src)
	now := time.Now()
	if !m.usageDue(now) {
		t.Fatal("limits never read: a read is due")
	}
	m = update(m, m.readUsage()())
	if got := m.petState().Mood; got != pet.Exhausted {
		t.Errorf("a limit at 93%%: mood %v, want exhausted", got)
	}
	m.usageAt = now
	if m.usageDue(now.Add(29 * time.Second)) {
		t.Error("a read is due before half a minute")
	}
	if !m.usageDue(now.Add(30 * time.Second)) {
		t.Error("no read due after half a minute")
	}
	if New(nil).usageDue(now) {
		t.Error("without sources no read is ever due")
	}
}
