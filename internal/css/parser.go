package css

import (
	"strconv"
	"strings"
)

// mediaViewportWidth is the CSS px width @media min/max-width conditions are
// tested against. The engine sets it per document; the default is the
// desktop viewport the parity references are captured at.
var mediaViewportWidth float32 = 1280

// mediaDevicePixelRatio is the resolution @media device-pixel-ratio conditions
// are tested against. Rendering is one device pixel per CSS pixel: no backend
// scales the frame, and the parity references are captured at dpr 1.
const mediaDevicePixelRatio float32 = 1

// SetMediaViewportWidth points media-query evaluation at a viewport width.
func SetMediaViewportWidth(w float32) {
	if w > 0 {
		mediaViewportWidth = w
	}
}

// Declaration is one CSS property-value pair.
type Declaration struct {
	Property  string
	Value     string
	Parsed    Value
	Important bool
}

// Rule is one CSS rule: selectors + declarations.
type Rule struct {
	Selectors    []Selector
	SelectorStrs []string
	Declarations []Declaration
}

// FontFaceRule is one @font-face rule: declarations only, no selectors.
type FontFaceRule struct {
	Declarations []Declaration
}

// Stylesheet is a parsed CSS stylesheet.
type Stylesheet struct {
	Rules      []Rule
	FontFaces  []FontFaceRule
}

// Parse parses a CSS stylesheet.
func Parse(input string) *Stylesheet {
	p := &parser{input: stripComments(input)}
	return p.parse()
}

// stripComments removes /* ... */ comment blocks from CSS source. A comment
// cannot start or continue inside a quoted string, so the scan tracks quote
// state and leaves anything between a real quote pair untouched, including a
// `/*` that appears inside url("...").
func stripComments(in string) string {
	if !strings.Contains(in, "/*") {
		return in
	}
	var b strings.Builder
	b.Grow(len(in))
	for i := 0; i < len(in); {
		c := in[i]
		if c == '\'' || c == '"' {
			quote := c
			b.WriteByte(c)
			i++
			for i < len(in) {
				if in[i] == '\\' && i+1 < len(in) {
					b.WriteByte(in[i])
					b.WriteByte(in[i+1])
					i += 2
					continue
				}
				b.WriteByte(in[i])
				if in[i] == quote {
					i++
					break
				}
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(in) && in[i+1] == '*' {
			end := strings.Index(in[i+2:], "*/")
			if end < 0 {
				// Unterminated comment swallows the rest of the sheet.
				break
			}
			i += 2 + end + 2
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

type parser struct {
	input string
	pos   int
}

func (p *parser) parse() *Stylesheet {
	sheet := &Stylesheet{}
	p.parseInto(sheet)
	return sheet
}

// parseInto runs the top-level rule loop against a caller-owned sheet so that
// an @media body can recurse into the same accumulation (Media Merge semantics).
func (p *parser) parseInto(sheet *Stylesheet) {
	for p.pos < len(p.input) {
		p.skipWhitespace()
		if p.pos >= len(p.input) {
			break
		}
		if p.input[p.pos] == '@' {
			switch p.atRuleName() {
			case "media":
				p.parseMediaRule(sheet)
			case "supports":
				p.parseSupportsRule(sheet)
			case "font-face":
				p.parseFontFaceRule(sheet)
			default:
				p.skipAtRule()
			}
			continue
		}
		rule := p.parseRule()
		if len(rule.Selectors) > 0 {
			sheet.Rules = append(sheet.Rules, rule)
		}
	}
}

// atRuleName reads an @rule name without consuming past its prelude, so the
// caller can decide whether the block is entered (media) or skipped.
func (p *parser) atRuleName() string {
	save := p.pos
	p.pos++ // '@'
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == '{' || c == ';' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			break
		}
		p.pos++
	}
	name := strings.ToLower(p.input[save+1 : p.pos])
	p.pos = save
	return name
}

// parseMediaRule tests an @media condition against the viewport the parser
// package was told about (SetMediaViewportWidth; 1280 by default) and splices
// true bodies into the sheet in place. Conditions on features this engine does
// not vary with - prefers-*, resolution, pointer - read as true, which matches
// a desktop screen far more often than false does.
func (p *parser) parseMediaRule(sheet *Stylesheet) {
	at := p.pos
	p.pos++ // '@'
	for p.pos < len(p.input) && p.input[p.pos] != '{' && p.input[p.pos] != ';' {
		p.pos++
	}
	if p.pos >= len(p.input) {
		return
	}
	if p.input[p.pos] == ';' {
		p.pos++
		return
	}
	cond := strings.TrimSpace(p.input[at:p.pos])
	if sp := strings.IndexByte(cond, ' '); sp >= 0 {
		cond = strings.TrimSpace(cond[sp+1:])
	} else {
		cond = strings.TrimSpace(strings.TrimPrefix(cond, "@media"))
	}
	p.pos++ // the '{'
	depth := 1
	bodyStart := p.pos
	for p.pos < len(p.input) && depth > 0 {
		switch p.input[p.pos] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				break
			}
		}
		if depth == 0 {
			break
		}
		p.pos++
	}
	body := p.input[bodyStart:p.pos]
	if p.pos < len(p.input) {
		p.pos++ // the final '}'
	}
	if mediaConditionTrue(cond) {
		inner := &parser{input: body}
		inner.parseInto(sheet)
	}
}

// parseSupportsRule reads an @supports block and splices its body into the sheet
// only when the condition tests a feature this engine actually lays out with, so
// a progressive-enhancement rule (cards that only go flex once grid works) matches
// the browser reference rather than silently dropping. Conditions we cannot judge
// - selector(), an unknown property, a feature we lack - read as false and skip.
func (p *parser) parseSupportsRule(sheet *Stylesheet) {
	at := p.pos
	p.pos++ // '@'
	for p.pos < len(p.input) && p.input[p.pos] != '{' && p.input[p.pos] != ';' {
		p.pos++
	}
	if p.pos >= len(p.input) {
		return
	}
	if p.input[p.pos] == ';' {
		p.pos++
		return
	}
	cond := strings.TrimSpace(p.input[at:p.pos])
	if sp := strings.IndexByte(cond, ' '); sp >= 0 {
		cond = strings.TrimSpace(cond[sp+1:])
	} else {
		cond = strings.TrimSpace(strings.TrimPrefix(cond, "@supports"))
	}
	p.pos++ // the '{'
	bodyStart := p.pos
	depth := 1
	for p.pos < len(p.input) && depth > 0 {
		switch p.input[p.pos] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				break
			}
		}
		if depth == 0 {
			break
		}
		p.pos++
	}
	body := p.input[bodyStart:p.pos]
	if p.pos < len(p.input) {
		p.pos++ // the final '}'
	}
	if supportsConditionTrue(cond) {
		inner := &parser{input: body}
		inner.parseInto(sheet)
	}
}

