package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/prs"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// diffW is the widest the diff view's window gets.
const diffW = 140

// gutterMinInner is the inside width from which a diff shows its line
// numbers; narrower, as on a phone, the code gets the room.
const gutterMinInner = 60

// tabWidth is how many columns a tab of the code takes on screen.
const tabWidth = 4

// prsDiffMsg is the answer of reading a pull request's diff.
type prsDiffMsg struct {
	repo   string
	number int
	files  []prs.FileDiff
	sha    string // the head commit the diff was read at; "" when unknown
	err    error
}

type diffRowKind int

const (
	rowFile diffRowKind = iota
	rowHunk
	rowLine
	rowNote
)

// diffRow is one row of the diff view.
type diffRow struct {
	kind diffRowKind
	text string       // the file's path, a hunk header or a note
	stat string       // a file's "+3 −1"
	file int          // index of the file in diffState.files
	line prs.DiffLine // for rowLine
}

// diffState is the diff view of one pull request.
type diffState struct {
	pr      prs.PR
	files   []prs.FileDiff
	rows    []diffRow
	loading bool
	err     error
	cur     int // the row under the cursor
	scroll  int // the first row shown
	sha     string

	mode  diffMode // what is going on besides browsing
	act   diffAct  // the action being typed, confirmed or sent
	draft string   // the text being typed
	tgt   lineTarget
	msg   string // a hint, or what the last action did
}

// prsDiffOpen tells whether the PRs tab shows a diff instead of the list.
func (m Model) prsDiffOpen() bool {
	return m.hubTab == tabPRs && m.pulls != nil && m.pulls.diff != nil
}

// prsOpenDiff opens the diff of the selected pull request and reads it.
func (m Model) prsOpenDiff() (Model, tea.Cmd) {
	p := m.pulls
	list := p.list()
	if p.run == nil || p.sel < 0 || p.sel >= len(list) {
		return m, nil
	}
	pr := list[p.sel]
	p.diff = &diffState{pr: pr, loading: true}
	run := p.run
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), prsReadTime)
		defer cancel()
		files, err := prs.Diff(ctx, run, pr.Repo, pr.Number)
		sha := ""
		if err == nil {
			// Only a line comment needs it, and the diff reads fine without.
			sha, _ = prs.HeadSHA(ctx, run, pr.Repo, pr.Number)
		}
		return prsDiffMsg{repo: pr.Repo, number: pr.Number, files: files, sha: sha, err: err}
	}
}

// prsDiffRead takes in a diff. One for a pull request the user already left
// is dropped.
func (m Model) prsDiffRead(msg prsDiffMsg) Model {
	p := m.pulls
	if p == nil || p.diff == nil || p.diff.pr.Repo != msg.repo || p.diff.pr.Number != msg.number {
		return m
	}
	d := p.diff
	d.loading, d.err = false, msg.err
	if msg.err == nil {
		d.files, d.sha = msg.files, msg.sha
		d.rows = diffRows(msg.files)
	}
	return m
}

// diffRows flattens the files into the rows the view scrolls through.
func diffRows(files []prs.FileDiff) []diffRow {
	var rows []diffRow
	for i, f := range files {
		name := cleanText(f.Path)
		if f.Status == prs.StatusRenamed && f.OldPath != "" {
			name = cleanText(f.OldPath) + " → " + name
		}
		stat := fmt.Sprintf("+%d −%d", f.Added, f.Removed)
		switch {
		case f.Binary:
			stat = "" // its note says it is binary; counts of lines would be noise
		case f.Status == prs.StatusAdded:
			stat = i18n.Tf("new · +%d", f.Added)
		case f.Status == prs.StatusDeleted:
			stat = i18n.Tf("deleted · −%d", f.Removed)
		}
		rows = append(rows, diffRow{kind: rowFile, text: name, stat: stat, file: i})
		switch {
		case f.Binary:
			rows = append(rows, diffRow{kind: rowNote, text: i18n.T("binary file"), file: i})
		case len(f.Hunks) == 0:
			rows = append(rows, diffRow{kind: rowNote, text: i18n.T("no lines to show"), file: i})
		}
		for _, h := range f.Hunks {
			rows = append(rows, diffRow{kind: rowHunk, text: cleanText(h.Header), file: i})
			for _, l := range h.Lines {
				l.Text = cleanText(l.Text)
				rows = append(rows, diffRow{kind: rowLine, file: i, line: l})
			}
		}
	}
	return rows
}

// cleanText makes text from somebody else safe to draw: tabs become spaces
// and every control character, including the escape that starts a terminal
// sequence, goes.
func cleanText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteString(strings.Repeat(" ", tabWidth))
		case unicode.IsControl(r):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// diffRoom is how many rows the diff view shows.
func (m Model) diffRoom() int {
	_, _, _, h := m.hubRect()
	return max(1, h-4)
}

// diffMove puts the cursor on row to, kept inside the diff, and scrolls to
// keep it in view.
func (m Model) diffMove(to int) Model {
	d := m.pulls.diff
	if d == nil || len(d.rows) == 0 {
		return m
	}
	room := m.diffRoom()
	d.cur = max(0, min(len(d.rows)-1, to))
	if d.cur < d.scroll {
		d.scroll = d.cur
	}
	if d.cur >= d.scroll+room {
		d.scroll = d.cur - room + 1
	}
	d.scroll = clampScroll(d.scroll, len(d.rows), room)
	return m
}

// diffJump moves the cursor to the next file's row, or the previous one.
func (m Model) diffJump(dir int) Model {
	d := m.pulls.diff
	for i := d.cur + dir; i >= 0 && i < len(d.rows); i += dir {
		if d.rows[i].kind == rowFile {
			return m.diffMove(i)
		}
	}
	return m
}

