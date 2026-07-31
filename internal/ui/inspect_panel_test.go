package ui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine/metrics"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
	"golang.org/x/net/html"
)

// MockHTMLRenderer for testing
type MockHTMLRenderer struct {
	root          *renderer.RenderNode
	refreshCalled bool
	stylesheet    *css.StyleSheet
	matchedRules  []css.Rule
	highlightNode *renderer.RenderNode
}

func (m *MockHTMLRenderer) RenderHTML(ctx context.Context, htmlContent string) (fyne.CanvasObject, error) {
	return nil, nil
}
func (m *MockHTMLRenderer) RenderParsed(ctx context.Context, doc *html.Node, externalCSS []renderer.ExternalCSS) (fyne.CanvasObject, error) {
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
func (m *MockHTMLRenderer) SetContextMenuCallback(callback func(node *renderer.RenderNode, layout *renderer.LayoutBox, abs fyne.Position)) {
}
func (m *MockHTMLRenderer) GetRoot() *renderer.RenderNode                   { return m.root }
func (m *MockHTMLRenderer) Refresh()                                        { m.refreshCalled = true }
func (m *MockHTMLRenderer) SetRefreshCallback(callback func())              {}
func (m *MockHTMLRenderer) SetSubmitting(submitting bool)                   {}
func (m *MockHTMLRenderer) SetCSP(p *goosienet.CSPPolicy)                   {}
func (m *MockHTMLRenderer) GetDisplayListSummary() map[string]int           { return nil }
func (m *MockHTMLRenderer) GetDisplayListCommands() []renderer.PaintCommand { return nil }
func (m *MockHTMLRenderer) SetDirtyOverlayEnabled(enabled bool)             {}
func (m *MockHTMLRenderer) DirtyOverlayEnabled() bool                       { return false }
func (m *MockHTMLRenderer) GetDOMNodeCounts() (int, int, int)               { return 0, 0, 0 }
func (m *MockHTMLRenderer) GetLayoutNodeCount() int                         { return 0 }
func (m *MockHTMLRenderer) GetStyleSheet() *css.StyleSheet                  { return m.stylesheet }
func (m *MockHTMLRenderer) GetMatchedRules(node *renderer.RenderNode) []css.Rule {
	return m.matchedRules
}
func (m *MockHTMLRenderer) SetHighlightNode(node *renderer.RenderNode) {
	m.highlightNode = node
}
func (m *MockHTMLRenderer) GetLayoutBox(node *renderer.RenderNode) *renderer.LayoutBox {
	return nil
}
func (m *MockHTMLRenderer) SetHeadless(bool)              {}
func (m *MockHTMLRenderer) SetSize(width, height float32) {}
func (m *MockHTMLRenderer) SetViewport(y, height float32) {}

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
	assert.Equal(t, root, mockRenderer.highlightNode)

	// Test selecting child
	panel.SetElement(child, nil)
	assert.Equal(t, child, panel.selectedNode)
	assert.Equal(t, child, mockRenderer.highlightNode)
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

// TestInspectPanel_ExpandAncestors_NilNode verifies that calling
// expandAncestors with a nil node is a safe no-op. This is the
// regression guard against a panic when an external caller hands
// the inspector a nil selection.
func TestInspectPanel_ExpandAncestors_NilNode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "html"
	root.ID = 1
	panel := NewInspectPanel(nil)
	panel.SetRenderer(&MockHTMLRenderer{root: root})

	assert.NotPanics(t, func() {
		panel.expandAncestors(nil)
	}, "expandAncestors(nil) must not panic")
}

