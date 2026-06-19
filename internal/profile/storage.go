package profile

import (
	"sort"
	"sync"
)

type StorageStore struct {
	mu      sync.Mutex
	profile *Profile
	data    map[string]map[string]string
}

func NewStorageStore(p *Profile) (*StorageStore, error) {
	store := &StorageStore{
		profile: p,
		data:    map[string]map[string]string{},
	}
	if err := store.reloadLocked(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *StorageStore) Get(origin, key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	values, ok := s.data[origin]
	if !ok {
		return "", false
	}
	value, ok := values[key]
	return value, ok
}

func (s *StorageStore) Set(origin, key, value string) error {
	return s.profile.withFileLock("storage.json", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if err := s.reloadLocked(); err != nil {
			return err
		}

		if s.data[origin] == nil {
			s.data[origin] = map[string]string{}
		}
		s.data[origin][key] = value
		return s.persist()
	})
}

func (s *StorageStore) Remove(origin, key string) error {
	return s.profile.withFileLock("storage.json", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if err := s.reloadLocked(); err != nil {
			return err
		}

		values, ok := s.data[origin]
		if !ok {
			return nil
		}
		delete(values, key)
		if len(values) == 0 {
			delete(s.data, origin)
		}

		return s.persist()
	})
}

func (s *StorageStore) Clear(origin string) error {
	return s.profile.withFileLock("storage.json", func() error {
		s.mu.Lock()
		defer s.mu.Unlock()

		if err := s.reloadLocked(); err != nil {
			return err
		}

		delete(s.data, origin)
		return s.persist()
	})
}

func (s *StorageStore) Keys(origin string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	values := s.data[origin]
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

func (s *StorageStore) Snapshot() map[string]map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := make(map[string]map[string]string, len(s.data))
	for origin, values := range s.data {
		copiedValues := make(map[string]string, len(values))
		for key, value := range values {
			copiedValues[key] = value
		}
		snapshot[origin] = copiedValues
	}

	return snapshot
}

func (s *StorageStore) reloadLocked() error {
	if s.profile.Private() {
		return nil
	}

	data := map[string]map[string]string{}
	if err := s.profile.LoadJSON("storage.json", &data); err != nil {
		return err
	}
	if data == nil {
		data = map[string]map[string]string{}
	}
	for origin, values := range data {
		if values == nil {
			data[origin] = map[string]string{}
		}
	}
	s.data = data

	return nil
}

func (s *StorageStore) persist() error {
	return s.profile.SaveJSON("storage.json", s.data)
}
