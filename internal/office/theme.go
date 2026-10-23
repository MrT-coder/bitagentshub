package office

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
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
// colors, the furniture's and the pet's colors, the characters' poses, the
// windows and the terminals' colors.
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

	Furniture Furniture
	Pet       PetColors

	// The windows (terminals, board, settings): background, text and
	// border. Terminal text without colors of its own uses them too.
	UIBg, UIText, UIAccent screen.RGB
	// Terminal recolors the 16 ANSI colors programs ask for; unset, they
	// keep the emulator's own.
	Terminal screen.Palette

	poses [poseCount][]frame
	// set holds the color settings its theme.toml gives, as "section.key";
	// the built-in theme sets none, since it is the base under every theme.
	set map[string]bool
}

// sets reports whether t's theme.toml gives the color setting key, as
// "section.key"; colors it leaves out came from the built-in theme.
func (t *Theme) sets(key string) bool { return t.set[key] }

// Furniture is the colors of the desks, of what hangs on the wall and of
// what stands on the floor.
type Furniture struct {
	DeskTop, DeskFront, DeskLeg, ChairBack, ChairEdge screen.RGB
	Bezel, ScreenOff, Mug, Coffee                     screen.RGB

	// What a monitor shows, by what its agent does: writing, blocked on a
	// prompt, thinking, reading or running a command. Each ink list cycles
	// down the lines of text.
	ScreenCode, ScreenBlocked, ScreenThinking, ThinkingDot screen.RGB
	ScreenReading, ScreenRunning                           screen.RGB
	CodeInk, BlockedInk, ReadingInk, RunningInk            []screen.RGB

	WindowFrame, Glass, GlassShine      screen.RGB
	PaintingFrame, PaintingHill         screen.RGB
	Paintings                           []screen.RGB // one canvas per painting, cycling
	ClockFace, ClockHand                screen.RGB
	Shelf, ShelfBack                    screen.RGB
	Books                               []screen.RGB
	Machine, MachinePanel               screen.RGB // the coffee machine
	MachineTray, MachineBase            screen.RGB
	MachineLight, MachineLightOff       screen.RGB // its light blinks between them
	CoolerBottle, CoolerBody, CoolerTap screen.RGB
	Sofa, SofaLeg                       screen.RGB
	Leaf, Pot, PotBase                  screen.RGB
}

