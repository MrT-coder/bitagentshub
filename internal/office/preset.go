package office

import "github.com/MrT-coder/bitagentshub/internal/screen"

// presets are the themes that come with the office besides the built-in
// one. They change colors only and keep the built-in poses; a theme folder
// can still start from any of them.
var presets = []struct {
	name  string
	apply func(t *Theme)
}{
	{"night", func(t *Theme) {
		t.WallTop, t.WallFill, t.WallPanel, t.Baseboard = rgb(0x12151b), rgb(0x1b1f27), rgb(0x171a21), rgb(0x0d0f13)
		t.FloorA, t.FloorB, t.FloorSeam = rgb(0x262a33), rgb(0x22262e), rgb(0x181b21)
		t.Carpets = colors(0x243246, 0x30243f, 0x1f3a33, 0x3d3324, 0x3d2430)
		t.Agents = colors(0x7aa2f7, 0xbb9af7, 0x9ece6a, 0xe0af68, 0xf7768e, 0x7dcfff, 0xff9e64, 0x73daca)
	}},
	{"retro", func(t *Theme) {
		t.WallTop, t.WallFill, t.WallPanel, t.Baseboard = rgb(0x5b3a29), rgb(0xd9b38c), rgb(0xc9a37c), rgb(0x4a2f21)
		t.FloorA, t.FloorB, t.FloorSeam = rgb(0x3f6f6a), rgb(0x386560), rgb(0x2a4b47)
		t.Carpets = colors(0xd9774a, 0xe3b04b, 0x4f9d8b, 0xc05a6b, 0x7b6bb0)
		t.Agents = colors(0xff6f59, 0xffd23f, 0x3bceac, 0xee4266, 0x540d6e, 0x0ead69, 0xf18f01, 0x2e86ab)
	}},
	// The Gentleman theme of Gentleman.Dots (GentlemanGhostty/config), the
	// one gentle-ai is used with: its terminal palette, its background and
	// text in the windows, and its colors on the agents.
	{"gentle-ai", func(t *Theme) {
		t.WallTop, t.WallFill, t.WallPanel, t.Baseboard = rgb(0x0b0f1a), rgb(0x141a2b), rgb(0x111727), rgb(0x06080f)
		t.FloorA, t.FloorB, t.FloorSeam = rgb(0x1c2335), rgb(0x192032), rgb(0x10141f)
		t.Carpets = colors(0x263356, 0x3a2a40, 0x2b3a2a, 0x3d3624, 0x1f3a3a)
		t.Agents = colors(0xcb7c94, 0xb7cc85, 0xffe066, 0x7fb4ca, 0xff8dd7, 0x7aa89f, 0xde8fa8, 0xa3d4d5)
		t.UIBg, t.UIText, t.UIAccent = rgb(0x06080f), rgb(0xf3f6f9), rgb(0x7fb4ca)
		t.Terminal.Set = true
		copy(t.Terminal.C[:], colors(
			0x06080f, 0xcb7c94, 0xb7cc85, 0xffe066, 0x7fb4ca, 0xff8dd7, 0x7aa89f, 0xf3f6f9,
			0x8a8fa3, 0xde8fa8, 0xd1e8a9, 0xfff7b1, 0xa3d4d5, 0xffaeea, 0x7fb4ca, 0xf3f6f9,
		))
	}},
}

// PresetNames are the themes that come with the office, besides "default".
func PresetNames() []string {
	names := make([]string, len(presets))
	for i, p := range presets {
		names[i] = p.name
	}
	return names
}

// Preset returns a theme that comes with the office; "default" is the
// built-in theme.
func Preset(name string) (Theme, bool) {
	t := BuiltinTheme()
	if name == "default" {
		return t, true
	}
	for _, p := range presets {
		if p.name == name {
			t.Name = name
			p.apply(&t)
			return t, true
		}
	}
	return Theme{}, false
}

func colors(hex ...uint32) []screen.RGB {
	out := make([]screen.RGB, len(hex))
	for i, h := range hex {
		out[i] = rgb(h)
	}
	return out
}
