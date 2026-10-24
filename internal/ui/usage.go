package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/screen"
	"github.com/MrT-coder/bitagentshub/internal/usage"
)

// usageEvery is how often an open usage tab reads the tools again.
const usageEvery = 5 * time.Second

type usageMsg []usage.Report

// Bar colors for a limit by how much of it is used.
var (
	limitOK   = screen.Fg(screen.RGB{R: 0x98, G: 0xc3, B: 0x79})
	limitWarn = screen.Fg(screen.RGB{R: 0xe5, G: 0xc0, B: 0x7b})
	limitHot  = screen.Fg(screen.RGB{R: 0xe0, G: 0x6c, B: 0x75})
)

// WithUsage sets the tools whose own usage and limits the usage tab
// reads from this machine.
func (m Model) WithUsage(sources ...usage.Source) Model {
	m.sources = sources
	return m
}

// readUsage reads every source in the background; tools that leave nothing
// on this machine are dropped.
func (m Model) readUsage() tea.Cmd {
	sources := m.sources
	if len(sources) == 0 {
		return nil
	}
	return func() tea.Msg {
		now := time.Now()
		var out usageMsg
		for _, s := range sources {
			if r, ok := s.Read(now); ok {
				r.Windows = usage.Fresh(r.Windows, now)
				out = append(out, r)
			}
		}
		return out
	}
}

// usageRow paints one row of the usage tab at x, y, inner cells wide.
type usageRow func(buf *screen.Buffer, x, y, inner int)

// drawUsage paints the hub's usage tab: a row per station with bars for
// today's and the session's tokens, the office totals, then what each tool
// reports, its real limits only when it has them. The rows scroll with ↑↓
// and the wheel.
func (m Model) drawUsage(buf *screen.Buffer) {
	rows := m.usageRows()
	x, y, w, h := hubFrame(buf.W, buf.H, usageW, len(rows)+4)
	m.hubBox(buf, x, y, w, h, i18n.T(" ←→ tab · ↑↓ scroll · esc close "))
	inner, room := w-4, h-4
	first := clampScroll(m.usageScroll, len(rows), room)
	for i, row := range rows[first:min(len(rows), first+room)] {
		row(buf, x+2, y+2+i, inner)
	}
}

// Widths of the station label and of a token count in the usage tab.
const (
	usageLabelW = 28
	usageNumW   = 6
)

// usageRows are the rows of the usage tab, top to bottom.
func (m Model) usageRows() []usageRow {
	agents := m.agents()
	byID := map[string]int{}
	for i, a := range agents {
		byID[a.ID] = i
	}
	var order []int // indexes into agents, in station order
	for _, id := range office.Order(m.layout) {
		if i, ok := byID[id]; ok {
			order = append(order, i)
		}
	}
	var maxToday, maxSession int64
	for _, i := range order {
		maxToday = max(maxToday, agents[i].Today.Counted())
		maxSession = max(maxSession, agents[i].Session.Counted())
	}
	text := func(s string, st screen.Style) usageRow {
		return func(buf *screen.Buffer, x, y, inner int) { buf.Text(x, y, truncate(s, inner), st) }
	}
	var rows []usageRow
	for n, i := range order {
		a := agents[i]
		label := m.kindOf(a.ID) + " " + a.Project
		if n < 9 {
			label = fmt.Sprintf("%d %s", n+1, label)
		}
		rows = append(rows, func(buf *screen.Buffer, x, y, inner int) {
			barW := usageBarW(inner)
			lx := buf.Text(x, y, pad(truncate(label, usageLabelW), usageLabelW), screen.Style{Fg: office.AgentColor(a.ID), HasFg: true, Bold: true})
			lx = buf.Text(lx, y, i18n.T(" today "), modalDim())
			lx = bar(buf, lx, y, barW, a.Today.Counted(), maxToday, modalBorder())
			lx = buf.Text(lx, y, " "+pad(office.FmtTokens(a.Today.Counted()), usageNumW-1), modalText())
			lx = buf.Text(lx, y, i18n.T(" session "), modalDim())
			lx = bar(buf, lx, y, barW, a.Session.Counted(), maxSession, modalBorder())
			buf.Text(lx, y, " "+office.FmtTokens(a.Session.Counted()), modalText())
		})
	}
	if s := m.snap; s != nil {
		rows = append(rows, text("", modalText()),
			text(i18n.Tf("office today %s · week %s", office.FmtTokens(s.Today.Counted()), office.FmtTokens(s.Week.Counted())), modalText()))
	}
	claudeLimits := false
	for _, r := range m.reports {
		claudeLimits = claudeLimits || (r.Tool == "claude" && len(r.Windows) > 0)
		if r.Today >= 0 {
			rows = append(rows, text(i18n.Tf("%s today %s", r.Tool, office.FmtTokens(r.Today)), modalText()))
		}
		for _, win := range r.Windows {
			style := limitOK
			switch {
			case win.UsedPercent >= 90:
				style = limitHot
			case win.UsedPercent >= 70:
				style = limitWarn
			}
			rows = append(rows, func(buf *screen.Buffer, x, y, inner int) {
				barW := usageBarW(inner)
				lx := buf.Text(x, y, pad(r.Tool+" "+win.Name, 16), modalText())
				lx = bar(buf, lx, y, barW, int64(win.UsedPercent*10), 1000, style)
				buf.Text(lx, y, i18n.Tf(" %3.0f%% · resets %s", win.UsedPercent, resetTime(win.ResetsAt)), modalText())
			})
		}
	}
	if !claudeLimits {
		// Claude tells its limits only to its status line, which is off until
		// turned on in the settings tab.
		rows = append(rows, text("", modalText()), text(i18n.T("Claude limits are off: turn them on in the settings tab."), modalDim()))
	}
	return rows
}

// bar draws a w-cell bar filled in proportion to v of most and returns the
// column after it.
func bar(buf *screen.Buffer, x, y, w int, v, most int64, fill screen.Style) int {
	n := 0
	if most > 0 {
		n = min(w, int(v*int64(w)/most))
	}
	if v > 0 && n == 0 {
		n = 1 // any use at all shows
	}
	x = buf.Text(x, y, strings.Repeat("█", n), fill)
	return buf.Text(x, y, strings.Repeat("░", w-n), modalDim())
}

// resetTime reads as a clock time today, with the weekday otherwise.
func resetTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	t = t.Local()
	if now := time.Now(); t.YearDay() == now.YearDay() && t.Year() == now.Year() {
		return t.Format("15:04")
	}
	return weekday(t.Weekday()) + " " + t.Format("15:04")
}

// weekday is a day's short name in the current language.
func weekday(d time.Weekday) string {
	return [...]string{i18n.T("Sun"), i18n.T("Mon"), i18n.T("Tue"), i18n.T("Wed"), i18n.T("Thu"), i18n.T("Fri"), i18n.T("Sat")}[d]
}

// usageBarW is how wide each of a station's two bars is in a usage row
// inner cells wide.
func usageBarW(inner int) int {
	today, session := i18n.T(" today "), i18n.T(" session ")
	return max(4, (inner-usageLabelW-2*(len([]rune(today))+usageNumW)-len([]rune(session)))/2)
}
