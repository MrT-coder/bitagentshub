package prs

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// LineKind says what a diff line does to the file.
type LineKind int

const (
	Context LineKind = iota // unchanged, shown for orientation
	Added
	Removed
)

// DiffLine is one line of a hunk. Old and New are its line numbers in the
// old and the new file, 0 on the side where the line does not exist: a
// comment on GitHub goes to New for context and added lines and to Old for
// removed ones.
type DiffLine struct {
	Kind     LineKind
	Old, New int
	Text     string // without the leading +, - or space
}

// Hunk is a run of changes with its "@@ -a,b +c,d @@" header.
type Hunk struct {
	Header string
	Lines  []DiffLine
}

// FileStatus is what happened to a file.
type FileStatus int

const (
	StatusModified FileStatus = iota
	StatusAdded
	StatusDeleted
	StatusRenamed
)

// FileDiff is the changes of one file.
type FileDiff struct {
	Path    string // the new path, or the old one for a deleted file
	OldPath string // the path before a rename
	Status  FileStatus
	Binary  bool // changed, but there are no lines to show
	Added   int
	Removed int
	Hunks   []Hunk
}

// Diff reads the diff of pull request number n of repo through gh.
func Diff(ctx context.Context, run Runner, repo string, n int) ([]FileDiff, error) {
	out, err := run(ctx, "pr", "diff", strconv.Itoa(n), "--repo", repo, "--color=never")
	if err != nil {
		return nil, fmt.Errorf("gh pr diff %s#%d: %w", repo, n, err)
	}
	return ParseDiff(string(out)), nil
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ParseDiff reads a unified git diff. What it does not understand it
// leaves out rather than guess.
func ParseDiff(text string) []FileDiff {
	var files []FileDiff
	var cur *FileDiff
	var hunk *Hunk
	var oldN, newN, oldLeft, newLeft int

	flush := func() {
		if cur != nil {
			if hunk != nil {
				cur.Hunks = append(cur.Hunks, *hunk)
				hunk = nil
			}
			files = append(files, *cur)
			cur = nil
		}
	}
	closeHunk := func() {
		if hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
			hunk = nil
		}
	}

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if hunk != nil && (oldLeft > 0 || newLeft > 0) {
			if len(line) > 0 && line[0] == '\\' {
				continue // "\ No newline at end of file"
			}
			var l DiffLine
			switch {
			case strings.HasPrefix(line, "+"):
				l = DiffLine{Added, 0, newN, line[1:]}
				newN++
				newLeft--
				cur.Added++
			case strings.HasPrefix(line, "-"):
				l = DiffLine{Removed, oldN, 0, line[1:]}
				oldN++
				oldLeft--
				cur.Removed++
			default:
				// A context line; an editor may have stripped its lone space.
				l = DiffLine{Context, oldN, newN, strings.TrimPrefix(line, " ")}
				oldN++
				newN++
				oldLeft--
				newLeft--
			}
			hunk.Lines = append(hunk.Lines, l)
			continue
		}
		closeHunk()

		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			cur = &FileDiff{}
			cur.OldPath, cur.Path = splitGitHeader(strings.TrimPrefix(line, "diff --git "))
		case cur == nil:
			// text before the first file
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = StatusAdded
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = StatusDeleted
		case strings.HasPrefix(line, "rename from "):
			cur.Status, cur.OldPath = StatusRenamed, strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			cur.Status, cur.Path = StatusRenamed, strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
			cur.Binary = true
		case strings.HasPrefix(line, "--- "):
			if p := diffPath(line[4:], "a/"); p != "" && cur.Status == StatusDeleted {
				cur.Path = p
			}
		case strings.HasPrefix(line, "+++ "):
			if p := diffPath(line[4:], "b/"); p != "" {
				cur.Path = p
			}
		case strings.HasPrefix(line, "@@"):
			m := hunkHeader.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			oldN, _ = strconv.Atoi(m[1])
			oldLeft = countOrOne(m[2])
			newN, _ = strconv.Atoi(m[3])
			newLeft = countOrOne(m[4])
			hunk = &Hunk{Header: line}
		}
	}
	flush()
	return files
}

// countOrOne is a hunk header's line count, which git leaves out for 1.
func countOrOne(s string) int {
	if s == "" {
		return 1
	}
	n, _ := strconv.Atoi(s)
	return n
}

// diffPath is a path from a "---" or "+++" line without its a/ or b/
// prefix, or "" for /dev/null.
func diffPath(s, prefix string) string {
	s = strings.TrimRight(s, "\t")
	if s == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(s, prefix)
}

// splitGitHeader reads "a/old b/new" from a "diff --git" line. Names with
// spaces are ambiguous here; the --- and +++ lines settle them.
func splitGitHeader(s string) (oldPath, newPath string) {
	i := strings.LastIndex(s, " b/")
	if i < 0 {
		return "", s
	}
	return strings.TrimPrefix(s[:i], "a/"), s[i+3:]
}
