package history_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/history"
)

func TestHistoryRecordAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := history.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s.Record("https://example.com", "Example")
	s.Record("https://other.example", "Other")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s2, err := history.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := s2.Entries()
	if len(got) != 2 {
		t.Fatalf("Entries() = %d records, want 2", len(got))
	}
	if got[0].URL != "https://example.com" || got[0].Title != "Example" {
		t.Fatalf("Entries()[0] = %+v, want the example.com visit", got[0])
	}
	if got[0].At.IsZero() {
		t.Fatal("Entries()[0].At is zero, want a visit timestamp")
	}
}

func TestHistoryDropsConsecutiveRepeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Record("https://example.com", "Example")
	s.Record("https://example.com", "Example")
	if got := s.Entries(); len(got) != 1 {
		t.Fatalf("Entries() = %d records, want 1 after a consecutive repeat", len(got))
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Entries(); len(got) != 1 || got[0].URL != "https://example.com" {
		t.Fatalf("reloaded consecutive visits = %+v, want one example.com visit", got)
	}
}

func TestHistoryCapsEntries(t *testing.T) {
	s, err := history.Load(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < history.MaxEntries+100; i++ {
		s.Record("https://page.example/"+string(rune('a'+i%26))+"/"+time.Unix(int64(i), 0).String(), "")
	}
	got := s.Entries()
	if len(got) != history.MaxEntries {
		t.Fatalf("Entries() = %d records, want the cap %d", len(got), history.MaxEntries)
	}
	if got[len(got)-1].At.Before(got[0].At) {
		t.Fatal("Entries() not ordered oldest-first after capping")
	}
}

func TestHistoryLoadMissingFileIsEmptyStore(t *testing.T) {
	s, err := history.Load(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("Load of a missing file = %v, want an empty store", err)
	}
	if len(s.Entries()) != 0 {
		t.Fatalf("Entries() = %+v, want empty", s.Entries())
	}
}

func TestHistoryLoadCorruptFileIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := history.Load(path); err == nil {
		t.Fatal("Load of a corrupt file succeeded, want an error")
	}
}

// TestHistoryLoadCapsOversizedFile: a legacy file past the cap loads only
// the newest MaxEntries visits, matching what Record keeps at runtime.
func TestHistoryLoadCapsOversizedFile(t *testing.T) {
	type entry struct {
		URL   string    `json:"url"`
		Title string    `json:"title"`
		At    time.Time `json:"at"`
	}
	var list []entry
	for i := 0; i < history.MaxEntries+100; i++ {
		list = append(list, entry{URL: "https://page.example/" + time.Unix(int64(i), 0).String(), At: time.Unix(int64(i), 0)})
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := history.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(s.Entries()); got != history.MaxEntries {
		t.Fatalf("loaded %d visits, want the cap %d (newest kept)", got, history.MaxEntries)
	}
}

func TestHistoryDeleteEntryAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := history.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	s.Record("https://a.example", "A")
	s.Record("https://b.example", "B")
	s.Record("https://a.example", "A again")
	entries := s.Entries()
	if len(entries) != 3 {
		t.Fatalf("Entries() = %d, want 3", len(entries))
	}

	// Delete the exact first visit; the later same-URL visit survives.
	if !s.DeleteEntry("https://a.example", entries[0].At) {
		t.Fatal("DeleteEntry of a recorded visit = false, want true")
	}
	if s.DeleteEntry("https://a.example", entries[0].At) {
		t.Fatal("second DeleteEntry of the same visit = true, want false")
	}
	if s.DeleteEntry("https://never.example", entries[0].At) {
		t.Fatal("DeleteEntry of an unknown URL = true, want false")
	}
	if s.DeleteEntry("https://b.example", entries[0].At.Add(time.Second)) {
		t.Fatal("DeleteEntry with a mismatched timestamp = true, want false")
	}

	got := s.Entries()
	if len(got) != 2 || got[0].URL != "https://b.example" || got[1].URL != "https://a.example" {
		t.Fatalf("Entries() = %+v, want b then the surviving a visit", got)
	}

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s2, err := history.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := s2.Entries(); len(got) != 2 || got[0].URL != "https://b.example" {
		t.Fatalf("reloaded Entries() = %+v, want the deletion persisted", got)
	}
}

// TestHistoryDeleteEntryTimestampRoundTrip pins the encoding the management UI
// uses: a timestamp serialized as RFC3339Nano (and through JSON) must still
// match the recorded visit exactly.
func TestHistoryDeleteEntryTimestampRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Record("https://a.example", "A")
	at := s.Entries()[0].At

	// Through the UI's query-parameter encoding.
	parsed, err := time.Parse(time.RFC3339Nano, at.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("RFC3339Nano round trip: %v", err)
	}
	// Through JSON persistence: save, reload, and delete on the reloaded
	// store using the URL-encoded timestamp.
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s2, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.DeleteEntry("https://a.example", parsed) {
		t.Fatal("DeleteEntry with an RFC3339Nano round-tripped timestamp = false, want true")
	}
}

func TestHistoryClearAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Record("https://a.example", "A")
	s.Record("https://b.example", "B")
	s.Clear()
	if got := s.Entries(); len(got) != 0 {
		t.Fatalf("Entries() after Clear = %+v, want empty", got)
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s2, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Entries(); len(got) != 0 {
		t.Fatalf("reloaded Entries() after Clear = %+v, want empty", got)
	}
	// The log still records after a clear.
	s2.Record("https://c.example", "C")
	if got := s2.Entries(); len(got) != 1 {
		t.Fatalf("Entries() after Clear+Record = %+v, want the new visit", got)
	}
}

func TestHistorySearch(t *testing.T) {
	s, err := history.Load(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Record("https://golang.org/doc", "Go Documentation")
	s.Record("https://example.com", "Example Domain")
	s.Record("https://blog.example.net", "")

	if got := s.Search(""); len(got) != 3 {
		t.Fatalf("Search(\"\") = %d entries, want all 3", len(got))
	}
	got := s.Search("golang")
	if len(got) != 1 || got[0].URL != "https://golang.org/doc" {
		t.Fatalf("Search(golang) = %+v, want the golang.org visit", got)
	}
	if got := s.Search("EXAMPLE"); len(got) != 2 {
		t.Fatalf("Search(EXAMPLE) = %d entries, want 2 (title + URL matches)", len(got))
	}
	if got := s.Search("documentation"); len(got) != 1 || got[0].Title != "Go Documentation" {
		t.Fatalf("Search(documentation) = %+v, want the title match", got)
	}
	if got := s.Search("nomatchxyz"); len(got) != 0 {
		t.Fatalf("Search(nomatchxyz) = %+v, want empty", got)
	}
	// Oldest-first order is preserved.
	got = s.Search("example")
	if got[0].URL != "https://example.com" || got[1].URL != "https://blog.example.net" {
		t.Fatalf("Search(example) order = %+v, want oldest first", got)
	}
}
