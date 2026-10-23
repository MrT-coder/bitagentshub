package office

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

func TestBuild_KeepsThePetInTheBottomLeftCorner(t *testing.T) {
	for _, n := range []int{0, 1, 4} {
		var agents []claude.Agent
		for i := range n {
			agents = append(agents, agent(string(rune('a'+i)), "g", "p", claude.StateTool))
		}
		l := Build(agents, testW, testH)
		p := l.Pet
		if p.x != 2 || p.y+p.h != l.H || p.w != petW || p.h != petH {
			t.Fatalf("%d agents: pet corner = %+v, want x 2 on the bottom edge of %d", n, p, l.H)
		}
		for _, it := range l.Items {
			if p.overlaps(rect{it.X, it.Y, it.W, it.H}) {
				t.Errorf("%d agents: %s at %d,%d sits on the pet's corner", n, it.Kind, it.X, it.Y)
			}
		}
		for y := p.y - spriteH; y < p.y+p.h; y++ {
			for x := p.x - spriteW; x < p.x+p.w; x++ {
				if feet := (rect{x + 1, y + 12, spriteW - 2, 2}); feet.overlaps(p) && l.Walkable(x, y) {
					t.Fatalf("%d agents: an agent could stand on the pet at %d,%d", n, x, y)
				}
			}
		}
	}
}

func TestPetHit_IsTheCornerInCells(t *testing.T) {
	l := Build(nil, testW, testH)
	x, y := l.Pet.x+l.Pet.w/2, (l.Pet.y+l.Pet.h/2)/2
	if !PetHit(l, x, y) {
		t.Errorf("cell %d,%d in the pet's corner is not a hit", x, y)
	}
	for _, c := range [][2]int{{l.Pet.x + l.Pet.w, y}, {x, l.Pet.y/2 - 1}, {x, testH}} {
		if PetHit(l, c[0], c[1]) {
			t.Errorf("cell %v outside the corner is a hit", c)
		}
	}
}

// petPixels is what the pet's corner shows.
func petPixels(l Layout, mood pet.Mood, tick int) string {
	buf := screen.New(testW, testH)
	DrawPet(buf, l, mood, tick)
	lines := buf.Lines()
	out := ""
	for y := l.Pet.y/2 - 2; y < (l.Pet.y+l.Pet.h)/2; y++ {
		out += lines[y] + "\n" // colors and text, bubbles above the bed included
	}
	return out
}

func TestDrawPet_EveryMoodLooksDifferentAndMoves(t *testing.T) {
	l := Build(nil, testW, testH)
	moods := []pet.Mood{pet.Sleeping, pet.Busy, pet.Tired, pet.Exhausted, pet.Hungry, pet.Alert, pet.Eating}
	seen := map[string]pet.Mood{}
	for _, m := range moods {
		a := petPixels(l, m, 0)
		if other, ok := seen[a]; ok {
			t.Errorf("%v looks like %v", m, other)
		}
		seen[a] = m
		moved := false
		for tick := 1; tick < 64; tick++ {
			if petPixels(l, m, tick) != a {
				moved = true
				break
			}
		}
		if !moved {
			t.Errorf("%v never moves", m)
		}
	}
}

func TestDrawPet_TheBowlIsFullUnlessHungry(t *testing.T) {
	l := Build(nil, testW, testH)
	bx, by := l.Pet.x+bowlDX+1, l.Pet.y+bowlDY
	for _, c := range []struct {
		mood pet.Mood
		full bool
	}{{pet.Busy, true}, {pet.Hungry, false}} {
		buf := screen.New(testW, testH)
		DrawPet(buf, l, c.mood, 0)
		if got := buf.PixelAt(bx, by) == cur().Pet.Food; got != c.full {
			t.Errorf("%v: food in the bowl = %v, want %v", c.mood, got, c.full)
		}
	}
}

