package config

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultPRRefresh = 2 * time.Minute
	minPRRefresh     = 30 * time.Second
)

// PRs is [prs]: the pull request review window. Orgs and Repos narrow which
// pull requests it shows and alerts about; both empty means every one.
type PRs struct {
	Enabled bool
	Orgs    []string // organizations or users, such as "acme"
	Repos   []string // repositories as "owner/name"
	Refresh time.Duration
}

// prsSection reads [prs]. Without it the window is on, unfiltered, and
// refreshes every two minutes.
func prsSection(section map[string]any) (PRs, error) {
	p := PRs{Enabled: true, Refresh: defaultPRRefresh}
	for key, v := range section {
		switch key {
		case "enabled":
			b, ok := v.(bool)
			if !ok {
				return PRs{}, fmt.Errorf("prs.enabled: %v is not true or false", v)
			}
			p.Enabled = b
		case "orgs", "repos":
			names, err := nameList(v)
			if err != nil {
				return PRs{}, fmt.Errorf("prs.%s: %w", key, err)
			}
			for _, n := range names {
				if err := checkPRName(key, n); err != nil {
					return PRs{}, err
				}
			}
			if key == "orgs" {
				p.Orgs = names
			} else {
				p.Repos = names
			}
		case "refresh":
			s, _ := v.(string)
			d, err := time.ParseDuration(s)
			if err != nil || d <= 0 {
				return PRs{}, fmt.Errorf("prs.refresh: %v is not a duration such as \"2m\"", quoted(v))
			}
			if d < minPRRefresh {
				return PRs{}, fmt.Errorf("prs.refresh: %v is under the %v minimum", d, minPRRefresh)
			}
			p.Refresh = d
		default:
			return PRs{}, fmt.Errorf("prs.%s: unknown field", key)
		}
	}
	return p, nil
}

// quoted spells a TOML value the way the errors do: strings in quotes.
func quoted(v any) string {
	if s, ok := v.(string); ok {
		return fmt.Sprintf("%q", s)
	}
	return fmt.Sprint(v)
}

// nameList reads a TOML array of strings.
func nameList(v any) ([]string, error) {
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%v is not a list of names such as [\"acme\"]", v)
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			return nil, fmt.Errorf("%v is not a list of names such as [\"acme\"]", v)
		}
		out = append(out, strings.TrimSpace(s))
	}
	return out, nil
}

func checkPRName(key, name string) error {
	slashes := strings.Count(name, "/")
	switch {
	case key == "orgs" && (name == "" || slashes > 0 || strings.ContainsAny(name, " \t")):
		return fmt.Errorf("prs.orgs: %q is not an organization name", name)
	case key == "repos" && (slashes != 1 || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.ContainsAny(name, " \t")):
		return fmt.Errorf("prs.repos: %q is not a repository such as \"owner/name\"", name)
	}
	return nil
}
