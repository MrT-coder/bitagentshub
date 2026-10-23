package ui

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/config"
)

// fakeSettings records what the settings window asks for and answers with
// scripted values and errors.
type fakeSettings struct {
	themes   []string
	scenes   []string // nil: every theme
	active   string
	limits   bool
	applyErr error
	newErr   error
	limitErr error
	calls    []string
	path     string // config.toml that SetKeys writes, in a test folder
	language string
	langErr  error
	petName  string
	palette  string
}

// Scenes are scenes when set, otherwise every theme has shapes of its own.
func (f *fakeSettings) Scenes() ([]string, error) {
	if f.scenes != nil {
		return slices.Clone(f.scenes), nil
	}
	return slices.Clone(f.themes), nil
}

func (f *fakeSettings) ActivePalette() string { return f.palette }

func (f *fakeSettings) ApplyPalette(name string) error {
	f.calls = append(f.calls, "palette "+name)
	if f.applyErr != nil {
		return f.applyErr
	}
	f.palette = name
	return nil
}

func (f *fakeSettings) SetPetName(name string) error {
	f.calls = append(f.calls, "pet "+name)
	f.petName = name
	return nil
}

func (f *fakeSettings) ConfigPath() string        { return `C:\cfg\bitagentshub\config.toml` }
func (f *fakeSettings) Themes() ([]string, error) { return slices.Clone(f.themes), nil }
func (f *fakeSettings) ActiveTheme() string       { return f.active }

func (f *fakeSettings) ApplyTheme(name string) error {
	f.calls = append(f.calls, "apply "+name)
	if f.applyErr != nil {
		return f.applyErr
	}
	f.active = name
	return nil
}

func (f *fakeSettings) NewTheme(name string) error {
	f.calls = append(f.calls, "new "+name)
	if f.newErr != nil {
		return f.newErr
	}
	f.themes = append(f.themes, name)
	slices.Sort(f.themes)
	return nil
}

func (f *fakeSettings) LimitsInstalled() (bool, error) { return f.limits, nil }

func (f *fakeSettings) ActiveLanguage() string {
	if f.language == "" {
		return "en"
	}
	return f.language
}

func (f *fakeSettings) ApplyLanguage(lang string) error {
	f.calls = append(f.calls, "language "+lang)
	if f.langErr != nil {
		return f.langErr
	}
	f.language = lang
	return nil
}

func (f *fakeSettings) SetLimits(on bool) error {
	if on {
		f.calls = append(f.calls, "limits on")
	} else {
		f.calls = append(f.calls, "limits off")
	}
	if f.limitErr != nil {
		return f.limitErr
	}
	f.limits = on
	return nil
}

// SetKeys saves in a real config.toml so clashes are refused as they are
// in the office.
func (f *fakeSettings) SetKeys(action string, keys []string) (config.Keymap, error) {
	f.calls = append(f.calls, strings.TrimSpace("keys "+action+" "+strings.Join(keys, ",")))
	if err := config.SaveKeys(f.path, action, keys); err != nil {
		return config.Keymap{}, err
	}
	cfg, err := config.Load(f.path)
	return cfg.Keys, err
}

func settingsOffice(t *testing.T, f *fakeSettings) Model {
	t.Helper()
	f.path = filepath.Join(t.TempDir(), "config.toml")
	m, _, _ := twoAgentOffice(t)
	return m.WithSettings(f)
}

func typeText(m Model, s string) Model {
	for _, r := range s {
		m = press(m, key(string(r)))
	}
	return m
}

func TestSettingsOpenFromTheBoardAndCloseWithEscOrS(t *testing.T) {
	m := settingsOffice(t, &fakeSettings{})
	m = press(m, key("s"))
	if !strings.Contains(view(m), "settings") || !strings.Contains(view(m), "default (built-in)") {
		t.Fatalf("s should open the settings window:\n%s", view(m))
	}
	m = press(m, key("esc"))
	if strings.Contains(view(m), "default (built-in)") {
		t.Fatalf("esc should close the settings window:\n%s", view(m))
	}
	m = press(press(m, key("s")), key("s"))
	if strings.Contains(view(m), "default (built-in)") {
		t.Errorf("s again should close the settings window:\n%s", view(m))
	}
}

