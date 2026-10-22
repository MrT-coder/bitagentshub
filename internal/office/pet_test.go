package office

import (
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
		if got := buf.PixelAt(bx, by) == petFood; got != c.full {
			t.Errorf("%v: food in the bowl = %v, want %v", c.mood, got, c.full)
		}
	}
}
