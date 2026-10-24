// Package ui shows the office board: the stations side by side on top, one
// room per workspace folder with its agents below, and windows with real
// terminals for each folder.
package ui

import (
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MrT-coder/bitagentshub/internal/board"
	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/daemon"
	"github.com/MrT-coder/bitagentshub/internal/herdr"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/projects"
	"github.com/MrT-coder/bitagentshub/internal/rest"
	"github.com/MrT-coder/bitagentshub/internal/screen"
	"github.com/MrT-coder/bitagentshub/internal/usage"
)

// Poller is the store boundary; Poll is only ever called from one goroutine
// at a time because the next poll is scheduled after the previous returns.
type Poller interface {
	Poll(now time.Time) claude.Snapshot
}

const (
	pollEvery = time.Second
	animEvery = 125 * time.Millisecond
	// Screen rows: title, gap, the three strip rows, the office's top wall,
	// the floor, its bottom wall and the footer.
	stripY  = 2 // screen row of the strip's first line
	officeY = 6 // screen row of the first floor line
	chromeH = 8 // every row that is not floor
)

type (
	snapshotMsg claude.Snapshot
	pollMsg     struct{}
	animMsg     struct{}
)

type Model struct {
	store       Poller
	herdr       Herdr
	snap        *claude.Snapshot
	layout      office.Layout
	crowd       *office.Crowd
	panes       []herdr.Pane
	links       map[string]herdr.Pane
	herdrErr    error
	herdrSeen   bool
	selected    string
	win         *window
	terms       map[string]*hosted // terminals the office runs, by agent id
	termNotify  chan string
	startTerm   startTermFunc
	hostTerm    startTermFunc // where agents and terminals run when not in the office: the daemon
	daemon      *daemon.Conn
	host        string              // where new agents run: "local" or "herdr"
	quitArmed   bool                // q was pressed once while local agents run
	canAttach   bool                // herdr supports direct attach here (not on Windows yet)
	shells      map[string][]string // plain terminals by folder
	shellCmd    []string
	retired     map[string]bool // sessions moved out of herdr
	closing     map[string]bool // herdr panes being closed, by agent id
	picker      *picker
	discover    func(known []claude.ProjectRef) []projects.Project
	agentCmd    []string       // what a new agent runs
	fresh       []claude.Agent // herdr agents with no transcript yet
	notice      string
	updated     time.Time
	frame       int
	width       int
	height      int
	animating   bool
	keys        config.Keymap
	prefixed    bool   // the prefix was pressed; the next key is a binding
	git         gitMsg // each station folder's branch and upstream distance
	sources     []usage.Source
	hubOpen     bool // the pet's hub is open on hubTab
	hubTab      int
	aboutScroll int // first changelog line the about tab shows
	usageScroll int // first row the usage tab shows
	floorScroll int // first row of the floor plan shown, when it does not fit
	changelog   string
	updateCheck func() string // nil when updates are not looked for
	newer       string        // a newer release's tag, once the check finds one
	petName     string        // "" until the pet is named
	renaming    bool          // the pet tab is taking a new name in petDraft
	petDraft    string
	petStatus   string         // what the pet tab says of the last rename
	hoverPet    bool           // the mouse rests on the pet
	reports     []usage.Report // what the sources said last
	usageAt     time.Time
	settings    Settings
	setWin      *settingsWin           // the open settings tab, nil when closed
	board       config.Board           // the task board; off when Dir is ""
	tasks       map[string]board.Count // today's tasks by project, nil before the first read
	stars       board.Stars            // tasks checked off on the board, in all and today
	cheers      map[string]time.Time   // until when each project's agents cheer
	eatUntil    time.Time              // the pet eats the star just checked off until then
	unseen      map[string]bool        // agents that worked while their window was closed
	stripFirst  int                    // first station shown after the user scrolled the strip; -1 follows the open one
	// The supervision clock, nil while [rest] is off; restPath is the day's
	// log, saved at most every restSaveEvery.
	rest      *rest.Clock
	restCfg   config.Rest
	restPath  string
	restSaved time.Time
	now       func() time.Time // the clock's time; tests move it
}

