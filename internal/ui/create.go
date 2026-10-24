package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/projects"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// picker chooses the folder a new agent or terminal starts in.
type picker struct {
	filter string
	all    []projects.Project
	sel    int
	status string
}

// choice is a picker row; create marks a typed folder that does not exist
// yet and is made when chosen.
type choice struct {
	projects.Project
	create bool
}

type createdMsg struct {
	name, pane string
	err        error
}

func (m Model) openPicker() (Model, tea.Cmd) {
	var known []claude.ProjectRef
	if m.snap != nil {
		known = m.snap.Projects
	}
	m.picker = &picker{all: m.discover(known)}
	if m.host == "herdr" && m.herdr == nil {
		m.picker.status = i18n.T("herdr is not connected, so agents cannot be started there.")
	}
	return m, nil
}

// choices are the projects matching the filter; with no filter only the
// recent ones, when there are any. A typed absolute path is offered first,
// and a typed folder name no project has is offered last, next to the most
// recent project: as it is when the folder exists, or to be created.
func (p *picker) choices() []choice {
	var list []choice
	typed := strings.TrimSpace(p.filter)
	if filepath.IsAbs(typed) {
		if c, ok := typedChoice(typed); ok {
			list = append(list, c)
		}
	}
	matches := projects.Filter(p.all, p.filter)
	if typed == "" && len(p.all) > 0 && p.all[0].Recent {
		matches = nil
		for _, proj := range p.all {
			if proj.Recent {
				matches = append(matches, proj)
			}
		}
	}
	for _, proj := range matches {
		list = append(list, choice{Project: proj})
	}
	if dir, ok := p.newFolder(typed); ok {
		if c, ok := typedChoice(dir); ok {
			list = append(list, c)
		}
	}
	return list
}

// typedChoice offers a typed folder: as it is, or to be created.
func typedChoice(path string) (choice, bool) {
	typed := projects.Project{Path: path, Name: filepath.Base(path), Group: filepath.Base(filepath.Dir(path))}
	info, err := os.Stat(path)
	switch {
	case err == nil && info.IsDir():
		return choice{Project: typed}, true
	case os.IsNotExist(err):
		return choice{Project: typed, create: true}, true
	}
	return choice{}, false
}

