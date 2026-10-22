// Package term runs a program in a pseudo terminal (ConPTY on Windows) and
// keeps its screen in a terminal emulator, so the office can show the real
// program, scroll its history and type into it.
package term

import (
	"context"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/charmbracelet/x/xpty"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// DefaultScrollback is how many lines of history a terminal keeps unless
// config.toml says otherwise. A full line of history costs about 112 bytes
// per character, so 2000 lines of output stay near 16 MB per terminal.
const DefaultScrollback = 2000

var scrollback atomic.Int64

func init() { scrollback.Store(DefaultScrollback) }

// SetScrollback sets how many lines of history terminals started from now
// on keep.
func SetScrollback(lines int) { scrollback.Store(int64(lines)) }

// Scrollback is how many lines of history a new terminal keeps.
func Scrollback() int { return int(scrollback.Load()) }

// Session is one program running in a pseudo terminal.
type Session struct {
	ID string

	emu    *vt.SafeEmulator
	rw     io.ReadWriteCloser
	notify chan<- string
	exited atomic.Bool
	close  sync.Once
	cmd    *exec.Cmd
	pty    xpty.Pty
}

// childEnv is the environment a program starts with: the office's, with its
// own terminal type and without the SSH session the office or its daemon may
// have been started from. Programs run on this machine, and a stale
// SSH_CONNECTION makes Claude Code turn off image paste.
func childEnv(environ []string) []string {
	var env []string
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY", "TERM", "COLORTERM":
			continue
		}
		env = append(env, kv)
	}
	return append(env, "TERM=xterm-256color", "COLORTERM=truecolor")
}

// Start runs argv in dir inside a new w×h pseudo terminal. Every time the
// program writes to the screen, the session id is sent on notify without
// blocking, so a burst of output wakes the UI once.
func Start(id, dir string, argv []string, w, h int, notify chan<- string) (*Session, error) {
	pty, err := xpty.NewPty(w, h)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = childEnv(os.Environ())
	if err := pty.Start(cmd); err != nil {
		pty.Close()
		return nil, err
	}
	s := newSession(id, pty, w, h, notify)
	s.cmd, s.pty = cmd, pty
	go func() {
		// cmd.Wait does not work for ConPTY children; WaitProcess does.
		_ = xpty.WaitProcess(context.Background(), cmd)
		s.markExited()
	}()
	return s, nil
}

func newSession(id string, rw io.ReadWriteCloser, w, h int, notify chan<- string) *Session {
	s := &Session{ID: id, emu: vt.NewSafeEmulator(w, h), rw: rw, notify: notify}
	s.emu.SetScrollbackSize(Scrollback())
	go s.readOutput()
	go func() { _, _ = io.Copy(rw, s.emu) }() // keys and replies the emulator encodes
	return s
}

func (s *Session) readOutput() {
	buf := make([]byte, 32*1024)
	var osc oscFilter
	for {
		n, err := s.rw.Read(buf)
		if n > 0 {
			osc.filter(buf[:n])
			_, _ = s.emu.Write(buf[:n])
			s.changed()
		}
		if err != nil {
			s.markExited()
			return
		}
	}
}

// oscFilter works around the emulator's parser ending an OSC string at any
// 0x9C byte, the 8-bit string terminator, even when it is part of a UTF-8
// character such as ✳ (E2 9C B3) in a window title; the rest of the title
// then lands on the screen. Inside an OSC string it rewrites such a
// continuation byte to another one. The title text changes, but nothing
// shows titles, and the string still ends where the program meant it to.
type oscFilter struct {
	state int // oscGround, oscEscape or oscString
	cont  int // UTF-8 continuation bytes still expected inside the string
}

const (
	oscGround = iota
	oscEscape
	oscString
)

func (f *oscFilter) filter(b []byte) {
	for i, c := range b {
		switch f.state {
		case oscGround:
			if c == 0x1b {
				f.state = oscEscape
			}
		case oscEscape:
			switch c {
			case ']':
				f.state, f.cont = oscString, 0
			case 0x1b:
			default:
				f.state = oscGround
			}
		case oscString:
			switch {
			case f.cont > 0 && c&0xc0 == 0x80:
				f.cont--
				if c == 0x9c {
					b[i] = 0xbf
				}
			case c == 0x07, c == 0x9c, c == 0x18, c == 0x1a:
				f.state = oscGround
			case c == 0x1b:
				f.state = oscEscape
			default:
				f.cont = utf8Continuations(c)
			}
		}
	}
}

// utf8Continuations is how many continuation bytes follow lead byte c.
func utf8Continuations(c byte) int {
	switch {
	case c >= 0xc2 && c <= 0xdf:
		return 1
	case c >= 0xe0 && c <= 0xef:
		return 2
	case c >= 0xf0 && c <= 0xf4:
		return 3
	}
	return 0
}

