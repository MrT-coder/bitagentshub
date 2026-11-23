package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/prs"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// prsMsg is one reading of the pull requests. A manual one, asked for by
// opening the tab or pressing r, does not schedule the next periodic one.
type prsMsg struct {
	status prs.Status
	inbox  prs.Inbox
	err    error
	manual bool
}

type prsTickMsg struct{}

// prsW is the widest the PRs tab's window gets.
const prsW = 100

// prsReadTime bounds one reading, which asks GitHub several times.
const prsReadTime = 30 * time.Second

// prsState is what the PRs tab knows. It is a pointer so the copies of the
// model share one reading.
type prsState struct {
	cfg    config.PRs
	run    prs.Runner
	status prs.Status
	loaded bool // an inbox has been read at least once
	inbox  prs.Inbox
	err    error // the last reading that failed, the inbox kept as it was
	sel    int   // index among the pull requests of the inbox, in group order
	scroll int   // first row shown
}

// WithPRs turns the pull request window on as [prs] says, reading through
// run, which is gh in the office and a fake in tests.
func (m Model) WithPRs(cfg config.PRs, run prs.Runner) Model {
	if cfg.Refresh <= 0 {
		cfg.Refresh = 2 * time.Minute
	}
	m.pulls = &prsState{cfg: cfg, run: run}
	return m
}

// prsPoll reads the pull requests off the UI thread.
func (m Model) prsPoll(manual bool) tea.Cmd {
	p := m.pulls
	if p == nil || !p.cfg.Enabled || p.run == nil {
		return nil
	}
	run, f := p.run, prs.Filter{Orgs: p.cfg.Orgs, Repos: p.cfg.Repos}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), prsReadTime)
		defer cancel()
		if st := prs.Setup(ctx, run); st != prs.Ready {
			return prsMsg{status: st, manual: manual}
		}
		in, err := prs.Load(ctx, run, f)
		return prsMsg{status: prs.Ready, inbox: in, err: err, manual: manual}
	}
}

// prsRead takes a reading in. A failed one keeps the list of the last good
// reading, so a network blip does not empty the window.
func (m Model) prsRead(msg prsMsg) Model {
	p := m.pulls
	if p == nil {
		return m
	}
	p.status = msg.status
	switch {
	case msg.status != prs.Ready:
		p.err = nil
	case msg.err != nil:
		p.err = msg.err
	default:
		p.inbox, p.loaded, p.err = msg.inbox, true, nil
	}
	p.sel = max(0, min(p.sel, len(p.list())-1))
	return m
}

// prsNext is what follows a reading: the next periodic one, unless the
// reading was asked for by hand.
func (m Model) prsNext(msg prsMsg) tea.Cmd {
	if msg.manual || m.pulls == nil || !m.pulls.cfg.Enabled {
		return nil
	}
	return tea.Tick(m.pulls.cfg.Refresh, func(time.Time) tea.Msg { return prsTickMsg{} })
}

// list is every pull request of the inbox, in the order the groups show them.
func (p *prsState) list() []prs.PR {
	var out []prs.PR
	for _, g := range prs.Groups() {
		out = append(out, p.inbox.Group(g)...)
	}
	return out
}

// Pending is how many pull requests wait for your review, or of your teams.
func (m Model) prsPending() int {
	if m.pulls == nil || !m.pulls.cfg.Enabled {
		return 0
	}
	return m.pulls.inbox.Pending()
}

// prRow is one row of the PRs tab; pr is the index of its pull request in
// the inbox list, or -1 for a heading or a message.
type prRow struct {
	text  string
	style screen.Style
	pr    int
}

func (m Model) prRows() []prRow {
	dim, text := modalDim(), modalText()
	p := m.pulls
	switch {
	case p == nil || !p.cfg.Enabled:
		return []prRow{{i18n.T("Pull requests are off ([prs] in the config turns them on)."), dim, -1}}
	case p.status == prs.NotInstalled:
		return []prRow{{i18n.T("gh is not installed. Get it at https://cli.github.com and press r."), modalBorder(), -1}}
	case p.status == prs.NotLoggedIn:
		return []prRow{{i18n.T("gh is not logged in. Run gh auth login in a terminal and press r."), modalBorder(), -1}}
	}
	rows := []prRow{{prsFilterLine(p.cfg), dim, -1}}
	if !p.loaded {
		return append(rows, prRow{i18n.T("Loading pull requests…"), dim, -1})
	}
	if p.err != nil {
		rows = append(rows, prRow{i18n.Tf("Could not read pull requests: %s", p.err), modalBorder(), -1})
	}
	heading := text
	heading.Bold = true
	n := 0
	for _, g := range prs.Groups() {
		list := p.inbox.Group(g)
		rows = append(rows, prRow{prsGroupLabel(g, len(list)), heading, -1})
		for _, pr := range list {
			marker, st := "  ", text
			if n == p.sel {
				marker, st.Reverse = "▸ ", true
			}
			rows = append(rows, prRow{fmt.Sprintf("%s%s#%d  %s  @%s", marker, pr.Repo, pr.Number, pr.Title, pr.Author), st, n})
			n++
		}
	}
	return rows
}

