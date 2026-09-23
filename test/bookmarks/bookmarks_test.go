package bookmarks_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/bookmarks"
)

func TestBookmarkToggleAndContains(t *testing.T) {
	s := bookmarks.NewStore(filepath.Join(t.TempDir(), "bookmarks.json"))
	if s.Contains("https://example.com") {
		t.Fatal("fresh store contains example.com")
	}
	if added := s.Toggle("https://example.com", "Example"); !added {
		t.Fatal("first Toggle = false, want true (added)")
	}
	if !s.Contains("https://example.com") {
		t.Fatal("Contains after Toggle = false, want true")
	}
	if added := s.Toggle("https://example.com", "Example"); added {
		t.Fatal("second Toggle = true, want false (removed)")
	}
	if s.Contains("https://example.com") {
		t.Fatal("Contains after second Toggle = true, want false")
	}
}

func TestBookmarkListOrder(t *testing.T) {
	s := bookmarks.NewStore(filepath.Join(t.TempDir(), "bookmarks.json"))
	s.Toggle("https://a.example", "A")
	s.Toggle("https://b.example", "B")
	s.Toggle("https://c.example", "C")
	got := s.List()
	if len(got) != 3 || got[0].URL != "https://a.example" || got[2].URL != "https://c.example" {
		t.Fatalf("List() = %+v, want insertion order a, b, c", got)
	}
}

func TestBookmarkSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.json")
	s := bookmarks.NewStore(path)
	s.Toggle("https://example.com", "Example")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s2, err := bookmarks.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !s2.Contains("https://example.com") {
		t.Fatal("reloaded store lost the bookmark")
	}
	got := s2.List()
	if len(got) != 1 || got[0].URL != "https://example.com" || got[0].Title != "Example" {
		t.Fatalf("reloaded List() = %+v, want the saved entry", got)
	}
}

func TestBookmarksLoadMissingFileIsEmptyStore(t *testing.T) {
	s, err := bookmarks.Load(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("Load of a missing file = %v, want an empty store", err)
	}
	if len(s.List()) != 0 {
		t.Fatalf("List() = %+v, want empty", s.List())
	}
}

func TestBookmarksLoadCorruptFileIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := bookmarks.Load(path); err == nil {
		t.Fatal("Load of a corrupt file succeeded, want an error")
	}
}

func TestBookmarkSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := bookmarks.Load(filepath.Join(dir, "bookmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Toggle("https://example.com", "Example")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if name := e.Name(); name != "bookmarks.json" {
			t.Errorf("Save left %q behind; want only bookmarks.json", name)
		}
	}
}
