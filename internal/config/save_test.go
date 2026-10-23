package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/config"
)

func configFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func contents(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// saved runs SaveTheme and checks the whole file and the round trip through
// Load.
func saved(t *testing.T, path, name, want string) {
	t.Helper()
	if err := config.SaveTheme(path, name); err != nil {
		t.Fatalf("SaveTheme(%q) = %v", name, err)
	}
	if got := contents(t, path); got != want {
		t.Errorf("config.toml =\n%q\nwant\n%q", got, want)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load after SaveTheme: %v", err)
	}
	if cfg.Theme != name {
		t.Errorf("Load().Theme = %q, want %q", cfg.Theme, name)
	}
}

// refused runs SaveTheme expecting an error that mentions wantErr and the
// file left byte for byte.
func refused(t *testing.T, body, name, wantErr string) {
	t.Helper()
	path := configFile(t, body)
	err := config.SaveTheme(path, name)
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Errorf("%q: SaveTheme err = %v, want one mentioning %q", body, err, wantErr)
	}
	if got := contents(t, path); got != body {
		t.Errorf("%q: file changed to %q", body, got)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Errorf("%q: left %d files in the folder, want only config.toml", body, len(entries))
	}
}

func TestSaveThemeCreatesAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "config.toml")
	saved(t, path, "dark", "[theme]\nname = \"dark\"\n")
}

func TestSaveThemeAppendsTheTableAndKeepsEverythingElse(t *testing.T) {
	body := "# my office\n\n[keys]\n# the prefix\nprefix = \"ctrl+b\" # tmux habit\n"
	saved(t, configFile(t, body), "dark",
		body+"\n[theme]\nname = \"dark\"\n")
}

func TestSaveThemeAppendsAfterALastLineWithoutNewline(t *testing.T) {
	saved(t, configFile(t, "[keys]\nprefix = \"ctrl+b\""), "dark",
		"[keys]\nprefix = \"ctrl+b\"\n\n[theme]\nname = \"dark\"\n")
}

func TestSaveThemeInAnEmptyFileAddsNoBlankLine(t *testing.T) {
	saved(t, configFile(t, ""), "dark", "[theme]\nname = \"dark\"\n")
}

func TestSaveThemeKeepsCRLFAndTheBOM(t *testing.T) {
	body := bom + "# mine\r\n[keys]\r\nprefix = \"ctrl+b\"\r\n"
	saved(t, configFile(t, body), "dark",
		body+"\r\n[theme]\r\nname = \"dark\"\r\n")
}

func TestSaveThemeInsertsNameRightAfterTheHeader(t *testing.T) {
	body := "[theme]\n# pick a folder under themes\n\n[keys]\nprefix = \"ctrl+b\"\n"
	saved(t, configFile(t, body), "dark",
		"[theme]\nname = \"dark\"\n# pick a folder under themes\n\n[keys]\nprefix = \"ctrl+b\"\n")
}

func TestSaveThemeInsertsAfterAHeaderOnTheLastLine(t *testing.T) {
	saved(t, configFile(t, "# x\r\n[ theme ] # look"), "dark",
		"# x\r\n[ theme ] # look\r\nname = \"dark\"\r\n")
}

func TestSaveThemeReplacesOnlyTheValue(t *testing.T) {
	body := "[theme]\n# before\n  name   =   'old'   # my favourite\n# after\n[keys]\nprefix = \"ctrl+b\"\n"
	saved(t, configFile(t, body), "new one",
		"[theme]\n# before\n  name   =   \"new one\"   # my favourite\n# after\n[keys]\nprefix = \"ctrl+b\"\n")
}

func TestSaveThemeReplacesTheValueInACRLFFile(t *testing.T) {
	body := bom + "[theme]\r\nname=\"old\"\r\n\r\n[keys]\r\nprefix = \"ctrl+b\"\r\n"
	saved(t, configFile(t, body), "dark",
		bom+"[theme]\r\nname=\"dark\"\r\n\r\n[keys]\r\nprefix = \"ctrl+b\"\r\n")
}

func TestSaveThemeWithAnEmptyNameRemovesTheNameLine(t *testing.T) {
	body := "[keys]\nprefix = \"ctrl+b\"\n\n[theme]\n# the theme\nname = \"dark\" # mine\n# end\n"
	saved(t, configFile(t, body), "",
		"[keys]\nprefix = \"ctrl+b\"\n\n[theme]\n# the theme\n# end\n")
}

func TestSaveThemeWithAnEmptyNameAndNoThemeChangesNothing(t *testing.T) {
	body := "# mine\n[keys]\nprefix = \"ctrl+b\"\n"
	saved(t, configFile(t, body), "", body)

	missing := filepath.Join(t.TempDir(), "config.toml")
	if err := config.SaveTheme(missing, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("SaveTheme(\"\") created %s: %v", missing, err)
	}
}

func TestSaveThemeRefusesFormsItCannotEditSafely(t *testing.T) {
	refused(t, "theme.name = \"old\"\n", "dark", "[theme]")
	refused(t, "theme = { name = \"old\" }\n", "dark", "[theme]")
	refused(t, "[theme]\nname = \"\"\"old\"\"\"\n", "dark", "name")
}

func TestSaveThemeRefusesAFileThatDoesNotParse(t *testing.T) {
	body := "[keys\nprefix = \"ctrl+b\"\n"
	path := configFile(t, body)
	err := config.SaveTheme(path, "dark")
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("SaveTheme err = %v, want one naming %s", err, path)
	}
	if got := contents(t, path); got != body {
		t.Errorf("file changed to %q", got)
	}
}

func TestSaveThemeRefusesANameThatIsNotAFolderName(t *testing.T) {
	for _, name := range []string{"..", ".", "a/b", `a\b`} {
		refused(t, "[keys]\nprefix = \"ctrl+b\"\n", name, "folder name")
	}
}

func TestSaveThemeRefusesAResultTheOfficeCannotLoad(t *testing.T) {
	refused(t, "[keys]\nnope = \"x\"\n", "dark", "unknown action")
}

// bom is the UTF-8 byte order mark some Windows editors write.
const bom = "\xef\xbb\xbf"

func TestSaveThemeKeepsItsMessageForADottedTheme(t *testing.T) {
	refused(t, "theme.name = \"old\"\n", "dark", "the theme is not set in a [theme] table (a dotted key or an inline table); edit it by hand")
}

func TestSavePaletteSetsItBesideTheThemeAndRemovesIt(t *testing.T) {
	path := configFile(t, "[theme]\nname = \"aliens\" # the scene\n")
	if err := config.SavePalette(path, "gentle-ai"); err != nil {
		t.Fatal(err)
	}
	want := "[theme]\npalette = \"gentle-ai\"\nname = \"aliens\" # the scene\n"
	if got := contents(t, path); got != want {
		t.Errorf("config.toml =\n%q\nwant\n%q", got, want)
	}
	cfg, err := config.Load(path)
	if err != nil || cfg.Theme != "aliens" || cfg.Palette != "gentle-ai" {
		t.Fatalf("Load = %+v, %v", cfg, err)
	}
	if err := config.SavePalette(path, ""); err != nil {
		t.Fatal(err)
	}
	if got := contents(t, path); got != "[theme]\nname = \"aliens\" # the scene\n" {
		t.Errorf("an empty palette should remove its line, got %q", got)
	}
	if err := config.SavePalette(path, "../x"); err == nil {
		t.Error("a path is not a palette")
	}
}
