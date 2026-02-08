package renderer

import (
	"regexp"
	"strconv"
	"strings"
)

// MediaQueryEvaluator evaluates CSS media queries against viewport dimensions
type MediaQueryEvaluator struct {
	viewportWidth  float32
	viewportHeight float32
}

// NewMediaQueryEvaluator creates a new media query evaluator
func NewMediaQueryEvaluator(width, height float32) *MediaQueryEvaluator {
	return &MediaQueryEvaluator{
		viewportWidth:  width,
		viewportHeight: height,
	}
}

// Evaluate checks if a media query prelude is satisfied
// Supports: screen, print, all, (min-width: X), (max-width: X), (min-height: X), (max-height: X)
func (mq *MediaQueryEvaluator) Evaluate(prelude string) bool {
	prelude = strings.TrimSpace(prelude)
	if prelude == "" {
		return true
	}

	// Split by comma for OR conditions (media query list)
	queries := strings.Split(prelude, ",")
	for _, query := range queries {
		if mq.evaluateSingleQuery(strings.TrimSpace(query)) {
			return true
		}
	}
	return false
}

// evaluateSingleQuery evaluates a single media query (no commas)
func (mq *MediaQueryEvaluator) evaluateSingleQuery(query string) bool {
	// Handle negation
	if strings.HasPrefix(query, "not ") {
		return !mq.evaluateSingleQuery(strings.TrimPrefix(query, "not "))
	}

	// Split by "and" for AND conditions
	parts := strings.Split(query, " and ")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !mq.evaluateCondition(part) {
			return false
		}
	}
	return true
}

// evaluateCondition evaluates a single condition
func (mq *MediaQueryEvaluator) evaluateCondition(condition string) bool {
	condition = strings.TrimSpace(condition)

	// Media types
	switch strings.ToLower(condition) {
	case "all":
		return true
	case "screen":
		return true // We're always rendering to screen
	case "print":
		return false // We don't support print media
	}

	// Parenthesized conditions like (max-width: 600px)
	if strings.HasPrefix(condition, "(") && strings.HasSuffix(condition, ")") {
		inner := condition[1 : len(condition)-1]
		return mq.evaluateFeature(inner)
	}

	// Unknown condition - be permissive
	return true
}

// evaluateFeature evaluates a media feature like "max-width: 600px"
func (mq *MediaQueryEvaluator) evaluateFeature(feature string) bool {
	parts := strings.SplitN(feature, ":", 2)
	if len(parts) != 2 {
		return false
	}

	name := strings.TrimSpace(strings.ToLower(parts[0]))
	value := strings.TrimSpace(parts[1])
	pixels := mq.parsePixelValue(value)

	switch name {
	case "min-width":
		return mq.viewportWidth >= pixels
	case "max-width":
		return mq.viewportWidth <= pixels
	case "min-height":
		return mq.viewportHeight >= pixels
	case "max-height":
		return mq.viewportHeight <= pixels
	case "width":
		return mq.viewportWidth == pixels
	case "height":
		return mq.viewportHeight == pixels
	case "orientation":
		if strings.ToLower(value) == "portrait" {
			return mq.viewportHeight > mq.viewportWidth
		} else if strings.ToLower(value) == "landscape" {
			return mq.viewportWidth >= mq.viewportHeight
		}
		return false
	case "aspect-ratio", "min-aspect-ratio", "max-aspect-ratio":
		return mq.evaluateAspectRatio(name, value)
	}

	return false
}

// parsePixelValue parses a CSS length value to pixels
func (mq *MediaQueryEvaluator) parsePixelValue(value string) float32 {
	value = strings.TrimSpace(strings.ToLower(value))

	// Handle common units
	if strings.HasSuffix(value, "px") {
		numStr := strings.TrimSuffix(value, "px")
		if num, err := strconv.ParseFloat(numStr, 32); err == nil {
			return float32(num)
		}
	} else if strings.HasSuffix(value, "em") {
		numStr := strings.TrimSuffix(value, "em")
		if num, err := strconv.ParseFloat(numStr, 32); err == nil {
			return float32(num) * 16 // Assume 16px base font size
		}
	} else if strings.HasSuffix(value, "rem") {
		numStr := strings.TrimSuffix(value, "rem")
		if num, err := strconv.ParseFloat(numStr, 32); err == nil {
			return float32(num) * 16 // Assume 16px root font size
		}
	} else {
		// Try parsing as plain number (assume pixels)
		if num, err := strconv.ParseFloat(value, 32); err == nil {
			return float32(num)
		}
	}

	return 0
}

// evaluateAspectRatio evaluates aspect ratio media features
func (mq *MediaQueryEvaluator) evaluateAspectRatio(name, value string) bool {
	// Parse aspect ratio like "16/9" or "4/3"
	re := regexp.MustCompile(`(\d+)\s*/\s*(\d+)`)
	matches := re.FindStringSubmatch(value)
	if len(matches) != 3 {
		return false
	}

	w, _ := strconv.ParseFloat(matches[1], 32)
	h, _ := strconv.ParseFloat(matches[2], 32)
	if h == 0 {
		return false
	}

	targetRatio := float32(w / h)
	actualRatio := mq.viewportWidth / mq.viewportHeight

	switch name {
	case "aspect-ratio":
		return actualRatio == targetRatio
	case "min-aspect-ratio":
		return actualRatio >= targetRatio
	case "max-aspect-ratio":
		return actualRatio <= targetRatio
	}

	return false
}

// UpdateViewport updates the viewport dimensions
func (mq *MediaQueryEvaluator) UpdateViewport(width, height float32) {
	mq.viewportWidth = width
	mq.viewportHeight = height
}
