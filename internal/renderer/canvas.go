package renderer

import (
	"image/color"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	imageloader "github.com/vyquocvu/goosie/internal/image"
)

// NavigationCallback is a function that is called when navigation is requested
type NavigationCallback func(url string)

// CanvasRenderer renders a render tree onto a Fyne canvas
type CanvasRenderer struct {
	canvasWidth  float32
	canvasHeight float32
	defaultSize  float32
	window       fyne.Window

	// Viewport for optimized rendering
	viewportY      float32
	viewportHeight float32

	// Cached display list for performance
	cachedDisplayList *DisplayList
	cachedLayoutRoot  *LayoutBox
	cachedRenderRoot  *RenderNode

	// fontMetrics provides accurate text measurement
	fontMetrics *FontMetrics

	// Navigation callback for link clicks
	onNavigate NavigationCallback

	// Current page URL for resolving relative links
	baseURL string

	// Image loader for loading and caching images
	imageLoader imageloader.Loader

	// OnRefresh is a test hook to signal when a refresh is triggered.
	OnRefresh func()

	// Inspect callback for element inspection
	onInspect func(node *RenderNode, layout *LayoutBox)
	renderer  *Renderer // Reference to main renderer for hit testing
}

// NewCanvasRenderer creates a new canvas renderer
func NewCanvasRenderer(width, height float32) *CanvasRenderer {
	defaultSize := float32(16.0)
	return &CanvasRenderer{
		canvasWidth:    width,
		canvasHeight:   height,
		defaultSize:    defaultSize,
		viewportY:      0,
		viewportHeight: height,
		fontMetrics:    NewFontMetrics(defaultSize),
	}
}

// SetWindow sets the Fyne window for the renderer
func (cr *CanvasRenderer) SetWindow(w fyne.Window) {
	cr.window = w
	if cr.imageLoader != nil {
		cr.imageLoader.SetOnLoadCallback(cr.onImageLoaded)
	}
}

func (cr *CanvasRenderer) onImageLoaded(source string) {
	if cr.window == nil {
		return
	}

	// Use fyne.Do to safely update the UI from any thread
	fyne.Do(func() {
		cr.ClearCache()
		cr.window.Canvas().Refresh(cr.window.Content())

		if cr.OnRefresh != nil {
			cr.OnRefresh()
		}
	})
}

// SetViewport sets the current viewport for optimized rendering
func (cr *CanvasRenderer) SetViewport(y, height float32) {
	cr.viewportY = y
	cr.viewportHeight = height
}

// SetNavigationCallback sets the navigation callback for link clicks
func (cr *CanvasRenderer) SetNavigationCallback(callback NavigationCallback, baseURL string) {
	cr.onNavigate = callback
	cr.baseURL = baseURL
}

// SetInspectCallback sets the inspect callback for element inspection
func (cr *CanvasRenderer) SetInspectCallback(callback func(node *RenderNode, layout *LayoutBox), renderer *Renderer) {
	cr.onInspect = callback
	cr.renderer = renderer
}

// isInViewport checks if a box intersects with the current viewport
func (cr *CanvasRenderer) isInViewport(box Rect) bool {
	// Add buffer zone above and below viewport for smoother scrolling
	bufferZone := cr.viewportHeight * 0.5
	viewportTop := cr.viewportY - bufferZone
	viewportBottom := cr.viewportY + cr.viewportHeight + bufferZone

	boxBottom := box.Y + box.Height

	// Check if box intersects with viewport
	return boxBottom >= viewportTop && box.Y <= viewportBottom
}

// Render renders the render tree and returns a Fyne container
func (cr *CanvasRenderer) Render(root *RenderNode) fyne.CanvasObject {
	if root == nil {
		return container.NewVBox()
	}

	objects := make([]fyne.CanvasObject, 0)
	cr.renderNode(root, &objects)

	return container.NewVBox(objects...)
}

// renderNode renders a single node and its children
func (cr *CanvasRenderer) renderNode(node *RenderNode, objects *[]fyne.CanvasObject) {
	if node == nil {
		return
	}

	if node.Type == NodeTypeText {
		cr.renderTextNode(node, objects)
	} else if node.Type == NodeTypeElement {
		cr.renderElementNode(node, objects)
	}
}

// renderTextNode renders a text node
func (cr *CanvasRenderer) renderTextNode(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := strings.TrimSpace(node.Text)
	if text == "" {
		return
	}

	// Create text widget
	textWidget := widget.NewLabel(text)
	textWidget.Wrapping = fyne.TextWrapWord

	// Get text style from parent if available
	if node.Parent != nil {
		textWidget.TextStyle = cr.fontMetrics.GetTextStyle(node.Parent.TagName)
	}

	*objects = append(*objects, textWidget)
}

