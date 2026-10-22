package main

import (
	"strings"
	"testing"
)

func TestHelpListsTheSubcommands(t *testing.T) {
	var b strings.Builder
	printUsage(&b)
	for _, want := range []string{"usage: bah [flags]", "bah daemon status|stop|serve", "bah theme init <name>", "bah statusline install|uninstall", "flags:"} {
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
	} {
		if c.err == nil || !strings.Contains(c.err.Error(), c.want) {
			t.Errorf("bah %s = %v, want %q", c.name, c.err, c.want)
		}
	}
}
