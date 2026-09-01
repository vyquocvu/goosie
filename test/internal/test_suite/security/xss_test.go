package security

import (
	"context"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestRendererDoSProtection(t *testing.T) {
	// Create a test app for Fyne dependencies
	testApp := test.NewApp()
	defer testApp.Quit()

	maliciousInputs := []string{
		// Deeply nested elements
		nestedHTML(1000),
		// Very long attributes
		`<div class="` + string(make([]byte, 100000)) + `"></div>`,
		// Malformed tags
		`<<<<<<<<div class="test">`,
		// Script tags (should be rendered as text or ignored, not executed)
		`<script>alert('xss')</script>`,
		// Event handlers (should be ignored by renderer)
		`<img src="x" onerror="alert('xss')">`,
	}

	r := renderer.NewRenderer(800, 600)

	for _, input := range maliciousInputs {
		t.Run("Malicious Input", func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Renderer panicked on malicious input: %v", r)
				}
			}()

			_, err := r.RenderHTML(context.Background(), input)
			// We don't necessarily expect an error, but we definitely don't want a panic
			if err != nil {
				t.Logf("Renderer handled invalid input with error: %v", err)
			}
		})
	}
}

func nestedHTML(depth int) string {
	start := ""
	end := ""
	for i := 0; i < depth; i++ {
		start += "<div>"
		end += "</div>"
	}
	return start + "Content" + end
}
