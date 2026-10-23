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

// runTheme handles "bah theme init <name> [--from <theme>]".
func runTheme(args []string) error {
	usage := errors.New("usage: bah theme init <name> [--from <theme>]")
	if len(args) < 2 || args[0] != "init" {
		return usage
	}
	from := ""
	switch {
	case len(args) == 4 && args[2] == "--from":
		from = args[3]
	case len(args) != 2:
		return usage
	}
	themes := config.ThemesDir(config.DefaultPath())
	var dir string
	var err error
	if from == "" {
		dir, err = themeInit(themes, args[1])
	} else {
		dir, err = themeInitFrom(themes, args[1], from)
	}
	if err != nil {
		return err
	}
	if from == "" {
		from = "built-in"
	}
	fmt.Printf("Wrote the %s theme to %s.\n", from, dir)
	fmt.Printf("Edit it, then select it in %s with:\n\n[theme]\nname = %q\n", config.DefaultPath(), args[1])
	return nil
}

// themeInit writes the built-in theme as themes/<name>, the starting point
// for a theme of one's own.
func themeInit(themes, name string) (string, error) {
	return writeThemeAs(themes, name, office.BuiltinTheme())
}

// themeInitFrom writes the theme named from, one that comes with the office
// or one in the themes folder, as themes/<name>.
func themeInitFrom(themes, name, from string) (string, error) {
	t, err := loadTheme(themes, from)
	if err != nil {
		return "", err
	}
	return writeThemeAs(themes, name, t)
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

// loadLook is what the office is drawn with: themes/<name>, "" being the
// built-in theme, in the colors of themes/<palette>, "" keeping its own.
func loadLook(themes, name, palette string) (office.Theme, error) {
	t := office.BuiltinTheme()
	if name != "" {
		var err error
		if t, err = loadTheme(themes, name); err != nil {
			return t, err
		}
	}
	switch palette {
	case "":
		return t, nil
	case "default":
		return t, errors.New(`the built-in theme sets no colors over another, so it cannot be a palette; leave palette out to keep the theme's own colors`)
	}
	p, err := loadTheme(themes, palette)
	if err != nil {
		return t, fmt.Errorf("palette: %w", err)
	}
	return office.WithPalette(t, p), nil
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
