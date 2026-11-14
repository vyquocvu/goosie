package engine_test

import (
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func TestDeferredImagesSafeUnderReflow(t *testing.T) {
	pngBytes := pngOf(t, 640, 292)
	ready := make(chan struct{})
	fetcher := func(base, url string) ([]byte, error) {
		<-ready
		return pngBytes, nil
	}
	html := `<html><body>`
	for i := 0; i < 8; i++ {
		html += `<img src="/pic.png">`
	}
	html += `</body></html>`
	s, err := engine.NewSession(html, nil, 800,
		engine.WithMetrics(&fixedMetrics{advance: 7}),
		engine.WithDeferredImages("https://example.com/", fetcher))
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.LoadDeferredImages(func(int) { close(done) })
	}()
	close(ready)
loop:
	for {
		if err := s.Reflow(800); err != nil {
			t.Fatal(err)
		}
		select {
		case <-done:
			break loop
		default:
		}
	}
	wg.Wait()

	if err := s.Reflow(800); err != nil {
		t.Fatal(err)
	}
	ids := s.Doc.ElementsByTagName("img")
	if len(ids) == 0 {
		t.Fatal("no img elements")
	}
	id, ok := s.Arena.ForNode(ids[0].ID)
	if !ok {
		t.Fatal("img not laid out")
	}
	if h := s.Arena.Get(id).H; h < 291 || h > 293 {
		t.Fatalf("post-load img height = %v, want ~292", h)
	}
}