func New(store Poller) Model {
	return Model{
		store:      store,
		crowd:      office.NewCrowd(rand.New(rand.NewSource(time.Now().UnixNano()))),
		terms:      map[string]*hosted{},
		shells:     map[string][]string{},
		shellCmd:   defaultShell(),
		retired:    map[string]bool{},
		closing:    map[string]bool{},
		unseen:     map[string]bool{},
		stripFirst: -1,
		termNotify: make(chan string, 64),
		startTerm:  startPTY,
		now:        time.Now,
		host:       "local",
		canAttach:  runtime.GOOS != "windows",
		discover:   func(known []claude.ProjectRef) []projects.Project { return projects.Discover(known, nil) },
		agentCmd:   []string{"claude"},
		links:      map[string]herdr.Pane{},
		width:      44,
		height:     24,
		keys:       config.Default(),
	}
}

// WithKeys sets the keybindings read from the config file.
func (m Model) WithKeys(k config.Keymap) Model {
	m.keys = k
	return m
}

// WithAgentCmd sets what agents started from the office run.
func (m Model) WithAgentCmd(argv []string) Model {
	if len(argv) > 0 {
		m.agentCmd = argv
	}
	return m
}

// WithRoots adds folders whose subfolders are offered as projects.
func (m Model) WithRoots(roots []string) Model {
	m.discover = func(known []claude.ProjectRef) []projects.Project { return projects.Discover(known, roots) }
	return m
}

// WithHerdr enables linking agents to herdr panes for chat and jumping.
func (m Model) WithHerdr(h Herdr) Model {
	m.herdr = h
	return m
}

// WithBoard turns on the task board kept in b.Dir.
func (m Model) WithBoard(b config.Board) Model {
	m.board = b
	return m
}

// WithShell sets the command plain terminals run.
func (m Model) WithShell(argv []string) Model {
	if len(argv) > 0 {
		m.shellCmd = argv
	}
	return m
}

// WithHost chooses where agents started from the office run: "local" (a
// terminal inside the office, the default) or "herdr".
func (m Model) WithHost(host string) Model {
	m.host = host
	return m
}

// WithDaemon runs new agents and terminals in the daemon at addr, starting
// it when none answers, so they outlive the office; the ones it already
// runs come back. When the daemon cannot be used they run inside the office
// as before, and the footer says why.
func (m Model) WithDaemon(addr string) Model {
	err := daemon.Ensure(addr)
	var running []daemon.Info
	if err == nil {
		_, running, err = daemon.List(addr)
	}
	var conn *daemon.Conn
	if err == nil {
		conn, err = daemon.Attach(addr, m.termNotify)
	}
	switch {
	case errors.Is(err, daemon.ErrProto):
		m.notice = i18n.T("The running daemon is from another build; `bah daemon stop` replaces it (its programs end). Until then terminals end with the office.")
		return m
	case err != nil:
		m.notice = i18n.Tf("No daemon (%s); terminals end with the office.", err)
		return m
	}
	m.daemon = conn
	m.hostTerm = func(id, dir string, argv []string, w, h int, _ chan<- string) (Terminal, error) {
		t := conn.Terminal(id)
		if err := daemon.Start(addr, id, dir, argv, w, h); err != nil {
			t.Detach()
			return nil, err
		}
		return t, nil
	}
	for _, s := range running {
		shell := strings.HasPrefix(s.ID, shellPrefix)
		m.terms[s.ID] = &hosted{term: conn.Terminal(s.ID), dir: s.Dir, shell: shell, kept: true, kind: agentKind(s.Argv),
			project: filepath.Base(s.Dir), group: filepath.Base(filepath.Dir(s.Dir))}
		if shell {
			fk := folderKey(s.Dir)
			m.shells[fk] = append(m.shells[fk], s.ID)
		}
	}
	if len(running) > 0 {
		m.notice = i18n.Tf("Reattached %d agent(s) and terminal(s) kept by the daemon.", len(running))
	}
	return m
}

