# Contributing to BitAgentsHub

Thanks for wanting to help. Bug reports, ideas, themes and code are all
welcome. This guide keeps everyone's time well spent.

## Issue first

1. **Open an issue** with the [bug report](https://github.com/MrT-coder/bitagentshub/issues/new?template=bug_report.yml)
   or [feature request](https://github.com/MrT-coder/bitagentshub/issues/new?template=feature_request.yml) form.
2. **Wait for approval.** A maintainer labels the issue `status:approved`
   when the change is wanted and its scope is clear.
3. **Say you are on it** in a comment, so nobody does the same work twice.
4. **Open a pull request** against `develop` that links the issue
   (`Closes #123`).

Pull requests without an approved issue may be closed. Typo and doc fixes
are the exception: send them straight away.

## Where help is most welcome

BitAgentsHub is an early preview: it has been used and tested on Windows
with Claude Code. Help is most valuable where that leaves gaps:

- **macOS and Linux**: run it day to day and report what looks or works
  wrong, with your terminal and OS version in the
  [bug report](https://github.com/MrT-coder/bitagentshub/issues/new?template=bug_report.yml).
- **Other agents** (Codex, Gemini CLI, opencode, ...): the office reads
  Claude Code's transcripts for desks, states and spend. Support for
  another agent means reading its session files the same way; open a
  feature request first so we agree on the shape.
- **Themes**: see [Themes](#themes) below.

## Development setup

You need Go (the version in `go.mod`) and git. Nothing else: no cgo, no
services, no credentials.

```sh
git clone https://github.com/MrT-coder/bitagentshub.git
cd bitagentshub
git checkout develop
go build -o bah .
./bah
```

On Windows build `bah.exe` instead.

## Tests

Run before every push; CI runs the same on Windows, Linux and macOS:

```sh
go vet ./...
go test ./...
```

- Every behavior change comes with a test that fails without it.
- The office drawing is pinned by image hashes in
  `internal/office/theme_test.go`. When a drawing change is intended,
  explain it in the pull request and retake the hashes; when it is not,
  the failing hash caught a regression.
- Rendering speed matters: the office redraws many times a second. For
  drawing changes, compare `go test -bench Frame -benchmem ./internal/ui/`
  before and after and paste both lines in the pull request.

## Code style

- `gofmt` and `go vet` clean.
- Match the code around your change: its naming, comment density and idiom.
- User-facing text is English and goes through `i18n.T`/`i18n.Tf`; add the
  Spanish line to `internal/i18n/spanish.go` (a test checks the catalog).
- New commands, keys or config options update `README.md` and, for themes,
  `docs/themes.md`.

## Branches and commits

Branches start from `develop`: `feat/<name>`, `fix/<name>`, `docs/<name>`,
`chore/<name>`. `master` only holds releases.

Commits follow [Conventional Commits](https://www.conventionalcommits.org):

```
<type>(<optional scope>): <what changed, in the imperative>
```

Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `chore`, `ci`.
A breaking change adds `!` after the type and a `BREAKING CHANGE:` footer.

```
feat(office): let a theme set the desk colors
fix(ui): keep the header inside narrow terminals
```

Keep pull requests small and focused, ideally under about 400 changed
lines. A larger change is easier to review split into several pull requests.

## AI-assisted contributions

AI tools are welcome; responsibility stays with you.

- You understand and can explain every line you submit.
- Say in the pull request which tool you used, for what, and how you
  verified the result.
- Do not submit generated code you have not run and tested.

## Themes

Themes are data, never code: a folder with a `theme.toml` and pose files,
described in [docs/themes.md](docs/themes.md). A theme may be inspired by a
movie, show or game, but must not ship their logos, official artwork or
trademarks as its own name; see [TRADEMARKS.md](TRADEMARKS.md).

## Security

Do not open a public issue for a security problem. Report it privately
through GitHub's
[security advisories](https://github.com/MrT-coder/bitagentshub/security/advisories/new).

## License

By contributing you agree that your contribution is licensed under the
[MIT License](LICENSE) of this project.
