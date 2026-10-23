package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/i18n"
)

// speak switches the office to lang for one test.
func speak(t *testing.T, lang i18n.Lang) {
	t.Helper()
	before := i18n.Current()
	i18n.Set(lang)
	t.Cleanup(func() { i18n.Set(before) })
}

// sayOnly fails unless v has every want and none of the English in avoid.
func sayOnly(t *testing.T, what, v string, want, avoid []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(v, w) {
			t.Errorf("%s should say %q:\n%s", what, w, v)
		}
	}
	for _, a := range avoid {
		if strings.Contains(v, a) {
			t.Errorf("%s should not say %q in Spanish:\n%s", what, a, v)
		}
	}
}

func TestTheOfficeSpeaksSpanish(t *testing.T) {
	speak(t, i18n.Spanish)
	m, _, _ := twoAgentOffice(t)
	sayOnly(t, "the office", view(m),
		[]string{"2 estaciones", "0 trabajando", " oficina ", "q salir", "nuevo agente", "inactivo"},
		[]string{"stations", " office ", "q quit", "new agent", "idle"})

	m = press(m, key("p"))
	sayOnly(t, "the pet tab", view(m),
		[]string{"La mascota está durmiendo.", "ningún agente trabajando", "mascota", "ajustes", "esc cerrar"},
		[]string{"The pet is", "no agent working", "settings", "esc close"})
}

func TestTheLanguageSectionSwitchesTheOfficeAtOnce(t *testing.T) {
	speak(t, i18n.English)
	f := &fakeSettings{}
	m := settingsOffice(t, f)
	m = press(m, key("s"))
	if v := view(m); !strings.Contains(v, "Language") || !strings.Contains(v, "● English") || !strings.Contains(v, "  Español") {
		t.Fatalf("settings should list the languages, the active one marked:\n%s", v)
	}
	m = press(press(m, key("tab")), key("tab"))
	m = press(m, key("down"))
	m = press(m, key("enter"))
	if !slices.Equal(f.calls, []string{"language es"}) {
		t.Fatalf("enter on Español should apply es, calls = %q", f.calls)
	}
	if i18n.Current() != i18n.Spanish {
		t.Fatalf("the office should speak Spanish now, speaks %q", i18n.Current())
	}
	sayOnly(t, "settings after the switch", view(m),
		[]string{"Idioma", "● Español", "Idioma Español aplicado y guardado.", "Teclas", "cerrar ventana", "q salir"},
		[]string{"Language", "Keys", "close window", "q quit"})
}
