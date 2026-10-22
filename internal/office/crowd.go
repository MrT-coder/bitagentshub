package office

import (
	"math/rand"

	"github.com/MrT-coder/bitagentshub/internal/claude"
)

type facing int

const (
	faceDown facing = iota
	faceUp
	faceSide
)

// Actor is an agent's body on the floor. X/Y is the sprite's top-left pixel.
type Actor struct {
	X, Y   int
	Seated bool   // at its workstation
	At     string // POI kind while idling there
	Pause  int    // ticks left before leaving the POI
	Moving bool
	Left   bool
	Face   facing
	Step   int // walk-cycle counter

	path        []Point
	slideX      int // when sliding, the point being slid to
	slideY      int
	slideActive bool
	afterSlide  func(*Actor)
	poi         int // index of the POI being visited, -1 for none
}

func (a *Actor) sliding() bool { return a.slideActive }

// Crowd holds actor positions between frames. Randomness is injected so
// movement is reproducible in tests.
type Crowd struct {
	actors    map[string]*Actor
	rng       *rand.Rand
	layoutKey string
}

func NewCrowd(rng *rand.Rand) *Crowd {
	return &Crowd{actors: map[string]*Actor{}, rng: rng}
}

func (c *Crowd) Actor(id string) *Actor { return c.actors[id] }

// Sync adds newcomers at the door on the left wall, drops agents that left
// and re-plans everyone after a layout change. Snapshots arrive every
// second, so an unchanged layout must leave walks in progress alone.
func (c *Crowd) Sync(l Layout, agents []claude.Agent) {
	changed := l.fingerprint() != c.layoutKey
	c.layoutKey = l.fingerprint()
	present := map[string]bool{}
	for _, a := range agents {
		present[a.ID] = true
		act, ok := c.actors[a.ID]
		if !ok {
			act = &Actor{X: 0, Y: c.door(l), poi: -1}
			c.actors[a.ID] = act
		}
		if !changed {
			continue
		}
		act.path, act.poi, act.At = nil, -1, ""
		if act.Seated {
			// Stations may have moved; sit straight at the new one.
			st := l.Stations[a.ID]
			act.X, act.Y = st.SeatX(), st.SeatY()
		} else if !act.slideActive && !l.Walkable(act.X, act.Y) {
			act.X, act.Y = 0, c.door(l)
		}
	}
	for id := range c.actors {
		if !present[id] {
			delete(c.actors, id)
		}
	}
}

// door is the first walkable spot on the left wall.
func (c *Crowd) door(l Layout) int {
	for y := l.H - spriteH; y >= wallH-12; y -= navStep {
		if l.Walkable(0, y) {
			return y
		}
	}
	return l.H - spriteH
}

// Step advances every actor by one tick: at most one pixel on each axis.
func (c *Crowd) Step(l Layout, agents []claude.Agent) {
	taken := map[int]bool{}
	for _, act := range c.actors {
		if act.poi >= 0 {
			taken[act.poi] = true
		}
	}
	for _, a := range agents {
		act := c.actors[a.ID]
		if act == nil {
			continue
		}
		if act.slideActive {
			c.slide(act)
			continue
		}
		if a.State == claude.StateWaiting {
			c.idle(l, act, taken)
			c.walk(act)
			c.arrive(l, act)
		} else {
			c.work(l, act, l.Stations[a.ID])
			c.walk(act)
		}
	}
}

// work sends the agent to its chair: up from the sofa or out of a POI,
// along a path to the stand point, then a slide down into the seat.
func (c *Crowd) work(l Layout, act *Actor, st Workstation) {
	if act.Seated {
		act.Face, act.Moving = faceDown, false
		return
	}
	if act.leavePOI(l) {
		return
	}
	if act.X == st.StandX() && act.Y == st.StandY() {
		act.startSlide(st.SeatX(), st.SeatY(), func(a *Actor) { a.Seated, a.Face = true, faceDown })
		return
	}
	if len(act.path) == 0 {
		act.path = l.Path(act.X, act.Y, st.StandX(), st.StandY())
	}
}

