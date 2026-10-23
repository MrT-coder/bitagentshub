package office

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// tile is n rows of row.
func tile(n int, row string) piece {
	rows := make(piece, n)
	for i := range rows {
		rows[i] = row
	}
	return rows
}

// The built-in tiles are the room as it was drawn before tiles: the wall a
// panel every 12 pixels under its top row and over the baseboard, the floor
// planks 4 pixels high whose seams shift half a plank from one to the next.
var (
	builtinWall  = append(append(piece{"TTTTTTTTTTTT"}, tile(9, "PFFFFFFFFFFF")...), "BBBBBBBBBBBB", "BBBBBBBBBBBB")
	builtinFloor = append(tile(4, "SAAAAAAAAAAAAAAA"), tile(4, "BBBBBBBBSBBBBBBB")...)
)

func TestLoadTheme_TakesBothAbsentTilesFromTheBuiltinTheme(t *testing.T) {
	dir := writeFiles(t, map[string]string{"theme.toml": "name = \"mine\"\n"})
	got, err := LoadTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.pieces[pieceWall], builtinWall) {
		t.Errorf("wall = %q, want %q", got.pieces[pieceWall], builtinWall)
	}
	if !reflect.DeepEqual(got.pieces[pieceFloor], builtinFloor) {
		t.Errorf("floor = %q, want %q", got.pieces[pieceFloor], builtinFloor)
	}
}

// A room file is what the office draws: the wall's tile repeats left to
// right from x=0, the floor's both ways from the top-left of the floor.
func TestLoadTheme_RepeatsTheRoomTilesAcrossTheRoom(t *testing.T) {
	wall := tile(wallH, "TP")
	floor := piece{"ABS", "SBA"}
	dir := writeFiles(t, map[string]string{
		"theme.toml": "[room]\nwall_top = \"#fe0001\"\nwall_panel = \"#fe0002\"\n" +
			"floor_a = \"#fe0003\"\nfloor_b = \"#fe0004\"\nfloor_seam = \"#fe0005\"\n",
		"room/wall.txt":  pieceText(wall),
		"room/floor.txt": pieceText(floor),
	})
	th, err := LoadTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	useTheme(t, th)
	color := map[byte]screen.RGB{'T': rgb(0xfe0001), 'P': rgb(0xfe0002), 'A': rgb(0xfe0003), 'B': rgb(0xfe0004), 'S': rgb(0xfe0005)}
	l := Layout{W: 31, H: wallH + 9} // no windows or paintings over the wall
	buf := screen.New(l.W, (l.H+1)/2)
	drawRoom(buf, l)
	for y := 0; y < l.H; y++ {
		for x := 0; x < l.W; x++ {
			ch := wall[y%len(wall)][x%2]
			if y >= wallH {
				row := floor[(y-wallH)%len(floor)]
				ch = row[x%len(row)]
			}
			if got, want := buf.PixelAt(x, y), color[ch]; got != want {
				t.Fatalf("pixel (%d, %d) = %s, want %s (%c)", x, y, formatColor(got), formatColor(want), ch)
			}
		}
	}
}

func TestLoadTheme_NamesTheFileLineAndWhatATileMustBe(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		body  string
		line  int // 0 when there is no line to name
		parts []string
	}{
		{"wall too tall", "wall.txt", pieceText(tile(13, "TF")),
			13, []string{"tile wall", "13 rows", "want 12"}},
		{"wall too short", "wall.txt", pieceText(tile(11, "TF")),
			11, []string{"tile wall", "11 rows", "want 12"}},
		{"wall 0 wide", "wall.txt", "\n" + pieceText(tile(11, "TF")),
			1, []string{"tile wall", "0 wide", "want 1-64"}},
		{"wall too wide", "wall.txt", pieceText(tile(12, strings.Repeat("T", 65))),
			1, []string{"tile wall", "65 wide", "want 1-64"}},
		{"floor too tall", "floor.txt", pieceText(tile(33, "AB")),
			33, []string{"tile floor", "33 rows", "want 1-32"}},
		{"floor too wide", "floor.txt", pieceText(tile(2, strings.Repeat("A", 65))),
			1, []string{"tile floor", "65 wide", "want 1-64"}},
		{"transparent cell", "floor.txt", "AB\nA.\n",
			2, []string{"tile floor", "'.'", "column 2", "use A B S"}},
		{"unknown letter lists the tile's letters", "wall.txt", pieceText(append(tile(11, "TF"), "TX")),
			12, []string{"tile wall", "'X'", "column 2", "use T F P B"}},
		{"rows of different widths", "floor.txt", "AB\nABA\n",
			2, []string{"tile floor", "3 wide", "want 2 like row 1"}},
		{"unknown room file", "ceiling.txt", "AB\n",
			0, []string{"unknown tile", "room files are wall, floor"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"theme.toml": "name = \"mine\"\n", "room/" + tt.file: tt.body})
			_, err := LoadTheme(dir)
			if err == nil {
				t.Fatal("LoadTheme accepted the theme")
			}
			msg := err.Error()
			path := filepath.Join(dir, "room", tt.file)
			want := append([]string{path}, tt.parts...)
			if tt.line > 0 {
				want = append(want, fmt.Sprintf("%s:%d:", path, tt.line))
			}
			for _, w := range want {
				if !strings.Contains(msg, w) {
					t.Errorf("error %q does not mention %q", msg, w)
				}
			}
			if strings.Contains(msg, "or .") {
				t.Errorf("error %q offers '.', which a tile cannot hold", msg)
			}
		})
	}
}

func TestWriteTheme_WritesBothTilesLoadThemeReadsBack(t *testing.T) {
	files := map[string]string{
		"room/wall.txt":  pieceText(tile(wallH, "TPB")),
		"room/floor.txt": pieceText(piece{"ABS", "SBA", "BBB"}),
	}
	src := maps.Clone(files)
	src["theme.toml"] = "name = \"mine\"\n"
	want, err := LoadTheme(writeFiles(t, src))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "mine")
	if err := WriteTheme(dir, want); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != body {
			t.Errorf("%s = %q, want %q", name, got, body)
		}
	}
	got, err := LoadTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !sameRoundTrip(t, got, want) {
		t.Errorf("round trip differs:\n got %q\nwant %q", got.pieces, want.pieces)
	}
}
