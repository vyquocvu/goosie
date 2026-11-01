package toolbar_test

import (
	"fmt"
	"testing"

	"github.com/vyquocvu/goosie/internal/toolbar"
)

// FR-NAV-05: one tab's back/forward trail is capped at 50 navigation entries
// per session. Pushing past the cap drops the oldest visit, not the newest.
func TestHistoryCapsTrailAtFiftyEntries(t *testing.T) {
	h := toolbar.NewHistory()
	for i := 1; i <= 60; i++ {
		h.Push(fmt.Sprintf("https://example.com/%d", i))
	}
	entries, index := h.Entries()
	if len(entries) != toolbar.MaxHistoryEntries {
		t.Fatalf("entry count = %d, want %d", len(entries), toolbar.MaxHistoryEntries)
	}
	if index != toolbar.MaxHistoryEntries-1 {
		t.Fatalf("cursor = %d, want %d", index, toolbar.MaxHistoryEntries-1)
	}
	if got := h.Current(); got != "https://example.com/60" {
		t.Fatalf("current = %q, want the newest visit", got)
	}
	if entries[0] != "https://example.com/11" {
		t.Fatalf("oldest retained = %q, want the visit after the ten dropped", entries[0])
	}

	for i := 0; i < toolbar.MaxHistoryEntries-1; i++ {
		if _, ok := h.Back(); !ok {
			t.Fatalf("back stopped after %d steps", i)
		}
	}
	if h.CanBack() {
		t.Fatal("back is still available past the oldest retained visit")
	}
	if got := h.Current(); got != "https://example.com/11" {
		t.Fatalf("oldest current = %q, want the first retained visit", got)
	}
}

// A restored trail is also subject to the cap: saved state from an older
// build (or a hand-edited file) must not smuggle in more than fifty entries.
func TestRestoreHistoryClampsToCap(t *testing.T) {
	saved := make([]string, toolbar.MaxHistoryEntries+20)
	for i := range saved {
		saved[i] = fmt.Sprintf("https://example.com/%d", i+1)
	}
	h := toolbar.RestoreHistory(saved, len(saved)-1)
	entries, index := h.Entries()
	if len(entries) != toolbar.MaxHistoryEntries {
		t.Fatalf("restored entry count = %d, want %d", len(entries), toolbar.MaxHistoryEntries)
	}
	if index != toolbar.MaxHistoryEntries-1 {
		t.Fatalf("restored cursor = %d, want %d", index, toolbar.MaxHistoryEntries-1)
	}
	if entries[index] != saved[len(saved)-1] {
		t.Fatalf("restored current = %q, want the newest saved visit", entries[index])
	}
	if entries[0] != saved[len(saved)-toolbar.MaxHistoryEntries] {
		t.Fatalf("restored oldest = %q, want %q", entries[0], saved[len(saved)-toolbar.MaxHistoryEntries])
	}
}

// The cap must not resurrect truncated forward entries: a back-then-push
// discards the forward trail first, and the trimmed trail keeps its cursor on
// the newly pushed visit.
func TestHistoryCapKeepsCursorAfterForwardTruncation(t *testing.T) {
	h := toolbar.NewHistory()
	for i := 1; i <= toolbar.MaxHistoryEntries; i++ {
		h.Push(fmt.Sprintf("https://example.com/%d", i))
	}
	for i := 0; i < 10; i++ {
		if _, ok := h.Back(); !ok {
			t.Fatalf("back stopped after %d steps", i)
		}
	}
	h.Push("https://example.com/new")
	entries, index := h.Entries()
	if len(entries) != 41 {
		t.Fatalf("entry count = %d, want 41 after truncating ten forward entries", len(entries))
	}
	if index != 40 {
		t.Fatalf("cursor = %d, want 40", index)
	}
	if entries[index] != "https://example.com/new" {
		t.Fatalf("current = %q, want the pushed visit", entries[index])
	}
	if h.CanForward() {
		t.Fatal("forward trail survived the push")
	}
}
