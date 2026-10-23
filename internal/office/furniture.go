package office

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// A piece is the shape of one kind of furniture: a grid of letters, one
// pixel each, where each letter names one of the piece's colors and '.' is
// transparent. Every piece keeps its size, so a theme changes its shape
// and never where it stands or what it blocks.
type piece []string

type pieceID int

const (
	pieceDesk pieceID = iota
	pieceChair
	pieceWindow
	piecePainting
	pieceClock
	pieceBookshelf
	pieceCoffee
	pieceCooler
	pieceSofa
	piecePlant
	pieceBed // the pet's corner
	pieceBowl
	pieceWall // the room's tiles
	pieceFloor
	pieceCount
)

// pieceSpec names a piece's folder and file, its size and its letters.
type pieceSpec struct {
	dir, name string
	w, h      int
	// A tile repeats across the room, so it may be any size up to
	// maxW×maxH (a side whose max is 0 stays w or h); w×h is the built-in
	// tile's. A tile has no '.': the room has no holes.
	maxW, maxH int
	letters    string
}

func (s pieceSpec) tile() bool { return s.maxW > 0 }

// pieceSpecs is every piece's spec. The floor items are named like their
// Item.Kind.
var pieceSpecs = [pieceCount]pieceSpec{
	pieceDesk:      {"furniture", "desk", stationW, 13, 0, 0, "TFLMUC#"},
	pieceChair:     {"furniture", "chair", 10, 6, 0, 0, "BE"},
	pieceWindow:    {"furniture", "window", 16, 8, 0, 0, "FGS"},
	piecePainting:  {"furniture", "painting", 8, 6, 0, 0, "FCH"},
	pieceClock:     {"furniture", "clock", 5, 5, 0, 0, "FC"},
	pieceBookshelf: {"furniture", "bookshelf", 14, 18, 0, 0, "Ss123456789"},
	pieceCoffee:    {"furniture", "coffee", 8, 14, 0, 0, "MPTBUL"},
	pieceCooler:    {"furniture", "cooler", 6, 12, 0, 0, "BCT"},
	pieceSofa:      {"furniture", "sofa", 20, 10, 0, 0, "SsaL"},
	piecePlant:     {"furniture", "plant", 6, 10, 0, 0, "LPB"},
	pieceBed:       {"pet", "bed", 14, 4, 0, 0, "RP"},
	pieceBowl:      {"pet", "bowl", 4, 3, 0, 0, "BF"},
	pieceWall:      {"room", "wall", 12, wallH, 64, 0, "TFPB"},
	pieceFloor:     {"room", "floor", 16, 8, 64, 32, "ABS"},
}

// The desk starts at the monitor's row of its station. Its '#' cells are
// the monitor's screen, which the office fills with what the agent does.
const (
	screenX, screenY = monitorX + 1, 1
	screenW, screenH = 6, 5
)

