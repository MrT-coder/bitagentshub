package office

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// solidPiece is piece p filled with letter ch, keeping the desk's screen.
func solidPiece(p pieceID, ch byte) piece {
	spec := pieceSpecs[p]
	rows := make(piece, spec.h)
	for y := range rows {
		row := []byte(strings.Repeat(string(ch), spec.w))
		if p == pieceDesk && y >= screenY && y < screenY+screenH {
			copy(row[screenX:], "######")
		}
		rows[y] = string(row)
	}
	return rows
}

func pieceText(rows piece) string { return strings.Join(rows, "\n") + "\n" }

func TestLoadTheme_TakesEveryAbsentPieceFromTheBuiltinTheme(t *testing.T) {
	for p := pieceID(0); p < pieceCount; p++ {
		spec := pieceSpecs[p]
		t.Run(spec.name, func(t *testing.T) {
			rows := solidPiece(p, spec.letters[0])
			dir := writeFiles(t, map[string]string{
				"theme.toml":                        "name = \"mine\"\n",
				spec.dir + "/" + spec.name + ".txt": pieceText(rows),
			})
			got, err := LoadTheme(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := BuiltinTheme()
			want.Name = "mine"
			want.pieces[p] = rows
			want.set = map[string]bool{}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("LoadTheme with only %s.txt differs from the built-in theme elsewhere", spec.name)
			}
		})
	}
}

// A furniture file is what the office draws: each piece, filled with one
// letter whose color shows nowhere else, puts that color on a pixel the
// built-in piece draws differently.
func TestLoadTheme_DrawsTheFurnitureFiles(t *testing.T) {
	l, _, _ := posedOffice()
	st := l.Stations["typer"]
	item := func(kind string) Item {
		for _, it := range l.Items {
			if it.Kind == kind {
				return it
			}
		}
		t.Fatalf("the posed office has no %s", kind)
		return Item{}
	}
	tests := []struct {
		piece  pieceID
		letter byte
		role   string // the color setting the letter names
		x, y   int    // a pixel of the piece the built-in theme draws otherwise
	}{
		{pieceDesk, 'T', "furniture.desk_top", st.X, st.Y + monitorY},
		{pieceChair, 'B', "furniture.chair_back", st.X + 16, st.Y + 3},
		{pieceWindow, 'G', "furniture.glass", l.Windows[0].x, l.Windows[0].y},
		{piecePainting, 'H', "furniture.painting_hill", l.Paintings[0].x, l.Paintings[0].y},
		{pieceClock, 'C', "room.baseboard", l.Clock.x, l.Clock.y},
		{pieceBookshelf, 's', "furniture.shelf_back", item("bookshelf").X, item("bookshelf").Y},
		{pieceCoffee, 'T', "furniture.machine_tray", item("coffee").X, item("coffee").Y},
		{pieceCooler, 'C', "furniture.cooler_body", item("cooler").X, item("cooler").Y},
		{pieceSofa, 'L', "furniture.sofa_leg", item("sofa").X, item("sofa").Y},
		{piecePlant, 'P', "furniture.pot", item("plant").X, item("plant").Y},
	}
	for _, tt := range tests {
		name := pieceSpecs[tt.piece].name
		t.Run(name, func(t *testing.T) {
			section, key, _ := strings.Cut(tt.role, ".")
			dir := writeFiles(t, map[string]string{
				"theme.toml":                 fmt.Sprintf("[%s]\n%s = \"#fe0102\"\n", section, key),
				"furniture/" + name + ".txt": pieceText(solidPiece(tt.piece, tt.letter)),
			})
			th, err := LoadTheme(dir)
			if err != nil {
				t.Fatal(err)
			}
			useTheme(t, th)
			l, c, agents := posedOffice()
			buf := screen.New(l.W, l.H/2)
			Draw(buf, l, c, agents, 0, "", pet.Sleeping)
			if got, want := buf.PixelAt(tt.x, tt.y), rgb(0xfe0102); got != want {
				t.Errorf("pixel (%d, %d) = %s, want %s from %s.txt", tt.x, tt.y, formatColor(got), formatColor(want), name)
			}
		})
	}
}

func TestLoadTheme_NamesTheFileLineAndWhatAPieceMustBe(t *testing.T) {
	builtin := BuiltinTheme().pieces
	edit := func(p pieceID, line int, row string) string { // line is 1-based
		rows := append(piece(nil), builtin[p]...)
		rows[line-1] = row
		return pieceText(rows)
	}
	manifest := "name = \"mine\"\n"
	desk := builtin[pieceDesk]
	tests := []struct {
		name  string
		file  string
		body  string
		line  int // 0 when there is no line to name
		parts []string
	}{
		{"row too wide", "chair.txt", edit(pieceChair, 2, "BBBBBBBBBBB"),
			2, []string{"piece chair", "11 wide", "want 10"}},
		{"too many rows", "chair.txt", pieceText(append(append(piece(nil), builtin[pieceChair]...), "BBBBBBBBBB")),
			7, []string{"piece chair", "7 rows", "want 6"}},
		{"too few rows", "chair.txt", pieceText(builtin[pieceChair][:5]),
			5, []string{"piece chair", "5 rows", "want 6"}},
		{"unknown letter", "chair.txt", edit(pieceChair, 3, "BBBXBBBBBB"),
			3, []string{"piece chair", "'X'", "column 4", "use B E or ."}},
		{"unknown letter lists the piece's letters", "bookshelf.txt", edit(pieceBookshelf, 1, "SSSSSSSSSSSSSX"),
			1, []string{"piece bookshelf", "'X'", "column 14", "use S s 1 2 3 4 5 6 7 8 9 or ."}},
		{"screen letter outside the desk", "chair.txt", edit(pieceChair, 1, "#EEEEEEEEE"),
			1, []string{"piece chair", "'#'", "column 1"}},
		{"blank line inside", "chair.txt", strings.Replace(pieceText(builtin[pieceChair]), "\n", "\n\n", 1),
			2, []string{"piece chair", "blank line"}},
		{"unknown furniture file", "table.txt", pieceText(builtin[pieceChair]),
			0, []string{"unknown piece", "desk", "chair", "plant"}},
		{"screen cell off the screen", "desk.txt", edit(pieceDesk, 1, "#"+desk[0][1:]),
			1, []string{"piece desk", "'#'", "column 1", "columns 3-8 of rows 2-6"}},
		{"screen cell missing", "desk.txt", edit(pieceDesk, 4, desk[3][:4]+"M"+desk[3][5:]),
			4, []string{"piece desk", "column 5", "must be '#'", "columns 3-8 of rows 2-6"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, map[string]string{"theme.toml": manifest, "furniture/" + tt.file: tt.body})
			_, err := LoadTheme(dir)
			if err == nil {
				t.Fatal("LoadTheme accepted the theme")
			}
			msg := err.Error()
			path := filepath.Join(dir, "furniture", tt.file)
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

func TestWriteTheme_WritesEveryPieceLoadThemeReadsBack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mine")
	want := BuiltinTheme()
	want.Name = "mine"
	want.pieces[pieceSofa] = solidPiece(pieceSofa, 'a')
	if err := WriteTheme(dir, want); err != nil {
		t.Fatal(err)
	}
	for _, spec := range pieceSpecs {
		if _, err := os.Stat(filepath.Join(dir, spec.dir, spec.name+".txt")); err != nil {
			t.Errorf("%s file: %v", spec.dir, err)
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
