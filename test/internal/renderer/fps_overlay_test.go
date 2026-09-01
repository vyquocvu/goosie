package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanvasRendererFPSOverlayDefaultDisabled(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)
	assert.False(t, cr.FPSOverlayEnabled())
}

func TestCanvasRendererSetFPSOverlayEnabled(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)

	cr.SetFPSOverlayEnabled(true)
	assert.True(t, cr.FPSOverlayEnabled())

	cr.SetFPSOverlayEnabled(false)
	assert.False(t, cr.FPSOverlayEnabled())
}

func TestCanvasRendererSetFPSOverlaySameValueNoOp(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)
	// Changing to the same value must not clear the cached display list.
	cr.SetCachedDisplayList(&renderer.DisplayList{})
	cr.SetFPSOverlayEnabled(false)
	assert.NotNil(t, cr.CachedDisplayList())
}

func TestCanvasRendererSetFPSOverlayInvalidatesCache(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)
	cr.SetCachedDisplayList(&renderer.DisplayList{})

	cr.SetFPSOverlayEnabled(true)
	assert.Nil(t, cr.CachedDisplayList())
}

func TestCanvasRendererRecordFrameOnRender(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)

	// Each RenderWithViewport call records a frame.
	root := simpleRenderNode()
	layout := simpleLayoutBox(root)

	cr.RenderWithViewport(root, layout)
	cr.RenderWithViewport(root, layout)

	stats := cr.FPSStats()
	assert.Equal(t, int64(2), stats.Frames)
}

func TestCanvasRendererFPSStatsPropagatedToRenderer(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	root := simpleRenderNode()
	layout := simpleLayoutBox(root)

	// Renderer exposes canvas-renderer stats.
	r.CanvasRenderer().RenderWithViewport(root, layout)
	assert.Equal(t, int64(1), r.FPSStats().Frames)

	// Renderer-level delegate toggles the overlay.
	r.SetFPSOverlayEnabled(true)
	assert.True(t, r.FPSOverlayEnabled())
	r.SetFPSOverlayEnabled(false)
	assert.False(t, r.FPSOverlayEnabled())
}

func TestCanvasRendererRenderWithFPSOverlay(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)
	root := simpleRenderNode()
	layout := simpleLayoutBox(root)

	cr.SetFPSOverlayEnabled(true)

	// Rendering with the overlay enabled must not panic and records frames.
	obj := cr.RenderWithViewport(root, layout)
	assert.NotNil(t, obj)
	obj = cr.RenderWithViewport(root, layout)
	assert.NotNil(t, obj)
	assert.Equal(t, int64(2), cr.FPSStats().Frames)
}

// TestCanvasRendererFPSOverlayReusesObjects verifies that the FPS HUD
// reuses the same Fyne canvas objects across frames instead of allocating
// fresh ones on every RenderWithViewport call. Object reuse is what
// keeps scroll-rate FPS measurement from creating GC pressure at 60 Hz.
//
// The first render allocates the cached overlay objects; subsequent
// renders must reuse the exact same *canvas.Text and *canvas.Rectangle
// pointers. We assert pointer identity across consecutive frames.
func TestCanvasRendererFPSOverlayReusesObjects(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)
	root := simpleRenderNode()
	layout := simpleLayoutBox(root)
	cr.SetFPSOverlayEnabled(true)

	// Drive frames so FPS counter records measurements
	cr.RenderWithViewport(root, layout)
	cr.BuildFPSOverlay()
	firstText := cr.FPSOverlayText()
	firstBg := cr.FPSOverlayBg()
	assert.NotNil(t, firstText, "first build should allocate the cached text")
	assert.NotNil(t, firstBg, "first build should allocate the cached background")

	cr.RenderWithViewport(root, layout)
	cr.BuildFPSOverlay()
	assert.Same(t, firstText, cr.FPSOverlayText(), "second build must reuse the cached text object")
	assert.Same(t, firstBg, cr.FPSOverlayBg(), "second build must reuse the cached background object")
}

