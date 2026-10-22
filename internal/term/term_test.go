package term

import (
	"bytes"
	"io"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// fakeProgram is the other end of the session's terminal: what it reads is
// what a real program would receive as keyboard input.
type fakeProgram struct {
	conn net.Conn
	in   chan []byte
}

func startFake(t *testing.T, w, h int) (*Session, *fakeProgram, chan string) {
	t.Helper()
	client, server := net.Pipe()
	notify := make(chan string, 64)
	s := newSession("s1", client, w, h, notify)
	p := &fakeProgram{conn: server, in: make(chan []byte, 64)}
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := server.Read(buf)
			if err != nil {
				close(p.in)
				return
			}
			p.in <- append([]byte(nil), buf[:n]...)
		}
	}()
	t.Cleanup(func() { s.Close(); server.Close() })
	return s, p, notify
}

func (p *fakeProgram) received(t *testing.T) string {
	t.Helper()
	select {
	case b := <-p.in:
		return string(b)
	case <-time.After(2 * time.Second):
		t.Fatal("program received nothing")
	}
	return ""
}

func (p *fakeProgram) print(t *testing.T, s string, notify chan string) {
	t.Helper()
	if _, err := io.WriteString(p.conn, s); err != nil {
		t.Fatal(err)
	}
	select {
	case <-notify:
	case <-time.After(2 * time.Second):
		t.Fatal("no change notification after output")
	}
	time.Sleep(20 * time.Millisecond) // let the rest of the chunk land
}

func TestKeysReachTheProgramAsTerminalInput(t *testing.T) {
	// Behind ConPTY shift+enter goes as Windows Terminal sends it, a
	// win32-input-mode key down and up, so PowerShell adds a line; other
	// terminals send it as plain enter.
	shiftEnter := "\r"
	if runtime.GOOS == "windows" {
		shiftEnter = "\x1b[13;28;13;1;16;1_\x1b[13;28;13;0;16;1_"
	}
	tests := []struct {
		name string
		msg  tea.KeyPressMsg
		want string
	}{
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}, "\r"},
		{"up", tea.KeyPressMsg{Code: tea.KeyUp}, "\x1b[A"},
		{"ctrl+c", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, "\x03"},
		{"backspace", tea.KeyPressMsg{Code: tea.KeyBackspace}, "\x7f"},
		{"esc", tea.KeyPressMsg{Code: tea.KeyEscape}, "\x1b"},
		{"shift+tab", tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, "\x1b[Z"},
		{"pgup", tea.KeyPressMsg{Code: tea.KeyPgUp}, "\x1b[5~"},
		{"text", tea.KeyPressMsg{Code: 'h', Text: "hé"}, "hé"},
		{"shifted text", tea.KeyPressMsg{Code: 'a', ShiftedCode: 'A', Text: "A", Mod: tea.ModShift}, "A"},
		{"alt+x", tea.KeyPressMsg{Code: 'x', Text: "x", Mod: tea.ModAlt}, "\x1bx"},
		{"space", tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, " "},
		{"shift+left", tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift}, "\x1b[1;2D"},
		{"shift+right", tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}, "\x1b[1;2C"},
		{"ctrl+home", tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModCtrl}, "\x1b[1;5H"},
		{"shift+end", tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModShift}, "\x1b[1;2F"},
		// Windows reports the lock keys as modifiers; programs never see them.
		{"ctrl+c with num lock", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl | tea.ModNumLock}, "\x03"},
		{"esc with num lock", tea.KeyPressMsg{Code: tea.KeyEscape, Mod: tea.ModNumLock}, "\x1b"},
		{"up with caps lock", tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCapsLock}, "\x1b[A"},
		{"text with caps lock", tea.KeyPressMsg{Code: 'a', Text: "A", Mod: tea.ModCapsLock}, "A"},
		{"shift+left with num lock", tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift | tea.ModNumLock}, "\x1b[1;2D"},
		{"alt+shift+a", tea.KeyPressMsg{Code: 'a', Mod: tea.ModAlt | tea.ModShift}, "\x1bA"},
		{"alt+shift+a with shifted code", tea.KeyPressMsg{Code: 'a', ShiftedCode: 'A', Mod: tea.ModAlt | tea.ModShift}, "\x1bA"},
		{"ctrl+shift+a", tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl | tea.ModShift}, "\x01"},
		{"ctrl+shift+A from modifyOtherKeys", tea.KeyPressMsg{Code: 'A', Mod: tea.ModCtrl | tea.ModShift}, "\x01"},
		{"shift+enter", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}, shiftEnter},
		{"alt+enter", tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}, "\x1b\r"},
		{"shift+space", tea.KeyPressMsg{Code: tea.KeySpace, Mod: tea.ModShift}, " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, p, _ := startFake(t, 40, 10)
			s.Key(tt.msg)
			if got := p.received(t); got != tt.want {
				t.Errorf("program got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPasteIsBracketedWhenTheProgramAsksForIt(t *testing.T) {
	s, p, notify := startFake(t, 40, 10)
	p.print(t, "\x1b[?2004h", notify)
	s.Paste("a\nb")
	var got strings.Builder
	for !strings.Contains(got.String(), "\x1b[201~") {
		got.WriteString(p.received(t))
	}
	if got.String() != "\x1b[200~a\nb\x1b[201~" {
		t.Errorf("program got %q", got.String())
	}
}

func draw(s *Session, w, h, scroll int) (plainText, raw string) {
	buf := screen.New(w, h)
	s.Draw(buf, 0, 0, w, h, scroll, screen.Style{})
	lines := buf.Lines()
	return stripANSI(strings.Join(lines, "\n")), strings.Join(lines, "\n")
}

func TestOutputIsDrawnWithItsColors(t *testing.T) {
	s, p, notify := startFake(t, 20, 3)
	p.print(t, "\x1b[38;2;255;0;0mred\x1b[0m ok", notify)
	text, raw := draw(s, 20, 3, 0)
	if !strings.HasPrefix(text, "red ok") {
		t.Errorf("screen = %q", text)
	}
	if !strings.Contains(raw, "38;2;255;0;0") {
		t.Errorf("red foreground lost: %q", raw)
	}
}

// Claude Code puts a status glyph such as ✳ (E2 9C B3) in its window title.
// 0x9C is also the 8-bit string terminator, which must not end the title
// halfway and print the rest of it on the screen.
func TestTitlesAreNotDrawnOnTheScreen(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string
	}{
		{"glyph with 0x9C", []string{"\x1b]0;✳ Retomar sesión anterior\x07ok"}},
		{"terminated by ESC backslash", []string{"\x1b]0;✳ Retomar\x1b\\ok"}},
		{"split inside the glyph", []string{"\x1b]0;\xe2\x9c", "\xb3 Retomar\x07ok"}},
		{"8-bit string terminator", []string{"\x1b]0;title\x9cok"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, p, notify := startFake(t, 30, 3)
			for _, c := range tt.chunks {
				p.print(t, c, notify)
			}
			text, _ := draw(s, 30, 3, 0)
			if line := strings.TrimRight(strings.Split(text, "\n")[0], " "); line != "ok" {
				t.Errorf("first line = %q, want %q", line, "ok")
			}
		})
	}
}

func TestScrollingShowsEarlierLines(t *testing.T) {
	s, p, notify := startFake(t, 20, 5)
	var out bytes.Buffer
	for i := 0; i < 30; i++ {
		out.WriteString("line " + string(rune('A'+i%26)) + "\r\n")
	}
	p.print(t, out.String(), notify)

	if s.ScrollbackLen() == 0 {
		t.Fatal("nothing went to the scrollback")
	}
	live, _ := draw(s, 20, 5, 0)
	if !strings.Contains(live, "line D") { // the 30th line
		t.Errorf("live view = %q", live)
	}
	back, _ := draw(s, 20, 5, 10)
	if strings.Contains(back, "line D") || !strings.Contains(back, "line T") {
		t.Errorf("scrolled view = %q, want lines from ten rows up", back)
	}
}

func TestExitIsReported(t *testing.T) {
	s, p, _ := startFake(t, 20, 5)
	p.conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for !s.Exited() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !s.Exited() {
		t.Error("session does not report that the program ended")
	}
}

// A real program in a real pseudo terminal: ConPTY on Windows, a pty
// elsewhere (CI runs this on Linux and macOS).
func TestStart_RunsARealProgramInAPseudoConsole(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a real program")
	}
	argv := []string{"sh", "-c", "echo hi-from-pty"}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd.exe", "/c", "echo hi-from-pty"}
	}
	notify := make(chan string, 64)
	s, err := Start("real", t.TempDir(), argv, 60, 10, notify)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if text, _ := draw(s, 60, 10, 0); strings.Contains(text, "hi-from-pty") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	text, _ := draw(s, 60, 10, 0)
	t.Errorf("screen never showed the output:\n%s", text)
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			inEsc = false
		case !inEsc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestMouseReachesProgramsThatTrackIt(t *testing.T) {
	s, p, notify := startFake(t, 40, 10)
	p.print(t, "\x1b[?1049h\x1b[?1000h\x1b[?1006h", notify) // fullscreen app with SGR mouse
	if !s.AltScreen() {
		t.Fatal("alt screen not detected")
	}
	s.Mouse(MouseWheelUp, 4, 2)
	if got := p.received(t); got != "\x1b[<64;5;3M" {
		t.Errorf("program got %q, want an SGR wheel-up at column 5, row 3", got)
	}
	s.Mouse(MouseLeftPress, 0, 0)
	if got := p.received(t); got != "\x1b[<0;1;1M" {
		t.Errorf("program got %q, want an SGR left press", got)
	}
}