// renderElementNode renders an element node
func (cr *CanvasRenderer) renderElementNode(node *RenderNode, objects *[]fyne.CanvasObject) {
	switch node.TagName {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		cr.renderHeading(node, objects)
	case "p":
		cr.renderParagraph(node, objects)
	case "div":
		cr.renderDiv(node, objects)
	case "a":
		cr.renderLink(node, objects)
	case "ul", "ol":
		cr.renderList(node, objects)
	case "li":
		cr.renderListItem(node, objects)
	case "img":
		cr.renderImage(node, objects)
	case "input":
		cr.renderInput(node, objects)
	case "button":
		cr.renderButton(node, objects)
	case "textarea":
		cr.renderTextarea(node, objects)
	case "table":
		cr.renderTable(node, objects)
	case "tbody", "thead", "tfoot", "tr", "td", "th":
		// These elements are handled by the renderTable function.
		// If they were rendered here, it would cause the cell contents to be
		// duplicated and rendered as separate text blocks.
		// By having these cases do nothing, we ensure that only the renderTable
		// function handles the rendering of the table and its contents.
	case "br":
		// Add a spacer for line break
		*objects = append(*objects, widget.NewLabel(""))
	case "code":
		cr.renderCode(node, objects)
	case "pre":
		cr.renderPre(node, objects)
	case "blockquote":
		cr.renderBlockquote(node, objects)
	case "span", "strong", "em", "b", "i":
		// Inline elements - render children
		for _, child := range node.Children {
			cr.renderNode(child, objects)
		}
	default:
		// Generic element - just render children
		for _, child := range node.Children {
			cr.renderNode(child, objects)
		}
	}
}

// renderHeading renders heading elements (h1-h6)
func (cr *CanvasRenderer) renderHeading(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	if text == "" {
		return
	}

	// Apply CSS styles if present
	styledObj := cr.applyStylesToLabel(node, text)

	// If it's a standard label (no CSS), apply heading styles
	if label, ok := styledObj.(*widget.Label); ok {
		label.TextStyle = fyne.TextStyle{Bold: true}
	}

	*objects = append(*objects, styledObj)
}

// renderParagraph renders paragraph elements
func (cr *CanvasRenderer) renderParagraph(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	if text == "" {
		return
	}

	// Apply CSS styles if present
	styledObj := cr.applyStylesToLabel(node, text)
	*objects = append(*objects, styledObj)
}

// renderDiv renders div elements
func (cr *CanvasRenderer) renderDiv(node *RenderNode, objects *[]fyne.CanvasObject) {
	// Render children
	for _, child := range node.Children {
		cr.renderNode(child, objects)
	}
}

// renderLink renders anchor (link) elements
func (cr *CanvasRenderer) renderLink(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	href, hasHref := node.GetAttribute("href")

	if text == "" {
		return
	}

	if hasHref && href != "" {
		// Resolve URL (absolute or relative)
		resolvedURL := cr.resolveURL(href)

		// Note: Link target attribute (_blank, _self, etc.) is available via node.GetAttribute("target")
		// but not currently implemented as the browser doesn't support tabs yet.
		// This is planned for Phase 1 UI Improvements (see ROADMAP.md).

		// Parse URL to create a proper Fyne URL object
		parsedURL, err := url.Parse(resolvedURL)
		if err != nil {
			// If URL parsing fails, display as text
			label := widget.NewLabel(text)
			label.Wrapping = fyne.TextWrapWord
			*objects = append(*objects, label)
			return
		}

		// Create a clickable hyperlink widget
		link := widget.NewHyperlink(text, parsedURL)
		link.Wrapping = fyne.TextWrapWord

		// Override the default tap handler to use our navigation callback
		if cr.onNavigate != nil {
			// Create a custom tappable widget
			tappableLink := newTappableHyperlink(text, resolvedURL, cr.onNavigate)
			*objects = append(*objects, tappableLink)
		} else {
			// Fallback to default hyperlink behavior
			*objects = append(*objects, link)
		}
	} else {
		// No href, just display as text
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapWord
		*objects = append(*objects, label)
	}
}

// resolveURL resolves a relative or absolute URL against the base URL
func (cr *CanvasRenderer) resolveURL(href string) string {
	// If href is already absolute, return as-is
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}

	// If no base URL, return href as-is
	if cr.baseURL == "" {
		return href
	}

	// Parse base URL
	baseURL, err := url.Parse(cr.baseURL)
	if err != nil {
		return href
	}

	// Parse relative href
	relURL, err := url.Parse(href)
	if err != nil {
		return href
	}

	// Resolve relative URL against base
	resolved := baseURL.ResolveReference(relURL)
	return resolved.String()
}

// TappableHyperlink is a custom hyperlink widget that can trigger navigation callbacks.
// It extends widget.Hyperlink, inheriting keyboard navigation support (Tab focus, Enter activation).
type TappableHyperlink struct {
	widget.Hyperlink
	url        string
	onNavigate NavigationCallback
}