// PetColors is the colors of the pet's corner: the pet, its bed and bowl.
type PetColors struct {
	Fur, Shade, Eye, Nose, Muzzle screen.RGB
	BedRim, BedPad, Bowl, Food    screen.RGB
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

// Chrome is the windows' colors in the active theme.
type Chrome struct{ Bg, Text, Accent screen.RGB }

// UI is the windows' colors of the active theme.
func UI() Chrome {
	t := cur()
	return Chrome{Bg: t.UIBg, Text: t.UIText, Accent: t.UIAccent}
}

// TerminalPalette is the terminals' palette of the active theme.
func TerminalPalette() screen.Palette { return cur().Terminal }

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

		Furniture: Furniture{
			DeskTop: rgb(0xc8a272), DeskFront: rgb(0x8b6a45), DeskLeg: rgb(0x5c4630),
			ChairBack: rgb(0x434c5e), ChairEdge: rgb(0x2e3440),
			Bezel: rgb(0x2b303b), ScreenOff: rgb(0x15181e), Mug: rgb(0xe5e9f0), Coffee: rgb(0x6b4226),

			ScreenCode:     rgb(0x1d2b45),
			CodeInk:        []screen.RGB{rgb(0x61afef), rgb(0xc678dd), rgb(0x98c379), rgb(0xe5c07b)},
			ScreenBlocked:  rgb(0x4a1d1d),
			BlockedInk:     []screen.RGB{rgb(0xff6b6b)},
			ScreenThinking: rgb(0x262a36),
			ThinkingDot:    rgb(0xe5c07b),
			ScreenReading:  rgb(0xe8e8e0),
			ReadingInk:     []screen.RGB{rgb(0x8a8a90)},
			ScreenRunning:  rgb(0x0d120d),
			RunningInk:     []screen.RGB{rgb(0x7fd962)},

			WindowFrame: rgb(0xd8dee9), Glass: rgb(0x88c0d0), GlassShine: rgb(0xb5dbe6),
			PaintingFrame: rgb(0xa3815b), PaintingHill: rgb(0xa3be8c),
			Paintings: []screen.RGB{rgb(0x81a1c1), rgb(0xbf616a)},
			ClockFace: rgb(0xe5e9f0), ClockHand: rgb(0xbf616a),
			Shelf: rgb(0x6b4a2f), ShelfBack: rgb(0x3e2b1c),
			Books:   []screen.RGB{rgb(0xbf616a), rgb(0x5e81ac), rgb(0xa3be8c), rgb(0xebcb8b), rgb(0xb48ead)},
			Machine: rgb(0x5a5f6b), MachinePanel: rgb(0x2e3440),
			MachineTray: rgb(0x1c1f26), MachineBase: rgb(0x434c5e),
			MachineLight: rgb(0xbf616a), MachineLightOff: rgb(0x7f2f35),
			CoolerBottle: rgb(0x88c0d0), CoolerBody: rgb(0xd8dee9), CoolerTap: rgb(0x5e81ac),
			Sofa: rgb(0x7a4b5e), SofaLeg: rgb(0x5c4630),
			Leaf: rgb(0x4caf50), Pot: rgb(0xa0522d), PotBase: rgb(0x7a3e22),
		},
		Pet: PetColors{
			Fur: rgb(0xd08770), Shade: rgb(0xa0583a), Eye: rgb(0x2e3440), Nose: rgb(0xbf616a), Muzzle: rgb(0xeceff4),
			BedRim: rgb(0x4c566a), BedPad: rgb(0x5e81ac), Bowl: rgb(0xd8dee9), Food: rgb(0xebcb8b),
		},

		UIBg:     rgb(0x16181d),
		UIText:   rgb(0xdcdfe4),
		UIAccent: rgb(0x61afef),

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
	f, p := &t.Furniture, &t.Pet
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
		{"furniture", "desk_top", &f.DeskTop, nil},
		{"furniture", "desk_front", &f.DeskFront, nil},
		{"furniture", "desk_leg", &f.DeskLeg, nil},
		{"furniture", "chair_back", &f.ChairBack, nil},
		{"furniture", "chair_edge", &f.ChairEdge, nil},
		{"furniture", "bezel", &f.Bezel, nil},
		{"furniture", "screen_off", &f.ScreenOff, nil},
		{"furniture", "mug", &f.Mug, nil},
		{"furniture", "coffee", &f.Coffee, nil},
		{"furniture", "screen_code", &f.ScreenCode, nil},
		{"furniture", "code_ink", nil, &f.CodeInk},
		{"furniture", "screen_blocked", &f.ScreenBlocked, nil},
		{"furniture", "blocked_ink", nil, &f.BlockedInk},
		{"furniture", "screen_thinking", &f.ScreenThinking, nil},
		{"furniture", "thinking_dot", &f.ThinkingDot, nil},
		{"furniture", "screen_reading", &f.ScreenReading, nil},
		{"furniture", "reading_ink", nil, &f.ReadingInk},
		{"furniture", "screen_running", &f.ScreenRunning, nil},
		{"furniture", "running_ink", nil, &f.RunningInk},
		{"furniture", "window_frame", &f.WindowFrame, nil},
		{"furniture", "glass", &f.Glass, nil},
		{"furniture", "glass_shine", &f.GlassShine, nil},
		{"furniture", "painting_frame", &f.PaintingFrame, nil},
		{"furniture", "paintings", nil, &f.Paintings},
		{"furniture", "painting_hill", &f.PaintingHill, nil},
		{"furniture", "clock_face", &f.ClockFace, nil},
		{"furniture", "clock_hand", &f.ClockHand, nil},
		{"furniture", "shelf", &f.Shelf, nil},
		{"furniture", "shelf_back", &f.ShelfBack, nil},
		{"furniture", "books", nil, &f.Books},
		{"furniture", "machine", &f.Machine, nil},
		{"furniture", "machine_panel", &f.MachinePanel, nil},
		{"furniture", "machine_tray", &f.MachineTray, nil},
		{"furniture", "machine_base", &f.MachineBase, nil},
		{"furniture", "machine_light", &f.MachineLight, nil},
		{"furniture", "machine_light_off", &f.MachineLightOff, nil},
		{"furniture", "cooler_bottle", &f.CoolerBottle, nil},
		{"furniture", "cooler_body", &f.CoolerBody, nil},
		{"furniture", "cooler_tap", &f.CoolerTap, nil},
		{"furniture", "sofa", &f.Sofa, nil},
		{"furniture", "sofa_leg", &f.SofaLeg, nil},
		{"furniture", "leaf", &f.Leaf, nil},
		{"furniture", "pot", &f.Pot, nil},
		{"furniture", "pot_base", &f.PotBase, nil},
		{"pet", "fur", &p.Fur, nil},
		{"pet", "shade", &p.Shade, nil},
		{"pet", "eye", &p.Eye, nil},
		{"pet", "nose", &p.Nose, nil},
		{"pet", "muzzle", &p.Muzzle, nil},
		{"pet", "bed_rim", &p.BedRim, nil},
		{"pet", "bed_pad", &p.BedPad, nil},
		{"pet", "bowl", &p.Bowl, nil},
		{"pet", "food", &p.Food, nil},
		{"ui", "background", &t.UIBg, nil},
		{"ui", "text", &t.UIText, nil},
		{"ui", "accent", &t.UIAccent, nil},
	}
}

