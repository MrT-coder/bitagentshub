package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/screen"
)

// Settings is what the settings tab acts on; main wires the real one.
type Settings interface {
	ConfigPath() string
	Themes() ([]string, error)      // theme folder names that have a theme.toml, sorted
	Scenes() ([]string, error)      // the themes whose shapes are their own, not the built-in ones
	ActiveTheme() string            // "" is the built-in theme
	ApplyTheme(name string) error   // load, make active and save in config.toml; "" = built-in
	NewTheme(name string) error     // write the active theme as themes/<name>
	ActivePalette() string          // the theme whose colors the active one wears; "" for its own
	ApplyPalette(name string) error // wear name's colors and save [theme] palette; "" = the theme's own
	LimitsInstalled() (bool, error) // Claude's status line is BitAgentsHub's
	SetLimits(on bool) error        // install or uninstall it
	// SetKeys binds an action ("prefix" for the prefix) to keys in
	// config.toml, no keys putting back its defaults, and returns the
	// keymap the file now gives.
	SetKeys(action string, keys []string) (config.Keymap, error)
	ActiveLanguage() string          // "en" or "es", the language in use
	ApplyLanguage(lang string) error // save [ui] language in config.toml
	SetPetName(name string) error    // save [pet] name in config.toml
}

// Sections of the settings tab that take keys; tab goes round them.
const (
	sectionTheme = iota
	sectionColors
	sectionLanguage
	sectionLimits
	sectionKeys
	sectionCount
)

// keyRow is a row of the Keys section: an action and its keys.
type keyRow struct{ action string }

// keyRows are the rows of the Keys section, the prefix first.
var keyRows = []keyRow{
	{"prefix"},
	{config.CloseWindow},
	{config.ToggleMaximize},
	{config.NewTerminal},
	{config.CloseTerminal},
	{config.FocusPaneLeft},
	{config.FocusPaneRight},
	{config.FocusStation},
	{config.OpenBoard},
	{config.NewAgent},
	{config.OpenHub},
}

// label names the row's action in the current language.
func (r keyRow) label() string {
	switch r.action {
	case config.CloseWindow:
		return i18n.T("close window")
	case config.ToggleMaximize:
		return i18n.T("maximize")
	case config.NewTerminal:
		return i18n.T("new terminal")
	case config.CloseTerminal:
		return i18n.T("close terminal")
	case config.FocusPaneLeft:
		return i18n.T("pane left")
	case config.FocusPaneRight:
		return i18n.T("pane right")
	case config.FocusStation:
		return i18n.T("open station")
	case config.OpenBoard:
		return i18n.T("board")
	case config.NewAgent:
		return i18n.T("new agent")
	case config.OpenHub:
		return i18n.T("hub")
	}
	return i18n.T("prefix")
}

// themeLabel names a theme in the list; the office's own themes are marked.
func themeLabel(name string) string {
	switch _, own := office.Preset(name); {
	case name == "":
		return i18n.T("default (built-in)") // the theme compiled into the office, always first
	case own:
		return i18n.Tf("%s (built-in)", name)
	}
	return name
}

// settingsWin is the hub's open settings tab. Every Settings call may touch
// disk, so it runs in the background and its answer lands here.
type settingsWin struct {
	loaded     bool
	configPath string
	themes     []string // theme folders; the list shows the built-in one before them
	scenes     []string // the themes with shapes of their own
	active     string
	palette    string // the theme whose colors the active one wears, "" for its own
	sel        int    // index into the Theme list, 0 being the built-in theme
	palSel     int    // index into the Colors list, 0 being the theme's own colors
	section    int
	limits     bool
	limitsOK   bool // limits was read; until then it is unknown
	naming     bool // the new-theme name input is open
	name       string
	busy       bool // a call is running; enter waits for it
	status     string
	language   i18n.Lang // the language in use
	langSel    int       // index into i18n.Langs under the cursor
	keySel     int       // the row of keyRows under the cursor
	capturing  bool      // waiting for the new key of keyRows[keySel]
	capPrefix  bool      // the prefix was pressed; the next key goes after it
	scroll     int       // the first body row shown
}

