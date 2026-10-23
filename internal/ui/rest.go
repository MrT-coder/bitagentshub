package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/rest"
)

// restSaveEvery is how often the day's supervised time is written down; a
// crash loses at most this much.
const restSaveEvery = 30 * time.Second

// WithRest turns the supervision clock on as [rest] says, carrying on the
// day saved at path.
func (m Model) WithRest(cfg config.Rest, path string) Model {
	m.restCfg, m.restPath = cfg, path
	if !cfg.Enabled {
		m.rest = nil
		return m
	}
	day, err := rest.Load(path, m.now())
	if err != nil {
		day = rest.Day{} // an unreadable log starts the day over
	}
	m.rest = rest.New(cfg.Clock, day, m.now())
	return m
}

// restInput tells the clock the user did something in the office.
func (m Model) restInput() {
	if m.rest != nil {
		m.rest.Input(m.now())
	}
}

// restTick moves the clock with the agents at work or waiting on the user,
// and saves the day now and then.
func (m *Model) restTick() tea.Cmd {
	if m.rest == nil {
		return nil
	}
	now := m.now()
	active := 0
	for _, a := range m.agents() {
		if a.Blocked || a.State.Working() {
			active++
		}
	}
	m.rest.Tick(now, active)
	if now.Sub(m.restSaved) < restSaveEvery {
		return nil
	}
	m.restSaved = now
	return m.saveRest()
}

// saveRest writes the day in the background.
func (m Model) saveRest() tea.Cmd {
	if m.rest == nil || m.restPath == "" {
		return nil
	}
	day, path := m.rest.Day(), m.restPath
	return func() tea.Msg {
		_ = rest.Save(path, day) // the clock goes on; the next save tries again
		return nil
	}
}

// restSummary is the clock's part of the header: the time since the last
// break, and a break suggested once it is due, when the agents are not in
// the middle of their work.
func (m Model) restSummary(working int) string {
	if m.rest == nil {
		return ""
	}
	s, out := m.rest.State(), ""
	cont := rest.Short(s.Continuous)
	switch {
	case s.Overdue:
		out = i18n.Tf(" · ☕ %s without a break", cont)
	case s.Due && working == 0:
		out = i18n.Tf(" · ☕ %s · a good moment for a break", cont)
	case s.Due:
		out = i18n.Tf(" · ☕ %s · a break when the agents finish", cont)
	default:
		out = " · ⏱ " + cont
	}
	if s.OverBudget {
		out += i18n.Tf(" · %s today", rest.Short(m.rest.Day().Supervised))
	}
	return out
}

// holdPrompt asks once before enter sends a prompt to the open window's
// agent while a break is overdue; the next enter sends it. An agent waiting
// on a permission prompt is answered at once.
func (m Model) holdPrompt() bool {
	w := m.win
	if m.rest == nil || !m.restCfg.Friction || !m.rest.State().Overdue {
		return false
	}
	if a, ok := m.agent(w.agentID); ok && a.Blocked {
		return false
	}
	if w.restAsked {
		w.restAsked = false
		return false
	}
	w.restAsked = true
	w.status = i18n.Tf("☕ %s without a break · enter again sends it", rest.Short(m.rest.State().Continuous))
	return true
}
