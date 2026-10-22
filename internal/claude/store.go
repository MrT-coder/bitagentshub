// Package claude reads Claude Code session transcripts from disk and turns
// them into per-agent state and token usage. It never talks to the network.
package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Usage is a token tally. Counted excludes cache reads because they are
// billed at a fraction of normal input and would dwarf everything else.
type Usage struct {
	Input       int64
	Output      int64
	CacheCreate int64
	CacheRead   int64
}

func (u Usage) Counted() int64 { return u.Input + u.Output + u.CacheCreate }

func (u *Usage) add(o Usage) {
	u.Input += o.Input
	u.Output += o.Output
	u.CacheCreate += o.CacheCreate
	u.CacheRead += o.CacheRead
}

func (u Usage) sub(o Usage) Usage {
	return Usage{u.Input - o.Input, u.Output - o.Output, u.CacheCreate - o.CacheCreate, u.CacheRead - o.CacheRead}
}

type State int

const (
	StateSleeping State = iota
	StateWaiting
	StateThinking
	StateTool
)

func (s State) String() string {
	switch s {
	case StateWaiting:
		return "waiting"
	case StateThinking:
		return "thinking"
	case StateTool:
		return "tool"
	default:
		return "asleep"
	}
}

// Working reports whether the agent is actively doing something.
func (s State) Working() bool { return s == StateThinking || s == StateTool }

// Activity is the visible kind of work behind a tool call.
type Activity int

const (
	ActivityNone Activity = iota
	ActivityTyping
	ActivityReading
	ActivityRunning
)

func (a Activity) String() string {
	switch a {
	case ActivityTyping:
		return "typing"
	case ActivityReading:
		return "reading"
	case ActivityRunning:
		return "running"
	}
	return "none"
}

func activityOf(state State, tool string) Activity {
	if state != StateTool {
		return ActivityNone
	}
	switch tool {
	case "Read", "Grep", "Glob", "WebFetch", "WebSearch":
		return ActivityReading
	case "Bash", "PowerShell":
		return ActivityRunning
	}
	return ActivityTyping
}

// lastToolName returns the name of the last tool_use block in a message's
// content, or "" when there is none or the content is not a block list.
func lastToolName(content json.RawMessage) string {
	if len(content) == 0 || content[0] != '[' {
		return ""
	}
	var blocks []struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return ""
	}
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Type == "tool_use" {
			return blocks[i].Name
		}
	}
	return ""
}

type Options struct {
	Location     *time.Location
	WeekStart    time.Weekday
	ActiveWindow time.Duration // sessions touched within this window are shown
	IdleAfter    time.Duration // silence longer than this means sleeping
	Horizon      time.Duration // files older than this are never read
	Rediscover   time.Duration // how often to walk the tree for new files
	// ColdBytes caps how much of the transcripts not read yet and not active
	// one poll reads, so the first poll does not wait for a week of them.
	ColdBytes int64
}

func (o Options) withDefaults() Options {
	if o.Location == nil {
		o.Location = time.Local
	}
	if o.ActiveWindow == 0 {
		o.ActiveWindow = 30 * time.Minute
	}
	if o.IdleAfter == 0 {
		o.IdleAfter = 5 * time.Minute
	}
	if o.Horizon == 0 {
		o.Horizon = 8 * 24 * time.Hour
	}
	if o.Rediscover == 0 {
		o.Rediscover = 10 * time.Second
	}
	if o.ColdBytes == 0 {
		o.ColdBytes = 32 << 20
	}
	return o
}

// Agent is an immutable view of one session for rendering.
type Agent struct {
	ID              string
	Project         string
	Group           string // parent folder of the working directory, e.g. "acme-company"
	Cwd             string
	State           State
	LastActivity    time.Time
	Today           Usage
	Session         Usage
	ActiveSubagents int
	Tool            string   // tool being run while State is StateTool
	Activity        Activity // what the tool looks like from the outside
	Blocked         bool     // waiting on a permission prompt; set from herdr
	Cheer           bool     // a task of its project was just checked off; set by the office
	Done            bool     // finished work nobody looked at yet; set by the office
}

// Snapshot is an immutable view of everything the store knows at one instant.
type Snapshot struct {
	Agents []Agent // active sessions, most recent first
	Today  Usage   // all sessions, since local midnight
	Week   Usage   // all sessions, since the configured week start
	Files  int     // transcript files being tracked
	// Pending are tracked files not read yet: until it is 0, Today and Week
	// are still growing.
	Pending int

	// Projects are the working directories of every session read, active or
	// not, most recently used first.
	Projects []ProjectRef
}

// ProjectRef is a directory an agent has worked in.
type ProjectRef struct {
	Cwd          string
	LastActivity time.Time
}

const bucketSize = 15 * 60 // seconds; aligns with every real timezone offset

type message struct {
	usage  Usage
	bucket int64
}

