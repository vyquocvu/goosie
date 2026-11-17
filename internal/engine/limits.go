package engine

import (
	"encoding/base64"
	"fmt"
	"math"
	stdnet "net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	imgdec "github.com/vyquocvu/goosie/internal/image"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

const (
	MaxDocumentBytes      = 16 << 20
	MaxDocumentNodes      = 1000000
	MaxDocumentDepth      = 128
	MaxAttributes         = 64
	MaxAttributeBytes     = 1 << 20
	MaxCSSBytes           = 4 << 20
	MaxCSSTokens          = 1 << 20
	MaxCSSRules           = 32768
	MaxCSSSelectors       = 65536
	MaxCSSDeclarations    = 1 << 18
	MaxCSSNesting         = 8
	MaxSelectorParts      = 64
	MaxSelectorConditions = 64
	MaxSelectorSearches   = 16
	MaxStyleWork          = 1 << 28
	MaxFontSize           = 512
	MaxGeometry           = 2 << 20
	MaxLayoutObjects      = 4 << 20
	MaxTextRunes          = 8 << 20
	MaxDPR                = 8
	MaxDeviceDimension    = 16384
	MaxDevicePixels       = 16 * 1024 * 1024
	MaxCanvasDimension    = MaxDeviceDimension
	MaxCanvasPixels       = MaxDevicePixels
	MaxDocumentTiles      = 524288
	MaxTileCacheBytes     = 64 << 20
)

func canvasBitmapFits(w, h int) bool {
	if w <= 0 || h <= 0 || w > MaxCanvasDimension || h > MaxCanvasDimension {
		return false
	}
	return int64(w)*int64(h) <= MaxCanvasPixels
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func validateScale(dpr float64) error {
	if !finite(dpr) || dpr <= 0 || dpr > MaxDPR {
		return fmt.Errorf("viewport DPR must be finite and in (0, %d]", MaxDPR)
	}
	return nil
}

func validateWidth(w float32) error {
	if !finite(float64(w)) || w <= 0 || w > MaxGeometry {
		return fmt.Errorf("viewport width must be finite and in (0, %d] CSS pixels", MaxGeometry)
	}
	return nil
}

func ValidateViewport(width, height int, dpr float64) error {
	if width <= 0 || height <= 0 || width > MaxGeometry || height > MaxGeometry {
		return fmt.Errorf("viewport CSS dimensions must be in (0, %d]", MaxGeometry)
	}
	if err := validateScale(dpr); err != nil {
		return err
	}
	w, h := float64(width)*dpr, float64(height)*dpr
	if !finite(w) || !finite(h) || w > MaxDeviceDimension || h > MaxDeviceDimension {
		return fmt.Errorf("viewport device dimensions exceed %d", MaxDeviceDimension)
	}
	rw, rh := math.Floor(w+0.5), math.Floor(h+0.5)
	if rw < 1 || rh < 1 {
		return fmt.Errorf("viewport rounds to zero device pixels")
	}
	if w*h > MaxDevicePixels || rw*rh > MaxDevicePixels {
		return fmt.Errorf("viewport device pixel limit exceeded (%d)", MaxDevicePixels)
	}
	return nil
}

func documentTiles(r frame.Rect) (int64, error) {
	const edge = MaxGeometry * MaxDPR
	if r.X0 < -edge || r.Y0 < -edge || r.X1 > edge || r.Y1 > edge || r.X1 < r.X0 || r.Y1 < r.Y0 {
		return 0, fmt.Errorf("unsafe document extent geometry")
	}
	if r.X0 == 0 && r.Y0 == 0 && r.X1 == 0 && r.Y1 == 0 {
		return 0, nil
	}
	lo := frame.CoordFor(frame.Point{X: min(r.X0, 0), Y: min(r.Y0, 0)}, frame.TileSize)
	hi := frame.CoordFor(frame.Point{X: max(r.X1, 0) - 1, Y: max(r.Y1, 0) - 1}, frame.TileSize)
	cols := int64(hi.Col) - int64(lo.Col) + 1
	rows := int64(hi.Row) - int64(lo.Row) + 1
	if cols <= 0 || rows <= 0 || cols > MaxDocumentTiles || rows > MaxDocumentTiles/cols {
		return 0, fmt.Errorf("document tile metadata limit exceeded (%d)", MaxDocumentTiles)
	}
	return cols * rows, nil
}

func ValidateExtent(r frame.Rect) error {
	_, err := documentTiles(r)
	return err
}

func TileCacheBudget(r frame.Rect) (tiles int, bytes int64, err error) {
	docTiles, err := documentTiles(r)
	if err != nil {
		return 0, 0, err
	}
	cacheTiles := min(docTiles+8, MaxTileCacheBytes/frame.TileSizeBytes())
	if cacheTiles <= 0 {
		cacheTiles = 8
	}
	return int(cacheTiles), cacheTiles * frame.TileSizeBytes(), nil
}

type cssBudget struct{ bytes, tokens, rules, selectors, declarations int }

func (b *cssBudget) check(src string, inline bool) error {
	if len(src) > MaxCSSBytes-b.bytes {
		return fmt.Errorf("CSS byte limit exceeded (%d)", MaxCSSBytes)
	}
	b.bytes += len(src)
	depth := 0
	for _, c := range src {
		switch c {
		case '(':
			depth++
			if depth > MaxCSSNesting {
				return fmt.Errorf("CSS nesting limit exceeded (%d)", MaxCSSNesting)
			}
		case ')':
			if depth > 0 {
				depth--
			}
		case '{':
			b.rules++
			if b.rules > MaxCSSRules {
				return fmt.Errorf("CSS rule limit exceeded (%d)", MaxCSSRules)
			}
		case ';':
			b.declarations++
			if b.declarations > MaxCSSDeclarations {
				return fmt.Errorf("CSS declaration limit exceeded (%d)", MaxCSSDeclarations)
			}
		}
	}
	tok := css.NewTokenizer(src)
	urlDepth := 0
	for {
		t := tok.Next()
		if t.Type == css.TokenEOF {
			break
		}
		b.tokens++
		if b.tokens > MaxCSSTokens {
			return fmt.Errorf("CSS token limit exceeded (%d)", MaxCSSTokens)
		}
		if t.Type == css.TokenFunction && strings.EqualFold(t.Value, "url") {
			urlDepth++
			continue
		}
		if urlDepth > 0 {
			switch t.Type {
			case css.TokenLParen:
				urlDepth++
			case css.TokenRParen:
				urlDepth--
			}
			continue
		}
		switch t.Type {
		case css.TokenNumber, css.TokenDimension, css.TokenPercentage:
			property := cssPropertyAt(src, tok.Offset())
			if property == "unicode-range" {
				continue
			}
			n, err := strconv.ParseFloat(t.Value, 64)
			if err != nil {
				if strings.Contains(err.Error(), "out of range") {
					return geometryRefusal(geometryBranchFloatOverflow, property, t.Value, t.Unit, math.NaN(), t.NumVal)
				}
				continue
			}
			if !finite(n) || !finite(t.NumVal) {
				return geometryRefusal(geometryBranchNonFinite, property, t.Value, t.Unit, n, t.NumVal)
			}
			if t.Type != css.TokenNumber && !isBorderRadiusProperty(property) &&
				(math.Abs(n) > MaxGeometry || math.Abs(t.NumVal) > MaxGeometry) {
				return geometryRefusal(geometryBranchMagnitude, property, t.Value, t.Unit, n, t.NumVal)
			}
		}
	}
	if inline {
		b.declarations++
	}
	if b.declarations > MaxCSSDeclarations {
		return fmt.Errorf("CSS declaration limit exceeded (%d)", MaxCSSDeclarations)
	}
	return nil
}

const (
	geometryBranchMagnitude     = "magnitude"
	geometryBranchFloatOverflow = "float64-overflow"
	geometryBranchNonFinite     = "non-finite"
	geometryMessageTokenLen     = 64
)

func geometryRefusal(branch, property, value, unit string, parsed, numVal float64) error {
	return fmt.Errorf("CSS numeric value outside safe geometry range: property %q, value %q, unit %q, parsed %g, numval %g, limit %d CSS pixels, branch %s",
		clipGeometryToken(property), clipGeometryToken(value), clipGeometryToken(unit),
		parsed, numVal, MaxGeometry, branch)
}

func clipGeometryToken(s string) string {
	if len(s) <= geometryMessageTokenLen {
		return s
	}
	return s[:geometryMessageTokenLen] + "..."
}

func cssPropertyAt(src string, offset int) string {
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}
	start := strings.LastIndexAny(src[:offset], "{;") + 1
	colon := strings.IndexByte(src[start:offset], ':')
	if colon < 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(src[start : start+colon]))
}

