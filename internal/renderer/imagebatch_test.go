package renderer

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImageLoadBatcher_CoalescesBurst — PR7: 100 image completions within
// one window produce exactly one flush carrying all 100 sources.
func TestImageLoadBatcher_CoalescesBurst(t *testing.T) {
	flushed := make(chan []string, 1)
	b := NewImageLoadBatcher(10*time.Millisecond, func(srcs []string) {
		flushed <- srcs
	})
	defer b.Close()

	for i := 0; i < 100; i++ {
		b.Signal(fmt.Sprintf("img%d.png", i))
	}

	select {
	case srcs := <-flushed:
		assert.Len(t, srcs, 100, "one flush must carry all 100 sources")
	case <-time.After(time.Second):
		t.Fatal("batcher never flushed the burst")
	}
	// A single batch must not produce a second flush.
	select {
	case <-flushed:
		t.Fatal("burst produced more than one flush")
	case <-time.After(60 * time.Millisecond):
	}

	batches, dropped := b.Metrics()
	assert.Equal(t, uint64(1), batches)
	assert.Equal(t, uint64(99), dropped)
}

// TestImageLoadBatcher_SeparateWindows — signals separated by more than
// the window produce distinct flushes.
func TestImageLoadBatcher_SeparateWindows(t *testing.T) {
	flushed := make(chan []string, 4)
	b := NewImageLoadBatcher(10*time.Millisecond, func(srcs []string) {
		flushed <- srcs
	})
	defer b.Close()

	b.Signal("a.png")
	select {
	case srcs := <-flushed:
		assert.Equal(t, []string{"a.png"}, srcs)
	case <-time.After(time.Second):
		t.Fatal("first window never flushed")
	}

	time.Sleep(20 * time.Millisecond) // ensure the next window is separate
	b.Signal("b.png")
	select {
	case srcs := <-flushed:
		assert.Equal(t, []string{"b.png"}, srcs)
	case <-time.After(time.Second):
		t.Fatal("second window never flushed")
	}

	batches, dropped := b.Metrics()
	assert.Equal(t, uint64(2), batches)
	assert.Equal(t, uint64(0), dropped)
}

// TestImageLoadBatcher_FlushForcesImmediate — Flush drains pending work
// without waiting for the window.
func TestImageLoadBatcher_FlushForcesImmediate(t *testing.T) {
	flushed := make(chan []string, 1)
	b := NewImageLoadBatcher(time.Hour, func(srcs []string) {
		flushed <- srcs
	})
	defer b.Close()

	b.Signal("a.png")
	b.Signal("b.png")
	b.Flush()

	select {
	case srcs := <-flushed:
		assert.Len(t, srcs, 2)
	case <-time.After(time.Second):
		t.Fatal("Flush did not drain pending work")
	}
}

// TestImageLoadBatcher_CloseRejectsSignals — after Close, signals are
// rejected and any pending work was flushed once.
func TestImageLoadBatcher_CloseRejectsSignals(t *testing.T) {
	flushed := make(chan []string, 1)
	b := NewImageLoadBatcher(time.Hour, func(srcs []string) {
		flushed <- srcs
	})

	b.Signal("a.png")
	b.Close() // flushes the pending batch
	select {
	case srcs := <-flushed:
		assert.Equal(t, []string{"a.png"}, srcs)
	case <-time.After(time.Second):
		t.Fatal("Close did not flush pending work")
	}

	b.Signal("b.png") // must be rejected
	select {
	case srcs := <-flushed:
		t.Fatalf("signal after Close was accepted: %v", srcs)
	case <-time.After(50 * time.Millisecond):
	}
}

// TestImageLoadBatcher_DeduplicatesSources — the same source signalled
// multiple times within a window appears once in the flush.
func TestImageLoadBatcher_DeduplicatesSources(t *testing.T) {
	flushed := make(chan []string, 1)
	b := NewImageLoadBatcher(10*time.Millisecond, func(srcs []string) {
		flushed <- srcs
	})
	defer b.Close()

	b.Signal("same.png")
	b.Signal("same.png")
	b.Signal("same.png")

	select {
	case srcs := <-flushed:
		assert.Equal(t, []string{"same.png"}, srcs)
	case <-time.After(time.Second):
		t.Fatal("batcher never flushed")
	}
	_, dropped := b.Metrics()
	// Two duplicate signals were collapsed into the batch.
	assert.Equal(t, uint64(2), dropped)
}

