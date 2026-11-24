package office

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

func withTheme(t *testing.T, th Theme) PRColors {
	t.Helper()
	t.Cleanup(func() { SetTheme(BuiltinTheme()) })
	SetTheme(th)
	return PR()
}

func TestTheBuiltinThemeGivesEachStateItsGitHubColor(t *testing.T) {
	c := withTheme(t, BuiltinTheme())
	for name, got := range map[string]screen.RGB{"open": c.Open, "draft": c.Draft, "merged": c.Merged, "closed": c.Closed, "pending": c.Pending} {
		if r := contrast(got, BuiltinTheme().UIBg); r < contrastMin {
			t.Errorf("%s %s has contrast %.1f on the window, want %.1f", name, formatColor(got), r, contrastMin)
		}
	}
	if c.Open.G <= c.Open.R || c.Merged.B <= c.Merged.G || c.Closed.R <= c.Closed.G {
		t.Errorf("open should be green, merged purple, closed red: %+v", c)
	}
}

func TestStateColorsComeFromTheTerminalPaletteWhenTheThemeHasOne(t *testing.T) {
	th := BuiltinTheme()
	th.Terminal.Set = true
	th.Terminal.C[2], th.Terminal.C[1], th.Terminal.C[5] = rgb(0x00ff00), rgb(0xff5555), rgb(0xff00ff)
	th.Terminal.C[3], th.Terminal.C[8] = rgb(0xffff00), rgb(0xaaaaaa)
	c := withTheme(t, th)
	if c.Open != rgb(0x00ff00) || c.Closed != rgb(0xff5555) || c.Merged != rgb(0xff00ff) || c.Pending != rgb(0xffff00) || c.Draft != rgb(0xaaaaaa) {
		t.Errorf("state colors %+v, want the palette's green, red, magenta, yellow and bright black", c)
	}
}

func TestADerivedStateColorIsMovedUntilItReadsOnALightWindow(t *testing.T) {
	th := BuiltinTheme()
	th.UIBg, th.UIText = rgb(0xffffff), rgb(0x111111)
	c := withTheme(t, th)
	for name, got := range map[string]screen.RGB{"open": c.Open, "draft": c.Draft, "merged": c.Merged, "closed": c.Closed, "pending": c.Pending} {
		if r := contrast(got, th.UIBg); r < contrastMin {
			t.Errorf("%s %s has contrast %.1f on white, want %.1f", name, formatColor(got), r, contrastMin)
		}
	}
}

func TestAStateColorTheThemeSetsIsKeptAsWritten(t *testing.T) {
	dir := fstest.MapFS{"theme.toml": {Data: []byte("[ui]\npr_merged = \"#202020\"\n")}}
	th, err := loadTheme(dir, "x", "x")
	if err != nil {
		t.Fatal(err)
	}
	c := withTheme(t, th)
	if c.Merged != rgb(0x202020) {
		t.Errorf("merged = %s, want the theme's #202020 untouched", formatColor(c.Merged))
	}
	if c.Open == rgb(0x202020) || c.Open != readable(rgb(0x3fb950), th.UIBg, th.UIText) {
		t.Errorf("open = %s, want the derived default", formatColor(c.Open))
	}
}

func TestTheThemeFileWritesTheStateColors(t *testing.T) {
	var sb strings.Builder
	bt := BuiltinTheme()
	for _, f := range bt.colorFields() {
		sb.WriteString(f.section + "." + f.key + "\n")
	}
	for _, k := range []string{"ui.pr_open", "ui.pr_draft", "ui.pr_merged", "ui.pr_closed", "ui.pr_pending"} {
		if !strings.Contains(sb.String(), k+"\n") {
			t.Errorf("colorFields lacks %s", k)
		}
	}
}
