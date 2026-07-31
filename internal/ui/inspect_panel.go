package ui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/engine/metrics"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// InspectPanel represents the comprehensive element inspector panel
type InspectPanel struct {
	container   *fyne.Container
	tree        *widget.Tree
	detailsTabs *container.AppTabs
	closeButton *widget.Button
	onClose     func()

	selectedNode   *renderer.RenderNode
	selectedLayout *renderer.LayoutBox

	htmlRenderer HTMLRenderer
	rootNode     *renderer.RenderNode
	nodeMap      map[string]*renderer.RenderNode // Map UID (string ID) to RenderNode

	// Details View Components
	propertiesContainer  *fyne.Container
	stylesContainer      *fyne.Container
	stylesFilter         *widget.Entry
	stylesFilterText     string
	layoutContainer      *fyne.Container
	performanceContainer *fyne.Container

	// lastMetrics is the most recent navigation metrics supplied by
	// the engine via SetMetrics. When zero-valued, the performance
	// tab falls back to a static "Total Nodes: N" line so the tab
	// is still useful while metrics are not yet wired in.
	lastMetrics metrics.Metrics

	// Search
	searchEntry *widget.Entry

	// State to prevent recursive updates
	updatingUI bool

	// onSelectNode is called whenever the selected element changes.
	onSelectNode func(node *renderer.RenderNode, layout *renderer.LayoutBox)

	// onScrollToNode is called when the "Scroll to Node" button is pressed.
	onScrollToNode func(x, y float32)

	// computedStyleView shows all CSS properties with search filter.
	computedStyleView *ComputedStyleView

	// inlineStyleEditor allows editing the element's inline style.
	inlineStyleEditor *InlineStyleEditor

	// statusBar shows live DOM node count and memory estimate.
	statusBar *widget.Label

	// breadcrumbsBar holds clickable ancestors.
	breadcrumbsBar *fyne.Container

	// lastRootID tracks the root node ID of the last SetRenderer call.
	// OpenBranch is only called once per unique root (i.e. per page load).
	// On subsequent calls with the same root (CSS reload, viewport scroll)
	// the user's manually collapsed/expanded state is preserved.
	lastRootID int64
}

// NewInspectPanel creates a new inspect panel
func NewInspectPanel(onClose func()) *InspectPanel {
	panel := &InspectPanel{
		onClose: onClose,
		nodeMap: make(map[string]*renderer.RenderNode),
	}

	// Create close button
	panel.closeButton = widget.NewButton("✕", func() {
		if panel.htmlRenderer != nil {
			panel.htmlRenderer.SetHighlightNode(nil)
			panel.refreshRenderer()
		}
		if panel.onClose != nil {
			panel.onClose()
		}
	})

	// Create Tree View
	panel.tree = widget.NewTree(
		func(id widget.TreeNodeID) []widget.TreeNodeID {
			node, ok := panel.nodeMap[id]
			if !ok {
				// If root
				if id == "" && panel.rootNode != nil {
					return []string{fmt.Sprintf("%d", panel.rootNode.ID)}
				}
				return nil
			}

			var children []string
			for _, child := range node.Children {
				children = append(children, fmt.Sprintf("%d", child.ID))
			}
			return children
		},
		func(id widget.TreeNodeID) bool {
			if id == "" {
				return panel.rootNode != nil
			}
			node, ok := panel.nodeMap[id]
			if !ok {
				return false
			}
			return len(node.Children) > 0
		},
		func(branch bool) fyne.CanvasObject {
			// Using our custom widget wrapper instead of raw canvas.Text.
			// This ensures Fyne's layout is correctly notified when size/text changes,
			// preventing alignment and color issues during recycling.
			return newDOMTreeNodeWidget("Node Template", color.Transparent)
		},
		func(id widget.TreeNodeID, branch bool, o fyne.CanvasObject) {
			node, ok := panel.nodeMap[id]
			if !ok {
				return
			}
			selected := panel.selectedNode != nil && panel.selectedNode.ID == node.ID
			updateDOMTreeNode(node, o, selected)
		},
	)

	panel.tree.OnSelected = func(id widget.TreeNodeID) {
		if node, ok := panel.nodeMap[id]; ok {
			panel.selectNode(node)
		}
	}

	// Create Details View
	panel.createDetailsView()

	// Search Bar
	panel.searchEntry = widget.NewEntry()
	panel.searchEntry.SetPlaceHolder("Search (tag, #id, .class)")
	panel.searchEntry.OnSubmitted = func(query string) {
		panel.PerformSearch(query)
	}
	searchBtn := widget.NewButton("Find", func() {
		panel.PerformSearch(panel.searchEntry.Text)
	})

	searchContainer := container.NewBorder(nil, nil, nil, searchBtn, panel.searchEntry)

	// Create Split Container
	// Left: Tree with search + breadcrumbs, Right: Details
	panel.breadcrumbsBar = container.NewHBox()
	breadcrumbsScroll := container.NewHScroll(panel.breadcrumbsBar)
	leftSide := container.NewBorder(searchContainer, breadcrumbsScroll, nil, nil, panel.tree)

	split := container.NewHSplit(
		leftSide,
		panel.detailsTabs,
	)
	split.Offset = 0.4

	// Top Bar
	topBar := container.NewBorder(
		nil, nil,
		widget.NewLabel("DOM Inspector"),
		container.NewHBox(panel.closeButton),
	)

	// Status bar
	panel.statusBar = widget.NewLabel("")
	panel.statusBar.TextStyle.Monospace = true

	panel.container = container.NewBorder(
		topBar, panel.statusBar, nil, nil,
		split,
	)

	return panel
}

