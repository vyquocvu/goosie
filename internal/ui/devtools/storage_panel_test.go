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

func TestStoragePanel_RefreshFromPopulates(t *testing.T) {
	p := newStoragePanel(nil).(*storagePanel)
	ctx := &TabContext{
		Storage: &mockStorage{
			data: map[string]map[string]string{
				"https://example.com": {"key1": "val1"},
			},
		},
	}
	p.RefreshFrom(ctx)
	assert.Equal(t, 2, len(p.entries))
}

func TestStoragePanel_DeleteSelected(t *testing.T) {
	removedOrigin := ""
	removedKey := ""
	p := newStoragePanel(nil).(*storagePanel)
	p.store = &mockStorage{
		data: map[string]map[string]string{
			"https://example.com": {"key1": "val1"},
		},
		onRemove: func(origin, key string) {
			removedOrigin = origin
			removedKey = key
		},
	}
	p.entries = []storageRow{
		{Origin: "https://example.com"},
		{Origin: "https://example.com", Key: "key1", Value: "val1"},
	}
	p.selectedIdx = 1
	p.deleteSelected()
	assert.Equal(t, "https://example.com", removedOrigin)
	assert.Equal(t, "key1", removedKey)
}

func TestStoragePanel_DeleteNoSelection(t *testing.T) {
	p := newStoragePanel(nil).(*storagePanel)
	p.store = &mockStorage{
		data: map[string]map[string]string{
			"https://example.com": {"key1": "val1"},
		},
	}
	p.selectedIdx = -1
	p.deleteSelected()
}

func TestStoragePanel_ClearSelectedOrigin(t *testing.T) {
	clearedOrigin := ""
	p := newStoragePanel(nil).(*storagePanel)
	p.store = &mockStorage{
		data: map[string]map[string]string{
			"https://example.com": {"key1": "val1"},
		},
		onClear: func(origin string) {
			clearedOrigin = origin
		},
	}
	p.entries = []storageRow{
		{Origin: "https://example.com"},
		{Origin: "https://example.com", Key: "key1", Value: "val1"},
	}
	p.selectedIdx = 0
	p.clearSelectedOrigin()
	assert.Equal(t, "https://example.com", clearedOrigin)
}

func TestStoragePanel_SelectedOrigin(t *testing.T) {
	p := newStoragePanel(nil).(*storagePanel)
	p.entries = []storageRow{
		{Origin: "https://example.com"},
		{Origin: "https://example.com", Key: "k", Value: "v"},
		{Origin: "https://other.com"},
	}
	p.selectedIdx = 1
	assert.Equal(t, "https://example.com", p.selectedOrigin())
	assert.Equal(t, "k", p.selectedKey())
}

func TestStoragePanel_SelectedOriginInvalid(t *testing.T) {
	p := newStoragePanel(nil).(*storagePanel)
	p.selectedIdx = -1
	assert.Equal(t, "", p.selectedOrigin())
	assert.Equal(t, "", p.selectedKey())

	p.selectedIdx = 99
	assert.Equal(t, "", p.selectedOrigin())
}

// mockStorage implements storageProvider for testing.
type mockStorage struct {
	data    map[string]map[string]string
	onSet    func(origin, key, value string)
	onRemove func(origin, key string)
	onClear  func(origin string)
}

func (m *mockStorage) Snapshot() map[string]map[string]string {
	return m.data
}
func (m *mockStorage) Set(origin, key, value string) error {
	if m.onSet != nil {
		m.onSet(origin, key, value)
	}
	return nil
}
func (m *mockStorage) Remove(origin, key string) error {
	if m.onRemove != nil {
		m.onRemove(origin, key)
	}
	return nil
}
func (m *mockStorage) Clear(origin string) error {
	if m.onClear != nil {
		m.onClear(origin)
	}
	return nil
}