// prsDiffKey handles the keys of the diff view: esc goes back to the list.
func (m Model) prsDiffKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	d := m.pulls.diff
	switch d.mode {
	case modeTyping:
		return m.diffTypeKey(msg)
	case modeConfirm:
		return m.diffConfirmKey(msg)
	case modeSending:
		return m, nil
	}
	d.msg = ""
	switch msg.String() {
	case "o":
		return m.prsOpenFile()
	case "c":
		return m.diffComment(), nil
	case "a":
		return m.diffStart(actApprove), nil
	case "x":
		return m.diffStart(actChanges), nil
	case "m":
		return m.diffStart(actNote), nil
	case "esc", "q", "backspace":
		m.pulls.diff = nil
	case "up", "k":
		return m.diffMove(d.cur - 1), nil
	case "down", "j":
		return m.diffMove(d.cur + 1), nil
	case "pgup":
		return m.diffMove(d.cur - m.diffRoom()), nil
	case "pgdown":
		return m.diffMove(d.cur + m.diffRoom()), nil
	case "home", "g":
		return m.diffMove(0), nil
	case "end", "G":
		return m.diffMove(len(d.rows) - 1), nil
	case "]":
		return m.diffJump(1), nil
	case "[":
		return m.diffJump(-1), nil
	}
	return m, nil
}

// diffClick moves the cursor to the row on window row y.
func (m Model) diffClick(y int) Model {
	d := m.pulls.diff
	first := clampScroll(d.scroll, len(d.rows), m.diffRoom())
	if y < 0 || y >= m.diffRoom() || first+y >= len(d.rows) {
		return m
	}
	return m.diffMove(first + y)
}

// Colors of the diff: added and removed lines on a tinted row.
var (
	diffAdded   = screen.Style{Fg: screen.RGB{R: 0xa3, G: 0xbe, B: 0x8c}, HasFg: true, Bg: screen.RGB{R: 0x20, G: 0x35, B: 0x28}, HasBg: true}
	diffRemoved = screen.Style{Fg: screen.RGB{R: 0xe0, G: 0x7a, B: 0x84}, HasFg: true, Bg: screen.RGB{R: 0x3a, G: 0x20, B: 0x24}, HasBg: true}
)

// drawDiff paints the diff view: the pull request on the first row, then the
// rows from the first shown, the cursor's reversed.
func (m Model) drawDiff(buf *screen.Buffer) {
	d := m.pulls.diff
	x, y, w, h := hubFrame(buf.W, buf.H, diffW, buf.H)
	m.hubBox(buf, x, y, w, h, i18n.T(" ↑↓ move · ]/[ file · esc back "))
	inner, room := w-4, max(1, h-4)

	title := modalText()
	title.Bold = true
	buf.Text(x+2, y+1, truncate(cleanText(fmt.Sprintf("%s#%d  %s  @%s", d.pr.Repo, d.pr.Number, d.pr.Title, d.pr.Author)), inner), title)

	switch {
	case d.loading:
		buf.Text(x+2, y+2, i18n.T("Loading diff…"), modalDim())
		return
	case d.err != nil:
		buf.Text(x+2, y+2, truncate(i18n.Tf("Could not read the diff: %s", cleanText(d.err.Error())), inner), modalBorder())
		return
	case len(d.rows) == 0:
		buf.Text(x+2, y+2, i18n.T("This pull request has no changes."), modalDim())
		return
	}
	first := clampScroll(d.scroll, len(d.rows), room)
	for i, r := range d.rows[first:min(len(d.rows), first+room)] {
		drawDiffRow(buf, x+2, y+2+i, inner, r, first+i == d.cur)
	}
	m.drawDiffPrompt(buf, x+2, y+h-2, inner)
}

// drawDiffRow paints one row inner cells wide at x, y.
func drawDiffRow(buf *screen.Buffer, x, y, inner int, r diffRow, cursor bool) {
	st, gutter := modalText(), modalDim()
	switch r.kind {
	case rowFile:
		st = modalBorder()
		st.Bold = true
	case rowHunk, rowNote:
		st = modalDim()
	case rowLine:
		switch r.line.Kind {
		case prs.Added:
			st, gutter = diffAdded, diffAdded
		case prs.Removed:
			st, gutter = diffRemoved, diffRemoved
		}
	}
	if cursor {
		st.Reverse, gutter.Reverse = true, true
	}
	buf.Fill(x, y, inner, 1, ' ', st)

	switch r.kind {
	case rowFile:
		stat := ""
		if r.stat != "" {
			stat = "  " + r.stat
		}
		buf.Text(x, y, truncate("▸ "+r.text, max(0, inner-len([]rune(stat)))), st)
		buf.Text(x+max(0, inner-len([]rune(stat))), y, truncate(stat, inner), st)
	case rowHunk, rowNote:
		buf.Text(x, y, truncate(r.text, inner), st)
	case rowLine:
		cx, room := x, inner
		if inner >= gutterMinInner {
			g := fmt.Sprintf("%4s %4s ", lineNumber(r.line.Old), lineNumber(r.line.New))
			buf.Text(cx, y, g, gutter)
			cx += len(g)
			room -= len(g)
		}
		marker := " "
		switch r.line.Kind {
		case prs.Added:
			marker = "+"
		case prs.Removed:
			marker = "-"
		}
		buf.Text(cx, y, truncate(marker+r.line.Text, room), st)
	}
}

func lineNumber(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}
