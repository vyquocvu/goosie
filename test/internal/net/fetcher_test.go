package net_test

import (
	"context"
	"testing"
	"time"
	"github.com/vyquocvu/goosie/internal/net"
)

func TestFetcher(t *testing.T) {
	fetcher := net.NewFetcher()
	if fetcher == nil {
		t.Fatal("NewFetcher() returned nil")
	}
	if fetcher.Client() == nil {
		t.Fatal("Fetcher client is nil")
	}
}

func TestFetchInvalidURL(t *testing.T) {
	fetcher := net.NewFetcher()
	_, err := fetcher.Fetch("not-a-url")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}

func TestFetchWithContextCancellation(t *testing.T) {
	fetcher := net.NewFetcher()

	// Create a context that's immediately cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := fetcher.FetchWithContext(ctx, "https://example.com", nil)
	if err == nil {
		t.Error("Expected error for cancelled context, got nil")
	}
}

func TestFetchWithContextTimeout(t *testing.T) {
	fetcher := net.NewFetcher()

	// Create a context with a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait a moment to ensure timeout
	time.Sleep(10 * time.Millisecond)

	_, err := fetcher.FetchWithContext(ctx, "https://example.com", nil)
	if err == nil {
		t.Error("Expected error for timed out context, got nil")
	}
}
