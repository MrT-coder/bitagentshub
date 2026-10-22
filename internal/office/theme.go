package office

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/BurntSushi/toml"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// Theme is everything a theme can change: the people's palettes, the room's
// colors and the characters' poses. Desks and furniture keep their look.
type Theme struct {
	Name, Author string

	// Each character gets a hair, skin and pants color picked from these
	// lists by its id; Agents are the shirt colors, picked by session id.
	Hair, Skin, Pants, Agents    []screen.RGB
	Eye, Shoes, Paper, PaperLine screen.RGB
	WallTop, WallFill, WallPanel screen.RGB
	Baseboard                    screen.RGB
	FloorA, FloorB, FloorSeam    screen.RGB
	Carpets                      []screen.RGB // one per area, cycling

	poses [poseCount][]frame
}

type poseID int

const (
	poseWalkDown poseID = iota
	poseWalkUp
	poseWalkSide
	poseStandDown
	poseStandUp
	poseStandSide
	poseTyping
	poseReading
	poseThinking
	poseBlocked
	poseAsleep
	poseLounging
	poseCount
)

// poseSpecs names each pose file and the frames it must hold. Seated poses
// sit behind a desk, which covers their last two rows.
var poseSpecs = [poseCount]struct {
	name   string
	frames int
	seated bool
}{
	poseWalkDown:  {"walk-down", 2, false},
	poseWalkUp:    {"walk-up", 2, false},
	poseWalkSide:  {"walk-side", 2, false},
	poseStandDown: {"stand-down", 1, false},
	poseStandUp:   {"stand-up", 1, false},
	poseStandSide: {"stand-side", 1, false},
	poseTyping:    {"typing", 2, true},
	poseReading:   {"reading", 2, true},
	poseThinking:  {"thinking", 2, true},
	poseBlocked:   {"blocked", 2, true},
	poseAsleep:    {"asleep", 2, true},
	poseLounging:  {"lounging", 1, false},
}

const spriteLetters = "HSEBbPFWw."

// active is the theme everything is drawn with. It is read on every frame,
// so it is a resolved value, never looked up by name; it is an atomic
// pointer because the settings window applies a theme from a background
// command while the office keeps drawing.
var active atomic.Pointer[Theme]

func init() { SetTheme(BuiltinTheme()) }

// SetTheme makes t the theme the office is drawn with, from the next frame
// on. t comes from BuiltinTheme or LoadTheme.
func SetTheme(t Theme) { active.Store(&t) }

// cur is the active theme. Load it once per call and read every field from
// that one value, so a theme applied meanwhile is never half seen.
func cur() *Theme { return active.Load() }

// BuiltinTheme is the office as it looks without a theme.
func BuiltinTheme() Theme {
	return Theme{
		Name:  "default",
		Hair:  []screen.RGB{rgb(0x2b1d14), rgb(0x5a3a22), rgb(0xc89b4a), rgb(0x8e3b2a), rgb(0x9a9aa0), rgb(0x1c1c24)},
		Skin:  []screen.RGB{rgb(0xf5d0a9), rgb(0xe0ac7e), rgb(0xc0865a), rgb(0x8d5a3b), rgb(0x5e3c27)},
		Pants: []screen.RGB{rgb(0x3b4a6b), rgb(0x2e3440), rgb(0x5a4a3a), rgb(0x4a5545)},
		Agents: []screen.RGB{
			rgb(0xe06c75), rgb(0x98c379), rgb(0xe5c07b), rgb(0x61afef),
			rgb(0xc678dd), rgb(0x56b6c2), rgb(0xd19a66), rgb(0xbe5046),
		},
		Eye:       rgb(0x22222a),
		Shoes:     rgb(0x24201c),
		Paper:     rgb(0xf0f0e8),
		PaperLine: rgb(0x8a8a90),

		WallTop:   rgb(0x3b4252),
		WallFill:  rgb(0x4c566a),
		WallPanel: rgb(0x465063),
		Baseboard: rgb(0x2e3440),
		FloorA:    rgb(0x6b4f3a),
		FloorB:    rgb(0x634935),
		FloorSeam: rgb(0x4e3a2b),
		Carpets:   []screen.RGB{rgb(0x3d5a6c), rgb(0x5c4a6b), rgb(0x4f6b4a), rgb(0x6b5a3d), rgb(0x6b3d4a)},

		poses: builtinPoses(),
	}
}

// colorField is one color setting of theme.toml and where it lands in a
// Theme: exactly one of one and list is set.
type colorField struct {
	section, key string
	one          *screen.RGB
	list         *[]screen.RGB
}

