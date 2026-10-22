package i18n

import "testing"

func TestTheLocaleNamesPickTheLanguage(t *testing.T) {
	for _, c := range []struct {
		names []string
		want  Lang
	}{
		{nil, English},
		{[]string{"es-ES"}, Spanish},
		{[]string{"es_AR.UTF-8"}, Spanish},
		{[]string{"ES"}, Spanish},
		{[]string{"en-US", "es-ES"}, English},
		{[]string{"C", "es-MX"}, Spanish},
		{[]string{"", "fr-FR"}, English},
		{[]string{"est"}, English},
	} {
		if got := fromLocales(c.names); got != c.want {
			t.Errorf("%q: %v, want %v", c.names, got, c.want)
		}
	}
}

func TestTextFollowsTheCurrentLanguage(t *testing.T) {
	defer Set(Current())
	Set(English)
	if got := Tf("%d agents working", 2); got != "2 agents working" {
		t.Errorf("English: %q", got)
	}
	Set(Spanish)
	if got := Tf("%d agents working", 2); got != "2 agentes trabajando" {
		t.Errorf("Spanish: %q", got)
	}
	if got := T("not in the catalog"); got != "not in the catalog" {
		t.Errorf("a missing text should stay English, got %q", got)
	}
	if Spanish.Name() != "Español" || English.Name() != "English" {
		t.Error("languages are named in themselves")
	}
}