func (ip *InspectPanel) PerformSearch(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return
	}

	// Simple search: iterate all nodes
	for id, node := range ip.nodeMap {
		match := false
		if strings.HasPrefix(query, "#") {
			if val, ok := node.GetAttribute("id"); ok && val == query[1:] {
				match = true
			}
		} else if strings.HasPrefix(query, ".") {
			if val, ok := node.GetAttribute("class"); ok && strings.Contains(val, query[1:]) {
				match = true
			}
		} else {
			if strings.EqualFold(node.TagName, query) {
				match = true
			}
		}

		if match {
			// Open every ancestor chain so the matched node is
			// visible. Without this the selection lands on a
			// hidden branch and the user has to expand it
			// manually to confirm the match landed in the right
			// place.
			ip.expandAncestors(node)
			ip.tree.Select(id)
			return // Stop after first match for now
		}
	}
}

// expandAncestors walks the parent chain from `node` to the root
// and opens every branch along the way. This is what lets a
// programmatically-selected node (search hit, hover, URL anchor
// scroll) actually become visible in the tree: a collapsed
// branch hides its descendants, so without this the tree.Select
// call selects an invisible id.
//
// The walk uses the cached parent pointer maintained by the
// render tree, which is always correct for nodes the panel has
// observed since SetRenderer. For nodes outside that set we fall
// back to a no-op; the caller can always call this again after
// SetRenderer.
func (ip *InspectPanel) expandAncestors(node *renderer.RenderNode) {
	if node == nil {
		return
	}
	current := node.Parent
	for current != nil {
		ip.tree.OpenBranch(fmt.Sprintf("%d", current.ID))
		current = current.Parent
	}
}

func (ip *InspectPanel) createDetailsView() {
	// Properties Tab
	ip.propertiesContainer = container.NewVBox()

	// Styles Tab
	ip.stylesContainer = container.NewVBox()
	ip.computedStyleView = NewComputedStyleView()
	ip.inlineStyleEditor = NewInlineStyleEditor(func(style string) {
		ip.refreshRenderer()
	})

	// Styles-tab filter: filters the matched-rules list by
	// property name. We allocate the widget here so it survives
	// tab rebuilds; the OnChanged handler simply calls
	// updateStylesTab() which re-runs the filter.
	ip.stylesFilter = widget.NewEntry()
	ip.stylesFilter.PlaceHolder = "Filter by property (e.g. color, margin)..."
	ip.stylesFilter.OnChanged = func(s string) {
		ip.stylesFilterText = strings.TrimSpace(strings.ToLower(s))
		ip.updateStylesTab()
	}

	// Layout Tab
	ip.layoutContainer = container.NewVBox()

	// Performance Tab
	ip.performanceContainer = container.NewVBox()

	ip.detailsTabs = container.NewAppTabs(
		container.NewTabItem("Properties", container.NewVScroll(ip.propertiesContainer)),
		// Styles: filter entry above the list of matched rules
		// so the user can narrow the list by CSS property name
		// (e.g. "color", "margin"). Empty filter shows all rules.
		container.NewTabItem("Styles", container.NewBorder(
			nil, ip.stylesFilter, nil, nil,
			container.NewVScroll(ip.stylesContainer),
		)),
		container.NewTabItem("Computed", container.NewBorder(nil, nil, nil, nil, ip.computedStyleView.CanvasObject())),
		container.NewTabItem("Layout", container.NewVScroll(ip.layoutContainer)),
		container.NewTabItem("Performance", container.NewVScroll(ip.performanceContainer)),
	)
}

