package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/statusline"
)

// officeSettings is what the settings tab changes: the theme and the
// language in config.toml and Claude Code's status line. The window calls it
// from background commands, so the active theme and language are guarded.
type officeSettings struct {
	configPath   string
	themesDir    string
	settingsPath string // Claude Code's settings.json
	state        string

	mu       sync.Mutex
	active   string // "" is the built-in theme
	palette  string // the theme whose colors it wears; "" for its own
	language string // "en" or "es", the language in use
}

func (s *officeSettings) ConfigPath() string { return s.configPath }

// Themes are the office's own themes, then the folders in the themes folder
// that hold a theme.toml.
func (s *officeSettings) Themes() ([]string, error) {
	entries, err := os.ReadDir(s.themesDir)
	if errors.Is(err, fs.ErrNotExist) {
		return office.PresetNames(), nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if _, taken := office.Preset(e.Name()); !e.IsDir() || taken {
			continue // a folder named like the office's own theme is never loaded
		}
		if _, err := os.Stat(filepath.Join(s.themesDir, e.Name(), "theme.toml")); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return append(office.PresetNames(), names...), nil
}

// Scenes are the themes whose shapes are their own. One that does not load
// is counted in, so choosing it shows why.
func (s *officeSettings) Scenes() ([]string, error) {
	names, err := s.Themes()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, name := range names {
		if t, err := loadTheme(s.themesDir, name); err != nil || t.OwnShapes() {
			out = append(out, name)
		}
	}
	return out, nil
}

func (s *officeSettings) ActiveTheme() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// ApplyTheme loads a theme, in the palette's colors if one is chosen, saves
// it in config.toml and draws with it; if either step fails nothing changes.
func (s *officeSettings) ApplyTheme(name string) error {
	t, err := s.load(name)
	if err != nil {
		return err
	}
	if err := config.SaveTheme(s.configPath, name); err != nil {
		return err
	}
	office.SetTheme(t)
	s.mu.Lock()
	s.active = name
	s.mu.Unlock()
	return nil
}

func (s *officeSettings) ActivePalette() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.palette
}

// ApplyPalette dresses the active theme in the colors of the theme name,
// "" for its own, saves it in config.toml and draws with it; if either step
// fails nothing changes.
func (s *officeSettings) ApplyPalette(name string) error {
	t, err := loadLook(s.themesDir, s.ActiveTheme(), name)
	if err != nil {
		return err
	}
	if err := config.SavePalette(s.configPath, name); err != nil {
		return err
	}
	office.SetTheme(t)
	s.mu.Lock()
	s.palette = name
	s.mu.Unlock()
	return nil
}

// SetPetName saves [pet] name in config.toml.
func (s *officeSettings) SetPetName(name string) error {
	return config.SavePetName(s.configPath, name)
}

func (s *officeSettings) ActiveLanguage() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.language
}

// ApplyLanguage saves [ui] language in config.toml; the window then speaks
// it. If saving fails nothing changes.
func (s *officeSettings) ApplyLanguage(lang string) error {
	if err := config.SaveLanguage(s.configPath, lang); err != nil {
		return err
	}
	s.mu.Lock()
	s.language = lang
	s.mu.Unlock()
	return nil
}

// NewTheme writes the active theme, in its palette's colors, as
// themes/<name> without applying it.
func (s *officeSettings) NewTheme(name string) error {
	t, err := s.load(s.ActiveTheme())
	if err != nil {
		return err
	}
	_, err = writeThemeAs(s.themesDir, name, t)
	return err
}

func (s *officeSettings) LimitsInstalled() (bool, error) {
	return statusline.Installed(s.settingsPath, s.state)
}

func (s *officeSettings) SetLimits(on bool) error {
	if on {
		_, err := installLimits(s.settingsPath, s.state)
		return err
	}
	return statusline.Uninstall(s.settingsPath, s.state)
}

// SetKeys saves the binding in config.toml and reads the keymap back, so
// the office uses exactly what the next start will.
func (s *officeSettings) SetKeys(action string, keys []string) (config.Keymap, error) {
	if err := config.SaveKeys(s.configPath, action, keys); err != nil {
		return config.Keymap{}, err
	}
	cfg, err := config.Load(s.configPath)
	return cfg.Keys, err
}

func (s *officeSettings) load(name string) (office.Theme, error) {
	s.mu.Lock()
	palette := s.palette
	s.mu.Unlock()
	return loadLook(s.themesDir, name, palette)
}
