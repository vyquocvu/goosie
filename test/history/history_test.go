package history_test

import (
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
