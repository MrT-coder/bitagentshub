package office

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

func agent(id, group, project string, st claude.State) claude.Agent {
	return claude.Agent{ID: id, Group: group, Project: project, State: st}
}

// The floor most tests use: 118 columns by 33 lines, a typical herdr pane.
const testW, testH = 118, 33

func TestBuild_PutsAgentsOfTheSameGroupInOneArea(t *testing.T) {
	agents := []claude.Agent{
		agent("1", "acme-company", "portal-front", claude.StateTool),
		agent("2", "hobbylabs", "pixel-blog", claude.StateTool),
		agent("3", "acme-company", "portal-service", claude.StateWaiting),
	}
	l := Build(agents, testW, testH)

	if len(l.Areas) != 2 {
		t.Fatalf("areas = %d, want 2", len(l.Areas))
	}
	got := map[string][]string{}
	for _, a := range l.Areas {
		for _, ag := range a.Agents {
			got[a.Name] = append(got[a.Name], ag.ID)
		}
	}
	if strings.Join(got["acme-company"], ",") != "1,3" || strings.Join(got["hobbylabs"], ",") != "2" {
		t.Errorf("areas = %v", got)
	}
}

func TestBuild_WorkstationsSitOnTheFloorWithoutOverlapping(t *testing.T) {
	var agents []claude.Agent
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		agents = append(agents, agent(id, "g"+id, id, claude.StateTool))
	}
	l := Build(agents, testW, 60)
	var seen []Workstation
	for id, w := range l.Stations {
		if w.X < 0 || w.X+stationW > l.W || w.Y < wallH || w.Y+stationH > l.H {
			t.Errorf("station %s at (%d,%d) is outside the %dx%d floor", id, w.X, w.Y, l.W, l.H)
		}
		for _, o := range seen {
			if w.X < o.X+stationW && o.X < w.X+stationW && w.Y < o.Y+stationH && o.Y < w.Y+stationH {
				t.Errorf("station %s overlaps %s", id, o.AgentID)
			}
		}
		if !l.Walkable(w.StandX(), w.StandY()) {
			t.Errorf("stand point of %s is not walkable", id)
		}
		seen = append(seen, w)
	}
}

func TestBuild_FurnishesTheOfficeWithPlacesToVisit(t *testing.T) {
	l := Build([]claude.Agent{agent("a", "g", "p", claude.StateTool)}, testW, testH)
	kinds := map[string]bool{}
	for _, p := range l.POIs {
		kinds[p.Kind] = true
		if !l.Walkable(p.X, p.Y) {
			t.Errorf("POI %s at (%d,%d) is not walkable", p.Kind, p.X, p.Y)
		}
	}
	for _, k := range []string{"coffee", "bookshelf", "window"} {
		if !kinds[k] {
			t.Errorf("office has no %s to visit; got %v", k, kinds)
		}
	}
}

func TestPath_GoesAroundFurnitureAndStaysWalkable(t *testing.T) {
	agents := []claude.Agent{agent("a", "g", "p", claude.StateTool)}
	l := Build(agents, testW, testH)
	st := l.Stations["a"]
	// From below the desk to its stand point behind the chair: the desk is
	// in the way, so the path must detour.
	fromX, fromY := st.X+stationW/2-spriteW/2, st.Y+stationH+4
	path := l.Path(fromX, fromY, st.StandX(), st.StandY())
	if len(path) == 0 {
		t.Fatal("no path found")
	}
	for _, p := range path {
		if !l.Walkable(p.X, p.Y) {
			t.Fatalf("path crosses blocked point %+v", p)
		}
	}
	last := path[len(path)-1]
	if last.X != st.StandX() || last.Y != st.StandY() {
		t.Errorf("path ends at %+v, want stand point (%d,%d)", last, st.StandX(), st.StandY())
	}
	if straight := abs(st.StandY() - fromY); len(path) <= straight/navStep+1 {
		t.Errorf("path of %d steps did not detour around the desk", len(path))
	}
}

func stepN(c *Crowd, l Layout, agents []claude.Agent, n int) {
	for i := 0; i < n; i++ {
		c.Step(l, agents)
	}
}

