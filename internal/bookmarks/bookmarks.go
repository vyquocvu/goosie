// Package bookmarks stores a profile's starred pages as an ordered list
// persisted to disk.
package bookmarks

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

// Entry is one starred page.
type Entry struct {
	URL   string    `json:"url"`
	Title string    `json:"title,omitempty"`
	Added time.Time `json:"added"`
}

// Store is an ordered bookmark list. It is safe for concurrent use; mutators
// (Toggle, Delete, Rename) only change memory and callers decide when to
// persist with Save.
type Store struct {
	mu   sync.Mutex
	path string
	list []Entry
}

// NewStore returns an in-memory store that saves to path. Nothing is loaded;
// call Load to build a store from disk.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// MaxBookmarks caps how many entries Load keeps: ten times a heavy real
// profile. Only hostile or hand-edited files ever reach it; Toggle can never
// create duplicates or empties through the API, so valid stores load byte
// for byte as before.
const MaxBookmarks = 10000

// Load reads the bookmark file. A missing file is an empty store, not an
// error; a file that exists but cannot be parsed is an error so a corrupt
// store is never overwritten on the next Save.
func Load(path string) (*Store, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NewStore(path), nil
		}
		return nil, err
	}
	var list []Entry
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	// Migration bound: drop unnavigable empties, fold duplicates keeping
	// first-seen order, and cap the total. Valid stores already satisfy all
	// three, so ordinary profiles load unchanged.
	seen := make(map[string]bool, len(list))
	kept := make([]Entry, 0, len(list))
	for _, e := range list {
		if e.URL == "" || seen[e.URL] {
			continue
		}
		seen[e.URL] = true
		kept = append(kept, e)
		if len(kept) >= MaxBookmarks {
			break
		}
	}
	return &Store{path: path, list: kept}, nil
}

// Toggle adds url (keeping insertion order) or removes it, and reports whether
// it is bookmarked afterwards.
func (s *Store) Toggle(url, title string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.list {
		if e.URL == url {
			s.list = append(s.list[:i:i], s.list[i+1:]...)
			return false
		}
	}
	s.list = append(s.list, Entry{URL: url, Title: title, Added: time.Now()})
	return true
}

// Delete removes the bookmark for url and reports whether it existed. Like
// Toggle, it only mutates memory; the caller decides when to persist with
// Save.
func (s *Store) Delete(url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.list {
		if e.URL == url {
			s.list = append(s.list[:i:i], s.list[i+1:]...)
			return true
		}
	}
	return false
}

// Rename sets the display title of the bookmark for url and reports whether
// it existed. The title is stored verbatim; presentation layers are
// responsible for escaping it.
func (s *Store) Rename(url, title string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.list {
		if e.URL == url {
			s.list[i].Title = title
			return true
		}
	}
	return false
}

// Search returns bookmarks whose URL or title contains query
// case-insensitively, in insertion order. An empty query returns the full
// list, matching List.
func (s *Store) Search(query string) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Entry, 0, len(s.list))
	for _, e := range s.list {
		if q == "" || strings.Contains(strings.ToLower(e.URL), q) || strings.Contains(strings.ToLower(e.Title), q) {
			out = append(out, e)
		}
	}
	return out
}

// Contains reports whether url is bookmarked.
func (s *Store) Contains(url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.list {
		if e.URL == url {
			return true
		}
	}
	return false
}

// List returns a copy of the bookmarks in insertion order.
func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.list))
	copy(out, s.list)
	return out
}

// Save writes the list atomically: the bytes land in a temp file first and
// rename in, so a truncated write never replaces a good file.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(s.list, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, data, ".bookmarks-*")
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