// startHosted runs an agent or terminal where it outlives the office when a
// daemon is in use; once the daemon is gone, inside the office.
func (m Model) startHosted(id, dir string, argv []string, w, h int) (Terminal, bool, error) {
	if m.hostTerm != nil && !m.daemon.Dead() {
		t, err := m.hostTerm(id, dir, argv, w, h, m.termNotify)
		return t, true, err
	}
	t, err := m.startTerm(id, dir, argv, w, h, m.termNotify)
	return t, false, err
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(win32Input(), m.poll(), m.herdrPoll(), m.waitTerm(), m.gitPoll(), m.boardPoll(), m.checkUpdate())
}

// modifierAlone reports a modifier or lock key pressed by itself, which
// win32-input-mode reports as a key; a terminal sends nothing for it.
func modifierAlone(code rune) bool {
	switch code {
	case tea.KeyLeftShift, tea.KeyLeftAlt, tea.KeyLeftCtrl, tea.KeyLeftSuper, tea.KeyLeftHyper, tea.KeyLeftMeta,
		tea.KeyRightShift, tea.KeyRightAlt, tea.KeyRightCtrl, tea.KeyRightSuper, tea.KeyRightHyper, tea.KeyRightMeta,
		tea.KeyIsoLevel3Shift, tea.KeyIsoLevel5Shift, tea.KeyCapsLock, tea.KeyNumLock, tea.KeyScrollLock:
		return true
	}
	return false
}

// Win32InputOff ends win32-input-mode; the program writes it on exit.
const Win32InputOff = "\x1b[?9001l"

// win32Input asks Windows Terminal (and ConPTY) for keys as win32-input-mode
// events. Bubble Tea reads Windows keys as VT input, where shift+enter and
// ctrl+enter arrive as plain enter; these events keep their modifiers.
func win32Input() tea.Cmd {
	if runtime.GOOS != "windows" {
		return nil
	}
	return tea.Raw("\x1b[?9001h")
}