func TestCrowd_WorkingAgentWalksToItsChairAndSits(t *testing.T) {
	agents := []claude.Agent{agent("a", "g", "p", claude.StateTool)}
	l := Build(agents, testW, testH)
	c := NewCrowd(rand.New(rand.NewSource(1)))
	c.Sync(l, agents)

	stepN(c, l, agents, 600)

	a, st := c.Actor("a"), l.Stations["a"]
	if !a.Seated || a.X != st.SeatX() || a.Y != st.SeatY() {
		t.Errorf("actor at (%d,%d) seated=%v, want seated at (%d,%d)", a.X, a.Y, a.Seated, st.SeatX(), st.SeatY())
	}
}

func TestCrowd_IdleAgentVisitsPlacesAndNeverWalksThroughFurniture(t *testing.T) {
	agents := []claude.Agent{agent("a", "g", "p", claude.StateWaiting)}
	l := Build(agents, testW, testH)
	c := NewCrowd(rand.New(rand.NewSource(7)))
	c.Sync(l, agents)

	visited := map[string]bool{}
	for i := 0; i < 4000; i++ {
		c.Step(l, agents)
		a := c.Actor("a")
		if !a.Seated && !a.sliding() && !l.Walkable(a.X, a.Y) {
			t.Fatalf("step %d: actor stands on blocked point (%d,%d)", i, a.X, a.Y)
		}
		if a.At != "" {
			visited[a.At] = true
		}
	}
	if len(visited) < 2 {
		t.Errorf("idle agent visited %v, want at least two different places", visited)
	}
}

func TestCrowd_ForgetsAgentsThatLeft(t *testing.T) {
	agents := []claude.Agent{agent("a", "g", "p", claude.StateTool)}
	c := NewCrowd(rand.New(rand.NewSource(1)))
	c.Sync(Build(agents, testW, testH), agents)
	c.Sync(Build(nil, testW, testH), nil)
	if c.Actor("a") != nil {
		t.Error("actor still present after agent left")
	}
}

func draw(l Layout, c *Crowd, agents []claude.Agent, selected string) string {
	buf := screen.New(l.W, l.H/2)
	Draw(buf, l, c, agents, 0, selected, pet.Sleeping)
	return stripANSI(strings.Join(buf.Lines(), "\n"))
}

func settled(t *testing.T, agents []claude.Agent) (Layout, *Crowd) {
	t.Helper()
	l := Build(agents, testW, testH)
	c := NewCrowd(rand.New(rand.NewSource(1)))
	c.Sync(l, agents)
	stepN(c, l, agents, 600)
	return l, c
}

func TestDraw_LabelsAreasAndStationsWithTodaysSpend(t *testing.T) {
	agents := []claude.Agent{
		agent("1", "acme-company", "shopping-portal-front", claude.StateTool),
		agent("2", "hobbylabs", "pixel-blog", claude.StateThinking),
	}
	agents[0].Today = claude.Usage{Output: 320_000}
	agents[0].ActiveSubagents = 2
	l, c := settled(t, agents)

	out := draw(l, c, agents, "1")
	for _, want := range []string{"acme-company · 320K", "hobbylabs", ">1 shopping-…rtal-front", "320K ● working +2", "0 ● working"} {
		if !strings.Contains(out, want) {
			t.Errorf("draw missing %q:\n%s", want, out)
		}
	}
}

func TestDraw_KeepsSimilarLongProjectNamesDistinguishable(t *testing.T) {
	agents := []claude.Agent{
		agent("1", "acme-company", "shopping-portal-front", claude.StateTool),
		agent("2", "acme-company", "shopping-portal-service", claude.StateTool),
	}
	l, c := settled(t, agents)
	out := draw(l, c, agents, "")
	for _, want := range []string{"-front", "-service"} {
		if !strings.Contains(out, want) {
			t.Errorf("draw missing %q:\n%s", want, out)
		}
	}
}

func TestDraw_ShowsABubbleForBlockedAndThinkingAgents(t *testing.T) {
	blocked := agent("1", "g", "a", claude.StateTool)
	blocked.Blocked = true
	agents := []claude.Agent{blocked, agent("2", "g", "b", claude.StateThinking)}
	l, c := settled(t, agents)
	out := draw(l, c, agents, "")
	for _, want := range []string{"!", "…"} {
		if !strings.Contains(out, want) {
			t.Errorf("draw missing bubble %q:\n%s", want, out)
		}
	}
}

