package office

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// The pet keeps the bottom-left corner of the floor: a bed with the pet on
// it and its bowl on the right. It never walks; its mood picks the frames.
const (
	petW   = 18
	petH   = 12
	bedDY  = 8  // the bed's offset in the corner
	bowlDX = 14 // the bowl's offset in the corner
	bowlDY = 9
	catW   = 12
	catH   = 8
)

// catFrame is a catW×catH grid: K fur, k shade or a closed eye, E eye,
// N nose, W muzzle, '.' transparent.
type catFrame [catH]string

// catLetters are the letters a cat frame draws with besides '.'.
const catLetters = "KkENW"

// moodCount is the number of moods, each with its own pet file.
const moodCount = int(pet.Eating) + 1

// petSpecs gives, by mood, the frames its pet file holds and every how many
// ticks they alternate; a still mood alternates nothing. The file is named
// like the mood.
var petSpecs = [moodCount]struct {
	frames, every int
}{
	pet.Sleeping:  {2, 8},  // breathing
	pet.Busy:      {3, 4},  // a tail wag; the third frame is a blink
	pet.Tired:     {2, 16}, // a slow wag
	pet.Exhausted: {2, 12},
	pet.Hungry:    {2, 6}, // meowing at the empty bowl
	pet.Alert:     {1, 0},
	pet.Eating:    {2, 3}, // chewing
}

// color is the color of letter c of a cat frame in the pet's colors.
func (p *PetColors) color(c byte) (screen.RGB, bool) {
	switch c {
	case 'K':
		return p.Fur, true
	case 'k':
		return p.Shade, true
	case 'E':
		return p.Eye, true
	case 'N':
		return p.Nose, true
	case 'W':
		return p.Muzzle, true
	}
	return screen.RGB{}, false
}

// builtinPet is the built-in cat's frames, indexed by mood.
func builtinPet() [moodCount][]catFrame {
	sit := catFrame{
		"..K....K....",
		"..KK..KK....",
		".KKKKKKKK...",
		".KEKKKKEK...",
		".KKKNNKKK...",
		"..KWWWWK....",
		"..KKKKKKK.K.",
		"..KkKKKkKKK.",
	}
	curl := catFrame{
		"............",
		"............",
		"............",
		"............",
		"...K..K.....",
		"..KKKKKKKK..",
		".KkKKkKKKKKK",
		".KKKKKKKKKKK",
	}
	flat := catFrame{
		"............",
		"............",
		"............",
		"............",
		"............",
		".K..K.......",
		"KKKKKKKKKKKK",
		"KEKKEKKKKKKK",
	}
	closed := sit.with(3, ".KkKKKKkK...")
	wag := func(f catFrame) catFrame {
		return f.with(6, "..KKKKKKK..K").with(7, "..KkKKKkKK.K")
	}
	return [moodCount][]catFrame{
		pet.Sleeping:  {curl, curl.with(5, "..KKKKKKKKK.")},
		pet.Busy:      {sit, wag(sit), closed},
		pet.Tired:     {closed, wag(closed)},
		pet.Exhausted: {flat, flat.with(6, "KKKKKKKKKKK.")},
		pet.Hungry:    {sit, sit.with(5, "..KWNNWK....")},
		pet.Alert:     {sit.with(0, ".K......K...")},
		pet.Eating:    {closed, closed.with(5, "..KWkkWK....")},
	}
}

// with returns f with row y replaced.
func (f catFrame) with(y int, row string) catFrame {
	f[y] = row
	return f
}

// catFrame is the frame t's pet shows in a mood at tick.
func (t *Theme) catFrame(mood pet.Mood, tick int) catFrame {
	spec, i := petSpecs[mood], 0
	if spec.every > 0 {
		i = (tick / spec.every) % 2
	}
	if mood == pet.Busy && i == 0 && tick%40 < 2 {
		i = 2 // a blink now and then
	}
	return t.moods[mood][i]
}

