package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/office"
)

// runTheme handles "bah theme init <name>".
func runTheme(args []string) error {
	if len(args) != 2 || args[0] != "init" {
		return errors.New("usage: bah theme init <name>")
	}
	dir, err := themeInit(config.ThemesDir(config.DefaultPath()), args[1])
	if err != nil {
		return err
	}
	fmt.Printf("Wrote the built-in theme to %s.\n", dir)
	fmt.Printf("Edit it, then select it in %s with:\n\n[theme]\nname = %q\n", config.DefaultPath(), args[1])
	return nil
}

// themeInit writes the built-in theme as themes/<name>, the starting point
// for a theme of one's own.
func themeInit(themes, name string) (string, error) {
	return writeThemeAs(themes, name, office.BuiltinTheme())
}

// writeThemeAs writes t as themes/<name>, named after its folder; an
// existing theme is never overwritten.
func writeThemeAs(themes, name string, t office.Theme) (string, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("%q is not a folder name", name)
	}
	if _, taken := office.Preset(name); taken {
		return "", fmt.Errorf("%q is a theme that comes with the office; pick another name", name)
	}
	dir := filepath.Join(themes, name)
	t.Name = name
	if err := office.WriteTheme(dir, t); err != nil {
		return "", err
	}
	return dir, nil
}

// loadTheme reads themes/<name>; a missing theme says how to make one.
func loadTheme(themes, name string) (office.Theme, error) {
	if t, ok := office.Preset(name); ok {
		return t, nil
	}
	dir := filepath.Join(themes, name)
	t, err := office.LoadTheme(dir)
	if !errors.Is(err, fs.ErrNotExist) {
		return t, err
	}
	if _, statErr := os.Stat(dir); statErr == nil {
		return t, fmt.Errorf("theme %q has no theme.toml in %s; every theme needs one, even if it only sets a name", name, dir)
	}
	return t, fmt.Errorf("theme %q is not in %s; `bah theme init %s` creates it from the built-in theme", name, themes, name)
}