// settingsMsg is what a background Settings call reports back.
type settingsMsg struct {
	load       bool // the first read when the window opens
	configPath string
	themes     []string
	scenes     []string
	themesSet  bool
	active     string
	activeSet  bool
	palette    string
	paletteSet bool
	selName    string // select this theme once the list is in
	selSet     bool
	limits     bool
	limitsSet  bool
	keys       config.Keymap
	keysSet    bool
	language   string
	langSet    bool
	status     string
	err        error
}

// WithSettings sets what the settings tab reads and changes.
func (m Model) WithSettings(s Settings) Model {
	m.settings = s
	return m
}

// readSettings reads the themes, the active one and the limits state.
func (m Model) readSettings() tea.Cmd {
	s := m.settings
	if s == nil {
		return nil
	}
	return func() tea.Msg {
		out := settingsMsg{load: true, configPath: s.ConfigPath(), active: s.ActiveTheme(), activeSet: true,
			palette: s.ActivePalette(), paletteSet: true, language: s.ActiveLanguage(), langSet: true}
		var err error
		out.themes, out.scenes, err = readThemes(s)
		out.themesSet = err == nil
		limits, lerr := s.LimitsInstalled()
		out.limits, out.limitsSet = limits, lerr == nil
		out.err = firstErr(err, lerr)
		return out
	}
}

// readThemes reads every theme and the ones with shapes of their own.
func readThemes(s Settings) (themes, scenes []string, err error) {
	if themes, err = s.Themes(); err != nil {
		return nil, nil, err
	}
	if scenes, err = s.Scenes(); err != nil {
		return nil, nil, err
	}
	return themes, scenes, nil
}

// firstErr keeps the first error; the window has one line for it.
func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// settingsDone takes a background answer into the open window; one that
// comes back after the window closed only changes what the next open reads.
func (m Model) settingsDone(msg settingsMsg) Model {
	if msg.keysSet {
		m.keys = msg.keys // saved in config.toml, so it applies at once
	}
	sw := m.setWin
	if sw == nil {
		return m
	}
	sw.busy = false
	if msg.load {
		sw.loaded, sw.configPath = true, msg.configPath
	}
	if msg.themesSet {
		sw.themes, sw.scenes = msg.themes, msg.scenes
	}
	if msg.activeSet {
		sw.active = msg.active
	}
	if msg.paletteSet {
		sw.palette = msg.palette
	}
	if msg.limitsSet {
		sw.limits, sw.limitsOK = msg.limits, true
	}
	if msg.langSet {
		sw.language = i18n.Lang(msg.language)
		if msg.load {
			sw.langSel = max(0, slices.Index(i18n.Langs, sw.language))
		}
	}
	switch {
	case msg.selSet && slices.Contains(sw.sceneList(), msg.selName):
		sw.sel = index(sw.sceneList(), msg.selName)
	case msg.selSet:
		// A theme with the built-in shapes only lends its colors.
		sw.palSel, sw.section = index(sw.colorList(), msg.selName), sectionColors
	case msg.load:
		sw.sel = index(sw.sceneList(), sw.active)
		sw.palSel = index(sw.colorList(), sw.palette)
	}
	sw.sel = min(sw.sel, len(sw.sceneList())-1)
	sw.palSel = min(sw.palSel, len(sw.colorList())-1)
	sw.status = msg.status
	if msg.err != nil {
		sw.status = msg.err.Error()
	}
	return m.followSettings()
}

// sceneList is the Theme column: the built-in theme, then the themes with
// shapes of their own. The active theme shows even with the built-in shapes,
// so what is in use is always marked.
func (sw *settingsWin) sceneList() []string {
	out := []string{""}
	for _, t := range sw.themes {
		if t == sw.active || slices.Contains(sw.scenes, t) {
			out = append(out, t)
		}
	}
	return out
}

// colorList is the Colors column: the theme's own colors, then the themes
// that only recolor; a scene shows only when it is the palette in use.
func (sw *settingsWin) colorList() []string {
	out := []string{""}
	for _, t := range sw.themes {
		if t == sw.palette || !slices.Contains(sw.scenes, t) {
			out = append(out, t)
		}
	}
	return out
}

// index finds a theme in a list; unknown names are its first row.
func index(list []string, name string) int { return max(0, slices.Index(list, name)) }

// selected is the theme under the Theme cursor, "" for the built-in one.
func (sw *settingsWin) selected() string { return at(sw.sceneList(), sw.sel) }

