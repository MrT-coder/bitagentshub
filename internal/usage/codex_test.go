package usage

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// zone is fixed so "today" does not depend on the machine running the tests.
var zone = time.FixedZone("test", -5*3600)

// codexNow is 15:00 local on 2026-10-20, 20:00 UTC.
var codexNow = time.Date(2026, 10, 20, 15, 0, 0, 0, zone)

// writeRollout writes a session file under home/sessions/YYYY/MM/DD.
func writeRollout(t *testing.T, home string, day time.Time, name string, lines ...string) {
	t.Helper()
	dir := filepath.Join(home, "sessions", day.Format("2006"), day.Format("01"), day.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tokenLine is a token_count event as the Codex rollout recorder writes it.
func tokenLine(at time.Time, total, input, cached, output int64, limits string) string {
	if limits == "" {
		limits = "null"
	}
	return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count",`+
		`"info":{"total_token_usage":{"input_tokens":0,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0,"total_tokens":%d},`+
		`"last_token_usage":{"input_tokens":%d,"cached_input_tokens":%d,"output_tokens":%d,"reasoning_output_tokens":5,"total_tokens":%d},`+
		`"model_context_window":272000},"rate_limits":%s}}`,
		at.UTC().Format("2006-01-02T15:04:05.000Z"), total, input, cached, output, input+output, limits)
}

func limitsJSON(primaryPct float64, primaryReset int64, secondaryPct float64, secondaryReset int64) string {
	return fmt.Sprintf(`{"limit_id":"codex","limit_name":null,`+
		`"primary":{"used_percent":%g,"window_minutes":300,"resets_at":%d},`+
		`"secondary":{"used_percent":%g,"window_minutes":10080,"resets_at":%d},`+
		`"credits":null,"plan_type":"plus"}`, primaryPct, primaryReset, secondaryPct, secondaryReset)
}

const sessionMeta = `{"timestamp":"2026-10-20T13:00:00.000Z","type":"session_meta","payload":{"id":"5973b6c0","cwd":"/work"}}`

func TestCodexSumsTodayAcrossFilesWithoutCachedInputOrYesterday(t *testing.T) {
	home := t.TempDir()
	today := codexNow
	yesterday := codexNow.AddDate(0, 0, -1)
	midnight := time.Date(2026, 10, 20, 0, 0, 0, 0, zone)

	writeRollout(t, home, today, "rollout-a.jsonl",
		sessionMeta,
		tokenLine(midnight.Add(9*time.Hour), 1500, 1000, 400, 200, ""),   // 600 + 200
		tokenLine(midnight.Add(10*time.Hour), 3000, 1200, 1000, 100, ""), // 200 + 100
	)
	writeRollout(t, home, today, "rollout-b.jsonl",
		tokenLine(midnight.Add(11*time.Hour), 500, 300, 0, 50, ""), // 300 + 50
	)
	// A session started yesterday that kept going after midnight.
	writeRollout(t, home, yesterday, "rollout-c.jsonl",
		tokenLine(midnight.Add(-time.Hour), 9000, 5000, 0, 4000, ""), // yesterday: not counted
		tokenLine(midnight.Add(time.Hour), 9500, 400, 100, 20, ""),   // 300 + 20
	)

	r, ok := NewCodex(home).Read(codexNow)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	want := Report{Tool: "codex", Today: 800 + 300 + 350 + 320}
	if !reflect.DeepEqual(r, want) {
		t.Fatalf("Read = %+v, want %+v", r, want)
	}
}

func TestCodexIgnoresOlderFolders(t *testing.T) {
	home := t.TempDir()
	twoDaysAgo := codexNow.AddDate(0, 0, -2)
	writeRollout(t, home, twoDaysAgo, "rollout-old.jsonl",
		tokenLine(codexNow.Add(-time.Hour), 100, 100, 0, 0, limitsJSON(50, codexNow.Add(time.Hour).Unix(), 10, codexNow.Add(48*time.Hour).Unix())),
	)

	r, ok := NewCodex(home).Read(codexNow)
	want := Report{Tool: "codex", Today: 0}
	if !ok || !reflect.DeepEqual(r, want) {
		t.Fatalf("Read = %+v, %v; want %+v, true", r, ok, want)
	}
}

func TestCodexLatestRateLimitsWin(t *testing.T) {
	home := t.TempDir()
	newReset5h := codexNow.Add(2 * time.Hour).Truncate(time.Second)
	newReset7d := codexNow.Add(72 * time.Hour).Truncate(time.Second)

	// The newest event sits in the file read first, the older ones after it.
	writeRollout(t, home, codexNow, "rollout-a.jsonl",
		tokenLine(codexNow.Add(-10*time.Minute), 10, 10, 0, 0, limitsJSON(42.5, newReset5h.Unix(), 17, newReset7d.Unix())),
	)
	writeRollout(t, home, codexNow, "rollout-b.jsonl",
		tokenLine(codexNow.Add(-2*time.Hour), 20, 20, 0, 0, limitsJSON(90, codexNow.Add(time.Hour).Unix(), 80, codexNow.Add(24*time.Hour).Unix())),
		tokenLine(codexNow.Add(-time.Hour), 30, 10, 0, 0, ""), // no rate_limits: keeps the newer snapshot
	)

	r, ok := NewCodex(home).Read(codexNow)
	want := Report{
		Tool:  "codex",
		Today: 40,
		Windows: []Window{
			{Name: "5h", UsedPercent: 42.5, ResetsAt: newReset5h},
			{Name: "7d", UsedPercent: 17, ResetsAt: newReset7d},
		},
	}
	if !ok || !reflect.DeepEqual(normalize(r), normalize(want)) {
		t.Fatalf("Read = %+v, %v; want %+v, true", r, ok, want)
	}
}

func TestCodexWindowNamesAndRelativeReset(t *testing.T) {
	home := t.TempDir()
	at := codexNow.Add(-30 * time.Minute)
	// Older Codex builds wrote resets_in_seconds instead of resets_at, and a
	// window without a length falls back to its slot name.
	limits := `{"primary":{"used_percent":12,"window_minutes":60,"resets_in_seconds":600},` +
		`"secondary":{"used_percent":3,"window_minutes":null,"resets_at":null}}`
	writeRollout(t, home, codexNow, "rollout-a.jsonl", tokenLine(at, 1, 1, 0, 0, limits))

	r, ok := NewCodex(home).Read(codexNow)
	want := Report{
		Tool:  "codex",
		Today: 1,
		Windows: []Window{
			{Name: "1h", UsedPercent: 12, ResetsAt: at.Truncate(time.Millisecond).Add(600 * time.Second)},
			{Name: "secondary", UsedPercent: 3},
		},
	}
	if !ok || !reflect.DeepEqual(normalize(r), normalize(want)) {
		t.Fatalf("Read = %+v, %v; want %+v, true", r, ok, want)
	}
}

func TestCodexSkipsRepeatedTokenCount(t *testing.T) {
	home := t.TempDir()
	// Codex repeats the last usage when only the rate limits refresh; the
	// running total does not move, so the repeat must not count twice.
	writeRollout(t, home, codexNow, "rollout-a.jsonl",
		tokenLine(codexNow.Add(-3*time.Hour), 700, 500, 0, 200, ""),
		tokenLine(codexNow.Add(-2*time.Hour), 700, 500, 0, 200, limitsJSON(5, codexNow.Add(time.Hour).Unix(), 1, codexNow.Add(time.Hour).Unix())),
	)

	r, _ := NewCodex(home).Read(codexNow)
	if r.Today != 700 {
		t.Fatalf("Today = %d, want 700", r.Today)
	}
}

func TestCodexSkipsMalformedAndLongLines(t *testing.T) {
	home := t.TempDir()
	long := `{"timestamp":"2026-10-20T14:00:00.000Z","type":"response_item","payload":{"type":"message","content":"` +
		strings.Repeat("x", 3<<20) + `"}}`
	writeRollout(t, home, codexNow, "rollout-a.jsonl",
		`{not json`,
		long,
		`{"timestamp":"yesterday-ish","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":999}}}}`,
		`{"timestamp":"2026-10-20T15:00:00.000Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":null}}`,
		`{"timestamp":"2026-10-20T15:00:00.000Z","type":"event_msg","payload":{"type":"agent_message","message":"hi"}}`,
		tokenLine(codexNow.Add(-time.Hour), 70, 50, 0, 20, ""),
	)
	writeRollout(t, home, codexNow, "notes.txt", tokenLine(codexNow.Add(-time.Hour), 1, 1000, 0, 0, ""))

	r, ok := NewCodex(home).Read(codexNow)
	want := Report{Tool: "codex", Today: 70}
	if !ok || !reflect.DeepEqual(r, want) {
		t.Fatalf("Read = %+v, %v; want %+v, true", r, ok, want)
	}
}

func TestCodexWithoutSessionsFolder(t *testing.T) {
	r, ok := NewCodex(t.TempDir()).Read(codexNow)
	if ok {
		t.Fatalf("ok = true with %+v, want false", r)
	}
}

func TestCodexWithEmptySessionsFolder(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, ok := NewCodex(home).Read(codexNow)
	want := Report{Tool: "codex", Today: 0}
	if !ok || !reflect.DeepEqual(r, want) {
		t.Fatalf("Read = %+v, %v; want %+v, true", r, ok, want)
	}
}

func TestCodexHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	if got := CodexHome(); got != dir {
		t.Fatalf("CodexHome() = %q, want %q", got, dir)
	}

	t.Setenv("CODEX_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory:", err)
	}
	if got, want := CodexHome(), filepath.Join(home, ".codex"); got != want {
		t.Fatalf("CodexHome() = %q, want %q", got, want)
	}
}

// normalize drops monotonic and location details so DeepEqual compares instants.
func normalize(r Report) Report {
	for i, w := range r.Windows {
		if !w.ResetsAt.IsZero() {
			r.Windows[i].ResetsAt = w.ResetsAt.UTC()
		}
	}
	return r
}
