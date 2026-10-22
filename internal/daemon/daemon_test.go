package daemon

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/screen"
	"github.com/MrT-coder/bitagentshub/internal/term"
)

// fakeProg stands in for a program in a pseudo terminal: it shows the lines
// it was given and records what it receives.
type fakeProg struct {
	id     string
	notify chan<- string

	mu       sync.Mutex
	lines    []string
	got      []string
	w, h     int
	exited   bool
	closed   bool
	sbLen    int
	drawSeen int
}

func (p *fakeProg) record(s string) {
	p.mu.Lock()
	p.got = append(p.got, s)
	p.mu.Unlock()
}

func (p *fakeProg) received() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.got...)
}

func (p *fakeProg) show(lines ...string) {
	p.mu.Lock()
	p.lines = lines
	p.mu.Unlock()
	p.notify <- p.id
}

func (p *fakeProg) exit() {
	p.mu.Lock()
	p.exited = true
	p.mu.Unlock()
	p.notify <- p.id
}

func (p *fakeProg) Key(msg tea.KeyPressMsg) { p.record("key " + msg.String()) }
func (p *fakeProg) Paste(text string)       { p.record("paste " + text) }
func (p *fakeProg) Mouse(kind term.MouseKind, x, y int) {
	p.record(fmt.Sprintf("mouse %d %d,%d", kind, x, y))
}
func (p *fakeProg) Draw(buf *screen.Buffer, x, y, w, h, scroll int, base screen.Style, pal screen.Palette) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, l := range p.lines {
		if i < h {
			buf.Text(x, y+i, l, base)
		}
	}
	if scroll > 0 {
		buf.Text(x, y+h-1, fmt.Sprintf("scrolled %d", scroll), base)
	}
	if pal.Set {
		buf.Text(x, y+h-1, fmt.Sprintf("red %02x%02x%02x", pal.C[1].R, pal.C[1].G, pal.C[1].B), base)
	}
}
func (p *fakeProg) Resize(w, h int) {
	p.mu.Lock()
	p.w, p.h = w, h
	p.mu.Unlock()
	p.record(fmt.Sprintf("resize %dx%d", w, h))
}
func (p *fakeProg) Size() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.w, p.h
}
func (p *fakeProg) ScrollbackLen() int { return p.sbLen }
func (p *fakeProg) AltScreen() bool    { return false }
func (p *fakeProg) Exited() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exited || p.closed
}
func (p *fakeProg) Close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
}

type fixture struct {
	addr  string
	srv   *Server
	done  chan error
	mu    sync.Mutex
	progs map[string]*fakeProg
}

func (f *fixture) prog(t *testing.T, id string) *fakeProg {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.progs[id]
	if !ok {
		t.Fatalf("program %s was never started", id)
	}
	return p
}

var addrSeq int

