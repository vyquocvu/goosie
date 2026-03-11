package renderer

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// collectAllText recursively collects all text strings from canvas objects in a
// container, including those inside nested containers.
func collectAllText(obj fyne.CanvasObject) []string {
	var texts []string
	switch v := obj.(type) {
	case *canvas.Text:
		t := strings.TrimSpace(v.Text)
		if t != "" {
			texts = append(texts, t)
		}
	case *fyne.Container:
		for _, child := range v.Objects {
			texts = append(texts, collectAllText(child)...)
		}
	}
	return texts
}

// TestBugFixDuplicateRendering is a regression test for the bug where
// HTML content was being rendered multiple times due to duplicate LayoutBox
// instances being created for inline content.
//
// The issue was that when text wrapped across multiple lines, each word
// got its own LayoutBox with the same NodeID, causing the display list
// builder to render the text multiple times (i.e. the full text of a block
// element would appear once per wrapped line instead of once total).
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

	// Verify the rendered output is a container
	container, ok := canvasObject.(*fyne.Container)
	if !ok {
		t.Fatalf("Expected canvasObject to be *fyne.Container, got %T", canvasObject)
	}

	if len(container.Objects) == 0 {
		t.Fatal("Expected at least one rendered object, got none")
	}

	// Collect all rendered text fragments from the container (and nested containers).
	// The display-list renderer produces one canvas.Text per inline text fragment;
	// each fragment contains only its own portion of text (never the full block text).
	allTexts := collectAllText(canvasObject)
	t.Logf("Total text fragments rendered: %d", len(allTexts))
	for i, txt := range allTexts {
		t.Logf("  [%d] %q", i, txt)
	}

	// Verify that the h1 heading text is present.
	// The heading "Example Domain" must appear in the rendered output.
	h1Found := countTextOccurrences(allTexts, "Example Domain")
	if h1Found == 0 {
		t.Error("h1 text 'Example Domain' not found in rendered output")
	}

	// Verify that the link text is present.
	linkFound := countTextOccurrences(allTexts, "Learn more")
	if linkFound == 0 {
		t.Error("link text 'Learn more' not found in rendered output")
	}

	// Verify there is no duplicate full-text rendering of the h1.
	// The original bug caused the entire h1 text to appear once per word-wrap line.
	// It should appear at most once as a complete string.
	if h1Found > 1 {
		t.Errorf("Duplicate full-text rendering detected for h1: found %d times", h1Found)
	}

	// Verify the paragraph body text is present somewhere in fragments.
	// The paragraph spans multiple inline fragments so we check for at least
	// one key word rather than the full string.
	paraKeyword := "documentation"
	keywordFound := false
	for _, txt := range allTexts {
		if strings.Contains(txt, paraKeyword) {
			keywordFound = true
			break
		}
	}
	if !keywordFound {
		t.Errorf("Paragraph content keyword %q not found in rendered output", paraKeyword)
	}
}

// countTextOccurrences returns the number of fragments in texts that exactly match target.
func countTextOccurrences(texts []string, target string) int {
	count := 0
	for _, txt := range texts {
		if txt == target {
			count++
		}
	}
	return count
}