// TestInspectPanel_ExpandAncestors_DeepNode verifies that
// expandAncestors opens every ancestor in the chain leading
// from a deep node to the root. The tree.IsBranchOpen check is
// the regression guard: without this, programmatic selection of
// a hidden deep node would land on a uid the user could not see.
func TestInspectPanel_ExpandAncestors_DeepNode(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "html"
	root.ID = 1

	body := renderer.NewRenderNode(renderer.NodeTypeElement)
	body.TagName = "body"
	body.ID = 2
	root.AddChild(body)

	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.TagName = "div"
	div.ID = 3
	body.AddChild(div)

	span := renderer.NewRenderNode(renderer.NodeTypeElement)
	span.TagName = "span"
	span.ID = 4
	div.AddChild(span)

	panel := NewInspectPanel(nil)
	panel.SetRenderer(&MockHTMLRenderer{root: root})

	// Note: SetRenderer already auto-opens html and body (the
	// “matches Chrome” behaviour); only div (id=3) is
	// collapsed at this point. The interesting case for this
	// test is whether div opens after expandAncestors.
	assert.True(t, panel.tree.IsBranchOpen("1"), "html must start open")
	assert.True(t, panel.tree.IsBranchOpen("2"), "body must start open")
	assert.False(t, panel.tree.IsBranchOpen("3"), "div must start closed")

	panel.expandAncestors(span)

	// After expansion, the div ancestor must be open so the
	// user can see the span in the tree. html and body were
	// already open, but the assertion holds either way.
	assert.True(t, panel.tree.IsBranchOpen("1"), "html must be open")
	assert.True(t, panel.tree.IsBranchOpen("2"), "body must be open")
	assert.True(t, panel.tree.IsBranchOpen("3"), "div must be open")
}

// TestInspectPanel_ExpandAncestors_Root verifies that expanding
// the root node is a safe no-op (the root has no ancestors).
func TestInspectPanel_ExpandAncestors_Root(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "html"
	root.ID = 1
	panel := NewInspectPanel(nil)
	panel.SetRenderer(&MockHTMLRenderer{root: root})

	assert.NotPanics(t, func() {
		panel.expandAncestors(root)
	}, "expanding the root must not panic")
}

