package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/office"
)

// afterPrefix handles the key pressed right after the prefix: the prefix
// again goes to the focused program, a bound key runs its action and any
// other key is dropped.
func (m Model) afterPrefix(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.win != nil {
		m.win.sel, m.win.confirmMove = nil, false
	}
	if msg.String() == m.keys.Prefix {
		if m.win != nil {
			return m.typeKey(msg)
		}
		return m, nil
	}
	if action, n, ok := m.keys.Action(msg.String(), true); ok {
		return m.runAction(action, n)
	}
	return m, nil
}

// runAction runs a bound action. Window actions do nothing on the board.
func (m Model) runAction(action string, n int) (Model, tea.Cmd) {
	switch action {
	case config.FocusStation:
		return m.focusStation(n)
	case config.OpenHub:
		return m.openHub(tabPet)
	case config.ShowUsage:
		return m.openHub(tabUsage)
	case config.ShowSettings:
		return m.openHub(tabSettings)
	case config.NewAgent:
		return m.openPicker() // over the window, which stays open under it
	}
	w := m.win
	if w == nil {
		return m, nil
	}
	switch action {
	case config.CloseWindow:
		return m.closeWindow(), nil
	case config.ToggleMaximize:
		w.max = !w.max
		m.resizePanes()
	case config.NewTerminal:
		w.max = false // the new terminal shows beside the others
		return m.newShell()
	case config.OpenBoard:
		w.max = false
		return m.openBoard()
	case config.CloseTerminal:
		return m.closeShell(), nil
	case config.FocusPaneLeft:
		w.focus = max(w.focus-1, 0)
	case config.FocusPaneRight:
		w.focus = min(w.focus+1, len(w.panes)-1)
	}
	if w.max {
		m.resizePanes() // the maximized pane follows the focus
	}
	return m, nil
}

// focusStation opens station n (1-based, in board order) in one step,
// closing the open window first.
func (m Model) focusStation(n int) (Model, tea.Cmd) {
	order := office.Order(m.layout)
	if n < 1 || n > len(order) {
		msg := i18n.Tf("No station %d.", n)
		if m.win != nil {
			m.win.status = msg
		} else {
			m.notice = msg
		}
		return m, nil
	}
	return m.switchTo(order[n-1])
}

// switchTo opens a station's window, closing the open one first.
func (m Model) switchTo(id string) (Model, tea.Cmd) {
	if m.win != nil {
		if m.win.agentID == id {
			return m, nil
		}
		m = m.closeWindow()
	}
	m.selected = id
	return m.openWindow(id)
}

// keyHint spells the first key bound to an action for footers, with the
// prefix written out ("ctrl+a d") and a range as "1-9".
func (m Model) keyHint(action string) string {
	keys := m.keys.Keys(action)
	if len(keys) == 0 {
		return ""
	}
	k := strings.Replace(keys[0], "..", "-", 1)
	if rest, ok := strings.CutPrefix(k, "prefix+"); ok {
		return m.keys.Prefix + " " + rest
	}
	return k
}

// windowHints is the window footer: only the main actions, those bound
// after the prefix, grouped behind it. Settings lists every key.
func (m Model) windowHints() string {
	var after []string
	for _, a := range []struct{ action, label string }{
		{config.CloseWindow, i18n.T("close")},
		{config.ToggleMaximize, i18n.T("maximize")},
		{config.OpenBoard, i18n.T("board")},
		{config.NewAgent, i18n.T("new agent")},
	} {
		for _, k := range m.keys.Keys(a.action) {
			if rest, ok := strings.CutPrefix(k, "prefix+"); ok {
				after = append(after, rest+" "+a.label)
				break
			}
		}
	}
	if len(after) == 0 {
		return ""
	}
	return m.keys.Prefix + i18n.T(" then: ") + strings.Join(after, " · ")
}
