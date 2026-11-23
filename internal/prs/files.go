package prs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// treePrefix starts the name of every folder the office makes for a file of
// a pull request; RemoveTree touches nothing else.
const treePrefix = "bah-pr-"

// Tree is a private temporary folder holding a file of a pull request, to
// read it in an editor. It needs no clone of the repository.
type Tree struct {
	Dir string // the temporary folder
	SHA string // the commit the file was read at
}

// FetchFile reads path of repo at commit sha through gh and writes it to a
// new temporary folder, at the same relative path so an editor shows where
// it belongs. It asks GitHub for that one file, so no clone is needed, and
// the folder is readable by the user alone. Remove it with RemoveTree.
func FetchFile(ctx context.Context, run Runner, repo, sha, path string) (Tree, error) {
	owner, name, ok := strings.Cut(repo, "/")
	switch {
	case !ok || owner == "" || name == "" || strings.Contains(name, "/"):
		return Tree{}, fmt.Errorf("%q is not a repository such as owner/name", repo)
	case sha == "":
		return Tree{}, errors.New("the head commit of the pull request is unknown")
	case !SafeRelPath(path):
		return Tree{}, fmt.Errorf("%q is not a path inside the repository", path)
	}
	out, err := run(ctx, "api", "-H", "Accept: application/vnd.github.raw+json",
		fmt.Sprintf("repos/%s/contents/%s?ref=%s", repo, escapePath(path), url.QueryEscape(sha)))
	if err != nil {
		return Tree{}, fmt.Errorf("gh file %s@%s: %w", path, shortSHA(sha), err)
	}
	dir, err := os.MkdirTemp("", treePrefix+owner+"-"+name+"-")
	if err != nil {
		return Tree{}, err
	}
	target := filepath.Join(dir, filepath.FromSlash(path))
	if rel, err := filepath.Rel(dir, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		os.RemoveAll(dir)
		return Tree{}, fmt.Errorf("%q is not a path inside the repository", path)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		os.RemoveAll(dir)
		return Tree{}, err
	}
	if err := os.WriteFile(target, out, 0o600); err != nil {
		os.RemoveAll(dir)
		return Tree{}, err
	}
	return Tree{Dir: dir, SHA: sha}, nil
}

// RemoveTree deletes the folder FetchFile made. It is safe to call twice,
// and it only removes folders the office made.
func RemoveTree(t Tree) {
	if t.Dir == "" || !strings.HasPrefix(filepath.Base(t.Dir), treePrefix) {
		return
	}
	os.RemoveAll(t.Dir)
}

// escapePath escapes each part of a path for a URL and keeps its slashes.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// SafeRelPath tells whether a path from a pull request's diff may be joined
// to a folder of ours: relative, inside it and outside any .git folder.
func SafeRelPath(p string) bool {
	if p == "" || !filepath.IsLocal(filepath.FromSlash(p)) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return false
	}
	for _, part := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." || strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}