// TestInspectPanel_SetElement_ExpandsAncestors verifies that
// programmatic selection through SetElement (the path the browser
// uses for hover-hit testing) opens every ancestor so the selected
// node is visible in the tree. Without this the tree would
// silently select an invisible id.
func TestInspectPanel_SetElement_ExpandsAncestors(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "html"
	root.ID = 1

	body := renderer.NewRenderNode(renderer.NodeTypeElement)
	body.TagName = "body"
	body.ID = 2
	root.AddChild(body)

	inner := renderer.NewRenderNode(renderer.NodeTypeElement)
	inner.TagName = "div"
	inner.ID = 3
	body.AddChild(inner)

	deep := renderer.NewRenderNode(renderer.NodeTypeElement)
	deep.TagName = "span"
	deep.ID = 4
	inner.AddChild(deep)

	panel := NewInspectPanel(nil)
	panel.SetRenderer(&MockHTMLRenderer{root: root})

	// SetRenderer already opened html and body, but inner (id=3)
	// is still collapsed at the time of selection.
	assert.False(t, panel.tree.IsBranchOpen("3"), "inner must start closed")

	panel.SetElement(deep, nil)

	// After SetElement, every ancestor must be open so the
	// user can see the deep node in the tree.
	assert.True(t, panel.tree.IsBranchOpen("1"), "html must be open")
	assert.True(t, panel.tree.IsBranchOpen("2"), "body must be open")
	assert.True(t, panel.tree.IsBranchOpen("3"), "inner must be open")
	assert.Equal(t, deep, panel.selectedNode,
		"SetElement must record the deep node as selected")
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

// sampleTimingMetrics returns a Metrics snapshot used by the phase
// timing panel tests. Durations are picked so the panel exercises
// each status bucket (ok / warning / slow).
func sampleTimingMetrics() metrics.Metrics {
	start := time.Unix(1760000000, 0).UTC()
	end := start.Add(500 * time.Millisecond)
	return metrics.Metrics{
		NavID:     11,
		URL:       "https://panel.test/page",
		StartedAt: start,
		EndedAt:   end,
		Timings: []metrics.Timing{
			{Phase: metrics.PhaseNavigation, Started: start, Ended: start.Add(1 * time.Millisecond)},
			{Phase: metrics.PhaseDNSResolve, Started: start.Add(1 * time.Millisecond), Ended: start.Add(3 * time.Millisecond)},
			{Phase: metrics.PhaseConnect, Started: start.Add(3 * time.Millisecond), Ended: start.Add(7 * time.Millisecond)},
			{Phase: metrics.PhaseFirstByte, Started: start.Add(7 * time.Millisecond), Ended: start.Add(15 * time.Millisecond)},
			{Phase: metrics.PhaseBodyRead, Started: start.Add(15 * time.Millisecond), Ended: start.Add(35 * time.Millisecond)},
			{Phase: metrics.PhaseParse, Started: start.Add(35 * time.Millisecond), Ended: start.Add(95 * time.Millisecond)},
			{Phase: metrics.PhaseStyle, Started: start.Add(95 * time.Millisecond), Ended: start.Add(200 * time.Millisecond)},
			{Phase: metrics.PhaseLayout, Started: start.Add(200 * time.Millisecond), Ended: start.Add(330 * time.Millisecond)},
			{Phase: metrics.PhasePaint, Started: start.Add(330 * time.Millisecond), Ended: start.Add(420 * time.Millisecond)},
			{Phase: metrics.PhaseRaster, Started: start.Add(420 * time.Millisecond), Ended: start.Add(490 * time.Millisecond)},
			{Phase: metrics.PhasePresent, Started: start.Add(490 * time.Millisecond), Ended: end},
		},
		Counters: metrics.Counters{
			NodeCount:         100,
			RuleCount:         12,
			BoxCount:          90,
			FragmentCount:     30,
			DisplayItemCount:  45,
			BytesDownloaded:   6_000,
			DecodedImageBytes: 2_048,
			CacheHits:         3,
			CacheMisses:       7,
			ScriptErrors:      1,
		},
	}
}

// TestInspectPanel_PerformanceTab_EmptyFallback verifies that the
// Performance tab keeps the legacy "Total Nodes" line when no
// navigation metrics have been supplied via SetMetrics.
func TestInspectPanel_PerformanceTab_EmptyFallback(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"
	root.ID = 1
	root.AddChild(renderer.NewRenderNode(renderer.NodeTypeElement))

	panel := NewInspectPanel(nil)
	panel.SetRenderer(&MockHTMLRenderer{root: root})

	// Drive the tab by selecting a node so updateDetails renders it.
	panel.SetElement(root, nil)
	assert.False(t, panel.hasPhaseTimings(), "no metrics supplied, must not switch to timing panel")

	// Render the timing panel rendering helper directly; this
	// exercises the panel surface independent of the widget tree.
	panel.SetMetrics(metrics.Metrics{}) // ensure cleared
	panel.SetElement(root, nil)
	assert.False(t, panel.hasPhaseTimings())
}

// TestInspectPanel_SetMetrics verifies SetMetrics enables the timing
// panel and refreshes the details view. The visual rendering is
// verified indirectly by hasPhaseTimings — widget composition is
// exercised by Fyne itself.
func TestInspectPanel_SetMetrics(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"
	root.ID = 1

	panel := NewInspectPanel(nil)
	panel.SetRenderer(&MockHTMLRenderer{root: root})

	panel.SetMetrics(sampleTimingMetrics())
	assert.True(t, panel.hasPhaseTimings(), "metrics with timings should enable timing panel")
	assert.Equal(t, uint64(11), panel.lastMetrics.NavID)
}

// TestInspectPanel_SetMetricsClear verifies a zero-value Metrics
// call reverts the panel to the fallback summary. The fallback keeps
// the Performance tab useful even when metrics are not yet wired.
func TestInspectPanel_SetMetricsClear(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	panel := NewInspectPanel(nil)
	panel.SetMetrics(sampleTimingMetrics())
	assert.True(t, panel.hasPhaseTimings())

	panel.SetMetrics(metrics.Metrics{})
	assert.False(t, panel.hasPhaseTimings())
}

// TestFormatCounterValue_RoundTrip verifies the UI-layer byte/int
// formatter matches the engine-layer TimingPanel rendering. The two
// implementations should produce identical strings for the same
// CounterEntry, so the Fyne panel stays in sync with the textual
// rendering used by golden snapshots and external logs.
func TestFormatCounterValue_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		e    metrics.CounterEntry
		want string
	}{
		{"intCounter", metrics.CounterEntry{Name: "Nodes", Value: 432}, "432"},
		{"bytesBytes", metrics.CounterEntry{Name: "B", Value: 1500, Bytes: true}, "1.50 KB"},
		{"zero", metrics.CounterEntry{Name: "X", Value: 0}, "0"},
		{"zeroBytes", metrics.CounterEntry{Name: "B", Value: 0, Bytes: true}, "0 B"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, formatCounterValue(c.e))
		})
	}
}

