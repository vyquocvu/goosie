package performance

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func generateLargeHTML(numItems int) string {
	var sb strings.Builder
	sb.WriteString("<html><body>")
	sb.WriteString("<h1>Performance Test</h1>")
	sb.WriteString("<ul>")
	for i := 0; i < numItems; i++ {
		sb.WriteString(fmt.Sprintf("<li class='item-%d'>Item %d</li>", i, i))
	}
	sb.WriteString("</ul>")
	sb.WriteString("</body></html>")
	return sb.String()
}

func BenchmarkRenderLargeHTML(b *testing.B) {
	testApp := test.NewApp()
	defer testApp.Quit()

	html := generateLargeHTML(1000)
	r := renderer.NewRenderer(800, 600)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.RenderHTML(context.Background(), html)
		if err != nil {
			b.Fatalf("RenderHTML failed: %v", err)
		}
	}
}

func BenchmarkRenderVeryLargeHTML(b *testing.B) {
	testApp := test.NewApp()
	defer testApp.Quit()

	html := generateLargeHTML(5000)
	r := renderer.NewRenderer(800, 600)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.RenderHTML(context.Background(), html)
		if err != nil {
			b.Fatalf("RenderHTML failed: %v", err)
		}
	}
}
