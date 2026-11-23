package office

import (
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

func TestDrawPetBadgeStarsTheCornerWhileReviewsWait(t *testing.T) {
	l := Build(nil, testW, testH)
	x, y, w, _ := l.PetCells()
	for _, tc := range []struct {
		n    int
		want string
	}{{1, "★"}, {3, "★3"}, {12, "★12"}} {
		buf := screen.New(l.W, l.H/2)
		DrawPetBadge(buf, l, tc.n)
		if got := strings.TrimSpace(buf.Row(y, x, x+w)); got != tc.want {
			t.Errorf("%d reviews waiting: the badge reads %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestDrawPetBadgeDrawsNothingWhenNoReviewWaits(t *testing.T) {
	l := Build(nil, testW, testH)
	buf := screen.New(l.W, l.H/2)
	DrawPetBadge(buf, l, 0)
	for y := range l.H / 2 {
		if row := strings.TrimSpace(buf.Row(y, 0, l.W)); row != "" {
			t.Fatalf("row %d = %q, want an empty buffer", y, row)
		}
	}
}

func TestTheBadgeDoesNotCoverThePetsBubble(t *testing.T) {
	l := Build(nil, testW, testH)
	x, y, _, _ := l.PetCells()
	bubbleX := x + 1 + catW - 2 // petBubble's column
	buf := screen.New(l.W, l.H/2)
	DrawPetBadge(buf, l, 99)
	if got := buf.Row(y, bubbleX, bubbleX+2); strings.TrimSpace(got) != "" {
		t.Errorf("the badge reaches the bubble's cells: %q", got)
	}
}
