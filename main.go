// BitAgentsHub shows your running Claude Code agents as pixel sprites in
// the terminal, with token spend per agent.
// Everything is read from local transcripts; nothing leaves the machine.
package main

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/claude"
	"github.com/MrT-coder/bitagentshub/internal/config"
	"github.com/MrT-coder/bitagentshub/internal/daemon"
	"github.com/MrT-coder/bitagentshub/internal/herdr"
	"github.com/MrT-coder/bitagentshub/internal/i18n"
	"github.com/MrT-coder/bitagentshub/internal/office"
	"github.com/MrT-coder/bitagentshub/internal/term"
	"github.com/MrT-coder/bitagentshub/internal/ui"
	"github.com/MrT-coder/bitagentshub/internal/usage"
	"github.com/MrT-coder/bitagentshub/internal/version"
)

// changelog is shown in the hub's About tab; it travels in the binary so
// the office never asks the network for it.
//
//go:embed CHANGELOG.md
var changelog string

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "statusline" {
		if err := runStatusline(os.Args[2:]); err != nil {
			fail(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "theme" {
		if err := runTheme(os.Args[2:]); err != nil {
			fail(err)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "daemon" {
		if err := runDaemon(os.Args[2:]); err != nil {
			fail(err)
		}
		return
	}
	home, _ := os.UserHomeDir()
	root := flag.String("root", filepath.Join(home, ".claude", "projects"), "Claude Code projects directory")
	weekStart := flag.String("week-start", "mon", "day the weekly window resets: sun, mon, ... sat")
	active := flag.Duration("active", 24*time.Hour, "read the spend and state of running agents whose transcript changed within this window")
	herdrSock := flag.String("herdr", herdr.DefaultSocketPath(), "herdr socket path for linking, jumping and -host herdr; empty disables")
	agentCmd := flag.String("agent-cmd", "claude", "command new agents run, e.g. \"claude --model sonnet\"")
	roots := flag.String("roots", "", "extra folders whose subfolders are offered as projects, comma separated")
	shell := flag.String("shell", "", "command plain terminals run (default pwsh, else the platform shell)")
	host := flag.String("host", "local", "where new agents run: local (a terminal inside the office) or herdr")
	useDaemon := flag.Bool("daemon", true, "keep local agents and terminals in a background daemon so they survive quitting and rebuilds")
	configPath := flag.String("config", config.DefaultPath(), "config file: keys, theme, language, task board, pet and terminal history")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() { printUsage(flag.CommandLine.Output()) }
	flag.Parse()

	if *showVersion {
		fmt.Println("bah", version.String())
		return
	}

	if *host != "local" && *host != "herdr" {
		fail(fmt.Errorf("invalid -host %q (use local or herdr)", *host))
	}
	wd, ok := weekdays[strings.ToLower(*weekStart)]
	if !ok {
		fail(fmt.Errorf("invalid -week-start %q (use sun, mon, ... sat)", *weekStart))
	}
	if _, err := os.Stat(*root); err != nil {
		fail(fmt.Errorf("cannot read %s: %w", *root, err))
	}

	if *configPath != config.DefaultPath() {
		if _, err := os.Stat(*configPath); err != nil {
			fail(fmt.Errorf("-config: %w", err))
		}
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fail(err)
	}
	lang := cfg.Language
	if lang == "" {
		lang = string(i18n.System())
	}
	i18n.Set(i18n.Lang(lang)) // before the model, whose notices are written as it is built
	term.SetScrollback(cfg.Scrollback)
	if cfg.Theme != "" {
		t, err := loadTheme(config.ThemesDir(*configPath), cfg.Theme)
		if err != nil {
			fail(err)
		}
		office.SetTheme(t)
	}

	store := claude.NewStore(*root, claude.Options{WeekStart: wd, ActiveWindow: *active})
	model := ui.New(store).WithAgentCmd(strings.Fields(*agentCmd)).WithRoots(splitList(*roots)).WithHost(*host).WithShell(strings.Fields(*shell)).WithKeys(cfg.Keys).WithBoard(cfg.Board).WithChangelog(changelog).WithPetName(cfg.PetName)
	if *herdrSock != "" {
		model = model.WithHerdr(herdr.NewClient(*herdrSock))
	}
	if state, err := stateDir(); err == nil {
		model = model.WithUsage(usage.NewClaude(state), usage.NewCodex(usage.CodexHome()))
		model = model.WithSettings(&officeSettings{
			configPath:   *configPath,
			themesDir:    config.ThemesDir(*configPath),
			settingsPath: filepath.Join(home, ".claude", "settings.json"),
			state:        state,
			active:       cfg.Theme,
			language:     lang,
		})
	}
	if *useDaemon {
		model = model.WithDaemon(daemon.DefaultAddr())
	}
	stopProfile, err := profileCPU("office", cpuProfileFor)
	if err != nil {
		fail(err)
	}
	final, err := tea.NewProgram(model).Run()
	stopProfile()
	if runtime.GOOS == "windows" {
		_, _ = os.Stdout.WriteString(ui.Win32InputOff) // keys go back to the shell as before
	}
	if m, ok := final.(ui.Model); ok {
		m.CloseAll() // the daemon keeps its programs; the office's own end here
	}
	if err != nil {
		fail(err)
	}
}

// printUsage is -h: what bah runs, its subcommands, then the flags.
func printUsage(w io.Writer) {
	fmt.Fprint(w, `usage: bah [flags]
  Opens the office: your running Claude Code agents as pixel sprites.

  bah daemon status|stop|serve   the background daemon that keeps agents running
  bah theme init <name>          write the built-in theme to a folder to edit
  bah statusline install|uninstall
                                 let Claude Code hand its limits to the usage tab

flags:
`)
	flag.CommandLine.SetOutput(w)
	flag.PrintDefaults()
}

// runDaemon handles "bah daemon serve|status|stop".
func runDaemon(args []string) error {
	usage := errors.New("usage: bah daemon serve|status|stop [-addr pipe]")
	if len(args) == 0 {
		return usage
	}
	fs := flag.NewFlagSet("daemon "+args[0], flag.ExitOnError)
	addr := fs.String("addr", daemon.DefaultAddr(), "the daemon's pipe (socket outside Windows)")
	_ = fs.Parse(args[1:])
	switch args[0] {
	case "serve":
		// The daemon keeps the terminals, so it reads their history size;
		// a config it cannot read leaves the default.
		if cfg, err := config.Load(config.DefaultPath()); err == nil {
			term.SetScrollback(cfg.Scrollback)
		}
		stop, err := profileCPU("daemon", cpuProfileFor)
		if err != nil {
			return err
		}
		defer stop()
		return daemon.Serve(*addr)
	case "status":
		pid, sessions, err := daemon.List(*addr)
		if err != nil && !errors.Is(err, daemon.ErrProto) {
			fmt.Println("no daemon running")
			return nil
		}
		if err != nil {
			return err
		}
		fmt.Printf("daemon pid %d, %d program(s)\n", pid, len(sessions))
		for _, s := range sessions {
			state := "running"
			if s.Exited {
				state = "ended"
			}
			fmt.Printf("  %-8s %s  %s  (%s)\n", state, strings.Join(s.Argv, " "), s.Dir, s.ID)
		}
		return nil
	case "stop":
		pid, sessions, err := daemon.List(*addr)
		if err != nil && !errors.Is(err, daemon.ErrProto) {
			fmt.Println("no daemon running")
			return nil
		}
		if err := daemon.Stop(*addr); err != nil {
			return err
		}
		if err != nil {
			fmt.Printf("stopped daemon pid %d (another build)\n", pid)
		} else {
			fmt.Printf("stopped daemon pid %d and its %d program(s)\n", pid, len(sessions))
		}
		return nil
	}
	return usage
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "bah:", err)
	os.Exit(1)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