func serve(t *testing.T, idle time.Duration) *fixture {
	t.Helper()
	addrSeq++
	f := &fixture{progs: map[string]*fakeProg{}, done: make(chan error, 1)}
	f.addr = testAddr(t, addrSeq)
	start := func(id, dir string, argv []string, w, h int, notify chan<- string) (Program, error) {
		p := &fakeProg{id: id, notify: notify, w: w, h: h, lines: []string{dir + " $ " + strings.Join(argv, " ")}}
		f.mu.Lock()
		f.progs[id] = p
		f.mu.Unlock()
		return p, nil
	}
	ln, err := Listen(f.addr)
	if err != nil {
		t.Fatal(err)
	}
	f.srv = NewServer(start, idle)
	go func() { f.done <- f.srv.Serve(ln) }()
	t.Cleanup(func() {
		f.srv.Stop()
		<-f.done
	})
	return f
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func screenText(r *Remote, w, h int) string {
	buf := screen.New(w, h)
	r.Draw(buf, 0, 0, w, h, 0, screen.Style{}, screen.Palette{})
	var lines []string
	for y := 0; y < h; y++ {
		var b strings.Builder
		for _, sp := range buf.Spans(y) {
			b.WriteString(sp.Text)
		}
		lines = append(lines, strings.TrimRight(b.String(), " "))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// shows draws r until its screen shows want.
func shows(t *testing.T, r *Remote, w, h int, want string) {
	t.Helper()
	var got string
	deadline := time.Now().Add(3 * time.Second)
	for {
		if got = screenText(r, w, h); got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("screen = %q, want %q", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStartedProgramIsListedAndShownToTheOffice(t *testing.T) {
	f := serve(t, time.Minute)
	if err := Start(f.addr, "a1", `C:\w\app`, []string{"claude", "--session-id", "a1"}, 40, 5); err != nil {
		t.Fatal(err)
	}
	_, list, err := List(f.addr)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "a1" || list[0].Dir != `C:\w\app` || list[0].Exited {
		t.Fatalf("list = %+v", list)
	}
	notify := make(chan string, 16)
	c, err := Attach(f.addr, notify)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r := c.Terminal("a1")
	shows(t, r, 40, 5, `C:\w\app $ claude --session-id a1`)
	f.prog(t, "a1").show("hello", "world")
	shows(t, r, 40, 5, "hello\nworld")
	if w, h := r.Size(); w != 40 || h != 5 {
		t.Errorf("size = %dx%d", w, h)
	}
}

func TestInputReachesTheProgram(t *testing.T) {
	f := serve(t, time.Minute)
	if err := Start(f.addr, "a1", `C:\w`, []string{"pwsh"}, 20, 4); err != nil {
		t.Fatal(err)
	}
	c, err := Attach(f.addr, make(chan string, 16))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r := c.Terminal("a1")
	r.Key(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	r.Paste("two\nlines")
	r.Mouse(term.MouseWheelUp, 3, 2)
	r.Resize(30, 6)
	p := f.prog(t, "a1")
	want := []string{"key ctrl+c", "paste two\nlines", "mouse 0 3,2", "resize 30x6"}
	waitFor(t, "input", func() bool { return len(p.received()) == len(want) })
	for i, got := range p.received() {
		if got != want[i] {
			t.Errorf("input %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestProgramsSurviveTheOfficeDisconnecting(t *testing.T) {
	f := serve(t, 50*time.Millisecond)
	if err := Start(f.addr, "a1", `C:\w`, []string{"claude"}, 20, 3); err != nil {
		t.Fatal(err)
	}
	c, err := Attach(f.addr, make(chan string, 16))
	if err != nil {
		t.Fatal(err)
	}
	f.prog(t, "a1").show("before quit")
	shows(t, c.Terminal("a1"), 20, 3, "before quit")
	c.Terminal("a1").Detach()
	c.Close()

	// Longer than the idle time: a running program keeps the daemon up.
	time.Sleep(200 * time.Millisecond)
	f.prog(t, "a1").show("while away")
	c2, err := Attach(f.addr, make(chan string, 16))
	if err != nil {
		t.Fatalf("daemon went away with a program running: %v", err)
	}
	defer c2.Close()
	shows(t, c2.Terminal("a1"), 20, 3, "while away")
	if f.prog(t, "a1").Exited() {
		t.Error("detaching ended the program")
	}
}

func TestScrollingAsksTheDaemonForThatViewport(t *testing.T) {
	f := serve(t, time.Minute)
	if err := Start(f.addr, "a1", `C:\w`, []string{"pwsh"}, 20, 3); err != nil {
		t.Fatal(err)
	}
	c, err := Attach(f.addr, make(chan string, 16))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r := c.Terminal("a1")
	f.prog(t, "a1").show("top")
	shows(t, r, 20, 3, "top")
	var got string
	waitFor(t, "scrolled view", func() bool {
		buf := screen.New(20, 3)
		r.Draw(buf, 0, 0, 20, 3, 4, screen.Style{}, screen.Palette{})
		var b strings.Builder
		for _, sp := range buf.Spans(2) {
			b.WriteString(sp.Text)
		}
		got = strings.TrimSpace(b.String())
		return got == "scrolled 4"
	})
}

func TestThePaletteReachesTheProgram(t *testing.T) {
	f := serve(t, time.Minute)
	if err := Start(f.addr, "a1", `C:\w`, []string{"pwsh"}, 20, 3); err != nil {
		t.Fatal(err)
	}
	c, err := Attach(f.addr, make(chan string, 16))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r := c.Terminal("a1")
	f.prog(t, "a1").show("top")
	shows(t, r, 20, 3, "top")
	var pal screen.Palette
	pal.Set = true
	pal.C[1] = screen.RGB{R: 0xcb, G: 0x7c, B: 0x94}
	var got string
	waitFor(t, "view with the palette", func() bool {
		buf := screen.New(20, 3)
		r.Draw(buf, 0, 0, 20, 3, 0, screen.Style{}, pal)
		var b strings.Builder
		for _, sp := range buf.Spans(2) {
			b.WriteString(sp.Text)
		}
		got = strings.TrimSpace(b.String())
		return got == "red cb7c94"
	})
}

func TestExitAndCloseReachTheOffice(t *testing.T) {
	f := serve(t, time.Minute)
	for _, id := range []string{"a1", "a2"} {
		if err := Start(f.addr, id, `C:\w`, []string{"claude"}, 20, 3); err != nil {
			t.Fatal(err)
		}
	}
	notify := make(chan string, 16)
	c, err := Attach(f.addr, notify)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	a1, a2 := c.Terminal("a1"), c.Terminal("a2")

	// Not viewed: the office still learns the program ended.
	f.prog(t, "a1").exit()
	waitFor(t, "exit of a1", a1.Exited)

	a2.Close()
	waitFor(t, "a2 closed in the daemon", func() bool { return f.prog(t, "a2").Exited() })
	waitFor(t, "a2 gone from the list", func() bool {
		_, list, err := List(f.addr)
		return err == nil && len(list) == 1 && list[0].ID == "a1"
	})
}

func TestDaemonExitsWhenIdleOnly(t *testing.T) {
	f := serve(t, 50*time.Millisecond)
	select {
	case <-f.done:
		f.done <- nil // for the cleanup
	case <-time.After(2 * time.Second):
		t.Fatal("a daemon with nothing to do kept running")
	}
	if _, _, err := List(f.addr); err == nil {
		t.Error("the stopped daemon still answers")
	}
}

func TestStartRejectsADuplicateID(t *testing.T) {
	f := serve(t, time.Minute)
	if err := Start(f.addr, "a1", `C:\w`, []string{"claude"}, 20, 3); err != nil {
		t.Fatal(err)
	}
	if err := Start(f.addr, "a1", `C:\w`, []string{"claude"}, 20, 3); err == nil {
		t.Error("a second program with the same id was started")
	}
}

func TestOfficeRefusesADaemonOfAnotherVersion(t *testing.T) {
	addrSeq++
	addr := testAddr(t, addrSeq)
	ln, err := Listen(addr)
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
			go func(c net.Conn) {
				w := newWire(c)
				var req Request
				_ = w.dec.Decode(&req)
				_ = w.enc.Encode(Event{Op: "welcome", Proto: Proto + 1, PID: 42})
				c.Close()
			}(c)
		}
	}()
	_, _, err = List(addr)
	if !errors.Is(err, ErrProto) {
		t.Fatalf("err = %v, want ErrProto", err)
	}
	if err := Ensure(addr); !errors.Is(err, ErrProto) {
		t.Errorf("Ensure err = %v, want ErrProto without starting another daemon", err)
	}
}

func TestDaemonGoingAwayEndsItsTerminals(t *testing.T) {
	f := serve(t, time.Minute)
	if err := Start(f.addr, "a1", `C:\w`, []string{"claude"}, 20, 3); err != nil {
		t.Fatal(err)
	}
	notify := make(chan string, 16)
	c, err := Attach(f.addr, notify)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r := c.Terminal("a1")
	if r.Exited() {
		t.Fatal("exited before the daemon stopped")
	}
	if err := Stop(f.addr); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "terminal ended", r.Exited)
	if !f.prog(t, "a1").Exited() {
		t.Error("stopping the daemon left its program running")
	}
}

func TestAnOfficeOfAnotherBuildCanOnlyStopTheDaemon(t *testing.T) {
	f := serve(t, time.Minute)
	if err := Start(f.addr, "a1", `C:\w`, []string{"claude"}, 20, 3); err != nil {
		t.Fatal(err)
	}
	other := func(op string) error {
		c, err := dialAddr(f.addr, dialTimeout)
		if err != nil {
			return err
		}
		defer c.Close()
		w := newWire(c)
		if err := w.enc.Encode(Request{Op: "hello", Proto: Proto + 1}); err != nil {
			return err
		}
		var ev Event
		if err := w.dec.Decode(&ev); err != nil {
			return err
		}
		if err := w.enc.Encode(Request{Op: op}); err != nil {
			return err
		}
		return w.dec.Decode(&ev)
	}
	if err := other("list"); err == nil {
		t.Error("another build was answered a list")
	}
	if err := other("stop"); err != nil {
		t.Fatalf("stop from another build: %v", err)
	}
	select {
	case <-f.done:
		f.done <- nil
	case <-time.After(2 * time.Second):
		t.Fatal("the daemon kept running")
	}
	if !f.prog(t, "a1").Exited() {
		t.Error("stopping left the program running")
	}
}

func TestAnAttachedOfficeOutlastsTheRequestTimeout(t *testing.T) {
	old := callTimeout
	callTimeout = 100 * time.Millisecond
	defer func() { callTimeout = old }()
	f := serve(t, time.Minute)
	if err := Start(f.addr, "a1", `C:\w`, []string{"claude"}, 20, 3); err != nil {
		t.Fatal(err)
	}
	c, err := Attach(f.addr, make(chan string, 16))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r := c.Terminal("a1")
	time.Sleep(300 * time.Millisecond)
	f.prog(t, "a1").show("still here")
	shows(t, r, 20, 3, "still here")
	if c.Dead() {
		t.Error("the attached connection timed out")
	}
}
