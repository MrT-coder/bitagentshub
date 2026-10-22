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

// drawUsage paints the hub's usage tab: a row per station with bars for
// today's and the session's tokens, the office totals, then what each tool
// reports, its real limits only when it has them.
func (m Model) drawUsage(buf *screen.Buffer) {
	fw, fh := buf.W, buf.H
	agents := m.agents()
	byID := map[string]int{}
	for i, a := range agents {
		byID[a.ID] = i
	}
	var rows []int // indexes into agents, in station order
	for _, id := range office.Order(m.layout) {
		if i, ok := byID[id]; ok {
			rows = append(rows, i)
		}
	}
	lines := len(rows) + 4
	for _, r := range m.reports {
		if r.Today >= 0 {
			lines++
		}
		lines += len(r.Windows)
	}
	claudeLimits := false
	for _, r := range m.reports {
		claudeLimits = claudeLimits || (r.Tool == "claude" && len(r.Windows) > 0)
	}
	if !claudeLimits {
		lines += 2
	}
	w := min(fw-4, 100)
	h := min(fh-2, lines+4)
	x, y := (fw-w)/2, (fh-h)/2
	m.hubBox(buf, x, y, w, h, i18n.T(" ←→ tab · esc close "))
	inner := w - 4
	cx, cy := x+2, y+2
	bottom := y + h - 2

	const labelW = 28
	numW := 6
	today, session := i18n.T(" today "), i18n.T(" session ")
	barW := max(4, (inner-labelW-2*(len([]rune(today))+numW)-len([]rune(session)))/2)
	var maxToday, maxSession int64
	for _, i := range rows {
		maxToday = max(maxToday, agents[i].Today.Counted())
		maxSession = max(maxSession, agents[i].Session.Counted())
	}
	for n, i := range rows {
		if cy >= bottom {
			return
		}
		a := agents[i]
		label := m.kindOf(a.ID) + " " + a.Project
		if n < 9 {
			label = fmt.Sprintf("%d %s", n+1, label)
		}
		lx := buf.Text(cx, cy, pad(truncate(label, labelW), labelW), screen.Style{Fg: office.AgentColor(a.ID), HasFg: true, Bold: true})
		lx = buf.Text(lx, cy, today, modalDim)
		lx = bar(buf, lx, cy, barW, a.Today.Counted(), maxToday, modalBorder)
		lx = buf.Text(lx, cy, " "+pad(office.FmtTokens(a.Today.Counted()), numW-1), modalText)
		lx = buf.Text(lx, cy, session, modalDim)
		lx = bar(buf, lx, cy, barW, a.Session.Counted(), maxSession, modalBorder)
		buf.Text(lx, cy, " "+office.FmtTokens(a.Session.Counted()), modalText)
		cy++
	}
	if s := m.snap; s != nil && cy < bottom {
		cy++
		buf.Text(cx, cy, i18n.Tf("office today %s · week %s", office.FmtTokens(s.Today.Counted()), office.FmtTokens(s.Week.Counted())), modalText)
		cy++
	}
	for _, r := range m.reports {
		if r.Today >= 0 && cy < bottom {
			buf.Text(cx, cy, i18n.Tf("%s today %s", r.Tool, office.FmtTokens(r.Today)), modalText)
			cy++
		}
		for _, win := range r.Windows {
			if cy >= bottom {
				return
			}
			style := limitOK
			switch {
			case win.UsedPercent >= 90:
				style = limitHot
			case win.UsedPercent >= 70:
				style = limitWarn
			}
			lx := buf.Text(cx, cy, pad(r.Tool+" "+win.Name, 16), modalText)
			lx = bar(buf, lx, cy, barW, int64(win.UsedPercent*10), 1000, style)
			buf.Text(lx, cy, i18n.Tf(" %3.0f%% · resets %s", win.UsedPercent, resetTime(win.ResetsAt)), modalText)
			cy++
		}
	}
	if !claudeLimits && cy+1 < bottom {
		// Claude tells its limits only to its status line, which is off until
		// turned on in the settings tab.
		buf.Text(cx, cy+1, truncate(i18n.T("Claude limits are off: turn them on in the settings tab."), inner), modalDim)
	}
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
	return buf.Text(x, y, strings.Repeat("░", w-n), modalDim)
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