// newTappableHyperlink creates a new tappable hyperlink
func newTappableHyperlink(text, urlStr string, onNavigate NavigationCallback) *TappableHyperlink {
	parsedURL := urlParse(urlStr)
	link := &TappableHyperlink{
		url:        urlStr,
		onNavigate: onNavigate,
	}
	link.ExtendBaseWidget(link)
	link.Text = text
	link.URL = parsedURL
	link.Wrapping = fyne.TextWrapWord
	return link
}

// Tapped handles tap events on the hyperlink
func (t *TappableHyperlink) Tapped(_ *fyne.PointEvent) {
	if t.onNavigate != nil {
		t.onNavigate(t.url)
	}
}

// urlParse is a helper that returns nil on parse error
func urlParse(urlStr string) *url.URL {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}
	return parsed
}

// InspectableContainer is a container that handles mouse events for element inspection
type InspectableContainer struct {
	widget.BaseWidget
	container      *fyne.Container
	canvasRenderer *CanvasRenderer
}

// newInspectableContainer creates a new inspectable container
func newInspectableContainer(content fyne.CanvasObject, cr *CanvasRenderer) *InspectableContainer {
	ic := &InspectableContainer{
		canvasRenderer: cr,
	}
	ic.container = container.NewWithoutLayout(content)
	ic.ExtendBaseWidget(ic)
	return ic
}

// CreateRenderer creates a renderer for the inspectable container
func (ic *InspectableContainer) CreateRenderer() fyne.WidgetRenderer {
	return &inspectableContainerRenderer{
		container: ic.container,
		objects:   []fyne.CanvasObject{ic.container},
	}
}

// MouseIn handles mouse enter events
func (ic *InspectableContainer) MouseIn(*fyne.PointEvent) {
	// Mouse enter - could show hover state
}

// MouseOut handles mouse leave events
func (ic *InspectableContainer) MouseOut() {
	// Mouse leave - could clear hover state
}

// MouseMoved handles mouse movement for hover inspection
func (ic *InspectableContainer) MouseMoved(event *fyne.PointEvent) {
	if ic.canvasRenderer.onInspect == nil || ic.canvasRenderer.renderer == nil {
		return
	}

	// Get the scroll offset if we're in a scroll container
	// For now, we'll use the viewport Y as the scroll offset
	scrollY := ic.canvasRenderer.viewportY

	// Convert mouse position to content coordinates
	contentX := event.Position.X
	contentY := event.Position.Y + scrollY

	// Perform hit test
	node, layout := ic.canvasRenderer.renderer.HitTest(contentX, contentY)
	if node != nil && layout != nil {
		// Call inspect callback on hover
		if ic.canvasRenderer.onInspect != nil {
			ic.canvasRenderer.onInspect(node, layout)
		}
	}
}

// MouseDown handles mouse click events for element selection
func (ic *InspectableContainer) MouseDown(event *fyne.PointEvent) {
	if ic.canvasRenderer.onInspect == nil || ic.canvasRenderer.renderer == nil {
		return
	}

	// Get the scroll offset
	scrollY := ic.canvasRenderer.viewportY

	// Convert mouse position to content coordinates
	contentX := event.Position.X
	contentY := event.Position.Y + scrollY

	// Perform hit test
	node, layout := ic.canvasRenderer.renderer.HitTest(contentX, contentY)
	if node != nil && layout != nil {
		// Call inspect callback on click (to select element)
		if ic.canvasRenderer.onInspect != nil {
			ic.canvasRenderer.onInspect(node, layout)
		}
	}
}

// inspectableContainerRenderer is the renderer for the inspectable container
type inspectableContainerRenderer struct {
	container *fyne.Container
	objects   []fyne.CanvasObject
}

func (r *inspectableContainerRenderer) Layout(size fyne.Size) {
	r.container.Resize(size)
}

func (r *inspectableContainerRenderer) MinSize() fyne.Size {
	return r.container.MinSize()
}

func (r *inspectableContainerRenderer) Refresh() {
	r.container.Refresh()
}

func (r *inspectableContainerRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *inspectableContainerRenderer) Destroy() {}

// renderList renders ul/ol elements
func (cr *CanvasRenderer) renderList(node *RenderNode, objects *[]fyne.CanvasObject) {
	// Render list items
	for _, child := range node.Children {
		if child.TagName == "li" {
			cr.renderListItem(child, objects)
		}
	}
}

// renderListItem renders li elements
func (cr *CanvasRenderer) renderListItem(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	if text == "" {
		return
	}

	// Add bullet point
	label := widget.NewLabel("• " + text)
	label.Wrapping = fyne.TextWrapWord

	*objects = append(*objects, label)
}

// renderCode renders code elements with monospace styling
func (cr *CanvasRenderer) renderCode(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	if text == "" {
		return
	}

	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	label.TextStyle = fyne.TextStyle{Monospace: true}

	*objects = append(*objects, label)
}

