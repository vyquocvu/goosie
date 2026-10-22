// Package history keeps a profile's global visit log, capped and persisted to
// disk.
package history

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MaxEntries caps the log: a hostile or busy session must not grow the file
// without bound.
const MaxEntries = 5000

// Entry is one recorded visit.
type Entry struct {
	URL   string    `json:"url"`
	Title string    `json:"title,omitempty"`
	At    time.Time `json:"at"`
}

// Store is a capped, oldest-first visit log. Record appends; Save persists.
type Store struct {
	mu      sync.Mutex
	path    string
	entries []Entry
}

// Load reads the history file. A missing file is an empty store, not an
// error; a file that exists but cannot be parsed is an error so a corrupt
// store is never overwritten on the next Save.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &Store{path: path}, nil
		}
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if len(entries) > MaxEntries {
		entries = entries[len(entries)-MaxEntries:]
	}
	return &Store{path: path, entries: entries}, nil
}

// Record appends one visit unless it repeats the immediately preceding entry.
// The log keeps only the newest MaxEntries visits.
func (s *Store) Record(url, title string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.entries); n > 0 && s.entries[n-1].URL == url {
		return
	}
	s.entries = append(s.entries, Entry{URL: url, Title: title, At: time.Now()})
	if len(s.entries) > MaxEntries {
		s.entries = s.entries[len(s.entries)-MaxEntries:]
	}
}

// Entries returns a copy of the log, oldest first.
func (s *Store) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

// DeleteEntry removes the single visit for url recorded at exactly at and
// reports whether it existed. Like Record, it only mutates memory; the
// caller decides when to persist with Save. Timestamps round-trip through
// RFC3339Nano encoding and JSON without loss of precision.
func (s *Store) DeleteEntry(url string, at time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.entries {
		if e.URL == url && e.At.Equal(at) {
			s.entries = append(s.entries[:i:i], s.entries[i+1:]...)
			return true
		}
	}
	return false
}

// Clear empties the log in memory. The caller decides when to persist with
// Save.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
}

// Search returns visits whose URL or title contains query case-insensitively,
// oldest first. An empty query returns the full log, matching Entries.
func (s *Store) Search(query string) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		if q == "" || strings.Contains(strings.ToLower(e.URL), q) || strings.Contains(strings.ToLower(e.Title), q) {
			out = append(out, e)
		}
	}
	return out
}

// Save writes the log atomically: the bytes land in a temp file first and
// rename in, so a truncated write never replaces a good file.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, data, ".history-*")
}

func writeFileAtomic(path string, data []byte, tempPattern string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, tempPattern)
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
	return os.Rename(tmp.Name(), path)
}
