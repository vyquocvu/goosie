package renderer

import (
	"log"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
)

// WhiteSpaceMode represents how white space should be handled
type WhiteSpaceMode int

const (
	// WhiteSpaceNormal collapses white space and wraps text normally
	WhiteSpaceNormal WhiteSpaceMode = iota
	// WhiteSpaceNoWrap collapses white space but prevents wrapping
	WhiteSpaceNoWrap
	// WhiteSpacePre preserves white space and prevents wrapping
	WhiteSpacePre
	// WhiteSpacePreWrap preserves white space but allows wrapping
	WhiteSpacePreWrap
	// WhiteSpacePreLine collapses white space except newlines and allows wrapping
	WhiteSpacePreLine
)

// VerticalAlign represents vertical alignment for inline elements
type VerticalAlign int

const (
	// VerticalAlignBaseline aligns to parent's baseline
	VerticalAlignBaseline VerticalAlign = iota
	// VerticalAlignTop aligns to top of line box
	VerticalAlignTop
	// VerticalAlignBottom aligns to bottom of line box
	VerticalAlignBottom
	// VerticalAlignMiddle aligns to middle of line box
	VerticalAlignMiddle
	// VerticalAlignTextTop aligns to top of parent's content area
	VerticalAlignTextTop
	// VerticalAlignTextBottom aligns to bottom of parent's content area
	VerticalAlignTextBottom
	// VerticalAlignSub subscript alignment
	VerticalAlignSub
	// VerticalAlignSuper superscript alignment
	VerticalAlignSuper
)

// LineBox represents a horizontal line containing inline elements
type LineBox struct {
	X              float32      // X position of line
	Y              float32      // Y position (baseline)
	Width          float32      // Total width of content in line
	Height         float32      // Height of line box
	Ascent         float32      // Distance from baseline to top
	Descent        float32      // Distance from baseline to bottom
	InlineBoxes    []*InlineBox // Inline boxes in this line
	AvailableWidth float32      // Available width for line
	TextAlign      string       // Text alignment for this line
	LineHeight     float32      // Preferred line height
}

// InlineBox represents an inline-level box (text or inline element)
type InlineBox struct {
	NodeID        int64         // ID of corresponding RenderNode
	X             float32       // X position relative to line
	Y             float32       // Y position relative to line (adjusted for vertical align)
	Width         float32       // Width of inline box
	Height        float32       // Height of inline box
	Ascent        float32       // Baseline to top
	Descent       float32       // Baseline to bottom
	Text          string        // Text content (for text nodes)
	IsText        bool          // True if this is a text node
	VerticalAlign VerticalAlign // Vertical alignment
	LayoutBox     *LayoutBox    // Reference to layout box for inline-block elements
	LetterSpacing float32       // Letter spacing for this box
}

// InlineLayoutEngine handles inline layout calculations
type InlineLayoutEngine struct {
	fontMetrics     *FontMetrics
	defaultFontSize float32
}

// NewInlineLayoutEngine creates a new inline layout engine
func NewInlineLayoutEngine(fontMetrics *FontMetrics, defaultFontSize float32) *InlineLayoutEngine {
	return &InlineLayoutEngine{
		fontMetrics:     fontMetrics,
		defaultFontSize: defaultFontSize,
	}
}

// LayoutInlineContent performs inline layout for a container with inline children
// Returns the lines created and the total height consumed
func (ile *InlineLayoutEngine) LayoutInlineContent(
	node *RenderNode,
	x, y, availableWidth float32,
	whiteSpaceMode WhiteSpaceMode,
) ([]*LineBox, float32) {

	lines := make([]*LineBox, 0)

	textAlign := ""
	lineHeight := float32(0)
	if node.ComputedStyle != nil {
		textAlign = node.ComputedStyle.TextAlign
		lineHeight = node.ComputedStyle.LineHeight
	}

	currentLine := ile.newLineBox(x, y, availableWidth, textAlign, lineHeight)

	// Process all inline children and text nodes
	for _, child := range node.Children {
		ile.addNodeToLines(child, &currentLine, &lines, x, availableWidth, whiteSpaceMode)
	}

	// Add the last line if it has content
	if len(currentLine.InlineBoxes) > 0 {
		ile.finalizeLine(currentLine)
		lines = append(lines, currentLine)
	}

	// Calculate total height
	totalHeight := float32(0)
	for _, line := range lines {
		totalHeight += line.Height
	}

	return lines, totalHeight
}