// renderPre renders pre elements with monospace styling and preserved whitespace
func (cr *CanvasRenderer) renderPre(node *RenderNode, objects *[]fyne.CanvasObject) {
	// For pre elements, we want to preserve whitespace and newlines
	// Extract text without trimming
	text := cr.extractTextPreserveWhitespace(node)
	if text == "" {
		return
	}

	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapOff // Pre elements typically don't wrap
	label.TextStyle = fyne.TextStyle{Monospace: true}

	*objects = append(*objects, label)
}

// renderBlockquote renders blockquote elements
func (cr *CanvasRenderer) renderBlockquote(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	if text == "" {
		return
	}

	// Add visual indication of quote (e.g., with prefix)
	label := widget.NewLabel("❝ " + text)
	label.Wrapping = fyne.TextWrapWord
	label.TextStyle = fyne.TextStyle{Italic: true}

	*objects = append(*objects, label)
}

// renderImage renders img elements
func (cr *CanvasRenderer) renderImage(node *RenderNode, objects *[]fyne.CanvasObject) {
	alt, hasAlt := node.GetAttribute("alt")
	src, hasSrc := node.GetAttribute("src")

	if !hasSrc || src == "" {
		// No source - show alt text or placeholder
		displayText := "[Image"
		if hasAlt {
			displayText += ": " + alt
		}
		displayText += "]"
		label := widget.NewLabel(displayText)
		label.Wrapping = fyne.TextWrapWord
		*objects = append(*objects, label)
		return
	}

	// Resolve relative URLs
	resolvedSrc := cr.resolveURL(src)

	// Try to load the image if loader is available
	if cr.imageLoader != nil {
		imageData, err := cr.imageLoader.Load(resolvedSrc)

		if err == nil && imageData != nil {
			switch imageData.State {
			case imageloader.StateLoaded:
				// Image loaded successfully - render it
				img := canvas.NewImageFromImage(imageData.Image)
				img.FillMode = canvas.ImageFillOriginal
				img.SetMinSize(fyne.NewSize(float32(imageData.Width), float32(imageData.Height)))

				// Add alt text below the image if available
				if hasAlt && alt != "" {
					altLabel := widget.NewLabel(alt)
					altLabel.Wrapping = fyne.TextWrapWord
					*objects = append(*objects, container.NewVBox(img, altLabel))
				} else {
					*objects = append(*objects, img)
				}
				return

			case imageloader.StateError:
				// Image failed to load - show error with alt text
				displayText := "[Image Load Failed"
				if hasAlt {
					displayText += ": " + alt
				}
				displayText += "]"
				label := widget.NewLabel(displayText)
				label.Wrapping = fyne.TextWrapWord
				*objects = append(*objects, label)
				return

			case imageloader.StateLoading:
				// Image is loading - show loading placeholder
				displayText := "[Loading Image"
				if hasAlt {
					displayText += ": " + alt
				}
				displayText += "]"
				label := widget.NewLabel(displayText)
				label.Wrapping = fyne.TextWrapWord

				// Show a gray rectangle as loading indicator
				rect := canvas.NewRectangle(theme.DisabledButtonColor())
				rect.SetMinSize(fyne.NewSize(100, 100))

				*objects = append(*objects, container.NewVBox(rect, label))
				return
			}
		}
	}

	// Fallback: Show placeholder if loader is not available or something went wrong
	displayText := "[Image: " + resolvedSrc
	if hasAlt {
		displayText += " - " + alt
	}
	displayText += "]"

	label := widget.NewLabel(displayText)
	label.Wrapping = fyne.TextWrapWord

	rect := canvas.NewRectangle(theme.DisabledButtonColor())
	rect.SetMinSize(fyne.NewSize(100, 100))
	*objects = append(*objects, container.NewVBox(rect, label))
}

// extractText extracts all text content from a node and its children
func (cr *CanvasRenderer) extractText(node *RenderNode) string {
	var text strings.Builder
	cr.extractTextRecursive(node, &text)
	return strings.TrimSpace(text.String())
}

// extractTextRecursive recursively extracts text from a node tree
func (cr *CanvasRenderer) extractTextRecursive(node *RenderNode, builder *strings.Builder) {
	if node == nil {
		return
	}

	if node.Type == NodeTypeText {
		text := strings.TrimSpace(node.Text)
		if text != "" {
			if builder.Len() > 0 {
				builder.WriteString(" ")
			}
			builder.WriteString(text)
		}
	}

	for _, child := range node.Children {
		cr.extractTextRecursive(child, builder)
	}
}

// extractTextPreserveWhitespace extracts text content while preserving whitespace and newlines
// This is used for <pre> elements where whitespace formatting is significant
func (cr *CanvasRenderer) extractTextPreserveWhitespace(node *RenderNode) string {
	var text strings.Builder
	cr.extractTextPreserveWhitespaceRecursive(node, &text)
	return text.String()
}

