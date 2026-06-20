package ui

import (
	"sync"

	"github.com/vyquocvu/goosie/internal/profile"
)

// Settings represents browser preferences/settings
type Settings struct {
	mu                  sync.RWMutex
	homepage            string
	defaultSearchEngine string
	enableJavaScript    bool
	enableImages        bool
}

// NewSettings creates a new Settings instance with default values
func NewSettings() *Settings {
	return &Settings{
		homepage:            "https://example.com",
		defaultSearchEngine: "https://www.google.com/search?q=",
		enableJavaScript:    true,
		enableImages:        true,
	}
}

// GetHomepage returns the homepage URL
func (s *Settings) GetHomepage() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.homepage
}

// SetHomepage sets the homepage URL
func (s *Settings) SetHomepage(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.homepage = url
}

// GetDefaultSearchEngine returns the default search engine URL
func (s *Settings) GetDefaultSearchEngine() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defaultSearchEngine
}

// SetDefaultSearchEngine sets the default search engine URL
func (s *Settings) SetDefaultSearchEngine(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultSearchEngine = url
}

// GetEnableJavaScript returns whether JavaScript is enabled
func (s *Settings) GetEnableJavaScript() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enableJavaScript
}

// SetEnableJavaScript sets whether JavaScript is enabled
func (s *Settings) SetEnableJavaScript(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enableJavaScript = enabled
}

// GetEnableImages returns whether images are enabled
func (s *Settings) GetEnableImages() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enableImages
}

// SetEnableImages sets whether images are enabled
func (s *Settings) SetEnableImages(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enableImages = enabled
}

func (s *Settings) ApplyProfileSettings(settings profile.Settings) {
	s.SetHomepage(settings.Homepage)
	s.SetDefaultSearchEngine(settings.DefaultSearchEngine)
	s.SetEnableJavaScript(settings.EnableJavaScript)
	s.SetEnableImages(settings.EnableImages)
}

func (s *Settings) ToProfileSettings() profile.Settings {
	return profile.Settings{
		Homepage:            s.GetHomepage(),
		DefaultSearchEngine: s.GetDefaultSearchEngine(),
		EnableJavaScript:    s.GetEnableJavaScript(),
		EnableImages:        s.GetEnableImages(),
	}
}
