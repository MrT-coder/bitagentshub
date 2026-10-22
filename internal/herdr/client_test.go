package herdr

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/MrT-coder/bitagentshub/internal/claude"
)

// fakeServer answers one request per connection, like herdr does, and
// records every request it receives.
type fakeServer struct {
	responses map[string]string // method -> raw result or error JSON
	requests  []map[string]any
}

func (f *fakeServer) dial() (io.ReadWriteCloser, error) {
	client, server := net.Pipe()
	go func() {
		defer server.Close()
		line, err := bufio.NewReader(server).ReadString('\n')
		if err != nil {
			return
		}
		var req map[string]any
		json.Unmarshal([]byte(line), &req)
		f.requests = append(f.requests, req)
		body := strings.ReplaceAll(f.responses[req["method"].(string)], "\n", "")
		io.WriteString(server, `{"id":"`+req["id"].(string)+`",`+body+"}\n")
	}()
	return client, nil
}

const agentList = `"result":{"type":"agent_list","agents":[
 {"agent":"claude","agent_session":{"value":"sess-1"},"agent_status":"working","cwd":"C:\\w\\a","pane_id":"w1:p1","terminal_id":"term_1","focused":false},
 {"agent":"codex","agent_session":null,"agent_status":"idle","cwd":"C:\\w\\b","pane_id":"w2:p1","terminal_id":"term_2","focused":true}]}`

func TestClient_ListsAgentPanesWithTheirClaudeSession(t *testing.T) {
	f := &fakeServer{responses: map[string]string{"agent.list": agentList}}
	panes, err := (&Client{dial: f.dial}).ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(panes) != 2 {
		t.Fatalf("panes = %d, want 2", len(panes))
	}
	want := Pane{PaneID: "w1:p1", TerminalID: "term_1", Agent: "claude", SessionID: "sess-1", Status: "working", Cwd: `C:\w\a`}
	if panes[0] != want {
		t.Errorf("pane = %+v, want %+v", panes[0], want)
	}
	if panes[1].SessionID != "" || !panes[1].Focused {
		t.Errorf("second pane = %+v, want no session and focused", panes[1])
	}
}

func TestClient_ReadsRecentPaneText(t *testing.T) {
	f := &fakeServer{responses: map[string]string{
		"agent.read": `"result":{"type":"pane_read","read":{"pane_id":"w1:p1","text":"hello\nworld"}}`,
	}}
	text, err := (&Client{dial: f.dial}).Read("term_1", 40)
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello\nworld" {
		t.Errorf("text = %q", text)
	}
	params := f.requests[0]["params"].(map[string]any)
	if params["target"] != "term_1" || params["source"] != "recent" || params["lines"].(float64) != 40 {
		t.Errorf("params = %v", params)
	}
}

func TestClient_SendLineTypesTextAndPressesEnter(t *testing.T) {
	f := &fakeServer{responses: map[string]string{"pane.send_input": `"result":{"type":"ok"}`}}
	if err := (&Client{dial: f.dial}).SendLine("w1:p1", "run the tests"); err != nil {
		t.Fatal(err)
	}
	params := f.requests[0]["params"].(map[string]any)
	if params["pane_id"] != "w1:p1" || params["text"] != "run the tests" {
		t.Errorf("params = %v", params)
	}
	if keys, _ := params["keys"].([]any); len(keys) != 1 || keys[0] != "enter" {
		t.Errorf("keys = %v, want [enter]", params["keys"])
	}
}

func TestClient_SurfacesServerErrors(t *testing.T) {
	f := &fakeServer{responses: map[string]string{
		"agent.focus": `"error":{"code":"agent_not_found","message":"agent target nope not found"}`,
	}}
	err := (&Client{dial: f.dial}).Focus("nope")
	if err == nil || !strings.Contains(err.Error(), "agent target nope not found") {
		t.Errorf("err = %v, want server message", err)
	}
}

func TestClient_RetriesTransientDialFailures(t *testing.T) {
	f := &fakeServer{responses: map[string]string{"agent.focus": `"result":{"type":"ok"}`}}
	fails := 1
	dial := func() (io.ReadWriteCloser, error) {
		if fails > 0 {
			fails--
			return nil, errors.New("pipe busy")
		}
		return f.dial()
	}
	if err := (&Client{dial: dial}).Focus("term_1"); err != nil {
		t.Errorf("err = %v, want success after retry", err)
	}
}

