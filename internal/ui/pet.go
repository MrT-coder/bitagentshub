package ui

import (
	"time"

	"github.com/MrT-coder/bitagentshub/internal/pet"
)

const (
	// petUsageEvery is how often the tools' limits are read for the pet
	// while the hub's usage tab is closed; open, it reads every usageEvery.
	petUsageEvery = 30 * time.Second
	eatFor        = 5 * time.Second
	// With nobody at a desk only the pet moves, so the frames advance in
	// slow steps of idleFrames.
	idleEvery  = time.Second
	idleFrames = 8
)

// usageDue reports whether the tools' limits should be read again.
func (m Model) usageDue(now time.Time) bool {
	every := petUsageEvery
	if m.hubOpen && m.hubTab == tabUsage {
		every = usageEvery
	}
	return len(m.sources) > 0 && now.Sub(m.usageAt) >= every
}

// petState reads the pet's mood from the agents, the limits read last and
// the board's stars.
func (m Model) petState() pet.State {
	t := pet.Telemetry{
		Reports:  m.reports,
		Board:    m.board.Dir != "",
		FedToday: m.stars.Today,
		Eating:   time.Now().Before(m.eatUntil),
	}
	if m.rest != nil {
		st := m.rest.State()
		t.Supervised, t.RestDue = st.Continuous, st.Due
	}
	for _, a := range m.agents() {
		switch {
		case a.Blocked:
			t.Blocked++
		case a.State.Working():
			t.Working++
		}
	}
	return pet.Read(t)
}
