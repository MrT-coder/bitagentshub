package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/MrT-coder/bitagentshub/internal/statusline"
)

// stateDir holds what BitAgentsHub keeps between runs that is not config:
// the limits Claude Code reports and the status line it replaced.
func stateDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bitagentshub"), nil
}

// runStatusline handles "bah statusline [install|uninstall]".
// Without an argument it is the command Claude Code runs on every status
// line refresh.
func runStatusline(args []string) error {
	state, err := stateDir()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return statusline.Run(os.Stdin, os.Stdout, state)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	settings := filepath.Join(home, ".claude", "settings.json")
	switch args[0] {
	case "install":
		bin, err := installLimits(settings, state)
		if err != nil {
			return err
		}
		fmt.Printf("Claude Code's status line now runs %s; your settings were backed up to %s%s.\n", bin, settings, statusline.BackupSuffix)
		fmt.Println("Its 5-hour and weekly limits show in the hub's usage tab (u) after the next reply in a Claude session.")
		return nil
	case "uninstall":
		if err := statusline.Uninstall(settings, state); err != nil {
			return err
		}
		fmt.Println("Claude Code's previous status line is back.")
		return nil
	}
	return errors.New("usage: bah statusline [install|uninstall]")
}

// installLimits points Claude Code's status line at a copy of this
// executable, the same from the command line and the settings window.
func installLimits(settings, state string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	bin, err := copyForStatusline(exe, state)
	if err != nil {
		return "", err
	}
	return bin, statusline.Install(settings, state, statuslineCommand(bin))
}

// copyForStatusline copies the running executable next to the state, so
// Claude Code runs a file that rebuilding BitAgentsHub never has to replace
// (Windows refuses to overwrite a running executable).
func copyForStatusline(exe, state string) (string, error) {
	dir := filepath.Join(state, "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, "bah-statusline"+filepath.Ext(exe))
	in, err := os.Open(exe)
	if err != nil {
		return "", err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(dir, "statusline-*")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return dst, nil
}

// statuslineCommand is what Claude Code runs: the path quoted for bash, with
// forward slashes so Git Bash on Windows reads it as written.
func statuslineCommand(bin string) string {
	return fmt.Sprintf("%q statusline", filepath.ToSlash(bin))
}
