package rest

import (
	"path/filepath"
	"testing"
	"time"
)

var start = time.Date(2026, 10, 23, 9, 0, 0, 0, time.Local)

// run ticks c once a second for d, with active agents, typing every
// inputEvery (0: no input).
func run(c *Clock, from time.Time, d time.Duration, active int, inputEvery time.Duration) time.Time {
	now := from
	for t := time.Duration(0); t < d; t += time.Second {
		now = now.Add(time.Second)
		if inputEvery > 0 && t%inputEvery == 0 {
			c.Input(now)
		}
		c.Tick(now, active)
	}
	return now
}

func TestClock_CountsTimeWhileTheUserIsPresent(t *testing.T) {
	c := New(Defaults(), Day{}, start)
	run(c, start, 10*time.Minute, 1, 30*time.Second)
	if got := c.State().Continuous; got < 9*time.Minute || got > 10*time.Minute {
		t.Errorf("continuous = %v, want about 10m", got)
	}
	if got := c.Day().Supervised; got < 9*time.Minute || got > 10*time.Minute {
		t.Errorf("today = %v, want about 10m", got)
	}
}

func TestClock_PausesWithoutInputAndTakesABreakAfterTheBreakLength(t *testing.T) {
	c := New(Defaults(), Day{}, start)
	now := run(c, start, 20*time.Minute, 1, 30*time.Second)
	before := c.State().Continuous
	now = run(c, now, 3*time.Minute, 1, 0) // away, agents still working
	if got := c.State().Continuous; got-before > 2*time.Minute+time.Second {
		t.Errorf("continuous grew %v while away, want at most the 2m presence window", got-before)
	}
	if c.Day().Breaks != 0 {
		t.Fatal("3 minutes away is not a break yet")
	}
	run(c, now, 3*time.Minute, 1, 0)
	if c.State().Continuous != 0 || c.Day().Breaks != 1 {
		t.Errorf("after 6 minutes away: continuous %v breaks %d, want 0 and 1", c.State().Continuous, c.Day().Breaks)
	}
	if got := c.Day().Supervised; got < 21*time.Minute {
		t.Errorf("a break keeps the day's time, got %v", got)
	}
}

func TestClock_WeighsTheTimeByTheAgentsAtWork(t *testing.T) {
	for _, tc := range []struct {
		active int
		want   time.Duration
	}{{0, 10 * time.Minute}, {1, 10 * time.Minute}, {3, 15 * time.Minute}, {9, 20 * time.Minute}} {
		c := New(Defaults(), Day{}, start)
		run(c, start, 10*time.Minute, tc.active, 30*time.Second)
		if got := c.State().Continuous; got < tc.want-time.Minute || got > tc.want {
			t.Errorf("%d agents: continuous %v, want about %v", tc.active, got, tc.want)
		}
		if got := c.Day().Supervised; got > 10*time.Minute {
			t.Errorf("%d agents: today %v, want real time only", tc.active, got)
		}
	}
}

func TestClock_DueOverdueAndOverBudget(t *testing.T) {
	cfg := Defaults()
	c := New(cfg, Day{}, start)
	now := run(c, start, cfg.Nudge-time.Minute, 1, 30*time.Second)
	if s := c.State(); s.Due || s.Overdue {
		t.Fatalf("before the nudge: %+v", s)
	}
	now = run(c, now, 2*time.Minute, 1, 30*time.Second)
	if s := c.State(); !s.Due || s.Overdue {
		t.Fatalf("past the nudge: %+v", s)
	}
	run(c, now, cfg.Overdue-cfg.Nudge, 1, 30*time.Second)
	if s := c.State(); !s.Overdue {
		t.Fatalf("past overdue: %+v", s)
	}
	c = New(cfg, Day{Date: Date(start), Supervised: cfg.Daily}, start)
	c.Tick(start.Add(time.Second), 0)
	if !c.State().OverBudget {
		t.Error("a day past the budget should say so")
	}
}

func TestClock_StartsANewDayAtMidnight(t *testing.T) {
	late := time.Date(2026, 10, 23, 23, 59, 0, 0, time.Local)
	c := New(Defaults(), Day{Date: Date(late), Supervised: time.Hour, Breaks: 2}, late)
	run(c, late, 2*time.Minute, 1, 30*time.Second)
	if d := c.Day(); d.Date != "2026-10-24" || d.Supervised > time.Minute+time.Second || d.Breaks != 0 {
		t.Errorf("after midnight: %+v, want a fresh day", d)
	}
}

func TestClock_ASleepingMachineIsAnAbsence(t *testing.T) {
	c := New(Defaults(), Day{}, start)
	now := run(c, start, 5*time.Minute, 1, 30*time.Second)
	c.Tick(now.Add(2*time.Hour), 1) // the laptop slept
	if c.Day().Supervised > 6*time.Minute || c.State().Continuous != 0 {
		t.Errorf("a long gap is not supervised time: %+v %+v", c.Day(), c.State())
	}
}

func TestDay_SurvivesARestartUnlessABreakPassed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rest.json")
	c := New(Defaults(), Day{}, start)
	now := run(c, start, 30*time.Minute, 1, 30*time.Second)
	if err := Save(path, c.Day()); err != nil {
		t.Fatal(err)
	}
	d, err := Load(path, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if c2 := New(Defaults(), d, now.Add(time.Minute)); c2.State().Continuous < 29*time.Minute {
		t.Errorf("a quick restart keeps the continuous time, got %v", c2.State().Continuous)
	}
	if c3 := New(Defaults(), d, now.Add(10*time.Minute)); c3.State().Continuous != 0 || c3.Day().Breaks != 1 {
		t.Errorf("a restart after a break starts over and counts it: %+v %+v", c3.State(), c3.Day())
	}
	if d, err := Load(filepath.Join(t.TempDir(), "none.json"), now); err != nil || d.Supervised != 0 {
		t.Errorf("no log yet: %+v %v", d, err)
	}
}