// parseFontFaceRule reads an @font-face block and stores its declarations.
// Font-face rules have no selectors, only properties like font-family, src,
// font-weight, and font-style.
func (p *parser) parseFontFaceRule(sheet *Stylesheet) {
	p.pos++ // '@'
	for p.pos < len(p.input) && p.input[p.pos] != '{' && p.input[p.pos] != ';' {
		p.pos++
	}
	if p.pos >= len(p.input) {
		return
	}
	if p.input[p.pos] == ';' {
		p.pos++
		return
	}
	p.pos++ // the '{'
	bodyStart := p.pos
	depth := 1
	for p.pos < len(p.input) && depth > 0 {
		switch p.input[p.pos] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				break
			}
		}
		if depth == 0 {
			break
		}
		p.pos++
	}
	body := p.input[bodyStart:p.pos]
	if p.pos < len(p.input) {
		p.pos++ // the final '}'
	}
	// Parse the declarations inside the font-face block.
	decls := parseDeclarations(body)
	if len(decls) > 0 {
		sheet.FontFaces = append(sheet.FontFaces, FontFaceRule{Declarations: decls})
	}
}

// supportsConditionTrue evaluates a CSS @supports condition: an optional leading
// `not`, and the boolean operators `or` (lowest precedence) then `and`, over
// parenthesized `<prop>: <value>` tests.
func supportsConditionTrue(cond string) bool {
	cond = strings.TrimSpace(strings.ToLower(cond))
	if strings.HasPrefix(cond, "not") {
		return !supportsConditionTrue(strings.TrimSpace(strings.TrimPrefix(cond, "not")))
	}
	if groups := splitOutsideParens(cond, " or "); len(groups) > 1 {
		for _, g := range groups {
			if supportsConditionTrue(g) {
				return true
			}
		}
		return false
	}
	if groups := splitOutsideParens(cond, " and "); len(groups) > 1 {
		for _, g := range groups {
			if !supportsConditionTrue(g) {
				return false
			}
		}
		return true
	}
	return supportsFeatureTrue(cond)
}

