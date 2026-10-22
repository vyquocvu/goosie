package bookmarks_test

import (
	"encoding/json"
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

// TestBookmarksLoadBoundsHostileFile pins migration safety: a legacy or
// hand-edited file with duplicates, unnavigable empty URLs and far more
// entries than any real profile loads bounded, deduplicated and clean.
// Valid stores already satisfy all three, so ordinary profiles are untouched.
func TestBookmarksLoadBoundsHostileFile(t *testing.T) {
	type entry struct {
		URL   string `json:"url"`
		Title string `json:"title,omitempty"`
	}
	list := []entry{{URL: "", Title: "empty"}, {URL: "https://dup.example/", Title: "a"}, {URL: "https://dup.example/", Title: "b"}}
	for i := 0; i < bookmarks.MaxBookmarks+5; i++ {
		list = append(list, entry{URL: "https://page.example/" + string(rune('a'+i%26)) + "/" + string(rune('a'+(i/26)%26)) + "/" + string(rune('0'+i%10)) + "/" + string(rune('0'+(i/10)%10)) + "/" + string(rune('0'+(i/100)%10))})
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "bookmarks.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := bookmarks.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := s.List()
	if len(got) > bookmarks.MaxBookmarks {
		t.Fatalf("loaded %d bookmarks, want at most MaxBookmarks %d", len(got), bookmarks.MaxBookmarks)
	}
	seen := map[string]int{}
	for _, e := range got {
		if e.URL == "" {
			t.Fatal("loaded an empty-URL bookmark")
		}
		seen[e.URL]++
	}
	for url, n := range seen {
		if n > 1 {
			t.Fatalf("bookmark %q loaded %d times, want deduplicated", url, n)
		}
	}
}

func TestBookmarkDeleteAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.json")
	s := bookmarks.NewStore(path)
	s.Toggle("https://a.example", "A")
	s.Toggle("https://b.example", "B")
	if !s.Delete("https://a.example") {
		t.Fatal("Delete of an existing bookmark = false, want true")
	}
	if s.Delete("https://a.example") {
		t.Fatal("second Delete of the same bookmark = true, want false")
	}
	if s.Delete("https://never.example") {
		t.Fatal("Delete of an unknown bookmark = true, want false")
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s2, err := bookmarks.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := s2.List()
	if len(got) != 1 || got[0].URL != "https://b.example" {
		t.Fatalf("reloaded List() = %+v, want only the surviving b.example entry", got)
	}
}

func TestBookmarkRenameAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bookmarks.json")
	s := bookmarks.NewStore(path)
	s.Toggle("https://a.example", "Old Title")
	if !s.Rename("https://a.example", "New Title") {
		t.Fatal("Rename of an existing bookmark = false, want true")
	}
	if s.Rename("https://never.example", "X") {
		t.Fatal("Rename of an unknown bookmark = true, want false")
	}
	// Rename stores verbatim, including hostile markup: escaping is the
	// presentation layer's job.
	hostile := `<script>alert("xss")</script>`
	if !s.Rename("https://a.example", hostile) {
		t.Fatal("Rename with hostile title = false, want true")
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s2, err := bookmarks.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := s2.List()
	if len(got) != 1 || got[0].Title != hostile || got[0].URL != "https://a.example" {
		t.Fatalf("reloaded List() = %+v, want the renamed entry with verbatim title", got)
	}
}

func TestBookmarkSearch(t *testing.T) {
	s := bookmarks.NewStore(filepath.Join(t.TempDir(), "bookmarks.json"))
	s.Toggle("https://golang.org/doc", "Go Documentation")
	s.Toggle("https://example.com", "Example Domain")
	s.Toggle("https://blog.example.net", "")

	if got := s.Search(""); len(got) != 3 {
		t.Fatalf("Search(\"\") = %d entries, want all 3", len(got))
	}
	got := s.Search("golang")
	if len(got) != 1 || got[0].URL != "https://golang.org/doc" {
		t.Fatalf("Search(golang) = %+v, want the golang.org entry", got)
	}
	// Case-insensitive, matches title or URL.
	if got := s.Search("EXAMPLE"); len(got) != 2 {
		t.Fatalf("Search(EXAMPLE) = %d entries, want 2 (title + URL matches)", len(got))
	}
	if got := s.Search("documentation"); len(got) != 1 || got[0].Title != "Go Documentation" {
		t.Fatalf("Search(documentation) = %+v, want the title match", got)
	}
	if got := s.Search("nomatchxyz"); len(got) != 0 {
		t.Fatalf("Search(nomatchxyz) = %+v, want empty", got)
	}
	// Search results are copies: mutating them must not touch the store.
	got = s.Search("golang")
	got[0].Title = "mutated"
	if s.List()[0].Title != "Go Documentation" {
		t.Fatal("Search returned aliases into the store's entries")
	}
}
