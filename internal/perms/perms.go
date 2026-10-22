// Package perms manages granular per-origin site permissions and content settings.
package perms

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Permission identifies a controllable browser capability for an origin.
type Permission string

const (
	// PermissionJavaScript controls execution of author and inline scripts.
	PermissionJavaScript Permission = "javascript"
	// PermissionImages controls loading and decoding of web images.
	PermissionImages Permission = "images"
	// PermissionCookies controls persistent cookie storage for an origin.
	PermissionCookies Permission = "cookies"
)

// PermissionState defines whether a permission is allowed, blocked, or using default.
type PermissionState string

const (
	// StateDefault indicates the permission inherits the global browser policy.
	StateDefault PermissionState = "default"
	// StateAllow explicitly permits the capability for the origin.
	StateAllow PermissionState = "allow"
	// StateBlock explicitly forbids the capability for the origin.
	StateBlock PermissionState = "block"
)

// OriginSettings holds the set of configured permissions for a single origin.
type OriginSettings struct {
	Origin      string                         `json:"origin"`
	Permissions map[Permission]PermissionState `json:"permissions"`
}

// Manager stores and manages per-origin permissions with thread-safe persistence.
type Manager struct {
	mu       sync.RWMutex
	filePath string
	origins  map[string]map[Permission]PermissionState
}

// NormalizeOrigin extracts the canonical scheme://host[:port] origin from a raw URL.
func NormalizeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.ToLower(raw)
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// NewManager creates a Manager bound to filePath.
func NewManager(filePath string) *Manager {
	return &Manager{
		filePath: filePath,
		origins:  make(map[string]map[Permission]PermissionState),
	}
}

// State returns the explicit permission state for an origin and permission.
func (m *Manager) State(origin string, perm Permission) PermissionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	canonical := NormalizeOrigin(origin)
	if pMap, ok := m.origins[canonical]; ok {
		if st, found := pMap[perm]; found {
			return st
		}
	}
	return StateDefault
}

// IsAllowed reports whether perm is allowed for origin.
// In StateDefault, all built-in capabilities (JS, Images, Cookies) are allowed.
func (m *Manager) IsAllowed(origin string, perm Permission) bool {
	st := m.State(origin, perm)
	return st != StateBlock
}

// Set updates the permission state for an origin.
func (m *Manager) Set(origin string, perm Permission, state PermissionState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	canonical := NormalizeOrigin(origin)
	if canonical == "" {
		return
	}
	pMap, ok := m.origins[canonical]
	if !ok {
		pMap = make(map[Permission]PermissionState)
		m.origins[canonical] = pMap
	}
	if state == StateDefault {
		delete(pMap, perm)
		if len(pMap) == 0 {
			delete(m.origins, canonical)
		}
	} else {
		pMap[perm] = state
	}
}

// Clear removes all custom permissions for an origin.
func (m *Manager) Clear(origin string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	canonical := NormalizeOrigin(origin)
	delete(m.origins, canonical)
}

// All returns all customized origin settings sorted by origin.
func (m *Manager) All() []OriginSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]OriginSettings, 0, len(m.origins))
	for o, pMap := range m.origins {
		copied := make(map[Permission]PermissionState, len(pMap))
		for k, v := range pMap {
			copied[k] = v
		}
		out = append(out, OriginSettings{
			Origin:      o,
			Permissions: copied,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Origin < out[j].Origin
	})
	return out
}

// Load loads permissions from disk.
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
	var loaded map[string]map[Permission]PermissionState
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	m.origins = loaded
	return nil
}

// Save atomically writes permissions to disk.
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.filePath == "" {
		return nil
	}
	data, err := json.MarshalIndent(m.origins, "", "  ")
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
