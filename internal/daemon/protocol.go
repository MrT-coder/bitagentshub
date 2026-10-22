// Package daemon keeps the office's agents and terminals alive in a
// background process, so quitting, reconnecting over SSH or rebuilding the
// office does not end them.
//
// The daemon owns each program's pseudo terminal and emulator. An office
// connects over a local, owner-only pipe, sends keys, pastes, mouse actions
// and the viewport it shows, and receives the rows of that viewport that
// changed. Keeping the emulator in the daemon keeps a program's input modes
// (mouse tracking, bracketed paste, cursor keys) across reattaches.
package daemon

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/screen"
	"github.com/MrT-coder/bitagentshub/internal/term"
)

// Proto is the wire version; an office and a daemon only talk when it
// matches.
const Proto = 1

// ErrProto means the daemon running was built with another protocol.
var ErrProto = errors.New("the running daemon is from an incompatible build")

// Program is a terminal session the daemon hosts; term.Session is the real
// one.
type Program interface {
	Key(msg tea.KeyPressMsg)
	Paste(text string)
	Mouse(kind term.MouseKind, x, y int)
	Draw(buf *screen.Buffer, x, y, w, h, scroll int, base screen.Style)
	Resize(w, h int)
	Size() (w, h int)
	ScrollbackLen() int
	AltScreen() bool
	Exited() bool
	Close()
}

// StartFunc starts a program; notify gets the id whenever its screen changes.
type StartFunc func(id, dir string, argv []string, w, h int, notify chan<- string) (Program, error)

// StartPTY runs programs in real pseudo terminals.
func StartPTY(id, dir string, argv []string, w, h int, notify chan<- string) (Program, error) {
	s, err := term.Start(id, dir, argv, w, h, notify)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Info describes a hosted program.
type Info struct {
	ID     string
	Dir    string
	Argv   []string
	Exited bool
}

// Request is what an office sends. Op selects the fields that matter.
//
//	hello                          first message on every connection
//	list | stop                    one-shot: answered, then the connection ends
//	start  ID Dir Argv W H         one-shot
//	attach                         the connection becomes a stream of the ops below
//	view   ID W H Scroll Base      show this viewport of ID; rows are pushed as they change
//	unview ID
//	key    ID Key · paste ID Text · mouse ID Mouse X Y · resize ID W H · close ID
type Request struct {
	Op     string
	Proto  int
	ID     string
	Dir    string
	Argv   []string
	W, H   int
	Scroll int
	Base   screen.Style
	Key    tea.KeyPressMsg
	Text   string
	Mouse  term.MouseKind
	X, Y   int
}

// Event is what the daemon sends.
//
//	welcome Proto PID · sessions Sessions · ok · error Err
//	frame ID Frame                pushed on an attached connection
type Event struct {
	Op       string
	Proto    int
	PID      int
	Err      string
	Sessions []Info
	ID       string
	Frame    Frame
}

// Frame updates what an office knows of a program. Rows holds the viewport
// rows that changed, by index; all of them when Full. A frame for a program
// the office does not view carries only the state fields.
type Frame struct {
	W, H, Scroll int
	Base         screen.Style
	Full         bool
	Rows         map[int][]screen.Span
	SizeW, SizeH int
	Scrollback   int
	Alt          bool
	Exited       bool
	Closed       bool // the program was closed and is gone
}

// wire wraps a connection with gob coding.
type wire struct {
	enc *gob.Encoder
	dec *gob.Decoder
	c   io.ReadWriteCloser
}

func newWire(c io.ReadWriteCloser) *wire {
	return &wire{enc: gob.NewEncoder(c), dec: gob.NewDecoder(c), c: c}
}

// hello opens a client connection and checks the daemon's version.
func (w *wire) hello() (Event, error) {
	if err := w.enc.Encode(Request{Op: "hello", Proto: Proto}); err != nil {
		return Event{}, err
	}
	var ev Event
	if err := w.dec.Decode(&ev); err != nil {
		return Event{}, err
	}
	if ev.Op != "welcome" {
		return ev, fmt.Errorf("unexpected reply %q", ev.Op)
	}
	if ev.Proto != Proto {
		return ev, fmt.Errorf("%w (daemon pid %d speaks v%d, this office v%d)", ErrProto, ev.PID, ev.Proto, Proto)
	}
	return ev, nil
}
