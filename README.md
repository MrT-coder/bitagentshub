# BitAgentsHub

<a href="https://github.com/Gentleman-Programming/gentle-ai">
  <img width="220" src="https://raw.githubusercontent.com/Gentleman-Programming/gentle-ai/main/docs/assets/brand/built-with-gentle-ai.png" alt="Built with Gentle-AI" />
</a>

A pixel-art office in your terminal where every running Claude Code agent
is a character at its desk: you see who is working, thinking or waiting,
open any agent's terminal in a window, and follow its token spend and your
plan's limits. A pet in the corner keeps an eye on it all. Everything is
read from local files (`~/.claude/projects`); nothing is sent anywhere.

It runs on Windows, macOS and Linux in any truecolor terminal, as one
binary called `bah`.

> **Status: early preview, under construction.** So far it has been used
> and tested on **Windows with Claude Code**. It builds for macOS and Linux
> and its tests pass there, but nobody has used it day to day on them yet.
> Other agents are partly there: `-agent-cmd codex` starts Codex and the
> usage tab reads Codex's limits, while the desks, states and spend come
> from Claude Code's transcripts only. Trying it on another system or with
> another agent, and reporting what breaks, is one of the most useful
> [contributions](CONTRIBUTING.md#where-help-is-most-welcome).

The top of the screen lists the stations side by side, as herdr does: number,
agent program and folder name (`1 claude · my-app`), the git branch of its folder with
the commits not pushed yet in green (`↑2`) and those to pull in red (`↓1`),
and the state, the open station marked. A glance from inside a window tells
which agent is free, and `ctrl+a N` or a click on its column jumps there.
With many stations, folder names and branches shrink so all of them fit;
when they still do not, the strip shows the stations around the open one,
and `‹ +2` / `+1 ›` count those out of view, as zellij's tab bar does. The
mouse wheel over the strip, or a click on a marker, scrolls it one station;
it stays there until a station is opened. Branches are read every 5
seconds from `.git` itself; `git` runs only when a commit, checkout, fetch
or push moved a ref, and the arrows need it on the PATH and an upstream
branch.

Only agents that are running appear: the ones the office starts itself
(`n`), and, when [herdr](#herdr-integration-optional) runs, the ones in its
panes. An agent started in another terminal does not appear. A session that
ended leaves the office, even though its transcript stays on disk.

## Install

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/MrT-coder/bitagentshub/master/install.ps1 | iex
```

Linux and macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/MrT-coder/bitagentshub/master/install.sh | sh
```

The installers download the release archive for your OS and architecture,
check it against `checksums.txt`, and install the binary into
`%LOCALAPPDATA%\bitagentshub\bin` (added to your user PATH) or
`~/.local/bin`. Running them again upgrades in place.

| Variable | Meaning |
|---|---|
| `BAH_VERSION` | Tag to install, e.g. `v0.2.0` (default: latest release) |
| `BAH_INSTALL_DIR` | Where the binary goes |
| `GITHUB_TOKEN` | Optional; avoids GitHub API rate limits |

A script downloaded from a release page instead may be blocked by the
execution policy; run it with
`powershell -ExecutionPolicy Bypass -File install.ps1`.

With Go: `go install github.com/MrT-coder/bitagentshub@latest`.

`bah -version` prints the installed version.

## Run

```sh
bah
```

From source: `go build -o bah.exe .`

| Flag | Default | Meaning |
|---|---|---|
| `-week-start` | `mon` | Day the weekly window resets |
| `-active` | `24h` | Running agents whose transcript changed within this window get their spend and state from it |
| `-root` | `~/.claude/projects` | Transcripts directory |
| `-herdr` | herdr's default socket | herdr socket for linking, jumping and `-host herdr`; `-herdr ""` disables |
| `-host` | `local` | Where new agents run: `local` (a terminal inside the office) or `herdr` |
| `-agent-cmd` | `claude` | Command new agents run, e.g. `"claude --model sonnet"` or `codex` |
| `-roots` | none | Extra folders whose subfolders are offered as projects, comma separated |
| `-shell` | `pwsh` if installed (on Windows, not the Microsoft Store one, which a pseudo terminal cannot start), else the platform shell | Command plain terminals run |
| `-daemon` | `true` | Keep local agents and terminals in a background daemon, so they survive quitting the office, a dropped SSH session and rebuilds; `-daemon=false` runs them inside the office |
| `-config` | `%APPDATA%\bitagentshub\config.toml` (`os.UserConfigDir()`) | Config file with keybindings, see [Keybindings](#keybindings) |
| `-version` | | Print the version and exit |

## Keys

| Key | Action |
|---|---|
| click an agent, `enter`, `c` | Open the agent's window |
| `ctrl+a` `1`…`9` | Open station N (its number is on the floor next to the name) |
| click the pet, `p` | Open the hub (see [The pet and its hub](#the-pet-and-its-hub)); `u` and `s` open it on the usage and settings tabs |
| `t` | Open a terminal in the selected agent's folder |
| `b` | Open the selected agent's task note for today (see [Task board](#task-board)) |
| `n` | Start a new agent or terminal in a project |
| `←` `→` `↑` `↓` / `h` `l` `j` `k` / `tab` | Move the selection |
| `g` | Jump to the agent's pane in herdr (only while herdr runs) |
| `q`, `esc` | Quit; agents and terminals keep running in the daemon (without it, asks again while they run) |

## The window

Each terminal keeps 2000 lines of history by default; a full line costs
about 112 bytes per character, so that is near 16 MB per terminal. Change it
in `config.toml` (100 to 100000 lines); the office and the daemon read it
when they start, so restart the daemon (`bah daemon stop`, which
ends its programs) to apply it to the terminals it hosts:

```toml
[terminal]
scrollback = 2000
```

A window shows one folder: the agent on the left and any plain terminals
beside it, each a real terminal (pseudo terminal, ConPTY on Windows, drawn
through [`charmbracelet/x/vt`](https://github.com/charmbracelet/x)). Output
streams in as it is written and every key goes to the focused pane exactly as
typed: Claude Code, Codex, `nvim`, a shell. Window actions sit behind a
prefix, `ctrl+a` as in herdr and tmux, so programs keep their keys; the old
F-keys still work.

| Key | Action |
|---|---|
| `ctrl+a` `c`, `F3` | New terminal in the folder, beside the others |
| `ctrl+a` `h` / `l` | Focus the pane on the left / right (a click also focuses a pane) |
| `ctrl+a` `x`, `F5` | Close the focused terminal |
| `ctrl+a` `z`, `F2` | Maximize the focused pane to the whole office, or restore the window; a `z` in the top right corner marks it; while maximized, `h` / `l` maximize the pane beside it instead, and a new terminal restores the window |
| `ctrl+a` `d`, `F10` | Close the window; `ctrl+q` too when the agent pane is focused (in a terminal pane `ctrl+q` goes to the program, it is nvim's visual block in Windows Terminal) |
| `ctrl+a` `1`…`9` | Close this window and open station N in one step |
| `ctrl+a` `ctrl+a` | Send `ctrl+a` itself to the program |
| `ctrl+a` `p` | Open the hub over this window; `ctrl+a` `u` / `s` open it on the usage / settings tab |
| `ctrl+a` `b` | Open this folder's task note for today in the board's editor, beside the other panes |
| `ctrl+a` `shift+n` | Start a new agent: the picker opens over this window |
| drag with the left button | Select text in a pane and copy it to the clipboard on release (OSC 52); a click, a key or the wheel clears it |
| mouse wheel | Scroll the pane under the cursor |

- **Scrolling**: programs on the normal screen scroll through the history
  the office keeps (2000 lines by default, see above). Full-screen programs that track the mouse
  (Claude Code after it loads, nvim) receive the wheel and clicks themselves.
- **Local agents** (`n`, the default `-host local`) run inside the office. For
  Claude the office passes a fresh `--session-id`, so the agent's transcript
  and spend attach to it as soon as it starts working.
- **Terminals** keep running when the window closes and come back when the
  folder's window opens again.
- **Shift+enter** starts a new line: agents get it as alt+enter, which Claude
  Code reads as a new line (what its `/terminal-setup` binds); terminals on
  Windows get shift+enter itself, so PowerShell adds a line. On Windows the
  office asks the terminal for win32-input-mode while it runs, since keys read
  as plain VT input lose the shift of shift+enter.
- **Ended programs** clear themselves: when a terminal or an agent exits
  (`exit`, `ctrl+d`, a crash), its pane leaves the window at once, an agent
  leaves the office, and the window closes when no pane is left.
- **The daemon** keeps local agents and terminals running after the office
  quits; the next office shows them again with their screen and history. See
  [The daemon](#the-daemon).
- **Agents in herdr**: on Linux and macOS their window runs
  `herdr agent attach`, so you talk to the real pane while herdr keeps it
  alive. herdr has no direct attach on Windows yet; there `g` jumps to the
  pane and `m` (pressed twice, only while the agent waits) moves it into the
  office: its herdr pane closes and a new local agent starts in the same
  folder, as a new conversation.

## Keybindings

Keys are set in `config.toml`, the same way herdr does it: `%APPDATA%\bitagentshub\config.toml`
on Windows (`os.UserConfigDir()/bitagentshub` elsewhere), or the file given
with `-config`. Without a file the defaults below apply. Each action takes one
key or a list; `prefix+K` is `K` pressed after the prefix and `prefix+1..9` is
a range for `focus_station`. An action set in the file replaces all its
default keys. When a key you bind is also a default key of another action
(or your prefix), yours wins and that action loses only that key, so a new
default in a later version never stops your config from loading. A key that
cannot be read, an unknown action or two actions in your file on the same
key stop the office at start with an error naming the file, the action and
the key.

```toml
[keys]
prefix = "ctrl+a"
focus_station    = "prefix+1..9"
close_window     = ["prefix+d", "f10", "ctrl+q"]
toggle_maximize  = ["prefix+z", "f2"]
new_terminal     = ["prefix+c", "f3"]
close_terminal   = ["prefix+x", "f5"]
focus_pane_left  = "prefix+h"
focus_pane_right = "prefix+l"
open_hub         = "prefix+p"  # `p` on the board too
show_usage       = "prefix+u"  # the hub on its usage tab; `u` on the board too
show_settings    = "prefix+s"  # the hub on its settings tab; `s` on the board too
open_board       = "prefix+b"  # `b` on the board too
new_agent        = "prefix+shift+n"  # `n` on the board too
```

A window's footer shows only the main keys after the prefix (close,
maximize, board, new agent); the hub's settings tab lists them all.

Keys are written as `ctrl+`, `alt+`, `shift+` plus one character, `f1`…`f12`
or a name (`enter`, `esc`, `tab`, `space`, `backspace`, `delete`, `insert`,
`home`, `end`, `pgup`, `pgdown`, `up`, `down`, `left`, `right`).

The picker (`n`) lists folders where agents worked recently (most recent
first). A repository counts once: a subfolder an agent worked in joins it
when an agent also worked at its root. Folders with the same name and group
show their whole parent folder. Typing filters these and every other folder
next to them and under `-roots`. A typed absolute path also works, and a
folder name no listed folder has goes next to the most recent one; either,
when the folder does not exist, is offered as "create". `enter` starts an
agent there, `ctrl+t` opens a terminal instead, `esc` closes the picker.

## The daemon

When it opens, the office attaches to the background daemon, starting it if
none runs. The daemon
owns the programs' pseudo terminals, so quitting the office, losing an SSH
connection or rebuilding `bah.exe` does not end them. It runs from
its own copy of the executable (in `%LOCALAPPDATA%\bitagentshub\daemon`, or
the user cache directory elsewhere), listens only on a pipe (a socket outside
Windows) that only your user can open, and exits by itself once no office is
attached and no program runs. Idle, it costs no CPU and about 18 MB.

```sh
bah daemon status   # pid and the programs it runs
bah daemon stop     # end the daemon and every program in it
bah daemon serve    # run it in the foreground (the office starts it for you)
```

An office from a build with another daemon protocol leaves a running daemon
alone and says so in its footer; `bah daemon stop` replaces it
(ending its programs).

## herdr integration (optional)

[herdr](https://github.com/herdrdev/herdr) is a separate terminal
multiplexer for coding agents. BitAgentsHub does not need it: without herdr
the office starts and hosts its own agents. When herdr runs, the office also
talks to it over its local socket and adds what only herdr knows:

- agents started in herdr panes appear in the office;
- the `blocked` status (an agent waiting on a permission prompt), with its
  red monitor and raised hand, and the pet's alert mood;
- `g` to jump to an agent's pane, `m` to move it into the office, and
  `-host herdr` to start new agents in herdr.

The footer shows `g herdr · herdr: N linked` while herdr answers, and
nothing about herdr otherwise. The socket is herdr's default, or
`HERDR_SOCKET_PATH`, or the session in `HERDR_SESSION`; `-herdr ""` turns
the integration off.

When herdr is running, each session is
linked to the pane that hosts it: first by the Claude session id herdr
reports, then by working directory when exactly one Claude pane sits there
(herdr keeps the old id after a resume). Linked agents show herdr's
permission prompts as `blocked`, `g` focuses their pane, and with
`-host herdr` new agents open in herdr: in the workspace already working in
that folder, split to the right, or in a new workspace for the project.
Fresh herdr agents appear in the office before they write a transcript. On
Windows the socket is the named pipe `\\.\pipe\<path to herdr.sock>`.

herdr is an independent project under its own license; BitAgentsHub is not
affiliated with it and only uses its public local API.

## The pet and its hub

A cat lives on a bed in the office's bottom-left corner. It never walks, but
its mood shows how the office is doing:

| Mood | When |
|---|---|
| eating `🐟` | A task was just checked off on the board: each one is a fish it eats |
| alert `!` | An agent waits on a permission prompt |
| exhausted `x` | A tool's limit is at 90% or more |
| hungry (empty bowl) | The board is on and no task was checked off today |
| tired `~` | A tool's limit is at 70% or more |
| busy | Agents are working |
| sleeping `z` | Nobody works |

The most urgent one shows. Token spend never feeds it: only finished tasks
do. Limits are read every half minute from the same local files as the
usage tab.

Clicking the pet, `p` on the board or `ctrl+a p` in a window opens the hub
over whatever is on screen: one window with the **pet**, **usage**,
**settings** and **about** tabs. `←`/`→` switch tabs; `esc`, `q`, `p` or
`ctrl+a p` close it, and nothing typed or clicked meanwhile reaches a
program. `u` / `ctrl+a u` and `s` / `ctrl+a s` open it on the usage and
settings tabs, or close it when it already shows that tab.

- **Pet**: its name, its mood and why, and the fish earned in all. `r`
  names it: type the name (up to 20 characters) and press `enter`; it is
  saved as `[pet] name` in `config.toml`, changing only that line. Resting
  the mouse on the pet shows its name over it.
- **About**: the version and this project's [changelog](CHANGELOG.md),
  carried inside the binary; `↑`/`↓` scroll it.

## Usage tab

Each station gets a row with bars for today's and
the session's tokens (scaled to the largest shown), then the office totals
for today and the week. Below come the tools that report their own use on
this machine and, only when a tool exposes them locally, its real limits
with the share used and the reset time. Nothing reads credentials or asks a
server: a tool without local limits shows usage only.

- **Claude Code** keeps its 5-hour and weekly limits to itself except for the
  status line it runs (only for Pro and Max plans, after the first reply in a
  session). `bah statusline install` sets Claude Code's status line
  to a copy of bah (in `%LOCALAPPDATA%\bitagentshub\bin`, so
  rebuilds never fight a running file); it saves only those two windows and
  then runs the status line you had before, if any. Your `settings.json` is
  backed up first to `settings.json.bah-backup`;
  `bah statusline uninstall` puts the previous status line back.
  Without it Claude shows usage only, and the tab says to turn on **Claude
  limits** in the settings tab, which does the same as the command.
- **Codex CLI** writes its usage and limits into its session files
  (`~/.codex/sessions`, or `CODEX_HOME`); they are read as they are, nothing
  to install.

## What the numbers mean

- **Tokens** = input + output + cache creation. Cache reads are excluded
  because they are billed at a fraction of input and would dominate the total.
- **Today** starts at local midnight; **Week** starts at `-week-start` 00:00.
- **Reading**: on start the office reads the active transcripts at once and
  the older ones of the last eight days in steps, newest first, so it opens
  right away; while the header says `reading N transcripts`, Today and Week
  are still growing.
  Both cover every session, not only the visible ones.
- Subagent tokens are added to the session that spawned them.

## Settings tab

`↑`/`↓` move and `tab` changes section:

- **Theme**: the themes that come with the office (`default`, `night`,
  `retro`) and every theme in the themes folder, the active one marked. `enter` applies the selected theme at once and saves it
  in `config.toml`, changing only the `name` line of `[theme]` (comments and
  everything else stay). `n` asks for a name and copies the active theme
  there to edit.
- **Language**: English and Español, the one in use marked. `enter`
  switches the whole office to the selected language at once and saves it in
  `config.toml` as `[ui] language`, changing only that line.
- **Claude limits**: turns the status line command on or off, the same as
  `bah statusline install` / `uninstall`.
- **Keys**: every action with its keys, and the prefix. `enter` waits for
  the new key: the prefix and then a key binds a key after the prefix, a key
  alone binds that key. It replaces the action's keys, is saved in
  `config.toml` `[keys]` (only that line changes) and works at once.
  `backspace` puts back the action's default keys. A key another action
  already uses is refused; change that one first. `open station` takes a
  range, so it is set in `config.toml`.
- Where `config.toml` is.

The language can also be set by hand in `config.toml`:

```toml
[ui]
language = "es"  # "en" or "es"; without it the office follows the system language
```

Without `[ui] language` the office speaks Spanish when the system language
is Spanish and English otherwise. Another value stops the office at start
with an error naming the file. Only what the office draws is translated:
command-line output, `-help`, this README and the changelog stay in English,
and the programs in the panes are never touched.

## Task board

The board keeps the tasks you have in mind as markdown notes in a folder
outside your repositories, so nothing reaches git: an Obsidian vault works
well. Turn it on in `config.toml`:

```toml
[board]
dir = 'C:\Users\me\Obsidian\board'  # an absolute folder
editor = "nvim"                     # the default
```

- **Notes**: one per project (the station folder's name) and day,
  `<dir>/<project>/<YYYY-MM-DD>.md`, with tasks as `- [ ]` and `- [x]`.
- **Opening**: `b` on the board or `ctrl+a b` in a window opens today's note
  in the editor, in a pane beside the others; it closes when the editor
  exits. Today's note is created the first time, with every unfinished task
  of the project's last note carried over; earlier notes are left as they
  are.
- **Agents**: Claude agents the office starts get `--add-dir <dir>` and an
  instruction to check off, in today's note, the tasks they finish.
- **Rewards**: when a task of today's note is checked off, the project's
  agents cheer (a ★ over them) for a few seconds and the pet eats a fish.
  The header shows the fish earned, `🐟` and one per task checked off on the
  board, and each station's tasks to do.

## Themes

Colors and the characters' pixel art come from a theme: a folder of plain
text files next to `config.toml`. `bah theme init <name>` writes
the built-in theme there to start from, and `[theme] name = "<name>"` in
`config.toml` selects it. See [docs/themes.md](docs/themes.md) for the
format, the letters each pixel uses and the rules a theme must follow.

## The office

A pixel-art office in the spirit of
[pixel-agents](https://github.com/pixel-agents-hq/pixel-agents), drawn with
half-block characters so it works in any truecolor terminal, including herdr
panes (which only pass Kitty graphics, unavailable in Windows Terminal). The
art is original and sized for the terminal; BitAgentsHub is not affiliated
with pixel-agents.

- **Room**: a back wall with windows, paintings and a clock; a wooden floor;
  a bookshelf, coffee machine, water cooler, sofa and plants where they fit,
  and the pet's corner (see [The pet and its hub](#the-pet-and-its-hub)).
- **Areas**: each workspace folder gets a carpet with its workstations.
  Sessions under `C:\work\acme-company\` share the `acme-company`
  carpet, and so on; sessions with no known directory go to `other`. The
  label shows the area's spend today.
- **Workstations**: chair, desk, monitor and mug. Under each desk: the
  project, today's tokens and status.
- **Status**: the desks and the station strip use herdr's four statuses, each
  with its own icon and color:

  | Status | Meaning |
  |---|---|
  | `● working` (yellow) | Thinking or running a tool |
  | `▲ blocked` (red) | Waiting on a permission prompt (reported by herdr) |
  | `✓ done` (blue) | Finished while its window was closed; opening it clears this |
  | `○ idle` (gray) | Waiting for you, already seen, or asleep |
- **People**: every agent gets a stable look (hair, skin, shirt in its color).
  They walk around furniture using pathfinding, sit at their desk to work and,
  while waiting for you, visit the window, bookshelf, coffee machine, cooler
  or sofa.

| State | On screen |
|---|---|
| `tool` editing (`Edit`, `Write`, …) | Typing; code scrolls on the monitor |
| `tool` reading (`Read`, `Grep`, `Glob`, `WebFetch`, `WebSearch`) | Holding a page; text on a white monitor |
| `tool` running (`Bash`, `PowerShell`) | Typing fast; green terminal on the monitor |
| `thinking` | Hand on chin, `…` bubble, dots on the monitor |
| `waiting` | Leaves the desk and roams the office with a `?` bubble |
| `blocked` (herdr reports a permission prompt) | Hand raised, red `!` bubble, red monitor |
| `asleep` (no activity for 5 minutes) | Head on the desk, `z`, monitor off |

The window opens as a modal over the office, which keeps animating around it.
Each pane's title shows the station number, the folder and the git branch
checked out there (`1 bitagentshub ~ develop`); a folder outside a repository
shows only its name.

`go run ./cmd/probe` prints the same data without the TUI, with load timings;
`go run ./cmd/probe render` prints one frame of the office, and
`go run ./cmd/probe render office.png` saves it as an image;
`go run ./cmd/probe term 5 claude` runs a program in the office's terminal
for five seconds and prints its screen.

## Profiling

Set `BAH_CPUPROFILE` to a folder and the office, and the daemon it
starts, each write a CPU profile of their first minute there
(`office-<pid>.pprof`, `daemon-<pid>.pprof`). The daemon only starts when
none runs, so stop it first (`bah daemon stop`, which ends its
programs) to profile it too. Read a profile with
`go tool pprof -top bah.exe office-<pid>.pprof`.

```powershell
bah daemon stop
$env:BAH_CPUPROFILE = "$env:TEMP\bah-prof"
bah
```

## Releasing

Versions follow [semver](https://semver.org) and live in git tags. Pushing a
tag publishes a GitHub Release through GoReleaser
(`.github/workflows/release.yml`): tests run, then binaries for Windows,
Linux and macOS (amd64 and arm64), `checksums.txt`, both install scripts and
a changelog grouped from the conventional commits since the previous tag.

```sh
git tag -a v0.2.0 -m "v0.2.0"
git push origin v0.2.0
```

Try the release locally without publishing:
`goreleaser release --snapshot --clean` (artifacts land in `dist/`).

## Contributing

Issues, ideas, themes and pull requests are welcome; read
[CONTRIBUTING.md](CONTRIBUTING.md) first.

## License

[MIT](LICENSE) © Josue Bladimir Morales Llanganate (MrT-coder). The name
and logos follow the [trademark policy](TRADEMARKS.md).
