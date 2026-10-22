// Package screen is a cell buffer that lets pixels, text and boxes be drawn
// on top of each other and then emitted as terminal lines. Pixels use
// half-block glyphs, so each cell holds two vertically stacked pixels.
package screen

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

type RGB struct{ R, G, B uint8 }

type Style struct {
	Fg, Bg       RGB
	HasFg, HasBg bool
	Bold         bool
	Reverse      bool
}

// Palette replaces the 16 ANSI colors a terminal program asks for. Without
// Set, programs keep the emulator's own colors.
type Palette struct {
	Set bool
	C   [16]RGB
}

func Fg(c RGB) Style { return Style{Fg: c, HasFg: true} }

func (s Style) WithBg(c RGB) Style {
	s.Bg, s.HasBg = c, true
	return s
}

type cell struct {
	ch    rune // 0 means a pixel cell
	style Style
	cont  bool // right half of a wide rune; emits nothing
	top   RGB
	bot   RGB
	hasT  bool
	hasB  bool
}

type Buffer struct {
	W, H  int
	cells []cell
}

func New(w, h int) *Buffer {
	b := &Buffer{W: w, H: h, cells: make([]cell, w*h)}
	for i := range b.cells {
		b.cells[i].ch = ' '
	}
	return b
}

// CopyFrom makes b hold exactly what src holds; it does nothing when their
// sizes differ.
func (b *Buffer) CopyFrom(src *Buffer) {
	if b.W == src.W && b.H == src.H {
		copy(b.cells, src.cells)
	}
}

func (b *Buffer) at(x, y int) *cell {
	if x < 0 || y < 0 || x >= b.W || y >= b.H {
		return nil
	}
	return &b.cells[y*b.W+x]
}

// Pixel sets one pixel; py counts pixels, two per cell row. A text cell
// under it becomes a pixel cell that keeps the text's background.
func (b *Buffer) Pixel(x, py int, c RGB) {
	cl := b.at(x, py/2)
	if cl == nil {
		return
	}
	if cl.ch != 0 {
		*cl = cell{style: Style{Bg: cl.style.Bg, HasBg: cl.style.HasBg}}
	}
	if py%2 == 0 {
		cl.top, cl.hasT = c, true
	} else {
		cl.bot, cl.hasB = c, true
	}
}

// PixelAt returns the color of pixel (x, py), or the zero color when it is
// unset or covered by text.
func (b *Buffer) PixelAt(x, py int) RGB {
	cl := b.at(x, py/2)
	if cl == nil || cl.ch != 0 {
		return RGB{}
	}
	if py%2 == 0 {
		return cl.top
	}
	return cl.bot
}

// Text writes s starting at (x, y), clipped to the buffer. Wide runes take
// two cells and are dropped when only one is left. Without an explicit
// background, text sits on the color of the pixels it covers.
func (b *Buffer) Text(x, y int, s string, st Style) int {
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w == 0 {
			continue
		}
		if x+w > b.W {
			break
		}
		if x >= 0 {
			if cl := b.at(x, y); cl != nil {
				cs := st
				if !cs.HasBg && cl.ch == 0 && cl.hasT {
					cs = cs.WithBg(cl.top)
				}
				*cl = cell{ch: r, style: cs}
				if w == 2 {
					if next := b.at(x+1, y); next != nil {
						*next = cell{ch: ' ', style: st, cont: true}
					}
				}
			}
		}
		x += w
	}
	return x
}

// Fill paints a rectangle with one rune and style.
func (b *Buffer) Fill(x, y, w, h int, r rune, st Style) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if cl := b.at(xx, yy); cl != nil {
				*cl = cell{ch: r, style: st}
			}
		}
	}
}

// Row returns the text of cells x0 to x1-1 on row y; pixel cells read as
// spaces and the right half of a wide rune adds nothing.
func (b *Buffer) Row(y, x0, x1 int) string {
	var s strings.Builder
	for x := max(x0, 0); x < min(x1, b.W); x++ {
		cl := b.at(x, y)
		switch {
		case cl == nil || cl.cont:
		case cl.ch == 0:
			s.WriteByte(' ')
		default:
			s.WriteRune(cl.ch)
		}
	}
	return s.String()
}

