package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// cellOf finds text on screen and returns the cell of its first character.
func cellOf(t *testing.T, m Model, text string) (x, y int) {
	t.Helper()
	for y, line := range strings.Split(view(m), "\n") {
		if i := strings.Index(line, text); i >= 0 {
			return ansi.StringWidth(line[:i]), y
		}
	}
	t.Fatalf("%q is not on screen:\n%s", text, view(m))
	return 0, 0
}

// clickOn clicks the middle of text on screen.
func clickOn(t *testing.T, m Model, text string) Model {
	t.Helper()
	x, y := cellOf(t, m, text)
	return press(m, click(x+ansi.StringWidth(text)/2, y))
}

func assertShows(t *testing.T, m Model, want ...string) {
	t.Helper()
	v := view(m)
	for _, w := range want {
		if !strings.Contains(v, w) {
			t.Errorf("screen missing %q:\n%s", w, v)
		}
	}
}

func assertHides(t *testing.T, m Model, unwanted ...string) {
	t.Helper()
	v := view(m)
	for _, w := range unwanted {
		if strings.Contains(v, w) {
			t.Errorf("screen should not show %q:\n%s", w, v)
		}
	}
}

func TestClickingAHubTabOpensIt(t *testing.T) {
	m := spendingOffice(t)
	m = press(m, key("p"))
	m = clickOn(t, m, " usage ")
	assertHubTab(t, m, "usage", "portal-front")
	m = clickOn(t, m, " about ")
	assertHubTab(t, m, "about", "MIT License")
	m = clickOn(t, m, " pet ")
	assertHubTab(t, m, "pet")
}

func TestClickingOutsideTheHubClosesItAndInsideKeepsIt(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(m, key("p"))
	m = clickOn(t, m, "Agents wake it")
	assertHubTab(t, m, "pet")
	m = press(m, click(1, officeY))
	if m.hubOpen {
		t.Fatalf("a click outside the hub should close it:\n%s", view(m))
	}
}

func TestAClickWhileThePetIsRenamedKeepsTheHub(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(press(m, key("p")), key("r"))
	m = press(m, click(1, officeY))
	if !m.hubOpen || !m.renaming {
		t.Fatalf("a click should not drop the name being typed (open %v, renaming %v)", m.hubOpen, m.renaming)
	}
}

func longChangelog() string {
	var b strings.Builder
	b.WriteString("# Changelog\n\n## v9.9.9 (2027-01-01)\n\n")
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&b, "- item %02d\n", i)
	}
	return b.String()
}

func TestTheWheelScrollsTheAboutTab(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = m.WithChangelog(longChangelog())
	m = press(press(m, key("p")), key("left"))
	assertShows(t, m, "- item 01")
	assertHides(t, m, "- item 60")

	m = press(m, wheel(false))
	assertShows(t, m, "- item 02")
	assertHides(t, m, "- item 01")
	m = press(m, wheel(true))
	assertShows(t, m, "- item 01")

	// Past the end the scroll stops, so the first turn back moves it.
	for range 30 {
		m = press(m, wheel(false))
	}
	assertShows(t, m, "- item 60")
	m = press(m, wheel(true))
	assertHides(t, m, "- item 60")
}

func TestTheUsageTabScrollsWithTheWheelAndTheArrows(t *testing.T) {
	m := spendingOffice(t)
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 16})
	m = press(press(m, key("p")), key("right"))
	const first = "1 claude portal-front" // the strip above says "1 claude · portal-front"
	assertShows(t, m, first)
	assertHides(t, m, "office today")

	m = press(m, wheel(false))
	assertShows(t, m, "office today")
	assertHides(t, m, first)
	m = press(m, wheel(true))
	assertShows(t, m, first)

	m = press(m, key("down"))
	assertHides(t, m, first)
	m = press(m, key("up"))
	assertShows(t, m, first)
}
