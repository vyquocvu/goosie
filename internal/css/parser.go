package css

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

const defaultMediaViewportWidth float32 = 1280

const mediaDevicePixelRatio float32 = 1

type Declaration struct {
	Property  string
	Value     string
	Parsed    Value
	Important bool
}

type Rule struct {
	Selectors      []Selector
	SelectorStrs   []string
	Declarations   []Declaration
	Layer          string
	ContainerQuery string
	ScopeRoot      string
	ScopeLimit     string
	StartingStyle  bool
}

type FontFaceRule struct {
	Declarations []Declaration
}

type FontPaletteValuesRule struct {
	Name           string
	FontFamily     string
	BasePalette    string
	OverrideColors map[int]string
}

type Stylesheet struct {
	Rules        []Rule
	FontFaces    []FontFaceRule
	Keyframes    []KeyframesRule
	Layers       []string
	Properties   []dom.PropertyDefinition
	FontPalettes []FontPaletteValuesRule
}

type MediaOptions struct {
	ViewportWidth float32
	DarkMode      bool
	HighContrast  bool
}

func Parse(input string) *Stylesheet {
	return ParseForViewport(input, defaultMediaViewportWidth)
}

func ParseWithOptions(input string, opts MediaOptions) *Stylesheet {
	vw := opts.ViewportWidth
	if vw <= 0 {
		vw = defaultMediaViewportWidth
	}
	p := &parser{
		input:        stripComments(stripCDATAMarkers(input)),
		mediaWidth:   vw,
		darkMode:     opts.DarkMode,
		highContrast: opts.HighContrast,
	}
	return p.parse()
}

func ParseForViewport(input string, viewportWidthPx float32) *Stylesheet {
	return ParseWithOptions(input, MediaOptions{ViewportWidth: viewportWidthPx})
}

func stripCDATAMarkers(in string) string {
	if !strings.Contains(in, "<![CDATA[") {
		return in
	}
	s := strings.ReplaceAll(in, "<![CDATA[", " ")
	return strings.ReplaceAll(s, "]]>", " ")
}

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
				break
			}
			i += 2 + end + 2
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}

func StripComments(in string) string {
	return stripComments(in)
}

type parser struct {
	input                 string
	pos                   int
	mediaWidth            float32
	darkMode              bool
	highContrast          bool
	currentLayer          string
	currentContainerQuery string
	currentScopeRoot      string
	currentScopeLimit     string
	anonLayerCount        int
}

func (p *parser) parse() *Stylesheet {
	sheet := &Stylesheet{}
	p.parseInto(sheet)
	return sheet
}

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
			case "layer":
				p.parseLayerRule(sheet)
			case "container":
				p.parseContainerRule(sheet)
			case "scope":
				p.parseScopeRule(sheet)
			case "font-face":
				p.parseFontFaceRule(sheet)
			case "keyframes", "-webkit-keyframes":
				p.parseKeyframesRule(sheet)
			case "property":
				p.parsePropertyRule(sheet)
			case "font-palette-values":
				p.parseFontPaletteValuesRule(sheet)
			case "starting-style":
				p.parseStartingStyleRule(sheet)
			default:
				p.skipAtRule()
			}
			continue
		}
		rule, nestedRules := p.parseRule()
		if len(rule.Selectors) > 0 {
			if p.currentLayer != "" && rule.Layer == "" {
				rule.Layer = p.currentLayer
			}
			if p.currentContainerQuery != "" && rule.ContainerQuery == "" {
				rule.ContainerQuery = p.currentContainerQuery
			}
			if p.currentScopeRoot != "" && rule.ScopeRoot == "" {
				rule.ScopeRoot = p.currentScopeRoot
			}
			if p.currentScopeLimit != "" && rule.ScopeLimit == "" {
				rule.ScopeLimit = p.currentScopeLimit
			}
			sheet.Rules = append(sheet.Rules, rule)
		}
		sheet.Rules = append(sheet.Rules, nestedRules...)
	}
}