// Invert flips the reverse attribute of text cells x0 to x1-1 on row y, so
// marked text stands out whatever its colors.
func (b *Buffer) Invert(y, x0, x1 int) {
	for x := x0; x < x1; x++ {
		if cl := b.at(x, y); cl != nil && cl.ch != 0 {
			cl.style.Reverse = !cl.style.Reverse
		}
	}
}

// Span is a run of text cells sharing one style.
type Span struct {
	Text  string
	Style Style
}

// Spans returns row y as runs of text, so it can be sent elsewhere and drawn
// again with Text. Pixel cells become spaces on their background.
func (b *Buffer) Spans(y int) []Span {
	var out []Span
	for x := 0; x < b.W; x++ {
		cl := &b.cells[y*b.W+x]
		if cl.cont {
			continue
		}
		ch, st := cl.ch, cl.style
		if ch == 0 {
			ch, st = ' ', Style{Bg: cl.style.Bg, HasBg: cl.style.HasBg}
		}
		if n := len(out); n > 0 && out[n-1].Style == st {
			out[n-1].Text += string(ch)
			continue
		}
		out = append(out, Span{Text: string(ch), Style: st})
	}
	return out
}

// Lines renders the buffer, emitting color codes only when they change.
// It runs for every frame, so it compares styles rather than their codes
// and writes into one reused byte slice.
func (b *Buffer) Lines() []string {
	out := make([]string, b.H)
	var line []byte
	for y := 0; y < b.H; y++ {
		line = line[:0]
		var cur Style // what the codes written so far show; zero shows nothing
		for x := 0; x < b.W; x++ {
			cl := &b.cells[y*b.W+x]
			if cl.cont {
				continue
			}
			st, glyph := cl.look()
			if st = st.shown(); st != cur {
				line = append(line, "[0m"...)
				line = st.appendSGR(line)
				cur = st
			}
			line = utf8.AppendRune(line, glyph)
		}
		if cur != (Style{}) {
			line = append(line, "[0m"...)
		}
		out[y] = string(line)
	}
	return out
}

// look is how a cell prints: its style and its glyph.
func (cl *cell) look() (Style, rune) {
	if cl.ch != 0 {
		return cl.style, cl.ch
	}
	bg := Style{Bg: cl.style.Bg, HasBg: cl.style.HasBg}
	switch {
	case cl.hasT && cl.hasB && cl.top == cl.bot:
		// A solid cell needs only a background; runs of floor then share
		// one color code.
		return Style{}.WithBg(cl.top), ' '
	case cl.hasT && cl.hasB:
		return Style{Fg: cl.top, HasFg: true}.WithBg(cl.bot), '▀'
	case cl.hasT:
		bg.Fg, bg.HasFg = cl.top, true
		return bg, '▀'
	case cl.hasB:
		bg.Fg, bg.HasFg = cl.bot, true
		return bg, '▄'
	}
	return bg, ' '
}

// shown keeps what the style's codes show: a color it does not use is
// left out, so two styles that print alike compare equal.
func (s Style) shown() Style {
	if !s.HasFg {
		s.Fg = RGB{}
	}
	if !s.HasBg {
		s.Bg = RGB{}
	}
	return s
}

// appendSGR appends the codes that set s.
func (s Style) appendSGR(b []byte) []byte {
	if s.Bold {
		b = append(b, "[1m"...)
	}
	if s.Reverse {
		b = append(b, "[7m"...)
	}
	if s.HasFg {
		b = appendRGB(append(b, "[38;2;"...), s.Fg)
		b = append(b, 'm')
	}
	if s.HasBg {
		b = appendRGB(append(b, "[48;2;"...), s.Bg)
		b = append(b, 'm')
	}
	return b
}

func appendRGB(b []byte, c RGB) []byte {
	b = strconv.AppendInt(b, int64(c.R), 10)
	b = append(b, ';')
	b = strconv.AppendInt(b, int64(c.G), 10)
	b = append(b, ';')
	return strconv.AppendInt(b, int64(c.B), 10)
}
