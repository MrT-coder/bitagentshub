package config

import (
	"strings"
	"testing"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/rest"
)

func TestTheRestSectionDefaultsToTheClockOn(t *testing.T) {
	cfg, err := Load(write(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Rest.Enabled || !cfg.Rest.Friction || cfg.Rest.Clock != rest.Defaults() {
		t.Errorf("rest = %+v, want on, with friction and the default clock", cfg.Rest)
	}
}

func TestTheRestSectionSetsTheClock(t *testing.T) {
	cfg, err := Load(write(t, `[rest]
enabled  = false
friction = false
nudge    = "40m"
overdue  = "1h"
break    = "10m"
daily    = "4h"
`))
	if err != nil {
		t.Fatal(err)
	}
	want := rest.Defaults()
	want.Nudge, want.Overdue, want.Break, want.Daily = 40*time.Minute, time.Hour, 10*time.Minute, 4*time.Hour
	if cfg.Rest.Enabled || cfg.Rest.Friction || cfg.Rest.Clock != want {
		t.Errorf("rest = %+v, want off, no friction and %+v", cfg.Rest, want)
	}
}

func TestTheRestSectionRefusesWhatItCannotUse(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"[rest]\nnudge = \"soon\"\n", `rest.nudge: "soon" is not a duration such as "50m"`},
		{"[rest]\nbreak = \"0s\"\n", `rest.break: "0s" is not a duration such as "50m"`},
		{"[rest]\nnudge = 50\n", `rest.nudge: 50 is not a duration such as "50m"`},
		{"[rest]\nenabled = \"yes\"\n", "rest.enabled: yes is not true or false"},
		{"[rest]\nnudge = \"2h\"\n", "rest.overdue: 1h30m0s must come after rest.nudge (2h0m0s)"},
		{"[rest]\nsnooze = \"5m\"\n", "rest.snooze: unknown field"},
	} {
		_, err := Load(write(t, tc.body))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: err = %v, want %q", tc.body, err, tc.want)
		}
	}
}
