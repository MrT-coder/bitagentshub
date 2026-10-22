package usage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/usage"
)

func TestClaudeReadsTheLimitsTheStatusLineSaved(t *testing.T) {
	dir := t.TempDir()
	saved := `{"observed":"2026-10-20T10:00:00Z","five_hour":{"used_percentage":42.5,"resets_at":1792504800},"seven_day":{"used_percentage":12,"resets_at":1792936800}}`
	if err := os.WriteFile(filepath.Join(dir, usage.ClaudeLimitsFile), []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}

	r, ok := usage.NewClaude(dir).Read(time.Now())
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if r.Tool != "claude" || r.Today != -1 {
		t.Errorf("Tool, Today = %q, %d; want claude, -1", r.Tool, r.Today)
	}
	want := []usage.Window{
		{Name: "5h", UsedPercent: 42.5, ResetsAt: time.Unix(1792504800, 0)},
		{Name: "7d", UsedPercent: 12, ResetsAt: time.Unix(1792936800, 0)},
	}
	if len(r.Windows) != len(want) {
		t.Fatalf("Windows = %+v, want %+v", r.Windows, want)
	}
	for i, w := range want {
		got := r.Windows[i]
		if got.Name != w.Name || got.UsedPercent != w.UsedPercent || !got.ResetsAt.Equal(w.ResetsAt) {
			t.Errorf("Windows[%d] = %+v, want %+v", i, got, w)
		}
	}
}

func TestClaudeShowsOnlyTheWindowsClaudeSent(t *testing.T) {
	dir := t.TempDir()
	saved := `{"observed":"2026-10-20T10:00:00Z","seven_day":{"used_percentage":30,"resets_at":1792936800}}`
	if err := os.WriteFile(filepath.Join(dir, usage.ClaudeLimitsFile), []byte(saved), 0o600); err != nil {
		t.Fatal(err)
	}
	r, ok := usage.NewClaude(dir).Read(time.Now())
	if !ok || len(r.Windows) != 1 || r.Windows[0].Name != "7d" {
		t.Errorf("Read = %+v, %v; want only the 7d window", r, ok)
	}
}

func TestClaudeWithoutSavedLimitsHasNothingToShow(t *testing.T) {
	dir := t.TempDir()
	if _, ok := usage.NewClaude(dir).Read(time.Now()); ok {
		t.Error("missing file: ok = true, want false")
	}
	if err := os.WriteFile(filepath.Join(dir, usage.ClaudeLimitsFile), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := usage.NewClaude(dir).Read(time.Now()); ok {
		t.Error("unreadable file: ok = true, want false")
	}
}
