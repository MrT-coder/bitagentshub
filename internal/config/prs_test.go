package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestThePRsSectionDefaultsToEverythingOn(t *testing.T) {
	cfg, err := Load(write(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	want := PRs{Enabled: true, Refresh: 2 * time.Minute}
	if !reflect.DeepEqual(cfg.PRs, want) {
		t.Errorf("prs = %+v, want %+v", cfg.PRs, want)
	}
}

func TestThePRsSectionSetsTheFilters(t *testing.T) {
	cfg, err := Load(write(t, `[prs]
enabled = false
orgs    = ["acme", "tools"]
repos   = ["MrT-coder/bitagentshub"]
refresh = "5m"
`))
	if err != nil {
		t.Fatal(err)
	}
	want := PRs{
		Enabled: false,
		Orgs:    []string{"acme", "tools"},
		Repos:   []string{"MrT-coder/bitagentshub"},
		Refresh: 5 * time.Minute,
	}
	if !reflect.DeepEqual(cfg.PRs, want) {
		t.Errorf("prs = %+v, want %+v", cfg.PRs, want)
	}
}

func TestThePRsSectionRefusesWhatItCannotUse(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"[prs]\nenabled = \"yes\"\n", "prs.enabled: yes is not true or false"},
		{"[prs]\norgs = \"acme\"\n", `prs.orgs: acme is not a list of names such as ["acme"]`},
		{"[prs]\norgs = [\"a/b\"]\n", `prs.orgs: "a/b" is not an organization name`},
		{"[prs]\nrepos = [\"bitagentshub\"]\n", `prs.repos: "bitagentshub" is not a repository such as "owner/name"`},
		{"[prs]\nrefresh = \"soon\"\n", `prs.refresh: "soon" is not a duration such as "2m"`},
		{"[prs]\nrefresh = \"10s\"\n", "prs.refresh: 10s is under the 30s minimum"},
		{"[prs]\ntoken = \"x\"\n", "prs.token: unknown field"},
	} {
		_, err := Load(write(t, tc.body))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: err = %v, want %q", tc.body, err, tc.want)
		}
	}
}