// builtinPieces is the built-in furniture, indexed by piece.
func builtinPieces() [pieceCount]piece {
	return [pieceCount]piece{
		pieceDesk: {
			".MMMMMMMM...............",
			".M######M...............",
			".M######M...............",
			".M######M...............",
			".M######M...............",
			".M######M..........CU...",
			".MMMMMMMM..........UU...",
			".TTTTTTTTTTTTTTTTTTTTTT.",
			".TTTTTTTTTTTTTTTTTTTTTT.",
			".FFFFFFFFFFFFFFFFFFFFFF.",
			".FFFFFFFFFFFFFFFFFFFFFF.",
			".FFFFFFFFFFFFFFFFFFFFFF.",
			"..L..................L..",
		},
		pieceChair: {
			"EEEEEEEEEE",
			"BBBBBBBBBB",
			"BBBBBBBBBB",
			"BBBBBBBBBB",
			"BBBBBBBBBB",
			"BBBBBBBBBB",
		},
		pieceWindow: {
			"FFFFFFFFFFFFFFFF",
			"FGGSGGGGFGGGGGGF",
			"FGGGSGGGFGGGGGGF",
			"FGGGGSGGFGGGGGGF",
			"FFFFFFFFFFFFFFFF",
			"FGGGGGGGFGGGGGGF",
			"FGGGGGGGFGGGGGGF",
			"FFFFFFFFFFFFFFFF",
		},
		piecePainting: {
			"FFFFFFFF",
			"FCCCCCCF",
			"FCCCCCCF",
			"FHHHHHHF",
			"FHHHHHHF",
			"FFFFFFFF",
		},
		pieceClock: {
			"FFFFF",
			"FFFFF",
			"FFCFF",
			"FFFFF",
			"FFFFF",
		},
		pieceBookshelf: {
			"SSSSSSSSSSSSSS",
			"Ssssss123ss1sS",
			"Sss3451234s12S",
			"S123451234512S",
			"S123451234512S",
			"S123451234512S",
			"SSSSSSSSSSSSSS",
			"Sssss12ssssssS",
			"S23s512s4ss2sS",
			"S234512345123S",
			"S234512345123S",
			"S234512345123S",
			"SSSSSSSSSSSSSS",
			"Ss451ss4s1ss4S",
			"S3451234s1234S",
			"S345123451234S",
			"S345123451234S",
			"S345123451234S",
		},
		pieceCoffee: {
			"MMMMMMMM",
			"MPPPPPPM",
			"MPPPPPLM",
			"MPPPPPPM",
			"MMMMMMMM",
			"MMMMMMMM",
			"MMTTTTMM",
			"MMTTTTMM",
			"MMTUUTMM",
			"MMTUUTMM",
			"MMMMMMMM",
			"MMMMMMMM",
			"BBBBBBBB",
			"BBBBBBBB",
		},
		pieceCooler: {
			".BBBB.",
			".BBBB.",
			".BBBB.",
			".BBBB.",
			".BBBB.",
			"CCCCCC",
			"CCCCCC",
			"CCTCCC",
			"CCCCCC",
			"CCCCCC",
			"CCCCCC",
			"CCCCCC",
		},
		pieceSofa: {
			"ssssssssssssssssssss",
			"ssssssssssssssssssss",
			"aassssssssssssssssaa",
			"aassssssssssssssssaa",
			"aassssssssssssssssaa",
			"aaSSSSSSSSSSSSSSSSaa",
			"aaSSSSSSSSSSSSSSSSaa",
			"aaSSSSSSSSSSSSSSSSaa",
			"aL................La",
			".L................L.",
		},
		piecePlant: {
			"...L..",
			"..L.L.",
			".L.L.L",
			"..LLL.",
			".L.L.L",
			".PPPP.",
			".PPPP.",
			".PPPP.",
			".PPPP.",
			"..BB..",
		},
		pieceBed: {
			".RRRRRRRRRRRR.",
			"PPPPPPPPPPPPPP",
			"PPPPPPPPPPPPPP",
			".RRRRRRRRRRRR.",
		},
		pieceBowl: {
			".FF.",
			"BBBB",
			".BB.",
		},
		// A panel every 12 pixels between the wall's top and its baseboard.
		pieceWall: {
			"TTTTTTTTTTTT",
			"PFFFFFFFFFFF",
			"PFFFFFFFFFFF",
			"PFFFFFFFFFFF",
			"PFFFFFFFFFFF",
			"PFFFFFFFFFFF",
			"PFFFFFFFFFFF",
			"PFFFFFFFFFFF",
			"PFFFFFFFFFFF",
			"PFFFFFFFFFFF",
			"BBBBBBBBBBBB",
			"BBBBBBBBBBBB",
		},
		// Planks 4 pixels high whose seams shift half a plank row by row.
		pieceFloor: {
			"SAAAAAAAAAAAAAAA",
			"SAAAAAAAAAAAAAAA",
			"SAAAAAAAAAAAAAAA",
			"SAAAAAAAAAAAAAAA",
			"BBBBBBBBSBBBBBBB",
			"BBBBBBBBSBBBBBBB",
			"BBBBBBBBSBBBBBBB",
			"BBBBBBBBSBBBBBBB",
		},
	}
}

