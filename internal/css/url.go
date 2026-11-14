package css

import "strings"

type URLToken struct {
	Payload   string
	Quote     byte
	Start     int
	AfterOpen int
	Close     int
	End       int
}

func ScanURL(s string) (URLToken, bool) {
	return ScanURLAt(s, IndexURL(s))
}

func URLPayload(t URLToken) string {
	return DecodeEscapes(t.Payload)
}

func ScanURLAt(s string, start int) (URLToken, bool) {
	if start < 0 || start+4 > len(s) || !hasURLPrefixFold(s[start:]) {
		return URLToken{}, false
	}
	afterOpen := start + 4
	p := afterOpen
	for p < len(s) && isCSSSpace(s[p]) {
		p++
	}
	t := URLToken{Start: start, AfterOpen: afterOpen}

	if p < len(s) && (s[p] == '"' || s[p] == '\'') {
		quote := s[p]
		q := -1
		for k := p + 1; k < len(s); k++ {
			if s[k] == '\\' {
				k++
				continue
			}
			if s[k] == quote {
				q = k
				break
			}
		}
		if q < 0 {
			return URLToken{}, false
		}
		if c := indexByteFrom(s, q+1, ')'); c >= 0 && onlyCSSSpace(s[q+1:c]) {
			t.Quote = quote
			t.Payload = s[p+1 : q]
			t.Close = c
			t.End = c + 1
			return t, true
		}
	}

	depth := 1
	for j := p; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				t.Payload, t.Quote = trimURLPayload(strings.TrimSpace(s[p:j]))
				t.Close = j
				t.End = j + 1
				return t, true
			}
		}
	}
	return URLToken{}, false
}

func trimURLPayload(raw string) (payload string, quote byte) {
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] {
		return raw[1 : len(raw)-1], raw[0]
	}
	return strings.Trim(raw, "\"'"), 0
}

func IndexURL(s string) int {
	for i := 0; i+4 <= len(s); i++ {
		if hasURLPrefixFold(s[i:]) {
			return i
		}
	}
	return -1
}

func hasURLPrefixFold(s string) bool {
	return len(s) >= 4 &&
		(s[0] == 'u' || s[0] == 'U') &&
		(s[1] == 'r' || s[1] == 'R') &&
		(s[2] == 'l' || s[2] == 'L') &&
		s[3] == '('
}

func indexByteFrom(s string, from int, b byte) int {
	if from >= len(s) {
		return -1
	}
	if k := strings.IndexByte(s[from:], b); k >= 0 {
		return from + k
	}
	return -1
}

func isCSSSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func onlyCSSSpace(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isCSSSpace(s[i]) {
			return false
		}
	}
	return true
}
