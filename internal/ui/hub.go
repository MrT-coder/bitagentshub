package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/screen"
	"github.com/MrT-coder/bitagentshub/internal/version"
)

// The hub is the pet's window: every system panel as a tab of one window,
// opened by clicking the pet or with the open_hub key.
const (
	tabPet = iota
	tabUsage
	tabSettings
	tabAbout
	tabCount
)

// tabName is a tab's name as its label shows it.
func tabName(tab int) string {
	switch tab {
	case tabUsage:
		return i18n.T("usage")
	case tabSettings:
		return i18n.T("settings")
	case tabAbout:
		return i18n.T("about")
	}
	return i18n.T("pet")
}

// WithChangelog sets the changelog the About tab shows.
func (m Model) WithChangelog(text string) Model {
	m.changelog = text
	return m
}

// openHub opens the hub on tab, or closes it when it already shows tab.
func (m Model) openHub(tab int) (Model, tea.Cmd) {
	if m.hubOpen && m.hubTab == tab {
		return m.closeHub(), nil
	}
	m.hubOpen = true
	return m.showTab(tab)
}

func (m Model) closeHub() Model {
	m.hubOpen, m.setWin = false, nil
	return m
}

// showTab switches the open hub to tab and starts reading what it shows.
func (m Model) showTab(tab int) (Model, tea.Cmd) {
	m.hubTab, m.setWin, m.aboutScroll = tab, nil, 0
	switch tab {
	case tabUsage:
		return m, m.readUsage()
	case tabSettings:
		m.setWin = &settingsWin{}
		return m, m.readSettings()
	}
	return m, nil
}

// hubAction runs a hub binding while the hub is open: its own key closes
// it, the usage and settings keys go to their tab or close it there.
func (m Model) hubAction(action string) (Model, tea.Cmd, bool) {
	switch action {
	case config.OpenHub:
		return m.closeHub(), nil, true
	case config.ShowUsage:
		m, cmd := m.openHub(tabUsage)
		return m, cmd, true
	case config.ShowSettings:
		m, cmd := m.openHub(tabSettings)
		return m, cmd, true
	}
	return m, nil, false
}

// hubKey handles keys while the hub is open. It drops every key it does not
// use, so nothing types into a program hidden under it.
func (m Model) hubKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.hubTab == tabPet && m.renaming {
		return m.renameKey(msg) // every key belongs to the name being typed
	}
	if sw := m.setWin; m.hubTab == tabSettings && sw != nil && (sw.capturing || sw.naming) {
		return m.settingsKey(msg) // every key belongs to the key or name being typed
	}
	k := msg.String()
	if m.prefixed {
		m.prefixed = false
		if action, _, ok := m.keys.Action(k, true); ok {
			m, cmd, _ := m.hubAction(action)
			return m, cmd
		}
		return m, nil
	}
	if k == m.keys.Prefix {
		m.prefixed = true
		return m, nil
	}
	if action, _, ok := m.keys.Action(k, false); ok {
		if m, cmd, done := m.hubAction(action); done {
			return m, cmd
		}
	}
	switch k {
	case "esc", "q", "p":
		return m.closeHub(), nil
	case "u":
		return m.openHub(tabUsage)
	case "s":
		return m.openHub(tabSettings)
	case "left":
		return m.showTab((m.hubTab + tabCount - 1) % tabCount)
	case "right":
		return m.showTab((m.hubTab + 1) % tabCount)
	}
	switch m.hubTab {
	case tabPet:
		if k == "r" {
			m.renaming, m.petDraft, m.petStatus = true, m.petName, ""
		}
	case tabSettings:
		return m.settingsKey(msg)
	case tabAbout:
		switch k {
		case "up", "k":
			m.aboutScroll = max(m.aboutScroll-1, 0)
		case "down", "j":
			m.aboutScroll++ // drawAbout keeps it in range
		}
	}
	return m, nil
}

// drawHub paints the open hub's tab.
func (m Model) drawHub(buf *screen.Buffer) {
	switch m.hubTab {
	case tabPet:
		m.drawPetTab(buf)
	case tabUsage:
		m.drawUsage(buf)
	case tabSettings:
		m.drawSettings(buf)
	case tabAbout:
		m.drawAbout(buf)
	}
}

