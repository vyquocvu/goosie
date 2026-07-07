package ui

import (
	"fmt"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// MockHTMLRenderer for testing
type MockHTMLRenderer struct {
	root          *renderer.RenderNode
	refreshCalled bool
}

func (m *MockHTMLRenderer) RenderHTML(htmlContent string) (fyne.CanvasObject, error) {
	return nil, nil
}
func (m *MockHTMLRenderer) UpdateViewport() fyne.CanvasObject               { return nil }
func (m *MockHTMLRenderer) SetCurrentURL(url string)                        {}
func (m *MockHTMLRenderer) ResolveURL(url string) string                    { return url }
func (m *MockHTMLRenderer) SetWindow(w fyne.Window)                         {}
func (m *MockHTMLRenderer) SetNavigationCallback(callback func(url string)) {}
func (m *MockHTMLRenderer) HitTest(x, y float32) (*renderer.RenderNode, *renderer.LayoutBox) {
	return nil, nil
}
func (m *MockHTMLRenderer) SetInspectCallback(callback func(node *renderer.RenderNode, layout *renderer.LayoutBox)) {
}
func (m *MockHTMLRenderer) GetRoot() *renderer.RenderNode      { return m.root }
func (m *MockHTMLRenderer) Refresh()                           { m.refreshCalled = true }
func (m *MockHTMLRenderer) SetRefreshCallback(callback func()) {}

func TestNewInspectPanel(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	panel := NewInspectPanel(nil)
	assert.NotNil(t, panel)
	assert.NotNil(t, panel.GetContainer())
	assert.NotNil(t, panel.CanvasObject())
}

func TestInspectPanel_SetRenderer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"
	root.ID = 1

	mockRenderer := &MockHTMLRenderer{root: root}
	panel := NewInspectPanel(nil)

	panel.SetRenderer(mockRenderer)

	// Since we can't easily access unexported fields like rootNode in test unless in same package
	// But we are in same package 'ui'
	assert.Equal(t, root, panel.rootNode)
	assert.NotEmpty(t, panel.nodeMap)
	assert.Contains(t, panel.nodeMap, fmt.Sprintf("%d", root.ID))
}

func TestInspectPanel_SetElement(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"
	root.ID = 1

	child := renderer.NewRenderNode(renderer.NodeTypeElement)
	child.TagName = "p"
	child.ID = 2
	root.AddChild(child)

	mockRenderer := &MockHTMLRenderer{root: root}
	panel := NewInspectPanel(nil)
	panel.SetRenderer(mockRenderer)

	// Test selecting root
	panel.SetElement(root, nil)
	assert.Equal(t, root, panel.selectedNode)

	// Test selecting child
	panel.SetElement(child, nil)
	assert.Equal(t, child, panel.selectedNode)
}

func TestInspectPanel_PerformSearch(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"
	root.ID = 1

	child := renderer.NewRenderNode(renderer.NodeTypeElement)
	child.TagName = "span"
	child.ID = 2
	child.SetAttribute("class", "foo")
	root.AddChild(child)

	child2 := renderer.NewRenderNode(renderer.NodeTypeElement)
	child2.TagName = "a"
	child2.ID = 3
	child2.SetAttribute("id", "bar")
	root.AddChild(child2)

	mockRenderer := &MockHTMLRenderer{root: root}
	panel := NewInspectPanel(nil)
	panel.SetRenderer(mockRenderer)

	// Search by tag
	panel.PerformSearch("span")
	assert.Equal(t, child, panel.selectedNode)

	// Search by class
	panel.PerformSearch(".foo")
	assert.Equal(t, child, panel.selectedNode)

	// Search by ID
	panel.PerformSearch("#bar")
	assert.Equal(t, child2, panel.selectedNode)

	// Search not found
	panel.selectedNode = nil
	panel.PerformSearch("nonexistent")
	assert.Nil(t, panel.selectedNode)
}

func TestInspectPanel_Refresh(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"
	root.ID = 1

	mockRenderer := &MockHTMLRenderer{root: root}
	panel := NewInspectPanel(nil)
	panel.SetRenderer(mockRenderer)

	panel.refreshRenderer()
	assert.True(t, mockRenderer.refreshCalled)
}
