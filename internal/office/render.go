package office

import (
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

func rgb(hex uint32) screen.RGB {
	return screen.RGB{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex)}
}

var (
	Dim       = screen.Fg(rgb(0x9aa0aa))
	areaStyle = screen.Style{Fg: rgb(0xeceff4), HasFg: true, Bold: true}

	// The wall, floor and carpet colors come from the active theme.

	deskTop   = rgb(0xc8a272)
	deskFront = rgb(0x8b6a45)
	deskLeg   = rgb(0x5c4630)
	chairBack = rgb(0x434c5e)
	chairEdge = rgb(0x2e3440)
	bezel     = rgb(0x2b303b)
	screenOff = rgb(0x15181e)
	mugWhite  = rgb(0xe5e9f0)
	coffeeCol = rgb(0x6b4226)

	windowFrame = rgb(0xd8dee9)
	glass       = rgb(0x88c0d0)
	glassShine  = rgb(0xb5dbe6)
	frameWood   = rgb(0xa3815b)
)

// Draw paints the office into buf (one cell per column, two pixels per
// line), the pet in its corner in mood.
func Draw(buf *screen.Buffer, l Layout, c *Crowd, agents []claude.Agent, tick int, selected string, mood pet.Mood) {
	drawBackground(buf, l)
	clock(buf, l.Clock, tick)
	for _, a := range l.Areas {
		end := buf.Text(a.X+1, a.Y/2, truncate(a.Name, a.W-2), areaStyle)
		buf.Text(end, a.Y/2, truncate(" · "+FmtTokens(a.Today.Counted()), max(0, a.X+a.W-1-end)), Dim)
	}

	byID := make(map[string]claude.Agent, len(agents))
	for _, a := range agents {
		byID[a.ID] = a
	}

	// Depth sort: whatever stands lower on screen is drawn in front.
	type drawable struct {
		base int
		draw func()
	}
	var ds []drawable
	for _, it := range l.Items {
		it := it
		ds = append(ds, drawable{it.Base, func() { drawItem(buf, it, tick) }})
	}
	for id, st := range l.Stations {
		st, ag := st, byID[id]
		ds = append(ds, drawable{st.Y + 8, func() { chair(buf, st) }})
		ds = append(ds, drawable{st.Y + deskY + 5, func() { desk(buf, st, ag, tick) }})
	}
	for _, ag := range agents {
		act, ag := c.Actor(ag.ID), ag
		if act == nil {
			continue
		}
		ds = append(ds, drawable{act.Y + spriteH - 1, func() {
			f, mirror := pose(act, ag, tick)
			sprite(buf, act.X, act.Y, f, mirror, LookOf(ag.ID))
		}})
	}
	ds = append(ds, drawable{l.Pet.y + l.Pet.h - 1, func() { DrawPet(buf, l, mood, tick) }})
	sort.SliceStable(ds, func(i, j int) bool { return ds[i].base < ds[j].base })
	for _, d := range ds {
		d.draw()
	}

	// Labels go over the sprites so names stay readable when someone walks
	// past, like the floating names in pixel-agents.
	number := map[string]int{}
	for i, id := range Order(l) {
		number[id] = i + 1
	}
	for _, a := range agents {
		labels(buf, l.Stations[a.ID], a, number[a.ID], a.ID == selected)
	}
	for _, ag := range agents {
		if act := c.Actor(ag.ID); act != nil {
			bubble(buf, act, ag, tick)
		}
	}
}

// background is the room and its carpets, which never move: drawn once for
// a floor plan, theme and buffer size, then copied into every frame. A new
// theme or layout draws it again, so a theme still applies at once.
var background struct {
	mu    sync.Mutex
	theme *Theme
	key   string
	buf   *screen.Buffer
}

func drawBackground(buf *screen.Buffer, l Layout) {
	key := fmt.Sprintf("%dx%d|%d|%s", buf.W, buf.H, l.Needed, l.fingerprint())
	t := cur()
	background.mu.Lock()
	defer background.mu.Unlock()
	if background.buf == nil || background.theme != t || background.key != key {
		bg := screen.New(buf.W, buf.H)
		drawRoom(bg, l)
		for i, a := range l.Areas {
			carpet(bg, a, t.Carpets[i%len(t.Carpets)])
		}
		background.buf, background.theme, background.key = bg, t, key
	}
	buf.CopyFrom(background.buf)
}