// LoadTheme reads the theme in dir: theme.toml and any characters/<pose>.txt.
// Whatever the theme leaves out comes from the built-in theme. Anything it
// cannot use is an error naming the file, the field or pose, and the line.
func LoadTheme(dir string) (Theme, error) {
	return loadTheme(os.DirFS(dir), dir, filepath.Base(dir))
}

// loadTheme reads the theme folder fsys, named name unless its theme.toml
// says otherwise. Errors name files as paths under dir.
func loadTheme(fsys fs.FS, dir, name string) (Theme, error) {
	t := BuiltinTheme()
	t.Name = name
	t.set = map[string]bool{}
	if err := t.readManifest(fsys, filepath.Join(dir, "theme.toml")); err != nil {
		return Theme{}, err
	}
	if err := t.readPoses(fsys, filepath.Join(dir, "characters")); err != nil {
		return Theme{}, err
	}
	return t, nil
}

func (t *Theme) readManifest(fsys fs.FS, path string) error {
	src, err := fs.ReadFile(fsys, "theme.toml")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var file struct {
		Name      string         `toml:"name"`
		Author    string         `toml:"author"`
		Palette   map[string]any `toml:"palette"`
		Room      map[string]any `toml:"room"`
		Furniture map[string]any `toml:"furniture"`
		Pet       map[string]any `toml:"pet"`
		UI        map[string]any `toml:"ui"`
		Terminal  map[string]any `toml:"terminal"`
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
	sections := map[string]map[string]any{
		"palette": file.Palette, "room": file.Room, "furniture": file.Furniture, "pet": file.Pet, "ui": file.UI,
	}
	for section, values := range sections {
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
			t.set[name] = true
		}
	}
	return t.readTerminal(path, src, file.Terminal)
}

// readTerminal reads [terminal]: palette is the 16 ANSI colors, black to
// bright white.
func (t *Theme) readTerminal(path string, src []byte, values map[string]any) error {
	for _, key := range sortedKeys(values) {
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%s:%d: terminal.%s: %s", path, lineOf(src, "terminal", key), key, fmt.Sprintf(format, args...))
		}
		if key != "palette" {
			return fail("unknown setting")
		}
		list, ok := values[key].([]any)
		if !ok {
			return fail("want a list of 16 colors, black to bright white")
		}
		if len(list) != len(t.Terminal.C) {
			return fail("has %d colors, want 16: black to bright white", len(list))
		}
		for i, e := range list {
			s, ok := e.(string)
			if !ok {
				return fail("want colors like \"#2b1d14\", got %v", e)
			}
			c, err := parseColor(s)
			if err != nil {
				return fail("%v", err)
			}
			t.Terminal.C[i] = c
		}
		t.Terminal.Set = true
		t.set["terminal.palette"] = true
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

func (t *Theme) readPoses(fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, "characters")
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
		src, err := fs.ReadFile(fsys, "characters/"+e.Name())
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
	if t.Terminal.Set {
		colors := make([]string, len(t.Terminal.C))
		for i, c := range t.Terminal.C {
			colors[i] = strconv.Quote(formatColor(c))
		}
		b.WriteString("\n[terminal]\n# The 16 ANSI colors: black, red, green, yellow, blue, magenta, cyan,\n# white, then their bright versions.\n")
		fmt.Fprintf(&b, "palette = [%s]\n", strings.Join(colors, ", "))
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