// addNodeToLines adds a render node to the line boxes
func (ile *InlineLayoutEngine) addNodeToLines(
	node *RenderNode,
	currentLine **LineBox,
	lines *[]*LineBox,
	lineX, availableWidth float32,
	whiteSpaceMode WhiteSpaceMode,
) {
	if node == nil {
		return
	}

	if node.Type == NodeTypeText {
		ile.addTextToLines(node, currentLine, lines, lineX, availableWidth, whiteSpaceMode)
	} else if node.Type == NodeTypeElement {
		// Check if inline-block
		if ile.isInlineBlock(node) {
			ile.addInlineBlockToLines(node, currentLine, lines, lineX, availableWidth)
		} else {
			// Regular inline element - process children
			for _, child := range node.Children {
				ile.addNodeToLines(child, currentLine, lines, lineX, availableWidth, whiteSpaceMode)
			}
		}
	}
}

// addTextToLines adds text content to line boxes with proper word wrapping
func (ile *InlineLayoutEngine) addTextToLines(
	node *RenderNode,
	currentLine **LineBox,
	lines *[]*LineBox,
	lineX, availableWidth float32,
	whiteSpaceMode WhiteSpaceMode,
) {
	text := node.Text
	if text == "" {
		return
	}

	// Process white space according to mode
	text = ile.processWhiteSpace(text, whiteSpaceMode)
	if text == "" {
		return
	}

	// Get font properties
	fontSize := ile.getFontSizeForNode(node)
	style := ile.fontMetrics.GetTextStyleFromNode(node)
	
	// Apply FontStyle
	if node != nil && node.ComputedStyle != nil && node.ComputedStyle.FontStyle == "italic" {
		style.Italic = true
	}
	
	// Apply TextTransform
	if node != nil && node.ComputedStyle != nil && node.ComputedStyle.TextTransform != "" {
		text = ile.applyTextTransform(text, node.ComputedStyle.TextTransform)
	}

	letterSpacing := float32(0)
	if node != nil && node.ComputedStyle != nil {
		letterSpacing = node.ComputedStyle.LetterSpacing
	}

	// Split text into words or characters based on white space mode
	if whiteSpaceMode == WhiteSpacePre || whiteSpaceMode == WhiteSpaceNoWrap {
		// No wrapping - add as single piece
		ile.addTextPiece(text, node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing, false)
	} else {
		// Word wrapping
		words := ile.splitTextForWrapping(text, whiteSpaceMode)
		for i, word := range words {
			// Add space before word if not first word
			addSpace := i > 0 && !strings.HasPrefix(word, " ")
			ile.addTextPiece(word, node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing, addSpace)
		}
	}
}

