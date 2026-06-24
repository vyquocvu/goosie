package css

import (
	"fmt"
	"strings"
)

// Parser processes CSS text and builds a StyleSheet.
type Parser struct {
	input string
	pos   int
}

// NewParser creates a new Parser.
func NewParser(input string) *Parser {
	return &Parser{input: strings.TrimPrefix(input, "\ufeff")}
}

// ParseStyleAttribute parses the content of a style attribute (e.g., "color: red; margin: 10px")
func ParseStyleAttribute(input string) ([]Declaration, error) {
	parser := NewParser(input)
	// parseDeclarations expects to consume until '}' or EOF
	// It doesn't require a starting '{'
	declarations := parser.parseDeclarations()
	return declarations, nil
}

// Parse parses the CSS input and returns a StyleSheet.
func (p *Parser) Parse() (*StyleSheet, error) {
	stylesheet := &StyleSheet{}
	p.consumeWhitespaceAndComments()

	for p.pos < len(p.input) {
		// Check for at-rules
		if p.peek() == '@' {
			atRule, err := p.parseAtRule()
			if err != nil {
				// Skip this at-rule on error
				p.skipToNextRule()
				p.consumeWhitespaceAndComments()
				continue
			}
			stylesheet.AtRules = append(stylesheet.AtRules, atRule)
			p.consumeWhitespaceAndComments()
			continue
		}

		// Parse regular rules
		selectors, err := p.parseSelectorSequences()
		if err != nil {
			p.skipToNextRule()
			p.consumeWhitespaceAndComments()
			continue
		}
		p.consumeWhitespaceAndComments()
		if !p.consumeChar('{') {
			p.skipToNextRule()
			p.consumeWhitespaceAndComments()
			continue
		}
		p.consumeWhitespaceAndComments()
		declarations := p.parseDeclarations()
		p.consumeWhitespaceAndComments()
		if !p.consumeChar('}') {
			p.skipToNextRule()
			p.consumeWhitespaceAndComments()
			continue
		}
		stylesheet.Rules = append(stylesheet.Rules, Rule{Selectors: selectors, Declarations: declarations})
		p.consumeWhitespaceAndComments()
	}
	return stylesheet, nil
}

// skipToNextRule skips input until it finds a point where a new rule or at-rule can start.
func (p *Parser) skipToNextRule() {
	braceCount := 0
	for p.pos < len(p.input) {
		ch := p.peek()
		if ch == '{' {
			braceCount++
			p.pos++
		} else if ch == '}' {
			braceCount--
			p.pos++
			if braceCount <= 0 {
				break
			}
		} else if ch == ';' && braceCount == 0 {
			p.pos++
			break
		} else {
			p.pos++
		}
	}
}

