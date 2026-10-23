package office

import (
	"embed"
	"fmt"
	"io/fs"
	"slices"
)

// presetFS holds the themes that come with the office besides the built-in
// one: theme folders like any other, read the way LoadTheme reads a folder,
// so each one can still be a starting point.
//
//go:embed presets
var presetFS embed.FS

// presetNames are the folders in presets, in the order they are listed.
var presetNames = []string{"night", "retro", "gentle-ai"}

// PresetNames are the themes that come with the office, besides "default".
func PresetNames() []string { return slices.Clone(presetNames) }

// Preset returns a theme that comes with the office; "default" is the
// built-in theme.
func Preset(name string) (Theme, bool) {
	if name == "default" {
		return BuiltinTheme(), true
	}
	if !slices.Contains(presetNames, name) {
		return Theme{}, false
	}
	dir := "presets/" + name
	sub, err := fs.Sub(presetFS, dir)
	if err == nil {
		var t Theme
		if t, err = loadTheme(sub, dir, name); err == nil {
			return t, true
		}
	}
	// The presets are part of the program and their tests load every one.
	panic(fmt.Sprintf("preset %s: %v", name, err))
}
