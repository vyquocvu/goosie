package engine

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

// Phase 1 resource caps apply per document, including all author/embedded/inline
// CSS. Geometry is CSS pixels; viewport device limits apply after scaling.
const (
	MaxDocumentBytes   = 8 << 20
	MaxDocumentNodes   = 50000
	MaxDocumentDepth   = 128
	MaxAttributes      = 64
	MaxAttributeBytes  = 65536
	MaxCSSBytes        = 4 << 20
	MaxCSSTokens       = 1 << 20
	MaxCSSRules        = 32768
	MaxCSSSelectors    = 65536
	MaxCSSDeclarations = 1 << 18
	MaxCSSNesting      = 8
	// A selector's parts and conditions are counted recursively through :not()
	// arguments, so these bound the whole tree a selector compiles to rather than
	// its visible compound count. Real stylesheets need the headroom: a MediaWiki
	// rule such as `html.x body.y:not(.z) .w table:not(.a):not(.b)` is already at
	// the old limit of 8 parts. Runaway matching is caught by MaxStyleWork, which
	// prices each selector against the elements carrying its key.
	MaxSelectorParts      = 64
	MaxSelectorConditions = 64
	// Descendant/general-sibling combinators increase matching cost linearly;
	// the old limit of 1 rejected any selector with more than one combinator.
	MaxSelectorSearches = 16
	// Matching + declaration-byte work estimate, before Resolve, priced the way the
	// matcher actually runs: each selector against the elements carrying its key.
	// Rules past it are clipped rather than failing the document.
	MaxStyleWork       = 1 << 28
	MaxFontSize        = 512
	MaxGeometry        = 1 << 20
	MaxLayoutObjects   = 131072
	MaxTextRunes       = 1 << 20
	MaxDPR             = 8
	MaxDeviceDimension = 16384
	MaxDevicePixels    = 16 * 1024 * 1024
	MaxDocumentTiles   = 65536
	MaxTileCacheBytes  = 64 << 20
)

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

// ValidateViewport must precede conversion to device int32 dimensions and any
// surface allocation. Subpixel sizes rounding to zero are not renderable.
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

// documentTiles returns the signed tile span of a document extent, including
// the origin. It uses floor division via frame.CoordFor so negative coordinates
// land in the tile that actually covers them.
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

// ValidateExtent checks a device-pixel document extent before tile-grid
// allocation. Unlike a viewport, a scrolling document can exceed 16384 pixels.
func ValidateExtent(r frame.Rect) error {
	_, err := documentTiles(r)
	return err
}

// TileCacheBudget returns the tile count and byte capacity for a document's
// tile cache. It adds 8 tiles of headroom to the document's tile count, capped
// at MaxTileCacheBytes. Empty content gets a positive cap (8 tiles). Invalid
// extents return an error with zero usable capacity.
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

// check scans actual CSS tokens before the recursive value/selector parsers.
// Raw delimiters are also capped because the current stylesheet parser treats
// delimiters inside strings/comments as structural. These are upper bounds,
// not an alternative CSS parser, and do not change cascade semantics.
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
	for {
		t := tok.Next()
		if t.Type == css.TokenEOF {
			break
		}
		b.tokens++
		if b.tokens > MaxCSSTokens {
			return fmt.Errorf("CSS token limit exceeded (%d)", MaxCSSTokens)
		}
		switch t.Type {
		case css.TokenNumber, css.TokenDimension, css.TokenPercentage:
			n, err := strconv.ParseFloat(t.Value, 64)
			if err != nil {
				// Check if this is an overflow error (value too large for float64).
				// Other parse errors are skipped and left to the CSS parser.
				if strings.Contains(err.Error(), "out of range") {
					return fmt.Errorf("CSS numeric value outside safe geometry range")
				}
				continue
			}
			if !finite(n) || math.Abs(n) > MaxGeometry || !finite(t.NumVal) || math.Abs(t.NumVal) > MaxGeometry {
				return fmt.Errorf("CSS numeric value outside safe geometry range")
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

// selectorCost includes recursively evaluated :not arguments. Search combinators
// are counted even when today's matcher would short-circuit them.
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
		// Each search combinator walks the ancestor chain once the key part
		// matches; the walk is bounded by depth, not by total node count.
		cost += int64(max(depth, 1)) * int64(searches)
	}
	return max(cost, 1), nil
}

// selectorFanOut counts the elements style resolution can test a selector
// against. The matcher buckets every selector on the id, classes and tag its key
// (rightmost) compound demands and only tries it on elements carrying one of
// those keys, so a selector's cost is set by how common its key is rather than by
// the size of the document. Summing the populations bounds the union of the
// buckets the selector is filed under, and the document's element count bounds it
// from above; a selector with no key at all, such as `*` or `[disabled]`, is tried
// against every element.
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
	// elements is the count style resolution actually visits; keyCount records how
	// many elements carry each id, class and tag, prefixed to keep the three
	// namespaces apart. The style matcher buckets selectors on their key compound,
	// so these are what a selector's matching can cost.
	elements int
	keyCount map[string]int
}

// keyID, keyClass and keyTag namespace the key counts so an id called "div"
// cannot be confused with the element of the same name.
const (
	keyID    = "i:"
	keyClass = "c:"
	keyTag   = "t:"
)

// walkDocument validates public retained DOMs iteratively before recursive work.
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
			stack = append(stack, entry{c, e.depth + 1})
		}
		stats.siblings = max(stats.siblings, siblings)
	}
	return stats, nil
}

func safeGeometry(values ...float32) bool {
	for _, v := range values {
		if !finite(float64(v)) || v < -MaxGeometry || v > MaxGeometry {
			return false
		}
	}
	return true
}

