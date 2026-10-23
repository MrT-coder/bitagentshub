package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/herdr"
	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/rest"
)

// restOffice is the two-agent office with the supervision clock on, its
// day read from a log holding continuous minutes since the last break, and
// a clock the test moves.
func restOffice(t *testing.T, continuous time.Duration, cfg config.Rest) (Model, *time.Time, string) {
	t.Helper()
	now := time.Now()
	path := filepath.Join(t.TempDir(), "rest.json")
	day := rest.Day{Date: rest.Date(now), Supervised: continuous, Continuous: continuous, LastActive: now}
	if err := rest.Save(path, day); err != nil {
		t.Fatal(err)
	}
	m, _, _ := twoAgentOffice(t)
	m.now = func() time.Time { return now }
	return m.WithRest(cfg, path), &now, path
}

func restOn() config.Rest { return config.Rest{Enabled: true, Friction: true, Clock: rest.Defaults()} }

func header(m Model) string { return strings.SplitN(view(m), "\n", 2)[0] }

// working is a snapshot with the front agent at work.
func working() snapshotMsg {
	return snapshotMsg(claude.Snapshot{Agents: []claude.Agent{
		{ID: "front", Group: "acme-company", Project: "portal-front", Cwd: frontDir, State: claude.StateTool, LastActivity: time.Now()},
		{ID: "service", Group: "acme-company", Project: "portal-service", Cwd: serviceDir, State: claude.StateWaiting, LastActivity: time.Now()},
	}})
}

func TestRest_TheHeaderShowsTheTimeSinceTheLastBreak(t *testing.T) {
	m, _, _ := restOffice(t, 12*time.Minute, restOn())
	if h := header(m); !strings.Contains(h, "⏱ 12m") {
		t.Errorf("header = %q, want the supervised minutes", h)
	}
	off := restOn()
	off.Enabled = false
	m, _, _ = restOffice(t, 12*time.Minute, off)
	if h := header(m); strings.Contains(h, "⏱") || strings.Contains(h, "☕") {
		t.Errorf("with [rest] enabled = false the header says nothing: %q", h)
	}
}

func TestRest_ABreakIsSuggestedWhenTheAgentsAreNotWorking(t *testing.T) {
	m, _, _ := restOffice(t, 52*time.Minute, restOn())
	if h := header(m); !strings.Contains(h, "☕ 52m · a good moment for a break") {
		t.Errorf("all agents waiting: header = %q", h)
	}
	m = update(m, working())
	if h := header(m); !strings.Contains(h, "☕ 52m · a break when the agents finish") {
		t.Errorf("an agent at work: header = %q", h)
	}
	m, _, _ = restOffice(t, 95*time.Minute, restOn())
	if h := header(m); !strings.Contains(h, "☕ 1h35m without a break") {
		t.Errorf("overdue: header = %q", h)
	}
}

func TestRest_TheHeaderSaysWhenTheDayIsLong(t *testing.T) {
	m, _, _ := restOffice(t, 5*time.Hour+10*time.Minute, restOn())
	if h := header(m); !strings.Contains(h, "5h10m today") {
		t.Errorf("header = %q, want the day past its budget", h)
	}
}

func TestRest_InputKeepsTheClockGoingAndSilenceIsABreak(t *testing.T) {
	m, now, _ := restOffice(t, 20*time.Minute, restOn())
	for i := range 36 { // three minutes with a key now and then: supervised
		*now = now.Add(5 * time.Second)
		if i%6 == 0 {
			m = press(m, key("right"))
		}
		m = update(m, working())
	}
	if got := m.rest.State().Continuous; got <= 20*time.Minute {
		t.Errorf("continuous = %v, want past 20m after supervised minutes", got)
	}
	for range 72 { // six minutes without input: a break, agents still at work
		*now = now.Add(5 * time.Second)
		m = update(m, working())
	}
	if d := m.rest.Day(); m.rest.State().Continuous != 0 || d.Breaks != 1 {
		t.Errorf("after 6 minutes away: continuous %v breaks %d, want 0 and 1", m.rest.State().Continuous, d.Breaks)
	}
}