func drawRoom(buf *screen.Buffer, l Layout) {
	h := max(l.H, l.Needed)
	for y := 0; y < h; y++ {
		for x := 0; x < l.W; x++ {
			buf.Pixel(x, y, roomPixel(x, y))
		}
	}
	for _, w := range l.Windows {
		fillRect(buf, w.x, w.y, w.w, w.h, windowFrame)
		fillRect(buf, w.x+1, w.y+1, w.w-2, w.h-2, glass)
		for i := 0; i < 3; i++ {
			buf.Pixel(w.x+3+i, w.y+1+i, glassShine)
		}
		fillRect(buf, w.x+w.w/2, w.y+1, 1, w.h-2, windowFrame)
		fillRect(buf, w.x+1, w.y+w.h/2, w.w-2, 1, windowFrame)
	}
	for i, p := range l.Paintings {
		fillRect(buf, p.x, p.y, p.w, p.h, frameWood)
		canvas := []screen.RGB{rgb(0x81a1c1), rgb(0xbf616a)}[i%2]
		fillRect(buf, p.x+1, p.y+1, p.w-2, p.h-2, canvas)
		fillRect(buf, p.x+1, p.y+p.h-3, p.w-2, 2, rgb(0xa3be8c))
	}
}

// roomPixel is the wall above wallH and wooden planks below it.
func roomPixel(x, y int) screen.RGB {
	t := cur()
	switch {
	case y == 0:
		return t.WallTop
	case y >= wallH-2 && y < wallH:
		return t.Baseboard
	case y < wallH:
		if x%12 == 0 {
			return t.WallPanel
		}
		return t.WallFill
	}
	plank := (y - wallH) / 4
	if (x+plank*8)%16 == 0 {
		return t.FloorSeam
	}
	if plank%2 == 0 {
		return t.FloorA
	}
	return t.FloorB
}

func carpet(buf *screen.Buffer, a Area, c screen.RGB) {
	fillRect(buf, a.X, a.Y, a.W, a.H, c)
	edge := scale(c, 1.25)
	fillRect(buf, a.X, a.Y, a.W, 1, edge)
	fillRect(buf, a.X, a.Y+a.H-1, a.W, 1, edge)
	fillRect(buf, a.X, a.Y, 1, a.H, edge)
	fillRect(buf, a.X+a.W-1, a.Y, 1, a.H, edge)
}

func clock(buf *screen.Buffer, r rect, tick int) {
	fillRect(buf, r.x, r.y, r.w, r.h, mugWhite)
	buf.Pixel(r.x+2, r.y+2, cur().Baseboard)
	// The hand turns once a minute at the default frame rate.
	hand := [][2]int{{2, 1}, {3, 1}, {3, 2}, {3, 3}, {2, 3}, {1, 3}, {1, 2}, {1, 1}}[(tick/60)%8]
	buf.Pixel(r.x+hand[0], r.y+hand[1], rgb(0xbf616a))
}

func chair(buf *screen.Buffer, st Workstation) {
	fillRect(buf, st.X+7, st.Y+3, 10, 6, chairBack)
	fillRect(buf, st.X+7, st.Y+3, 10, 1, chairEdge)
}

// desk draws the desk with its monitor and a mug. The monitor shows what
// the agent is doing and is off while it is not working.
func desk(buf *screen.Buffer, st Workstation, ag claude.Agent, tick int) {
	x, y := st.X, st.Y
	fillRect(buf, x+monitorX, y+monitorY, 8, 7, bezel)
	monitorScreen(buf, x+monitorX+1, y+monitorY+1, ag, tick)
	fillRect(buf, x+monitorX+3, y+monitorY+7, 2, 1, bezel)
	fillRect(buf, x+1, y+deskY, stationW-2, 2, deskTop)
	fillRect(buf, x+1, y+deskY+2, stationW-2, 3, deskFront)
	fillRect(buf, x+2, y+deskY+5, 1, 1, deskLeg)
	fillRect(buf, x+stationW-3, y+deskY+5, 1, 1, deskLeg)
	fillRect(buf, x+19, y+deskY-2, 2, 2, mugWhite)
	buf.Pixel(x+19, y+deskY-2, coffeeCol)
}

