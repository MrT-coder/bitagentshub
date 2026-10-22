package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/config"
)

// savedKeys runs SaveKeys and checks the whole file.
func savedKeys(t *testing.T, path, action string, keys []string, want string) {
	t.Helper()
	if err := config.SaveKeys(path, action, keys); err != nil {
		t.Fatalf("SaveKeys(%s, %v) = %v", action, keys, err)
	}
	if got := contents(t, path); got != want {
		t.Errorf("config.toml =\n%q\nwant\n%q", got, want)
	}
}

func TestSaveKeysCreatesAMissingFileAndTheBindingLoads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	savedKeys(t, path, config.CloseWindow, []string{"prefix+q"}, "[keys]\nclose_window = \"prefix+q\"\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if action, _, ok := cfg.Keys.Action("q", true); !ok || action != config.CloseWindow {
		t.Errorf("ctrl+a q = %q %v, want close_window", action, ok)
	}
	if _, _, ok := cfg.Keys.Action("d", true); ok {
		t.Error("the new binding should replace the default ctrl+a d")
	}
}

func TestSaveKeysEditsOnlyThatLineAndKeepsEverythingElse(t *testing.T) {
	body := "# mine\r\n[keys]\r\nprefix = \"ctrl+b\" # comment\r\nclose_window = \"prefix+q\"\r\n\r\n[theme]\r\nname = \"night\"\r\n"
	path := configFile(t, bom+body)
	savedKeys(t, path, config.CloseWindow, []string{"prefix+w", "f10"},
		bom+strings.Replace(body, `close_window = "prefix+q"`, `close_window = ["prefix+w", "f10"]`, 1))
	savedKeys(t, path, "prefix", []string{"ctrl+g"},
		bom+strings.NewReplacer(`close_window = "prefix+q"`, `close_window = ["prefix+w", "f10"]`, `prefix = "ctrl+b"`, `prefix = "ctrl+g"`).Replace(body))
}

func TestSaveKeysAddsTheLineUnderAnExistingTable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "board") // absolute on every system
	board := "\n[board]\ndir = '" + dir + "'\n"
	path := configFile(t, "[keys]\nprefix = \"ctrl+b\"\n"+board)
	savedKeys(t, path, config.NewAgent, []string{"prefix+a"}, "[keys]\nnew_agent = \"prefix+a\"\nprefix = \"ctrl+b\"\n"+board)
}

func TestSaveKeysWithNoKeysPutsBackTheDefault(t *testing.T) {
	path := configFile(t, "[keys]\nclose_window = \"prefix+q\"\nprefix = \"ctrl+b\"\n")
	savedKeys(t, path, config.CloseWindow, nil, "[keys]\nprefix = \"ctrl+b\"\n")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Keys.Keys(config.CloseWindow); !slices.Equal(got, []string{"prefix+d", "f10", "ctrl+q"}) {
		t.Errorf("close_window keys = %v, want the defaults", got)
	}
}

func TestSaveKeysRefusesAKeyAnotherActionUses(t *testing.T) {
	body := "[keys]\nclose_window = \"prefix+q\"\n"
	path := configFile(t, body)
	err := config.SaveKeys(path, config.OpenBoard, []string{"prefix+q"})
	if err == nil || !strings.Contains(err.Error(), "close_window") {
		t.Errorf("err = %v, want one naming close_window", err)
	}
	if got := contents(t, path); got != body {
		t.Errorf("file changed to %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("left %d files in the folder, want only config.toml", len(entries))
	}
}

func TestSaveKeysRefusesUnknownActionsAndBadKeys(t *testing.T) {
	path := configFile(t, "")
	for _, c := range []struct {
		action string
		keys   []string
		want   string
	}{
		{"nope", []string{"x"}, "unknown action"},
		{config.CloseWindow, []string{"prefix+nope"}, "not a key"},
		{"prefix", []string{"a", "b"}, "one key"},
		{"prefix", nil, "one key"},
	} {
		if err := config.SaveKeys(path, c.action, c.keys); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("SaveKeys(%s, %v) = %v, want one mentioning %q", c.action, c.keys, err, c.want)
		}
	}
	if got := contents(t, path); got != "" {
		t.Errorf("file changed to %q", got)
	}
}

func TestSaveKeysRefusesAFileThatDoesNotParse(t *testing.T) {
	body := "[keys\nprefix = \"ctrl+b\"\n"
	path := configFile(t, body)
	if err := config.SaveKeys(path, config.CloseWindow, []string{"f9"}); err == nil {
		t.Error("want an error")
	}
	if got := contents(t, path); got != body {
		t.Errorf("file changed to %q", got)
	}
}
