package session_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/session"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	want := session.State{
		Tabs: []session.TabState{
			{URL: "https://a.example/", Title: "A", History: []string{"https://a.example/"}, HistoryIndex: 0},
			{URL: "https://b.example/", Title: "B", History: []string{"https://a.example/", "https://b.example/"}, HistoryIndex: 1},
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
	if got.Active != want.Active {
		t.Errorf("Active = %d, want %d", got.Active, want.Active)
	}
	if len(got.Tabs) != len(want.Tabs) {
		t.Fatalf("tab count = %d, want %d", len(got.Tabs), len(want.Tabs))
	}
	for i, wt := range want.Tabs {
		gt := got.Tabs[i]
		if gt.URL != wt.URL || gt.Title != wt.Title || gt.HistoryIndex != wt.HistoryIndex {
			t.Errorf("tab %d = %+v, want %+v", i, gt, wt)
		}
		if strings.Join(gt.History, "|") != strings.Join(wt.History, "|") {
			t.Errorf("tab %d history = %v, want %v", i, gt.History, wt.History)
		}
	}
}

func TestLoadMissingFileIsFreshState(t *testing.T) {
	got, err := session.Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	if len(got.Tabs) != 0 {
		t.Errorf("missing file yielded %d tabs, want 0", len(got.Tabs))
	}
}

func TestLoadCorruptFileIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Load(path); err == nil {
		t.Fatal("Load on corrupt file succeeded, want error")
	}
}

func TestSaveCreatesParentDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "nested", "session.json")
	if err := session.Save(path, session.State{}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("state file missing after Save: %v", err)
	}
}

func TestSaveIsAtomicNotTruncated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	if err := session.Save(path, session.State{Tabs: []session.TabState{{URL: "https://x/"}}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "https://x/") {
		t.Errorf("state file missing saved URL; got %q", data)
	}
}

func TestLoadCapsTabsAndHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	tabs := make([]session.TabState, session.MaxTabs+10)
	for i := range tabs {
		hist := make([]string, session.MaxHistoryPerTab+10)
		for j := range hist {
			hist[j] = "https://h/"
		}
		tabs[i] = session.TabState{URL: "https://t/", History: hist, HistoryIndex: len(hist) - 1}
	}
	if err := session.Save(path, session.State{Tabs: tabs, Active: len(tabs) - 1}); err != nil {
		t.Fatal(err)
	}
	got, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tabs) != session.MaxTabs {
		t.Errorf("restored %d tabs, want capped at %d", len(got.Tabs), session.MaxTabs)
	}
	if got.Active >= len(got.Tabs) {
		t.Errorf("Active = %d out of range for %d tabs", got.Active, len(got.Tabs))
	}
	for i, tab := range got.Tabs {
		if len(tab.History) != session.MaxHistoryPerTab {
			t.Fatalf("tab %d history = %d entries, want capped at %d", i, len(tab.History), session.MaxHistoryPerTab)
		}
		if tab.HistoryIndex != session.MaxHistoryPerTab-1 {
			t.Errorf("tab %d HistoryIndex = %d, want %d", i, tab.HistoryIndex, session.MaxHistoryPerTab-1)
		}
	}
}

func TestLoadFixesOutOfRangeActive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	raw := `{"tabs":[{"url":"https://a/"},{"url":"https://b/"}],"active":99}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := session.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active != 1 {
		t.Errorf("Active = %d, want clamped to last tab (1)", got.Active)
	}
}
