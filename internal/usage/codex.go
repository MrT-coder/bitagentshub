package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// The Codex CLI writes one rollout per session under
// sessions/YYYY/MM/DD (local date the session started), one JSON line per
// item: {"timestamp": "...Z", "type": "event_msg", "payload": {"type":
// "token_count", "info": {...}, "rate_limits": {...}}}. Format confirmed in
// https://github.com/openai/codex/blob/main/codex-rs/protocol/src/protocol.rs
// (TokenCountEvent, TokenUsageInfo, TokenUsage, RateLimitSnapshot,
// RateLimitWindow) and
// https://github.com/openai/codex/blob/main/codex-rs/rollout/src/recorder.rs
// (RolloutLineRef, precompute_new_rollout_path). Rollouts older than a week
// may be recompressed to .jsonl.zst; those never hold today's events.

// CodexHome is where the Codex CLI keeps its files: CODEX_HOME, else ~/.codex.
func CodexHome() string {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// NewCodex reads the session rollouts under home.
func NewCodex(home string) Source { return codex{home} }

type codex struct{ home string }

type codexLine struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type string `json:"type"`
		Info *struct {
			Total codexTokens `json:"total_token_usage"`
			Last  codexTokens `json:"last_token_usage"`
		} `json:"info"`
		RateLimits *struct {
			Primary   *codexWindow `json:"primary"`
			Secondary *codexWindow `json:"secondary"`
		} `json:"rate_limits"`
	} `json:"payload"`
}

// Counts are floats so an unexpected decimal does not drop the whole line.
type codexTokens struct {
	Input  float64 `json:"input_tokens"`
	Cached float64 `json:"cached_input_tokens"`
	Output float64 `json:"output_tokens"`
	Total  float64 `json:"total_tokens"`
}

// counted matches the Claude side: cache reads are left out. Codex's
// input_tokens includes the cached part, and output_tokens already includes
// reasoning_output_tokens (Codex prints reasoning as a detail of output).
func (t codexTokens) counted() int64 {
	return int64(max(t.Input-t.Cached, 0) + max(t.Output, 0))
}

type codexWindow struct {
	UsedPercent float64  `json:"used_percent"`
	Minutes     *int64   `json:"window_minutes"`
	ResetsAt    *int64   `json:"resets_at"`
	ResetsIn    *float64 `json:"resets_in_seconds"` // older Codex builds
}

func (w codexWindow) window(slot string, at time.Time) Window {
	out := Window{Name: windowName(w.Minutes, slot), UsedPercent: w.UsedPercent}
	switch {
	case w.ResetsAt != nil:
		out.ResetsAt = time.Unix(*w.ResetsAt, 0)
	case w.ResetsIn != nil:
		out.ResetsAt = at.Add(time.Duration(*w.ResetsIn * float64(time.Second)))
	}
	return out
}

// windowName names a window by its length, as Codex itself shows it.
func windowName(minutes *int64, slot string) string {
	switch {
	case minutes == nil || *minutes <= 0:
		return slot
	case *minutes%(24*60) == 0:
		return strconv.FormatInt(*minutes/(24*60), 10) + "d"
	case *minutes%60 == 0:
		return strconv.FormatInt(*minutes/60, 10) + "h"
	}
	return slot
}

func (c codex) Read(now time.Time) (Report, bool) {
	sessions := filepath.Join(c.home, "sessions")
	if fi, err := os.Stat(sessions); err != nil || !fi.IsDir() {
		return Report{}, false
	}
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	r := Report{Tool: "codex"}
	var latest time.Time
	// A session lives in the folder of the day it started, so today's events
	// sit in today's folder or, past midnight, in yesterday's.
	for _, day := range []time.Time{midnight.AddDate(0, 0, -1), midnight} {
		files, _ := filepath.Glob(filepath.Join(sessions, day.Format("2006"), day.Format("01"), day.Format("02"), "*.jsonl"))
		for _, f := range files {
			c.readFile(f, midnight, &r, &latest)
		}
	}
	return r, true
}

// readFile adds the file's tokens since midnight to r.Today and keeps the
// newest rate limits seen so far in r.Windows.
func (c codex) readFile(path string, midnight time.Time, r *Report, latest *time.Time) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	// Lines can carry whole tool outputs, so they are read with no length
	// cap, one at a time, never the whole file.
	br := bufio.NewReaderSize(f, 64<<10)
	var total float64
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			c.readLine(line, midnight, &total, r, latest)
		}
		if err != nil {
			return // io.EOF or a read error: keep what was read
		}
	}
}

func (c codex) readLine(line []byte, midnight time.Time, total *float64, r *Report, latest *time.Time) {
	var l codexLine
	if json.Unmarshal(line, &l) != nil || l.Type != "event_msg" || l.Payload.Type != "token_count" {
		return
	}
	at, err := time.Parse(time.RFC3339Nano, l.Timestamp)
	if err != nil {
		return
	}
	if info := l.Payload.Info; info != nil {
		// Codex repeats the last usage when only the rate limits refresh; the
		// running total stays put then, so the repeat is not new work.
		if info.Total.Total != *total {
			*total = info.Total.Total
			if !at.Before(midnight) {
				r.Today += info.Last.counted()
			}
		}
	}
	if rl := l.Payload.RateLimits; rl != nil && at.After(*latest) {
		*latest = at
		r.Windows = nil
		if rl.Primary != nil {
			r.Windows = append(r.Windows, rl.Primary.window("primary", at))
		}
		if rl.Secondary != nil {
			r.Windows = append(r.Windows, rl.Secondary.window("secondary", at))
		}
	}
}
