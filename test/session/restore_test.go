package session_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/session"
)

func TestLoadReSyncsURLFromHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := `{
		"tabs": [{
			"url": "https://stale/",
			"title": "Test",
			"history": ["https://old/", "https://current/"],
			"historyIndex": 1
		}],
		"active": 0
	}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tabs) != 1 {
		t.Fatalf("tabs = %d, want 1", len(got.Tabs))
	}
	if got.Tabs[0].URL != "https://current/" {
		t.Errorf("URL = %q, want https://current/ (re-synced from history)", got.Tabs[0].URL)
	}
}

func TestLoadClampsNegativeActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := `{"tabs":[{"url":"https://a/"}],"active":-5}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active < 0 {
		t.Errorf("Active = %d, want >= 0 (clamped from -5)", got.Active)
	}
}

func TestSaveEmptyStateProducesValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if err := session.Save(path, session.State{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed session.State
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("saved state is not valid JSON: %v", err)
	}
	if len(parsed.Tabs) != 0 {
		t.Errorf("loaded %d tabs from empty state, want 0", len(parsed.Tabs))
	}
}

func TestSaveOverwritesPreviousState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")

	first := session.State{Tabs: []session.TabState{{URL: "https://first/"}}}
	if err := session.Save(path, first); err != nil {
		t.Fatal(err)
	}

	second := session.State{Tabs: []session.TabState{{URL: "https://second/"}}}
	if err := session.Save(path, second); err != nil {
		t.Fatal(err)
	}

	got, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tabs) != 1 || got.Tabs[0].URL != "https://second/" {
		t.Errorf("after overwrite, got %+v, want the second state", got.Tabs)
	}
}

func TestLoadHistoryIndexClampToNegativeOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := `{
		"tabs": [{
			"url": "",
			"history": [],
			"historyIndex": -1
		}],
		"active": 0
	}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tabs) != 1 {
		t.Fatalf("tabs = %d, want 1", len(got.Tabs))
	}
	if got.Tabs[0].HistoryIndex != -1 {
		t.Errorf("HistoryIndex = %d, want -1 (empty history sentinel)", got.Tabs[0].HistoryIndex)
	}
}

func TestSaveDoesNotLeaveTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	if err := session.Save(path, session.State{Tabs: []session.TabState{{URL: "https://x/"}}}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if len(e.Name()) > 0 && e.Name()[0] == '.' {
			t.Errorf("leftover temp file: %q", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("directory has %d entries, want 1 (just session.json)", len(entries))
	}
}

func TestLoadWithNoHistoryFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := `{"tabs":[{"url":"https://minimal/"}],"active":0}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tabs) != 1 {
		t.Fatalf("tabs = %d, want 1", len(got.Tabs))
	}
	tab := got.Tabs[0]
	if tab.URL != "https://minimal/" {
		t.Errorf("URL = %q, want https://minimal/", tab.URL)
	}
	if len(tab.History) != 0 {
		t.Errorf("History = %v, want empty", tab.History)
	}
}
