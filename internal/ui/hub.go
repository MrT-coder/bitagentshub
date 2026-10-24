package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/rest"
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
	m.hubTab, m.setWin, m.aboutScroll, m.usageScroll = tab, nil, 0, 0
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
		m, cmd := m.settingsKey(msg) // every key belongs to the key or name being typed
		return m.followSettings(), cmd
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
		m, cmd := m.settingsKey(msg)
		return m.followSettings(), cmd
	case tabUsage, tabAbout:
		switch k {
		case "up", "k":
			return m.scrollHub(-1), nil
		case "down", "j":
			return m.scrollHub(1), nil
		}
	}
	return m, nil
}

// hubMouse handles the mouse while the hub is open: a click on a tab opens
// it, a click outside the window closes it, the wheel scrolls the tab.
func (m Model) hubMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	if m.renaming || (m.setWin != nil && (m.setWin.capturing || m.setWin.naming)) {
		return m, nil // the name or key being typed keeps the hub as it is
	}
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			return m.scrollHub(-wheelStep), nil
		case tea.MouseWheelDown:
			return m.scrollHub(wheelStep), nil
		}
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		fx, fy := msg.X-1, msg.Y-officeY
		x, y, w, h := m.hubRect()
		if fy == y {
			for _, l := range tabLabels(x, w) {
				if fx >= l.x && fx < l.x+len([]rune(l.text)) {
					return m.showTab(l.tab)
				}
			}
		}
		if fx < x || fx >= x+w || fy < y || fy >= y+h {
			return m.closeHub(), nil
		}
		if m.hubTab == tabSettings {
			return m.settingsClick(fx-x, fy-y-1)
		}
	}
	return m, nil
}

// scrollHub scrolls the open tab by delta rows, kept between its first row
// and the last one that still fills the window.
func (m Model) scrollHub(delta int) Model {
	_, _, _, h := m.hubRect()
	switch m.hubTab {
	case tabUsage:
		m.usageScroll = clampScroll(m.usageScroll+delta, len(m.usageRows()), h-4)
	case tabSettings:
		return m.scrollSettings(delta)
	case tabAbout:
		head, log := m.aboutLines()
		m.aboutScroll = clampScroll(m.aboutScroll+delta, len(log), aboutRoom(h, len(head)))
	}
	return m
}

// clampScroll keeps the first of n rows shown in a window of room rows
// where the window stays full.
func clampScroll(first, n, room int) int {
	return max(0, min(first, n-max(1, room)))
}

// hubFrame centers a hub window up to maxW wide and wantH high on a floor
// of fw by fh.
func hubFrame(fw, fh, maxW, wantH int) (x, y, w, h int) {
	w, h = min(fw-4, maxW), min(fh-2, wantH)
	return (fw - w) / 2, (fh - h) / 2, w, h
}

// The widest each tab's window gets.
const (
	petW   = 64
	usageW = 100
	aboutW = 84
)

// hubRect is where the open tab's window sits on the floor.
func (m Model) hubRect() (x, y, w, h int) {
	fw, fh := m.floorW(), m.floorH()
	switch m.hubTab {
	case tabUsage:
		return hubFrame(fw, fh, usageW, len(m.usageRows())+4)
	case tabSettings:
		x, y, w, h, _ = m.settingsLayout(fw, fh)
		return x, y, w, h
	case tabAbout:
		head, log := m.aboutLines()
		return hubFrame(fw, fh, aboutW, len(head)+len(log)+4)
	}
	return hubFrame(fw, fh, petW, len(m.petLines())+4)
}

// tabLabel is a tab's label in a hub window's top border.
type tabLabel struct {
	tab  int
	x    int
	text string
}

// tabLabels lays the tab labels along the top border of a window at x, w
// wide; the labels that do not fit are left out.
func tabLabels(x, w int) []tabLabel {
	var out []tabLabel
	cx := x + 2
	for i := range tabCount {
		label := " " + tabName(i) + " "
		n := len([]rune(label))
		if cx+n > x+w-2 {
			break
		}
		out = append(out, tabLabel{i, cx, label})
		cx += n + 1
	}
	return out
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
	for _, l := range tabLabels(x, w) {
		st := modalDim()
		if l.tab == m.hubTab {
			st = modalText()
			st.Bold, st.Reverse = true, true
		}
		buf.Text(l.x, y, l.text, st)
	}
}

