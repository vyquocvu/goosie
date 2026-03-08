package integration

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/testutil"
)

func TestRendererIntegration(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	// 1. Setup
	html := `
		<html>
		<head>
			<style>
				body { margin: 0; padding: 0; }
				.box { width: 100px; height: 100px; background-color: red; }
				#header { font-size: 20px; font-weight: bold; }
			</style>
		</head>
		<body>
			<div id="header">Header</div>
			<div class="box">Box 1</div>
			<div class="box">Box 2</div>
		</body>
		</html>
	`

	// 2. Parse HTML & CSS (Implicitly handled by Renderer)
	r := renderer.NewRenderer(800, 600)
	canvasObj, err := r.RenderHTML(html)
	if err != nil {
		t.Fatalf("Failed to render HTML: %v", err)
	}
	assert.NotNil(t, canvasObj, "Canvas object should not be nil")

	// 3. Verify DOM parsing
	parser := dom.NewParser()
	bodyText, err := parser.ParseBodyText(html)
	assert.NoError(t, err)
	assert.Contains(t, bodyText, "Header", "Body text should contain 'Header'")
	assert.Contains(t, bodyText, "Box 1", "Body text should contain 'Box 1'")

	// 4. Verify Element retrieval
	header, err := parser.GetElementByIDFull(html, "header")
	assert.NoError(t, err)
	assert.NotNil(t, header)
	assert.Equal(t, "div", header.TagName)
	assert.Equal(t, "header", header.ID)

	boxes, err := parser.GetElementsByClassName(html, "box")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(boxes), "Should find 2 elements with class 'box'")

	// 5. Visual Verification (optional, if screenshot dir is set)
	if testutil.ShouldSaveScreenshots() {
		path, err := testutil.SaveTestScreenshot(canvasObj, "TestRendererIntegration", 800, 600)
		if err != nil {
			t.Logf("Failed to save screenshot: %v", err)
		} else {
			t.Logf("Saved screenshot to: %s", path)
		}
	}
}