func TestRest_TheDayIsSavedForTheNextStart(t *testing.T) {
	m, now, path := restOffice(t, 20*time.Minute, restOn())
	*now = now.Add(time.Second)
	m = press(m, key("right"))
	if cmd := m.saveRest(); cmd != nil {
		cmd()
	}
	d, err := rest.Load(path, *now)
	if err != nil || d.Continuous < 20*time.Minute || d.Supervised < 20*time.Minute {
		t.Errorf("saved %+v (%v), want the day carried on", d, err)
	}
}

func TestRest_ThePetTiresWhenABreakIsDue(t *testing.T) {
	m, _, _ := restOffice(t, 52*time.Minute, restOn())
	if s := m.petState(); s.Mood != pet.Tired || !strings.Contains(strings.Join(s.Why, "\n"), "52m supervised without a break") {
		t.Errorf("pet = %v %q, want tired and why", s.Mood, s.Why)
	}
	m = press(m, key("p")) // the hub opens on the pet tab
	if v := view(m); !strings.Contains(v, "52m supervised today · 0 breaks") {
		t.Errorf("the pet tab should show the day:\n%s", v)
	}
}

func TestRest_AnOverdueBreakAsksBeforeAPromptIsSent(t *testing.T) {
	m, _, _ := restOffice(t, 95*time.Minute, restOn())
	rec := &termRecorder{}
	m.startTerm = rec.start
	m = press(m, key("enter")) // open the front agent
	agent := rec.last()
	m = typeText(m, "go")
	m = press(m, key("enter"))
	if got := strings.Join(agent.keys, ","); got != "g,o" {
		t.Fatalf("the first enter should wait, the agent got %q", got)
	}
	if v := view(m); !strings.Contains(v, "☕ 1h35m without a break · enter again sends it") {
		t.Errorf("the window should say why it waits:\n%s", v)
	}
	m = press(m, key("enter"))
	if got := strings.Join(agent.keys, ","); got != "g,o,enter" {
		t.Errorf("the second enter sends the prompt, the agent got %q", got)
	}
	m = press(m, key("enter"))
	if got := strings.Join(agent.keys, ","); got != "g,o,enter" {
		t.Errorf("the next prompt asks again, the agent got %q", got)
	}
}

func TestRest_FrictionNeverTouchesShellsNorWorksWithoutOverdueOrWhenOff(t *testing.T) {
	cases := map[string]struct {
		continuous time.Duration
		cfg        func() config.Rest
	}{
		"not overdue":  {52 * time.Minute, restOn},
		"friction off": {95 * time.Minute, func() config.Rest { c := restOn(); c.Friction = false; return c }},
		"clock off":    {95 * time.Minute, func() config.Rest { c := restOn(); c.Enabled = false; return c }},
	}
	for name, tc := range cases {
		m, _, _ := restOffice(t, tc.continuous, tc.cfg())
		rec := &termRecorder{}
		m.startTerm = rec.start
		m = press(m, key("enter"))
		agent := rec.last()
		m = press(m, key("enter"))
		if got := strings.Join(agent.keys, ","); got != "enter" {
			t.Errorf("%s: the agent got %q, want enter at once", name, got)
		}
	}
	m, _, _ := restOffice(t, 95*time.Minute, restOn())
	rec := &termRecorder{}
	m.startTerm = rec.start
	m = press(press(m, key("enter")), key("f3")) // a shell beside the agent
	shell := rec.last()
	m = press(m, key("enter"))
	if got := strings.Join(shell.keys, ","); got != "enter" {
		t.Errorf("a shell is never asked, it got %q", got)
	}
}

func TestRest_AnAgentWaitingOnAPermissionIsAnsweredAtOnce(t *testing.T) {
	m, _, _ := restOffice(t, 95*time.Minute, restOn())
	m = update(m, herdrMsg{panes: []herdr.Pane{
		{PaneID: "w1:p1", TerminalID: "term_1", Agent: "claude", SessionID: "front", Cwd: frontDir, Status: "blocked"},
		{PaneID: "w2:p1", TerminalID: "term_2", Agent: "claude", SessionID: "service", Cwd: serviceDir},
	}})
	rec := &termRecorder{}
	m.startTerm = rec.start
	m = press(m, key("enter"))
	agent := rec.last()
	m = press(m, key("enter"))
	if got := strings.Join(agent.keys, ","); got != "enter" {
		t.Errorf("a blocked agent gets its answer at once, it got %q", got)
	}
}
