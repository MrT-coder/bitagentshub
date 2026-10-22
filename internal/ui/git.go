package ui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// gitBranch is the branch checked out in the repository that holds dir,
// the short commit when HEAD is detached, or "" outside a repository. It
// reads .git directly, so titles cost no process per refresh.
func gitBranch(dir string) string {
	gitDir := findGitDir(dir)
	if gitDir == "" {
		return ""
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	h := strings.TrimSpace(string(head))
	if ref, ok := strings.CutPrefix(h, "ref: refs/heads/"); ok {
		return ref
	}
	if len(h) >= 7 && !strings.HasPrefix(h, "ref:") {
		return h[:7]
	}
	return ""
}

// findGitDir is the git dir of the repository that holds dir, or "".
func findGitDir(dir string) string {
	for d := filepath.Clean(dir); ; {
		gitPath := filepath.Join(d, ".git")
		if fi, err := os.Stat(gitPath); err == nil {
			if fi.IsDir() {
				return gitPath
			}
			// A worktree or submodule: ".git" names the real git dir.
			b, err := os.ReadFile(gitPath)
			if err != nil {
				return ""
			}
			target, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
			if !ok {
				return ""
			}
			gitDir := strings.TrimSpace(target)
			if !filepath.IsAbs(gitDir) {
				gitDir = filepath.Join(d, gitDir)
			}
			return gitDir
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// refsStamp changes whenever a commit, checkout, fetch or push moves HEAD,
// a branch or a remote branch of the repository whose git dir is gitDir.
// It only reads file times and sizes, never runs git.
func refsStamp(gitDir string) string {
	common := gitDir // a worktree keeps its refs in the main git dir
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		common = strings.TrimSpace(string(b))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitDir, common)
		}
	}
	var stamp strings.Builder
	add := func(path string) {
		if fi, err := os.Stat(path); err == nil {
			fmt.Fprintf(&stamp, "%s:%d:%d|", path, fi.ModTime().UnixNano(), fi.Size())
		}
	}
	add(filepath.Join(gitDir, "HEAD"))
	add(filepath.Join(common, "packed-refs"))
	add(filepath.Join(common, "FETCH_HEAD"))
	add(filepath.Join(common, "config")) // a new upstream
	for _, refs := range []string{"heads", "remotes"} {
		_ = filepath.WalkDir(filepath.Join(common, "refs", refs), func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				add(path)
			}
			return nil
		})
	}
	return stamp.String()
}