func (s *Session) changed() {
	select {
	case s.notify <- s.ID:
	default:
	}
}

func (s *Session) markExited() {
	if !s.exited.Swap(true) {
		s.changed()
	}
}

// Exited reports whether the program has ended. Its last screen stays
// visible until the session is closed.
func (s *Session) Exited() bool { return s.exited.Load() }

// Size is the terminal size in cells.
func (s *Session) Size() (w, h int) { return s.emu.Width(), s.emu.Height() }

// Resize changes the terminal size; the program is told through the PTY.
func (s *Session) Resize(w, h int) {
	if w < 1 || h < 1 || (w == s.emu.Width() && h == s.emu.Height()) {
		return
	}
	s.emu.Resize(w, h)
	if s.pty != nil {
		_ = s.pty.Resize(w, h)
	}
}

// Close ends the program and releases the terminal.
func (s *Session) Close() {
	s.close.Do(func() {
		if s.cmd != nil && s.cmd.Process != nil && !s.Exited() {
			_ = s.cmd.Process.Kill()
		}
		_ = s.rw.Close()
		// Unblocks the goroutine copying keys out of the emulator, so the
		// emulator and its history can be freed.
		_ = s.emu.Close()
	})
}

// ScrollbackLen is how many lines of history sit above the screen.
func (s *Session) ScrollbackLen() int { return s.emu.ScrollbackLen() }

// AltScreen reports whether the program drew on the alternate screen,
// which has no history of its own: full-screen programs such as Claude Code
// scroll themselves, so the wheel must go to them.
func (s *Session) AltScreen() bool { return s.emu.IsAltScreen() }

// MouseKind is a mouse action the office passes to a program.
type MouseKind int

const (
	MouseWheelUp MouseKind = iota
	MouseWheelDown
	MouseLeftPress
	MouseLeftRelease
)

// Mouse sends a mouse action at terminal cell (x, y). It is encoded in the
// program's requested mouse mode, and dropped when the program asked for
// none.
func (s *Session) Mouse(kind MouseKind, x, y int) {
	at := uv.Mouse{X: x, Y: y}
	switch kind {
	case MouseWheelUp, MouseWheelDown:
		at.Button = uv.MouseWheelUp
		if kind == MouseWheelDown {
			at.Button = uv.MouseWheelDown
		}
		s.emu.SendMouse(uv.MouseWheelEvent(at))
	case MouseLeftPress:
		at.Button = uv.MouseLeft
		s.emu.SendMouse(uv.MouseClickEvent(at))
	case MouseLeftRelease:
		at.Button = uv.MouseLeft
		s.emu.SendMouse(uv.MouseReleaseEvent(at))
	}
}

// Key types a key into the program, encoded the way a terminal would
// (honoring modes such as application cursor keys).
func (s *Session) Key(msg tea.KeyPressMsg) {
	k := uv.KeyPressEvent(msg)
	// Windows reports caps and num lock as modifiers; terminals never encode
	// them, and x/vt sends nothing for a key with a modifier it does not know.
	k.Mod &^= uv.ModCapsLock | uv.ModNumLock | uv.ModScrollLock
	switch {
	case k.Code == uv.KeyEnter && k.Mod == uv.ModShift && runtime.GOOS == "windows":
		// Encoded, shift+enter is plain enter. ConPTY also reads the key
		// events Windows Terminal sends (win32-input-mode), so shift+enter
		// reaches the program as itself: PowerShell adds a line.
		s.emu.SendText(win32ShiftEnter)
	case k.Text != "" && k.Mod&(uv.ModCtrl|uv.ModAlt) == 0:
		s.emu.SendText(k.Text)
	case k.Mod&(uv.ModCtrl|uv.ModAlt) == uv.ModAlt && unicode.IsPrint(k.Code):
		// Meta sends ESC before the character, shifted when shift is held.
		s.emu.SendText("\x1b" + altText(k))
	case k.Mod != 0 && modifiedSeq(k) != "":
		// x/vt v0.1.0 encodes no modifiers on special keys, so these are
		// written in xterm's form directly.
		s.emu.SendText(modifiedSeq(k))
	default:
		if k.Mod&uv.ModCtrl != 0 || plainWithShift[k.Code] {
			// Legacy terminals send these the same with or without shift;
			// modifyOtherKeys reports ctrl+shift letters in upper case.
			k.Mod &^= uv.ModShift
			k.Code = unicode.ToLower(k.Code)
		}
		s.emu.SendKey(k)
	}
}

// win32ShiftEnter is shift+enter pressed and released in win32-input-mode:
// virtual key 13, scan code 28, char CR, shift held, repeat 1.
const win32ShiftEnter = "\x1b[13;28;13;1;16;1_\x1b[13;28;13;0;16;1_"

