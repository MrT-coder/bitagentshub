package statusline_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/statusline"
)

const ours = "bah statusline"

const backupSuffix = ".bah-backup"

// original is a settings file the way Claude Code writes it, with a status
// line of the user's own and keys BitAgentsHub knows nothing about.
const original = `{
  "model": "opus",
  "statusLine": {
    "type": "command",
    "command": "~/.claude/mine.sh",
    "padding": 0
  },
  "zeta": {
    "nested": [
      1,
      2.50
    ],
    "s": "é"
  },
  "env": {
    "A": "1"
  }
}
`

func writeSettings(t *testing.T, content string) (settings, state string) {
	t.Helper()
	dir := t.TempDir()
	settings = filepath.Join(dir, "settings.json")
	if content != "" {
		if err := os.WriteFile(settings, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return settings, filepath.Join(t.TempDir(), "state")
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstallSetsTheStatusLineAndKeepsEverythingElse(t *testing.T) {
	settings, state := writeSettings(t, original)
	if err := statusline.Install(settings, state, ours); err != nil {
		t.Fatal(err)
	}
	want := `{
  "model": "opus",
  "statusLine": {
    "type": "command",
    "command": "bah statusline"
  },
  "zeta": {
    "nested": [
      1,
      2.50
    ],
    "s": "é"
  },
  "env": {
    "A": "1"
  }
}
`
	if got := read(t, settings); got != want {
		t.Errorf("settings =\n%s\nwant\n%s", got, want)
	}
	if got := read(t, settings+backupSuffix); got != original {
		t.Errorf("backup =\n%s\nwant the original file", got)
	}
	if _, err := os.Stat(filepath.Join(state, "statusline.json")); err != nil {
		t.Errorf("previous status line not remembered: %v", err)
	}
}

func TestInstallAppendsTheStatusLineWhenThereWasNone(t *testing.T) {
	settings, state := writeSettings(t, "{\"model\": \"opus\"}")
	if err := statusline.Install(settings, state, ours); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"model\": \"opus\",\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"bah statusline\"\n  }\n}\n"
	if got := read(t, settings); got != want {
		t.Errorf("settings =\n%s\nwant\n%s", got, want)
	}
}

func TestInstallWithoutASettingsFileStartsFromEmpty(t *testing.T) {
	settings, state := writeSettings(t, "")
	if err := statusline.Install(settings, state, ours); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"bah statusline\"\n  }\n}\n"
	if got := read(t, settings); got != want {
		t.Errorf("settings =\n%s\nwant\n%s", got, want)
	}
	if _, err := os.Stat(settings + backupSuffix); !os.IsNotExist(err) {
		t.Errorf("backup of a missing file: stat err = %v, want not exist", err)
	}

	// Uninstall leaves no statusLine behind.
	if err := statusline.Uninstall(settings, state); err != nil {
		t.Fatal(err)
	}
	if got := read(t, settings); got != "{}\n" {
		t.Errorf("after uninstall settings = %q, want {}", got)
	}
}

func TestInstallRefusesInvalidSettings(t *testing.T) {
	for _, bad := range []string{"{\"model\": ", "[1, 2]", "{} {}"} {
		settings, state := writeSettings(t, bad)
		if err := statusline.Install(settings, state, ours); err == nil {
			t.Errorf("%q: Install err = nil, want an error", bad)
		}
		if got := read(t, settings); got != bad {
			t.Errorf("%q: settings changed to %q", bad, got)
		}
		if _, err := os.Stat(filepath.Join(state, "statusline.json")); !os.IsNotExist(err) {
			t.Errorf("%q: state written although install failed", bad)
		}
		if _, err := os.Stat(settings + backupSuffix); !os.IsNotExist(err) {
			t.Errorf("%q: backup written although install failed", bad)
		}
	}
}

func TestInstallTwiceThenUninstallRestoresTheUsersStatusLine(t *testing.T) {
	settings, state := writeSettings(t, original)
	for i := 0; i < 2; i++ {
		if err := statusline.Install(settings, state, ours); err != nil {
			t.Fatal(err)
		}
	}
	if got := read(t, settings+backupSuffix); got != original {
		t.Errorf("second install replaced the backup of the original:\n%s", got)
	}
	if err := statusline.Uninstall(settings, state); err != nil {
		t.Fatal(err)
	}
	if got := read(t, settings); got != original {
		t.Errorf("after uninstall settings =\n%s\nwant the original\n%s", got, original)
	}
	if _, err := os.Stat(filepath.Join(state, "statusline.json")); !os.IsNotExist(err) {
		t.Errorf("statusline.json still there after uninstall: %v", err)
	}
}

func TestUninstallWithoutInstallIsAnError(t *testing.T) {
	settings, state := writeSettings(t, original)
	err := statusline.Uninstall(settings, state)
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("Uninstall err = %v, want a not installed error", err)
	}
	if got := read(t, settings); got != original {
		t.Errorf("settings changed to\n%s", got)
	}
}

func TestUninstallKeepsAStatusLineTheUserSetAfterInstall(t *testing.T) {
	settings, state := writeSettings(t, original)
	if err := statusline.Install(settings, state, ours); err != nil {
		t.Fatal(err)
	}
	theirs := "{\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"other\"\n  }\n}\n"
	if err := os.WriteFile(settings, []byte(theirs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := statusline.Uninstall(settings, state); err != nil {
		t.Fatal(err)
	}
	if got := read(t, settings); got != theirs {
		t.Errorf("settings =\n%s\nwant the user's newer status line kept", got)
	}
}
