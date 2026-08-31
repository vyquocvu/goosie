package ui_test

import (
	"github.com/vyquocvu/goosie/internal/ui"
	"context"
	"image/color"
	"net/url"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/renderer"
	"golang.org/x/net/html"
)

// urlRecordingRenderer records the URL passed to SetCurrentURL so tests
// can assert that the renderer learns the page's base URL. ResolveURL
// mirrors the real renderer's RFC 3986 resolution against the base URL.
type urlRecordingRenderer struct {
	MockHTMLRenderer
	baseURL string
}

func (r *urlRecordingRenderer) SetCurrentURL(u string) { r.baseURL = u }
func (r *urlRecordingRenderer) ResolveURL(u string) string {
	if r.baseURL == "" {
		return u
	}
	base, err := url.Parse(r.baseURL)
	if err != nil {
		return u
	}
	ref, err := url.Parse(u)
	if err != nil {
		return u
	}
	return base.ResolveReference(ref).String()
}
func (r *urlRecordingRenderer) RenderHTML(_ context.Context, _ string) (fyne.CanvasObject, error) {
	return canvas.NewRectangle(color.RGBA{R: 255, G: 255, B: 255, A: 255}), nil
}
func (r *urlRecordingRenderer) RenderParsed(_ context.Context, _ *html.Node, _ []renderer.ExternalCSS) (fyne.CanvasObject, error) {
	return canvas.NewRectangle(color.RGBA{R: 255, G: 255, B: 255, A: 255}), nil
}

func TestTabRendererLearnsCurrentURL(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w, true)

	rec := &urlRecordingRenderer{}
	browser.RendererFactory = func() ui.HTMLRenderer { return rec }

	tab := browser.NewTab()
	tab.State().AddToHistory("https://example.com/page")

	err := tab.RenderHTML(context.Background(), `<html><body><a href="/path">Path</a></body></html>`)
	require.NoError(t, err)

	// The renderer must know the current page URL so path-only hrefs
	// resolve to an absolute URL (domain + path) when clicked.
	assert.Equal(t, "https://example.com/page", rec.baseURL,
		"renderer should be given the current page URL before/at render")
	assert.Equal(t, "https://example.com/path", tab.HTMLRenderer().ResolveURL("/path"),
		"path-only href must resolve against the current page URL")
	assert.Equal(t, "https://example.com/next", tab.HTMLRenderer().ResolveURL("/next"),
		"root-relative href must resolve against the current page URL")

	// A subsequent navigation must refresh the renderer's base URL so
	// links on the new page resolve against the new location.
	tab.State().AddToHistory("https://example.com/dir/page2")
	require.NoError(t, tab.RenderHTML(context.Background(), `<html><body><a href="/x">X</a></body></html>`))
	assert.Equal(t, "https://example.com/dir/page2", rec.baseURL)
	assert.Equal(t, "https://example.com/x", tab.HTMLRenderer().ResolveURL("/x"))
	assert.Equal(t, "https://example.com/dir/sibling.html", tab.HTMLRenderer().ResolveURL("sibling.html"))
}
