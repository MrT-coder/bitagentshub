package screen

import (
	"strings"
	"testing"
)

func plain(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		inEsc := false
		for _, r := range l {
			switch {
			case r == '\x1b':
				inEsc = true
			case inEsc && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
				inEsc = false
			case !inEsc:
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func TestBuffer_TextIsClippedToTheBuffer(t *testing.T) {
	b := New(5, 2)
	b.Text(3, 0, "hello", Style{})
	b.Text(-2, 1, "abcdef", Style{})
	if got := plain(b.Lines()); got != "   he\ncdef " {
		t.Errorf("got %q", got)
	}
}

func TestBuffer_LaterDrawingCoversEarlierDrawing(t *testing.T) {
	b := New(6, 1)
	b.Text(0, 0, "office", Style{})
	b.Fill(1, 0, 3, 1, ' ', Style{})
	b.Text(2, 0, "X", Style{})
	if got := plain(b.Lines()); got != "o X ce" {
		t.Errorf("got %q", got)
	}
}

func TestBuffer_WideRunesTakeTwoCellsAndNeverOverflow(t *testing.T) {
	b := New(4, 1)
	b.Text(0, 0, "a界b界", Style{})
	if got := plain(b.Lines()); got != "a界b" {
		t.Errorf("got %q, want wide rune dropped when it does not fit", got)
	}
}

func TestBuffer_PixelsBecomeHalfBlocks(t *testing.T) {
	b := New(2, 1)
	red := RGB{255, 0, 0}
	b.Pixel(0, 0, red)
	b.Pixel(1, 1, red)
	if got := plain(b.Lines()); got != "▀▄" {
		t.Errorf("got %q", got)
	}
}

func TestBuffer_SpansGroupTextByStyleAndRedraw(t *testing.T) {
	red := Fg(RGB{R: 255})
	b := New(7, 1)
	b.Text(0, 0, "ab", red)
	b.Text(2, 0, "世x", Style{})
	spans := b.Spans(0)
	want := []Span{{Text: "ab", Style: red}, {Text: "世x  ", Style: Style{}}}
	if len(spans) != len(want) {
		t.Fatalf("spans = %+v", spans)
	}
	for i := range want {
		if spans[i] != want[i] {
			t.Errorf("span %d = %+v, want %+v", i, spans[i], want[i])
		}
	}
	// Drawing the spans back gives the same row.
	c := New(7, 1)
	x := 0
	for _, s := range spans {
		x = c.Text(x, 0, s.Text, s.Style)
	}
	if got, orig := c.Lines()[0], b.Lines()[0]; got != orig {
		t.Errorf("redrawn %q, want %q", got, orig)
	}
}

func TestRowReadsTextAndInvertMarksIt(t *testing.T) {
	b := New(8, 1)
	b.Text(0, 0, "a世b", Style{})
	b.Pixel(6, 0, RGB{R: 1})
	if got := b.Row(0, 0, 8); got != "a世b    " {
		t.Errorf("row = %q, want wide rune once and pixels as spaces", got)
	}
	if got := b.Row(0, 3, 4); got != "b" {
		t.Errorf("row 3..4 = %q, want %q", got, "b")
	}
	b.Invert(0, 0, 1)
	if !strings.Contains(b.Lines()[0], "\x1b[7ma") {
		t.Errorf("inverted cell should render reversed: %q", b.Lines()[0])
	}
	b.Invert(0, 0, 1)
	if strings.Contains(b.Lines()[0], "\x1b[7m") {
		t.Errorf("inverting twice should restore the cell: %q", b.Lines()[0])
	}
}
