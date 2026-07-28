package integration

import (
	"context"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// TestCSSBackgroundColors_Render verifies that the renderer accepts
// the most common CSS background-color value forms without errors:
// hex (3, 6 digits), rgb()/rgba(), hsl()/hsla(), and named colors.
// The acceptance criterion is that RenderHTML returns a non-nil
// canvas and no error — i.e., the CSS value parser and the style
// resolver agree the value is well-formed for the paint pipeline.
//
// The test exercises the integration of CSS parsing → style
// resolution → layout → rendering, which is exactly the chain a
// visual regression test goes through. The pixels are checked
// separately by the e2e suite; here we lock down the contract that
// "color value is well-formed and reaches the renderer".
func TestCSSBackgroundColors_Render(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	cases := []struct {
		name  string
		value string
	}{
		{"hex3Digit", "#f00"},
		{"hex6Digit", "#3366cc"},
		{"rgb", "rgb(255, 128, 0)"},
		{"rgba", "rgba(0, 128, 128, 0.5)"},
		{"hsl", "hsl(120, 50%, 50%)"},
		{"hsla", "hsla(240, 100%, 50%, 0.75)"},
		{"namedColor", "tomato"},
		{"transparent", "transparent"},
		{"currentColor", "currentColor"},
	}
	r := renderer.NewRenderer(800, 600)
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			html := `<html><body><div id="t" style="width:50px;height:50px;background-color:` + c.value + `"></div></body></html>`
			obj, err := r.RenderHTML(context.Background(), html)
			require.NoError(t, err, "background-color: %s", c.value)
			require.NotNil(t, obj, "background-color: %s", c.value)
		})
	}
}

// TestCSSBackgroundColors_ComputedStyle verifies that for the most
// common well-formed color values, the computed style on the
// styled element actually carries a non-nil / non-zero background
// color. This is the regression guard against a CSS value being
// accepted by the parser but silently dropped by the style
// resolver — a class of bug that visual regression tests miss
// because the rendered background ends up as the default white.
func TestCSSBackgroundColors_ComputedStyle(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	cases := []struct {
		name  string
		value string
	}{
		{"hex", "#3366cc"},
		{"rgb", "rgb(255, 128, 0)"},
		{"namedColor", "tomato"},
	}
	r := renderer.NewRenderer(800, 600)
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			html := `<html><body><div id="t" style="width:50px;height:50px;background-color:` + c.value + `"></div></body></html>`
			_, err := r.RenderHTML(context.Background(), html)
			require.NoError(t, err)

			root := r.GetRoot()
			require.NotNil(t, root)
			target := findFirstByID(root, "t")
			require.NotNil(t, target, "expected element with id=t")
			require.NotNil(t, target.ComputedStyle, "computed style must be populated for #t")

			bg := target.ComputedStyle.BackgroundColor
			// RGBA(0,0,0,0) is the zero value (transparent black); a real
			// color resolution must produce something else.
			r, g, b, a := bg.RGBA()
			haveColor := r != 0 || g != 0 || b != 0 || a != 0
			assert.True(t, haveColor,
				"background-color %q must resolve to a non-zero color (got r=%d g=%d b=%d a=%d)",
				c.value, r, g, b, a)
		})
	}
}

// TestCSSBorderRadius_Render verifies that the renderer accepts
// the most common border-radius value forms: a single length, a
// single percentage, and the shorthand with two values. The CSS
// spec defines border-radius as a corner-radii list, so we also
// exercise the multi-value shorthand.
func TestCSSBorderRadius_Render(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	cases := []struct {
		name  string
		value string
	}{
		{"singlePx", "10px"},
		{"singleEm", "1em"},
		{"singlePercent", "50%"},
		{"twoValues", "10px 30px"},
		{"threeValues", "10px 20px 30px"},
		{"fourValues", "5px 10px 15px 20px"},
		{"largeRadius", "50px"},
		{"circle", "50%"},
	}
	r := renderer.NewRenderer(800, 600)
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			html := `<html><body><div id="t" style="width:100px;height:100px;background:blue;border-radius:` + c.value + `;"></div></body></html>`
			obj, err := r.RenderHTML(context.Background(), html)
			require.NoError(t, err, "border-radius: %s", c.value)
			require.NotNil(t, obj, "border-radius: %s", c.value)
		})
	}
}