func isBorderRadiusProperty(property string) bool {
	return property == "border-radius" ||
		(strings.HasPrefix(property, "border-") && strings.HasSuffix(property, "-radius"))
}

func selectorCost(sel css.Selector, depth, siblings int) (int64, error) {
	parts, conditions, searches := 0, 0, 0
	cost := int64(0)
	var visit func(css.Selector) error
	visit = func(s css.Selector) error {
		parts += len(s.Parts)
		if parts > MaxSelectorParts {
			return fmt.Errorf("CSS selector part limit exceeded (%d)", MaxSelectorParts)
		}
		for _, p := range s.Parts {
			if p.Combinator == ' ' || p.Combinator == '~' {
				searches++
			}
			for _, c := range p.Conditions {
				conditions++
				if conditions > MaxSelectorConditions {
					return fmt.Errorf("CSS selector condition limit exceeded (%d)", MaxSelectorConditions)
				}
				cost += int64(1 + len(c.Value) + len(c.Attr))
				if c.Type == css.CondPseudoClass && c.Value == "not" {
					if err := visit(css.ParseSelector(c.Pseudo)); err != nil {
						return err
					}
				}
				if c.Type == css.CondPseudoClass && (c.Value == "first-child" || c.Value == "last-child" || c.Value == "only-child") {
					cost += int64(siblings)
				}
			}
		}
		return nil
	}
	if err := visit(sel); err != nil {
		return 0, err
	}
	if searches > MaxSelectorSearches {
		return 0, fmt.Errorf("CSS selector search limit exceeded (%d)", MaxSelectorSearches)
	}
	if searches > 0 {
		cost += int64(max(depth, 1)) * int64(searches)
	}
	return max(cost, 1), nil
}

