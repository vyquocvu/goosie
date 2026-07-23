package devtools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollectOrigins(t *testing.T) {
	snapshot := map[string]map[string]string{
		"https://example.com": {"key1": "val1", "key2": "val2"},
		"https://other.com":   {"akey": "aval"},
	}
	origins := collectOrigins(snapshot)
	assert.Equal(t, 2, len(origins))
}

func TestFilterOrigins(t *testing.T) {
	snapshot := map[string]map[string]string{
		"https://example.com": {"key1": "val1"},
		"https://other.com":   {"akey": "aval"},
	}
	filtered := filterSnapshot(snapshot, "example")
	assert.Equal(t, 1, len(filtered))
	_, ok := filtered["https://example.com"]
	assert.True(t, ok)
}

func TestFilterOriginsByKey(t *testing.T) {
	snapshot := map[string]map[string]string{
		"https://example.com": {"username": "alice", "role": "admin"},
		"https://other.com":   {"theme": "dark"},
	}
	filtered := filterSnapshot(snapshot, "user")
	assert.Equal(t, 1, len(filtered))
	_, ok := filtered["https://example.com"]
	assert.True(t, ok)
}

func TestFilterOriginsNoMatch(t *testing.T) {
	snapshot := map[string]map[string]string{
		"https://example.com": {"key1": "val1"},
	}
	filtered := filterSnapshot(snapshot, "zzzzz")
	assert.Equal(t, 0, len(filtered))
}

func TestStoragePanelRefresh(t *testing.T) {
	p := newStoragePanel(nil)
	assert.NotNil(t, p)
}

// mockStorage implements storageProvider for testing.
type mockStorage struct {
	data map[string]map[string]string
}

func (m *mockStorage) Snapshot() map[string]map[string]string {
	return m.data
}
func (m *mockStorage) Set(origin, key, value string) error { return nil }
func (m *mockStorage) Remove(origin, key string) error     { return nil }
func (m *mockStorage) Clear(origin string) error           { return nil }