type session struct {
	id       string
	project  string
	group    string
	cwd      string
	buckets  map[int64]Usage
	total    Usage
	state    State
	tool     string               // tool of the pending tool_use, if any
	mainSeen time.Time            // mtime of the main transcript
	subs     map[string]time.Time // subagent path -> mtime
}

type file struct {
	offset    int64
	size      int64
	mtime     time.Time
	sessionID string
	subagent  bool
	read      bool // read at least once
}

// Store incrementally tails transcripts under root. It is not safe for
// concurrent use; callers poll it from a single goroutine.
type Store struct {
	root         string
	opts         Options
	files        map[string]*file
	sessions     map[string]*session
	messages     map[string]message
	lastDiscover time.Time
}

func NewStore(root string, opts Options) *Store {
	return &Store{
		root:     root,
		opts:     opts.withDefaults(),
		files:    map[string]*file{},
		sessions: map[string]*session{},
		messages: map[string]message{},
	}
}

// Poll picks up new lines from changed transcripts and returns a snapshot.
// Unchanged files cost a single stat. Transcripts not read yet and not
// active are read newest first, up to ColdBytes per poll.
func (s *Store) Poll(now time.Time) Snapshot {
	full := s.lastDiscover.IsZero() || now.Sub(s.lastDiscover) >= s.opts.Rediscover
	if full {
		s.discover(now)
		s.lastDiscover = now
	}
	type unread struct {
		path string
		f    *file
		info fs.FileInfo
	}
	var cold []unread
	for path, f := range s.files {
		// Cold files are only re-checked on the slow discovery cycle.
		if f.read && !full && now.Sub(f.mtime) > s.opts.ActiveWindow {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if f.read && info.Size() == f.size && info.ModTime().Equal(f.mtime) {
			continue
		}
		if !f.read && now.Sub(info.ModTime()) > s.opts.ActiveWindow {
			cold = append(cold, unread{path, f, info})
			continue
		}
		s.read(path, f, info)
	}
	sort.Slice(cold, func(i, j int) bool { return cold[i].info.ModTime().After(cold[j].info.ModTime()) })
	budget := s.opts.ColdBytes
	for _, c := range cold {
		if budget <= 0 {
			break
		}
		s.read(c.path, c.f, c.info)
		budget -= c.info.Size()
	}
	return s.snapshot(now)
}

func (s *Store) read(path string, f *file, info fs.FileInfo) {
	f.mtime = info.ModTime()
	s.tail(path, f, info.Size())
	f.read = true
}

func (s *Store) discover(now time.Time) {
	_ = filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "tool-results" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		if _, ok := s.files[path]; ok {
			return nil
		}
		info, err := d.Info()
		if err != nil || now.Sub(info.ModTime()) > s.opts.Horizon {
			return nil
		}
		s.files[path] = s.classify(path)
		return nil
	})
}

// classify maps <project>/<session>.jsonl and
// <project>/<session>/subagents/agent-*.jsonl to their owning session.
func (s *Store) classify(path string) *file {
	rel, _ := filepath.Rel(s.root, path)
	parts := strings.Split(filepath.ToSlash(rel), "/")
	f := &file{}
	if len(parts) >= 4 && parts[len(parts)-2] == "subagents" {
		f.subagent = true
		f.sessionID = parts[len(parts)-3]
	} else {
		f.sessionID = strings.TrimSuffix(parts[len(parts)-1], ".jsonl")
	}
	if _, ok := s.sessions[f.sessionID]; !ok {
		s.sessions[f.sessionID] = &session{
			id:      f.sessionID,
			project: projectFromSlug(parts[0]),
			group:   "other",
			buckets: map[int64]Usage{},
			subs:    map[string]time.Time{},
		}
	}
	return f
}

func (s *Store) tail(path string, f *file, size int64) {
	if size < f.offset { // truncated or rewritten
		f.offset = 0
	}
	fh, err := os.Open(path)
	if err != nil {
		return
	}
	defer fh.Close()
	if _, err := fh.Seek(f.offset, io.SeekStart); err != nil {
		return
	}
	sess := s.sessions[f.sessionID]
	r := bufio.NewReaderSize(fh, 256*1024)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			break // EOF; an unterminated tail is re-read next time
		}
		f.offset += int64(len(line))
		s.apply(sess, f, line)
	}
	f.size = size
	if f.subagent {
		sess.subs[path] = f.mtime
	} else {
		sess.mainSeen = f.mtime
	}
}

var (
	markAssistant = []byte(`"type":"assistant"`)
	markUser      = []byte(`"type":"user"`)
)

type rawLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Cwd       string    `json:"cwd"`
	Message   struct {
		ID         string          `json:"id"`
		StopReason *string         `json:"stop_reason"`
		Content    json.RawMessage `json:"content"` // decoded lazily; may not be an array
		Usage      *struct {
			Input       int64 `json:"input_tokens"`
			Output      int64 `json:"output_tokens"`
			CacheCreate int64 `json:"cache_creation_input_tokens"`
			CacheRead   int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

func (s *Store) apply(sess *session, f *file, line []byte) {
	// Cheap prefilter: most lines are attachments, metadata or huge tool
	// results we never need to decode.
	if !bytes.Contains(line, markAssistant) {
		// User lines carry no usage and are often large tool results, so
		// they only update state and are never decoded.
		if !f.subagent && bytes.Contains(line, markUser) {
			sess.state = StateThinking
		}
		return
	}
	var raw rawLine
	if json.Unmarshal(line, &raw) != nil {
		return
	}
	if raw.Cwd != "" && !f.subagent {
		sess.cwd = raw.Cwd
		sess.project = baseName(raw.Cwd)
		if g := baseName(parentDir(raw.Cwd)); g != "" {
			sess.group = g
		}
	}
	if raw.Type != "assistant" {
		return
	}
	if !f.subagent {
		sess.state = stateFromStop(raw.Message.StopReason)
		switch {
		case sess.state != StateTool:
			sess.tool = ""
		case lastToolName(raw.Message.Content) != "":
			// Only the line carrying the tool_use block names it; sibling
			// lines of the same message keep the name already seen.
			sess.tool = lastToolName(raw.Message.Content)
		}
	}
	if u := raw.Message.Usage; u != nil && raw.Message.ID != "" {
		s.record(sess, raw.Message.ID, raw.Timestamp, Usage{u.Input, u.Output, u.CacheCreate, u.CacheRead})
	}
}

// record adds a message's usage once. Claude Code writes one line per content
// block, each repeating the message usage, so only growth is counted.
func (s *Store) record(sess *session, id string, ts time.Time, u Usage) {
	prev, seen := s.messages[id]
	bucket := prev.bucket
	if !seen {
		bucket = ts.Unix() / bucketSize * bucketSize
	}
	if seen && u.Counted() <= prev.usage.Counted() {
		return
	}
	delta := u.sub(prev.usage)
	s.messages[id] = message{usage: u, bucket: bucket}
	b := sess.buckets[bucket]
	b.add(delta)
	sess.buckets[bucket] = b
	sess.total.add(delta)
}

func stateFromStop(stop *string) State {
	if stop == nil {
		return StateThinking // still streaming
	}
	if *stop == "tool_use" {
		return StateTool
	}
	return StateWaiting
}

func (s *Store) snapshot(now time.Time) Snapshot {
	local := now.In(s.opts.Location)
	dayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, s.opts.Location)
	back := (int(local.Weekday()) - int(s.opts.WeekStart) + 7) % 7
	weekStart := dayStart.AddDate(0, 0, -back)

	snap := Snapshot{Files: len(s.files)}
	for _, f := range s.files {
		if !f.read {
			snap.Pending++
		}
	}
	projects := map[string]ProjectRef{}
	for _, sess := range s.sessions {
		var today Usage
		for b, u := range sess.buckets {
			t := time.Unix(b, 0)
			if !t.Before(dayStart) {
				today.add(u)
			}
			if !t.Before(weekStart) {
				snap.Week.add(u)
			}
		}
		snap.Today.add(today)

		last := sess.mainSeen
		active := 0
		for _, m := range sess.subs {
			if m.After(last) {
				last = m
			}
			if now.Sub(m) <= time.Minute {
				active++
			}
		}
		if sess.cwd != "" && !last.IsZero() {
			key := strings.ToLower(sess.cwd)
			if prev, ok := projects[key]; !ok || last.After(prev.LastActivity) {
				projects[key] = ProjectRef{Cwd: sess.cwd, LastActivity: last}
			}
		}
		if last.IsZero() || now.Sub(last) > s.opts.ActiveWindow {
			continue
		}
		state := sess.state
		if now.Sub(last) > s.opts.IdleAfter {
			state = StateSleeping
		}
		snap.Agents = append(snap.Agents, Agent{
			ID:              sess.id,
			Project:         sess.project,
			Group:           sess.group,
			Cwd:             sess.cwd,
			State:           state,
			LastActivity:    last,
			Today:           today,
			Session:         sess.total,
			ActiveSubagents: active,
			Tool:            sess.tool,
			Activity:        activityOf(state, sess.tool),
		})
	}
	for _, p := range projects {
		snap.Projects = append(snap.Projects, p)
	}
	sort.Slice(snap.Projects, func(i, j int) bool {
		return snap.Projects[i].LastActivity.After(snap.Projects[j].LastActivity)
	})
	sort.Slice(snap.Agents, func(i, j int) bool {
		return snap.Agents[i].LastActivity.After(snap.Agents[j].LastActivity)
	})
	return snap
}

// projectFromSlug is the fallback when no cwd was seen: Claude Code names
// project folders after the path with separators replaced by dashes.
func projectFromSlug(slug string) string {
	if i := strings.LastIndex(slug, "-"); i >= 0 && i < len(slug)-1 {
		return slug[i+1:]
	}
	return slug
}

func baseName(p string) string {
	p = strings.TrimRight(p, `\/`)
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func parentDir(p string) string {
	p = strings.TrimRight(p, `\/`)
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[:i]
	}
	return ""
}
