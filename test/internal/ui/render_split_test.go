package ui_test

import (
	"github.com/vyquocvu/goosie/internal/ui"
	"context"
	"errors"
	"image/color"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/renderer"
	"golang.org/x/net/html"
)

// splitSpy implements frameSplitter on top of MockHTMLRendererComp and
// records whether the tab drove the two-phase render path.
type splitSpy struct {
	MockHTMLRendererComp

	mu          sync.Mutex
	buildHTML   int
	buildParsed int
	presented   int
	legacy      int
	buildErr    error
}

func (s *splitSpy) BuildHTML(ctx context.Context, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buildHTML++
	return s.buildErr
}

func (s *splitSpy) BuildParsed(ctx context.Context, _ *html.Node, _ []renderer.ExternalCSS) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buildParsed++
	return s.buildErr
}

func (s *splitSpy) PresentFrame() fyne.CanvasObject {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.presented++
	return nil
}

func (s *splitSpy) RenderHTML(ctx context.Context, _ string) (fyne.CanvasObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.legacy++
	return nil, nil
}

func (s *splitSpy) counts() (buildHTML, buildParsed, presented, legacy int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buildHTML, s.buildParsed, s.presented, s.legacy
}

func newSplitTestTab(t *testing.T, spy ui.HTMLRenderer) *ui.Tab {
	t.Helper()
	app := test.NewApp()
	t.Cleanup(app.Quit)
	w := app.NewWindow("test")
	browser := ui.NewBrowserInternal(app, w, true)
	browser.RendererFactory = func() ui.HTMLRenderer { return spy }
	return browser.ActiveTab()
}

// TestTabRenderUsesSplitPath — PR4: renderers implementing frameSplitter
// drive BuildHTML/BuildParsed on the caller's goroutine and present via
// PresentFrame, never the legacy single-phase RenderHTML.
func TestTabRenderUsesSplitPath(t *testing.T) {
	spy := &splitSpy{}
	tab := newSplitTestTab(t, spy)

	require.NoError(t, tab.RenderHTML(context.Background(), "<html><body>hi</body></html>"))
	require.NoError(t, tab.RenderParsedContent(context.Background(), &html.Node{Type: html.ElementNode, Data: "body"}, nil))

	b, p, pr, l := spy.counts()
	assert.Equal(t, 1, b, "BuildHTML should run once")
	assert.Equal(t, 1, p, "BuildParsed should run once")
	assert.Equal(t, 2, pr, "PresentFrame should run after each build")
	assert.Equal(t, 0, l, "legacy RenderHTML must not be used when the splitter exists")
}

// TestTabRenderSplitErrorPropagates — a build error surfaces to the caller.
func TestTabRenderSplitErrorPropagates(t *testing.T) {
	spy := &splitSpy{buildErr: errors.New("boom")}
	tab := newSplitTestTab(t, spy)

	err := tab.RenderHTML(context.Background(), "<html><body>hi</body></html>")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

// TestTabRenderSplitCancellationSwallowed — superseded navigation
// cancels builds; the tab treats Canceled/DeadlineExceeded as a no-op.
func TestTabRenderSplitCancellationSwallowed(t *testing.T) {
	spy := &splitSpy{buildErr: context.Canceled}
	tab := newSplitTestTab(t, spy)

	assert.NoError(t, tab.RenderHTML(context.Background(), "<html><body>hi</body></html>"))
	assert.NoError(t, tab.RenderParsedContent(context.Background(), &html.Node{Type: html.ElementNode, Data: "body"}, nil))
}

// legacySpy implements HTMLRenderer without frameSplitter and returns a
// real canvas object so the legacy single-phase path can publish it.
type legacySpy struct {
	MockHTMLRendererComp
}

func (s *legacySpy) RenderHTML(ctx context.Context, _ string) (fyne.CanvasObject, error) {
	return canvas.NewRectangle(color.RGBA{R: 255, G: 255, B: 255, A: 255}), nil
}

func (s *legacySpy) RenderParsed(ctx context.Context, _ *html.Node, _ []renderer.ExternalCSS) (fyne.CanvasObject, error) {
	return canvas.NewRectangle(color.RGBA{R: 255, G: 255, B: 255, A: 255}), nil
}

// TestTabRenderLegacyFallback — renderers without frameSplitter keep the
// legacy single-phase path marshalled onto the UI thread.
func TestTabRenderLegacyFallback(t *testing.T) {
	spy := &legacySpy{}
	tab := newSplitTestTab(t, spy)

	require.NoError(t, tab.RenderHTML(context.Background(), "<html><body>hi</body></html>"))
	require.NoError(t, tab.RenderParsedContent(context.Background(), &html.Node{Type: html.ElementNode, Data: "body"}, nil))
}