// plainWithShift are keys a terminal encodes the same when shift is held.
var plainWithShift = map[rune]bool{uv.KeyEnter: true, uv.KeySpace: true, uv.KeyBackspace: true, uv.KeyEscape: true}

// altText is the character an alt key combination types after ESC.
func altText(k uv.KeyPressEvent) string {
	switch {
	case k.Text != "":
		return k.Text
	case k.Mod&uv.ModShift != 0 && k.ShiftedCode != 0:
		return string(k.ShiftedCode)
	case k.Mod&uv.ModShift != 0:
		return string(unicode.ToUpper(k.Code))
	}
	return string(k.Code)
}

// Paste sends text the way a terminal pastes: wrapped in bracketed paste
// markers when the program asked for them, so line breaks stay text.
func (s *Session) Paste(text string) { s.emu.Paste(text) }

// modifiedSeq returns the xterm sequence for a cursor or editing key with
// modifiers ("\x1b[1;2D" is shift+left), or "" for other keys.
func modifiedSeq(k uv.KeyPressEvent) string {
	mod := 1
	if k.Mod&uv.ModShift != 0 {
		mod++
	}
	if k.Mod&uv.ModAlt != 0 {
		mod += 2
	}
	if k.Mod&uv.ModCtrl != 0 {
		mod += 4
	}
	final := map[rune]string{uv.KeyUp: "A", uv.KeyDown: "B", uv.KeyRight: "C", uv.KeyLeft: "D", uv.KeyHome: "H", uv.KeyEnd: "F"}
	if f, ok := final[k.Code]; ok {
		return fmt.Sprintf("\x1b[1;%d%s", mod, f)
	}
	tilde := map[rune]int{uv.KeyInsert: 2, uv.KeyDelete: 3, uv.KeyPgUp: 5, uv.KeyPgDown: 6}
	if n, ok := tilde[k.Code]; ok {
		return fmt.Sprintf("\x1b[%d;%d~", n, mod)
	}
	return ""
}

// Draw paints the terminal into the w×h box at (x, y) of buf. scroll is
// how many lines the view is moved up into the history; 0 is live, and then
// the cursor is shown. Cells without colors of their own use base, and pal
// recolors the 16 ANSI colors when set.
func (s *Session) Draw(buf *screen.Buffer, x, y, w, h, scroll int, base screen.Style, pal screen.Palette) {
	sb := s.emu.ScrollbackLen()
	scroll = min(max(scroll, 0), sb)
	top := sb - scroll // first logical line shown; history comes before the screen
	cur := s.emu.CursorPosition()
	showCursor := scroll == 0 && !s.emu.CursorHidden()
	for row := 0; row < h; row++ {
		line := top + row
		for col := 0; col < w; {
			var c *uv.Cell
			if line < sb {
				c = s.emu.ScrollbackCellAt(col, line)
			} else {
				c = s.emu.CellAt(col, line-sb)
			}
			st := cellStyle(c, base, pal)
			if showCursor && line-sb == cur.Y && col == cur.X {
				st.Reverse = !st.Reverse
			}
			text, width := " ", 1
			if c != nil && c.Content != "" {
				text, width = c.Content, max(c.Width, 1)
			}
			if col+width > w {
				break
			}
			buf.Text(x+col, y+row, text, st)
			col += width
		}
	}
}

func cellStyle(c *uv.Cell, base screen.Style, pal screen.Palette) screen.Style {
	st := base
	if c == nil {
		return st
	}
	if c.Style.Fg != nil {
		st.Fg, st.HasFg = paletteRGB(c.Style.Fg, pal), true
	}
	if c.Style.Bg != nil {
		st.Bg, st.HasBg = paletteRGB(c.Style.Bg, pal), true
	}
	st.Bold = c.Style.Attrs&uv.AttrBold != 0
	st.Reverse = c.Style.Attrs&uv.AttrReverse != 0
	if c.Style.Attrs&uv.AttrFaint != 0 && st.HasFg {
		st.Fg = screen.RGB{R: st.Fg.R / 3 * 2, G: st.Fg.G / 3 * 2, B: st.Fg.B / 3 * 2}
	}
	return st
}

// paletteRGB is c, or its entry in pal when c is one of the 16 ANSI colors.
func paletteRGB(c color.Color, pal screen.Palette) screen.RGB {
	if pal.Set {
		switch v := c.(type) {
		case ansi.BasicColor:
			if v < 16 {
				return pal.C[v]
			}
		case ansi.IndexedColor:
			if v < 16 {
				return pal.C[v]
			}
		}
	}
	return rgb(c)
}

func rgb(c color.Color) screen.RGB {
	r, g, b, _ := c.RGBA()
	return screen.RGB{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)}
}
