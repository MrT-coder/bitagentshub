package main

import (
	"strings"
	"testing"
)

func TestHelpListsTheSubcommands(t *testing.T) {
	var b strings.Builder
	printUsage(&b)
	for _, want := range []string{"usage: bah [flags]", "bah daemon status|stop|serve", "bah theme init <name>", "bah statusline install|uninstall", "bah upgrade", "flags:"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("help misses %q:\n%s", want, b.String())
		}
	}
}

func TestSubcommandsWithBadArgumentsSayHowToUseThem(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want string
	}{
		{"daemon", runDaemon(nil), "usage: bah daemon serve|status|stop"},
		{"daemon foo", runDaemon([]string{"foo"}), "usage: bah daemon serve|status|stop"},
		{"theme", runTheme(nil), "usage: bah theme init <name> [--from <theme>]"},
		{"theme init", runTheme([]string{"init"}), "usage: bah theme init <name> [--from <theme>]"},
		{"statusline foo", runStatusline([]string{"foo"}), "usage: bah statusline [install|uninstall]"},
		{"upgrade foo", runUpgrade([]string{"foo"}), "usage: bah upgrade"},
	} {
		if c.err == nil || !strings.Contains(c.err.Error(), c.want) {
			t.Errorf("bah %s = %v, want %q", c.name, c.err, c.want)
		}
	}
}

// A go install or local build has no release version, so upgrading would
// swap it for a binary it was not built as; it is refused before any
// request, and the office does not look for releases either.
func TestUpgradeLeavesBuildsThatAreNotReleasesAlone(t *testing.T) {
	err := runUpgrade(nil)
	if err == nil || !strings.Contains(err.Error(), "not installed from a release") {
		t.Errorf("bah upgrade on a test build = %v, want it refused", err)
	}
	if got := updateCheck(t.TempDir())(); got != "" {
		t.Errorf("update check on a test build = %q, want nothing", got)
	}
}
