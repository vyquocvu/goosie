package devtools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestAccessibilityPanelEmpty(t *testing.T) {
	panel := newAccessibilityPanel(nil)
	assert.NotNil(t, panel)
}

func TestAccessibilityPanel_NilTabContext(t *testing.T) {
	panel := newAccessibilityPanel(nil).(*accessibilityPanel)
	panel.RefreshFrom(nil)
	assert.Contains(t, panel.label.Text, "No accessibility data")
}

func TestAccessibilityPanel_NilProvider(t *testing.T) {
	panel := newAccessibilityPanel(nil).(*accessibilityPanel)
	ctx := &TabContext{Accessibility: nil}
	panel.RefreshFrom(ctx)
	assert.Contains(t, panel.label.Text, "No accessibility data")
}

func TestAccessibilityPanel_EmptyTree(t *testing.T) {
	panel := newAccessibilityPanel(nil).(*accessibilityPanel)
	ctx := &TabContext{
		Accessibility: &mockA11yProvider{nodes: nil},
	}
	panel.RefreshFrom(ctx)
	assert.Contains(t, panel.label.Text, "No accessibility nodes")
}

func TestAccessibilityPanel_RendersTree(t *testing.T) {
	panel := newAccessibilityPanel(nil).(*accessibilityPanel)
	ctx := &TabContext{
		Accessibility: &mockA11yProvider{
			nodes: []*A11yNode{
				{
					Tag:  "nav",
					Role: "navigation",
					Name: "",
					Children: []*A11yNode{
						{Tag: "a", Role: "link", Name: "/home"},
						{Tag: "a", Role: "link", Name: "/about"},
					},
				},
				{Tag: "main", Role: "main", Children: []*A11yNode{
					{Tag: "h1", Role: "heading", Name: "Welcome"},
					{Tag: "p", Role: "presentation",
						Children: []*A11yNode{
							{Tag: "", Role: "text", Description: "Hello world"},
						},
					},
				}},
			},
		},
	}
	panel.RefreshFrom(ctx)
	assert.Contains(t, panel.label.Text, "navigation")
	assert.Contains(t, panel.label.Text, "/home")
	assert.Contains(t, panel.label.Text, "main")
	assert.Contains(t, panel.label.Text, "heading")
	assert.Contains(t, panel.label.Text, "Hello world")
	// Verify monospace is set
	assert.True(t, panel.label.TextStyle.Monospace)
}

func TestA11yFormat(t *testing.T) {
	nodes := []*A11yNode{
		{Tag: "div", Role: "presentation", Children: []*A11yNode{
			{Tag: "button", Role: "button", Name: "Click me"},
		}},
	}
	formatted := formatA11yTree(nodes, "")
	assert.Contains(t, formatted, "<div>")
	assert.Contains(t, formatted, "role=presentation")
	assert.Contains(t, formatted, "<button>")
	assert.Contains(t, formatted, "role=button")
	assert.Contains(t, formatted, `"Click me"`)
}

// --- mock a11y provider ---

type mockA11yProvider struct {
	nodes []*A11yNode
}

func (m *mockA11yProvider) GetAccessibilityTree() []*A11yNode {
	return m.nodes
}

// --- walkA11yNode tests ---

func buildRenderNode(tag string, attrs map[string]string, children ...*renderer.RenderNode) *renderer.RenderNode {
	return &renderer.RenderNode{
		TagName:  tag,
		Attrs:    attrs,
		Children: children,
	}
}

func TestWalkA11yNode_RoleInference(t *testing.T) {
	tests := []struct {
		tag      string
		attrs    map[string]string
		wantRole string
	}{
		{tag: "a", attrs: map[string]string{"href": "/"}, wantRole: "link"},
		{tag: "a", attrs: nil, wantRole: "anchor"},
		{tag: "button", wantRole: "button"},
		{tag: "img", wantRole: "img"},
		{tag: "input", attrs: map[string]string{"type": "checkbox"}, wantRole: "checkbox"},
		{tag: "input", attrs: map[string]string{"type": "text"}, wantRole: "textbox"},
		{tag: "nav", wantRole: "navigation"},
		{tag: "main", wantRole: "main"},
		{tag: "h1", wantRole: "heading"},
		{tag: "div", wantRole: "presentation"},
		{tag: "span", wantRole: "presentation"},
		{tag: "table", wantRole: "table"},
		{tag: "ul", wantRole: "list"},
		{tag: "li", wantRole: "listitem"},
		{tag: "unknown-tag", wantRole: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			node := buildRenderNode(tt.tag, tt.attrs)
			an := walkA11yNode(node)
			assert.NotNil(t, an)
			assert.Equal(t, tt.wantRole, an.Role)
		})
	}
}

func TestWalkA11yNode_ExplicitRole(t *testing.T) {
	node := buildRenderNode("div", map[string]string{"role": "button"})
	an := walkA11yNode(node)
	assert.Equal(t, "button", an.Role)
}

func TestWalkA11yNode_NameFromAriaLabel(t *testing.T) {
	node := buildRenderNode("button", map[string]string{"aria-label": "Close"})
	an := walkA11yNode(node)
	assert.Equal(t, "Close", an.Name)
}

func TestWalkA11yNode_NameFromAlt(t *testing.T) {
	node := buildRenderNode("img", map[string]string{"alt": "A photo"})
	an := walkA11yNode(node)
	assert.Equal(t, "A photo", an.Name)
}

func TestWalkA11yNode_TextNode(t *testing.T) {
	node := &renderer.RenderNode{
		Type: renderer.NodeTypeText,
		Text: "Hello world",
	}
	an := walkA11yNode(node)
	assert.Equal(t, "text", an.Role)
	assert.Equal(t, "Hello world", an.Description)
}

func TestWalkA11yNode_EmptyNode(t *testing.T) {
	an := walkA11yNode(nil)
	assert.Nil(t, an)
}
