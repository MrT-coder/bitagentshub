package office

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/pet"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// posedOffice is a fixed office where every pose is on screen: six agents
// seated in each seated pose and the rest placed by hand walking, standing
// and lounging in every direction.
func posedOffice() (Layout, *Crowd, []claude.Agent) {
	mk := func(id, group string, st claude.State) claude.Agent { return agent(id, group, id, st) }
	agents := []claude.Agent{
		mk("typer", "desk", claude.StateTool), mk("editor", "desk", claude.StateTool),
		mk("reader", "desk", claude.StateTool), mk("thinker", "desk", claude.StateThinking),
		mk("blocked", "desk", claude.StateTool), mk("sleeper", "desk", claude.StateSleeping),
		mk("walk-left", "floor", claude.StateWaiting), mk("walk-right", "floor", claude.StateWaiting),
		mk("walk-up", "floor", claude.StateWaiting), mk("walk-down", "floor", claude.StateWaiting),
		mk("stand-left", "floor", claude.StateWaiting), mk("stand-up", "floor", claude.StateWaiting),
		mk("stand-down", "floor", claude.StateWaiting), mk("lounger", "floor", claude.StateWaiting),
	}
	agents[0].Activity = claude.ActivityRunning
	agents[2].Activity = claude.ActivityReading
	agents[4].Blocked = true
	agents[0].Today = claude.Usage{Output: 320_000}

	l := Build(agents, 160, 90)
	c := NewCrowd(rand.New(rand.NewSource(1)))
	c.Sync(l, agents)
	for i, ag := range agents {
		a := c.Actor(ag.ID)
		a.path, a.slideActive, a.Pause = nil, false, 999
		if i < 6 {
			st := l.Stations[ag.ID]
			a.X, a.Y, a.Seated, a.Moving, a.Face = st.SeatX(), st.SeatY(), true, false, faceDown
			continue
		}
		a.Seated, a.X, a.Y = false, 4+(i-6)*12, l.H-20
		switch ag.ID {
		case "walk-left":
			a.Moving, a.Face, a.Left = true, faceSide, true
		case "walk-right":
			a.Moving, a.Face, a.Left = true, faceSide, false
		case "walk-up":
			a.Moving, a.Face = true, faceUp
		case "walk-down":
			a.Moving, a.Face = true, faceDown
		case "stand-left":
			a.Face, a.Left = faceSide, true
		case "stand-up":
			a.Face = faceUp
		case "lounger":
			a.At = "sofa"
		}
	}
	return l, c, agents
}