// colorFields lists every color setting in the order theme.toml writes them.
func (t *Theme) colorFields() []colorField {
	return []colorField{
		{"palette", "hair", nil, &t.Hair},
		{"palette", "skin", nil, &t.Skin},
		{"palette", "pants", nil, &t.Pants},
		{"palette", "agents", nil, &t.Agents},
		{"palette", "eye", &t.Eye, nil},
		{"palette", "shoes", &t.Shoes, nil},
		{"palette", "paper", &t.Paper, nil},
		{"palette", "paper_line", &t.PaperLine, nil},
		{"room", "wall_top", &t.WallTop, nil},
		{"room", "wall_fill", &t.WallFill, nil},
		{"room", "wall_panel", &t.WallPanel, nil},
		{"room", "baseboard", &t.Baseboard, nil},
		{"room", "floor_a", &t.FloorA, nil},
		{"room", "floor_b", &t.FloorB, nil},
		{"room", "floor_seam", &t.FloorSeam, nil},
		{"room", "carpets", nil, &t.Carpets},
	}
}

// LoadTheme reads the theme in dir: theme.toml and any characters/<pose>.txt.
// Whatever the theme leaves out comes from the built-in theme. Anything it
// cannot use is an error naming the file, the field or pose, and the line.
func LoadTheme(dir string) (Theme, error) {
	t := BuiltinTheme()
	t.Name = filepath.Base(dir)
	if err := t.readManifest(filepath.Join(dir, "theme.toml")); err != nil {
		return Theme{}, err
	}
	if err := t.readPoses(filepath.Join(dir, "characters")); err != nil {
		return Theme{}, err
	}
	return t, nil
}

func (t *Theme) readManifest(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var file struct {
		Name    string         `toml:"name"`
		Author  string         `toml:"author"`
		Palette map[string]any `toml:"palette"`
		Room    map[string]any `toml:"room"`
	}
	md, err := toml.Decode(string(src), &file)
	if err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			// Error repeats the line we already print; keep the message and key.
			line, msg := pe.Position.Line, pe.Error()
			msg = strings.TrimPrefix(msg, fmt.Sprintf("toml: line %d: ", line))
			msg = strings.TrimPrefix(msg, fmt.Sprintf("toml: line %d (last key %q): ", line, pe.LastKey))
			if pe.LastKey != "" {
				msg = pe.LastKey + ": " + msg
			}
			return fmt.Errorf("%s:%d: %s", path, line, msg)
		}
		return fmt.Errorf("%s: %w", path, err)
	}
	if u := md.Undecoded(); len(u) > 0 {
		k := u[0]
		line := lineOf(src, strings.Join(k[:len(k)-1], "."), k[len(k)-1])
		return fmt.Errorf("%s:%d: %s: unknown setting", path, line, k)
	}
	if md.IsDefined("name") {
		t.Name = file.Name
	}
	t.Author = file.Author

	fields := map[string]colorField{}
	for _, f := range t.colorFields() {
		fields[f.section+"."+f.key] = f
	}
	for section, values := range map[string]map[string]any{"palette": file.Palette, "room": file.Room} {
		for _, key := range sortedKeys(values) {
			name := section + "." + key
			fail := func(format string, args ...any) error {
				return fmt.Errorf("%s:%d: %s: %s", path, lineOf(src, section, key), name, fmt.Sprintf(format, args...))
			}
			f, ok := fields[name]
			if !ok {
				return fail("unknown setting")
			}
			switch v := values[key].(type) {
			case string:
				if f.one == nil {
					return fail("want a list of colors like [\"#2b1d14\"], got %q", v)
				}
				c, err := parseColor(v)
				if err != nil {
					return fail("%v", err)
				}
				*f.one = c
			case []any:
				if f.list == nil {
					return fail("want one color like \"#2b1d14\", got a list")
				}
				if len(v) == 0 {
					return fail("the list is empty; give at least one color")
				}
				list := make([]screen.RGB, len(v))
				for i, e := range v {
					s, ok := e.(string)
					if !ok {
						return fail("want colors like \"#2b1d14\", got %v", e)
					}
					if list[i], err = parseColor(s); err != nil {
						return fail("%v", err)
					}
				}
				*f.list = list
			default:
				return fail("want a color like \"#2b1d14\", got %v", v)
			}
		}
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// lineOf finds the line where key is set inside [section] ("" for the top
// of the file), or 0 when it cannot tell. The TOML decoder does not report
// positions of values it accepted, so errors about them find it here.
func lineOf(src []byte, section, key string) int {
	current := ""
	sc := bufio.NewScanner(bytes.NewReader(src))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			current = strings.TrimSpace(strings.Trim(line, "[]"))
			// A whole unknown table is reported by its header.
			if current == strings.TrimPrefix(section+"."+key, ".") {
				return n
			}
			continue
		}
		k, _, ok := strings.Cut(line, "=")
		if ok && current == section && strings.Trim(strings.TrimSpace(k), `"`) == key {
			return n
		}
	}
	return 0
}

