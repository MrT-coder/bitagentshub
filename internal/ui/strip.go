package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// agentKind names the program an agent runs ("claude", "codex") from its
// command line.
func agentKind(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	base := filepath.Base(strings.ReplaceAll(argv[0], `\`, "/"))
	return strings.TrimSuffix(strings.ToLower(base), ".exe")
}

// kindOf is the program behind a station: what herdr detected, what the
// office started, or Claude, whose transcripts the office reads.
func (m Model) kindOf(id string) string {
	if p, ok := m.paneOf(id); ok && p.Agent != "" {
		return p.Agent
	}
	if hs, ok := m.terms[id]; ok && hs.kind != "" {
		return hs.kind
	}
	return "claude"
}

// Colors of the commits not pushed and not pulled yet.
var (
	aheadStyle  = screen.Fg(screen.RGB{R: 0x98, G: 0xc3, B: 0x79})
	behindStyle = screen.Fg(screen.RGB{R: 0xe0, G: 0x6c, B: 0x75})
)

// Names and branches start at maxBranch cells and shrink as far as
// minBranch to fit every station; past that the strip scrolls.
const (
	maxBranch = 18
	minBranch = 6
	stripGap  = 3
)

// stripCol is one station in the strip, at column x and w cells wide.
type stripCol struct {
	id                  string
	x, w                int
	name, branch, state string
	ahead, behind       string
	nameStyle           screen.Style
	color               screen.RGB
}

// station is what the strip knows of one station before it is laid out.
type station struct {
	id, label, folder, state string
	git                      gitState
	color                    screen.RGB
	open                     bool
}

// stripView is the laid out strip: the columns in view, the index of the
// first one among all stations, and how many stations are out of view on
// each side.
type stripView struct {
	cols                   []stripCol
	first, hidden, hiddenR int
}

// marker is what stands for n stations out of view on one side.
func marker(n int, left bool) string {
	if left {
		return fmt.Sprintf("‹ +%d", n)
	}
	return fmt.Sprintf("+%d ›", n)
}

// strip is the three header rows that list the stations side by side, as
// herdr does: number, agent program and folder name, the git branch with commits ahead
// (green) and behind (red) its upstream, then the state. It lets a glance
// from inside a window tell which station is free. The open station is
// marked; when stations are out of view, "‹ +2" and "+1 ›" count them, as
// zellij's tab bar does.
func (m Model) strip() []string {
	buf := screen.New(max(1, m.width), 3)
	v := m.stripLayout()
	if v.hidden > 0 {
		buf.Text(0, 0, marker(v.hidden, true), office.Dim)
	}
	if v.hiddenR > 0 {
		r := marker(v.hiddenR, false)
		buf.Text(max(0, buf.W-len([]rune(r))), 0, r, office.Dim)
	}
	for _, c := range v.cols {
		buf.Text(c.x, 0, c.name, c.nameStyle)
		gx := buf.Text(c.x, 1, c.branch, office.Dim)
		if c.ahead+c.behind != "" {
			gx = buf.Text(gx+1, 1, c.ahead, aheadStyle)
			buf.Text(gx, 1, c.behind, behindStyle)
		}
		buf.Text(c.x, 2, c.state, screen.Fg(c.color))
	}
	return buf.Lines()
}

// stripHit is the station whose column holds screen column x.
func (m Model) stripHit(x int) (string, bool) {
	for _, c := range m.stripLayout().cols {
		if x >= c.x && x < c.x+c.w {
			return c.id, true
		}
	}
	return "", false
}

// markerHit is -1 or +1 when screen column x is on the ‹ or › marker.
func (m Model) markerHit(x int) int {
	v := m.stripLayout()
	switch {
	case v.hidden > 0 && x < len([]rune(marker(v.hidden, true))):
		return -1
	case v.hiddenR > 0 && x >= m.width-len([]rune(marker(v.hiddenR, false))):
		return 1
	}
	return 0
}

// scrollStrip moves the strip by delta stations; from then on it stays
// where the user left it until a station is opened.
func (m Model) scrollStrip(delta int) Model {
	v := m.stripLayout()
	if v.hidden+v.hiddenR == 0 {
		return m
	}
	m.stripFirst = max(0, min(v.first+delta, v.first+len(v.cols)+v.hiddenR-1))
	return m
}

// stripMouse handles the mouse over the strip: a click on a station opens
// it, a click on a marker or the wheel scrolls the strip.
func (m Model) stripMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || m.picker != nil {
			return m, nil
		}
		if d := m.markerHit(msg.X); d != 0 {
			return m.scrollStrip(d), nil
		}
		if id, ok := m.stripHit(msg.X); ok {
			return m.switchTo(id)
		}
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp, tea.MouseWheelLeft:
			return m.scrollStrip(-1), nil
		case tea.MouseWheelDown, tea.MouseWheelRight:
			return m.scrollStrip(1), nil
		}
	}
	return m, nil
}

// stripLayout places the stations in board order. Names shrink until every
// station fits; when even the shortest names do not, the strip shows a run
// of stations: from where the user scrolled it, or else around the open
// station (or the selected one).
func (m Model) stripLayout() stripView {
	stations := m.stations()
	if len(stations) == 0 {
		return stripView{}
	}
	var cols []stripCol
	for limit := maxBranch; limit >= minBranch; limit-- {
		cols = layoutCols(stations, limit)
		if span(cols) <= m.width {
			place(cols, 0)
			return stripView{cols: cols}
		}
	}
	n := len(cols)
	// A run fits with room for the markers of the stations it leaves out.
	fits := func(s, e int) bool {
		w := span(cols[s : e+1])
		if s > 0 {
			w += len([]rune(marker(s, true))) + 2
		}
		if e < n-1 {
			w += len([]rune(marker(n-1-e, false))) + 2
		}
		return w <= m.width
	}
	var s, e int
	if m.stripFirst >= 0 {
		// Rightwards from where the user scrolled, then back left at the end.
		s = min(m.stripFirst, n-1)
		e = s
		for e+1 < n && fits(s, e+1) {
			e++
		}
		for s > 0 && fits(s-1, e) {
			s--
		}
	} else {
		focus := 0
		for i, st := range stations {
			if st.open || (focus == 0 && st.id == m.selected) {
				focus = i
			}
		}
		// Grow from the focus, rightwards first.
		s, e = focus, focus
		for {
			if e+1 < n && fits(s, e+1) {
				e++
			} else if s > 0 && fits(s-1, e) {
				s--
			} else {
				break
			}
		}
	}
	x := 0
	if s > 0 {
		x = len([]rune(marker(s, true))) + 2
	}
	cols = cols[s : e+1]
	place(cols, x)
	return stripView{cols: cols, first: s, hidden: s, hiddenR: n - 1 - e}
}

// stations gathers the strip's stations in board order.
func (m Model) stations() []station {
	byID := map[string]claude.Agent{}
	for _, a := range m.agents() {
		byID[a.ID] = a
	}
	var out []station
	for i, id := range office.Order(m.layout) {
		a, ok := byID[id]
		if !ok {
			continue
		}
		status := office.StatusOf(a)
		state := status.Label()
		if c, ok := m.tasks[projectOfAgent(a)]; ok && m.board.Dir != "" && c.Pending > 0 {
			state += i18n.Tf(" · %d todo", c.Pending)
		}
		folder := a.Project
		if a.Cwd != "" {
			folder = filepath.Base(a.Cwd)
		}
		label := m.kindOf(id)
		if i < 9 {
			label = fmt.Sprintf("%d %s", i+1, label)
		}
		out = append(out, station{id: id, label: label, folder: folder, state: state, color: status.Color(),
			git: m.git[folderKey(a.Cwd)], open: m.win != nil && m.win.agentID == id})
	}
	return out
}

// layoutCols builds the columns with folder names and branches cut to limit
// cells; x is set by place.
func layoutCols(stations []station, limit int) []stripCol {
	cols := make([]stripCol, len(stations))
	for i, st := range stations {
		name := st.label
		if st.folder != "" {
			name += " · " + truncate(st.folder, limit)
		}
		c := stripCol{id: st.id, name: name, branch: truncate(st.git.branch, limit), state: st.state, color: st.color,
			nameStyle: screen.Style{Fg: office.AgentColor(st.id), HasFg: true, Bold: true}}
		c.nameStyle.Reverse = st.open
		if st.git.ahead > 0 {
			c.ahead = fmt.Sprintf("↑%d", st.git.ahead)
		}
		if st.git.behind > 0 {
			c.behind = fmt.Sprintf("↓%d", st.git.behind)
		}
		gitW := len([]rune(c.branch))
		if c.ahead+c.behind != "" {
			gitW += 1 + len([]rune(c.ahead+c.behind))
		}
		c.w = max(len([]rune(c.name)), gitW, len([]rune(c.state)))
		cols[i] = c
	}
	return cols
}

// span is how many cells the columns take side by side.
func span(cols []stripCol) int {
	w := 0
	for _, c := range cols {
		w += c.w
	}
	return w + stripGap*max(0, len(cols)-1)
}

// place sets each column's x, starting at x.
func place(cols []stripCol, x int) {
	for i := range cols {
		cols[i].x = x
		x += cols[i].w + stripGap
	}
}