func selectorFanOut(sel css.Selector, stats documentStats) int64 {
	if len(sel.Parts) == 0 {
		return 1
	}
	key := sel.Parts[len(sel.Parts)-1]
	sum := int64(0)
	for _, c := range key.Conditions {
		switch c.Type {
		case css.CondID:
			sum += int64(stats.keyCount[keyID+c.Value])
		case css.CondClass:
			sum += int64(stats.keyCount[keyClass+c.Value])
		case css.CondType:
			sum += int64(stats.keyCount[keyTag+c.Value])
		}
	}
	if sum < 1 {
		sum = 1
	}
	return min(sum, int64(stats.elements))
}

type documentStats struct {
	nodes, depth, siblings int
	elements               int
	keyCount               map[string]int
}

const (
	keyID    = "i:"
	keyClass = "c:"
	keyTag   = "t:"
)

func walkDocument(doc *dom.Document, visit func(*dom.Node) error) (documentStats, error) {
	var stats documentStats
	stats.keyCount = make(map[string]int)
	if doc == nil {
		return stats, fmt.Errorf("missing document")
	}
	type entry struct {
		n     *dom.Node
		depth int
	}
	stack := []entry{{&doc.Node, 0}}
	seen := make(map[*dom.Node]bool)
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[e.n] {
			return stats, fmt.Errorf("document node cycle")
		}
		seen[e.n] = true
		stats.nodes++
		if stats.nodes > MaxDocumentNodes {
			return stats, fmt.Errorf("HTML node limit exceeded (%d)", MaxDocumentNodes)
		}
		if e.depth > MaxDocumentDepth {
			return stats, fmt.Errorf("HTML tree depth limit exceeded (%d)", MaxDocumentDepth)
		}
		stats.depth = max(stats.depth, e.depth)
		if len(e.n.Attr) > MaxAttributes {
			return stats, fmt.Errorf("HTML attribute count limit exceeded (%d)", MaxAttributes)
		}
		for _, a := range e.n.Attr {
			if len(a.Name)+len(a.Value) > MaxAttributeBytes {
				return stats, fmt.Errorf("HTML attribute byte limit exceeded (%d)", MaxAttributeBytes)
			}
		}
		if e.n.Element() {
			stats.elements++
			stats.keyCount[keyTag+e.n.Data]++
			if id := e.n.GetAttribute("id"); id != "" {
				stats.keyCount[keyID+id]++
			}
			for _, cls := range e.n.ClassList() {
				stats.keyCount[keyClass+cls]++
			}
		}
		if visit != nil {
			if err := visit(e.n); err != nil {
				return stats, err
			}
		}
		siblings := 0
		for c := e.n.FirstChild; c != nil; c = c.NextSibling {
			siblings++
			if siblings > MaxDocumentNodes || c.Parent != e.n {
				return stats, fmt.Errorf("invalid document node links")
			}
		}
		for c := e.n.LastChild; c != nil; c = c.PrevSibling {
			stack = append(stack, entry{c, e.depth + 1})
		}
		stats.siblings = max(stats.siblings, siblings)
	}
	return stats, nil
}

