package statusline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// BackupSuffix names the copy Install keeps of the user's settings file. Only
// the first install writes it: a later install would back up a file that
// BitAgentsHub already changed, and the point is to never lose the original.
const BackupSuffix = ".bah-backup"

const (
	stateFile = "statusline.json"
	key       = "statusLine"
)

// installState is what Install remembers so Run can chain the status line it
// replaced and Uninstall can put it back.
type installState struct {
	Command  string          `json:"command"`            // the one BitAgentsHub set
	Previous json.RawMessage `json:"previous,omitempty"` // absent when there was none
}

// Install points Claude Code's status line at command. The settings file keeps
// every other key, in order and with its value as written; only statusLine
// changes or is appended.
func Install(settingsPath, stateDir, command string) error {
	pairs, original, mode, err := readSettings(settingsPath)
	if err != nil {
		return err
	}
	st, _, err := readState(stateDir)
	if err != nil {
		return err
	}
	// Installing again must not remember BitAgentsHub itself as the user's
	// status line; a status line the user set since then is theirs.
	previous := st.Previous
	current, has := get(pairs, key)
	switch {
	case !has:
		previous = nil
	case !runs(current, command, st.Command):
		previous = current
	}

	value, err := marshal(struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}{"command", command})
	if err != nil {
		return err
	}
	out, err := encode(set(pairs, key, value))
	if err != nil {
		return err
	}
	if original != nil {
		if err := backup(settingsPath+BackupSuffix, original); err != nil {
			return err
		}
	}
	if err := writeState(stateDir, installState{Command: command, Previous: previous}); err != nil {
		return err
	}
	return writeAtomic(settingsPath, out, mode)
}

// Uninstall puts back the status line Install replaced, or removes the key
// when there was none. A status line the user set after installing stays.
func Uninstall(settingsPath, stateDir string) error {
	st, installed, err := readState(stateDir)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("statusline is not installed: %s not found", filepath.Join(stateDir, stateFile))
	}
	pairs, _, mode, err := readSettings(settingsPath)
	if err != nil {
		return err
	}
	if current, has := get(pairs, key); has && runs(current, st.Command) {
		if st.Previous != nil {
			pairs = set(pairs, key, st.Previous)
		} else {
			pairs = remove(pairs, key)
		}
		out, err := encode(pairs)
		if err != nil {
			return err
		}
		if err := writeAtomic(settingsPath, out, mode); err != nil {
			return err
		}
	}
	return os.Remove(filepath.Join(stateDir, stateFile))
}

// previousCommand is the status line command Install replaced, if any.
func previousCommand(stateDir string) (string, error) {
	st, installed, err := readState(stateDir)
	if err != nil || !installed || st.Previous == nil {
		return "", err
	}
	var prev struct{ Type, Command string }
	if json.Unmarshal(st.Previous, &prev) != nil || prev.Type != "command" {
		return "", nil
	}
	return prev.Command, nil
}

// runs reports whether a statusLine value runs one of commands.
func runs(statusLine json.RawMessage, commands ...string) bool {
	var v struct{ Command string }
	if json.Unmarshal(statusLine, &v) != nil {
		return false
	}
	for _, c := range commands {
		if c != "" && v.Command == c {
			return true
		}
	}
	return false
}

func readState(stateDir string) (installState, bool, error) {
	var st installState
	data, err := os.ReadFile(filepath.Join(stateDir, stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return st, false, nil
	}
	if err != nil {
		return st, false, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, false, fmt.Errorf("%s: %w", filepath.Join(stateDir, stateFile), err)
	}
	return st, true, nil
}

func writeState(stateDir string, st installState) error {
	data, err := marshal(st)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(stateDir, stateFile), append(data, '\n'), 0o600)
}

// readSettings returns the top-level keys in file order, the file as it was
// (nil when missing) and its mode. A missing file reads as {}.
func readSettings(path string) ([]pair, []byte, fs.FileMode, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, 0o600, nil
	}
	if err != nil {
		return nil, nil, 0, err
	}
	mode := fs.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	pairs, err := decode(data)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s: %w", path, err)
	}
	return pairs, data, mode, nil
}

// pair is one top-level key with its value kept exactly as written.
type pair struct {
	key   string
	value json.RawMessage
}

func decode(data []byte) ([]pair, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("settings are not a JSON object")
	}
	var pairs []pair
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		pairs = append(pairs, pair{key: tok.(string), value: value})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after the settings object")
	}
	return pairs, nil
}

// encode writes the object with two-space indentation; values are only
// re-indented, never re-encoded.
func encode(pairs []pair) ([]byte, error) {
	var flat bytes.Buffer
	flat.WriteByte('{')
	for i, p := range pairs {
		if i > 0 {
			flat.WriteByte(',')
		}
		k, err := marshal(p.key)
		if err != nil {
			return nil, err
		}
		flat.Write(k)
		flat.WriteByte(':')
		flat.Write(p.value)
	}
	flat.WriteByte('}')
	var out bytes.Buffer
	if err := json.Indent(&out, flat.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func get(pairs []pair, k string) (json.RawMessage, bool) {
	for _, p := range pairs {
		if p.key == k {
			return p.value, true
		}
	}
	return nil, false
}

// set replaces the key where it first appears, or appends it.
func set(pairs []pair, k string, v json.RawMessage) []pair {
	var out []pair
	done := false
	for _, p := range pairs {
		switch {
		case p.key != k:
			out = append(out, p)
		case !done:
			out = append(out, pair{k, v})
			done = true
		}
	}
	if !done {
		out = append(out, pair{k, v})
	}
	return out
}

func remove(pairs []pair, k string) []pair {
	var out []pair
	for _, p := range pairs {
		if p.key != k {
			out = append(out, p)
		}
	}
	return out
}

// marshal is json.Marshal without HTML escaping, so a command such as
// "a && b" stays readable in the user's settings.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// backup keeps the first copy only; see BackupSuffix.
func backup(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	return f.Close()
}

// writeAtomic replaces path in one rename, so Claude Code never reads a half
// written file.
func writeAtomic(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}
