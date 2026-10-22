package office

// navStep is the walk grid spacing in pixels. Paths visit every node, so
// consecutive points are one step apart.
const navStep = 2

type Point struct{ X, Y int }

// Path finds a shortest route for a sprite from (fx, fy) to (tx, ty) over
// walkable grid nodes, ending exactly on the target. The start may be
// blocked (an agent getting up from a chair); the target must not be. It
// returns nil when the target cannot be reached.
func (l Layout) Path(fx, fy, tx, ty int) []Point {
	if !l.Walkable(tx, ty) {
		return nil
	}
	cols := l.W/navStep + 1
	rows := max(l.H, l.Needed)/navStep + 1
	idx := func(p Point) int { return (p.Y/navStep)*cols + p.X/navStep }
	snap := func(v int) int { return v - v%navStep }

	start := Point{snap(fx), snap(fy)}
	// An off-grid target is reached from the nearest walkable grid node,
	// then with one last step.
	goal, ok := Point{}, false
	for _, d := range [5]Point{{0, 0}, {navStep, 0}, {0, navStep}, {navStep, navStep}, {-navStep, 0}} {
		if g := (Point{snap(tx) + d.X, snap(ty) + d.Y}); l.Walkable(g.X, g.Y) {
			goal, ok = g, true
			break
		}
	}
	if !ok {
		return nil
	}
	prev := make([]int32, cols*rows)
	for i := range prev {
		prev[i] = -1
	}
	queue := []Point{start}
	prev[idx(start)] = int32(idx(start))
	found := start == goal
	for len(queue) > 0 && !found {
		p := queue[0]
		queue = queue[1:]
		for _, d := range [4]Point{{navStep, 0}, {-navStep, 0}, {0, navStep}, {0, -navStep}} {
			n := Point{p.X + d.X, p.Y + d.Y}
			if n.X < 0 || n.Y < 0 || n.X/navStep >= cols || n.Y/navStep >= rows {
				continue
			}
			if prev[idx(n)] != -1 || !l.Walkable(n.X, n.Y) {
				continue
			}
			prev[idx(n)] = int32(idx(p))
			if n == goal {
				found = true
				break
			}
			queue = append(queue, n)
		}
	}
	if !found {
		return nil
	}
	var path []Point
	for p := goal; p != start; {
		path = append(path, p)
		i := int(prev[idx(p)])
		p = Point{(i % cols) * navStep, (i / cols) * navStep}
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	if last := (Point{tx, ty}); len(path) == 0 || path[len(path)-1] != last {
		path = append(path, last)
	}
	return path
}
