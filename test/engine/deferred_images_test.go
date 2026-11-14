package engine_test

import (
	"bytes"
	"image"
	"image/png"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// qc:BR-084
func TestDeferredImagesPaintAfterLoad(t *testing.T) {
	pngBytes := pngOf(t, 640, 292)
	var fetched int64
	fetcher := func(base, url string) ([]byte, error) {
		atomic.AddInt64(&fetched, 1)
		return pngBytes, nil
	}

	html := `<html><body><img id="pic" src="/pic.png"></body></html>`
	s, err := engine.NewSession(html, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithDeferredImages("https://example.com/", fetcher))
	if err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt64(&fetched); n != 0 {
		t.Fatalf("fetched %d images during NewSession, want 0 (first paint must not wait)", n)
	}
	if p := s.DeferredImagesPending(); p != 1 {
		t.Fatalf("DeferredImagesPending = %d, want 1", p)
	}
	img := s.Doc.ElementByID("pic")
	id, ok := s.Arena.ForNode(img.ID)
	if !ok {
		t.Fatal("img not laid out")
	}
	if h := s.Arena.Get(id).H; h != 0 {
		t.Fatalf("pre-load img height = %v, want 0", h)
	}

	done := make(chan int, 1)
	s.LoadDeferredImages(func(loaded int) { done <- loaded })
	select {
	case loaded := <-done:
		if loaded != 1 {
			t.Fatalf("loaded = %d, want 1", loaded)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("LoadDeferredImages never reported")
	}
	if p := s.DeferredImagesPending(); p != 0 {
		t.Fatalf("DeferredImagesPending after load = %d, want 0", p)
	}
	if err := s.Reflow(800); err != nil {
		t.Fatal(err)
	}
	id, _ = s.Arena.ForNode(img.ID)
	if h := s.Arena.Get(id).H; h < 291 || h > 293 {
		t.Fatalf("post-load img height = %v, want ~292", h)
	}
}

func TestDeferredImagesWithoutLoadStaysQuiet(t *testing.T) {
	pngBytes := pngOf(t, 640, 292)
	s, err := engine.NewSession(`<html><body><img id="pic" src="/pic.png"></body></html>`, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithImages("https://example.com/", func(base, url string) ([]byte, error) { return pngBytes, nil }))
	if err != nil {
		t.Fatal(err)
	}
	if p := s.DeferredImagesPending(); p != 0 {
		t.Fatalf("DeferredImagesPending = %d, want 0 for the synchronous path", p)
	}
	img := s.Doc.ElementByID("pic")
	id, _ := s.Arena.ForNode(img.ID)
	if h := s.Arena.Get(id).H; h < 291 || h > 293 {
		t.Fatalf("img height = %v, want ~292 (synchronous fetch should already have applied)", h)
	}
	ready := make(chan int, 1)
	s.LoadDeferredImages(func(loaded int) { ready <- loaded })
	select {
	case loaded := <-ready:
		if loaded != 0 {
			t.Fatalf("loaded = %d, want 0 (nothing was deferred)", loaded)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("LoadDeferredImages on a synchronous session never reported")
	}
}
