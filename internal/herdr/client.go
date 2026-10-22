// Package herdr talks to a running herdr server over its local socket API to
// find which pane hosts each agent, read its screen and type into it.
package herdr

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/MrT-coder/bitagentshub/internal/claude"
)

// Pane is one herdr pane that hosts a detected agent.
type Pane struct {
	PaneID      string
	WorkspaceID string
	TerminalID  string
	Agent       string // "claude", "codex", ...
	SessionID   string // agent session id as reported to herdr; may be stale
	Status      string // idle, working, blocked, done, unknown
	Cwd         string
	Focused     bool
}

// Target is the most stable handle herdr accepts for agent methods.
func (p Pane) Target() string {
	if p.TerminalID != "" {
		return p.TerminalID
	}
	return p.PaneID
}

const callTimeout = 2 * time.Second

// Client opens one connection per request; herdr closes it after replying.
type Client struct {
	dial func() (io.ReadWriteCloser, error)
	seq  atomic.Uint64
}

// NewClient connects to the herdr socket at path, using a named pipe on
// Windows and a Unix socket elsewhere.
func NewClient(path string) *Client {
	return &Client{dial: func() (io.ReadWriteCloser, error) {
		if runtime.GOOS == "windows" {
			return os.OpenFile(`\\.\pipe\`+path, os.O_RDWR, 0)
		}
		return net.DialTimeout("unix", path, callTimeout)
	}}
}

// DefaultSocketPath mirrors herdr's own resolution for the default session.
func DefaultSocketPath() string {
	if p := os.Getenv("HERDR_SOCKET_PATH"); p != "" {
		return p
	}
	if s := os.Getenv("HERDR_SESSION"); s != "" {
		return filepath.Join(configDir(), "herdr", "sessions", s, "herdr.sock")
	}
	return filepath.Join(configDir(), "herdr", "herdr.sock")
}

func configDir() string {
	if runtime.GOOS != "windows" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".config")
		}
	}
	dir, _ := os.UserConfigDir()
	return dir
}

type wireAgent struct {
	Agent        *string `json:"agent"`
	AgentSession *struct {
		Value string `json:"value"`
	} `json:"agent_session"`
	AgentStatus string  `json:"agent_status"`
	Cwd         *string `json:"cwd"`
	PaneID      string  `json:"pane_id"`
	WorkspaceID string  `json:"workspace_id"`
	TerminalID  string  `json:"terminal_id"`
	Focused     bool    `json:"focused"`
}

func (c *Client) ListAgents() ([]Pane, error) {
	var res struct {
		Agents []wireAgent `json:"agents"`
	}
	if err := c.call("agent.list", struct{}{}, &res); err != nil {
		return nil, err
	}
	panes := make([]Pane, 0, len(res.Agents))
	for _, a := range res.Agents {
		p := Pane{PaneID: a.PaneID, WorkspaceID: a.WorkspaceID, TerminalID: a.TerminalID, Status: a.AgentStatus, Focused: a.Focused}
		if a.Agent != nil {
			p.Agent = *a.Agent
		}
		if a.AgentSession != nil {
			p.SessionID = a.AgentSession.Value
		}
		if a.Cwd != nil {
			p.Cwd = *a.Cwd
		}
		panes = append(panes, p)
	}
	return panes, nil
}

// Read returns the last lines of the agent's pane as plain text.
func (c *Client) Read(target string, lines int) (string, error) {
	var res struct {
		Read struct {
			Text string `json:"text"`
		} `json:"read"`
	}
	params := map[string]any{"target": target, "source": "recent", "lines": lines, "strip_ansi": true}
	if err := c.call("agent.read", params, &res); err != nil {
		return "", err
	}
	return res.Read.Text, nil
}

// SendLine types text into the pane and presses Enter. Success only means
// herdr queued the input, not that the agent acted on it.
func (c *Client) SendLine(paneID, text string) error {
	return c.call("pane.send_input", map[string]any{"pane_id": paneID, "text": text, "keys": []string{"enter"}}, nil)
}

// Focus brings the agent's pane to the front in herdr.
func (c *Client) Focus(target string) error {
	return c.call("agent.focus", map[string]any{"target": target}, nil)
}

func (c *Client) call(method string, params, out any) error {
	type result struct {
		raw []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		raw, err := c.roundTrip(method, params)
		done <- result{raw, err}
	}()
	var r result
	select {
	case r = <-done:
	case <-time.After(callTimeout):
		return fmt.Errorf("herdr %s: timed out after %v", method, callTimeout)
	}
	if r.err != nil {
		return fmt.Errorf("herdr %s: %w", method, r.err)
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(r.raw, &resp); err != nil {
		return fmt.Errorf("herdr %s: bad response: %w", method, err)
	}
	if resp.Error != nil {
		return fmt.Errorf("herdr %s: %s (%s)", method, resp.Error.Message, resp.Error.Code)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(resp.Result, out)
}

func (c *Client) roundTrip(method string, params any) ([]byte, error) {
	req, err := json.Marshal(map[string]any{
		"id":     "bah-" + strconv.FormatUint(c.seq.Add(1), 10),
		"method": method,
		"params": params,
	})
	if err != nil {
		return nil, err
	}
	// A pipe instance can be briefly unavailable right after the previous
	// connection closes, so dialing is retried a few times.
	var conn io.ReadWriteCloser
	for attempt := 0; attempt < 3; attempt++ {
		if conn, err = c.dial(); err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return nil, err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && !(errors.Is(err, io.EOF) && len(line) > 0) {
		return nil, err
	}
	return line, nil
}

// Link pairs each Claude session with the herdr pane running it: first by
// the session id herdr reports, then by working directory when exactly one
// unclaimed Claude pane sits there (herdr's id goes stale after a resume).
func Link(agents []claude.Agent, panes []Pane) map[string]Pane {
	links := map[string]Pane{}
	claimed := map[string]bool{}
	for _, a := range agents {
		for _, p := range panes {
			if p.SessionID != "" && p.SessionID == a.ID {
				links[a.ID] = p
				claimed[p.PaneID] = true
			}
		}
	}
	for _, a := range agents {
		if _, ok := links[a.ID]; ok || a.Cwd == "" {
			continue
		}
		var match []Pane
		for _, p := range panes {
			if p.Agent == "claude" && !claimed[p.PaneID] && strings.EqualFold(p.Cwd, a.Cwd) {
				match = append(match, p)
			}
		}
		if len(match) == 1 {
			links[a.ID] = match[0]
			claimed[match[0].PaneID] = true
		}
	}
	return links
}

// StartAgent runs argv as a new agent in dir and returns its pane. It joins
// the workspace of an agent already working in dir, split to the right;
// otherwise it creates a workspace for the project and runs argv in its
// root pane.
func (c *Client) StartAgent(dir string, argv []string, existing []Pane) (string, error) {
	name := lastElem(dir)
	for _, p := range existing {
		if p.WorkspaceID == "" || !strings.EqualFold(p.Cwd, dir) {
			continue
		}
		var res struct {
			Agent struct {
				PaneID string `json:"pane_id"`
			} `json:"agent"`
		}
		params := map[string]any{"name": name, "cwd": dir, "argv": argv, "workspace_id": p.WorkspaceID, "split": "right"}
		if err := c.call("agent.start", params, &res); err != nil {
			return "", err
		}
		return res.Agent.PaneID, nil
	}
	var res struct {
		RootPane struct {
			PaneID string `json:"pane_id"`
		} `json:"root_pane"`
	}
	if err := c.call("workspace.create", map[string]any{"cwd": dir, "label": name}, &res); err != nil {
		return "", err
	}
	if err := c.SendLine(res.RootPane.PaneID, strings.Join(argv, " ")); err != nil {
		return "", err
	}
	return res.RootPane.PaneID, nil
}

// Placeholders turns Claude panes that no transcript claimed yet (an agent
// just started from the office, before its first message) into agents, so
// they show up right away. Each one is added to links under its id.
func Placeholders(panes []Pane, links map[string]Pane) []claude.Agent {
	claimed := map[string]bool{}
	for _, p := range links {
		claimed[p.PaneID] = true
	}
	var out []claude.Agent
	for _, p := range panes {
		if p.Agent != "claude" || claimed[p.PaneID] {
			continue
		}
		dir := strings.TrimRight(p.Cwd, `\/`)
		parent := ""
		if i := strings.LastIndexAny(dir, `\/`); i >= 0 {
			parent = dir[:i]
		}
		a := claude.Agent{
			ID:      "herdr:" + p.PaneID,
			Project: lastElem(dir),
			Group:   lastElem(parent),
			Cwd:     p.Cwd,
			State:   claude.StateWaiting,
		}
		if a.Group == "" {
			a.Group = "other"
		}
		if p.Status == "working" || p.Status == "blocked" {
			a.State = claude.StateTool
		}
		links[a.ID] = p
		out = append(out, a)
	}
	return out
}

func lastElem(p string) string {
	p = strings.TrimRight(p, `\/`)
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// Annotate returns a copy of agents with Blocked set for every agent whose
// herdr pane reports it is waiting on a permission prompt.
func Annotate(agents []claude.Agent, links map[string]Pane) []claude.Agent {
	out := make([]claude.Agent, len(agents))
	for i, a := range agents {
		a.Blocked = links[a.ID].Status == "blocked"
		out[i] = a
	}
	return out
}

// ClosePane closes a pane, ending whatever runs in it.
func (c *Client) ClosePane(paneID string) error {
	return c.call("pane.close", map[string]any{"pane_id": paneID}, nil)
}
