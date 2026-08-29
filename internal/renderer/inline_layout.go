package renderer

import (
	"strings"
	"sync"
	"unicode"

	"fyne.io/fyne/v2"
	imageloader "github.com/vyquocvu/goosie/internal/image"
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
	floatCtx        *FloatContext
	mu              sync.RWMutex
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
	floatCtx *FloatContext,
) ([]*LineBox, float32) {
	return ile.LayoutInlineChildren(node.Children, x, y, availableWidth, whiteSpaceMode, floatCtx)
}

// LayoutInlineChildren performs inline layout for an explicit list of sibling
// nodes. It backs LayoutInlineContent and lets the block layout engine lay out
// inline runs between block-level children (anonymous block boxes).
func (ile *InlineLayoutEngine) LayoutInlineChildren(
	children []*RenderNode,
	x, y, availableWidth float32,
	whiteSpaceMode WhiteSpaceMode,
	floatCtx *FloatContext,
) ([]*LineBox, float32) {
	ile.mu.Lock()
	ile.floatCtx = floatCtx
	ile.mu.Unlock()
	defer func() {
		ile.mu.Lock()
		ile.floatCtx = nil
		ile.mu.Unlock()
	}()

	lines := make([]*LineBox, 0)

	textAlign := ""
	lineHeight := float32(0)
	if len(children) > 0 && children[0].Parent != nil && children[0].Parent.ComputedStyle != nil {
		textAlign = children[0].Parent.ComputedStyle.TextAlign
		lineHeight = children[0].Parent.ComputedStyle.LineHeight
	}

	lineX, widthForLine := ile.getLineXAndWidth(x, y, availableWidth, lineHeight)
	currentLine := ile.newLineBox(lineX, y, widthForLine, textAlign, lineHeight)

	// Process all inline children and text nodes
	for _, child := range children {
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

func (ile *InlineLayoutEngine) getLineXAndWidth(x, y, availableWidth, lineHeight float32) (float32, float32) {
	ile.mu.RLock()
	floatCtx := ile.floatCtx
	ile.mu.RUnlock()
	if floatCtx == nil {
		return x, availableWidth
	}
	expectedHeight := lineHeight
	if expectedHeight <= 0 {
		expectedHeight = 16.0 * 1.2
	}
	leftOffset, bfcAvailableWidth := floatCtx.GetAvailableWidth(y, expectedHeight)

	bfcLeft := floatCtx.containerX + leftOffset
	bfcRight := bfcLeft + bfcAvailableWidth

	lineLeft := maxFloat32(x, bfcLeft)
	lineRight := minFloat32(x+availableWidth, bfcRight)

	lineWidth := lineRight - lineLeft
	if lineWidth < 0 {
		lineWidth = 0
	}
	return lineLeft, lineWidth
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

	// Skip elements that should not participate in normal text flow
	if node.ComputedStyle != nil {
		// Skip display:none elements
		if node.ComputedStyle.Display == "none" {
			return
		}
		// Skip absolute and fixed positioned elements - they don't participate in inline flow
		if node.ComputedStyle.Position == "absolute" || node.ComputedStyle.Position == "fixed" {
			return
		}
		// Skip visibility:hidden elements (they take up space but don't render)
		// Actually visibility:hidden still takes space in inline flow, so we still process text
		// but the text node text should still flow (hidden from rendering, not from layout)
	}

	if node.Type == NodeTypeText {
		ile.addTextToLines(node, currentLine, lines, lineX, availableWidth, whiteSpaceMode)
	} else if node.Type == NodeTypeElement {
		// A forced line break: finalize the current line and start a new one.
		if node.TagName == "br" {
			ile.addLineBreak(currentLine, lines, lineX, availableWidth)
			return
		}
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

// addLineBreak handles a <br> element: emit the current line (even when
// empty, so consecutive <br>s produce visible blank lines) and open a fresh
// line below it. An empty line is given the block's strut height so vertical
// progress is preserved.
func (ile *InlineLayoutEngine) addLineBreak(
	currentLine **LineBox,
	lines *[]*LineBox,
	lineX, availableWidth float32,
) {
	line := *currentLine
	if line.Height <= 0 {
		if line.LineHeight > 0 {
			line.Height = line.LineHeight
		} else {
			line.Height = ile.defaultFontSize * 1.2
		}
	}
	if len(line.InlineBoxes) > 0 {
		ile.finalizeLine(line)
	}
	*lines = append(*lines, line)

	nextY := line.Y + line.Height
	newLineX, newWidth := ile.getLineXAndWidth(lineX, nextY, availableWidth, line.LineHeight)
	*currentLine = ile.newLineBox(newLineX, nextY, newWidth, line.TextAlign, line.LineHeight)
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
	if whiteSpaceMode == WhiteSpacePre || whiteSpaceMode == WhiteSpaceNoWrap {
		text = ile.processWhiteSpace(text, whiteSpaceMode)
		if text == "" {
			return
		}
		fontSize := ile.getFontSizeForNode(node)
		style := ile.fontMetrics.GetTextStyleFromNode(node)
		if node != nil && node.ComputedStyle != nil && node.ComputedStyle.FontStyle == "italic" {
			style.Italic = true
		}
		letterSpacing := float32(0)
		if node != nil && node.ComputedStyle != nil {
			letterSpacing = node.ComputedStyle.LetterSpacing
		}
		// pre mode honors embedded newlines as forced breaks; nowrap keeps
		// the whole run on one line.
		if whiteSpaceMode == WhiteSpacePre && strings.Contains(text, "\n") {
			segments := strings.Split(text, "\n")
			for i, seg := range segments {
				if seg != "" {
					ile.addTextPiece(seg, node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing, false, true)
				}
				if i < len(segments)-1 {
					ile.addLineBreak(currentLine, lines, lineX, availableWidth)
				}
			}
			return
		}
		ile.addTextPiece(text, node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing, false, true)
		return
	}

	// Determine if text starts/ends with whitespace or is only whitespace
	hasLeadingSpace := unicode.IsSpace(rune(text[0]))
	hasTrailingSpace := unicode.IsSpace(rune(text[len(text)-1]))
	isOnlyWhitespace := true
	for _, r := range text {
		if !unicode.IsSpace(r) {
			isOnlyWhitespace = false
			break
		}
	}

	fontSize := ile.getFontSizeForNode(node)
	style := ile.fontMetrics.GetTextStyleFromNode(node)
	if node != nil && node.ComputedStyle != nil && node.ComputedStyle.FontStyle == "italic" {
		style.Italic = true
	}

	letterSpacing := float32(0)
	if node != nil && node.ComputedStyle != nil {
		letterSpacing = node.ComputedStyle.LetterSpacing
	}

	if isOnlyWhitespace {
		if len((*currentLine).InlineBoxes) > 0 {
			lastBox := (*currentLine).InlineBoxes[len((*currentLine).InlineBoxes)-1]
			if lastBox.IsText && !strings.HasSuffix(lastBox.Text, " ") {
				ile.addTextPiece("", node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing, true, false)
			}
		}
		return
	}

	if node != nil && node.ComputedStyle != nil && node.ComputedStyle.TextTransform != "" {
		text = ile.applyTextTransform(text, node.ComputedStyle.TextTransform)
	}

	// Split text into words
	words := ile.splitTextForWrapping(text, whiteSpaceMode)
	for i, word := range words {
		addSpace := i > 0
		if i == 0 && hasLeadingSpace {
			addSpace = true
		}

		if addSpace && len((*currentLine).InlineBoxes) > 0 {
			lastBox := (*currentLine).InlineBoxes[len((*currentLine).InlineBoxes)-1]
			if lastBox.IsText && strings.HasSuffix(lastBox.Text, " ") {
				addSpace = false
			}
		}

		ile.addTextPiece(word, node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing, addSpace, false)
	}

	if hasTrailingSpace && len((*currentLine).InlineBoxes) > 0 {
		lastBox := (*currentLine).InlineBoxes[len((*currentLine).InlineBoxes)-1]
		if lastBox.IsText && !strings.HasSuffix(lastBox.Text, " ") {
			ile.addTextPiece("", node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing, true, false)
		}
	}
}

// addTextPiece adds a piece of text to the current line or creates a new
// line. When noWrap is set (white-space: nowrap/pre) the piece never wraps:
// it stays on the current line and may overflow it, as in browsers.
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
	noWrap bool,
) {
	if text == "" && !addSpaceBefore {
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
	if !noWrap && (*currentLine).Width+totalWidth > (*currentLine).AvailableWidth && len((*currentLine).InlineBoxes) > 0 {
		// Text doesn't fit - finalize current line and create new one
		ile.finalizeLine(*currentLine)
		*lines = append(*lines, *currentLine)

		textAlign := (*currentLine).TextAlign
		lineHeight := (*currentLine).LineHeight
		nextY := (*currentLine).Y + (*currentLine).Height
		newLineX, newWidth := ile.getLineXAndWidth(lineX, nextY, availableWidth, lineHeight)
		*currentLine = ile.newLineBox(newLineX, nextY, newWidth, textAlign, lineHeight)
		spaceWidth = 0 // No space at start of new line
		addSpaceBefore = false

		// Re-measure for new line
		totalWidth = metrics.Width
	}

	// If text still doesn't fit (very long word), break it into characters
	if !noWrap && metrics.Width > (*currentLine).AvailableWidth && len((*currentLine).InlineBoxes) == 0 {
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
				newLineX, newWidth := ile.getLineXAndWidth(lineX, nextY, availableWidth, lineHeight)
				*currentLine = ile.newLineBox(newLineX, nextY, newWidth, textAlign, lineHeight)
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
				newLineX, newWidth := ile.getLineXAndWidth(lineX, nextY, availableWidth, lineHeight)
				*currentLine = ile.newLineBox(newLineX, nextY, newWidth, textAlign, lineHeight)
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

	// Determine size from styling, attributes, or intrinsic image size
	fontSize := ile.getFontSizeForNode(node)
	var width, height float32
	width = -1
	height = -1

	// 1. Check computed styles
	if node.ComputedStyle != nil {
		if node.ComputedStyle.Width != "" && node.ComputedStyle.Width != "auto" {
			width = parseLengthWithViewport(node.ComputedStyle.Width, fontSize, availableWidth, availableWidth, availableWidth)
		}
		if node.ComputedStyle.Height != "" && node.ComputedStyle.Height != "auto" {
			height = parseLengthWithViewport(node.ComputedStyle.Height, fontSize, availableWidth, availableWidth, availableWidth)
		}
	}

	// 2. Check HTML attributes
	if width < 0 {
		if wAttr, ok := node.GetAttribute("width"); ok && wAttr != "" {
			width = parseLength(wAttr, fontSize)
		}
	}
	if height < 0 {
		if hAttr, ok := node.GetAttribute("height"); ok && hAttr != "" {
			height = parseLength(hAttr, fontSize)
		}
	}

	// 3. Fallback to image intrinsic size if loaded
	if node.TagName == "img" && node.ImageData != nil && node.ImageData.State == imageloader.StateLoaded {
		if width < 0 {
			width = float32(node.ImageData.Width)
		}
		if height < 0 {
			height = float32(node.ImageData.Height)
		}
	}

	// 4. Default fallbacks
	if width < 0 {
		width = fontSize * 5 // Placeholder width
	}
	if height < 0 {
		height = fontSize * 1.5 // Placeholder height
	}

	// Check if inline-block fits on current line
	if (*currentLine).Width+width > (*currentLine).AvailableWidth && len((*currentLine).InlineBoxes) > 0 {
		// Doesn't fit - start new line
		ile.finalizeLine(*currentLine)
		*lines = append(*lines, *currentLine)

		textAlign := (*currentLine).TextAlign
		lineHeight := (*currentLine).LineHeight
		nextY := (*currentLine).Y + (*currentLine).Height
		newLineX, newWidth := ile.getLineXAndWidth(lineX, nextY, availableWidth, lineHeight)
		*currentLine = ile.newLineBox(newLineX, nextY, newWidth, textAlign, lineHeight)
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
		// log.Printf("DEBUG: finalizeLine before coalescing: %d boxes", len(line.InlineBoxes))
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
		// log.Printf("DEBUG: finalizeLine after coalescing: %d boxes", len(line.InlineBoxes))
	}

	// Guard: ensure line ascent/descent reflect the tallest inline box before computing offsets.
	// This prevents negative boxY values when line.Ascent is still 0.
	for _, box := range line.InlineBoxes {
		if box.Ascent > line.Ascent {
			line.Ascent = box.Ascent
		}
		if box.Descent > line.Descent {
			line.Descent = box.Descent
		}
	}

	// Set line height based on maximum ascent and descent
	lineHeight := line.Ascent + line.Descent

	// Also consider the maximum inline box height (includes leading/line-height)
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
	if node.ComputedStyle != nil {
		disp := node.ComputedStyle.Display
		if disp == "inline-block" || disp == "inline-flex" || disp == "inline-grid" {
			return true
		}
	}
	switch node.TagName {
	case "img", "button", "input", "select":
		return true
	}
	return false
}
