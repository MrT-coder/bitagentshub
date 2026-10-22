package ui

import (
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MrT-coder/bitagentshub/internal/board"
	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/herdr"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/screen"
	"github.com/MrT-coder/bitagentshub/internal/term"
)

// Herdr is the subset of the herdr client the board uses.
type Herdr interface {
	ListAgents() ([]herdr.Pane, error)
	Focus(target string) error
	StartAgent(dir string, argv []string, existing []herdr.Pane) (string, error)
	ClosePane(paneID string) error
}

// Terminal is a program running in a pseudo terminal; term.Session is the
// real one.
type Terminal interface {
	Key(msg tea.KeyPressMsg)
	Paste(text string)
	Draw(buf *screen.Buffer, x, y, w, h, scroll int, base screen.Style)
	Resize(w, h int)
	Size() (w, h int)
	ScrollbackLen() int
	Exited() bool
	Close()
	AltScreen() bool
	Mouse(kind term.MouseKind, x, y int)
}

type startTermFunc func(id, dir string, argv []string, w, h int, notify chan<- string) (Terminal, error)

func startPTY(id, dir string, argv []string, w, h int, notify chan<- string) (Terminal, error) {
	s, err := term.Start(id, dir, argv, w, h, notify)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// defaultShell is PowerShell 7 when installed, then the platform shell.
func defaultShell() []string {
	if runtime.GOOS == "windows" {
		if p := findPwsh(os.Getenv("PATH")); p != "" {
			return []string{p, "-NoLogo"}
		}
		return []string{"powershell.exe", "-NoLogo"}
	}
	if p, err := exec.LookPath("pwsh"); err == nil {
		return []string{p, "-NoLogo"}
	}
	if sh := os.Getenv("SHELL"); sh != "" {
		return []string{sh}
	}
	return []string{"/bin/sh"}
}

// findPwsh returns the first pwsh.exe on path that a pseudo terminal can
// start. The Microsoft Store install is skipped: its alias and its package
// folder both live under a WindowsApps folder, and creating a process from
// either through ConPTY fails with "Access is denied". The Store PowerShell
// also puts its package folder first on PATH, so a plain lookup finds it.
func findPwsh(path string) string {
	for _, dir := range filepath.SplitList(path) {
		if dir == "" || underWindowsApps(dir) {
			continue
		}
		p := filepath.Join(dir, "pwsh.exe")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return ""
}

func underWindowsApps(dir string) bool {
	for _, part := range strings.FieldsFunc(dir, func(r rune) bool { return r == '\\' || r == '/' }) {
		if strings.EqualFold(part, "WindowsApps") {
			return true
		}
	}
	return false
}

const (
	herdrEvery  = 2 * time.Second
	wheelStep   = 3
	frameTime   = 16 * time.Millisecond
	attachPrefx = "attach:"
	shellPrefix = "shell:"
)

type (
	herdrMsg struct {
		panes []herdr.Pane
		err   error
	}
	herdrTickMsg struct{}
	termMsg      string // a terminal's screen changed
	focusMsg     struct{ err error }
	movedMsg     struct {
		agentID, dir string
		err          error
	}
)

// hosted is a terminal the office runs: a local agent, a plain terminal,
// or a herdr attach that only lives while its window is open.
type hosted struct {
	term    Terminal
	dir     string
	attach  bool
	shell   bool
	project string
	group   string
	kept    bool   // runs in the daemon and outlives the office
	kind    string // the agent program, "claude" or "codex"; "" for terminals
}

// pane is one terminal in the window. key indexes Model.terms; an agent
// pane with nothing running has an empty key and shows a message instead.
type pane struct {
	key    string
	shell  bool
	scroll int // lines scrolled up into the history
}

// window is the open modal: the panes of one folder, side by side.
type window struct {
	dir         string
	agentID     string // "" for a window of terminals only
	panes       []*pane
	focus       int
	max         bool
	status      string
	confirmMove bool // m was pressed once on a herdr agent
	sel         *selection
	branch      string // checked out in dir; refreshed with each poll
}

func (w *window) focused() *pane { return w.panes[w.focus] }

// selection is text marked with the left button in one pane, in that pane's
// cells. It stays highlighted until the next click, wheel, key or paste.
type selection struct {
	pane           int
	ax, ay, bx, by int  // where the drag started and where it is now
	moved          bool // the mouse left the start cell: a drag, not a click
	copied         int  // characters put on the clipboard once released
}

// span returns the selection's ends in reading order.
func (s *selection) span() (sx, sy, ex, ey int) {
	if s.by < s.ay || (s.by == s.ay && s.bx < s.ax) {
		return s.bx, s.by, s.ax, s.ay
	}
	return s.ax, s.ay, s.bx, s.by
}

// rows calls f with the marked cells x0 to x1-1 of each selected row of a
// pane w cells wide: whole rows between the ends, like a terminal selects.
func (s *selection) rows(w int, f func(y, x0, x1 int)) {
	sx, sy, ex, ey := s.span()
	for y := sy; y <= ey; y++ {
		x0, x1 := 0, w
		if y == sy {
			x0 = sx
		}
		if y == ey {
			x1 = ex + 1
		}
		f(y, x0, x1)
	}
}

type rect struct{ x, y, w, h int }

func folderKey(dir string) string { return strings.ToLower(filepath.Clean(dir)) }

func (m Model) herdrPoll() tea.Cmd {
	if m.herdr == nil {
		return nil
	}
	h := m.herdr
	return func() tea.Msg {
		panes, err := h.ListAgents()
		return herdrMsg{panes: panes, err: err}
	}
}

// waitTerm wakes the UI when a terminal draws. Output arrives in many small
// chunks while a program streams, so notifications are gathered for one
// frame (~16 ms) and the burst costs a single repaint.
func (m Model) waitTerm() tea.Cmd {
	ch := m.termNotify
	return func() tea.Msg {
		id := <-ch
		time.Sleep(frameTime)
		for {
			select {
			case <-ch:
			default:
				return termMsg(id)
			}
		}
	}
}

func (m Model) paneOf(agentID string) (herdr.Pane, bool) {
	p, ok := m.links[agentID]
	return p, ok
}

func (m Model) agent(agentID string) (claude.Agent, bool) {
	for _, a := range m.agents() {
		if a.ID == agentID {
			return a, true
		}
	}
	return claude.Agent{}, false
}

func (m Model) projectOf(agentID string) string {
	if a, ok := m.agent(agentID); ok {
		return a.Project
	}
	return agentID
}

// openWindow shows an agent with the terminals of its folder.
func (m Model) openWindow(agentID string) (Model, tea.Cmd) {
	m.selected = agentID
	m.stripFirst = -1 // the strip follows the station just opened
	delete(m.unseen, agentID)
	a, _ := m.agent(agentID)
	m.win = &window{dir: a.Cwd, agentID: agentID, branch: gitBranch(a.Cwd)}
	ap := &pane{}
	m.win.panes = []*pane{ap}
	m.addShellPanes()
	w, h := m.termSize(0)
	switch hs, ok := m.terms[agentID]; {
	case ok && !hs.attach:
		ap.key = agentID
	case m.hasPane(agentID):
		p, _ := m.paneOf(agentID)
		if !m.canAttach {
			m.win.status = i18n.T("In herdr (no attach on Windows): g jumps to its herdr pane, m moves it here.")
			break
		}
		key := attachPrefx + agentID
		t, err := m.startTerm(key, a.Cwd, []string{"herdr", "agent", "attach", p.Target()}, w, h, m.termNotify)
		if err != nil {
			m.win.status = i18n.Tf("Could not attach to herdr: %s", err)
			break
		}
		m.terms[key] = &hosted{term: t, attach: true}
		ap.key = key
	default:
		m.win.status = i18n.T("This agent is no longer running.")
	}
	m.resizePanes()
	return m, nil
}

func (m Model) hasPane(agentID string) bool {
	_, ok := m.paneOf(agentID)
	return ok
}

// openFolder shows a window of plain terminals for dir, starting one when
// the folder has none yet.
func (m Model) openFolder(dir string) (Model, tea.Cmd) {
	m.win = &window{dir: dir, branch: gitBranch(dir)}
	m.addShellPanes()
	if len(m.win.panes) == 0 {
		return m.newShell()
	}
	m.resizePanes()
	return m, nil
}

// addShellPanes adds the folder's running terminals to the window.
func (m Model) addShellPanes() {
	for _, key := range m.shells[folderKey(m.win.dir)] {
		m.win.panes = append(m.win.panes, &pane{key: key, shell: true})
	}
}

// newShell starts a plain terminal in the window's folder, beside the
// other panes, and focuses it.
func (m Model) newShell() (Model, tea.Cmd) {
	return m.newPane(m.shellCmd, i18n.T("Could not open a terminal: "))
}

// openBoard opens the window folder's task note for today, creating it with
// the unfinished tasks of the last one, in the board's editor beside the
// other panes. It is one of the folder's terminals until the editor exits.
func (m Model) openBoard() (Model, tea.Cmd) {
	if m.board.Dir == "" {
		m.win.status = i18n.T("No task board: set [board] dir in config.toml.")
		return m, nil
	}
	note, err := board.Today(m.board.Dir, filepath.Base(m.win.dir), time.Now())
	if err != nil {
		m.win.status = i18n.T("Could not open the board: ") + err.Error()
		return m, nil
	}
	return m.newPane([]string{m.board.Editor, note}, i18n.T("Could not open the board: "))
}

// newPane runs argv in the window's folder as a terminal beside the other
// panes and focuses it; failing, the window shows fail and the error.
func (m Model) newPane(argv []string, fail string) (Model, tea.Cmd) {
	key := shellPrefix + newUUID()
	m.win.panes = append(m.win.panes, &pane{key: key, shell: true})
	w, h := m.termSize(len(m.win.panes) - 1)
	t, kept, err := m.startHosted(key, m.win.dir, argv, w, h)
	if err != nil {
		m.win.panes = m.win.panes[:len(m.win.panes)-1]
		m.win.status = fail + err.Error()
		if len(m.win.panes) == 0 {
			m.win = nil
		}
		return m, nil
	}
	m.terms[key] = &hosted{term: t, dir: m.win.dir, shell: true, kept: kept}
	fk := folderKey(m.win.dir)
	m.shells[fk] = append(m.shells[fk], key)
	m.win.focus = len(m.win.panes) - 1
	m.resizePanes()
	return m, nil
}

// closeShell ends the focused plain terminal and removes its pane.
func (m Model) closeShell() Model {
	p := m.win.focused()
	if !p.shell {
		return m
	}
	if hs, ok := m.terms[p.key]; ok {
		hs.term.Close()
		delete(m.terms, p.key)
	}
	m.forgetShell(p.key)
	return m.removePane(m.win.focus)
}

// removePane takes pane i out of the window and closes the window when no
// pane is left. Without its agent pane the window holds terminals only.
func (m Model) removePane(i int) Model {
	w := m.win
	if !w.panes[i].shell {
		w.agentID = ""
	}
	w.panes = append(w.panes[:i], w.panes[i+1:]...)
	w.sel, w.confirmMove = nil, false
	if len(w.panes) == 0 {
		m.win = nil
		return m
	}
	if w.focus > i {
		w.focus--
	}
	w.focus = min(w.focus, len(w.panes)-1)
	m.resizePanes()
	return m
}

// endPane clears a program that ended (exit, ctrl+d, a crash) without
// waiting for a key: its pane leaves the window, and a terminal or a herdr
// attach is released. A local agent then leaves the office in dropEnded.
func (m Model) endPane(key string) Model {
	hs, ok := m.terms[key]
	if !ok || !hs.term.Exited() {
		return m
	}
	if hs.shell || hs.attach {
		hs.term.Close()
		delete(m.terms, key)
		if hs.shell {
			m.forgetShell(key)
		}
	}
	if m.win != nil {
		for i, p := range m.win.panes {
			if p.key == key {
				return m.removePane(i)
			}
		}
	}
	return m
}

// closeWindow hides the window. Local agents and terminals keep running; a
// herdr attach ends, since herdr keeps the agent alive; ended programs are
// cleared.
func (m Model) closeWindow() Model {
	for _, p := range m.win.panes {
		if hs, ok := m.terms[p.key]; ok && (hs.attach || hs.term.Exited()) {
			hs.term.Close()
			delete(m.terms, p.key)
			if p.shell {
				m.forgetShell(p.key)
			}
		}
	}
	m.win = nil
	return m
}

func (m Model) forgetShell(key string) {
	for fk, keys := range m.shells {
		out := keys[:0]
		for _, k := range keys {
			if k != key {
				out = append(out, k)
			}
		}
		m.shells[fk] = out
	}
}

// updateWindow routes keys: keys bound without the prefix (F-keys by
// default) belong to the window; everything else goes to the focused pane.
func (m Model) updateWindow(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	w := m.win
	w.sel = nil
	p := w.focused()
	if _, running := m.terms[p.key]; msg.String() != "m" || running {
		// Moving needs two consecutive m presses on the agent pane; any
		// other key, F-keys and keys typed into terminals included, cancels.
		w.confirmMove = false
	}
	k := msg.String()
	// ctrl+q in a plain terminal is the program's (nvim's visual block in
	// Windows Terminal); on the agent pane it closes the window.
	if action, n, ok := m.keys.Action(k, false); ok && (k != "ctrl+q" || !p.shell) {
		return m.runAction(action, n)
	}
	return m.typeKey(msg)
}

// typeKey sends a key to the focused pane's program.
func (m Model) typeKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	w := m.win
	p := w.focused()
	hs, ok := m.terms[p.key]
	if !ok {
		return m.agentPaneKey(msg)
	}
	if hs.term.Exited() {
		w.status = i18n.Tf("The program ended. %s closes the window.", m.keyHint(config.CloseWindow))
		return m, nil
	}
	p.scroll = 0
	locks := tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock
	if !hs.shell && msg.Code == tea.KeyEnter && msg.Mod&^locks == tea.ModShift {
		// Agents read alt+enter as a new line; Claude Code's /terminal-setup
		// binds shift+enter to it the same way.
		msg.Mod = msg.Mod&locks | tea.ModAlt
	}
	hs.term.Key(msg)
	return m, nil
}

// windowPaste types a paste into the focused pane's program as one bracketed
// paste, so its line breaks reach the program as text, not as enter.
func (m Model) windowPaste(text string) Model {
	w := m.win
	w.confirmMove, w.sel = false, nil
	hs, ok := m.terms[w.focused().key]
	if !ok || hs.term.Exited() {
		return m
	}
	w.focused().scroll = 0
	hs.term.Paste(text)
	return m
}

// agentPaneKey handles keys on an agent pane with nothing running in it.
func (m Model) agentPaneKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	w := m.win
	inHerdr := m.hasPane(w.agentID)
	k := msg.String()
	switch {
	case k == "g" && inHerdr:
		return m.focusPane(w.agentID)
	case k == "m" && inHerdr:
		return m.moveHere()
	}
	return m, nil
}

