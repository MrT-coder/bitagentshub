package office

import (
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

func TestDrawPRChipLabelsTheCornerAndSitsRightOfTheStar(t *testing.T) {
	l := Build(nil, testW, testH)
	x, y, _, _ := l.PetCells()
	buf := screen.New(l.W, l.H/2)
	DrawPetBadge(buf, l, 100) // the widest star, "★99+"
	DrawPRChip(buf, l)
	if got := strings.TrimSpace(buf.Row(y, x, x+20)); got != "★99+  PRs" {
		t.Errorf("the top row of the corner reads %q, want the star then the chip", got)
	}
}

func TestTheChipDoesNotCoverThePetsBubble(t *testing.T) {
	l := Build(nil, testW, testH)
	x, y, _, _ := l.PetCells()
	bubbleX := x + 1 + catW - 2
	buf := screen.New(l.W, l.H/2)
	DrawPRChip(buf, l)
	if got := buf.Row(y, bubbleX, bubbleX+2); strings.TrimSpace(got) != "" {
		t.Errorf("the chip reaches the bubble's cells: %q", got)
	}
}

func TestPRHitIsTheStarAndTheChipOnTheTopRowOnly(t *testing.T) {
	l := Build(nil, testW, testH)
	x, y, _, h := l.PetCells()
	for _, tc := range []struct {
		name string
		x, y int
		want bool
	}{
		{"the star", x + 1, y, true},
		{"the chip", x + prChipX + 2, y, true},
		{"left of the star", x, y, false},
		{"the pet's bubble", x + 1 + catW - 2, y, false},
		{"the body of the pet", x + prChipX + 2, y + h/2, false},
	} {
		if got := PRHit(l, tc.x, tc.y); got != tc.want {
			t.Errorf("%s: PRHit = %v, want %v", tc.name, got, tc.want)
		}
	}
}
