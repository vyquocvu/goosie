package ui

import (
	"context"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestBrowserDisplayListButtonCreated(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.displayListButton)
	assert.Equal(t, "DL", browser.displayListButton.Text)
}

func TestBrowserShowDisplayListNoRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	browser.showDisplayListDialog()
}

func TestBrowserShowDisplayListWithSummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.htmlRenderer = &MockHTMLRendererComp{
		summary: map[string]int{"Text": 5, "Rect": 3, "Border": 2},
	}

	browser.showDisplayListDialog()
}

func TestBrowserShowDisplayListWithEmptySummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.htmlRenderer = &MockHTMLRendererComp{
		summary: map[string]int{},
	}

	browser.showDisplayListDialog()
}

func TestBrowserShowDisplayListWithNilSummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	tab := browser.ActiveTab()
	tab.htmlRenderer = &MockHTMLRendererComp{
		summary: nil,
	}

	browser.showDisplayListDialog()
}

func TestBrowserDisplayListButtonInNavBar(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	w := app.NewWindow("test")
	browser := newBrowserInternal(app, w)

	assert.NotNil(t, browser.displayListButton)
	assert.Equal(t, "DL", browser.displayListButton.Text)

	browser.displayListButton.OnTapped()
}

func TestDisplayListTypeOrder(t *testing.T) {
	order := displayListTypeOrder()
	assert.Equal(t, "Text", order[0])
	assert.Equal(t, "PopClip", order[9])
	assert.Len(t, order, 10)
}

// MockHTMLRendererComp implements HTMLRenderer with configurable summary.
type MockHTMLRendererComp struct {
	summary             map[string]int
	dirtyOverlayEnabled bool
}

func (m *MockHTMLRendererComp) RenderHTML(ctx context.Context, s string) (fyne.CanvasObject, error) {
	return nil, nil
}
func (m *MockHTMLRendererComp) UpdateViewport() fyne.CanvasObject               { return nil }
func (m *MockHTMLRendererComp) SetCurrentURL(url string)                        {}
func (m *MockHTMLRendererComp) ResolveURL(url string) string                    { return url }
func (m *MockHTMLRendererComp) SetWindow(w fyne.Window)                         {}
func (m *MockHTMLRendererComp) SetNavigationCallback(callback func(url string)) {}
func (m *MockHTMLRendererComp) HitTest(x, y float32) (*renderer.RenderNode, *renderer.LayoutBox) {
	return nil, nil
}
func (m *MockHTMLRendererComp) SetInspectCallback(callback func(node *renderer.RenderNode, layout *renderer.LayoutBox)) {
}
func (m *MockHTMLRendererComp) GetRoot() *renderer.RenderNode         { return nil }
func (m *MockHTMLRendererComp) Refresh()                              {}
func (m *MockHTMLRendererComp) SetRefreshCallback(callback func())    {}
func (m *MockHTMLRendererComp) SetSubmitting(submitting bool)         {}
func (m *MockHTMLRendererComp) SetCSP(p *goosienet.CSPPolicy)         {}
func (m *MockHTMLRendererComp) GetDisplayListSummary() map[string]int { return m.summary }
func (m *MockHTMLRendererComp) SetDirtyOverlayEnabled(enabled bool) {
	m.dirtyOverlayEnabled = enabled
}
func (m *MockHTMLRendererComp) DirtyOverlayEnabled() bool { return m.dirtyOverlayEnabled }
