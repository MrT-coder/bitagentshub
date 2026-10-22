package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// overPet is a buttonless mouse move onto the middle of the pet's corner.
func overPet(m Model) tea.MouseMotionMsg {
	x, y, w, h := m.layout.PetCells()
	return tea.MouseMotionMsg{X: 1 + x + w/2, Y: officeY + y + h/2}
}

func TestHoveringThePetShowsItsName(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = m.WithPetName("Michi")
	m = press(m, overPet(m))
	if !strings.Contains(view(m), "Michi") {
		t.Fatalf("hovering the pet should show its name:\n%s", view(m))
	}
	m = press(m, tea.MouseMotionMsg{X: 60, Y: officeY + 2})
	if strings.Contains(view(m), "Michi") {
		t.Errorf("moving away should hide the name:\n%s", view(m))
	}

	m, _, _ = twoAgentOffice(t)
	m = press(m, overPet(m))
	if !strings.Contains(view(m), " pet ") {
		t.Errorf("an unnamed pet is called pet:\n%s", view(m))
	}
}

func TestAMouseMoveWithoutAButtonNeverSelectsInAWindow(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m, _ = m.openWindow("front")
	r := m.paneRects()[0]
	m = press(m, tea.MouseMotionMsg{X: 1 + r.x + 2, Y: officeY + r.y + 2})
	if m.win.sel != nil {
		t.Error("a buttonless move should not start a selection")
	}
}

func TestRRenamesThePetAndSavesTheName(t *testing.T) {
	f := &fakeSettings{}
	m, _, _ := twoAgentOffice(t)
	m = m.WithSettings(f).WithPetName("Michi")
	m = press(m, key("p"))
	if v := view(m); !strings.Contains(v, "Michi") || !strings.Contains(v, "r rename") {
		t.Fatalf("the pet tab should show the name and how to change it:\n%s", v)
	}
	m = press(m, key("r"))
	for range "Michi" {
		m = press(m, key("backspace"))
	}
	for _, k := range []string{"L", "u", "n", "a"} {
		m = press(m, key(k))
	}
	if !strings.Contains(view(m), "Luna█") {
		t.Fatalf("the new name should be typed in the tab:\n%s", view(m))
	}
	m = press(m, key("enter"))
	if !slices.Contains(f.calls, "pet Luna") || m.petName != "Luna" {
		t.Fatalf("enter should save the name: calls %v, name %q", f.calls, m.petName)
	}
	if !m.hubOpen || !strings.Contains(view(m), "Luna") {
		t.Errorf("the tab should stay open with the new name:\n%s", view(m))
	}

	m = press(m, key("r"))
	m = press(m, key("x"))
	m = press(m, key("esc"))
	if m.petName != "Luna" || !m.hubOpen || slices.Contains(f.calls, "pet Lunax") {
		t.Errorf("esc should drop the edit and keep the hub open: name %q, open %v", m.petName, m.hubOpen)
	}

	m = press(m, key("r"))
	for range "Luna" {
		m = press(m, key("backspace"))
	}
	m = press(m, key("enter"))
	if m.petName != "Luna" || len(f.calls) != 1 {
		t.Errorf("a blank name should not be saved: name %q, calls %v", m.petName, f.calls)
	}
}
