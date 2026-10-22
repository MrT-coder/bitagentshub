package daemon

import (
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// IdleExit is how long a daemon with no office and no running program waits
// before it exits.
const IdleExit = 5 * time.Second

// Serve runs a daemon on addr until it is stopped or idle.
func Serve(addr string) error {
	ln, err := Listen(addr)
	if err != nil {
		return err
	}
	return NewServer(StartPTY, IdleExit).Serve(ln)
}

// Ensure makes sure a daemon answers on addr, starting one from a copy of
// this executable when none does. The copy lets the office's own binary be
// rebuilt while the daemon runs. ErrProto is returned as is: that daemon
// must be stopped by the user, since stopping it ends its programs.
func Ensure(addr string) error {
	_, _, err := List(addr)
	if err == nil || errors.Is(err, ErrProto) {
		return err
	}
	exe, err := daemonCopy()
	if err != nil {
		return fmt.Errorf("copy the daemon: %w", err)
	}
	if err := spawn(exe, []string{"daemon", "serve", "-addr", addr}); err != nil {
		return fmt.Errorf("start the daemon: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, _, err := List(addr)
		if err == nil || errors.Is(err, ErrProto) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// daemonCopy copies this executable into the user's cache, named after its
// size and time so each build gets its own copy, and removes older copies
// no daemon runs from anymore.
func daemonCopy() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	info, err := os.Stat(self)
	if err != nil {
		return "", err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "bitagentshub", "daemon")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%d-%d", info.Size(), info.ModTime().UnixNano())
	name := exeName(fmt.Sprintf("bah-daemon-%x", h.Sum64()))
	dst := filepath.Join(dir, name)
	if old, _ := filepath.Glob(filepath.Join(dir, "bah-daemon-*")); len(old) > 0 {
		for _, o := range old {
			if filepath.Base(o) != name {
				_ = os.Remove(o) // fails while a daemon still runs from it
			}
		}
	}
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	if err := copyFile(self, dst); err != nil {
		return "", err
	}
	return dst, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
