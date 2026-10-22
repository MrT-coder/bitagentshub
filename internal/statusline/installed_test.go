package statusline_test

import (
	"os"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/statusline"
)

func installedNow(t *testing.T, settings, state string) bool {
	t.Helper()
	ok, err := statusline.Installed(settings, state)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestInstalledFollowsInstallAndUninstall(t *testing.T) {
	settings, state := writeSettings(t, original)
	if installedNow(t, settings, state) {
		t.Error("Installed = true before Install, want false")
	}
	if err := statusline.Install(settings, state, ours); err != nil {
		t.Fatal(err)
	}
	if !installedNow(t, settings, state) {
		t.Error("Installed = false after Install, want true")
	}
	if err := statusline.Uninstall(settings, state); err != nil {
		t.Fatal(err)
	}
	if installedNow(t, settings, state) {
		t.Error("Installed = true after Uninstall, want false")
	}
}

func TestInstalledIsFalseWhenAnotherStatusLineReplacedOurs(t *testing.T) {
	settings, state := writeSettings(t, original)
	if err := statusline.Install(settings, state, ours); err != nil {
		t.Fatal(err)
	}
	theirs := "{\n  \"statusLine\": {\n    \"type\": \"command\",\n    \"command\": \"other\"\n  }\n}\n"
	if err := os.WriteFile(settings, []byte(theirs), 0o644); err != nil {
		t.Fatal(err)
	}
	if installedNow(t, settings, state) {
		t.Error("Installed = true with another status line, want false")
	}
}

func TestInstalledIsFalseWithoutASettingsFile(t *testing.T) {
	settings, state := writeSettings(t, "")
	if installedNow(t, settings, state) {
		t.Error("Installed = true without a settings file, want false")
	}
}

func TestInstalledRefusesInvalidSettings(t *testing.T) {
	settings, state := writeSettings(t, "{\"model\": ")
	if _, err := statusline.Installed(settings, state); err == nil {
		t.Error("Installed err = nil for invalid JSON, want an error")
	}
}