func TestDraw_ACheeringAgentShowsAStar(t *testing.T) {
	cheering := agent("1", "g", "a", claude.StateTool)
	cheering.Cheer = true
	agents := []claude.Agent{cheering, agent("2", "g", "b", claude.StateTool)}
	l, c := settled(t, agents)
	if out := draw(l, c, agents, ""); strings.Count(out, "★") != 1 {
		t.Errorf("one agent cheers, want one star:\n%s", out)
	}
}

func TestDraw_MonitorIsOnOnlyWhileTheAgentWorks(t *testing.T) {
	agents := []claude.Agent{agent("w", "g", "a", claude.StateTool), agent("s", "g", "b", claude.StateSleeping)}
	agents[0].Activity = claude.ActivityRunning
	l, c := settled(t, agents)
	if !screenLit(l, c, agents, "w") {
		t.Error("working agent's monitor is off")
	}
	if screenLit(l, c, agents, "s") {
		t.Error("sleeping agent's monitor is on")
	}
}

// screenLit reports whether the centre pixel of a station's monitor screen
// differs from the switched-off color.
func screenLit(l Layout, c *Crowd, agents []claude.Agent, id string) bool {
	buf := screen.New(l.W, l.H/2)
	Draw(buf, l, c, agents, 0, "", pet.Sleeping)
	st := l.Stations[id]
	x, y := st.X+monitorX+3, st.Y+monitorY+3
	return buf.PixelAt(x, y) != screenOff
}

func TestHitTest_FindsTheAgentWhereItStandsOrAtItsStation(t *testing.T) {
	agents := []claude.Agent{
		agent("seated", "g", "a", claude.StateTool),
		agent("roamer", "g", "b", claude.StateWaiting),
	}
	l, c := settled(t, agents)
	r := c.Actor("roamer")
	r.X, r.Y, r.path, r.Seated, r.Pause = 100, 40, nil, false, 999

	seat, away := l.Stations["seated"], l.Stations["roamer"]
	lx, ly := away.LabelCell()
	tests := []struct {
		name string
		x, y int
		want string
	}{
		{"seated sprite", seat.SeatX() + 3, (seat.SeatY() + 4) / 2, "seated"},
		{"roaming sprite where it stands", 103, (40 + 6) / 2, "roamer"},
		{"label of the roamer's empty station", lx + 1, ly, "roamer"},
		{"bare floor", 2, testH - 2, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := HitTest(l, c, tt.x, tt.y); got != tt.want {
				t.Errorf("HitTest(%d,%d) = %q, want %q", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestOrder_ListsAgentsAreaByAreaInStationOrder(t *testing.T) {
	agents := []claude.Agent{
		agent("z", "b-area", "a", claude.StateTool),
		agent("y", "a-area", "b", claude.StateTool),
		agent("x", "a-area", "a", claude.StateTool),
	}
	if got := strings.Join(Order(Build(agents, testW, testH)), ","); got != "x,y,z" {
		t.Errorf("order = %s, want x,y,z", got)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			inEsc = false
		case !inEsc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestFrames_AreAllEightByFourteen(t *testing.T) {
	var all []frame
	for p, frames := range BuiltinTheme().poses {
		if len(frames) != poseSpecs[p].frames {
			t.Errorf("pose %s has %d frames, want %d", poseSpecs[p].name, len(frames), poseSpecs[p].frames)
		}
		all = append(all, frames...)
	}
	for i, f := range all {
		for r, row := range f {
			if len(row) != spriteW {
				t.Errorf("frame %d row %d is %d wide: %q", i, r, len(row), row)
			}
		}
	}
}

func TestCrowd_SyncWithTheSameLayoutKeepsWhereAgentsAreHeading(t *testing.T) {
	agents := []claude.Agent{agent("a", "g", "p", claude.StateWaiting)}
	l := Build(agents, testW, testH)
	c := NewCrowd(rand.New(rand.NewSource(7)))
	c.Sync(l, agents)
	stepN(c, l, agents, 3)
	a := c.Actor("a")
	poi, steps := a.poi, len(a.path)
	if poi < 0 || steps == 0 {
		t.Fatalf("setup: actor should be walking to a POI, poi=%d path=%d", poi, steps)
	}

	c.Sync(Build(agents, testW, testH), agents) // the next snapshot

	if a.poi != poi || len(a.path) != steps {
		t.Errorf("after an unchanged sync poi=%d path=%d, want %d %d", a.poi, len(a.path), poi, steps)
	}
}
