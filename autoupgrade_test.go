package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/update"
)

func TestAutoUpgradeInstallsANewerReleaseThenStartsIt(t *testing.T) {
	var out strings.Builder
	upgraded := false
	code, took := autoUpgrade(&out, "v0.8.0",
		func() string { return "v0.9.0" },
		func() (string, error) { upgraded = true; return "v0.9.0", nil },
		func() int {
			if !upgraded {
				t.Error("the new bah started before the upgrade was installed")
			}
			return 3
		})
	if !took || code != 3 {
		t.Errorf("took=%v code=%d, want the new bah's exit code 3", took, code)
	}
	for _, want := range []string{"Upgrading bah v0.8.0 to v0.9.0", "Upgraded to v0.9.0"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output misses %q:\n%s", want, out.String())
		}
	}
}

func TestAutoUpgradeDoesNothingWithoutANewerRelease(t *testing.T) {
	var out strings.Builder
	_, took := autoUpgrade(&out, "v0.8.0",
		func() string { return "" },
		func() (string, error) { t.Error("upgraded with no newer release"); return "", nil },
		func() int { t.Error("started a new bah"); return 0 })
	if took || out.Len() != 0 {
		t.Errorf("took=%v output=%q, want a silent start", took, out.String())
	}
}

func TestAFailedAutoUpgradeStartsTheCurrentBahAndSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{errors.New("no network"), "Could not upgrade (no network); starting bah v0.8.0."},
		{update.ErrUpToDate, ""},
	} {
		var out strings.Builder
		_, took := autoUpgrade(&out, "v0.8.0",
			func() string { return "v0.9.0" },
			func() (string, error) { return "", tc.err },
			func() int { t.Error("started a new bah after a failed upgrade"); return 0 })
		if took {
			t.Errorf("%v: took the place of the current bah", tc.err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Errorf("%v: output %q misses %q", tc.err, out.String(), tc.want)
		}
	}
}

// A go install or local build is not a release, so it is never replaced.
func TestUpgradeAtStartLeavesBuildsThatAreNotReleasesAlone(t *testing.T) {
	if _, took := upgradeAtStart(t.TempDir()); took {
		t.Error("a test build was replaced")
	}
}

func TestUpgradeAtStartNeverLoopsInTheBahItStarted(t *testing.T) {
	t.Setenv(noAutoUpgradeEnv, "1")
	if _, took := upgradeAtStart(t.TempDir()); took {
		t.Error("upgraded again inside the bah an upgrade started")
	}
}
