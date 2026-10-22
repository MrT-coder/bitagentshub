//go:build !windows

package daemon

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// DefaultAddr is this user's daemon socket, in a directory only they can
// enter.
func DefaultAddr() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("bah-%d", os.Getuid()), "daemon.sock")
}

// Listen opens the socket. A socket nobody answers on is left from a daemon
// that died and is replaced; a live one means a daemon already runs.
func Listen(addr string) (net.Listener, error) {
	dir := filepath.Dir(addr)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	if c, err := net.DialTimeout("unix", addr, dialTimeout); err == nil {
		c.Close()
		return nil, errors.New("a daemon already listens on " + addr)
	}
	_ = os.Remove(addr)
	return net.Listen("unix", addr)
}

func dialAddr(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("unix", addr, timeout)
}

// spawn starts the daemon in its own session, so closing the terminal or the
// SSH connection does not signal it.
func spawn(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func exeName(stem string) string { return stem }
