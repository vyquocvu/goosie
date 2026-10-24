package session_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/session"
)

// TestScrollRoundTrip guards Bug C4: a saved session must carry the viewport
// scroll position so a restored tab reopens where the user left it.
func TestScrollRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	want := session.State{
		Tabs: []session.TabState{
			{URL: "https://a.example/", Title: "A", ScrollX: 10, ScrollY: 320},
			{URL: "https://b.example/", Title: "B", ScrollX: 0, ScrollY: 640},
		},
		Active: 1,
	}
	if err := session.Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := session.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Tabs) != 2 {
		t.Fatalf("tabs = %d, want 2", len(got.Tabs))
	}
	for i, tab := range want.Tabs {
		if got.Tabs[i].ScrollX != tab.ScrollX || got.Tabs[i].ScrollY != tab.ScrollY {
			t.Errorf("tab %d scroll = (%d,%d), want (%d,%d)",
				i, got.Tabs[i].ScrollX, got.Tabs[i].ScrollY, tab.ScrollX, tab.ScrollY)
		}
	}
}

// TestLoadLegacyStateWithoutScroll keeps forward compatibility: state files
// written before scroll persistence have no scroll keys and must load as 0.
func TestLoadLegacyStateWithoutScroll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	legacy := `{"tabs":[{"url":"https://a.example/","title":"A","history":["https://a.example/"],"historyIndex":0}],"active":0}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}
	got, err := session.Load(path)
	if err != nil {
		t.Fatalf("Load legacy state: %v", err)
	}
	if len(got.Tabs) != 1 {
		t.Fatalf("tabs = %d, want 1", len(got.Tabs))
	}
	if got.Tabs[0].ScrollX != 0 || got.Tabs[0].ScrollY != 0 {
		t.Errorf("legacy scroll = (%d,%d), want (0,0)", got.Tabs[0].ScrollX, got.Tabs[0].ScrollY)
	}
	if got.Tabs[0].URL != "https://a.example/" || got.Tabs[0].HistoryIndex != 0 {
		t.Errorf("legacy tab lost its fields: %+v", got.Tabs[0])
	}
}

// TestTabStateJSONFieldNames pins the serialized shape so old and new binaries
// read each other's files.
func TestTabStateJSONFieldNames(t *testing.T) {
	data, err := json.Marshal(session.TabState{URL: "https://a.example/", ScrollX: 5, ScrollY: 7})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"scrollX", "scrollY"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("marshalled TabState missing %q: %s", key, data)
		}
	}
}
