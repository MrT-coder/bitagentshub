package office

import (
	"math"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// PRColors is the colors the pull request window gives each state: open,
// draft, merged and closed pull requests, and checks still running.
type PRColors struct {
	Open, Draft, Merged, Closed, Pending screen.RGB
}

// contrastMin is the contrast ratio a derived state color keeps against the
// window's background; WCAG asks 4.5 for text.
const contrastMin = 4.5

// PR is the pull request state colors of the active theme.
func PR() PRColors { return cur().PR }

// resolvePR fills each state color the theme does not set: from its
// [terminal] palette when it has one (green, red, magenta, yellow and the
// bright black for drafts), else from GitHub's own colors, then moves it
// toward the window's text color until it reads on the window's background.
// A color the theme sets is kept as written.
func (t *Theme) resolvePR() {
	gh := PRColors{Open: rgb(0x3fb950), Draft: rgb(0x8b949e), Merged: rgb(0xa371f7), Closed: rgb(0xf85149), Pending: rgb(0xd29922)}
	if p := t.Terminal; p.Set {
		gh = PRColors{Open: p.C[2], Draft: p.C[8], Merged: p.C[5], Closed: p.C[1], Pending: p.C[3]}
	}
	for _, f := range []struct {
		key string
		to  *screen.RGB
		def screen.RGB
	}{
		{"ui.pr_open", &t.PR.Open, gh.Open},
		{"ui.pr_draft", &t.PR.Draft, gh.Draft},
		{"ui.pr_merged", &t.PR.Merged, gh.Merged},
		{"ui.pr_closed", &t.PR.Closed, gh.Closed},
		{"ui.pr_pending", &t.PR.Pending, gh.Pending},
	} {
		if !t.sets(f.key) {
			*f.to = readable(f.def, t.UIBg, t.UIText)
		}
	}
}

// readable moves c toward toward in steps until its contrast against bg
// reaches contrastMin, or it is toward.
func readable(c, bg, toward screen.RGB) screen.RGB {
	for i := 0; i <= 10 && contrast(c, bg) < contrastMin; i++ {
		c = mix(c, toward, float64(i+1)/10)
	}
	return c
}

// mix is a moved fraction f of the way to b.
func mix(a, b screen.RGB, f float64) screen.RGB {
	ch := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*f)) }
	return screen.RGB{R: ch(a.R, b.R), G: ch(a.G, b.G), B: ch(a.B, b.B)}
}

// luminance is the WCAG relative luminance of c.
func luminance(c screen.RGB) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// contrast is the WCAG contrast ratio of a and b, from 1 to 21.
func contrast(a, b screen.RGB) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
