package profile

import (
	"sync"
)

type SessionTab struct {
	URL    string `json:"url"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}

type SessionStore struct {
	mu      sync.Mutex
	profile *Profile
	tabs    []SessionTab
}

func NewSessionStore(p *Profile) (*SessionStore, error) {
	store := &SessionStore{
		profile: p,
		tabs:    []SessionTab{},
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

		s.tabs = append([]SessionTab(nil), tabs...)
		return s.persist()
	})
}

func (s *SessionStore) SessionTabs() []SessionTab {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]SessionTab(nil), s.tabs...)
}

func (s *SessionStore) reloadLocked() error {
	if s.profile.Private() {
		return nil
	}

	var tabs []SessionTab
	if err := s.profile.LoadJSON("session.json", &tabs); err != nil {
		return err
	}
	if tabs == nil {
		tabs = []SessionTab{}
	}
	s.tabs = tabs

	return nil
}

func (s *SessionStore) persist() error {
	return s.profile.SaveJSON("session.json", s.tabs)
}
