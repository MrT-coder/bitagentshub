// Package pet reads the office pet's mood from what the office already
// knows: the agents at work, the token limits the tools report and the
// tasks checked off on the board, each one a fish it eats. Token spend never
// feeds it.
package pet

import (
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/usage"
)

// Mood is what the pet shows; each one has its own animation.
type Mood int

const (
	Sleeping  Mood = iota // nobody works
	Busy                  // agents work
	Tired                 // a limit is at 70% or more
	Exhausted             // a limit is at 90% or more
	Hungry                // no task was checked off today
	Alert                 // an agent waits on a permission prompt
	Eating                // a task was just checked off
)

// Limits at these percents tire the pet out.
const (
	tiredAt     = 70
	exhaustedAt = 90
)

func (m Mood) String() string {
	return [...]string{"sleeping", "busy", "tired", "exhausted", "hungry", "alert", "eating"}[m]
}

// Name is the mood as the office shows it, in the current language.
func (m Mood) Name() string {
	switch m {
	case Busy:
		return i18n.T("busy")
	case Tired:
		return i18n.T("tired")
	case Exhausted:
		return i18n.T("exhausted")
	case Hungry:
		return i18n.T("hungry")
	case Alert:
		return i18n.T("alert")
	case Eating:
		return i18n.T("eating")
	}
	return i18n.T("sleeping")
}

// Telemetry is what the pet reads.
type Telemetry struct {
	Working  int            // agents working
	Blocked  int            // agents waiting on a permission prompt
	Reports  []usage.Report // what each tool says of its limits
	Board    bool           // a board is on; without one the pet never goes hungry
	FedToday int            // tasks checked off today, its fish
	Eating   bool           // a task was just checked off
}

// State is the pet's mood and the reasons behind it, one per line.
type State struct {
	Mood Mood
	Why  []string
}

// Read gives the pet's mood. The most urgent reason wins: a fish being
// eaten, then a blocked agent, the limits, hunger and work.
func Read(t Telemetry) State {
	var s State
	switch {
	case t.Blocked == 1:
		s.Why = append(s.Why, i18n.T("1 agent waits on a permission prompt"))
	case t.Blocked > 1:
		s.Why = append(s.Why, i18n.Tf("%d agents wait on a permission prompt", t.Blocked))
	}
	switch {
	case t.Working == 1:
		s.Why = append(s.Why, i18n.T("1 agent working"))
	case t.Working > 1:
		s.Why = append(s.Why, i18n.Tf("%d agents working", t.Working))
	default:
		s.Why = append(s.Why, i18n.T("no agent working"))
	}
	tool, win, used := highest(t.Reports)
	if tool == "" {
		s.Why = append(s.Why, i18n.T("no limits reported"))
	} else {
		s.Why = append(s.Why, i18n.Tf("highest limit: %s %s at %.0f%%", tool, win, used))
	}
	hungry := t.Board && t.FedToday == 0
	switch {
	case !t.Board:
		s.Why = append(s.Why, i18n.T("no board: it never goes hungry"))
	case hungry:
		s.Why = append(s.Why, i18n.T("no fish today: it is hungry"))
	case t.FedToday == 1:
		s.Why = append(s.Why, i18n.T("1 fish eaten today"))
	default:
		s.Why = append(s.Why, i18n.Tf("%d fish eaten today", t.FedToday))
	}

	switch {
	case t.Eating:
		s.Mood = Eating
	case t.Blocked > 0:
		s.Mood = Alert
	case used >= exhaustedAt:
		s.Mood = Exhausted
	case hungry:
		s.Mood = Hungry
	case used >= tiredAt:
		s.Mood = Tired
	case t.Working > 0:
		s.Mood = Busy
	default:
		s.Mood = Sleeping
	}
	return s
}

// highest finds the most used limit of every tool.
func highest(reports []usage.Report) (tool, window string, used float64) {
	for _, r := range reports {
		for _, w := range r.Windows {
			if tool == "" || w.UsedPercent > used {
				tool, window, used = r.Tool, w.Name, w.UsedPercent
			}
		}
	}
	return tool, window, used
}