// parseAtRule parses an at-rule like @media, @import, @keyframes
func (p *Parser) parseAtRule() (AtRule, error) {
	atRule := AtRule{}

	if !p.consumeChar('@') {
		return atRule, fmt.Errorf("expected '@'")
	}

	atRule.Name = p.consumeIdentifier()
	p.consumeWhitespaceAndComments()

	// Parse prelude (everything before { or ;)
	prelude := ""
	for p.pos < len(p.input) && p.peek() != '{' && p.peek() != ';' {
		prelude += string(p.peek())
		p.pos++
	}
	atRule.Prelude = strings.TrimSpace(prelude)

	p.consumeWhitespaceAndComments()

	// If it has a block, parse it
	if p.peek() == '{' {
		p.consumeChar('{')
		p.consumeWhitespaceAndComments()

		// For @media and similar, parse nested rules
		if atRule.Name == "media" || atRule.Name == "supports" {
			for p.peek() != '}' && p.pos < len(p.input) {
				if p.peek() == '@' {
					nested, err := p.parseAtRule()
					if err != nil {
						return atRule, fmt.Errorf("error parsing nested at-rule: %w", err)
					}
					atRule.AtRules = append(atRule.AtRules, nested)
					p.consumeWhitespaceAndComments()
					continue
				}
				selectors, err := p.parseSelectorSequences()
				if err != nil {
					return atRule, fmt.Errorf("error parsing nested rule selectors: %w", err)
				}
				p.consumeWhitespaceAndComments()
				if !p.consumeChar('{') {
					return atRule, fmt.Errorf("expected '{' in nested rule")
				}
				p.consumeWhitespaceAndComments()
				declarations := p.parseDeclarations()
				p.consumeWhitespaceAndComments()
				if !p.consumeChar('}') {
					return atRule, fmt.Errorf("expected '}' in nested rule")
				}
				atRule.Rules = append(atRule.Rules, Rule{Selectors: selectors, Declarations: declarations})
				p.consumeWhitespaceAndComments()
			}
		} else if atRule.Name == "keyframes" || strings.HasSuffix(atRule.Name, "keyframes") {
			// For @keyframes, parse nested rules with simple selectors (identifiers or percentages)
			for p.peek() != '}' && p.pos < len(p.input) {
				p.consumeWhitespaceAndComments()
				if p.peek() == '}' {
					break
				}

				var selectors []SelectorSequence

				// Parse comma-separated selectors
				for {
					// Consume until comma or open brace
					var selectorPart string
					for p.pos < len(p.input) {
						ch := p.peek()
						if ch == ',' || ch == '{' {
							break
						}
						selectorPart += string(ch)
						p.pos++
					}
					selectorPart = strings.TrimSpace(selectorPart)
					if selectorPart != "" {
						selectors = append(selectors, SelectorSequence{
							Simple: SimpleSelector{TagName: selectorPart},
						})
					}

					p.consumeWhitespaceAndComments()
					if p.peek() == ',' {
						p.pos++ // consume comma
						p.consumeWhitespaceAndComments()
					} else {
						break
					}
				}

				if !p.consumeChar('{') {
					return atRule, fmt.Errorf("expected '{' in keyframe rule")
				}

				p.consumeWhitespaceAndComments()
				declarations := p.parseDeclarations()
				p.consumeWhitespaceAndComments()
				if !p.consumeChar('}') {
					return atRule, fmt.Errorf("expected '}' in keyframe rule")
				}

				atRule.Rules = append(atRule.Rules, Rule{Selectors: selectors, Declarations: declarations})
				p.consumeWhitespaceAndComments()
			}
		} else {
			// For others, just parse declarations
			atRule.Declarations = p.parseDeclarations()
		}

		p.consumeWhitespaceAndComments()
		if !p.consumeChar('}') {
			return atRule, fmt.Errorf("expected '}'")
		}
	} else if p.peek() == ';' {
		p.consumeChar(';')
	}

	return atRule, nil
}

// parseSelectorSequences parses a comma-separated list of selector sequences
func (p *Parser) parseSelectorSequences() ([]SelectorSequence, error) {
	var sequences []SelectorSequence
	for {
		seq, err := p.parseSelectorSequence()
		if err != nil {
			return nil, err
		}
		sequences = append(sequences, seq)

		p.consumeWhitespaceAndComments()
		if p.peek() == ',' {
			p.consumeChar(',')
			p.consumeWhitespaceAndComments()
		} else {
			break
		}
	}
	return sequences, nil
}

// parseSelectorSequence parses a single selector sequence with combinators
// e.g., "div > p.class" or "h1 + p span"
func (p *Parser) parseSelectorSequence() (SelectorSequence, error) {
	var root SelectorSequence
	current := &root

	for {
		simple, err := p.parseSimpleSelector()
		if err != nil {
			return root, err
		}
		current.Simple = simple

		// Save position before consuming whitespace
		savedPos := p.pos
		p.consumeWhitespace() // Don't consume comments here to detect combinators
		hadWhitespace := p.pos > savedPos

		// Check for combinator
		if p.pos >= len(p.input) {
			break
		}

		ch := p.peek()
		if ch == ',' || ch == '{' || ch == ')' {
			break
		}

		combinator := ""
		if ch == '>' {
			combinator = ">"
			p.consumeChar('>')
		} else if ch == '+' {
			combinator = "+"
			p.consumeChar('+')
		} else if ch == '~' {
			combinator = "~"
			p.consumeChar('~')
		} else if hadWhitespace && (isIdentifierStart(ch) || ch == '*' || ch == '#' || ch == '.' || ch == ':' || ch == '[') {
			// We had whitespace and now there's another selector, so it's a descendant combinator
			combinator = " "
		} else {
			break
		}

		if combinator != "" {
			p.consumeWhitespaceAndComments()
			current.Combinator = combinator
			current.Next = &SelectorSequence{}
			current = current.Next
		} else {
			break
		}
	}

	return root, nil
}

