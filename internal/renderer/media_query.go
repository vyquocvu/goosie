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

// aspectRatioRE parses ratio values like "16/9". Package-level so it is
// compiled once instead of per evaluation call.
var aspectRatioRE = regexp.MustCompile(`(\d+)\s*/\s*(\d+)`)

// evaluateFeature evaluates a media feature like "max-width: 600px" or the
// Media Queries Level 4 range forms "width <= 600px", "600px >= width", and
// the chained "400px <= width <= 700px".
func (mq *MediaQueryEvaluator) evaluateFeature(feature string) bool {
	feature = strings.TrimSpace(feature)
	if feature == "" {
		return false
	}

	for _, op := range rangeOperators {
		if strings.Contains(feature, op) {
			return mq.evaluateRangeFeature(feature)
		}
	}

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
	matches := aspectRatioRE.FindStringSubmatch(value)
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

// rangeOperators is ordered longest-first so "<=" is matched before "<".
var rangeOperators = []string{"<=", ">=", "<", ">", "="}

// evaluateRangeFeature evaluates the Media Queries Level 4 range forms:
//
//	width <= 600px        (feature op value)
//	600px >= width        (value op feature)
//	400px <= width <= 700px (chained)
//
// The middle operand of a chain (and exactly one operand of a simple form)
// must be a known media feature; the others are values.
func (mq *MediaQueryEvaluator) evaluateRangeFeature(expr string) bool {
	tokens, ok := splitRangeExpr(expr)
	if !ok {
		return false
	}

	if len(tokens) == 3 { // lhs op rhs
		lhs, op, rhs := tokens[0], tokens[1], tokens[2]
		lhsFeature, rhsFeature := mediaFeatureKind(lhs), mediaFeatureKind(rhs)
		if lhsFeature != featureNone && rhsFeature == featureNone {
			return mq.compareFeature(lhsFeature, op, rhs)
		}
		if lhsFeature == featureNone && rhsFeature != featureNone {
			return mq.compareFeature(rhsFeature, reverseRangeOp(op), lhs)
		}
		return false
	}

	// Chained: value op feature op value — the middle operand must be the
	// feature and both comparisons must hold.
	valueA, opA, mid, opB, valueB := tokens[0], tokens[1], tokens[2], tokens[3], tokens[4]
	midFeature := mediaFeatureKind(mid)
	if midFeature == featureNone {
		return false
	}
	return mq.compareFeature(midFeature, reverseRangeOp(opA), valueA) &&
		mq.compareFeature(midFeature, opB, valueB)
}

// splitRangeExpr tokenizes a range expression into alternating operands and
// operators: "width <= 600px" → [width <= 600px],
// "400px <= width <= 700px" → [400px <= width <= 700px].
// It reports ok=false for anything that is not a 2- or 3-comparison shape.
func splitRangeExpr(expr string) ([]string, bool) {
	var tokens []string
	start := 0
	for i := 0; i < len(expr); i++ {
		op := ""
		switch {
		case strings.HasPrefix(expr[i:], "<="):
			op = "<="
		case strings.HasPrefix(expr[i:], ">="):
			op = ">="
		case expr[i] == '<' || expr[i] == '>' || expr[i] == '=':
			op = string(expr[i])
		}
		if op == "" {
			continue
		}
		tokens = append(tokens, strings.TrimSpace(expr[start:i]), op)
		i += len(op) - 1
		start = i + 1
	}
	tokens = append(tokens, strings.TrimSpace(expr[start:]))

	if len(tokens) != 3 && len(tokens) != 5 {
		return nil, false
	}
	for i, t := range tokens {
		if i%2 == 0 && t == "" { // empty operand
			return nil, false
		}
	}
	return tokens, true
}

// mediaFeatureKind classifies an operand: a known numeric feature, the
// ratio feature, or not a feature (a value).
type featureKind int

const (
	featureNone featureKind = iota
	featureWidth
	featureHeight
	featureAspectRatio
)

func mediaFeatureKind(operand string) featureKind {
	switch strings.ToLower(strings.TrimSpace(operand)) {
	case "width", "min-width", "max-width":
		return featureWidth
	case "height", "min-height", "max-height":
		return featureHeight
	case "aspect-ratio", "min-aspect-ratio", "max-aspect-ratio":
		return featureAspectRatio
	}
	return featureNone
}

// reverseRangeOp mirrors an operator so a reversed form ("600px >= width")
// can be evaluated as "width <= 600px".
func reverseRangeOp(op string) string {
	switch op {
	case "<":
		return ">"
	case "<=":
		return ">="
	case ">":
		return "<"
	case ">=":
		return "<="
	}
	return op
}

// compareFeature compares the viewport's value of feature against val using
// the operator seen from the feature's perspective.
func (mq *MediaQueryEvaluator) compareFeature(kind featureKind, op string, val string) bool {
	val = strings.TrimSpace(val)
	var actual float32
	switch kind {
	case featureWidth:
		actual = mq.viewportWidth
	case featureHeight:
		actual = mq.viewportHeight
	case featureAspectRatio:
		actual = mq.viewportWidth / mq.viewportHeight
	default:
		return false
	}

	target, ok := mq.parseFeatureValue(val, kind)
	if !ok {
		return false
	}
	switch op {
	case "<":
		return actual < target
	case "<=":
		return actual <= target
	case ">":
		return actual > target
	case ">=":
		return actual >= target
	case "=":
		return actual == target
	}
	return false
}

// parseFeatureValue parses the value operand for a feature kind. Ratios
// ("16/9") only apply to aspect-ratio; everything else goes through
// parsePixelValue.
func (mq *MediaQueryEvaluator) parseFeatureValue(val string, kind featureKind) (float32, bool) {
	if kind == featureAspectRatio {
		if w, h, ok := parseAspectRatioValue(val); ok {
			return w / h, true
		}
		return 0, false
	}
	// A feature name on the value side ("width <= height") is invalid.
	if mediaFeatureKind(val) != featureNone {
		return 0, false
	}
	return mq.parsePixelValue(val), true
}

// parseAspectRatioValue parses "16/9" into its components.
func parseAspectRatioValue(val string) (float32, float32, bool) {
	matches := aspectRatioRE.FindStringSubmatch(val)
	if len(matches) != 3 {
		return 0, 0, false
	}
	w, errW := strconv.ParseFloat(matches[1], 32)
	h, errH := strconv.ParseFloat(matches[2], 32)
	if errW != nil || errH != nil || h == 0 {
		return 0, 0, false
	}
	return float32(w), float32(h), true
}

// UpdateViewport updates the viewport dimensions
func (mq *MediaQueryEvaluator) UpdateViewport(width, height float32) {
	mq.viewportWidth = width
	mq.viewportHeight = height
}
