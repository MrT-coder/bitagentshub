package office

import (
	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// The pet keeps the bottom-left corner of the floor: a bed with the pet on
// it and its bowl on the right. It never walks; its mood picks the frames.
const (
	petW   = 18
	petH   = 12
	bowlDX = 14 // the bowl's offset in the corner
	bowlDY = 9
	catW   = 12
	catH   = 8
)

// catFrame is a catW×catH grid: K fur, k shade or a closed eye, E eye,
// N nose, W muzzle, '.' transparent.
type catFrame [catH]string

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

var (
	catSit = catFrame{
		"..K....K....",
		"..KK..KK....",
		".KKKKKKKK...",
		".KEKKKKEK...",
		".KKKNNKKK...",
		"..KWWWWK....",
		"..KKKKKKK.K.",
		"..KkKKKkKKK.",
	}
	catCurl = catFrame{
		"............",
		"............",
		"............",
		"............",
		"...K..K.....",
		"..KKKKKKKK..",
		".KkKKkKKKKKK",
		".KKKKKKKKKKK",
	}
	catFlat = catFrame{
		"............",
		"............",
		"............",
		"............",
		"............",
		".K..K.......",
		"KKKKKKKKKKKK",
		"KEKKEKKKKKKK",
	}
)

// with returns f with row y replaced.
func (f catFrame) with(y int, row string) catFrame {
	f[y] = row
	return f
}

// catFrames gives the frame for a mood at tick and the bubble over it.
func catFrames(mood pet.Mood, tick int) (catFrame, string, screen.Style) {
	odd := func(every int) bool { return (tick/every)%2 == 1 }
	wag := func(f catFrame, every int) catFrame {
		if odd(every) {
			return f.with(6, "..KKKKKKK..K").with(7, "..KkKKKkKK.K")
		}
		return f
	}
	closed := ".KkKKKKkK..."
	bubble := screen.Style{Fg: rgb(0x2e3440), HasFg: true, Bg: rgb(0xebcb8b), HasBg: true, Bold: true}
	switch mood {
	case pet.Sleeping:
		f := catCurl
		if odd(8) {
			f = f.with(5, "..KKKKKKKKK.") // breathing
		}
		return f, []string{"z", "Z"}[(tick/12)%2], screen.Fg(rgb(0xd8dee9))
	case pet.Tired:
		return wag(catSit.with(3, closed), 16), "~", screen.Fg(rgb(0xd8dee9))
	case pet.Exhausted:
		f := catFlat
		if odd(12) {
			f = f.with(6, "KKKKKKKKKKK.")
		}
		return f, "x", screen.Fg(rgb(0xd8dee9))
	case pet.Hungry:
		f := catSit
		if odd(6) {
			f = f.with(5, "..KWNNWK....") // meowing at the empty bowl
		}
		return f, "", screen.Style{}
	case pet.Alert:
		f := catSit.with(0, ".K......K...")
		if odd(4) {
			return f, "", screen.Style{}
		}
		return f, "!", screen.Style{Fg: rgb(0xffffff), HasFg: true, Bg: rgb(0xbf3b3b), HasBg: true, Bold: true}
	case pet.Eating:
		f := catSit.with(3, closed)
		if odd(3) {
			f = f.with(5, "..KWkkWK....") // chewing
		}
		return f, "🐟", bubble
	}
	f := wag(catSit, 4) // busy
	if tick%40 < 2 {
		f = f.with(3, closed) // a blink now and then
	}
	return f, "", screen.Style{}
}

// DrawPet paints the pet's corner: the bed, the bowl, full unless the pet
// is hungry, and the pet in its mood's frame with its bubble.
func DrawPet(buf *screen.Buffer, l Layout, mood pet.Mood, tick int) {
	p, colors := l.Pet, &cur().Pet
	fillRect(buf, p.x+1, p.y+8, 12, 1, colors.BedRim)
	fillRect(buf, p.x, p.y+9, 14, 2, colors.BedPad)
	fillRect(buf, p.x+1, p.y+11, 12, 1, colors.BedRim)

	bx, by := p.x+bowlDX, p.y+bowlDY
	if mood != pet.Hungry {
		fillRect(buf, bx+1, by, 2, 1, colors.Food)
	}
	fillRect(buf, bx, by+1, 4, 1, colors.Bowl)
	fillRect(buf, bx+1, by+2, 2, 1, colors.Bowl)

	f, text, st := catFrames(mood, tick)
	cx, cy := p.x+1, p.y+2
	for dy, row := range f {
		for dx := 0; dx < len(row); dx++ {
			if c, ok := colors.color(row[dx]); ok {
				buf.Pixel(cx+dx, cy+dy, c)
			}
		}
	}
	if text != "" {
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