// parseSimpleSelector parses a simple selector like "div.class#id:hover[attr]"
func (p *Parser) parseSimpleSelector() (SimpleSelector, error) {
	selector := SimpleSelector{}
	p.consumeWhitespaceAndComments()

	// Check for universal selector
	if p.peek() == '*' {
		p.consumeChar('*')
		selector.Universal = true
	} else if isIdentifierStart(p.peek()) {
		// Parse tag name
		selector.TagName = p.consumeIdentifier()
	}

	// Parse classes, IDs, pseudo-classes, pseudo-elements, and attributes
	for {
		ch := p.peek()
		if ch == '#' {
			p.consumeChar('#')
			selector.ID = p.consumeIdentifier()
		} else if ch == '.' {
			p.consumeChar('.')
			selector.Classes = append(selector.Classes, p.consumeIdentifier())
		} else if ch == ':' {
			p.consumeChar(':')
			// Check for pseudo-element (::)
			if p.peek() == ':' {
				p.consumeChar(':')
				pseudoElement := p.consumeIdentifier()
				// Handle functional pseudo-elements
				if p.peek() == '(' {
					pseudoElement += p.consumeFunctionArgs()
				}
				selector.PseudoElements = append(selector.PseudoElements, pseudoElement)
			} else {
				// Pseudo-class
				pseudoClass := p.consumeIdentifier()
				// Handle functional pseudo-classes like :nth-child(2)
				if p.peek() == '(' {
					pseudoClass += p.consumeFunctionArgs()
				}
				selector.PseudoClasses = append(selector.PseudoClasses, pseudoClass)
			}
		} else if ch == '[' {
			attr, err := p.parseAttributeSelector()
			if err != nil {
				return selector, err
			}
			selector.Attributes = append(selector.Attributes, attr)
		} else {
			break
		}
	}

	return selector, nil
}

// parseAttributeSelector parses an attribute selector like [type="text"]
func (p *Parser) parseAttributeSelector() (AttributeSelector, error) {
	attr := AttributeSelector{}

	if !p.consumeChar('[') {
		return attr, fmt.Errorf("expected '['")
	}

	p.consumeWhitespaceAndComments()
	attr.Name = p.consumeIdentifier()
	p.consumeWhitespaceAndComments()

	// Check for operator
	if p.peek() == '=' {
		attr.Operator = "="
		p.consumeChar('=')
	} else if p.peek() == '~' || p.peek() == '|' || p.peek() == '^' || p.peek() == '$' || p.peek() == '*' {
		attr.Operator = string(p.peek())
		p.pos++
		if p.peek() == '=' {
			attr.Operator += "="
			p.consumeChar('=')
		}
	}

	if attr.Operator != "" {
		p.consumeWhitespaceAndComments()
		// Parse value (can be quoted or unquoted)
		if p.peek() == '"' || p.peek() == '\'' {
			quote := p.peek()
			p.pos++
			attr.Value = p.consumeUntilChar(quote)
			p.consumeChar(quote)
		} else {
			attr.Value = p.consumeIdentifier()
		}
	}

	p.consumeWhitespaceAndComments()
	if !p.consumeChar(']') {
		return attr, fmt.Errorf("expected ']'")
	}

	return attr, nil
}

// consumeFunctionArgs consumes function arguments including parentheses
func (p *Parser) consumeFunctionArgs() string {
	if p.peek() != '(' {
		return ""
	}

	result := "("
	p.pos++
	depth := 1

	for p.pos < len(p.input) && depth > 0 {
		ch := p.peek()
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
		}
		result += string(ch)
		p.pos++
	}

	return result
}

