package ui

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// petNameMax matches the longest name config.toml takes.
const petNameMax = 20

// petNameMsg is what saving the pet's name reports back.
type petNameMsg struct {
	name string
	err  error
}

// WithPetName sets the pet's name read from the config file.
func (m Model) WithPetName(name string) Model {
	m.petName = name
	return m
}

// petLabel is what the pet is called: its name, or "pet" until named.
func (m Model) petLabel() string {
	if m.petName != "" {
		return m.petName
	}
	return i18n.T("pet")
}

// hover follows a mouse move without a button: over the pet, its name
// shows. Only the office floor hovers; a window or modal on top hides it.
func (m Model) hover(mouse tea.Mouse) Model {
	m.hoverPet = !m.hubOpen && m.win == nil && m.picker == nil &&
		office.PetHit(m.layout, mouse.X-1, mouse.Y-officeY+m.floorScroll)
	return m
}

// drawPetName shows the pet's name over its corner while it is hovered.
func (m Model) drawPetName(buf *screen.Buffer) {
	if !m.hoverPet {
		return
	}
	x, y, _, _ := m.layout.PetCells()
	buf.Text(x, y-1-m.floorScroll, " "+m.petLabel()+" ", screen.Style{Fg: screen.RGB{R: 0x2e, G: 0x34, B: 0x40}, HasFg: true,
		Bg: screen.RGB{R: 0xeb, G: 0xcb, B: 0x8b}, HasBg: true, Bold: true})
}

// renameKey edits the pet's name in the pet tab: enter saves it, esc gives
// up and leaves the tab open.
func (m Model) renameKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.renaming, m.petStatus = false, ""
	case "backspace":
		if r := []rune(m.petDraft); len(r) > 0 {
			m.petDraft = string(r[:len(r)-1])
		}
	case "enter":
		name := strings.TrimSpace(m.petDraft)
		switch {
		case name == "":
			m.petStatus = i18n.T("A name needs at least one letter.")
		case m.settings == nil:
			m.petStatus = i18n.T("Settings are not available here.")
		default:
			m.renaming, m.petStatus = false, i18n.T("Saving…")
			s := m.settings
			return m, func() tea.Msg { return petNameMsg{name: name, err: s.SetPetName(name)} }
		}
	default:
		if text, ok := keyText(msg); ok && utf8.RuneCountInString(m.petDraft+text) <= petNameMax {
			m.petDraft += text
		}
	}
	return m, nil
}

// petNamed takes in the answer of saving the name.
func (m Model) petNamed(msg petNameMsg) Model {
	if msg.err != nil {
		m.petStatus = msg.err.Error()
		return m
	}
	m.petName, m.petStatus = msg.name, i18n.T("Name saved in config.toml.")
	return m
}