// monitorScreen fills the 6×5 screen at (x, y).
func monitorScreen(buf *screen.Buffer, x, y int, ag claude.Agent, tick int) {
	if !ag.State.Working() && !ag.Blocked {
		fillRect(buf, x, y, 6, 5, screenOff)
		return
	}
	bg, ink := rgb(0x1d2b45), []screen.RGB{rgb(0x61afef), rgb(0xc678dd), rgb(0x98c379), rgb(0xe5c07b)}
	switch {
	case ag.Blocked:
		bg, ink = rgb(0x4a1d1d), []screen.RGB{rgb(0xff6b6b)}
	case ag.State == claude.StateThinking:
		fillRect(buf, x, y, 6, 5, rgb(0x262a36))
		buf.Pixel(x+1+(tick/3)%4, y+2, rgb(0xe5c07b))
		return
	case ag.Activity == claude.ActivityReading:
		bg, ink = rgb(0xe8e8e0), []screen.RGB{rgb(0x8a8a90)}
	case ag.Activity == claude.ActivityRunning:
		bg, ink = rgb(0x0d120d), []screen.RGB{rgb(0x7fd962)}
	}
	fillRect(buf, x, y, 6, 5, bg)
	scroll := tick / 4
	for row := 0; row < 5; row++ {
		n := int(hash(strconv.Itoa(row+scroll)+ag.ID) % 5)
		fillRect(buf, x, y+row, 1+n, 1, ink[(row+scroll)%len(ink)])
	}
}

func drawItem(buf *screen.Buffer, it Item, tick int) {
	x, y := it.X, it.Y
	switch it.Kind {
	case "bookshelf":
		fillRect(buf, x, y, it.W, it.H, rgb(0x6b4a2f))
		books := []screen.RGB{rgb(0xbf616a), rgb(0x5e81ac), rgb(0xa3be8c), rgb(0xebcb8b), rgb(0xb48ead)}
		for shelf := 0; shelf < 3; shelf++ {
			sy := y + 1 + shelf*6
			fillRect(buf, x+1, sy, it.W-2, 5, rgb(0x3e2b1c))
			for b := 0; b < it.W-2; b++ {
				height := 3 + int(hash(strconv.Itoa(b*7+shelf))%3)
				fillRect(buf, x+1+b, sy+5-height, 1, height, books[(b+shelf)%len(books)])
			}
		}
	case "coffee":
		fillRect(buf, x, y, it.W, it.H, rgb(0x5a5f6b))
		fillRect(buf, x+1, y+1, it.W-2, 3, rgb(0x2e3440))
		buf.Pixel(x+it.W-2, y+2, []screen.RGB{rgb(0xbf616a), rgb(0x7f2f35)}[(tick/8)%2])
		fillRect(buf, x+2, y+6, it.W-4, 4, rgb(0x1c1f26))
		fillRect(buf, x+3, y+8, 2, 2, mugWhite)
		fillRect(buf, x, y+it.H-2, it.W, 2, rgb(0x434c5e))
	case "cooler":
		fillRect(buf, x+1, y, it.W-2, 5, rgb(0x88c0d0))
		fillRect(buf, x, y+5, it.W, it.H-5, rgb(0xd8dee9))
		buf.Pixel(x+2, y+7, rgb(0x5e81ac))
	case "sofa":
		c := rgb(0x7a4b5e)
		fillRect(buf, x, y, it.W, 5, scale(c, 0.85))
		fillRect(buf, x, y+5, it.W, 3, c)
		fillRect(buf, x, y+2, 2, 7, scale(c, 0.7))
		fillRect(buf, x+it.W-2, y+2, 2, 7, scale(c, 0.7))
		fillRect(buf, x+1, y+it.H-2, 1, 2, deskLeg)
		fillRect(buf, x+it.W-2, y+it.H-2, 1, 2, deskLeg)
	case "plant":
		leaf := rgb(0x4caf50)
		for _, p := range [][2]int{{2, 0}, {1, 1}, {3, 1}, {0, 2}, {2, 2}, {4, 2}, {1, 3}, {2, 3}, {3, 3}, {2, 4}, {0, 4}, {4, 4}} {
			buf.Pixel(x+p[0]+1, y+p[1], leaf)
		}
		fillRect(buf, x+1, y+5, 4, 4, rgb(0xa0522d))
		fillRect(buf, x+2, y+9, 2, 1, rgb(0x7a3e22))
	}
}

