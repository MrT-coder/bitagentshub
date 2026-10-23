package ui

import tea "charm.land/bubbletea/v2"

// updateMsg is a newer release's tag, or "" when there is none.
type updateMsg string

// WithUpdateCheck looks for a newer release once, as the office opens, with
// check, which returns its tag or "". The header then says how to get it.
func (m Model) WithUpdateCheck(check func() string) Model {
	m.updateCheck = check
	return m
}

func (m Model) checkUpdate() tea.Cmd {
	check := m.updateCheck
	if check == nil {
		return nil
	}
	return func() tea.Msg { return updateMsg(check()) }
}
