package profile

import (
	"sync"
)

type SessionTab struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}

type sessionDocument struct {
	Session []SessionTab `json:"session"`
}

type SessionStore struct {
	mu      sync.Mutex
	profile *Profile
	doc     sessionDocument
}

func NewSessionStore(p *Profile) (*SessionStore, error) {
	store := &SessionStore{
		profile: p,
		doc: sessionDocument{
			Session: []SessionTab{},
		},
	}
	if err := store.reloadLocked(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *SessionStore) SaveSession(tabs []SessionTab) error {
	return s.profile.withFileLock("session.json", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if err := s.reloadLocked(); err != nil {
			return err
		}

		s.doc.Session = append([]SessionTab(nil), tabs...)
		return s.persist()
	})
}

func (s *SessionStore) SessionTabs() []SessionTab {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]SessionTab(nil), s.doc.Session...)
}

func (s *SessionStore) reloadLocked() error {
	if s.profile.Private() {
		return nil
	}

	doc := sessionDocument{
		Session: []SessionTab{},
	}
	if err := s.profile.LoadJSON("session.json", &doc); err != nil {
		return err
	}
	if doc.Session == nil {
		doc.Session = []SessionTab{}
	}
	s.doc = doc

	return nil
}

func (s *SessionStore) persist() error {
	return s.profile.SaveJSON("session.json", s.doc)
}
