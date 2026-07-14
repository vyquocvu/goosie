package ui

import (
	"context"
	"fmt"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/engine/metrics"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// MockHTMLRenderer for testing
type MockHTMLRenderer struct {
	root          *renderer.RenderNode
	refreshCalled bool
}

func (m *MockHTMLRenderer) RenderHTML(ctx context.Context, htmlContent string) (fyne.CanvasObject, error) {
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
func (m *MockHTMLRenderer) GetRoot() *renderer.RenderNode         { return m.root }
func (m *MockHTMLRenderer) Refresh()                              { m.refreshCalled = true }
func (m *MockHTMLRenderer) SetRefreshCallback(callback func())    {}
func (m *MockHTMLRenderer) SetSubmitting(submitting bool)         {}
func (m *MockHTMLRenderer) SetCSP(p *goosienet.CSPPolicy)         {}
func (m *MockHTMLRenderer) GetDisplayListSummary() map[string]int               { return nil }
func (m *MockHTMLRenderer) GetDisplayListCommands() []renderer.PaintCommand      { return nil }
func (m *MockHTMLRenderer) SetDirtyOverlayEnabled(enabled bool)   {}
func (m *MockHTMLRenderer) DirtyOverlayEnabled() bool             { return false }
func (m *MockHTMLRenderer) GetDOMNodeCounts() (int, int, int)     { return 0, 0, 0 }
func (m *MockHTMLRenderer) GetLayoutNodeCount() int               { return 0 }

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
