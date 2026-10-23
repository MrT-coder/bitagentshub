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
	active     string
	palette    string // the theme whose colors the active one wears, "" for its own
	sel        int    // index into the list, 0 being the built-in theme
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
}

// settingsMsg is what a background Settings call reports back.
type settingsMsg struct {
	load       bool // the first read when the window opens
	configPath string
	themes     []string
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
		themes, err := s.Themes()
		out.themes, out.themesSet = themes, err == nil
		limits, lerr := s.LimitsInstalled()
		out.limits, out.limitsSet = limits, lerr == nil
		out.err = firstErr(err, lerr)
		return out
	}
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
		sw.themes = msg.themes
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
	case msg.selSet:
		sw.sel = sw.index(msg.selName)
	case msg.load:
		sw.sel = sw.index(sw.active)
	}
	sw.sel = min(sw.sel, len(sw.themes))
	sw.status = msg.status
	if msg.err != nil {
		sw.status = msg.err.Error()
	}
	return m
}

// index finds a theme in the list; "" and unknown names are the built-in.
func (sw *settingsWin) index(name string) int {
	for i, t := range sw.themes {
		if t == name {
			return i + 1
		}
	}
	return 0
}

// selected is the theme name under the cursor, "" for the built-in one.
func (sw *settingsWin) selected() string {
	if sw.sel == 0 || sw.sel > len(sw.themes) {
		return ""
	}
	return sw.themes[sw.sel-1]
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
		case sectionLanguage:
			sw.langSel = max(sw.langSel-1, 0)
		case sectionKeys:
			sw.keySel = max(sw.keySel-1, 0)
		}
	case "down", "j":
		switch sw.section {
		case sectionTheme:
			sw.sel = min(sw.sel+1, len(sw.themes))
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
	case "c":
		// The built-in theme sets no colors over another: on it, c goes
		// back to the theme's own colors.
		if sw.section == sectionTheme && !sw.busy {
			return m, m.applyPalette(sw.selected())
		}
	case "enter":
		if sw.busy {
			return m, nil
		}
		switch sw.section {
		case sectionTheme:
			return m, m.applyTheme(sw.selected())
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
		themes, err := s.Themes()
		if err != nil {
			return settingsMsg{err: err}
		}
		return settingsMsg{themes: themes, themesSet: true, selName: name, selSet: true,
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

// drawSettings paints the hub's settings tab: the themes, Claude's limits,
// the keys in use and where config.toml lives.
func (m Model) drawSettings(buf *screen.Buffer) {
	fw, fh := buf.W, buf.H
	w := min(fw-4, 90)
	inner := max(1, w-4)
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

	var top, rest []settingsLine
	var list []settingsLine
	add := func(to *[]settingsLine, text string, st screen.Style) {
		*to = append(*to, settingsLine{truncate(text, inner), st})
	}
	switch {
	case m.settings == nil:
		add(&top, i18n.T("Settings are not available here."), modalDim())
	case !sw.loaded:
		add(&top, i18n.T("Reading…"), modalDim())
	default:
		add(&top, i18n.T("Theme"), focus(sectionTheme))
		for i, name := range append([]string{""}, sw.themes...) {
			label := themeLabel(name)
			if name != "" && name == sw.palette {
				label += i18n.T(" · palette")
			}
			mark := "  "
			if name == sw.active {
				mark = "● "
			}
			st := modalText()
			if i == sw.sel && sw.section == sectionTheme {
				st.Reverse = true
			}
			add(&list, mark+label, st)
		}
		if sw.naming {
			add(&rest, i18n.T("new theme name: ")+sw.name+"█", modalBorder())
		}
		add(&rest, "", modalText())
		add(&rest, i18n.T("Language"), focus(sectionLanguage))
		for i, l := range i18n.Langs {
			mark := "  "
			if l == sw.language {
				mark = "● "
			}
			st := modalText()
			if i == sw.langSel && sw.section == sectionLanguage {
				st.Reverse = true
			}
			add(&rest, mark+l.Name(), st)
		}
		add(&rest, "", modalText())
		add(&rest, i18n.T("Claude limits"), focus(sectionLimits))
		state := i18n.T("unknown")
		if sw.limitsOK {
			state = i18n.T("off")
			if sw.limits {
				state = i18n.T("on")
			}
		}
		add(&rest, i18n.Tf("Claude limits: %s (BitAgentsHub as Claude Code's status line)", state), modalText())
		add(&rest, i18n.T("enter turns it on or off; settings.json is backed up before it changes."), modalDim())
	}
	add(&rest, "", modalText())
	add(&rest, i18n.T("Keys"), focus(sectionKeys))
	for i, r := range keyRows {
		st := modalText()
		if i == sw.keySel && sw.section == sectionKeys {
			st.Reverse = true
		}
		add(&rest, fmt.Sprintf("  %-15s %s", r.label(), m.keysText(r.action)), st)
	}
	// The outcome of the last action and where config.toml is always show:
	// on a short floor the keys and blank lines above them go first.
	var tail []settingsLine
	if sw.configPath != "" {
		add(&tail, "config "+leftTruncate(sw.configPath, inner-len("config ")), modalDim())
	}
	if sw.status != "" {
		add(&tail, sw.status, modalBorder())
	}

	// The theme list scrolls around the selection when the floor is short.
	maxH := max(3, fh-2)
	if room := max(0, maxH-2-len(top)-len(tail)-1); len(rest) > room {
		rest = rest[:room]
	}
	rest = append(rest, tail...)
	rows := max(1, min(len(list), maxH-2-len(top)-len(rest)))
	first := max(0, min(sw.sel-rows/2, len(list)-rows))
	if len(list) > 0 {
		list = list[first:min(len(list), first+rows)]
	}
	lines := append(append(top, list...), rest...)
	h := min(maxH, len(lines)+2)
	x, y := (fw-w)/2, (fh-h)/2
	footer := i18n.T(" ←→ tab · ↑↓ move · tab section · enter apply · c use its colors · n new theme · esc close ")
	switch {
	case sw.naming:
		footer = i18n.T(" type a folder name · enter create · esc cancel ")
	case sw.capturing:
		footer = i18n.T(" press the new key · esc cancel ")
	case sw.section == sectionLanguage:
		footer = i18n.T(" ←→ tab · ↑↓ move · tab section · enter apply · esc close ")
	case sw.section == sectionKeys:
		footer = i18n.T(" ←→ tab · ↑↓ move · tab section · enter change · backspace default · esc close ")
	}
	m.hubBox(buf, x, y, w, h, footer)
	for i, l := range lines {
		if i >= h-2 {
			return
		}
		buf.Text(x+2, y+1+i, l.text, l.style)
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
