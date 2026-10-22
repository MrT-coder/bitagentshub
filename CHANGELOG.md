# Changelog

Notable changes to BitAgentsHub, newest first. The office shows this file
in the hub's About tab, so every release adds its section here.

## Unreleased

- Marked as an early preview, tested on Windows with Claude Code, in the
  README, the contributing guide, the bug report form and the About tab.
- **Renamed to BitAgentsHub**, run as `bah`. Config, themes and state live
  in `bitagentshub` folders (`%APPDATA%\bitagentshub` on Windows).
- `bah -h` lists the subcommands (`daemon`, `theme`, `statusline`).
- The footer mentions herdr only while herdr answers; herdr stays an
  optional integration.
- The usage tab says where to turn on Claude limits when they are off.
- The About tab shows the license and the repository; release archives
  carry the LICENSE file.
- Open source under the MIT License, with a contributing guide, issue and
  pull request templates, and a trademark policy.
- Faster frames: color codes are written only when the color changes and
  the office background is drawn once per layout and theme (about 2.5 times
  cheaper per frame). `git` runs only when a branch moved, not every 5
  seconds per station. Terminals keep 2000 lines of history by default
  (`[terminal] scrollback` in `config.toml`), down from 5000.
- The office speaks English or Spanish, chosen in the settings tab's
  Language section and saved as `[ui] language` in `config.toml`; without
  it, the office follows the system language.
- A pet sleeps, works, gets tired and eats in the office's bottom-left
  corner. Its mood comes from the agents, the token limits the tools report
  and the tasks checked off on the board: each one is a fish it eats, and
  the header counts them with `🐟`.
- The pet takes a name (`r` in its tab, saved in `config.toml`) and shows it
  when the mouse rests on it.
- One hub, opened by clicking the pet or with `p` / `ctrl+a p`, holds the
  pet, usage, settings and about tabs.
- The station strip fits every station, scrolls with the wheel or its
  `‹ +2` / `+1 ›` markers, and a click opens a station.
- Every key binding is listed and can be changed in settings.
- `ctrl+a shift+n` starts a new agent from a window; the window footer only
  shows the prefix keys.

## v0.1.2 (2026-10-21)

- Task board: daily task notes per project, opened with `b`; checked tasks
  are stars and make the project's agents cheer.
- Agents show herdr's four statuses with icons.
- The new agent picker lists each recent folder once and can create a
  folder.
- The office opens at once and reads older transcripts in steps.
- `BAH_CPUPROFILE` writes a CPU profile.

## v0.1.1 (2026-10-21)

- shift+enter starts a new line on Windows.
- `ctrl+a z` maximizes the focused pane.
- Ended terminals and agents clear on their own.

## v0.1.0 (2026-10-21)

- First release: the pixel office with agents at their desks, windows with
  real terminals, the agent daemon, themes, the usage window and the
  installers.
