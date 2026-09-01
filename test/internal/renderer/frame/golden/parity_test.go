package golden_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/golden"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

// TestBackendParity_SimpleRect verifies that CPU and CoreGraphics backends
// produce identical output for simple rectangles.
func TestBackendParity_SimpleRect(t *testing.T) {
	vp := frame.NewViewport(200, 200, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(10, 10, 50, 50),
			Color: frame.NewColor(255, 0, 0, 255),
		},
		{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(100, 100, 80, 60),
			Color: frame.NewColor(0, 255, 0, 255),
		},
	}

	result := golden.AssertBackendParity(t, "simple_rect", vp, cmds)
	if result == nil {
		t.Fatal("AssertBackendParity returned nil")
	}
	if result.CPUImage == nil {
		t.Fatal("CPU image is nil")
	}
	// BackendsMatch is true when CG is unavailable (trivially) or when both match.
	if !result.BackendsMatch {
		t.Errorf("backends do not match: %v", result.Comparison)
	}
}

// TestBackendParity_MultipleCommands tests parity with a more complex scene.
func TestBackendParity_MultipleCommands(t *testing.T) {
	vp := frame.NewViewport(400, 300, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		// Background.
		{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(0, 0, 400, 300),
			Color: frame.NewColor(240, 240, 240, 255),
		},
		// Red box.
		{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(50, 50, 100, 80),
			Color: frame.NewColor(255, 0, 0, 255),
		},
		// Green box.
		{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(200, 100, 150, 120),
			Color: frame.NewColor(0, 200, 0, 255),
		},
		// Blue box.
		{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(100, 200, 80, 60),
			Color: frame.NewColor(0, 0, 255, 255),
		},
	}

	result := golden.AssertBackendParity(t, "multiple_cmds", vp, cmds)
	if result == nil {
		t.Fatal("AssertBackendParity returned nil")
	}
	if !result.BackendsMatch {
		t.Errorf("backends do not match: %v", result.Comparison)
	}
}

// TestBackendParity_EmptyCommands tests parity with no display commands.
func TestBackendParity_EmptyCommands(t *testing.T) {
	vp := frame.NewViewport(100, 100, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{}

	result := golden.AssertBackendParity(t, "empty", vp, cmds)
	if result == nil {
		t.Fatal("AssertBackendParity returned nil")
	}
	if !result.BackendsMatch {
		t.Errorf("backends do not match: %v", result.Comparison)
	}
}

// BenchmarkBackendParity_Render measures the performance difference between
// CPU and CoreGraphics backends.
func BenchmarkBackendParity_Render(b *testing.B) {
	vp := frame.NewViewport(800, 600, frame.PixelScaleDefault)
	cmds := []raster.DisplayCmd{
		{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(0, 0, 800, 600),
			Color: frame.NewColor(255, 255, 255, 255),
		},
		{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(100, 100, 200, 150),
			Color: frame.NewColor(255, 0, 0, 255),
		},
		{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(400, 200, 300, 200),
			Color: frame.NewColor(0, 255, 0, 255),
		},
	}

	b.Run("CPU", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			backend := raster.NewCPUBackend(800, 600)
			_ = backend.BeginFrame(vp)
			_, _ = backend.Rasterize(cmds, nil)
			_ = backend.EndFrame()
			backend.Close()
		}
	})

	b.Run("CoreGraphics", func(b *testing.B) {
		backend, _, err := raster.NewBackend(800, 600, raster.WithBackend(raster.BackendCoreGraphics))
		if err != nil {
			b.Skip("CoreGraphics not available")
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = backend.BeginFrame(vp)
			_, _ = backend.Rasterize(cmds, nil)
			_ = backend.EndFrame()
		}
		backend.Close()
	})
}
