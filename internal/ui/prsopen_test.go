package ui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/prs"
)

// treeRecorder stands in for GitHub: it hands out a folder for the file and
// records what was asked for and what was taken away.
type treeRecorder struct {
	dir     string
	err     error
	opened  []string
	removed chan prs.Tree
}

func (r *treeRecorder) open(_ context.Context, repo, sha, path string) (prs.Tree, error) {
	r.opened = append(r.opened, fmt.Sprintf("%s@%s:%s", repo, sha, path))
	if r.err != nil {
		return prs.Tree{}, r.err
	}
	if sha == "" {
		return prs.Tree{}, errors.New("the head commit of the pull request is unknown")
	}
	return prs.Tree{Dir: r.dir, SHA: sha}, nil
}

func (r *treeRecorder) remove(t prs.Tree) { r.removed <- t }

// waitRemoved is the folder that was taken away, or a failed test.
func (r *treeRecorder) waitRemoved(t *testing.T) prs.Tree {
	t.Helper()
	select {
	case tree := <-r.removed:
		return tree
	case <-time.After(2 * time.Second):
		t.Fatal("the checkout was not removed")
		return prs.Tree{}
	}
}

// openOffice has the diff of pull request 7 open, with GitHub and the editor
// faked.
func openOffice(t *testing.T, diff string) (Model, *termRecorder, *treeRecorder) {
	t.Helper()
	return openOfficeAt(t, diff, "abc123")
}

// openOfficeAt is openOffice with the head commit the diff was read at; ""
// makes reading it fail.
func openOfficeAt(t *testing.T, diff, sha string) (Model, *termRecorder, *treeRecorder) {
	t.Helper()
	list, _ := ghAnswering(prNode("acme/api", 7, "Fix login"))
	g := &recGH{list: list, sha: sha, diff: diff}
	m := prsOffice(t, prsOn, g.run)
	rec, tr := &termRecorder{}, &treeRecorder{dir: t.TempDir(), removed: make(chan prs.Tree, 8)}
	m.startTerm = rec.start
	m.pulls.openTree, m.pulls.removeTree = tr.open, tr.remove
	m.pulls.editor = func() string { return "nvim" }
	return press(press(m, key("r")), key("enter")), rec, tr
}

func TestOpeningALineShowsItInAReadOnlyEditorInsideTheWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  int
		want string // the line the editor starts at
	}{
		{"an added line", 4, "+2"},
		{"a removed line, which the new file has no number for", 3, "+2"},
		{"a context line", 5, "+3"},
		{"a file row, which starts at the first line", 0, "+1"},
	} {
		m, rec, tr := openOffice(t, "")
		m = press(m.diffMove(tc.row), key("o"))
		if want := []string{"acme/api@abc123:app.go"}; !reflect.DeepEqual(tr.opened, want) {
			t.Errorf("%s: read %q, want %q", tc.name, tr.opened, want)
		}
		if len(rec.started) != 1 {
			t.Fatalf("%s: %d terminals started, want 1:\n%s", tc.name, len(rec.started), view(m))
		}
		term := rec.last()
		want := []string{"nvim", "--cmd", "set nomodeline", "-R", "-M", tc.want, "--", "app.go"}
		if !reflect.DeepEqual(term.argv, want) || term.dir != tr.dir {
			t.Errorf("%s: started %q in %q, want %q in %q", tc.name, term.argv, term.dir, want, tr.dir)
		}
		if m.hubOpen || m.win == nil || len(m.win.panes) != 1 {
			t.Errorf("%s: the editor is not alone in a window of the workspace (hub open: %v)", tc.name, m.hubOpen)
		}
	}
}

func TestTheEditorCannotChangeAnythingAndIgnoresModelines(t *testing.T) {
	m, rec, _ := openOffice(t, "")
	m = press(m.diffMove(4), key("o"))
	argv := strings.Join(rec.last().argv, " ")
	for _, flag := range []string{"-R", "-M", "set nomodeline"} {
		if !strings.Contains(argv, flag) {
			t.Errorf("the editor runs without %q: %s", flag, argv)
		}
	}
}

func TestEndingTheEditorTakesItsFolderAway(t *testing.T) {
	m, rec, tr := openOffice(t, "")
	m = press(m.diffMove(4), key("o"))
	term := rec.last()
	term.exited = true
	m = update(m, termMsg(term.id))
	if got := tr.waitRemoved(t); got.Dir != tr.dir {
		t.Errorf("removed %+v, want the folder in %s", got, tr.dir)
	}
	if m.win != nil || len(m.pulls.trees) != 0 {
		t.Errorf("the window or the folder is still tracked after the editor ended (window %v, trees %v)", m.win != nil, m.pulls.trees)
	}
}

func TestClosingTheEditorsTerminalTakesItsFolderAway(t *testing.T) {
	m, _, tr := openOffice(t, "")
	m = press(m.diffMove(4), key("o"))
	m = m.closeShell()
	if got := tr.waitRemoved(t); got.Dir != tr.dir {
		t.Errorf("removed %+v, want the folder in %s", got, tr.dir)
	}
}

func TestQuittingTheOfficeTakesEveryFolderAway(t *testing.T) {
	m, _, tr := openOffice(t, "")
	m = press(m.diffMove(4), key("o"))
	m.CloseAll()
	select {
	case got := <-tr.removed:
		if got.Dir != tr.dir {
			t.Errorf("removed %+v", got)
		}
	default:
		t.Error("quitting left the folder behind")
	}
}