// idle gets the agent up from its desk and wanders between POIs, pausing
// at each one.
func (c *Crowd) idle(l Layout, act *Actor, taken map[int]bool) {
	if act.Seated {
		act.Seated = false
		act.startSlide(act.X, act.Y-6, func(*Actor) {})
		return
	}
	if act.At != "" {
		if act.Pause > 0 {
			act.Pause--
			return
		}
		if act.leavePOI(l) {
			return
		}
	}
	if len(act.path) > 0 || act.poi >= 0 {
		return
	}
	choices := make([]int, 0, len(l.POIs))
	for i := range l.POIs {
		if !taken[i] {
			choices = append(choices, i)
		}
	}
	if len(choices) == 0 {
		return
	}
	i := choices[c.rng.Intn(len(choices))]
	p := l.POIs[i]
	if path := l.Path(act.X, act.Y, p.X, p.Y); path != nil {
		act.path, act.poi = path, i
		taken[i] = true
		act.Pause = 40 + c.rng.Intn(80)
	}
}

// leavePOI slides an agent off a seat at its POI, or simply lets go of it.
// It reports whether the agent is now busy sliding.
func (a *Actor) leavePOI(l Layout) bool {
	if a.poi < 0 {
		return false
	}
	p := l.POIs[a.poi]
	a.poi, a.At = -1, ""
	if p.Sit && a.X == p.SitX && a.Y == p.SitY {
		a.startSlide(p.X, p.Y, func(*Actor) {})
		return true
	}
	return false
}

// walk follows the current path one pixel per axis per tick and settles
// at the POI it leads to.
func (c *Crowd) walk(act *Actor) {
	act.Moving = false
	if len(act.path) == 0 {
		return
	}
	next := act.path[0]
	dx, dy := sign(next.X-act.X), sign(next.Y-act.Y)
	act.X += dx
	act.Y += dy
	act.Moving, act.Step = true, act.Step+1
	switch {
	case dx != 0:
		act.Face, act.Left = faceSide, dx < 0
	case dy < 0:
		act.Face = faceUp
	case dy > 0:
		act.Face = faceDown
	}
	if act.X == next.X && act.Y == next.Y {
		act.path = act.path[1:]
	}
}

// arrive settles an idle agent at the POI its path led to; a path that
// ended anywhere else releases the POI so a new one is picked.
func (c *Crowd) arrive(l Layout, act *Actor) {
	if act.poi < 0 || len(act.path) > 0 || act.At != "" || act.slideActive {
		return
	}
	p := l.POIs[act.poi]
	if act.X != p.X || act.Y != p.Y {
		act.poi = -1
		return
	}
	act.At = p.Kind
	if p.FaceUp {
		act.Face = faceUp
	} else {
		act.Face = faceDown
	}
	if p.Sit {
		act.startSlide(p.SitX, p.SitY, func(a *Actor) { a.Face = faceDown })
	}
}

func (a *Actor) startSlide(x, y int, then func(*Actor)) {
	a.path, a.slideX, a.slideY, a.slideActive, a.afterSlide = nil, x, y, true, then
}

func (c *Crowd) slide(act *Actor) {
	act.X += sign(act.slideX - act.X)
	act.Y += sign(act.slideY - act.Y)
	act.Moving = false
	if act.X == act.slideX && act.Y == act.slideY {
		act.slideActive = false
		if act.afterSlide != nil {
			act.afterSlide(act)
		}
	}
}

// HitTest maps a terminal cell of the floor to the agent drawn there,
// preferring the sprite in front, then the station the agent owns.
func HitTest(l Layout, c *Crowd, x, y int) (string, bool) {
	hit, hitBase := "", -1
	for id, act := range c.actors {
		if x >= act.X && x < act.X+spriteW && y >= act.Y/2 && y <= (act.Y+spriteH-1)/2 && act.Y > hitBase {
			hit, hitBase = id, act.Y
		}
	}
	if hit != "" {
		return hit, true
	}
	for id, st := range l.Stations {
		if x >= st.X && x < st.X+stationW && y >= st.Y/2 && y < (st.Y+stationH)/2 {
			return id, true
		}
	}
	return "", false
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