// supportsFeatureTrue judges a single `(prop: value)` test against what the engine
// implements. Only `display` is consulted; every other property - or a selector
// test - reads false so an @supports block gated on it stays out.
func supportsFeatureTrue(test string) bool {
	test = strings.TrimSpace(test)
	if !strings.HasPrefix(test, "(") || !strings.HasSuffix(test, ")") {
		return false
	}
	inner := strings.TrimSpace(test[1 : len(test)-1])
	colon := strings.IndexByte(inner, ':')
	if colon < 0 {
		return false
	}
	prop := strings.TrimSpace(inner[:colon])
	value := strings.TrimSpace(inner[colon+1:])
	if prop != "display" {
		return false
	}
	switch value {
	case "grid", "inline-grid", "flex", "inline-flex", "subgrid":
		// subgrid is named for completeness but the engine has no subgrid layout,
		// so it is reported as unsupported alongside anything absent here.
		return value != "subgrid"
	case "block", "inline-block", "table", "table-row", "table-cell", "table-row-group":
		return true
	}
	return false
}

// splitOutsideParens splits s on sep occurrences that sit at paren depth zero,
// so `or`/`and` inside a `(...)` test are not treated as operators.
func splitOutsideParens(s, sep string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); {
		switch s[i] {
		case '(':
			depth++
			i++
		case ')':
			if depth > 0 {
				depth--
			}
			i++
		default:
			if depth == 0 && i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
				out = append(out, strings.TrimSpace(s[start:i]))
				i += len(sep)
				start = i
				continue
			}
			i++
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// mediaConditionTrue reports whether any comma-separated group applies.
func mediaConditionTrue(cond string) bool {
	for _, group := range splitTopLevel(cond, ',') {
		if mediaGroupTrue(group) {
			return true
		}
	}
	return false
}

func mediaGroupTrue(group string) bool {
	group = strings.TrimSpace(strings.ToLower(group))
	if group == "" || group == "all" {
		return true
	}
	negate := false
	if strings.HasPrefix(group, "not") {
		negate = true
		group = strings.TrimSpace(strings.TrimPrefix(group, "not"))
	}
	result := mediaPartsTrue(group)
	if negate {
		return !result
	}
	return result
}

func mediaPartsTrue(group string) bool {
	// `or` binds looser than the implicit `and`, so a group of alternatives is
	// true when any one of them is. Reading every connector as `and` made
	// `(A) or (B)` require both, which is how a mobile-only rule survived onto a
	// desktop viewport.
	if alts := splitOutsideParens(group, " or "); len(alts) > 1 {
		for _, a := range alts {
			if mediaPartsTrue(strings.TrimSpace(a)) {
				return true
			}
		}
		return false
	}
	for _, part := range splitTopLevel(group, ' ') {
		// Media Queries 4 range syntax - `(width <= 769px)`, `(400px < width)` -
		// carries no colon, so the feature switch below cannot see it.
		if holds, ok := mediaRangeHolds(part); ok {
			if !holds {
				return false
			}
			continue
		}
		part = strings.TrimSpace(part)
		if part == "" || part == "and" || part == "only" {
			continue
		}
		// A bare media type: anything but print applies on screen.
		if !strings.Contains(part, "(") {
			if part == "print" {
				return false
			}
			continue
		}
		name, val, ok := mediaFeature(part)
		if !ok {
			continue
		}
		switch name {
		case "min-width", "min-device-width":
			if px, ok := cssPx(val); ok && mediaViewportWidth < px {
				return false
			}
		case "max-width", "max-device-width":
			if px, ok := cssPx(val); ok && mediaViewportWidth > px {
				return false
			}
		case "orientation":
			if val == "portrait" {
				return false
			}
		case "prefers-color-scheme":
			// The engine has no dark palette and the references are captured in a
			// light Chromium, so `dark` asks for a look neither one paints.
			if val == "dark" {
				return false
			}
		case "prefers-reduced-motion":
			if val == "reduce" {
				return false
			}
		case "prefers-contrast":
			// A bare `(prefers-contrast)` only asks whether the feature exists, and
			// it does; only a non-default value asks for one we do not render.
			if val != "" && val != "no-preference" {
				return false
			}
		case "min-device-pixel-ratio", "-webkit-min-device-pixel-ratio":
			if f, err := strconv.ParseFloat(val, 32); err == nil && mediaDevicePixelRatio < float32(f) {
				return false
			}
		}
	}
	return true
}

// cssPx resolves the width units media queries actually use.
func cssPx(v string) (float32, bool) {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "calc(") && strings.HasSuffix(v, ")") {
		return calcPx(v[len("calc(") : len(v)-1])
	}
	switch {
	case strings.HasSuffix(v, "px"):
		f, err := strconv.ParseFloat(strings.TrimSuffix(v, "px"), 32)
		return float32(f), err == nil
	case strings.HasSuffix(v, "rem"), strings.HasSuffix(v, "em"):
		unit := "rem"
		if strings.HasSuffix(v, "em") && !strings.HasSuffix(v, "rem") {
			unit = "em"
		}
		f, err := strconv.ParseFloat(strings.TrimSuffix(v, unit), 32)
		return float32(f) * 16, err == nil
	}
	return 0, false
}