// selectedPalette is the theme under the Colors cursor, "" for the theme's
// own colors.
func (sw *settingsWin) selectedPalette() string { return at(sw.colorList(), sw.palSel) }

func at(list []string, i int) string {
	if i < 0 || i >= len(list) {
		return ""
	}
	return list[i]
}

// settingsKey handles the keys the hub passes to its settings tab: every
// key while a key or a theme name is typed, otherwise those the hub itself
// does not use.
func (m Model) settingsKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.setWin.capturing {
		return m.captureKey(msg)
	}
	k := msg.String()
	sw := m.setWin
	if sw.naming {
		return m.nameKey(msg)
	}
	if m.settings == nil || !sw.loaded {
		return m, nil
	}
	switch k {
	case "tab":
		sw.section = (sw.section + 1) % sectionCount
	case "shift+tab":
		sw.section = (sw.section + sectionCount - 1) % sectionCount
	case "up", "k":
		switch sw.section {
		case sectionTheme:
			sw.sel = max(sw.sel-1, 0)
		case sectionColors:
			sw.palSel = max(sw.palSel-1, 0)
		case sectionLanguage:
			sw.langSel = max(sw.langSel-1, 0)
		case sectionKeys:
			sw.keySel = max(sw.keySel-1, 0)
		}
	case "down", "j":
		switch sw.section {
		case sectionTheme:
			sw.sel = min(sw.sel+1, len(sw.sceneList())-1)
		case sectionColors:
			sw.palSel = min(sw.palSel+1, len(sw.colorList())-1)
		case sectionLanguage:
			sw.langSel = min(sw.langSel+1, len(i18n.Langs)-1)
		case sectionKeys:
			sw.keySel = min(sw.keySel+1, len(keyRows)-1)
		}
	case "backspace", "delete":
		if sw.section == sectionKeys && !sw.busy {
			row := keyRows[sw.keySel]
			if row.action == "prefix" {
				return m, m.setKeys(row, []string{config.Default().Prefix})
			}
			// config.toml would drop a default key another action holds now
			// and leave this one short of keys; refuse like a clash.
			if k, other := m.defaultTaken(row.action); other != "" {
				sw.status = i18n.Tf("Its default %s is bound to %s now; change that one first.", k, other)
				return m, nil
			}
			return m, m.setKeys(row, nil)
		}
	case "n":
		if sw.section == sectionTheme {
			sw.naming, sw.name = true, ""
		}
	case "enter":
		if sw.busy {
			return m, nil
		}
		switch sw.section {
		case sectionTheme:
			return m, m.applyTheme(sw.selected())
		case sectionColors:
			return m, m.applyPalette(sw.selectedPalette())
		case sectionLanguage:
			return m, m.applyLanguage(i18n.Langs[sw.langSel])
		case sectionKeys:
			if keyRows[sw.keySel].action == config.FocusStation {
				sw.status = i18n.T("open station takes a range such as prefix+1..9; set it in config.toml.")
				return m, nil
			}
			sw.capturing, sw.capPrefix = true, false
			sw.status = i18n.Tf("Press the new key for %s (%s first for a key after it) · esc cancels", keyRows[sw.keySel].label(), m.keys.Prefix)
			return m, nil
		}
		if sw.limitsOK {
			return m, m.setLimits(!sw.limits)
		}
	}
	return m, nil
}

// nameKey edits the new theme's name: enter writes it, esc gives up on the
// name and leaves the window open.
func (m Model) nameKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	sw := m.setWin
	switch msg.String() {
	case "esc":
		sw.naming, sw.name = false, ""
	case "backspace":
		if r := []rune(sw.name); len(r) > 0 {
			sw.name = string(r[:len(r)-1])
		}
	case "enter":
		name := strings.TrimSpace(sw.name)
		if name == "" || sw.busy {
			return m, nil
		}
		sw.naming, sw.name = false, ""
		return m, m.newTheme(name)
	default:
		if text, ok := keyText(msg); ok {
			sw.name += text
		}
	}
	return m, nil
}

