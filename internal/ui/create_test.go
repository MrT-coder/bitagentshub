package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/herdr"
	"github.com/MrT-coder/bitagentshub/internal/projects"
)

func withProjects(m Model, list ...projects.Project) Model {
	m.discover = func([]claude.ProjectRef) []projects.Project { return list }
	return m
}

var (
	portal = projects.Project{Path: osPath(`C:\w\acme-company\shopping-portal`), Name: "shopping-portal", Group: "acme-company", Recent: true}
	deck   = projects.Project{Path: osPath(`C:\w\hobbylabs\minigames`), Name: "minigames", Group: "hobbylabs"}
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNOpensTheProjectPicker(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(withProjects(m, portal, deck), key("n"))
	v := plain(m.render())
	for _, want := range []string{"type to filter", "shopping-portal", "acme-company"} {
		if !strings.Contains(v, want) {
			t.Errorf("picker missing %q:\n%s", want, v)
		}
	}
}

func TestTheEmptyPickerShowsOnlyRecentFoldersAndTypingFindsTheRest(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(withProjects(m, portal, deck), key("n"))
	if v := plain(m.render()); strings.Contains(v, "minigames") {
		t.Errorf("a folder no agent worked in should wait for a filter:\n%s", v)
	}
	m = press(m, tea.PasteMsg{Content: "mini"})
	if v := plain(m.render()); !strings.Contains(v, "minigames") {
		t.Errorf("typing should find minigames:\n%s", v)
	}
}

func TestWithoutRecentFoldersThePickerShowsEveryFolder(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(withProjects(m, deck), key("n"))
	if v := plain(m.render()); !strings.Contains(v, "minigames") {
		t.Errorf("picker should list minigames:\n%s", v)
	}
}

func TestThePickerSaysANewFolderCanBeTyped(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(withProjects(m, portal), key("n"))
	if v := plain(m.render()); !strings.Contains(v, "or a new folder") {
		t.Errorf("picker should hint at creating a folder:\n%s", v)
	}
}

func TestATypedNameCreatesAFolderNextToTheMostRecentProject(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	group := filepath.Join(t.TempDir(), "acme")
	recent := projects.Project{Path: filepath.Join(group, "api"), Name: "api", Group: "acme", Recent: true}
	m = press(withProjects(m, recent, deck), key("n"))
	m = press(m, tea.PasteMsg{Content: "brand-new"})
	if v := view(m); !strings.Contains(v, "create brand-new in") {
		t.Fatalf("picker should offer to create brand-new:\n%s", v)
	}
	m = press(m, key("enter"))
	dir := filepath.Join(group, "brand-new")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	if len(rec.started) != 1 || rec.started[0].dir != dir || rec.started[0].argv[0] != "claude" {
		t.Errorf("started = %+v", rec.started)
	}
}

func TestFoldersWithTheSameNameAndGroupShowWhereTheyAre(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	c := projects.Project{Path: osPath(`C:\w\hobbylabs\web`), Name: "web", Group: "hobbylabs", Recent: true}
	e := projects.Project{Path: osPath(`E:\old\hobbylabs\web`), Name: "web", Group: "hobbylabs", Recent: true}
	m = press(withProjects(m, c, e), key("n"))
	v := plain(m.render())
	for _, want := range []string{filepath.Dir(c.Path), filepath.Dir(e.Path)} {
		if !strings.Contains(v, want) {
			t.Errorf("picker should show %q to tell the two apart:\n%s", want, v)
		}
	}
}

func TestATypedNameOfAListedFolderIsNotOfferedForCreation(t *testing.T) {
	m, _, _ := twoAgentOffice(t)
	m = press(withProjects(m, portal, deck), key("n"))
	m = press(m, tea.PasteMsg{Content: "MiniGames"})
	if v := view(m); strings.Contains(v, "create ") {
		t.Errorf("minigames is listed, so it should not be offered for creation:\n%s", v)
	}
}

func TestAClaudeAgentGetsTheBoardFolderAndHowToCheckOffTasks(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	dir := t.TempDir()
	m = m.WithBoard(config.Board{Dir: dir, Editor: "nvim"})
	m = press(withProjects(m, deck), key("n"))
	m = press(m, key("enter"))
	argv := rec.last().argv
	if len(argv) != 7 || argv[0] != "claude" || argv[1] != "--session-id" || argv[3] != "--add-dir" || argv[4] != dir || argv[5] != "--append-system-prompt" {
		t.Fatalf("argv = %q, want claude --session-id <id> --add-dir <board> --append-system-prompt <how>", argv)
	}
	prompt := argv[6]
	for _, want := range []string{filepath.Join(dir, "minigames"), "YYYY-MM-DD.md", "- [ ]", "- [x]"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the prompt should mention %q:\n%s", want, prompt)
		}
	}
}

