package rest

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Load reads the day saved at path; another day's record, or none, is an
// empty day for now.
func Load(path string, now time.Time) (Day, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Day{Date: Date(now)}, nil
	}
	if err != nil {
		return Day{}, err
	}
	var d Day
	if err := json.Unmarshal(b, &d); err != nil {
		return Day{}, err
	}
	if d.Date != Date(now) {
		return Day{Date: Date(now)}, nil
	}
	return d, nil
}

// Save writes d to path, replacing it whole.
func Save(path string, d Day) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