func TestSettingsOpenFromAWindowWithThePrefixAndLeaveItOpen(t *testing.T) {
	m := settingsOffice(t, &fakeSettings{})
	m = press(m, key("enter"))
	m = prefixed(m, "s")
	if !strings.Contains(view(m), "default (built-in)") {
		t.Fatalf("ctrl+a s should open the settings window:\n%s", view(m))
	}
	m = prefixed(m, "s")
	if strings.Contains(view(m), "default (built-in)") || m.win == nil {
		t.Errorf("ctrl+a s again should close settings and leave the window:\n%s", view(m))
	}
}

func TestSettingsSwallowKeysPastesAndClicks(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = m.WithSettings(&fakeSettings{})
	m = press(m, key("enter"))
	m = prefixed(m, "s")
	term := rec.started[0]
	keys, mouse := len(term.keys), len(term.mouse)
	m = press(m, key("x"))
	m = press(m, tea.PasteMsg{Content: "hello"})
	m = press(m, click(20, 15))
	if len(term.keys) != keys || len(term.mouse) != mouse {
		t.Errorf("nothing should reach the program under settings, keys %q mouse %q", term.keys[keys:], term.mouse[mouse:])
	}
	if m.win == nil {
		t.Fatal("the window should stay open under settings")
	}

	b := settingsOffice(t, &fakeSettings{})
	sel := b.selected
	b = press(b, key("s"))
	b = press(b, key("right"))
	b = press(b, key("n"))
	b = press(b, key("esc")) // cancels the name input only
	if b.selected != sel || b.picker != nil {
		t.Errorf("board keys should not act under settings: selected %q picker %v", b.selected, b.picker != nil)
	}
}

func TestSettingsListThemesWithTheActiveOneMarked(t *testing.T) {
	m := settingsOffice(t, &fakeSettings{themes: []string{"night", "retro"}, active: "retro"})
	m = press(m, key("s"))
	v := view(m)
	iDef, iNight, iRetro := strings.Index(v, "default (built-in)"), strings.Index(v, "night"), strings.Index(v, "retro")
	if iDef < 0 || iNight < 0 || iRetro < 0 || !(iDef < iNight && iNight < iRetro) {
		t.Fatalf("themes should list the built-in one first, then by name:\n%s", v)
	}
	if !strings.Contains(v, "● retro") || strings.Contains(v, "● night") || strings.Contains(v, "● default") {
		t.Errorf("only the active theme should be marked:\n%s", v)
	}
	for _, want := range []string{`C:\cfg\bitagentshub\config.toml`, "ctrl+a", "close"} {
		if !strings.Contains(v, want) {
			t.Errorf("settings missing %q:\n%s", want, v)
		}
	}
}

func TestSettingsEnterAppliesTheSelectedTheme(t *testing.T) {
	f := &fakeSettings{themes: []string{"night", "retro"}}
	m := settingsOffice(t, f)
	m = press(m, key("s"))
	m = press(m, key("down"))
	m = press(m, key("down"))
	m = press(m, key("enter"))
	if !slices.Equal(f.calls, []string{"apply retro"}) {
		t.Fatalf("calls = %q, want apply retro", f.calls)
	}
	if !strings.Contains(view(m), "● retro") {
		t.Errorf("the applied theme should be marked active:\n%s", view(m))
	}
	m = press(m, key("up"))
	m = press(m, key("up"))
	m = press(m, key("enter"))
	if !slices.Equal(f.calls, []string{"apply retro", "apply "}) {
		t.Errorf("the built-in theme should apply as \"\", calls = %q", f.calls)
	}
}

