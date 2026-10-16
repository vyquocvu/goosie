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
	s, err := history.Load(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Record("https://example.com", "Example")
	s.Record("https://example.com", "Example")
	if got := s.Entries(); len(got) != 1 {
		t.Fatalf("Entries() = %d records, want 1 after a consecutive repeat", len(got))
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
