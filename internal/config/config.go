// Package config reads the office's config.toml. Its [keys] section binds
// window actions to keys the way herdr does: a prefix key, then a key.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"
)

// Actions a key can be bound to.
const (
	FocusStation   = "focus_station" // indexed: prefix+1..9 opens station N
	CloseWindow    = "close_window"
	ToggleMaximize = "toggle_maximize"
	NewTerminal    = "new_terminal"
	CloseTerminal  = "close_terminal"
	FocusPaneLeft  = "focus_pane_left"
	FocusPaneRight = "focus_pane_right"
	OpenHub        = "open_hub"      // also `p` on the board: the pet's hub
	ShowUsage      = "show_usage"    // the hub on its usage tab; also `u` on the board
	ShowSettings   = "show_settings" // the hub on its settings tab; also `s` on the board
	OpenBoard      = "open_board"    // also `b` on the board
	NewAgent       = "new_agent"     // also `n` on the board
)

// indexed actions take a range such as "prefix+1..9".
var indexed = map[string]bool{FocusStation: true}

var defaults = map[string][]string{
	FocusStation:   {"prefix+1..9"},
	CloseWindow:    {"prefix+d", "f10", "ctrl+q"},
	ToggleMaximize: {"prefix+z", "f2"},
	NewTerminal:    {"prefix+c", "f3"},
	CloseTerminal:  {"prefix+x", "f5"},
	FocusPaneLeft:  {"prefix+h"},
	FocusPaneRight: {"prefix+l"},
	OpenHub:        {"prefix+p"},
	ShowUsage:      {"prefix+u"},
	ShowSettings:   {"prefix+s"},
	OpenBoard:      {"prefix+b"},
	NewAgent:       {"prefix+shift+n"},
}

const defaultPrefix = "ctrl+a"

// Config is the parsed config file.
type Config struct {
	Keys       Keymap
	Theme      string // a folder under ThemesDir; "" is the built-in theme
	Palette    string // a theme whose colors go over Theme's; "" keeps Theme's own
	Board      Board
	Language   string // "en" or "es"; "" follows the system
	PetName    string // "" while the pet has no name
	Scrollback int    // lines of history each terminal keeps
}

// Board is the task board: daily notes per project in Dir, opened with
// Editor. An empty Dir turns the board off.
type Board struct {
	Dir    string
	Editor string
}

const defaultEditor = "nvim"

// Keymap resolves keys to actions. Keys are spelled as Bubble Tea reports
// them ("ctrl+a", "f10", "d"); "prefix+K" is K pressed after the prefix.
type Keymap struct {
	Prefix string
	keys   map[string][]string // action -> keys as written, lower case
	bind   map[string]binding  // "prefix+d", "f10" -> action
}

type binding struct {
	action string
	n      int // the index of an indexed action
}

// Action returns the action bound to key, pressed right after the prefix
// or on its own.
func (k Keymap) Action(key string, afterPrefix bool) (action string, n int, ok bool) {
	key = Spell(key)
	if afterPrefix {
		key = "prefix+" + key
	}
	b, ok := k.bind[key]
	return b.action, b.n, ok
}

// Spell writes a key the way bindings do: Bubble Tea reports shift and a
// letter by its text ("N"), bindings say "shift+n".
func Spell(key string) string {
	if r := []rune(key); len(r) == 1 && unicode.IsUpper(r[0]) {
		return "shift+" + string(unicode.ToLower(r[0]))
	}
	return key
}

// Keys returns the keys bound to an action as written in the config, for
// hints.
func (k Keymap) Keys(action string) []string { return k.keys[action] }

// Default is the keymap used when the config file sets nothing.
func Default() Keymap {
	k, err := build(defaultPrefix, defaults, nil)
	if err != nil {
		panic(err) // the defaults are fixed; a test covers them
	}
	return k
}

// DefaultPath is config.toml in the user config folder, next to herdr's
// (%APPDATA%\bitagentshub on Windows).
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "bitagentshub", "config.toml")
}

