package office

import (
	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// Status is what a station tells about its agent, in herdr's four words.
// The transcript states fold into it: thinking and tool are working,
// waiting and asleep are idle, or done while nobody has looked yet.
type Status int

const (
	StatusIdle Status = iota
	StatusWorking
	StatusBlocked
	StatusDone
)

// StatusOf is the status a station shows for a.
func StatusOf(a claude.Agent) Status {
	switch {
	case a.Blocked:
		return StatusBlocked
	case a.State.Working():
		return StatusWorking
	case a.Done:
		return StatusDone
	}
	return StatusIdle
}

// Label is the icon and the word, so the status reads without color too.
func (s Status) Label() string {
	switch s {
	case StatusWorking:
		return i18n.T("● working")
	case StatusBlocked:
		return i18n.T("▲ blocked")
	case StatusDone:
		return i18n.T("✓ done")
	}
	return i18n.T("○ idle")
}

// Color is the color a status is written in, under the desks and in the
// station strip.
func (s Status) Color() screen.RGB {
	switch s {
	case StatusWorking:
		return rgb(0xe5c07b)
	case StatusBlocked:
		return rgb(0xe06c75)
	case StatusDone:
		return rgb(0x61afef)
	}
	return rgb(0x9aa0aa)
}