func (m Model) poll() tea.Cmd {
	if m.store == nil {
		return nil
	}
	store := m.store
	return func() tea.Msg { return snapshotMsg(store.Poll(time.Now())) }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if modifierAlone(msg.Code) {
			return m, nil
		}
		m.restInput()
		if m.picker != nil {
			return m.updatePicker(msg)
		}
		if m.hubOpen {
			return m.hubKey(msg)
		}
		if m.win == nil && (m.prefixed || msg.String() == m.keys.Prefix) {
			// Like any other board key, prefix keys disarm a pending quit.
			m.notice, m.quitArmed = "", false
		}
		if m.prefixed {
			m.prefixed = false
			return m.afterPrefix(msg)
		}
		if msg.String() == m.keys.Prefix {
			m.prefixed = true
			return m, nil
		}
		if m.win != nil {
			return m.updateWindow(msg) // ctrl+c belongs to the program here
		}
		return m.updateBoard(msg)
	case tea.PasteMsg:
		m.restInput()
		m.prefixed = false // a paste is not the key the prefix waits for
		if m.hubOpen {
			return m, nil // nothing types into a program under the hub
		}
		// One message for the whole paste, so its line breaks never submit.
		if m.picker != nil {
			return m.pickerPaste(msg.Content), nil
		}
		if m.win != nil {
			return m.windowPaste(msg.Content), nil
		}
	case tea.MouseMsg:
		if mv, ok := msg.(tea.MouseMotionMsg); ok && mv.Button == tea.MouseNone {
			return m.hover(mv.Mouse()), nil // a move without a button only hovers
		}
		m.restInput()
		if m.hubOpen {
			return m.hubMouse(msg) // the hub is on top
		}
		switch msg.(type) {
		case tea.MouseClickMsg, tea.MouseWheelMsg:
			// The strip sits above any window: clicks there switch station,
			// the wheel scrolls it.
			if y := msg.Mouse().Y; y >= stripY && y < stripY+3 {
				return m.stripMouse(msg)
			}
		}
		if m.picker != nil {
			return m.pickerMouse(msg) // the picker is on top of any window
		}
		if m.win != nil {
			return m.windowMouse(msg)
		}
		if wh, ok := msg.(tea.MouseWheelMsg); ok {
			switch wh.Button {
			case tea.MouseWheelUp:
				return m.scrollFloor(-wheelStep), nil
			case tea.MouseWheelDown:
				return m.scrollFloor(wheelStep), nil
			}
		}
		click, ok := msg.(tea.MouseClickMsg)
		if !ok || click.Button != tea.MouseLeft {
			return m, nil
		}
		fx, fy := click.X-1, click.Y-officeY+m.floorScroll
		if office.PetHit(m.layout, fx, fy) {
			return m.openHub(tabPet)
		}
		if id, ok := office.HitTest(m.layout, m.crowd, fx, fy); ok {
			return m.openWindow(id)
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.relayout()
		m.resizePanes()
	case snapshotMsg:
		snap := claude.Snapshot(msg)
		m.snap = &snap
		m.updated = time.Now()
		m.watch()
		m.relayout()
		cmds := []tea.Cmd{tea.Tick(pollEvery, func(time.Time) tea.Msg { return pollMsg{} }), m.restTick()}
		// The pet is always on screen, so the office always animates.
		if !m.animating {
			m.animating = true
			cmds = append(cmds, animTick())
		}
		return m, tea.Batch(cmds...)
	case usageMsg:
		m.reports, m.usageAt = msg, time.Now()
	case updateMsg:
		m.newer = string(msg)
	case petNameMsg:
		return m.petNamed(msg), nil
	case settingsMsg:
		return m.settingsDone(msg), nil
	case pollMsg:
		if m.usageDue(time.Now()) {
			m.usageAt = time.Now()
			return m, tea.Batch(m.poll(), m.readUsage())
		}
		if m.win != nil {
			m.win.branch = gitBranch(m.win.dir)
		}
		return m, m.poll()
	case herdrMsg:
		m.herdrSeen = true
		m.herdrErr = msg.err
		if msg.err == nil {
			m.panes = msg.panes
		}
		m.relayout()
		return m, tea.Tick(herdrEvery, func(time.Time) tea.Msg { return herdrTickMsg{} })
	case herdrTickMsg:
		return m, m.herdrPoll()
	case gitMsg:
		m.git = msg
		return m, tea.Tick(gitEvery, func(time.Time) tea.Msg { return gitTickMsg{} })
	case gitTickMsg:
		return m, m.gitPoll()
	case boardMsg:
		m = m.boardRead(msg)
		return m, tea.Tick(boardEvery, func(time.Time) tea.Msg { return boardTickMsg{} })
	case boardTickMsg:
		return m, m.boardPoll()
	case termMsg:
		// Returning re-renders; the open terminal is drawn from its live state.
		m = m.endPane(string(msg))
		m.dropEnded(string(msg))
		return m, m.waitTerm()
	case movedMsg:
		return m.moved(msg)
	case createdMsg:
		return m.created(msg)
	case focusMsg:
		if msg.err != nil {
			m.notice = i18n.Tf("Could not focus the pane: %s", msg.err)
		} else {
			m.notice = i18n.T("Focused in herdr.")
		}
	case animMsg:
		if len(m.agents()) == 0 {
			// Only the pet moves: slow steps keep it breathing for little.
			m.frame += idleFrames
			return m, tea.Tick(idleEvery, func(time.Time) tea.Msg { return animMsg{} })
		}
		m.frame++
		m.crowd.Step(m.layout, m.agents())
		return m, animTick()
	}
	return m, nil
}

