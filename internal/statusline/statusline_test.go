package statusline_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/statusline"
	"github.com/MrT-coder/bitagentshub/internal/usage"
)

// input is what Claude Code passes a status line, with more than BitAgentsHub
// may keep.
const input = `{
  "session_id": "secret-session",
  "transcript_path": "/home/me/.claude/projects/x.jsonl",
  "cwd": "/home/me/private",
  "model": {"id": "claude-opus", "display_name": "Opus"},
  "cost": {"total_cost_usd": 1.5},
  "rate_limits": {
    "five_hour": {"used_percentage": 23.5, "resets_at": 1792504800},
    "seven_day": {"used_percentage": 41, "resets_at": 1792936800}
  }
}`

func run(t *testing.T, stdin, state string) string {
	t.Helper()
	var out bytes.Buffer
	if err := statusline.Run(strings.NewReader(stdin), &out, state); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return out.String()
}

func TestRunSavesOnlyTheRateLimits(t *testing.T) {
	state := filepath.Join(t.TempDir(), "not", "yet")
	before := time.Now().Add(-time.Second)
	if out := run(t, input, state); out != "" {
		t.Errorf("stdout = %q, want nothing without a previous status line", out)
	}

	data := read(t, filepath.Join(state, usage.ClaudeLimitsFile))
	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 || keys["observed"] == nil || keys["five_hour"] == nil || keys["seven_day"] == nil {
		t.Errorf("saved keys = %s, want only observed, five_hour, seven_day", data)
	}
	for _, leak := range []string{"secret-session", "transcript_path", "private", "Opus", "cost"} {
		if strings.Contains(data, leak) {
			t.Errorf("saved file carries %q: %s", leak, data)
		}
	}
	var l usage.ClaudeLimits
	if err := json.Unmarshal([]byte(data), &l); err != nil {
		t.Fatal(err)
	}
	if l.FiveHour == nil || *l.FiveHour != (usage.ClaudeWindow{UsedPercentage: 23.5, ResetsAt: 1792504800}) {
		t.Errorf("five_hour = %+v", l.FiveHour)
	}
	if l.SevenDay == nil || *l.SevenDay != (usage.ClaudeWindow{UsedPercentage: 41, ResetsAt: 1792936800}) {
		t.Errorf("seven_day = %+v", l.SevenDay)
	}
	if l.Observed.Before(before) || l.Observed.After(time.Now().Add(time.Second)) {
		t.Errorf("observed = %v, want about now", l.Observed)
	}
	r, ok := usage.NewClaude(state).Read(time.Now())
	if !ok || len(r.Windows) != 2 {
		t.Errorf("the reader sees %+v, %v", r, ok)
	}
}

func TestRunSavesOnlyTheWindowsClaudeSent(t *testing.T) {
	state := t.TempDir()
	run(t, `{"rate_limits": {"seven_day": {"used_percentage": 5, "resets_at": 1792936800}}}`, state)
	var l usage.ClaudeLimits
	if err := json.Unmarshal([]byte(read(t, filepath.Join(state, usage.ClaudeLimitsFile))), &l); err != nil {
		t.Fatal(err)
	}
	if l.FiveHour != nil || l.SevenDay == nil {
		t.Errorf("saved %+v, want only seven_day", l)
	}
}

func TestRunWithoutRateLimitsKeepsThePreviousFile(t *testing.T) {
	state := t.TempDir()
	run(t, `{"session_id": "a"}`, state)
	if _, err := os.Stat(filepath.Join(state, usage.ClaudeLimitsFile)); !os.IsNotExist(err) {
		t.Errorf("a file was saved without rate limits: %v", err)
	}

	run(t, input, state)
	saved := read(t, filepath.Join(state, usage.ClaudeLimitsFile))
	for _, in := range []string{`{"session_id": "a"}`, `{"rate_limits": {}}`, "not json"} {
		run(t, in, state)
		if got := read(t, filepath.Join(state, usage.ClaudeLimitsFile)); got != saved {
			t.Errorf("%q replaced the saved limits with %s", in, got)
		}
	}
}

// installed installs over a settings file whose status line is previous.
func installed(t *testing.T, previous string) string {
	t.Helper()
	cmd, _ := json.Marshal(previous)
	settings, state := writeSettings(t, `{"statusLine": {"type": "command", "command": `+string(cmd)+`}}`)
	if err := statusline.Install(settings, state, ours); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestRunChainsThePreviousStatusLine(t *testing.T) {
	state := installed(t, "echo chained")
	if out := strings.TrimSpace(run(t, input, state)); out != "chained" {
		t.Errorf("stdout = %q, want the previous status line's output", out)
	}
	if _, err := os.Stat(filepath.Join(state, usage.ClaudeLimitsFile)); err != nil {
		t.Errorf("limits not saved while chaining: %v", err)
	}
}

func TestRunPassesThePreviousStatusLineTheSameInput(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		if _, err := exec.LookPath("sh"); err != nil || runtime.GOOS == "windows" {
			t.Skip("no POSIX shell to run cat")
		}
	}
	state := installed(t, "cat")
	if out := run(t, input, state); out != input {
		t.Errorf("stdout = %q, want the input echoed back", out)
	}
}

func TestRunSurvivesAFailingPreviousStatusLine(t *testing.T) {
	state := installed(t, "exit 3")
	if out := run(t, input, state); out != "" {
		t.Errorf("stdout = %q, want nothing", out)
	}
	if _, err := os.Stat(filepath.Join(state, usage.ClaudeLimitsFile)); err != nil {
		t.Errorf("limits not saved: %v", err)
	}
}