// SetRenderer sets the HTML renderer and refreshes the tree.
// Branch open-state is preserved across CSS reloads and viewport updates;
// OpenBranch is only called once per new page (root node change).
func (ip *InspectPanel) SetRenderer(r HTMLRenderer) {
	if ip.updatingUI {
		return
	}
	ip.updatingUI = true
	defer func() {
		ip.updatingUI = false
	}()

	var rootChanged bool
	var newRoot *renderer.RenderNode
	if r != nil {
		newRoot = r.GetRoot()
		rootChanged = ip.rootNode != newRoot
	}

	if ip.htmlRenderer != nil && (ip.htmlRenderer != r || rootChanged) {
		ip.htmlRenderer.SetHighlightNode(nil)
		if ip.htmlRenderer != r {
			ip.refreshRenderer()
		}
	}

	ip.htmlRenderer = r
	if r != nil {
		ip.rootNode = newRoot
		ip.rebuildNodeMap()

		// Only auto-open branches when the root node changes (i.e. a new page
		// was loaded). On CSS-reload / tab-switch back / viewport updates the
		// same root is reused and we must NOT call OpenBranch again, otherwise
		// the user's manually-collapsed nodes (like <head>) will be forced open.
		newRootID := int64(0)
		if newRoot != nil {
			newRootID = newRoot.ID
		}
		if newRoot != nil && newRootID != ip.lastRootID {
			ip.lastRootID = newRootID
			// Auto-open: html root, then body only (not head — matches Chrome).
			ip.tree.OpenBranch(fmt.Sprintf("%d", newRoot.ID))
			for _, child := range newRoot.Children {
				// Open body automatically; leave head collapsed by default.
				if strings.EqualFold(child.TagName, "body") {
					ip.tree.OpenBranch(fmt.Sprintf("%d", child.ID))
				}
			}
		}

		ip.tree.Refresh()
		// Also update details if selection exists but might be stale
		if ip.selectedNode != nil {
			// Check if node still exists in new map
			if _, ok := ip.nodeMap[fmt.Sprintf("%d", ip.selectedNode.ID)]; ok {
				ip.htmlRenderer.SetHighlightNode(ip.selectedNode)
				ip.selectedLayout = ip.htmlRenderer.GetLayoutBox(ip.selectedNode)
				ip.updateDetails()
			} else {
				ip.selectedNode = nil
				ip.selectedLayout = nil
				ip.updateDetails()
			}
		}
	}
	ip.updateStatusBar()
}

// updateStatusBar refreshes the bottom status bar with DOM node counts.
func (ip *InspectPanel) updateStatusBar() {
	if ip.htmlRenderer == nil {
		ip.statusBar.SetText("")
		return
	}
	total, elements, text := ip.htmlRenderer.GetDOMNodeCounts()
	layoutCount := ip.htmlRenderer.GetLayoutNodeCount()
	if total > 0 {
		memEstimate := total * 256 // rough bytes-per-node estimate
		ip.statusBar.SetText(fmt.Sprintf("%d nodes (%d elem, %d text, %d layout)  ~%s",
			total, elements, text, layoutCount, formatBytes(int64(memEstimate))))
	} else {
		ip.statusBar.SetText(fmt.Sprintf("%d nodes in nodeMap", len(ip.nodeMap)))
	}
	ip.statusBar.Refresh()
}

// SetSelectNodeCallback sets a callback invoked when the selected node changes.
func (ip *InspectPanel) SetSelectNodeCallback(cb func(node *renderer.RenderNode, layout *renderer.LayoutBox)) {
	ip.onSelectNode = cb
}

// SetScrollToCallback sets a callback invoked when the "Scroll to Node" button is pressed.
// The callback receives the x and y position of the selected layout box.
func (ip *InspectPanel) SetScrollToCallback(cb func(x, y float32)) {
	ip.onScrollToNode = cb
}

// SetElement sets the element to inspect (called from renderer hit test)
func (ip *InspectPanel) SetElement(node *renderer.RenderNode, layout *renderer.LayoutBox) {
	if ip.updatingUI {
		return
	}
	// Skip redundant updates when hovering the same element
	if ip.selectedNode != nil && node != nil && ip.selectedNode.ID == node.ID {
		return
	}
	ip.updatingUI = true
	defer func() {
		ip.updatingUI = false
	}()

	// Update tree selection
	if node != nil {
		// Open every ancestor chain so the hit-tested node is
		// visible. This is the “sync” half of M1: hovering a
		// region of the page must reveal the corresponding tree
		// node, not just select an id the user cannot see.
		ip.expandAncestors(node)
		id := fmt.Sprintf("%d", node.ID)
		ip.tree.Select(id)
	}

	ip.selectedNode = node
	ip.selectedLayout = layout
	if ip.htmlRenderer != nil {
		ip.htmlRenderer.SetHighlightNode(node)
		ip.refreshRenderer()
	}
	ip.updateDetails()
	if ip.onSelectNode != nil {
		ip.onSelectNode(node, layout)
	}
}