// TestRendererImageLoadsBatchSingleRender — PR7 integration: an
// image-heavy page completing 5 loads in a burst produces exactly one
// refresh (one style+layout+present cycle), not one per image, and the
// CoalescedImages metric records the batch size.
func TestRendererImageLoadsBatchSingleRender(t *testing.T) {
	loader := newMockImageLoader()
	r := NewRenderer(800, 600)
	r.imageLoader = loader
	r.SetCurrentURL("https://example.com/page")

	refreshes := make(chan struct{}, 16)
	r.SetRefreshCallback(func() {
		select {
		case refreshes <- struct{}{}:
		default:
		}
	})

	var imgs strings.Builder
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&imgs, `<img src="img%d.png">`, i)
	}
	_, err := r.RenderHTML(context.Background(), `<html><body>`+imgs.String()+`</body></html>`)
	require.NoError(t, err)

	// No immediate present: signals are batched, nothing refreshes yet.
	select {
	case <-refreshes:
		t.Fatal("image completion must not force an immediate present before the batch window")
	case <-time.After(5 * time.Millisecond):
	}

	// Complete all 5 loads in a burst.
	close(loader.loadChan)

	select {
	case <-refreshes:
	case <-time.After(time.Second):
		t.Fatal("expected one batched refresh after the burst")
	}
	// The burst must not produce further refreshes.
	select {
	case <-refreshes:
		t.Fatal("image burst produced more than one refresh")
	case <-time.After(80 * time.Millisecond):
	}

	require.NotNil(t, r.FrameMetrics)
	if got := r.FrameMetrics().CoalescedImages; got != 5 {
		t.Fatalf("CoalescedImages = %d, want 5", got)
	}
}

// TestCanvasImageCallbackDelegatesToRendererBatcher — PR12 regression:
// the canvas's onImageLoaded must route through the renderer's batched
// owner when a Renderer owns the canvas, so SetWindow (which registers
// the canvas callback on the loader's single slot) can never clobber the
// PR7 batching with one refresh per image.
func TestCanvasImageCallbackDelegatesToRendererBatcher(t *testing.T) {
	loader := newMockImageLoader()
	r := NewRenderer(800, 600)
	r.imageLoader = loader
	r.SetCurrentURL("https://example.com/page")

	if r.canvasRenderer.renderer != r {
		t.Fatal("NewRenderer must wire the canvas owner back-reference")
	}

	refreshes := make(chan struct{}, 16)
	r.SetRefreshCallback(func() {
		select {
		case refreshes <- struct{}{}:
		default:
		}
	})

	// No <img> nodes: RenderHTML's own loadImages pass fires no signals,
	// so every signal below comes from the delegation under test.
	_, err := r.RenderHTML(context.Background(), `<html><body><div>page</div></body></html>`)
	require.NoError(t, err)

	// A present already registered the renderer's batched callback; a
	// later SetWindow re-registers cr.onImageLoaded, which must delegate
	// to that same batched path rather than refreshing per image.
	r.canvasRenderer.onImageLoaded("img0.png")
	r.canvasRenderer.onImageLoaded("img1.png")

	// No immediate present: delegated signals are batched.
	select {
	case <-refreshes:
		t.Fatal("delegated completion must not force an immediate present before the batch window")
	case <-time.After(5 * time.Millisecond):
	}

	select {
	case <-refreshes:
	case <-time.After(time.Second):
		t.Fatal("expected one batched refresh after the delegated burst")
	}
	select {
	case <-refreshes:
		t.Fatal("delegated burst produced more than one refresh")
	case <-time.After(80 * time.Millisecond):
	}

	require.NotNil(t, r.FrameMetrics)
	if got := r.FrameMetrics().CoalescedImages; got != 2 {
		t.Fatalf("CoalescedImages = %d, want 2", got)
	}
}

// TestRendererImageLoadsFlushAppliesData — after the batch flush, every
// img node in the current render tree carries the loaded image data.
func TestRendererImageLoadsFlushAppliesData(t *testing.T) {
	loader := newMockImageLoader()
	r := NewRenderer(800, 600)
	r.imageLoader = loader
	r.SetCurrentURL("https://example.com/page")

	_, err := r.RenderHTML(context.Background(), `<html><body><img src="pic.png"></body></html>`)
	require.NoError(t, err)

	close(loader.loadChan)
	// Wait for the batcher window + flush. Read the render tree under
	// treeMu: the load goroutine writes node.ImageData concurrently.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r.treeMu.RLock()
		img := findFirstImg(r.GetRoot())
		loaded := img != nil && img.ImageData != nil
		r.treeMu.RUnlock()
		if loaded {
			return // data applied
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("image data was never applied to the render tree")
}

func findFirstImg(n *RenderNode) *RenderNode {
	if n == nil {
		return nil
	}
	if n.TagName == "img" {
		return n
	}
	for _, c := range n.Children {
		if found := findFirstImg(c); found != nil {
			return found
		}
	}
	return nil
}
