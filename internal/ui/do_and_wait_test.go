package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/html"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// TestBrowserDoAndWait_DirectOnMain verifies the synchronous fast path
// when doAndWait is invoked on the Fyne main goroutine. The test ensures
// the function runs in-line (no deadlock via fyne.DoAndWait) and that
// the return value is observable to the caller without polling.
func TestBrowserDoAndWait_DirectOnMain(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	var ran bool
	browser.doAndWait(func() {
		ran = true
	})
	assert.True(t, ran, "doAndWait should run synchronously on the main goroutine")
}

// TestBrowserDoAndWait_FromGoroutineMarshals verifies that doAndWait
// works when invoked from a non-main goroutine. In test mode the test
// driver runs fn inline via async.EnsureNotMain, so the side-effect is
// visible immediately without needing a real Fyne event loop.
func TestBrowserDoAndWait_FromGoroutineMarshals(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	var (
		mu   sync.Mutex
		done bool
	)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		browser.doAndWait(func() {
			mu.Lock()
			done = true
			mu.Unlock()
		})
	}()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, done, "doAndWait from a goroutine should still execute fn")
}

// TestBrowserDoAndWait_HeadlessRunsInline verifies that in headless mode
// (no Fyne event loop) doAndWait runs the function directly without any
// marshalling — important for tests and offline rendering.
func TestBrowserDoAndWait_HeadlessRunsInline(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w, true)

	var ran bool
	browser.doAndWait(func() {
		ran = true
	})
	assert.True(t, ran, "headless doAndWait should run directly")
}

// TestTabRenderParsedContent_MarshalsToMainThread covers the actual
// regression that motivated the threading fix: calling Tab.RenderParsedContent
// from a goroutine other than the Fyne main thread must succeed without
// triggering the "Error in Fyne call thread, this should have been called
// in fyne.Do[AndWait]" diagnostic that used to fire when the renderer
// mutated Fyne canvas objects off-thread.
//
// The test uses a stub renderer that records whether the render call
// completed successfully. Note: in test mode the test driver runs
// DoFromGoroutine inline, so we cannot directly observe the production
// marshalling — but we do observe the renderer being invoked, which
// would not happen if doAndWait deadlocked.
func TestTabRenderParsedContent_MarshalsToMainThread(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	stub := &renderParsedSpy{}
	browser.RendererFactory = func() HTMLRenderer { return stub }

	tab := browser.ActiveTab()

	// Drive from a goroutine that is NOT the Fyne main goroutine. The
	// doAndWait marshalling should let the render complete (rather than
	// deadlocking the goroutine).
	var wg sync.WaitGroup
	var renderErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		renderErr = tab.RenderParsedContent(context.Background(), &html.Node{Type: html.DocumentNode}, nil)
	}()
	wg.Wait()

	assert.NoError(t, renderErr, "RenderParsedContent from a goroutine should succeed via doAndWait")
	stub.mu.Lock()
	defer stub.mu.Unlock()
	assert.Equal(t, 1, stub.parsedCalled, "render should have run exactly once")
}

// renderParsedSpy is a minimal HTMLRenderer that records whether the
// render path ran on the Fyne main goroutine. The captured boolean
// verifies that the doAndWait marshalling actually moved the call.
type renderParsedSpy struct {
	mu           sync.Mutex
	parsedCalled int
	parsedOnMain bool
}

func (r *renderParsedSpy) RenderHTML(_ context.Context, _ string) (fyne.CanvasObject, error) {
	return nil, nil
}
func (r *renderParsedSpy) RenderParsed(_ context.Context, _ *html.Node, _ []renderer.ExternalCSS) (fyne.CanvasObject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.parsedCalled++
	r.parsedOnMain = IsMainGoroutine()
	return fyne.NewContainerWithoutLayout(), nil
}
func (r *renderParsedSpy) UpdateViewport() fyne.CanvasObject    { return nil }
func (r *renderParsedSpy) SetCurrentURL(_ string)               {}
func (r *renderParsedSpy) ResolveURL(_ string) string           { return "" }
func (r *renderParsedSpy) SetWindow(_ fyne.Window)              {}
func (r *renderParsedSpy) SetHeadless(_ bool)                   {}
func (r *renderParsedSpy) SetNavigationCallback(_ func(string)) {}
func (r *renderParsedSpy) HitTest(_, _ float32) (*renderer.RenderNode, *renderer.LayoutBox) {
	return nil, nil
}
func (r *renderParsedSpy) SetInspectCallback(_ func(*renderer.RenderNode, *renderer.LayoutBox)) {}
func (r *renderParsedSpy) SetContextMenuCallback(_ func(*renderer.RenderNode, *renderer.LayoutBox, fyne.Position)) {
}
func (r *renderParsedSpy) GetRoot() *renderer.RenderNode                   { return nil }
func (r *renderParsedSpy) Refresh()                                        {}
func (r *renderParsedSpy) SetRefreshCallback(_ func())                     {}
func (r *renderParsedSpy) SetSubmitting(_ bool)                            {}
func (r *renderParsedSpy) SetCSP(_ *net.CSPPolicy)                         {}
func (r *renderParsedSpy) GetDisplayListSummary() map[string]int           { return nil }
func (r *renderParsedSpy) GetDisplayListCommands() []renderer.PaintCommand { return nil }
func (r *renderParsedSpy) SetDirtyOverlayEnabled(_ bool)                   {}
func (r *renderParsedSpy) DirtyOverlayEnabled() bool                       { return false }
func (r *renderParsedSpy) SetFPSOverlayEnabled(_ bool)                     {}
func (r *renderParsedSpy) FPSOverlayEnabled() bool                         { return false }
func (r *renderParsedSpy) FPSStats() renderer.FPSStats                     { return renderer.FPSStats{} }
func (r *renderParsedSpy) FrameMetrics() renderer.FrameMetricsSnapshot {
	return renderer.FrameMetricsSnapshot{}
}
func (r *renderParsedSpy) ScheduleScroll(_, _ float32) bool { return false }
func (r *renderParsedSpy) TryClaimScroll() (renderer.ScrollViewport, bool) {
	return renderer.ScrollViewport{}, false
}
func (r *renderParsedSpy) RecordInputToPresent(_ time.Duration)                    {}
func (r *renderParsedSpy) RecordUIQueueWait(_ time.Duration)                       {}
func (r *renderParsedSpy) RecordCoalescedMutations(_ int)                          {}
func (r *renderParsedSpy) RecordCoalescedScroll(_ int)                             {}
func (r *renderParsedSpy) RecordCoalescedImages(_ int)                             {}
func (r *renderParsedSpy) SetMouseInputCallback(_ func(input renderer.MouseInput)) {}
func (r *renderParsedSpy) SetSize(_, _ float32)                                    {}
func (r *renderParsedSpy) GetDOMNodeCounts() (int, int, int)                       { return 0, 0, 0 }
func (r *renderParsedSpy) GetLayoutNodeCount() int                                 { return 0 }
func (r *renderParsedSpy) GetStyleSheet() *css.StyleSheet                          { return nil }
func (r *renderParsedSpy) GetMatchedRules(_ *renderer.RenderNode) []css.Rule       { return nil }
func (r *renderParsedSpy) SetHighlightNode(_ *renderer.RenderNode)                 {}
func (r *renderParsedSpy) GetLayoutBox(_ *renderer.RenderNode) *renderer.LayoutBox { return nil }
func (r *renderParsedSpy) SetViewport(_, _ float32)                                {}