// pieceNamed is the piece whose file is name.txt, which is also the Kind
// of the floor items drawn with it.
func pieceNamed(name string) (pieceID, bool) {
	for p, spec := range pieceSpecs {
		if spec.name == name {
			return pieceID(p), true
		}
	}
	return 0, false
}

// drawPiece draws t's piece p with its top-left pixel at (x, y). n is what
// changes from one copy or frame to the next: which painting it is, or the
// tick the coffee machine's light blinks by, or 1 when the pet's bowl is
// full.
func (t *Theme) drawPiece(buf *screen.Buffer, p pieceID, x, y, n int) {
	for dy, row := range t.pieces[p] {
		for dx := 0; dx < len(row); dx++ {
			if c, ok := t.ink(p, row[dx], n); ok {
				buf.Pixel(x+dx, y+dy, c)
			}
		}
	}
}

// ink is the color letter ch stands for in piece p, or one of t's own
// letters. '.' and the desk's screen draw nothing.
func (t *Theme) ink(p pieceID, ch byte, n int) (screen.RGB, bool) {
	f := &t.Furniture
	var c screen.RGB
	var ok bool
	switch p {
	case pieceDesk:
		c, ok = pick(ch, "TFLMUC", f.DeskTop, f.DeskFront, f.DeskLeg, f.Bezel, f.Mug, f.Coffee)
	case pieceChair:
		c, ok = pick(ch, "BE", f.ChairBack, f.ChairEdge)
	case pieceWindow:
		c, ok = pick(ch, "FGS", f.WindowFrame, f.Glass, f.GlassShine)
	case piecePainting:
		c, ok = pick(ch, "FCH", f.PaintingFrame, f.Paintings[n%len(f.Paintings)], f.PaintingHill)
	case pieceClock:
		c, ok = pick(ch, "FC", f.ClockFace, t.Baseboard)
	case pieceBookshelf:
		if ch >= '1' && ch <= '9' {
			return f.Books[int(ch-'1')%len(f.Books)], true
		}
		c, ok = pick(ch, "Ss", f.Shelf, f.ShelfBack)
	case pieceCoffee:
		light := f.MachineLight
		if (n/8)%2 == 1 {
			light = f.MachineLightOff
		}
		c, ok = pick(ch, "MPTBUL", f.Machine, f.MachinePanel, f.MachineTray, f.MachineBase, f.Mug, light)
	case pieceCooler:
		c, ok = pick(ch, "BCT", f.CoolerBottle, f.CoolerBody, f.CoolerTap)
	case pieceSofa:
		c, ok = pick(ch, "SsaL", f.Sofa, scale(f.Sofa, 0.85), scale(f.Sofa, 0.7), f.SofaLeg)
	case piecePlant:
		c, ok = pick(ch, "LPB", f.Leaf, f.Pot, f.PotBase)
	case pieceBed:
		c, ok = pick(ch, "RP", t.Pet.BedRim, t.Pet.BedPad)
	case pieceBowl:
		if ch == 'F' && n == 0 {
			return c, false
		}
		c, ok = pick(ch, "BF", t.Pet.Bowl, t.Pet.Food)
	case pieceWall:
		c, ok = pick(ch, "TFPB", t.WallTop, t.WallFill, t.WallPanel, t.Baseboard)
	case pieceFloor:
		c, ok = pick(ch, "ABS", t.FloorA, t.FloorB, t.FloorSeam)
	}
	if ok {
		return c, true
	}
	return t.ownInk(ch)
}

// pick is the color in colors at the place of ch in letters, if ch is one.
func pick(ch byte, letters string, colors ...screen.RGB) (screen.RGB, bool) {
	if i := strings.IndexByte(letters, ch); i >= 0 {
		return colors[i], true
	}
	return screen.RGB{}, false
}

