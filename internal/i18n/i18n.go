// Package i18n holds the languages the office speaks. Text is written in
// English in the code and looked up in the current language's catalog, so a
// string the catalog lacks still reads in English rather than as a key.
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
)

// Lang is a language code as config.toml writes it.
type Lang string

const (
	English Lang = "en"
	Spanish Lang = "es"
)

// Langs lists the languages in the order the settings tab shows them.
var Langs = []Lang{English, Spanish}

// Name is the language's name in itself, as a language list shows it.
func (l Lang) Name() string {
	if l == Spanish {
		return "Español"
	}
	return "English"
}

var current atomic.Value // Lang; drawing reads it while settings may set it

// Set makes l the language of everything drawn from now on.
func Set(l Lang) { current.Store(l) }

// Current is the language in use, English until Set.
func Current() Lang {
	if l, ok := current.Load().(Lang); ok {
		return l
	}
	return English
}

// T returns s, English text, in the current language.
func T(s string) string {
	if Current() == Spanish {
		if t, ok := spanish[s]; ok {
			return t
		}
	}
	return s
}

// Tf formats the current language's version of format, English text.
func Tf(format string, args ...any) string { return fmt.Sprintf(T(format), args...) }

// System is the language of this machine: Spanish when the user's
// language is Spanish, English otherwise.
func System() Lang {
	var names []string
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" {
			names = append(names, v)
			break // the first one set wins, as in POSIX
		}
	}
	return fromLocales(append(names, uiLanguages()...))
}

// fromLocales reads the first locale name ("es-ES", "es_AR.UTF-8", "en").
func fromLocales(names []string) Lang {
	for _, n := range names {
		n = strings.ToLower(n)
		if n == "" || n == "c" || n == "posix" {
			continue
		}
		if n == "es" || strings.HasPrefix(n, "es-") || strings.HasPrefix(n, "es_") || strings.HasPrefix(n, "es.") {
			return Spanish
		}
		return English
	}
	return English
}
