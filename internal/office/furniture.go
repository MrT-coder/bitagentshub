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
	pieceCount
)

// pieceSpecs names each piece's folder and file, its size and its letters.
// The floor items are named like their Item.Kind.
var pieceSpecs = [pieceCount]struct {
	dir, name string
	w, h      int
	letters   string
}{
	pieceDesk:      {"furniture", "desk", stationW, 13, "TFLMUC#"},
	pieceChair:     {"furniture", "chair", 10, 6, "BE"},
	pieceWindow:    {"furniture", "window", 16, 8, "FGS"},
	piecePainting:  {"furniture", "painting", 8, 6, "FCH"},
	pieceClock:     {"furniture", "clock", 5, 5, "FC"},
	pieceBookshelf: {"furniture", "bookshelf", 14, 18, "Ss123456789"},
	pieceCoffee:    {"furniture", "coffee", 8, 14, "MPTBUL"},
	pieceCooler:    {"furniture", "cooler", 6, 12, "BCT"},
	pieceSofa:      {"furniture", "sofa", 20, 10, "SsaL"},
	piecePlant:     {"furniture", "plant", 6, 10, "LPB"},
	pieceBed:       {"pet", "bed", 14, 4, "RP"},
	pieceBowl:      {"pet", "bowl", 4, 3, "BF"},
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

// ink is the color letter ch stands for in piece p. '.' and the desk's
// screen draw nothing.
func (t *Theme) ink(p pieceID, ch byte, n int) (screen.RGB, bool) {
	f := &t.Furniture
	var c screen.RGB
	switch p {
	case pieceDesk:
		c = pick(ch, "TFLMUC", f.DeskTop, f.DeskFront, f.DeskLeg, f.Bezel, f.Mug, f.Coffee)
	case pieceChair:
		c = pick(ch, "BE", f.ChairBack, f.ChairEdge)
	case pieceWindow:
		c = pick(ch, "FGS", f.WindowFrame, f.Glass, f.GlassShine)
	case piecePainting:
		c = pick(ch, "FCH", f.PaintingFrame, f.Paintings[n%len(f.Paintings)], f.PaintingHill)
	case pieceClock:
		c = pick(ch, "FC", f.ClockFace, t.Baseboard)
	case pieceBookshelf:
		if ch >= '1' && ch <= '9' {
			return f.Books[int(ch-'1')%len(f.Books)], true
		}
		c = pick(ch, "Ss", f.Shelf, f.ShelfBack)
	case pieceCoffee:
		light := f.MachineLight
		if (n/8)%2 == 1 {
			light = f.MachineLightOff
		}
		c = pick(ch, "MPTBUL", f.Machine, f.MachinePanel, f.MachineTray, f.MachineBase, f.Mug, light)
	case pieceCooler:
		c = pick(ch, "BCT", f.CoolerBottle, f.CoolerBody, f.CoolerTap)
	case pieceSofa:
		c = pick(ch, "SsaL", f.Sofa, scale(f.Sofa, 0.85), scale(f.Sofa, 0.7), f.SofaLeg)
	case piecePlant:
		c = pick(ch, "LPB", f.Leaf, f.Pot, f.PotBase)
	case pieceBed:
		c = pick(ch, "RP", t.Pet.BedRim, t.Pet.BedPad)
	case pieceBowl:
		if ch == 'F' && n == 0 {
			return c, false
		}
		c = pick(ch, "BF", t.Pet.Bowl, t.Pet.Food)
	}
	if ch == '.' || ch == '#' {
		return c, false
	}
	return c, true
}

// pick is the color in colors at the place of ch in letters.
func pick(ch byte, letters string, colors ...screen.RGB) screen.RGB {
	if i := strings.IndexByte(letters, ch); i >= 0 {
		return colors[i]
	}
	return screen.RGB{}
}

func (t *Theme) readPieces(fsys fs.FS, dir string) error {
	var ids []pieceID
	var names []string
	for p, spec := range pieceSpecs {
		if spec.dir == "furniture" {
			ids, names = append(ids, pieceID(p)), append(names, spec.name)
		}
	}
	return readFiles(fsys, "furniture", dir, "piece", "furniture", names, func(i int, path string, src []byte) error {
		rows, err := parsePiece(path, src, ids[i])
		if err != nil {
			return err
		}
		t.pieces[ids[i]] = rows
		return nil
	})
}

// parsePiece reads the file of piece p at path: one grid of the piece's
// size in its letters.
func parsePiece(path string, src []byte, p pieceID) (piece, error) {
	spec := pieceSpecs[p]
	fail := func(line int, format string, args ...any) error {
		at := path
		if line > 0 {
			at += ":" + strconv.Itoa(line)
		}
		return fmt.Errorf("%s: piece %s: %s", at, spec.name, fmt.Sprintf(format, args...))
	}
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	var rows piece
	for i, raw := range lines {
		n, row := i+1, strings.TrimRight(raw, " \t\r")
		if row == "" {
			return nil, fail(n, "a piece is one grid with no blank lines")
		}
		if len(rows) == spec.h {
			return nil, fail(n, "has %d rows, want %d", len(lines), spec.h)
		}
		if len(row) != spec.w {
			return nil, fail(n, "row is %d wide, want %d", len(row), spec.w)
		}
		for col := 0; col < len(row); col++ {
			ch := row[col]
			if ch != '.' && !strings.ContainsRune(spec.letters, rune(ch)) {
				return nil, fail(n, "unknown letter %q in column %d; use %s or .", ch, col+1, spaced(spec.letters))
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
	if len(rows) != spec.h {
		return nil, fail(len(rows), "has %d rows, want %d", len(rows), spec.h)
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
