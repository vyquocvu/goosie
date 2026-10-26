package css

import "strings"

// URLToken is one url(...) reference located by ScanURL or ScanURLAt. Every
// field is a byte offset into the exact string passed in, so a caller that has
// to slice the original declaration — to re-emit an untouched payload verbatim
// (absolutizeCSSURLs) or to take the remainder after the token (the shorthand
// parsers and importRef) — can do it without re-scanning.
type URLToken struct {
	// Payload is the URL with any surrounding quotes removed and outer
	// whitespace trimmed. It is the raw text between the delimiters: no percent
	// or escape decoding happens here — the callers and the data-URI decoder
	// own that. Empty means the token carried no URL (e.g. `url()`).
	Payload string
	// Quote is the delimiter the payload was wrapped in — '"' or '\'' — or 0
	// when the URL was written unquoted. absolutizeCSSURLs re-emits a rewritten
	// reference wrapped in the same quote so a sheet round-trips unchanged.
	Quote byte
	// Start is the index of the 'u' in the "url(" that opened the token.
	Start int
	// AfterOpen is the index just past "url(" (Start+4): the first byte of the
	// raw inner text, before any leading whitespace or opening quote.
	AfterOpen int
	// Close is the index of the ')' that terminates the token. For a quoted
	// payload it is the ')' that follows the closing quote, never a ')' inside
	// the string; for an unquoted payload it is the ')' that closes the
	// balanced parentheses.
	Close int
	// End is the index just past Close (Close+1): where the next token begins.
	End int
}

// ScanURL returns the first url(...) reference in s and reports whether one was
// found. It is the single quote-aware, balanced-paren payload scanner shared by
// every second-pass url() reader in the engine (internal/style's
// extractBackgroundURL, internal/css's borderImageURL, internal/engine's
// absolutizeCSSURLs, importRef and extractFontURL).
//
// The tokenizer already produces correct url() tokens, but those readers
// re-extract a URL from an already-parsed declaration *string*, where the only
// cheap way to find the end was strings.IndexByte(rest, ')') — blind to whether
// that ')' sits inside a quoted payload. An SVG data URI
// (`url("…fill='rgb(0,0,255)'…")`) is the common case: the payload legitimately
// contains ')', '(' and ';', and must survive whole (GOOSIE-CSSURLPAREN-001).
//
// Behaviour, each rule pinned by a test in internal/css/url_test.go and
// test/css/url_paren_test.go:
//
//   - A quoted payload ends at the matching quote and the ')' immediately
//     following it (modulo whitespace) closes the token, so `url("a)b")` → `a)b`
//     and `url('a)b')` → `a)b`.
//   - An unquoted payload ends at the ')' that closes balanced parentheses, so
//     `url(a(b)c)` → `a(b)c`. Goosie is deliberately more lenient than a strict
//     CSS Syntax §4.3.9 tokenizer here: a bare '(' or ')' in an unquoted url is
//     a bad-url token in Chromium (measured — see docs/tasks/TASK-050 §9), but
//     the engine's first-paint path already accepted it and real pages write
//     unquoted data URIs constantly.
//   - A payload wrapped in quotes that themselves contain unescaped quotes —
//     `url('data:…<svg xmlns='…' fill='lime'/>')`, extremely common in the wild
//     and pinned by test/engine/vector_bg_test.go — is not a well-formed string,
//     but the text up to the first ')' is a usable URL. When the bytes between
//     the candidate closing quote and that ')' are not pure whitespace the token
//     is re-read as an unquoted one and only a matched outer quote pair is
//     stripped. That is exactly what the per-site scanners did before this
//     helper existed, so this leniency is preserved rather than newly granted.
//   - Fail-closed on a malformed token: an unterminated quoted payload
//     (`url("abc`) and an escaped quote inside a quoted payload (`url("a\"b")`,
//     a bad string per §4.3.9) return ok=false with an empty payload rather than
//     handing back half a URI — the same recovery stance TASK-041 took for bad
//     strings.
//   - A payload-free `url()` is well-formed but empty: ok=true, Payload="".
//   - Only the first url() in a value is returned; a `none` keyword carries no
//     "url(" and so is never a match.
//
// It makes one linear pass over a value that has already passed the engine's
// admission caps (internal/engine/limits.go) and adds no second size limit.
func ScanURL(s string) (URLToken, bool) {
	return ScanURLAt(s, IndexURL(s))
}

// ScanURLAt reads the url(...) token whose "url(" begins at s[start]. It exists
// so a caller that is already scanning a large input for "url(" occurrences
// (absolutizeCSSURLs over a whole stylesheet) can share that search instead of
// re-lowering and re-indexing the remaining text once per token.
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
		// Consume to the matching quote. A backslash inside makes this a bad
		// string under §4.3.9 → fail closed.
		q := -1
		for k := p + 1; k < len(s); k++ {
			if s[k] == '\\' {
				return URLToken{}, false
			}
			if s[k] == quote {
				q = k
				break
			}
		}
		if q < 0 {
			return URLToken{}, false // unterminated quoted payload
		}
		if c := indexByteFrom(s, q+1, ')'); c >= 0 && onlyCSSSpace(s[q+1:c]) {
			t.Quote = quote
			t.Payload = s[p+1 : q]
			t.Close = c
			t.End = c + 1
			return t, true
		}
		// Otherwise the "quote" was a stray delimiter inside the payload: fall
		// through and read the token as unquoted, which is what the engine
		// accepted before this helper existed.
	}

	// Unquoted payload: balanced-paren scan from the first non-space byte.
	depth := 1
	for j := p; j < len(s); j++ {
		switch s[j] {
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
	return URLToken{}, false // the parentheses never close
}

// trimURLPayload strips the outer whitespace-quoted wrapper from an unquoted
// url() body. A matched pair of identical quotes is stripped and reported as
// the quote actually used; anything else falls back to the lenient trim the
// per-site scanners used before this helper existed.
func trimURLPayload(raw string) (payload string, quote byte) {
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] {
		return raw[1 : len(raw)-1], raw[0]
	}
	return strings.Trim(raw, "\"'"), 0
}

// IndexURL returns the index of the first case-insensitive "url(" in s, or -1.
// It is exported for absolutizeCSSURLs, which iterates every url() token in a
// fetched stylesheet and must not build a lower-cased copy of the remaining text
// once per token (that is quadratic in the sheet size, and strings.ToLower can
// change byte length, so offsets would not map back to the original sheet).
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
