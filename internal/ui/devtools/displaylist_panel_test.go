package devtools

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/renderer"
)

type mockRendererProvider struct {
	summary         map[string]int
	commands        []renderer.PaintCommand
	domNodeCounts   (func() (total int, elements int, text int))
	layoutNodeCount int
	root            *renderer.RenderNode
	dirtyEnabled    bool
}

func (m *mockRendererProvider) GetDisplayListSummary() map[string]int {
	return m.summary
}
func (m *mockRendererProvider) GetDisplayListCommands() []renderer.PaintCommand {
	return m.commands
}
func (m *mockRendererProvider) GetDOMNodeCounts() (int, int, int) {
	if m.domNodeCounts != nil {
		return m.domNodeCounts()
	}
	return 0, 0, 0
}
func (m *mockRendererProvider) GetLayoutNodeCount() int       { return m.layoutNodeCount }
func (m *mockRendererProvider) GetRoot() *renderer.RenderNode { return m.root }
func (m *mockRendererProvider) DirtyOverlayEnabled() bool     { return m.dirtyEnabled }
func (m *mockRendererProvider) SetDirtyOverlayEnabled(v bool) { m.dirtyEnabled = v }
func (m *mockRendererProvider) Refresh()                      {}

func TestDisplayListPanel_InitialState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newDisplayListPanelContent(func() *TabContext { return &TabContext{} })
	assert.NotNil(t, p)
}

func TestDisplayListPanel_NoRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{}
	p := newDisplayListPanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}

func TestDisplayListPanel_EmptyCommands(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		Renderer: &mockRendererProvider{
			summary:  map[string]int{},
			commands: []renderer.PaintCommand{},
		},
	}
	p := newDisplayListPanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}

func TestDisplayListPanel_WithCommands(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		Renderer: &mockRendererProvider{
			summary: map[string]int{"Text": 1, "Rect": 2, "Image": 1},
			commands: []renderer.PaintCommand{
				{Type: renderer.PaintRect, Box: renderer.Rect{X: 0, Y: 0, Width: 100, Height: 50}},
				{Type: renderer.PaintText, Text: "Hello", FontSize: 14, Box: renderer.Rect{X: 10, Y: 10, Width: 50, Height: 20}},
				{Type: renderer.PaintRect, Box: renderer.Rect{X: 0, Y: 50, Width: 200, Height: 100}},
				{Type: renderer.PaintImage, ImageSrc: "test.png", Box: renderer.Rect{X: 0, Y: 150, Width: 32, Height: 32}},
				{Type: renderer.PaintLink, LinkURL: "https://example.com", Box: renderer.Rect{X: 0, Y: 200, Width: 100, Height: 20}},
				{Type: renderer.PaintBorder, Box: renderer.Rect{X: 0, Y: 220, Width: 50, Height: 50}, StrokeWidth: 2},
				{Type: renderer.PaintButton, ButtonText: "Click", Box: renderer.Rect{X: 0, Y: 280, Width: 60, Height: 30}},
				{Type: renderer.PaintInput, InputType: "text", InputValue: "val", Placeholder: "enter", Box: renderer.Rect{X: 0, Y: 320, Width: 100, Height: 24}},
				{Type: renderer.PushClip, Box: renderer.Rect{X: 0, Y: 0, Width: 500, Height: 500}, ClipOverflow: "hidden"},
				{Type: renderer.PopClip},
			},
		},
	}
	p := newDisplayListPanelContent(func() *TabContext { return ctx })
	assert.NotNil(t, p)
}

func TestDisplayList_TypeOrder(t *testing.T) {
	expected := []string{"Text", "Rect", "Image", "Link", "Border", "Button", "Input", "Textarea", "PushClip", "PopClip"}
	assert.Equal(t, expected, displayListTypeOrder())
}
