// Package usage holds what agent tools report about their own token use and
// limits from files on this machine. Nothing here reads credentials or asks
// a server: a tool that does not expose its limits locally shows usage only.
package usage

import "time"

// Window is one real limit a tool reports, such as Claude's 5-hour window.
type Window struct {
	Name        string // "5h", "7d", "primary"
	UsedPercent float64
	ResetsAt    time.Time
}

// Report is what one tool exposes now.
type Report struct {
	Tool    string   // "claude", "codex"
	Today   int64    // tokens since local midnight; -1 when the tool does not say
	Windows []Window // real limits; empty when the tool exposes none
}

// Source reads one tool's report; ok is false when the tool leaves nothing
// on this machine.
type Source interface {
	Read(now time.Time) (r Report, ok bool)
}

// Fresh drops windows that already reset: their percent no longer holds.
func Fresh(ws []Window, now time.Time) []Window {
	var out []Window
	for _, w := range ws {
		if w.ResetsAt.IsZero() || w.ResetsAt.After(now) {
			out = append(out, w)
		}
	}
	return out
}
