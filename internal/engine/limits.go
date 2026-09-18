package engine

import (
	"fmt"
	"math"
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
	MaxDocumentBytes      = 8 << 20
	MaxDocumentNodes      = 10000
	MaxDocumentDepth      = 128
	MaxAttributes         = 64
	MaxAttributeBytes     = 4096
	MaxCSSBytes           = 256 << 10
	MaxCSSTokens          = 32768
	MaxCSSRules           = 512
	MaxCSSSelectors       = 1024
	MaxCSSDeclarations    = 4096
	MaxCSSNesting         = 8
	MaxSelectorParts      = 8
	MaxSelectorConditions = 16
	// Descendant/general-sibling combinators increase matching cost linearly;
	// the old limit of 1 rejected any selector with more than one combinator.
	MaxSelectorSearches = 16
	// Conservative matching + declaration-byte work estimate, before Resolve.
	MaxStyleWork       = 16 << 20
	MaxFontSize        = 512
	MaxGeometry        = 1 << 20
	MaxLayoutObjects   = 65536
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

// ValidateExtent checks a device-pixel document extent before tile-grid
// allocation. Unlike a viewport, a scrolling document can exceed 16384 pixels.
func ValidateExtent(r frame.Rect) error {
	const edge = MaxGeometry * MaxDPR
	if r.X0 < -edge || r.Y0 < -edge || r.X1 > edge || r.Y1 > edge || r.X1 < r.X0 || r.Y1 < r.Y0 {
		return fmt.Errorf("unsafe document extent geometry")
	}
	// Include the origin: binaries create their document layer from (0,0).
	w := int64(r.X1) - min(int64(r.X0), 0)
	h := int64(r.Y1) - min(int64(r.Y0), 0)
	if w < 0 || h < 0 {
		return fmt.Errorf("unsafe document extent geometry")
	}
	cols := (w + int64(frame.TileSize) - 1) / int64(frame.TileSize)
	rows := (h + int64(frame.TileSize) - 1) / int64(frame.TileSize)
	if cols*rows > MaxDocumentTiles {
		return fmt.Errorf("document tile metadata limit exceeded (%d)", MaxDocumentTiles)
	}
	return nil
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
		cost *= int64(max(depth, siblings))
	}
	return max(cost, 1), nil
}

type documentStats struct {
	nodes, depth, siblings int
}

// walkDocument validates public retained DOMs iteratively before recursive work.
func walkDocument(doc *dom.Document, visit func(*dom.Node) error) (documentStats, error) {
	var stats documentStats
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
		s.LineHeight, s.TextIndent, s.WordSpacing, s.LetterSpacing, s.FlexGrow, s.FlexShrink, s.FlexBasis, s.Gap, s.Opacity) {
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

func checkedSheets(doc *dom.Document, sources []string) ([]*css.Stylesheet, error) {
	budget := &cssBudget{}
	all := append([]string(nil), sources...)
	stats, err := walkDocument(doc, func(n *dom.Node) error {
		if n.Element() && n.Data == "style" {
			all = append(all, n.TextContent())
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
		if err := budget.check(src, false); err != nil {
			return nil, err
		}
		sheet := css.Parse(src)
		for _, rule := range sheet.Rules {
			budget.selectors += len(rule.Selectors)
			if budget.selectors > MaxCSSSelectors {
				return nil, fmt.Errorf("CSS selector count limit exceeded (%d)", MaxCSSSelectors)
			}
			declarations += len(rule.Declarations)
			if declarations > MaxCSSDeclarations {
				return nil, fmt.Errorf("CSS declaration limit exceeded (%d)", MaxCSSDeclarations)
			}
			// A rule's declarations are applied once per matched element, not
			// once per selector, so this cost stays outside the selector loop.
			// Inside it the budget grew with selector count squared and a
			// three-cell table could fail the limit.
			declCost := int64(0)
			for _, d := range rule.Declarations {
				declCost += int64(1+len(d.Property)+len(d.Value)) * int64(stats.nodes)
			}
			work += declCost
			for _, sel := range rule.Selectors {
				cost, err := selectorCost(sel, stats.depth, stats.siblings)
				if err != nil {
					return nil, err
				}
				work += cost * int64(stats.nodes)
				if work > MaxStyleWork {
					return nil, fmt.Errorf("CSS matching work limit exceeded (%d > %d)", work, MaxStyleWork)
				}
			}
		}
		sheets = append(sheets, sheet)
	}
	return sheets, nil
}
