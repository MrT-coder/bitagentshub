package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// testSettings is the office's settings over files in a temp folder.
func testSettings(t *testing.T, config string) (*officeSettings, string) {
	t.Helper()
	root := t.TempDir()
	cfg := filepath.Join(root, "config.toml")
	if config != "" {
		if err := os.WriteFile(cfg, []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := &officeSettings{
		configPath:   cfg,
		themesDir:    filepath.Join(root, "themes"),
		settingsPath: filepath.Join(root, "claude", "settings.json"),
		state:        filepath.Join(root, "state"),
	}
	t.Cleanup(func() { office.SetTheme(office.BuiltinTheme()) })
	return s, root
}

func TestSettingsListThemesThatHaveAManifest(t *testing.T) {
	s, _ := testSettings(t, "")
	if got, err := s.Themes(); err != nil || !reflect.DeepEqual(got, []string{"night", "retro", "gentle-ai"}) {
		t.Errorf("no themes folder: %v %v; want the office's own themes", got, err)
	}
	for _, name := range []string{"sunset", "noche"} {
		if _, err := writeThemeAs(s.themesDir, name, office.BuiltinTheme()); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(s.themesDir, "half"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := s.Themes()
	if err != nil || !reflect.DeepEqual(got, []string{"night", "retro", "gentle-ai", "noche", "sunset"}) {
		t.Errorf("themes = %v, %v; want the folders with a theme.toml, sorted", got, err)
	}
}

func TestApplyingAThemeDrawsWithItAndSavesItKeepingComments(t *testing.T) {
	s, _ := testSettings(t, "# my keys\n[keys]\nprefix = \"ctrl+b\"\n")
	th := office.BuiltinTheme()
	th.Agents = []screen.RGB{{R: 1, G: 2, B: 3}} // a color the built-in theme does not use
	if _, err := writeThemeAs(s.themesDir, "sunset", th); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyTheme("sunset"); err != nil {
		t.Fatal(err)
	}
	if got := office.AgentColor("anyone"); got != th.Agents[0] {
		t.Errorf("the office should draw with the applied theme, agent color %v", got)
	}
	if s.ActiveTheme() != "sunset" {
		t.Errorf("active = %q", s.ActiveTheme())
	}
	b, _ := os.ReadFile(s.configPath)
	if want := "# my keys\n[keys]\nprefix = \"ctrl+b\"\n\n[theme]\nname = \"sunset\"\n"; string(b) != want {
		t.Errorf("config.toml =\n%s\nwant\n%s", b, want)
	}
	if err := s.ApplyTheme(""); err != nil {
		t.Fatal(err)
	}
	if office.AgentColor("anyone") == th.Agents[0] {
		t.Error("the built-in theme should be back")
	}
	if b, _ := os.ReadFile(s.configPath); strings.Contains(string(b), "name =") {
		t.Errorf("the built-in theme removes the name line:\n%s", b)
	}
}

func TestABrokenThemeChangesNothing(t *testing.T) {
	s, _ := testSettings(t, "[theme]\nname = \"sunset\"\n")
	s.active = "sunset"
	dir := filepath.Join(s.themesDir, "bad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "theme.toml"), []byte("[palette]\nhair = [\"#zz\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.configPath)
	if err := s.ApplyTheme("bad"); err == nil || !strings.Contains(err.Error(), "palette.hair") {
		t.Errorf("error = %v, want the theme's own error", err)
	}
	if after, _ := os.ReadFile(s.configPath); string(after) != string(before) || s.ActiveTheme() != "sunset" {
		t.Errorf("nothing should change: config %q, active %q", after, s.ActiveTheme())
	}
}

func TestANewThemeCopiesTheActiveOne(t *testing.T) {
	s, _ := testSettings(t, "")
	th := office.BuiltinTheme()
	th.Carpets = th.Carpets[:1]
	if _, err := writeThemeAs(s.themesDir, "sunset", th); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyTheme("sunset"); err != nil {
		t.Fatal(err)
	}
	if err := s.NewTheme("mine"); err != nil {
		t.Fatal(err)
	}
	got, err := office.LoadTheme(filepath.Join(s.themesDir, "mine"))
	if err != nil || len(got.Carpets) != 1 || got.Name != "mine" {
		t.Errorf("new theme = %d carpets, name %q, %v; want a copy of sunset named mine", len(got.Carpets), got.Name, err)
	}
	if s.ActiveTheme() != "sunset" {
		t.Error("creating a theme should not apply it")
	}
	if err := s.NewTheme("mine"); err == nil {
		t.Error("an existing theme must not be overwritten")
	}
}

func TestLimitsTurnOnAndOff(t *testing.T) {
	s, _ := testSettings(t, "")
	if on, err := s.LimitsInstalled(); err != nil || on {
		t.Fatalf("installed = %v, %v; want off", on, err)
	}
	if err := s.SetLimits(true); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.LimitsInstalled(); !on {
		t.Error("SetLimits(true) should install the status line")
	}
	if _, err := os.Stat(filepath.Join(s.state, "bin")); err != nil {
		t.Errorf("the executable copy should sit in the state folder: %v", err)
	}
	if err := s.SetLimits(false); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.LimitsInstalled(); on {
		t.Error("SetLimits(false) should uninstall it")
	}
}

func TestThemesThatComeWithTheOfficeAreListedAndApplied(t *testing.T) {
	s, _ := testSettings(t, "")
	if _, err := writeThemeAs(s.themesDir, "mine", office.BuiltinTheme()); err != nil {
		t.Fatal(err)
	}
	got, err := s.Themes()
	if err != nil || !reflect.DeepEqual(got, []string{"night", "retro", "gentle-ai", "mine"}) {
		t.Errorf("themes = %v, %v; want the office's own first, then the folders", got, err)
	}
	if err := s.ApplyTheme("night"); err != nil {
		t.Fatal(err)
	}
	night, _ := office.Preset("night")
	drawn := false
	for _, c := range night.Agents {
		drawn = drawn || office.AgentColor("x") == c
	}
	if !drawn {
		t.Error("the office should draw with night's agent colors")
	}
	if b, _ := os.ReadFile(s.configPath); !strings.Contains(string(b), `name = "night"`) {
		t.Errorf("config.toml = %s", b)
	}
}

func TestTheOfficesOwnThemeNamesAreReserved(t *testing.T) {
	themes := filepath.Join(t.TempDir(), "themes")
	for _, name := range []string{"night", "retro", "gentle-ai", "default"} {
		if _, err := themeInit(themes, name); err == nil {
			t.Errorf("theme init %s should refuse a name the office uses", name)
		}
	}
	if th, err := loadTheme(themes, "retro"); err != nil || th.Name != "retro" {
		t.Errorf("loadTheme(retro) = %q, %v; want the office's own retro without a folder", th.Name, err)
	}
}

func TestApplyingALanguageSavesItKeepingTheRest(t *testing.T) {
	s, _ := testSettings(t, "# my keys\n[keys]\nprefix = \"ctrl+b\"\n")
	s.language = "en"
	if err := s.ApplyLanguage("es"); err != nil {
		t.Fatal(err)
	}
	if s.ActiveLanguage() != "es" {
		t.Errorf("active language = %q, want es", s.ActiveLanguage())
	}
	b, _ := os.ReadFile(s.configPath)
	if want := "# my keys\n[keys]\nprefix = \"ctrl+b\"\n\n[ui]\nlanguage = \"es\"\n"; string(b) != want {
		t.Errorf("config.toml =\n%s\nwant\n%s", b, want)
	}
	if err := s.ApplyLanguage("fr"); err == nil || s.ActiveLanguage() != "es" {
		t.Errorf("an unknown language should fail and change nothing: %v, active %q", err, s.ActiveLanguage())
	}
}

func TestNamingThePetSavesItKeepingTheRest(t *testing.T) {
	s, _ := testSettings(t, "# my keys\n[keys]\nprefix = \"ctrl+b\"\n")
	if err := s.SetPetName("Michi"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(s.configPath)
	if want := "# my keys\n[keys]\nprefix = \"ctrl+b\"\n\n[pet]\nname = \"Michi\"\n"; string(b) != want {
		t.Errorf("config.toml =\n%s\nwant\n%s", b, want)
	}
	if err := s.SetPetName(" "); err == nil {
		t.Error("a blank name should fail")
	}
}
