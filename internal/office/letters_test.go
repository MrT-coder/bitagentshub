package office

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// ownLetters is a theme.toml giving four colors of its own and a letter for
// each: Z for the characters, Y for the chair, X for the cat and D for the
// floor.
const ownLetters = "[custom]\nalien = \"#fe0a01\"\nhull = \"#fe0a02\"\neye_3 = \"#fe0a03\"\ndeck = \"#fe0a04\"\n" +
	"[letters]\nZ = \"custom.alien\"\nY = \"custom.hull\"\nX = \"custom.eye_3\"\nD = \"custom.deck\"\n"

// lettered is frames with every letter but '.' replaced by ch.
func lettered(frames []frame, ch byte) []frame {
	out := make([]frame, len(frames))
	for i, f := range frames {
		for y, row := range f {
			b := []byte(row)
			for x := range b {
				if b[x] != '.' {
					b[x] = ch
				}
			}
			out[i][y] = string(b)
		}
	}
	return out
}

// A theme's own letters draw with the colors they name in every grid file:
// the characters' poses, the furniture, the pet's frames and the room.
func TestLoadTheme_DrawsTheThemesOwnLettersInEveryGrid(t *testing.T) {
	files := map[string]string{
		"theme.toml":          ownLetters,
		"furniture/chair.txt": pieceText(solidPiece(pieceChair, 'Y')),
		"room/floor.txt":      "D\n",
	}
	for p, spec := range poseSpecs {
		files["characters/"+spec.name+".txt"] = gridText(lettered(BuiltinTheme().poses[p], 'Z'))
	}
	for m := pet.Mood(0); int(m) < moodCount; m++ {
		frames := BuiltinTheme().moods[m]
		out := make([]catFrame, len(frames))
		for i, f := range frames {
			out[i] = f.with(0, "X"+f[0][1:])
		}
		files["pet/"+m.String()+".txt"] = moodText(out)
	}
	th, err := LoadTheme(writeFiles(t, files))
	if err != nil {
		t.Fatal(err)
	}
	useTheme(t, th)
	for name, c := range map[string]screen.RGB{"character": rgb(0xfe0a01), "chair": rgb(0xfe0a02), "floor": rgb(0xfe0a04)} {
		if !officeShows(c) {
			t.Errorf("the %s's own letter, %s, is never drawn", name, formatColor(c))
		}
	}
	l := Build(nil, testW, testH)
	buf := screen.New(testW, testH)
	DrawPet(buf, l, pet.Busy, 4)
	if got, want := buf.PixelAt(l.Pet.x+1, l.Pet.y+2), rgb(0xfe0a03); got != want {
		t.Errorf("the cat's top-left pixel = %s, want %s from its own letter X", formatColor(got), formatColor(want))
	}
}

// Every [custom] and [letters] setting the office cannot use is an error
// naming the file, the line and the setting.
func TestLoadTheme_NamesTheLineAndSettingOfALetterOrCustomColorItCannotUse(t *testing.T) {
	const free = "D I J O Q V X Y Z c d e f g h i j l m n o p q r t u v x y z"
	tests := []struct {
		name, manifest, want string
	}{
		{"a key longer than one letter", "[letters]\nZZ = \"furniture.glass\"\n",
			":2: letters.ZZ: a letter is one letter from a to z or A to Z"},
		{"a key that is not a letter", "[letters]\n\"1\" = \"furniture.glass\"\n",
			":2: letters.1: a letter is one letter from a to z or A to Z"},
		{"a letter a built-in grid uses", "[letters]\nA = \"furniture.glass\"\n",
			":2: letters.A: the built-in grids use A; free letters are " + free},
		{"an unknown role", "[letters]\nZ = \"furniture.nope\"\n",
			":2: letters.Z: unknown role \"furniture.nope\"; use a color like \"furniture.glass\" or one of [custom] like \"custom.antenna\""},
		{"a custom role the theme does not give", "[custom]\nhull = \"#000000\"\n[letters]\nZ = \"custom.nope\"\n",
			":4: letters.Z: unknown role \"custom.nope\""},
		{"a list role", "[letters]\nZ = \"palette.agents\"\n",
			":2: letters.Z: palette.agents is a list of colors; a letter draws with one"},
		{"a role that is not a string", "[letters]\nZ = 3\n",
			":2: letters.Z: want a role like \"custom.antenna\", got 3"},
		{"a custom key with other characters", "[custom]\nAntenna = \"#000000\"\n",
			":2: custom.Antenna: a key is lowercase letters, digits and _"},
		{"a bad custom color", "[custom]\nantenna = \"green\"\n",
			":2: custom.antenna: \"green\" is not a #rrggbb color"},
		{"a list custom color", "[custom]\nantenna = [\"#000000\"]\n",
			":2: custom.antenna: want one color like \"#2b1d14\", got a list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"theme.toml": tt.manifest})
			_, err := LoadTheme(dir)
			want := filepath.Join(dir, "theme.toml") + tt.want
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("error = %v\nwant it to contain %s", err, want)
			}
		})
	}
}

