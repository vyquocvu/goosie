package devtools_test

import (
	"github.com/vyquocvu/goosie/internal/ui/devtools"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCollectOrigins(t *testing.T) {
	snapshot := map[string]map[string]string{
		"https://example.com": {"key1": "val1", "key2": "val2"},
		"https://other.com":   {"akey": "aval"},
	}
	origins := devtools.CollectOrigins(snapshot)
	assert.Equal(t, 2, len(origins))
}

func TestFilterOrigins(t *testing.T) {
	snapshot := map[string]map[string]string{
		"https://example.com": {"key1": "val1"},
		"https://other.com":   {"akey": "aval"},
	}
	filtered := devtools.FilterSnapshot(snapshot, "example")
	assert.Equal(t, 1, len(filtered))
	_, ok := filtered["https://example.com"]
	assert.True(t, ok)
}

func TestFilterOriginsByKey(t *testing.T) {
	snapshot := map[string]map[string]string{
		"https://example.com": {"username": "alice", "role": "admin"},
		"https://other.com":   {"theme": "dark"},
	}
	filtered := devtools.FilterSnapshot(snapshot, "user")
	assert.Equal(t, 1, len(filtered))
	_, ok := filtered["https://example.com"]
	assert.True(t, ok)
}

func TestFilterOriginsNoMatch(t *testing.T) {
	snapshot := map[string]map[string]string{
		"https://example.com": {"key1": "val1"},
	}
	filtered := devtools.FilterSnapshot(snapshot, "zzzzz")
	assert.Equal(t, 0, len(filtered))
}

func TestStoragePanelRefresh(t *testing.T) {
	p := devtools.NewStoragePanel(nil)
	assert.NotNil(t, p)
}

func TestStoragePanel_RefreshFromPopulates(t *testing.T) {
	p := devtools.NewStoragePanel(nil).(*devtools.StoragePanel)
	ctx := &devtools.TabContext{
		Storage: &mockStorage{
			data: map[string]map[string]string{
				"https://example.com": {"key1": "val1"},
			},
		},
	}
	p.RefreshFrom(ctx)
	assert.Equal(t, 2, len(p.Entries()))
}

func TestStoragePanel_DeleteSelected(t *testing.T) {
	removedOrigin := ""
	removedKey := ""
	p := devtools.NewStoragePanel(nil).(*devtools.StoragePanel)
	p.SetStore(&mockStorage{
		data: map[string]map[string]string{
			"https://example.com": {"key1": "val1"},
		},
		onRemove: func(origin, key string) {
			removedOrigin = origin
			removedKey = key
		},
	})
	p.SetEntries([]devtools.StorageRow{
		{Origin: "https://example.com"},
		{Origin: "https://example.com", Key: "key1", Value: "val1"},
	})
	p.SetSelectedIdx(1)
	p.DeleteSelected()
	assert.Equal(t, "https://example.com", removedOrigin)
	assert.Equal(t, "key1", removedKey)
}

func TestStoragePanel_DeleteNoSelection(t *testing.T) {
	p := devtools.NewStoragePanel(nil).(*devtools.StoragePanel)
	p.SetStore(&mockStorage{
		data: map[string]map[string]string{
			"https://example.com": {"key1": "val1"},
		},
	})
	p.SetSelectedIdx(-1)
	p.DeleteSelected()
}

func TestStoragePanel_ClearSelectedOrigin(t *testing.T) {
	clearedOrigin := ""
	p := devtools.NewStoragePanel(nil).(*devtools.StoragePanel)
	p.SetStore(&mockStorage{
		data: map[string]map[string]string{
			"https://example.com": {"key1": "val1"},
		},
		onClear: func(origin string) {
			clearedOrigin = origin
		},
	})
	p.SetEntries([]devtools.StorageRow{
		{Origin: "https://example.com"},
		{Origin: "https://example.com", Key: "key1", Value: "val1"},
	})
	p.SetSelectedIdx(0)
	p.ClearSelectedOrigin()
	assert.Equal(t, "https://example.com", clearedOrigin)
}

func TestStoragePanel_SelectedOrigin(t *testing.T) {
	p := devtools.NewStoragePanel(nil).(*devtools.StoragePanel)
	p.SetEntries([]devtools.StorageRow{
		{Origin: "https://example.com"},
		{Origin: "https://example.com", Key: "k", Value: "v"},
		{Origin: "https://other.com"},
	})
	p.SetSelectedIdx(1)
	assert.Equal(t, "https://example.com", p.SelectedOrigin())
	assert.Equal(t, "k", p.SelectedKey())
}

func TestStoragePanel_SelectedOriginInvalid(t *testing.T) {
	p := devtools.NewStoragePanel(nil).(*devtools.StoragePanel)
	p.SetSelectedIdx(-1)
	assert.Equal(t, "", p.SelectedOrigin())
	assert.Equal(t, "", p.SelectedKey())

	p.SetSelectedIdx(99)
	assert.Equal(t, "", p.SelectedOrigin())
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