// ThemesDir is where themes live: a themes folder next to the config file.
func ThemesDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "themes")
}

// Load reads path. A missing file gives the defaults; anything it cannot
// use is an error naming the file, the action and the key.
func Load(path string) (Config, error) {
	var file struct {
		Keys  map[string]any `toml:"keys"`
		Theme map[string]any `toml:"theme"`
		Board map[string]any `toml:"board"`
		UI    map[string]any `toml:"ui"`
		Pet   map[string]any `toml:"pet"`
		Term  map[string]any `toml:"terminal"`
	}
	if path != "" {
		_, err := toml.DecodeFile(path, &file)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return Config{}, fmt.Errorf("%s: %w", path, err)
		}
	}
	prefix := defaultPrefix
	actions := map[string][]string{}
	yours := map[string]bool{} // actions the file sets
	for action, keys := range defaults {
		actions[action] = keys
	}
	names := make([]string, 0, len(file.Keys))
	for name := range file.Keys {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		keys, err := keyList(file.Keys[name])
		if err != nil {
			return Config{}, fmt.Errorf("%s: keys.%s: %w", path, name, err)
		}
		switch _, known := defaults[name]; {
		case name == "prefix":
			if len(keys) != 1 {
				return Config{}, fmt.Errorf("%s: keys.prefix: want one key", path)
			}
			prefix = keys[0]
		case !known:
			return Config{}, fmt.Errorf("%s: keys.%s: unknown action", path, name)
		default:
			actions[name] = keys
			yours[name] = true
		}
	}
	k, err := build(prefix, actions, yours)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	theme, palette, err := themeSection(file.Theme)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	b, err := boardSection(file.Board)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	lang, err := uiSection(file.UI)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	pet, err := petSection(file.Pet)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	lines, err := terminalSection(file.Term)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return Config{Keys: k, Theme: theme, Palette: palette, Board: b, Language: lang, PetName: pet, Scrollback: lines}, nil
}

// boardSection reads [board]: an absolute folder, since the office runs from
// any directory, and an editor program.
func boardSection(section map[string]any) (Board, error) {
	b := Board{Editor: defaultEditor}
	for key, v := range section {
		s, isString := v.(string)
		switch {
		case key != "dir" && key != "editor":
			return Board{}, fmt.Errorf("board.%s: unknown field", key)
		case !isString:
			return Board{}, fmt.Errorf("board.%s: want text, got %v", key, v)
		case key == "dir" && !filepath.IsAbs(s):
			return Board{}, fmt.Errorf("board.dir: %q is not an absolute folder", s)
		case key == "dir":
			b.Dir = filepath.Clean(s)
		case s != "":
			b.Editor = s
		}
	}
	return b, nil
}

// themeSection reads [theme]: name, the theme drawn, and palette, a theme
// whose colors go over it. Both are only folder names, never paths, so a
// theme always comes from the themes folder.
func themeSection(section map[string]any) (name, palette string, err error) {
	for key := range section {
		if key != "name" && key != "palette" {
			return "", "", fmt.Errorf("theme.%s: unknown field", key)
		}
	}
	if name, err = themeFolder(section, "name"); err != nil {
		return "", "", err
	}
	palette, err = themeFolder(section, "palette")
	return name, palette, err
}

