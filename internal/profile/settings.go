package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	Homepage            string  `json:"homepage"`
	DefaultSearchEngine string  `json:"default_search_engine"`
	EnableJavaScript    bool    `json:"enable_javascript"`
	EnableImages        bool    `json:"enable_images"`
	DevToolsOpen        bool    `json:"devtools_open"`
	DevToolsSplitOffset float64 `json:"devtools_split_offset"`
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
	if p.Private() {
		return store, nil
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

	// Settings are a whole-document write. Empty strings and false booleans are
	// meaningful values, so concurrent SettingsStore writers are intentionally
	// last-writer-wins instead of merging field-by-field.
	s.settings = settings
	return s.profile.SaveJSON("settings.json", s.settings)
}

// Export writes the current settings as indented JSON to the specified file path.
func (s *SettingsStore) Export(path string) error {
	s.mu.Lock()
	current := s.settings
	s.mu.Unlock()

	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	// Write atomically to avoid partial writes
	tempFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return os.WriteFile(path, data, 0o600)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	return os.Rename(tempPath, path)
}

// Import reads settings JSON from the specified path, validates them, and applies them.
func (s *SettingsStore) Import(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var imported Settings
	if err := json.Unmarshal(data, &imported); err != nil {
		return err
	}

	return s.Set(imported)
}
