package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/herdr"
	"github.com/MrT-coder/bitagentshub/internal/office"
)

// crowdedOffice runs n agents in herdr, in folders with long names, on a
// terminal width columns wide.
func crowdedOffice(t *testing.T, n, width int) Model {
	t.Helper()
	f := &fakeHerdr{}
	var agents []claude.Agent
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("a%d", i)
		dir := osPath(fmt.Sprintf(`C:\w\acme-company\shopping-portal-service-%d`, i))
		f.panes = append(f.panes, herdr.Pane{PaneID: fmt.Sprintf("w%d:p1", i), TerminalID: "term_" + id, Agent: "claude", SessionID: id, Cwd: dir})
		agents = append(agents, claude.Agent{ID: id, Group: "acme-company", Project: fmt.Sprintf("shopping-portal-service-%d", i),
			Cwd: dir, State: claude.StateWaiting, LastActivity: time.Now().Add(-time.Duration(i) * time.Minute)})
	}
	m := New(nil).WithHerdr(f).WithShell([]string{"pwsh"})
	m.startTerm = (&termRecorder{}).start
	m.canAttach = true
	m = update(m, tea.WindowSizeMsg{Width: width, Height: 50})
	m = update(m, snapshotMsg(claude.Snapshot{Agents: agents}))
	panes, _ := f.ListAgents()
	return update(m, herdrMsg{panes: panes})
}

func TestTheStripShrinksNamesSoEveryStationFits(t *testing.T) {
	m := crowdedOffice(t, 6, 150)
	names, _ := stripRows(m)
	for i := 1; i <= 6; i++ {
		if !strings.Contains(names, fmt.Sprintf("%d claude", i)) {
			t.Errorf("station %d is missing from the strip: %q", i, names)
		}
	}
	if strings.ContainsAny(names, "‹›") {
		t.Errorf("nothing should be scrolled out of view: %q", names)
	}
}

func TestATooNarrowStripScrollsToTheOpenStation(t *testing.T) {
	m := crowdedOffice(t, 6, 70)
	last := office.Order(m.layout)[5]
	m, _ = m.openWindow(last)
	names, _ := stripRows(m)
	if !strings.Contains(names, "6 claude") {
		t.Errorf("the open station should be in view: %q", names)
	}
	if strings.Contains(names, "1 claude") || !strings.HasPrefix(names, "‹") {
		t.Errorf("stations scrolled out on the left should be marked with ‹: %q", names)
	}
}

// stripX is the column where station n's name starts in the strip.
func stripX(t *testing.T, m Model, n int) int {
	t.Helper()
	names, _ := stripRows(m)
	i := strings.Index(names, fmt.Sprintf("%d claude", n))
	if i < 0 {
		t.Fatalf("station %d is not in the strip: %q", n, names)
	}
	return len([]rune(names[:i]))
}

func TestClickingAStationInTheStripOpensIt(t *testing.T) {
	m := crowdedOffice(t, 3, 150)
	order := office.Order(m.layout)
	m = press(m, click(stripX(t, m, 2)+1, 2))
	if m.win == nil || m.win.agentID != order[1] {
		t.Fatalf("a click on station 2 should open it, win = %+v", m.win)
	}
	m = press(m, click(stripX(t, m, 3)+1, 4)) // on its state row, from inside a window
	if m.win == nil || m.win.agentID != order[2] {
		t.Errorf("a click on station 3 should switch to it, win = %+v", m.win)
	}
}

func TestTheMarkersCountTheStationsOutOfView(t *testing.T) {
	m := crowdedOffice(t, 6, 70)
	m, _ = m.openWindow(office.Order(m.layout)[5])
	names, _ := stripRows(m)
	shown := strings.Count(names, " claude")
	if want := fmt.Sprintf("‹ +%d", 6-shown); !strings.HasPrefix(names, want) {
		t.Errorf("the left marker should read %q: %q", want, names)
	}
	if strings.Contains(names, "›") {
		t.Errorf("nothing is hidden on the right: %q", names)
	}
}

func wheelAt(x, y int, b tea.MouseButton) tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: x, Y: y, Button: b}
}

func TestTheWheelOverTheStripScrollsIt(t *testing.T) {
	m := crowdedOffice(t, 6, 70)
	before, _ := stripRows(m)
	if !strings.Contains(before, "1 claude") || !strings.Contains(before, "›") {
		t.Fatalf("the strip should start at station 1 with more on the right: %q", before)
	}
	m = press(m, wheelAt(10, 3, tea.MouseWheelDown))
	names, _ := stripRows(m)
	if strings.Contains(names, "1 claude") || !strings.HasPrefix(names, "‹ +1") {
		t.Errorf("the wheel should move the strip one station right: %q", names)
	}
	m = press(m, wheelAt(10, 3, tea.MouseWheelRight))
	m = press(m, wheelAt(10, 3, tea.MouseWheelLeft))
	m = press(m, wheelAt(10, 3, tea.MouseWheelUp))
	if names, _ := stripRows(m); names != before {
		t.Errorf("scrolling back should restore the strip:\n%q\nwant\n%q", names, before)
	}
}

func TestClickingAMarkerScrollsTheStrip(t *testing.T) {
	m := crowdedOffice(t, 6, 70)
	m = press(m, click(69, 2)) // the › marker in the last cell
	names, _ := stripRows(m)
	if strings.Contains(names, "1 claude") || m.win != nil {
		t.Fatalf("a click on › should scroll, not open a station: %q", names)
	}
	m = press(m, click(0, 2)) // the ‹ marker
	if names, _ := stripRows(m); !strings.Contains(names, "1 claude") {
		t.Errorf("a click on ‹ should scroll back: %q", names)
	}
}

func TestOpeningAStationBringsTheStripBackToIt(t *testing.T) {
	m := crowdedOffice(t, 6, 70)
	for range 5 {
		m = press(m, wheelAt(10, 3, tea.MouseWheelDown))
	}
	m = prefixed(m, "1")
	if names, _ := stripRows(m); !strings.Contains(names, "1 claude") {
		t.Errorf("the strip should follow the station just opened: %q", names)
	}
}
