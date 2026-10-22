// Package office is a pixel-art office in the terminal, in the spirit of
// pixel-agents: a wall with windows, a wooden floor, a carpeted area of
// workstations per workspace folder, and furniture agents walk around.
//
// Coordinates are pixels. One pixel is one terminal column wide and half a
// line tall, so a floor of w columns by h lines is w×2h pixels.
package office

import (
	"fmt"
	"sort"
	"strings"

	"github.com/MrT-coder/bitagentshub/internal/claude"
)

const (
	wallH = 12 // the back wall, in pixels

	// A workstation is chair, seated agent, desk and monitor, plus two label
	// lines under the desk.
	stationW = 24
	stationH = 20
	slotW    = 26           // station plus aisle; even so every coordinate stays even
	rowPitch = stationH + 2 // the aisle behind a chair overlaps the labels above it

	areaPad   = 2
	areaLabel = 4 // label line plus a margin above the first row
	areaGapX  = 4
	areaGapY  = 2

	// Offsets inside a station.
	seatDX   = 8 // sprite left edge while seated
	monitorX = 1
	monitorY = 2
	deskY    = 9
)

// Workstation is one agent's desk. X and Y are its top-left pixel.
type Workstation struct {
	AgentID string
	Area    int
	X, Y    int
}

// SeatX/SeatY place the seated sprite; StandX/StandY is where the agent
// stands behind the chair before sitting down.
func (w Workstation) SeatX() int  { return w.X + seatDX }
func (w Workstation) SeatY() int  { return w.Y }
func (w Workstation) StandX() int { return w.X + seatDX }
func (w Workstation) StandY() int { return w.Y - 6 }

// LabelCell is the terminal cell where the station's name label starts.
func (w Workstation) LabelCell() (x, y int) { return w.X, (w.Y + 16) / 2 }

// Area is the carpeted cluster of workstations for one workspace folder.
type Area struct {
	Name       string
	X, Y, W, H int
	Agents     []claude.Agent
	Today      claude.Usage
}

// POI is a place idle agents walk to. X/Y is where the sprite stands; when
// Sit is set the agent then slides to SitX/SitY and sits there.
type POI struct {
	Kind       string
	X, Y       int
	Sit        bool
	SitX, SitY int
	FaceUp     bool // stands with its back to the viewer, e.g. at a window
}

type rect struct{ x, y, w, h int }

func (r rect) overlaps(o rect) bool {
	return r.x < o.x+o.w && o.x < r.x+r.w && r.y < o.y+o.h && o.y < r.y+r.h
}

// Item is a piece of furniture. Base is its bottom pixel row, used to sort
// it against agents so whoever is lower on screen is drawn in front.
type Item struct {
	Kind       string
	X, Y, W, H int
	Base       int
}

// Layout is the office floor of W×H pixels. Needed is the height the areas
// require; it can exceed H on a small terminal.
type Layout struct {
	W, H      int
	Needed    int
	Areas     []Area
	Stations  map[string]Workstation
	Items     []Item
	Windows   []rect
	Paintings []rect
	Clock     rect
	Pet       rect // the pet's corner: its bed and bowl
	POIs      []POI
	blocked   []rect // pixel rects agents' feet may not enter
}

// Build lays out the office for a floor of w columns by h lines. Areas and
// stations are ordered by name so nobody changes seat when someone else
// comes or goes.
func Build(agents []claude.Agent, w, h int) Layout {
	l := Layout{W: max(w, slotW+2*areaPad), H: max(h*2, wallH+rowPitch), Stations: map[string]Workstation{}}

	byKey := map[string]*Area{}
	var keys []string
	for _, a := range agents {
		k := strings.ToLower(a.Group)
		ar, ok := byKey[k]
		if !ok {
			ar = &Area{Name: a.Group}
			byKey[k] = ar
			keys = append(keys, k)
		}
		ar.Agents = append(ar.Agents, a)
		ar.Today.Input += a.Today.Input
		ar.Today.Output += a.Today.Output
		ar.Today.CacheCreate += a.Today.CacheCreate
		ar.Today.CacheRead += a.Today.CacheRead
	}
	sort.Strings(keys)

	// Keep a lounge strip on the right of wide floors for the furniture.
	usable := l.W - areaPad
	if l.W >= 100 {
		usable = l.W - 34
	}
	perRow := max(1, (usable-2*areaPad)/slotW)
	x, y, shelf := areaPad, wallH, 0
	for i, k := range keys {
		ar := byKey[k]
		sort.Slice(ar.Agents, func(a, b int) bool {
			if ar.Agents[a].Project != ar.Agents[b].Project {
				return ar.Agents[a].Project < ar.Agents[b].Project
			}
			return ar.Agents[a].ID < ar.Agents[b].ID
		})
		cols := min(len(ar.Agents), perRow)
		rows := (len(ar.Agents) + cols - 1) / cols
		ar.W = cols*slotW + 2*areaPad - (slotW - stationW)
		ar.H = areaLabel + rows*rowPitch - (rowPitch - stationH) + areaPad
		if x > areaPad && x+ar.W > usable {
			x, y, shelf = areaPad, y+shelf+areaGapY, 0
		}
		ar.X, ar.Y = x, y
		for j, a := range ar.Agents {
			st := Workstation{
				AgentID: a.ID,
				Area:    i,
				X:       ar.X + areaPad + (j%cols)*slotW,
				Y:       ar.Y + areaLabel + (j/cols)*rowPitch,
			}
			l.Stations[a.ID] = st
			l.blocked = append(l.blocked, rect{st.X + 1, st.Y + deskY, stationW - 2, 6})
		}
		x += ar.W + areaGapX
		shelf = max(shelf, ar.H)
		l.Areas = append(l.Areas, *ar)
	}
	l.Needed = y + shelf
	l.furnish()
	return l
}

