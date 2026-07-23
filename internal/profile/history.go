package profile

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Visit struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	VisitedAt time.Time `json:"visited_at"`
}

type historyDocument struct {
	Visits []Visit `json:"visits"`
}

type HistoryStore struct {
	mu          sync.Mutex
	profile     *Profile
	doc         historyDocument
	lastLoaded  time.Time
	lastModTime time.Time
	lastVersion uint64
}

func NewHistoryStore(p *Profile) (*HistoryStore, error) {
	store := &HistoryStore{
		profile: p,
		doc: historyDocument{
			Visits: []Visit{},
		},
	}
	if err := store.reloadLocked(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *HistoryStore) AddVisit(url, title string) error {
	return s.profile.withFileLock("history.json", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if err := s.reloadLocked(); err != nil {
			return err
		}

		s.doc.Visits = append(s.doc.Visits, Visit{
			URL:       url,
			Title:     title,
			VisitedAt: time.Now().UTC(),
		})
		return s.persist()
	})
}

func (s *HistoryStore) VisitURLs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	_ = s.reloadLocked()

	urls := make([]string, len(s.doc.Visits))
	for i, visit := range s.doc.Visits {
		urls[i] = visit.URL
	}

	return urls
}

func (s *HistoryStore) reloadLocked() error {
	if s.profile.Private() {
		return nil
	}

	currentVersion := s.profile.SnapshotVersion("history.json")
	if !s.lastLoaded.IsZero() && currentVersion == s.lastVersion {
		path := filepath.Join(s.profile.Root(), "history.json")
		if info, err := os.Stat(path); err == nil {
			if !info.ModTime().After(s.lastModTime) {
				return nil
			}
			s.lastModTime = info.ModTime()
		} else if os.IsNotExist(err) {
			return nil
		} else {
			return err
		}
	}

	doc := historyDocument{
		Visits: []Visit{},
	}
	if err := s.profile.LoadJSON("history.json", &doc); err != nil {
		return err
	}
	if doc.Visits == nil {
		doc.Visits = []Visit{}
	}
	s.doc = doc
	s.lastLoaded = time.Now()
	s.lastVersion = currentVersion

	path := filepath.Join(s.profile.Root(), "history.json")
	if info, err := os.Stat(path); err == nil {
		s.lastModTime = info.ModTime()
	}

	return nil
}

func (s *HistoryStore) persist() error {
	err := s.profile.SaveJSON("history.json", s.doc)
	if err == nil {
		s.lastLoaded = time.Now()
		s.lastVersion = s.profile.SnapshotVersion("history.json")
	}
	return err
}

func (s *HistoryStore) Visits() []Visit {
	s.mu.Lock()
	defer s.mu.Unlock()

	_ = s.reloadLocked()

	visits := make([]Visit, len(s.doc.Visits))
	copy(visits, s.doc.Visits)
	return visits
}

func (s *HistoryStore) Clear() error {
	return s.profile.withFileLock("history.json", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.doc.Visits = []Visit{}
		return s.persist()
	})
}