func (m Model) applyTheme(name string) tea.Cmd {
	s := m.settings
	m.setWin.busy, m.setWin.status = true, i18n.T("Applying…")
	return func() tea.Msg {
		if err := s.ApplyTheme(name); err != nil {
			return settingsMsg{err: err}
		}
		return settingsMsg{active: name, activeSet: true, status: i18n.Tf("Theme %s applied and saved.", themeLabel(name))}
	}
}

// applyPalette dresses the active theme in name's colors and saves it;
// "" goes back to the theme's own colors.
func (m Model) applyPalette(name string) tea.Cmd {
	s := m.settings
	m.setWin.busy, m.setWin.status = true, i18n.T("Applying…")
	return func() tea.Msg {
		if err := s.ApplyPalette(name); err != nil {
			return settingsMsg{err: err}
		}
		status := i18n.Tf("Wearing the colors of %s; saved.", themeLabel(name))
		if name == "" {
			status = i18n.T("Wearing the theme's own colors; saved.")
		}
		return settingsMsg{palette: name, paletteSet: true, status: status}
	}
}

// applyLanguage saves the language in config.toml and, once saved, speaks
// it everywhere at once.
func (m Model) applyLanguage(lang i18n.Lang) tea.Cmd {
	s := m.settings
	m.setWin.busy, m.setWin.status = true, i18n.T("Applying…")
	return func() tea.Msg {
		if err := s.ApplyLanguage(string(lang)); err != nil {
			return settingsMsg{err: err}
		}
		i18n.Set(lang)
		return settingsMsg{language: string(lang), langSet: true, status: i18n.Tf("Language %s applied and saved.", lang.Name())}
	}
}

// newTheme writes the active theme under a new name, then reads the list
// again so the new one shows selected; it is not applied.
func (m Model) newTheme(name string) tea.Cmd {
	s := m.settings
	m.setWin.busy, m.setWin.status = true, i18n.Tf("Creating %s…", name)
	return func() tea.Msg {
		if err := s.NewTheme(name); err != nil {
			return settingsMsg{err: err}
		}
		themes, scenes, err := readThemes(s)
		if err != nil {
			return settingsMsg{err: err}
		}
		return settingsMsg{themes: themes, scenes: scenes, themesSet: true, selName: name, selSet: true,
			status: i18n.Tf("Created themes/%s; enter applies it.", name)}
	}
}

func (m Model) setLimits(on bool) tea.Cmd {
	s := m.settings
	m.setWin.busy, m.setWin.status = true, i18n.T("Working…")
	return func() tea.Msg {
		if err := s.SetLimits(on); err != nil {
			return settingsMsg{err: err}
		}
		status := i18n.T("Claude's status line removed.")
		if on {
			status = i18n.T("Claude's status line installed.")
		}
		return settingsMsg{limits: on, limitsSet: true, status: status}
	}
}

// captureKey takes the new key of the selected action. Every key counts,
// even those that otherwise close the window; esc cancels unless it comes
// right after the prefix. The prefix itself starts a key after it, except
// for the prefix row, which takes one key alone.
func (m Model) captureKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	sw := m.setWin
	row := keyRows[sw.keySel]
	k := config.Spell(msg.String())
	switch {
	case k == "esc" && !sw.capPrefix:
		sw.capturing, sw.status = false, ""
		return m, nil
	case row.action != "prefix" && !sw.capPrefix && k == m.keys.Prefix:
		sw.capPrefix = true
		sw.status = i18n.Tf("%s … now the key for %s", m.keys.Prefix, row.label())
		return m, nil
	}
	after := sw.capPrefix
	sw.capturing, sw.capPrefix = false, false
	// config.toml lets a binding take a default key from another action
	// silently; here the user sees the clash and decides.
	if other, _, taken := m.keys.Action(k, after); taken && other != row.action {
		shown := k
		if after {
			shown = m.keys.Prefix + " " + k
		}
		sw.status = i18n.Tf("%s is already bound to %s; change that one first.", shown, other)
		return m, nil
	}
	if after {
		k = "prefix+" + k
	}
	return m, m.setKeys(row, []string{k})
}