func parseColor(s string) (screen.RGB, error) {
	if len(s) != 7 || s[0] != '#' {
		return screen.RGB{}, fmt.Errorf("%q is not a #rrggbb color", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return screen.RGB{}, fmt.Errorf("%q is not a #rrggbb color", s)
	}
	return rgb(uint32(v)), nil
}

func formatColor(c screen.RGB) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func (t *Theme) readPoses(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}
	byName := map[string]poseID{}
	names := make([]string, poseCount)
	for p, spec := range poseSpecs {
		byName[spec.name] = poseID(p)
		names[p] = spec.name
	}
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".txt")
		if !ok || e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		p, known := byName[name]
		if !known {
			return fmt.Errorf("%s: unknown pose; pose files are %s", path, strings.Join(names, ", "))
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		frames, err := parseFrames(path, src, p)
		if err != nil {
			return err
		}
		t.poses[p] = frames
	}
	return nil
}

// parseFrames reads the pose file at path: frames of spriteH rows of
// spriteW letters, separated by one blank line.
func parseFrames(path string, src []byte, p poseID) ([]frame, error) {
	spec := poseSpecs[p]
	fail := func(line int, format string, args ...any) error {
		at := path
		if line > 0 {
			at += ":" + strconv.Itoa(line)
		}
		return fmt.Errorf("%s: pose %s: %s", at, spec.name, fmt.Sprintf(format, args...))
	}
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	var frames []frame
	var rows []string
	start := 1 // line of the first row of the frame being read
	flush := func() error {
		if len(rows) != spriteH {
			return fail(start, "frame %d has %d rows, want %d", len(frames)+1, len(rows), spriteH)
		}
		var f frame
		copy(f[:], rows)
		frames, rows = append(frames, f), nil
		return nil
	}
	for i, raw := range lines {
		n, row := i+1, strings.TrimRight(raw, " \t\r")
		if row == "" {
			if len(rows) == 0 {
				return nil, fail(n, "frames are separated by one blank line")
			}
			if err := flush(); err != nil {
				return nil, err
			}
			start = n + 1
			continue
		}
		if len(row) != spriteW {
			return nil, fail(n, "row is %d wide, want %d", len(row), spriteW)
		}
		for col := 0; col < len(row); col++ {
			if !strings.ContainsRune(spriteLetters, rune(row[col])) {
				return nil, fail(n, "unknown letter %q in column %d; use H S E B b P F W w or .", row[col], col+1)
			}
		}
		if spec.seated && len(rows) >= spriteH-2 && row != strings.Repeat(".", spriteW) {
			return nil, fail(n, "rows %d and %d must be all '.': the desk covers them", spriteH-1, spriteH)
		}
		rows = append(rows, row)
	}
	if len(rows) > 0 {
		if err := flush(); err != nil {
			return nil, err
		}
	}
	if len(frames) != spec.frames {
		plural := "s"
		if len(frames) == 1 {
			plural = ""
		}
		return nil, fail(0, "has %d frame%s, want %d", len(frames), plural, spec.frames)
	}
	return frames, nil
}

// WriteTheme writes t into dir in the format LoadTheme reads: theme.toml
// and one file per pose. It refuses a folder that already has files, so it
// never overwrites someone's theme.
func WriteTheme(dir string, t Theme) error {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", dir, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s already exists and is not empty", dir)
	}
	if err := os.MkdirAll(filepath.Join(dir, "characters"), 0o755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# A bah theme. Colors are #rrggbb; anything left out comes\n")
	b.WriteString("# from the built-in theme. Character poses live in characters/.\n")
	fmt.Fprintf(&b, "name = %s\nauthor = %s\n", tomlString(t.Name), tomlString(t.Author))
	section := ""
	for _, f := range t.colorFields() {
		if f.section != section {
			section = f.section
			fmt.Fprintf(&b, "\n[%s]\n", section)
		}
		if f.one != nil {
			fmt.Fprintf(&b, "%s = %q\n", f.key, formatColor(*f.one))
			continue
		}
		colors := make([]string, len(*f.list))
		for i, c := range *f.list {
			colors[i] = strconv.Quote(formatColor(c))
		}
		fmt.Fprintf(&b, "%s = [%s]\n", f.key, strings.Join(colors, ", "))
	}
	if err := os.WriteFile(filepath.Join(dir, "theme.toml"), []byte(b.String()), 0o644); err != nil {
		return err
	}

	for p, spec := range poseSpecs {
		var parts []string
		for _, f := range t.poses[p] {
			parts = append(parts, strings.Join(f[:], "\n"))
		}
		path := filepath.Join(dir, "characters", spec.name+".txt")
		if err := os.WriteFile(path, []byte(strings.Join(parts, "\n\n")+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, "\\u%04X", r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