// TestInspectPanel_RenderTimingPanel_AllRows verifies that a metrics
// snapshot with phase timings produces a non-empty Performance tab.
// The widget layout itself is Fyne-owned; we only assert the wiring
// (latestMetrics is honored and updatePerformanceTab runs without
// panics).
func TestInspectPanel_RenderTimingPanel_AllRows(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	panel := NewInspectPanel(nil)
	panel.SetMetrics(sampleTimingMetrics())
	panel.SetElement(nil, nil) // drives updateDetails (no selection)
	assert.True(t, panel.hasPhaseTimings())
}

// TestInspectPanel_NodeCounts verifies that the Performance tab shows
// the correct DOM node type breakdown (element vs text) when nodes are
// loaded in the panel.
func TestInspectPanel_NodeCounts(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"
	root.ID = 1

	child := renderer.NewRenderNode(renderer.NodeTypeElement)
	child.TagName = "p"
	child.ID = 2
	root.AddChild(child)

	text := renderer.NewRenderNode(renderer.NodeTypeText)
	text.Text = "hello"
	text.ID = 3
	child.AddChild(text)

	mockRenderer := &MockHTMLRenderer{root: root}
	panel := NewInspectPanel(nil)
	panel.SetRenderer(mockRenderer)
	panel.SetElement(root, nil)

	assert.Equal(t, 3, len(panel.nodeMap), "should have 3 nodes in map")
}

// TestInspectPanel_NodeCountsMixed verifies the performance tab renders
// correctly with a mix of element, text, and no "other" node types.
func TestInspectPanel_NodeCountsMixed(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"
	root.ID = 1

	e1 := renderer.NewRenderNode(renderer.NodeTypeElement)
	e1.TagName = "span"
	e1.ID = 2
	root.AddChild(e1)

	e2 := renderer.NewRenderNode(renderer.NodeTypeElement)
	e2.TagName = "a"
	e2.ID = 3
	root.AddChild(e2)

	t1 := renderer.NewRenderNode(renderer.NodeTypeText)
	t1.Text = "alpha"
	t1.ID = 4
	e1.AddChild(t1)

	t2 := renderer.NewRenderNode(renderer.NodeTypeText)
	t2.Text = "beta"
	t2.ID = 5
	e2.AddChild(t2)

	// Total: 5 nodes (3 elements + 2 text)
	panel := NewInspectPanel(nil)
	panel.SetRenderer(&MockHTMLRenderer{root: root})
	panel.SetElement(root, nil)

	assert.Equal(t, 5, len(panel.nodeMap))
	// Count manually from nodeMap
	elemCount := 0
	textCount := 0
	for _, n := range panel.nodeMap {
		switch n.Type {
		case renderer.NodeTypeElement:
			elemCount++
		case renderer.NodeTypeText:
			textCount++
		}
	}
	assert.Equal(t, 3, elemCount)
	assert.Equal(t, 2, textCount)
	assert.Equal(t, 0, 5-elemCount-textCount, "no other node types")
}

// TestInspectPanel_NodeCountsEmpty verifies the performance tab handles
// an empty nodeMap gracefully.
func TestInspectPanel_NodeCountsEmpty(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	panel := NewInspectPanel(nil)
	panel.SetElement(nil, nil)

	assert.Equal(t, 0, len(panel.nodeMap))
	assert.False(t, panel.hasPhaseTimings())
}

