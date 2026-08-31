package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func objectChildren(o fyne.CanvasObject) []fyne.CanvasObject {
	switch c := o.(type) {
	case *fyne.Container:
		return c.Objects
	case *container.ThemeOverride:
		return []fyne.CanvasObject{c.Content}
	default:
		return nil
	}
}

func TestLinkColorThemeOverridesHyperlinkColorOnly(t *testing.T) {
	link := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	lt := renderer.NewLinkColorTheme(theme.DarkTheme(), link)

	assert.Equal(t, color.Color(link), lt.Color(theme.ColorNameHyperlink, fyne.ThemeVariant(0)))
	assert.Equal(t, theme.DarkTheme().Color(theme.ColorNameForeground, fyne.ThemeVariant(0)), lt.Color(theme.ColorNameForeground, fyne.ThemeVariant(0)))
}

func TestApplyLinkColorWrapsWhenComputedColorSet(t *testing.T) {
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	node := &renderer.RenderNode{TagName: "a", ComputedStyle: &renderer.Style{Color: white}}
	link := widget.NewHyperlink("Home", nil)
	wrapped := renderer.ApplyLinkColor(node, link)

	override, ok := wrapped.(*container.ThemeOverride)
	require.True(t, ok, "expected ThemeOverride wrapper, got %T", wrapped)

	// The override must resolve the hyperlink color to the computed color.
	lt, ok := override.Theme.(renderer.LinkColorTheme)
	require.True(t, ok, "expected renderer.LinkColorTheme, got %T", override.Theme)
	assert.Equal(t, white, lt.LinkColor())

	// No computed color: object passes through untouched.
	plain := &renderer.RenderNode{TagName: "a", ComputedStyle: &renderer.Style{}}
	assert.Equal(t, fyne.CanvasObject(link), renderer.ApplyLinkColor(plain, link))
}

func TestPaintLinkUsesComputedColor(t *testing.T) {
	r := renderer.NewRenderer(1280, 800)
	_, err := r.RenderHTML(t.Context(), `<html><body><nav style="background:#333;color:white;padding:10px;"><a href="#home" style="color:white;">Home</a></nav><a href="#about">About</a></body></html>`)
	require.NoError(t, err)

	obj := r.PresentFrame()
	require.NotNil(t, obj)

	var overrides, plainLinks int
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		switch o.(type) {
		case *container.ThemeOverride:
			overrides++
		case *widget.Hyperlink, *renderer.TappableHyperlink:
			plainLinks++
		}
		for _, child := range objectChildren(o) {
			walk(child)
		}
	}
	walk(obj)

	// The styled anchor must be wrapped in a theme override; the unstyled
	// anchor keeps the default theme hyperlink blue. The override's content
	// is itself a hyperlink, hence two link widgets overall.
	assert.Equal(t, 1, overrides, "styled anchor should be wrapped in ThemeOverride")
	assert.Equal(t, 2, plainLinks, "expected both anchors to render as hyperlink widgets")
}