func mediaFeature(part string) (name, value string, ok bool) {
	if !strings.HasPrefix(part, "(") || !strings.HasSuffix(part, ")") {
		return "", "", false
	}
	inner := part[1 : len(part)-1]
	if colon := strings.Index(inner, ":"); colon >= 0 {
		return strings.TrimSpace(inner[:colon]), strings.TrimSpace(inner[colon+1:]), true
	}
	return strings.TrimSpace(inner), "", true
}

// calcPx evaluates the arithmetic media queries use: a sum of lengths, each
// optionally scaled by a plain number (`1rem * 2`). No function other than calc
// itself is resolved, and anything unparseable fails the whole expression - a
// media feature the engine cannot measure must not read as satisfied.
func calcPx(expr string) (float32, bool) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return 0, false
	}
	// Split on the top-level + and - that separate terms, keeping the sign.
	var terms []string
	start := 0
	depth := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '+', '-':
			if depth == 0 && i > start {
				terms = append(terms, expr[start:i])
				start = i
			}
		}
	}
	terms = append(terms, expr[start:])
	total := float32(0)
	for _, t := range terms {
		neg := strings.HasPrefix(t, "-")
		if neg || strings.HasPrefix(t, "+") {
			t = t[1:]
		}
		px, ok := productPx(t)
		if !ok {
			return 0, false
		}
		if neg {
			total -= px
		} else {
			total += px
		}
	}
	return total, true
}

// productPx reads `LENGTH * N` or `N * LENGTH`, the only product a media query
// can legally contain. A calc with no length term is not a length.
func productPx(term string) (float32, bool) {
	scale := float32(1)
	base := float32(-1)
	for _, f := range strings.Split(term, "*") {
		if n, err := strconv.ParseFloat(f, 32); err == nil {
			scale *= float32(n)
			continue
		}
		px, ok := cssPx(f)
		if !ok || base >= 0 {
			return 0, false
		}
		base = px
	}
	if base < 0 {
		return 0, false
	}
	return base * scale, true
}

// splitRange breaks a media range expression on its comparison operators,
// keeping each operand verbatim so a `calc(1rem * 2 + 15rem)` with internal
// spaces survives as one operand instead of shattering into words.
func splitRange(inner string) (operands, ops []string) {
	start, depth := 0, 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '(':
			depth++
		case ')':
			depth--
		case '<', '>':
			if depth != 0 {
				continue
			}
			operands = append(operands, inner[start:i])
			k := i
			for k < len(inner) && (inner[k] == '<' || inner[k] == '>' || inner[k] == '=') {
				k++
			}
			ops = append(ops, inner[i:k])
			start = k
			i = k - 1
		}
	}
	return append(operands, inner[start:]), ops
}

