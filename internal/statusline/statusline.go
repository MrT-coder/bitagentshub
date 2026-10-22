// Package statusline is the status line command bah installs in
// Claude Code. Claude hands its status line the account's rate limits, which
// it writes nowhere else on disk; the command keeps only those windows so the
// usage window can show real limits, without reading or storing credentials.
package statusline

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/usage"
)

// chainTimeout bounds the status line BitAgentsHub replaced, so a slow one
// cannot hold Claude Code's status line forever.
const chainTimeout = 5 * time.Second

// Run saves the rate limits in the JSON Claude Code passes on stdin under
// stateDir, then runs the status line Install replaced with the same input
// and prints its output. Input without rate limits leaves the saved file as
// it was. A failing previous status line is not an error.
func Run(stdin io.Reader, stdout io.Writer, stateDir string) error {
	in, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	saveErr := saveLimits(in, stateDir, time.Now())
	command, err := previousCommand(stateDir)
	if err != nil {
		return err
	}
	if command != "" {
		if _, err := stdout.Write(chain(command, in)); err != nil {
			return err
		}
	}
	return saveErr
}

// saveLimits decodes only rate_limits, so nothing else from the input (the
// session, paths, prompts) is ever held, let alone written.
func saveLimits(in []byte, stateDir string, now time.Time) error {
	var v struct {
		RateLimits struct {
			FiveHour *usage.ClaudeWindow `json:"five_hour"`
			SevenDay *usage.ClaudeWindow `json:"seven_day"`
		} `json:"rate_limits"`
	}
	if json.Unmarshal(in, &v) != nil {
		return nil
	}
	l := usage.ClaudeLimits{Observed: now.UTC(), FiveHour: v.RateLimits.FiveHour, SevenDay: v.RateLimits.SevenDay}
	if l.FiveHour == nil && l.SevenDay == nil {
		return nil
	}
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(stateDir, usage.ClaudeLimitsFile), append(data, '\n'), 0o600)
}

// chain runs command with in on stdin and returns what it printed, whatever
// its exit status.
func chain(command string, in []byte) []byte {
	ctx, cancel := context.WithTimeout(context.Background(), chainTimeout)
	defer cancel()
	name, args := shell()
	cmd := exec.CommandContext(ctx, name, append(args, command)...)
	cmd.Stdin = bytes.NewReader(in)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.WaitDelay = time.Second // a child left holding stdout must not hang us
	_ = cmd.Run()
	return out.Bytes()
}

// shell picks a shell the way Claude Code runs status lines: bash when there
// is one, else cmd on Windows, else sh. On Windows the bash in System32 is
// the WSL launcher, which would run the command in another system.
func shell() (string, []string) {
	if path, err := exec.LookPath("bash"); err == nil && !wslLauncher(path) {
		return path, []string{"-c"}
	}
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C"}
	}
	return "sh", []string{"-c"}
}

func wslLauncher(path string) bool {
	root := os.Getenv("SystemRoot")
	return runtime.GOOS == "windows" && root != "" &&
		strings.EqualFold(filepath.Dir(path), filepath.Join(root, "System32"))
}
