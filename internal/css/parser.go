package css

import "strings"

// Declaration is one CSS property-value pair.
type Declaration struct {
	Property string
	Value    string
	Parsed   Value
	Important bool
}

// Rule is one CSS rule: selectors + declarations.
type Rule struct {
	Selectors    []Selector
	SelectorStrs []string
	Declarations []Declaration
}

// Stylesheet is a parsed CSS stylesheet.
type Stylesheet struct {
	Rules []Rule
}

// Parse parses a CSS stylesheet.
func Parse(input string) *Stylesheet {
	p := &parser{input: input}
	return p.parse()
}

type parser struct {
	input string
	pos   int
}

func (p *parser) parse() *Stylesheet {
	sheet := &Stylesheet{}
	for p.pos < len(p.input) {
		p.skipWhitespace()
		if p.pos >= len(p.input) {
			break
		}
		if p.input[p.pos] == '@' {
			p.skipAtRule()
			continue
		}
		rule := p.parseRule()
		if len(rule.Selectors) > 0 {
			sheet.Rules = append(sheet.Rules, rule)
		}
	}
	return sheet
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
