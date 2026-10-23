package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func assistantLine(id, ts, stop string, in, out, cc, cr int64) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"cwd":"C:\\work\\demo-app","message":{"id":%q,"stop_reason":%q,"usage":{"input_tokens":%d,"output_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d}}}`+"\n",
		ts, id, stop, in, out, cc, cr)
}

func userLine(ts string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"role":"user","content":"hi"}}`+"\n", ts)
}

func writeFile(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestStore_CountsEachMessageOnceAcrossDuplicateLines(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	path := filepath.Join(root, "C--work-demo-app", "s1.jsonl")
	line := assistantLine("msg_1", "2026-10-20T14:59:00Z", "tool_use", 10, 100, 1000, 5000)
	writeFile(t, path, line+line+line, now)

	s := NewStore(root, Options{})
	snap := s.Poll(now)

	if len(snap.Agents) != 1 {
		t.Fatalf("agents = %d, want 1", len(snap.Agents))
	}
	got := snap.Agents[0]
	if got.Session.Counted() != 1110 {
		t.Errorf("session counted = %d, want 1110", got.Session.Counted())
	}
	if got.Session.CacheRead != 5000 {
		t.Errorf("cache read = %d, want 5000", got.Session.CacheRead)
	}
	if got.Project != "demo-app" {
		t.Errorf("project = %q, want demo-app", got.Project)
	}
}

// The first poll must not wait for a week of transcripts: active sessions
// show at once and older files are read a few at a time, newest first.
func TestStore_ReadsActiveTranscriptsFirstAndOlderOnesInSteps(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	writeFile(t, filepath.Join(root, "C--work-demo-app", "live.jsonl"),
		assistantLine("m_live", "2026-10-20T14:59:00Z", "tool_use", 1, 0, 0, 0), now)
	for i, age := range []time.Duration{3 * time.Hour, 2 * time.Hour, 4 * time.Hour} {
		ts := now.Add(-age)
		writeFile(t, filepath.Join(root, "C--work-demo-app", fmt.Sprintf("old%d.jsonl", i)),
			assistantLine(fmt.Sprintf("m_old%d", i), ts.Format(time.RFC3339), "end_turn", 10<<i, 0, 0, 0), ts)
	}

	s := NewStore(root, Options{ColdBytes: 1}) // one older file per poll
	snap := s.Poll(now)
	if len(snap.Agents) != 1 || snap.Agents[0].ID != "live" {
		t.Fatalf("the active session should show on the first poll: %+v", snap.Agents)
	}
	if snap.Pending != 2 || snap.Today.Counted() != 1+20 {
		t.Errorf("first poll: pending %d today %d, want 2 left and the live file plus the newest older one (21)", snap.Pending, snap.Today.Counted())
	}
	snap = s.Poll(now)
	if snap.Pending != 1 || snap.Today.Counted() != 1+20+10 {
		t.Errorf("second poll: pending %d today %d, want 1 and 31", snap.Pending, snap.Today.Counted())
	}
	snap = s.Poll(now)
	if snap.Pending != 0 || snap.Today.Counted() != 1+20+10+40 {
		t.Errorf("third poll: pending %d today %d, want 0 and 71", snap.Pending, snap.Today.Counted())
	}
}

func TestStore_IgnoresTrailingPartialLineUntilCompleted(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	path := filepath.Join(root, "p", "s1.jsonl")
	full := assistantLine("msg_1", "2026-10-20T14:59:00Z", "end_turn", 1, 1, 0, 0)
	second := assistantLine("msg_2", "2026-10-20T14:59:30Z", "end_turn", 2, 2, 0, 0)
	writeFile(t, path, full+second[:20], now)

	s := NewStore(root, Options{})
	if got := s.Poll(now).Today.Counted(); got != 2 {
		t.Fatalf("after partial write today = %d, want 2", got)
	}

	appendFile(t, path, second[20:], now.Add(time.Second))
	if got := s.Poll(now.Add(time.Second)).Today.Counted(); got != 6 {
		t.Fatalf("after completing line today = %d, want 6", got)
	}
}

func TestStore_SplitsTotalsIntoDailyAndWeeklyWindows(t *testing.T) {
	root := t.TempDir()
	loc := time.UTC
	// Tuesday 2026-10-20; week starts Monday 2026-10-19.
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, loc)
	path := filepath.Join(root, "p", "s1.jsonl")
	writeFile(t, path,
		assistantLine("today", "2026-10-20T09:00:00Z", "end_turn", 0, 100, 0, 0)+
			assistantLine("monday", "2026-10-19T09:00:00Z", "end_turn", 0, 20, 0, 0)+
			assistantLine("lastweek", "2026-10-18T09:00:00Z", "end_turn", 0, 3, 0, 0),
		now)

	s := NewStore(root, Options{Location: loc, WeekStart: time.Monday})
	snap := s.Poll(now)

	if snap.Today.Counted() != 100 {
		t.Errorf("today = %d, want 100", snap.Today.Counted())
	}
	if snap.Week.Counted() != 120 {
		t.Errorf("week = %d, want 120", snap.Week.Counted())
	}
	if snap.Agents[0].Session.Counted() != 123 {
		t.Errorf("session = %d, want 123", snap.Agents[0].Session.Counted())
	}
}

func TestStore_AttributesSubagentTokensToParentSession(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	main := filepath.Join(root, "p", "sess.jsonl")
	sub := filepath.Join(root, "p", "sess", "subagents", "agent-a1.jsonl")
	writeFile(t, main, assistantLine("m1", "2026-10-20T14:59:00Z", "tool_use", 0, 10, 0, 0), now)
	writeFile(t, sub, assistantLine("s1", "2026-10-20T14:59:50Z", "tool_use", 0, 5, 0, 0), now)

	snap := NewStore(root, Options{}).Poll(now)

	if len(snap.Agents) != 1 {
		t.Fatalf("agents = %d, want 1 (subagent folds into parent)", len(snap.Agents))
	}
	a := snap.Agents[0]
	if a.Session.Counted() != 15 {
		t.Errorf("session = %d, want 15", a.Session.Counted())
	}
	if a.ActiveSubagents != 1 {
		t.Errorf("active subagents = %d, want 1", a.ActiveSubagents)
	}
}

func TestStore_DerivesAgentStateFromLatestEvent(t *testing.T) {
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		content string
		mtime   time.Time
		want    State
	}{
		{"user prompt pending means thinking", userLine("2026-10-20T14:59:59Z"), now, StateThinking},
		{"tool_use stop means running a tool", assistantLine("a", "2026-10-20T14:59:59Z", "tool_use", 0, 1, 0, 0), now, StateTool},
		{"end_turn means waiting for the human", assistantLine("a", "2026-10-20T14:59:59Z", "end_turn", 0, 1, 0, 0), now, StateWaiting},
		{"long silence means sleeping", assistantLine("a", "2026-10-20T14:40:00Z", "tool_use", 0, 1, 0, 0), now.Add(-20 * time.Minute), StateSleeping},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "p", "s.jsonl"), tt.content, tt.mtime)
			snap := NewStore(root, Options{}).Poll(now)
			if len(snap.Agents) != 1 {
				t.Fatalf("agents = %d, want 1", len(snap.Agents))
			}
			if snap.Agents[0].State != tt.want {
				t.Errorf("state = %v, want %v", snap.Agents[0].State, tt.want)
			}
		})
	}
}

func TestStore_GroupsSessionsByParentFolderOfTheirWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	line := func(id, cwd string) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":"2026-10-20T14:59:00Z","cwd":%q,"message":{"id":%q,"stop_reason":"end_turn","usage":{"output_tokens":1}}}`+"\n", cwd, id)
	}
	writeFile(t, filepath.Join(root, "a", "s1.jsonl"), line("m1", `C:\work\acme-company\shopping-portal-front`), now)
	writeFile(t, filepath.Join(root, "b", "s2.jsonl"), line("m2", `C:\workspace\hobbylabs\bah`), now)
	writeFile(t, filepath.Join(root, "c", "s3.jsonl"), userLine("2026-10-20T14:59:00Z"), now)

	groups := map[string]string{}
	for _, a := range NewStore(root, Options{}).Poll(now).Agents {
		groups[a.ID] = a.Group
	}
	want := map[string]string{"s1": "acme-company", "s2": "hobbylabs", "s3": "other"}
	for id, g := range want {
		if groups[id] != g {
			t.Errorf("group[%s] = %q, want %q", id, groups[id], g)
		}
	}
}