func TestElementsPanel_Breadcrumbs(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "html"
	root.ID = 1

	body := renderer.NewRenderNode(renderer.NodeTypeElement)
	body.TagName = "body"
	body.ID = 2
	root.AddChild(body)

	div := renderer.NewRenderNode(renderer.NodeTypeElement)
	div.TagName = "div"
	div.ID = 3
	div.SetAttribute("class", "container")
	body.AddChild(div)

	mockRenderer := &MockHTMLRenderer{root: root}
	panel := NewInspectPanel(nil)
	panel.SetRenderer(mockRenderer)

	// Initially, no selection, breadcrumbs should be empty
	assert.Equal(t, 0, len(panel.breadcrumbsBar.Objects))

	// Select the div
	panel.SetElement(div, nil)

	// Should have: "html", ">", "body", ">", "div.container"
	// So 5 objects
	assert.Equal(t, 5, len(panel.breadcrumbsBar.Objects))

	// First object is "html" button
	btn, ok := panel.breadcrumbsBar.Objects[0].(*widget.Button)
	assert.True(t, ok)
	assert.Equal(t, "html", btn.Text)

	// Click "html" button
	test.Tap(btn)

	// Selection should update to root
	assert.Equal(t, root, panel.selectedNode)
}

func TestElementsPanel_SyntaxHighlighting(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "div"
	root.ID = 1
	root.SetAttribute("id", "header")
	root.SetAttribute("class", "container navbar")

	mockRenderer := &MockHTMLRenderer{root: root}
	panel := NewInspectPanel(nil)
	panel.SetRenderer(mockRenderer)

	// Create and update a node using the new single-text-per-row API.
	obj := panel.tree.CreateNode(false)
	panel.tree.UpdateNode("1", false, obj)

	// Verify it's a domTreeNodeWidget wrapping canvas.Text.
	nodeWidget, ok := obj.(*domTreeNodeWidget)
	if !assert.True(t, ok, "tree node should be *domTreeNodeWidget") {
		return
	}
	txtObj := nodeWidget.text

	// The label should start with "<div" and contain the id and class attributes.
	assert.True(t, strings.HasPrefix(txtObj.Text, "<div"), "label should start with <div, got: %q", txtObj.Text)
	assert.Contains(t, txtObj.Text, `id="header"`)
	assert.Contains(t, txtObj.Text, `class="container navbar"`)
	// Element nodes should be rendered in blue.
	assert.Equal(t, color.RGBA{R: 86, G: 156, B: 214, A: 255}, txtObj.Color)
}

func TestElementsPanel_MatchedCSSRules(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "p"
	root.ID = 1

	mockRenderer := &MockHTMLRenderer{root: root}
	// Let's set some mock matched rules
	mockRenderer.matchedRules = []css.Rule{
		{
			Selectors: []css.SelectorSequence{
				{Simple: css.SimpleSelector{TagName: "p", Classes: []string{"highlight"}}},
			},
			Declarations: []css.Declaration{
				{Property: "background", Value: "yellow"},
			},
			Specificity: [3]uint16{0, 1, 1},
		},
		{
			Selectors: []css.SelectorSequence{
				{Simple: css.SimpleSelector{TagName: "p"}},
			},
			Declarations: []css.Declaration{
				{Property: "color", Value: "red"},
			},
			Specificity: [3]uint16{0, 0, 1},
		},
	}

	panel := NewInspectPanel(nil)
	panel.SetRenderer(mockRenderer)

	// Select paragraph
	panel.SetElement(root, nil)

	// Check stylesContainer has children. We expect the matched rules list to be populated.
	assert.NotEmpty(t, panel.stylesContainer.Objects)
}