// furnish places wall decorations and furniture wherever the areas left
// room, and the places idle agents can visit.
func (l *Layout) furnish() {
	// The pet's corner comes first: furniture and walkers keep off it.
	l.Pet = rect{2, l.H - petH, petW, petH}
	l.blocked = append(l.blocked, l.Pet)
	areaRects := make([]rect, 0, len(l.Areas))
	for _, a := range l.Areas {
		areaRects = append(areaRects, rect{a.X - 2, a.Y - 2, a.W + 4, a.H + 4})
	}
	free := func(r rect) bool {
		if r.x < 0 || r.y < 0 || r.x+r.w > l.W || r.y+r.h > l.H {
			return false
		}
		for _, a := range areaRects {
			if a.overlaps(r) {
				return false
			}
		}
		for _, b := range l.blocked {
			if b.overlaps(r) {
				return false
			}
		}
		return true
	}
	// place adds a floor item whose footprint (the part on the floor) must
	// be free, and returns whether it fit.
	place := func(kind string, x, top, w, h, footH int) bool {
		foot := rect{x, top + h - footH, w, footH}
		// Keep room in front of it to stand, except against the bottom edge.
		front := min(foot.h+10, l.H-foot.y)
		if !free(rect{foot.x - 2, foot.y, foot.w + 4, front}) {
			return false
		}
		l.Items = append(l.Items, Item{Kind: kind, X: x, Y: top, W: w, H: h, Base: top + h - 1})
		l.blocked = append(l.blocked, foot)
		return true
	}

	// Wall furniture stands on the floor against the wall, from the right.
	right := l.W - 4
	if place("bookshelf", right-14, wallH-10, 14, 18, 8) {
		l.addPOI(POI{Kind: "bookshelf", X: right - 11, Y: wallH - 2, FaceUp: true})
		right -= 18
	}
	if place("coffee", right-8, wallH-6, 8, 14, 8) {
		l.addPOI(POI{Kind: "coffee", X: right - 8, Y: wallH - 2, FaceUp: true})
		right -= 12
	}
	if place("cooler", right-6, wallH-4, 6, 12, 6) {
		l.addPOI(POI{Kind: "cooler", X: right - 7, Y: wallH - 4, FaceUp: true})
	}

	// Sofa and plants on the open floor near the bottom right.
	sx, sy := l.W-26, l.H-14
	if place("sofa", sx, sy, 20, 10, 6) {
		for _, dx := range []int{2, 10} {
			l.addPOI(POI{Kind: "sofa", X: sx + dx, Y: sy + 10, Sit: true, SitX: sx + dx, SitY: sy - 4})
		}
	}
	place("plant", petW+4, l.H-10, 6, 10, 4)
	place("plant", l.W-8, l.H-10, 6, 10, 4)

	// Windows, paintings and a clock on the wall; standing at a window is a
	// place to visit when the floor below it is free.
	for _, wx := range []int{6, l.W/2 - 8} {
		win := rect{wx, 1, 16, 8}
		l.Windows = append(l.Windows, win)
		l.addPOI(POI{Kind: "window", X: wx + 4, Y: wallH - 10, FaceUp: true})
	}
	l.Paintings = append(l.Paintings, rect{l.W/4 + 2, 2, 8, 6}, rect{l.W*3/4 - 10, 2, 8, 6})
	l.Clock = rect{l.W/2 + 12, 2, 5, 5}
}

func (l *Layout) addPOI(p POI) {
	if l.Walkable(p.X, p.Y) {
		l.POIs = append(l.POIs, p)
	}
}

// Order lists agent ids in the order their stations appear.
func Order(l Layout) []string {
	var ids []string
	for _, a := range l.Areas {
		for _, ag := range a.Agents {
			ids = append(ids, ag.ID)
		}
	}
	return ids
}

// Walkable reports whether a sprite whose top-left pixel is (x, y) can
// stand there: inside the floor, below the wall and with its feet clear of
// furniture.
func (l Layout) Walkable(x, y int) bool {
	if x < 0 || y < 0 || x+spriteW > l.W || y+spriteH > max(l.H, l.Needed) || y+12 < wallH {
		return false
	}
	feet := rect{x + 1, y + 12, spriteW - 2, 2}
	for _, b := range l.blocked {
		if b.overlaps(feet) {
			return false
		}
	}
	return true
}

// fingerprint identifies the floor plan: size and where every station is.
// Spend and state changes do not alter it.
func (l Layout) fingerprint() string {
	ids := make([]string, 0, len(l.Stations))
	for id := range l.Stations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	fmt.Fprintf(&b, "%dx%d", l.W, l.H)
	for _, id := range ids {
		st := l.Stations[id]
		fmt.Fprintf(&b, "|%s@%d,%d", id, st.X, st.Y)
	}
	return b.String()
}
