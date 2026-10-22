package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sync"
	"time"
)

// cpuProfileEnv names a folder; when set, the office and its daemon each
// write a CPU profile of their first minute there, to find what costs time.
const cpuProfileEnv = "BAH_CPUPROFILE"

const cpuProfileFor = time.Minute

// profileCPU starts a CPU profile written to <folder>/<role>-<pid>.pprof that
// stops after d, or earlier through the returned stop. Without the variable
// it does nothing.
func profileCPU(role string, d time.Duration) (stop func(), err error) {
	dir := os.Getenv(cpuProfileEnv)
	if dir == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%s: %w", cpuProfileEnv, err)
	}
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%d.pprof", role, os.Getpid())))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", cpuProfileEnv, err)
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", cpuProfileEnv, err)
	}
	var once sync.Once
	stop = func() {
		once.Do(func() {
			pprof.StopCPUProfile()
			f.Close()
		})
	}
	time.AfterFunc(d, stop)
	return stop, nil
}
