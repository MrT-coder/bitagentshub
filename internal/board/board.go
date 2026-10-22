// Package board keeps the task board (a "whiteboard"): one markdown note per
// project and day in a folder outside the repositories, such as an Obsidian
// vault. Tasks are checkboxes, "- [ ]" pending and "- [x]" done.
package board

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const dayLayout = "2006-01-02"

// task matches a markdown checkbox line and captures its mark.
var task = regexp.MustCompile(`^\s*[-*+] \[([ xX])\](\s|$)`)

// NotePath is project's note for day: <dir>/<project>/<YYYY-MM-DD>.md.
func NotePath(dir, project string, day time.Time) string {
	return filepath.Join(dir, project, day.Format(dayLayout)+".md")
}

// Today returns the path of project's note for now's day, creating it when
// missing with a title and every unchecked task of the project's latest
// earlier note. Earlier notes are never changed.
func Today(dir, project string, now time.Time) (string, error) {
	path := NotePath(dir, project, now)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	text := "# " + project + " · " + now.Format(dayLayout) + "\n\n"
	if prev, err := latestBefore(filepath.Dir(path), now); err != nil {
		return "", err
	} else if prev != "" {
		b, err := os.ReadFile(prev)
		if err != nil {
			return "", err
		}
		for _, line := range lines(string(b)) {
			if m := task.FindStringSubmatch(line); m != nil && m[1] == " " {
				text += line + "\n"
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	// O_EXCL: a note created meanwhile (by the user or another office) wins.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close()
		return "", err
	}
	return path, f.Close()
}

// latestBefore is the newest daily note in dir dated before now's day, or ""
// when there is none.
func latestBefore(dir string, now time.Time) (string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	today := now.Format(dayLayout)
	var days []string
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if !ok || e.IsDir() {
			continue
		}
		if _, err := time.Parse(dayLayout, name); err == nil && name < today {
			days = append(days, name)
		}
	}
	if len(days) == 0 {
		return "", nil
	}
	sort.Strings(days)
	return filepath.Join(dir, days[len(days)-1]+".md"), nil
}

// Count is how many tasks a note holds.
type Count struct{ Pending, Done int }

// Tasks counts the checkboxes in a note's text.
func Tasks(text string) Count {
	var c Count
	for _, line := range lines(text) {
		switch m := task.FindStringSubmatch(line); {
		case m == nil:
		case m[1] == " ":
			c.Pending++
		default:
			c.Done++
		}
	}
	return c
}

// Stars are done tasks: Total in every note of the board, Today in the
// notes of the day.
type Stars struct{ Total, Today int }

// Points counts the stars of every note on the board, today's being the
// notes named after now's day; a board not made yet has none.
func Points(dir string, now time.Time) (Stars, error) {
	var s Stars
	today := now.Format(dayLayout) + ".md"
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		done := Tasks(string(b)).Done
		s.Total += done
		if d.Name() == today {
			s.Today += done
		}
		return nil
	})
	return s, err
}

func lines(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}