func (ip *InspectPanel) selectNode(node *renderer.RenderNode) {
	if ip.updatingUI {
		return
	}
	ip.updatingUI = true
	defer func() {
		ip.updatingUI = false
	}()

	ip.selectedNode = node
	var layout *renderer.LayoutBox
	if ip.htmlRenderer != nil {
		if node != nil {
			layout = ip.htmlRenderer.GetLayoutBox(node)
			ip.htmlRenderer.SetHighlightNode(node)
		} else {
			ip.htmlRenderer.SetHighlightNode(nil)
		}
		ip.refreshRenderer()
	}
	ip.selectedLayout = layout
	ip.updateDetails()
	if ip.onSelectNode != nil {
		ip.onSelectNode(node, ip.selectedLayout)
	}
}

func (ip *InspectPanel) updateDetails() {
	ip.updateBreadcrumbs()
	if ip.selectedNode == nil {
		ip.propertiesContainer.Objects = nil
		ip.stylesContainer.Objects = nil
		ip.layoutContainer.Objects = nil
		ip.performanceContainer.Objects = nil

		ip.propertiesContainer.Refresh()
		ip.stylesContainer.Refresh()
		ip.layoutContainer.Refresh()
		ip.performanceContainer.Refresh()
		return
	}

	ip.updatePropertiesTab()
	ip.updateStylesTab()
	ip.updateLayoutTab()
	ip.updatePerformanceTab()
}

func (ip *InspectPanel) updateBreadcrumbs() {
	if ip.breadcrumbsBar == nil {
		return
	}
	ip.breadcrumbsBar.Objects = nil
	if ip.selectedNode == nil {
		ip.breadcrumbsBar.Refresh()
		return
	}

	// Gather ancestors from selectedNode up to rootNode
	var ancestors []*renderer.RenderNode
	current := ip.selectedNode
	for current != nil {
		ancestors = append(ancestors, current)
		current = current.Parent
	}

	// Reverse so it starts from root down to selectedNode
	for i, j := 0, len(ancestors)-1; i < j; i, j = i+1, j-1 {
		ancestors[i], ancestors[j] = ancestors[j], ancestors[i]
	}

	for idx, ancestor := range ancestors {
		if idx > 0 {
			// Separator
			sep := widget.NewLabel(">")
			sep.TextStyle.Bold = true
			ip.breadcrumbsBar.Objects = append(ip.breadcrumbsBar.Objects, sep)
		}

		ancestor := ancestor // capture variable
		// Build label text: e.g. "div.container" or "div#header"
		label := ancestor.TagName
		if id, ok := ancestor.GetAttribute("id"); ok {
			label += "#" + id
		} else if class, ok := ancestor.GetAttribute("class"); ok {
			// Just use the first class to keep breadcrumb concise
			classes := strings.Fields(class)
			if len(classes) > 0 {
				label += "." + classes[0]
			}
		}
		if label == "" && ancestor.Type == renderer.NodeTypeText {
			label = "text"
		}

		btn := widget.NewButton(label, func() {
			ip.tree.Select(fmt.Sprintf("%d", ancestor.ID))
			ip.selectNode(ancestor)
		})
		btn.Importance = widget.LowImportance
		ip.breadcrumbsBar.Objects = append(ip.breadcrumbsBar.Objects, btn)
	}

	ip.breadcrumbsBar.Refresh()
}

