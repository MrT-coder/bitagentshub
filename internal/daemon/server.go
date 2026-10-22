package daemon

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// frameTime gathers a burst of output into one push, like the office does.
const frameTime = 16 * time.Millisecond

// Server hosts programs and serves offices.
type Server struct {
	start  StartFunc
	idle   time.Duration
	notify chan string

	mu      sync.Mutex
	progs   map[string]*hostedProg
	order   []string // start order, so offices list programs stably
	clients map[*client]struct{}

	quit     chan struct{}
	quitOnce sync.Once
}

type hostedProg struct {
	info      Info
	prog      Program
	announced bool // its exit was told to the offices
}

// client is one attached office.
type client struct {
	w     *wire
	out   chan Event
	views map[string]*view // guarded by Server.mu
	dead  bool
}

// view is the viewport an office shows of a program, with the rows it was
// last sent.
type view struct {
	w, h, scroll int
	base         screen.Style
	rows         [][]screen.Span
	meta         Frame
}

// NewServer makes a server that starts programs with start and exits once
// it has had no office and no running program for idle.
func NewServer(start StartFunc, idle time.Duration) *Server {
	return &Server{
		start:   start,
		idle:    idle,
		notify:  make(chan string, 256),
		progs:   map[string]*hostedProg{},
		clients: map[*client]struct{}{},
		quit:    make(chan struct{}),
	}
}

// Serve accepts offices on ln until the server stops or becomes idle; every
// program still running is closed then.
func (s *Server) Serve(ln net.Listener) error {
	go s.pump()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				s.shutdown()
				return
			}
			go s.handle(c)
		}
	}()
	s.armIdle()
	<-s.quit
	_ = ln.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.progs {
		p.prog.Close()
	}
	s.progs = map[string]*hostedProg{}
	for cl := range s.clients {
		s.dropLocked(cl)
	}
	return nil
}

// Stop ends the server and its programs.
func (s *Server) Stop() { s.shutdown() }

func (s *Server) shutdown() { s.quitOnce.Do(func() { close(s.quit) }) }

// armIdle stops the server after the idle time if, by then, no office is
// attached and no program runs.
func (s *Server) armIdle() {
	time.AfterFunc(s.idle, func() {
		s.mu.Lock()
		idle := len(s.clients) == 0 && s.runningLocked() == 0
		s.mu.Unlock()
		if idle {
			s.shutdown()
		}
	})
}

func (s *Server) runningLocked() int {
	n := 0
	for _, p := range s.progs {
		if !p.prog.Exited() {
			n++
		}
	}
	return n
}

func (s *Server) handle(c net.Conn) {
	w := newWire(c)
	var req Request
	if err := w.dec.Decode(&req); err != nil || req.Op != "hello" {
		c.Close()
		return
	}
	if err := w.enc.Encode(Event{Op: "welcome", Proto: Proto, PID: os.Getpid()}); err != nil {
		c.Close()
		return
	}
	other := req.Proto != Proto
	if err := w.dec.Decode(&req); err != nil || other && req.Op != "stop" {
		// An office of another build may only stop this daemon, so a new
		// build can replace it.
		c.Close()
		return
	}
	switch req.Op {
	case "list":
		_ = w.enc.Encode(Event{Op: "sessions", Sessions: s.list()})
	case "start":
		if err := s.startProg(req); err != nil {
			_ = w.enc.Encode(Event{Op: "error", Err: err.Error()})
		} else {
			_ = w.enc.Encode(Event{Op: "ok"})
		}
	case "stop":
		_ = w.enc.Encode(Event{Op: "ok"})
		s.shutdown()
	case "attach":
		s.attach(w)
		return
	default:
		_ = w.enc.Encode(Event{Op: "error", Err: fmt.Sprintf("unknown op %q", req.Op)})
	}
	c.Close()
}

func (s *Server) list() []Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Info, 0, len(s.order))
	for _, id := range s.order {
		p := s.progs[id]
		info := p.info
		info.Exited = p.prog.Exited()
		out = append(out, info)
	}
	return out
}

func (s *Server) startProg(req Request) error {
	if req.ID == "" || len(req.Argv) == 0 {
		return errors.New("start needs an id and a command")
	}
	w, h := req.W, req.H
	if w < 1 || h < 1 {
		w, h = 80, 24
	}
	s.mu.Lock()
	_, taken := s.progs[req.ID]
	s.mu.Unlock()
	if taken {
		return fmt.Errorf("%s is already running", req.ID)
	}
	prog, err := s.start(req.ID, req.Dir, req.Argv, w, h, s.notify)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progs[req.ID] = &hostedProg{info: Info{ID: req.ID, Dir: req.Dir, Argv: req.Argv}, prog: prog}
	s.order = append(s.order, req.ID)
	return nil
}

