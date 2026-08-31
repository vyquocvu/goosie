package net_test

import "github.com/vyquocvu/goosie/internal/net"

import "testing"

func TestRequestLogConstructorReturnsInitializedEmptyLog(t *testing.T) {
	log := net.NewRequestLog()
	if log == nil {
		t.Fatal("NewRequestLog returned nil")
	}
	if entries := log.Entries(); len(entries) != 0 {
		t.Fatalf("initial entries = %d, want 0", len(entries))
	}

	log.Add(net.RequestLogEntry{URL: "https://example.test/"})
	if entries := log.Entries(); len(entries) != 1 {
		t.Fatalf("entries after Add = %d, want 1", len(entries))
	}
}
