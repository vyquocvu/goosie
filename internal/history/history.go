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

const MaxEntries = 5000

type Transition string

const (
	TransitionTyped  Transition = "typed"
	TransitionLink   Transition = "link"
	TransitionReload Transition = "reload"
	TransitionOther  Transition = "other"
)

type Entry struct {
	URL        string     `json:"url"`
	Title      string     `json:"title,omitempty"`
	At         time.Time  `json:"at"`
	Visits     int        `json:"visits,omitempty"`
	Transition Transition `json:"transition,omitempty"`
}

type Store struct {
	mu      sync.Mutex
	path    string
	entries []Entry
	counts  map[string]int
}

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

func (s *Store) Record(url, title string) {
	s.RecordVisit(url, title, TransitionOther)
}

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

func (s *Store) Entries() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.entries))
	copy(out, s.entries)
	return out
}

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

func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
}

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

func (s *Store) DeleteLastHour(now time.Time) int {
	return s.DeleteSince(now.Add(-time.Hour))
}

func (s *Store) DeleteToday(now time.Time) int {
	y, m, d := now.Date()
	return s.DeleteSince(time.Date(y, m, d, 0, 0, 0, 0, now.Location()))
}

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
