package daemon

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-runewidth"

	"github.com/MrT-coder/bitagentshub/internal/screen"
	"github.com/MrT-coder/bitagentshub/internal/term"
)

const dialTimeout = time.Second

// callTimeout bounds a one-shot request, so a hung daemon cannot hang the
// office or the daemon command.
var callTimeout = 10 * time.Second

func dial(addr string) (*wire, Event, error) {
	c, err := dialAddr(addr, dialTimeout)
	if err != nil {
		return nil, Event{}, err
	}
	_ = c.SetDeadline(time.Now().Add(callTimeout))
	w := newWire(c)
	ev, err := w.hello()
	if err != nil {
		c.Close()
		return nil, ev, err
	}
	return w, ev, nil
}

// call sends one request and returns the daemon's answer.
func call(addr string, req Request) (Event, error) {
	w, _, err := dial(addr)
	if err != nil {
		return Event{}, err
	}
	defer w.c.Close()
	if err := w.enc.Encode(req); err != nil {
		return Event{}, err
	}
	var ev Event
	if err := w.dec.Decode(&ev); err != nil {
		return Event{}, err
	}
	if ev.Op == "error" {
		return ev, errors.New(ev.Err)
	}
	return ev, nil
}

// List returns the daemon's pid and its programs.
func List(addr string) (pid int, sessions []Info, err error) {
	w, hello, err := dial(addr)
	if err != nil {
		return hello.PID, nil, err
	}
	defer w.c.Close()
	if err := w.enc.Encode(Request{Op: "list"}); err != nil {
		return 0, nil, err
	}
	var ev Event
	if err := w.dec.Decode(&ev); err != nil {
		return 0, nil, err
	}
	return hello.PID, ev.Sessions, nil
}

// Start runs argv in dir as program id inside the daemon.
func Start(addr, id, dir string, argv []string, w, h int) error {
	_, err := call(addr, Request{Op: "start", ID: id, Dir: dir, Argv: argv, W: w, H: h})
	return err
}

// Stop ends the daemon and every program it runs, whatever build it is.
func Stop(addr string) error {
	c, err := dialAddr(addr, dialTimeout)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(callTimeout))
	w := newWire(c)
	if _, err := w.hello(); err != nil && !errors.Is(err, ErrProto) {
		return err
	}
	if err := w.enc.Encode(Request{Op: "stop"}); err != nil {
		return err
	}
	var ev Event
	return w.dec.Decode(&ev)
}

// Conn is an office's attached connection; its terminals share it.
type Conn struct {
	w      *wire
	notify chan<- string
	wmu    sync.Mutex
	dead   atomic.Bool

	mu    sync.Mutex
	terms map[string]*Remote
}

// Attach connects an office. notify gets a program's id whenever what the
// office knows of it changes.
func Attach(addr string, notify chan<- string) (*Conn, error) {
	w, _, err := dial(addr)
	if err != nil {
		return nil, err
	}
	if err := w.enc.Encode(Request{Op: "attach"}); err != nil {
		w.c.Close()
		return nil, err
	}
	if nc, ok := w.c.(net.Conn); ok {
		_ = nc.SetDeadline(time.Time{}) // an attached office waits for output indefinitely
	}
	c := &Conn{w: w, notify: notify, terms: map[string]*Remote{}}
	go c.read()
	return c, nil
}

// Close detaches; the daemon keeps the programs running.
func (c *Conn) Close() { _ = c.w.c.Close() }

// Dead reports whether the daemon went away.
func (c *Conn) Dead() bool { return c.dead.Load() }

// Terminal is the office's handle on program id.
func (c *Conn) Terminal(id string) *Remote {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r, ok := c.terms[id]; ok {
		return r
	}
	r := &Remote{c: c, id: id}
	c.terms[id] = r
	return r
}

func (c *Conn) forget(id string) {
	c.mu.Lock()
	delete(c.terms, id)
	c.mu.Unlock()
}