// petHash draws the pet's corner in every mood over ticks 0..80 and hashes
// what it shows.
func petHash() string {
	l := Build(nil, testW, testH)
	h := sha256.New()
	for m := pet.Sleeping; m <= pet.Eating; m++ {
		for tick := 0; tick <= 80; tick++ {
			fmt.Fprintf(h, "%v %d\n%s", m, tick, petPixels(l, m, tick))
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// petRef was captured from the drawing code before pet files existed, so
// the built-in pet must keep every mood and tick pixel for pixel.
const petRef = "3d2a0abe0500411224a0c90eed2b4a98fd88ca7d5a8ba33d3ef1ff7cd3cc2db2"

func TestBuiltinTheme_DrawsThePetExactlyAsBefore(t *testing.T) {
	if got := petHash(); got != petRef {
		t.Errorf("pet hash = %s, want %s", got, petRef)
	}
}

// moodText is a pet file holding frames.
func moodText(frames []catFrame) string {
	parts := make([]string, len(frames))
	for i, f := range frames {
		parts[i] = strings.Join(f[:], "\n")
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// eyed is frames with an extra eye in the top-left pixel, which every
// built-in frame leaves transparent.
func eyed(frames []catFrame) []catFrame {
	out := make([]catFrame, len(frames))
	for i, f := range frames {
		out[i] = f.with(0, "E"+f[0][1:])
	}
	return out
}

func TestLoadTheme_TakesEveryAbsentPetFileFromTheBuiltinTheme(t *testing.T) {
	for m := pet.Mood(0); int(m) < moodCount; m++ {
		t.Run(m.String(), func(t *testing.T) {
			frames := eyed(BuiltinTheme().moods[m])
			dir := writeFiles(t, map[string]string{
				"theme.toml":                 "name = \"mine\"\n",
				"pet/" + m.String() + ".txt": moodText(frames),
			})
			got, err := LoadTheme(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := BuiltinTheme()
			want.Name = "mine"
			want.moods[m] = frames
			want.set = map[string]bool{}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("LoadTheme with only %s.txt differs from the built-in theme elsewhere", m)
			}
		})
	}
}

// A pet file is what the corner shows: each mood's frames with an extra
// eye show it at every tick, and the bed and bowl files fill pixels the
// built-in ones leave empty.
func TestLoadTheme_DrawsThePetFiles(t *testing.T) {
	eye := rgb(0xfe0102)
	files := map[string]string{
		"theme.toml":   "[pet]\neye = \"#fe0102\"\nbed_pad = \"#fe0103\"\nbowl = \"#fe0104\"\n",
		"pet/bed.txt":  pieceText(solidPiece(pieceBed, 'P')),
		"pet/bowl.txt": "BBBB\nBBBB\nBBBB\n",
	}
	for m := pet.Mood(0); int(m) < moodCount; m++ {
		files["pet/"+m.String()+".txt"] = moodText(eyed(BuiltinTheme().moods[m]))
	}
	th, err := LoadTheme(writeFiles(t, files))
	if err != nil {
		t.Fatal(err)
	}
	useTheme(t, th)
	l := Build(nil, testW, testH)
	p := l.Pet
	for m := pet.Mood(0); int(m) < moodCount; m++ {
		for tick := 0; tick <= 80; tick++ {
			buf := screen.New(testW, testH)
			DrawPet(buf, l, m, tick)
			if got := buf.PixelAt(p.x+1, p.y+2); got != eye {
				t.Fatalf("%v at tick %d: the cat's top-left pixel = %s, want the eye from %s.txt", m, tick, formatColor(got), m)
			}
			if got, want := buf.PixelAt(p.x, p.y+bedDY), rgb(0xfe0103); got != want {
				t.Fatalf("%v: the bed's top-left pixel = %s, want %s from bed.txt", m, formatColor(got), formatColor(want))
			}
			if got, want := buf.PixelAt(p.x+bowlDX, p.y+bowlDY), rgb(0xfe0104); got != want {
				t.Fatalf("%v: the bowl's top-left pixel = %s, want %s from bowl.txt", m, formatColor(got), formatColor(want))
			}
		}
	}
}

func TestDrawPet_AThemedBowlIsFullUnlessHungry(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"theme.toml":   "[pet]\nfood = \"#fe0102\"\n",
		"pet/bowl.txt": "FFFF\nFBBF\n.BB.\n",
	})
	th, err := LoadTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	useTheme(t, th)
	l := Build(nil, testW, testH)
	bx, by := l.Pet.x+bowlDX, l.Pet.y+bowlDY
	for _, c := range []struct {
		mood pet.Mood
		full bool
	}{{pet.Busy, true}, {pet.Eating, true}, {pet.Hungry, false}} {
		buf := screen.New(testW, testH)
		DrawPet(buf, l, c.mood, 0)
		for _, at := range [][2]int{{bx, by}, {bx + 3, by}, {bx, by + 1}} {
			if got := buf.PixelAt(at[0], at[1]) == rgb(0xfe0102); got != c.full {
				t.Errorf("%v: food at %v = %v, want %v", c.mood, at, got, c.full)
			}
		}
	}
}

func TestLoadTheme_NamesTheFileLineAndWhatAPetFileMustBe(t *testing.T) {
	builtin := BuiltinTheme().moods
	busy := moodText(builtin[pet.Busy])
	tests := []struct {
		name  string
		file  string
		body  string
		line  int // 0 when there is no line to name
		parts []string
	}{
		{"row too wide", "busy.txt", strings.Replace(busy, "..K....K....\n", "..K....K.....\n", 1),
			1, []string{"pet busy", "13 wide", "want 12"}},
		{"frame too short", "alert.txt", moodText(builtin[pet.Alert])[13:],
			1, []string{"pet alert", "frame 1 has 7 rows", "want 8"}},
		{"unknown letter", "hungry.txt", strings.Replace(moodText(builtin[pet.Hungry]), ".KEKKKKEK...", ".KEKKKKEX...", 1),
			4, []string{"pet hungry", "'X'", "column 9", "use K k E N W or ."}},
		{"too many frames", "sleeping.txt", moodText(append(append([]catFrame(nil), builtin[pet.Sleeping]...), builtin[pet.Sleeping][0])),
			0, []string{"pet sleeping", "has 3 frames", "want 2"}},
		{"too few frames", "busy.txt", moodText(builtin[pet.Busy][:2]),
			0, []string{"pet busy", "has 2 frames", "want 3"}},
		{"unknown pet file", "dog.txt", busy,
			0, []string{"unknown pet file", "sleeping", "busy", "eating", "bed", "bowl"}},
		{"bed too wide", "bed.txt", strings.Repeat("PPPPPPPPPPPPPPP\n", 4),
			1, []string{"piece bed", "15 wide", "want 14"}},
		{"bowl too tall", "bowl.txt", strings.Repeat("BBBB\n", 4),
			4, []string{"piece bowl", "4 rows", "want 3"}},
		{"unknown bowl letter", "bowl.txt", "BBBB\nBKBB\nBBBB\n",
			2, []string{"piece bowl", "'K'", "column 2", "use B F or ."}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"theme.toml": "name = \"mine\"\n", "pet/" + tt.file: tt.body})
			_, err := LoadTheme(dir)
			if err == nil {
				t.Fatal("LoadTheme accepted the theme")
			}
			msg := err.Error()
			path := filepath.Join(dir, "pet", tt.file)
			want := append([]string{path}, tt.parts...)
			if tt.line > 0 {
				want = append(want, fmt.Sprintf("%s:%d:", path, tt.line))
			}
			for _, w := range want {
				if !strings.Contains(msg, w) {
					t.Errorf("error %q does not mention %q", msg, w)
				}
			}
		})
	}
}

func TestWriteTheme_WritesEveryPetFileLoadThemeReadsBack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mine")
	want := BuiltinTheme()
	want.Name = "mine"
	want.moods[pet.Busy] = eyed(want.moods[pet.Busy])
	want.pieces[pieceBowl] = solidPiece(pieceBowl, 'F')
	if err := WriteTheme(dir, want); err != nil {
		t.Fatal(err)
	}
	for m := pet.Mood(0); int(m) < moodCount; m++ {
		if _, err := os.Stat(filepath.Join(dir, "pet", m.String()+".txt")); err != nil {
			t.Errorf("pet file: %v", err)
		}
	}
	for _, name := range []string{"bed", "bowl"} {
		if _, err := os.Stat(filepath.Join(dir, "pet", name+".txt")); err != nil {
			t.Errorf("pet file: %v", err)
		}
	}
	got, err := LoadTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !sameRoundTrip(t, got, want) {
		t.Errorf("round trip differs:\n got %q\nwant %q", got.moods, want.moods)
	}
}
