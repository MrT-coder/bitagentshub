package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MrT-coder/bitagentshub/internal/board"
	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/herdr"
	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/screen"
	"github.com/MrT-coder/bitagentshub/internal/term"
)

type fakeHerdr struct {
	panes   []herdr.Pane
	focused []string
	started []string // "dir: argv"
	closed  []string // pane ids
}

func (f *fakeHerdr) ListAgents() ([]herdr.Pane, error) { return f.panes, nil }
func (f *fakeHerdr) Focus(target string) error {
	f.focused = append(f.focused, target)
	return nil
}
func (f *fakeHerdr) StartAgent(dir string, argv []string, _ []herdr.Pane) (string, error) {
	f.started = append(f.started, dir+": "+strings.Join(argv, " "))
	return "w9:p1", nil
}
func (f *fakeHerdr) ClosePane(pane string) error {
	f.closed = append(f.closed, pane)
	return nil
}

// fakeTerm stands in for a program running in a pseudo terminal.
type fakeTerm struct {
	id, dir    string
	argv       []string
	keys       []string
	w, h       int
	scroll     int
	closed     bool
	exited     bool
	scrollback int
	alt        bool
	mouse      []string // "kind@x,y"
	lines      []string // screen rows; nil draws the argv
	base       screen.Style
	pal        screen.Palette
}

func (f *fakeTerm) Key(msg tea.KeyPressMsg) { f.keys = append(f.keys, msg.String()) }
func (f *fakeTerm) Paste(text string)       { f.keys = append(f.keys, "paste:"+text) }
func (f *fakeTerm) Draw(buf *screen.Buffer, x, y, w, h, scroll int, base screen.Style, pal screen.Palette) {
	f.scroll, f.base, f.pal = scroll, base, pal
	if f.lines != nil {
		for i, l := range f.lines {
			buf.Text(x, y+i, l, base)
		}
		return
	}
	buf.Text(x, y, "TERM "+strings.Join(f.argv, " "), base)
}
func (f *fakeTerm) Resize(w, h int)    { f.w, f.h = w, h }
func (f *fakeTerm) Size() (int, int)   { return f.w, f.h }
func (f *fakeTerm) ScrollbackLen() int { return f.scrollback }
func (f *fakeTerm) Exited() bool       { return f.exited }
func (f *fakeTerm) Close()             { f.closed = true }
func (f *fakeTerm) AltScreen() bool    { return f.alt }
func (f *fakeTerm) Mouse(kind term.MouseKind, x, y int) {
	f.mouse = append(f.mouse, fmt.Sprintf("%d@%d,%d", kind, x, y))
}

type termRecorder struct{ started []*fakeTerm }

func (r *termRecorder) start(id, dir string, argv []string, w, h int, _ chan<- string) (Terminal, error) {
	t := &fakeTerm{id: id, dir: dir, argv: argv, w: w, h: h, scrollback: 100}
	r.started = append(r.started, t)
	return t, nil
}

// last returns the most recently started terminal.
func (r *termRecorder) last() *fakeTerm { return r.started[len(r.started)-1] }

// officeTop is the screen row of the first floor line: title, gap, the
// three rows of the station strip and the office top wall come first.
const officeTop = 6

// Folder names come from filepath.Base, so the agents' folders are written
// the way the platform running the tests writes paths.
var (
	frontDir   = osPath(`C:\w\acme-company\portal-front`)
	serviceDir = osPath(`C:\w\acme-company\portal-service`)
)