// readPieces reads the pieces of folder (furniture or room) of fsys; dir is
// its path in errors and kind what a file there is.
func (t *Theme) readPieces(fsys fs.FS, folder, dir, kind string) error {
	var ids []pieceID
	var names []string
	for p, spec := range pieceSpecs {
		if spec.dir == folder {
			ids, names = append(ids, pieceID(p)), append(names, spec.name)
		}
	}
	return readFiles(fsys, folder, dir, kind, folder, names, func(i int, path string, src []byte) error {
		rows, err := parsePiece(path, src, ids[i], t.ownLetters())
		if err != nil {
			return err
		}
		t.pieces[ids[i]] = rows
		return nil
	})
}

// parsePiece reads the file of piece p at path: one grid of the piece's
// size in its letters or own, the theme's own letters.
func parsePiece(path string, src []byte, p pieceID, own string) (piece, error) {
	spec := pieceSpecs[p]
	what, maxH, rowsWant, colsWant := "piece", spec.h, strconv.Itoa(spec.h), strconv.Itoa(spec.w)
	if spec.tile() {
		what, colsWant = "tile", fmt.Sprintf("1-%d", spec.maxW)
		if spec.maxH > 0 {
			maxH, rowsWant = spec.maxH, fmt.Sprintf("1-%d", spec.maxH)
		}
	}
	fail := func(line int, format string, args ...any) error {
		at := path
		if line > 0 {
			at += ":" + strconv.Itoa(line)
		}
		return fmt.Errorf("%s: %s %s: %s", at, what, spec.name, fmt.Sprintf(format, args...))
	}
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	var rows piece
	for i, raw := range lines {
		n, row := i+1, strings.TrimRight(raw, " \t\r")
		switch {
		case row == "" && spec.tile():
			return nil, fail(n, "row is 0 wide, want %s", colsWant)
		case row == "":
			return nil, fail(n, "a piece is one grid with no blank lines")
		case len(rows) == maxH:
			return nil, fail(n, "has %d rows, want %s", len(lines), rowsWant)
		case spec.tile() && len(rows) > 0 && len(row) != len(rows[0]):
			return nil, fail(n, "row is %d wide, want %d like row 1", len(row), len(rows[0]))
		case spec.tile() && len(row) > spec.maxW:
			return nil, fail(n, "row is %d wide, want %s", len(row), colsWant)
		case !spec.tile() && len(row) != spec.w:
			return nil, fail(n, "row is %d wide, want %d", len(row), spec.w)
		}
		for col := 0; col < len(row); col++ {
			ch := row[col]
			known := strings.IndexByte(spec.letters, ch) >= 0 || strings.IndexByte(own, ch) >= 0
			if !known && (ch != '.' || spec.tile()) {
				return nil, fail(n, "unknown letter %q in column %d; %s", ch, col+1, useLetters(spec.letters, own, !spec.tile()))
			}
			if p != pieceDesk {
				continue
			}
			onScreen := col >= screenX && col < screenX+screenW && i >= screenY && i < screenY+screenH
			where := fmt.Sprintf("the screen is columns %d-%d of rows %d-%d", screenX+1, screenX+screenW, screenY+1, screenY+screenH)
			if ch == '#' && !onScreen {
				return nil, fail(n, "'#' in column %d is off the screen; %s", col+1, where)
			}
			if ch != '#' && onScreen {
				return nil, fail(n, "column %d must be '#': %s", col+1, where)
			}
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 || len(rows) != spec.h && maxH == spec.h {
		return nil, fail(len(rows), "has %d rows, want %s", len(rows), rowsWant)
	}
	return rows, nil
}

// writePieces writes one file per piece of t into its folder of dir.
func writePieces(dir string, t Theme) error {
	for p, spec := range pieceSpecs {
		path := filepath.Join(dir, spec.dir, spec.name+".txt")
		if err := os.WriteFile(path, []byte(strings.Join(t.pieces[p], "\n")+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}