func TestWithoutABoardAgentsStartAsBefore(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(withProjects(m, deck), key("n"))
	m = press(m, key("enter"))
	if argv := rec.last().argv; len(argv) != 3 {
		t.Errorf("argv = %q, want only claude --session-id <id>", argv)
	}
}

func TestANewAgentRunsLocallyWithItsOwnSessionID(t *testing.T) {
	m, f, rec := twoAgentOffice(t)
	m = press(withProjects(m, portal, deck), key("n"))
	for _, r := range "games" {
		m = press(m, key(string(r)))
	}
	if v := plain(m.render()); strings.Contains(v, "shopping-portal ") {
		t.Errorf("filter should hide shopping-portal:\n%s", v)
	}
	m = press(m, key("enter"))

	if len(f.started) != 0 {
		t.Errorf("herdr was used for a local agent: %v", f.started)
	}
	if len(rec.started) != 1 {
		t.Fatalf("terminals = %+v", rec.started)
	}
	st := rec.started[0]
	if st.dir != deck.Path || len(st.argv) != 3 || st.argv[0] != "claude" || st.argv[1] != "--session-id" || st.argv[2] != st.id || !uuidRE.MatchString(st.id) {
		t.Errorf("started %+v, want claude --session-id <uuid> in the project", st)
	}
	v := plain(m.render())
	for _, want := range []string{"TERM claude --session-id", "minigames"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q (chat should open on the new agent):\n%s", want, v)
		}
	}
}

func TestWithHerdrHostANewAgentStartsInHerdr(t *testing.T) {
	m, f, rec := twoAgentOffice(t)
	m = m.WithHost("herdr")
	m = press(withProjects(m, deck), key("n"))
	m = press(m, key("enter"))
	if len(f.started) != 1 || f.started[0] != deck.Path+": claude" || len(rec.started) != 0 {
		t.Fatalf("herdr started = %v, local = %d", f.started, len(rec.started))
	}
	if v := plain(m.render()); !strings.Contains(v, "Started claude in minigames") {
		t.Errorf("view should confirm the start:\n%s", v)
	}
}

func TestATypedFolderPathCanBeOpenedEvenIfNotListed(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	dir := t.TempDir()
	m = press(withProjects(m, portal), key("n"))
	m = press(m, tea.PasteMsg{Content: dir})
	m = press(m, key("enter"))
	if len(rec.started) != 1 || rec.started[0].dir != dir {
		t.Errorf("started = %+v, want the typed folder", rec.started)
	}
}

func TestEscClosesThePickerWithoutStartingAnything(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(withProjects(m, portal), key("n"))
	m = press(m, key("q")) // typed into the filter, not quitting
	m = press(m, key("esc"))
	if v := plain(m.render()); strings.Contains(v, "type to filter") {
		t.Errorf("picker still open:\n%s", v)
	}
	if len(rec.started) != 0 {
		t.Errorf("started = %v", rec.started)
	}
}

func TestAFreshHerdrAgentWalksInBeforeItHasATranscript(t *testing.T) {
	m, f, _ := twoAgentOffice(t)
	f.panes = append(f.panes, herdr.Pane{PaneID: "w9:p1", Agent: "claude", Status: "idle", Cwd: `C:\w\hobbylabs\minigames`})
	m = update(m, herdrMsg{panes: f.panes})
	if v := plain(m.render()); !strings.Contains(v, "minigames") || !strings.Contains(v, "hobbylabs") {
		t.Errorf("fresh agent not in the office:\n%s", v)
	}
}

func TestCtrlTInThePickerOpensATerminalInsteadOfAnAgent(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	m = press(withProjects(m, deck), key("n"))
	m = press(m, key("ctrl+t"))
	if len(rec.started) != 1 || strings.Join(rec.started[0].argv, " ") != "pwsh" || rec.started[0].dir != deck.Path {
		t.Fatalf("started = %+v", rec.started)
	}
	if !strings.Contains(view(m), "TERM pwsh") {
		t.Errorf("view:\n%s", view(m))
	}
}

func TestATypedNewFolderIsCreatedBeforeStartingThere(t *testing.T) {
	m, _, rec := twoAgentOffice(t)
	dir := filepath.Join(t.TempDir(), "brand-new")
	m = press(withProjects(m, portal), key("n"))
	m = press(m, tea.PasteMsg{Content: dir})
	if !strings.Contains(view(m), "create brand-new") {
		t.Fatalf("picker should offer to create the folder:\n%s", view(m))
	}
	m = press(m, key("enter"))
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	if len(rec.started) != 1 || rec.started[0].dir != dir || rec.started[0].argv[0] != "claude" {
		t.Errorf("started = %+v", rec.started)
	}
}