func (p *parser) parseLayerRule(sheet *Stylesheet) {
	p.pos++
	for p.pos < len(p.input) && p.input[p.pos] != ' ' && p.input[p.pos] != '\t' && p.input[p.pos] != '\n' && p.input[p.pos] != '\r' && p.input[p.pos] != '{' && p.input[p.pos] != ';' {
		p.pos++
	}
	p.skipWhitespace()
	preludeStart := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '{' && p.input[p.pos] != ';' {
		p.pos++
	}
	if p.pos >= len(p.input) {
		return
	}
	prelude := strings.TrimSpace(p.input[preludeStart:p.pos])
	if p.input[p.pos] == ';' {
		p.pos++
		if prelude != "" {
			parts := strings.Split(prelude, ",")
			for _, part := range parts {
				name := strings.TrimSpace(part)
				if name != "" {
					if p.currentLayer != "" {
						name = p.currentLayer + "." + name
					}
					sheet.Layers = append(sheet.Layers, name)
				}
			}
		}
		return
	}
	p.pos++
	layerName := prelude
	if layerName == "" {
		p.anonLayerCount++
		layerName = "anon#" + strconv.Itoa(p.anonLayerCount)
	} else if p.currentLayer != "" {
		layerName = p.currentLayer + "." + layerName
	}
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
		p.pos++
	}
	inner := &parser{
		input:                 body,
		mediaWidth:            p.mediaWidth,
		darkMode:              p.darkMode,
		highContrast:          p.highContrast,
		currentLayer:          layerName,
		currentContainerQuery: p.currentContainerQuery,
		currentScopeRoot:      p.currentScopeRoot,
		currentScopeLimit:     p.currentScopeLimit,
		anonLayerCount:        p.anonLayerCount,
	}
	inner.parseInto(sheet)
	p.anonLayerCount = inner.anonLayerCount
}

func (p *parser) parseContainerRule(sheet *Stylesheet) {
	p.pos++
	for p.pos < len(p.input) && p.input[p.pos] != ' ' && p.input[p.pos] != '\t' && p.input[p.pos] != '\n' && p.input[p.pos] != '\r' && p.input[p.pos] != '{' && p.input[p.pos] != ';' && p.input[p.pos] != '(' {
		p.pos++
	}
	p.skipWhitespace()
	preludeStart := p.pos
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
	prelude := strings.TrimSpace(p.input[preludeStart:p.pos])
	p.pos++
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
		p.pos++
	}
	cq := prelude
	if p.currentContainerQuery != "" {
		cq = p.currentContainerQuery + " and " + prelude
	}
	inner := &parser{
		input:                 body,
		mediaWidth:            p.mediaWidth,
		darkMode:              p.darkMode,
		highContrast:          p.highContrast,
		currentLayer:          p.currentLayer,
		currentContainerQuery: cq,
		currentScopeRoot:      p.currentScopeRoot,
		currentScopeLimit:     p.currentScopeLimit,
		anonLayerCount:        p.anonLayerCount,
	}
	inner.parseInto(sheet)
	p.anonLayerCount = inner.anonLayerCount
}

func (p *parser) parseScopeRule(sheet *Stylesheet) {
	p.pos++
	for p.pos < len(p.input) && p.input[p.pos] != ' ' && p.input[p.pos] != '\t' && p.input[p.pos] != '\n' && p.input[p.pos] != '\r' && p.input[p.pos] != '{' && p.input[p.pos] != ';' && p.input[p.pos] != '(' {
		p.pos++
	}
	p.skipWhitespace()
	preludeStart := p.pos
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
	prelude := strings.TrimSpace(p.input[preludeStart:p.pos])
	p.pos++

	var root, limit string
	open1 := strings.Index(prelude, "(")
	if open1 >= 0 {
		close1 := strings.Index(prelude[open1:], ")")
		if close1 >= 0 {
			close1 += open1
			before := strings.TrimSpace(prelude[:open1])
			if before == "to" {
				limit = strings.TrimSpace(prelude[open1+1 : close1])
			} else {
				root = strings.TrimSpace(prelude[open1+1 : close1])
				rest := strings.TrimSpace(prelude[close1+1:])
				if strings.HasPrefix(rest, "to") {
					rest = strings.TrimSpace(rest[2:])
					open2 := strings.Index(rest, "(")
					if open2 >= 0 {
						close2 := strings.Index(rest[open2:], ")")
						if close2 >= 0 {
							limit = strings.TrimSpace(rest[open2+1 : open2+close2])
						}
					}
				}
			}
		}
	}

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
		p.pos++
	}

	inner := &parser{
		input:                 body,
		mediaWidth:            p.mediaWidth,
		darkMode:              p.darkMode,
		highContrast:          p.highContrast,
		currentLayer:          p.currentLayer,
		currentContainerQuery: p.currentContainerQuery,
		currentScopeRoot:      root,
		currentScopeLimit:     limit,
		anonLayerCount:        p.anonLayerCount,
	}
	inner.parseInto(sheet)
	p.anonLayerCount = inner.anonLayerCount
}

