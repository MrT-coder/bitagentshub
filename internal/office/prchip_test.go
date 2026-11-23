package office

import (
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

func shelf(t *testing.T, l Layout) Item {
	t.Helper()
	for _, it := range l.Items {
		if it.Kind == "bookshelf" {
			return it
		}
	}
	t.Fatal("the office has no bookshelf")
	return Item{}
}

func TestTheChipSitsOnTheBookshelfsTopRightCornerRightOfTheStar(t *testing.T) {
	l := Build(nil, testW, testH)
	it := shelf(t, l)
	buf := screen.New(l.W, l.H/2)
	DrawPetBadge(buf, l, 100) // the widest star, "★99+"
	DrawPRChip(buf, l)
	y := it.Y / 2
	if got := strings.TrimSpace(buf.Row(y, it.X, it.X+it.W)); got != "★99+  PRs" {
		t.Errorf("the shelf's top row reads %q, want the star then the chip", got)
	}
	if got := buf.Row(y, it.X+it.W-len(prChipLabel), it.X+it.W); got != prChipLabel {
		t.Errorf("the chip is not flush with the shelf's right edge: %q", got)
	}
}

func TestTheChipLeavesThePetsCornerAlone(t *testing.T) {
	l := Build(nil, testW, testH)
	x, y, w, h := l.PetCells()
	buf := screen.New(l.W, l.H/2)
	DrawPetBadge(buf, l, 5)
	DrawPRChip(buf, l)
	for row := y; row < y+h; row++ {
		if got := strings.TrimSpace(buf.Row(row, x, x+w)); got != "" {
			t.Errorf("row %d of the pet's corner reads %q, want it untouched", row, got)
		}
	}
}

func TestPRHitIsTheStarAndTheChipOnTheShelfsTopRowOnly(t *testing.T) {
	l := Build(nil, testW, testH)
	it := shelf(t, l)
	y := it.Y / 2
	cx, _ := l.prAnchor()
	for _, tc := range []struct {
		name string
		x, y int
		want bool
	}{
		{"the star", cx - prStarGap, y, true},
		{"the chip", cx + 2, y, true},
		{"left of the star", cx - prStarGap - 1, y, false},
		{"under the chip", cx + 2, y + 3, false},
	} {
		if got := PRHit(l, tc.x, tc.y); got != tc.want {
			t.Errorf("%s: PRHit = %v, want %v", tc.name, got, tc.want)
		}
	}
	px, py, _, _ := l.PetCells()
	if PRHit(l, px+2, py) {
		t.Error("the pet's corner still opens the PRs window")
	}
}