// defaultTaken names a default key of action that another action holds in
// the keymap in use, spelled for the window, and that action.
func (m Model) defaultTaken(action string) (key, other string) {
	for _, spec := range config.Default().Keys(action) {
		k, after := strings.CutPrefix(spec, "prefix+")
		if strings.Contains(k, "..") || (!after && k == m.keys.Prefix) {
			continue // ranges belong to open station; the prefix never binds alone
		}
		if a, _, taken := m.keys.Action(k, after); taken && a != action {
			if after {
				k = m.keys.Prefix + " " + k
			}
			return k, a
		}
	}
	return "", ""
}

// setKeys saves an action's keys in the background; none puts back its
// defaults.
func (m Model) setKeys(row keyRow, keys []string) tea.Cmd {
	s := m.settings
	m.setWin.busy, m.setWin.status = true, i18n.T("Saving…")
	return func() tea.Msg {
		km, err := s.SetKeys(row.action, keys)
		if err != nil {
			return settingsMsg{err: err}
		}
		status := i18n.Tf("%s saved in config.toml.", row.label())
		if len(keys) == 0 {
			status = i18n.Tf("%s is back to its default keys.", row.label())
		}
		return settingsMsg{keys: km, keysSet: true, status: status}
	}
}

// keysText spells an action's keys for the Keys section, with the prefix
// written out ("ctrl+a d, f10").
func (m Model) keysText(action string) string {
	if action == "prefix" {
		return m.keys.Prefix
	}
	var out []string
	for _, k := range m.keys.Keys(action) {
		k = strings.Replace(k, "..", "-", 1)
		if rest, ok := strings.CutPrefix(k, "prefix+"); ok {
			k = m.keys.Prefix + " " + rest
		}
		out = append(out, k)
	}
	if len(out) == 0 {
		return i18n.T("(none)")
	}
	return strings.Join(out, ", ")
}

// settingsLine is one row of a hub tab.
type settingsLine struct {
	text  string
	style screen.Style
}

// settingsRow is a row of the settings tab; right is drawn from the middle
// of the box on, for the Colors column beside the themes. hit and rightHit
// are what a click on either side of the row picks.
type settingsRow struct {
	settingsLine
	right         settingsLine
	hit, rightHit pick
}

// pick is what a click on a settings row picks: a section and its row, or
// only the section when row is -1. The zero pick picks nothing.
type pick struct {
	ok           bool
	section, row int
}

// drawSettings paints the hub's settings tab: the themes, their colors, the
// language, Claude's limits, the keys in use and where config.toml lives.
func (m Model) drawSettings(buf *screen.Buffer) {
	x, y, w, h, p := m.settingsLayout(buf.W, buf.H)
	m.hubBox(buf, x, y, w, h, p.footer)
	for i, l := range p.shown() {
		if i >= h-2 {
			return
		}
		buf.Text(x+2, y+1+i, l.text, l.style)
		if l.right.text != "" {
			buf.Text(x+2+p.col, y+1+i, l.right.text, l.right.style)
		}
	}
}

// settingsPage is the settings tab laid out on a floor: the heading on top,
// the body that scrolls under it, the outcome and config.toml at the
// bottom, where the Colors column starts and the footer.
type settingsPage struct {
	col             int
	top, body, tail []settingsRow
	first, room     int // the first body row shown and how many body rows fit
	cursor          int // the body row under the cursor, -1 for none
	footer          string
}

// shown is the rows on screen, top to bottom.
func (p settingsPage) shown() []settingsRow {
	body := p.body[p.first:min(len(p.body), p.first+p.room)]
	return append(append(slices.Clone(p.top), body...), p.tail...)
}

