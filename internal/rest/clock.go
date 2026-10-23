// Package rest measures how long the user has supervised agents without a
// break. Nothing has to be started: the office tells the clock when the user
// gives it input and how many agents are at work, and the clock tells when a
// break is due. Time is always passed in, so the clock is plain to test.
//
// The thresholds are design heuristics, not research findings: no study has
// measured how fast supervising coding agents tires people.
package rest

import (
	"fmt"
	"time"
)

// Config is how the clock judges presence, breaks and load.
type Config struct {
	Present time.Duration // input this recent means the user is there
	Break   time.Duration // this long without input is a break
	Nudge   time.Duration // continuous supervised time that suggests a break
	Overdue time.Duration // continuous supervised time past which a break is overdue
	Daily   time.Duration // supervised time in a day past which the day is long
}

// Defaults are the clock's settings when config.toml says nothing.
func Defaults() Config {
	return Config{
		Present: 2 * time.Minute,
		Break:   5 * time.Minute,
		Nudge:   50 * time.Minute,
		Overdue: 90 * time.Minute,
		Daily:   5 * time.Hour,
	}
}

// Day is the clock's record of one day, kept in the state folder.
type Day struct {
	Date       string        `json:"date"` // YYYY-MM-DD, local time
	Supervised time.Duration `json:"supervised"`
	Breaks     int           `json:"breaks"`
	// Continuous and LastActive let a restart carry on the time since the
	// last break.
	Continuous time.Duration `json:"continuous"`
	LastActive time.Time     `json:"last_active"`
}

// State is what the clock says now.
type State struct {
	Continuous time.Duration // weighted supervised time since the last break
	Due        bool          // a break is suggested
	Overdue    bool          // a break is overdue
	OverBudget bool          // today's supervised time is past the daily budget
}

// Clock counts supervised time. Feed it Input on every key, paste or click
// and Tick about once a second.
type Clock struct {
	cfg       Config
	day       Day
	lastInput time.Time
	lastTick  time.Time
	away      bool // the current absence was already counted as a break
}

// maxStep bounds the time one tick can add: a longer gap is a suspended
// machine or a stalled office, not supervision.
const maxStep = 5 * time.Second

// New starts a clock at now from day, the record Load read. A restart after
// a break starts the continuous time over and counts that break.
func New(cfg Config, day Day, now time.Time) *Clock {
	c := &Clock{cfg: cfg, day: day, lastInput: now, lastTick: now}
	c.rollover(now)
	if c.day.Continuous > 0 && now.Sub(c.day.LastActive) >= cfg.Break {
		c.day.Continuous = 0
		c.day.Breaks++
	}
	return c
}

// Date is the day now falls on, as Day.Date writes it.
func Date(now time.Time) string { return now.Format("2006-01-02") }

func (c *Clock) rollover(now time.Time) {
	if d := Date(now); c.day.Date != d {
		c.day = Day{Date: d}
	}
}

// Input records that the user did something in the office.
func (c *Clock) Input(now time.Time) {
	c.lastInput = now
	c.away = false
}

// Tick moves the clock to now with active agents working or waiting on the
// user.
func (c *Clock) Tick(now time.Time, active int) {
	step := now.Sub(c.lastTick)
	c.lastTick = now
	c.rollover(now)
	if step <= 0 {
		return
	}
	idle := now.Sub(c.lastInput)
	switch {
	case idle >= c.cfg.Break:
		if !c.away {
			c.away = true
			if c.day.Continuous > 0 {
				c.day.Breaks++
			}
			c.day.Continuous = 0
		}
	case idle < c.cfg.Present && step <= maxStep:
		c.day.Supervised += step
		c.day.Continuous += time.Duration(float64(step) * weight(active))
		c.day.LastActive = now
	}
}

// weight is how much a second of supervision counts: watching more agents
// at once tires more, up to twice as much.
func weight(active int) float64 {
	return min(2, 1+0.25*float64(max(0, active-1)))
}

// State reports the clock now.
func (c *Clock) State() State {
	cont := c.day.Continuous
	return State{
		Continuous: cont,
		Due:        cont >= c.cfg.Nudge,
		Overdue:    cont >= c.cfg.Overdue,
		OverBudget: c.day.Supervised >= c.cfg.Daily,
	}
}

// Day is today's record, to save.
func (c *Clock) Day() Day { return c.day }

// Short writes d in minutes, as "52m" or "1h35m".
func Short(d time.Duration) string {
	m := int(d / time.Minute)
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", m/60, m%60)
}
