package style

import (
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
)

// CSS counters (css-lists-3 §5-7): counter-reset/set/increment establish and
// adjust named counters in nested scopes, and counter()/counters() in
// `content` render them. Scopes are tree state threaded through layout in
// document order, not inherited computed values.

// ParseCounterDecls parses `counter-reset`, `counter-set` and
// `counter-increment` values: `none` or runs of `name [integer]`. Bare names
// default to def (0 for reset/set, 1 for increment). It returns nil for
// `none`, empty input, or any malformed pair, in which case the declaration
// is dropped like any other invalid one.
func ParseCounterDecls(value string, def int) map[string]int {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return nil
	}
	if len(fields) == 1 && strings.EqualFold(fields[0], "none") {
		return nil
	}
	out := make(map[string]int)
	for i := 0; i < len(fields); {
		name := fields[i]
		i++
		if !isCounterName(name) {
			return nil
		}
		v := def
		if i < len(fields) && isDecimalInt(fields[i]) {
			n, err := strconv.Atoi(fields[i])
			if err != nil {
				return nil
			}
			v = n
			i++
		}
		out[name] = v
	}
	return out
}

func isCounterName(s string) bool {
	if s == "" {
		return false
	}
	c := s[0]
	if c >= '0' && c <= '9' {
		return false
	}
	if c == '-' && (len(s) < 2 || (s[1] >= '0' && s[1] <= '9')) {
		return false
	}
	if strings.EqualFold(s, "none") || strings.EqualFold(s, "inherit") ||
		strings.EqualFold(s, "initial") || strings.EqualFold(s, "unset") {
		return false
	}
	return true
}

func isDecimalInt(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i = 1
	}
	if i >= len(s) {
		return false
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// FormatCounter renders value in a predefined counter style. Unknown styles
// fall back to decimal, which is also the spec's last resort: rendering
// SOMETHING wrong-looking beats rendering nothing, and matches what the
// engine did before a style existed at all.
func FormatCounter(value int, style string) string {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "", "decimal":
		return strconv.Itoa(value)
	case "decimal-leading-zero":
		return formatPadded(value)
	case "upper-roman":
		return formatRoman(value, true)
	case "lower-roman":
		return formatRoman(value, false)
	case "upper-alpha", "upper-latin":
		return formatAlpha(value, true)
	case "lower-alpha", "lower-latin":
		return formatAlpha(value, false)
	case "georgian":
		return formatAdditive(value, georgianSymbols, 1, 19999)
	case "armenian", "upper-armenian", "lower-armenian":
		return formatAdditive(value, armenianSymbols, 1, 9999)
	case "cjk-decimal":
		return formatCJKDecimal(value)
	default:
		if zero, ok := digitZeroStyle(strings.ToLower(strings.TrimSpace(style))); ok {
			return formatDigitMapped(value, zero)
		}
		return strconv.Itoa(value)
	}
}

func formatPadded(value int) string {
	sign := ""
	abs := value
	if abs < 0 {
		sign = "-"
		abs = -abs
	}
	d := strconv.Itoa(abs)
	if len(d) < 2 {
		d = "0" + d
	}
	return sign + d
}

