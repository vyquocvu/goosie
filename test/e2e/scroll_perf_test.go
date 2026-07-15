package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"

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
