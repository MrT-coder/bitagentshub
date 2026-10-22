package office

import (
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/claude"
)

func TestStatusOfFoldsTheTranscriptStatesIntoHerdrsFour(t *testing.T) {
	blocked := agent("b", "g", "p", claude.StateTool)
	blocked.Blocked = true
	done := agent("d", "g", "p", claude.StateWaiting)
	done.Done = true
	cases := []struct {
		a    claude.Agent
		want string
	}{
		{agent("t", "g", "p", claude.StateThinking), "● working"},
		{agent("o", "g", "p", claude.StateTool), "● working"},
		{blocked, "▲ blocked"},
		{done, "✓ done"},
		{agent("w", "g", "p", claude.StateWaiting), "○ idle"},
		{agent("s", "g", "p", claude.StateSleeping), "○ idle"},
	}
	for _, c := range cases {
		if got := StatusOf(c.a).Label(); got != c.want {
			t.Errorf("%s (state %v): got %q, want %q", c.a.ID, c.a.State, got, c.want)
		}
	}
}

func TestEveryStatusHasItsOwnColor(t *testing.T) {
	seen := map[any]Status{}
	for _, s := range []Status{StatusIdle, StatusWorking, StatusBlocked, StatusDone} {
		if prev, ok := seen[s.Color()]; ok {
			t.Errorf("%q and %q share a color", prev.Label(), s.Label())
		}
		seen[s.Color()] = s
	}
}

func TestDeskLabelsShowTheStatusIcon(t *testing.T) {
	blocked := agent("2", "g", "b", claude.StateTool)
	blocked.Blocked = true
	agents := []claude.Agent{agent("1", "g", "a", claude.StateSleeping), blocked}
	l, c := settled(t, agents)
	out := draw(l, c, agents, "")
	for _, want := range []string{"○ idle", "▲ blocked"} {
		if !strings.Contains(out, want) {
			t.Errorf("draw missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "asleep") {
		t.Errorf("asleep is shown as idle now:\n%s", out)
	}
}
