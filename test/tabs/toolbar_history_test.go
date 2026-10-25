package tabs_test

import (
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestToolbarAcceptsExternalHistory(t *testing.T) {
	tb := toolbar.NewState(800, nil)
	external := toolbar.NewHistory()
	external.Push("https://example.com")

	tb.SetHistory(external)

	if tb.History.Current() != "https://example.com" {
		t.Fatalf("toolbar should show external history URL, got %q", tb.History.Current())
	}
}

func TestToolbarNavigatePushesToCurrentHistory(t *testing.T) {
	tb := toolbar.NewState(800, nil)
	h := toolbar.NewHistory()
	tb.SetHistory(h)

	tb.Navigate("https://test.com")

	if h.Current() != "https://test.com" {
		t.Fatalf("navigate should push to current history, got %q", h.Current())
	}
}

// qc:BR-044
func TestHistoryPushAfterBackDiscardsForwardTrail(t *testing.T) {
	h := toolbar.NewHistory()
	for _, url := range []string{"https://a/", "https://b/", "https://c/"} {
		h.Push(url)
	}
	if got, ok := h.Back(); !ok || got != "https://b/" {
		t.Fatalf("Back() = %q,%t, want https://b/", got, ok)
	}
	baseEntries, base := h.Entries()
	if !h.Step(-1, base) {
		t.Fatal("Step(-1) = false, want successful traversal commit")
	}
	if h.Step(1, base) {
		t.Fatal("Step(1) committed with a stale cursor after another traversal moved it")
	}
	// Return to B, then push a new page. C must disappear from the forward trail.
	h.Step(1, base-1)
	h.Push("https://d/")
	entries, index := h.Entries()
	want := []string{"https://a/", "https://b/", "https://d/"}
	if !reflect.DeepEqual(entries, want) || index != 2 || h.CanForward() {
		t.Fatalf("history after branching = %v@%d forward=%t, want %v@2 forward=false (started from %v@%d)", entries, index, h.CanForward(), want, baseEntries, base)
	}
}