// moveHere closes an idle herdr agent and continues in the office: a fresh
// local agent in the same folder starts a new conversation there.
func (m Model) moveHere() (Model, tea.Cmd) {
	w := m.win
	if m.closing[w.agentID] {
		return m, nil // the close is on its way; never send a second one
	}
	a, _ := m.agent(w.agentID)
	p, _ := m.paneOf(w.agentID)
	// herdr's own status can be ahead of the transcript, so both must agree
	// that the agent waits.
	if a.State.Working() || a.Blocked || p.Status == "working" || p.Status == "blocked" {
		w.status = i18n.Tf("%s is working; move it once it waits for you.", a.Project)
		w.confirmMove = false
		return m, nil
	}
	if !w.confirmMove {
		w.confirmMove = true
		w.status = i18n.T("Press m again to close it in herdr and start a new agent here (a new conversation).")
		return m, nil
	}
	w.confirmMove = false
	m.closing[w.agentID] = true
	h, id, dir := m.herdr, w.agentID, a.Cwd
	w.status = i18n.T("Closing it in herdr…")
	return m, func() tea.Msg { return movedMsg{agentID: id, dir: dir, err: h.ClosePane(p.PaneID)} }
}

func (m Model) moved(msg movedMsg) (Model, tea.Cmd) {
	delete(m.closing, msg.agentID)
	if msg.err != nil {
		if m.win != nil {
			m.win.status = i18n.Tf("Could not close it in herdr: %s", msg.err)
		}
		return m, nil
	}
	m.retired[msg.agentID] = true
	m.relink()
	return m.startLocal(msg.dir)
}

