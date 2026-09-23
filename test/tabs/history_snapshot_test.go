package tabs_test

import (
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestHistoryEntriesSnapshot(t *testing.T) {
	h := toolbar.NewHistory()
	h.Push("https://a/")
	h.Push("https://b/")
	h.Push("https://c/")
	h.Back()

	entries, idx := h.Entries()
	if !reflect.DeepEqual(entries, []string{"https://a/", "https://b/", "https://c/"}) {
		t.Errorf("Entries() = %v, want the three pushed URLs", entries)
	}
	if idx != 1 {
		t.Errorf("index = %d, want 1 after one Back", idx)
	}

	entries[0] = "mutated"
	if h.Current() != "https://b/" {
		t.Error("Entries() must return a copy, caller mutation leaked into history")
	}
}

func TestHistoryRoundTripThroughEntries(t *testing.T) {
	h := toolbar.NewHistory()
	h.Push("https://a/")
	h.Push("https://b/")
	h.Push("https://c/")
	h.Back()
	entries, idx := h.Entries()

	restored := toolbar.RestoreHistory(entries, idx)
	gotEntries, gotIdx := restored.Entries()
	if !reflect.DeepEqual(gotEntries, entries) || gotIdx != idx {
		t.Fatalf("restored = %v@%d, want %v@%d", gotEntries, gotIdx, entries, idx)
	}

	if got := restored.Current(); got != "https://b/" {
		t.Errorf("Current() = %q, want the saved position https://b/", got)
	}
	if url, ok := restored.Forward(); !ok || url != "https://c/" {
		t.Errorf("Forward() = %q,%v; forward trail must survive restore", url, ok)
	}
	if url, ok := restored.Back(); !ok || url != "https://b/" {
		t.Errorf("Back() = %q,%v; back trail must survive restore", url, ok)
	}
}

func TestRestoreHistoryClampsBadIndex(t *testing.T) {
	cases := []struct {
		name    string
		entries []string
		index   int
		wantIdx int
	}{
		{"index past end", []string{"https://a/"}, 5, 0},
		{"index very negative", []string{"https://a/"}, -9, -1},
		{"empty entries negative", nil, -1, -1},
		{"empty entries zero", nil, 0, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := toolbar.RestoreHistory(tc.entries, tc.index)
			_, idx := h.Entries()
			if idx != tc.wantIdx {
				t.Errorf("index = %d, want %d", idx, tc.wantIdx)
			}
			if h.Current() != "" && len(tc.entries) == 0 {
				t.Error("empty entries must yield no current URL")
			}
		})
	}
}

func TestRestoreHistoryEmpty(t *testing.T) {
	h := toolbar.RestoreHistory(nil, -1)
	if h.CanBack() || h.CanForward() {
		t.Error("empty restored history must have no traversal")
	}
	if h.Current() != "" {
		t.Errorf("Current() = %q, want empty", h.Current())
	}
}
