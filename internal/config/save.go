package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

const bom = "\xef\xbb\xbf"

// entryRe matches, in a table named table: its header (bare or quoted, with
// an optional comment), the start of a key = ... line, and that line split
// into the part before the value, a one-line value (a string, a bare value
// or an array of strings) and what follows it.
type entryRe struct{ header, key, line *regexp.Regexp }

func entryRegexps(table, key string) entryRe {
	name := func(n string) string {
		q := regexp.QuoteMeta(n)
		return `(?:` + q + `|"` + q + `"|'` + q + `')`
	}
	str := `"(?:[^"\\]|\\.)*"|'[^']*'`
	value := str + `|\[\s*(?:(?:` + str + `)\s*,?\s*)*\]|[^\s#\[]+`
	return entryRe{
		header: regexp.MustCompile(`^\[\s*` + name(table) + `\s*\]\s*(?:#.*)?$`),
		key:    regexp.MustCompile(`^\s*` + name(key) + `\s*=`),
		line:   regexp.MustCompile(`^(\s*` + name(key) + `\s*=\s*)(` + value + `)(\s*(?:#.*)?)$`),
	}
}

// SaveTheme sets [theme] name in the config file at path; "" removes the
// name line, which selects the built-in theme. It edits the text rather than
// re-encoding the file, so the user's comments, [keys], blank lines, line
// endings and byte order mark stay as they were. A file that does not parse,
// or that sets the theme in a form it cannot edit safely (a dotted key, an
// inline table), is left untouched and reported. The file is replaced in one
// rename, and only once Load reads the new name back from the result.
func SaveTheme(path, name string) error {
	if name != "" {
		if _, _, err := themeSection(map[string]any{"name": name}); err != nil {
			return err
		}
	}
	return saveEntry(path, "theme", "name", name, "theme", func(c Config) string { return c.Theme })
}

// SavePalette sets [theme] palette in the config file at path the way
// SaveTheme sets the name; "" removes the line, which keeps the theme's own
// colors.
func SavePalette(path, name string) error {
	if name != "" {
		if _, _, err := themeSection(map[string]any{"palette": name}); err != nil {
			return err
		}
	}
	return saveEntry(path, "theme", "palette", name, "palette", func(c Config) string { return c.Palette })
}

// saveEntry sets [table] key to value, a string, in the config file at
// path, or removes its line when value is "", the way SaveTheme describes.
// noun names the setting in errors; read gives it back from a loaded file.
func saveEntry(path, table, key, value, noun string, read func(Config) string) error {
	data, err := os.ReadFile(path)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		return err
	}
	if missing && value == "" {
		return nil
	}
	text, hasBOM := strings.CutPrefix(string(data), bom)
	before := map[string]any{}
	if _, err := toml.Decode(text, &before); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	_, defined := before[table]
	literal := ""
	if value != "" {
		literal = quote(value)
	}
	out, changed, err := editEntry(text, table, key, literal, "the "+noun, defined)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if !changed {
		return nil
	}
	// The edit must change this setting and nothing else; a line the
	// scanner misread would show up here instead of in the user's file.
	after := map[string]any{}
	if _, err := toml.Decode(out, &after); err != nil {
		return fmt.Errorf("%s: setting the %s would break the file: %w", path, noun, err)
	}
	var want any
	if value != "" {
		want = value
	}
	if !reflect.DeepEqual(after, withEntry(before, table, key, want)) {
		return fmt.Errorf("%s: setting the %s would change more than [%s] %s; edit it by hand", path, noun, table, key)
	}
	if hasBOM {
		out = bom + out
	}
	return replace(path, []byte(out), func(cfg Config) error {
		if got := read(cfg); got != value {
			return fmt.Errorf("%s: saved %s %q but the file reads %q", path, noun, value, got)
		}
		return nil
	})
}

// editEntry returns text with [table] key set to value, a TOML literal, or
// its line removed when value is "". defined tells whether the parsed file
// has the table at all, so one set without a [table] header is refused
// rather than duplicated; what names the setting in that error.
func editEntry(text, table, key, value, what string, defined bool) (string, bool, error) {
	re := entryRegexps(table, key)
	nl := "\n"
	if i := strings.Index(text, "\n"); i > 0 && text[i-1] == '\r' {
		nl = "\r\n"
	}
	lines := strings.SplitAfter(text, "\n")
	header, at := -1, -1
	inTable, inString := false, ""
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r\n")
		if inString != "" {
			// A header or name inside a multi-line string is text, not TOML.
			if strings.Count(line, inString)%2 == 1 {
				inString = ""
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			if inTable {
				break
			}
			if re.header.MatchString(trimmed) {
				header, inTable = i, true
			}
			continue
		}
		if inTable && re.key.MatchString(line) {
			at = i
		}
		for _, delim := range []string{`"""`, `'''`} {
			if strings.Count(line, delim)%2 == 1 {
				inString = delim
			}
		}
	}

	switch {
	case header < 0 && defined:
		return "", false, fmt.Errorf("%s is not set in a [%s] table (a dotted key or an inline table); edit it by hand", what, table)
	case value == "" && at < 0:
		return text, false, nil
	case value == "":
		lines = append(lines[:at], lines[at+1:]...)
		return strings.Join(lines, ""), true, nil
	case header < 0:
		out := text
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += nl
		}
		if out != "" {
			out += nl
		}
		return out + "[" + table + "]" + nl + key + " = " + value + nl, true, nil
	case at >= 0:
		raw := lines[at]
		line := strings.TrimRight(raw, "\r\n")
		m := re.line.FindStringSubmatch(line)
		if m == nil || strings.HasPrefix(m[2], `""`) || strings.HasPrefix(m[2], `''`) {
			return "", false, fmt.Errorf("%s.%s is not a one-line value; edit it by hand", table, key)
		}
		lines[at] = m[1] + value + m[3] + raw[len(line):]
		return strings.Join(lines, ""), true, nil
	default:
		if !strings.HasSuffix(lines[header], "\n") {
			lines[header] += nl
		}
		rest := append([]string{key + " = " + value + nl}, lines[header+1:]...)
		lines = append(lines[:header+1], rest...)
		return strings.Join(lines, ""), true, nil
	}
}

// withEntry is the parsed file as it should read after the edit: [table]
// key set to value, or removed when value is nil.
func withEntry(file map[string]any, table, key string, value any) map[string]any {
	out := map[string]any{}
	for k, v := range file {
		out[k] = v
	}
	t := map[string]any{}
	if old, ok := file[table].(map[string]any); ok {
		for k, v := range old {
			t[k] = v
		}
	}
	if value == nil {
		delete(t, key)
	} else {
		t[key] = value
	}
	out[table] = t
	return out
}

// quote writes a TOML basic string.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// replace writes data next to path, checks that Load reads it and that
// check agrees with what it read, and renames it over path, so the office
// never reads a half-written or unusable config.
func replace(path string, data []byte, check func(Config) error) error {
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err == nil {
		var cfg Config
		cfg, err = Load(tmp)
		switch {
		case err != nil:
			err = errors.New(strings.ReplaceAll(err.Error(), tmp, path))
		default:
			err = check(cfg)
		}
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