// windowMouse handles the mouse over the window. A click focuses the pane
// under it. Dragging with the left button marks text in that pane and copies
// it to the clipboard on release, in every pane. Full-screen programs (Claude
// Code switches to the alternate screen and tracks the mouse) get the wheel
// and left clicks that did not drag; otherwise the wheel scrolls the history
// the office keeps.
func (m Model) windowMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	mouse := msg.Mouse()
	_, press := msg.(tea.MouseClickMsg)
	_, motion := msg.(tea.MouseMotionMsg)
	_, release := msg.(tea.MouseReleaseMsg)
	_, wheel := msg.(tea.MouseWheelMsg)
	// Screen to floor coordinates: the left wall and the rows above the floor.
	fx, fy := mouse.X-1, mouse.Y-officeY
	rects := m.paneRects()
	if s := m.win.sel; s != nil && s.copied == 0 && (motion || release) && s.pane < len(rects) {
		return m.dragTo(s, fx-rects[s.pane].x, fy-rects[s.pane].y, release)
	}
	if press || wheel {
		m.win.sel = nil
	}
	for i, r := range rects {
		x, y := fx-r.x, fy-r.y
		if x < 0 || y < 0 || x >= r.w || y >= r.h {
			continue
		}
		p := m.win.panes[i]
		if press && mouse.Button == tea.MouseLeft {
			m.win.focus = i
		}
		hs, ok := m.terms[p.key]
		if !ok {
			return m, nil
		}
		if press && mouse.Button == tea.MouseLeft {
			// Whether this is a click for the program or a drag that selects
			// is only known once the mouse moves or the button is released.
			m.win.sel = &selection{pane: i, ax: x, ay: y, bx: x, by: y}
			return m, nil
		}
		wheelUp, wheelDown := wheel && mouse.Button == tea.MouseWheelUp, wheel && mouse.Button == tea.MouseWheelDown
		if !hs.term.AltScreen() {
			switch {
			case wheelUp:
				p.scroll = min(p.scroll+wheelStep, hs.term.ScrollbackLen())
			case wheelDown:
				p.scroll = max(p.scroll-wheelStep, 0)
			}
			return m, nil
		}
		if hs.term.Exited() {
			return m, nil
		}
		switch {
		case wheelUp:
			hs.term.Mouse(term.MouseWheelUp, x, y)
		case wheelDown:
			hs.term.Mouse(term.MouseWheelDown, x, y)
		}
		return m, nil
	}
	return m, nil
}

