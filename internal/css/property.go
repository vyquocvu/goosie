package css

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
)

// CleanSyntax strips enclosing quotes from a syntax string, e.g. "'<color>'" -> "<color>".
func CleanSyntax(syntax string) string {
	return dom.CleanSyntax(syntax)
}

// ValidatePropertySyntax checks whether a syntax descriptor conforms to the
// CSS Properties and Values API Level 1 grammar.
func ValidatePropertySyntax(syntax string) error {
	return dom.ValidatePropertySyntax(syntax)
}

// MatchesPropertySyntax returns true if value conforms to the registered syntax.
func MatchesPropertySyntax(syntax, val string) bool {
	syntax = CleanSyntax(syntax)
	val = strings.TrimSpace(val)
	if syntax == "<color>" {
		_, ok := ParseColor(val)
		return ok
	}
	return dom.MatchesPropertySyntax(syntax, val)
}

// parsePropertyRule parses an @property --name { ... } at-rule.
func (p *parser) parsePropertyRule(sheet *Stylesheet) {
	p.pos++ // '@'
	for p.pos < len(p.input) && p.input[p.pos] != ' ' && p.input[p.pos] != '\t' && p.input[p.pos] != '\n' && p.input[p.pos] != '\r' && p.input[p.pos] != '{' {
		p.pos++
	}
	p.skipWhitespace()
	preludeStart := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '{' && p.input[p.pos] != ';' {
		p.pos++
	}
	if p.pos >= len(p.input) || p.input[p.pos] != '{' {
		return
	}
	name := strings.TrimSpace(p.input[preludeStart:p.pos])
	p.pos++ // '{'

	bodyStart := p.pos
	depth := 1
	for p.pos < len(p.input) && depth > 0 {
		switch p.input[p.pos] {
		case '{':
			depth++
		case '}':
			depth--
		}
		p.pos++
	}
	bodyEnd := p.pos - 1
	if depth > 0 {
		return
	}
	body := p.input[bodyStart:bodyEnd]

	// CSS Property name MUST start with '--'
	if !strings.HasPrefix(name, "--") || len(name) <= 2 {
		return
	}

	var syntax, initialValue string
	hasInherits := false
	inherits := false
	hasSyntax := false
	hasInitialValue := false

	for _, declStr := range splitDeclarations(body) {
		declStr = strings.TrimSpace(declStr)
		if declStr == "" {
			continue
		}
		colon := strings.IndexByte(declStr, ':')
		if colon < 0 {
			continue
		}
		desc := strings.TrimSpace(strings.ToLower(declStr[:colon]))
		val := strings.TrimSpace(declStr[colon+1:])

		switch desc {
		case "syntax":
			syntax = CleanSyntax(val)
			hasSyntax = true
		case "inherits":
			lowerVal := strings.ToLower(val)
			if lowerVal == "true" {
				inherits = true
				hasInherits = true
			} else if lowerVal == "false" {
				inherits = false
				hasInherits = true
			}
		case "initial-value":
			initialValue = val
			hasInitialValue = true
		}
	}

	// Check required descriptors
	if !hasSyntax || !hasInherits {
		return
	}
	if err := ValidatePropertySyntax(syntax); err != nil {
		return
	}
	if syntax != "*" {
		if !hasInitialValue || !MatchesPropertySyntax(syntax, initialValue) {
			return
		}
	}

	sheet.Properties = append(sheet.Properties, dom.PropertyDefinition{
		Name:         name,
		Syntax:       syntax,
		Inherits:     inherits,
		InitialValue: initialValue,
	})
}

// parseStartingStyleRule parses top-level @starting-style { ... }.
func (p *parser) parseStartingStyleRule(sheet *Stylesheet) {
	p.pos++ // '@'
	for p.pos < len(p.input) && p.input[p.pos] != ' ' && p.input[p.pos] != '\t' && p.input[p.pos] != '\n' && p.input[p.pos] != '\r' && p.input[p.pos] != '{' {
		p.pos++
	}
	p.skipWhitespace()
	if p.pos >= len(p.input) || p.input[p.pos] != '{' {
		return
	}
	p.pos++ // '{'

	bodyStart := p.pos
	depth := 1
	for p.pos < len(p.input) && depth > 0 {
		switch p.input[p.pos] {
		case '{':
			depth++
		case '}':
			depth--
		}
		p.pos++
	}
	bodyEnd := p.pos - 1
	if depth > 0 {
		return
	}
	body := p.input[bodyStart:bodyEnd]

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

	innerSheet := inner.parse()
	for i := range innerSheet.Rules {
		innerSheet.Rules[i].StartingStyle = true
		sheet.Rules = append(sheet.Rules, innerSheet.Rules[i])
	}
	sheet.Properties = append(sheet.Properties, innerSheet.Properties...)
}

// parseFontPaletteValuesRule parses an @font-palette-values --name { ... } at-rule.
func (p *parser) parseFontPaletteValuesRule(sheet *Stylesheet) {
	p.pos++ // '@'
	for p.pos < len(p.input) && p.input[p.pos] != ' ' && p.input[p.pos] != '\t' && p.input[p.pos] != '\n' && p.input[p.pos] != '\r' && p.input[p.pos] != '{' {
		p.pos++
	}
	p.skipWhitespace()
	preludeStart := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '{' && p.input[p.pos] != ';' {
		p.pos++
	}
	if p.pos >= len(p.input) || p.input[p.pos] != '{' {
		return
	}
	name := strings.TrimSpace(p.input[preludeStart:p.pos])
	p.pos++ // '{'

	bodyStart := p.pos
	depth := 1
	for p.pos < len(p.input) && depth > 0 {
		switch p.input[p.pos] {
		case '{':
			depth++
		case '}':
			depth--
		}
		p.pos++
	}
	bodyEnd := p.pos - 1
	if depth > 0 {
		return
	}
	body := p.input[bodyStart:bodyEnd]

	rule := FontPaletteValuesRule{
		Name:           name,
		OverrideColors: make(map[int]string),
	}

	decls := parseDeclarations(body)
	for _, d := range decls {
		switch strings.ToLower(d.Property) {
		case "font-family":
			rule.FontFamily = strings.Trim(strings.TrimSpace(d.Value), `"'`)
		case "base-palette":
			rule.BasePalette = strings.TrimSpace(d.Value)
		case "override-colors":
			items := strings.Split(d.Value, ",")
			for _, item := range items {
				fields := strings.Fields(strings.TrimSpace(item))
				if len(fields) >= 2 {
					if idx, err := strconv.Atoi(fields[0]); err == nil {
						rule.OverrideColors[idx] = fields[1]
					}
				}
			}
		}
	}
	sheet.FontPalettes = append(sheet.FontPalettes, rule)
}
