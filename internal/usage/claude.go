package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ClaudeLimitsFile is where `bah statusline` keeps the limits Claude
// Code hands its status line. Claude writes them nowhere else on disk, so
// without the status line there is nothing to read.
const ClaudeLimitsFile = "claude-limits.json"

// ClaudeLimits is the saved file: only the rate limit windows Claude sent and
// when they were seen, nothing else from the status line input.
type ClaudeLimits struct {
	Observed time.Time     `json:"observed"`
	FiveHour *ClaudeWindow `json:"five_hour,omitempty"`
	SevenDay *ClaudeWindow `json:"seven_day,omitempty"`
}

// ClaudeWindow matches one window of the status line's rate_limits field.
type ClaudeWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       float64 `json:"resets_at"` // Unix seconds
}

type claudeSource struct{ path string }

// NewClaude reads the limits the status line saved under stateDir. Claude's
// token use comes from its transcripts, so Today is always unknown here.
func NewClaude(stateDir string) Source {
	return claudeSource{path: filepath.Join(stateDir, ClaudeLimitsFile)}
}

func (s claudeSource) Read(time.Time) (Report, bool) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return Report{}, false
	}
	var l ClaudeLimits
	if err := json.Unmarshal(data, &l); err != nil {
		return Report{}, false
	}
	r := Report{Tool: "claude", Today: -1}
	for _, w := range []struct {
		name string
		w    *ClaudeWindow
	}{{"5h", l.FiveHour}, {"7d", l.SevenDay}} {
		if w.w != nil {
			r.Windows = append(r.Windows, Window{
				Name:        w.name,
				UsedPercent: w.w.UsedPercentage,
				ResetsAt:    time.Unix(int64(w.w.ResetsAt), 0),
			})
		}
	}
	return r, true
}
