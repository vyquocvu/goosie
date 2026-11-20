package style

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
)

type contentItemKind int

const (
	contentStringItem contentItemKind = iota
	contentURLItem
	contentAttrItem
	contentEchoItem
	contentKeywordItem
	contentSlashItem
	contentInvalidItem
)

type contentItem struct {
	kind      contentItemKind
	text      string
	fallback  string
	hasFB     bool
	stringFB  bool
	invalidFB bool
}

func contentFuncPrefix(v string, name string) bool {
	return len(v) >= len(name) && strings.EqualFold(v[:len(name)], name)
}

func contentIsSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func scanCSSString(v string, i int) (string, int, bool) {
	if i >= len(v) || (v[i] != '"' && v[i] != '\'') {
		return "", i, false
	}
	q := v[i]
	i++
	var b strings.Builder
	for i < len(v) {
		if v[i] == '\\' && i+1 < len(v) {
			next := contentEscapeEnd(v, i)
			b.WriteString(css.DecodeEscapes(v[i:next]))
			i = next
			continue
		}
		if v[i] == q {
			return b.String(), i + 1, true
		}
		b.WriteByte(v[i])
		i++
	}
	return "", i, false
}

func contentEscapeEnd(v string, i int) int {
	j := i + 1
	if j >= len(v) {
		return len(v)
	}
	if !isContentHex(v[j]) {
		return j + 1
	}
	start := j
	for j < len(v) && j-start < 6 && isContentHex(v[j]) {
		j++
	}
	if j < len(v) && contentIsSpace(v[j]) {
		j++
	}
	return j
}

func isContentHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func scanContentFunc(v string, i int, name string) (string, int, bool) {
	open := i + len(name) + 1
	depth := 1
	j := open
	for j < len(v) {
		if v[j] == '"' || v[j] == '\'' {
			_, next, ok := scanCSSString(v, j)
			if !ok {
				return "", 0, false
			}
			j = next
			continue
		}
		if v[j] == '(' {
			depth++
		}
		if v[j] == ')' {
			depth--
			if depth == 0 {
				return v[open:j], j + 1, true
			}
		}
		j++
	}
	return "", 0, false
}

func splitContentArgs(inner string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(inner); i++ {
		if inner[i] == '"' || inner[i] == '\'' {
			_, next, ok := scanCSSString(inner, i)
			if !ok {
				break
			}
			i = next - 1
			continue
		}
		switch inner[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, inner[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, inner[start:])
	return out
}

func unwrapContentURLRef(inner string) (string, bool) {
	trimmed := strings.TrimSpace(inner)
	if trimmed == "" {
		return "", false
	}
	if trimmed[0] == '"' || trimmed[0] == '\'' {
		s, next, ok := scanCSSString(trimmed, 0)
		if !ok || next != len(trimmed) {
			return "", false
		}
		return s, true
	}
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] <= ' ' || trimmed[i] == '"' {
			return "", false
		}
	}
	return trimmed, true
}