func (p *parser) atRuleName() string {
	save := p.pos
	p.pos++
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if c == '{' || c == ';' || c == '(' || c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			break
		}
		p.pos++
	}
	name := strings.ToLower(p.input[save+1 : p.pos])
	p.pos = save
	return name
}

func atRuleCondition(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "@") {
		return text
	}
	for i := 1; i < len(text); i++ {
		switch text[i] {
		case '(', '{', ';', ' ', '\t', '\n', '\r':
			return strings.TrimSpace(text[i:])
		}
	}
	return ""
}

func (p *parser) parseMediaRule(sheet *Stylesheet) {
	at := p.pos
	p.pos++
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
	cond := atRuleCondition(p.input[at:p.pos])
	p.pos++
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
		p.pos++
	}
	if p.mediaConditionTrue(cond) {
		inner := &parser{
			input:                 body,
			mediaWidth:            p.mediaWidth,
			darkMode:              p.darkMode,
			highContrast:          p.highContrast,
			currentLayer:          p.currentLayer,
			currentContainerQuery: p.currentContainerQuery,
			currentScopeRoot:      p.currentScopeRoot,
			currentScopeLimit:     p.currentScopeLimit,
			anonLayerCount:        p.anonLayerCount,
		}
		inner.parseInto(sheet)
		p.anonLayerCount = inner.anonLayerCount
	}
}

func (p *parser) parseSupportsRule(sheet *Stylesheet) {
	at := p.pos
	p.pos++
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
	cond := atRuleCondition(p.input[at:p.pos])
	p.pos++
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
		p.pos++
	}
	if supportsConditionTrue(cond) {
		inner := &parser{
			input:                 body,
			mediaWidth:            p.mediaWidth,
			darkMode:              p.darkMode,
			highContrast:          p.highContrast,
			currentLayer:          p.currentLayer,
			currentContainerQuery: p.currentContainerQuery,
			currentScopeRoot:      p.currentScopeRoot,
			currentScopeLimit:     p.currentScopeLimit,
			anonLayerCount:        p.anonLayerCount,
		}
		inner.parseInto(sheet)
		p.anonLayerCount = inner.anonLayerCount
	}
}

func (p *parser) parseFontFaceRule(sheet *Stylesheet) {
	p.pos++
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
	p.pos++
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
		p.pos++
	}
	decls := parseDeclarations(body)
	if len(decls) > 0 {
		sheet.FontFaces = append(sheet.FontFaces, FontFaceRule{Declarations: decls})
	}
}

func (p *parser) parseKeyframesRule(sheet *Stylesheet) {
	p.pos++
	for p.pos < len(p.input) && p.input[p.pos] != '{' && p.input[p.pos] != ';' && p.input[p.pos] != ' ' && p.input[p.pos] != '\t' && p.input[p.pos] != '\n' && p.input[p.pos] != '\r' {
		p.pos++
	}
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t' || p.input[p.pos] == '\n' || p.input[p.pos] == '\r') {
		p.pos++
	}
	nameStart := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '{' && p.input[p.pos] != ';' && p.input[p.pos] != ' ' && p.input[p.pos] != '\t' && p.input[p.pos] != '\n' && p.input[p.pos] != '\r' {
		p.pos++
	}
	name := strings.TrimSpace(p.input[nameStart:p.pos])

	for p.pos < len(p.input) && p.input[p.pos] != '{' && p.input[p.pos] != ';' {
		p.pos++
	}
	if p.pos >= len(p.input) || p.input[p.pos] == ';' {
		if p.pos < len(p.input) {
			p.pos++
		}
		return
	}
	p.pos++

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
		p.pos++
	}

	if name == "" {
		return
	}

	stops, err := ParseKeyframes(body)
	if err != nil || len(stops) == 0 {
		return
	}

	sheet.Keyframes = append(sheet.Keyframes, KeyframesRule{
		Name:  name,
		Stops: stops,
	})
}

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
	if prop == "color" {
		_, ok := ParseColor(value)
		return ok
	}
	if prop != "display" {
		return false
	}
	switch value {
	case "grid", "inline-grid", "flex", "inline-flex", "subgrid":
		return value != "subgrid"
	case "block", "inline-block", "table", "table-row", "table-cell", "table-row-group":
		return true
	}
	return false
}

