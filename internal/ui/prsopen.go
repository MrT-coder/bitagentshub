package ui

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/prs"
)

// prsTreeTime bounds reading one file of a pull request.
const prsTreeTime = 30 * time.Second

// prsTreeMsg is what checking out a pull request for the editor reports.
type prsTreeMsg struct {
	repo   string
	number int
	tree   prs.Tree
	path   string
	line   int
	err    error
}

// defaultEditor is the editor that opens files read-only: nvim, else vim.
func defaultEditor() string {
	for _, e := range []string{"nvim", "vim"} {
		if _, err := exec.LookPath(e); err == nil {
			return e
		}
	}
	return ""
}

// openLine is the line of the new file the cursor is at or nearest to: its
// own, else the next one of the same file, else the previous one, else the
// first.
func (d *diffState) openLine() int {
	row := d.rows[d.cur]
	if row.kind == rowLine && row.line.New > 0 {
		return row.line.New
	}
	for i := d.cur; i < len(d.rows) && d.rows[i].file == row.file; i++ {
		if d.rows[i].kind == rowLine && d.rows[i].line.New > 0 {
			return d.rows[i].line.New
		}
	}
	for i := d.cur; i >= 0 && d.rows[i].file == row.file; i-- {
		if d.rows[i].kind == rowLine && d.rows[i].line.New > 0 {
			return d.rows[i].line.New
		}
	}
	return 1
}

// prsOpenFile starts reading the file under the cursor, at the commit the
// diff is of, to show it in a read-only editor. It needs no clone of the
// repository and no agent: the file comes from GitHub through gh.
func (m Model) prsOpenFile() (Model, tea.Cmd) {
	p := m.pulls
	d := p.diff
	if d.cur < 0 || d.cur >= len(d.rows) {
		return m, nil
	}
	f := d.files[d.rows[d.cur].file]
	switch {
	case p.editor == nil || p.editor() == "":
		d.msg = i18n.T("nvim (or vim) is not installed.")
	case f.Status == prs.StatusDeleted:
		d.msg = i18n.T("This file is deleted in the pull request.")
	case f.Binary:
		d.msg = i18n.T("A binary file has no text to open.")
	case !prs.SafeRelPath(f.Path):
		d.msg = i18n.T("Not a path inside the repository.")
	default:
		line, path, open, run := d.openLine(), f.Path, p.openTree, p.run
		repo, n, sha := d.pr.Repo, d.pr.Number, d.sha
		d.mode, d.msg = modeSending, i18n.T("Preparing the files…")
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), prsTreeTime)
			defer cancel()
			if sha == "" { // reading it when the diff opened failed; once more
				sha, _ = prs.HeadSHA(ctx, run, repo, n)
			}
			tree, err := open(ctx, repo, sha, path)
			return prsTreeMsg{repo: repo, number: n, tree: tree, path: path, line: line, err: err}
		}
	}
	return m, nil
}

// prsTreeOpened takes in the file: the editor opens in a window of the
// workspace on it, or what went wrong shows. A file nobody waits for any
// more is removed at once.
func (m Model) prsTreeOpened(msg prsTreeMsg) (Model, tea.Cmd) {
	p := m.pulls
	if p == nil {
		if msg.err == nil {
			go prs.RemoveTree(msg.tree)
		}
		return m, nil
	}
	d := p.diff
	stale := d == nil || d.pr.Repo != msg.repo || d.pr.Number != msg.number
	switch {
	case msg.err != nil:
		if !stale {
			d.mode, d.msg = modeBrowse, i18n.Tf("Could not open the files: %s", cleanText(msg.err.Error()))
		}
		return m, nil
	case stale:
		go p.removeTree(msg.tree)
		return m, nil
	}
	argv := []string{p.editor(), "--cmd", "set nomodeline", "-R", "-M", fmt.Sprintf("+%d", msg.line), "--", msg.path}
	m, key, err := m.openEditorWindow(msg.tree.Dir, argv, fmt.Sprintf("PR #%d", msg.number))
	if err != nil {
		go p.removeTree(msg.tree)
		d.mode, d.msg = modeBrowse, i18n.Tf("Could not open the files: %s", cleanText(err.Error()))
		return m, nil
	}
	p.trees[key] = msg.tree
	d.mode, d.msg = modeBrowse, ""
	return m.closeHub(), nil
}

// openEditorWindow shows a window of the workspace with one terminal running
// argv in dir. It runs inside the office, never in the daemon: the folder it
// shows is deleted when the office ends, and the editor must end with it.
func (m Model) openEditorWindow(dir string, argv []string, branch string) (Model, string, error) {
	if m.win != nil {
		m = m.closeWindow()
	}
	key := shellPrefix + newUUID()
	m.win = &window{dir: dir, branch: branch}
	m.win.panes = []*pane{{key: key, shell: true}}
	w, h := m.termSize(0)
	t, err := m.startTerm(key, dir, argv, w, h, m.termNotify)
	if err != nil {
		m.win = nil
		return m, "", err
	}
	m.terms[key] = &hosted{term: t, dir: dir, shell: true}
	fk := folderKey(dir)
	m.shells[fk] = append(m.shells[fk], key)
	m.resizePanes()
	return m, key, nil
}

// dropTree removes the folder of the terminal key, if it had one.
func (m Model) dropTree(key string) {
	p := m.pulls
	if p == nil {
		return
	}
	if t, ok := p.trees[key]; ok {
		delete(p.trees, key)
		go p.removeTree(t)
	}
}

// removeTrees removes every folder left, before the office ends.
func (m Model) removeTrees() {
	p := m.pulls
	if p == nil {
		return
	}
	for key, t := range p.trees {
		delete(p.trees, key)
		p.removeTree(t)
	}
}
