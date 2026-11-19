package ui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/usage"
)

func TestTheFooterOffersHerdrOnlyWhileItIsConnected(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	if f := m.footer(); !strings.Contains(f, "g herdr") || !strings.Contains(f, "herdr: 2 linked") {
		t.Errorf("a connected herdr should show its key and links: %q", f)
	}
	m = update(m, herdrMsg{err: errors.New("no herdr here")})
	if f := m.footer(); strings.Contains(f, "herdr") {
		t.Errorf("without herdr running, the footer should not mention it: %q", f)
	}
	m = New(nil)
	if f := m.footer(); strings.Contains(f, "herdr") {
		t.Errorf("with herdr disabled, the footer should not mention it: %q", f)
	}
}

func TestTheUsageTabSaysHowToSeeClaudeLimits(t *testing.T) {
	m := spendingOffice(t)
	m = press(m, key("u"))
	if v := view(m); !strings.Contains(v, "Claude limits are off: turn them on in the settings tab.") {
		t.Errorf("without Claude limits the usage tab should say where to turn them on:\n%s", v)
	}

	m = spendingOffice(t, &fakeSource{ok: true, r: usage.Report{Tool: "claude", Today: -1, Windows: []usage.Window{{Name: "5h", UsedPercent: 10}}}})
	m = press(m, key("u"))
	if v := view(m); strings.Contains(v, "Claude limits are off") {
		t.Errorf("with Claude limits shown the hint should go away:\n%s", v)
	}
}

func TestTheFooterFitsAPhoneWidthWithTheKeysThatMatter(t *testing.T) {
	m := update(New(nil), tea.WindowSizeMsg{Width: 44, Height: 30})
	f := m.footer()
	if w := len([]rune(f)); w > m.width {
		t.Errorf("footer is %d columns on a %d column screen: %q", w, m.width, f)
	}
	for _, want := range []string{"q quit", "enter open", "p hub", "n new"} {
		if !strings.Contains(f, want) {
			t.Errorf("footer %q lacks %q", f, want)
		}
	}
}