// drawPetTab says how the pet is and why.
func (m Model) drawPetTab(buf *screen.Buffer) {
	footer := i18n.T(" ←→ tab · r rename · esc close ")
	if m.renaming {
		footer = i18n.T(" type a name · enter save · esc cancel ")
	}
	m.drawHubLines(buf, petW, m.petLines(), footer)
}

// petLines are the pet tab's lines.
func (m Model) petLines() []settingsLine {
	ps := m.petState()
	heading := modalText()
	heading.Bold = true
	title := i18n.Tf("The pet is %s.", ps.Mood.Name())
	if m.petName != "" {
		title = i18n.Tf("%s is %s.", m.petName, ps.Mood.Name())
	}
	lines := []settingsLine{{title, heading}}
	switch {
	case m.renaming:
		lines = append(lines, settingsLine{i18n.T("name: ") + m.petDraft + "█", modalBorder()})
	case m.petName == "":
		lines = append(lines, settingsLine{i18n.T("No name yet; r gives it one."), modalDim()})
	default:
		lines = append(lines, settingsLine{i18n.Tf("Name: %s · r renames it", m.petName), modalDim()})
	}
	if m.petStatus != "" {
		lines = append(lines, settingsLine{m.petStatus, modalBorder()})
	}
	lines = append(lines, settingsLine{"", modalText()})
	for _, why := range ps.Why {
		lines = append(lines, settingsLine{"· " + why, modalText()})
	}
	if m.board.Dir != "" {
		lines = append(lines, settingsLine{"", modalText()}, settingsLine{i18n.Tf("🐟 %d fish earned in all, one per task checked off", m.stars.Total), modalDim()})
	}
	if m.rest != nil {
		d := m.rest.Day()
		lines = append(lines, settingsLine{"", modalText()}, settingsLine{i18n.Tf("⏱ %s supervised today · %d breaks", rest.Short(d.Supervised), d.Breaks), modalDim()})
	}
	return append(lines, settingsLine{"", modalText()},
		settingsLine{i18n.T("Agents wake it, token limits and long stretches without a break tire it, finished tasks feed it."), modalDim()})
}

// drawAbout shows the version and the changelog, scrolled with ↑↓ and the
// wheel.
func (m Model) drawAbout(buf *screen.Buffer) {
	head, log := m.aboutLines()
	_, _, _, h := hubFrame(buf.W, buf.H, aboutW, len(head)+len(log)+4)
	room := aboutRoom(h, len(head))
	first := clampScroll(m.aboutScroll, len(log), room)
	lines := append(head, log[first:min(len(log), first+room)]...)
	m.drawHubLines(buf, aboutW, lines, i18n.T(" ←→ tab · ↑↓ scroll · esc close "))
}

// aboutRoom is how many changelog lines fit in an about window h high under
// its head lines.
func aboutRoom(h, head int) int { return max(1, h-4-head) }

// aboutLines are the about tab's fixed head and the changelog under it.
func (m Model) aboutLines() (head, log []settingsLine) {
	heading := modalText()
	heading.Bold = true
	head = []settingsLine{{"BitAgentsHub " + version.String(), heading},
		{"MIT License · github.com/MrT-coder/bitagentshub", modalDim()},
		{i18n.T("Early preview: tested on Windows with Claude Code so far."), modalBorder()},
		{i18n.T("Other systems and agents are welcome; reports and help are too."), modalDim()},
		{"", modalText()}}
	text := strings.ReplaceAll(m.changelog, "\r\n", "\n")
	if i := strings.Index(text, "\n## "); i >= 0 {
		text = text[i+1:] // the file's title and intro say nothing here
	}
	text = strings.TrimSpace(text)
	if text == "" {
		head = append(head, settingsLine{i18n.T("No changelog in this build."), modalDim()})
	}
	for _, l := range strings.Split(text, "\n") {
		if text == "" {
			break
		}
		st := modalText()
		if strings.HasPrefix(l, "#") {
			st = modalBorder()
		}
		log = append(log, settingsLine{l, st})
	}
	return head, log
}

// drawHubLines centers a hub window up to maxW wide around lines.
func (m Model) drawHubLines(buf *screen.Buffer, maxW int, lines []settingsLine, footer string) {
	x, y, w, h := hubFrame(buf.W, buf.H, maxW, len(lines)+4)
	m.hubBox(buf, x, y, w, h, footer)
	for i, l := range lines {
		if i >= h-4 {
			return
		}
		buf.Text(x+2, y+2+i, truncate(l.text, w-4), l.style)
	}
}