// hubBox draws a hub tab's window with the tab row in its top border, the
// open tab highlighted.
func (m Model) hubBox(buf *screen.Buffer, x, y, w, h int, footer string) {
	box(buf, x, y, w, h, "", footer)
	cx := x + 2
	for i := range tabCount {
		label := " " + tabName(i) + " "
		if cx+len([]rune(label)) > x+w-2 {
			return
		}
		st := modalDim
		if i == m.hubTab {
			st = modalText
			st.Bold, st.Reverse = true, true
		}
		cx = buf.Text(cx, y, label, st) + 1
	}
}

// drawPetTab says how the pet is and why.
func (m Model) drawPetTab(buf *screen.Buffer) {
	ps := m.petState()
	heading := modalText
	heading.Bold = true
	title := i18n.Tf("The pet is %s.", ps.Mood.Name())
	if m.petName != "" {
		title = i18n.Tf("%s is %s.", m.petName, ps.Mood.Name())
	}
	lines := []settingsLine{{title, heading}}
	switch {
	case m.renaming:
		lines = append(lines, settingsLine{i18n.T("name: ") + m.petDraft + "█", modalBorder})
	case m.petName == "":
		lines = append(lines, settingsLine{i18n.T("No name yet; r gives it one."), modalDim})
	default:
		lines = append(lines, settingsLine{i18n.Tf("Name: %s · r renames it", m.petName), modalDim})
	}
	if m.petStatus != "" {
		lines = append(lines, settingsLine{m.petStatus, modalBorder})
	}
	lines = append(lines, settingsLine{"", modalText})
	for _, why := range ps.Why {
		lines = append(lines, settingsLine{"· " + why, modalText})
	}
	if m.board.Dir != "" {
		lines = append(lines, settingsLine{"", modalText}, settingsLine{i18n.Tf("🐟 %d fish earned in all, one per task checked off", m.stars.Total), modalDim})
	}
	lines = append(lines, settingsLine{"", modalText},
		settingsLine{i18n.T("Agents wake it, token limits tire it, finished tasks feed it."), modalDim})
	footer := i18n.T(" ←→ tab · r rename · esc close ")
	if m.renaming {
		footer = i18n.T(" type a name · enter save · esc cancel ")
	}
	m.drawHubLines(buf, 64, lines, footer)
}

// drawAbout shows the version and the changelog, scrolled with ↑↓.
func (m Model) drawAbout(buf *screen.Buffer) {
	heading := modalText
	heading.Bold = true
	lines := []settingsLine{{"BitAgentsHub " + version.String(), heading},
		{"MIT License · github.com/MrT-coder/bitagentshub", modalDim},
		{i18n.T("Early preview: tested on Windows with Claude Code so far."), modalBorder},
		{i18n.T("Other systems and agents are welcome; reports and help are too."), modalDim},
		{"", modalText}}
	text := strings.ReplaceAll(m.changelog, "\r\n", "\n")
	if i := strings.Index(text, "\n## "); i >= 0 {
		text = text[i+1:] // the file's title and intro say nothing here
	}
	text = strings.TrimSpace(text)
	if text == "" {
		lines = append(lines, settingsLine{i18n.T("No changelog in this build."), modalDim})
	}
	var log []settingsLine
	for _, l := range strings.Split(text, "\n") {
		if text == "" {
			break
		}
		st := modalText
		if strings.HasPrefix(l, "#") {
			st = modalBorder
		}
		log = append(log, settingsLine{l, st})
	}
	room := max(1, buf.H-2-4-len(lines)) // drawHubLines' height less its borders and margins
	first := min(m.aboutScroll, max(0, len(log)-room))
	lines = append(lines, log[first:min(len(log), first+room)]...)
	m.drawHubLines(buf, 84, lines, i18n.T(" ←→ tab · ↑↓ scroll · esc close "))
}

// drawHubLines centers a hub window up to maxW wide around lines.
func (m Model) drawHubLines(buf *screen.Buffer, maxW int, lines []settingsLine, footer string) {
	fw, fh := buf.W, buf.H
	w := min(fw-4, maxW)
	h := min(fh-2, len(lines)+4)
	x, y := (fw-w)/2, (fh-h)/2
	m.hubBox(buf, x, y, w, h, footer)
	for i, l := range lines {
		if i >= h-4 {
			return
		}
		buf.Text(x+2, y+2+i, truncate(l.text, w-4), l.style)
	}
}