// extractTextPreserveWhitespaceRecursive recursively extracts text without trimming whitespace
func (cr *CanvasRenderer) extractTextPreserveWhitespaceRecursive(node *RenderNode, builder *strings.Builder) {
	if node == nil {
		return
	}

	if node.Type == NodeTypeText {
		// Don't trim whitespace for pre elements
		builder.WriteString(node.Text)
	}

	for _, child := range node.Children {
		cr.extractTextPreserveWhitespaceRecursive(child, builder)
	}
}

// getFontSize returns font size for an element type (delegates to fontMetrics)
func (cr *CanvasRenderer) getFontSize(tagName string) float32 {
	return cr.fontMetrics.GetFontSize(tagName)
}

// getTextStyle returns text style for an element type (delegates to fontMetrics)
func (cr *CanvasRenderer) getTextStyle(tagName string) fyne.TextStyle {
	return cr.fontMetrics.GetTextStyle(tagName)
}

// RenderWithViewport renders the render tree with viewport culling for better performance
func (cr *CanvasRenderer) RenderWithViewport(root *RenderNode, layoutRoot *LayoutBox) fyne.CanvasObject {
	if root == nil || layoutRoot == nil {
		return container.NewVBox()
	}

	// Build or reuse display list
	var displayList *DisplayList
	if cr.cachedDisplayList != nil && cr.cachedRenderRoot == root && cr.cachedLayoutRoot == layoutRoot {
		// Reuse cached display list
		displayList = cr.cachedDisplayList
	} else {
		// Build new display list
		dlb := NewDisplayListBuilder()
		displayList = dlb.Build(layoutRoot, root)

		// Cache for next time
		cr.cachedDisplayList = displayList
		cr.cachedRenderRoot = root
		cr.cachedLayoutRoot = layoutRoot
	}

	// Filter commands based on viewport
	objects := make([]fyne.CanvasObject, 0)
	for _, cmd := range displayList.Commands {
		if cr.isInViewport(cmd.Box) {
			cr.renderCommand(cmd, &objects)
		}
	}

	if len(objects) == 0 {
		return container.NewWithoutLayout()
	}

	content := container.NewWithoutLayout(objects...)

	// Wrap in a mouse-aware container if inspect callback is set
	if cr.onInspect != nil && cr.renderer != nil {
		return newInspectableContainer(content, cr)
	}

	return content
}

