package ui

import (
	"encoding/gob"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/daemon"
	"github.com/MrT-coder/bitagentshub/internal/screen"
	"github.com/MrT-coder/bitagentshub/internal/term"
)

// daemonProg is a program inside a test daemon.
type daemonProg struct {
	mu     sync.Mutex
	argv   []string
	closed bool
}

func (p *daemonProg) Key(tea.KeyPressMsg)            {}
func (p *daemonProg) Paste(string)                   {}
func (p *daemonProg) Mouse(term.MouseKind, int, int) {}
func (p *daemonProg) Resize(int, int)                {}
func (p *daemonProg) Size() (int, int)               { return 80, 24 }
func (p *daemonProg) ScrollbackLen() int             { return 0 }
func (p *daemonProg) AltScreen() bool                { return false }
func (p *daemonProg) Draw(buf *screen.Buffer, x, y, w, h, _ int, base screen.Style) {
	buf.Text(x, y, "running "+strings.Join(p.argv, " "), base)
}
func (p *daemonProg) Exited() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}
func (p *daemonProg) Close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
}

var daemonSeq int

func testDaemonAddr(t *testing.T) string {
	daemonSeq++
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\bah-uitest-%d-%d`, os.Getpid(), daemonSeq)
	}
	// A short path: macOS refuses socket paths over 104 bytes.
	dir, err := os.MkdirTemp("", "bah")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "d", "daemon.sock")
}

func runDaemon(t *testing.T) string {
	t.Helper()
	addr := testDaemonAddr(t)
	ln, err := daemon.Listen(addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := daemon.NewServer(func(id, dir string, argv []string, w, h int, notify chan<- string) (daemon.Program, error) {
		return &daemonProg{argv: argv}, nil
	}, time.Minute)
	done := make(chan struct{})
	go func() { _ = srv.Serve(ln); close(done) }()
	t.Cleanup(func() { srv.Stop(); <-done })
	return addr
}

func listed(t *testing.T, addr string) []daemon.Info {
	t.Helper()
	_, list, err := daemon.List(addr)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestAgentsAndTerminalsOutliveTheOfficeInTheDaemon(t *testing.T) {
	addr := runDaemon(t)
	m, _, rec := twoAgentOffice(t)
	m = m.WithDaemon(addr)
	m = press(m, key("t")) // a terminal in the front folder
	m = press(m, key("f10"))
	agentDir := `C:\w\hobbylabs\minigames`
	m, _ = m.startLocal(agentDir)
	m = press(m, key("f10"))
	if len(hostedIn(rec)) != 0 {
		t.Fatalf("terminals ran inside the office instead of the daemon: %v", hostedIn(rec))
	}
	list := listed(t, addr)
	if len(list) != 2 || !strings.HasPrefix(list[0].ID, shellPrefix) || list[0].Dir != frontDir || list[1].Dir != agentDir {
		t.Fatalf("daemon runs %+v", list)
	}
	agentID := list[1].ID
	if got := strings.Join(list[1].Argv, " "); got != "claude --session-id "+agentID {
		t.Errorf("agent argv = %q", got)
	}

	// Nothing ends with the office, so q quits at once.
	if _, cmd := m.Update(key("q")); cmd == nil || cmd() != tea.Quit() {
		t.Fatal("q should quit without asking when everything lives in the daemon")
	}
	m.CloseAll()
	for _, s := range listed(t, addr) {
		if s.Exited {
			t.Fatalf("quitting ended %s", s.ID)
		}
	}

	// A new office finds both again.
	m2, _, rec2 := twoAgentOffice(t)
	m2 = m2.WithDaemon(addr)
	defer m2.CloseAll()
	found := false
	for _, a := range m2.agents() {
		if a.ID == agentID && a.Cwd == agentDir {
			found = true
		}
	}
	if !found {
		t.Errorf("the agent did not walk back in: %+v", m2.agents())
	}
	if !strings.Contains(view(m2), "Reattached 2") {
		t.Errorf("the office should say it reattached:\n%s", view(m2))
	}
	m2 = update(m2, key("enter")) // front's window, with its terminal drawn by the daemon
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(view(m2), "running pwsh") {
		if time.Now().After(deadline) {
			t.Fatalf("the folder's terminal did not come back:\n%s", view(m2))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(hostedIn(rec2)) != 0 {
		t.Errorf("reattaching started programs: %v", hostedIn(rec2))
	}
}

func TestAnIncompatibleDaemonLeavesTerminalsInTheOffice(t *testing.T) {
	addr := testDaemonAddr(t)
	ln, err := daemon.Listen(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go answerAsOtherVersion(c)
		}
	}()
	m, _, rec := twoAgentOffice(t)
	m = m.WithDaemon(addr)
	if !strings.Contains(view(m), "bah daemon stop") {
		t.Errorf("the office should say how to replace the daemon:\n%s", view(m))
	}
	m = press(m, key("t"))
	if len(hostedIn(rec)) != 1 {
		t.Errorf("the terminal should run inside the office: %v", hostedIn(rec))
	}
}

// answerAsOtherVersion greets like a daemon of a future build.
func answerAsOtherVersion(c net.Conn) {
	defer c.Close()
	var hello daemon.Request
	_ = gob.NewDecoder(c).Decode(&hello)
	_ = gob.NewEncoder(c).Encode(daemon.Event{Op: "welcome", Proto: daemon.Proto + 1, PID: 42})
}

// hostedIn lists the agents and terminals the office ran itself; herdr
// attaches always run there.
func hostedIn(rec *termRecorder) []string {
	var out []string
	for _, ft := range rec.started {
		if len(ft.argv) > 0 && ft.argv[0] != "herdr" {
			out = append(out, strings.Join(ft.argv, " "))
		}
	}
	return out
}

func TestTerminalsRunInTheOfficeOnceTheDaemonIsGone(t *testing.T) {
	addr := runDaemon(t)
	m, _, rec := twoAgentOffice(t)
	m = m.WithDaemon(addr)
	defer m.CloseAll()
	if err := daemon.Stop(addr); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !m.daemon.Dead() {
		if time.Now().After(deadline) {
			t.Fatal("the office never noticed the daemon left")
		}
		time.Sleep(10 * time.Millisecond)
	}
	m = press(m, key("t"))
	if len(hostedIn(rec)) != 1 {
		t.Errorf("the terminal should start inside the office: %v\n%s", hostedIn(rec), view(m))
	}
}
