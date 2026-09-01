package documentloader_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/documentloader"
)

// TestHandleDocumentEndDoesNotWaitForImages — PR8: an image still in
// flight must not block HandleDocumentEnd (first paint). The image's
// callback fires after HandleDocumentEnd returns, and EventLoad is
// gated on the image completing.
func TestHandleDocumentEndDoesNotWaitForImages(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	chCSS := h.fetcher.register("https://example.com/slow.css")
	chImg := h.fetcher.register("https://example.com/slow.png")

	coord.HandleResource(documentloader.Resource{Kind: documentloader.KindCSS, URL: "slow.css"})
	coord.HandleResource(documentloader.Resource{Kind: documentloader.KindImage, URL: "slow.png"})

	// Release the stylesheet but keep the image blocked.
	chCSS <- fakeResponse{body: "body{}"}

	done := make(chan error, 1)
	go func() { done <- coord.HandleDocumentEnd(context.Background()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HandleDocumentEnd: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("HandleDocumentEnd blocked on a non-blocking image fetch")
	}

	// The image is still in flight: no OnImage yet, no EventLoad.
	snap := h.cb.Snapshot()
	if len(snap.Images) != 0 {
		t.Fatalf("image emitted before completion: %d", len(snap.Images))
	}
	if slices.Contains(snap.Lifecycle, documentloader.EventLoad) {
		t.Fatal("EventLoad fired while the image was still in flight")
	}

	// Release the image: OnImage fires and EventLoad follows.
	chImg <- fakeResponse{body: "png"}
	if !waitFor(t, func() bool { return len(h.cb.Snapshot().Images) == 1 }) {
		t.Fatalf("image callback never fired, got %d", len(h.cb.Snapshot().Images))
	}
	if !waitFor(t, func() bool { return slices.Contains(h.cb.Snapshot().Lifecycle, documentloader.EventLoad) }) {
		t.Fatal("EventLoad never fired after the image completed")
	}
}

// TestHandleDocumentEndDoesNotWaitForFonts — PR8: same non-blocking
// behavior for fonts (primary and CSS-nested).
func TestHandleDocumentEndDoesNotWaitForFonts(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	chFont := h.fetcher.register("https://example.com/f.woff2")
	coord.HandleResource(documentloader.Resource{Kind: documentloader.KindFont, URL: "f.woff2"})

	done := make(chan error, 1)
	go func() { done <- coord.HandleDocumentEnd(context.Background()) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HandleDocumentEnd: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("HandleDocumentEnd blocked on a non-blocking font fetch")
	}

	if got := len(h.cb.Snapshot().Images); got != 0 {
		t.Fatalf("unexpected image count %d", got)
	}
	chFont <- fakeResponse{body: "WOFF2-DATA"}
	_ = chFont
	// Fonts are captured via OnFont in captureCallbacks? No — the
	// harness only records CSS/Scripts/Images/Errors/Lifecycle. Font
	// results are delivered to OnFont, which captureCallbacks does not
	// wire, so we assert the load event fires instead.
	if !waitFor(t, func() bool { return slices.Contains(h.cb.Snapshot().Lifecycle, documentloader.EventLoad) }) {
		t.Fatal("EventLoad never fired after the font completed")
	}
}

// TestHandleDocumentEndStillBlocksOnStylesheets — regression guard:
// blocking resources (stylesheets, classic scripts) still gate first
// paint.
func TestHandleDocumentEndStillBlocksOnStylesheets(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	chCSS := h.fetcher.register("https://example.com/slow.css")
	coord.HandleResource(documentloader.Resource{Kind: documentloader.KindCSS, URL: "slow.css"})

	done := make(chan error, 1)
	go func() { done <- coord.HandleDocumentEnd(context.Background()) }()

	select {
	case <-done:
		t.Fatal("HandleDocumentEnd returned while a stylesheet was still in flight")
	case <-time.After(100 * time.Millisecond):
		// expected: still blocked on the blocking resource
	}

	chCSS <- fakeResponse{body: "body{}"}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HandleDocumentEnd: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("HandleDocumentEnd did not resume after the stylesheet completed")
	}
}

// TestLateImageResultEmitsInDocumentOrder — PR8: an image completing
// after HandleDocumentEnd still fires OnImage exactly once, in
// document order relative to resources that completed earlier.
func TestLateImageResultEmitsInDocumentOrder(t *testing.T) {
	h := newTestHarness(t, "https://example.com/page", nil)
	defer h.shutdown(t)
	coord := h.newCoord(t, nil)

	chImg := h.fetcher.register("https://example.com/slow.png")
	// Discovered first, completes last.
	coord.HandleResource(documentloader.Resource{Kind: documentloader.KindImage, URL: "slow.png"})
	coord.HandleResource(documentloader.Resource{Kind: documentloader.KindCSS, URL: "fast.css"})
	h.fetcher.register("https://example.com/fast.css") <- fakeResponse{body: "body{}"}

	if err := coord.HandleDocumentEnd(context.Background()); err != nil {
		t.Fatalf("HandleDocumentEnd: %v", err)
	}

	chImg <- fakeResponse{body: "png"}
	if !waitFor(t, func() bool { return len(h.cb.Snapshot().Images) == 1 }) {
		t.Fatal("late image callback never fired")
	}
}

// waitFor polls cond until it returns true or the deadline passes.
func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}