func SplitOutsideParens(s, sep string) []string {
	return splitOutsideParens(s, sep)
}

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

func MatchMediaWithOptions(cond string, opts MediaOptions) bool {
	cond = strings.TrimSpace(cond)
	if cond == "" || strings.EqualFold(cond, "all") {
		return true
	}
	vw := opts.ViewportWidth
	if vw <= 0 {
		vw = defaultMediaViewportWidth
	}
	p := &parser{
		mediaWidth:   vw,
		darkMode:     opts.DarkMode,
		highContrast: opts.HighContrast,
	}
	return p.mediaConditionTrue(cond)
}

func MatchMedia(cond string, viewportWidth float32) bool {
	return MatchMediaWithOptions(cond, MediaOptions{ViewportWidth: viewportWidth})
}

func (p *parser) mediaConditionTrue(cond string) bool {
	for _, group := range splitTopLevel(cond, ',') {
		if p.mediaGroupTrue(group) {
			return true
		}
	}
	return false
}

func (p *parser) mediaGroupTrue(group string) bool {
	group = strings.TrimSpace(strings.ToLower(group))
	if group == "" || group == "all" {
		return true
	}
	negate := false
	if strings.HasPrefix(group, "not") {
		negate = true
		group = strings.TrimSpace(strings.TrimPrefix(group, "not"))
	}
	result := p.mediaPartsTrue(group)
	if negate {
		return !result
	}
	return result
}

func (p *parser) mediaPartsTrue(group string) bool {
	if alts := splitOutsideParens(group, " or "); len(alts) > 1 {
		for _, a := range alts {
			if p.mediaPartsTrue(strings.TrimSpace(a)) {
				return true
			}
		}
		return false
	}
	for _, part := range splitTopLevel(group, ' ') {
		if holds, ok := p.mediaRangeHolds(part); ok {
			if !holds {
				return false
			}
			continue
		}
		part = strings.TrimSpace(part)
		if part == "" || part == "and" || part == "only" {
			continue
		}
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
			if px, ok := cssPx(val); ok && p.mediaWidth < px {
				return false
			}
		case "max-width", "max-device-width":
			if px, ok := cssPx(val); ok && p.mediaWidth > px {
				return false
			}
		case "orientation":
			if val == "portrait" {
				return false
			}
		case "prefers-color-scheme":
			if p.darkMode {
				if val == "light" {
					return false
				}
			} else {
				if val == "dark" {
					return false
				}
			}
		case "prefers-reduced-motion":
			if val == "reduce" {
				return false
			}
		case "prefers-contrast":
			if p.highContrast {
				if val != "" && val != "more" && val != "custom" {
					return false
				}
			} else {
				if val != "" && val != "no-preference" {
					return false
				}
			}
		case "min-device-pixel-ratio", "-webkit-min-device-pixel-ratio":
			if f, err := strconv.ParseFloat(val, 32); err == nil && mediaDevicePixelRatio < float32(f) {
				return false
			}
		}
	}
	return true
}

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

func calcPx(expr string) (float32, bool) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return 0, false
	}
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

func (p *parser) mediaRangeHolds(part string) (holds bool, ok bool) {
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
			return isLen && p.widthSatisfies(ops[0], px), isLen
		}
		if operands[1] == "width" {
			px, isLen := cssPx(operands[0])
			return isLen && p.widthSatisfies(flipOp(ops[0]), px), isLen
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
		return p.widthSatisfies(flipOp(ops[0]), lo) && p.widthSatisfies(ops[1], hi), true
	}
	return false, false
}