// renderCommand renders a single paint command to canvas objects
// renderCommand renders a single paint command to canvas objects
func (cr *CanvasRenderer) renderCommand(cmd *PaintCommand, objects *[]fyne.CanvasObject) {
	switch cmd.Type {
	case PaintText:
		if strings.TrimSpace(cmd.Text) == "" {
			return
		}

		// Check if the node has CSS styles that require custom rendering
		if cr.hasCustomStyles(cmd.Node) {
			// Create a canvas.Text object with CSS styles
			textObj := canvas.NewText(cmd.Text, theme.ForegroundColor())
			textObj.TextSize = cr.defaultSize

			style := cmd.Node.ComputedStyle

			if style.Color != nil {
				textObj.Color = style.Color
			}

			if style.FontSize > 0 {
				textObj.TextSize = style.FontSize
			}

			// Apply text style
			textStyle := fyne.TextStyle{}
			if style.FontWeight == "bold" || cmd.Bold {
				textStyle.Bold = true
			}
			if cmd.Italic {
				textStyle.Italic = true
			}
			textObj.TextStyle = textStyle

			cr.addObjectToDisplay(textObj, cmd, objects)
		} else {
			// Use standard label widget
			label := widget.NewLabel(cmd.Text)
			label.Wrapping = fyne.TextWrapWord

			if cmd.Bold && cmd.Italic {
				label.TextStyle = fyne.TextStyle{Bold: true, Italic: true}
			} else if cmd.Bold {
				label.TextStyle = fyne.TextStyle{Bold: true}
			} else if cmd.Italic {
				label.TextStyle = fyne.TextStyle{Italic: true}
			}

			cr.addObjectToDisplay(label, cmd, objects)
		}

	case PaintRect:
		rect := canvas.NewRectangle(cmd.FillColor)
		cr.addObjectToDisplay(rect, cmd, objects)

	case PaintImage:
		// Try to load and render the actual image if loader is available
		if cr.imageLoader != nil && cmd.Node.ImageData != nil {
			imageData := cmd.Node.ImageData

			if imageData != nil {
				switch imageData.State {
				case imageloader.StateLoaded:
					// Image loaded successfully - render it
					img := canvas.NewImageFromImage(imageData.Image)
					img.FillMode = canvas.ImageFillOriginal
					img.SetMinSize(fyne.NewSize(float32(imageData.Width), float32(imageData.Height)))

					// Add alt text below the image if available
					if cmd.ImageAlt != "" {
						altLabel := widget.NewLabel(cmd.ImageAlt)
						altLabel.Wrapping = fyne.TextWrapWord
						cr.addObjectToDisplay(container.NewVBox(img, altLabel), cmd, objects)
					} else {
						cr.addObjectToDisplay(img, cmd, objects)
					}
					return

				case imageloader.StateError:
					// Image failed to load - show error with alt text
					displayText := "[Image Load Failed"
					if cmd.ImageAlt != "" {
						displayText += ": " + cmd.ImageAlt
					}
					displayText += "]"
					label := widget.NewLabel(displayText)
					label.Wrapping = fyne.TextWrapWord
					cr.addObjectToDisplay(label, cmd, objects)
					return

				case imageloader.StateLoading:
					// Image is loading - show loading placeholder
					displayText := "[Loading Image"
					if cmd.ImageAlt != "" {
						displayText += ": " + cmd.ImageAlt
					}
					displayText += "]"
					label := widget.NewLabel(displayText)
					label.Wrapping = fyne.TextWrapWord

					rect := canvas.NewRectangle(theme.DisabledButtonColor())
					rect.SetMinSize(fyne.NewSize(100, 100))

					cr.addObjectToDisplay(container.NewVBox(rect, label), cmd, objects)
					return
				}
			}
		}

		// Fallback: Render image placeholder
		displayText := "[Image"
		if cmd.ImageSrc != "" {
			displayText += ": " + cmd.ImageSrc
		}
		if cmd.ImageAlt != "" {
			displayText += " - " + cmd.ImageAlt
		}
		displayText += "]"

		label := widget.NewLabel(displayText)
		label.Wrapping = fyne.TextWrapWord

		rect := canvas.NewRectangle(theme.DisabledButtonColor())
		rect.SetMinSize(fyne.NewSize(100, 100))

		cr.addObjectToDisplay(container.NewVBox(rect, label), cmd, objects)

	case PaintLink:
		// Render clickable link
		if cmd.LinkText == "" {
			return
		}

		// Resolve URL (absolute or relative)
		resolvedURL := cr.resolveURL(cmd.LinkURL)

		// Create a clickable hyperlink widget
		if cr.onNavigate != nil {
			// Create a custom tappable widget
			tappableLink := newTappableHyperlink(cmd.LinkText, resolvedURL, cr.onNavigate)
			cr.addObjectToDisplay(tappableLink, cmd, objects)
		} else {
			// Fallback to default hyperlink behavior
			parsedURL, err := url.Parse(resolvedURL)
			if err == nil {
				link := widget.NewHyperlink(cmd.LinkText, parsedURL)
				link.Wrapping = fyne.TextWrapWord
				cr.addObjectToDisplay(link, cmd, objects)
			} else {
				// If URL parsing fails, display as text
				label := widget.NewLabel(cmd.LinkText)
				label.Wrapping = fyne.TextWrapWord
				cr.addObjectToDisplay(label, cmd, objects)
			}
		}

	case PaintBorder:
		// Render borders as lines or rectangles
		// Borders meet at corners without overlapping
		borderContainer := container.NewWithoutLayout()

		getBorderColor := func(c color.Color) color.Color {
			if c == nil {
				return theme.ForegroundColor()
			}
			return c
		}

		// Top border (full width)
		if cmd.BorderTopWidth > 0 && cmd.BorderTopStyle != "" && cmd.BorderTopStyle != "none" {
			topBorder := canvas.NewRectangle(getBorderColor(cmd.BorderTopColor))
			topBorder.Resize(fyne.NewSize(cmd.Box.Width, cmd.BorderTopWidth))
			topBorder.Move(fyne.NewPos(0, 0))
			borderContainer.Add(topBorder)
		}

		// Right border (height minus top and bottom border widths to avoid overlap)
		if cmd.BorderRightWidth > 0 && cmd.BorderRightStyle != "" && cmd.BorderRightStyle != "none" {
			rightBorder := canvas.NewRectangle(getBorderColor(cmd.BorderRightColor))
			rightHeight := cmd.Box.Height - cmd.BorderTopWidth - cmd.BorderBottomWidth
			rightBorder.Resize(fyne.NewSize(cmd.BorderRightWidth, rightHeight))
			rightBorder.Move(fyne.NewPos(cmd.Box.Width-cmd.BorderRightWidth, cmd.BorderTopWidth))
			borderContainer.Add(rightBorder)
		}

		// Bottom border (full width)
		if cmd.BorderBottomWidth > 0 && cmd.BorderBottomStyle != "" && cmd.BorderBottomStyle != "none" {
			bottomBorder := canvas.NewRectangle(getBorderColor(cmd.BorderBottomColor))
			bottomBorder.Resize(fyne.NewSize(cmd.Box.Width, cmd.BorderBottomWidth))
			bottomBorder.Move(fyne.NewPos(0, cmd.Box.Height-cmd.BorderBottomWidth))
			borderContainer.Add(bottomBorder)
		}

		// Left border (height minus top and bottom border widths to avoid overlap)
		if cmd.BorderLeftWidth > 0 && cmd.BorderLeftStyle != "" && cmd.BorderLeftStyle != "none" {
			leftBorder := canvas.NewRectangle(getBorderColor(cmd.BorderLeftColor))
			leftHeight := cmd.Box.Height - cmd.BorderTopWidth - cmd.BorderBottomWidth
			leftBorder.Resize(fyne.NewSize(cmd.BorderLeftWidth, leftHeight))
			leftBorder.Move(fyne.NewPos(0, cmd.BorderTopWidth))
			borderContainer.Add(leftBorder)
		}

		if len(borderContainer.Objects) > 0 {
			cr.addObjectToDisplay(borderContainer, cmd, objects)
		}
	
	case PaintButton:
		// Render button widget
		if cmd.ButtonText == "" {
			return
		}
		
		button := widget.NewButton(cmd.ButtonText, func() {
			// Handle onclick if JavaScript runtime is available
			// For now, we'll just create an empty handler
			// The onclick attribute would need to be executed via JavaScript runtime
		})
		cr.addObjectToDisplay(button, cmd, objects)

	case PaintInput:
		// Render input field widget
		entry := widget.NewEntry()
		if cmd.InputPlaceholder != "" {
			entry.SetPlaceHolder(cmd.InputPlaceholder)
		}
		cr.addObjectToDisplay(entry, cmd, objects)

	case PaintTextarea:
		// Render textarea widget
		entry := widget.NewMultiLineEntry()
		if cmd.TextareaPlaceholder != "" {
			entry.SetPlaceHolder(cmd.TextareaPlaceholder)
		}
		cr.addObjectToDisplay(entry, cmd, objects)

	case PaintTable:
		// Render table widget
		if len(cmd.TableData) == 0 || cmd.TableMaxCols == 0 {
			return
		}

		data := cmd.TableData
		maxCols := cmd.TableMaxCols

		table := widget.NewTable(
			func() (int, int) {
				return len(data), maxCols
			},
			func() fyne.CanvasObject {
				return widget.NewLabel("")
			},
			func(i widget.TableCellID, o fyne.CanvasObject) {
				if i.Row < len(data) && i.Col < len(data[i.Row]) {
					o.(*widget.Label).SetText(data[i.Row][i.Col])
				}
			},
		)

		// Calculate column widths based on content
		colWidths := make([]float32, maxCols)
		for col := 0; col < maxCols; col++ {
			maxWidth := float32(80)
			for row := 0; row < len(data); row++ {
				if col < len(data[row]) {
					textWidth := float32(len(data[row][col])) * 8
					if textWidth > maxWidth {
						maxWidth = textWidth
					}
				}
			}
			if maxWidth > 200 {
				maxWidth = 200
			}
			colWidths[col] = maxWidth
		}

		var totalWidth float32
		for i := 0; i < maxCols; i++ {
			table.SetColumnWidth(i, colWidths[i])
			totalWidth += colWidths[i]
		}

		// Resize happens in addObjectToDisplay
		// But table content size needs to be set separately?
		// widget.Table calculates content size based on Length/CreateCell/UpdateCell.
		// LayoutEngine sets cmd.Box.Height.
		// addObjectToDisplay calls Resize on table.
		cr.addObjectToDisplay(table, cmd, objects)
	}
}

