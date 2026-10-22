// Package projects finds the folders an agent can be started in: the
// working directories of past sessions and their sibling folders.
package projects

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/claude"
)

type Project struct {
	Path         string
	Name         string // folder name
	Group        string // parent folder name, as in the office areas
	Recent       bool   // an agent has worked here recently
	LastActivity time.Time
}

// skipped folders are never projects.
var skipped = map[string]bool{"node_modules": true, "vendor": true, "bin": true, "obj": true, "dist": true}

// Discover lists recent projects first (most recent on top, a repository
// and the subfolders agents worked in as one), then every other folder next
// to them or under extraRoots, alphabetically. Folders whose name starts
// with a dot are ignored.
func Discover(known []claude.ProjectRef, extraRoots []string) []Project {
	seen := map[string]bool{}
	var recent, others []Project
	add := func(list *[]Project, p Project) {
		key := strings.ToLower(filepath.Clean(p.Path))
		if seen[key] {
			return
		}
		seen[key] = true
		*list = append(*list, p)
	}

	roots := append([]string(nil), extraRoots...)
	var live []claude.ProjectRef
	worked := map[string]bool{}
	for _, k := range known {
		if info, err := os.Stat(k.Cwd); err != nil || !info.IsDir() {
			continue // the folder is gone
		}
		live = append(live, k)
		worked[strings.ToLower(filepath.Clean(k.Cwd))] = true
	}
	latest := map[string]int{} // recent index by folder key
	for _, k := range live {
		dir := filepath.Clean(k.Cwd)
		// A subfolder counts for its repository when an agent worked at
		// the repository's root too.
		if root := repoRoot(dir); worked[strings.ToLower(root)] {
			dir = root
		}
		if i, ok := latest[strings.ToLower(dir)]; ok {
			if k.LastActivity.After(recent[i].LastActivity) {
				recent[i].LastActivity = k.LastActivity
			}
			continue
		}
		latest[strings.ToLower(dir)] = len(recent)
		add(&recent, Project{Path: dir, Name: filepath.Base(dir), Group: filepath.Base(filepath.Dir(dir)), Recent: true, LastActivity: k.LastActivity})
		roots = append(roots, filepath.Dir(dir))
	}
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].LastActivity.After(recent[j].LastActivity) })

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || skipped[strings.ToLower(e.Name())] {
				continue
			}
			add(&others, Project{Path: filepath.Join(root, e.Name()), Name: e.Name(), Group: filepath.Base(root)})
		}
	}
	sort.SliceStable(others, func(i, j int) bool {
		if others[i].Group != others[j].Group {
			return strings.ToLower(others[i].Group) < strings.ToLower(others[j].Group)
		}
		return strings.ToLower(others[i].Name) < strings.ToLower(others[j].Name)
	})
	return append(recent, others...)
}

// repoRoot is the root of the git repository holding dir, or dir itself
// when it is in no repository.
func repoRoot(dir string) string {
	dir = filepath.Clean(dir)
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}

// Filter keeps projects whose name or group contains q, ignoring case.
func Filter(all []Project, q string) []Project {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return all
	}
	var out []Project
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Name), q) || strings.Contains(strings.ToLower(p.Group), q) {
			out = append(out, p)
		}
	}
	return out
}