func prsFilterLine(c config.PRs) string {
	var parts []string
	for _, o := range c.Orgs {
		parts = append(parts, "org "+o)
	}
	for _, r := range c.Repos {
		parts = append(parts, "repo "+r)
	}
	if len(parts) == 0 {
		return i18n.T("Showing: every pull request")
	}
	return i18n.Tf("Showing: %s", strings.Join(parts, " · "))
}

func prsGroupLabel(g prs.Group, n int) string {
	switch g {
	case prs.NeedsReview:
		return i18n.Tf("Needs your review (%d)", n)
	case prs.NeedsTeamReview:
		return i18n.Tf("Needs your teams' review (%d)", n)
	case prs.Drafts:
		return i18n.Tf("Your drafts (%d)", n)
	case prs.Waiting:
		return i18n.Tf("Waiting for review or checks (%d)", n)
	case prs.NeedsAction:
		return i18n.Tf("Needs action (%d)", n)
	}
	return i18n.Tf("Ready to merge (%d)", n)
}

// rowOfSelection is the row that shows the selected pull request, or -1.
func (m Model) prsSelectedRow() int {
	for i, r := range m.prRows() {
		if r.pr >= 0 && r.pr == m.pulls.sel {
			return i
		}
	}
	return -1
}

// prsFollow scrolls the tab so the selected pull request, and the heading
// above it when it is the first of its group, stay in view.
func (m Model) prsFollow() Model {
	if m.pulls == nil {
		return m
	}
	rows := m.prRows()
	_, _, _, h := m.hubRect()
	room := max(1, h-4)
	p := m.pulls
	if i := m.prsSelectedRow(); i >= 0 {
		if i-1 < p.scroll {
			p.scroll = max(0, i-1)
		}
		if i >= p.scroll+room {
			p.scroll = i - room + 1
		}
	}
	p.scroll = clampScroll(p.scroll, len(rows), room)
	return m
}

// prsKey handles the keys of the PRs tab.
func (m Model) prsKey(k string) (Model, tea.Cmd) {
	p := m.pulls
	if p == nil {
		return m, nil
	}
	switch k {
	case "up", "k":
		p.sel = max(0, p.sel-1)
	case "down", "j":
		p.sel = max(0, min(len(p.list())-1, p.sel+1))
	case "r":
		return m, m.prsPoll(true)
	default:
		return m, nil
	}
	return m.prsFollow(), nil
}

// prsClick selects the pull request on the window row y, counted from the
// first row inside the border's frame.
func (m Model) prsClick(y int) Model {
	p := m.pulls
	if p == nil {
		return m
	}
	rows := m.prRows()
	_, _, _, h := m.hubRect()
	room := max(1, h-4)
	first := clampScroll(p.scroll, len(rows), room)
	if y < 0 || y >= room || first+y >= len(rows) {
		return m
	}
	if r := rows[first+y]; r.pr >= 0 {
		p.sel = r.pr
	}
	return m
}

// scrollPRs scrolls the PRs tab by delta rows.
func (m Model) scrollPRs(delta int) Model {
	if m.pulls == nil {
		return m
	}
	_, _, _, h := m.hubRect()
	m.pulls.scroll = clampScroll(m.pulls.scroll+delta, len(m.prRows()), h-4)
	return m
}

// drawPRs paints the PRs tab: the inbox groups of GitHub, scrolled with ↑↓
// and the wheel.
func (m Model) drawPRs(buf *screen.Buffer) {
	rows := m.prRows()
	x, y, w, h := hubFrame(buf.W, buf.H, prsW, len(rows)+4)
	m.hubBox(buf, x, y, w, h, i18n.T(" ←→ tab · ↑↓ select · r refresh · esc close "))
	room := h - 4
	first := 0
	if m.pulls != nil {
		first = clampScroll(m.pulls.scroll, len(rows), room)
	}
	for i, r := range rows[first:min(len(rows), first+room)] {
		buf.Text(x+2, y+2+i, truncate(r.text, w-4), r.style)
	}
}