// An agent that cd's into a folder of its project stays in its project's
// room: its project is the shallowest folder its lines are in.
func TestStore_KeepsTheProjectWhenTheAgentMovesIntoOneOfItsFolders(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	line := func(id, cwd string) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":"2026-10-20T14:59:00Z","cwd":%q,"message":{"id":%q,"stop_reason":"end_turn","usage":{"output_tokens":1}}}`+"\n", cwd, id)
	}
	web := `C:\workspace\JBMLLNUBE\plataforma-web`
	writeFile(t, filepath.Join(root, "a", "into.jsonl"),
		line("m1", web)+line("m2", web+`\src\main`)+line("m3", "/c/workspace/JBMLLNUBE/plataforma-web/sessions/infrastructure"), now)
	// Reading only the end of a long transcript can start in a subfolder.
	writeFile(t, filepath.Join(root, "a", "back.jsonl"), line("m4", web+`\src\main`)+line("m5", web), now)
	// A session that moves to another project follows it, as before.
	writeFile(t, filepath.Join(root, "a", "moved.jsonl"), line("m6", web)+line("m7", `C:\workspace\JBMLLNUBE\bitagentshub`), now)

	got := map[string]Agent{}
	for _, a := range NewStore(root, Options{}).Poll(now).Agents {
		got[a.ID] = a
	}
	for id, want := range map[string][3]string{
		"into":  {web, "plataforma-web", "JBMLLNUBE"},
		"back":  {web, "plataforma-web", "JBMLLNUBE"},
		"moved": {`C:\workspace\JBMLLNUBE\bitagentshub`, "bitagentshub", "JBMLLNUBE"},
	} {
		if a := got[id]; a.Cwd != want[0] || a.Project != want[1] || a.Group != want[2] {
			t.Errorf("%s: cwd, project, group = %q, %q, %q; want %q", id, a.Cwd, a.Project, a.Group, want)
		}
	}
}

