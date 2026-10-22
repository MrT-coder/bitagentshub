package config_test

import (
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/config"
)

func TestSaveLanguageChangesOnlyItsLine(t *testing.T) {
	body := "# mine\n[keys]\nprefix = \"ctrl+b\"\n"
	path := configFile(t, body)
	if err := config.SaveLanguage(path, "es"); err != nil {
		t.Fatal(err)
	}
	if got, want := contents(t, path), body+"\n[ui]\nlanguage = \"es\"\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	if err := config.SaveLanguage(path, "en"); err != nil {
		t.Fatal(err)
	}
	if c, err := config.Load(path); err != nil || c.Language != "en" {
		t.Errorf("after saving en: %q, %v", c.Language, err)
	}
	if err := config.SaveLanguage(path, ""); err != nil {
		t.Fatal(err)
	}
	if got, want := contents(t, path), body+"\n[ui]\n"; got != want {
		t.Errorf("an empty language should remove its line: %q, want %q", got, want)
	}
	if err := config.SaveLanguage(path, "fr"); err == nil || !strings.Contains(err.Error(), "fr") {
		t.Errorf("SaveLanguage(fr) = %v, want it refused", err)
	}
}

func TestSavePetNameChangesOnlyItsLine(t *testing.T) {
	body := "[theme]\nname = \"night\"\n"
	path := configFile(t, body)
	if err := config.SavePetName(path, `Mi "gato"`); err != nil {
		t.Fatal(err)
	}
	if got, want := contents(t, path), body+"\n[pet]\nname = \"Mi \\\"gato\\\"\"\n"; got != want {
		t.Errorf("file = %q, want %q", got, want)
	}
	if c, err := config.Load(path); err != nil || c.PetName != `Mi "gato"` || c.Theme != "night" {
		t.Errorf("after saving: pet %q theme %q, %v", c.PetName, c.Theme, err)
	}
	if err := config.SavePetName(path, "  "); err == nil {
		t.Error("a blank name should be refused")
	}
}