// addTextPiece adds a piece of text to the current line or creates a new line
func (ile *InlineLayoutEngine) addTextPiece(
	text string,
	node *RenderNode,
	currentLine **LineBox,
	lines *[]*LineBox,
	lineX, availableWidth float32,
	fontSize float32,
	style fyne.TextStyle,
	letterSpacing float32,
	addSpaceBefore bool,
) {
	if text == "" {
		return
	}

	// If letterSpacing is non-zero, we break the text into characters to allow for correct positioning
	if letterSpacing != 0 {
		ile.addTextWithCharacterBreaking(text, node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing)
		return
	}

	// Measure text
	metrics := ile.fontMetrics.MeasureText(text, fontSize, style, letterSpacing)

	// Add space width if needed
	spaceWidth := float32(0)
	if addSpaceBefore && len((*currentLine).InlineBoxes) > 0 {
		spaceMetrics := ile.fontMetrics.MeasureText(" ", fontSize, style, letterSpacing)
		spaceWidth = spaceMetrics.Width
	}

	totalWidth := metrics.Width + spaceWidth

	// Check if text fits on current line
	if (*currentLine).Width+totalWidth > (*currentLine).AvailableWidth && len((*currentLine).InlineBoxes) > 0 {
		// Text doesn't fit - finalize current line and create new one
		ile.finalizeLine(*currentLine)
		*lines = append(*lines, *currentLine)

		textAlign := (*currentLine).TextAlign
		nextY := (*currentLine).Y + (*currentLine).Height
		*currentLine = ile.newLineBox(lineX, nextY, availableWidth, textAlign, (*currentLine).LineHeight)
		spaceWidth = 0 // No space at start of new line

		// Re-measure for new line
		totalWidth = metrics.Width
	}

	// If text still doesn't fit (very long word), break it into characters
	if metrics.Width > (*currentLine).AvailableWidth && len((*currentLine).InlineBoxes) == 0 {
		ile.addTextWithCharacterBreaking(text, node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing)
		return
	}

	// Create inline box for text
	boxText := text
	if addSpaceBefore {
		boxText = " " + text
	}

	inlineBox := &InlineBox{
		NodeID:        node.ID,
		X:             (*currentLine).Width,
		Y:             0, // Will be adjusted based on vertical alignment
		Width:         totalWidth,
		Height:        metrics.Height,
		Ascent:        metrics.Ascent,
		Descent:       metrics.Descent,
		Text:          boxText,
		IsText:        true,
		VerticalAlign: VerticalAlignBaseline,
	}

	// Apply vertical alignment for sub/sup if parent indicates it
	if node.Parent != nil {
		switch node.Parent.TagName {
		case "sub":
			inlineBox.VerticalAlign = VerticalAlignSub
		case "sup":
			inlineBox.VerticalAlign = VerticalAlignSuper
		}
	}

	(*currentLine).InlineBoxes = append((*currentLine).InlineBoxes, inlineBox)
	(*currentLine).Width += totalWidth

	// Update line metrics
	if inlineBox.Ascent > (*currentLine).Ascent {
		(*currentLine).Ascent = inlineBox.Ascent
	}
	if inlineBox.Descent > (*currentLine).Descent {
		(*currentLine).Descent = inlineBox.Descent
	}
}

