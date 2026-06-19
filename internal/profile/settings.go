package profile

import "sync"

type Settings struct {
	Homepage            string `json:"homepage"`
	DefaultSearchEngine string `json:"default_search_engine"`
	EnableJavaScript    bool   `json:"enable_javascript"`
	EnableImages        bool   `json:"enable_images"`
}

func DefaultSettings() Settings {
	return Settings{
		Homepage:            "https://example.com",
		DefaultSearchEngine: "https://www.google.com/search?q=",
		EnableJavaScript:    true,
		EnableImages:        true,
	}
}

type SettingsStore struct {
	mu       sync.Mutex
	profile  *Profile
	settings Settings
}

func NewSettingsStore(p *Profile) (*SettingsStore, error) {
	store := &SettingsStore{
		profile:  p,
		settings: DefaultSettings(),
	}
	if err := p.LoadJSON("settings.json", &store.settings); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *SettingsStore) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.settings
}

func (s *SettingsStore) Set(settings Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.settings = settings
	return s.profile.SaveJSON("settings.json", s.settings)
}