func (p *parser) widthSatisfies(op string, px float32) bool {
	switch op {
	case "<":
		return p.mediaWidth < px
	case "<=":
		return p.mediaWidth <= px
	case ">":
		return p.mediaWidth > px
	case ">=":
		return p.mediaWidth >= px
	case "=":
		return p.mediaWidth == px
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

func SplitTopLevel(s string, sep byte) []string {
	return splitTopLevel(s, sep)
}

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

func closesStringAt(quote, c byte) bool { return c == quote || c == '\n' }

func startsEscape(c byte) bool { return c == '\\' }

func stripEscapedNewlines(s string) string {
	var b []byte
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(s) && (s[i+1] == '\n' || s[i+1] == '\r') {
				i++
				if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
					i++
				}
				continue
			}
			if c == quote {
				quote = 0
			}
		} else if c == '\'' || c == '"' {
			quote = c
		}
		b = append(b, c)
	}
	return string(b)
}

func (p *parser) parseRule() (Rule, []Rule) {
	var rule Rule

	selectorStart := p.pos
	var quote byte
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if startsEscape(c) && p.pos+1 < len(p.input) {
			p.pos += 2
			continue
		}
		if quote != 0 {
			if closesStringAt(quote, c) {
				quote = 0
			}
		} else if c == '\'' || c == '"' {
			quote = c
		} else if c == '{' {
			break
		}
		p.pos++
	}
	selectorText := strings.TrimSpace(p.input[selectorStart:p.pos])

	if p.pos >= len(p.input) {
		return rule, nil
	}
	p.pos++

	p.skipWhitespace()
	declStart := p.pos
	depth := 0
	parens := 0
	quote = 0
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		if startsEscape(c) && p.pos+1 < len(p.input) {
			p.pos += 2
			continue
		}
		if quote != 0 {
			if closesStringAt(quote, c) {
				quote = 0
			}
		} else if (c == '\'' || c == '"') && parens == 0 {
			quote = c
		} else if c == '(' {
			parens++
		} else if c == ')' {
			if parens > 0 {
				parens--
			}
		} else if c == '{' {
			depth++
		} else if c == '}' {
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

	decls, nestedRules := p.parseBlockWithRules(declText, rule.SelectorStrs)
	rule.Declarations = decls
	return rule, nestedRules
}