// settingsLayout lays the settings tab out on a floor of fw by fh.
func (m Model) settingsLayout(fw, fh int) (x, y, w, h int, p settingsPage) {
	w = min(fw-4, 90)
	inner := max(1, w-4)
	p.col = inner / 2 // where the Colors column starts
	p.cursor = -1
	sw := m.setWin
	heading := modalText()
	heading.Bold = true
	focus := func(section int) screen.Style {
		if sw.section == section {
			st := modalBorder()
			st.Bold = true
			return st
		}
		return heading
	}
	add := func(to *[]settingsRow, text string, st screen.Style, hit pick) {
		*to = append(*to, settingsRow{settingsLine: settingsLine{truncate(text, inner), st}, hit: hit})
	}
	// here makes the next body row the cursor's when on is true.
	here := func(on bool) {
		if on {
			p.cursor = len(p.body)
		}
	}
	section := func(s int) pick { return pick{true, s, -1} }
	switch {
	case m.settings == nil:
		add(&p.top, i18n.T("Settings are not available here."), modalDim(), pick{})
	case !sw.loaded:
		add(&p.top, i18n.T("Reading…"), modalDim(), pick{})
	default:
		// Theme picks the shapes, Colors the theme whose colors they wear,
		// side by side over the same names. The built-in theme is the base
		// under every theme and has no colors to lend: in Colors its row
		// stands for the theme's own colors.
		row := func(text string, st screen.Style, right string, rst screen.Style) settingsRow {
			return settingsRow{settingsLine: settingsLine{truncate(text, p.col-1), st}, right: settingsLine{truncate(right, inner-p.col), rst}}
		}
		head := row(i18n.T("Theme"), focus(sectionTheme), i18n.T("Colors"), focus(sectionColors))
		head.hit, head.rightHit = section(sectionTheme), section(sectionColors)
		p.top = append(p.top, head)
		scenes, colors := sw.sceneList(), sw.colorList()
		for i := range max(len(scenes), len(colors)) {
			var left, right string
			st, pst := modalText(), modalText()
			var hit, rightHit pick
			if i < len(scenes) {
				mark := "  "
				if scenes[i] == sw.active {
					mark = "● "
				}
				left = mark + themeLabel(scenes[i])
				st.Reverse = i == sw.sel && sw.section == sectionTheme
				hit = pick{true, sectionTheme, i}
			}
			if i < len(colors) {
				mark, label := "  ", themeLabel(colors[i])
				if colors[i] == sw.palette {
					mark = "● "
				}
				if colors[i] == "" {
					label = i18n.T("the theme's own colors")
				}
				right = mark + label
				pst.Reverse = i == sw.palSel && sw.section == sectionColors
				rightHit = pick{true, sectionColors, i}
			}
			here(st.Reverse || pst.Reverse)
			r := row(left, st, right, pst)
			r.hit, r.rightHit = hit, rightHit
			p.body = append(p.body, r)
		}
		if sw.naming {
			here(true)
			add(&p.body, i18n.T("new theme name: ")+sw.name+"█", modalBorder(), pick{})
		}
		add(&p.body, "", modalText(), pick{})
		add(&p.body, i18n.T("Language"), focus(sectionLanguage), section(sectionLanguage))
		for i, l := range i18n.Langs {
			mark := "  "
			if l == sw.language {
				mark = "● "
			}
			st := modalText()
			if i == sw.langSel && sw.section == sectionLanguage {
				st.Reverse = true
			}
			here(st.Reverse)
			add(&p.body, mark+l.Name(), st, pick{true, sectionLanguage, i})
		}
		add(&p.body, "", modalText(), pick{})
		add(&p.body, i18n.T("Claude limits"), focus(sectionLimits), section(sectionLimits))
		state := i18n.T("unknown")
		if sw.limitsOK {
			state = i18n.T("off")
			if sw.limits {
				state = i18n.T("on")
			}
		}
		here(sw.section == sectionLimits)
		add(&p.body, i18n.Tf("Claude limits: %s (BitAgentsHub as Claude Code's status line)", state), modalText(), pick{true, sectionLimits, 0})
		add(&p.body, i18n.T("enter turns it on or off; settings.json is backed up before it changes."), modalDim(), pick{true, sectionLimits, 0})
	}
	add(&p.body, "", modalText(), pick{})
	add(&p.body, i18n.T("Keys"), focus(sectionKeys), section(sectionKeys))
	for i, r := range keyRows {
		st := modalText()
		if i == sw.keySel && sw.section == sectionKeys {
			st.Reverse = true
		}
		here(st.Reverse)
		add(&p.body, fmt.Sprintf("  %-15s %s", r.label(), m.keysText(r.action)), st, pick{true, sectionKeys, i})
	}
	// The outcome of the last action and where config.toml is always show;
	// the body scrolls between them and the heading.
	if sw.configPath != "" {
		add(&p.tail, "config "+leftTruncate(sw.configPath, inner-len("config ")), modalDim(), pick{})
	}
	if sw.status != "" {
		add(&p.tail, sw.status, modalBorder(), pick{})
	}

	maxH := max(3, fh-2)
	p.room = max(1, maxH-2-len(p.top)-len(p.tail))
	p.first = clampScroll(sw.scroll, len(p.body), p.room)
	h = min(maxH, len(p.top)+min(len(p.body), p.room)+len(p.tail)+2)
	x, y = (fw-w)/2, (fh-h)/2
	p.footer = i18n.T(" ←→ tab · ↑↓ move · tab section · enter apply · n new theme · esc close ")
	switch {
	case sw.naming:
		p.footer = i18n.T(" type a folder name · enter create · esc cancel ")
	case sw.capturing:
		p.footer = i18n.T(" press the new key · esc cancel ")
	case sw.section == sectionLanguage, sw.section == sectionColors:
		p.footer = i18n.T(" ←→ tab · ↑↓ move · tab section · enter apply · esc close ")
	case sw.section == sectionKeys:
		p.footer = i18n.T(" ←→ tab · ↑↓ move · tab section · enter change · backspace default · esc close ")
	}
	return x, y, w, h, p
}

