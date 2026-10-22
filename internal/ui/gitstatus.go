package ui

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
)

// gitEvery is how often the stations' branches and their distance to the
// upstream are read again.
const gitEvery = 5 * time.Second

// gitState is a folder's branch and how many commits it is ahead of (not
// pushed) and behind its upstream.
type gitState struct {
	branch        string
	ahead, behind int
}

type (
	gitMsg     map[string]gitState // by folderKey
	gitTickMsg struct{}
)

// gitRuns counts the git processes readGit starts.
var gitRuns atomic.Int64

// gitSeen is what readGit last learned of each folder, with the refs it
// read then: while they stay the same, git is not asked again. Starting a
// process every few seconds per station costs more than the rest of the
// office, on Windows above all.
var gitSeen = struct {
	sync.Mutex
	by map[string]gitSeenState
}{by: map[string]gitSeenState{}}

type gitSeenState struct {
	stamp string
	state gitState
}

// readGit reads dir's branch from .git and asks git for the distance to the
// upstream, only when the refs changed since the last read; without git or
// an upstream only the branch is known.
func readGit(dir string) gitState {
	g := gitState{branch: gitBranch(dir)}
	if g.branch == "" {
		return g
	}
	stamp := refsStamp(findGitDir(dir))
	gitSeen.Lock()
	seen, ok := gitSeen.by[dir]
	gitSeen.Unlock()
	if ok && seen.stamp == stamp {
		return seen.state
	}
	defer func() {
		gitSeen.Lock()
		gitSeen.by[dir] = gitSeenState{stamp, g}
		gitSeen.Unlock()
	}()
	gitRuns.Add(1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-list", "--left-right", "--count", "HEAD...@{upstream}").Output()
	if err != nil {
		return g
	}
	if f := strings.Fields(string(out)); len(f) == 2 {
		g.ahead, _ = strconv.Atoi(f[0])
		g.behind, _ = strconv.Atoi(f[1])
	}
	return g
}

// gitPoll reads every station's folder in the background, so a slow git
// never holds the screen.
func (m Model) gitPoll() tea.Cmd {
	var dirs []string
	seen := map[string]bool{}
	for _, a := range m.agents() {
		if k := folderKey(a.Cwd); a.Cwd != "" && !seen[k] {
			seen[k] = true
			dirs = append(dirs, a.Cwd)
		}
	}
	return func() tea.Msg {
		out := gitMsg{}
		for _, d := range dirs {
			out[folderKey(d)] = readGit(d)
		}
		return out
	}
}
