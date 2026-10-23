package office

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// letterInk is the color one of a theme's own letters draws with.
type letterInk struct {
	c  screen.RGB
	ok bool
}

// reservedLetters is every character a built-in grid draws with, '.' and
// '#' and the digits included: a theme's own letters must be others, so a
// built-in letter always keeps its meaning.
var reservedLetters = func() string {
	s := ".#0123456789" + spriteLetters + catLetters
	for _, spec := range pieceSpecs {
		s += spec.letters
	}
	return s
}()

// freeLetters is the letters a theme may make its own, the way errors list
// them.
func freeLetters() string {
	var free []byte
	for _, ch := range []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz") {
		if strings.IndexByte(reservedLetters, ch) < 0 {
			free = append(free, ch)
		}
	}
	return spaced(string(free))
}

// ownInk is the color t's own letter ch draws with. A letter t does not
// define, a built-in one included, draws nothing here.
func (t *Theme) ownInk(ch byte) (screen.RGB, bool) {
	i := t.inks[ch]
	return i.c, i.ok
}

// ownLetters is t's own letters, sorted, which its grid files may use
// besides each grid's built-in ones.
func (t *Theme) ownLetters() string {
	b := make([]byte, 0, len(t.letters))
	for ch := range t.letters {
		b = append(b, ch)
	}
	slices.Sort(b)
	return string(b)
}

// resolveLetters works out the color each of t's own letters draws with,
// from the colors t has now, so drawing never looks a role up by name. Call
// it whenever t's colors change.
func (t *Theme) resolveLetters() {
	t.inks = [256]letterInk{}
	if len(t.letters) == 0 {
		return
	}
	colors := map[string]screen.RGB{}
	for _, f := range t.colorFields() {
		if f.one != nil {
			colors[f.section+"."+f.key] = *f.one
		}
	}
	for k, c := range t.Custom {
		colors["custom."+k] = c
	}
	for ch, role := range t.letters {
		if c, ok := colors[role]; ok {
			t.inks[ch] = letterInk{c, true}
		}
	}
}

// readCustom reads [custom]: the theme's own colors, each a key of
// lowercase letters, digits and _ to one color.
func (t *Theme) readCustom(path string, src []byte, values map[string]any) error {
	for _, key := range sortedKeys(values) {
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%s:%d: custom.%s: %s", path, lineOf(src, "custom", key), key, fmt.Sprintf(format, args...))
		}
		if strings.Trim(key, "abcdefghijklmnopqrstuvwxyz0123456789_") != "" {
			return fail("a key is lowercase letters, digits and _, like antenna")
		}
		switch v := values[key].(type) {
		case string:
			c, err := parseColor(v)
			if err != nil {
				return fail("%v", err)
			}
			if t.Custom == nil {
				t.Custom = map[string]screen.RGB{}
			}
			t.Custom[key] = c
		case []any:
			return fail("want one color like \"#2b1d14\", got a list")
		default:
			return fail("want a color like \"#2b1d14\", got %v", v)
		}
		t.set["custom."+key] = true
	}
	return nil
}

// readLetters reads [letters]: the theme's own letters, each one ASCII
// letter no built-in grid uses, to the role it draws with: one standard
// color or one of the theme's [custom] colors, as "section.key".
func (t *Theme) readLetters(path string, src []byte, values map[string]any) error {
	fields := map[string]colorField{}
	for _, f := range t.colorFields() {
		fields[f.section+"."+f.key] = f
	}
	for _, key := range sortedKeys(values) {
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%s:%d: letters.%s: %s", path, lineOf(src, "letters", key), key, fmt.Sprintf(format, args...))
		}
		if len(key) != 1 || !('a' <= key[0] && key[0] <= 'z' || 'A' <= key[0] && key[0] <= 'Z') {
			return fail("a letter is one letter from a to z or A to Z")
		}
		if strings.Contains(reservedLetters, key) {
			return fail("the built-in grids use %s; free letters are %s", key, freeLetters())
		}
		role, ok := values[key].(string)
		if !ok {
			return fail("want a role like \"custom.antenna\", got %v", values[key])
		}
		if f, ok := fields[role]; ok && f.list != nil {
			return fail("%s is a list of colors; a letter draws with one", role)
		}
		known := fields[role].one != nil
		if key, ok := strings.CutPrefix(role, "custom."); ok {
			_, known = t.Custom[key]
		}
		if !known {
			return fail("unknown role %q; use a color like \"furniture.glass\" or one of [custom] like \"custom.antenna\"", role)
		}
		if t.letters == nil {
			t.letters = map[byte]string{}
		}
		t.letters[key[0]] = role
	}
	return nil
}

// writeOwn writes t's [custom] colors and [letters], when it has them, the
// way readCustom and readLetters read them.
func writeOwn(b *strings.Builder, t Theme) {
	if len(t.Custom) > 0 {
		b.WriteString("\n[custom]\n# This theme's own colors; its [letters] draw with them as custom.<key>.\n")
		keys := make([]string, 0, len(t.Custom))
		for k := range t.Custom {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			fmt.Fprintf(b, "%s = %q\n", k, formatColor(t.Custom[k]))
		}
	}
	if letters := t.ownLetters(); letters != "" {
		b.WriteString("\n[letters]\n# Letters this theme's grid files use besides the built-in ones.\n")
		for i := range len(letters) {
			fmt.Fprintf(b, "%c = %s\n", letters[i], strconv.Quote(t.letters[letters[i]]))
		}
	}
}