// osPath turns a Windows path into a Unix one off Windows.
func osPath(p string) string {
	if runtime.GOOS == "windows" {
		return p
	}
	return strings.ReplaceAll(strings.TrimPrefix(p, "C:"), `\`, "/")
}

func twoAgentOffice(t *testing.T) (Model, *fakeHerdr, *termRecorder) {
	t.Helper()
	f := &fakeHerdr{panes: []herdr.Pane{
		{PaneID: "w1:p1", TerminalID: "term_1", Agent: "claude", SessionID: "front", Cwd: frontDir},
		{PaneID: "w2:p1", TerminalID: "term_2", Agent: "claude", SessionID: "service", Cwd: serviceDir},
	}}
	rec := &termRecorder{}
	m := New(nil).WithHerdr(f).WithShell([]string{"pwsh"})
	m.startTerm = rec.start
	m.canAttach = true
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = update(m, snapshotMsg(claude.Snapshot{Agents: []claude.Agent{
		{ID: "front", Group: "acme-company", Project: "portal-front", Cwd: frontDir, State: claude.StateWaiting, LastActivity: time.Now()},
		{ID: "service", Group: "acme-company", Project: "portal-service", Cwd: serviceDir, State: claude.StateWaiting, LastActivity: time.Now().Add(-10 * time.Minute)},
		// A session whose program is gone: it has a transcript but runs nowhere.
		{ID: "gone", Group: "hobbylabs", Project: "old-project", Cwd: `C:\w\hobbylabs\old-project`, State: claude.StateWaiting, LastActivity: time.Now().Add(-5 * time.Minute)},
	}}))
	panes, _ := f.ListAgents()
	m = update(m, herdrMsg{panes: panes})
	return m, f, rec
}

// press runs a key through Update and executes the command it returns once.
func press(m Model, msg tea.Msg) Model {
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		if out := cmd(); out != nil {
			m = update(m, out)
		}
	}
	return m
}

// key builds the key press a terminal reports for a keystroke such as
// "enter", "ctrl+q", "f10" or typed text.
func key(s string) tea.KeyPressMsg {
	codes := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "right": tea.KeyRight, "up": tea.KeyUp,
		"down": tea.KeyDown, "tab": tea.KeyTab, "backspace": tea.KeyBackspace,
		"f2": tea.KeyF2, "f3": tea.KeyF3, "f4": tea.KeyF4, "f5": tea.KeyF5, "f10": tea.KeyF10,
	}
	if c, ok := codes[s]; ok {
		return tea.KeyPressMsg{Code: c}
	}
	if r, ok := strings.CutPrefix(s, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(r[0]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func wheel(up bool) tea.MouseWheelMsg {
	b := tea.MouseWheelDown
	if up {
		b = tea.MouseWheelUp
	}
	return tea.MouseWheelMsg{X: 20, Y: 20, Button: b}
}

func view(m Model) string { return plain(m.render()) }

func TestClickingAHerdrAgentOpensItsRealTerminalThroughAttach(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	x, y := m.layout.Stations["front"].LabelCell()
	m = press(m, click(1+x+1, officeTop+y))

	if len(rec.started) != 1 || strings.Join(rec.started[0].argv, " ") != "herdr agent attach term_1" {
		t.Fatalf("started = %+v", rec.started)
	}
	for _, want := range []string{"TERM herdr agent attach term_1", "portal-front", "─ office ─"} {
		if !strings.Contains(view(m), want) {
			t.Errorf("view missing %q:\n%s", want, view(m))
		}
	}
}

func TestAPasteReachesTheFocusedPaneAsOnePaste(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	text := strings.Repeat("line of pasted text\r\n", 200)
	m = press(m, tea.PasteMsg{Content: text})
	if keys := rec.started[0].keys; len(keys) != 1 || keys[0] != "paste:"+text {
		t.Errorf("terminal got %d inputs, want the paste once", len(keys))
	}
	if !strings.Contains(view(m), "TERM ") {
		t.Errorf("the window closed on paste:\n%s", view(m))
	}
}

func TestTheOfficeDrawsFullScreenWithMouse(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	v := m.View()
	if !v.AltScreen || v.MouseMode != tea.MouseModeAllMotion {
		t.Errorf("view alt=%v mouse=%v, want alt screen with all motion, for the pet's hover", v.AltScreen, v.MouseMode)
	}
}

func TestEveryKeyGoesToTheFocusedPane(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	for _, k := range []string{"r", "esc", "ctrl+c", "q", "up", "enter", "ctrl+t"} {
		m = press(m, key(k))
	}
	if got := strings.Join(rec.started[0].keys, ","); got != "r,esc,ctrl+c,q,up,enter,ctrl+t" {
		t.Errorf("terminal keys = %q", got)
	}
	if !strings.Contains(view(m), "TERM ") {
		t.Errorf("the window closed while typing:\n%s", view(m))
	}
}

func TestCtrlQOnTheAgentClosesTheWindowAndEndsTheAttach(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("ctrl+q"))
	if strings.Contains(view(m), "TERM ") {
		t.Errorf("window still open:\n%s", view(m))
	}
	if !rec.started[0].closed || len(rec.started[0].keys) != 0 {
		t.Errorf("attach closed=%v keys=%v", rec.started[0].closed, rec.started[0].keys)
	}
}

func TestF3OpensATerminalBesideTheAgentInItsFolder(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	agentW, _ := rec.started[0].Size()
	m = press(m, key("f3"))

	shell := rec.last()
	if strings.Join(shell.argv, " ") != "pwsh" || shell.dir != frontDir {
		t.Fatalf("shell = %+v", shell)
	}
	if w, _ := rec.started[0].Size(); w >= agentW {
		t.Errorf("agent pane width %d -> %d, want it to make room", agentW, w)
	}
	v := view(m)
	for _, want := range []string{"TERM herdr agent attach", "TERM pwsh"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	m = press(m, key("x"))
	if len(shell.keys) != 1 || len(rec.started[0].keys) != 0 {
		t.Errorf("keys should go to the new, focused terminal: shell=%v agent=%v", shell.keys, rec.started[0].keys)
	}
}

func TestTerminalsUseTheThemesWindowAndTerminalColors(t *testing.T) {
	th, _ := office.Preset("gentle-ai")
	office.SetTheme(th)
	t.Cleanup(func() { office.SetTheme(office.BuiltinTheme()) })
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	_ = view(m)
	agent := rec.started[0]
	if want := screen.Fg(th.UIText).WithBg(th.UIBg); agent.base != want {
		t.Errorf("terminal text = %+v, want %+v", agent.base, want)
	}
	if agent.pal != th.Terminal {
		t.Errorf("terminal palette = %+v, want the theme's", agent.pal)
	}
}

func TestCtrlAHSwitchesPanesAndCtrlQBelongsToATerminal(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("f3"))
	shell := rec.last()
	m = press(m, key("ctrl+q")) // nvim's visual block in Windows Terminal
	if len(shell.keys) != 1 || shell.keys[0] != "ctrl+q" || !strings.Contains(view(m), "TERM pwsh") {
		t.Fatalf("ctrl+q in a terminal pane should reach the program: %v", shell.keys)
	}
	m = prefixed(m, "h")
	m = press(m, key("a"))
	if got := strings.Join(rec.started[0].keys, ","); got != "a" {
		t.Errorf("after ctrl+a h the agent pane should get keys, got %q", got)
	}
}

func TestF5ClosesTheFocusedTerminalOnly(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("f3"))
	shell := rec.last()
	m = press(m, key("f5"))
	if !shell.closed || strings.Contains(view(m), "TERM pwsh") || !strings.Contains(view(m), "TERM herdr") {
		t.Errorf("closed=%v view:\n%s", shell.closed, view(m))
	}
}

func TestTerminalsSurviveClosingTheWindowAndComeBack(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("f3"))
	shell := rec.last()
	m = press(m, key("f10"))
	if shell.closed || strings.Contains(view(m), "TERM pwsh") {
		t.Fatalf("closing the window should hide, not end, the terminal (closed=%v)", shell.closed)
	}
	m = press(m, key("enter"))
	if !strings.Contains(view(m), "TERM pwsh") {
		t.Errorf("the folder's terminal did not come back:\n%s", view(m))
	}
}

func TestTOpensATerminalInTheSelectedAgentsFolder(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("t"))
	if len(rec.started) == 0 || strings.Join(rec.last().argv, " ") != "pwsh" || rec.last().dir != frontDir {
		t.Fatalf("started = %+v", rec.started)
	}
	if !strings.Contains(view(m), "TERM pwsh") {
		t.Errorf("view:\n%s", view(m))
	}
}

func TestClickingAPaneFocusesIt(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("f3")) // focus moves to the terminal on the right
	rects := m.paneRects()
	m = press(m, click(1+rects[0].x+2, officeTop+rects[0].y+2))
	m = press(m, key("b"))
	if got := strings.Join(rec.started[0].keys, ","); got != "b" {
		t.Errorf("click should focus the agent pane, keys = %q", got)
	}
}

func TestWheelScrollsTheHistoryAndTypingReturnsToLive(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, wheel(true))
	m = press(m, wheel(true))
	_ = m.render()
	if rec.started[0].scroll != 6 {
		t.Errorf("scroll = %d, want 6 after two wheel notches", rec.started[0].scroll)
	}
	m = press(m, key("a"))
	_ = m.render()
	if rec.started[0].scroll != 0 {
		t.Errorf("scroll = %d, want back to live after typing", rec.started[0].scroll)
	}
}

// drag marks pane cells (x0, y0) to (x1, y1) with the left button and
// returns the text the office put on the clipboard ("" when it put none).
func drag(t *testing.T, m Model, pane, x0, y0, x1, y1 int) (Model, string) {
	t.Helper()
	r := m.paneRects()[pane]
	at := func(x, y int) tea.Mouse {
		return tea.Mouse{X: 1 + r.x + x, Y: officeTop + r.y + y, Button: tea.MouseLeft}
	}
	m = press(m, tea.MouseClickMsg(at(x0, y0)))
	m = press(m, tea.MouseMotionMsg(at(x1, y1)))
	next, cmd := m.Update(tea.MouseReleaseMsg(at(x1, y1)))
	m = next.(Model)
	if cmd == nil {
		return m, ""
	}
	return m, fmt.Sprint(cmd())
}

// screenRow is the rendered row (with color codes) of pane row y.
func screenRow(m Model, pane, y int) string {
	return strings.Split(m.render(), "\n")[officeTop+m.paneRects()[pane].y+y]
}

func TestDraggingSelectsTextAndCopiesIt(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	ft := rec.started[0]
	ft.lines = []string{"hello world", "second line", "third"}
	m, got := drag(t, m, 0, 6, 0, 5, 1)
	if want := "world\nsecond"; got != want {
		t.Errorf("clipboard = %q, want %q", got, want)
	}
	if !strings.Contains(view(m), "Copied 12 characters") {
		t.Errorf("footer should say what was copied:\n%s", view(m))
	}
	if !strings.Contains(screenRow(m, 0, 1), "\x1b[7m") {
		t.Errorf("the selected text should be highlighted: %q", screenRow(m, 0, 1))
	}
	if len(ft.keys) != 0 || len(ft.mouse) != 0 {
		t.Errorf("selecting sent input to the program: keys %v mouse %v", ft.keys, ft.mouse)
	}
	m = press(m, key("a"))
	if strings.Contains(screenRow(m, 0, 1), "\x1b[7m") || strings.Contains(view(m), "Copied") {
		t.Errorf("typing should clear the selection: %q", screenRow(m, 0, 1))
	}
}

func TestDraggingBackwardsAndPastThePaneEdge(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	rec.started[0].lines = []string{"one", "two"}
	r := m.paneRects()[0]
	// From the end of row 1 back to before the pane's left edge on row 0.
	_, got := drag(t, m, 0, 2, 1, -5, 0)
	if want := "one\ntwo"; got != want {
		t.Errorf("clipboard = %q, want %q (pane %dx%d)", got, want, r.w, r.h)
	}
}

func TestDraggingOverAFullScreenProgramSelectsInsteadOfClicking(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	ft := rec.started[0]
	ft.alt = true
	ft.lines = []string{"claude says hi"}
	_, got := drag(t, m, 0, 0, 0, 5, 0)
	if got != "claude" {
		t.Errorf("clipboard = %q, want %q", got, "claude")
	}
	if len(ft.mouse) != 0 {
		t.Errorf("a drag should not reach the program, got mouse %v", ft.mouse)
	}
}

func TestFullScreenProgramsGetTheWheelAndClicks(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	ft := rec.started[0]
	ft.alt = true
	r := m.paneRects()[0]
	sx, sy := 1+r.x+3, officeTop+r.y+2 // terminal cell (3, 2)
	m = press(m, tea.MouseWheelMsg{X: sx, Y: sy, Button: tea.MouseWheelUp})
	m = press(m, tea.MouseClickMsg{X: sx, Y: sy, Button: tea.MouseLeft})
	m = press(m, tea.MouseReleaseMsg{X: sx, Y: sy, Button: tea.MouseLeft})
	_ = m.render()
	want := fmt.Sprintf("%d@3,2 %d@3,2 %d@3,2", term.MouseWheelUp, term.MouseLeftPress, term.MouseLeftRelease)
	if got := strings.Join(ft.mouse, " "); got != want {
		t.Errorf("mouse = %q, want %q", got, want)
	}
	if ft.scroll != 0 {
		t.Errorf("the office scrolled its own history (%d) instead of passing the wheel on", ft.scroll)
	}
}

func TestF2MaximizesTheWindow(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	w0, h0 := rec.started[0].Size()
	m = press(m, key("f2"))
	w1, h1 := rec.started[0].Size()
	if w1 <= w0 || h1 <= h0 || len(rec.started[0].keys) != 0 {
		t.Errorf("size %dx%d -> %dx%d keys=%v", w0, h0, w1, h1, rec.started[0].keys)
	}
}

func TestZMaximizesOnlyTheFocusedPane(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	agent := rec.last()
	m = press(m, key("f3"))
	shell := rec.last()
	aw, ah := agent.Size()
	m = prefixed(m, "z")
	if v := view(m); !strings.Contains(v, "TERM pwsh") || strings.Contains(v, "TERM herdr") {
		t.Fatalf("only the focused terminal should show:\n%s", v)
	}
	if w, h := shell.Size(); w != m.floorW()-2 || h != m.floorH()-2 {
		t.Errorf("the focused pane should fill the floor: %dx%d, floor %dx%d", w, h, m.floorW(), m.floorH())
	}
	if w, h := agent.Size(); w != aw || h != ah {
		t.Errorf("the hidden agent pane was resized: %dx%d -> %dx%d", aw, ah, w, h)
	}
	m = press(m, key("a"))
	if strings.Join(shell.keys, ",") != "a" || len(agent.keys) != 0 {
		t.Errorf("keys should reach the maximized pane: shell %v agent %v", shell.keys, agent.keys)
	}
	m = prefixed(m, "h")
	if v := view(m); !strings.Contains(v, "TERM herdr") || strings.Contains(v, "TERM pwsh") {
		t.Errorf("ctrl+a h should maximize the pane on the left instead:\n%s", v)
	}
	m = prefixed(m, "z")
	if v := view(m); !strings.Contains(v, "TERM herdr") || !strings.Contains(v, "TERM pwsh") {
		t.Errorf("ctrl+a z again should show both panes:\n%s", v)
	}
	if w, _ := shell.Size(); w >= m.floorW()-2 {
		t.Errorf("restored, the terminal should share the window again: width %d", w)
	}
}

func boardOffice(t *testing.T) (Model, *termRecorder, string) {
	t.Helper()
	m, _, rec := twoAgentOffice(t)
	dir := t.TempDir()
	return m.WithBoard(config.Board{Dir: dir, Editor: "nvim"}), rec, dir
}

func TestCtrlABOpensTodaysNoteInTheEditorBesideTheAgent(t *testing.T) {
	m, rec, dir := boardOffice(t)
	m = press(m, key("enter"))
	m = prefixed(m, "b")
	note := board.NotePath(dir, "portal-front", time.Now())
	ed := rec.last()
	if strings.Join(ed.argv, "|") != "nvim|"+note || ed.dir != frontDir {
		t.Fatalf("editor started as %q in %q, want nvim on %s", ed.argv, ed.dir, note)
	}
	if len(m.win.panes) != 2 || m.win.focus != 1 {
		t.Errorf("the note should open beside the agent, focused: panes %d focus %d", len(m.win.panes), m.win.focus)
	}
	if b, err := os.ReadFile(note); err != nil || !strings.HasPrefix(string(b), "# portal-front · ") {
		t.Errorf("today's note was not created: %q %v", b, err)
	}
	ed.exited = true
	m = update(m, termMsg(ed.id))
	if len(m.win.panes) != 1 {
		t.Errorf("leaving the editor should close its pane: panes %d", len(m.win.panes))
	}
}

func TestBOnTheBoardOpensTheSelectedStationsNote(t *testing.T) {
	m, rec, dir := boardOffice(t)
	m = press(m, key("b"))
	if m.win == nil || m.win.agentID == "" {
		t.Fatal("b should open the selected station's window")
	}
	note := board.NotePath(dir, "portal-front", time.Now())
	if got := strings.Join(rec.last().argv, "|"); got != "nvim|"+note {
		t.Errorf("editor = %q, want nvim on %s", got, note)
	}
}

// boardPoll runs the board read the office does every few seconds.
func boardPoll(t *testing.T, m Model) Model {
	t.Helper()
	cmd := m.boardPoll()
	if cmd == nil {
		t.Fatal("no board poll with a board folder")
	}
	return update(m, cmd())
}

func cheering(m Model) []string {
	var ids []string
	for _, a := range m.agents() {
		if a.Cheer {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func TestCheckingOffATaskMakesTheProjectsAgentCheer(t *testing.T) {
	m, _, dir := boardOffice(t)
	note := board.NotePath(dir, "portal-front", time.Now())
	if err := os.MkdirAll(filepath.Dir(note), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(note, []byte("- [ ] ship it\n- [ ] test it\n- [x] plan it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = boardPoll(t, m)
	if got := cheering(m); len(got) != 0 {
		t.Errorf("tasks done before the office looked are not news: %v cheer", got)
	}
	if v := view(m); !strings.Contains(v, "2 todo") || !strings.Contains(v, "🐟 1") {
		t.Errorf("the strip should count 2 tasks to do and the title 1 point:\n%s", v)
	}
	if err := os.WriteFile(note, []byte("- [x] ship it\n- [ ] test it\n- [x] plan it\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = boardPoll(t, m)
	if got := cheering(m); len(got) != 1 || got[0] != "front" {
		t.Errorf("only portal-front's agent should cheer, got %v", got)
	}
	if v := view(m); !strings.Contains(v, "1 todo") || !strings.Contains(v, "🐟 2") {
		t.Errorf("after checking one off: 1 to do, 2 points:\n%s", v)
	}
	m.cheers["portal-front"] = time.Now().Add(-time.Second) // the cheer is over
	if got := cheering(m); len(got) != 0 {
		t.Errorf("the cheer should end, still cheering %v", got)
	}
}

func TestTheHeaderSaysWhileOlderTranscriptsAreStillRead(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	snap := *m.snap
	snap.Pending = 120
	m = update(m, snapshotMsg(snap))
	if v := view(m); !strings.Contains(v, "reading 120 transcripts") {
		t.Errorf("the header should say transcripts are still read:\n%s", v)
	}
	snap.Pending = 0
	m = update(m, snapshotMsg(snap))
	if v := view(m); strings.Contains(v, "reading") {
		t.Errorf("the note should go once every transcript is read:\n%s", v)
	}
}

func TestWithoutABoardNothingIsPolledOrShown(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	if m.boardPoll() != nil {
		t.Error("the board is polled without a board folder")
	}
	if v := view(m); strings.Contains(v, "🐟") || strings.Contains(v, "todo") {
		t.Errorf("board marks shown without a board:\n%s", v)
	}
}

func TestWithoutABoardFolderTheKeysSayHowToSetOne(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	started := len(rec.started)
	m = prefixed(m, "b")
	if len(rec.started) != started || len(m.win.panes) != 1 {
		t.Fatalf("no editor should start without a board folder")
	}
	if !strings.Contains(m.win.status, "[board] dir") {
		t.Errorf("status = %q, want it to point at [board] dir", m.win.status)
	}
}

// On Windows Bubble Tea reads keys as VT input, where shift+enter is plain
// enter, unless the office asks the terminal for win32-input-mode.
func TestTheOfficeAsksWindowsForKeysWithTheirModifiers(t *testing.T) {
	cmd := win32Input()
	if runtime.GOOS != "windows" {
		if cmd != nil {
			t.Fatalf("win32-input-mode is asked for off Windows: %v", cmd())
		}
		return
	}
	if cmd == nil {
		t.Fatal("the office does not ask for win32-input-mode")
	}
	if got, ok := cmd().(tea.RawMsg); !ok || got.Msg != "\x1b[?9001h" {
		t.Errorf("win32Input sends %#v, want \\x1b[?9001h", cmd())
	}
	if Win32InputOff != "\x1b[?9001l" {
		t.Errorf("Win32InputOff = %q", Win32InputOff)
	}
}

// win32-input-mode reports shift, ctrl and alt pressed alone as keys.
func TestModifierKeysPressedAloneAreIgnored(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	agent := rec.last()
	m = press(m, key("f3"))
	shell := rec.last()
	m = press(m, tea.KeyPressMsg{Code: tea.KeyLeftShift, Mod: tea.ModShift})
	m = press(m, tea.KeyPressMsg{Code: tea.KeyLeftCtrl, Mod: tea.ModCtrl})
	m = press(m, key("ctrl+a"))
	m = press(m, tea.KeyPressMsg{Code: tea.KeyRightAlt, Mod: tea.ModAlt})
	m = press(m, key("h"))
	if m.win.focus != 0 {
		t.Errorf("a modifier between the prefix and h should not cancel ctrl+a h: focus %d", m.win.focus)
	}
	if len(shell.keys) != 0 || len(agent.keys) != 0 {
		t.Errorf("modifier keys reached programs: shell %v agent %v", shell.keys, agent.keys)
	}
}

// Claude Code reads alt+enter (ESC CR) as a new line, as its /terminal-setup
// binds shift+enter; a shell gets shift+enter itself (PowerShell adds a line).
func TestShiftEnterStartsANewLineInAgentsAndShells(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	agent := rec.last()
	shiftEnter := tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	m = press(m, shiftEnter)
	m = press(m, key("f3"))
	shell := rec.last()
	m = press(m, shiftEnter)
	if got := strings.Join(agent.keys, ","); got != "alt+enter" {
		t.Errorf("the agent got %q, want alt+enter", got)
	}
	if got := strings.Join(shell.keys, ","); got != "shift+enter" {
		t.Errorf("the shell got %q, want shift+enter", got)
	}
}

func TestAMaximizedPaneIsMarkedWithZInTheCorner(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("f3"))
	if strings.Contains(view(m), " z ─┐") {
		t.Fatalf("the mark shows before maximizing:\n%s", view(m))
	}
	m = prefixed(m, "z")
	if !strings.Contains(view(m), " z ─┐") {
		t.Errorf("a maximized pane should show z in the top right corner:\n%s", view(m))
	}
	m = prefixed(m, "z")
	if strings.Contains(view(m), " z ─┐") {
		t.Errorf("the mark should go once the window is restored:\n%s", view(m))
	}
}

func TestANewTerminalShowsBesideAMaximizedPane(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = prefixed(m, "z")
	m = prefixed(m, "c")
	if v := view(m); m.win.max || !strings.Contains(v, "TERM herdr") || !strings.Contains(v, "TERM pwsh") {
		t.Errorf("a new terminal should restore the window to show it beside the agent (max=%v):\n%s", m.win.max, v)
	}
	if w, _ := rec.last().Size(); w <= 1 {
		t.Errorf("the new terminal got width %d", w)
	}
}

func TestTheNextPaneKeysAreGone(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("f3"))
	m = prefixed(m, "o")
	if m.win.focus != 1 {
		t.Errorf("ctrl+a o should no longer move the focus, focus %d", m.win.focus)
	}
	m = press(m, key("f4"))
	if m.win.focus != 1 || strings.Join(rec.last().keys, ",") != "f4" {
		t.Errorf("F4 should reach the program now: focus %d keys %v", m.win.focus, rec.last().keys)
	}
	if strings.Contains(m.windowHints(), "next pane") {
		t.Errorf("window hints still offer next pane: %q", m.windowHints())
	}
}

func TestQuittingWithLocalTerminalsRunningAsksFirst(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(m, key("t"))   // a local terminal now runs
	m = press(m, key("f10")) // back to the office
	next, cmd := m.Update(key("q"))
	m = next.(Model)
	if cmd != nil && cmd() == tea.Quit() {
		t.Fatal("quit without confirmation while a terminal runs")
	}
	if !strings.Contains(view(m), "press q again") {
		t.Errorf("view should warn before quitting:\n%s", view(m))
	}
	if _, cmd = m.Update(key("q")); cmd == nil || cmd() != tea.Quit() {
		t.Error("second q should quit")
	}
}

func TestWithoutDirectAttachAHerdrAgentPointsToItsPane(t *testing.T) {
	m, f, rec := twoAgentOffice(t)
	m.canAttach = false // herdr has no direct attach on Windows yet
	m = press(m, key("enter"))
	if len(rec.started) != 0 {
		t.Fatalf("started a terminal anyway: %+v", rec.started)
	}
	if !strings.Contains(view(m), "g jumps to its herdr pane") {
		t.Errorf("view should explain and offer the jump:\n%s", view(m))
	}
	m = press(m, key("g"))
	if len(f.focused) != 1 || f.focused[0] != "term_1" {
		t.Errorf("focused = %v", f.focused)
	}
}

func TestMovingAnIdleHerdrAgentIntoTheOffice(t *testing.T) {
	m, f, rec := twoAgentOffice(t)
	m.canAttach = false
	m = press(m, key("enter"))
	m = press(m, key("m"))
	if len(f.closed) != 0 || !strings.Contains(view(m), "Press m again") {
		t.Fatalf("first m should only ask; closed=%v view:\n%s", f.closed, view(m))
	}
	m = press(m, key("m"))

	if len(f.closed) != 1 || f.closed[0] != "w1:p1" {
		t.Fatalf("herdr panes closed = %v", f.closed)
	}
	st := rec.last()
	if st.dir != frontDir || len(st.argv) != 3 || st.argv[1] != "--session-id" || st.argv[2] == "front" {
		t.Errorf("started %+v, want a fresh local claude in the same folder", st)
	}
	for _, a := range m.agents() {
		if a.ID == "front" {
			t.Error("the moved session is still in the office")
		}
	}
	if !strings.Contains(view(m), "TERM claude --session-id") {
		t.Errorf("the window should show the moved agent:\n%s", view(m))
	}
}

func TestAWorkingHerdrAgentIsNotMoved(t *testing.T) {
	m, f, _ := twoAgentOffice(t)
	m.canAttach = false
	snap := *m.snap
	snap.Agents = append([]claude.Agent(nil), snap.Agents...)
	snap.Agents[0].State = claude.StateTool
	m = update(m, snapshotMsg(snap))
	m = press(m, key("enter"))
	m = press(m, key("m"))
	m = press(m, key("m"))
	if len(f.closed) != 0 || !strings.Contains(view(m), "is working") {
		t.Errorf("closed=%v view:\n%s", f.closed, view(m))
	}
}

func TestGJumpsToTheSelectedAgentPaneInHerdr(t *testing.T) {
	m, f, _ := twoAgentOffice(t)
	m = press(m, key("g"))
	if len(f.focused) != 1 || f.focused[0] != "term_1" {
		t.Errorf("focused = %v, want [term_1]", f.focused)
	}
}

func TestArrowKeysMoveTheSelection(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	if !strings.Contains(view(m), ">1 portal-front") {
		t.Errorf("first agent should start selected:\n%s", view(m))
	}
	m = press(m, key("right"))
	if !strings.Contains(view(m), ">2 portal-service") {
		t.Errorf("right should select the next agent:\n%s", view(m))
	}
}

func TestAgentBlockedInHerdrIsShownAsBlocked(t *testing.T) {
	m, f, _ := twoAgentOffice(t)
	f.panes[0].Status = "blocked"
	m = update(m, herdrMsg{panes: f.panes})
	if !strings.Contains(view(m), "▲ blocked") {
		t.Errorf("view should flag the blocked agent:\n%s", view(m))
	}
}

func TestAnyOtherKeyIncludingFKeysCancelsAnArmedMove(t *testing.T) {
	for _, between := range [][]string{{"f2"}, {"f3", "f4"}, {"f3", "x", "f4"}} {
		t.Run(strings.Join(between, "+"), func(t *testing.T) {
			m, f, _ := twoAgentOffice(t)
			m.canAttach = false
			m = press(m, key("enter"))
			m = press(m, key("m"))
			for _, k := range between {
				m = press(m, key(k))
			}
			m = press(m, key("m"))
			if len(f.closed) != 0 {
				t.Errorf("m, %v, m closed herdr panes %v", between, f.closed)
			}
		})
	}
}

func TestAHerdrPaneReportedWorkingIsNotMoved(t *testing.T) {
	m, f, _ := twoAgentOffice(t)
	m.canAttach = false
	f.panes[0].Status = "working" // herdr is ahead of the transcript
	m = update(m, herdrMsg{panes: f.panes})
	m = press(m, key("enter"))
	m = press(m, key("m"))
	m = press(m, key("m"))
	if len(f.closed) != 0 || !strings.Contains(view(m), "is working") {
		t.Errorf("closed=%v view:\n%s", f.closed, view(m))
	}
}

func TestAMoveInProgressIgnoresFurtherPresses(t *testing.T) {
	m, f, _ := twoAgentOffice(t)
	m.canAttach = false
	m = press(m, key("enter"))
	m = press(m, key("m"))
	next, cmd := m.Update(key("m")) // the close is issued but has not returned
	m = next.(Model)
	m = press(m, key("m"))
	m = press(m, key("m"))
	if cmd != nil {
		cmd()
	}
	if len(f.closed) != 1 {
		t.Errorf("closed = %v, want exactly one close", f.closed)
	}
}

func TestAFailedStartAfterAMoveIsReported(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m.canAttach = false
	m.startTerm = func(string, string, []string, int, int, chan<- string) (Terminal, error) {
		return nil, fmt.Errorf("claude not found")
	}
	m = press(m, key("enter"))
	m = press(m, key("m"))
	m = press(m, key("m"))
	if !strings.Contains(view(m), "claude not found") {
		t.Errorf("the start failure should be visible:\n%s", view(m))
	}
}

func TestAMovedSessionNoLongerClaimsHerdrPanes(t *testing.T) {
	m, f, _ := twoAgentOffice(t)
	m.retired["front"] = true
	f.panes = append(f.panes, herdr.Pane{PaneID: "w2:p1", Agent: "claude", Cwd: frontDir})
	m = update(m, herdrMsg{panes: f.panes})
	if _, ok := m.links["front"]; ok {
		t.Error("a retired session is still linked to a herdr pane")
	}
}

func TestAMoveInProgressSurvivesReopeningTheWindow(t *testing.T) {
	m, f, _ := twoAgentOffice(t)
	m.canAttach = false
	m = press(m, key("enter"))
	m = press(m, key("m"))
	next, cmd := m.Update(key("m")) // close issued, not returned yet
	m = next.(Model)
	m = press(m, key("f10"))
	m = press(m, key("enter"))
	m = press(m, key("m"))
	m = press(m, key("m"))
	if cmd != nil {
		cmd()
	}
	if len(f.closed) != 1 {
		t.Errorf("closed = %v, want exactly one close", f.closed)
	}
}

func TestOnlyAgentsThatAreRunningAreShown(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	if strings.Contains(view(m), "old-project") {
		t.Errorf("a session that runs nowhere is in the office:\n%s", view(m))
	}
	for _, id := range office.Order(m.layout) {
		if id == "gone" {
			t.Error("a session that runs nowhere can be selected")
		}
	}
	if !strings.Contains(view(m), "portal-front") || !strings.Contains(view(m), "portal-service") {
		t.Errorf("agents running in herdr should be shown:\n%s", view(m))
	}
}

func TestALocalAgentWhoseProgramEndedLeavesTheOffice(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(withProjects(m, deck), key("n"))
	m = press(m, key("enter"))
	local := rec.last()
	if !strings.Contains(view(m), "minigames") {
		t.Fatalf("the new local agent should be in the office:\n%s", view(m))
	}
	local.exited = true
	m = update(m, termMsg(local.id))
	if m.win != nil {
		t.Errorf("the window of an ended agent with no terminals should close on its own:\n%s", view(m))
	}
	if !local.closed {
		t.Error("the ended agent's terminal was not released")
	}
	for _, a := range m.agents() {
		if a.ID == local.id {
			t.Error("an ended local agent is still in the office")
		}
	}
}

func TestATerminalThatEndsLeavesTheWindowOnItsOwn(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("f3"))
	shell := rec.last()
	shell.exited = true // exit, ctrl+d, a crash: the program is gone
	m = update(m, termMsg(shell.id))
	if m.win == nil || len(m.win.panes) != 1 || m.win.focus != 0 {
		t.Fatalf("only the agent pane should remain, focused: %+v", m.win)
	}
	if !shell.closed || strings.Contains(view(m), "TERM pwsh") || !strings.Contains(view(m), "TERM herdr") {
		t.Errorf("closed=%v view:\n%s", shell.closed, view(m))
	}
	m = press(m, key("f10"))
	m = press(m, key("enter"))
	if strings.Contains(view(m), "TERM pwsh") {
		t.Errorf("an ended terminal came back with its folder:\n%s", view(m))
	}
}

func TestTheLastTerminalEndingClosesTheWindow(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m, _ = m.openFolder(t.TempDir()) // a folder with no agent: terminals only
	shell := rec.last()
	if m.win == nil || len(m.win.panes) != 1 || !strings.Contains(view(m), "TERM pwsh") {
		t.Fatalf("the folder should open with one terminal:\n%s", view(m))
	}
	shell.exited = true
	m = update(m, termMsg(shell.id))
	if m.win != nil {
		t.Errorf("a window with no panes left should close:\n%s", view(m))
	}
}

func TestAnEndedAgentLeavesItsTerminalsOpen(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(withProjects(m, deck), key("n"))
	m = press(m, key("enter"))
	local := rec.last()
	m = press(m, key("f3"))
	shell := rec.last()
	m = prefixed(m, "h") // back to the agent pane
	local.exited = true
	m = update(m, termMsg(local.id))
	if m.win == nil || len(m.win.panes) != 1 || m.win.agentID != "" || m.win.focus != 0 {
		t.Fatalf("the window should keep only the terminal: %+v", m.win)
	}
	if shell.closed || !strings.Contains(view(m), "TERM pwsh") {
		t.Errorf("the terminal beside the agent should keep running:\n%s", view(m))
	}
	m = press(m, key("a"))
	if got := strings.Join(shell.keys, ","); got != "a" {
		t.Errorf("keys should reach the remaining terminal, got %q", got)
	}
	for _, a := range m.agents() {
		if a.ID == local.id {
			t.Error("an ended local agent is still in the office")
		}
	}
}

func TestTheWindowFooterShowsOnlyTheMainPrefixKeys(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(m, key("enter"))
	want := "ctrl+a then: d close · z maximize · b board · shift+n new agent"
	if hints := m.windowHints(); hints != want {
		t.Errorf("window hints = %q, want %q", hints, want)
	}
}

func TestPrefixShiftNStartsANewAgentFromAWindow(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = prefixed(m, "N")
	if m.picker == nil {
		t.Fatal("ctrl+a shift+n should open the new agent picker")
	}
	if m.win == nil || m.win.agentID != "front" {
		t.Error("the window should stay open under the picker")
	}
	m = press(m, key("esc"))
	if m.picker != nil || m.win == nil {
		t.Error("esc should close only the picker")
	}
}

func TestTheFishInTheHeaderNeverPushesItPastTheTerminal(t *testing.T) {
	m, _, _ := boardOffice(t)
	m.stars = board.Stars{Total: 12}
	for w := 30; w <= 90; w++ {
		m = update(m, tea.WindowSizeMsg{Width: w, Height: 30})
		header := strings.SplitN(view(m), "\n", 2)[0]
		if got := ansi.StringWidth(header); got > w {
			t.Errorf("width %d: header is %d columns: %q", w, got, header)
		}
	}
}
