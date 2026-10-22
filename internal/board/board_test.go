package board

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var day = time.Date(2026, 10, 21, 9, 30, 0, 0, time.Local)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestANoteLivesInItsProjectFolderNamedByDay(t *testing.T) {
	got := NotePath("tasks", "bitagentshub", day)
	if want := filepath.Join("tasks", "bitagentshub", "2026-10-21.md"); got != want {
		t.Errorf("NotePath = %q, want %q", got, want)
	}
}

func TestTodayCreatesTheNoteWithItsTitle(t *testing.T) {
	dir := t.TempDir()
	path, err := Today(dir, "bitagentshub", day)
	if err != nil {
		t.Fatal(err)
	}
	if path != NotePath(dir, "bitagentshub", day) {
		t.Errorf("path = %q", path)
	}
	if got, want := read(t, path), "# bitagentshub · 2026-10-21\n\n"; got != want {
		t.Errorf("note = %q, want %q", got, want)
	}
}

func TestUnfinishedTasksCarryOverFromTheLatestEarlierNote(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "bitagentshub", "2026-10-15.md")
	latest := filepath.Join(dir, "bitagentshub", "2026-10-19.md")
	write(t, older, "# old\n\n- [ ] from long ago\n")
	latestText := "# bitagentshub · 2026-10-19\n\n- [x] shipped\n- [ ] write the board\n  - [ ] carry subtasks too\n* [ ] star list\nsome note text\n- [X] also done\n"
	write(t, latest, latestText)
	write(t, filepath.Join(dir, "bitagentshub", "notes.md"), "- [ ] not a daily note\n")
	write(t, filepath.Join(dir, "bitagentshub", "2026-10-22.md"), "- [ ] tomorrow's\n")

	path, err := Today(dir, "bitagentshub", day)
	if err != nil {
		t.Fatal(err)
	}
	want := "# bitagentshub · 2026-10-21\n\n- [ ] write the board\n  - [ ] carry subtasks too\n* [ ] star list\n"
	if got := read(t, path); got != want {
		t.Errorf("today's note = %q, want %q", got, want)
	}
	if got := read(t, latest); got != latestText {
		t.Errorf("the earlier note changed: %q", got)
	}
}

func TestTodayLeavesAnExistingNoteAlone(t *testing.T) {
	dir := t.TempDir()
	path := NotePath(dir, "bitagentshub", day)
	write(t, path, "- [ ] mine\n")
	write(t, filepath.Join(dir, "bitagentshub", "2026-10-20.md"), "- [ ] yesterday\n")
	if _, err := Today(dir, "bitagentshub", day); err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != "- [ ] mine\n" {
		t.Errorf("an existing note was rewritten: %q", got)
	}
}

func TestTasksCountsPendingAndDone(t *testing.T) {
	got := Tasks("# t\n- [ ] a\n  - [x] b\n* [X] c\n+ [ ] d\n-[ ] not a task\n- [ ]tight is not one either\ntext [x]\n- [x]\n")
	if want := (Count{Pending: 2, Done: 3}); got != want {
		t.Errorf("Tasks = %+v, want %+v", got, want)
	}
}

func TestPointsAreEveryDoneTaskInEveryNote(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a", "2026-10-20.md"), "- [x] one\n- [ ] open\n")
	write(t, filepath.Join(dir, "a", "2026-10-21.md"), "- [x] two\n- [x] three\n")
	write(t, filepath.Join(dir, "b", "2026-10-21.md"), "- [X] four\n")
	write(t, filepath.Join(dir, "b", "image.png"), "- [x] not a note\n")
	got, err := Points(dir, time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if got != (Stars{Total: 4, Today: 3}) {
		t.Errorf("Points = %+v, want 4 in all, 3 in today's notes", got)
	}
}

func TestPointsOfABoardNotMadeYetAreZero(t *testing.T) {
	got, err := Points(filepath.Join(t.TempDir(), "missing"), time.Now())
	if err != nil || got != (Stars{}) {
		t.Errorf("Points = %+v, %v", got, err)
	}
}
