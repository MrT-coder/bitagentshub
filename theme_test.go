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
