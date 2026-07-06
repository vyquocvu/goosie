package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/renderer"
)

// InspectPanel represents the comprehensive element inspector panel
type InspectPanel struct {
	container      *fyne.Container
	tree           *widget.Tree
	detailsTabs    *container.AppTabs
	closeButton    *widget.Button
	onClose        func()
	
	selectedNode   *renderer.RenderNode
	selectedLayout *renderer.LayoutBox
	
	htmlRenderer   HTMLRenderer
	rootNode       *renderer.RenderNode
	nodeMap        map[string]*renderer.RenderNode // Map UID (string ID) to RenderNode
	
	// Details View Components
	propertiesContainer  *fyne.Container
	stylesContainer      *fyne.Container
	layoutContainer      *fyne.Container
	performanceContainer *fyne.Container
	
	// Search
	searchEntry *widget.Entry
	
	// State to prevent recursive updates
	updatingUI bool
}

// NewInspectPanel creates a new inspect panel
func NewInspectPanel(onClose func()) *InspectPanel {
	panel := &InspectPanel{
		onClose: onClose,
		nodeMap: make(map[string]*renderer.RenderNode),
	}

	// Create close button
	panel.closeButton = widget.NewButton("✕", func() {
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
			node, ok := panel.nodeMap[id]
			if !ok {
				return false
			}
			return len(node.Children) > 0
		},
		func(branch bool) fyne.CanvasObject {
			return widget.NewLabel("Node")
		},
		func(id widget.TreeNodeID, branch bool, o fyne.CanvasObject) {
			node, ok := panel.nodeMap[id]
			if !ok {
				return
			}
			label := o.(*widget.Label)
			if node.Type == renderer.NodeTypeElement {
				txt := "<" + node.TagName
				if idAttr, ok := node.GetAttribute("id"); ok {
					txt += " #" + idAttr
				}
				if classAttr, ok := node.GetAttribute("class"); ok {
					// Truncate class if too long
					if len(classAttr) > 15 {
						classAttr = classAttr[:12] + "..."
					}
					txt += " ." + strings.ReplaceAll(classAttr, " ", ".")
				}
				txt += ">"
				label.SetText(txt)
				label.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				text := strings.TrimSpace(node.Text)
				if len(text) > 20 {
					text = text[:17] + "..."
				}
				label.SetText("Text: " + text)
				label.TextStyle = fyne.TextStyle{Italic: true}
			}
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
	// Left: Tree with search, Right: Details
	leftSide := container.NewBorder(searchContainer, nil, nil, nil, panel.tree)
	
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

	panel.container = container.NewBorder(
		topBar, nil, nil, nil,
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
			ip.tree.Select(id)
			// Note: Fyne tree scrolling to item is not easily exposed yet, 
			// but selection will update the details view
			return // Stop after first match for now
		}
	}
}

func (ip *InspectPanel) createDetailsView() {
	// Properties Tab
	ip.propertiesContainer = container.NewVBox()
	
	// Styles Tab
	ip.stylesContainer = container.NewVBox()
	
	// Layout Tab
	ip.layoutContainer = container.NewVBox()
	
	// Performance Tab
	ip.performanceContainer = container.NewVBox()
	
	ip.detailsTabs = container.NewAppTabs(
		container.NewTabItem("Properties", container.NewVScroll(ip.propertiesContainer)),
		container.NewTabItem("Styles", container.NewVScroll(ip.stylesContainer)),
		container.NewTabItem("Layout", container.NewVScroll(ip.layoutContainer)),
		container.NewTabItem("Performance", container.NewVScroll(ip.performanceContainer)),
	)
}

// SetRenderer sets the HTML renderer and refreshes the tree
func (ip *InspectPanel) SetRenderer(r HTMLRenderer) {
	ip.htmlRenderer = r
	if r != nil {
		ip.rootNode = r.GetRoot()
		ip.rebuildNodeMap()
		ip.tree.Refresh()
		// Also update details if selection exists but might be stale
		if ip.selectedNode != nil {
			// Check if node still exists in new map
			if _, ok := ip.nodeMap[fmt.Sprintf("%d", ip.selectedNode.ID)]; ok {
				ip.updateDetails()
			} else {
				ip.selectedNode = nil
				ip.selectedLayout = nil
				ip.updateDetails()
			}
		}
	}
}

// SetElement sets the element to inspect (called from renderer hit test)
func (ip *InspectPanel) SetElement(node *renderer.RenderNode, layout *renderer.LayoutBox) {
	// Skip redundant updates when hovering the same element
	if ip.selectedNode != nil && node != nil && ip.selectedNode.ID == node.ID {
		return
	}

	// Update tree selection
	if node != nil {
		id := fmt.Sprintf("%d", node.ID)
		ip.tree.Select(id)
	}
	
	ip.selectedNode = node
	ip.selectedLayout = layout
	ip.updateDetails()
}

func (ip *InspectPanel) selectNode(node *renderer.RenderNode) {
	ip.selectedNode = node
	// Note: selectedLayout might be stale if we just clicked tree node
	// In a real implementation we'd ask renderer for layout box of this node
	ip.updateDetails()
}

func (ip *InspectPanel) updateDetails() {
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
	
	style := node.ComputedStyle
	
	// Helper to add style row
	addStyleRow := func(label string, value string, onUpdate func(string)) {
		entry := widget.NewEntry()
		entry.SetText(value)
		entry.OnSubmitted = func(s string) {
			onUpdate(s)
			ip.refreshRenderer()
		}
		
		row := container.NewBorder(nil, nil, widget.NewLabel(label), nil, entry)
		ip.stylesContainer.Add(row)
	}
	
	ip.stylesContainer.Add(widget.NewLabelWithStyle("Computed Styles", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	
	addStyleRow("Display", style.Display, func(s string) { style.Display = s })
	addStyleRow("Font Size", fmt.Sprintf("%.1fpx", style.FontSize), func(s string) { 
		if f, err := strconv.ParseFloat(strings.TrimSuffix(s, "px"), 32); err == nil {
			style.FontSize = float32(f)
		}
	})
	addStyleRow("Width", style.Width, func(s string) { style.Width = s })
	addStyleRow("Height", style.Height, func(s string) { style.Height = s })
	addStyleRow("Color", fmt.Sprintf("%v", style.Color), func(s string) {
		// Read-only for now
	})
	
	ip.stylesContainer.Refresh()
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
	
	ip.layoutContainer.Refresh()
}

func (ip *InspectPanel) updatePerformanceTab() {
	ip.performanceContainer.Objects = nil
	
	nodeCount := len(ip.nodeMap)
	
	ip.performanceContainer.Add(widget.NewLabelWithStyle("Metrics", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	ip.performanceContainer.Add(widget.NewLabel(fmt.Sprintf("Total Nodes: %d", nodeCount)))
	
	ip.performanceContainer.Refresh()
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

// CanvasObject returns the underlying canvas object for the panel
func (ip *InspectPanel) CanvasObject() fyne.CanvasObject {
	return ip.container
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
func (b bindingStringImpl) AddListener(l binding.DataListener) {}
func (b bindingStringImpl) RemoveListener(l binding.DataListener) {}
