package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
)

// SaveKeys sets the keys of one action, or the prefix with action
// "prefix", in the [keys] table of the config file at path; no keys removes
// the action's line, which puts back its defaults. Like SaveTheme it edits
// only that line, refuses a file it cannot edit safely and replaces the file
// only once Load accepts the result, so a key another action uses is
// refused with Load's reason and the file stays as it was.
func SaveKeys(path, action string, keys []string) error {
	spec := make([]string, len(keys))
	for i, k := range keys {
		spec[i] = strings.ToLower(strings.TrimSpace(k))
	}
	if err := checkKeys(action, spec); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return err
	}
	if missing && len(spec) == 0 {
		return nil
	}
	text, hasBOM := strings.CutPrefix(string(data), bom)
	before := map[string]any{}
	if _, err := toml.Decode(text, &before); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	_, defined := before["keys"]
	value, want := keysValue(spec)
	out, changed, err := editEntry(text, "keys", action, value, "keys."+action, defined)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if !changed {
		return nil
	}
	after := map[string]any{}
	if _, err := toml.Decode(out, &after); err != nil {
		return fmt.Errorf("%s: setting keys.%s would break the file: %w", path, action, err)
	}
	if !reflect.DeepEqual(after, withEntry(before, "keys", action, want)) {
		return fmt.Errorf("%s: setting keys.%s would change more than that line; edit it by hand", path, action)
	}
	if hasBOM {
		out = bom + out
	}
	return replace(path, []byte(out), func(cfg Config) error {
		if action == "prefix" && cfg.Keys.Prefix != spec[0] {
			return fmt.Errorf("%s: saved prefix %q but the file reads %q", path, spec[0], cfg.Keys.Prefix)
		}
		return nil
	})
}

// checkKeys refuses an unknown action and keys that cannot be read before
// the file is touched; clashes with other actions are Load's to find.
func checkKeys(action string, keys []string) error {
	if action == "prefix" {
		if len(keys) != 1 {
			return errors.New("keys.prefix: want one key")
		}
		if !validKey(keys[0]) {
			return fmt.Errorf("keys.prefix: %q is not a key", keys[0])
		}
		return nil
	}
	if _, known := defaults[action]; !known {
		return fmt.Errorf("keys.%s: unknown action", action)
	}
	for _, k := range keys {
		if _, err := expand(k, "", indexed[action]); err != nil {
			return fmt.Errorf("keys.%s: %q: %w", action, k, err)
		}
	}
	return nil
}

// keysValue is the TOML literal for keys ("" for none) and how the parsed
// file holds it: one key as text, several as a list.
func keysValue(keys []string) (string, any) {
	switch len(keys) {
	case 0:
		return "", nil
	case 1:
		return quote(keys[0]), keys[0]
	}
	quoted := make([]string, len(keys))
	list := make([]any, len(keys))
	for i, k := range keys {
		quoted[i], list[i] = quote(k), k
	}
	return "[" + strings.Join(quoted, ", ") + "]", list
}
