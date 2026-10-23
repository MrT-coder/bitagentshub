package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/office"
)

func TestThemeInitWritesTheBuiltinThemeAndRefusesToOverwrite(t *testing.T) {
	themes := filepath.Join(t.TempDir(), "themes")
	dir, err := themeInit(themes, "mine")
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(themes, "mine") {
		t.Errorf("dir = %q", dir)
	}
	got, err := office.LoadTheme(dir)
	if err != nil {
		t.Fatalf("the written theme should load: %v", err)
	}
	if got.Name != "mine" {
		t.Errorf("name = %q, want the folder's name", got.Name)
	}
	if _, err := os.Stat(filepath.Join(dir, "characters", "typing.txt")); err != nil {
		t.Errorf("every pose should be written: %v", err)
	}
	if _, err := themeInit(themes, "mine"); err == nil {
		t.Error("a second init should refuse to overwrite the folder")
	}
	if _, err := themeInit(themes, "../escape"); err == nil {
		t.Error("a name must be a folder inside the themes folder")
	}
}

func TestThemeInitFromAPresetCopiesItsWindowAndTerminalColors(t *testing.T) {
	themes := filepath.Join(t.TempDir(), "themes")
	dir, err := themeInitFrom(themes, "mine", "gentle-ai")
	if err != nil {
		t.Fatal(err)
	}
	got, err := office.LoadTheme(dir)
	if err != nil {
		t.Fatalf("the written theme should load: %v", err)
	}
	want, _ := office.Preset("gentle-ai")
	if got.Name != "mine" {
		t.Errorf("name = %q, want the folder's name", got.Name)
	}
	if got.UIBg != want.UIBg || got.UIText != want.UIText || got.UIAccent != want.UIAccent {
		t.Errorf("ui = %v %v %v, want gentle-ai's", got.UIBg, got.UIText, got.UIAccent)
	}
	if got.Terminal != want.Terminal {
		t.Errorf("terminal = %v, want gentle-ai's %v", got.Terminal, want.Terminal)
	}
	if _, err := themeInitFrom(themes, "other", "nope"); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("error = %v, want it to name the missing theme", err)
	}
	if _, err := os.Stat(filepath.Join(themes, "other")); err == nil {
		t.Error("a missing source should not leave a folder behind")
	}
}

func TestThemeInitTakesFromAfterTheName(t *testing.T) {
	if err := runTheme([]string{"init", "mine", "--from"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("error = %v, want the usage", err)
	}
}

func TestLoadingAMissingThemeSaysHowToCreateIt(t *testing.T) {
	_, err := loadTheme(filepath.Join(t.TempDir(), "themes"), "nope")
	if err == nil || !strings.Contains(err.Error(), "nope") || !strings.Contains(err.Error(), "theme init nope") {
		t.Errorf("error = %v, want the name and the init command", err)
	}
}

func TestAThemeFolderWithoutItsManifestSaysSo(t *testing.T) {
	themes := filepath.Join(t.TempDir(), "themes")
	if err := os.MkdirAll(filepath.Join(themes, "t", "characters"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := loadTheme(themes, "t")
	if err == nil || !strings.Contains(err.Error(), "theme.toml") || strings.Contains(err.Error(), "theme init") {
		t.Errorf("error = %v, want it to ask for theme.toml, not to run theme init on an existing folder", err)
	}
}

func TestLoadLookDressesTheThemeInThePalette(t *testing.T) {
	themes := t.TempDir()
	if _, err := themeInitFrom(themes, "mine", "night"); err != nil {
		t.Fatal(err)
	}
	look, err := loadLook(themes, "mine", "gentle-ai")
	if err != nil {
		t.Fatal(err)
	}
	gentle, _ := office.Preset("gentle-ai")
	if look.Name != "mine" || look.FloorA != gentle.FloorA || look.UIBg != gentle.UIBg || look.Terminal != gentle.Terminal {
		t.Errorf("look = %s with floor %v, window %v; want mine in gentle-ai's colors", look.Name, look.FloorA, look.UIBg)
	}
	if plain, _ := loadLook(themes, "", ""); plain.Name != office.BuiltinTheme().Name {
		t.Errorf("no theme and no palette is the built-in theme, got %s", plain.Name)
	}
	if _, err := loadLook(themes, "mine", "nope"); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Errorf("a missing palette = %v, want an error naming it", err)
	}
	if _, err := loadLook(themes, "mine", "default"); err == nil || !strings.Contains(err.Error(), "palette") {
		t.Errorf("the built-in theme as palette = %v, want an error: it sets no colors over another", err)
	}
}
