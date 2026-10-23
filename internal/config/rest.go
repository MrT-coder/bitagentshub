package config

import (
	"fmt"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/rest"
)

// Rest is [rest]: the supervision clock, which suggests breaks.
type Rest struct {
	Enabled  bool
	Friction bool // ask before sending a prompt once a break is overdue
	Clock    rest.Config
}

// restSection reads [rest]. Without it the clock is on, with friction and
// its default times.
func restSection(section map[string]any) (Rest, error) {
	r := Rest{Enabled: true, Friction: true, Clock: rest.Defaults()}
	durations := map[string]*time.Duration{
		"nudge": &r.Clock.Nudge, "overdue": &r.Clock.Overdue, "break": &r.Clock.Break, "daily": &r.Clock.Daily,
	}
	for key, v := range section {
		switch key {
		case "enabled", "friction":
			b, ok := v.(bool)
			if !ok {
				return Rest{}, fmt.Errorf("rest.%s: %v is not true or false", key, v)
			}
			if key == "enabled" {
				r.Enabled = b
			} else {
				r.Friction = b
			}
		case "nudge", "overdue", "break", "daily":
			s, _ := v.(string)
			d, err := time.ParseDuration(s)
			if err != nil || d <= 0 {
				if _, isString := v.(string); isString {
					return Rest{}, fmt.Errorf("rest.%s: %q is not a duration such as \"50m\"", key, s)
				}
				return Rest{}, fmt.Errorf("rest.%s: %v is not a duration such as \"50m\"", key, v)
			}
			*durations[key] = d
		default:
			return Rest{}, fmt.Errorf("rest.%s: unknown field", key)
		}
	}
	if r.Clock.Overdue <= r.Clock.Nudge {
		return Rest{}, fmt.Errorf("rest.overdue: %v must come after rest.nudge (%v)", r.Clock.Overdue, r.Clock.Nudge)
	}
	return r, nil
}