// ClearCache clears the cached display list to force re-rendering
func (cr *CanvasRenderer) ClearCache() {
	cr.cachedDisplayList = nil
	cr.cachedLayoutRoot = nil
	cr.cachedRenderRoot = nil
}

func (cr *CanvasRenderer) renderInput(node *RenderNode, objects *[]fyne.CanvasObject) {
	entry := widget.NewEntry()
	if placeholder, ok := node.GetAttribute("placeholder"); ok {
		entry.SetPlaceHolder(placeholder)
	}
	// Resize to ensure the input is visible
	entry.Resize(fyne.NewSize(200, 36))
	*objects = append(*objects, entry)
}

func (cr *CanvasRenderer) renderTable(node *RenderNode, objects *[]fyne.CanvasObject) {
	data := [][]string{}
	var maxCols int

	// Helper function to extract rows from a node (handles tbody, thead, tfoot)
	var extractRows func(*RenderNode)
	extractRows = func(n *RenderNode) {
		for _, child := range n.Children {
			if child.TagName == "tr" {
				row := []string{}
				for _, td := range child.Children {
					if td.TagName == "td" || td.TagName == "th" {
						row = append(row, cr.extractText(td))
					}
				}
				if len(row) > maxCols {
					maxCols = len(row)
				}
				data = append(data, row)
			} else if child.TagName == "tbody" || child.TagName == "thead" || child.TagName == "tfoot" {
				// Recursively process tbody, thead, tfoot
				extractRows(child)
			}
		}
	}

	extractRows(node)

	if len(data) == 0 || maxCols == 0 {
		return
	}

	// Calculate column widths based on content
	colWidths := make([]float32, maxCols)
	for col := 0; col < maxCols; col++ {
		maxWidth := float32(80) // Minimum column width
		for row := 0; row < len(data); row++ {
			if col < len(data[row]) {
				// Estimate width based on text length (approx 8 pixels per character)
				textWidth := float32(len(data[row][col])) * 8
				if textWidth > maxWidth {
					maxWidth = textWidth
				}
			}
		}
		// Cap at reasonable maximum
		if maxWidth > 200 {
			maxWidth = 200
		}
		colWidths[col] = maxWidth
	}

	table := widget.NewTable(
		func() (int, int) {
			return len(data), maxCols
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(i widget.TableCellID, o fyne.CanvasObject) {
			if i.Row < len(data) && i.Col < len(data[i.Row]) {
				o.(*widget.Label).SetText(data[i.Row][i.Col])
			}
		},
	)

	// Set column widths
	var totalWidth float32
	for i := 0; i < maxCols; i++ {
		table.SetColumnWidth(i, colWidths[i])
		totalWidth += colWidths[i]
	}

	// Resize to ensure the table is visible
	// Row height is approximately 30 pixels
	rowHeight := float32(30)
	tableHeight := float32(len(data)) * rowHeight
	if tableHeight < 60 {
		tableHeight = 60
	}
	table.Resize(fyne.NewSize(totalWidth, tableHeight))

	*objects = append(*objects, table)
}

func (cr *CanvasRenderer) renderButton(node *RenderNode, objects *[]fyne.CanvasObject) {
	text := cr.extractText(node)
	button := widget.NewButton(text, func() {})
	// Resize to ensure the button is visible
	button.Resize(fyne.NewSize(100, 36))
	*objects = append(*objects, button)
}

func (cr *CanvasRenderer) renderTextarea(node *RenderNode, objects *[]fyne.CanvasObject) {
	entry := widget.NewMultiLineEntry()
	if placeholder, ok := node.GetAttribute("placeholder"); ok {
		entry.SetPlaceHolder(placeholder)
	}
	// Resize to ensure the textarea is visible
	entry.Resize(fyne.NewSize(200, 100))
	*objects = append(*objects, entry)
}

// hasCustomStyles checks if a node has CSS styles that require custom rendering
func (cr *CanvasRenderer) hasCustomStyles(node *RenderNode) bool {
	return node != nil && node.ComputedStyle != nil && (node.ComputedStyle.Color != nil ||
		node.ComputedStyle.FontSize > 0 ||
		node.ComputedStyle.FontWeight == "bold")
}

// applyStylesToLabel applies CSS styles from ComputedStyle to a label widget.
// Since Fyne's standard Label widget doesn't support custom colors or font sizes,
// this function creates a styled canvas.Text object when custom styles are present.
// Note: canvas.Text objects don't support text wrapping, which is a known limitation.
func (cr *CanvasRenderer) applyStylesToLabel(node *RenderNode, text string) fyne.CanvasObject {
	if !cr.hasCustomStyles(node) {
		// No custom styles, use selectable text widget
		label := widget.NewLabel(text)
		label.Wrapping = fyne.TextWrapWord

		// Apply tag-based styles (bold, italic, etc.)
		if node.Parent != nil {
			label.TextStyle = cr.fontMetrics.GetTextStyle(node.Parent.TagName)
		}

		return label
	}

	// Create a styled canvas.Text object for custom colors/sizes
	// Note: canvas.Text doesn't support selection, but we need it for custom colors
	textObj := canvas.NewText(text, theme.ForegroundColor())
	textObj.TextSize = cr.defaultSize

	// Apply computed styles
	style := node.ComputedStyle

	if style.Color != nil {
		textObj.Color = style.Color
	}

	if style.FontSize > 0 {
		textObj.TextSize = style.FontSize
	}

	if style.FontWeight == "bold" {
		textObj.TextStyle = fyne.TextStyle{Bold: true}
	}

	return textObj
}

// addObjectToDisplay adds an object to the display list with correct positioning and sizing
func (cr *CanvasRenderer) addObjectToDisplay(obj fyne.CanvasObject, cmd *PaintCommand, objects *[]fyne.CanvasObject) {
	if obj == nil {
		return
	}
	// Calculate size and position
	width := cmd.Box.Width
	height := cmd.Box.Height
	x := cmd.Box.X
	y := cmd.Box.Y - cr.viewportY
	
	// Ensure no negative dimensions
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	// Set size and position
	obj.Resize(fyne.NewSize(width, height))
	obj.Move(fyne.NewPos(x, y))

	*objects = append(*objects, obj)
}