// TestCSSBorderRadius_StoredOnComputedStyle verifies that the
// border-radius value is preserved verbatim on the computed style
// after style resolution. This guards against a regression where
// the property is parsed but stripped before the renderer reads it
// for corner drawing.
func TestCSSBorderRadius_StoredOnComputedStyle(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	r := renderer.NewRenderer(800, 600)
	html := `<html><body><div id="t" style="width:100px;height:100px;border-radius:12px;"></div></body></html>`
	_, err := r.RenderHTML(context.Background(), html)
	require.NoError(t, err)

	root := r.GetRoot()
	require.NotNil(t, root)
	target := findFirstByID(root, "t")
	require.NotNil(t, target, "expected element with id=t")
	require.NotNil(t, target.ComputedStyle)
	assert.Equal(t, "12px", target.ComputedStyle.BorderRadius,
		"border-radius must be preserved verbatim on computed style")
}

// TestCSSBoxSizing_AcceptsBothValues verifies that the renderer
// accepts both content-box and border-box. The contract under test
// is that the parser recognizes the keyword and the style resolver
// stores it without error; the layout math for each value is
// covered by dedicated layout tests in the renderer package.
func TestCSSBoxSizing_AcceptsBothValues(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	r := renderer.NewRenderer(800, 600)
	for _, value := range []string{"content-box", "border-box"} {
		value := value
		t.Run(value, func(t *testing.T) {
			html := `<html><body><div id="t" style="width:100px;height:100px;padding:10px;border:5px solid black;box-sizing:` + value + `;"></div></body></html>`
			_, err := r.RenderHTML(context.Background(), html)
			require.NoError(t, err, "box-sizing: %s", value)

			root := r.GetRoot()
			require.NotNil(t, root)
			target := findFirstByID(root, "t")
			require.NotNil(t, target, "expected element with id=t")
			require.NotNil(t, target.ComputedStyle)
			assert.Equal(t, value, target.ComputedStyle.BoxSizing,
				"box-sizing value must be stored verbatim")
		})
	}
}

// TestCSSCursor_AcceptsCommonValues verifies that the cursor
// property accepts the most common pointer / text / default values.
// The renderer may not yet change the mouse cursor at the Fyne
// layer, but the property must still parse and reach the computed
// style so future input wiring can rely on it.
func TestCSSCursor_AcceptsCommonValues(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	r := renderer.NewRenderer(800, 600)
	for _, value := range []string{"pointer", "text", "default", "not-allowed", "wait"} {
		value := value
		t.Run(value, func(t *testing.T) {
			html := `<html><body><button id="t" style="cursor:` + value + `;">x</button></body></html>`
			_, err := r.RenderHTML(context.Background(), html)
			require.NoError(t, err, "cursor: %s", value)

			root := r.GetRoot()
			require.NotNil(t, root)
			target := findFirstByID(root, "t")
			require.NotNil(t, target)
			require.NotNil(t, target.ComputedStyle)
			assert.Equal(t, value, target.ComputedStyle.Cursor)
		})
	}
}

// TestCSSTextDecoration_AcceptsValues verifies that the renderer
// accepts text-decoration values used in real content: underline,
// line-through, and overline. The values are preserved on the
// computed style so the text paint step can use them.
func TestCSSTextDecoration_AcceptsValues(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	r := renderer.NewRenderer(800, 600)
	for _, value := range []string{"underline", "line-through", "overline", "none"} {
		value := value
		t.Run(value, func(t *testing.T) {
			html := `<html><body><p id="t" style="text-decoration:` + value + `;">text</p></body></html>`
			_, err := r.RenderHTML(context.Background(), html)
			require.NoError(t, err, "text-decoration: %s", value)

			root := r.GetRoot()
			require.NotNil(t, root)
			target := findFirstByID(root, "t")
			require.NotNil(t, target)
			require.NotNil(t, target.ComputedStyle)
			assert.Equal(t, value, target.ComputedStyle.TextDecoration)
		})
	}
}