func TestLink_MatchesBySessionIDThenByUniqueWorkingDirectory(t *testing.T) {
	agents := []claude.Agent{
		{ID: "sess-1", Cwd: `C:\w\a`},
		{ID: "sess-stale", Cwd: `C:\W\B`}, // herdr still reports an older session id here
		{ID: "sess-3", Cwd: `C:\w\shared`},
		{ID: "sess-4", Cwd: `C:\w\none`},
	}
	panes := []Pane{
		{PaneID: "w1:p1", Agent: "claude", SessionID: "sess-1", Cwd: `C:\w\a`},
		{PaneID: "w2:p1", Agent: "claude", SessionID: "old", Cwd: `C:\w\b`},
		{PaneID: "w3:p1", Agent: "claude", Cwd: `C:\w\shared`},
		{PaneID: "w3:p2", Agent: "claude", Cwd: `C:\w\shared`},
		{PaneID: "w4:p1", Agent: "codex", Cwd: `C:\w\none`},
	}
	got := Link(agents, panes)
	want := map[string]string{"sess-1": "w1:p1", "sess-stale": "w2:p1"}
	if len(got) != len(want) {
		t.Errorf("links = %v, want %v", got, want)
	}
	for id, pane := range want {
		if got[id].PaneID != pane {
			t.Errorf("link[%s] = %q, want %q", id, got[id].PaneID, pane)
		}
	}
}

func TestAnnotate_MarksAgentsWhosePaneIsBlockedOnAPermission(t *testing.T) {
	agents := []claude.Agent{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	links := map[string]Pane{"a": {Status: "blocked"}, "b": {Status: "working"}}
	got := Annotate(agents, links)
	if !got[0].Blocked || got[1].Blocked || got[2].Blocked {
		t.Errorf("blocked = %v %v %v, want true false false", got[0].Blocked, got[1].Blocked, got[2].Blocked)
	}
	if agents[0].Blocked {
		t.Error("Annotate must not modify the snapshot it was given")
	}
}

func TestClient_StartsAnAgentNextToAnExistingOneInTheSameDirectory(t *testing.T) {
	f := &fakeServer{responses: map[string]string{
		"agent.start": `"result":{"type":"agent_started","agent":{"pane_id":"w1:p2"},"argv":["claude"]}`,
	}}
	c := &Client{dial: f.dial}
	existing := []Pane{{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", Cwd: `C:\w\app`}}
	pane, err := c.StartAgent(`C:\W\app`, []string{"claude"}, existing)
	if err != nil || pane != "w1:p2" {
		t.Fatalf("pane=%q err=%v", pane, err)
	}
	p := f.requests[0]["params"].(map[string]any)
	if p["workspace_id"] != "w1" || p["split"] != "right" || p["cwd"] != `C:\W\app` || p["name"] != "app" {
		t.Errorf("agent.start params = %v", p)
	}
}

func TestClient_StartsAnAgentInANewWorkspaceForANewProject(t *testing.T) {
	f := &fakeServer{responses: map[string]string{
		"workspace.create": `"result":{"type":"workspace_created","workspace":{"workspace_id":"w9"},"root_pane":{"pane_id":"w9:p1"}}`,
		"pane.send_input":  `"result":{"type":"ok"}`,
	}}
	pane, err := (&Client{dial: f.dial}).StartAgent(`C:\w\new-app`, []string{"claude"}, nil)
	if err != nil || pane != "w9:p1" {
		t.Fatalf("pane=%q err=%v", pane, err)
	}
	ws := f.requests[0]["params"].(map[string]any)
	if f.requests[0]["method"] != "workspace.create" || ws["cwd"] != `C:\w\new-app` || ws["label"] != "new-app" {
		t.Errorf("workspace.create = %v", f.requests[0])
	}
	run := f.requests[1]["params"].(map[string]any)
	if f.requests[1]["method"] != "pane.send_input" || run["pane_id"] != "w9:p1" || run["text"] != "claude" {
		t.Errorf("run command = %v", f.requests[1])
	}
}

func TestPlaceholders_ShowFreshAgentPanesThatHaveNoTranscriptYet(t *testing.T) {
	agents := []claude.Agent{{ID: "s1", Cwd: `C:\w\a`}}
	panes := []Pane{
		{PaneID: "w1:p1", Agent: "claude", SessionID: "s1", Cwd: `C:\w\a`},
		{PaneID: "w2:p1", Agent: "claude", Status: "working", Cwd: `C:\w\grp\fresh`},
		{PaneID: "w3:p1", Agent: "", Cwd: `C:\w\shell`},
	}
	links := Link(agents, panes)

	extra := Placeholders(panes, links)

	if len(extra) != 1 {
		t.Fatalf("placeholders = %+v, want only the unclaimed claude pane", extra)
	}
	a := extra[0]
	if a.ID != "herdr:w2:p1" || a.Project != "fresh" || a.Group != "grp" || a.State != claude.StateTool {
		t.Errorf("placeholder = %+v", a)
	}
	if links[a.ID].PaneID != "w2:p1" {
		t.Errorf("placeholder is not linked to its pane: %v", links)
	}
}

func TestClient_ClosesAPane(t *testing.T) {
	f := &fakeServer{responses: map[string]string{"pane.close": `"result":{"type":"ok"}`}}
	if err := (&Client{dial: f.dial}).ClosePane("w1:p1"); err != nil {
		t.Fatal(err)
	}
	if f.requests[0]["method"] != "pane.close" || f.requests[0]["params"].(map[string]any)["pane_id"] != "w1:p1" {
		t.Errorf("request = %v", f.requests[0])
	}
}
