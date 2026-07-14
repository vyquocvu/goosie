package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanvasRendererDirtyOverlayDefaultDisabled(t *testing.T) {
	cr := NewCanvasRenderer(800, 600)
	assert.False(t, cr.DirtyOverlayEnabled())
}

func TestCanvasRendererSetDirtyOverlayEnabled(t *testing.T) {
	cr := NewCanvasRenderer(800, 600)

	cr.SetDirtyOverlayEnabled(true)
	assert.True(t, cr.DirtyOverlayEnabled())

	cr.SetDirtyOverlayEnabled(false)
	assert.False(t, cr.DirtyOverlayEnabled())
}

func TestCanvasRendererSetDirtyOverlaySameValueNoOp(t *testing.T) {
	cr := NewCanvasRenderer(800, 600)
	genBefore := cr.dlBuildGen

	// Setting to same value should not bump generation
	cr.SetDirtyOverlayEnabled(false)
	assert.Equal(t, genBefore, cr.dlBuildGen)
}

func TestCanvasRendererSetDirtyOverlayInvalidatesCache(t *testing.T) {
	cr := NewCanvasRenderer(800, 600)

	// Set cached display list
	cr.cachedDisplayList = &DisplayList{}

	// Toggle on — should clear cache
	cr.SetDirtyOverlayEnabled(true)
	assert.Nil(t, cr.cachedDisplayList)
}

func TestCommandTypeToOverlayColorAllTypes(t *testing.T) {
	types := []PaintCommandType{
		PaintText,
		PaintRect,
		PaintImage,
		PaintLink,
		PaintBorder,
		PaintButton,
		PaintInput,
		PaintTextarea,
	}

	for _, ct := range types {
		clr := CommandTypeToOverlayColor(ct)
		assert.NotNil(t, clr, "color should not be nil for type %d", ct)
	}
}

func TestCommandTypeToOverlayColorUnknown(t *testing.T) {
	clr := CommandTypeToOverlayColor(PaintCommandType(99))
	assert.NotNil(t, clr)
}