// TestCSSTextTransform_AcceptsValues verifies that the renderer
// accepts text-transform values: uppercase, lowercase, capitalize,
// and none. The values are preserved on the computed style.
func TestCSSTextTransform_AcceptsValues(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	r := renderer.NewRenderer(800, 600)
	for _, value := range []string{"uppercase", "lowercase", "capitalize", "none"} {
		value := value
		t.Run(value, func(t *testing.T) {
			html := `<html><body><p id="t" style="text-transform:` + value + `;">text</p></body></html>`
			_, err := r.RenderHTML(context.Background(), html)
			require.NoError(t, err, "text-transform: %s", value)

			root := r.GetRoot()
			require.NotNil(t, root)
			target := findFirstByID(root, "t")
			require.NotNil(t, target)
			require.NotNil(t, target.ComputedStyle)
			assert.Equal(t, value, target.ComputedStyle.TextTransform)
		})
	}
}

// TestCSSVerticalAlign_AcceptsValues verifies that vertical-align
// accepts the spec keywords used by inline content (super, sub,
// top, middle, baseline, bottom, text-top, text-bottom). The
// renderer may not yet position glyphs differently for each
// keyword, but the value must be preserved on the computed style.
func TestCSSVerticalAlign_AcceptsValues(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	r := renderer.NewRenderer(800, 600)
	for _, value := range []string{
		"baseline", "sub", "super", "top", "middle",
		"bottom", "text-top", "text-bottom",
	} {
		value := value
		t.Run(value, func(t *testing.T) {
			html := `<html><body><p>base <span id="t" style="vertical-align:` + value + `;">x</span> tail</p></body></html>`
			_, err := r.RenderHTML(context.Background(), html)
			require.NoError(t, err, "vertical-align: %s", value)

			root := r.GetRoot()
			require.NotNil(t, root)
			target := findFirstByID(root, "t")
			require.NotNil(t, target)
			require.NotNil(t, target.ComputedStyle)
			assert.Equal(t, value, target.ComputedStyle.VerticalAlign)
		})
	}
}

// TestCSSOpacity_AcceptsValues verifies that opacity values from
// 0.0 to 1.0 are accepted by the parser and stored on the computed
// style as a float. The renderer applies this alpha multiplier at
// paint time.
func TestCSSOpacity_AcceptsValues(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	r := renderer.NewRenderer(800, 600)
	for _, value := range []float32{0.0, 0.25, 0.5, 0.75, 1.0} {
		value := value
		t.Run(fmtFloat(value), func(t *testing.T) {
			html := `<html><body><div id="t" style="width:50px;height:50px;background:red;opacity:` + fmtFloat(value) + `;"></div></body></html>`
			_, err := r.RenderHTML(context.Background(), html)
			require.NoError(t, err, "opacity: %v", value)

			root := r.GetRoot()
			require.NotNil(t, root)
			target := findFirstByID(root, "t")
			require.NotNil(t, target)
			require.NotNil(t, target.ComputedStyle)
			assert.InDelta(t, value, target.ComputedStyle.Opacity, 0.001,
				"opacity must be stored as a float matching the input")
		})
	}
}

// fmtFloat formats a float as a CSS-friendly value without
// trailing zeros or scientific notation. Used by opacity tests
// because the CSS string must round-trip exactly.
func fmtFloat(v float32) string {
	switch v {
	case 0:
		return "0"
	case 0.25:
		return "0.25"
	case 0.5:
		return "0.5"
	case 0.75:
		return "0.75"
	case 1:
		return "1"
	}
	return "0.5"
}

// findFirstByID is a tiny helper that walks the rendered tree and
// returns the first node whose "id" attribute matches the given
// id. We use it instead of importing the renderer's full ID-index
// helpers so each test is self-contained.
func findFirstByID(node *renderer.RenderNode, id string) *renderer.RenderNode {
	if node == nil {
		return nil
	}
	if v, ok := node.Attrs["id"]; ok && v == id {
		return node
	}
	for _, child := range node.Children {
		if found := findFirstByID(child, id); found != nil {
			return found
		}
	}
	return nil
}