func TestSettingsShowAThemeErrorAndChangeNothing(t *testing.T) {
	f := &fakeSettings{themes: []string{"broken"}, active: "", applyErr: errors.New("theme broken: line 3: bad color")}
	m := settingsOffice(t, f)
	m = press(m, key("s"))
	m = press(m, key("down"))
	m = press(m, key("enter"))
	v := view(m)
	if !strings.Contains(v, "line 3: bad color") {
		t.Errorf("the error should show in the window:\n%s", v)
	}
	if !strings.Contains(v, "● default (built-in)") || strings.Contains(v, "● broken") {
		t.Errorf("a failed theme should leave the active one as it was:\n%s", v)
	}
	if !strings.Contains(v, "default (built-in)") {
		t.Error("the window should stay open")
	}
}

func TestSettingsNewThemeAsksForANameAndSelectsIt(t *testing.T) {
	f := &fakeSettings{themes: []string{"night", "retro"}, active: "night"}
	m := settingsOffice(t, f)
	m = press(m, key("s"))
	m = press(m, key("n"))
	m = typeText(m, "oceanx")
	m = press(m, key("backspace"))
	if !strings.Contains(view(m), "ocean") || strings.Contains(view(m), "oceanx") {
		t.Fatalf("the name input should show the typed name:\n%s", view(m))
	}
	m = press(m, key("enter"))
	if !slices.Equal(f.calls, []string{"new ocean"}) {
		t.Fatalf("calls = %q, want new ocean", f.calls)
	}
	if !strings.Contains(view(m), "● night") {
		t.Errorf("creating a theme should not apply it:\n%s", view(m))
	}
	m = press(m, key("enter")) // the new theme is selected: enter applies it
	if !slices.Equal(f.calls, []string{"new ocean", "apply ocean"}) {
		t.Errorf("the new theme should be selected, calls = %q", f.calls)
	}

	f.newErr = errors.New(`themes/ocean already exists`)
	m = press(m, key("n"))
	m = typeText(m, "ocean")
	m = press(m, key("enter"))
	if !strings.Contains(view(m), "already exists") {
		t.Errorf("a refused name should show why:\n%s", view(m))
	}
}

func TestSettingsEscCancelsTheNameInputOnly(t *testing.T) {
	f := &fakeSettings{}
	m := settingsOffice(t, f)
	m = press(m, key("s"))
	m = press(m, key("n"))
	m = typeText(m, "abc")
	m = press(m, key("esc"))
	if !strings.Contains(view(m), "default (built-in)") {
		t.Fatalf("esc in the name input should keep settings open:\n%s", view(m))
	}
	m = press(m, key("enter")) // applies the built-in theme, no theme is created
	if !slices.Equal(f.calls, []string{"apply "}) {
		t.Errorf("calls = %q, want only apply", f.calls)
	}
	m = press(m, key("esc"))
	if strings.Contains(view(m), "default (built-in)") {
		t.Error("esc again should close settings")
	}
}

func TestSettingsLimitsToggleByCurrentState(t *testing.T) {
	f := &fakeSettings{}
	m := settingsOffice(t, f)
	m = press(m, key("s"))
	if !strings.Contains(view(m), "Claude limits: off") || !strings.Contains(view(m), "backed up") {
		t.Fatalf("settings should say limits are off and that settings.json is backed up:\n%s", view(m))
	}
	m = press(press(press(m, key("tab")), key("tab")), key("tab"))
	m = press(m, key("enter"))
	if !slices.Equal(f.calls, []string{"limits on"}) || !strings.Contains(view(m), "Claude limits: on") {
		t.Fatalf("enter should turn limits on, calls = %q:\n%s", f.calls, view(m))
	}
	m = press(m, key("enter"))
	if !slices.Equal(f.calls, []string{"limits on", "limits off"}) {
		t.Errorf("enter again should turn limits off, calls = %q", f.calls)
	}
	f.limitErr = errors.New("settings.json: permission denied")
	m = press(m, key("enter"))
	if v := view(m); !strings.Contains(v, "permission denied") || !strings.Contains(v, "Claude limits: off") {
		t.Errorf("a failed toggle should show why and keep the state:\n%s", v)
	}
}

