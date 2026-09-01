package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// BenchmarkE2EScrollPerformance measures scroll performance with a large document.
func BenchmarkE2EScrollPerformance(b *testing.B) {
	test.NewApp() // Setup headless fyne app to prevent panics in fyne.Do

	var sb strings.Builder
	sb.WriteString("<html><head><style>.item { height: 20px; color: blue; margin: 5px; } </style></head><body>")
	for i := 0; i < 600; i++ {
		sb.WriteString(fmt.Sprintf("<div class='item'>Item %d</div>\n", i))
	}
	sb.WriteString("</body></html>")
	htmlStr := sb.String()

	r := renderer.NewRenderer(800, 600)

	_, err := r.RenderHTML(context.Background(), htmlStr)
	if err != nil {
		b.Fatalf("Failed to render: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	y := float32(0.0)
	for i := 0; i < b.N; i++ {
		// Simulate scrolling down
		y += 10.0
		r.SetViewport(y, 600)
		_ = r.UpdateViewport()
	}
}

// TestScrollFPSLiveLogging simulates continuous scrolling and logs live FPS and frame timing metrics.
func TestScrollFPSLiveLogging(t *testing.T) {
	test.NewApp()

	var sb strings.Builder
	sb.WriteString("<html><head><style>.card { height: 100px; margin: 10px; background-color: #eee; border: 1px solid #ccc; padding: 10px; } </style></head><body>")
	for i := 0; i < 100; i++ {
		sb.WriteString(fmt.Sprintf("<div class='card'><h2>Article Section %d</h2><p>Description text for scroll testing with multiple paragraphs and links.</p></div>\n", i))
	}
	sb.WriteString("</body></html>")

	r := renderer.NewRenderer(1000, 700)
	r.SetFPSOverlayEnabled(true)

	_, err := r.RenderHTML(context.Background(), sb.String())
	if err != nil {
		t.Fatalf("Failed to render HTML: %v", err)
	}

	t.Log("=== Starting Continuous Scroll FPS Test (60 scroll steps) ===")

	for frame := 1; frame <= 60; frame++ {
		viewportY := float32(frame * 15)
		r.SetViewport(viewportY, 700)
		_ = r.UpdateViewport()

		stats := r.FPSStats()
		metrics := r.FrameMetrics()

		if frame%10 == 0 || frame == 1 || frame == 60 {
			t.Logf("[Frame %2d] Viewport Y: %4.0fpx | Current FPS: %5.1f | Avg FPS: %5.1f | Min FPS: %5.1f | Max FPS: %5.1f | Render Duration: %v | Dropped: %d",
				frame, viewportY, stats.CurrentFPS, stats.AverageFPS, stats.MinFPS, stats.MaxFPS, metrics.RenderDuration, stats.Dropped)
		}
	}

	finalStats := r.FPSStats()
	finalMetrics := r.FrameMetrics()

	t.Log("=== Final Scroll Performance Summary ===")
	t.Logf("Total Frames Presented : %d", finalStats.Frames)
	t.Logf("Target Refresh Rate     : %.1f Hz", r.CanvasRenderer().FPSStats().CurrentFPS)
	t.Logf("Average Frame FPS       : %.1f FPS", finalStats.AverageFPS)
	t.Logf("Min / Max FPS           : %.1f / %.1f FPS", finalStats.MinFPS, finalStats.MaxFPS)
	t.Logf("Average Render Latency  : %v", finalMetrics.RenderDuration)
	t.Logf("Max Render Latency      : %v", finalMetrics.MaxRenderDuration)
	t.Logf("Dropped Frames          : %d", finalStats.Dropped)
}

// TestScrollFPSRealtimePaced simulates scrolling paced to real display refresh rates (60Hz & 120Hz).
func TestScrollFPSRealtimePaced(t *testing.T) {
	test.NewApp()

	var sb strings.Builder
	sb.WriteString("<html><head><style>.block { height: 80px; margin: 8px; background: #fafafa; } </style></head><body>")
	for i := 0; i < 80; i++ {
		sb.WriteString(fmt.Sprintf("<div class='block'>Row %d content</div>\n", i))
	}
	sb.WriteString("</body></html>")

	rates := []struct {
		name     string
		targetFPS float64
	}{
		{"60Hz Standard Display", 60.0},
		{"120Hz ProMotion Display", 120.0},
	}

	for _, tc := range rates {
		t.Run(tc.name, func(t *testing.T) {
			r := renderer.NewRenderer(1000, 700)
			r.SetTargetFPS(tc.targetFPS)
			r.SetFPSOverlayEnabled(true)

			_, err := r.RenderHTML(context.Background(), sb.String())
			if err != nil {
				t.Fatalf("Failed to render: %v", err)
			}

			frameInterval := time.Duration(float64(time.Second) / tc.targetFPS)
			t.Logf("--- Simulating real-time scrolling at target %.1f FPS (interval %v) ---", tc.targetFPS, frameInterval)

			for frame := 1; frame <= 15; frame++ {
				time.Sleep(frameInterval)
				viewportY := float32(frame * 20)
				r.SetViewport(viewportY, 700)
				_ = r.UpdateViewport()

				stats := r.FPSStats()
				metrics := r.FrameMetrics()

				if frame%5 == 0 || frame == 1 {
					t.Logf("[Frame %2d] Viewport Y: %4.0fpx | Live FPS: %5.1f | Avg FPS: %5.1f | Render Duration: %v | Dropped: %d",
						frame, viewportY, stats.CurrentFPS, stats.AverageFPS, metrics.RenderDuration, stats.Dropped)
				}
			}

			finalStats := r.FPSStats()
			t.Logf("Summary: Final FPS: %.1f | Avg FPS: %.1f | Dropped: %d", finalStats.CurrentFPS, finalStats.AverageFPS, finalStats.Dropped)
		})
	}
}
