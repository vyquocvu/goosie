package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/vyquocvu/goosie/internal/renderer"
)

func main() {
	// Read the long page HTML
	htmlContent, err := os.ReadFile("examples/long_page.html")
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	fmt.Println("=== Scroll Performance Test ===")

	// Font metrics demo is available in a separate file

	// Create renderer
	r := renderer.NewRenderer(800, 600)

	// Measure initial render time
	start := time.Now()
	canvasObj, err := r.RenderHTML(context.Background(), string(htmlContent))
	if err != nil {
		fmt.Printf("Error rendering HTML: %v\n", err)
		return
	}
	initialRenderTime := time.Since(start)

	fmt.Printf("Initial render time: %v\n", initialRenderTime)
	fmt.Printf("Content height: %.2f pixels\n", r.GetContentHeight())
	fmt.Printf("Canvas object created: %v\n", canvasObj != nil)
	fmt.Println()

	// Simulate scrolling
	fmt.Println("Simulating scroll performance...")

	scrollPositions := []float32{0, 100, 200, 300, 400, 500, 600, 700, 800, 900, 1000}
	totalScrollTime := time.Duration(0)

	for i, pos := range scrollPositions {
		start := time.Now()
		r.SetViewport(pos, 600)
		_ = r.UpdateViewport()
		scrollTime := time.Since(start)
		totalScrollTime += scrollTime

		if i%3 == 0 {
			fmt.Printf("Scroll to Y=%.0f: %v\n", pos, scrollTime)
		}
	}

	avgScrollTime := totalScrollTime / time.Duration(len(scrollPositions))
	fmt.Println()
	fmt.Printf("Average scroll update time: %v\n", avgScrollTime)
	fmt.Printf("Estimated FPS during scrolling: %.0f\n", float64(time.Second)/float64(avgScrollTime))
	fmt.Println()

	// Performance summary
	fmt.Println("=== Performance Summary ===")
	fmt.Printf("Initial render: %v (one-time cost)\n", initialRenderTime)
	fmt.Printf("Scroll updates: %v average (per scroll event)\n", avgScrollTime)
	fmt.Println()

	if avgScrollTime < 2*time.Millisecond {
		fmt.Println("✓ Scroll performance is EXCELLENT (< 2ms per update)")
	} else if avgScrollTime < 16*time.Millisecond {
		fmt.Println("✓ Scroll performance is GOOD (< 16ms = 60 FPS)")
	} else {
		fmt.Println("⚠ Scroll performance could be improved")
	}

	fmt.Println()
	fmt.Println("The viewport-based rendering ensures smooth scrolling by:")
	fmt.Println("1. Only rendering visible elements (viewport culling)")
	fmt.Println("2. Caching display lists to avoid rebuilding")
	fmt.Println("3. Using constant-time scroll updates")
}
