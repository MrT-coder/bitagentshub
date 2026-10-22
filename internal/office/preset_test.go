package office

import (
	"reflect"
	"testing"
)

func TestPresetsComeWithTheOffice(t *testing.T) {
	if got := PresetNames(); !reflect.DeepEqual(got, []string{"night", "retro"}) {
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
	if _, ok := Preset("nope"); ok {
		t.Error("an unknown name is not a preset")
	}
}