func (p *parser) parseBlockWithRules(body string, parentSelectors []string) ([]Declaration, []Rule) {
	var decls []Declaration
	var nestedRules []Rule
	i := 0
	n := len(body)
	for i < n {
		for i < n && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n' || body[i] == '\r') {
			i++
		}
		if i >= n {
			break
		}
		if i+1 < n && body[i] == '/' && body[i+1] == '*' {
			end := strings.Index(body[i+2:], "*/")
			if end < 0 {
				break
			}
			i += end + 4
			continue
		}
		if body[i] == '@' {
			atStart := i
			for i < n && body[i] != ' ' && body[i] != '\t' && body[i] != '\n' && body[i] != '\r' && body[i] != '{' && body[i] != ';' {
				i++
			}
			atName := strings.ToLower(strings.TrimSpace(body[atStart+1 : i]))
			preludeStart := i
			var quote byte
			for i < n {
				c := body[i]
				if startsEscape(c) && i+1 < n {
					i += 2
					continue
				}
				if quote != 0 {
					if c == quote {
						quote = 0
					}
				} else if c == '\'' || c == '"' {
					quote = c
				} else if c == '{' || c == ';' {
					break
				}
				i++
			}
			prelude := strings.TrimSpace(body[preludeStart:i])
			if i < n && body[i] == ';' {
				i++
				continue
			}
			if i >= n {
				break
			}
			i++
			innerBodyStart := i
			depth := 1
			quote = 0
			for i < n && depth > 0 {
				c := body[i]
				if startsEscape(c) && i+1 < n {
					i += 2
					continue
				}
				if quote != 0 {
					if c == quote {
						quote = 0
					}
				} else if c == '\'' || c == '"' {
					quote = c
				} else if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
				i++
			}
			innerBody := body[innerBodyStart:i]
			if i < n {
				i++
			}
			atRules := p.handleNestedAtRule(atName, prelude, innerBody, parentSelectors)
			nestedRules = append(nestedRules, atRules...)
			continue
		}

		stmtStart := i
		var quote byte
		depth := 0
		stmtBadString := false
		hitChar := byte(0)
		hitPos := -1
		for i < n {
			c := body[i]
			if startsEscape(c) && i+1 < n {
				i += 2
				continue
			}
			if quote != 0 {
				if c == quote {
					quote = 0
				} else if c == '\n' {
					quote = 0
					stmtBadString = true
				}
			} else if (c == '\'' || c == '"') && depth == 0 {
				quote = c
			} else if c == '(' {
				depth++
			} else if c == ')' {
				if depth > 0 {
					depth--
				}
			} else if depth == 0 {
				if c == ';' || c == '{' {
					hitChar = c
					hitPos = i
					break
				}
			}
			i++
		}

		if hitChar == ';' {
			stmt := strings.TrimSpace(body[stmtStart:hitPos])
			i = hitPos + 1
			if stmt != "" && !stmtBadString {
				decls = append(decls, parseDeclarations(stmt)...)
			}
		} else if hitChar == '{' {
			nestedSelText := strings.TrimSpace(body[stmtStart:hitPos])
			i = hitPos + 1
			innerBodyStart := i
			depth := 1
			quote = 0
			for i < n && depth > 0 {
				c := body[i]
				if startsEscape(c) && i+1 < n {
					i += 2
					continue
				}
				if quote != 0 {
					if c == quote {
						quote = 0
					}
				} else if c == '\'' || c == '"' {
					quote = c
				} else if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
				i++
			}
			innerBody := body[innerBodyStart:i]
			if i < n {
				i++
			}
			expandedSelectors := ExpandNestedSelectors(parentSelectors, splitSelectors(nestedSelText))
			if len(expandedSelectors) > 0 {
				rules := p.parseNestedStyleRule(expandedSelectors, innerBody)
				nestedRules = append(nestedRules, rules...)
			}
		} else {
			trailing := strings.TrimSpace(body[stmtStart:])
			if trailing != "" && strings.Contains(trailing, ":") && !stmtBadString {
				decls = append(decls, parseDeclarations(trailing)...)
			}
			break
		}
	}
	return decls, nestedRules
}

func (p *parser) parseNestedStyleRule(expandedSelectors []string, innerBody string) []Rule {
	var rules []Rule
	var rule Rule
	for _, sel := range expandedSelectors {
		sel = strings.TrimSpace(sel)
		if sel != "" {
			rule.Selectors = append(rule.Selectors, ParseSelector(sel))
			rule.SelectorStrs = append(rule.SelectorStrs, sel)
		}
	}
	rule.Layer = p.currentLayer
	rule.ContainerQuery = p.currentContainerQuery
	rule.ScopeRoot = p.currentScopeRoot
	rule.ScopeLimit = p.currentScopeLimit

	innerDecls, deeperRules := p.parseBlockWithRules(innerBody, expandedSelectors)
	rule.Declarations = innerDecls

	if len(rule.Selectors) > 0 && len(rule.Declarations) > 0 {
		rules = append(rules, rule)
	}
	rules = append(rules, deeperRules...)
	return rules
}

