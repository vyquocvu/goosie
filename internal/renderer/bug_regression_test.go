package renderer

import (
	"context"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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
	canvasObject, err := htmlRenderer.RenderHTML(context.Background(), htmlContent)
	if err != nil {
		t.Fatalf("Error rendering HTML: %v", err)
	}

	// Count the number of rendered objects
	vbox, ok := canvasObject.(*fyne.Container)
	if !ok {
		t.Fatalf("Expected canvasObject to be *fyne.Container, got %T", canvasObject)
	}

	// Expected: 5 objects (1 background rectangle, 1 h1 text, 2 paragraph lines, 1 link text)
	// Before fix: many objects (each word rendered separately, causing duplication)
	expectedCount := 5
	actualCount := len(vbox.Objects)

	if actualCount != expectedCount {
		t.Errorf("Expected %d rendered objects, got %d", expectedCount, actualCount)
		for i, obj := range vbox.Objects {
			t.Logf("  Object %d Type=%T Value=%#v", i, obj, obj)
		}
	}

	// Verify the content is correct
	if actualCount >= 5 {
		// Import "fyne.io/fyne/v2/canvas" if not present, but it should be
		// Check background
		if rect, ok := vbox.Objects[0].(*canvas.Rectangle); ok {
			expectedBgColor := color.RGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
			if rect.FillColor != expectedBgColor {
				t.Errorf("Expected background color %v, got %v", expectedBgColor, rect.FillColor)
			}
		} else {
			t.Errorf("Expected Object 0 to be *canvas.Rectangle, got %T", vbox.Objects[0])
		}

		// Check h1
		if txt, ok := vbox.Objects[1].(*canvas.Text); ok {
			if txt.Text != "Example Domain" {
				t.Errorf("Expected h1 text 'Example Domain', got '%s'", txt.Text)
			}
		} else {
			t.Errorf("Expected Object 1 to be *canvas.Text, got %T", vbox.Objects[1])
		}

		// Check paragraph line 1
		if txt, ok := vbox.Objects[2].(*canvas.Text); ok {
			expectedText := "This domain is for use in documentation examples without"
			if txt.Text != expectedText {
				t.Errorf("Expected paragraph line 1 text, got '%s'", txt.Text)
			}
		} else {
			t.Errorf("Expected Object 2 to be *canvas.Text, got %T", vbox.Objects[2])
		}

		// Check paragraph line 2
		if txt, ok := vbox.Objects[3].(*canvas.Text); ok {
			expectedText := "needing permission. Avoid use in operations."
			if txt.Text != expectedText {
				t.Errorf("Expected paragraph line 2 text, got '%s'", txt.Text)
			}
		} else {
			t.Errorf("Expected Object 3 to be *canvas.Text, got %T", vbox.Objects[3])
		}

		// Check link. Anchors with a computed color are wrapped in a
		// ThemeOverride so the hyperlink honors the CSS color.
		linkObj := vbox.Objects[4]
		if override, ok := linkObj.(*container.ThemeOverride); ok {
			linkObj = override.Content
		}
		switch link := linkObj.(type) {
		case *TappableHyperlink:
			if link.Text != "Learn more" {
				t.Errorf("Expected link text 'Learn more', got '%s'", link.Text)
			}
		case *widget.Hyperlink:
			if link.Text != "Learn more" {
				t.Errorf("Expected link text 'Learn more', got '%s'", link.Text)
			}
		default:
			t.Errorf("Expected Object 4 to be a hyperlink, got %T", linkObj)
		}
	}
}
