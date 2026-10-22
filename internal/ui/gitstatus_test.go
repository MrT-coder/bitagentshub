package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestReadGitCountsCommitsAheadAndBehindItsUpstream(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	remote, a, b := filepath.Join(root, "remote.git"), filepath.Join(root, "a"), filepath.Join(root, "b")
	run(t, root, "init", "-q", "--bare", "-b", "main", remote)
	run(t, root, "clone", "-q", remote, a)
	run(t, a, "commit", "-q", "--allow-empty", "-m", "one")
	run(t, a, "push", "-q", "origin", "HEAD:main")
	run(t, root, "clone", "-q", remote, b)
	run(t, b, "commit", "-q", "--allow-empty", "-m", "theirs")
	run(t, b, "push", "-q", "origin", "HEAD:main")
	run(t, a, "commit", "-q", "--allow-empty", "-m", "mine 1")
	run(t, a, "commit", "-q", "--allow-empty", "-m", "mine 2")
	run(t, a, "fetch", "-q")

	if g := readGit(a); g.branch != "main" || g.ahead != 2 || g.behind != 1 {
		t.Errorf("readGit = %+v, want main ↑2 ↓1", g)
	}
	local := t.TempDir()
	run(t, local, "init", "-q", "-b", "solo")
	if g := readGit(local); g.branch != "solo" || g.ahead != 0 || g.behind != 0 {
		t.Errorf("no upstream: %+v, want the branch only", g)
	}
	if g := readGit(t.TempDir()); g.branch != "" {
		t.Errorf("no repository: %+v", g)
	}
}

func TestReadGitAsksGitOnlyWhenTheRefsChange(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	remote, a, b := filepath.Join(root, "remote.git"), filepath.Join(root, "a"), filepath.Join(root, "b")
	run(t, root, "init", "-q", "--bare", "-b", "main", remote)
	run(t, root, "clone", "-q", remote, a)
	run(t, a, "commit", "-q", "--allow-empty", "-m", "one")
	run(t, a, "push", "-q", "origin", "HEAD:main")

	runs := gitRuns.Load()
	for range 3 {
		if g := readGit(a); g.ahead != 0 || g.behind != 0 {
			t.Fatalf("in step with its upstream: %+v", g)
		}
	}
	if got := gitRuns.Load() - runs; got != 1 {
		t.Errorf("three reads of unchanged refs ran git %d times, want 1", got)
	}

	run(t, a, "commit", "-q", "--allow-empty", "-m", "mine")
	if g := readGit(a); g.ahead != 1 {
		t.Errorf("after a commit: %+v, want ↑1", g)
	}
	run(t, root, "clone", "-q", remote, b)
	run(t, b, "commit", "-q", "--allow-empty", "-m", "theirs")
	run(t, b, "push", "-q", "origin", "HEAD:main")
	run(t, a, "fetch", "-q")
	if g := readGit(a); g.ahead != 1 || g.behind != 1 {
		t.Errorf("after a fetch: %+v, want ↑1 ↓1", g)
	}
	if got := gitRuns.Load() - runs; got != 3 {
		t.Errorf("git ran %d times, want once per change of the refs (3)", got)
	}
}
