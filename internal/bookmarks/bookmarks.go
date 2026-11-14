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

type Entry struct {
	URL   string    `json:"url"`
	Title string    `json:"title,omitempty"`
	Added time.Time `json:"added"`
}

type Store struct {
	mu   sync.Mutex
	path string
	list []Entry
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

const MaxBookmarks = 10000

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

func (s *Store) List() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.list))
	copy(out, s.list)
	return out
}

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
