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

// FR-DAT-02: a recorded visit carries its transition type and the running
// visit count for its URL, and both survive persistence.
func TestHistoryRecordsTransitionAndVisitCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s.RecordVisit("https://a.example", "A", history.TransitionTyped)
	s.RecordVisit("https://b.example", "B", history.TransitionLink)
	s.RecordVisit("https://a.example", "A again", history.TransitionLink)
	s.RecordVisit("https://a.example", "A reload", history.TransitionReload)

	// The final reload labels the visit already at the head rather than
	// appending a fourth entry (BR-045).
	want := []struct {
		url        string
		transition history.Transition
		visits     int
	}{
		{"https://a.example", history.TransitionTyped, 1},
		{"https://b.example", history.TransitionLink, 1},
		{"https://a.example", history.TransitionReload, 2},
	}
	assertVisits := func(entries []history.Entry) {
		t.Helper()
		if len(entries) != len(want) {
			t.Fatalf("Entries() = %d records, want %d", len(entries), len(want))
		}
		for i, w := range want {
			if entries[i].URL != w.url || entries[i].Transition != w.transition || entries[i].Visits != w.visits {
				t.Fatalf("Entries()[%d] = %+v, want %+v", i, entries[i], w)
			}
		}
	}
	assertVisits(s.Entries())

	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	assertVisits(reloaded.Entries())
}

// A reload of the page already at the head labels that visit instead of
// appending a duplicate (BR-045): reloading must not grow the visit log.
func TestHistoryReloadLabelsHeadVisitWithoutGrowingLog(t *testing.T) {
	s, err := history.Load(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.Record("https://a.example", "A")
	s.RecordVisit("https://a.example", "A", history.TransitionReload)
	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("Entries() = %d records, want 1 after a reload", len(entries))
	}
	if entries[0].Transition != history.TransitionReload || entries[0].Visits != 1 {
		t.Fatalf("head visit = %+v, want transition reload and unchanged visit count", entries[0])
	}
}

// FR-DAT-02: deletion is granular - the last hour, today, or all time.
func TestHistoryDeleteLastHourAndToday(t *testing.T) {
	now := time.Date(2026, 11, 2, 15, 0, 0, 0, time.Local)
	legacy := []map[string]any{
		{"url": "https://ancient.example", "title": "Ancient", "at": now.Add(-48 * time.Hour)},
		{"url": "https://yesterday.example", "title": "Yesterday", "at": now.Add(-26 * time.Hour)},
		{"url": "https://today.example", "title": "Today", "at": now.Add(-2 * time.Hour)},
		{"url": "https://boundary.example", "title": "Boundary", "at": now.Add(-time.Hour)},
		{"url": "https://recent.example", "title": "Recent", "at": now.Add(-10 * time.Minute)},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if removed := s.DeleteLastHour(now); removed != 2 {
		t.Fatalf("DeleteLastHour removed %d, want 2 (boundary is inclusive)", removed)
	}
	assertURLs := func(want ...string) {
		t.Helper()
		entries := s.Entries()
		if len(entries) != len(want) {
			t.Fatalf("Entries() = %d records, want %d", len(entries), len(want))
		}
		for i, url := range want {
			if entries[i].URL != url {
				t.Fatalf("Entries()[%d] = %q, want %q", i, entries[i].URL, url)
			}
		}
	}
	assertURLs("https://ancient.example", "https://yesterday.example", "https://today.example")

	if removed := s.DeleteToday(now); removed != 1 {
		t.Fatalf("DeleteToday removed %d, want 1", removed)
	}
	assertURLs("https://ancient.example", "https://yesterday.example")

	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := history.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(reloaded.Entries()); got != 2 {
		t.Fatalf("reloaded entries = %d, want the two surviving visits", got)
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

// TestHistoryFrecencyScore pins the ranking shared by the omnibox: frequency
// multiplies a recency bucket, so a busy page can outrank a fresher one and a
// stale one-off cannot float to the top.
func TestHistoryFrecencyScore(t *testing.T) {
	now := time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		visits int
		last   time.Time
		want   float64
	}{
		{"last hour", 1, now.Add(-30 * time.Minute), 4},
		{"last day", 1, now.Add(-2 * time.Hour), 2},
		{"last week", 1, now.Add(-3 * 24 * time.Hour), 1},
		{"last month", 1, now.Add(-14 * 24 * time.Hour), 0.5},
		{"older", 1, now.Add(-60 * 24 * time.Hour), 0.25},
		{"frequency multiplies", 3, now.Add(-2 * time.Hour), 6},
		{"future visit clamps to fresh", 2, now.Add(time.Hour), 8},
		{"no visits", 0, now, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := history.FrecencyScore(tc.visits, tc.last, now); got != tc.want {
				t.Fatalf("FrecencyScore(%d, %v) = %v, want %v", tc.visits, tc.last, got, tc.want)
			}
		})
	}
}
