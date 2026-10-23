package main

import (
	"context"
	"errors"
	"fmt"
	"os"
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
