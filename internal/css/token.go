package css

import (
	"strings"
	"unicode"
)

// TokenType identifies a CSS token.
type TokenType uint8

const (
	TokenIdent TokenType = iota
	TokenString
	TokenNumber
	TokenPercentage
	TokenDimension
	TokenHash
	TokenDelim
	TokenColon
	TokenSemicolon
	TokenComma
	TokenLBrace
	TokenRBrace
	TokenLParen
	TokenRParen
	TokenLBracket
	TokenRBracket
	TokenGreaterThan
	TokenPlus
	TokenTilde
	TokenStar
	TokenDot
	TokenWhitespace
	TokenCDO
	TokenCDC
	TokenAtKeyword
	TokenFunction
	TokenEOF
)

// Token is one CSS token.
type Token struct {
	Type    TokenType
	Value   string
	Unit    string
	NumVal  float64
	IntVal  int64
	IsInt   bool
}

// Tokenizer splits CSS text into tokens.
type Tokenizer struct {
	input string
	pos   int
}

// NewTokenizer returns a tokenizer for the given CSS text.
func NewTokenizer(input string) *Tokenizer {
	return &Tokenizer{input: input}
}

// Next returns the next token.
func (t *Tokenizer) Next() Token {
	t.skipWhitespaceAndComments()
	if t.pos >= len(t.input) {
		return Token{Type: TokenEOF}
	}

	c := t.input[t.pos]

	switch {
	case c == '{':
		t.pos++
		return Token{Type: TokenLBrace, Value: "{"}
	case c == '}':
		t.pos++
		return Token{Type: TokenRBrace, Value: "}"}
	case c == '(':
		t.pos++
		return Token{Type: TokenLParen, Value: "("}
	case c == ')':
		t.pos++
		return Token{Type: TokenRParen, Value: ")"}
	case c == '[':
		t.pos++
		return Token{Type: TokenLBracket, Value: "["}
	case c == ']':
		t.pos++
		return Token{Type: TokenRBracket, Value: "]"}
	case c == ':':
		t.pos++
		return Token{Type: TokenColon, Value: ":"}
	case c == ';':
		t.pos++
		return Token{Type: TokenSemicolon, Value: ";"}
	case c == ',':
		t.pos++
		return Token{Type: TokenComma, Value: ","}
	case c == '>':
		t.pos++
		return Token{Type: TokenGreaterThan, Value: ">"}
	case c == '+':
		t.pos++
		return Token{Type: TokenPlus, Value: "+"}
	case c == '~':
		t.pos++
		return Token{Type: TokenTilde, Value: "~"}
	case c == '*':
		t.pos++
		peek := t.pos
		if peek < len(t.input) && isIdentStart(t.input[peek]) {
			return t.readIdentOrFunc()
		}
		return Token{Type: TokenStar, Value: "*"}
	case c == '.':
		if t.pos+1 < len(t.input) && t.input[t.pos+1] >= '0' && t.input[t.pos+1] <= '9' {
			// `.` followed by a digit starts a number: `.9em` is a dimension,
			// not a class token.
			return t.readNumber()
		}
		t.pos++
		if t.pos < len(t.input) && isIdentStart(t.input[t.pos]) {
			tok := t.readIdent()
			return Token{Type: TokenDot, Value: tok.Value}
		}
		return Token{Type: TokenDot, Value: "."}
	case c == '#':
		t.pos++
		tok := t.readIdent()
		return Token{Type: TokenHash, Value: tok.Value}
	case c == '"' || c == '\'':
		return t.readString(c)
	case c == '@':
		t.pos++
		tok := t.readIdent()
		return Token{Type: TokenAtKeyword, Value: tok.Value}
	case c == '<' && t.pos+3 < len(t.input) && t.input[t.pos+1:t.pos+4] == "!--":
		t.pos += 4
		return Token{Type: TokenCDO, Value: "<!--"}
	case c == '-' && t.pos+2 < len(t.input) && t.input[t.pos+1:t.pos+3] == "->":
		t.pos += 3
		return Token{Type: TokenCDC, Value: "-->"}
	case c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9'):
		return t.readNumber()
	case isIdentStart(c) || c == '-' || c == '_':
		return t.readIdentOrFunc()
	default:
		t.pos++
		return Token{Type: TokenDelim, Value: string(c)}
	}
}

func (t *Tokenizer) skipWhitespaceAndComments() {
	for t.pos < len(t.input) {
		if unicode.IsSpace(rune(t.input[t.pos])) {
			t.pos++
			continue
		}
		if t.pos+1 < len(t.input) && t.input[t.pos] == '/' && t.input[t.pos+1] == '*' {
			t.pos += 2
			for t.pos+1 < len(t.input) {
				if t.input[t.pos] == '*' && t.input[t.pos+1] == '/' {
					t.pos += 2
					break
				}
				t.pos++
			}
			continue
		}
		break
	}
}