// newFolder is where a typed folder name goes: next to the most recent
// project. A name that is a path, or that a listed project already has, is
// not a new folder.
func (p *picker) newFolder(name string) (string, bool) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\:`) || len(p.all) == 0 {
		return "", false
	}
	for _, proj := range p.all {
		if strings.EqualFold(proj.Name, name) {
			return "", false
		}
	}
	return filepath.Join(filepath.Dir(p.all[0].Path), name), true
}

func (m Model) updatePicker(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	p := m.picker
	list := p.choices()
	switch msg.String() {
	case "esc", "ctrl+q":
		m.picker = nil
		return m, nil
	case "up", "ctrl+p":
		p.sel = max(0, p.sel-1)
		return m, nil
	case "down", "ctrl+n":
		p.sel = min(max(0, len(list)-1), p.sel+1)
		return m, nil
	case "backspace":
		if r := []rune(p.filter); len(r) > 0 {
			p.filter, p.sel = string(r[:len(r)-1]), 0
		}
		return m, nil
	case "enter", "ctrl+t":
		if len(list) == 0 {
			p.status = i18n.T("No folder matches. Type a folder name or an absolute path to open or create one.")
			return m, nil
		}
		c := list[min(p.sel, len(list)-1)]
		if c.create {
			if err := os.MkdirAll(c.Path, 0o755); err != nil {
				p.status = i18n.Tf("Could not create the folder: %s", err)
				return m, nil
			}
		}
		if msg.String() == "ctrl+t" {
			m.picker = nil
			return m.openFolder(c.Path)
		}
		return m.startAgent(c.Project)
	}
	if text, ok := keyText(msg); ok {
		return m.pickerPaste(text), nil
	}
	return m, nil
}

// pickerPaste adds typed or pasted text to the filter, on one line.
func (m Model) pickerPaste(text string) Model {
	p := m.picker
	p.filter += strings.NewReplacer("\r", "", "\n", "").Replace(text)
	p.sel = 0
	return m
}

// startAgent starts an agent in the project: inside the office, or in herdr
// with -host herdr.
func (m Model) startAgent(proj projects.Project) (Model, tea.Cmd) {
	if m.host != "herdr" {
		return m.startLocal(proj.Path)
	}
	if m.herdr == nil {
		return m, nil
	}
	h, argv, panes := m.herdr, m.agentCmd, m.panes
	m.picker.status = i18n.Tf("Starting %s in %s…", strings.Join(argv, " "), proj.Name)
	return m, func() tea.Msg {
		pane, err := h.StartAgent(proj.Path, argv, panes)
		return createdMsg{name: proj.Name, pane: pane, err: err}
	}
}

func (m Model) created(msg createdMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		if m.picker != nil {
			m.picker.status = i18n.Tf("Could not start: %s", msg.err)
		}
		return m, nil
	}
	m.picker = nil
	m.notice = i18n.Tf("Started %s in %s (%s); it walks in as soon as herdr reports it.", m.agentCmd[0], msg.name, msg.pane)
	return m, m.herdrPoll()
}

// pickerFrame is where the picker sits on the floor, and the first and the
// number of its list rows it shows; the list scrolls around the selection.
func (m Model) pickerFrame() (x, y, w, h, first, rows int) {
	fw, fh := m.floorW(), m.floorH()
	w, h = min(max(fw-8, 20), 90), min(max(fh-4, 8), 26)
	w, h = min(w, fw), min(h, fh)
	rows = h - 4
	first = max(0, min(m.picker.sel-rows/2, len(m.picker.choices())-rows))
	return (fw - w) / 2, (fh - h) / 2, w, h, first, rows
}

// pickerMouse handles the mouse over the picker: the wheel moves the
// selection, a click selects a folder and a click on the selected one
// starts it like enter. A click outside closes the picker.
func (m Model) pickerMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	p := m.picker
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			return m.updatePicker(tea.KeyPressMsg{Code: tea.KeyUp})
		case tea.MouseWheelDown:
			return m.updatePicker(tea.KeyPressMsg{Code: tea.KeyDown})
		}
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		fx, fy := msg.X-1, msg.Y-officeY
		x, y, w, h, first, rows := m.pickerFrame()
		if fx < x || fx >= x+w || fy < y || fy >= y+h {
			m.picker = nil
			return m, nil
		}
		i := first + fy - (y + 3)
		if fy < y+3 || fy >= y+3+rows || i >= len(p.choices()) {
			return m, nil
		}
		if i == p.sel {
			return m.updatePicker(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		p.sel = i
	}
	return m, nil
}

// drawPicker paints the project list as a modal.
func (m Model) drawPicker(buf *screen.Buffer) {
	x, y, w, h, first, rows := m.pickerFrame()
	p := m.picker
	footer := i18n.Tf(" ↑↓ choose · enter start %s · ctrl+t terminal · esc close ", m.agentCmd[0])
	if p.status != "" {
		footer = " " + p.status + " "
	}
	box(buf, x, y, w, h, i18n.T(" new agent or terminal "), footer)

	inner := w - 4
	end := buf.Text(x+2, y+1, "> ", modalBorder())
	end = buf.Text(end, y+1, ansi.Truncate(p.filter, max(0, inner-3), "…"), modalText())
	cursor := modalText()
	cursor.Reverse = true
	buf.Text(end, y+1, " ", cursor)
	if p.filter == "" {
		buf.Text(end+2, y+1, ansi.Truncate(i18n.T("type to filter, or a new folder name or path"), max(0, inner-5), "…"), modalDim())
	}

	list := p.choices()
	if len(list) == 0 {
		buf.Text(x+2, y+3, i18n.T("No folder matches."), modalDim())
		return
	}
	nameW := min(28, inner/2)
	// Folders with the same name and group show their whole parent folder.
	same := map[string]int{}
	for _, c := range list {
		same[strings.ToLower(c.Name+"\x00"+c.Group)]++
	}
	for i := 0; i < rows && first+i < len(list); i++ {
		c := list[first+i]
		where := c.Group
		if same[strings.ToLower(c.Name+"\x00"+c.Group)] > 1 {
			where = filepath.Dir(c.Path)
		}
		line := fmt.Sprintf("%-*s %s", nameW, ansi.Truncate(c.Name, nameW, "…"), where)
		switch {
		case c.create:
			line = i18n.Tf("create %s in %s", c.Name, filepath.Dir(c.Path))
		case c.Recent:
			line += i18n.T(" · recent")
		}
		st := modalText()
		if first+i == p.sel {
			st.Reverse = true
		}
		buf.Text(x+2, y+3+i, ansi.Truncate(line, inner, "…"), st)
	}
}

// keyText returns the literal text a key types, if any.
func keyText(msg tea.KeyPressMsg) (string, bool) {
	if msg.Text == "" || msg.Mod&(tea.ModCtrl|tea.ModAlt) != 0 {
		return "", false
	}
	return msg.Text, true
}
