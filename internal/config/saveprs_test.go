package config_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/config"
)

func TestSavePRsFilterCreatesTheSectionAndTheFilterLoads(t *testing.T) {
	path := configFile(t, "")
	if err := config.SavePRsFilter(path, []string{"acme", "tools"}, []string{"me/side"}); err != nil {
		t.Fatal(err)
	}
	want := "[prs]\norgs = [\"acme\", \"tools\"]\nrepos = [\"me/side\"]\n"
	if got := contents(t, path); got != want {
		t.Errorf("config.toml =\n%q\nwant\n%q", got, want)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.PRs.Orgs, []string{"acme", "tools"}) || !reflect.DeepEqual(cfg.PRs.Repos, []string{"me/side"}) {
		t.Errorf("Load().PRs = %+v", cfg.PRs)
	}
}

func TestSavePRsFilterEditsOnlyItsLinesAndKeepsTheRest(t *testing.T) {
	body := "# mine\n[prs]\norgs = [\"old\"] # why\nrefresh = \"5m\"\n\n[rest]\nenabled = false\n"
	path := configFile(t, body)
	if err := config.SavePRsFilter(path, []string{"acme"}, []string{"me/side"}); err != nil {
		t.Fatal(err)
	}
	want := "# mine\n[prs]\nrepos = [\"me/side\"]\norgs = [\"acme\"] # why\nrefresh = \"5m\"\n\n[rest]\nenabled = false\n"
	if got := contents(t, path); got != want {
		t.Errorf("config.toml =\n%q\nwant\n%q", got, want)
	}
}

func TestSavePRsFilterWithNothingRemovesTheFilter(t *testing.T) {
	path := configFile(t, "[prs]\norgs = [\"acme\"]\nrepos = [\"me/side\"]\nrefresh = \"5m\"\n")
	if err := config.SavePRsFilter(path, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := contents(t, path), "[prs]\nrefresh = \"5m\"\n"; got != want {
		t.Errorf("config.toml =\n%q\nwant\n%q", got, want)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PRs.Orgs)+len(cfg.PRs.Repos) != 0 {
		t.Errorf("the filter is still %+v", cfg.PRs)
	}
}

func TestSavePRsFilterRefusesNamesItCannotUseAndLeavesTheFile(t *testing.T) {
	for _, tc := range []struct {
		orgs, repos []string
		want        string
	}{
		{[]string{"a/b"}, nil, `prs.orgs: "a/b" is not an organization name`},
		{nil, []string{"bitagentshub"}, `prs.repos: "bitagentshub" is not a repository such as "owner/name"`},
	} {
		body := "[prs]\norgs = [\"acme\"]\n"
		path := configFile(t, body)
		err := config.SavePRsFilter(path, tc.orgs, tc.repos)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("err = %v, want one mentioning %q", err, tc.want)
		}
		if got := contents(t, path); got != body {
			t.Errorf("the file changed to %q", got)
		}
	}
}

func TestSavePRsFilterRefusesAFileItCannotEditSafely(t *testing.T) {
	body := "prs.orgs = [\"acme\"]\n"
	path := configFile(t, body)
	if err := config.SavePRsFilter(path, []string{"tools"}, nil); err == nil {
		t.Error("a dotted key was edited as if it were a [prs] table")
	}
	if got := contents(t, path); got != body {
		t.Errorf("the file changed to %q", got)
	}
}