func (p *parser) handleNestedAtRule(atName, prelude, innerBody string, parentSelectors []string) []Rule {
	var rules []Rule
	switch atName {
	case "media":
		cond := prelude
		if !p.mediaConditionTrue(cond) {
			return nil
		}
		inner := &parser{
			input:                 innerBody,
			mediaWidth:            p.mediaWidth,
			darkMode:              p.darkMode,
			highContrast:          p.highContrast,
			currentLayer:          p.currentLayer,
			currentContainerQuery: p.currentContainerQuery,
			currentScopeRoot:      p.currentScopeRoot,
			currentScopeLimit:     p.currentScopeLimit,
			anonLayerCount:        p.anonLayerCount,
		}
		decls, nested := inner.parseBlockWithRules(innerBody, parentSelectors)
		if len(decls) > 0 && len(parentSelectors) > 0 {
			var r Rule
			for _, sel := range parentSelectors {
				r.Selectors = append(r.Selectors, ParseSelector(sel))
				r.SelectorStrs = append(r.SelectorStrs, sel)
			}
			r.Declarations = decls
			r.Layer = p.currentLayer
			r.ContainerQuery = p.currentContainerQuery
			r.ScopeRoot = p.currentScopeRoot
			r.ScopeLimit = p.currentScopeLimit
			rules = append(rules, r)
		}
		rules = append(rules, nested...)
		p.anonLayerCount = inner.anonLayerCount
	case "container":
		cq := prelude
		if p.currentContainerQuery != "" {
			cq = p.currentContainerQuery + " and " + cq
		}
		inner := &parser{
			input:                 innerBody,
			mediaWidth:            p.mediaWidth,
			darkMode:              p.darkMode,
			highContrast:          p.highContrast,
			currentLayer:          p.currentLayer,
			currentContainerQuery: cq,
			currentScopeRoot:      p.currentScopeRoot,
			currentScopeLimit:     p.currentScopeLimit,
			anonLayerCount:        p.anonLayerCount,
		}
		decls, nested := inner.parseBlockWithRules(innerBody, parentSelectors)
		if len(decls) > 0 && len(parentSelectors) > 0 {
			var r Rule
			for _, sel := range parentSelectors {
				r.Selectors = append(r.Selectors, ParseSelector(sel))
				r.SelectorStrs = append(r.SelectorStrs, sel)
			}
			r.Declarations = decls
			r.Layer = p.currentLayer
			r.ContainerQuery = cq
			r.ScopeRoot = p.currentScopeRoot
			r.ScopeLimit = p.currentScopeLimit
			rules = append(rules, r)
		}
		rules = append(rules, nested...)
		p.anonLayerCount = inner.anonLayerCount
	case "layer":
		layerName := prelude
		if layerName == "" {
			p.anonLayerCount++
			layerName = "anon#" + strconv.Itoa(p.anonLayerCount)
		} else if p.currentLayer != "" {
			layerName = p.currentLayer + "." + layerName
		}
		inner := &parser{
			input:                 innerBody,
			mediaWidth:            p.mediaWidth,
			darkMode:              p.darkMode,
			highContrast:          p.highContrast,
			currentLayer:          layerName,
			currentContainerQuery: p.currentContainerQuery,
			currentScopeRoot:      p.currentScopeRoot,
			currentScopeLimit:     p.currentScopeLimit,
			anonLayerCount:        p.anonLayerCount,
		}
		decls, nested := inner.parseBlockWithRules(innerBody, parentSelectors)
		if len(decls) > 0 && len(parentSelectors) > 0 {
			var r Rule
			for _, sel := range parentSelectors {
				r.Selectors = append(r.Selectors, ParseSelector(sel))
				r.SelectorStrs = append(r.SelectorStrs, sel)
			}
			r.Declarations = decls
			r.Layer = layerName
			r.ContainerQuery = p.currentContainerQuery
			r.ScopeRoot = p.currentScopeRoot
			r.ScopeLimit = p.currentScopeLimit
			rules = append(rules, r)
		}
		rules = append(rules, nested...)
		p.anonLayerCount = inner.anonLayerCount
	case "supports":
		cond := prelude
		if !supportsConditionTrue(cond) {
			return nil
		}
		inner := &parser{
			input:                 innerBody,
			mediaWidth:            p.mediaWidth,
			darkMode:              p.darkMode,
			highContrast:          p.highContrast,
			currentLayer:          p.currentLayer,
			currentContainerQuery: p.currentContainerQuery,
			currentScopeRoot:      p.currentScopeRoot,
			currentScopeLimit:     p.currentScopeLimit,
			anonLayerCount:        p.anonLayerCount,
		}
		decls, nested := inner.parseBlockWithRules(innerBody, parentSelectors)
		if len(decls) > 0 && len(parentSelectors) > 0 {
			var r Rule
			for _, sel := range parentSelectors {
				r.Selectors = append(r.Selectors, ParseSelector(sel))
				r.SelectorStrs = append(r.SelectorStrs, sel)
			}
			r.Declarations = decls
			r.Layer = p.currentLayer
			r.ContainerQuery = p.currentContainerQuery
			r.ScopeRoot = p.currentScopeRoot
			r.ScopeLimit = p.currentScopeLimit
			rules = append(rules, r)
		}
		rules = append(rules, nested...)
		p.anonLayerCount = inner.anonLayerCount
	case "starting-style":
		inner := &parser{
			input:                 innerBody,
			mediaWidth:            p.mediaWidth,
			darkMode:              p.darkMode,
			highContrast:          p.highContrast,
			currentLayer:          p.currentLayer,
			currentContainerQuery: p.currentContainerQuery,
			currentScopeRoot:      p.currentScopeRoot,
			currentScopeLimit:     p.currentScopeLimit,
			anonLayerCount:        p.anonLayerCount,
		}
		decls, nested := inner.parseBlockWithRules(innerBody, parentSelectors)
		if len(decls) > 0 && len(parentSelectors) > 0 {
			var r Rule
			for _, sel := range parentSelectors {
				r.Selectors = append(r.Selectors, ParseSelector(sel))
				r.SelectorStrs = append(r.SelectorStrs, sel)
			}
			r.Declarations = decls
			r.Layer = p.currentLayer
			r.ContainerQuery = p.currentContainerQuery
			r.ScopeRoot = p.currentScopeRoot
			r.ScopeLimit = p.currentScopeLimit
			r.StartingStyle = true
			rules = append(rules, r)
		}
		for i := range nested {
			nested[i].StartingStyle = true
		}
		rules = append(rules, nested...)
		p.anonLayerCount = inner.anonLayerCount
	}
	return rules
}

