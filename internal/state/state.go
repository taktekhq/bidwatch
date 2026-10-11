// Package state keeps the small on-disk record of which listings bidwatch
// has already posted, so a run only announces what's new.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// State maps a listing's ID (its URL) to when bidwatch first saw it.
type State struct {
	Seen map[string]time.Time `json:"seen"`
	path string
}

// Load reads path, or returns an empty State if the file doesn't exist yet
// (the first run of a brand-new install).
func Load(path string) (*State, error) {
	s := &State{Seen: map[string]time.Time{}, path: path}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	s.path = path
	if s.Seen == nil {
		s.Seen = map[string]time.Time{}
	}
	return s, nil
}

// IsNew reports whether id has never been seen before.
func (s *State) IsNew(id string) bool {
	_, ok := s.Seen[id]
	return !ok
}

// Mark records id as seen now, if it wasn't already recorded.
func (s *State) Mark(id string, now time.Time) {
	if _, ok := s.Seen[id]; !ok {
		s.Seen[id] = now
	}
}

// Save writes the state atomically (write to a temp file, then rename) so a
// run that's killed mid-write never leaves a half-written state file.
func (s *State) Save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
