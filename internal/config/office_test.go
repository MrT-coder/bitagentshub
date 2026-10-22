package config

import (
	"strings"
	"testing"
)

func TestTheUISectionPicksTheLanguage(t *testing.T) {
	for _, lang := range []string{"en", "es"} {
		c, err := Load(write(t, "[ui]\nlanguage = \""+lang+"\"\n"))
		if err != nil || c.Language != lang {
			t.Errorf("language %s: got %q, %v", lang, c.Language, err)
		}
	}
	if c, err := Load(write(t, "[keys]\nprefix = \"ctrl+b\"\n")); err != nil || c.Language != "" {
		t.Errorf("no [ui] should leave the language to the system, got %q, %v", c.Language, err)
	}
	for _, bad := range []string{`language = "fr"`, `language = ""`, `language = 3`, `scale = 2`} {
		path := write(t, "[ui]\n"+bad+"\n")
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "ui.") {
			t.Errorf("%s: error %v should name the file and the ui field", bad, err)
		}
	}
}

func TestThePetSectionNamesThePet(t *testing.T) {
	c, err := Load(write(t, "[pet]\nname = \"Michi\"\n"))
	if err != nil || c.PetName != "Michi" {
		t.Errorf("pet name: got %q, %v", c.PetName, err)
	}
	if c, _ := Load(write(t, "")); c.PetName != "" {
		t.Errorf("no [pet] should leave it unnamed, got %q", c.PetName)
	}
	long := `name = "` + strings.Repeat("x", 21) + `"`
	for _, bad := range []string{`name = ""`, `name = "   "`, long, `name = "a\tb"`, `name = 3`, `color = "red"`} {
		path := write(t, "[pet]\n"+bad+"\n")
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "pet.") {
			t.Errorf("%s: error %v should name the file and the pet field", bad, err)
		}
	}
}

func TestTheTerminalSectionSetsTheHistory(t *testing.T) {
	if c, err := Load(write(t, "")); err != nil || c.Scrollback != 2000 {
		t.Errorf("no [terminal]: history %d, %v, want 2000", c.Scrollback, err)
	}
	if c, err := Load(write(t, "[terminal]\nscrollback = 500\n")); err != nil || c.Scrollback != 500 {
		t.Errorf("scrollback 500: got %d, %v", c.Scrollback, err)
	}
	for _, bad := range []string{`scrollback = 99`, `scrollback = 100001`, `scrollback = "lots"`, `history = 3`} {
		path := write(t, "[terminal]\n"+bad+"\n")
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "terminal.") {
			t.Errorf("%s: error %v should name the file and the terminal field", bad, err)
		}
	}
}
