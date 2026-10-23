package config

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Languages the office speaks.
var languages = map[string]bool{"en": true, "es": true}

// petNameMax is the longest pet name, in characters, so it fits over the
// pet's corner.
const petNameMax = 20

// Lines of terminal history: the default and the range config.toml takes.
const (
	DefaultScrollback = 2000
	minScrollback     = 100
	maxScrollback     = 100000
)

// terminalSection reads [terminal]: how much history each terminal keeps.
func terminalSection(section map[string]any) (int, error) {
	lines := DefaultScrollback
	for key, v := range section {
		if key != "scrollback" {
			return 0, fmt.Errorf("terminal.%s: unknown field", key)
		}
		n, ok := v.(int64)
		if !ok || n < minScrollback || n > maxScrollback {
			return 0, fmt.Errorf("terminal.scrollback: %v is not a number of lines from %d to %d", v, minScrollback, maxScrollback)
		}
		lines = int(n)
	}
	return lines, nil
}

// updatesSection reads [updates]: whether to look for a newer release,
// which it does unless told not to.
func updatesSection(section map[string]any) (bool, error) {
	check := true
	for key, v := range section {
		if key != "check" {
			return false, fmt.Errorf("updates.%s: unknown field", key)
		}
		b, ok := v.(bool)
		if !ok {
			return false, fmt.Errorf("updates.check: %v is not true or false", v)
		}
		check = b
	}
	return check, nil
}

// uiSection reads [ui]: the office's language.
func uiSection(section map[string]any) (string, error) {
	lang := ""
	for key, v := range section {
		if key != "language" {
			return "", fmt.Errorf("ui.%s: unknown field", key)
		}
		s, ok := v.(string)
		if !ok || !languages[s] {
			return "", fmt.Errorf("ui.language: %v is not a language; use \"en\" or \"es\"", v)
		}
		lang = s
	}
	return lang, nil
}

// petSection reads [pet]: the pet's name.
func petSection(section map[string]any) (string, error) {
	name := ""
	for key, v := range section {
		if key != "name" {
			return "", fmt.Errorf("pet.%s: unknown field", key)
		}
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("pet.name: want text, got %v", v)
		}
		if err := checkPetName(s); err != nil {
			return "", fmt.Errorf("pet.name: %w", err)
		}
		name = s
	}
	return name, nil
}

// checkPetName accepts a name of 1 to petNameMax printable characters that
// is not only spaces.
func checkPetName(s string) error {
	switch {
	case strings.TrimSpace(s) == "":
		return fmt.Errorf("the name is empty")
	case utf8.RuneCountInString(s) > petNameMax:
		return fmt.Errorf("%q is longer than %d characters", s, petNameMax)
	case strings.IndexFunc(s, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0:
		return fmt.Errorf("%q has characters that do not print", s)
	}
	return nil
}

// SaveLanguage sets [ui] language in the config file at path, "en" or "es";
// "" removes the line so the office follows the system. It edits the file
// as SaveTheme does.
func SaveLanguage(path, lang string) error {
	if lang != "" && !languages[lang] {
		return fmt.Errorf("%q is not a language; use \"en\" or \"es\"", lang)
	}
	return saveEntry(path, "ui", "language", lang, "language", func(c Config) string { return c.Language })
}

// SavePetName sets [pet] name in the config file at path. It edits the file
// as SaveTheme does.
func SavePetName(path, name string) error {
	if err := checkPetName(name); err != nil {
		return err
	}
	return saveEntry(path, "pet", "name", name, "pet's name", func(c Config) string { return c.PetName })
}
