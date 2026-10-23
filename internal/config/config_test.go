package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultsMatchTheApprovedBindings(t *testing.T) {
	k := Default()
	if k.Prefix != "ctrl+a" {
		t.Errorf("prefix = %q, want ctrl+a", k.Prefix)
	}
	cases := []struct {
		key         string
		afterPrefix bool
		action      string
		n           int
	}{
		{"d", true, CloseWindow, 0},
		{"f10", false, CloseWindow, 0},
		{"ctrl+q", false, CloseWindow, 0},
		{"z", true, ToggleMaximize, 0},
		{"f2", false, ToggleMaximize, 0},
		{"c", true, NewTerminal, 0},
		{"f3", false, NewTerminal, 0},
		{"x", true, CloseTerminal, 0},
		{"f5", false, CloseTerminal, 0},
		{"h", true, FocusPaneLeft, 0},
		{"l", true, FocusPaneRight, 0},
		{"p", true, OpenHub, 0},
		{"u", true, ShowUsage, 0},
		{"s", true, ShowSettings, 0},
		{"b", true, OpenBoard, 0},
		{"shift+n", true, NewAgent, 0},
		{"N", true, NewAgent, 0}, // Bubble Tea reports shift+n by its text
		{"1", true, FocusStation, 1},
		{"9", true, FocusStation, 9},
	}
	for _, c := range cases {
		action, n, ok := k.Action(c.key, c.afterPrefix)
		if !ok || action != c.action || n != c.n {
			t.Errorf("Action(%q, %v) = %q %d %v, want %q %d", c.key, c.afterPrefix, action, n, ok, c.action, c.n)
		}
	}
	for _, c := range []struct {
		key         string
		afterPrefix bool
	}{{"d", false}, {"a", true}, {"0", true}, {"f10", true}} {
		if action, _, ok := k.Action(c.key, c.afterPrefix); ok {
			t.Errorf("Action(%q, %v) = %q, want no binding", c.key, c.afterPrefix, action)
		}
	}
}

func TestAMissingFileMeansDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Keys.Prefix != "ctrl+a" {
		t.Errorf("prefix = %q, want the default", c.Keys.Prefix)
	}
}

func TestTheFileOverridesPrefixAndActions(t *testing.T) {
	path := write(t, `
# other sections are left for later features
[later]
theme = "x"

[keys]
prefix = "ctrl+b"
new_terminal = "prefix+t"
close_window = ["prefix+q", "F12"]
focus_station = "prefix+1..3"
`)
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	k := c.Keys
	if k.Prefix != "ctrl+b" {
		t.Errorf("prefix = %q", k.Prefix)
	}
	if a, _, _ := k.Action("t", true); a != NewTerminal {
		t.Errorf("prefix+t = %q, want new_terminal", a)
	}
	if a, _, ok := k.Action("c", true); ok {
		t.Errorf("prefix+c = %q, an overridden action keeps no default key", a)
	}
	if a, _, _ := k.Action("f12", false); a != CloseWindow {
		t.Errorf("f12 = %q, want close_window (keys are case-insensitive)", a)
	}
	if a, _, ok := k.Action("f10", false); ok {
		t.Errorf("f10 = %q, want no binding after the override", a)
	}
	if _, _, ok := k.Action("4", true); ok {
		t.Error("prefix+4 should be outside the 1..3 range")
	}
	if a, n, _ := k.Action("3", true); a != FocusStation || n != 3 {
		t.Errorf("prefix+3 = %q %d", a, n)
	}
	if a, _, _ := k.Action("z", true); a != ToggleMaximize {
		t.Errorf("prefix+z = %q, actions not in the file keep their defaults", a)
	}
	if got := k.Keys(CloseWindow); strings.Join(got, ",") != "prefix+q,f12" {
		t.Errorf("Keys(close_window) = %v", got)
	}
}

func TestBadBindingsStopWithAClearError(t *testing.T) {
	cases := []struct {
		name, body string
		want       []string
	}{
		{"unknown action", "[keys]\nfly = \"prefix+f\"\n", []string{"keys.fly", "unknown action"}},
		{"removed action", "[keys]\nnext_pane = \"prefix+o\"\n", []string{"keys.next_pane", "unknown action"}},
		{"bad key", "[keys]\nclose_terminal = \"ctrl+\"\n", []string{"keys.close_terminal", `"ctrl+"`}},
		{"unknown key name", "[keys]\nclose_terminal = \"prefix+banana\"\n", []string{"keys.close_terminal", `"prefix+banana"`}},
		{"conflict", "[keys]\nclose_window = \"prefix+x\"\nclose_terminal = [\"prefix+x\", \"f5\"]\n", []string{"prefix+x", "close_window", "close_terminal"}},
		{"range on a plain action", "[keys]\nclose_terminal = \"prefix+1..3\"\n", []string{"keys.close_terminal", "range"}},
		{"prefix used alone", "[keys]\nclose_terminal = \"ctrl+a\"\n", []string{"keys.close_terminal", "prefix"}},
		{"not a string", "[keys]\nclose_terminal = 3\n", []string{"keys.close_terminal"}},
		{"bad toml", "[keys\n", []string{"config.toml"}},
	}
	for _, c := range cases {
		path := write(t, c.body)
		_, err := Load(path)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		msg := err.Error()
		for _, w := range append(c.want, path) {
			if !strings.Contains(msg, w) {
				t.Errorf("%s: error %q should mention %q", c.name, msg, w)
			}
		}
	}
}