func (m Model) updateBoard(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	m.notice = ""
	armed := m.quitArmed
	m.quitArmed = false
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		if n := m.runningLocal(); n > 0 && !armed {
			m.quitArmed = true
			m.notice = i18n.Tf("%d local agent(s) or terminal(s) will stop; press q again to quit.", n)
			return m, nil
		}
		return m, tea.Quit
	case "right", "down", "l", "j", "tab":
		m.moveSelection(1)
	case "left", "up", "h", "k", "shift+tab":
		m.moveSelection(-1)
	case "enter", "c":
		if m.selected != "" {
			return m.openWindow(m.selected)
		}
	case "g":
		if m.selected != "" {
			return m.focusPane(m.selected)
		}
	case "t":
		if m.selected != "" {
			m, _ = m.openWindow(m.selected)
			if len(m.win.panes) > 1 {
				m.win.focus = 1
				return m, nil
			}
			return m.newShell()
		}
		return m.openPicker()
	case "b":
		if m.selected != "" {
			m, _ = m.openWindow(m.selected)
			return m.openBoard()
		}
	case "n":
		return m.openPicker()
	case "p":
		return m.openHub(tabPet)
	case "u":
		return m.openHub(tabUsage)
	case "s":
		return m.openHub(tabSettings)
	}
	return m, nil
}

func (m *Model) moveSelection(delta int) {
	order := office.Order(m.layout)
	if len(order) == 0 {
		return
	}
	i := 0
	for j, id := range order {
		if id == m.selected {
			i = j
		}
	}
	m.selected = order[(i+delta+len(order))%len(order)]
}

// relayout links agents to herdr panes and rebuilds the floor plan. Both
// transcripts and herdr can add agents, so it runs after either changes.
func (m *Model) relayout() {
	if m.snap == nil {
		return
	}
	m.relink()
	agents := m.agents()
	m.layout = office.Build(agents, m.floorW(), m.floorH())
	m.floorScroll = clampScroll(m.floorScroll, m.floorRows(), m.floorH())
	m.crowd.Sync(m.layout, agents)
	order := office.Order(m.layout)
	for _, id := range order {
		if id == m.selected {
			return
		}
	}
	m.selected = ""
	if len(order) > 0 {
		m.selected = order[0]
	}
}

func (m *Model) relink() {
	if m.snap == nil {
		return
	}
	// Sessions moved out of herdr never claim a pane again.
	live := make([]claude.Agent, 0, len(m.snap.Agents))
	for _, a := range m.snap.Agents {
		if !m.retired[a.ID] {
			live = append(live, a)
		}
	}
	m.links = herdr.Link(live, m.panes)
	m.fresh = herdr.Placeholders(m.panes, m.links)
}

func animTick() tea.Cmd {
	return tea.Tick(animEvery, func(time.Time) tea.Msg { return animMsg{} })
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// View draws the office full screen, with mouse cell motion reporting.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion // every move, so hovering the pet shows its name
	return v
}