// followSettings scrolls the settings body just enough to show the row
// under the cursor.
func (m Model) followSettings() Model {
	sw := m.setWin
	if sw == nil || !m.hubOpen || m.hubTab != tabSettings {
		return m
	}
	_, _, _, _, p := m.settingsLayout(m.floorW(), m.floorH())
	sw.scroll = p.first
	switch {
	case p.cursor < 0:
	case p.cursor < p.first:
		sw.scroll = p.cursor
	case p.cursor >= p.first+p.room:
		sw.scroll = p.cursor - p.room + 1
	}
	return m
}

// scrollSettings scrolls the settings body by delta rows.
func (m Model) scrollSettings(delta int) Model {
	_, _, _, _, p := m.settingsLayout(m.floorW(), m.floorH())
	m.setWin.scroll = clampScroll(p.first+delta, len(p.body), p.room)
	return m
}

// settingsClick takes a click at column cx of row cy inside the settings
// window's border: it moves the cursor to the row clicked, and a click on
// the row already under the cursor acts like enter.
func (m Model) settingsClick(cx, cy int) (Model, tea.Cmd) {
	sw := m.setWin
	if m.settings == nil || !sw.loaded {
		return m, nil
	}
	_, _, _, h, p := m.settingsLayout(m.floorW(), m.floorH())
	rows := p.shown()
	if cy < 0 || cy >= len(rows) || cy >= h-2 {
		return m, nil
	}
	pk := rows[cy].hit
	if cx >= p.col+2 && rows[cy].rightHit.ok {
		pk = rows[cy].rightHit
	}
	switch {
	case !pk.ok:
		return m, nil
	case pk.row >= 0 && pk.section == sw.section && pk.row == sw.cursor():
		m, cmd := m.settingsKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		return m.followSettings(), cmd
	}
	sw.section = pk.section
	if pk.row >= 0 {
		sw.setCursor(pk.row)
	}
	return m.followSettings(), nil
}

// cursor is the row under the cursor in the section it is on.
func (sw *settingsWin) cursor() int {
	switch sw.section {
	case sectionTheme:
		return sw.sel
	case sectionColors:
		return sw.palSel
	case sectionLanguage:
		return sw.langSel
	case sectionKeys:
		return sw.keySel
	}
	return 0
}

// setCursor puts the cursor on a row of the section it is on.
func (sw *settingsWin) setCursor(row int) {
	switch sw.section {
	case sectionTheme:
		sw.sel = row
	case sectionColors:
		sw.palSel = row
	case sectionLanguage:
		sw.langSel = row
	case sectionKeys:
		sw.keySel = row
	}
}

// leftTruncate keeps the end of a path, the part that names the file.
func leftTruncate(s string, n int) string {
	r := []rune(s)
	if n <= 1 || len(r) <= n {
		return s
	}
	return "…" + string(r[len(r)-n+1:])
}

// wrapHints breaks a " · " separated hint line into lines of at most n
// cells, keeping each hint whole.
func wrapHints(s string, n int) []string {
	var out []string
	line := ""
	for _, part := range strings.Split(s, " · ") {
		switch {
		case line == "":
			line = part
		case len([]rune(line))+3+len([]rune(part)) <= n:
			line += " · " + part
		default:
			out = append(out, line)
			line = part
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}