func TestTheEditorEndsWithTheOfficeSoItIsNeverKeptByTheDaemon(t *testing.T) {
	m, rec, _ := openOffice(t, "")
	hosted := 0
	m.hostTerm = func(string, string, []string, int, int, chan<- string) (Terminal, error) {
		hosted++
		return &fakeTerm{}, nil
	}
	m = press(m.diffMove(4), key("o"))
	if hosted != 0 || len(rec.started) != 1 {
		t.Errorf("the editor went to the daemon (%d hosted, %d local): its folder would be deleted under it", hosted, len(rec.started))
	}
}

func TestAFileThatCannotBeReadSaysWhy(t *testing.T) {
	m, rec, tr := openOffice(t, "")
	tr.err = errors.New("boom")
	m = press(m.diffMove(4), key("o"))
	if !strings.Contains(view(m), "Could not open the files: ") || !strings.Contains(view(m), "boom") || len(rec.started) != 0 {
		t.Errorf("view:\n%s", view(m))
	}
	tr.err = nil
	m = press(m, key("o"))
	if len(rec.started) != 1 {
		t.Error("it cannot be tried again after a failure")
	}
}

func TestWithoutAnEditorNothingIsFetched(t *testing.T) {
	m, rec, tr := openOffice(t, "")
	m.pulls.editor = func() string { return "" }
	m = press(m.diffMove(4), key("o"))
	if !strings.Contains(view(m), "nvim (or vim) is not installed") || len(tr.opened) != 0 || len(rec.started) != 0 {
		t.Errorf("(requests %v):\n%s", tr.opened, view(m))
	}
}

func TestADeletedOrBinaryFileHasNothingToOpen(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/old.go b/old.go", "deleted file mode 100644", "--- a/old.go", "+++ /dev/null", "@@ -1 +0,0 @@", "-gone",
		"diff --git a/logo.png b/logo.png", "Binary files a/logo.png and b/logo.png differ", "",
	}, "\n")
	m, rec, tr := openOffice(t, diff)
	m = press(m.diffMove(2), key("o")) // the removed line of the deleted file
	if !strings.Contains(view(m), "deleted in the pull request") {
		t.Errorf("a deleted file:\n%s", view(m))
	}
	m = press(m.diffMove(3), key("o")) // the binary file's row
	if !strings.Contains(view(m), "A binary file has no text to open.") {
		t.Errorf("a binary file:\n%s", view(m))
	}
	if len(tr.opened) != 0 || len(rec.started) != 0 {
		t.Errorf("something was fetched for them: %v", tr.opened)
	}
}

func TestAPathFromThePullRequestCannotLeaveTheCheckout(t *testing.T) {
	for _, path := range []string{"../../etc/passwd", ".git/config", "dir/.git/hooks/post-checkout"} {
		diff := strings.Join([]string{
			"diff --git a/" + path + " b/" + path, "--- a/" + path, "+++ b/" + path, "@@ -1 +1 @@", "-a", "+b", "",
		}, "\n")
		m, rec, tr := openOffice(t, diff)
		m = press(m.diffMove(3), key("o"))
		if !strings.Contains(view(m), "Not a path inside the repository.") || len(tr.opened) != 0 || len(rec.started) != 0 {
			t.Errorf("%s: it was opened (requests %v):\n%s", path, tr.opened, view(m))
		}
	}
}

func TestAFileThatArrivesAfterTheUserLeftIsRemoved(t *testing.T) {
	m, _, tr := openOffice(t, "")
	m = press(m, key("esc")) // back to the list; the diff is gone
	m = update(m, prsTreeMsg{repo: "acme/api", number: 7, tree: prs.Tree{Dir: tr.dir}, path: "app.go", line: 2})
	if got := tr.waitRemoved(t); got.Dir != tr.dir {
		t.Errorf("removed %+v", got)
	}
	if m.win != nil {
		t.Error("a window opened for a diff the user had left")
	}
}

func TestAKeyWhilePreparingTheFileIsDropped(t *testing.T) {
	m, rec, _ := openOffice(t, "")
	next, cmd := m.diffMove(4).Update(key("o"))
	m = next.(Model)
	if cmd == nil || m.pulls.diff.mode != modeSending {
		t.Fatalf("reading the file did not start (mode %v)", m.pulls.diff.mode)
	}
	m = press(m, key("esc"))
	if m.pulls.diff == nil || len(rec.started) != 0 {
		t.Error("a key acted while the files were being prepared")
	}
}

func TestTheHintRowOffersTheEditor(t *testing.T) {
	m, _, _ := openOffice(t, "")
	if !strings.Contains(view(m), "o nvim") {
		t.Errorf("the diff does not offer o:\n%s", view(m))
	}
}

func TestTheFileIsReadAtTheCommitOfTheDiffWithNoCloneAndNoAgentNeeded(t *testing.T) {
	// The office has no folder of this repository anywhere: its one agent runs
	// elsewhere, and nothing here looks for a clone.
	m, rec, tr := openOffice(t, "")
	m = press(m.diffMove(4), key("o"))
	if want := []string{"acme/api@abc123:app.go"}; !reflect.DeepEqual(tr.opened, want) || len(rec.started) != 1 {
		t.Errorf("read %q, started %d terminals; want %q and 1", tr.opened, len(rec.started), want)
	}
}

func TestWithoutTheCommitFromTheDiffItIsReadAgainAndOtherwiseSaidSo(t *testing.T) {
	m, rec, tr := openOfficeAt(t, "", "") // reading the head commit fails, every time
	m = press(m.diffMove(4), key("o"))
	if !strings.Contains(view(m), "head commit") || len(rec.started) != 0 {
		t.Errorf("(requests %v):\n%s", tr.opened, view(m))
	}
}