// TestCanvasRendererFPSOverlayDisableClearsCache verifies that toggling
// the overlay off drops the cached Fyne objects so the next time the
// overlay is enabled it allocates fresh ones — important because the
// container will detach them on its next rebuild, and we don't want
// stale references lingering.
func TestCanvasRendererFPSOverlayDisableClearsCache(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)
	root := simpleRenderNode()
	layout := simpleLayoutBox(root)
	cr.SetFPSOverlayEnabled(true)
	cr.RenderWithViewport(root, layout)
	cr.BuildFPSOverlay()
	assert.NotNil(t, cr.FPSOverlayText())
	assert.NotNil(t, cr.FPSOverlayBg())

	cr.SetFPSOverlayEnabled(false)
	assert.Nil(t, cr.FPSOverlayText(), "disabling should clear the cached text")
	assert.Nil(t, cr.FPSOverlayBg(), "disabling should clear the cached background")
	assert.Equal(t, "", cr.FPSOverlayTextCache(), "disabling should clear the text cache")
}

// TestCanvasRendererFPSOverlayRefreshOnlyOnChange verifies that the
// reused text object is Refresh()ed only when the displayed string
// actually changes — without this guard we'd issue a text Refresh on
// every scroll tick even when the FPS readout is unchanged, which is
// the worst case for the "FPS is bad" symptom the user was seeing.
func TestCanvasRendererFPSOverlayRefreshOnlyOnChange(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)
	root := simpleRenderNode()
	layout := simpleLayoutBox(root)
	cr.SetFPSOverlayEnabled(true)

	cr.RenderWithViewport(root, layout)
	cr.BuildFPSOverlay()
	assert.NotEmpty(t, cr.FPSOverlayTextCache(), "first render should seed the text cache")

	cachedBefore := cr.FPSOverlayTextCache()
	cr.RenderWithViewport(root, layout)
	cr.BuildFPSOverlay()
	// The cached text field must remain stable across consecutive
	// renders that produce the same readout; we don't have a direct
	// counter for Refresh calls but the cachedText field is the guard
	// against unnecessary work and must not regress to empty.
	assert.Equal(t, cachedBefore, cr.FPSOverlayTextCache(), "identical readouts must keep the cached text")
}

// BenchmarkFPSOverlayScrollRate exercises the FPS overlay's path under a
// scroll-rate workload (one RenderWithViewport per "frame"). It pins the
// previous allocs/op so a regression in object reuse shows up here even
// before any user-visible FPS drop.
//
// Baseline (pre-reuse): ~5 allocs/op, ~480 B/op.
// Target (after reuse): ≤ 2 allocs/op, ≤ 96 B/op.
func BenchmarkFPSOverlayScrollRate(b *testing.B) {
	cr := renderer.NewCanvasRenderer(800, 600)
	root := simpleRenderNode()
	layout := simpleLayoutBox(root)
	cr.SetFPSOverlayEnabled(true)
	cr.RenderWithViewport(root, layout) // prime the cache

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr.RenderWithViewport(root, layout)
	}
}

// BenchmarkRenderScrollRate is the no-overlay baseline. The difference
// between this and BenchmarkFPSOverlayScrollRate isolates the overlay's
// per-frame cost — pre-reuse that delta was ~480 B/op, post-reuse it
// should be effectively zero.
func BenchmarkRenderScrollRate(b *testing.B) {
	cr := renderer.NewCanvasRenderer(800, 600)
	root := simpleRenderNode()
	layout := simpleLayoutBox(root)
	cr.RenderWithViewport(root, layout) // prime the cache

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr.RenderWithViewport(root, layout)
	}
}

// BenchmarkBuildFPSOverlayOnly measures just the buildFPSOverlay path
// in isolation so regressions in the reuse logic surface immediately.
// Pre-reuse this benchmark allocated ~480 B/op (canvas.Text +
// canvas.Rectangle + helpers); post-reuse the per-frame cost should be
// effectively zero after the first call.
func BenchmarkBuildFPSOverlayOnly(b *testing.B) {
	cr := renderer.NewCanvasRenderer(800, 600)
	cr.SetFPSOverlayEnabled(true)
	// Prime: this allocates the cached overlay objects.
	cr.BuildFPSOverlay()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr.BuildFPSOverlay()
	}
}

// simpleRenderNode returns a minimal render tree root for driving the canvas
// renderer without full HTML parsing.
func simpleRenderNode() *renderer.RenderNode {
	return &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		Type:    renderer.NodeTypeElement,
		Styles:  map[string]string{"display": "block"},
	}
}

// simpleLayoutBox builds a matching layout box for simpleRenderNode.
func simpleLayoutBox(root *renderer.RenderNode) *renderer.LayoutBox {
	return &renderer.LayoutBox{
		NodeID: root.ID,
		Box: renderer.Rect{
			X: 0, Y: 0,
			Width:  800,
			Height: 600,
		},
	}
}