func scanContentItems(value string) ([]contentItem, bool) {
	var out []contentItem
	i := 0
	for i < len(value) {
		c := value[i]
		rest := value[i:]
		switch {
		case contentIsSpace(c):
			i++
		case c == '/':
			out = append(out, contentItem{kind: contentSlashItem})
			i++
		case c == '"' || c == '\'':
			s, next, ok := scanCSSString(value, i)
			if !ok {
				return nil, false
			}
			out = append(out, contentItem{kind: contentStringItem, text: s})
			i = next
		case contentFuncPrefix(rest, "url("):
			inner, next, ok := scanContentFunc(value, i, "url")
			if !ok {
				return nil, false
			}
			ref, ok := unwrapContentURLRef(inner)
			if !ok {
				return nil, false
			}
			out = append(out, contentItem{kind: contentURLItem, text: ref})
			i = next
		case contentFuncPrefix(rest, "attr("):
			inner, next, ok := scanContentFunc(value, i, "attr")
			if !ok {
				return nil, false
			}
			args := splitContentArgs(inner)
			name := strings.ToLower(strings.TrimSpace(args[0]))
			if name == "" {
				return nil, false
			}
			item := contentItem{kind: contentAttrItem, text: name}
			if len(args) > 1 {
				fb := strings.TrimSpace(args[1])
				if s, after, ok2 := scanCSSString(fb, 0); ok2 && after == len(fb) {
					item.hasFB = true
					item.stringFB = true
					item.fallback = s
				} else {
					item.hasFB = true
					item.invalidFB = true
				}
			}
			out = append(out, item)
			i = next
		case contentFuncPrefix(rest, "counter("):
			inner, next, ok := scanContentFunc(value, i, "counter")
			if !ok {
				return nil, false
			}
			args := splitContentArgs(inner)
			name := strings.TrimSpace(args[0])
			if name == "" {
				return nil, false
			}
			if len(args) > 1 {
				out = append(out, contentItem{kind: contentEchoItem, text: "counter(" + name + ", " + strings.TrimSpace(args[1]) + ")"})
			} else {
				out = append(out, contentItem{kind: contentEchoItem, text: "counter(" + name + ")"})
			}
			i = next
		case contentFuncPrefix(rest, "counters("):
			inner, next, ok := scanContentFunc(value, i, "counters")
			if !ok {
				return nil, false
			}
			args := splitContentArgs(inner)
			if len(args) < 2 {
				return nil, false
			}
			name := strings.TrimSpace(args[0])
			sep := strings.TrimSpace(args[1])
			s, after, ok2 := scanCSSString(sep, 0)
			if name == "" || !ok2 || after != len(sep) {
				return nil, false
			}
			if len(args) > 2 {
				out = append(out, contentItem{kind: contentEchoItem, text: "counters(" + name + ", " + cssStringLiteral(s) + ", " + strings.TrimSpace(args[2]) + ")"})
			} else {
				out = append(out, contentItem{kind: contentEchoItem, text: "counters(" + name + ", " + cssStringLiteral(s) + ")"})
			}
			i = next
		default:
			j := i
			for j < len(value) && !contentIsSpace(value[j]) && value[j] != '/' && value[j] != '(' {
				j++
			}
			if j == i {
				return nil, false
			}
			if j < len(value) && value[j] == '(' {
				return []contentItem{{kind: contentInvalidItem}}, true
			}
			lower := strings.ToLower(value[i:j])
			switch lower {
			case "open-quote", "close-quote", "no-open-quote", "no-close-quote":
				out = append(out, contentItem{kind: contentEchoItem, text: lower})
			default:
				out = append(out, contentItem{kind: contentKeywordItem, text: lower})
			}
			i = j
		}
	}
	return out, true
}

func cssStringLiteral(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20:
			b.WriteByte('\\')
			b.WriteString(strconv.FormatInt(int64(r), 16))
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func writeContentItems(items []contentItem, attr func(string) (string, bool), resolveURL func(string) string) (string, bool) {
	var b strings.Builder
	sep := false
	emit := func(chunk string) {
		if sep {
			b.WriteByte(' ')
		}
		b.WriteString(chunk)
		sep = true
	}
	pending := strings.Builder{}
	run := false
	flush := func() {
		if !run {
			return
		}
		emit(cssStringLiteral(pending.String()))
		pending.Reset()
		run = false
	}
	for _, it := range items {
		switch it.kind {
		case contentSlashItem:
			flush()
			if sep {
				b.WriteByte(' ')
			}
			b.WriteString("/ ")
			sep = false
		case contentStringItem:
			pending.WriteString(it.text)
			run = true
		case contentAttrItem:
			if it.invalidFB {
				return "", false
			}
			if attr == nil {
				return "", false
			}
			value, found := attr(it.text)
			if !found {
				value = ""
				if it.hasFB && it.stringFB {
					value = it.fallback
				}
			}
			pending.WriteString(value)
			run = true
		case contentURLItem:
			flush()
			ref := it.text
			if resolveURL != nil {
				ref = resolveURL(ref)
			}
			emit(`url("` + ref + `")`)
		case contentEchoItem, contentKeywordItem:
			flush()
			emit(it.text)
		}
	}
	flush()
	return b.String(), true
}

func readContentValue(raw string, attr func(string) (string, bool), resolveURL func(string) string) (string, bool) {
	items, ok := scanContentItems(raw)
	if !ok || len(items) == 0 {
		return "", false
	}
	needsAttr := false
	for _, it := range items {
		if it.kind == contentInvalidItem {
			return "normal", true
		}
		if it.kind != contentAttrItem {
			continue
		}
		needsAttr = true
		if it.invalidFB {
			return "normal", true
		}
	}
	if needsAttr && attr == nil {
		return "", false
	}
	text, ok := writeContentItems(items, attr, resolveURL)
	if !ok {
		return "", false
	}
	return text, true
}
