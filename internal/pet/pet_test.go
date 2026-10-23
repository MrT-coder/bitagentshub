package pet

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/usage"
)

func limit(tool, name string, used float64) usage.Report {
	return usage.Report{Tool: tool, Windows: []usage.Window{{Name: name, UsedPercent: used}}}
}

func TestTheMoodFollowsTheTelemetry(t *testing.T) {
	for _, c := range []struct {
		name string
		in   Telemetry
		want Mood
	}{
		{"nobody works", Telemetry{}, Sleeping},
		{"an agent works", Telemetry{Working: 1}, Busy},
		{"a limit at 70%", Telemetry{Working: 1, Reports: []usage.Report{limit("claude", "5h", 70)}}, Tired},
		{"a limit at 90%", Telemetry{Reports: []usage.Report{limit("codex", "7d", 90)}}, Exhausted},
		{"a board and no star today", Telemetry{Working: 1, Board: true}, Hungry},
		{"a board and a star today", Telemetry{Working: 1, Board: true, FedToday: 1}, Busy},
		{"hungry and exhausted", Telemetry{Board: true, Reports: []usage.Report{limit("codex", "5h", 95)}}, Exhausted},
		{"an agent blocked", Telemetry{Blocked: 1, Board: true, Reports: []usage.Report{limit("codex", "5h", 95)}}, Alert},
		{"a star just checked", Telemetry{Eating: true, Blocked: 1, Board: true, FedToday: 1}, Eating},
		{"a break is due", Telemetry{Working: 1, RestDue: true}, Tired},
		{"a break is due and no fish", Telemetry{Working: 1, Board: true, RestDue: true}, Tired},
	} {
		if got := Read(c.in).Mood; got != c.want {
			t.Errorf("%s: mood %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTheStateSaysWhy(t *testing.T) {
	got := Read(Telemetry{
		Working: 2, Blocked: 1, Board: true, FedToday: 3,
		Reports: []usage.Report{limit("claude", "5h", 40), limit("codex", "7d", 82.4)},
	}).Why
	want := []string{
		"1 agent waits on a permission prompt",
		"2 agents working",
		"highest limit: codex 7d at 82%",
		"3 fish eaten today",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("why = %q\nwant %q", got, want)
	}
}

func TestWithoutBoardOrLimitsTheStateSaysSo(t *testing.T) {
	got := Read(Telemetry{}).Why
	want := []string{
		"no agent working",
		"no limits reported",
		"no board: it never goes hungry",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("why = %q\nwant %q", got, want)
	}
	if got := Read(Telemetry{Board: true, Working: 1}).Why[2]; got != "no fish today: it is hungry" {
		t.Errorf("hungry line = %q", got)
	}
}

func TestMoodsHaveNames(t *testing.T) {
	for m, want := range map[Mood]string{Sleeping: "sleeping", Busy: "busy", Tired: "tired", Exhausted: "exhausted", Hungry: "hungry", Alert: "alert", Eating: "eating"} {
		if m.String() != want {
			t.Errorf("%d: %q, want %q", m, m.String(), want)
		}
	}
}

func TestTheStateSaysHowLongSinceTheLastBreak(t *testing.T) {
	why := Read(Telemetry{Working: 1, Supervised: 52 * time.Minute, RestDue: true}).Why
	if !slices.Contains(why, "52m supervised without a break") {
		t.Errorf("why = %q, want the time since the last break", why)
	}
	if why := Read(Telemetry{Working: 1, Supervised: 10 * time.Minute}).Why; slices.Contains(why, "10m supervised without a break") {
		t.Errorf("before a break is due the pet says nothing of it: %q", why)
	}
}
