package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakePwsh(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "pwsh.exe")
	if err := os.WriteFile(p, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// The Microsoft Store PowerShell sits under WindowsApps (its alias and its
// package folder) and ConPTY cannot start it: "Access is denied".
func TestFindPwshSkipsTheStoreInstall(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(root, "Local", "Microsoft", "WindowsApps")
	pkg := filepath.Join(root, "Program Files", "WindowsApps", "Microsoft.PowerShell_7.6.7.0_x64__8wekyb3d8bbwe")
	empty := filepath.Join(root, "empty")
	fakePwsh(t, alias)
	fakePwsh(t, pkg)
	real := fakePwsh(t, filepath.Join(root, "Program Files", "PowerShell", "7"))
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}

	path := strings.Join([]string{pkg, alias, empty, filepath.Dir(real)}, string(os.PathListSeparator))
	if got := findPwsh(path); got != real {
		t.Fatalf("findPwsh = %q, want %q", got, real)
	}
	onlyStore := strings.Join([]string{pkg, alias}, string(os.PathListSeparator))
	if got := findPwsh(onlyStore); got != "" {
		t.Fatalf("findPwsh with only the Store install = %q, want none", got)
	}
}
