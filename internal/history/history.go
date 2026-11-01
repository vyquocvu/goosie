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

// Transition names how a visit began, matching the navigation categories the
// management UI reports.
type Transition string

const (
	TransitionTyped  Transition = "typed"
	TransitionLink   Transition = "link"
	TransitionReload Transition = "reload"
	TransitionOther  Transition = "other"
)

// Entry is one recorded visit. Visits counts every visit recorded for URL up
// to and including this one, so trimming older entries never rewrites the
// history a later visit reports.
type Entry struct {
	URL        string     `json:"url"`
	Title      string     `json:"title,omitempty"`
	At         time.Time  `json:"at"`
	Visits     int        `json:"visits,omitempty"`
	Transition Transition `json:"transition,omitempty"`
}

// Store is a capped, oldest-first visit log. Record appends; Save persists.
type Store struct {
	mu      sync.Mutex
	path    string
	entries []Entry
	counts  map[string]int
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
	counts := make(map[string]int, len(entries))
	for _, e := range entries {
		counts[e.URL]++
		if e.Visits > counts[e.URL] {
			counts[e.URL] = e.Visits
		}
	}
	return &Store{path: path, entries: entries, counts: counts}, nil
}

// Record appends one visit unless it repeats the immediately preceding entry.
// The log keeps only the newest MaxEntries visits.
func (s *Store) Record(url, title string) {
	s.RecordVisit(url, title, TransitionOther)
}

// RecordVisit records a visit with its navigation transition. A URL that
// continues the head of the log is not appended again: a reload labels that
// visit, and any other repeated navigation leaves it untouched (BR-045).
func (s *Store) RecordVisit(url, title string, transition Transition) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.counts == nil {
		s.counts = make(map[string]int)
	}
	if n := len(s.entries); n > 0 && s.entries[n-1].URL == url {
		if transition == TransitionReload {
			s.entries[n-1].Transition = TransitionReload
			if title != "" {
				s.entries[n-1].Title = title
			}
		}
		return
	}
	s.counts[url]++
	s.entries = append(s.entries, Entry{
		URL:        url,
		Title:      title,
		At:         time.Now(),
		Visits:     s.counts[url],
		Transition: transition,
	})
	if len(s.entries) > MaxEntries {
		s.entries = s.entries[len(s.entries)-MaxEntries:]
	}
}

// FrecencyScore ranks one URL for the omnibox: visit frequency multiplied by
// a recency bucket. A visit from the last hour counts four times a last-week
// visit and sixteen times one older than a month, so a page that is merely
// recent but never revisited cannot outrank a habit.
func FrecencyScore(visits int, lastVisit, now time.Time) float64 {
	if visits <= 0 {
		return 0
	}
	age := now.Sub(lastVisit)
	if age < 0 {
		age = 0
	}
	weight := 0.25
	switch {
	case age <= time.Hour:
		weight = 4
	case age <= 24*time.Hour:
		weight = 2
	case age <= 7*24*time.Hour:
		weight = 1
	case age <= 28*24*time.Hour:
		weight = 0.5
	}
	return float64(visits) * weight
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

// DeleteSince removes every visit recorded at or after cutoff and reports how
// many were removed. The caller decides when to persist with Save.
func (s *Store) DeleteSince(cutoff time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.entries[:0]
	removed := 0
	for _, e := range s.entries {
		if e.At.Before(cutoff) {
			kept = append(kept, e)
		} else {
			removed++
		}
	}
	clear(s.entries[len(kept):])
	s.entries = kept
	return removed
}

// DeleteLastHour removes the visits recorded in the hour ending at now.
func (s *Store) DeleteLastHour(now time.Time) int {
	return s.DeleteSince(now.Add(-time.Hour))
}

// DeleteToday removes the visits recorded since local midnight of now's day.
func (s *Store) DeleteToday(now time.Time) int {
	y, m, d := now.Date()
	return s.DeleteSince(time.Date(y, m, d, 0, 0, 0, 0, now.Location()))
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