func (t *Tokenizer) readIdent() Token {
	start := t.pos
	for t.pos < len(t.input) && isIdentChar(t.input[t.pos]) {
		t.pos++
	}
	return Token{Type: TokenIdent, Value: t.input[start:t.pos]}
}

func (t *Tokenizer) readIdentOrFunc() Token {
	start := t.pos
	for t.pos < len(t.input) && isIdentChar(t.input[t.pos]) {
		t.pos++
	}
	if t.pos < len(t.input) && t.input[t.pos] == '(' {
		t.pos++
		return Token{Type: TokenFunction, Value: t.input[start:t.pos-1]}
	}
	return Token{Type: TokenIdent, Value: t.input[start:t.pos]}
}

func (t *Tokenizer) readString(quote byte) Token {
	t.pos++
	var sb strings.Builder
	for t.pos < len(t.input) {
		c := t.input[t.pos]
		if c == quote {
			t.pos++
			return Token{Type: TokenString, Value: sb.String()}
		}
		if c == '\\' && t.pos+1 < len(t.input) {
			t.pos++
			next := t.input[t.pos]
			if isHex(next) {
				start := t.pos
				for t.pos < len(t.input) && t.pos-start < 6 && isHex(t.input[t.pos]) {
					t.pos++
				}
				cp := uint32(0)
				for j := start; j < t.pos; j++ {
					cp = cp*16 + hexVal(t.input[j])
				}
				if t.pos < len(t.input) && (t.input[t.pos] == ' ' || t.input[t.pos] == '\t' || t.input[t.pos] == '\n' || t.input[t.pos] == '\r' || t.input[t.pos] == '\f') {
					t.pos++
				}
				if cp == 0 || cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF) {
					sb.WriteRune('\uFFFD')
				} else {
					sb.WriteRune(rune(cp))
				}
				continue
			}
			sb.WriteByte(next)
			t.pos++
			continue
		}
		sb.WriteByte(c)
		t.pos++
	}
	return Token{Type: TokenString, Value: sb.String()}
}

func (t *Tokenizer) readNumber() Token {
	start := t.pos
	if t.pos < len(t.input) && (t.input[t.pos] == '-' || t.input[t.pos] == '+') {
		t.pos++
	}
	isFloat := false
	for t.pos < len(t.input) && t.input[t.pos] >= '0' && t.input[t.pos] <= '9' {
		t.pos++
	}
	if t.pos < len(t.input) && t.input[t.pos] == '.' {
		isFloat = true
		t.pos++
		for t.pos < len(t.input) && t.input[t.pos] >= '0' && t.input[t.pos] <= '9' {
			t.pos++
		}
	}
	if t.pos < len(t.input) && (t.input[t.pos] == 'e' || t.input[t.pos] == 'E') {
		// Only treat as scientific notation if followed by digit or +/-
		nextPos := t.pos + 1
		if nextPos < len(t.input) {
			nextChar := t.input[nextPos]
			if nextChar >= '0' && nextChar <= '9' || nextChar == '+' || nextChar == '-' {
				isFloat = true
				t.pos++
				if t.pos < len(t.input) && (t.input[t.pos] == '+' || t.input[t.pos] == '-') {
					t.pos++
				}
				for t.pos < len(t.input) && t.input[t.pos] >= '0' && t.input[t.pos] <= '9' {
					t.pos++
				}
			}
		}
	}

	numStr := t.input[start:t.pos]

	if t.pos < len(t.input) && t.input[t.pos] == '%' {
		t.pos++
		val := parseFloat(numStr)
		return Token{Type: TokenPercentage, Value: numStr, NumVal: val}
	}

	if t.pos < len(t.input) && isIdentStart(t.input[t.pos]) {
		unitStart := t.pos
		for t.pos < len(t.input) && isIdentChar(t.input[t.pos]) {
			t.pos++
		}
		unit := t.input[unitStart:t.pos]
		val := parseFloat(numStr)
		return Token{Type: TokenDimension, Value: numStr, Unit: unit, NumVal: val}
	}

	val := parseFloat(numStr)
	if !isFloat {
		iv := parseInt(numStr)
		return Token{Type: TokenNumber, Value: numStr, NumVal: val, IntVal: iv, IsInt: true}
	}
	return Token{Type: TokenNumber, Value: numStr, NumVal: val}
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c >= 0x80
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '-'
}

func parseFloat(s string) float64 {
	var v float64
	neg := false
	i := 0
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	} else if i < len(s) && s[i] == '+' {
		i++
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		v = v*10 + float64(s[i]-'0')
		i++
	}
	if i < len(s) && s[i] == '.' {
		i++
		frac := 0.1
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			v += float64(s[i]-'0') * frac
			frac *= 0.1
			i++
		}
	}
	if neg {
		v = -v
	}
	return v
}

func parseInt(s string) int64 {
	var v int64
	neg := false
	i := 0
	if i < len(s) && s[i] == '-' {
		neg = true
		i++
	} else if i < len(s) && s[i] == '+' {
		i++
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		v = v*10 + int64(s[i]-'0')
		i++
	}
	if neg {
		v = -v
	}
	return v
}
