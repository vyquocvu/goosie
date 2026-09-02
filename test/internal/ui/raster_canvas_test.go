package ui_test

import (
	"image"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/ui"
)

// TestRasterCanvas_Creation verifies that the canvas can be created.
func TestRasterCanvas_Creation(t *testing.T) {
	// Create a mock renderer (nil for now, just test creation)
	irc := ui.NewInteractiveRasterCanvas(nil)
	if irc == nil {
		t.Fatal("NewInteractiveRasterCanvas returned nil")
	}
}

// TestRasterCanvas_SetFrame verifies that SetFrame updates the display.
func TestRasterCanvas_SetFrame(t *testing.T) {
	irc := ui.NewInteractiveRasterCanvas(nil)

	// Create a test frame
	frame := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// Set the frame - should not panic
	irc.SetFrame(frame)

	// Verify the frame was set (indirectly via Refresh)
	// The actual rendering is handled by Fyne
}

// TestRasterCanvas_ScrollOffset verifies that scroll offset is tracked.
func TestRasterCanvas_ScrollOffset(t *testing.T) {
	irc := ui.NewInteractiveRasterCanvas(nil)

	// Set scroll offset
	irc.SetScrollOffset(100.0)

	// The offset is used internally for hit test coordinate conversion
	// We can't directly verify it without exposing it, but we ensure no panic
}

// TestRasterCanvas_Callbacks verifies that callbacks can be set.
func TestRasterCanvas_Callbacks(t *testing.T) {
	irc := ui.NewInteractiveRasterCanvas(nil)

	// Set callbacks - should not panic
	navigateCalled := false
	irc.SetNavigateCallback(func(url string) {
		navigateCalled = true
	})

	inspectCalled := false
	irc.SetInspectCallback(func(node *renderer.RenderNode, layout *renderer.LayoutBox) {
		inspectCalled = true
	})

	contextMenuCalled := false
	irc.SetContextMenuCallback(func(node *renderer.RenderNode, layout *renderer.LayoutBox, pos fyne.Position) {
		contextMenuCalled = true
	})

	// Callbacks are set but not triggered without actual events
	_ = navigateCalled
	_ = inspectCalled
	_ = contextMenuCalled
}

// TestRasterCanvas_Tapped verifies that Tapped handles clicks.
func TestRasterCanvas_Tapped(t *testing.T) {
	// Create a canvas with nil renderer (should handle gracefully)
	irc := ui.NewInteractiveRasterCanvas(nil)

	// Create a tap event
	event := &fyne.PointEvent{
		Position:         fyne.NewPos(50, 50),
		AbsolutePosition: fyne.NewPos(50, 50),
	}

	// Should not panic even with nil renderer
	irc.Tapped(event)
}

// TestRasterCanvas_TappedSecondary verifies right-click handling.
func TestRasterCanvas_TappedSecondary(t *testing.T) {
	irc := ui.NewInteractiveRasterCanvas(nil)

	event := &fyne.PointEvent{
		Position:         fyne.NewPos(50, 50),
		AbsolutePosition: fyne.NewPos(50, 50),
	}

	// Should not panic
	irc.TappedSecondary(event)
}

// TestRasterCanvas_Scrolled verifies scroll wheel handling.
func TestRasterCanvas_Scrolled(t *testing.T) {
	irc := ui.NewInteractiveRasterCanvas(nil)

	event := &fyne.ScrollEvent{
		Scrolled: fyne.NewDelta(0, -1), // Scroll down
	}

	// Should not panic
	irc.Scrolled(event)
}

// TestRasterCanvas_Focus verifies focus handling.
func TestRasterCanvas_Focus(t *testing.T) {
	irc := ui.NewInteractiveRasterCanvas(nil)

	// Should not panic
	irc.FocusGained()
	irc.FocusLost()
}

// TestRasterCanvas_TypedRune verifies keyboard input handling.
func TestRasterCanvas_TypedRune(t *testing.T) {
	irc := ui.NewInteractiveRasterCanvas(nil)

	// Should not panic
	irc.TypedRune('a')
}

// TestRasterCanvas_TypedKey verifies key event handling.
// Note: This test requires a Fyne app context to avoid nil pointer issues.
func TestRasterCanvas_TypedKey(t *testing.T) {
	// Skip this test as it requires a full Fyne app context
	// The TypedKey forwarding is tested via integration tests
	t.Skip("Requires Fyne app context - tested via integration")
}

// BenchmarkRasterCanvas_SetFrame measures frame update performance.
func BenchmarkRasterCanvas_SetFrame(b *testing.B) {
	irc := ui.NewInteractiveRasterCanvas(nil)
	frame := image.NewRGBA(image.Rect(0, 0, 1920, 1080))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		irc.SetFrame(frame)
	}
}
