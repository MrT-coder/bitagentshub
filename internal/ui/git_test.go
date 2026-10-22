package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/office"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGitBranchReadsHEAD(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/feat/x\n")
	sub := filepath.Join(repo, "internal", "ui")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := gitBranch(repo); got != "feat/x" {
		t.Errorf("repo root: %q", got)
	}
	if got := gitBranch(sub); got != "feat/x" {
		t.Errorf("a folder inside the repo: %q", got)
	}

	detached := t.TempDir()
	writeFile(t, filepath.Join(detached, ".git", "HEAD"), "bb0670a1234567890abcdef\n")
	if got := gitBranch(detached); got != "bb0670a" {
		t.Errorf("detached HEAD: %q, want the short commit", got)
	}

	// A worktree: .git is a file pointing at the real git dir.
	wt, gitdir := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(gitdir, "HEAD"), "ref: refs/heads/spike\n")
	writeFile(t, filepath.Join(wt, ".git"), "gitdir: "+gitdir+"\n")
	if got := gitBranch(wt); got != "spike" {
		t.Errorf("worktree: %q", got)
	}
	rel := t.TempDir()
	writeFile(t, filepath.Join(rel, "meta", "HEAD"), "ref: refs/heads/rel\n")
	writeFile(t, filepath.Join(rel, ".git"), "gitdir: meta\n")
	if got := gitBranch(rel); got != "rel" {
		t.Errorf("relative gitdir: %q", got)
	}

	if got := gitBranch(t.TempDir()); got != "" {
		t.Errorf("no repository: %q, want empty", got)
	}
}

func TestPaneTitlesShowTheFolderAndBranch(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "my-project")
	writeFile(t, filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/develop\n")
	rec := &termRecorder{}
	m := New(nil).WithShell([]string{"pwsh"})
	m.startTerm = rec.start
	// The agent runs in the office; one that runs nowhere is not shown.
	m.terms["a"] = &hosted{term: &fakeTerm{}, dir: repo, project: "my-project", group: "g"}
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m = update(m, snapshotMsg(claude.Snapshot{Agents: []claude.Agent{
		{ID: "a", Group: "g", Project: "my-project", Cwd: repo, State: claude.StateWaiting, LastActivity: time.Now()},
	}}))
	m = press(m, key("enter"))
	m = press(m, key("f3"))
	top := strings.Split(view(m), "\n")[officeTop+m.paneRects()[0].y-1]
	if n := strings.Count(top, " 1 my-project ~ develop "); n != 2 {
		t.Errorf("both panes should be titled \"1 my-project ~ develop\" (station, folder, branch), got %d in %q", n, top)
	}
	for _, old := range []string{"session", "local", "terminal"} {
		if strings.Contains(top, old) {
			t.Errorf("the title should hold nothing else, found %q in %q", old, top)
		}
	}
	writeFile(t, filepath.Join(repo, ".git", "HEAD"), "ref: refs/heads/feat/next\n")
	m = update(m, pollMsg{})
	top = strings.Split(view(m), "\n")[officeTop+m.paneRects()[0].y-1]
	if !strings.Contains(top, " 1 my-project ~ feat/next ") {
		t.Errorf("the title should follow a branch switch: %q", top)
	}
}

func TestPaneTitlesWithoutARepositoryShowOnlyTheFolder(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(m, key("enter"))
	top := strings.Split(view(m), "\n")[officeTop+m.paneRects()[0].y-1]
	n := 1
	for i, id := range office.Order(m.layout) {
		if id == m.win.agentID {
			n = i + 1
		}
	}
	if want := fmt.Sprintf(" %d portal-front ", n); !strings.Contains(top, want) || strings.Contains(top, "~") {
		t.Errorf("title = %q, want only %q", top, want)
	}
}

func TestATerminalOnlyWindowHasNoStationNumber(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scratch")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m, _, _ := twoAgentOffice(t)
	m, _ = m.openFolder(dir)
	top := strings.Split(view(m), "\n")[officeTop+m.paneRects()[0].y-1]
	if !strings.Contains(top, "─ scratch ─") {
		t.Errorf("title = %q, want only the folder", top)
	}
}
