package office

import (
	"reflect"
	"testing"
)

// gentle-ai is the Gentleman theme of Gentleman.Dots (GentlemanGhostty/config).
func TestGentleAIUsesTheGentlemanColors(t *testing.T) {
	p, ok := Preset("gentle-ai")
	if !ok {
		t.Fatal("gentle-ai is not a preset")
	}
	want := []uint32{
		0x06080f, 0xcb7c94, 0xb7cc85, 0xffe066, 0x7fb4ca, 0xff8dd7, 0x7aa89f, 0xf3f6f9,
		0x8a8fa3, 0xde8fa8, 0xd1e8a9, 0xfff7b1, 0xa3d4d5, 0xffaeea, 0x7fb4ca, 0xf3f6f9,
	}
	if !p.Terminal.Set {
		t.Fatal("gentle-ai sets the terminal palette")
	}
	for i, h := range want {
		if p.Terminal.C[i] != rgb(h) {
			t.Errorf("color %d = %s, want #%06x", i, formatColor(p.Terminal.C[i]), h)
		}
	}
	if p.UIBg != rgb(0x06080f) || p.UIText != rgb(0xf3f6f9) {
		t.Errorf("window = %s on %s, want #f3f6f9 on #06080f", formatColor(p.UIText), formatColor(p.UIBg))
	}
}

func TestPresetsComeWithTheOffice(t *testing.T) {
	if got := PresetNames(); !reflect.DeepEqual(got, []string{"night", "retro", "gentle-ai"}) {
		t.Errorf("PresetNames = %v", got)
	}
	builtin := BuiltinTheme()
	if d, ok := Preset("default"); !ok || !reflect.DeepEqual(d, builtin) {
		t.Error(`Preset("default") should be the built-in theme`)
	}
	for _, name := range PresetNames() {
		p, ok := Preset(name)
		if !ok || p.Name != name {
			t.Fatalf("Preset(%q) = %q, %v", name, p.Name, ok)
		}
		if p.FloorA == builtin.FloorA || p.WallFill == builtin.WallFill || reflect.DeepEqual(p.Carpets, builtin.Carpets) {
			t.Errorf("%s should change the room colors", name)
		}
		if !reflect.DeepEqual(p.poses, builtin.poses) {
			t.Errorf("%s keeps the built-in poses", name)
		}
		for _, list := range [][]any{{p.Hair}, {p.Skin}, {p.Pants}, {p.Agents}, {p.Carpets}} {
			if reflect.ValueOf(list[0]).Len() == 0 {
				t.Errorf("%s has an empty color list", name)
			}
		}
	}
	if builtin.Terminal.Set {
		t.Error("the built-in theme keeps the emulator's own terminal colors")
	}
	if _, ok := Preset("nope"); ok {
		t.Error("an unknown name is not a preset")
	}
}