func (m Model) render() string {
	w := m.width
	if m.snap == nil {
		return titleStyle.Render("BitAgentsHub") + "\n\n" + dimStyle.Render(truncate(i18n.T("Reading transcripts…"), w))
	}
	s := m.snap
	stations, working := m.agents(), 0
	for _, a := range stations {
		if a.State.Working() {
			working++
		}
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("BitAgentsHub"))
	summary := i18n.Tf(" · %d stations · %d working", len(stations), working)
	if m.board.Dir != "" {
		summary += fmt.Sprintf(" · 🐟 %d", m.stars.Total)
	}
	if s.Pending > 0 {
		summary += i18n.Tf(" · reading %d transcripts", s.Pending)
	}
	summary += m.restSummary(working)
	if m.newer != "" {
		summary += i18n.Tf(" · %s available: bah upgrade", m.newer)
	}
	b.WriteString(dimStyle.Render(ansi.Truncate(summary, max(0, w-13), "…"))) // 🐟 takes two columns
	b.WriteString("\n\n")
	for _, line := range m.strip() {
		b.WriteString(line + "\n")
	}

	fw, fh := m.floorW(), m.floorH()
	buf := screen.New(fw, fh)
	m.drawFloor(buf)
	if len(s.Agents) == 0 {
		msg := truncate(i18n.T("No agents running. Press n to start one."), fw)
		buf.Text((fw-len([]rune(msg)))/2, fh/2, msg, office.Dim)
	}
	if m.win != nil {
		m.drawWindow(buf)
	}
	if m.picker != nil {
		m.drawPicker(buf)
	}
	m.drawPetName(buf)
	if m.hubOpen {
		m.drawHub(buf)
	}

	title := truncate(i18n.T(" office "), max(0, fw-2))
	b.WriteString(dimStyle.Render("┌─" + title + strings.Repeat("─", max(0, fw-1-len([]rune(title)))) + "┐"))
	b.WriteString("\n")
	wall := dimStyle.Render("│")
	for _, line := range buf.Lines() {
		b.WriteString(wall + line + wall + "\n")
	}
	b.WriteString(dimStyle.Render("└" + strings.Repeat("─", fw) + "┘"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(truncate(m.footer(), w)))
	return b.String()
}

// floorW and floorH are the office floor inside its walls.
func (m Model) floorW() int { return max(1, m.width-2) }
func (m Model) floorH() int { return max(1, m.height-chromeH) }

// floorRows is how many rows the whole floor plan takes; more than floorH
// when its desks do not fit, and then the floor scrolls.
func (m Model) floorRows() int { return (max(m.layout.H, m.layout.Needed) + 1) / 2 }

// drawFloor paints the office, scrolled down floorScroll rows when the
// floor plan is taller than the floor.
func (m Model) drawFloor(buf *screen.Buffer) {
	full := buf
	if rows := m.floorRows(); rows > buf.H {
		full = screen.New(buf.W, rows)
	}
	office.Draw(full, m.layout, m.crowd, m.agents(), m.frame, m.selected, m.petState().Mood)
	if full != buf {
		buf.CopyRows(full, m.floorScroll)
	}
}

// scrollFloor scrolls the floor by delta rows, between its top and the row
// where its bottom shows.
func (m Model) scrollFloor(delta int) Model {
	m.floorScroll = clampScroll(m.floorScroll+delta, m.floorRows(), m.floorH())
	return m
}

func (m Model) footer() string {
	if m.prefixed {
		return m.keys.Prefix + i18n.T(" … waiting for a key: 1-9 opens that station")
	}
	if m.notice != "" {
		return m.notice
	}
	if m.layout.Needed > m.layout.H {
		return i18n.T("Some desks are off screen; scroll the floor with the wheel.")
	}
	keys := i18n.Tf("q quit · ←→ select · enter open · %s station · p hub · t terminal · n new agent", m.keyHint(config.FocusStation))
	if m.herdr != nil && m.herdrSeen && m.herdrErr == nil {
		// herdr is optional: it shows up only once it answers.
		keys += i18n.Tf(" · g herdr · herdr: %d linked", len(m.links))
	}
	return fmt.Sprintf("%s · %s", keys, m.updated.Format("15:04:05"))
}

// agents is the snapshot with herdr's view merged in: an agent whose pane
// waits on a permission prompt is marked blocked.
func (m Model) agents() []claude.Agent {
	if m.snap == nil {
		return nil
	}
	all := append(append([]claude.Agent(nil), m.snap.Agents...), m.fresh...)
	known := make(map[string]bool, len(all))
	for _, a := range all {
		known[a.ID] = true
	}
	all = append(all, m.localAgents(known)...)
	kept := all[:0]
	for _, a := range all {
		if !m.retired[a.ID] && m.running(a.ID) {
			kept = append(kept, a)
		}
	}
	kept = m.cheer(herdr.Annotate(kept, m.links))
	for i, a := range kept {
		kept[i].Done = m.unseen[a.ID]
	}
	return kept
}

// watch notes the agents that work while their window is closed, so they
// show done once they finish, as in herdr; looking at an agent clears it.
func (m Model) watch() {
	for _, a := range m.agents() {
		switch {
		case m.win != nil && m.win.agentID == a.ID:
			delete(m.unseen, a.ID)
		case a.State.Working() || a.Blocked:
			m.unseen[a.ID] = true
		}
	}
}

// running reports whether an agent's program is alive: in a terminal the
// office runs, or in a herdr pane. Sessions that only left a transcript
// stay out of the office.
func (m Model) running(id string) bool {
	if hs, ok := m.terms[id]; ok && !hs.attach && !hs.shell {
		return true
	}
	_, inHerdr := m.links[id]
	return inHerdr
}
