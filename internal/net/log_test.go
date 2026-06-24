package net

import "testing"

func TestRequestLogConstructorReturnsInitializedEmptyLog(t *testing.T) {
	log := NewRequestLog()
	if log == nil {
		t.Fatal("NewRequestLog returned nil")
	}
	if log.entries == nil {
		t.Fatal("NewRequestLog entries were nil")
	}
	if entries := log.Entries(); len(entries) != 0 {
		t.Fatalf("initial entries = %d, want 0", len(entries))
	}

	log.Add(RequestLogEntry{URL: "https://example.test/"})
	if entries := log.Entries(); len(entries) != 1 {
		t.Fatalf("entries after Add = %d, want 1", len(entries))
	}
}