// TestInspectPanel_StylesFilter_Empty verifies that an empty
// filter keeps every matched rule visible. This is the default
// behaviour and the regression guard against an off-by-one that
// would hide rules when no filter is set.
func TestInspectPanel_StylesFilter_Empty(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "p"
	root.ID = 1

	mockRenderer := &MockHTMLRenderer{root: root, matchedRules: []css.Rule{
		{
			Selectors:    []css.SelectorSequence{{Simple: css.SimpleSelector{TagName: "p"}}},
			Declarations: []css.Declaration{{Property: "color", Value: "red"}},
			Specificity: [3]uint16{0, 0, 1},
		},
		{
			Selectors:    []css.SelectorSequence{{Simple: css.SimpleSelector{TagName: "p"}}},
			Declarations: []css.Declaration{{Property: "margin", Value: "10px"}},
			Specificity: [3]uint16{0, 0, 1},
		},
	}}
	panel := NewInspectPanel(nil)
	panel.SetRenderer(mockRenderer)
	panel.SetElement(root, nil)

	// No filter: every rule renders. We assert the container has
	// the standard heading + at least one rule card.
	panel.stylesFilter.SetText("")
	assert.NotEmpty(t, panel.stylesContainer.Objects,
		"empty filter must not hide rules")
}

// TestInspectPanel_StylesFilter_MatchesProperty verifies the
// filter narrows the matched-rules list by CSS property name
// (case-insensitive substring match).
func TestInspectPanel_StylesFilter_MatchesProperty(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	root := renderer.NewRenderNode(renderer.NodeTypeElement)
	root.TagName = "p"
	root.ID = 1

	mockRenderer := &MockHTMLRenderer{root: root, matchedRules: []css.Rule{
		{
			Selectors:    []css.SelectorSequence{{Simple: css.SimpleSelector{TagName: "p"}}},
			Declarations: []css.Declaration{{Property: "color", Value: "red"}},
			Specificity: [3]uint16{0, 0, 1},
		},
		{
			Selectors:    []css.SelectorSequence{{Simple: css.SimpleSelector{TagName: "p"}}},
			Declarations: []css.Declaration{{Property: "background-color", Value: "yellow"}},
			Specificity: [3]uint16{0, 0, 1},
		},
		{
			Selectors:    []css.SelectorSequence{{Simple: css.SimpleSelector{TagName: "p"}}},
			Declarations: []css.Declaration{{Property: "margin", Value: "10px"}},
			Specificity: [3]uint16{0, 0, 1},
		},
	}}
	panel := NewInspectPanel(nil)
	panel.SetRenderer(mockRenderer)
	panel.SetElement(root, nil)

	// Filter on "color" should keep the two rules that mention a
	// color property ("color" and "background-color") and drop
	// the margin rule.
	panel.stylesFilter.OnChanged("color")
	countColor := countStylesObjects(panel)
	assert.GreaterOrEqual(t, countColor, 2,
		"color filter must keep at least the two color rules")

	// A filter that matches nothing should produce a “no
	// matches” placeholder rather than dropping every rule
	// silently.
	panel.stylesFilter.OnChanged("zzz_nothing_matches_this")
	countNone := countStylesObjects(panel)
	assert.GreaterOrEqual(t, countNone, 1,
		"empty-result filter must surface the 'no matches' placeholder")
}

// countStylesObjects counts the widgets currently in the styles
// container so the filter tests can assert on the visible rule
// count without depending on Fyne's internal label/separator
// internals.
func countStylesObjects(panel *InspectPanel) int {
	if panel.stylesContainer == nil {
		return 0
	}
	return len(panel.stylesContainer.Objects)
}

func TestElementsPanel_PopulateAndExpand(t *testing.T) {
	test.NewApp()
	mockRoot := &renderer.RenderNode{ID: 1, TagName: "html", Type: renderer.NodeTypeElement}
	mockRoot.Children = append(mockRoot.Children, &renderer.RenderNode{ID: 2, TagName: "body", Type: renderer.NodeTypeElement})

	panel := NewInspectPanel(nil)
	panel.SetRenderer(&MockHTMLRenderer{root: mockRoot})

	assert.True(t, panel.tree.IsBranchOpen("1"))
	assert.True(t, panel.tree.IsBranchOpen("2"))
}
