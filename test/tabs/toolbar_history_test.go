package tabs_test

import (
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