func TestSettingsWithoutAHookSayTheyAreUnavailable(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(m, key("s"))
	v := view(m)
	if !strings.Contains(v, "not available") {
		t.Fatalf("settings with nothing wired should say so:\n%s", v)
	}
	m = press(m, key("enter"))
	m = press(m, key("n"))
	m = press(m, key("esc"))
	if strings.Contains(view(m), "not available") {
		t.Errorf("esc should close the unavailable settings:\n%s", view(m))
	}
}

func TestSettingsAndUsageNeverShowTogether(t *testing.T) {
	m := settingsOffice(t, &fakeSettings{})
	m = press(m, key("enter"))
	m = prefixed(m, "u")
	m = prefixed(m, "s")
	if !strings.Contains(view(m), "default (built-in)") || (m.hubOpen && m.hubTab == tabUsage) {
		t.Fatalf("opening settings should close usage:\n%s", view(m))
	}
	m = prefixed(m, "u")
	if !(m.hubOpen && m.hubTab == tabUsage) || strings.Contains(view(m), "default (built-in)") {
		t.Errorf("opening usage should close settings:\n%s", view(m))
	}
}

func TestSettingsMarkTheThemesThatComeWithTheOffice(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	f := &fakeSettings{themes: []string{"night", "mine"}}
	m = m.WithSettings(f)
	m = press(m, key("s"))
	v := view(m)
	if !strings.Contains(v, "night (built-in)") || strings.Contains(v, "mine (built-in)") {
		t.Errorf("only the office's own themes are marked built-in:\n%s", v)
	}
}

func TestSettingsShowTheOutcomeAndConfigPathOnASmallTerminal(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {44, 24}} {
		m, _, _ := twoAgentOffice(t)
		m = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = m.WithSettings(&fakeSettings{themes: []string{"night", "retro", "a", "b", "c"}, applyErr: errors.New("BOOMERR")})
		m = press(m, key("s"))
		m = press(m, key("down"))
		m = press(m, key("enter"))
		v := view(m)
		if !strings.Contains(v, "BOOMERR") || !strings.Contains(v, "config.toml") {
			t.Errorf("%dx%d: the error and the config path must stay visible:\n%s", size[0], size[1], v)
		}
	}
}

// keysSection opens settings on the board and moves to its Keys section.
func keysSection(t *testing.T, f *fakeSettings) Model {
	t.Helper()
	m := settingsOffice(t, f)
	m = press(m, key("s"))
	for range 4 {
		m = press(m, key("tab"))
	}
	return m
}

func TestSettingsListEveryKeyBinding(t *testing.T) {
	m := keysSection(t, &fakeSettings{})
	v := view(m)
	for _, want := range []string{"prefix", "close window", "ctrl+a d, f10, ctrl+q", "maximize", "new terminal",
		"close terminal", "pane left", "pane right", "open station", "ctrl+a 1-9", "board", "new agent", "ctrl+a shift+n", "usage"} {
		if !strings.Contains(v, want) {
			t.Errorf("settings should list %q:\n%s", want, v)
		}
	}
}

func TestSettingsRebindAKeyAfterThePrefix(t *testing.T) {
	f := &fakeSettings{}
	m := keysSection(t, f)
	m = press(m, key("down")) // close window
	m = press(m, key("enter"))
	if !strings.Contains(view(m), "press") {
		t.Fatalf("enter should ask for the new key:\n%s", view(m))
	}
	m = press(press(m, key("ctrl+a")), key("q"))
	if !slices.Equal(f.calls, []string{"keys close_window prefix+q"}) {
		t.Fatalf("calls = %q", f.calls)
	}
	if v := view(m); !strings.Contains(v, "ctrl+a q") || strings.Contains(v, "ctrl+a d,") {
		t.Errorf("the list should show the new key:\n%s", v)
	}
	m = press(m, key("esc"))
	m = press(m, key("enter"))
	m = prefixed(m, "q")
	if m.win != nil {
		t.Error("ctrl+a q should close the window at once")
	}
}

