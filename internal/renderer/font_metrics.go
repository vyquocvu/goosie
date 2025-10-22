package renderer

import (
	"fyne.io/fyne/v2"
)

// FontMetrics provides accurate text measurement using font metrics
type FontMetrics struct {
	defaultFontSize float32
	// Cache for Fyne app availability
	fyneAvailable bool
	checkedFyne   bool
}

// NewFontMetrics creates a new FontMetrics instance
func NewFontMetrics(defaultSize float32) *FontMetrics {
	return &FontMetrics{
		defaultFontSize: defaultSize,
		fyneAvailable:   false,
		checkedFyne:     false,
	}
}

// TextMetrics represents the measured dimensions of text
type TextMetrics struct {
	Width   float32
	Height  float32
	Ascent  float32
	Descent float32
}

// MeasureText measures text using actual font metrics
// Returns accurate width, height, ascent, and descent values
func (fm *FontMetrics) MeasureText(text string, fontSize float32, style fyne.TextStyle) TextMetrics {
	if text == "" {
		return TextMetrics{}
	}
	
	// Try to use Fyne's MeasureText if available (runtime environment)
	if !fm.checkedFyne {
		fm.fyneAvailable = isFyneAppAvailable()
		fm.checkedFyne = true
	}
	
	var width, height float32
	
	if fm.fyneAvailable {
		// Use Fyne's accurate measurement when app is running
		size := fyne.MeasureText(text, fontSize, style)
		width = size.Width
		height = size.Height
	} else {
		// Fallback to improved estimation for tests
		width = fm.estimateTextWidth(text, fontSize, style)
		height = fontSize * 1.2 // Line height with spacing
	}
	
	// Calculate ascent and descent based on font metrics
	// For most fonts, ascent is about 75-80% of font size, descent is about 20-25%
	ascent := fontSize * 0.75
	descent := fontSize * 0.25
	
	return TextMetrics{
		Width:   width,
		Height:  height,
		Ascent:  ascent,
		Descent: descent,
	}
}

// estimateTextWidth provides a better estimation for text width
// Uses character-specific widths for improved accuracy
func (fm *FontMetrics) estimateTextWidth(text string, fontSize float32, style fyne.TextStyle) float32 {
	if text == "" {
		return 0
	}
	
	// Base character width (average for proportional fonts)
	baseCharWidth := fontSize * 0.5
	
	// Adjust for font style
	if style.Bold {
		baseCharWidth *= 1.1 // Bold is slightly wider
	}
	if style.Monospace {
		baseCharWidth = fontSize * 0.6 // Monospace has uniform width
	}
	
	// Simple estimation: count characters and multiply by average width
	// This is more accurate than using a fixed multiplier
	totalWidth := float32(0)
	for _, ch := range text {
		charWidth := baseCharWidth
		
		// Adjust for specific character types
		switch {
		case ch >= 'A' && ch <= 'Z':
			// Uppercase letters are wider
			charWidth *= 1.2
		case ch >= 'a' && ch <= 'z':
			// Lowercase letters
			charWidth *= 1.0
		case ch >= '0' && ch <= '9':
			// Numbers
			charWidth *= 1.0
		case ch == ' ':
			// Space
			charWidth = fontSize * 0.25
		case ch == 'i' || ch == 'l' || ch == 'I':
			// Narrow letters
			charWidth *= 0.5
		case ch == 'm' || ch == 'w' || ch == 'M' || ch == 'W':
			// Wide letters
			charWidth *= 1.3
		default:
			// Other characters (punctuation, etc.)
			charWidth *= 0.8
		}
		
		totalWidth += charWidth
	}
	
	return totalWidth
}

// isFyneAppAvailable checks if Fyne app is available
func isFyneAppAvailable() bool {
	defer func() {
		if r := recover(); r != nil {
			// Fyne panicked, app not available
		}
	}()
	
	// Try to get current app - if it fails, Fyne is not initialized
	app := fyne.CurrentApp()
	return app != nil
}