// addTextWithCharacterBreaking breaks very long words at character boundaries
func (ile *InlineLayoutEngine) addTextWithCharacterBreaking(
	text string,
	node *RenderNode,
	currentLine **LineBox,
	lines *[]*LineBox,
	lineX, availableWidth float32,
	fontSize float32,
	style fyne.TextStyle,
	letterSpacing float32,
) {
	if letterSpacing != 0 {
		// For letter-spacing, we must emit each character separately
		for _, ch := range text {
			charStr := string(ch)
			metrics := ile.fontMetrics.MeasureText(charStr, fontSize, style, letterSpacing)

			if metrics.Width > (*currentLine).AvailableWidth && (*currentLine).Width > 0 {
				ile.finalizeLine(*currentLine)
				*lines = append(*lines, *currentLine)
				textAlign := (*currentLine).TextAlign
				lineHeight := (*currentLine).LineHeight
				nextY := (*currentLine).Y + (*currentLine).Height
				*currentLine = ile.newLineBox(lineX, nextY, availableWidth, textAlign, lineHeight)
			}

			box := &InlineBox{
				NodeID:        node.ID,
				Text:          charStr,
				Width:         metrics.Width,
				Height:        metrics.Height,
				X:             (*currentLine).Width,
				Y:             0,
				Ascent:        metrics.Ascent,
				Descent:       metrics.Descent,
				IsText:        true,
				LetterSpacing: letterSpacing,
				VerticalAlign: VerticalAlignBaseline,
			}
			(*currentLine).InlineBoxes = append((*currentLine).InlineBoxes, box)
			(*currentLine).Width += metrics.Width
			if box.Ascent > (*currentLine).Ascent {
				(*currentLine).Ascent = box.Ascent
			}
			if box.Descent > (*currentLine).Descent {
				(*currentLine).Descent = box.Descent
			}
		}
		return
	}

	// Normal text accumulation
	currentPiece := strings.Builder{}
	for _, ch := range text {
		charStr := string(ch)
		piece := currentPiece.String()

		// Measure piece + this char
		fullMetrics := ile.fontMetrics.MeasureText(piece+charStr, fontSize, style, 0)

		if fullMetrics.Width > (*currentLine).AvailableWidth {
			// Doesn't fit on this line
			if currentPiece.Len() > 0 {
				// Emit current piece
				p := currentPiece.String()
				m := ile.fontMetrics.MeasureText(p, fontSize, style, 0)
				box := &InlineBox{
					NodeID:        node.ID,
					Text:          p,
					Width:         m.Width,
					Height:        m.Height,
					X:             (*currentLine).Width,
					Y:             0,
					Ascent:        m.Ascent,
					Descent:       m.Descent,
					IsText:        true,
					VerticalAlign: VerticalAlignBaseline,
				}
				(*currentLine).InlineBoxes = append((*currentLine).InlineBoxes, box)
				(*currentLine).Width += m.Width
				if box.Ascent > (*currentLine).Ascent {
					(*currentLine).Ascent = box.Ascent
				}
				if box.Descent > (*currentLine).Descent {
					(*currentLine).Descent = box.Descent
				}
				currentPiece.Reset()
			}

			// Wrap if line is not empty OR if even this single char doesn't fit on empty line
			if (*currentLine).Width > 0 || ile.fontMetrics.MeasureText(charStr, fontSize, style, 0).Width > (*currentLine).AvailableWidth {
				ile.finalizeLine(*currentLine)
				*lines = append(*lines, *currentLine)
				textAlign := (*currentLine).TextAlign
				lineHeight := (*currentLine).LineHeight
				nextY := (*currentLine).Y + (*currentLine).Height
				*currentLine = ile.newLineBox(lineX, nextY, availableWidth, textAlign, lineHeight)
			}
		}
		currentPiece.WriteRune(ch)
	}

	// Final piece
	if currentPiece.Len() > 0 {
		p := currentPiece.String()
		m := ile.fontMetrics.MeasureText(p, fontSize, style, 0)
		box := &InlineBox{
			NodeID:        node.ID,
			Text:          p,
			Width:         m.Width,
			Height:        m.Height,
			X:             (*currentLine).Width,
			Y:             0,
			Ascent:        m.Ascent,
			Descent:       m.Descent,
			IsText:        true,
			VerticalAlign: VerticalAlignBaseline,
		}
		(*currentLine).InlineBoxes = append((*currentLine).InlineBoxes, box)
		(*currentLine).Width += m.Width
		if box.Ascent > (*currentLine).Ascent {
			(*currentLine).Ascent = box.Ascent
		}
		if box.Descent > (*currentLine).Descent {
			(*currentLine).Descent = box.Descent
		}
	}
}

// addInlineBlockToLines adds an inline-block element to lines
func (ile *InlineLayoutEngine) addInlineBlockToLines(
	node *RenderNode,
	currentLine **LineBox,
	lines *[]*LineBox,
	lineX, availableWidth float32,
) {
	// For inline-block, we need to compute its layout first
	// This is a placeholder - actual implementation would use the layout engine

	// Estimate size (in real implementation, would compute actual layout)
	fontSize := ile.getFontSizeForNode(node)
	height := fontSize * 1.5
	width := fontSize * 5 // Placeholder width

	// Check if inline-block fits on current line
	if (*currentLine).Width+width > (*currentLine).AvailableWidth && len((*currentLine).InlineBoxes) > 0 {
		// Doesn't fit - start new line
		ile.finalizeLine(*currentLine)
		*lines = append(*lines, *currentLine)

		textAlign := (*currentLine).TextAlign
		nextY := (*currentLine).Y + (*currentLine).Height
		*currentLine = ile.newLineBox(lineX, nextY, availableWidth, textAlign, (*currentLine).LineHeight)
	}

	// Create inline box for inline-block
	inlineBox := &InlineBox{
		NodeID:        node.ID,
		X:             (*currentLine).Width,
		Y:             0,
		Width:         width,
		Height:        height,
		Ascent:        height * 0.75,
		Descent:       height * 0.25,
		Text:          "",
		IsText:        false,
		VerticalAlign: VerticalAlignBaseline,
	}

	(*currentLine).InlineBoxes = append((*currentLine).InlineBoxes, inlineBox)
	(*currentLine).Width += width

	// Update line metrics
	if inlineBox.Ascent > (*currentLine).Ascent {
		(*currentLine).Ascent = inlineBox.Ascent
	}
	if inlineBox.Descent > (*currentLine).Descent {
		(*currentLine).Descent = inlineBox.Descent
	}
}