func TestSettingsRebindToAKeyAloneAndPutBackTheDefault(t *testing.T) {
	f := &fakeSettings{}
	m := keysSection(t, f)
	m = press(press(m, key("down")), key("down")) // maximize
	m = press(press(m, key("enter")), key("f4"))
	m = press(m, key("backspace"))
	want := []string{"keys toggle_maximize f4", "keys toggle_maximize"}
	if !slices.Equal(f.calls, want) {
		t.Errorf("calls = %q, want %q", f.calls, want)
	}
	if !strings.Contains(view(m), "ctrl+a z, f2") {
		t.Errorf("backspace should put back the defaults:\n%s", view(m))
	}
}

func TestSettingsRefuseAKeyAnotherActionUses(t *testing.T) {
	f := &fakeSettings{}
	m := keysSection(t, f)
	for range 8 {
		m = press(m, key("down")) // board
	}
	m = press(press(m, key("enter")), key("ctrl+a"))
	m = press(m, key("d"))
	if v := view(m); !strings.Contains(v, "close_window") {
		t.Errorf("the clash should be explained:\n%s", v)
	}
	if action, _, _ := m.keys.Action("d", true); action != config.CloseWindow {
		t.Errorf("ctrl+a d = %q, want close_window still", action)
	}
}

func TestSettingsKeyCaptureTakesKeysThatOtherwiseCloseIt(t *testing.T) {
	f := &fakeSettings{}
	m := keysSection(t, f)
	m = press(m, key("down"))
	m = press(press(m, key("enter")), key("esc"))
	if m.setWin == nil || len(f.calls) != 0 {
		t.Fatalf("esc should only cancel the capture: open %v calls %q", m.setWin != nil, f.calls)
	}
	m = press(press(m, key("enter")), key("s"))
	if m.setWin == nil || !slices.Equal(f.calls, []string{"keys close_window s"}) {
		t.Errorf("s should be captured, not close settings: open %v calls %q", m.setWin != nil, f.calls)
	}
}

func TestSettingsRefuseADefaultAnotherActionNowUses(t *testing.T) {
	f := &fakeSettings{}
	m := keysSection(t, f)
	for range 9 {
		m = press(m, key("down")) // new agent
	}
	m = press(press(press(m, key("enter")), key("ctrl+a")), key("m"))
	m = press(m, key("up")) // board
	m = press(press(press(m, key("enter")), key("ctrl+a")), key("N"))
	m = press(press(m, key("down")), key("backspace")) // new agent back to ctrl+a shift+n
	want := []string{"keys new_agent prefix+m", "keys open_board prefix+shift+n"}
	if !slices.Equal(f.calls, want) {
		t.Errorf("calls = %q, want %q (the default is taken, nothing is saved)", f.calls, want)
	}
	if v := view(m); !strings.Contains(v, "open_board") || !strings.Contains(v, "ctrl+a m") {
		t.Errorf("the clash should be explained and new agent keep ctrl+a m:\n%s", v)
	}
}

// columns is the text of the Theme column and of the Colors column beside
// it, which starts under the Colors heading.
func columns(t *testing.T, v string) (theme, colors string) {
	t.Helper()
	lines := strings.Split(v, "\n")
	col := -1
	for _, l := range lines {
		if strings.Contains(l, "Theme") && strings.Contains(l, "Colors") {
			col = len([]rune(l[:strings.Index(l, "Colors")]))
		}
	}
	if col < 0 {
		t.Fatalf("no Theme and Colors headings side by side:\n%s", v)
	}
	for _, l := range lines {
		if r := []rune(l); len(r) > col {
			theme += string(r[:col]) + "\n"
			colors += string(r[col:]) + "\n"
		}
	}
	return theme, colors
}