// MeasureTextWithWrapping measures text with word wrapping
// Returns the dimensions when text is wrapped to fit within maxWidth
func (fm *FontMetrics) MeasureTextWithWrapping(text string, fontSize float32, style fyne.TextStyle, maxWidth float32) TextMetrics {
	if text == "" {
		return TextMetrics{}
	}
	
	// Measure single line
	singleLine := fm.MeasureText(text, fontSize, style)
	
	// If text fits on one line, return as-is
	if singleLine.Width <= maxWidth {
		return singleLine
	}
	
	// Calculate number of lines needed using word wrapping
	words := splitIntoWords(text)
	if len(words) == 0 {
		return TextMetrics{}
	}
	
	lines := []string{""}
	currentLine := 0
	
	for _, word := range words {
		// Try adding word to current line
		testLine := lines[currentLine]
		if testLine != "" {
			testLine += " "
		}
		testLine += word
		
		testMetrics := fm.MeasureText(testLine, fontSize, style)
		
		if testMetrics.Width <= maxWidth {
			// Word fits on current line
			lines[currentLine] = testLine
		} else {
			// Word doesn't fit, start new line
			if lines[currentLine] == "" {
				// Word is too long for a single line, break it
				lines[currentLine] = word
			} else {
				// Start new line with this word
				lines = append(lines, word)
				currentLine++
			}
		}
	}
	
	// Calculate total height (number of lines * line height)
	lineHeight := singleLine.Height
	if lineHeight == 0 {
		// Fallback to font size if height measurement is zero
		lineHeight = fontSize * 1.2
	}
	
	totalHeight := float32(len(lines)) * lineHeight
	
	// Width is the maximum of all line widths
	maxLineWidth := float32(0)
	for _, line := range lines {
		if line != "" {
			lineMetrics := fm.MeasureText(line, fontSize, style)
			if lineMetrics.Width > maxLineWidth {
				maxLineWidth = lineMetrics.Width
			}
		}
	}
	
	return TextMetrics{
		Width:   maxLineWidth,
		Height:  totalHeight,
		Ascent:  singleLine.Ascent,
		Descent: singleLine.Descent,
	}
}

// GetFontSize returns the appropriate font size for a given HTML element
func (fm *FontMetrics) GetFontSize(tagName string) float32 {
	fontSizes := map[string]float32{
		"h1": fm.defaultFontSize * 2.0,
		"h2": fm.defaultFontSize * 1.5,
		"h3": fm.defaultFontSize * 1.17,
		"h4": fm.defaultFontSize * 1.0,
		"h5": fm.defaultFontSize * 0.83,
		"h6": fm.defaultFontSize * 0.67,
		"p":  fm.defaultFontSize,
	}
	
	if size, ok := fontSizes[tagName]; ok {
		return size
	}
	return fm.defaultFontSize
}

// GetTextStyle returns the text style for a given HTML element
func (fm *FontMetrics) GetTextStyle(tagName string) fyne.TextStyle {
	switch tagName {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return fyne.TextStyle{Bold: true}
	case "strong", "b":
		return fyne.TextStyle{Bold: true}
	case "em", "i":
		return fyne.TextStyle{Italic: true}
	case "code", "pre":
		return fyne.TextStyle{Monospace: true}
	default:
		return fyne.TextStyle{}
	}
}

// GetTextStyleFromNode returns the text style based on the node and its parents
func (fm *FontMetrics) GetTextStyleFromNode(node *RenderNode) fyne.TextStyle {
	style := fyne.TextStyle{}
	
	// Traverse up the tree to collect style properties
	current := node
	for current != nil {
		switch current.TagName {
		case "h1", "h2", "h3", "h4", "h5", "h6", "strong", "b":
			style.Bold = true
		case "em", "i":
			style.Italic = true
		case "code", "pre":
			style.Monospace = true
		}
		current = current.Parent
	}
	
	return style
}

// splitIntoWords splits text into words for wrapping
func splitIntoWords(text string) []string {
	words := []string{}
	currentWord := ""
	
	for _, ch := range text {
		if ch == ' ' || ch == '\t' || ch == '\n' {
			if currentWord != "" {
				words = append(words, currentWord)
				currentWord = ""
			}
		} else {
			currentWord += string(ch)
		}
	}
	
	if currentWord != "" {
		words = append(words, currentWord)
	}
	
	return words
}