func (c *Conn) send(req Request) {
	if c.dead.Load() {
		return
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if err := c.w.enc.Encode(req); err != nil {
		_ = c.w.c.Close()
	}
}

func (c *Conn) read() {
	for {
		var ev Event
		if err := c.w.dec.Decode(&ev); err != nil {
			break
		}
		if ev.Op != "frame" {
			continue
		}
		c.mu.Lock()
		r := c.terms[ev.ID]
		c.mu.Unlock()
		if r != nil {
			r.apply(ev.Frame)
			c.wake(ev.ID)
		}
	}
	// The daemon is gone, and its programs with it.
	c.dead.Store(true)
	c.mu.Lock()
	ids := make([]string, 0, len(c.terms))
	for id := range c.terms {
		ids = append(ids, id)
	}
	c.mu.Unlock()
	for _, id := range ids {
		c.wake(id)
	}
}

func (c *Conn) wake(id string) {
	select {
	case c.notify <- id:
	default:
	}
}

// Remote is a program in the daemon, shown and driven like a local
// terminal.
type Remote struct {
	c  *Conn
	id string

	mu       sync.Mutex
	asked    bool
	want     view // the viewport last asked for
	rows     [][]screen.Span
	rw, rh   int
	state    Frame
	closed   bool
	detached bool
}

func (r *Remote) apply(f Frame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if f.Closed {
		r.closed = true
	}
	r.state.SizeW, r.state.SizeH, r.state.Scrollback, r.state.Alt = f.SizeW, f.SizeH, f.Scrollback, f.Alt
	r.state.Exited = r.state.Exited || f.Exited
	if f.W == 0 {
		return
	}
	if f.Full || f.W != r.rw || f.H != r.rh {
		r.rows, r.rw, r.rh = make([][]screen.Span, f.H), f.W, f.H
	}
	for y, row := range f.Rows {
		if y >= 0 && y < len(r.rows) {
			r.rows[y] = row
		}
	}
}

// Draw paints the last rows the daemon sent, and asks for another viewport
// when the box, scroll, style or palette changed; the answer repaints.
func (r *Remote) Draw(buf *screen.Buffer, x, y, w, h, scroll int, base screen.Style, pal screen.Palette) {
	r.mu.Lock()
	ask := !r.asked || r.want.w != w || r.want.h != h || r.want.scroll != scroll || r.want.base != base || r.want.pal != pal
	if ask {
		r.asked, r.want = true, view{w: w, h: h, scroll: scroll, base: base, pal: pal}
	}
	rows := r.rows
	r.mu.Unlock()
	if ask {
		r.c.send(Request{Op: "view", ID: r.id, W: w, H: h, Scroll: scroll, Base: base, Palette: pal})
	}
	for row := 0; row < h && row < len(rows); row++ {
		col := 0
		for _, sp := range rows[row] {
			text := sp.Text
			if col+runewidth.StringWidth(text) > w {
				text = runewidth.Truncate(text, w-col, "")
			}
			if text == "" {
				break
			}
			col = buf.Text(x+col, y+row, text, sp.Style) - x
		}
	}
}

func (r *Remote) Key(msg tea.KeyPressMsg) { r.c.send(Request{Op: "key", ID: r.id, Key: msg}) }

func (r *Remote) Paste(text string) { r.c.send(Request{Op: "paste", ID: r.id, Text: text}) }

func (r *Remote) Mouse(kind term.MouseKind, x, y int) {
	r.c.send(Request{Op: "mouse", ID: r.id, Mouse: kind, X: x, Y: y})
}

func (r *Remote) Resize(w, h int) {
	if w < 1 || h < 1 {
		return
	}
	r.mu.Lock()
	same := w == r.state.SizeW && h == r.state.SizeH
	r.state.SizeW, r.state.SizeH = w, h
	r.mu.Unlock()
	if !same {
		r.c.send(Request{Op: "resize", ID: r.id, W: w, H: h})
	}
}

func (r *Remote) Size() (w, h int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state.SizeW == 0 {
		return r.want.w, r.want.h
	}
	return r.state.SizeW, r.state.SizeH
}

func (r *Remote) ScrollbackLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.Scrollback
}

func (r *Remote) AltScreen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.Alt
}

// Exited reports whether the program ended, was closed, or the daemon went
// away.
func (r *Remote) Exited() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.Exited || r.closed || r.c.Dead()
}

// Close ends the program.
func (r *Remote) Close() {
	r.mu.Lock()
	done := r.closed || r.detached
	r.closed = true
	r.mu.Unlock()
	if !done {
		r.c.send(Request{Op: "close", ID: r.id})
	}
	r.c.forget(r.id)
}

// Detach stops showing the program and leaves it running in the daemon.
func (r *Remote) Detach() {
	r.mu.Lock()
	done := r.closed || r.detached
	r.detached = true
	r.mu.Unlock()
	if !done {
		r.c.send(Request{Op: "unview", ID: r.id})
	}
	r.c.forget(r.id)
}