func safeGeometry(values ...float32) bool {
	for _, v := range values {
		if v != v || v > MaxGeometry || v < -MaxGeometry {
			return false
		}
	}
	return true
}

func validateStyle(s *style.ComputedStyle) error {
	if s == nil {
		return nil
	}
	if s.FontSize != s.FontSize || s.FontSize < 0 || s.FontSize > MaxFontSize {
		return fmt.Errorf("font size must be finite and in [0, %d] CSS pixels", MaxFontSize)
	}
	if !safeGeometry(s.Width, s.Height, s.MinWidth, s.MinHeight, s.MaxWidth, s.MaxHeight,
		s.MarginTop, s.MarginRight, s.MarginBottom, s.MarginLeft, s.PaddingTop, s.PaddingRight, s.PaddingBottom, s.PaddingLeft,
		s.BorderTopWidth, s.BorderRightWidth, s.BorderBottomWidth, s.BorderLeftWidth, s.Top, s.Right, s.Bottom, s.Left,
		s.LineHeight, s.TextIndent, s.WordSpacing, s.LetterSpacing, s.FlexGrow, s.FlexShrink, s.FlexBasis, s.RowGap, s.ColumnGap, s.Opacity) {
		return fmt.Errorf("computed style geometry must be finite with absolute value <= %d CSS pixels", MaxGeometry)
	}
	return nil
}

func validateArena(a *layout.Arena) error {
	if a == nil || len(a.Objects) < 2 {
		return fmt.Errorf("missing layout arena")
	}
	if len(a.Objects) > MaxLayoutObjects {
		return fmt.Errorf("layout object limit exceeded (%d)", MaxLayoutObjects)
	}
	for i := 1; i < len(a.Objects); i++ {
		o := &a.Objects[i]
		if err := validateStyle(o.Style); err != nil {
			return fmt.Errorf("object %d: %w", i, err)
		}
		if !safeGeometry(o.X, o.Y, o.W, o.H, o.MarginTop, o.MarginRight, o.MarginBottom, o.MarginLeft,
			o.PaddingTop, o.PaddingRight, o.PaddingBottom, o.PaddingLeft, o.BorderTop, o.BorderRight, o.BorderBottom, o.BorderLeft, o.Baseline) ||
			!safeGeometry(o.ContentRect()) || !safeGeometry(o.BorderRect()) {
			return fmt.Errorf("layout object %d geometry must be finite with absolute value <= %d CSS pixels", i, MaxGeometry)
		}
	}
	return nil
}