// petBubble is what the pet says over its head in a mood at tick.
func petBubble(mood pet.Mood, tick int) (string, screen.Style) {
	odd := func(every int) bool { return (tick/every)%2 == 1 }
	quiet := screen.Fg(rgb(0xd8dee9))
	switch mood {
	case pet.Sleeping:
		return []string{"z", "Z"}[(tick/12)%2], quiet
	case pet.Tired:
		return "~", quiet
	case pet.Exhausted:
		return "x", quiet
	case pet.Alert:
		if odd(4) {
			return "", screen.Style{}
		}
		return "!", screen.Style{Fg: rgb(0xffffff), HasFg: true, Bg: rgb(0xbf3b3b), HasBg: true, Bold: true}
	case pet.Eating:
		return "🐟", screen.Style{Fg: rgb(0x2e3440), HasFg: true, Bg: rgb(0xebcb8b), HasBg: true, Bold: true}
	}
	return "", screen.Style{}
}

// DrawPet paints the pet's corner: the bed, the bowl, full unless the pet
// is hungry, and the pet in its mood's frame with its bubble.
func DrawPet(buf *screen.Buffer, l Layout, mood pet.Mood, tick int) {
	if mood < 0 || int(mood) >= moodCount {
		mood = pet.Busy
	}
	p, t := l.Pet, cur()
	t.drawPiece(buf, pieceBed, p.x, p.y+bedDY, 0)
	full := 0
	if mood != pet.Hungry {
		full = 1
	}
	t.drawPiece(buf, pieceBowl, p.x+bowlDX, p.y+bowlDY, full)

	cx, cy := p.x+1, p.y+2
	for dy, row := range t.catFrame(mood, tick) {
		for dx := 0; dx < len(row); dx++ {
			if c, ok := t.Pet.color(row[dx]); ok {
				buf.Pixel(cx+dx, cy+dy, c)
			}
		}
	}
	if text, st := petBubble(mood, tick); text != "" {
		buf.Text(cx+catW-2, cy/2-1, text, st)
	}
}

// PetCells is the pet's corner in terminal cells of the floor.
func (l Layout) PetCells() (x, y, w, h int) {
	p := l.Pet
	return p.x, p.y / 2, p.w, p.h / 2
}

// PetHit reports whether terminal cell (x, y) of the floor falls on the
// pet's corner.
func PetHit(l Layout, x, y int) bool {
	p := l.Pet
	return x >= p.x && x < p.x+p.w && y >= p.y/2 && y < (p.y+p.h)/2
}

// readPet reads the theme's pet/ folder: a file of frames per mood, named
// like the mood, and the bed's and the bowl's pieces.
func (t *Theme) readPet(fsys fs.FS, dir string) error {
	names := make([]string, 0, moodCount+2)
	for m := range moodCount {
		names = append(names, pet.Mood(m).String())
	}
	names = append(names, pieceSpecs[pieceBed].name, pieceSpecs[pieceBowl].name)
	return readFiles(fsys, "pet", dir, "pet file", "pet", names, func(i int, path string, src []byte) error {
		if i >= moodCount {
			p := []pieceID{pieceBed, pieceBowl}[i-moodCount]
			rows, err := parsePiece(path, src, p)
			if err != nil {
				return err
			}
			t.pieces[p] = rows
			return nil
		}
		g := grids{what: "pet " + names[i], w: catW, h: catH, frames: petSpecs[i].frames, letters: catLetters}
		rows, err := g.parse(path, src)
		if err != nil {
			return err
		}
		frames := make([]catFrame, len(rows))
		for j, f := range rows {
			copy(frames[j][:], f)
		}
		t.moods[i] = frames
		return nil
	})
}

// writePet writes one file of frames per mood of t's pet into dir.
func writePet(dir string, t Theme) error {
	for m, frames := range t.moods {
		parts := make([]string, len(frames))
		for i, f := range frames {
			parts[i] = strings.Join(f[:], "\n")
		}
		path := filepath.Join(dir, pet.Mood(m).String()+".txt")
		if err := os.WriteFile(path, []byte(strings.Join(parts, "\n\n")+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}
