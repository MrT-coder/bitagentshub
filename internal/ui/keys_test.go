package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/office"
)

// prefixed presses the prefix (ctrl+a by default) and then k.
func prefixed(m Model, k string) Model {
	return press(press(m, key("ctrl+a")), key(k))
}

func TestPrefixKeysRunWindowActions(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = prefixed(m, "c")
	if len(rec.started) != 2 || m.win.focus != 1 {
		t.Fatalf("ctrl+a c should open a terminal and focus it: started %d, focus %d", len(rec.started), m.win.focus)
	}
	m = prefixed(m, "h")
	if m.win.focus != 0 {
		t.Errorf("ctrl+a h should focus the pane on the left, focus %d", m.win.focus)
	}
	m = prefixed(m, "l")
	if m.win.focus != 1 {
		t.Errorf("ctrl+a l should focus the pane on the right, focus %d", m.win.focus)
	}
	m = prefixed(m, "z")
	if !m.win.max {
		t.Error("ctrl+a z should maximize the focused pane")
	}
	m = prefixed(m, "x")
	if len(m.win.panes) != 1 || !rec.started[1].closed {
		t.Errorf("ctrl+a x should close the focused terminal: panes %d", len(m.win.panes))
	}
	m = prefixed(m, "d")
	if m.win != nil {
		t.Error("ctrl+a d should close the window")
	}
	for _, f := range rec.started {
		if len(f.keys) != 0 {
			t.Errorf("prefix keys reached a program: %v", f.keys)
		}
	}
}

func TestPrefixTwiceSendsItAndUnboundKeysAreDropped(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("ctrl+a"))
	if !strings.Contains(view(m), "ctrl+a") || !strings.Contains(view(m), "waiting") {
		t.Errorf("the footer should show the prefix is waiting:\n%s", view(m))
	}
	m = press(m, key("ctrl+a"))
	m = prefixed(m, "q")
	m = press(m, key("b"))
	if got := strings.Join(rec.started[0].keys, ","); got != "ctrl+a,b" {
		t.Errorf("program keys = %q, want the prefix sent once and q dropped", got)
	}
	if m.win == nil {
		t.Error("an unbound key after the prefix should not close the window")
	}
}

func TestStationsAreNumberedAndTheNumberJumpsThere(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	order := office.Order(m.layout)
	if len(order) < 2 {
		t.Fatalf("want two stations, got %v", order)
	}
	for i, id := range order {
		a, _ := m.agent(id)
		if label := itoa(i+1) + " " + a.Project; !strings.Contains(view(m), label) {
			t.Errorf("station %d should show %q on the floor:\n%s", i+1, label, view(m))
		}
	}
	m = prefixed(m, "2")
	if m.win == nil || m.win.agentID != order[1] {
		t.Fatalf("ctrl+a 2 on the board should open station 2 (%s)", order[1])
	}
	m = prefixed(m, "1")
	if m.win == nil || m.win.agentID != order[0] {
		t.Fatalf("ctrl+a 1 in a window should close it and open station 1 (%s)", order[0])
	}
	m = prefixed(m, "9")
	if m.win == nil || m.win.agentID != order[0] || !strings.Contains(view(m), "No station 9") {
		t.Errorf("a missing station keeps the window and says so:\n%s", view(m))
	}
}

func itoa(i int) string { return string(rune('0' + i)) }

func TestFKeysStillRunWindowActions(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("f3"))
	if len(rec.started) != 2 {
		t.Fatalf("f3 should still open a terminal, started %d", len(rec.started))
	}
	m = press(m, key("f10"))
	if m.win != nil {
		t.Error("f10 should still close the window")
	}
}

func TestBindingsFromTheConfigFileReplaceTheDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[keys]\nprefix = \"ctrl+b\"\nnew_terminal = \"prefix+t\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m, _, rec := twoAgentOffice(t)
	m = m.WithKeys(cfg.Keys)
	m = press(m, key("enter"))
	m = press(press(m, key("ctrl+b")), key("t"))
	if len(rec.started) != 2 {
		t.Errorf("ctrl+b t should open a terminal, started %d", len(rec.started))
	}
	m = press(m, key("ctrl+a"))
	if got := strings.Join(rec.started[1].keys, ","); got != "ctrl+a" {
		t.Errorf("ctrl+a is no longer the prefix and should reach the program, keys %q", got)
	}
	if !strings.Contains(view(m), "ctrl+b") {
		t.Errorf("the footer should show the configured prefix:\n%s", view(m))
	}
}

func TestPrefixKeysOnTheBoardDisarmTheQuitConfirmation(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(m, key("t"))   // a local terminal now runs
	m = press(m, key("f10")) // back to the office
	m = press(m, key("q"))   // armed: "press q again"
	m = prefixed(m, "2")
	m = press(m, key("f10"))
	if strings.Contains(view(m), "press q again") {
		t.Errorf("the quit warning should be gone after a prefix key:\n%s", view(m))
	}
	if _, cmd := m.Update(key("q")); cmd != nil && cmd() == tea.Quit() {
		t.Error("q after a prefix key should ask again, not quit")
	}
}

func TestAPasteCancelsAWaitingPrefix(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(m, key("enter"))
	m = press(m, key("ctrl+a"))
	m = press(m, tea.PasteMsg{Content: "hi"})
	m = press(m, key("d"))
	if m.win == nil {
		t.Fatal("d after a paste should be typed, not close the window")
	}
	if got := strings.Join(rec.started[0].keys, ","); got != "paste:hi,d" {
		t.Errorf("program keys = %q", got)
	}
}
