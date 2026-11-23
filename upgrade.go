package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/update"
	"github.com/MrT-coder/bitagentshub/internal/version"
)

// runUpgrade handles "bah upgrade": the latest release, checked against its
// checksums, in place of this executable.
func runUpgrade(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: bah upgrade")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	current := version.String()
	if !version.Release() {
		return fmt.Errorf("this bah (%s) was not installed from a release, so it is not replaced; upgrade it the way you built it", current)
	}
	tag, err := update.NewClient().Upgrade(context.Background(), current, exe, runtime.GOOS, runtime.GOARCH)
	if errors.Is(err, update.ErrUpToDate) {
		fmt.Printf("bah %s is the latest release.\n", current)
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("Upgraded bah %s to %s at %s.\n", current, tag, exe)
	fmt.Println("Restart the office to use it. Agents the daemon keeps run on until `bah daemon stop`.")
	return nil
}

// noAutoUpgradeEnv is set on the office an upgrade starts, so that one never
// upgrades again: a release that still reads as newer cannot loop.
const noAutoUpgradeEnv = "BAH_NO_AUTOUPGRADE"

// autoUpgrade installs a newer release before the office shows anything, so
// no agent is ever running in the way of a replacement. When one was
// installed it starts the new bah through relaunch and returns its exit
// code with true, and this one must exit with it. Any failure leaves this
// bah to run as it is: a missed upgrade is never worth a missed session.
func autoUpgrade(out io.Writer, current string, check func() string, upgrade func() (string, error), relaunch func() int) (int, bool) {
	tag := check()
	if tag == "" {
		return 0, false
	}
	fmt.Fprintf(out, "Upgrading bah %s to %s...\n", current, tag)
	got, err := upgrade()
	switch {
	case errors.Is(err, update.ErrUpToDate):
		return 0, false
	case err != nil:
		fmt.Fprintf(out, "Could not upgrade (%v); starting bah %s.\n", err, current)
		return 0, false
	}
	fmt.Fprintf(out, "Upgraded to %s.\n", got)
	return relaunch(), true
}

// upgradeAtStart is autoUpgrade with the real release check, this
// executable and the new bah as a child that owns the terminal.
func upgradeAtStart(state string) (int, bool) {
	if os.Getenv(noAutoUpgradeEnv) != "" || !version.Release() {
		return 0, false
	}
	exe, err := os.Executable()
	if err != nil {
		return 0, false
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return 0, false
	}
	current := version.String()
	upgrade := func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return update.NewClient().Upgrade(ctx, current, exe, runtime.GOOS, runtime.GOARCH)
	}
	relaunch := func() int {
		cmd := exec.Command(exe, os.Args[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.Env = append(os.Environ(), noAutoUpgradeEnv+"=1")
		err := cmd.Run()
		var exit *exec.ExitError
		switch {
		case errors.As(err, &exit):
			return exit.ExitCode()
		case err != nil:
			fmt.Fprintln(os.Stderr, "bah:", err)
			return 1
		}
		return 0
	}
	return autoUpgrade(os.Stderr, current, updateCheck(state), upgrade, relaunch)
}

// updateCheck is the office's check for a newer release: at most one
// request a day, remembered in the state folder, and quiet when offline.
func updateCheck(state string) func() string {
	return func() string {
		if !version.Release() {
			return ""
		}
		client := update.NewClient()
		client.HTTP.Timeout = 5 * time.Second
		latest := func() (string, error) {
			r, err := client.Latest(context.Background())
			return r.Tag, err
		}
		return update.Check(filepath.Join(state, "update-check.json"), version.String(), time.Now(), latest)
	}
}