// A grid file's unknown letter keeps today's message, and lists the
// theme's own letters when it has some.
func TestLoadTheme_AnUnknownLetterListsTheThemesOwnLetters(t *testing.T) {
	typing := gridText(lettered(BuiltinTheme().poses[poseTyping], 'B'))
	typing = strings.Replace(typing, "BBBBBBBB", "QBBBBBBB", 1)
	tests := []struct {
		name, manifest, file, body, want string
	}{
		{"pose without letters", "", "characters/typing.txt", typing,
			"typing.txt:8: pose typing: unknown letter 'Q' in column 1; use H S E B b P F W w or ."},
		{"pose with letters", "[letters]\nY = \"furniture.glass\"\ng = \"room.baseboard\"\n", "characters/typing.txt", typing,
			"typing.txt:8: pose typing: unknown letter 'Q' in column 1; use H S E B b P F W w or ., or this theme's letters Y g"},
		{"piece with letters", "[letters]\nY = \"furniture.glass\"\n", "furniture/chair.txt", pieceText(solidPiece(pieceChair, 'Q')),
			"chair.txt:1: piece chair: unknown letter 'Q' in column 1; use B E or ., or this theme's letters Y"},
		{"tile with letters", "[letters]\nY = \"furniture.glass\"\n", "room/floor.txt", "Q\n",
			"floor.txt:1: tile floor: unknown letter 'Q' in column 1; use A B S, or this theme's letters Y"},
		{"pet with letters", "[letters]\nY = \"furniture.glass\"\n", "pet/alert.txt", strings.Replace(moodText(BuiltinTheme().moods[pet.Alert]), ".K......K...", "QK......K...", 1),
			"alert.txt:1: pet alert: unknown letter 'Q' in column 1; use K k E N W or ., or this theme's letters Y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadTheme(writeFiles(t, map[string]string{"theme.toml": tt.manifest, tt.file: tt.body}))
			if err == nil || !strings.HasSuffix(err.Error(), tt.want) {
				t.Errorf("error = %v\nwant it to end in %s", err, tt.want)
			}
		})
	}
}

// A palette's [custom] colors replace the scene's; the scene keeps its
// letters and shapes, and a letter of a standard role takes the palette's
// color for it.
func TestWithPalette_DressesTheScenesOwnLettersInThePalettesColors(t *testing.T) {
	scene, err := LoadTheme(writeFiles(t, map[string]string{
		"theme.toml":           "[custom]\nhull = \"#111111\"\nkeel = \"#121212\"\n[letters]\nY = \"custom.hull\"\ng = \"furniture.glass\"\n",
		"furniture/chair.txt":  pieceText(solidPiece(pieceChair, 'Y')),
		"furniture/window.txt": pieceText(solidPiece(pieceWindow, 'g')),
	}))
	if err != nil {
		t.Fatal(err)
	}
	palette, err := LoadTheme(writeFiles(t, map[string]string{
		"theme.toml": "[custom]\nhull = \"#222222\"\n[furniture]\nglass = \"#333333\"\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	got := WithPalette(scene, palette)
	if want := map[string]screen.RGB{"hull": rgb(0x222222), "keel": rgb(0x121212)}; !reflect.DeepEqual(got.Custom, want) {
		t.Errorf("Custom = %v, want %v: the palette's hull over the scene's, the scene's keel", got.Custom, want)
	}
	if !got.sets("custom.hull") || !got.sets("custom.keel") {
		t.Error("the dressed scene sets custom.hull and custom.keel")
	}
	if !reflect.DeepEqual(got.letters, scene.letters) || !reflect.DeepEqual(got.pieces, scene.pieces) {
		t.Error("the scene keeps its letters and its shapes")
	}
	if scene.Custom["hull"] != rgb(0x111111) {
		t.Error("WithPalette must not change the scene it was given")
	}
	useTheme(t, got)
	for role, c := range map[string]screen.RGB{"custom.hull": rgb(0x222222), "furniture.glass": rgb(0x333333)} {
		if !officeShows(c) {
			t.Errorf("a letter of %s draws %s, the palette's, nowhere", role, formatColor(c))
		}
	}
	if officeShows(rgb(0x111111)) {
		t.Error("the scene's hull is still drawn under the palette's")
	}
}

func TestWriteTheme_WritesTheThemesOwnColorsAndLettersBack(t *testing.T) {
	want, err := LoadTheme(writeFiles(t, map[string]string{
		"theme.toml":          ownLetters,
		"furniture/chair.txt": pieceText(solidPiece(pieceChair, 'Y')),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !want.sets("custom.alien") || want.sets("custom.nope") {
		t.Error("a theme sets the custom colors its theme.toml gives, and only those")
	}
	dir := filepath.Join(t.TempDir(), "mine")
	if err := WriteTheme(dir, want); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "theme.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"\n[custom]\n", "\nalien = \"#fe0a01\"\ndeck = \"#fe0a04\"\n", "\n[letters]\n", "\nD = \"custom.deck\"\nX = \"custom.eye_3\"\n"} {
		if !strings.Contains(string(b), line) {
			t.Errorf("theme.toml lacks %q:\n%s", line, b)
		}
	}
	got, err := LoadTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !sameRoundTrip(t, got, want) {
		t.Errorf("round trip differs:\n got %+v\nwant %+v", got, want)
	}
}