func TestStore_HidesSessionsOutsideActiveWindowButKeepsTheirTokens(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)
	writeFile(t, filepath.Join(root, "p", "old.jsonl"),
		assistantLine("o", "2026-10-20T13:00:00Z", "end_turn", 0, 40, 0, 0), old)

	snap := NewStore(root, Options{}).Poll(now)

	if len(snap.Agents) != 0 {
		t.Errorf("agents = %d, want 0", len(snap.Agents))
	}
	if snap.Today.Counted() != 40 {
		t.Errorf("today = %d, want 40 (limits are account-wide)", snap.Today.Counted())
	}
}

func toolLine(id, tool string) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":"2026-10-20T14:59:59Z","message":{"id":%q,"stop_reason":"tool_use","content":[{"type":"text","text":"ok"},{"type":"tool_use","name":%q,"input":{}}],"usage":{"output_tokens":1}}}`+"\n", id, tool)
}

func TestStore_DerivesActivityFromTheLastToolUsed(t *testing.T) {
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		tool string
		want Activity
	}{
		{"Edit", ActivityTyping},
		{"Write", ActivityTyping},
		{"MultiEdit", ActivityTyping},
		{"NotebookEdit", ActivityTyping},
		{"Read", ActivityReading},
		{"Grep", ActivityReading},
		{"Glob", ActivityReading},
		{"WebFetch", ActivityReading},
		{"WebSearch", ActivityReading},
		{"Bash", ActivityRunning},
		{"PowerShell", ActivityRunning},
		{"TodoWrite", ActivityTyping},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "p", "s.jsonl"), toolLine("m1", tt.tool), now)
			a := NewStore(root, Options{}).Poll(now).Agents[0]
			if a.Tool != tt.tool || a.Activity != tt.want {
				t.Errorf("tool=%q activity=%v, want %q %v", a.Tool, a.Activity, tt.tool, tt.want)
			}
		})
	}
}

func TestStore_ActivityClearsWhenTheTurnEnds(t *testing.T) {
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "p", "s.jsonl"),
		toolLine("m1", "Bash")+assistantLine("m2", "2026-10-20T14:59:59Z", "end_turn", 0, 1, 0, 0), now)
	a := NewStore(root, Options{}).Poll(now).Agents[0]
	if a.Activity != ActivityNone || a.State != StateWaiting {
		t.Errorf("activity=%v state=%v, want none while waiting", a.Activity, a.State)
	}
}

func TestStore_ListsTheProjectsOfEverySessionItRead(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC)
	line := func(id, cwd, ts string) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"cwd":%q,"message":{"id":%q,"stop_reason":"end_turn","usage":{"output_tokens":1}}}`+"\n", ts, cwd, id)
	}
	writeFile(t, filepath.Join(root, "a", "s1.jsonl"), line("m1", `C:\w\g\old`, "2026-10-18T10:00:00Z"), now.Add(-50*time.Hour))
	writeFile(t, filepath.Join(root, "b", "s2.jsonl"), line("m2", `C:\w\g\new`, "2026-10-20T14:59:00Z"), now)

	snap := NewStore(root, Options{}).Poll(now)

	if len(snap.Projects) != 2 {
		t.Fatalf("projects = %+v, want 2 (inactive sessions included)", snap.Projects)
	}
	if snap.Projects[0].Cwd != `C:\w\g\new` || snap.Projects[1].Cwd != `C:\w\g\old` {
		t.Errorf("projects = %+v, want most recent first", snap.Projects)
	}
}