// marked says whether name is marked in the Theme column and in the Colors
// column.
func marked(t *testing.T, v, name string) (theme, colors bool) {
	t.Helper()
	left, right := columns(t, v)
	return strings.Contains(left, "● "+name), strings.Contains(right, "● "+name)
}

func TestSettingsColorsSectionDressesTheThemeInAnotherThemesColors(t *testing.T) {
	f := &fakeSettings{themes: []string{"aliens", "mine"}, scenes: []string{"mine"}, active: "mine", palette: "mine"}
	m := settingsOffice(t, f)
	m = press(m, key("s"))
	if v := view(m); !strings.Contains(v, "the theme's own colors") {
		t.Fatalf("Colors should offer the theme's own colors:\n%s", v)
	}
	if theme, colors := marked(t, view(m), "mine"); !theme || !colors {
		t.Fatalf("mine should be marked as theme and as colors:\n%s", view(m))
	}
	m = press(m, key("tab"))
	m = press(m, key("up")) // aliens
	m = press(m, key("enter"))
	if !slices.Equal(f.calls, []string{"palette aliens"}) {
		t.Fatalf("calls = %q, want palette aliens", f.calls)
	}
	if theme, colors := marked(t, view(m), "aliens"); theme || !colors {
		t.Errorf("aliens should be the colors in use, not the theme:\n%s", view(m))
	}
	if theme, colors := marked(t, view(m), "mine"); !theme || colors {
		t.Errorf("mine should still be the theme and no longer the colors:\n%s", view(m))
	}
	m = press(m, key("up")) // the theme's own colors
	m = press(m, key("enter"))
	if !slices.Equal(f.calls, []string{"palette aliens", "palette "}) {
		t.Errorf("the theme's own colors should drop the palette, calls = %q", f.calls)
	}
	if _, colors := marked(t, view(m), "the theme's own colors"); !colors {
		t.Errorf("the theme's own colors should be marked:\n%s", view(m))
	}
}

func TestSettingsThemeListsScenesAndColorsTheRestEachOnce(t *testing.T) {
	f := &fakeSettings{themes: []string{"gentle-ai", "night", "prueba", "space"}, scenes: []string{"space"},
		active: "space", palette: "gentle-ai"}
	m := settingsOffice(t, f)
	m = press(m, key("s"))
	v := view(m)
	theme, colors := columns(t, v)
	for _, name := range []string{"gentle-ai", "night", "prueba"} {
		if strings.Contains(theme, name) {
			t.Errorf("%s has no shapes of its own and is not the theme in use; Theme lists it:\n%s", name, v)
		}
		if !strings.Contains(colors, name) {
			t.Errorf("Colors should list %s:\n%s", name, v)
		}
	}
	if !strings.Contains(theme, "space") || strings.Contains(colors, "space") {
		t.Errorf("space draws its own shapes: only Theme lists it:\n%s", v)
	}
	if th, _ := marked(t, v, "space"); !th {
		t.Errorf("space should be the theme in use:\n%s", v)
	}
	if _, colors := marked(t, v, "gentle-ai"); !colors {
		t.Errorf("gentle-ai should still be the colors in use:\n%s", v)
	}
	m = press(m, key("up")) // the built-in theme, right above space
	m = press(m, key("enter"))
	if !slices.Equal(f.calls, []string{"apply "}) {
		t.Errorf("up from space should reach the built-in theme, calls = %q", f.calls)
	}
}

// A new theme copied from the built-in shapes only lends colors until its
// files are redrawn, so it is picked under Colors.
func TestSettingsNewThemeWithTheBuiltinShapesIsPickedUnderColors(t *testing.T) {
	f := &fakeSettings{themes: []string{"night"}, scenes: []string{}}
	m := settingsOffice(t, f)
	m = press(m, key("s"))
	m = press(m, key("n"))
	m = typeText(m, "mine")
	m = press(press(m, key("enter")), key("enter"))
	if !slices.Equal(f.calls, []string{"new mine", "palette mine"}) {
		t.Errorf("calls = %q, want the new theme applied as colors", f.calls)
	}
}
