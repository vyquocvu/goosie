package a11y

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/dom"
)

func TestARIAAttributes(t *testing.T) {
	html := `
		<html>
		<body>
			<div role="navigation" aria-label="Main Menu">
				<button aria-expanded="false">Menu</button>
			</div>
			<div role="main">
				<img src="test.png" alt="Test Image">
				<input type="text" aria-required="true">
			</div>
		</body>
		</html>
	`

	parser := dom.NewParser()
	
	// Verify navigation role
	nav, err := parser.QuerySelector(html, "[role=navigation]")
	assert.NoError(t, err)
	assert.NotNil(t, nav)
	assert.Equal(t, "Main Menu", nav.Attributes["aria-label"])
	
	// Verify button state
	btn, err := parser.QuerySelector(html, "button")
	assert.NoError(t, err)
	assert.NotNil(t, btn)
	assert.Equal(t, "false", btn.Attributes["aria-expanded"])
	
	// Verify image alt text (WCAG requirement)
	img, err := parser.QuerySelector(html, "img")
	assert.NoError(t, err)
	assert.NotNil(t, img)
	assert.Equal(t, "Test Image", img.Attributes["alt"])
	
	// Verify required input
	input, err := parser.QuerySelector(html, "input")
	assert.NoError(t, err)
	assert.NotNil(t, input)
	assert.Equal(t, "true", input.Attributes["aria-required"])
}