// finalizeLine finalizes a line box by computing final positions and height
func (ile *InlineLayoutEngine) finalizeLine(line *LineBox) {
	if len(line.InlineBoxes) == 0 {
		return
	}

	// Coalesce inline boxes with same NodeID and 0 letter-spacing on same line
	if len(line.InlineBoxes) > 1 {
		log.Printf("DEBUG: finalizeLine before coalescing: %d boxes", len(line.InlineBoxes))
		coalesced := make([]*InlineBox, 0, len(line.InlineBoxes))
		for _, box := range line.InlineBoxes {
			if len(coalesced) > 0 {
				last := coalesced[len(coalesced)-1]
				// Only merge text boxes with the same NodeID, no letter-spacing, and compatible layout metrics
				if last.IsText && box.IsText && last.NodeID == box.NodeID &&
					last.LetterSpacing == 0 && box.LetterSpacing == 0 &&
					last.VerticalAlign == box.VerticalAlign &&
					last.Ascent == box.Ascent && last.Descent == box.Descent {
					last.Text += box.Text
					last.Width += box.Width
					continue
				}
			}
			coalesced = append(coalesced, box)
		}
		line.InlineBoxes = coalesced
		log.Printf("DEBUG: finalizeLine after coalescing: %d boxes", len(line.InlineBoxes))
	}

	// Set line height based on maximum ascent and descent
	lineHeight := line.Ascent + line.Descent

	// Also consider the maximum inline box height (includes leading/line-height)
	// so that the line height matches the visual height of the tallest inline box
	for _, box := range line.InlineBoxes {
		if box.Height > lineHeight {
			lineHeight = box.Height
		}
	}

	// If we have a preferred line height, use it if it's larger or if explicitly set
	if line.LineHeight > 0 {
		if line.LineHeight > lineHeight {
			line.Height = line.LineHeight
		} else {
			line.Height = lineHeight
		}
	} else {
		line.Height = lineHeight
	}

	// Adjust vertical positions of inline boxes based on vertical alignment
	// and center them if we have extra line height
	verticalOffset := (line.Height - (line.Ascent + line.Descent)) / 2
	
	for _, box := range line.InlineBoxes {
		boxY := float32(0)
		switch box.VerticalAlign {
		case VerticalAlignBaseline:
			boxY = line.Ascent - box.Ascent + verticalOffset
		case VerticalAlignTop:
			boxY = 0
		case VerticalAlignBottom:
			boxY = line.Height - box.Height
		case VerticalAlignMiddle:
			boxY = (line.Height - box.Height) / 2
		case VerticalAlignTextTop:
			boxY = verticalOffset
		case VerticalAlignTextBottom:
			boxY = line.Height - box.Height - verticalOffset
		case VerticalAlignSub:
			boxY = line.Ascent - box.Ascent + verticalOffset + box.Height*0.2
		case VerticalAlignSuper:
			boxY = line.Ascent - box.Ascent + verticalOffset - box.Height*0.3
		default:
			boxY = line.Ascent - box.Ascent + verticalOffset
		}
		box.Y = boxY
	}

	// Horizontal alignment
	remainingWidth := line.AvailableWidth - line.Width
	if remainingWidth > 0 {
		var startX float32
		switch line.TextAlign {
		case "center":
			startX = remainingWidth / 2
		case "right":
			startX = remainingWidth
		default:
			startX = 0
		}

		if startX > 0 {
			for _, box := range line.InlineBoxes {
				box.X += startX
			}
		}
	}
}

