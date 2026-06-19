package profile

import (
	"sync"
	"time"
)

type Visit struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	VisitedAt time.Time `json:"visited_at"`
}

type SessionTab struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}

type historyDocument struct {
	Visits  []Visit      `json:"visits"`
	Session []SessionTab `json:"session"`
}

type HistoryStore struct {
	mu      sync.Mutex
	profile *Profile
	doc     historyDocument
}

func NewHistoryStore(p *Profile) (*HistoryStore, error) {
	store := &HistoryStore{
		profile: p,
		doc: historyDocument{
			Visits:  []Visit{},
			Session: []SessionTab{},
		},
	}
	if err := store.reloadLocked(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *HistoryStore) AddVisit(url, title string) error {
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
}

func (s *HistoryStore) VisitURLs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	urls := make([]string, len(s.doc.Visits))
	for i, visit := range s.doc.Visits {
		urls[i] = visit.URL
	}

	return urls
}

func (s *HistoryStore) SaveSession(tabs []SessionTab) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.reloadLocked(); err != nil {
		return err
	}

	s.doc.Session = append([]SessionTab(nil), tabs...)
	return s.persist()
}

func (s *HistoryStore) SessionTabs() []SessionTab {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]SessionTab(nil), s.doc.Session...)
}

func (s *HistoryStore) reloadLocked() error {
	if s.profile.Private() {
		return nil
	}

	doc := historyDocument{
		Visits:  []Visit{},
		Session: []SessionTab{},
	}
	if err := s.profile.LoadJSON("history.json", &doc); err != nil {
		return err
	}
	if doc.Visits == nil {
		doc.Visits = []Visit{}
	}
	if doc.Session == nil {
		doc.Session = []SessionTab{}
	}
	s.doc = doc

	return nil
}

func (s *HistoryStore) persist() error {
	return s.profile.SaveJSON("history.json", s.doc)
}
