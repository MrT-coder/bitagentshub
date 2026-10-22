package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStatuslineCommandQuotesTheCopyForBash(t *testing.T) {
	bin, want := `/home/me/.local/state/bitagentshub/bin/bah-statusline`, `"/home/me/.local/state/bitagentshub/bin/bah-statusline" statusline`
	if runtime.GOOS == "windows" {
		bin = `C:\Users\me\AppData\Local\bitagentshub\bin\bah-statusline.exe`
		want = `"C:/Users/me/AppData/Local/bitagentshub/bin/bah-statusline.exe" statusline`
	}
	if got := statuslineCommand(bin); got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}

func TestInstallCopyIsAStandaloneExecutable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.exe")
	if err := os.WriteFile(src, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	dst, err := copyForStatusline(src, filepath.Join(dir, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(dst); err != nil || string(b) != "binary" {
		t.Errorf("copy %s = %q, %v", dst, b, err)
	}
	// Rebuilding replaces the source; the copy Claude Code runs is not locked by it.
	if err := os.WriteFile(src, []byte("rebuilt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "binary" {
		t.Errorf("the copy should not follow the source: %q", b)
	}
}