// attach serves one office until it disconnects.
func (s *Server) attach(w *wire) {
	cl := &client{w: w, out: make(chan Event, 256), views: map[string]*view{}}
	s.mu.Lock()
	s.clients[cl] = struct{}{}
	s.mu.Unlock()
	go func() {
		for ev := range cl.out {
			if err := w.enc.Encode(ev); err != nil {
				_ = w.c.Close()
			}
		}
	}()
	for {
		var req Request
		if err := w.dec.Decode(&req); err != nil {
			break
		}
		s.serveRequest(cl, req)
	}
	s.mu.Lock()
	s.dropLocked(cl)
	s.mu.Unlock()
	s.armIdle()
}

func (s *Server) serveRequest(cl *client, req Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.progs[req.ID]
	if !ok {
		if req.Op != "unview" {
			s.sendLocked(cl, Event{Op: "frame", ID: req.ID, Frame: Frame{Closed: true, Exited: true}})
		}
		return
	}
	switch req.Op {
	case "view":
		v := &view{w: max(req.W, 1), h: max(req.H, 1), scroll: req.Scroll, base: req.Base}
		cl.views[req.ID] = v
		s.sendLocked(cl, Event{Op: "frame", ID: req.ID, Frame: render(p.prog, v, true)})
	case "unview":
		delete(cl.views, req.ID)
	case "key":
		p.prog.Key(req.Key)
	case "paste":
		p.prog.Paste(req.Text)
	case "mouse":
		p.prog.Mouse(req.Mouse, req.X, req.Y)
	case "resize":
		p.prog.Resize(req.W, req.H)
		s.changed(req.ID) // the emulator reflows even if the program draws nothing
	case "close":
		p.prog.Close()
		s.removeLocked(req.ID)
	}
}

// removeLocked forgets a program and tells every office it is gone.
func (s *Server) removeLocked(id string) {
	delete(s.progs, id)
	for i, o := range s.order {
		if o == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	for cl := range s.clients {
		delete(cl.views, id)
		s.sendLocked(cl, Event{Op: "frame", ID: id, Frame: Frame{Closed: true, Exited: true}})
	}
	go s.armIdle()
}

// sendLocked queues an event; an office too slow to keep up is dropped
// rather than stalling the others.
func (s *Server) sendLocked(cl *client, ev Event) {
	if cl.dead {
		return
	}
	select {
	case cl.out <- ev:
	default:
		s.dropLocked(cl)
	}
}

func (s *Server) dropLocked(cl *client) {
	if cl.dead {
		return
	}
	cl.dead = true
	delete(s.clients, cl)
	close(cl.out)
	_ = cl.w.c.Close()
}

func (s *Server) changed(id string) {
	select {
	case s.notify <- id:
	default:
	}
}

// pump turns program output into frames, one push per burst.
func (s *Server) pump() {
	for {
		var first string
		select {
		case first = <-s.notify:
		case <-s.quit:
			return
		}
		ids := map[string]bool{first: true}
		time.Sleep(frameTime)
	drain:
		for {
			select {
			case id := <-s.notify:
				ids[id] = true
			default:
				break drain
			}
		}
		s.publish(ids)
	}
}

func (s *Server) publish(ids map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range ids {
		p, ok := s.progs[id]
		if !ok {
			continue
		}
		exitedNow := p.prog.Exited() && !p.announced
		if exitedNow {
			p.announced = true
		}
		for cl := range s.clients {
			if v, ok := cl.views[id]; ok {
				f := render(p.prog, v, false)
				if len(f.Rows) > 0 || f.metaChanged(v.meta) || exitedNow {
					s.sendLocked(cl, Event{Op: "frame", ID: id, Frame: f})
				}
				v.meta = f
			} else if exitedNow {
				s.sendLocked(cl, Event{Op: "frame", ID: id, Frame: state(p.prog)})
			}
		}
		if exitedNow && len(s.clients) == 0 {
			// Nobody watches: an office reattaching later has no use for it.
			p.prog.Close()
			s.removeLocked(id)
		}
	}
}

// render draws v of prog and returns the rows that differ from what was
// last sent (all of them when full), remembering them in v.
func render(prog Program, v *view, full bool) Frame {
	buf := screen.New(v.w, v.h)
	prog.Draw(buf, 0, 0, v.w, v.h, v.scroll, v.base)
	f := state(prog)
	f.W, f.H, f.Scroll, f.Base, f.Full = v.w, v.h, v.scroll, v.base, full
	f.Rows = map[int][]screen.Span{}
	if full || len(v.rows) != v.h {
		v.rows = make([][]screen.Span, v.h)
		f.Full = true
	}
	for y := 0; y < v.h; y++ {
		row := buf.Spans(y)
		if f.Full || !spansEqual(row, v.rows[y]) {
			f.Rows[y] = row
			v.rows[y] = row
		}
	}
	if full {
		v.meta = f
	}
	return f
}

func state(prog Program) Frame {
	w, h := prog.Size()
	return Frame{SizeW: w, SizeH: h, Scrollback: prog.ScrollbackLen(), Alt: prog.AltScreen(), Exited: prog.Exited()}
}

func (f Frame) metaChanged(old Frame) bool {
	return f.SizeW != old.SizeW || f.SizeH != old.SizeH || f.Scrollback != old.Scrollback || f.Alt != old.Alt || f.Exited != old.Exited
}

func spansEqual(a, b []screen.Span) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
