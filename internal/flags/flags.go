// Package flags provides an experimental browser feature flags registry and persistence engine.
package flags

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Flag describes a configurable browser capability or experimental engine feature.
type Flag struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
	Enabled     bool   `json:"enabled"`
}

// DefaultFlags returns the built-in system flags and their canonical default states.
func DefaultFlags() []Flag {
	return []Flag{
		{
			ID:          "smooth-scrolling",
			Name:        "Smooth Scrolling",
			Description: "Enables fluid frame-pacing transitions when scrolling through long documents with keyboard or wheel.",
			Default:     true,
			Enabled:     true,
		},
		{
			ID:          "strict-csp",
			Name:        "Strict Content Security Policy Level 3",
			Description: "Enforces strict Level 3 CSP parsing and blocks non-compliant script/style evaluations.",
			Default:     true,
			Enabled:     true,
		},
		{
			ID:          "doh-dns",
			Name:        "DNS-over-HTTPS (DoH) Enforcement",
			Description: "Routes remote DNS queries securely over HTTPS via RFC 8484 resolvers.",
			Default:     false,
			Enabled:     false,
		},
		{
			ID:          "preload-fonts",
			Name:        "Preload Web Fonts",
			Description: "Eagerly initiates web font retrieval during CSS cascade before the initial layout pass.",
			Default:     true,
			Enabled:     true,
		},
		{
			ID:          "experimental-svg",
			Name:        "Hardware Vector Shapes Pipeline",
			Description: "Emits specialized display list instructions for complex inline SVG polygons, lines, and paths.",
			Default:     true,
			Enabled:     true,
		},
		{
			ID:          "pwa-standalone",
			Name:        "Progressive Web App Standalone Mode",
			Description: "Permits websites declaring W3C standalone manifests to launch in dedicated chromeless application windows.",
			Default:     true,
			Enabled:     true,
		},
		{
			ID:          "pdf-viewer",
			Name:        "Built-in PDF Document Viewer",
			Description: "Renders portable document format (PDF) files natively in-viewport instead of dispatching to downloads.",
			Default:     true,
			Enabled:     true,
		},
		{
			ID:          "userscripts-enabled",
			Name:        "User Scripts & Extensions Engine",
			Description: "Scans profile userscripts directory and automatically injects matching Greasemonkey/Tampermonkey extensions.",
			Default:     true,
			Enabled:     true,
		},
	}
}

// Manager coordinates feature flags and synchronizes state to disk.
type Manager struct {
	mu       sync.RWMutex
	filePath string
	flags    map[string]Flag
}

// NewManager creates a Manager seeded with system defaults.
func NewManager(filePath string) *Manager {
	m := &Manager{
		filePath: filePath,
		flags:    make(map[string]Flag),
	}
	for _, f := range DefaultFlags() {
		m.flags[f.ID] = f
	}
	return m
}

// IsEnabled reports whether flag ID is currently enabled. If unknown, returns false.
func (m *Manager) IsEnabled(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	f, ok := m.flags[id]
	if !ok {
		return false
	}
	return f.Enabled
}

// SetEnabled updates the state of flag ID.
func (m *Manager) SetEnabled(id string, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.flags[id]; ok {
		f.Enabled = enabled
		m.flags[id] = f
	}
}

// ResetAll resets all feature flags back to their default values.
func (m *Manager) ResetAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range DefaultFlags() {
		m.flags[f.ID] = f
	}
}

// Flags returns all flags sorted alphabetically by Name.
func (m *Manager) Flags() []Flag {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Flag, 0, len(m.flags))
	for _, f := range m.flags {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// HasNonDefaultReports whether any flag has been modified from its default setting.
func (m *Manager) HasNonDefault() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, f := range m.flags {
		if f.Enabled != f.Default {
			return true
		}
	}
	return false
}

// Load loads persisted flags from disk.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.filePath == "" {
		return nil
	}
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var saved map[string]bool
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	for id, val := range saved {
		if f, ok := m.flags[id]; ok {
			f.Enabled = val
			m.flags[id] = f
		}
	}
	return nil
}

// Save writes active flag states to disk atomically.
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.filePath == "" {
		return nil
	}
	saved := make(map[string]bool)
	for id, f := range m.flags {
		saved[id] = f.Enabled
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := m.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, m.filePath)
}