func validateStyle(s *style.ComputedStyle) error {
	if s == nil {
		return nil
	}
	if !finite(float64(s.FontSize)) || s.FontSize < 0 || s.FontSize > MaxFontSize {
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

// Bound word/run expansion before Inline allocates one object per word or space.
func (s *Session) validateLayoutInput() error {
	for id, st := range s.Styles {
		if err := validateStyle(st); err != nil {
			return fmt.Errorf("node %d: %w", id, err)
		}
	}
	objects, runes := 1, 0
	_, err := walkDocument(s.Doc, func(n *dom.Node) error {
		objects++
		if n.Type == dom.NodeText {
			// Hidden subtrees never enter inline layout.
			for p := n; p != nil; p = p.Parent {
				if st := s.Styles[p.ID]; st != nil && st.Display == style.DisplayNone {
					return nil
				}
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

// cssSource is one style sheet text and where it came from. Remote sheets are
// treated leniently - one bloated or adversarial sheet from the network skips
// itself rather than killing the document - while sheets the caller supplied
// keep the hard-fail contract the bounds tests pin down.
type cssSource struct {
	text   string
	remote bool
}

func checkedSheets(doc *dom.Document, sources []string, base string, linker CSSLinker) ([]*css.Stylesheet, error) {
	budget := &cssBudget{}
	all := make([]cssSource, 0, len(sources)+8)
	for _, src := range sources {
		all = append(all, cssSource{text: src})
	}
	linked := map[string]bool{}
	linksLeft := maxLinkedSheets
	stats, err := walkDocument(doc, func(n *dom.Node) error {
		if n.Element() && n.Data == "style" {
			all = append(all, cssSource{text: n.TextContent()})
		}
		if n.Element() && n.Data == "link" && linker != nil && linksLeft > 0 {
			if href := stylesheetHref(n); href != "" {
				if abs, ok := resolveSheetURL(base, href); ok && !linked[abs] {
					linked[abs] = true
					linksLeft--
					if sheet, err := linker(base, abs); err == nil {
						// The sheet's own @import statements are spliced in before it is parsed
						// so an imported theme cascades where the page put the statement.
						text := absolutizeCSSURLs(sheet, abs)
						all = append(all, cssSource{text: expandCSSImports(linker, abs, text, linked, 1), remote: true})
					}
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
		sheet := css.Parse(src.text)
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
			// Declarations are parsed once per sheet and applied only when the
			// rule matches, so their byte cost is not charged per node.
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
				// A selector is tried against the elements carrying its key, not
				// against the whole document.
				work += cost * selectorFanOut(sel, stats)
			}
			if bad {
				continue
			}
			// The work estimate is heuristic, so it ends the sheet where it
			// lands instead of ending the document.
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

// maxLinkedSheets bounds how many <link rel=stylesheet> fetches one document
// may trigger. Real pages rarely link more than a dozen.
const maxLinkedSheets = 32

// stylesheetHref returns the href of a link element whose rel token list
// includes stylesheet, or "" for anything else.
func stylesheetHref(n *dom.Node) string {
	isCSS := false
	for _, tok := range strings.Fields(n.GetAttribute("rel")) {
		if strings.EqualFold(tok, "stylesheet") {
			isCSS = true
		}
	}
	if !isCSS {
		return ""
	}
	return strings.TrimSpace(n.GetAttribute("href"))
}

// resolveSheetURL turns a link href into an absolute http(s) URL against the
// document's final URL. Anything the fetcher cannot dial is rejected here so
// the linker only ever sees absolute web URLs.
func resolveSheetURL(base, href string) (string, bool) {
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
	return u.String(), true
}

// absolutizeCSSURLs rewrites every relative url() reference in a fetched style
// sheet against the sheet's own absolute URL. CSS resolves subresource URLs
// against the stylesheet, not the document, so an icon declared as
// `url(icon_nav.png)` in /rAF/arcticFox.css must load from /rAF/, even though
// the page lives at a different path. data: URIs, fragments and already-
// absolute references are left untouched.
func absolutizeCSSURLs(sheet, sheetURL string) string {
	lower := strings.ToLower(sheet)
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(lower[i:], "url(")
		if j < 0 {
			b.WriteString(sheet[i:])
			return b.String()
		}
		j += i
		b.WriteString(sheet[i : j+len("url(")])
		rest := sheet[j+len("url("):]
		end := strings.IndexByte(rest, ')')
		if end < 0 {
			b.WriteString(rest)
			return b.String()
		}
		inner := rest[:end]
		trimmed := strings.TrimSpace(inner)
		var quote byte
		if len(trimmed) > 0 && (trimmed[0] == '"' || trimmed[0] == '\'') {
			quote = trimmed[0]
			trimmed = strings.Trim(trimmed[1:], string(quote))
		}
		ref := strings.TrimSpace(trimmed)
		if out, ok := rewriteURLRef(sheetURL, ref); ok {
			if quote != 0 {
				b.WriteByte(quote)
			}
			b.WriteString(out)
			if quote != 0 {
				b.WriteByte(quote)
			}
		} else {
			b.WriteString(inner)
		}
		b.WriteByte(')')
		i = j + len("url(") + end + 1
	}
}

// rewriteURLRef resolves one url() target against the sheet URL, leaving data
// URIs, fragments, empty and already-absolute references unchanged.
func rewriteURLRef(sheetURL, ref string) (string, bool) {
	if ref == "" || strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "#") {
		return "", false
	}
	if abs, ok := resolveSheetURL(sheetURL, ref); ok {
		return abs, true
	}
	return "", false
}