var romanTable = []struct {
	v int
	s string
}{
	{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"},
	{100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"},
	{10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"},
}

func formatRoman(value int, upper bool) string {
	if value < 1 || value > 3999 {
		return strconv.Itoa(value)
	}
	var b strings.Builder
	for _, e := range romanTable {
		for value >= e.v {
			b.WriteString(e.s)
			value -= e.v
		}
	}
	if upper {
		return b.String()
	}
	return strings.ToLower(b.String())
}

func formatAlpha(value int, upper bool) string {
	if value < 1 {
		return strconv.Itoa(value)
	}
	var rev []byte
	for v := value; v > 0; {
		v--
		rev = append(rev, byte('A'+v%26))
		v /= 26
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	if upper {
		return string(rev)
	}
	return strings.ToLower(string(rev))
}

// additiveSymbol is one weight of an additive (non-positional) numbering
// system like Georgian or Armenian: greedy largest-first decomposition.
type additiveSymbol struct {
	v int
	s string
}

// georgianSymbols is the predefined Georgian numbering table. The archaic
// letters sit inside the sequence (8 is ჱ, 60 is ჲ, 400 is ჳ), so the weights
// do not follow the modern alphabet — the table below is read off the CSS
// css-lists WPT reference renders (counter-004, counters-004), where 19999
// renders as ჵჰშჟთ = 10000+9000+600+70+9, confirming greedy decomposition.
var georgianSymbols = []additiveSymbol{
	{10000, "ჵ"}, {9000, "ჰ"}, {8000, "ჯ"}, {7000, "ჴ"},
	{6000, "ხ"}, {5000, "ჭ"}, {4000, "წ"}, {3000, "ძ"},
	{2000, "ც"}, {1000, "ჩ"}, {900, "შ"}, {800, "ყ"},
	{700, "ღ"}, {600, "ქ"}, {500, "ფ"}, {400, "ჳ"},
	{300, "ტ"}, {200, "ს"}, {100, "რ"}, {90, "ჟ"},
	{80, "პ"}, {70, "ო"}, {60, "ჲ"}, {50, "ნ"},
	{40, "მ"}, {30, "ლ"}, {20, "კ"}, {10, "ი"},
	{9, "თ"}, {8, "ჱ"}, {7, "ზ"}, {6, "ვ"},
	{5, "ე"}, {4, "დ"}, {3, "გ"}, {2, "ბ"}, {1, "ა"},
}

// armenianSymbols is the classical Armenian numbering table, 1-9999.
var armenianSymbols = []additiveSymbol{
	{9000, "Ք"}, {8000, "Փ"}, {7000, "Ւ"}, {6000, "Ց"},
	{5000, "Ր"}, {4000, "Տ"}, {3000, "Վ"}, {2000, "Ս"},
	{1000, "Ռ"}, {900, "Ջ"}, {800, "Պ"}, {700, "Չ"},
	{600, "Ո"}, {500, "Շ"}, {400, "Ն"}, {300, "Յ"},
	{200, "Մ"}, {100, "Ճ"}, {90, "Ղ"}, {80, "Ձ"},
	{70, "Հ"}, {60, "Կ"}, {50, "Ծ"}, {40, "Խ"},
	{30, "Լ"}, {20, "Ի"}, {10, "Ժ"}, {9, "Թ"},
	{8, "Ը"}, {7, "Է"}, {6, "Զ"}, {5, "Ե"},
	{4, "Դ"}, {3, "Գ"}, {2, "Բ"}, {1, "Ա"},
}

func formatAdditive(value int, table []additiveSymbol, lo, hi int) string {
	if value < lo || value > hi {
		return strconv.Itoa(value)
	}
	var b strings.Builder
	for _, e := range table {
		for value >= e.v {
			b.WriteString(e.s)
			value -= e.v
		}
	}
	return b.String()
}

var cjkDecimalDigits = []string{"〇", "一", "二", "三", "四", "五", "六", "七", "八", "九"}

func formatCJKDecimal(value int) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	d := strconv.Itoa(value)
	var b strings.Builder
	b.WriteString(sign)
	for i := 0; i < len(d); i++ {
		b.WriteString(cjkDecimalDigits[d[i]-'0'])
	}
	return b.String()
}

// digitZeroStyle maps counter styles that substitute decimal digits
// one-for-one, to the code point of their zero. Only entries certain from the
// Unicode blocks are listed; anything else falls back to decimal.
func digitZeroStyle(style string) (rune, bool) {
	switch style {
	case "arabic-indic":
		return 0x0660, true
	case "persian":
		return 0x06F0, true
	case "bengali":
		return 0x09E6, true
	case "devanagari":
		return 0x0966, true
	case "gujarati":
		return 0x0AE6, true
	case "gurmukhi":
		return 0x0A66, true
	case "kannada":
		return 0x0CE6, true
	case "khmer":
		return 0x17E0, true
	case "lao":
		return 0x0ED0, true
	case "malayalam":
		return 0x0D66, true
	case "mongolian":
		return 0x1810, true
	case "myanmar":
		return 0x1040, true
	case "oriya":
		return 0x0B66, true
	case "tamil":
		return 0x0BE6, true
	case "telugu":
		return 0x0C66, true
	case "thai":
		return 0x0E50, true
	case "tibetan":
		return 0x0F20, true
	}
	return 0, false
}

func formatDigitMapped(value int, zero rune) string {
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	d := strconv.Itoa(value)
	var b strings.Builder
	b.WriteString(sign)
	for i := 0; i < len(d); i++ {
		b.WriteRune(zero + rune(d[i]-'0'))
	}
	return b.String()
}

// ContentPart is one piece of a parsed `content` value: a literal, a
// counter()/counters() function, or (never produced, kept for symmetry) raw.
type ContentPartKind int

const (
	ContentText ContentPartKind = iota
	ContentCounter
	ContentCounters
)

// ContentPart carries a counter() or counters() reference with its style.
type ContentPart struct {
	Kind  ContentPartKind
	Text  string // literal text for ContentText
	Name  string // counter name for Counter/Counters
	Sep   string // separator for Counters
	Style string // counter style, "" means decimal
}

// ParseContentParts splits a `content` value into literals and counter
// functions. It reports ok=false for anything it does not understand, in
// which case the caller keeps the historical raw-string behavior byte for
// byte. Pure string content never reaches here (the cascade handles quoted
// strings on its existing path); only values naming counter() or counters()
// do.
func ParseContentParts(value string) (parts []ContentPart, ok bool) {
	if !hasCounterFunc(value) {
		return nil, false
	}
	// A top-level counter()/counters() outside any string is required: a
	// quoted literal merely mentioning one stays on the plain-string path,
	// which decodes hex escapes exactly as before.
	if !hasTopLevelCounterFunc(value) {
		return nil, false
	}
	var out []ContentPart
	i := 0
	for i < len(value) {
		for i < len(value) && isSpace(value[i]) {
			i++
		}
		if i >= len(value) {
			break
		}
		c := value[i]
		if c == '"' || c == '\'' {
			raw, next, good := scanQuoted(value, i)
			if !good {
				return nil, false
			}
			out = append(out, ContentPart{Kind: ContentText, Text: css.DecodeEscapes(raw)})
			i = next
			continue
		}
		if isIdentStart(c) {
			j := i
			for j < len(value) && isIdentChar(value[j]) {
				j++
			}
			name := value[i:j]
			lower := strings.ToLower(name)
			if (lower == "counter" || lower == "counters") && j < len(value) && value[j] == '(' {
				part, next, good := parseCounterFunc(lower == "counters", value, j)
				if !good {
					return nil, false
				}
				out = append(out, part)
				i = next
				continue
			}
			return nil, false
		}
		return nil, false
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func hasCounterFunc(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "counter(") || strings.Contains(lower, "counters(")
}

// hasTopLevelCounterFunc finds counter( or counters( outside strings and
// nested parens: only those evaluate, everything else keeps raw behavior.
func hasTopLevelCounterFunc(value string) bool {
	depth := 0
	inStr := byte(0)
	for i := 0; i < len(value); i++ {
		c := value[i]
		if inStr != 0 {
			if c == '\\' {
				i++
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		switch {
		case c == '"' || c == '\'':
			inStr = c
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		case depth == 0 && isIdentStart(c):
			j := i
			for j < len(value) && isIdentChar(value[j]) {
				j++
			}
			lower := strings.ToLower(value[i:j])
			if (lower == "counter" || lower == "counters") && j < len(value) && value[j] == '(' {
				return true
			}
			i = j - 1
		}
	}
	return false
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

// isIdentStart/isIdentChar live in gradient.go and are reused here: idents
// outside ASCII fall back to raw-string behavior rather than misparsing.

// scanQuoted reads a quoted string starting at i (the quote), returning the
// raw inner text; callers decode escapes with css.DecodeEscapes, the same
// function plain string content goes through.
func scanQuoted(s string, i int) (string, int, bool) {
	q := s[i]
	var b strings.Builder
	for j := i + 1; j < len(s); j++ {
		c := s[j]
		if c == '\\' {
			if j+1 >= len(s) {
				return "", 0, false
			}
			b.WriteByte(c)
			b.WriteByte(s[j+1])
			j++
			continue
		}
		if c == q {
			return b.String(), j + 1, true
		}
		b.WriteByte(c)
	}
	return "", 0, false
}

// parseCounterFunc parses the argument list at s[i] (the opening paren).
func parseCounterFunc(plural bool, s string, i int) (ContentPart, int, bool) {
	depth := 0
	j := i
	var args []string
	cur := ""
	inStr := byte(0)
	for ; j < len(s); j++ {
		c := s[j]
		if inStr != 0 {
			cur += string(c)
			if c == '\\' && j+1 < len(s) {
				cur += string(s[j+1])
				j++
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inStr = c
			cur += string(c)
			continue
		}
		if c == '(' {
			depth++
			if depth > 1 {
				cur += string(c)
			}
			continue
		}
		if c == ')' {
			depth--
			if depth == 0 {
				break
			}
			cur += string(c)
			continue
		}
		if c == ',' && depth == 1 {
			args = append(args, strings.TrimSpace(cur))
			cur = ""
			continue
		}
		cur += string(c)
	}
	if depth != 0 {
		return ContentPart{}, 0, false
	}
	args = append(args, strings.TrimSpace(cur))
	if plural {
		if len(args) != 2 && len(args) != 3 {
			return ContentPart{}, 0, false
		}
		name := strings.TrimSpace(args[0])
		sep, ok := unquoteArg(args[1])
		if !ok || !isBareIdent(name) {
			return ContentPart{}, 0, false
		}
		style := "decimal"
		if len(args) == 3 {
			if !isBareIdent(strings.TrimSpace(args[2])) {
				return ContentPart{}, 0, false
			}
			style = strings.TrimSpace(args[2])
		}
		return ContentPart{Kind: ContentCounters, Name: name, Sep: sep, Style: style}, j + 1, true
	}
	if len(args) != 1 && len(args) != 2 {
		return ContentPart{}, 0, false
	}
	name := strings.TrimSpace(args[0])
	if !isBareIdent(name) {
		return ContentPart{}, 0, false
	}
	style := "decimal"
	if len(args) == 2 {
		if !isBareIdent(strings.TrimSpace(args[1])) {
			return ContentPart{}, 0, false
		}
		style = strings.TrimSpace(args[1])
	}
	return ContentPart{Kind: ContentCounter, Name: name, Style: style}, j + 1, true
}

func unquoteArg(s string) (string, bool) {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return css.DecodeEscapes(s[1 : len(s)-1]), true
	}
	return "", false
}

func isBareIdent(s string) bool {
	if s == "" {
		return false
	}
	if !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isIdentChar(s[i]) {
			return false
		}
	}
	return true
}

// CounterScope is one level of CSS counter state. Scopes nest: reset
// establishes a child, set and increment adjust the innermost instance or
// create the counter in a fresh child. The zero value is an empty root.
type CounterScope struct {
	parent *CounterScope
	vals   map[string]int
}

func (s *CounterScope) owner(name string) *CounterScope {
	for sc := s; sc != nil; sc = sc.parent {
		if sc.vals != nil {
			if _, ok := sc.vals[name]; ok {
				return sc
			}
		}
	}
	return nil
}

// ApplyReset establishes a fresh nested counter set. An empty declaration
// list leaves the scope untouched.
func (s *CounterScope) ApplyReset(m map[string]int) *CounterScope {
	if len(m) == 0 {
		return s
	}
	c := &CounterScope{parent: s, vals: make(map[string]int, len(m))}
	for k, v := range m {
		c.vals[k] = v
	}
	return c
}

// ApplySet assigns counters in place, creating missing ones in a fresh child
// scope so siblings sharing the parent never observe them.
func (s *CounterScope) ApplySet(m map[string]int) *CounterScope {
	cur := s
	for k, v := range m {
		if o := cur.owner(k); o != nil {
			o.vals[k] = v
			continue
		}
		if cur == s {
			cur = &CounterScope{parent: s, vals: make(map[string]int)}
		}
		cur.vals[k] = v
	}
	return cur
}

// ApplyIncrement adds to the innermost instance of each counter, creating
// missing ones at 0 first. Like set, creation happens in a fresh child.
func (s *CounterScope) ApplyIncrement(m map[string]int) *CounterScope {
	cur := s
	for k, v := range m {
		if o := cur.owner(k); o != nil {
			o.vals[k] += v
			continue
		}
		if cur == s {
			cur = &CounterScope{parent: s, vals: make(map[string]int)}
		}
		cur.vals[k] = v
	}
	return cur
}

// Lookup reports the innermost value of a counter and whether any scope owns
// one. Missing counters read as absent; evaluation renders those as 0.
func (s *CounterScope) Lookup(name string) (int, bool) {
	if o := s.owner(name); o != nil {
		return o.vals[name], true
	}
	return 0, false
}

// Levels collects a counter's value at every scope level holding one,
// outermost first, for counters().
func (s *CounterScope) Levels(name string) []int {
	var rev []int
	for sc := s; sc != nil; sc = sc.parent {
		if sc.vals != nil {
			if v, ok := sc.vals[name]; ok {
				rev = append(rev, v)
			}
		}
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// EvaluateContent renders parsed content parts against a scope. Counter
// styles default to decimal; a counter with no instance anywhere renders 0.
func EvaluateContent(parts []ContentPart, scope *CounterScope) string {
	var b strings.Builder
	for _, p := range parts {
		switch p.Kind {
		case ContentText:
			b.WriteString(p.Text)
		case ContentCounter:
			v, _ := scope.Lookup(p.Name)
			b.WriteString(FormatCounter(v, p.Style))
		case ContentCounters:
			levels := scope.Levels(p.Name)
			if len(levels) == 0 {
				levels = []int{0}
			}
			strs := make([]string, len(levels))
			for i, v := range levels {
				strs[i] = FormatCounter(v, p.Style)
			}
			b.WriteString(strings.Join(strs, p.Sep))
		}
	}
	return b.String()
}