// officeHash renders the posed office over enough ticks to show every
// animation frame and hashes the terminal output.
func officeHash() string {
	l, c, agents := posedOffice()
	h := sha256.New()
	buf := screen.New(l.W, l.H/2)
	for _, tick := range []int{0, 1, 2, 3, 4, 5, 6, 7, 9, 13, 60, 125} {
		for _, ag := range agents {
			c.Actor(ag.ID).Step = tick
		}
		Draw(buf, l, c, agents, tick, "reader", pet.Sleeping)
		fmt.Fprintln(h, strings.Join(buf.Lines(), "\n"))
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// crowdHash lets a seeded crowd walk around and hashes every frame.
func crowdHash() string {
	agents := []claude.Agent{
		agent("a", "g", "p", claude.StateTool), agent("b", "g", "q", claude.StateWaiting),
		agent("c", "h", "r", claude.StateWaiting), agent("d", "h", "s", claude.StateSleeping),
	}
	l := Build(agents, testW, testH)
	c := NewCrowd(rand.New(rand.NewSource(7)))
	c.Sync(l, agents)
	h := sha256.New()
	buf := screen.New(l.W, l.H/2)
	for tick := 0; tick < 400; tick++ {
		c.Step(l, agents)
		if tick%5 == 0 {
			Draw(buf, l, c, agents, tick, "", pet.Sleeping)
			fmt.Fprintln(h, strings.Join(buf.Lines(), "\n"))
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// The references were captured from the drawing code before themes
// existed, so the built-in theme must keep the office pixel for pixel. They
// were taken again when the desk labels switched to herdr's statuses, and
// when the pet's corner moved the left plant, closed the corner to walkers
// and showed the pet asleep there.
const (
	posedOfficeRef = "4ec61fa74ba60e726b94e4894e53ebcda5101afc26d947f0e3572510321ca88d"
	crowdRef       = "fe92a4261e89fc4eaa8b0a0d7ddb31583a8356cded81e9839cace49d4046924d"
)

func TestBuiltinTheme_DrawsTheOfficeExactlyAsBefore(t *testing.T) {
	if got := officeHash(); got != posedOfficeRef {
		t.Errorf("posed office hash = %s, want %s", got, posedOfficeRef)
	}
	if got := crowdHash(); got != crowdRef {
		t.Errorf("walking crowd hash = %s, want %s", got, crowdRef)
	}
}

func BenchmarkDraw(b *testing.B) {
	l, c, agents := posedOffice()
	buf := screen.New(l.W, l.H/2)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Draw(buf, l, c, agents, i, "", pet.Sleeping)
	}
}

// useTheme makes th active for one test.
func useTheme(tb testing.TB, th Theme) {
	tb.Helper()
	SetTheme(th)
	tb.Cleanup(func() { SetTheme(BuiltinTheme()) })
}

func TestPosedOffice_ShowsEveryPose(t *testing.T) {
	for p := poseID(0); p < poseCount; p++ {
		t.Run(poseSpecs[p].name, func(t *testing.T) {
			th := BuiltinTheme()
			th.poses[p] = blockFrames(poseSpecs[p].frames)
			useTheme(t, th)
			if officeHash() == posedOfficeRef {
				t.Errorf("changing %s does not change the posed office", poseSpecs[p].name)
			}
		})
	}
}

// blockFrames is n frames of solid shirt that leave the desk rows empty,
// so they are valid for every pose.
func blockFrames(n int) []frame {
	var f frame
	for r := range f {
		f[r] = "BBBBBBBB"
	}
	f[spriteH-2], f[spriteH-1] = "........", "........"
	out := make([]frame, n)
	for i := range out {
		out[i] = f
	}
	return out
}

func gridText(frames []frame) string {
	var parts []string
	for _, f := range frames {
		parts = append(parts, strings.Join(f[:], "\n"))
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadTheme_TakesEveryAbsentColorFromTheBuiltinTheme(t *testing.T) {
	custom := screen.RGB{R: 0x12, G: 0x34, B: 0x56}
	var keys Theme
	for i, f := range keys.colorFields() {
		t.Run(f.section+"."+f.key, func(t *testing.T) {
			value := `"#123456"`
			if f.list != nil {
				value = `["#123456"]`
			}
			dir := writeFiles(t, map[string]string{
				"theme.toml": fmt.Sprintf("name = \"mine\"\n[%s]\n%s = %s\n", f.section, f.key, value),
			})
			got, err := LoadTheme(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := BuiltinTheme()
			want.Name = "mine"
			if wf := want.colorFields()[i]; wf.list != nil {
				*wf.list = []screen.RGB{custom}
			} else {
				*wf.one = custom
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("LoadTheme with only %s.%s set:\n got %+v\nwant %+v", f.section, f.key, got, want)
			}
		})
	}
}

func TestLoadTheme_TakesEveryAbsentPoseFromTheBuiltinTheme(t *testing.T) {
	for p := poseID(0); p < poseCount; p++ {
		spec := poseSpecs[p]
		t.Run(spec.name, func(t *testing.T) {
			frames := blockFrames(spec.frames)
			dir := writeFiles(t, map[string]string{
				"theme.toml":                       "name = \"mine\"\n",
				"characters/" + spec.name + ".txt": gridText(frames),
			})
			got, err := LoadTheme(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := BuiltinTheme()
			want.Name = "mine"
			want.poses[p] = frames
			if !reflect.DeepEqual(got, want) {
				t.Errorf("LoadTheme with only %s.txt differs from the built-in theme elsewhere", spec.name)
			}
		})
	}
}

func TestLoadTheme_NamesTheFileFieldAndLineOfWhatItCannotUse(t *testing.T) {
	typing := BuiltinTheme().poses[poseTyping]
	text := strings.Split(strings.TrimSuffix(gridText(typing), "\n"), "\n")
	edit := func(line int, row string) string { // line is 1-based
		lines := append([]string(nil), text...)
		lines[line-1] = row
		return strings.Join(lines, "\n") + "\n"
	}
	manifest := "name = \"mine\"\n"
	tests := []struct {
		name  string
		files map[string]string
		file  string // the file the error must name
		line  int    // 0 when there is no line to name
		parts []string
	}{
		{"bad color in a list", map[string]string{"theme.toml": "[palette]\nhair = [\"#2b1d14\", \"#zzzzzz\"]\n"},
			"theme.toml", 2, []string{"palette.hair", "#zzzzzz", "#rrggbb"}},
		{"short color", map[string]string{"theme.toml": "name = \"x\"\n\n[room]\nwall_top = \"#fff\"\n"},
			"theme.toml", 4, []string{"room.wall_top", "#fff"}},
		{"unknown palette key", map[string]string{"theme.toml": "[palette]\neye = \"#000000\"\nhat = \"#000000\"\n"},
			"theme.toml", 3, []string{"palette.hat", "unknown"}},
		{"unknown room key", map[string]string{"theme.toml": "[room]\nfloor = \"#000000\"\n"},
			"theme.toml", 2, []string{"room.floor", "unknown"}},
		{"empty list", map[string]string{"theme.toml": "[room]\ncarpets = []\n"},
			"theme.toml", 2, []string{"room.carpets", "empty"}},
		{"list where one color goes", map[string]string{"theme.toml": "[palette]\neye = [\"#000000\"]\n"},
			"theme.toml", 2, []string{"palette.eye"}},
		{"one color where a list goes", map[string]string{"theme.toml": "[palette]\nskin = \"#000000\"\n"},
			"theme.toml", 2, []string{"palette.skin"}},
		{"unknown top-level key", map[string]string{"theme.toml": "name = \"x\"\ncolour = \"#000000\"\n"},
			"theme.toml", 2, []string{"colour", "unknown"}},
		{"broken toml", map[string]string{"theme.toml": "name = \"x\"\n[palette]\neye = #000000\n"},
			"theme.toml", 3, []string{"palette.eye"}},
		{"missing manifest", map[string]string{"characters/typing.txt": gridText(typing)},
			"theme.toml", 0, nil},
		{"row too wide", map[string]string{"theme.toml": manifest, "characters/typing.txt": edit(3, ".HSSSSH..")},
			"characters/typing.txt", 3, []string{"pose typing", "9 wide", "8"}},
		{"unknown letter", map[string]string{"theme.toml": manifest, "characters/typing.txt": edit(2, ".HHXHHH.")},
			"characters/typing.txt", 2, []string{"pose typing", "'X'", "column 4"}},
		{"too few rows", map[string]string{"theme.toml": manifest,
			"characters/stand-up.txt": strings.Join(BuiltinTheme().poses[poseStandUp][0][:13], "\n") + "\n"},
			"characters/stand-up.txt", 1, []string{"pose stand-up", "13 rows", "14"}},
		{"wrong frame count", map[string]string{"theme.toml": manifest, "characters/typing.txt": gridText(typing[:1])},
			"characters/typing.txt", 0, []string{"pose typing", "1 frame", "want 2"}},
		{"seated pose drawing under the desk", map[string]string{"theme.toml": manifest, "characters/typing.txt": edit(13, "..FF....")},
			"characters/typing.txt", 13, []string{"pose typing", "rows 13", "14"}},
		{"two blank lines between frames", map[string]string{"theme.toml": manifest,
			"characters/typing.txt": strings.Replace(gridText(typing), "\n\n", "\n\n\n", 1)},
			"characters/typing.txt", 16, []string{"pose typing", "blank line"}},
		{"unknown pose file", map[string]string{"theme.toml": manifest, "characters/sitting.txt": gridText(typing)},
			"characters/sitting.txt", 0, []string{"unknown pose", "typing"}},
		{"unknown ui key", map[string]string{"theme.toml": "[ui]\nborder = \"#000000\"\n"},
			"theme.toml", 2, []string{"ui.border", "unknown"}},
		{"terminal palette too short", map[string]string{"theme.toml": "[terminal]\npalette = [\"#000000\", \"#111111\"]\n"},
			"theme.toml", 2, []string{"terminal.palette", "2 colors", "want 16"}},
		{"terminal palette bad color", map[string]string{"theme.toml": "[terminal]\npalette = [" + strings.Repeat("\"#000000\", ", 15) + "\"#00000\"]\n"},
			"theme.toml", 2, []string{"terminal.palette", "#00000", "#rrggbb"}},
		{"terminal palette not a list", map[string]string{"theme.toml": "[terminal]\npalette = \"#000000\"\n"},
			"theme.toml", 2, []string{"terminal.palette", "16 colors"}},
		{"unknown terminal key", map[string]string{"theme.toml": "[terminal]\nfont = \"mono\"\n"},
			"theme.toml", 2, []string{"terminal.font", "unknown"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFiles(t, tt.files)
			_, err := LoadTheme(dir)
			if err == nil {
				t.Fatal("LoadTheme accepted the theme")
			}
			msg := err.Error()
			path := filepath.Join(dir, filepath.FromSlash(tt.file))
			want := append([]string{path}, tt.parts...)
			if tt.line > 0 {
				want = append(want, fmt.Sprintf("%s:%d:", path, tt.line))
			}
			for _, w := range want {
				if !strings.Contains(msg, w) {
					t.Errorf("error %q does not mention %q", msg, w)
				}
			}
		})
	}
}

func TestLoadTheme_MissingManifestIsNotExist(t *testing.T) {
	_, err := LoadTheme(filepath.Join(t.TempDir(), "nope"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
}

func TestLoadTheme_AcceptsWindowsLineEndingsAndATrailingNewline(t *testing.T) {
	crlf := func(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }
	typing := BuiltinTheme().poses[poseTyping]
	dir := writeFiles(t, map[string]string{
		"theme.toml":            crlf("name = \"crlf\"\n[palette]\neye = \"#0A0b0c\"\n"),
		"characters/typing.txt": crlf(gridText(typing)),
	})
	got, err := LoadTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Eye != (screen.RGB{R: 0x0a, G: 0x0b, B: 0x0c}) || !reflect.DeepEqual(got.poses[poseTyping], typing) {
		t.Errorf("eye = %+v, typing = %q", got.Eye, got.poses[poseTyping])
	}
}

func TestWriteTheme_WritesWhatLoadThemeReadsBack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mine")
	want := BuiltinTheme()
	want.Name, want.Author = "mine", "Ana \"pixels\" Pérez"
	if err := WriteTheme(dir, want); err != nil {
		t.Fatal(err)
	}
	for _, spec := range poseSpecs {
		if _, err := os.Stat(filepath.Join(dir, "characters", spec.name+".txt")); err != nil {
			t.Errorf("pose file: %v", err)
		}
	}
	got, err := LoadTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip differs:\n got %+v\nwant %+v", got, want)
	}
}

func TestLoadTheme_ReadsTheTerminalPalette(t *testing.T) {
	var colors []string
	var want screen.Palette
	want.Set = true
	for i := range want.C {
		want.C[i] = screen.RGB{R: uint8(i), G: 0x10, B: 0x20}
		colors = append(colors, fmt.Sprintf("%q", formatColor(want.C[i])))
	}
	dir := writeFiles(t, map[string]string{
		"theme.toml": "[terminal]\npalette = [" + strings.Join(colors, ", ") + "]\n",
	})
	got, err := LoadTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Terminal != want {
		t.Errorf("Terminal = %+v, want %+v", got.Terminal, want)
	}
	if BuiltinTheme().Terminal.Set {
		t.Error("the built-in theme keeps the emulator's own terminal colors")
	}
}

func TestWriteTheme_WritesTheTerminalPaletteBack(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mine")
	want, _ := Preset("gentle-ai")
	want.Name = "mine"
	if err := WriteTheme(dir, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTheme(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip differs:\n got %+v\nwant %+v", got, want)
	}
}

func TestWriteTheme_RefusesAFolderThatIsNotEmpty(t *testing.T) {
	dir := writeFiles(t, map[string]string{"notes.txt": "mine"})
	if err := WriteTheme(dir, BuiltinTheme()); err == nil || !strings.Contains(err.Error(), dir) {
		t.Fatalf("err = %v, want a refusal naming %s", err, dir)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "notes.txt")); string(b) != "mine" {
		t.Error("existing file was touched")
	}
	if _, err := os.Stat(filepath.Join(dir, "theme.toml")); err == nil {
		t.Error("theme.toml was written into a non-empty folder")
	}
}

// hasPixel reports whether any pixel of buf is c.
func hasPixel(buf *screen.Buffer, c screen.RGB) bool {
	for y := 0; y < buf.H*2; y++ {
		for x := 0; x < buf.W; x++ {
			if buf.PixelAt(x, y) == c {
				return true
			}
		}
	}
	return false
}

func TestSetTheme_ChangesTheColorsTheOfficeIsDrawnIn(t *testing.T) {
	carpet, hair, floor := rgb(0xff00ff), rgb(0x00ff01), rgb(0x0102ff)
	draw := func() *screen.Buffer {
		l, c, agents := posedOffice()
		buf := screen.New(l.W, l.H/2)
		Draw(buf, l, c, agents, 0, "", pet.Sleeping)
		return buf
	}
	before := draw()
	th := BuiltinTheme()
	th.Carpets, th.Hair, th.FloorA = []screen.RGB{carpet}, []screen.RGB{hair}, floor
	useTheme(t, th)
	after := draw()
	for name, c := range map[string]screen.RGB{"carpet": carpet, "hair": hair, "floor": floor} {
		if hasPixel(before, c) || !hasPixel(after, c) {
			t.Errorf("%s color %+v: before=%v after=%v, want only after", name, c, hasPixel(before, c), hasPixel(after, c))
		}
	}
}

func TestSetTheme_ChangesAgentColors(t *testing.T) {
	c := rgb(0x123456)
	th := BuiltinTheme()
	th.Agents = []screen.RGB{c}
	useTheme(t, th)
	if got := AgentColor("any"); got != c {
		t.Errorf("AgentColor = %+v, want %+v", got, c)
	}
	if got := LookOf("any").Shirt; got != c {
		t.Errorf("LookOf shirt = %+v, want %+v", got, c)
	}
}

func TestSetTheme_ChangesTheWindowAndTerminalColors(t *testing.T) {
	th := BuiltinTheme()
	th.UIBg, th.UIText, th.UIAccent = rgb(0x010203), rgb(0x040506), rgb(0x070809)
	th.Terminal.Set = true
	th.Terminal.C[1] = rgb(0x0a0b0c)
	useTheme(t, th)
	if got := UI(); got != (Chrome{Bg: th.UIBg, Text: th.UIText, Accent: th.UIAccent}) {
		t.Errorf("UI = %+v", got)
	}
	if got := TerminalPalette(); got != th.Terminal {
		t.Errorf("TerminalPalette = %+v", got)
	}
}

func TestLoadTheme_AMisspelledSectionNamesItsLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.toml")
	if err := os.WriteFile(path, []byte("name = \"x\"\n\n[pallete]\nhair = [\"#000000\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTheme(dir)
	if err == nil || !strings.Contains(err.Error(), path+":3: pallete") {
		t.Errorf("error = %v, want %s:3: pallete …", err, path)
	}
}
