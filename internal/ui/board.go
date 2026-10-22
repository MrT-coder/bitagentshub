package ui

import (
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MrT-coder/bitagentshub/internal/board"
	"github.com/MrT-coder/bitagentshub/internal/claude"
)

const (
	boardEvery = 3 * time.Second
	cheerFor   = 5 * time.Second
)

type (
	// boardMsg is what the board held at one read: today's tasks by
	// project folder name, and the stars (done tasks, in all and today).
	boardMsg struct {
		tasks map[string]board.Count
		stars board.Stars
	}
	boardTickMsg struct{}
)

// projectOfAgent is the board project an agent works on: its folder's name.
func projectOfAgent(a claude.Agent) string {
	if a.Cwd != "" {
		return filepath.Base(a.Cwd)
	}
	return a.Project
}

// boardPoll reads today's note of every station's project and the points,
// off the UI goroutine. It never creates a note. Without a board it is nil.
func (m Model) boardPoll() tea.Cmd {
	if m.board.Dir == "" {
		return nil
	}
	dir := m.board.Dir
	var projects []string
	seen := map[string]bool{}
	for _, a := range m.agents() {
		if p := projectOfAgent(a); p != "" && !seen[p] {
			seen[p] = true
			projects = append(projects, p)
		}
	}
	return func() tea.Msg {
		now := time.Now()
		out := boardMsg{tasks: map[string]board.Count{}}
		for _, p := range projects {
			if b, err := os.ReadFile(board.NotePath(dir, p, now)); err == nil {
				out.tasks[p] = board.Tasks(string(b))
			}
		}
		out.stars, _ = board.Points(dir, now)
		return out
	}
}

// boardRead takes in a read of the board: a project whose done tasks grew
// since the last read cheers for a while. The first read only sets the
// baseline, so tasks done while the office was closed are not news.
func (m Model) boardRead(msg boardMsg) Model {
	if m.cheers == nil {
		m.cheers = map[string]time.Time{}
	}
	if m.tasks != nil {
		for p, c := range msg.tasks {
			if c.Done > m.tasks[p].Done {
				m.cheers[p] = time.Now().Add(cheerFor)
			}
		}
		if msg.stars.Total > m.stars.Total {
			m.eatUntil = time.Now().Add(eatFor) // the pet eats the new star
		}
	}
	m.tasks, m.stars = msg.tasks, msg.stars
	return m
}

// cheer marks the agents whose project just checked off a task.
func (m Model) cheer(agents []claude.Agent) []claude.Agent {
	if len(m.cheers) == 0 {
		return agents
	}
	now := time.Now()
	for i, a := range agents {
		agents[i].Cheer = now.Before(m.cheers[projectOfAgent(a)])
	}
	return agents
}
