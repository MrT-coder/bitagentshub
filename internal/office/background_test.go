package office

import (
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// frameOf draws one frame into a new buffer.
func frameOf(l Layout, c *Crowd, agents []claude.Agent) string {
	buf := screen.New(l.W, l.H/2)
	Draw(buf, l, c, agents, 0, "", pet.Sleeping)
	return strings.Join(buf.Lines(), "\n")
}

// fresh draws the frame with nothing cached, as the office did before the
// background was kept.
func fresh(l Layout, c *Crowd, agents []claude.Agent) string {
	background.mu.Lock()
	background.buf = nil
	background.mu.Unlock()
	return frameOf(l, c, agents)
}

func TestTheBackgroundFollowsTheThemeAndTheLayout(t *testing.T) {
	defer SetTheme(BuiltinTheme())
	l, c, agents := posedOffice()
	before := frameOf(l, c, agents)
	night, ok := Preset("night")
	if !ok {
		t.Fatal("no night preset")
	}
	SetTheme(night)
	got := frameOf(l, c, agents)
	if got == before {
		t.Fatal("a new theme should show in the next frame")
	}
	if got != fresh(l, c, agents) {
		t.Error("the cached background should draw the night theme as a fresh frame does")
	}
	wider := Build(agents, testW+20, testH)
	c.Sync(wider, agents)
	if frameOf(wider, c, agents) != fresh(wider, c, agents) {
		t.Error("a new layout should draw its own background")
	}
}