func ExpandNestedSelectors(parentSelectors, nestedSelectors []string) []string {
	if len(parentSelectors) == 0 {
		return nestedSelectors
	}
	var result []string
	for _, nested := range nestedSelectors {
		nested = strings.TrimSpace(nested)
		if nested == "" {
			continue
		}
		for _, parent := range parentSelectors {
			parent = strings.TrimSpace(parent)
			if parent == "" {
				continue
			}
			exp := ExpandSingleNestedSelector(parent, nested)
			if exp != "" {
				result = append(result, exp)
			}
		}
	}
	return result
}

func ExpandSingleNestedSelector(parent, nested string) string {
	hasAmpersand := false
	var quote byte
	for i := 0; i < len(nested); i++ {
		c := nested[i]
		if startsEscape(c) && i+1 < len(nested) {
			i++
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
		} else if c == '\'' || c == '"' {
			quote = c
		} else if c == '&' {
			hasAmpersand = true
			break
		}
	}

	if hasAmpersand {
		var buf strings.Builder
		quote = 0
		for i := 0; i < len(nested); i++ {
			c := nested[i]
			if startsEscape(c) && i+1 < len(nested) {
				i++
				continue
			}
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				buf.WriteByte(c)
			} else if c == '\'' || c == '"' {
				quote = c
				buf.WriteByte(c)
			} else if c == '&' {
				buf.WriteString(parent)
			} else {
				buf.WriteByte(c)
			}
		}
		return buf.String()
	}

	trimmed := strings.TrimLeft(nested, " \t\r\n")
	if len(trimmed) > 0 && (trimmed[0] == '>' || trimmed[0] == '+' || trimmed[0] == '~') {
		return parent + " " + trimmed
	}
	if len(trimmed) > 0 && trimmed[0] == ':' {
		return parent + trimmed
	}
	return parent + " " + trimmed
}

func splitSelectors(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"', '\'':
			i = quotedRunEnd(s, i) - 1
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
		val = stripEscapedNewlines(val)

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
	return SplitDeclarations(s)
}

func SplitDeclarations(s string) []string {
	var parts []string
	depth := 0
	var quote byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if startsEscape(c) && i+1 < len(s) {
			i++
			continue
		}
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			if depth == 0 {
				quote = c
			}
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
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

func (s *Stylesheet) Merge(other *Stylesheet) {
	if other == nil {
		return
	}
	s.Rules = append(s.Rules, other.Rules...)
	s.Keyframes = append(s.Keyframes, other.Keyframes...)
	s.Properties = append(s.Properties, other.Properties...)
}