func (s *Session) validateLayoutInput() error {
	for id, st := range s.Styles {
		if err := validateStyle(st); err != nil {
			return fmt.Errorf("node %d: %w", id, err)
		}
	}
	objects, runes := 1, 0
	hidden := make(map[dom.NodeID]bool)
	var isHidden func(n *dom.Node) bool
	isHidden = func(n *dom.Node) bool {
		if h, ok := hidden[n.ID]; ok {
			return h
		}
		h := false
		if st := s.Styles[n.ID]; st != nil && st.Display == style.DisplayNone {
			h = true
		} else if n.Parent != nil {
			h = isHidden(n.Parent)
		}
		hidden[n.ID] = h
		return h
	}
	_, err := walkDocument(s.Doc, func(n *dom.Node) error {
		objects++
		if n.Type == dom.NodeText {
			if isHidden(n) {
				return nil
			}
			inWord := false
			for _, r := range n.DataContent {
				runes++
				if unicode.IsSpace(r) {
					objects++
					inWord = false
				} else if !inWord {
					objects++
					inWord = true
				}
				if objects > MaxLayoutObjects || runes > MaxTextRunes {
					return fmt.Errorf("layout text/object limit exceeded (%d objects, %d runes)", MaxLayoutObjects, MaxTextRunes)
				}
			}
		}
		if objects > MaxLayoutObjects {
			return fmt.Errorf("layout object limit exceeded (%d)", MaxLayoutObjects)
		}
		return nil
	})
	return err
}

type cssSource struct {
	text   string
	remote bool
}

type cssTextCache struct {
	mu    sync.Mutex
	texts map[string]string
}

func (c *cssTextCache) lookup(url string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	text, ok := c.texts[url]
	return text, ok
}

func (c *cssTextCache) store(url, text string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.texts == nil {
		c.texts = make(map[string]string)
	}
	c.texts[url] = text
}

func cssSheetText(linker CSSLinker, base, abs string, sub *subBudget, cache *cssTextCache) (string, bool) {
	if text, ok := cache.lookup(abs); ok {
		return text, true
	}
	text, err := linker(base, abs)
	if err != nil {
		return "", false
	}
	if !sub.take(len(text)) {
		return "", false
	}
	cache.store(abs, text)
	return text, true
}