// bubble shows what the agent needs: a red ! when a permission prompt
// blocks it, … while thinking, ? while it waits for you, z while asleep.
func bubble(buf *screen.Buffer, act *Actor, ag claude.Agent, tick int) {
	text, st := "", screen.Style{}
	switch {
	case ag.Blocked:
		text, st = "!", screen.Style{Fg: rgb(0xffffff), HasFg: true, Bg: rgb(0xbf3b3b), HasBg: true, Bold: true}
	case ag.Cheer:
		text, st = "★", screen.Style{Fg: rgb(0x2e3440), HasFg: true, Bg: rgb(0xebcb8b), HasBg: true, Bold: true}
	case ag.State == claude.StateThinking:
		text, st = "…", screen.Fg(rgb(0x2e3440)).WithBg(rgb(0xe5e9f0))
	case ag.State == claude.StateWaiting && act.At == "":
		text, st = "?", screen.Style{Fg: rgb(0x2e3440), HasFg: true, Bg: rgb(0xebcb8b), HasBg: true, Bold: true}
	case ag.State == claude.StateSleeping && act.Seated:
		text, st = []string{"z", "Z"}[(tick/12)%2], screen.Fg(rgb(0xd8dee9))
	default:
		return
	}
	line := act.Y/2 - 1
	if act.Seated && ag.State == claude.StateSleeping {
		line = act.Y/2 + 1 // the head is down on the desk
	}
	buf.Text(act.X+spriteW-2, line, text, st)
}

// labels prints two lines under a desk: the station number (1-9, the key
// that jumps there) and the project, then today's spend and the state.
func labels(buf *screen.Buffer, st Workstation, a claude.Agent, number int, selected bool) {
	x, line := st.LabelCell()
	w := stationW - 1
	head := ""
	if number >= 1 && number <= 9 {
		head = fmt.Sprintf("%d ", number)
	}
	name := head + truncateMiddle(a.Project, w-len(head))
	style := screen.Style{Fg: AgentColor(a.ID), HasFg: true, Bold: true}
	if selected {
		// Reverse video plus an ASCII marker so selection survives
		// terminals without color.
		name, style.Reverse = ">"+head+truncateMiddle(a.Project, w-1-len(head)), true
	}
	buf.Text(x, line, name, style)

	spend := FmtTokens(a.Today.Counted())
	status := StatusOf(a)
	state := status.Label()
	if a.ActiveSubagents > 0 {
		state += fmt.Sprintf(" +%d", a.ActiveSubagents)
	}
	end := buf.Text(x, line+1, truncate(spend+" ", w), Dim)
	buf.Text(end, line+1, truncate(state, max(0, x+w-end)), screen.Fg(status.Color()))
}

func sprite(buf *screen.Buffer, x, y int, f frame, mirror bool, look Look) {
	for dy, row := range f {
		for i := 0; i < len(row); i++ {
			ch := row[i]
			if mirror {
				ch = row[len(row)-1-i]
			}
			if c, ok := look.color(ch); ok {
				buf.Pixel(x+i, y+dy, c)
			}
		}
	}
}

func fillRect(buf *screen.Buffer, x, y, w, h int, c screen.RGB) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			buf.Pixel(xx, yy, c)
		}
	}
}

// FmtTokens prints compact token counts: 950, 1.5K, 320K, 1.1M.
func FmtTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%dK", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	default:
		return strconv.FormatInt(n, 10)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// truncateMiddle keeps both ends, since sibling projects often share a
// prefix ("shopping-portal-front" vs "shopping-portal-service").
func truncateMiddle(s string, n int) string {
	r := []rune(s)
	if len(r) <= n || n < 5 {
		return truncate(s, n)
	}
	head := (n - 1) / 2
	tail := n - 1 - head
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}