func (p *Parser) parseDeclarations() []Declaration {
	var declarations []Declaration
	for {
		p.consumeWhitespaceAndComments()
		if p.peek() == '}' || p.pos >= len(p.input) {
			break
		}

		property := p.consumeIdentifier()
		if property == "" {
			// Skip malformed declaration
			// Consume until we find ; or }
			for p.pos < len(p.input) && p.peek() != ';' && p.peek() != '}' {
				p.pos++
			}
			if p.peek() == ';' {
				p.consumeChar(';')
			}
			continue
		}

		p.consumeWhitespaceAndComments()
		if !p.consumeChar(':') {
			// Skip malformed declaration
			for p.pos < len(p.input) && p.peek() != ';' && p.peek() != '}' {
				p.pos++
			}
			if p.peek() == ';' {
				p.consumeChar(';')
			}
			continue
		}
		p.consumeWhitespaceAndComments()

		value := p.consumeDeclarationValue()
		important := false

		// Check for !important
		trimmedValue := strings.TrimSpace(value)
		if strings.HasSuffix(trimmedValue, "!important") {
			important = true
			value = strings.TrimSpace(strings.TrimSuffix(trimmedValue, "!important"))
		}

		p.consumeWhitespaceAndComments()
		if p.peek() == ';' {
			p.consumeChar(';')
		}

		declarations = append(declarations, Declaration{
			Property:  property,
			Value:     value,
			Important: important,
		})
		p.consumeWhitespaceAndComments()
		if p.peek() == '}' {
			break
		}
	}
	return declarations
}

// consumeDeclarationValue consumes a declaration value, handling nested functions and strings
func (p *Parser) consumeDeclarationValue() string {
	var result string
	depth := 0

	for p.pos < len(p.input) {
		ch := p.peek()

		// Stop at ; or } if not inside parentheses or quotes
		if depth == 0 && (ch == ';' || ch == '}') {
			break
		}

		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
		} else if ch == '"' || ch == '\'' {
			// Handle quoted strings
			quote := ch
			result += string(ch)
			p.pos++
			for p.pos < len(p.input) {
				ch = p.peek()
				result += string(ch)
				p.pos++
				if ch == quote {
					break
				}
				if ch == '\\' && p.pos < len(p.input) {
					// Escape sequence
					result += string(p.peek())
					p.pos++
				}
			}
			continue
		}

		result += string(ch)
		p.pos++
	}

	return strings.TrimSpace(result)
}

func (p *Parser) consumeIdentifier() string {
	var result string
	for p.pos < len(p.input) {
		ch := p.peek()
		if isIdentifierChar(ch) {
			result += string(p.input[p.pos])
			p.pos++
		} else if ch == '\\' {
			// Handle escape sequence
			p.pos++ // consume backslash
			if p.pos < len(p.input) {
				// For now, just consume the next character literally
				// This handles cases like .w-1\/2 -> class "w-1/2"
				result += string(p.input[p.pos])
				p.pos++
			}
		} else {
			break
		}
	}
	return result
}

func (p *Parser) consumeUntil(stopChar byte) string {
	var result string
	for p.pos < len(p.input) && p.peek() != stopChar {
		result += string(p.input[p.pos])
		p.pos++
	}
	return strings.TrimSpace(result)
}

func (p *Parser) consumeUntilChar(stopChar byte) string {
	var result string
	for p.pos < len(p.input) && p.peek() != stopChar {
		ch := p.peek()
		if ch == '\\' && p.pos+1 < len(p.input) {
			// Escape sequence
			p.pos++
			result += string(p.peek())
			p.pos++
			continue
		}
		result += string(ch)
		p.pos++
	}
	return result
}

func (p *Parser) consumeWhitespace() {
	for p.pos < len(p.input) && isWhitespace(p.peek()) {
		p.pos++
	}
}

func (p *Parser) consumeWhitespaceAndComments() {
	for {
		// Consume whitespace
		for p.pos < len(p.input) && isWhitespace(p.peek()) {
			p.pos++
		}

		// Check for comment
		if p.pos < len(p.input)-1 && p.input[p.pos] == '/' && p.input[p.pos+1] == '*' {
			// Consume comment
			p.pos += 2
			for p.pos < len(p.input)-1 {
				if p.input[p.pos] == '*' && p.input[p.pos+1] == '/' {
					p.pos += 2
					break
				}
				p.pos++
			}
		} else {
			break
		}
	}
}

func (p *Parser) peek() byte {
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *Parser) consumeChar(char byte) bool {
	if p.peek() == char {
		p.pos++
		return true
	}
	return false
}

func isWhitespace(char byte) bool {
	return char == ' ' || char == '\n' || char == '\t' || char == '\r'
}

func isIdentifierChar(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_'
}

func isIdentifierStart(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '-' || char == '_'
}