// newLineBox creates a new line box
func (ile *InlineLayoutEngine) newLineBox(x, y, availableWidth float32, textAlign string, lineHeight float32) *LineBox {
	return &LineBox{
		X:              x,
		Y:              y,
		Width:          0,
		Height:         0,
		Ascent:         0,
		Descent:        0,
		InlineBoxes:    make([]*InlineBox, 0),
		AvailableWidth: availableWidth,
		TextAlign:      textAlign,
		LineHeight:     lineHeight,
	}
}

// processWhiteSpace processes white space according to the mode
func (ile *InlineLayoutEngine) processWhiteSpace(text string, mode WhiteSpaceMode) string {
	switch mode {
	case WhiteSpaceNormal, WhiteSpaceNoWrap:
		// Collapse white space: convert sequences of white space to single space
		return ile.collapseWhiteSpace(text)
	case WhiteSpacePre:
		// Preserve all white space
		return text
	case WhiteSpacePreWrap:
		// Preserve white space but allow wrapping
		return text
	case WhiteSpacePreLine:
		// Collapse white space except newlines
		return ile.collapseWhiteSpacePreserveNewlines(text)
	default:
		return ile.collapseWhiteSpace(text)
	}
}

// collapseWhiteSpace collapses sequences of white space into single spaces
func (ile *InlineLayoutEngine) collapseWhiteSpace(text string) string {
	var result strings.Builder
	prevWasSpace := false

	for _, ch := range text {
		if unicode.IsSpace(ch) {
			if !prevWasSpace {
				result.WriteRune(' ')
				prevWasSpace = true
			}
		} else {
			result.WriteRune(ch)
			prevWasSpace = false
		}
	}

	return strings.TrimSpace(result.String())
}

// collapseWhiteSpacePreserveNewlines collapses white space but preserves newlines
func (ile *InlineLayoutEngine) collapseWhiteSpacePreserveNewlines(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = ile.collapseWhiteSpace(line)
	}
	return strings.Join(lines, "\n")
}

// splitTextForWrapping splits text into wrappable pieces
func (ile *InlineLayoutEngine) splitTextForWrapping(text string, mode WhiteSpaceMode) []string {
	// For normal and pre-line modes, split on white space
	words := make([]string, 0)
	currentWord := strings.Builder{}

	for _, ch := range text {
		if unicode.IsSpace(ch) {
			if currentWord.Len() > 0 {
				words = append(words, currentWord.String())
				currentWord.Reset()
			}
		} else {
			currentWord.WriteRune(ch)
		}
	}

	if currentWord.Len() > 0 {
		words = append(words, currentWord.String())
	}

	return words
}

// getFontSizeForNode returns the font size for a node
func (ile *InlineLayoutEngine) getFontSizeForNode(node *RenderNode) float32 {
	if node == nil {
		return ile.defaultFontSize
	}

	// Priority 1: Computed style from CSS or inline style attribute
	if node.ComputedStyle != nil && node.ComputedStyle.FontSize > 0 {
		return node.ComputedStyle.FontSize
	}

	// Priority 2: Tag-based default size
	if node.Type == NodeTypeElement {
		return ile.fontMetrics.GetFontSize(node.TagName)
	}

	// Priority 3: Inherit from parent
	if node.Parent != nil {
		// Handle sub/sup font size reduction relative to grandparent
		if node.Parent.TagName == "sub" || node.Parent.TagName == "sup" {
			base := ile.getFontSizeForNode(node.Parent.Parent)
			return base * 0.83
		}
		return ile.getFontSizeForNode(node.Parent)
	}
	
	return ile.defaultFontSize
}

// applyTextTransform applies text-transform property logic
func (ile *InlineLayoutEngine) applyTextTransform(text string, transform string) string {
	switch transform {
	case "uppercase":
		return strings.ToUpper(text)
	case "lowercase":
		return strings.ToLower(text)
	case "capitalize":
		return strings.Title(text)
	default:
		return text
	}
}

// isInlineBlock checks if a node should be treated as inline-block
func (ile *InlineLayoutEngine) isInlineBlock(node *RenderNode) bool {
	// In a real implementation, this would check computed styles
	// For now, we'll treat certain elements as inline-block
	inlineBlockElements := map[string]bool{
		"img":    true,
		"button": true,
		"input":  true,
		"select": true,
	}
	return inlineBlockElements[node.TagName]
}
