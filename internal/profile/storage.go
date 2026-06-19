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
	if err := p.LoadJSON("storage.json", &store.data); err != nil {
		return nil, err
	}
	if store.data == nil {
		store.data = map[string]map[string]string{}
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data[origin] == nil {
		s.data[origin] = map[string]string{}
	}
	s.data[origin][key] = value
	return s.persist()
}

func (s *StorageStore) Remove(origin, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	values, ok := s.data[origin]
	if !ok {
		return nil
	}
	delete(values, key)
	if len(values) == 0 {
		delete(s.data, origin)
	}

	return s.persist()
}

func (s *StorageStore) Clear(origin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, origin)
	return s.persist()
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

func (s *StorageStore) persist() error {
	return s.profile.SaveJSON("storage.json", s.data)
}
