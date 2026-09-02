package renderer

import (
	"strings"
	"sync"
	"unicode"

	"fyne.io/fyne/v2"
	"github.com/vyquocvu/goosie/internal/css"
	imageloader "github.com/vyquocvu/goosie/internal/image"
)

// WhiteSpaceMode represents how white space should be handled
type WhiteSpaceMode int

const (
	WhiteSpaceNormal WhiteSpaceMode = iota
	WhiteSpaceNoWrap
	WhiteSpacePre
	WhiteSpacePreWrap
	WhiteSpacePreLine
)

// VerticalAlign represents vertical alignment for inline elements
type VerticalAlign int

const (
	VerticalAlignBaseline VerticalAlign = iota
	VerticalAlignTop
	VerticalAlignBottom
	VerticalAlignMiddle
	VerticalAlignTextTop
	VerticalAlignTextBottom
	VerticalAlignSub
	VerticalAlignSuper
)

// LineBox represents a horizontal line containing inline elements
type LineBox struct {
	X              float32
	Y              float32
	Width          float32
	Height         float32
	Ascent         float32
	Descent        float32
	InlineBoxes    []*InlineBox
	AvailableWidth float32
	TextAlign      string
	LineHeight     float32
}

// InlineBox represents an inline-level box (text or inline element)
type InlineBox struct {
	NodeID        int64
	X             float32
	Y             float32
	Width         float32
	Height        float32
	Ascent        float32
	Descent       float32
	Text          string
	IsText        bool
	VerticalAlign VerticalAlign
	LayoutBox     *LayoutBox
	LetterSpacing float32
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
		textAlign = children[0].Parent.ComputedStyle.TextAlign.String()
		lineHeight = children[0].Parent.ComputedStyle.LineHeight
	}

	lineX, widthForLine := ile.getLineXAndWidth(x, y, availableWidth, lineHeight)
	currentLine := ile.newLineBox(lineX, y, widthForLine, textAlign, lineHeight)

	for _, child := range children {
		ile.addNodeToLines(child, &currentLine, &lines, x, availableWidth, whiteSpaceMode)
	}

	if len(currentLine.InlineBoxes) > 0 {
		ile.finalizeLine(currentLine)
		lines = append(lines, currentLine)
	}

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

	lineLeft := max(x, bfcLeft)
	lineRight := min(x+availableWidth, bfcRight)

	lineWidth := lineRight - lineLeft
	if lineWidth < 0 {
		lineWidth = 0
	}
	return lineLeft, lineWidth
}

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

	if node.ComputedStyle != nil {
		if node.ComputedStyle.Display == css.DisplayAtomNone {
			return
		}
		if node.ComputedStyle.Position == css.PositionAtomAbsolute || node.ComputedStyle.Position == css.PositionAtomFixed {
			return
		}
	}

	switch node.Type {
	case NodeTypeText:
		ile.addTextToLines(node, currentLine, lines, lineX, availableWidth, whiteSpaceMode)
	case NodeTypeElement:
		if node.TagName == "br" {
			ile.addLineBreak(currentLine, lines, lineX, availableWidth)
			return
		}
		if ile.isInlineBlock(node) {
			ile.addInlineBlockToLines(node, currentLine, lines, lineX, availableWidth)
		} else {
			marginLeft := float32(0)
			marginRight := float32(0)
			if node.ComputedStyle != nil {
				fontSize := ile.getFontSizeForNode(node)
				if node.ComputedStyle.MarginLeft != "" && node.ComputedStyle.MarginLeft != "auto" {
					marginLeft += parseLength(node.ComputedStyle.MarginLeft, fontSize)
				}
				if node.ComputedStyle.MarginRight != "" && node.ComputedStyle.MarginRight != "auto" {
					marginRight += parseLength(node.ComputedStyle.MarginRight, fontSize)
				}
				if node.ComputedStyle.PaddingLeft != "" && node.ComputedStyle.PaddingLeft != "auto" {
					marginLeft += parseLength(node.ComputedStyle.PaddingLeft, fontSize)
				}
				if node.ComputedStyle.PaddingRight != "" && node.ComputedStyle.PaddingRight != "auto" {
					marginRight += parseLength(node.ComputedStyle.PaddingRight, fontSize)
				}
			}

			if marginLeft > 0 && len((*currentLine).InlineBoxes) > 0 {
				(*currentLine).Width += marginLeft
			}

			for _, child := range node.Children {
				ile.addNodeToLines(child, currentLine, lines, lineX, availableWidth, whiteSpaceMode)
			}

			if marginRight > 0 {
				(*currentLine).Width += marginRight
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

	if whiteSpaceMode == WhiteSpacePre || whiteSpaceMode == WhiteSpaceNoWrap {
		text = ile.processWhiteSpace(text, whiteSpaceMode)
		if text == "" {
			return
		}
		fontSize := ile.getFontSizeForNode(node)
		style := ile.fontMetrics.GetTextStyleFromNode(node)
		if node != nil && node.ComputedStyle != nil && node.ComputedStyle.FontStyle == css.FontStyleAtomItalic {
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
	if node != nil && node.ComputedStyle != nil && node.ComputedStyle.FontStyle == css.FontStyleAtomItalic {
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

	if node != nil && node.ComputedStyle != nil && node.ComputedStyle.TextTransform != css.TextTransformAtomNone {
		text = ile.applyTextTransform(text, node.ComputedStyle.TextTransform.String())
	}

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

	if letterSpacing != 0 {
		ile.addTextWithCharacterBreaking(text, node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing)
		return
	}

	metrics := ile.fontMetrics.MeasureText(text, fontSize, style, letterSpacing)

	spaceWidth := float32(0)
	if addSpaceBefore && len((*currentLine).InlineBoxes) > 0 {
		spaceMetrics := ile.fontMetrics.MeasureText(" ", fontSize, style, letterSpacing)
		spaceWidth = spaceMetrics.Width
	}

	totalWidth := metrics.Width + spaceWidth

	if !noWrap && (*currentLine).Width+totalWidth > (*currentLine).AvailableWidth && len((*currentLine).InlineBoxes) > 0 {
		ile.finalizeLine(*currentLine)
		*lines = append(*lines, *currentLine)

		textAlign := (*currentLine).TextAlign
		lineHeight := (*currentLine).LineHeight
		nextY := (*currentLine).Y + (*currentLine).Height
		newLineX, newWidth := ile.getLineXAndWidth(lineX, nextY, availableWidth, lineHeight)
		*currentLine = ile.newLineBox(newLineX, nextY, newWidth, textAlign, lineHeight)
		spaceWidth = 0
		addSpaceBefore = false

		totalWidth = metrics.Width
	}

	if !noWrap && metrics.Width > (*currentLine).AvailableWidth && len((*currentLine).InlineBoxes) == 0 {
		ile.addTextWithCharacterBreaking(text, node, currentLine, lines, lineX, availableWidth, fontSize, style, letterSpacing)
		return
	}

	boxText := text
	if addSpaceBefore {
		boxText = " " + text
	}

	inlineBox := &InlineBox{
		NodeID:        node.ID,
		X:             (*currentLine).Width,
		Y:             0,
		Width:         totalWidth,
		Height:        metrics.Height,
		Ascent:        metrics.Ascent,
		Descent:       metrics.Descent,
		Text:          boxText,
		IsText:        true,
		VerticalAlign: VerticalAlignBaseline,
	}

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

	if inlineBox.Ascent > (*currentLine).Ascent {
		(*currentLine).Ascent = inlineBox.Ascent
	}
	if inlineBox.Descent > (*currentLine).Descent {
		(*currentLine).Descent = inlineBox.Descent
	}
}

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

	currentPiece := strings.Builder{}
	for _, ch := range text {
		charStr := string(ch)
		piece := currentPiece.String()

		fullMetrics := ile.fontMetrics.MeasureText(piece+charStr, fontSize, style, 0)

		if fullMetrics.Width > (*currentLine).AvailableWidth {
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
				currentPiece.Reset()
			}

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

func (ile *InlineLayoutEngine) addInlineBlockToLines(
	node *RenderNode,
	currentLine **LineBox,
	lines *[]*LineBox,
	lineX, availableWidth float32,
) {
	fontSize := ile.getFontSizeForNode(node)
	var width, height float32
	width = -1
	height = -1

	if node.ComputedStyle != nil {
		if node.ComputedStyle.Width != "" && node.ComputedStyle.Width != "auto" {
			width = parseLengthWithViewport(node.ComputedStyle.Width, fontSize, availableWidth, availableWidth, availableWidth)
		}
		if node.ComputedStyle.Height != "" && node.ComputedStyle.Height != "auto" {
			height = parseLengthWithViewport(node.ComputedStyle.Height, fontSize, availableWidth, availableWidth, availableWidth)
		}
	}

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

	if node.TagName == "img" && node.ImageData != nil && node.ImageData.State == imageloader.StateLoaded {
		if width < 0 {
			width = float32(node.ImageData.Width)
		}
		if height < 0 {
			height = float32(node.ImageData.Height)
		}
	}

	if width < 0 {
		width = fontSize * 5
	}
	if height < 0 {
		height = fontSize * 1.5
	}

	if (*currentLine).Width+width > (*currentLine).AvailableWidth && len((*currentLine).InlineBoxes) > 0 {
		ile.finalizeLine(*currentLine)
		*lines = append(*lines, *currentLine)

		textAlign := (*currentLine).TextAlign
		lineHeight := (*currentLine).LineHeight
		nextY := (*currentLine).Y + (*currentLine).Height
		newLineX, newWidth := ile.getLineXAndWidth(lineX, nextY, availableWidth, lineHeight)
		*currentLine = ile.newLineBox(newLineX, nextY, newWidth, textAlign, lineHeight)
	}

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

	if inlineBox.Ascent > (*currentLine).Ascent {
		(*currentLine).Ascent = inlineBox.Ascent
	}
	if inlineBox.Descent > (*currentLine).Descent {
		(*currentLine).Descent = inlineBox.Descent
	}
}

func (ile *InlineLayoutEngine) finalizeLine(line *LineBox) {
	if len(line.InlineBoxes) == 0 {
		return
	}

	// Coalesce inline boxes with same NodeID and 0 letter-spacing on same line
	if len(line.InlineBoxes) > 1 {
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

	lineHeight := line.Ascent + line.Descent

	for _, box := range line.InlineBoxes {
		if box.Height > lineHeight {
			lineHeight = box.Height
		}
	}

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

func (ile *InlineLayoutEngine) processWhiteSpace(text string, mode WhiteSpaceMode) string {
	switch mode {
	case WhiteSpaceNormal, WhiteSpaceNoWrap:
		return ile.collapseWhiteSpace(text)
	case WhiteSpacePre:
		return text
	case WhiteSpacePreWrap:
		return text
	case WhiteSpacePreLine:
		return ile.collapseWhiteSpacePreserveNewlines(text)
	default:
		return ile.collapseWhiteSpace(text)
	}
}

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

func (ile *InlineLayoutEngine) collapseWhiteSpacePreserveNewlines(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = ile.collapseWhiteSpace(line)
	}
	return strings.Join(lines, "\n")
}

func (ile *InlineLayoutEngine) splitTextForWrapping(text string, mode WhiteSpaceMode) []string {
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

func (ile *InlineLayoutEngine) getFontSizeForNode(node *RenderNode) float32 {
	if node == nil {
		return ile.defaultFontSize
	}

	if node.ComputedStyle != nil && node.ComputedStyle.FontSize > 0 {
		return node.ComputedStyle.FontSize
	}

	if node.Type == NodeTypeElement {
		return ile.fontMetrics.GetFontSize(node.TagName)
	}

	if node.Parent != nil {
		if node.Parent.TagName == "sub" || node.Parent.TagName == "sup" {
			base := ile.getFontSizeForNode(node.Parent.Parent)
			return base * 0.83
		}
		return ile.getFontSizeForNode(node.Parent)
	}

	return ile.defaultFontSize
}

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

func (ile *InlineLayoutEngine) isInlineBlock(node *RenderNode) bool {
	if node.ComputedStyle != nil {
		disp := node.ComputedStyle.Display
		if disp == css.DisplayAtomInlineBlock {
			return true
		}
	}
	switch node.TagName {
	case "img", "button", "input", "select":
		return true
	}
	return false
}