func TestTheThemeSectionNamesAThemeFolder(t *testing.T) {
	c, err := Load(write(t, "[theme]\nname = \"pixel-retro\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme != "pixel-retro" {
		t.Errorf("theme = %q, want pixel-retro", c.Theme)
	}
	if c, _ := Load(write(t, "[keys]\nprefix = \"ctrl+b\"\n")); c.Theme != "" {
		t.Errorf("no [theme] should mean the built-in theme, got %q", c.Theme)
	}
	for _, bad := range []string{`name = ""`, `name = "../x"`, `name = "a/b"`, `name = 'a\b'`, `name = 3`, `colour = "x"`} {
		path := write(t, "[theme]\n"+bad+"\n")
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "theme.") {
			t.Errorf("%s: error %v should name the file and the theme field", bad, err)
		}
	}
}

func TestTheBoardSectionNamesAFolderAndAnEditor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "My Second Brain", "tasks")
	c, err := Load(write(t, "[board]\ndir = '"+dir+"'\neditor = \"hx\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Board != (Board{Dir: dir, Editor: "hx"}) {
		t.Errorf("board = %+v", c.Board)
	}
	c, err = Load(write(t, "[board]\ndir = '"+dir+"'\n"))
	if err != nil || c.Board.Editor != "nvim" {
		t.Errorf("the editor should default to nvim: %+v, %v", c.Board, err)
	}
	if c, err := Load(write(t, "[keys]\nprefix = \"ctrl+b\"\n")); err != nil || c.Board.Dir != "" {
		t.Errorf("no [board] should leave the board off: %+v, %v", c.Board, err)
	}
}

func TestBadBoardSectionsStopWithAClearError(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"relative folder", "[board]\ndir = \"tasks\"\n", "board.dir"},
		{"not a string", "[board]\ndir = 3\n", "board.dir"},
		{"editor not a string", "[board]\ndir = '" + filepath.Join(t.TempDir(), "p") + "'\neditor = 3\n", "board.editor"},
		{"unknown field", "[board]\nfolder = \"x\"\n", "board.folder"},
	}
	for _, c := range cases {
		_, err := Load(write(t, c.body))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to name %s", c.name, err, c.want)
		}
	}
}

func TestThemesFolderSitsNextToTheConfigFile(t *testing.T) {
	if got, want := ThemesDir(filepath.Join("x", "config.toml")), filepath.Join("x", "themes"); got != want {
		t.Errorf("ThemesDir = %q, want %q", got, want)
	}
}

func TestYourBindingWinsOverADefaultKey(t *testing.T) {
	c, err := Load(write(t, "[keys]\nclose_terminal = \"prefix+s\"\nprefix = \"f10\"\nnew_terminal = \"prefix+3\"\n"))
	if err != nil {
		t.Fatalf("a binding of yours that a default also uses should load: %v", err)
	}
	k := c.Keys
	if a, _, _ := k.Action("s", true); a != CloseTerminal {
		t.Errorf("prefix+s = %q, want your close_terminal", a)
	}
	if got := k.Keys(ShowSettings); len(got) != 0 {
		t.Errorf("show_settings keeps no key it lost: %v", got)
	}
	if a, _, ok := k.Action("f10", false); ok {
		t.Errorf("f10 is your prefix now, not %q", a)
	}
	if a, _, _ := k.Action("d", true); a != CloseWindow {
		t.Errorf("close_window keeps its other default keys, prefix+d = %q", a)
	}
	if a, _, _ := k.Action("3", true); a != NewTerminal {
		t.Errorf("prefix+3 = %q, want your new_terminal", a)
	}
	if a, n, _ := k.Action("4", true); a != FocusStation || n != 4 {
		t.Errorf("the rest of focus_station's range stays: prefix+4 = %q %d", a, n)
	}
}

func TestThePaletteNamesAThemeFolderToo(t *testing.T) {
	c, err := Load(write(t, "[theme]\nname = \"aliens\"\npalette = \"gentle-ai\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Theme != "aliens" || c.Palette != "gentle-ai" {
		t.Errorf("theme = %q, palette = %q, want aliens and gentle-ai", c.Theme, c.Palette)
	}
	if c, _ := Load(write(t, "[theme]\nname = \"aliens\"\n")); c.Palette != "" {
		t.Errorf("no palette should keep the theme's own colors, got %q", c.Palette)
	}
	for _, bad := range []string{`palette = ""`, `palette = "../x"`, `palette = 'a\b'`, `palette = 3`} {
		path := write(t, "[theme]\n"+bad+"\n")
		_, err := Load(path)
		if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "theme.palette") {
			t.Errorf("%s: error %v should name the file and theme.palette", bad, err)
		}
	}
}
