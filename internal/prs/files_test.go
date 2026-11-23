package prs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// fileGH answers a contents request with body, and records what was asked.
type fileGH struct {
	body  []byte
	err   error
	calls [][]string
}

func (g *fileGH) run(_ context.Context, args ...string) ([]byte, error) {
	g.calls = append(g.calls, args)
	return g.body, g.err
}

func TestFetchFileWritesTheFileAtItsPathAndAsksForItsCommit(t *testing.T) {
	g := &fileGH{body: []byte("package main\r\n\x00\xff bytes stay as they are\n")}
	tree, err := FetchFile(context.Background(), g.run, "acme/api", "abc123", "internal/app.go")
	if err != nil {
		t.Fatal(err)
	}
	defer RemoveTree(tree)

	got, err := os.ReadFile(filepath.Join(tree.Dir, "internal", "app.go"))
	if err != nil || string(got) != string(g.body) {
		t.Errorf("the file holds %q (%v), want exactly what gh sent", got, err)
	}
	want := []string{"api", "-H", "Accept: application/vnd.github.raw+json", "repos/acme/api/contents/internal/app.go?ref=abc123"}
	if len(g.calls) != 1 || !reflect.DeepEqual(g.calls[0], want) {
		t.Errorf("gh asked %q, want %q", g.calls, want)
	}
	if tree.SHA != "abc123" {
		t.Errorf("tree = %+v", tree)
	}
}

func TestFetchFileEscapesWhatAPathMayHold(t *testing.T) {
	g := &fileGH{body: []byte("x")}
	tree, err := FetchFile(context.Background(), g.run, "acme/api", "abc123", "docs/my file #1.md")
	if err != nil {
		t.Fatal(err)
	}
	defer RemoveTree(tree)
	if want := "repos/acme/api/contents/docs/my%20file%20%231.md?ref=abc123"; g.calls[0][3] != want {
		t.Errorf("the request is %q, want %q", g.calls[0][3], want)
	}
}

func TestFetchFileLeavesNothingBehindWhenGHFails(t *testing.T) {
	before := tempDirs(t)
	g := &fileGH{err: errors.New("boom")}
	if _, err := FetchFile(context.Background(), g.run, "acme/api", "abc123", "app.go"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want it to carry the gh failure", err)
	}
	if after := tempDirs(t); len(after) != len(before) {
		t.Errorf("a failed fetch left %d folder(s) in the temp dir", len(after)-len(before))
	}
}

// tempDirs lists the folders the office makes in the temp dir.
func tempDirs(t *testing.T) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(os.TempDir(), "bah-pr-*"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestFetchFileRefusesAPathThatCouldLeaveTheFolderWithoutAsking(t *testing.T) {
	for _, p := range []string{"../../etc/passwd", "/etc/passwd", ".git/config", "dir/.git/hooks/post-checkout", "", `..\x`} {
		g := &fileGH{body: []byte("x")}
		if tree, err := FetchFile(context.Background(), g.run, "acme/api", "abc123", p); err == nil {
			RemoveTree(tree)
			t.Errorf("%q was fetched", p)
		}
		if len(g.calls) != 0 {
			t.Errorf("%q still asked gh: %q", p, g.calls)
		}
	}
}

func TestFetchFileNeedsACommitAndARepo(t *testing.T) {
	g := &fileGH{body: []byte("x")}
	for _, c := range []struct{ repo, sha string }{{"acme/api", ""}, {"", "abc"}, {"acme", "abc"}} {
		if tree, err := FetchFile(context.Background(), g.run, c.repo, c.sha, "a.go"); err == nil {
			RemoveTree(tree)
			t.Errorf("repo %q at %q was fetched", c.repo, c.sha)
		}
	}
	if len(g.calls) != 0 {
		t.Errorf("gh was asked %q", g.calls)
	}
}

func TestTheFolderAndTheFileArePrivateToTheUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has no permission bits to read")
	}
	g := &fileGH{body: []byte("secret")}
	tree, err := FetchFile(context.Background(), g.run, "acme/api", "abc123", "a.go")
	if err != nil {
		t.Fatal(err)
	}
	defer RemoveTree(tree)
	if fi, _ := os.Stat(tree.Dir); fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("the folder is %v: other users can read a private repository's file", fi.Mode().Perm())
	}
	if fi, _ := os.Stat(filepath.Join(tree.Dir, "a.go")); fi.Mode().Perm()&0o077 != 0 {
		t.Errorf("the file is %v", fi.Mode().Perm())
	}
}

func TestRemoveTreeTakesEverythingAwayAndIsSafeTwice(t *testing.T) {
	g := &fileGH{body: []byte("x")}
	tree, err := FetchFile(context.Background(), g.run, "acme/api", "abc123", "deep/er/a.go")
	if err != nil {
		t.Fatal(err)
	}
	RemoveTree(tree)
	if _, err := os.Stat(tree.Dir); err == nil {
		t.Errorf("%s is still there", tree.Dir)
	}
	RemoveTree(tree)
	RemoveTree(Tree{}) // nothing to remove must never remove something else
}

func TestAPathFromAPullRequestMustStayInsideTheFolder(t *testing.T) {
	for path, want := range map[string]bool{
		"a.txt":            true,
		"dir/b.txt":        true,
		"../etc/x":         false,
		"dir/../../x":      false,
		"/etc/passwd":      false,
		"":                 false,
		".git/config":      false,
		"dir/.git/hooks/x": false,
	} {
		if got := SafeRelPath(path); got != want {
			t.Errorf("SafeRelPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestEscapePathEscapesQueryAndFragmentCharactersButKeepsSlashes(t *testing.T) {
	if got, want := escapePath("a b/c?d/e#f/100%/ñ.go"), "a%20b/c%3Fd/e%23f/100%25/%C3%B1.go"; got != want {
		t.Errorf("escapePath = %q, want %q", got, want)
	}
}