func (ip *InspectPanel) updatePropertiesTab() {
	ip.propertiesContainer.Objects = nil

	node := ip.selectedNode

	// Tag Name
	ip.propertiesContainer.Add(widget.NewLabelWithStyle("Tag Name", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	ip.propertiesContainer.Add(widget.NewLabel(node.TagName))

	// ID
	ip.propertiesContainer.Add(widget.NewLabelWithStyle("ID", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	idEntry := widget.NewEntry()
	if val, ok := node.GetAttribute("id"); ok {
		idEntry.SetText(val)
	}
	idEntry.OnSubmitted = func(s string) {
		node.SetAttribute("id", s)
		ip.refreshRenderer()
	}
	ip.propertiesContainer.Add(idEntry)

	// Classes
	ip.propertiesContainer.Add(widget.NewLabelWithStyle("Classes", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	classEntry := widget.NewEntry()
	if val, ok := node.GetAttribute("class"); ok {
		classEntry.SetText(val)
	}
	classEntry.OnSubmitted = func(s string) {
		node.SetAttribute("class", s)
		ip.refreshRenderer()
	}
	ip.propertiesContainer.Add(classEntry)

	// Text Content (if applicable)
	if node.Type == renderer.NodeTypeText || (len(node.Children) == 1 && node.Children[0].Type == renderer.NodeTypeText) {
		ip.propertiesContainer.Add(widget.NewLabelWithStyle("Text Content", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		textEntry := widget.NewMultiLineEntry()

		targetNode := node
		if node.Type == renderer.NodeTypeElement && len(node.Children) > 0 {
			targetNode = node.Children[0]
		}

		textEntry.SetText(targetNode.Text)
		// Add an "Update" button for text since multiline entry submission is tricky
		updateBtn := widget.NewButton("Update Text", func() {
			targetNode.Text = textEntry.Text
			ip.refreshRenderer()
		})

		ip.propertiesContainer.Add(textEntry)
		ip.propertiesContainer.Add(updateBtn)
	}

	// Other Attributes
	ip.propertiesContainer.Add(widget.NewLabelWithStyle("Attributes", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	// Filter out id and class as they are handled above
	for k, v := range node.Attrs {
		if k == "id" || k == "class" {
			continue
		}

		key := k // capture loop var
		val := v

		box := container.NewBorder(nil, nil, widget.NewLabel(key+":"), nil,
			container.NewHBox(
				widget.NewEntryWithData(bindingString(val, func(s string) {
					node.SetAttribute(key, s)
				})),
				widget.NewButton("✓", func() { ip.refreshRenderer() }),
			),
		)
		ip.propertiesContainer.Add(box)
	}

	ip.propertiesContainer.Refresh()
}

func (ip *InspectPanel) updateStylesTab() {
	ip.stylesContainer.Objects = nil
	node := ip.selectedNode
	if node.ComputedStyle == nil {
		ip.stylesContainer.Add(widget.NewLabel("No computed styles"))
		ip.stylesContainer.Refresh()
		return
	}

	ip.stylesContainer.Add(widget.NewLabelWithStyle("Inline Styles", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	ip.stylesContainer.Add(ip.inlineStyleEditor.CanvasObject())

	ip.stylesContainer.Add(widget.NewLabelWithStyle("Matched CSS Rules", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	if ip.htmlRenderer != nil {
		rules := ip.htmlRenderer.GetMatchedRules(node)
		// Apply the property-name filter. An empty filter passes
		// every rule through; a non-empty filter requires at
		// least one declaration whose property contains the
		// filter text (case-insensitive substring match).
		filteredRules := rules[:0:0]
		if ip.stylesFilterText == "" {
			filteredRules = append(filteredRules, rules...)
		} else {
			for _, rule := range rules {
				for _, decl := range rule.Declarations {
					if strings.Contains(strings.ToLower(decl.Property), ip.stylesFilterText) {
						filteredRules = append(filteredRules, rule)
						break
					}
				}
			}
		}

		if len(filteredRules) == 0 {
			if ip.stylesFilterText != "" {
				ip.stylesContainer.Add(widget.NewLabel("No rules match the filter."))
			} else {
				ip.stylesContainer.Add(widget.NewLabel("No matching CSS rules"))
			}
		} else {
			for _, rule := range filteredRules {
				var b strings.Builder
				var sels []string
				for _, seq := range rule.Selectors {
					sels = append(sels, selectorToString(seq))
				}
				b.WriteString(strings.Join(sels, ", "))
				specStr := fmt.Sprintf(" /* specificity: [%d,%d,%d] */", rule.Specificity[0], rule.Specificity[1], rule.Specificity[2])
				b.WriteString(specStr + " {\n")
				for _, decl := range rule.Declarations {
					important := ""
					if decl.Important {
						important = " !important"
					}
					b.WriteString(fmt.Sprintf("    %s: %s%s;\n", decl.Property, decl.Value, important))
				}
				b.WriteString("}")

				ruleLabel := widget.NewLabel(b.String())
				ruleLabel.TextStyle.Monospace = true
				ruleLabel.Wrapping = fyne.TextWrapWord

				ip.stylesContainer.Add(container.NewPadded(ruleLabel))
			}
		}
	}

	ip.stylesContainer.Refresh()

	// Update the inline style editor and computed style viewer
	ip.inlineStyleEditor.SetNode(node)
	ip.computedStyleView.SetNode(node)
}

func (ip *InspectPanel) updateLayoutTab() {
	ip.layoutContainer.Objects = nil
	if ip.selectedLayout == nil {
		ip.layoutContainer.Add(widget.NewLabel("No layout information available"))
		ip.layoutContainer.Refresh()
		return
	}

	box := ip.selectedLayout

	ip.layoutContainer.Add(widget.NewLabelWithStyle("Box Model", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))

	grid := container.NewGridWithColumns(2,
		widget.NewLabel("X:"), widget.NewLabel(fmt.Sprintf("%.2f", box.Box.X)),
		widget.NewLabel("Y:"), widget.NewLabel(fmt.Sprintf("%.2f", box.Box.Y)),
		widget.NewLabel("Width:"), widget.NewLabel(fmt.Sprintf("%.2f", box.Box.Width)),
		widget.NewLabel("Height:"), widget.NewLabel(fmt.Sprintf("%.2f", box.Box.Height)),
	)

	ip.layoutContainer.Add(grid)

	ip.layoutContainer.Add(widget.NewLabelWithStyle("Margins", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	ip.layoutContainer.Add(widget.NewLabel(fmt.Sprintf("Top: %.1f, Right: %.1f, Bottom: %.1f, Left: %.1f",
		box.MarginTop, box.MarginRight, box.MarginBottom, box.MarginLeft)))

	ip.layoutContainer.Add(widget.NewLabelWithStyle("Padding", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	ip.layoutContainer.Add(widget.NewLabel(fmt.Sprintf("Top: %.1f, Right: %.1f, Bottom: %.1f, Left: %.1f",
		box.PaddingTop, box.PaddingRight, box.PaddingBottom, box.PaddingLeft)))

	scrollBtn := widget.NewButton("Scroll to Node", func() {
		if ip.onScrollToNode != nil {
			ip.onScrollToNode(box.Box.X, box.Box.Y)
		}
	})
	scrollBtn.Importance = widget.MediumImportance
	ip.layoutContainer.Add(scrollBtn)

	ip.layoutContainer.Refresh()
}

func (ip *InspectPanel) updatePerformanceTab() {
	ip.performanceContainer.Objects = nil

	nodeCount := len(ip.nodeMap)
	elemCount := 0
	textCount := 0
	if ip.htmlRenderer != nil {
		if t, e, tx := ip.htmlRenderer.GetDOMNodeCounts(); t > 0 {
			nodeCount = t
			elemCount = e
			textCount = tx
		} else {
			// Fallback: count from cached nodeMap
			for _, n := range ip.nodeMap {
				switch n.Type {
				case renderer.NodeTypeElement:
					elemCount++
				case renderer.NodeTypeText:
					textCount++
				}
			}
		}
	} else {
		for _, n := range ip.nodeMap {
			switch n.Type {
			case renderer.NodeTypeElement:
				elemCount++
			case renderer.NodeTypeText:
				textCount++
			}
		}
	}

	layoutCount := 0
	if ip.htmlRenderer != nil {
		layoutCount = ip.htmlRenderer.GetLayoutNodeCount()
	}

	ip.performanceContainer.Add(widget.NewLabelWithStyle("Metrics", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))

	if ip.hasPhaseTimings() {
		ip.renderTimingPanel(metrics.NewTimingPanel(ip.lastMetrics))
	} else {
		ip.performanceContainer.Add(widget.NewLabel(fmt.Sprintf("Total Nodes: %d", nodeCount)))
		ip.performanceContainer.Add(widget.NewLabel("No navigation timings yet"))
	}

	ip.performanceContainer.Add(widget.NewLabelWithStyle("Node Counts", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	ip.performanceContainer.Add(widget.NewLabel(fmt.Sprintf("Elements: %d  Text: %d  Other: %d",
		elemCount, textCount, nodeCount-elemCount-textCount)))
	ip.performanceContainer.Add(widget.NewLabel(fmt.Sprintf("Layout Boxes: %d", layoutCount)))

	ip.performanceContainer.Refresh()
}

// hasPhaseTimings reports whether the inspect panel has recorded any
// phase timings via SetMetrics. The Performance tab renders the
// phase timing panel only when this is true; otherwise it falls back
// to a static summary.
func (ip *InspectPanel) hasPhaseTimings() bool {
	return len(ip.lastMetrics.Timings) > 0 || ip.lastMetrics.NavID != 0
}

// SetMetrics updates the Performance tab with the given navigation
// metrics. A zero-value Metrics clears the timing surface back to
// the static summary. Safe to call from any goroutine — Fyne widgets
// are touched only on the UI goroutine via a fyne.Do scheduling
// pattern that the inspect panel relies on.
//
// Integration note: callers should invoke SetMetrics from the UI
// goroutine; the simplest production wire-up is a goroutine-safe
// helper on the engine session that reads the latest Recorder
// snapshot and posts it through the Fyne main loop. Tests can call
// SetMetrics directly.
func (ip *InspectPanel) SetMetrics(m metrics.Metrics) {
	ip.lastMetrics = m
	ip.updateDetails()
}

// renderTimingPanel converts a metrics.TimingPanel snapshot into a
// vertical stack of Fyne labels, mirroring the textual layout
// produced by TimingPanel.String. It keeps the panel independent of
// any Fyne canvas primitives that would force tab-size measurement
// regressions — every label is a static-format widget.
func (ip *InspectPanel) renderTimingPanel(panel metrics.TimingPanel) {
	header := widget.NewLabelWithStyle(
		fmt.Sprintf("Performance — Navigation %d", panel.NavID),
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true},
	)
	ip.performanceContainer.Add(header)
	ip.performanceContainer.Add(widget.NewLabel(panel.URL))

	totalRow := widget.NewLabel(fmt.Sprintf("Total: %.2f ms  Status: %s",
		panel.TotalDuration.Seconds()*1000, panel.OverallStatus))
	totalRow.TextStyle = fyne.TextStyle{Bold: true}
	ip.performanceContainer.Add(totalRow)

	summary := widget.NewLabel(fmt.Sprintf("Phases: %d ok, %d warning, %d slow",
		panel.StatusSummary.OK, panel.StatusSummary.Warning, panel.StatusSummary.Slow))
	ip.performanceContainer.Add(summary)

	for _, row := range panel.Rows {
		line := fmt.Sprintf("  %-12s %8.2f ms (%5.1f%%)%s  %s",
			row.PhaseLabel,
			row.DurationMs,
			row.Percentage,
			rowIntervalLabel(row.IntervalCount),
			row.Status,
		)
		ip.performanceContainer.Add(widget.NewLabel(line))
	}

	for _, g := range panel.CounterGroups {
		if len(g.Entries) == 0 {
			continue
		}
		ip.performanceContainer.Add(widget.NewLabelWithStyle(g.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		ip.performanceContainer.Add(widget.NewLabel(formatCounterGroup(g)))
	}
}

func rowIntervalLabel(n int) string {
	if n <= 1 {
		return ""
	}
	return fmt.Sprintf(" [%dx]", n)
}

// formatCounterGroup renders a CounterGroup's entries as a single
// comma-separated label. This is a UI-layer helper to keep the
// engine package free of Fyne imports.
func formatCounterGroup(g metrics.CounterGroup) string {
	parts := make([]string, 0, len(g.Entries))
	for _, e := range g.Entries {
		parts = append(parts, fmt.Sprintf("%s=%s", e.Name, formatCounterValue(e)))
	}
	return strings.Join(parts, ", ")
}

func formatCounterValue(e metrics.CounterEntry) string {
	if !e.Bytes {
		return fmt.Sprintf("%d", e.Value)
	}
	return formatBytes(e.Value)
}

// formatBytes mirrors metrics.formatBytes so the Fyne layer does not
// need to import any private helper. Kept tiny and readonly; the
// authoritative byte-formatter lives in the engine package.
func formatBytes(b int64) string {
	const step = 1000
	if b <= 0 {
		return "0 B"
	}
	abs := float64(b)
	units := []string{"B", "KB", "MB", "GB"}
	idx := 0
	for abs >= step && idx < len(units)-1 {
		abs /= step
		idx++
	}
	return fmt.Sprintf("%.2f %s", abs, units[idx])
}

func (ip *InspectPanel) refreshRenderer() {
	if ip.htmlRenderer != nil {
		ip.htmlRenderer.Refresh()
	}
}

func (ip *InspectPanel) rebuildNodeMap() {
	ip.nodeMap = make(map[string]*renderer.RenderNode)
	if ip.rootNode != nil {
		ip.traverseAndMap(ip.rootNode)
	}
}

func (ip *InspectPanel) traverseAndMap(node *renderer.RenderNode) {
	ip.nodeMap[fmt.Sprintf("%d", node.ID)] = node
	for _, child := range node.Children {
		ip.traverseAndMap(child)
	}
}

// GetContainer returns the inspect panel's container
func (ip *InspectPanel) GetContainer() *fyne.Container {
	return ip.container
}

type inspectTheme struct {
	fyne.Theme
}

func (t *inspectTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameText {
		return 11 // Smaller font size for standard text
	}
	if name == theme.SizeNamePadding {
		return 2 // Smaller padding/margins
	}
	if name == theme.SizeNameInlineIcon {
		return 10 // Smaller icons/arrows
	}
	return t.Theme.Size(name)
}

// CanvasObject returns the underlying canvas object for the panel
func (ip *InspectPanel) CanvasObject() fyne.CanvasObject {
	return container.NewThemeOverride(ip.container, &inspectTheme{Theme: fyne.CurrentApp().Settings().Theme()})
}

// formatNodeLabel produces a compact single-line label for a DOM tree row.
// It is used by updateDOMTreeNode so that only the .Text field of the
// template canvas.Text object needs to change — no widget replacement occurs.
func formatNodeLabel(node *renderer.RenderNode) (text string, col color.Color) {
	if node.Type == renderer.NodeTypeElement {
		var sb strings.Builder
		sb.WriteByte('<')
		sb.WriteString(node.TagName)

		// id attribute first
		if idVal, ok := node.GetAttribute("id"); ok {
			sb.WriteString(` id="`)
			sb.WriteString(idVal)
			sb.WriteByte('"')
		}
		// class attribute second
		if classVal, ok := node.GetAttribute("class"); ok {
			sb.WriteString(` class="`)
			sb.WriteString(classVal)
			sb.WriteByte('"')
		}
		// up to 2 other attributes
		count := 0
		for k, v := range node.Attrs {
			if k == "id" || k == "class" {
				continue
			}
			if count >= 2 {
				sb.WriteString(" ...")
				break
			}
			sb.WriteByte(' ')
			sb.WriteString(k)
			sb.WriteString(`="`)
			sb.WriteString(v)
			sb.WriteByte('"')
			count++
		}
		sb.WriteByte('>')
		return sb.String(), color.RGBA{R: 86, G: 156, B: 214, A: 255}
	}
	// Text node
	textVal := strings.TrimSpace(node.Text)
	if len(textVal) > 40 {
		textVal = textVal[:37] + "..."
	}
	return fmt.Sprintf("%q", textVal), color.RGBA{R: 181, G: 206, B: 168, A: 255}
}

func updateDOMTreeNode(node *renderer.RenderNode, o fyne.CanvasObject, selected bool) {
	w, ok := o.(*domTreeNodeWidget)
	if !ok {
		return
	}
	label, col := formatNodeLabel(node)
	if selected {
		col = theme.ForegroundColor()
	}
	w.Update(label, col)
}

type domTreeNodeWidget struct {
	widget.BaseWidget
	text *canvas.Text
}

func newDOMTreeNodeWidget(val string, col color.Color) *domTreeNodeWidget {
	w := &domTreeNodeWidget{
		text: monospaceCanvasText(val, col),
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *domTreeNodeWidget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(w.text)
}

func (w *domTreeNodeWidget) Update(val string, col color.Color) {
	if w.text.Text == val && w.text.Color == col {
		return
	}
	w.text.Text = val
	w.text.Color = col
	w.text.Refresh()
	w.Refresh()
}

func monospaceCanvasText(text string, col color.Color) *canvas.Text {
	t := canvas.NewText(text, col)
	t.TextStyle.Monospace = true
	t.TextSize = 10
	return t
}

func selectorToString(seq css.SelectorSequence) string {
	var parts []string
	current := &seq
	for current != nil {
		part := simpleSelectorToString(current.Simple)
		if current.Combinator != "" {
			part += " " + current.Combinator + " "
		}
		parts = append(parts, part)
		current = current.Next
	}
	return strings.Join(parts, "")
}

func simpleSelectorToString(simple css.SimpleSelector) string {
	if simple.Universal {
		return "*"
	}
	var res strings.Builder
	res.WriteString(simple.TagName)
	if simple.ID != "" {
		res.WriteString("#" + simple.ID)
	}
	for _, class := range simple.Classes {
		res.WriteString("." + class)
	}
	for _, pseudo := range simple.PseudoClasses {
		res.WriteString(":" + pseudo)
	}
	for _, pseudoElem := range simple.PseudoElements {
		res.WriteString("::" + pseudoElem)
	}
	for _, attr := range simple.Attributes {
		res.WriteString("[" + attr.Name)
		if attr.Operator != "" {
			res.WriteString(attr.Operator + "\"" + attr.Value + "\"")
		}
		res.WriteString("]")
	}
	return res.String()
}

// Helper for binding
func bindingString(val string, onChange func(string)) bindingStringImpl {
	return bindingStringImpl{val: val, onChange: onChange}
}

type bindingStringImpl struct {
	val      string
	onChange func(string)
}

func (b bindingStringImpl) Get() (string, error) { return b.val, nil }
func (b bindingStringImpl) Set(s string) error {
	b.val = s
	b.onChange(s)
	return nil
}
func (b bindingStringImpl) AddListener(l binding.DataListener)    {}
func (b bindingStringImpl) RemoveListener(l binding.DataListener) {}