// mediaRangeHolds evaluates a Media Queries 4 range expression - `(width <=
// 769px)`, `(769px > width)`, `(400px <= width < 1200px)` - against the viewport
// width. ok=false means "not a width range", leaving the caller to the classic
// feature handling. The strict operators compare inclusively: the difference is
// one pixel exactly at the breakpoint, and the engine has no sub-pixel viewport.
func mediaRangeHolds(part string) (holds bool, ok bool) {
	if !strings.HasPrefix(part, "(") || !strings.HasSuffix(part, ")") {
		return false, false
	}
	operands, ops := splitRange(part[1 : len(part)-1])
	for i := range operands {
		operands[i] = strings.TrimSpace(operands[i])
	}
	switch len(operands) {
	case 2:
		if operands[0] == "width" {
			px, isLen := cssPx(operands[1])
			return isLen && widthSatisfies(ops[0], px), isLen
		}
		if operands[1] == "width" {
			px, isLen := cssPx(operands[0])
			return isLen && widthSatisfies(flipOp(ops[0]), px), isLen
		}
	case 3:
		if operands[1] != "width" {
			return false, false
		}
		lo, okLo := cssPx(operands[0])
		hi, okHi := cssPx(operands[2])
		if !okLo || !okHi {
			return false, false
		}
		return widthSatisfies(ops[0], lo) && widthSatisfies(ops[1], hi), true
	}
	return false, false
}

func widthSatisfies(op string, px float32) bool {
	switch op {
	case "<", "<=":
		return mediaViewportWidth <= px
	case ">", ">=":
		return mediaViewportWidth >= px
	}
	return false
}

func flipOp(op string) string {
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

// splitTopLevel splits on sep outside parentheses and brackets.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[':
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
			}
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

func (p *parser) skipWhitespace() {
	for p.pos < len(p.input) {
		if p.input[p.pos] == ' ' || p.input[p.pos] == '\t' ||
			p.input[p.pos] == '\n' || p.input[p.pos] == '\r' {
			p.pos++
			continue
		}
		if p.pos+1 < len(p.input) && p.input[p.pos] == '/' && p.input[p.pos+1] == '*' {
			p.pos += 2
			for p.pos+1 < len(p.input) {
				if p.input[p.pos] == '*' && p.input[p.pos+1] == '/' {
					p.pos += 2
					break
				}
				p.pos++
			}
			continue
		}
		break
	}
}

func (p *parser) skipAtRule() {
	p.pos++
	for p.pos < len(p.input) && p.input[p.pos] != ';' && p.input[p.pos] != '{' {
		p.pos++
	}
	if p.pos < len(p.input) && p.input[p.pos] == '{' {
		depth := 1
		p.pos++
		for p.pos < len(p.input) && depth > 0 {
			if p.input[p.pos] == '{' {
				depth++
			} else if p.input[p.pos] == '}' {
				depth--
			}
			p.pos++
		}
	} else if p.pos < len(p.input) {
		p.pos++
	}
}

func (p *parser) parseRule() Rule {
	var rule Rule

	selectorStart := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '{' {
		p.pos++
	}
	selectorText := strings.TrimSpace(p.input[selectorStart:p.pos])

	if p.pos >= len(p.input) {
		return rule
	}
	p.pos++

	p.skipWhitespace()
	declStart := p.pos
	depth := 0
	for p.pos < len(p.input) {
		if p.input[p.pos] == '{' {
			depth++
		} else if p.input[p.pos] == '}' {
			if depth == 0 {
				break
			}
			depth--
		}
		p.pos++
	}
	declText := p.input[declStart:p.pos]
	if p.pos < len(p.input) {
		p.pos++
	}

	for _, sel := range splitSelectors(selectorText) {
		sel = strings.TrimSpace(sel)
		if sel != "" {
			rule.Selectors = append(rule.Selectors, ParseSelector(sel))
			rule.SelectorStrs = append(rule.SelectorStrs, sel)
		}
	}

	rule.Declarations = parseDeclarations(declText)
	return rule
}

func splitSelectors(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func parseDeclarations(s string) []Declaration {
	var decls []Declaration
	for _, part := range splitDeclarations(s) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colonIdx := strings.Index(part, ":")
		if colonIdx < 0 {
			continue
		}
		prop := strings.TrimSpace(part[:colonIdx])
		val := strings.TrimSpace(part[colonIdx+1:])

		important := false
		if idx := strings.Index(strings.ToLower(val), "!important"); idx >= 0 {
			important = true
			val = strings.TrimSpace(val[:idx])
		}

		decls = append(decls, Declaration{
			Property:  strings.ToLower(prop),
			Value:     val,
			Parsed:    ParseValue(val),
			Important: important,
		})
	}
	return decls
}

func splitDeclarations(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ';':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

// Merge merges two stylesheets, appending rules from other after those in the receiver.
func (s *Stylesheet) Merge(other *Stylesheet) {
	if other == nil {
		return
	}
	s.Rules = append(s.Rules, other.Rules...)
}