// dragTo moves the end of a selection to pane cell (x, y), kept inside the
// pane. On release a drag copies the marked text; a press and release on
// one cell was a click, which full-screen programs receive.
func (m Model) dragTo(s *selection, x, y int, release bool) (Model, tea.Cmd) {
	r := m.paneRects()[s.pane]
	s.bx, s.by = min(max(x, 0), r.w-1), min(max(y, 0), r.h-1)
	if s.bx != s.ax || s.by != s.ay {
		s.moved = true
	}
	if !release {
		return m, nil
	}
	if !s.moved {
		m.win.sel = nil
		if hs, ok := m.terms[m.win.panes[s.pane].key]; ok && hs.term.AltScreen() && !hs.term.Exited() {
			hs.term.Mouse(term.MouseLeftPress, s.ax, s.ay)
			hs.term.Mouse(term.MouseLeftRelease, s.ax, s.ay)
		}
		return m, nil
	}
	text := m.selectedText(s)
	if text == "" {
		m.win.sel = nil
		return m, nil
	}
	s.copied = utf8.RuneCountInString(text)
	return m, tea.SetClipboard(text)
}

// selectedText reads the marked cells from what the pane shows, so it works
// the same for terminals the office runs and those the daemon keeps. Rows
// lose their trailing blanks and join with line breaks.
func (m Model) selectedText(s *selection) string {
	r := m.paneRects()[s.pane]
	p := m.win.panes[s.pane]
	hs, ok := m.terms[p.key]
	if !ok {
		return ""
	}
	buf := screen.New(r.w, r.h)
	hs.term.Draw(buf, 0, 0, r.w, r.h, p.scroll, modalText)
	var lines []string
	s.rows(r.w, func(y, x0, x1 int) {
		lines = append(lines, strings.TrimRight(buf.Row(y, x0, x1), " "))
	})
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// startLocal runs a new agent in dir inside the office and opens it.
// Claude gets a fresh session id so its transcript links to this terminal.
func (m Model) startLocal(dir string) (Model, tea.Cmd) {
	id := newUUID()
	argv := append([]string(nil), m.agentCmd...)
	if isClaude(argv) {
		argv = append(argv, "--session-id", id)
		if m.board.Dir != "" {
			argv = append(argv, "--add-dir", m.board.Dir, "--append-system-prompt", boardPrompt(m.board.Dir, filepath.Base(dir)))
		}
	}
	w, h := m.termSize(0)
	t, kept, err := m.startHosted(id, dir, argv, w, h)
	if err != nil {
		switch msg := i18n.Tf("Could not start: %s", err); {
		case m.picker != nil:
			m.picker.status = msg
		case m.win != nil:
			m.win.status = msg
		default:
			m.notice = msg
		}
		return m, nil
	}
	m.terms[id] = &hosted{term: t, dir: dir, project: filepath.Base(dir), group: filepath.Base(filepath.Dir(dir)), kept: kept, kind: agentKind(argv)}
	m.picker = nil
	m.relayout()
	return m.openWindow(id)
}

// boardPrompt tells an agent where its project's task notes are and how to
// check off what it finishes.
func boardPrompt(dir, project string) string {
	return "The user's task board for this project is the folder " + filepath.Join(dir, project) +
		", one markdown note per day named YYYY-MM-DD.md; the note named with today's date is the current one. " +
		"When you finish a task listed in it, change that task's \"- [ ]\" to \"- [x]\". " +
		"Do not add, remove or reword tasks unless the user asks you to."
}

// localAgents are the office's own agents that have no transcript yet.
func (m Model) localAgents(known map[string]bool) []claude.Agent {
	var out []claude.Agent
	for id, hs := range m.terms {
		if hs.attach || hs.shell || known[id] {
			continue
		}
		out = append(out, claude.Agent{ID: id, Project: hs.project, Group: hs.group, Cwd: hs.dir, State: claude.StateWaiting})
	}
	return out
}

// runningLocal counts local agents and terminals that end with the office:
// their program still runs and no daemon keeps it.
func (m Model) runningLocal() int {
	n := 0
	for _, hs := range m.terms {
		if !hs.attach && !hs.kept && !hs.term.Exited() {
			n++
		}
	}
	return n
}

// CloseAll ends every terminal the office runs itself and leaves the
// daemon's running. Call it after the program exits.
func (m Model) CloseAll() {
	for _, hs := range m.terms {
		if d, ok := hs.term.(interface{ Detach() }); ok && hs.kept {
			d.Detach()
			continue
		}
		hs.term.Close()
	}
	if m.daemon != nil {
		m.daemon.Close()
	}
}

func isClaude(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	name := strings.ToLower(filepath.Base(argv[0]))
	return strings.TrimSuffix(name, ".exe") == "claude"
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (m Model) focusPane(agentID string) (Model, tea.Cmd) {
	p, ok := m.paneOf(agentID)
	if !ok || m.herdr == nil {
		m.notice = i18n.Tf("%s is not running in a herdr pane.", m.projectOf(agentID))
		return m, nil
	}
	h := m.herdr
	return m, func() tea.Msg { return focusMsg{err: h.Focus(p.Target())} }
}

var (
	modalBg     = screen.RGB{R: 0x16, G: 0x18, B: 0x1d}
	modalText   = screen.Fg(screen.RGB{R: 0xdc, G: 0xdf, B: 0xe4}).WithBg(modalBg)
	modalDim    = office.Dim.WithBg(modalBg)
	modalBorder = screen.Fg(screen.RGB{R: 0x61, G: 0xaf, B: 0xef}).WithBg(modalBg)
)

// modalRect is the window: centered with office visible around it, or the
// whole floor when maximized.
func (m Model) modalRect() (x, y, w, h int) {
	fw, fh := m.floorW(), m.floorH()
	if m.win != nil && m.win.max {
		return 0, 0, fw, fh
	}
	w = min(max(fw-8, 20), fw)
	h = min(max(fh-2, 8), fh)
	return (fw - w) / 2, (fh - h) / 2, w, h
}

// paneRects splits the window's inside into side-by-side panes, one column
// apart, in floor coordinates. Maximized, the focused pane fills the window
// and the others get an empty rect: hidden, they keep their size.
func (m Model) paneRects() []rect {
	x, y, w, h := m.modalRect()
	n := 1
	if m.win != nil {
		n = max(1, len(m.win.panes))
	}
	rects := make([]rect, n)
	if m.win != nil && m.win.max {
		rects[m.win.focus] = rect{x + 1, y + 1, max(1, w-2), max(1, h-2)}
		return rects
	}
	inner := max(n, w-2-(n-1))
	cx := x + 1
	for i := range rects {
		pw := inner / n
		if i == n-1 {
			pw = inner - (n-1)*(inner/n)
		}
		rects[i] = rect{cx, y + 1, max(1, pw), max(1, h-2)}
		cx += pw + 1
	}
	return rects
}

// termSize is the size of pane i once laid out.
func (m Model) termSize(i int) (w, h int) {
	rects := m.paneRects()
	r := rects[min(i, len(rects)-1)]
	if r.w == 0 { // hidden behind the maximized pane
		r = rects[m.win.focus]
	}
	return r.w, r.h
}

// resizePanes tells every shown terminal in the window its pane's size.
func (m Model) resizePanes() {
	if m.win == nil {
		return
	}
	for i, r := range m.paneRects() {
		if hs, ok := m.terms[m.win.panes[i].key]; ok && r.w > 0 {
			hs.term.Resize(r.w, r.h)
		}
	}
}

// box paints a bordered modal with a title and a footer hint.
func box(buf *screen.Buffer, x, y, w, h int, title, footer string) {
	buf.Fill(x, y, w, h, ' ', modalText)
	for i := 1; i < w-1; i++ {
		buf.Text(x+i, y, "─", modalBorder)
		buf.Text(x+i, y+h-1, "─", modalBorder)
	}
	for j := 1; j < h-1; j++ {
		buf.Text(x, y+j, "│", modalBorder)
		buf.Text(x+w-1, y+j, "│", modalBorder)
	}
	buf.Text(x, y, "┌", modalBorder)
	buf.Text(x+w-1, y, "┐", modalBorder)
	buf.Text(x, y+h-1, "└", modalBorder)
	buf.Text(x+w-1, y+h-1, "┘", modalBorder)
	if title != "" {
		titleStyle := modalText
		titleStyle.Bold = true
		buf.Text(x+2, y, ansi.Truncate(title, max(0, w-4), "…"), titleStyle)
	}
	buf.Text(x+2, y+h-1, ansi.Truncate(footer, max(0, w-4), "…"), modalDim)
}

// paneTitle names a pane in the window's top border: the station number
// (when the window is a station, 1-9), the folder and the branch checked out
// there, the same for every pane of the window.
func (m Model) paneTitle() string {
	title := filepath.Base(m.win.dir)
	if b := m.win.branch; b != "" {
		title += " ~ " + b
	}
	for i, id := range office.Order(m.layout) {
		if id == m.win.agentID && m.win.agentID != "" && i < 9 {
			title = fmt.Sprintf("%d %s", i+1, title)
		}
	}
	return " " + title + " "
}

// drawWindow paints the window: one bordered box, panes side by side.
func (m Model) drawWindow(buf *screen.Buffer) {
	x, y, w, h := m.modalRect()
	win := m.win
	footer := " " + m.windowHints() + " "
	if p := win.focused(); p.scroll > 0 {
		footer = i18n.Tf(" scrolled up %d lines · wheel down or type to go back ", p.scroll)
	}
	if win.status != "" {
		footer = " " + win.status + " "
	}
	if m.prefixed {
		footer = i18n.Tf(" %s … waiting for a key (%s again sends it to the program) ", m.keys.Prefix, m.keys.Prefix)
	}
	if s := win.sel; s != nil && s.copied > 0 {
		footer = i18n.Tf(" Copied %d characters · click or type to clear ", s.copied)
	}
	box(buf, x, y, w, h, "", footer)
	titleW := func(r rect) int { return max(0, r.w-2) }
	if win.max {
		// A maximized pane is marked in the top right corner, as in herdr.
		const mark = " z "
		buf.Text(x+w-2-len(mark), y, mark, modalBorder)
		titleW = func(r rect) int { return max(0, r.w-3-len(mark)) }
	}
	for i, r := range m.paneRects() {
		if r.w == 0 {
			continue // hidden behind the maximized pane
		}
		p := win.panes[i]
		titleStyle := modalDim
		if i == win.focus {
			titleStyle = modalBorder
			titleStyle.Reverse, titleStyle.Bold = true, true
		}
		buf.Text(r.x+1, y, ansi.Truncate(m.paneTitle(), titleW(r), "…"), titleStyle)
		if i > 0 && !win.max {
			for j := 0; j < r.h; j++ {
				buf.Text(r.x-1, r.y+j, "│", modalBorder)
			}
			buf.Text(r.x-1, y, "┬", modalBorder)
			buf.Text(r.x-1, y+h-1, "┴", modalBorder)
		}
		if hs, ok := m.terms[p.key]; ok {
			hs.term.Draw(buf, r.x, r.y, r.w, r.h, p.scroll, modalText)
			if s := win.sel; s != nil && s.pane == i && s.moved {
				s.rows(r.w, func(y, x0, x1 int) { buf.Invert(r.y+y, r.x+x0, r.x+x1) })
			}
			continue
		}
		lines := strings.Split(ansi.Wordwrap(win.status, max(10, r.w-4), " "), "\n")
		top := r.y + max(0, (r.h-len(lines))/2)
		for k, line := range lines {
			buf.Text(r.x+max(0, (r.w-ansi.StringWidth(line))/2), top+k, line, modalText)
		}
	}
}

// dropEnded removes a local agent whose program ended.
func (m *Model) dropEnded(key string) {
	hs, ok := m.terms[key]
	if !ok || hs.attach || hs.shell || !hs.term.Exited() {
		return
	}
	hs.term.Close()
	delete(m.terms, key)
	m.relayout()
}
