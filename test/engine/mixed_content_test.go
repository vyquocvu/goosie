package engine_test

import (
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func TestMixedContentBlocking(t *testing.T) {
	var fetched int64
	fetcher := func(base, url string) ([]byte, error) {
		atomic.AddInt64(&fetched, 1)
		return nil, nil
	}
	html := `<img src="http://cdn.example.com/img.png">`
	sess, err := engine.NewSession(html, nil, 400,
		engine.WithImages("https://secure.example.com/page", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
	if n := atomic.LoadInt64(&fetched); n != 0 {
		t.Fatalf("mixed-content image was fetched %d times", n)
	}
}

func TestMixedContentAllowsSameScheme(t *testing.T) {
	var fetched int64
	fetcher := func(base, url string) ([]byte, error) {
		atomic.AddInt64(&fetched, 1)
		return nil, nil
	}
	html := `<img src="https://cdn.example.com/img.png">`
	sess, err := engine.NewSession(html, nil, 400,
		engine.WithImages("https://secure.example.com/page", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
	if n := atomic.LoadInt64(&fetched); n != 1 {
		t.Fatalf("same-scheme image fetch count = %d, want 1", n)
	}
}
