package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestCPUProfileIsWrittenWhenAskedFor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "prof")
	t.Setenv(cpuProfileEnv, dir)
	stop, err := profileCPU("office", 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	stop() // stopping again after the timer is harmless
	path := filepath.Join(dir, "office-"+strconv.Itoa(os.Getpid())+".pprof")
	if fi, err := os.Stat(path); err != nil || fi.Size() == 0 {
		t.Fatalf("no profile at %s: %v", path, err)
	}
}

func TestNoProfileUnlessAskedFor(t *testing.T) {
	t.Setenv(cpuProfileEnv, "")
	stop, err := profileCPU("office", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	stop()
}

func TestAProfileFolderThatCannotBeMadeIsReported(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(cpuProfileEnv, filepath.Join(file, "prof")) // under a file
	if _, err := profileCPU("office", time.Minute); err == nil {
		t.Error("expected an error for a folder that cannot be made")
	}
}