// themeFolder reads [theme] key, a folder in the themes folder; "" when unset.
func themeFolder(section map[string]any, key string) (string, error) {
	v, ok := section[key]
	if !ok {
		return "", nil
	}
	name, isString := v.(string)
	switch {
	case !isString:
		return "", fmt.Errorf("theme.%s: want a folder name, got %v", key, v)
	case name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`):
		return "", fmt.Errorf("theme.%s: %q is not a folder name inside the themes folder", key, name)
	}
	return name, nil
}

// keyList accepts one key or a list of keys.
func keyList(v any) ([]string, error) {
	switch v := v.(type) {
	case string:
		return []string{v}, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("want a key or a list of keys, got %v", v)
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fmt.Errorf("want a key or a list of keys, got %v", v)
}

// build checks every key and indexes them. The file's own bindings come
// first and a key two of them claim is an error; a default key that one of
// them (or the prefix) already uses is dropped, so a new default never stops
// an existing config from loading.
func build(prefix string, actions map[string][]string, yours map[string]bool) (Keymap, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if !validKey(prefix) {
		return Keymap{}, fmt.Errorf("keys.prefix: %q is not a key", prefix)
	}
	k := Keymap{Prefix: prefix, keys: map[string][]string{}, bind: map[string]binding{}}
	names := make([]string, 0, len(actions))
	for name := range actions {
		names = append(names, name)
	}
	sort.SliceStable(names, func(i, j int) bool {
		if yours[names[i]] != yours[names[j]] {
			return yours[names[i]]
		}
		return names[i] < names[j]
	})
	for _, name := range names {
		for _, raw := range actions[name] {
			spec := strings.ToLower(strings.TrimSpace(raw))
			keys, err := expand(spec, prefix, indexed[name])
			if err != nil && !yours[name] && errors.Is(err, errPrefixAlone) {
				continue // the file made this default key its prefix
			}
			if err != nil {
				return Keymap{}, fmt.Errorf("keys.%s: %q: %w", name, raw, err)
			}
			bound := false
			for key, n := range keys {
				if other, taken := k.bind[key]; taken {
					if !yours[name] {
						continue // the file's binding wins over a default
					}
					return Keymap{}, fmt.Errorf("keys.%s: %q is already bound to %s; %s and %s cannot share %s",
						name, raw, other.action, other.action, name, key)
				}
				k.bind[key] = binding{action: name, n: n}
				bound = true
			}
			if bound {
				k.keys[name] = append(k.keys[name], spec)
			}
		}
	}
	return k, nil
}

var errPrefixAlone = errors.New("the prefix cannot be bound on its own")

// expand turns one spec into the keys it binds, with their index for a
// range ("prefix+1..9").
func expand(spec, prefix string, isIndexed bool) (map[string]int, error) {
	key, afterPrefix := strings.CutPrefix(spec, "prefix+")
	if !afterPrefix && key == prefix {
		return nil, errPrefixAlone
	}
	lead := ""
	if afterPrefix {
		lead = "prefix+"
	}
	if lo, hi, isRange := strings.Cut(key, ".."); isRange {
		if !isIndexed {
			return nil, errors.New("a range only fits an indexed action such as focus_station")
		}
		a, errA := strconv.Atoi(lo)
		b, errB := strconv.Atoi(hi)
		if errA != nil || errB != nil || a < 1 || b > 9 || a > b {
			return nil, errors.New("a range runs between digits 1..9")
		}
		out := map[string]int{}
		for i := a; i <= b; i++ {
			out[lead+strconv.Itoa(i)] = i
		}
		return out, nil
	}
	if isIndexed {
		return nil, errors.New("an indexed action takes a range such as prefix+1..9")
	}
	if !validKey(key) {
		return nil, errors.New("not a key")
	}
	return map[string]int{lead + key: 0}, nil
}

var named = map[string]bool{
	"enter": true, "esc": true, "tab": true, "space": true, "backspace": true,
	"delete": true, "insert": true, "home": true, "end": true, "pgup": true, "pgdown": true,
	"up": true, "down": true, "left": true, "right": true,
}

// validKey accepts modifiers (ctrl, alt, shift) and one base key: a single
// character, f1..f12 or a named key.
func validKey(key string) bool {
	parts := strings.Split(key, "+")
	base := parts[len(parts)-1]
	for _, mod := range parts[:len(parts)-1] {
		if mod != "ctrl" && mod != "alt" && mod != "shift" {
			return false
		}
	}
	if len([]rune(base)) == 1 || named[base] {
		return true
	}
	if n, ok := strings.CutPrefix(base, "f"); ok {
		i, err := strconv.Atoi(n)
		return err == nil && i >= 1 && i <= 12
	}
	return false
}
