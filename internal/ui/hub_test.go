package ui

import (
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/version"
)

// hubTabs is the tab row every hub tab shows in its top border.
var hubTabs = []string{" pet ", " usage ", " settings ", " about "}

func assertHubTab(t *testing.T, m Model, tab string, want ...string) {
	t.Helper()
	if !m.hubOpen || tabName(m.hubTab) != tab {
		t.Fatalf("hub open %v on %q, want it open on %q", m.hubOpen, tabName(m.hubTab), tab)
	}
	v := view(m)
	for _, w := range append(append([]string(nil), hubTabs...), want...) {
		if !strings.Contains(v, w) {
			t.Errorf("%s tab missing %q:\n%s", tab, w, v)
		}
	}
}

func TestPOpensTheHubOnThePetTabAndTheSameKeyClosesIt(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(m, key("p"))
	assertHubTab(t, m, "pet", "sleeping", "no agent working", "no limits reported")
	m = press(m, key("p"))
	if m.hubOpen {
		t.Fatal("p should close the hub")
	}
	m = press(press(m, key("ctrl+a")), key("p"))
	assertHubTab(t, m, "pet")
	m = press(press(m, key("ctrl+a")), key("p"))
	if m.hubOpen {
		t.Fatal("ctrl+a p should close the hub")
	}
	for _, k := range []string{"esc", "q"} {
		m = press(press(m, key("p")), key(k))
		if m.hubOpen {
			t.Errorf("%s should close the hub", k)
		}
	}
}

func TestArrowsGoRoundTheHubTabs(t *testing.T) {
	m := spendingOffice(t)
	m = press(m, key("p"))
	m = press(m, key("right"))
	assertHubTab(t, m, "usage", "portal-front", "1.2M")
	m = press(m, key("right"))
	assertHubTab(t, m, "settings", "Settings are not available here.")
	m = press(m, key("right"))
	assertHubTab(t, m, "about", "BitAgentsHub "+version.String(), "MIT License · github.com/MrT-coder/bitagentshub", "Early preview: tested on Windows with Claude Code so far.")
	m = press(m, key("right"))
	assertHubTab(t, m, "pet")
	m = press(m, key("left"))
	assertHubTab(t, m, "about")
}

func TestTheUsageAndSettingsKeysOpenTheirHubTabs(t *testing.T) {
	m := spendingOffice(t)
	m = press(m, key("u"))
	assertHubTab(t, m, "usage", "1.2M")
	m = press(m, key("s"))
	assertHubTab(t, m, "settings")
	m = press(press(m, key("ctrl+a")), key("u"))
	assertHubTab(t, m, "usage")
	m = press(m, key("u"))
	if m.hubOpen {
		t.Fatal("u on the usage tab should close the hub, as it closed the usage window")
	}
	m = press(press(m, key("ctrl+a")), key("s"))
	assertHubTab(t, m, "settings")
}

func TestClickingThePetOpensTheHub(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	x, y, w, h := m.layout.PetCells()
	m = press(m, click(1+x+w/2, officeY+y+h/2))
	assertHubTab(t, m, "pet")
}

func TestTheAboutTabShowsTheVersionAndTheChangelog(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = m.WithChangelog("# Changelog\n\n## v9.9.9 (2027-01-01)\n\n- A thing that changed.\n")
	m = press(press(m, key("p")), key("left"))
	assertHubTab(t, m, "about", "BitAgentsHub "+version.String(), "## v9.9.9 (2027-01-01)", "- A thing that changed.")
	if strings.Contains(view(m), "# Changelog") {
		t.Errorf("the changelog's own title should not show:\n%s", view(m))
	}

	m, _, _ = twoAgentOffice(t)
	m = press(press(m, key("p")), key("left"))
	assertHubTab(t, m, "about", "No changelog in this build.")
}

func TestTheHubIsTheOnlyKeyInTheFooterAndTheSettingsKeys(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	v := view(m)
	if !strings.Contains(v, "p hub") || strings.Contains(v, "u usage") || strings.Contains(v, "s settings") {
		t.Errorf("footer should offer p hub only:\n%s", v)
	}
	m = press(m, key("s"))
	v = view(m)
	if !strings.Contains(v, "hub") || !strings.Contains(v, "ctrl+a p") {
		t.Errorf("settings keys should list the hub and its key:\n%s", v)
	}
	for _, gone := range []string{"usage           ctrl+a u", "settings        ctrl+a s"} {
		if strings.Contains(v, gone) {
			t.Errorf("settings keys still list %q:\n%s", gone, v)
		}
	}
}
