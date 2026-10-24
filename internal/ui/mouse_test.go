package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
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

func TestTheSettingsBodyScrollsWithTheWheelOnAShortTerminal(t *testing.T) {
	m := settingsOffice(t, &fakeSettings{})
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 16})
	m = press(m, key("s"))
	assertShows(t, m, "default (built-in)", "config.toml")
	assertHides(t, m, "Español")

	m = press(m, wheel(false))
	assertShows(t, m, "Español", "config.toml")
	assertHides(t, m, "default (built-in)")
	m = press(m, wheel(true))
	assertShows(t, m, "default (built-in)")
}

func TestSettingsKeepTheCursorRowInViewOnAShortTerminal(t *testing.T) {
	m := settingsOffice(t, &fakeSettings{})
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 16})
	m = press(m, key("s"))
	for range 4 {
		m = press(m, key("tab"))
	}
	prefix, hub := fmt.Sprintf("%-15s", "prefix"), fmt.Sprintf("%-15s", "hub")
	assertShows(t, m, prefix)
	for range len(keyRows) - 1 {
		m = press(m, key("down"))
	}
	assertShows(t, m, hub)
	assertHides(t, m, prefix)
}

func TestClickingASettingsRowSelectsItAndClickingItAgainActs(t *testing.T) {
	f := &fakeSettings{themes: []string{"night", "mine"}, scenes: []string{"night"}}
	m := settingsOffice(t, f)
	m = press(m, key("s"))

	m = clickOn(t, m, "mine")
	if len(f.calls) != 0 || m.setWin.section != sectionColors || m.setWin.selectedPalette() != "mine" {
		t.Fatalf("one click should only select mine in Colors: calls %q, section %d", f.calls, m.setWin.section)
	}
	m = clickOn(t, m, "mine")
	if !slices.Equal(f.calls, []string{"palette mine"}) {
		t.Fatalf("a click on the selected row should apply it, calls %q", f.calls)
	}

	m = clickOn(t, clickOn(t, m, "night (built-in)"), "night (built-in)")
	if !slices.Equal(f.calls, []string{"palette mine", "apply night"}) {
		t.Fatalf("two clicks on a theme should apply it, calls %q", f.calls)
	}

	m = clickOn(t, m, "Español")
	if m.setWin.section != sectionLanguage || i18n.Langs[m.setWin.langSel] != i18n.Spanish || len(f.calls) != 2 {
		t.Errorf("a click should select Español in Language: section %d, calls %q", m.setWin.section, f.calls)
	}

	m = clickOn(t, m, "Claude limits: off")
	m = clickOn(t, m, "Claude limits: off")
	if !slices.Equal(f.calls, []string{"palette mine", "apply night", "limits on"}) {
		t.Errorf("two clicks on the limits row should turn them on, calls %q", f.calls)
	}

	m = clickOn(t, m, "maximize")
	if m.setWin.capturing {
		t.Fatal("one click on a key row should only select it")
	}
	m = clickOn(t, m, "maximize")
	if !m.setWin.capturing || keyRows[m.setWin.keySel].action != config.ToggleMaximize {
		t.Errorf("a click on the selected key row should wait for its new key:\n%s", view(m))
	}
}
