package renderer

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// TestBugFixDuplicateRendering is a regression test for the bug where
// HTML content was being rendered multiple times due to duplicate LayoutBox
// instances being created for inline content.
//
// The issue was that when text wrapped across multiple lines, each word
// got its own LayoutBox with the same NodeID, causing the display list
// builder to render the text multiple times.
func TestBugFixDuplicateRendering(t *testing.T) {
	// This is the exact HTML from the bug report
	htmlContent := `<!doctype html>
<html lang="en">
<head>
  <title>Example Domain</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    body{background:#eee;width:60vw;margin:15vh auto;font-family:system-ui,sans-serif}
    h1{font-size:1.5em}
    div{opacity:0.8}
    a:link,a:visited{color:#348}
  </style>
<body>
  <div>
    <h1>Example Domain</h1>
    <p>This domain is for use in documentation examples without needing permission. Avoid use in operations.
    <p><a href="https://iana.org/domains/example">Learn more</a>
  </div>
</body>
</html>`

	// Create renderer and render the HTML
	htmlRenderer := NewRenderer(800, 600)
	htmlRenderer.SetViewport(0, 100000)
	canvasObject, err := htmlRenderer.RenderHTML(htmlContent)
	if err != nil {
		t.Fatalf("Error rendering HTML: %v", err)
	}

	// Count the number of rendered objects
	vbox, ok := canvasObject.(*fyne.Container)
	if !ok {
		t.Fatalf("Expected canvasObject to be *fyne.Container, got %T", canvasObject)
	}

	// Expected: 3 objects (h1, p, link)
	// Before fix: 19 objects (each word rendered separately, causing duplication)
	expectedCount := 3
	actualCount := len(vbox.Objects)

	if actualCount != expectedCount {
		t.Errorf("Expected %d rendered objects, got %d", expectedCount, actualCount)
		for i, obj := range vbox.Objects {
			if label, isLabel := obj.(*widget.Label); isLabel {
				t.Logf("  Object %d: Text=%q", i, label.Text)
			}
		}
	}

	// Verify the content is correct
	if actualCount >= 3 {
		// Check h1
		if label, ok := vbox.Objects[0].(*widget.Label); ok {
			if label.Text != "Example Domain" {
				t.Errorf("Expected h1 text 'Example Domain', got '%s'", label.Text)
			}
		}

		// Check paragraph
		if label, ok := vbox.Objects[1].(*widget.Label); ok {
			expectedText := "This domain is for use in documentation examples without needing permission. Avoid use in operations."
			if label.Text != expectedText {
				t.Errorf("Expected paragraph text, got '%s'", label.Text)
			}
		}

		// Check link
		if label, ok := vbox.Objects[2].(*widget.Label); ok {
			if label.Text != "Learn more" {
				t.Errorf("Expected link text 'Learn more', got '%s'", label.Text)
			}
		}
	}
}
