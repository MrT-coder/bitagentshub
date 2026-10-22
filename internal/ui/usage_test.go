package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/usage"
)

type fakeSource struct {
	r     usage.Report
	ok    bool
	reads int
}

func (f *fakeSource) Read(time.Time) (usage.Report, bool) {
	f.reads++
	return f.r, f.ok
}

// spendingOffice is the two-agent office with token spend on both agents.
func spendingOffice(t *testing.T, sources ...usage.Source) Model {
	t.Helper()
	m, _, _ := twoAgentOffice(t)
	m = m.WithUsage(sources...)
	snap := *m.snap
	snap.Agents = append([]claude.Agent(nil), snap.Agents...)
	snap.Agents[0].Today = claude.Usage{Output: 1_200_000}
	snap.Agents[0].Session = claude.Usage{Output: 3_400_000}
	snap.Agents[1].Today = claude.Usage{Output: 300_000}
	snap.Today = claude.Usage{Output: 7_500_000}
	snap.Week = claude.Usage{Output: 16_600_000}
	return update(m, snapshotMsg(snap))
}

func TestUsageWindowShowsEachAgentAndTheOfficeTotals(t *testing.T) {
	m := spendingOffice(t)
	m = press(m, key("u"))
	v := view(m)
	for _, want := range []string{"usage", "claude", "portal-front", "portal-service", "1.2M", "3.4M", "300K", "7.5M", "16.6M", "█"} {
		if !strings.Contains(v, want) {
			t.Errorf("usage window missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "5h") || strings.Contains(v, "resets") {
		t.Errorf("no tool exposes limits, so nothing about limits should show:\n%s", v)
	}
	m = press(m, key("esc"))
	if strings.Contains(view(m), "3.4M") {
		t.Error("esc should close the usage window")
	}
}

func TestUsageWindowShowsRealLimitsOnlyWhenAToolReportsThem(t *testing.T) {
	resets := time.Now().Add(2 * time.Hour).Truncate(time.Minute)
	claudeSrc := &fakeSource{ok: true, r: usage.Report{Tool: "claude", Today: -1, Windows: []usage.Window{
		{Name: "5h", UsedPercent: 34, ResetsAt: resets},
		{Name: "7d", UsedPercent: 12, ResetsAt: resets.Add(72 * time.Hour)},
	}}}
	codexSrc := &fakeSource{ok: true, r: usage.Report{Tool: "codex", Today: 1_100_000}}
	missing := &fakeSource{ok: false, r: usage.Report{Tool: "gemini", Today: 5}}
	m := spendingOffice(t, claudeSrc, codexSrc, missing)
	m = press(m, key("u"))
	v := view(m)
	for _, want := range []string{"claude 5h", "34%", "resets " + resetTime(resets), "claude 7d", "12%", "codex", "1.1M"} {
		if !strings.Contains(v, want) {
			t.Errorf("usage window missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "gemini") {
		t.Errorf("a tool that is not on this machine should not show:\n%s", v)
	}
	stale := &fakeSource{ok: true, r: usage.Report{Tool: "old", Today: -1, Windows: []usage.Window{
		{Name: "5h", UsedPercent: 99, ResetsAt: time.Now().Add(-time.Minute)},
	}}}
	m2 := press(spendingOffice(t, stale), key("u"))
	if strings.Contains(view(m2), "99%") {
		t.Errorf("a window that already reset should not show:\n%s", view(m2))
	}
	if strings.Contains(v, "codex 5h") || strings.Contains(v, "codex primary") {
		t.Errorf("codex reported no limits, so none should show:\n%s", v)
	}
}

func TestUsageWindowOpensFromAWindowAndReturnsToIt(t *testing.T) {
	m := spendingOffice(t)
	m = press(m, key("enter"))
	m = prefixed(m, "u")
	if !strings.Contains(view(m), "3.4M") {
		t.Fatalf("ctrl+a u should open the usage window:\n%s", view(m))
	}
	m = press(m, key("a"))
	if m.win == nil || len(m.terms) == 0 {
		t.Fatal("the window should stay open under the usage window")
	}
	m = prefixed(m, "u")
	if strings.Contains(view(m), "3.4M") || m.win == nil {
		t.Errorf("ctrl+a u again should close the usage window and leave the window:\n%s", view(m))
	}
}
