package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanvasRendererDirtyOverlayDefaultDisabled(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)
	assert.False(t, cr.DirtyOverlayEnabled())
}

func TestCanvasRendererSetDirtyOverlayEnabled(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)

	cr.SetDirtyOverlayEnabled(true)
	assert.True(t, cr.DirtyOverlayEnabled())

	cr.SetDirtyOverlayEnabled(false)
	assert.False(t, cr.DirtyOverlayEnabled())
}

func TestCanvasRendererSetDirtyOverlaySameValueNoOp(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)
	genBefore := cr.DLBuildGen()

	// Setting to same value should not bump generation
	cr.SetDirtyOverlayEnabled(false)
	assert.Equal(t, genBefore, cr.DLBuildGen())
}

func TestCanvasRendererSetDirtyOverlayInvalidatesCache(t *testing.T) {
	cr := renderer.NewCanvasRenderer(800, 600)

	// Set cached display list
	cr.SetCachedDisplayList(&renderer.DisplayList{})

	// Toggle on — should clear cache
	cr.SetDirtyOverlayEnabled(true)
	assert.Nil(t, cr.CachedDisplayList())
}

func TestCommandTypeToOverlayColorAllTypes(t *testing.T) {
	types := []renderer.PaintCommandType{
		renderer.PaintText,
		renderer.PaintRect,
		renderer.PaintImage,
		renderer.PaintLink,
		renderer.PaintBorder,
		renderer.PaintButton,
		renderer.PaintInput,
		renderer.PaintTextarea,
	}

	for _, ct := range types {
		clr := renderer.CommandTypeToOverlayColor(ct)
		assert.NotNil(t, clr, "color should not be nil for type %d", ct)
	}
}

func TestCommandTypeToOverlayColorUnknown(t *testing.T) {
	clr := renderer.CommandTypeToOverlayColor(renderer.PaintCommandType(99))
	assert.NotNil(t, clr)
}
