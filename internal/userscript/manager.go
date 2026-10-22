package userscript

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Manager manages a collection of user scripts loaded from disk or registered at runtime.
type Manager struct {
	mu      sync.RWMutex
	dir     string
	scripts []*Script
}

// NewManager creates a script manager targeting dir.
func NewManager(dir string) *Manager {
	return &Manager{
		dir: dir,
	}
}

// Dir returns the directory path managed by this manager.
func (m *Manager) Dir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dir
}

// AddScript registers a script directly in the manager.
func (m *Manager) AddScript(s *Script) {
	if s == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, existing := range m.scripts {
		if existing.ID == s.ID {
			m.scripts[i] = s
			return
		}
	}
	m.scripts = append(m.scripts, s)
}

// RemoveScript unregisters a script by ID.
func (m *Manager) RemoveScript(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, s := range m.scripts {
		if s.ID == id {
			m.scripts = append(m.scripts[:i], m.scripts[i+1:]...)
			return
		}
	}
}

// ScriptByID finds a script by ID.
func (m *Manager) ScriptByID(id string) *Script {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, s := range m.scripts {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// SetEnabled toggles a script's enabled status.
func (m *Manager) SetEnabled(id string, enabled bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, s := range m.scripts {
		if s.ID == id {
			s.Enabled = enabled
			return true
		}
	}
	return false
}

// Scripts returns a snapshot slice of all registered scripts.
func (m *Manager) Scripts() []*Script {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]*Script, len(m.scripts))
	copy(res, m.scripts)
	return res
}

// MatchingScripts returns all enabled scripts matching rawURL and scheduled to run at stage.
func (m *Manager) MatchingScripts(rawURL string, stage RunAt) []*Script {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var res []*Script
	for _, s := range m.scripts {
		if !s.Enabled {
			continue
		}
		if s.RunAt != stage {
			// document-idle scripts run with document-end in our engine
			if !(stage == RunAtDocumentEnd && s.RunAt == RunAtDocumentIdle) {
				continue
			}
		}
		if s.MatchesURL(rawURL) {
			res = append(res, s)
		}
	}
	return res
}

// Load scans the configured directory for *.user.js and *.js files, parsing each.
func (m *Manager) Load() error {
	if m.dir == "" {
		return nil
	}

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	var loaded []*Script
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".user.js") && !strings.HasSuffix(name, ".js") {
			continue
		}

		fullPath := filepath.Join(m.dir, name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		s, err := Parse(string(data), fullPath)
		if err != nil {
			continue
		}
		loaded = append(loaded, s)
	}

	m.mu.Lock()
	m.scripts = loaded
	m.mu.Unlock()
	return nil
}