func checkedSheets(doc *dom.Document, sources []string, base string, linker CSSLinker, mediaOpts css.MediaOptions, sub *subBudget, cache *cssTextCache, onMixed func(MixedContentKind, string)) ([]*css.Stylesheet, error) {
	budget := &cssBudget{}
	all := make([]cssSource, 0, len(sources)+8)
	for _, src := range sources {
		all = append(all, cssSource{text: src})
	}
	type pendingLink struct {
		slot int
		abs  string
	}
	linked := map[string]bool{}
	scheduled := map[string]int{}
	var pending []pendingLink
	linksLeft := maxLinkedSheets
	stats, err := walkDocument(doc, func(n *dom.Node) error {
		if n.Element() && n.Data == "style" {
			if css.MatchMediaWithOptions(n.GetAttribute("media"), mediaOpts) {
				all = append(all, cssSource{text: n.TextContent()})
			}
		}
		if n.Element() && n.Data == "link" && linker != nil && linksLeft > 0 && !n.HasAttribute("disabled") {
			if href := stylesheetHref(n); href != "" {
				abs, resolved := resolveSubresourceURL(base, href)
				applies := css.MatchMediaWithOptions(n.GetAttribute("media"), mediaOpts)
				if !resolved {
					if applies && onMixed != nil {
						if bad, mixed := mixedContentURL(base, href); mixed {
							onMixed(MixedContentStylesheet, bad)
						}
					}
				} else if idx, ok := scheduled[abs]; ok {
					if applies && !linked[abs] {
						linked[abs] = true
						all = append(all, cssSource{})
						pending[idx].slot = len(all) - 1
					}
				} else {
					scheduled[abs] = len(pending)
					slot := -1
					if applies {
						linked[abs] = true
						all = append(all, cssSource{})
						slot = len(all) - 1
					}
					pending = append(pending, pendingLink{slot: slot, abs: abs})
					linksLeft--
				}
			}
		}
		for _, a := range n.Attr {
			if strings.EqualFold(a.Name, "style") {
				if err := budget.check(a.Value, true); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if len(pending) > 0 {
		texts := make([]*string, len(pending))
		deadline := time.Now().Add(CSSFetchBudget)
		const cssWorkers = 8
		sem := make(chan struct{}, cssWorkers)
		var wg sync.WaitGroup
		for i, p := range pending {
			if time.Now().After(deadline) {
				break
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(i int, abs string) {
				defer wg.Done()
				defer func() { <-sem }()
				if sheet, ok := cssSheetText(linker, base, abs, sub, cache); ok {
					texts[i] = &sheet
				}
			}(i, p.abs)
		}
		wg.Wait()
		for i, p := range pending {
			if texts[i] == nil || p.slot < 0 {
				continue
			}
			text := absolutizeCSSURLs(*texts[i], p.abs)
			all[p.slot] = cssSource{text: expandCSSImports(linker, p.abs, text, linked, sub, cache, 1, onMixed), remote: true}
		}
	}
	if err != nil {
		return nil, err
	}
	sheets := make([]*css.Stylesheet, 0, len(all))
	work := int64(0)
	declarations := 0
	for _, src := range all {
		if err := budget.check(src.text, false); err != nil {
			if src.remote {
				continue
			}
			return nil, err
		}
		sheet := css.ParseWithOptions(src.text, mediaOpts)
		kept := sheet.Rules[:0]
		reject := false
		for _, rule := range sheet.Rules {
			if reject {
				break
			}
			budget.selectors += len(rule.Selectors)
			declarations += len(rule.Declarations)
			if budget.selectors > MaxCSSSelectors || declarations > MaxCSSDeclarations {
				if src.remote {
					reject = true
					continue
				}
				if budget.selectors > MaxCSSSelectors {
					return nil, fmt.Errorf("CSS selector count limit exceeded (%d)", MaxCSSSelectors)
				}
				return nil, fmt.Errorf("CSS declaration limit exceeded (%d)", MaxCSSDeclarations)
			}
			declCost := int64(0)
			for _, d := range rule.Declarations {
				declCost += int64(1 + len(d.Property) + len(d.Value))
			}
			work += declCost
			bad := false
			for _, sel := range rule.Selectors {
				cost, err := selectorCost(sel, stats.depth, stats.siblings)
				if err != nil {
					if src.remote {
						bad = true
						break
					}
					return nil, err
				}
				work += cost * selectorFanOut(sel, stats)
			}
			if bad {
				continue
			}
			if work > MaxStyleWork {
				break
			}
			kept = append(kept, rule)
		}
		if reject {
			continue
		}
		sheet.Rules = kept
		sheets = append(sheets, sheet)
	}
	return sheets, nil
}

const maxSubresourceBytes = 128 << 20

const maxCSSImports = 64

type subBudget struct {
	mu   sync.Mutex
	used int64
}

func (b *subBudget) take(n int) bool {
	if b == nil {
		return true
	}
	if n < 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used+int64(n) > maxSubresourceBytes {
		return false
	}
	b.used += int64(n)
	return true
}

func (b *subBudget) Take(n int) bool {
	return b.take(n)
}

const maxLinkedSheets = 32

const CSSFetchBudget = 6 * time.Second

func stylesheetHref(n *dom.Node) string {
	isCSS := false
	alternate := false
	for _, tok := range strings.Fields(n.GetAttribute("rel")) {
		switch {
		case strings.EqualFold(tok, "stylesheet"):
			isCSS = true
		case strings.EqualFold(tok, "alternate"):
			alternate = true
		}
	}
	if !isCSS || alternate {
		return ""
	}
	return strings.TrimSpace(n.GetAttribute("href"))
}

func resolveSubresourceURL(base, href string) (string, bool) {
	b, err := url.Parse(base)
	if err != nil {
		return "", false
	}
	r, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	u := b.ResolveReference(r)
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	if b.Scheme == "https" && u.Scheme == "http" {
		return "", false
	}
	if isLocalAuthority(u.Hostname()) && !initiatorIsLocal(b) {
		return "", false
	}
	return u.String(), true
}

func initiatorIsLocal(b *url.URL) bool {
	return b.Scheme == "file" || isLocalAuthority(b.Hostname())
}

func InitiatorIsLocal(raw string) bool {
	b, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return initiatorIsLocal(b)
}

var localOnlyPrefixes = []*stdnet.IPNet{mustCIDR("0.0.0.0/8"), mustCIDR("100.64.0.0/10")}

func mustCIDR(cidr string) *stdnet.IPNet {
	_, n, err := stdnet.ParseCIDR(cidr)
	if err != nil {
		panic("engine: bad CIDR literal " + cidr)
	}
	return n
}

func isLocalAuthority(host string) bool {
	if host == "" {
		return false
	}
	if ip := stdnet.ParseIP(host); ip != nil {
		return AddressBlocked(ip)
	}
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if name == "localhost" {
		return true
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".home.arpa"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func AddressBlocked(ip stdnet.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, n := range localOnlyPrefixes {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func decodeDataImageURL(u string) ([]byte, error) {
	rest := strings.TrimPrefix(u, "data:")
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return nil, fmt.Errorf("engine: malformed data URI (no payload)")
	}
	meta, payload := rest[:comma], rest[comma+1:]
	isB64 := strings.HasSuffix(meta, ";base64")
	media := strings.TrimSuffix(meta, ";base64")
	if i := strings.IndexByte(media, ';'); i >= 0 {
		media = media[:i]
	}
	media = strings.TrimSpace(media)
	if media != "" && !strings.HasPrefix(media, "image/") {
		return nil, fmt.Errorf("engine: data URI is %q, not an image", media)
	}
	var data []byte
	if isB64 {
		payload = strings.TrimSpace(payload)
		if len(payload) > imgdec.MaxEncodedImageBytes*4/3+8 {
			return nil, fmt.Errorf("engine: data URI payload exceeds image byte limit")
		}
		var err error
		data, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			data, err = base64.RawStdEncoding.DecodeString(payload)
		}
		if err != nil {
			return nil, fmt.Errorf("engine: data URI base64: %w", err)
		}
	} else {
		if len(payload) > imgdec.MaxEncodedImageBytes*3 {
			return nil, fmt.Errorf("engine: data URI payload exceeds image byte limit")
		}
		s, err := url.PathUnescape(payload)
		if err != nil {
			return nil, fmt.Errorf("engine: data URI percent-decoding: %w", err)
		}
		data = []byte(s)
	}
	if len(data) == 0 || len(data) > imgdec.MaxEncodedImageBytes {
		return nil, fmt.Errorf("engine: data URI decodes to %d bytes, outside the image byte limit", len(data))
	}
	return data, nil
}

func absolutizeCSSURLs(sheet, sheetURL string) string {
	var b strings.Builder
	i := 0
	for i < len(sheet) {
		j := css.IndexURL(sheet[i:])
		if j < 0 {
			b.WriteString(sheet[i:])
			return b.String()
		}
		t, ok := css.ScanURLAt(sheet, i+j)
		if !ok {
			b.WriteString(sheet[i:])
			return b.String()
		}
		b.WriteString(sheet[i:t.AfterOpen])
		inner := sheet[t.AfterOpen:t.Close]
		if out, ok := rewriteURLRef(sheetURL, css.URLPayload(t)); ok {
			b.WriteString(emitURLRef(out, t.Quote))
		} else {
			b.WriteString(inner)
		}
		b.WriteByte(')')
		i = t.End
	}
	return b.String()
}

func emitURLRef(out string, quote byte) string {
	if quote != 0 {
		return string(quote) + out + string(quote)
	}
	if needsURLQuote(out) {
		return `"` + out + `"`
	}
	return out
}

func needsURLQuote(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= ' ' || c >= 0x7f {
			return true
		}
		switch c {
		case '"', '\'', '(', ')', '\\':
			return true
		}
	}
	return false
}

func rewriteURLRef(sheetURL, ref string) (string, bool) {
	if ref == "" || strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "#") {
		return "", false
	}
	if abs, ok := resolveSubresourceURL(sheetURL, ref); ok {
		return abs, true
	}
	return "", false
}
