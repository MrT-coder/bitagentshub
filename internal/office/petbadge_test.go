package office

import (
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

func TestDrawPetBadgeStarsTheShelfWhileReviewsWait(t *testing.T) {
	l := Build(nil, testW, testH)
	it := shelf(t, l)
	x, y, w := it.X, it.Y/2, it.W
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
