package dom

import (
	"strings"
	"unicode/utf16"
)

func MaxLength(n *Node) (int, bool) {
	return parseNonNegativeIntegerAttribute(n, "maxlength")
}

func MinLength(n *Node) (int, bool) {
	return parseNonNegativeIntegerAttribute(n, "minlength")
}

func parseNonNegativeIntegerAttribute(n *Node, name string) (int, bool) {
	if n == nil || !n.HasAttribute(name) {
		return 0, false
	}
	value := n.GetAttribute(name)
	position := 0
	for position < len(value) && isASCIIWhitespace(value[position]) {
		position++
	}
	negative := false
	if position < len(value) && (value[position] == '+' || value[position] == '-') {
		negative = value[position] == '-'
		position++
	}
	start := position
	maxInt := int(^uint(0) >> 1)
	parsed := 0
	overflow := false
	for position < len(value) && value[position] >= '0' && value[position] <= '9' {
		digit := int(value[position] - '0')
		if parsed > (maxInt-digit)/10 {
			overflow = true
		} else if !overflow {
			parsed = parsed*10 + digit
		}
		position++
	}
	if position == start {
		return 0, false
	}
	if negative {
		if overflow || parsed != 0 {
			return 0, false
		}
		return 0, true
	}
	if overflow {
		return maxInt, true
	}
	return parsed, true
}

func UTF16Length(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func IsTooLong(n *Node) bool {
	if n == nil || !n.ValueChangedByUser {
		return false
	}
	max, hasMax := MaxLength(n)
	value, supported := formControlLengthValue(n)
	if !hasMax || !supported {
		return false
	}
	return UTF16Length(value) > max
}

func IsTooShort(n *Node) bool {
	if n == nil || !n.ValueChangedByUser {
		return false
	}
	min, hasMin := MinLength(n)
	value, supported := formControlLengthValue(n)
	if !hasMin || !supported || value == "" {
		return false
	}
	return UTF16Length(value) < min
}

func formControlLengthValue(n *Node) (string, bool) {
	var value string
	switch n.Data {
	case "textarea":
		value = n.InputControlValue()
	case "input":
		typ := strings.ToLower(n.GetAttribute("type"))
		switch typ {
		case "", "text", "search", "tel", "url", "email", "password":
		default:
			return "", false
		}
		value = n.InputControlValue()
	default:
		return "", false
	}
	return value, true
}

func isASCIIWhitespace(b byte) bool {
	switch b {
	case '\t', '\n', '\f', '\r', ' ':
		return true
	default:
		return false
	}
}
