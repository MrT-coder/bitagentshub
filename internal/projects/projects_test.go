package projects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/claude"
)

func mkdirs(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(root, n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDiscover_ListsRecentProjectsFirstThenTheirSiblingFolders(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "acme/web", "acme/api", "acme/.git", "acme/node_modules", "solo/tool")
	now := time.Now()
	known := []claude.ProjectRef{
		{Cwd: filepath.Join(root, "acme", "api"), LastActivity: now.Add(-time.Hour)},
		{Cwd: filepath.Join(root, "solo", "tool"), LastActivity: now},
	}

	got := Discover(known, nil)

	var names []string
	for _, p := range got {
		names = append(names, p.Group+"/"+p.Name)
	}
	if strings.Join(names, ",") != "solo/tool,acme/api,acme/web" {
		t.Errorf("projects = %v", names)
	}
	if !got[0].Recent || got[2].Recent {
		t.Errorf("recent flags = %v %v, want recent sessions marked", got[0].Recent, got[2].Recent)
	}
}

func TestDiscover_AddsFoldersUnderExtraRoots(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "lab/one", "lab/two")
	got := Discover(nil, []string{filepath.Join(root, "lab")})
	if len(got) != 2 || got[0].Name != "one" || got[1].Name != "two" {
		t.Errorf("projects = %+v", got)
	}
}

func TestFilter_MatchesNameOrGroupIgnoringCase(t *testing.T) {
	all := []Project{{Name: "shopping-portal", Group: "acme-company"}, {Name: "minigames", Group: "hobbylabs"}}
	if got := Filter(all, "PORTAL"); len(got) != 1 || got[0].Name != "shopping-portal" {
		t.Errorf("filter by name = %+v", got)
	}
	if got := Filter(all, "HOBBY"); len(got) != 1 || got[0].Name != "minigames" {
		t.Errorf("filter by group = %+v", got)
	}
	if got := Filter(all, ""); len(got) != 2 {
		t.Errorf("empty filter = %+v", got)
	}
}

func TestDiscover_ShowsARepositoryOnceWhenAgentsWorkedInItsSubfolders(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "acme/api/.git", "acme/api/odd/tasks", "acme/web")
	now := time.Now()
	known := []claude.ProjectRef{
		{Cwd: filepath.Join(root, "acme", "api"), LastActivity: now.Add(-time.Hour)},
		{Cwd: filepath.Join(root, "acme", "api", "odd", "tasks"), LastActivity: now},
	}

	got := Discover(known, nil)

	var recent []Project
	for _, p := range got {
		if p.Recent {
			recent = append(recent, p)
		}
	}
	if len(recent) != 1 || recent[0].Path != filepath.Join(root, "acme", "api") || !recent[0].LastActivity.Equal(now) {
		t.Errorf("recent = %+v, want acme/api once with the latest activity", recent)
	}
}
