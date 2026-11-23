package ui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/prs"
)

// prNode is one pull request whose review is asked of "me".
func prNode(repo string, n int, title string) string {
	return fmt.Sprintf(`{"number":%d,"title":%q,"url":"https://github.com/%s/pull/%d","isDraft":false,
"updatedAt":"2026-11-20T10:00:00Z","reviewDecision":"","repository":{"nameWithOwner":%q},
"author":{"login":"ana"},"reviewRequests":{"nodes":[{"requestedReviewer":{"login":"me"}}]},
"commits":{"nodes":[]}}`, n, title, repo, n, repo)
}

// ghAnswering is a gh that answers every call with the same pull requests.
func ghAnswering(nodes ...string) (prs.Runner, *int) {
	calls := 0
	return func(context.Context, ...string) ([]byte, error) {
		calls++
		return []byte(`{"data":{"viewer":{"login":"me"},"search":{"nodes":[` + strings.Join(nodes, ",") + `]}}}`), nil
	}, &calls
}

func prsOffice(t *testing.T, cfg config.PRs, run prs.Runner) Model {
	t.Helper()
	m := New(nil).WithPRs(cfg, run)
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	return update(m, snapshotMsg(claude.Snapshot{Files: 1, Agents: []claude.Agent{{ID: "a", Project: "p", Group: "g", State: claude.StateWaiting}}}))
}

var prsOn = config.PRs{Enabled: true, Refresh: time.Minute}

func TestThePRsTabListsWhatWaitsOnYouByGroup(t *testing.T) {
	run, _ := ghAnswering(prNode("acme/api", 7, "Fix login"), prNode("acme/api", 8, "Add cache"))
	m := press(prsOffice(t, prsOn, run), key("r")) // r on the board opens the PRs tab
	for _, want := range []string{"Needs your review (2)", "acme/api#7", "Fix login", "acme/api#8", "Ready to merge (0)", "Showing: every pull request"} {
		if !strings.Contains(view(m), want) {
			t.Errorf("view missing %q:\n%s", want, view(m))
		}
	}
}

func TestThePRsTabSaysWhatGHNeeds(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{exec.ErrNotFound, "gh is not installed"},
		{errors.New("exit status 1"), "gh is not logged in"},
	} {
		run := func(context.Context, ...string) ([]byte, error) { return nil, tc.err }
		m := press(prsOffice(t, prsOn, run), key("r"))
		if !strings.Contains(view(m), tc.want) {
			t.Errorf("err %v: view missing %q:\n%s", tc.err, tc.want, view(m))
		}
	}
}

func TestThePRsTabNamesTheFilter(t *testing.T) {
	run, _ := ghAnswering()
	cfg := config.PRs{Enabled: true, Refresh: time.Minute, Orgs: []string{"acme"}, Repos: []string{"me/side"}}
	m := press(prsOffice(t, cfg, run), key("r"))
	if !strings.Contains(view(m), "Showing: org acme · repo me/side") {
		t.Errorf("view does not name the filter:\n%s", view(m))
	}
}

func TestThePRsTabMovesItsSelectionWithTheArrows(t *testing.T) {
	run, _ := ghAnswering(prNode("acme/api", 7, "Fix login"), prNode("acme/api", 8, "Add cache"))
	m := press(prsOffice(t, prsOn, run), key("r"))
	marked := func() string {
		for _, l := range strings.Split(view(m), "\n") {
			if strings.Contains(l, "▸ acme/api#") {
				return l
			}
		}
		return ""
	}
	if !strings.Contains(marked(), "#7") {
		t.Fatalf("the first PR is not selected: %q", marked())
	}
	m = press(m, key("down"))
	if !strings.Contains(marked(), "#8") {
		t.Errorf("down did not select the second PR: %q", marked())
	}
	m = press(press(m, key("down")), key("up"))
	if !strings.Contains(marked(), "#7") {
		t.Errorf("up did not come back to the first PR: %q", marked())
	}
}

func TestClickingAPRSelectsIt(t *testing.T) {
	run, _ := ghAnswering(prNode("acme/api", 7, "Fix login"), prNode("acme/api", 8, "Add cache"))
	m := press(prsOffice(t, prsOn, run), key("r"))
	x, y, _, _ := m.hubRect()
	// rows: filter line, group header, #7, #8
	m = press(m, click(1+x+4, officeY+y+2+3))
	if !strings.Contains(view(m), "▸ acme/api#8") {
		t.Errorf("the click did not select #8:\n%s", view(m))
	}
}

func TestAFailedRefreshKeepsTheListAndSaysWhy(t *testing.T) {
	run, _ := ghAnswering(prNode("acme/api", 7, "Fix login"))
	m := press(prsOffice(t, prsOn, run), key("r"))
	m = update(m, prsMsg{status: prs.Ready, err: errors.New("network down")})
	for _, want := range []string{"Fix login", "Could not read pull requests: network down"} {
		if !strings.Contains(view(m), want) {
			t.Errorf("view missing %q:\n%s", want, view(m))
		}
	}
}

func TestThePRsTabIsQuietWhenTurnedOff(t *testing.T) {
	run, calls := ghAnswering(prNode("acme/api", 7, "Fix login"))
	cfg := config.PRs{Enabled: false, Refresh: time.Minute}
	m := press(prsOffice(t, cfg, run), key("r"))
	if *calls != 0 {
		t.Errorf("gh ran %d times although [prs] is off", *calls)
	}
	if !strings.Contains(view(m), "Pull requests are off") {
		t.Errorf("view does not say it is off:\n%s", view(m))
	}
}

func TestThePRsPollRearmsItselfButARefreshByHandDoesNot(t *testing.T) {
	run, _ := ghAnswering()
	m := prsOffice(t, prsOn, run)
	if _, cmd := m.Update(prsMsg{status: prs.Ready}); cmd == nil {
		t.Error("the periodic poll does not schedule the next one")
	}
	if _, cmd := m.Update(prsMsg{status: prs.Ready, manual: true}); cmd != nil {
		t.Error("a refresh by hand started a second poll chain")
	}
}

func TestWithoutPRsSetUpTheTabSaysSo(t *testing.T) {
	m := New(nil)
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = update(m, snapshotMsg(claude.Snapshot{Files: 1, Agents: []claude.Agent{{ID: "a", Project: "p", Group: "g", State: claude.StateWaiting}}}))
	m, _ = m.openHub(tabPRs)
	if !strings.Contains(view(m), "Pull requests are off") {
		t.Errorf("view:\n%s", view(m))
	}
}