func TestProgramsDoNotInheritTheSSHSessionTheOfficeStartedIn(t *testing.T) {
	env := childEnv([]string{"PATH=/bin", "SSH_CONNECTION=1.2.3.4 5 6.7.8.9 22", "SSH_CLIENT=1.2.3.4 5 22", "SSH_TTY=windows-pty", "ssh_auth_sock=x", "TERM=dumb"})
	got := strings.Join(env, "|")
	for _, gone := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY", "TERM=dumb"} {
		if strings.Contains(got, gone) {
			t.Errorf("env should not carry %s: %s", gone, got)
		}
	}
	for _, kept := range []string{"PATH=/bin", "ssh_auth_sock=x", "TERM=xterm-256color", "COLORTERM=truecolor"} {
		if !strings.Contains(got, kept) {
			t.Errorf("env should carry %s: %s", kept, got)
		}
	}
}

func TestATerminalKeepsAsMuchHistoryAsItWasGiven(t *testing.T) {
	defer SetScrollback(Scrollback())
	if Scrollback() != DefaultScrollback || DefaultScrollback != 2000 {
		t.Fatalf("default history = %d lines, want 2000", Scrollback())
	}
	SetScrollback(10)
	s, p, notify := startFake(t, 20, 5)
	var out bytes.Buffer
	for i := 0; i < 40; i++ {
		out.WriteString("line\r\n")
	}
	p.print(t, out.String(), notify)
	if got := s.ScrollbackLen(); got != 10 {
		t.Errorf("history = %d lines, want the 10 it was given", got)
	}
}
