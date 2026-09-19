package style

import (
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
)

// Display is the CSS display property value.
type Display uint8

const (
	DisplayNone Display = iota
	DisplayBlock
	DisplayInline
	DisplayInlineBlock
	DisplayFlex
	DisplayInlineFlex
	DisplayListItem
	DisplayTable
	DisplayTableRow
	DisplayTableCell
	DisplayTableRowGroup
	DisplayTableHeaderGroup
	DisplayTableFooterGroup
	DisplayTableColumn
	DisplayTableColumnGroup
	DisplayTableCaption
	DisplayGrid
	DisplayInlineGrid
	DisplayContents
)

// Position is the CSS position property value.
type Position uint8

const (
	PositionStatic Position = iota
	PositionRelative
	PositionAbsolute
	PositionFixed
	PositionSticky
)

// Overflow is the CSS overflow property value.
type Overflow uint8

const (
	OverflowVisible Overflow = iota
	OverflowHidden
	OverflowScroll
	OverflowAuto
)

// Float is the CSS float property value.
type Float uint8

const (
	FloatNone Float = iota
	FloatLeft
	FloatRight
)

// Clear is the CSS clear property value.
type Clear uint8

const (
	ClearNone Clear = iota
	ClearLeft
	ClearRight
	ClearBoth
)

// TextAlign is the CSS text-align property value.
type TextAlign uint8

const (
	TextAlignLeft TextAlign = iota
	TextAlignRight
	TextAlignCenter
	TextAlignJustify
)

// WhiteSpace is the CSS white-space property value.
type WhiteSpace uint8

const (
	WhiteSpaceNormal WhiteSpace = iota
	WhiteSpaceNowrap
	WhiteSpacePre
	WhiteSpacePrewrite
	WhiteSpacePreline
)

// FontWeight is the CSS font-weight.
type FontWeight uint16

const (
	WeightNormal FontWeight = 400
	WeightBold   FontWeight = 700
)

// FontStyle is the CSS font-style.
type FontStyle uint8

const (
	FontStyleNormal FontStyle = iota
	FontStyleItalic
	FontStyleOblique
)

// TextDecoration is the CSS text-decoration.
type TextDecoration uint8

const (
	TextDecorationNone      TextDecoration = 0
	TextDecorationUnderline TextDecoration = 1 << iota
	TextDecorationOverline
	TextDecorationLineThrough
)

// BgRepeat is the CSS background-repeat mode for a background image layer.
type BgRepeat uint8

const (
	BgRepeatRepeat BgRepeat = iota // default: tile in both axes
	BgRepeatRepeatX
	BgRepeatRepeatY
	BgRepeatNoRepeat
)

// BgSize is the CSS background-size keyword family.
type BgSize uint8

const (
	BgSizeAuto    BgSize = iota // intrinsic image size
	BgSizeContain               // largest size fitting the box, aspect kept
	BgSizeCover                 // smallest size covering the box, aspect kept
	BgSizeLength                // explicit length(s) in BgSizeW/BgSizeH
)

// BgPosMode is how one background-position axis is expressed.
type BgPosMode uint8

const (
	BgPosStart  BgPosMode = iota // left/top edge
	BgPosCenter                  // centred on that axis
	BgPosEnd                     // right/bottom edge
	BgPosLength                  // offset in px from the start edge
)

// BoxSizing is the CSS box-sizing property.
type BoxSizing uint8

const (
	BoxSizingContentBox BoxSizing = iota
	BoxSizingBorderBox
)

// ListStyleType is the CSS list-style-type.
type ListStyleType uint8

const (
	ListStyleNone ListStyleType = iota
	ListStyleDisc
	ListStyleCircle
	ListStyleSquare
	ListStyleDecimal
)

// FlexDirection is the CSS flex-direction.
type FlexDirection uint8

const (
	FlexRow FlexDirection = iota
	FlexRowReverse
	FlexColumn
	FlexColumnReverse
)

// FlexWrap is the CSS flex-wrap.
type FlexWrap uint8

const (
	FlexNowrap FlexWrap = iota
	FlexWrapValue
	FlexWrapReverse
)

// ComputedStyle holds every CSS property fully resolved to concrete values.
// Every length is float32 in CSS pixels; no parse step remains for layout.
type ComputedStyle struct {
	// Custom holds the winning values of CSS custom properties (--*), raw as
	// authored; var() references are expanded against this map when the real
	// properties apply. It is inherited, and shared read-only with the parent
	// when an element declares none of its own.
	Custom     map[string]string
	Display    Display
	Position   Position
	Float      Float
	Clear      Clear
	Overflow   Overflow
	OverflowX  Overflow
	OverflowY  Overflow
	BoxSizing  BoxSizing
	Visibility string
	Opacity    float32
	ZIndex     int32
	HasZIndex  bool

	Width     float32
	Height    float32
	MinWidth  float32
	MinHeight float32
	MaxWidth  float32
	MaxHeight float32

	MarginTop    float32
	MarginRight  float32
	MarginBottom float32
	MarginLeft   float32

	PaddingTop    float32
	PaddingRight  float32
	PaddingBottom float32
	PaddingLeft   float32

	BorderTopWidth    float32
	BorderRightWidth  float32
	BorderBottomWidth float32
	BorderLeftWidth   float32

	BorderTopStyle    string
	BorderRightStyle  string
	BorderBottomStyle string
	BorderLeftStyle   string

	BorderTopColor    css.Color
	BorderRightColor  css.Color
	BorderBottomColor css.Color
	BorderLeftColor   css.Color

	// BorderRadius is each corner's horizontal and vertical radius in the order
	// top-left, top-right, bottom-right, bottom-left. A percentage arrives still
	// encoded as one, the way every other box length does: a radius is a share of
	// the box, and the cascade has no box to measure it against.
	BorderRadius [4][2]float32

	Top    float32
	Right  float32
	Bottom float32
	Left   float32
	// The insets default to 0 in the numeric fields, which is also what the
	// initial `auto` stores. Layout has to tell "top: 0" from "top: auto" to
	// resolve an absolutely positioned box, so each side carries whether it was
	// specified.
	HasTop, HasRight, HasBottom, HasLeft bool

	Color           css.Color
	BackgroundColor css.Color
	// BackgroundGradient is a linear-gradient() taken from the background
	// shorthand or background-image, painted over the colour.
	BackgroundGradient Gradient
	// BackgroundImage is the url() layer of the background shorthand or
	// background-image, kept as authored (possibly relative); the engine
	// resolves and fetches it. Empty means no image layer.
	BackgroundImage  string
	BackgroundRepeat BgRepeat
	BackgroundSize   BgSize
	// BgSizeW/BgSizeH are the explicit background-size lengths; -1 means auto
	// for that axis. When the matching Pct flag is set the value is a fraction
	// of the positioning area (0.2 for 20%), otherwise it is CSS px. Only
	// meaningful when BackgroundSize is BgSizeLength.
	BgSizeW, BgSizeH       float32
	BgSizeWPct, BgSizeHPct bool
	// BackgroundPosX/YMode place the image within the box; when a mode is
	// BgPosLength the corresponding BackgroundPosX/Y is an offset from that
	// edge — CSS px, or a fraction of the free space when the Pct flag is set.
	BackgroundPosXMode, BackgroundPosYMode BgPosMode
	BackgroundPosX, BackgroundPosY         float32
	BgPosXPct, BgPosYPct                   bool

	// FontFamily is the family resolved from the declared font-family list:
	// the first entry the engine can serve, or the standard font when none of
	// them can be. See parseFontFamily.
	FontFamily frame.FontFamily
	FontSize   float32
	FontWeight FontWeight
	FontStyle  FontStyle
	LineHeight float32
	// LineHeightRatio is the number or percentage form of a declared
	// line-height, which is what actually inherits: each element re-derives its
	// own height from its own font size, so a 1.6 on the body gives a 32px
	// heading 51px rather than the 26px its 16px parent computed. Zero means the
	// declaration was a length, or there was none.
	LineHeightRatio float32
	// LineHeightEm is the em form of a declared line-height, held back because
	// an em is a share of this element's own *final* font size: the size may be
	// declared after the line-height, or in a rule that wins later in the
	// cascade. Like a length it never inherits - a child takes the resolved
	// px value - so it is left out of the inherit pass. Zero means none.
	LineHeightEm   float32
	TextAlign      TextAlign
	TextIndent     float32
	TextTransform  string
	TextDecoration TextDecoration
	WhiteSpace     WhiteSpace
	WordSpacing    float32
	LetterSpacing  float32

	VerticalAlign string

	ListStyleType ListStyleType

	FlexDirection  FlexDirection
	FlexWrap       FlexWrap
	JustifyContent string
	AlignItems     string
	AlignSelf      string
	FlexGrow       float32
	FlexShrink     float32
	FlexBasis      float32
	// RowGap and ColumnGap are the two axes of the `gap` shorthand and of its
	// longhands. Flexbox spends them by direction and grid by axis, so a page that
	// sets only `column-gap` - as Wikipedia's page grid does - has to keep the two
	// apart instead of treating one gap as both.
	RowGap    float32
	ColumnGap float32
	// ObjectFit selects how a replaced element's intrinsic pixels map into its
	// content box: "fill" (the default) stretches, "contain" fits whole, "cover"
	// fills and clips. Paint reads it; "" is fill.
	ObjectFit string
	// Order reorders a flex (or grid) item within its line without touching the
	// DOM. It is an integer; the CSS initial value is 0.
	Order int

	// The grid track lists stay as declared text: sizing a track needs the
	// container's own width, which the cascade does not have, so the layout pass
	// parses them.
	GridTemplateColumns string
	GridTemplateRows    string
	GridTemplateAreas   string
	GridArea            string
	GridColumnSpan      int
	GridRowSpan         int
	// Grids use the bare keywords "start" and "end" where flexbox writes
	// "flex-start" and "flex-end", so each axis keeps its own value.
	JustifyItems string
	AlignContent string

	TableLayout    string
	BorderCollapse bool
	// BorderSpacing is the gap a separated table leaves between and around its
	// cells. `collapse` ignores both values.
	BorderSpacingH float32
	BorderSpacingV float32

	// Content is the CSS content property for ::before/::after pseudo-elements.
	// "normal" (default) or "none" means no generated content; any other string
	// is the text to generate.
	Content string
}

// AnonymousBlockStyle is the style of the block box layout invents to hold
// inline content that shares a container with block-level children: the parent's
// typography, with the box properties that belong to the parent rather than to
// the content inside it dropped.
func AnonymousBlockStyle(parent *ComputedStyle) *ComputedStyle {
	var s ComputedStyle
	if parent == nil {
		s = DefaultStyle()
	} else {
		s = *parent
	}
	s.Display = DisplayBlock
	s.Position = PositionStatic
	s.Width = -1
	s.Height = -1
	s.MinWidth, s.MinHeight, s.MaxWidth, s.MaxHeight = 0, 0, -1, -1
	s.MarginTop, s.MarginRight, s.MarginBottom, s.MarginLeft = 0, 0, 0, 0
	s.PaddingTop, s.PaddingRight, s.PaddingBottom, s.PaddingLeft = 0, 0, 0, 0
	s.BorderTopWidth, s.BorderRightWidth, s.BorderBottomWidth, s.BorderLeftWidth = 0, 0, 0, 0
	// The parent's background already covers this area; painting it twice would
	// double the alpha of a translucent box.
	s.BackgroundColor = css.Color{}
	return &s
}

// DefaultStyle returns the initial computed style with CSS defaults.
func DefaultStyle() ComputedStyle {
	return ComputedStyle{
		Display:    DisplayInline,
		Position:   PositionStatic,
		Opacity:    1.0,
		Width:      -1,
		Height:     -1,
		MinWidth:   0,
		MinHeight:  0,
		MaxWidth:   -1,
		MaxHeight:  -1,
		Color:      css.Color{R: 0, G: 0, B: 0, A: 255},
		FontSize:   16,
		FontWeight: WeightNormal,
		// -1 means unset: a declared line-height resolves to a px value in
		// resolveLineHeight, but the initial value cannot be one because a
		// unitless multiplier recomputes per element's font size. Consumers
		// fall back to fontSize * 1.2 when they see a value at or below zero.
		LineHeight: -1,
		// Serif, not the embedded sans: a document that names no family is
		// drawn in the engine's standard font, and Blink's on macOS is Times
		// New Roman. Matching that is most of the difference in where text
		// wraps.
		FontFamily: frame.FontTimes,
		// A flex item gives back main-axis space it cannot keep unless the author
		// says otherwise, so shrink defaults to 1 while grow defaults to 0.
		FlexShrink: 1,
		Visibility: "visible",
		WhiteSpace: WhiteSpaceNormal,
		FlexBasis:  -1,
		Content:    "normal",
	}
}

// FontSlot is the face a computed style draws with: the resolved family at the
// resolved weight and slant.
//
// Layout measures with this and paint draws with it, which is the only way a
// word's width and its glyphs can't disagree. CSS weights are coarser here than
// in a browser with a variable font - this engine loads one regular and one bold
// file per family - so anything at or above 700 is the bold face. Oblique takes
// the italic face rather than synthesizing a slant.
func (cs *ComputedStyle) FontSlot() frame.FontSlot {
	if cs == nil {
		return frame.FontSlot{}
	}
	return frame.FontSlot{
		Family: cs.FontFamily,
		Bold:   cs.FontWeight >= WeightBold,
		Italic: cs.FontStyle != FontStyleNormal,
		Light:  cs.FontWeight > 0 && cs.FontWeight < WeightNormal,
	}
}

// UserAgentStylesheet returns the default UA stylesheet. It is parsed once and
// the same pointer comes back every time, which is how the cascade recognises
// UA rules: origin outranks specificity, so an author `*` declaration beats a
// UA element rule no matter how the two score on (id, class, type).
func UserAgentStylesheet() *css.Stylesheet {
	uaSheetOnce.Do(func() { uaSheet = css.Parse(uaCSS) })
	return uaSheet
}

var (
	uaSheetOnce sync.Once
	uaSheet     *css.Stylesheet
)

var uaCSS = `
input, button, select, textarea, table {
	box-sizing: border-box;
}
html { display: block; }
head { display: none; }
style, script, title, meta, link, template, noscript { display: none; }
body { display: block; margin: 8px; }
div { display: block; }
center { display: block; text-align: center; }
span { display: inline; }
p { display: block; margin-top: 1em; margin-bottom: 1em; }
h1 { display: block; font-size: 2em; font-weight: 700; margin-top: 0.67em; margin-bottom: 0.67em; }
h2 { display: block; font-size: 1.5em; font-weight: 700; margin-top: 0.83em; margin-bottom: 0.83em; }
h3 { display: block; font-size: 1.17em; font-weight: 700; margin-top: 1em; margin-bottom: 1em; }
h4 { display: block; font-weight: 700; margin-top: 1.33em; margin-bottom: 1.33em; }
h5 { display: block; font-size: 0.83em; font-weight: 700; margin-top: 1.67em; margin-bottom: 1.67em; }
h6 { display: block; font-size: 0.67em; font-weight: 700; margin-top: 2.33em; margin-bottom: 2.33em; }
ul, ol { display: block; margin-top: 1em; margin-bottom: 1em; padding-left: 40px; }
ul { list-style-type: disc; }
ol { list-style-type: decimal; }
li { display: list-item; }
a { color: #0000ee; text-decoration: underline; }
table { display: table; border-collapse: separate; border-spacing: 2px; }
thead { display: table-header-group; }
tbody { display: table-row-group; }
tr { display: table-row; }
td { display: table-cell; padding: 1px; }
th { display: table-cell; padding: 1px; font-weight: 700; }
caption { display: table-caption; text-align: center; }
colgroup { display: table-column-group; }
col { display: table-column; }
pre { display: block; white-space: pre; font-family: monospace; margin: 1em 0; }
code, kbd, samp, tt { font-family: monospace; }
blockquote { display: block; margin: 1em 40px; }
hr { display: block; margin-top: 0.5em; margin-bottom: 0.5em; border-top: 1px solid; }
br { display: inline; }
img { display: inline; }
b, strong { font-weight: 700; }
i, em { font-style: italic; }
u { text-decoration: underline; }
small { font-size: 0.83em; }
sub { font-size: 0.83em; vertical-align: sub; }
sup { font-size: 0.83em; vertical-align: super; }
header, footer { display: block; }
nav { display: block; }
main { display: block; }
section { display: block; }
article { display: block; }
aside { display: block; }
figure { display: block; margin: 1em 40px; }
figcaption { display: block; }
details { display: block; }
summary { display: block; }
/* A closed <details> discloses nothing but its summary: the rest of its children
   are hidden by the widget, not by the author, so the rule belongs to the UA
   sheet. Every story row on lobste.rs carries a dropdown menu this left in the
   flow, adding a line box per row. */
details:not([open]) > *:not(summary) { display: none; }
dialog { display: none; }
fieldset { display: block; margin: 0 2px; padding: 0.35em 0.75em 0.625em; border: 2px groove; }
legend { display: block; padding: 0 0.25em; }
form { display: block; }
label { display: inline; }
select, textarea { display: inline; }
/* A text or tick control is a replaced box: Chrome gives it a box of its own
   whose size comes from the control rather than from content. The other input
   types shrink to a label the engine does not read out of an attribute, or draw
   a widget it cannot reproduce, so they stay inline and take no box. */
input { display: inline-block; background-color: #ffffff; }
input[type=submit], input[type=reset], input[type=button], input[type=image],
input[type=file], input[type=date], input[type=time], input[type=datetime-local],
input[type=datetime], input[type=month], input[type=week], input[type=color],
input[type=range] { display: inline; }
input[type=hidden] { display: none; }
/* Chrome lays a button out as an inline-level box that shrinks to its label, so
   a row of buttons sits side by side. */
button { display: inline-block; text-align: center; }
dl { display: block; margin-top: 1em; margin-bottom: 1em; }
dt { display: block; font-weight: 700; }
dd { display: block; margin-left: 40px; }
address { display: block; font-style: italic; }
/* A list hanging off another list loses its block-axis margins: the parent's
   indentation survives, only the blank line around the child goes. */
dir dir, dir dl, dir menu, dir ol, dir ul,
dl dir, dl dl, dl menu, dl ol, dl ul,
menu dir, menu dl, menu menu, menu ol, menu ul,
ol dir, ol dl, ol menu, ol ol, ol ul,
ul dir, ul dl, ul menu, ul ol, ul ul { margin-top: 0; margin-bottom: 0; }
`

// Viewport is the size a document is laid out in. The viewport units are a share
// of it, and nothing else in the cascade depends on the frame's size.
type Viewport struct {
	W, H float32
}

// inViewport rewrites a declaration's viewport units as the pixels they stand
// for. Resolving them here, where the frame's size is known, is what keeps vw/vh
// out of every consumer downstream; a declaration with no viewport to speak of
// keeps the fallback the value parser assumed.
func inViewport(d css.Declaration, vp Viewport) css.Declaration {
	if vp.W <= 0 || vp.H <= 0 {
		return d
	}
	v := d.Parsed
	if v.Type == css.ValueLength {
		switch v.Unit {
		case "vw", "vh", "vmin", "vmax", "svw", "svh", "dvw", "dvh", "lvw", "lvh":
		default:
			return d
		}
		d.Parsed = css.Value{Type: css.ValueLength, Num: float64(v.ToLengthWithContext(0, vp.W, vp.H)), Unit: "px"}
		return d
	}
	// Every other value keeps its authored text on the way to the property switch,
	// and a shorthand's arguments only exist as that text, so the viewport terms
	// are rewritten there and the parsed form rebuilt from it. Both copies have to
	// agree: one resolved and the not-yet-resolved is enough to make a property
	// disagree with itself.
	text, ok := css.ResolveViewportUnits(d.Value, vp.W, vp.H)
	if !ok {
		return d
	}
	d.Value = text
	d.Parsed = css.ParseValue(text)
	return d
}

// Resolve computes the style for every element in a document laid out in an
// unspecified viewport, which is what the viewport units then fall back to.
func Resolve(doc *dom.Document, sheets []*css.Stylesheet) map[dom.NodeID]*ComputedStyle {
	return ResolveViewport(doc, sheets, Viewport{})
}

// ResolveViewport computes the style for every element in a document laid out in
// the given viewport.
func ResolveViewport(doc *dom.Document, sheets []*css.Stylesheet, vp Viewport) map[dom.NodeID]*ComputedStyle {
	allSheets := []*css.Stylesheet{UserAgentStylesheet()}
	allSheets = append(allSheets, sheets...)

	index := buildRuleIndex(allSheets)
	result := make(map[dom.NodeID]*ComputedStyle)
	for c := doc.Node.FirstChild; c != nil; c = c.NextSibling {
		resolveNode(c, index, result, vp)
	}
	return result
}

func resolveNode(n *dom.Node, index *ruleIndex, result map[dom.NodeID]*ComputedStyle, vp Viewport) {
	if n == nil {
		return
	}
	if n.Element() {
		parentStyle := findParentStyle(n, result)
		cs := computeStyle(n, index, parentStyle, vp)
		result[n.ID] = cs
	} else if n.Type == 2 {
		parentStyle := findParentStyle(n, result)
		if parentStyle != nil {
			textStyle := inheritStyle(parentStyle)
			result[n.ID] = textStyle
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		resolveNode(c, index, result, vp)
	}
}

func findParentStyle(n *dom.Node, result map[dom.NodeID]*ComputedStyle) *ComputedStyle {
	for p := n.Parent; p != nil; p = p.Parent {
		if cs, ok := result[p.ID]; ok {
			return cs
		}
	}
	return nil
}

func inheritStyle(parent *ComputedStyle) *ComputedStyle {
	cs := *parent
	cs.Display = DisplayInline
	return &cs
}

// cascadeEntry is one declaration plus the origin and specificity it arrived
// under. A declaration's value survives only by beating every other declaration
// for the same property, and the order that decides it is CSS's own: importance
// splits the list into two passes, then origin outranks specificity, and
// specificity outranks document order.
type cascadeEntry struct {
	decl    css.Declaration
	origin  int
	a, b, c int
	order   int
}

func (e cascadeEntry) before(o cascadeEntry) bool {
	if e.origin != o.origin {
		return e.origin < o.origin
	}
	if e.a != o.a {
		return e.a < o.a
	}
	if e.b != o.b {
		return e.b < o.b
	}
	if e.c != o.c {
		return e.c < o.c
	}
	return e.order < o.order
}

func sortCascade(entries []cascadeEntry) {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].before(entries[j]) })
}

func computeStyle(n *dom.Node, index *ruleIndex, parent *ComputedStyle, vp Viewport) *ComputedStyle {
	cs := DefaultStyle()
	if parent != nil {
		inheritFromParent(&cs, parent)
	}

	parentFontSize := float32(16)
	if parent != nil {
		parentFontSize = parent.FontSize
	}

	var normal, important []cascadeEntry
	var allDecls []css.Declaration
	order := index.declCount
	// Candidates arrive in sheet, rule and selector order, so the cascade sees the
	// same sequence it would from walking every sheet: the index narrows which
	// selectors get tested, never how they rank. Declarations that share a
	// selector share its rank and keep their relative order through the stable sort.
	for _, cand := range index.candidates(n) {
		if !cand.sel.Matches(n) {
			continue
		}
		for _, d := range cand.rule.Declarations {
			e := cascadeEntry{decl: inViewport(d, vp), origin: cand.origin, a: cand.a, b: cand.b, c: cand.c, order: cand.rank}
			if d.Important {
				important = append(important, e)
			} else {
				normal = append(normal, e)
			}
		}
		allDecls = append(allDecls, cand.rule.Declarations...)
	}

	if inline := n.GetAttribute("style"); inline != "" {
		decls := parseInlineDeclarations(inline)
		for _, d := range decls {
			e := cascadeEntry{decl: inViewport(d, vp), origin: 2, a: 1, b: 1, c: 1, order: order}
			order++
			if d.Important {
				important = append(important, e)
			} else {
				normal = append(normal, e)
			}
		}
		allDecls = append(allDecls, decls...)
	}

	// Legacy presentational attributes (bgcolor, color) are author-origin hints
	// with zero specificity: they beat the UA default but lose to any real
	// author rule for the same property. Pages like news.ycombinator.com paint
	// their whole header bar through <td bgcolor>, with no CSS fallback.
	for _, d := range presentationalHints(n) {
		normal = append(normal, cascadeEntry{decl: d, origin: 1, a: 0, b: 0, c: 0, order: -1})
		allDecls = append(allDecls, d)
	}

	sortCascade(normal)
	sortCascade(important)
	cs.Custom = resolveCustom(parent, normal, important)
	vars := newVarExpander(cs.Custom)
	for _, e := range normal {
		applyCascadeEntry(&cs, e, parentFontSize, vars)
	}
	for _, e := range important {
		applyCascadeEntry(&cs, e, parentFontSize, vars)
	}

	// A font-relative length is a share of the element's own font size, and that
	// size is only final now that every rule has spoken. Re-apply the same
	// declarations in the same order so `width: 22em` reads 14.08px from a sibling
	// `font-size: 88%` no matter which of the two the author wrote first.
	for _, e := range normal {
		if againstOwnFont(e.decl.Property) {
			applyCascadeEntry(&cs, e, parentFontSize, vars)
		}
	}
	for _, e := range important {
		if againstOwnFont(e.decl.Property) {
			applyCascadeEntry(&cs, e, parentFontSize, vars)
		}
	}

	// An inherited `text-align: center` stops at a table. HTML's <center> - and
	// any author rule that centres a block - reaches the text inside it, but the
	// table algorithm resets the alignment at its own boundary: measured against
	// Chromium, `<center><table><tr><td>TEXT` puts TEXT at the cell's start edge,
	// while `<td style="text-align:center">` still centres it. Without the reset
	// every cell of a centred table - Hacker News is one - drifts right by half
	// the leftover line.
	if cs.TextAlign == TextAlignCenter && cs.Display == DisplayTable && !declaresTextAlign(normal, important) {
		cs.TextAlign = TextAlignLeft
	}

	// A flex or grid item has its inline-level display blockified (Flexbox §4,
	// Grid §8.1). The item is the box the container sizes, so one left inline is
	// measured across no width at all: every word of a sentence lands on its own
	// line and the container's height becomes the sum of them. An abspos or fixed
	// child is not an item, so it keeps its own display.
	if parent != nil && cs.Position != PositionAbsolute && cs.Position != PositionFixed {
		switch parent.Display {
		case DisplayFlex, DisplayInlineFlex, DisplayGrid, DisplayInlineGrid:
			switch cs.Display {
			case DisplayInline, DisplayInlineBlock:
				cs.Display = DisplayBlock
			case DisplayInlineFlex:
				cs.Display = DisplayFlex
			case DisplayInlineGrid:
				cs.Display = DisplayGrid
			}
		}
	}

	// A float is blockified the same way (CSS 2.1 §9.7, Display §3.2): `<img
	// style="float:left">` and `<span style="float:right">note</span>` both take a
	// box of their own that the block flow places against one edge. Without it the
	// box stays inline-level, joins its siblings' run, and never reaches the side
	// it was thrown to.
	if cs.Float != FloatNone {
		switch cs.Display {
		case DisplayInline, DisplayInlineBlock, DisplayInlineFlex:
			cs.Display = DisplayBlock
		case DisplayInlineGrid:
			cs.Display = DisplayGrid
		}
	}

	// A unitless or percentage line-height is a share of the element's own font
	// size, so it only resolves once that size is final.
	if cs.LineHeightRatio > 0 {
		cs.LineHeight = cs.LineHeightRatio * cs.FontSize
	}
	// So does an em length: `line-height: 1em` before `font-size: 22px` in the
	// same rule is still 22px of line.
	if cs.LineHeightEm > 0 {
		cs.LineHeight = cs.LineHeightEm * cs.FontSize
	}

	resolveCurrentColor(&cs, allDecls)

	return &cs
}

// presentationalHints maps the legacy HTML presentational attributes onto their
// CSS equivalents. They are author-origin hints with zero specificity: they beat
// the UA default but lose to any real author rule for the same property. Only
// values that parse produce a declaration; anything else is left to the normal
// cascade.
func presentationalHints(n *dom.Node) []css.Declaration {
	var out []css.Declaration
	add := func(prop, v string) {
		if v == "" {
			return
		}
		if _, ok := css.ParseColor(v); !ok {
			return
		}
		out = append(out, css.Declaration{Property: prop, Value: v, Parsed: css.ParseValue(v)})
	}
	// A presentational length is a pixel count with the unit omitted, or a
	// percentage: `width="85%"` and `width="600"` are both valid markup.
	addLen := func(prop, v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if !strings.HasSuffix(v, "%") {
			f, err := strconv.ParseFloat(v, 32)
			if err != nil || f < 0 {
				return
			}
			v = strconv.FormatFloat(f, 'f', -1, 32) + "px"
		}
		parsed := css.ParseValue(v)
		if parsed.Type != css.ValueLength && parsed.Type != css.ValuePercentage {
			return
		}
		out = append(out, css.Declaration{Property: prop, Value: v, Parsed: parsed})
	}
	add("background-color", n.GetAttribute("bgcolor"))
	add("color", n.GetAttribute("color"))
	addLen("width", n.GetAttribute("width"))
	addLen("height", n.GetAttribute("height"))
	addLen("border-spacing", n.GetAttribute("cellspacing"))
	// `cellpadding` is declared on the table and paid for by its cells, and
	// `align` on a cell is its text alignment.
	if n.Data == "td" || n.Data == "th" {
		for p := n.Parent; p != nil; p = p.Parent {
			if p.Data == "table" {
				addLen("padding", p.GetAttribute("cellpadding"))
				break
			}
		}
		switch v := strings.ToLower(strings.TrimSpace(n.GetAttribute("align"))); v {
		case "left", "right", "center", "justify":
			out = append(out, css.Declaration{Property: "text-align", Value: v, Parsed: css.ParseValue(v)})
		}
	}
	return out
}

// declaresTextAlign reports whether `text-align` is one of the element's own
// declarations - from a matched rule or a style attribute - as opposed to the
// value it inherited.
func declaresTextAlign(normal, important []cascadeEntry) bool {
	for _, list := range [][]cascadeEntry{normal, important} {
		for _, e := range list {
			if e.decl.Property == "text-align" {
				return true
			}
		}
	}
	return false
}

// resolveCustom collects the winning --* declarations in cascade order on top
// of the inherited set. Elements that declare none share the parent's map
// read-only; anything that mutates must copy first.
func resolveCustom(parent *ComputedStyle, normal, important []cascadeEntry) map[string]string {
	declares := false
	for _, es := range [][]cascadeEntry{normal, important} {
		for _, e := range es {
			if strings.HasPrefix(e.decl.Property, "--") {
				declares = true
			}
		}
	}
	if !declares {
		if parent != nil {
			return parent.Custom
		}
		return nil
	}
	m := map[string]string{}
	if parent != nil {
		for k, v := range parent.Custom {
			m[k] = v
		}
	}
	for _, es := range [][]cascadeEntry{normal, important} {
		for _, e := range es {
			if strings.HasPrefix(e.decl.Property, "--") {
				m[e.decl.Property] = e.decl.Value
			}
		}
	}
	return m
}

// applyCascadeEntry applies one winning declaration, expanding var()
// references first. A custom property is not a real property: its value lives
// in cs.Custom and never reaches applyProperty.
func applyCascadeEntry(cs *ComputedStyle, e cascadeEntry, parentFontSize float32, vars *varExpander) {
	if strings.HasPrefix(e.decl.Property, "--") {
		return
	}
	value, parsed := e.decl.Value, e.decl.Parsed
	if strings.Contains(value, "var(") {
		value = vars.expand(value)
		if strings.TrimSpace(value) == "" {
			// Nothing resolved and no fallback was given: the declaration is
			// invalid at computed-value time, so the property keeps the value it
			// would have had without it. Parsing the empty string instead would
			// store a zero - a transparent colour, no border, no background.
			return
		}
		parsed = css.ParseValue(value)
	}
	applyProperty(cs, e.decl.Property, value, parsed, parentFontSize)
}

// maxVarSubstitutions bounds how much a single element may expand. It replaces
// the old recursion-depth cap, which neither bounded total work nor stopped a
// self-referential property from looping.
const maxVarSubstitutions = 4096

// varExpander resolves var() references against one element's custom
// properties.
//
// Every property is expanded at most once and the result reused. Without that,
// a reference re-walks its property's whole definition tree, and a sheet that
// chains custom properties the way Wikipedia's does costs minutes per element.
// A property already being expanded is reported as missing, so a definition
// that reaches itself falls back or collapses instead of looping forever.
type varExpander struct {
	custom   map[string]string
	done     map[string]string
	visiting map[string]bool
	budget   int
}

func newVarExpander(custom map[string]string) *varExpander {
	return &varExpander{custom: custom, budget: maxVarSubstitutions}
}

// expand substitutes every var() call in value, leaving the text alone once the
// element's budget is spent.
func (e *varExpander) expand(value string) string {
	for e.budget > 0 {
		i := strings.Index(value, "var(")
		if i < 0 {
			return value
		}
		paren := 1
		j := i + 4
		for j < len(value) {
			switch value[j] {
			case '(':
				paren++
			case ')':
				paren--
			}
			if paren == 0 {
				break
			}
			j++
		}
		if paren != 0 {
			return value[:i]
		}
		e.budget--
		inner := value[i+4 : j]
		repl := ""
		name, fallback, hasFallback := inner, "", false
		if comma := strings.IndexByte(inner, ','); comma >= 0 {
			name, fallback, hasFallback = strings.TrimSpace(inner[:comma]), strings.TrimSpace(inner[comma+1:]), true
		} else {
			name = strings.TrimSpace(name)
		}
		if v, ok := e.resolve(name); ok {
			repl = v
		} else if hasFallback {
			repl = e.expand(fallback)
		}
		// A resolved value is var-free by construction, so each pass retires one
		// reference and the loop always terminates.
		value = value[:i] + repl + value[j+1:]
	}
	return value
}

// varInvalid marks a custom property that takes part in a reference cycle. CSS
// treats such a property as guaranteed-invalid, which lets a var() using it fall
// back rather than substitute an empty value.
const varInvalid = "\x00cycle"

// resolve returns the fully expanded value of one custom property.
func (e *varExpander) resolve(name string) (string, bool) {
	if e.done == nil {
		e.done = make(map[string]string)
		e.visiting = make(map[string]bool)
	}
	if v, ok := e.done[name]; ok {
		return v, v != varInvalid
	}
	raw, ok := e.custom[name]
	if !ok {
		return "", false
	}
	if e.visiting[name] {
		e.done[name] = varInvalid
		return "", false
	}
	e.visiting[name] = true
	v := e.expand(raw)
	delete(e.visiting, name)
	// A cycle deeper in this property's definition tree poisoned this name on the
	// way through; it stays invalid so the reference above falls back.
	if existing, bad := e.done[name]; bad && existing == varInvalid {
		return "", false
	}
	e.done[name] = v
	return v, true
}

// clampFontSize rescues a non-finite or non-positive computed font size (NaN
// from an unparseable length) back to the medium default. Oversized finite
// values stay on the rejection path the engine's bounds contract pins.
func clampFontSize(f float32) float32 {
	if !(f > 0) {
		return 16
	}
	return f
}

func inheritFromParent(cs *ComputedStyle, parent *ComputedStyle) {
	cs.Color = parent.Color
	cs.FontSize = parent.FontSize
	cs.FontFamily = parent.FontFamily
	cs.FontWeight = parent.FontWeight
	cs.FontStyle = parent.FontStyle
	cs.LineHeight = parent.LineHeight
	cs.LineHeightRatio = parent.LineHeightRatio
	cs.TextAlign = parent.TextAlign
	cs.Visibility = parent.Visibility
	cs.WhiteSpace = parent.WhiteSpace
	cs.WordSpacing = parent.WordSpacing
	cs.LetterSpacing = parent.LetterSpacing
	cs.TextIndent = parent.TextIndent
	cs.TextTransform = parent.TextTransform
	cs.TextDecoration = parent.TextDecoration
	cs.VerticalAlign = parent.VerticalAlign
	cs.ListStyleType = parent.ListStyleType
}

func applyInlineStyle(cs *ComputedStyle, inline string, parentFontSize float32) {
	if inline == "" {
		return
	}
	decls := parseInlineDeclarations(inline)
	applyDeclarations(cs, decls, 1, 0, 0, true, parentFontSize)
}

func parseInlineDeclaration(s string) css.Declaration {
	colonIdx := strings.Index(s, ":")
	if colonIdx < 0 {
		return css.Declaration{}
	}
	prop := strings.TrimSpace(s[:colonIdx])
	val := strings.TrimSpace(s[colonIdx+1:])
	important := false
	if idx := strings.Index(strings.ToLower(val), "!important"); idx >= 0 {
		important = true
		val = strings.TrimSpace(val[:idx])
	}
	return css.Declaration{
		Property:  strings.ToLower(prop),
		Value:     val,
		Parsed:    css.ParseValue(val),
		Important: important,
	}
}

func parseInlineDeclarations(s string) []css.Declaration {
	var decls []css.Declaration
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		decls = append(decls, parseInlineDeclaration(part))
	}
	return decls
}

type specificity struct {
	a, b, c int
	inline  bool
}

func applyDeclarations(cs *ComputedStyle, decls []css.Declaration, a, b, c int, inline bool, parentFontSize float32) {
	// Font-relative lengths resolve against the element's own computed font size,
	// so a rule's `width: 22em` cannot be laid down before the `font-size: 88%`
	// that gives it meaning. Seed the size from the last font declaration the rule
	// carries, then let the in-order pass below state everything, including that
	// size again, so nothing but the seed order changes.
	for i := len(decls) - 1; i >= 0; i-- {
		if p := decls[i].Property; p == "font-size" || p == "font" {
			applyProperty(cs, p, decls[i].Value, decls[i].Parsed, parentFontSize)
			break
		}
	}
	for _, d := range decls {
		applyProperty(cs, d.Property, d.Value, d.Parsed, parentFontSize)
	}
}

func applyProperty(cs *ComputedStyle, prop, value string, parsed css.Value, parentFontSize float32) {
	switch prop {
	case "display":
		cs.Display = parseDisplay(value)
	case "position":
		cs.Position = parsePosition(value)
	case "float":
		cs.Float = parseFloatProp(value)
	case "clear":
		cs.Clear = parseClear(value)
	case "overflow":
		cs.Overflow = parseOverflowProp(value)
		cs.OverflowX = cs.Overflow
		cs.OverflowY = cs.Overflow
	case "overflow-x":
		cs.OverflowX = parseOverflowProp(value)
	case "overflow-y":
		cs.OverflowY = parseOverflowProp(value)
	case "box-sizing":
		if value == "border-box" {
			cs.BoxSizing = BoxSizingBorderBox
		}
	case "visibility":
		cs.Visibility = value
	case "content":
		// Content takes the parsed string value (quotes stripped, escapes decoded)
		// rather than the raw CSS text, so the generated text is what the author wrote.
		if parsed.Type == css.ValueString {
			cs.Content = parsed.Str
		} else {
			cs.Content = value
		}
	case "opacity":
		cs.Opacity = float32(parsed.Num)
	case "z-index":
		if value == "auto" {
			cs.HasZIndex = false
		} else {
			cs.HasZIndex = true
			cs.ZIndex = int32(parsed.Num)
		}

	// Font-relative units in a box size resolve against the element's own font,
	// the same way margins already do: `width:10em` on a 20px element is 200px.
	case "width":
		cs.Width = resolveLengthEm(parsed, -1, cs.FontSize)
	case "height":
		cs.Height = resolveLengthEm(parsed, -1, cs.FontSize)
	case "min-width":
		cs.MinWidth = resolveLengthEm(parsed, 0, cs.FontSize)
	case "min-height":
		cs.MinHeight = resolveLengthEm(parsed, 0, cs.FontSize)
	case "max-width":
		cs.MaxWidth = resolveMaxLength(parsed, cs.FontSize)
	case "max-height":
		cs.MaxHeight = resolveMaxLength(parsed, cs.FontSize)

	case "margin":
		t, r, b, l := parseMarginShorthand(value, cs.FontSize)
		cs.MarginTop = t
		cs.MarginRight = r
		cs.MarginBottom = b
		cs.MarginLeft = l
	case "margin-top":
		cs.MarginTop = resolveLengthEm(parsed, 0, cs.FontSize)
	case "margin-right":
		cs.MarginRight = resolveLengthEm(parsed, MarginAuto, cs.FontSize)
	case "margin-bottom":
		cs.MarginBottom = resolveLengthEm(parsed, 0, cs.FontSize)
	case "margin-left":
		cs.MarginLeft = resolveLengthEm(parsed, MarginAuto, cs.FontSize)

	case "padding":
		t, r, b, l := parsePaddingShorthand(value, cs.FontSize)
		cs.PaddingTop = t
		cs.PaddingRight = r
		cs.PaddingBottom = b
		cs.PaddingLeft = l
	case "padding-top":
		cs.PaddingTop = resolveLengthEm(parsed, 0, cs.FontSize)
	case "padding-right":
		cs.PaddingRight = resolveLengthEm(parsed, 0, cs.FontSize)
	case "padding-bottom":
		cs.PaddingBottom = resolveLengthEm(parsed, 0, cs.FontSize)
	case "padding-left":
		cs.PaddingLeft = resolveLengthEm(parsed, 0, cs.FontSize)

	// Logical box properties. goosie lays out left-to-right, so inline maps onto
	// the horizontal axis and block onto the vertical one, start onto left and
	// top. The names still matter: a modern page such as MDN's sheet sizes its
	// gutters with `padding-inline: var(...)` and nothing else, so without the
	// alias the whole page loses its side padding.
	case "margin-block":
		a, b := logicalPair(value, 0, cs.FontSize)
		cs.MarginTop, cs.MarginBottom = a, b
	case "margin-inline":
		a, b := logicalPair(value, MarginAuto, cs.FontSize)
		cs.MarginLeft, cs.MarginRight = a, b
	case "margin-block-start":
		cs.MarginTop = resolveLengthEm(parsed, 0, cs.FontSize)
	case "margin-block-end":
		cs.MarginBottom = resolveLengthEm(parsed, 0, cs.FontSize)
	case "margin-inline-start":
		cs.MarginLeft = resolveLengthEm(parsed, MarginAuto, cs.FontSize)
	case "margin-inline-end":
		cs.MarginRight = resolveLengthEm(parsed, MarginAuto, cs.FontSize)
	case "padding-block":
		a, b := logicalPair(value, 0, cs.FontSize)
		cs.PaddingTop, cs.PaddingBottom = a, b
	case "padding-inline":
		a, b := logicalPair(value, 0, cs.FontSize)
		cs.PaddingLeft, cs.PaddingRight = a, b
	case "padding-block-start":
		cs.PaddingTop = resolveLengthEm(parsed, 0, cs.FontSize)
	case "padding-block-end":
		cs.PaddingBottom = resolveLengthEm(parsed, 0, cs.FontSize)
	case "padding-inline-start":
		cs.PaddingLeft = resolveLengthEm(parsed, 0, cs.FontSize)
	case "padding-inline-end":
		cs.PaddingRight = resolveLengthEm(parsed, 0, cs.FontSize)

	case "border":
		w, s, c := parseBorderShorthand(value)
		cs.BorderTopWidth = w
		cs.BorderRightWidth = w
		cs.BorderBottomWidth = w
		cs.BorderLeftWidth = w
		cs.BorderTopStyle = s
		cs.BorderRightStyle = s
		cs.BorderBottomStyle = s
		cs.BorderLeftStyle = s
		cs.BorderTopColor = c
		cs.BorderRightColor = c
		cs.BorderBottomColor = c
		cs.BorderLeftColor = c
	case "border-width":
		t, r, b, l := parseBoxShorthand(value)
		cs.BorderTopWidth = t
		cs.BorderRightWidth = r
		cs.BorderBottomWidth = b
		cs.BorderLeftWidth = l
	case "border-top-width":
		cs.BorderTopWidth = resolveLength(parsed, 0)
	case "border-right-width":
		cs.BorderRightWidth = resolveLength(parsed, 0)
	case "border-bottom-width":
		cs.BorderBottomWidth = resolveLength(parsed, 0)
	case "border-left-width":
		cs.BorderLeftWidth = resolveLength(parsed, 0)
	case "border-style":
		t, r, b, l := parseBorderStyles(value)
		cs.BorderTopStyle = t
		cs.BorderRightStyle = r
		cs.BorderBottomStyle = b
		cs.BorderLeftStyle = l
	case "border-top-style":
		cs.BorderTopStyle = value
	case "border-right-style":
		cs.BorderRightStyle = value
	case "border-bottom-style":
		cs.BorderBottomStyle = value
	case "border-left-style":
		cs.BorderLeftStyle = value
	case "border-color":
		colors := parseBorderColors(value)
		cs.BorderTopColor = colors[0]
		cs.BorderRightColor = colors[1]
		cs.BorderBottomColor = colors[2]
		cs.BorderLeftColor = colors[3]
	case "border-top-color":
		cs.BorderTopColor = parseColorValue(value)
	case "border-right-color":
		cs.BorderRightColor = parseColorValue(value)
	case "border-bottom-color":
		cs.BorderBottomColor = parseColorValue(value)
	case "border-left-color":
		cs.BorderLeftColor = parseColorValue(value)
	case "border-top":
		w, s, c := parseBorderShorthand(value)
		cs.BorderTopWidth = w
		cs.BorderTopStyle = s
		cs.BorderTopColor = c
	case "border-right":
		w, s, c := parseBorderShorthand(value)
		cs.BorderRightWidth = w
		cs.BorderRightStyle = s
		cs.BorderRightColor = c
	case "border-bottom":
		w, s, c := parseBorderShorthand(value)
		cs.BorderBottomWidth = w
		cs.BorderBottomStyle = s
		cs.BorderBottomColor = c
	case "border-left":
		w, s, c := parseBorderShorthand(value)
		cs.BorderLeftWidth = w
		cs.BorderLeftStyle = s
		cs.BorderLeftColor = c
	case "border-radius":
		parseBorderRadius(cs, value)
	case "border-top-left-radius":
		parseCornerRadius(cs, &cs.BorderRadius[0], value)
	case "border-top-right-radius":
		parseCornerRadius(cs, &cs.BorderRadius[1], value)
	case "border-bottom-right-radius":
		parseCornerRadius(cs, &cs.BorderRadius[2], value)
	case "border-bottom-left-radius":
		parseCornerRadius(cs, &cs.BorderRadius[3], value)

	case "top":
		cs.Top = resolveLengthEm(parsed, 0, cs.FontSize)
		cs.HasTop = lengthSpecified(parsed)
	case "right":
		cs.Right = resolveLengthEm(parsed, 0, cs.FontSize)
		cs.HasRight = lengthSpecified(parsed)
	case "bottom":
		cs.Bottom = resolveLengthEm(parsed, 0, cs.FontSize)
		cs.HasBottom = lengthSpecified(parsed)
	case "left":
		cs.Left = resolveLengthEm(parsed, 0, cs.FontSize)
		cs.HasLeft = lengthSpecified(parsed)

	case "color":
		// A value that is not a colour at all - `initial`, a `light-dark()` this
		// engine does not evaluate, the residue of a var() that never resolved -
		// is invalid at computed-value time, so the property keeps the colour it
		// inherited. Storing the zero instead paints the text transparent, which
		// reads as a blank gap where a word should be.
		if c, ok := css.ParseColor(value); ok {
			cs.Color = c
		}
	case "background-color":
		cs.BackgroundColor = parseColorValue(value)
	case "background":
		cs.BackgroundColor, cs.BackgroundGradient = parseBackground(value)
		cs.BackgroundImage = extractBackgroundURL(value)
		parseBackgroundShorthandExtras(cs, value)
	case "background-image":
		cs.BackgroundGradient = parseGradient(value)
		cs.BackgroundImage = extractBackgroundURL(value)
	case "background-repeat":
		cs.BackgroundRepeat = parseBackgroundRepeat(value)
	case "background-size":
		parseBackgroundSize(cs, value)
	case "background-position":
		parseBackgroundPosition(cs, value)

	case "font-family":
		cs.FontFamily = parseFontFamily(value)
	case "font-size":
		cs.FontSize = clampFontSize(resolveFontSize(parsed, parentFontSize))
	case "font-weight":
		cs.FontWeight = parseFontWeight(value)
	case "font-style":
		cs.FontStyle = parseFontStyle(value)
	case "font":
		parseFontShorthand(cs, value, parentFontSize)
	case "line-height":
		cs.LineHeight = resolveLineHeight(parsed, cs.FontSize)
		cs.LineHeightRatio = lineHeightRatio(parsed)
		cs.LineHeightEm = lineHeightEm(parsed)
	case "text-align":
		cs.TextAlign = parseTextAlign(value)
	case "text-indent":
		cs.TextIndent = resolveLength(parsed, 0)
	case "text-transform":
		cs.TextTransform = value
	case "text-decoration":
		cs.TextDecoration = parseTextDecoration(value)
	case "white-space":
		cs.WhiteSpace = parseWhiteSpace(value)
	case "word-spacing":
		cs.WordSpacing = resolveLength(parsed, 0)
	case "letter-spacing":
		cs.LetterSpacing = resolveLength(parsed, 0)

	case "vertical-align":
		cs.VerticalAlign = value

	case "list-style-type":
		cs.ListStyleType = parseListStyleType(value)
	case "list-style":
		cs.ListStyleType = parseListStyleType(value)

	case "flex-direction":
		cs.FlexDirection = parseFlexDirection(value)
	case "flex-wrap":
		cs.FlexWrap = parseFlexWrap(value)
	case "justify-content":
		cs.JustifyContent = value
	case "align-items":
		cs.AlignItems = value
	case "align-self":
		cs.AlignSelf = value
	case "flex-grow":
		cs.FlexGrow = float32(parsed.Num)
	case "flex-shrink":
		cs.FlexShrink = float32(parsed.Num)
	case "flex-basis":
		cs.FlexBasis = resolveLength(parsed, -1)
	case "order":
		// `order` is an integer; a non-number (e.g. `initial`) leaves the 0 default.
		if parsed.Type == css.ValueNumber {
			cs.Order = int(parsed.Num)
		}
	case "object-fit":
		cs.ObjectFit = value
	case "gap":
		g := resolveLength(parsed, 0)
		cs.RowGap, cs.ColumnGap = g, g
	case "grid-gap":
		// The superseded name for gap, still what a page declares.
		g := resolveLength(parsed, 0)
		cs.RowGap, cs.ColumnGap = g, g
	case "row-gap":
		cs.RowGap = resolveLength(parsed, 0)
	case "column-gap":
		// A grid spends this on its column axis; a row-direction flexbox on its
		// main axis. Wikipedia's page grid declares only this and no `gap`.
		cs.ColumnGap = resolveLength(parsed, 0)
	case "grid-template-columns":
		cs.GridTemplateColumns = value
	case "grid-template-rows":
		cs.GridTemplateRows = value
	case "grid-template-areas":
		cs.GridTemplateAreas = value
	case "grid-template":
		parseGridTemplateShorthand(cs, value)
	case "grid-area":
		// Stored verbatim: `grid-area` names a `grid-template-areas` token, and CSS
		// identifiers are case-sensitive, so lowercasing here left every camelCase
		// area unmatched and the item fell through to auto-placement.
		cs.GridArea = strings.TrimSpace(value)
	case "grid-column":
		cs.GridColumnSpan = parseGridSpan(value)
	case "grid-row":
		cs.GridRowSpan = parseGridSpan(value)
	case "justify-items":
		cs.JustifyItems = value
	case "align-content":
		cs.AlignContent = value
	case "flex":
		parseFlexShorthand(cs, value)

	case "table-layout":
		cs.TableLayout = value
	case "border-collapse":
		cs.BorderCollapse = value == "collapse"
	case "border-spacing":
		// One value sets both axes; two split between horizontal and vertical.
		// A negative gap is clamped to zero rather than letting cells overlap.
		parts := strings.Fields(value)
		if len(parts) > 0 {
			h := resolveLengthEm(css.ParseValue(parts[0]), 0, cs.FontSize)
			v := h
			if len(parts) > 1 {
				v = resolveLengthEm(css.ParseValue(parts[1]), 0, cs.FontSize)
			}
			if h < 0 {
				h = 0
			}
			if v < 0 {
				v = 0
			}
			cs.BorderSpacingH = h
			cs.BorderSpacingV = v
		}
	}
}

// MarginAuto is the sentinel value stored in a margin field when the CSS
// value is "auto". Block layout detects this to implement horizontal centering
// (margin-left: auto / margin-right: auto). The same -1 convention is used by
// Width and Height for their auto value.
const MarginAuto float32 = -1

// logicalPair reads a `-inline`/`-block` shorthand: one length for both sides,
// or the start and end side separately.
func logicalPair(v string, auto, fontSize float32) (float32, float32) {
	parts := splitTopLevelSpace(v)
	if len(parts) == 0 {
		return auto, auto
	}
	a := resolveLengthEm(css.ParseValue(parts[0]), auto, fontSize)
	if len(parts) < 2 {
		return a, a
	}
	return a, resolveLengthEm(css.ParseValue(parts[1]), auto, fontSize)
}

// splitTopLevelSpace cuts a value on the whitespace outside its parentheses.
// `max(1rem,calc(50vw - 720px + 1rem))` is one length, and the spaces inside the
// arithmetic would otherwise read as a list of them.
func splitTopLevelSpace(v string) []string {
	var out []string
	depth, start := 0, -1
	for i := 0; i < len(v); i++ {
		switch c := v[i]; c {
		case '(':
			depth++
		case ')':
			depth--
		case ' ', '\t', '\n':
			if depth == 0 {
				if start >= 0 {
					out = append(out, v[start:i])
					start = -1
				}
				continue
			}
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, v[start:])
	}
	return out
}

// againstOwnFont lists the declarations whose lengths are a share of the
// element's own font size, so they have to be re-resolved once that size is
// final. Everything else is settled by the first pass.
func againstOwnFont(prop string) bool {
	switch prop {
	case "width", "height", "min-width", "min-height", "max-width", "max-height",
		"margin", "margin-top", "margin-right", "margin-bottom", "margin-left",
		"margin-block", "margin-block-start", "margin-block-end",
		"margin-inline", "margin-inline-start", "margin-inline-end",
		"padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
		"padding-block", "padding-block-start", "padding-block-end",
		"padding-inline", "padding-inline-start", "padding-inline-end",
		"top", "right", "bottom", "left":
		return true
	}
	return false
}

// resolveMaxLength caps a box. `none` states that there is no cap, which the
// layout reads as the same -1 an auto size does; reading it as a length turned
// every `max-width: none` box into a zero-width one.
func resolveMaxLength(v css.Value, fontSize float32) float32 {
	if v.Type == css.ValueKeyword && v.Str == "none" {
		return -1
	}
	return resolveLengthEm(v, -1, fontSize)
}

func resolveLength(v css.Value, auto float32) float32 {
	if v.Type == css.ValueKeyword && (v.Str == "auto" || v.Str == "") {
		return auto
	}
	return v.ToLength()
}

// resolveLengthEm is like resolveLength but passes fontSize to ToLengthWithEm
// so that em units are computed against the element's own font-size instead of
// the hardcoded 16px fallback in ToLength. Use this for all box-model lengths
// (margin, padding, top/right/bottom/left, width when not percentage, etc.).
func resolveLengthEm(v css.Value, auto, fontSize float32) float32 {
	if v.Type == css.ValueKeyword && (v.Str == "auto" || v.Str == "") {
		return auto
	}
	return v.ToLengthWithEm(fontSize)
}

// lengthSpecified reports whether a top/right/bottom/left value is a used
// length rather than the initial `auto`. A unitless 0 counts: it is the one
// unitless number CSS accepts as a length, so the number type carries it.
func lengthSpecified(v css.Value) bool {
	switch v.Type {
	case css.ValueLength, css.ValuePercentage:
		return true
	case css.ValueNumber:
		return v.Num == 0
	}
	return false
}

func resolveFontSize(v css.Value, parent float32) float32 {
	switch v.Type {
	case css.ValueKeyword:
		switch v.Str {
		case "xx-small":
			return 9
		case "x-small":
			return 10
		case "small":
			return 13
		case "medium":
			return 16
		case "large":
			return 18
		case "x-large":
			return 24
		case "xx-large":
			return 32
		case "smaller":
			return parent * 0.83
		case "larger":
			return parent * 1.2
		}
	case css.ValuePercentage:
		return parent * float32(v.Num) / 100
	case css.ValueLength:
		if v.Unit == "rem" {
			// rem is the root font size, which goosie leaves at the 16px default.
			return float32(v.Num) * 16
		}
		if v.Unit == "em" || v.Unit == "ex" {
			return parent * float32(v.Num)
		}
		return v.ToLength()
	case css.ValueFunc:
		// Inside a font size, `em` is the parent's size and `rem` the root's, so
		// the expression is evaluated in the parent's context. A call this engine
		// cannot answer - one holding a percentage, which has no containing block
		// yet - leaves the size inherited rather than zero, which would erase the
		// element's text.
		if f := v.ToLengthWithEm(parent); f > 0 {
			return f
		}
		return parent
	}
	return float32(v.Num)
}

// lineHeightRatio reports the inheritable form of a line-height: a number or a
// percentage. A length, and the keyword normal, return zero because they carry
// no share of the font size forward.
func lineHeightRatio(v css.Value) float32 {
	switch v.Type {
	case css.ValueNumber:
		if v.Num <= 0 {
			return 0
		}
		return float32(v.Num)
	case css.ValuePercentage:
		if v.Num <= 0 {
			return 0
		}
		return float32(v.Num) / 100
	}
	return 0
}

// lineHeightEm reports how many em a declared line-height is worth, which is
// only known once the element's own font size is final. Anything else - a px
// length, a number, `normal` - returns zero.
func lineHeightEm(v css.Value) float32 {
	if v.Type == css.ValueLength && v.Unit == "em" && v.Num > 0 {
		return float32(v.Num)
	}
	return 0
}

func resolveLineHeight(v css.Value, fontSize float32) float32 {
	switch v.Type {
	case css.ValueNumber:
		return fontSize * float32(v.Num)
	case css.ValuePercentage:
		return fontSize * float32(v.Num) / 100
	case css.ValueLength:
		// `1.5em` is 1.5 times this element's font size, not the 16px default a
		// plain length conversion would use.
		return v.ToLengthWithEm(fontSize)
	case css.ValueKeyword:
		if v.Str == "normal" {
			return fontSize * 1.2
		}
	}
	return fontSize * 1.2
}

func parseDisplay(v string) Display {
	switch v {
	case "none":
		return DisplayNone
	case "block":
		return DisplayBlock
	case "inline":
		return DisplayInline
	case "inline-block":
		return DisplayInlineBlock
	case "flex":
		return DisplayFlex
	case "inline-flex":
		return DisplayInlineFlex
	case "grid":
		return DisplayGrid
	case "inline-grid":
		return DisplayInlineGrid
	case "contents":
		return DisplayContents
	case "list-item":
		return DisplayListItem
	case "table":
		return DisplayTable
	case "table-row":
		return DisplayTableRow
	case "table-cell":
		return DisplayTableCell
	case "table-row-group":
		return DisplayTableRowGroup
	case "table-header-group":
		return DisplayTableHeaderGroup
	case "table-footer-group":
		return DisplayTableFooterGroup
	case "table-column":
		return DisplayTableColumn
	case "table-column-group":
		return DisplayTableColumnGroup
	case "table-caption":
		return DisplayTableCaption
	}
	return DisplayInline
}

func parsePosition(v string) Position {
	switch v {
	case "relative":
		return PositionRelative
	case "absolute":
		return PositionAbsolute
	case "fixed":
		return PositionFixed
	case "sticky":
		return PositionSticky
	}
	return PositionStatic
}

func parseFloatProp(v string) Float {
	switch v {
	case "left":
		return FloatLeft
	case "right":
		return FloatRight
	}
	return FloatNone
}

func parseClear(v string) Clear {
	switch v {
	case "left":
		return ClearLeft
	case "right":
		return ClearRight
	case "both":
		return ClearBoth
	}
	return ClearNone
}

func parseOverflowProp(v string) Overflow {
	switch v {
	case "hidden":
		return OverflowHidden
	case "scroll":
		return OverflowScroll
	case "auto":
		return OverflowAuto
	}
	return OverflowVisible
}

func parseTextAlign(v string) TextAlign {
	switch v {
	case "right":
		return TextAlignRight
	case "center":
		return TextAlignCenter
	case "justify":
		return TextAlignJustify
	}
	return TextAlignLeft
}

func parseWhiteSpace(v string) WhiteSpace {
	switch v {
	case "nowrap":
		return WhiteSpaceNowrap
	case "pre":
		return WhiteSpacePre
	case "pre-wrap":
		return WhiteSpacePrewrite
	case "pre-line":
		return WhiteSpacePreline
	}
	return WhiteSpaceNormal
}

func parseFontWeight(v string) FontWeight {
	switch v {
	case "bold":
		return WeightBold
	case "bolder":
		return WeightBold
	case "lighter":
		return 100
	case "normal":
		return WeightNormal
	}
	if parsed := css.ParseValue(v); parsed.Type == css.ValueNumber {
		return FontWeight(parsed.Num)
	}
	return WeightNormal
}

func parseFontStyle(v string) FontStyle {
	switch v {
	case "italic":
		return FontStyleItalic
	case "oblique":
		return FontStyleOblique
	}
	return FontStyleNormal
}

// fontFamilies names every family this engine can serve, by the lowercased CSS
// name. Generic keywords are here too, which is what makes a list like
// `Georgia, serif` resolve without special-casing the tail.
//
// cursive and fantasy have no matching host face here - Blink answers with
// Apple Chancery and Papyrus - so they take Georgia, the closest available
// calligraphic serif.
var fontFamilies = map[string]frame.FontFamily{
	"serif":              frame.FontTimes,
	"times":              frame.FontTimes,
	"times new roman":    frame.FontTimes,
	"liberation serif":   frame.FontTimes,
	"sans-serif":         frame.FontArial,
	"arial":              frame.FontArial,
	"helvetica":          frame.FontHelvetica,
	"helvetica neue":     frame.FontHelvetica,
	"liberation sans":    frame.FontArial,
	"dejavu sans":        frame.FontArial,
	"system-ui":          frame.FontArial,
	"-apple-system":      frame.FontArial,
	"blinkmacsystemfont": frame.FontArial,
	"segoe ui":           frame.FontArial,
	"monospace":          frame.FontCourier,
	"courier":            frame.FontCourier,
	"courier new":        frame.FontCourier,
	"menlo":              frame.FontCourier,
	"consolas":           frame.FontCourier,
	"georgia":            frame.FontGeorgia,
	"cursive":            frame.FontGeorgia,
	"fantasy":            frame.FontGeorgia,
	"verdana":            frame.FontVerdana,
	"tahoma":             frame.FontVerdana,
	"go":                 frame.FontGo,
	"go regular":         frame.FontGo,
}

// parseFontFamily walks a declared font-family list and returns the first family
// this engine can serve. A name it does not know is unavailable rather than
// unknown-but-drawn, exactly as a browser skips a family that is not installed,
// so the list keeps falling through to its generic keyword. An exhausted list
// lands on the standard font.
func parseFontFamily(v string) frame.FontFamily {
	for _, part := range strings.Split(v, ",") {
		if fam, ok := fontFamilies[normalizeFontName(part)]; ok {
			return fam
		}
	}
	return frame.FontTimes
}

// normalizeFontName folds away what CSS allows around a family name: quoting,
// case, and repeated spaces, so "Times  New Roman" and times new roman are one
// name.
func normalizeFontName(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.Trim(v, "\"'")
	v = strings.TrimSpace(v)
	for strings.Contains(v, "  ") {
		v = strings.ReplaceAll(v, "  ", " ")
	}
	return v
}

func parseTextDecoration(v string) TextDecoration {
	var td TextDecoration
	for _, part := range strings.Fields(v) {
		switch part {
		case "underline":
			td |= TextDecorationUnderline
		case "overline":
			td |= TextDecorationOverline
		case "line-through":
			td |= TextDecorationLineThrough
		case "none":
			return TextDecorationNone
		}
	}
	return td
}

func parseListStyleType(v string) ListStyleType {
	switch v {
	case "none":
		return ListStyleNone
	case "disc":
		return ListStyleDisc
	case "circle":
		return ListStyleCircle
	case "square":
		return ListStyleSquare
	case "decimal":
		return ListStyleDecimal
	}
	return ListStyleDisc
}

func parseFlexDirection(v string) FlexDirection {
	switch v {
	case "row-reverse":
		return FlexRowReverse
	case "column":
		return FlexColumn
	case "column-reverse":
		return FlexColumnReverse
	}
	return FlexRow
}

func parseFlexWrap(v string) FlexWrap {
	switch v {
	case "wrap":
		return FlexWrapValue
	case "wrap-reverse":
		return FlexWrapReverse
	}
	return FlexNowrap
}

func parseColorValue(v string) css.Color {
	c, _ := css.ParseColor(v)
	return c
}

func resolveCurrentColor(cs *ComputedStyle, decls []css.Declaration) {
	handled := make(map[string]bool)
	for i := len(decls) - 1; i >= 0; i-- {
		d := decls[i]
		lower := strings.TrimSpace(strings.ToLower(d.Value))
		isCC := lower == "currentcolor"

		switch d.Property {
		case "border-color":
			if handled["border-color"] {
				continue
			}
			if isCC {
				if !handled["border-top-color"] {
					cs.BorderTopColor = cs.Color
				}
				if !handled["border-right-color"] {
					cs.BorderRightColor = cs.Color
				}
				if !handled["border-bottom-color"] {
					cs.BorderBottomColor = cs.Color
				}
				if !handled["border-left-color"] {
					cs.BorderLeftColor = cs.Color
				}
			}
			handled["border-color"] = true
			handled["border-top-color"] = true
			handled["border-right-color"] = true
			handled["border-bottom-color"] = true
			handled["border-left-color"] = true
		case "border-top-color":
			if handled["border-top-color"] {
				continue
			}
			if isCC {
				cs.BorderTopColor = cs.Color
			}
			handled["border-top-color"] = true
		case "border-right-color":
			if handled["border-right-color"] {
				continue
			}
			if isCC {
				cs.BorderRightColor = cs.Color
			}
			handled["border-right-color"] = true
		case "border-bottom-color":
			if handled["border-bottom-color"] {
				continue
			}
			if isCC {
				cs.BorderBottomColor = cs.Color
			}
			handled["border-bottom-color"] = true
		case "border-left-color":
			if handled["border-left-color"] {
				continue
			}
			if isCC {
				cs.BorderLeftColor = cs.Color
			}
			handled["border-left-color"] = true
		case "background-color":
			if handled["background-color"] {
				continue
			}
			if isCC {
				cs.BackgroundColor = cs.Color
			}
			handled["background-color"] = true
		}
	}
}

// parseBackgroundColor extracts the color from a background shorthand value.
// The CSS background shorthand may contain url(), keywords (no-repeat, center,
// cover, etc.), and a color in any order. This function tries each whitespace-
// separated token (skipping url() function calls) and returns the first one
// that parses as a valid color.
func parseBackgroundColor(v string) css.Color {
	// Try the whole value first (handles plain "background: #fff").
	if c, ok := css.ParseColor(v); ok {
		return c
	}
	// Skip past any url(...) call, then try each token.
	s := v
	for {
		urlIdx := strings.Index(strings.ToLower(s), "url(")
		if urlIdx < 0 {
			break
		}
		// find matching paren
		depth := 0
		end := urlIdx
		for end < len(s) {
			if s[end] == '(' {
				depth++
			} else if s[end] == ')' {
				depth--
				if depth == 0 {
					end++
					break
				}
			}
			end++
		}
		s = strings.TrimSpace(s[end:])
	}
	for _, tok := range strings.Fields(s) {
		if c, ok := css.ParseColor(tok); ok {
			return c
		}
	}
	return css.Color{}
}

// extractBackgroundURL returns the inner target of the first url() in a
// background value, unquoted, or "" when there is none (or it is `none`).
func extractBackgroundURL(v string) string {
	lower := strings.ToLower(v)
	i := strings.Index(lower, "url(")
	if i < 0 {
		return ""
	}
	rest := v[i+len("url("):]
	end := strings.IndexByte(rest, ')')
	if end < 0 {
		return ""
	}
	target := strings.TrimSpace(rest[:end])
	target = strings.Trim(target, "\"'")
	if target == "" || target == "none" {
		return ""
	}
	return target
}

// parseBackgroundRepeat reads the background-repeat value. Two-axis forms like
// `repeat no-repeat` are rare; the first token decides.
func parseBackgroundRepeat(v string) BgRepeat {
	switch strings.ToLower(strings.TrimSpace(strings.Fields(v)[0])) {
	case "repeat-x":
		return BgRepeatRepeatX
	case "repeat-y":
		return BgRepeatRepeatY
	case "no-repeat":
		return BgRepeatNoRepeat
	default:
		return BgRepeatRepeat
	}
}

// parseBackgroundShorthandExtras pulls the repeat, position and size out of a
// background shorthand. The shorthand lets position and size share one
// declaration (`url(x) center / cover no-repeat`), so a value that only sets a
// longhand through `background` still has to reach those fields or the layer
// lands at its default top-left origin.
func parseBackgroundShorthandExtras(cs *ComputedStyle, v string) {
	toks := tokenizeBackground(v)
	var pos []string
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t == "/" {
			var size []string
			for j := i + 1; j < len(toks) && toks[j] != "/"; j++ {
				if !isSizeTok(toks[j]) {
					break
				}
				size = append(size, toks[j])
				i = j
			}
			if len(size) > 0 {
				parseBackgroundSize(cs, strings.Join(size, " "))
			}
			continue
		}
		switch strings.ToLower(t) {
		case "repeat":
			cs.BackgroundRepeat = BgRepeatRepeat
		case "repeat-x":
			cs.BackgroundRepeat = BgRepeatRepeatX
		case "repeat-y":
			cs.BackgroundRepeat = BgRepeatRepeatY
		case "no-repeat":
			cs.BackgroundRepeat = BgRepeatNoRepeat
		default:
			if isPositionTok(t) {
				pos = append(pos, t)
			}
		}
	}
	if len(pos) > 0 {
		parseBackgroundPosition(cs, strings.Join(pos, " "))
	}
}

// tokenizeBackground splits a background shorthand on whitespace and the `/`
// that separates position from size, keeping each `func(...)` intact so a
// `url(a/b.png)` or `rgba(0,0,0,.3)` never breaks into stray tokens.
func tokenizeBackground(v string) []string {
	var (
		toks  []string
		cur   strings.Builder
		depth int
	)
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '(':
			depth++
			cur.WriteByte(c)
		case c == ')':
			if depth > 0 {
				depth--
			}
			cur.WriteByte(c)
		case depth == 0 && (c == ' ' || c == '\t' || c == '\n' || c == '\r'):
			flush()
		case depth == 0 && c == '/':
			flush()
			toks = append(toks, "/")
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return toks
}

// isPositionTok reports whether a shorthand token can start a background
// position: a position keyword or a length/percentage offset.
func isPositionTok(t string) bool {
	switch strings.ToLower(t) {
	case "top", "bottom", "left", "right", "center":
		return true
	}
	if strings.ContainsRune(t, '(') {
		return false
	}
	_, _, ok := parseBgLen(t)
	return ok
}

// isSizeTok reports whether a token can be a background-size value.
func isSizeTok(t string) bool {
	switch strings.ToLower(t) {
	case "auto", "contain", "cover":
		return true
	}
	if strings.ContainsRune(t, '(') {
		return false
	}
	_, _, ok := parseBgLen(t)
	return ok
}

// parseBackgroundSize reads auto / contain / cover / one-or-two lengths or
// percentages. An unparseable token falls back to auto, which keeps the image
// at its natural size rather than collapsing it to zero.
func parseBackgroundSize(cs *ComputedStyle, v string) {
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return
	}
	switch strings.ToLower(parts[0]) {
	case "auto":
		cs.BackgroundSize = BgSizeAuto
		return
	case "contain":
		cs.BackgroundSize = BgSizeContain
		return
	case "cover":
		cs.BackgroundSize = BgSizeCover
		return
	}
	w, wPct, okW := parseBgLen(parts[0])
	if !okW {
		cs.BackgroundSize = BgSizeAuto
		return
	}
	cs.BackgroundSize = BgSizeLength
	cs.BgSizeW, cs.BgSizeWPct = w, wPct
	cs.BgSizeH = -1
	if len(parts) > 1 && strings.ToLower(parts[1]) != "auto" {
		if h, hPct, okH := parseBgLen(parts[1]); okH {
			cs.BgSizeH, cs.BgSizeHPct = h, hPct
		}
	}
}

// parseBackgroundPosition reads a one- or two-token position. Keywords map to
// start/center/end; a length is an offset from the start edge. Percentages are
// treated as the nearest keyword so a centred banner still lands centred.
func parseBackgroundPosition(cs *ComputedStyle, v string) {
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return
	}
	setX := func(tok string) {
		switch strings.ToLower(tok) {
		case "left":
			cs.BackgroundPosXMode = BgPosStart
		case "center":
			cs.BackgroundPosXMode = BgPosCenter
		case "right":
			cs.BackgroundPosXMode = BgPosEnd
		default:
			if v, pct, ok := parseBgLen(tok); ok {
				cs.BackgroundPosXMode = BgPosLength
				cs.BackgroundPosX, cs.BgPosXPct = v, pct
			}
		}
	}
	setY := func(tok string) {
		switch strings.ToLower(tok) {
		case "top":
			cs.BackgroundPosYMode = BgPosStart
		case "center":
			cs.BackgroundPosYMode = BgPosCenter
		case "bottom":
			cs.BackgroundPosYMode = BgPosEnd
		default:
			if v, pct, ok := parseBgLen(tok); ok {
				cs.BackgroundPosYMode = BgPosLength
				cs.BackgroundPosY, cs.BgPosYPct = v, pct
			}
		}
	}
	if len(parts) == 1 {
		lower := strings.ToLower(parts[0])
		switch lower {
		case "top", "bottom":
			setY(parts[0])
		default:
			setX(parts[0])
			cs.BackgroundPosYMode = BgPosCenter
		}
		return
	}
	// Two tokens: order is x then y, but CSS also allows y-first when the first
	// token is a vertical keyword.
	if isVerticalKeyword(parts[0]) {
		setY(parts[0])
		setX(parts[1])
	} else {
		setX(parts[0])
		setY(parts[1])
	}
}

func isVerticalKeyword(tok string) bool {
	switch strings.ToLower(tok) {
	case "top", "bottom":
		return true
	}
	return false
}

// parseBgLen reads a background length or percentage. It returns the numeric
// value, whether it was a percentage (true) or a CSS-px length (false), and
// whether the token parsed at all. Unitless 0 is accepted as a length.
func parseBgLen(tok string) (float32, bool, bool) {
	tok = strings.ToLower(strings.TrimSpace(tok))
	if tok == "0" {
		return 0, false, true
	}
	if strings.HasSuffix(tok, "%") {
		f, err := strconv.ParseFloat(strings.TrimSpace(tok[:len(tok)-1]), 32)
		if err != nil {
			return 0, false, false
		}
		return float32(f) / 100, true, true
	}
	if strings.HasSuffix(tok, "px") {
		f, err := strconv.ParseFloat(strings.TrimSpace(tok[:len(tok)-2]), 32)
		if err != nil {
			return 0, false, false
		}
		return float32(f), false, true
	}
	return 0, false, false
}

// parseMarginShorthand parses the CSS margin shorthand (1–4 values), using
// MarginAuto as the resolved value for "auto". fontSize is used to resolve
// em-valued lengths against the element's own computed font-size.
func parseMarginShorthand(v string, fontSize float32) (top, right, bottom, left float32) {
	parts := strings.Fields(v)
	switch len(parts) {
	case 1:
		val := resolveLengthEm(css.ParseValue(parts[0]), MarginAuto, fontSize)
		return val, val, val, val
	case 2:
		tb := resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize)
		rl := resolveLengthEm(css.ParseValue(parts[1]), MarginAuto, fontSize)
		return tb, rl, tb, rl
	case 3:
		top = resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize)
		rl := resolveLengthEm(css.ParseValue(parts[1]), MarginAuto, fontSize)
		bottom = resolveLengthEm(css.ParseValue(parts[2]), 0, fontSize)
		return top, rl, bottom, rl
	case 4:
		top = resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize)
		right = resolveLengthEm(css.ParseValue(parts[1]), MarginAuto, fontSize)
		bottom = resolveLengthEm(css.ParseValue(parts[2]), 0, fontSize)
		left = resolveLengthEm(css.ParseValue(parts[3]), MarginAuto, fontSize)
		return
	}
	return
}

// parsePaddingShorthand parses the CSS padding shorthand (1–4 values).
// Padding cannot be "auto", so 0 is always the fallback. fontSize is used to
// resolve em-valued lengths.
func parsePaddingShorthand(v string, fontSize float32) (top, right, bottom, left float32) {
	parts := strings.Fields(v)
	switch len(parts) {
	case 1:
		val := resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize)
		return val, val, val, val
	case 2:
		tb := resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize)
		rl := resolveLengthEm(css.ParseValue(parts[1]), 0, fontSize)
		return tb, rl, tb, rl
	case 3:
		top = resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize)
		rl := resolveLengthEm(css.ParseValue(parts[1]), 0, fontSize)
		bottom = resolveLengthEm(css.ParseValue(parts[2]), 0, fontSize)
		return top, rl, bottom, rl
	case 4:
		top = resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize)
		right = resolveLengthEm(css.ParseValue(parts[1]), 0, fontSize)
		bottom = resolveLengthEm(css.ParseValue(parts[2]), 0, fontSize)
		left = resolveLengthEm(css.ParseValue(parts[3]), 0, fontSize)
		return
	}
	return
}

func parseBoxShorthand(v string) (top, right, bottom, left float32) {
	parts := strings.Fields(v)
	switch len(parts) {
	case 1:
		val := resolveLength(css.ParseValue(parts[0]), 0)
		return val, val, val, val
	case 2:
		tb := resolveLength(css.ParseValue(parts[0]), 0)
		rl := resolveLength(css.ParseValue(parts[1]), 0)
		return tb, rl, tb, rl
	case 3:
		top = resolveLength(css.ParseValue(parts[0]), 0)
		rl := resolveLength(css.ParseValue(parts[1]), 0)
		bottom = resolveLength(css.ParseValue(parts[2]), 0)
		return top, rl, bottom, rl
	case 4:
		top = resolveLength(css.ParseValue(parts[0]), 0)
		right = resolveLength(css.ParseValue(parts[1]), 0)
		bottom = resolveLength(css.ParseValue(parts[2]), 0)
		left = resolveLength(css.ParseValue(parts[3]), 0)
		return
	}
	return
}

func parseBorderShorthand(v string) (width float32, style string, color css.Color) {
	parts := strings.Fields(v)
	for _, p := range parts {
		lower := strings.ToLower(p)
		if isBorderStyle(lower) {
			style = lower
		} else if _, ok := css.ParseColor(lower); ok {
			color, _ = css.ParseColor(lower)
		} else {
			width = resolveLength(css.ParseValue(p), 0)
		}
	}
	if width == 0 && style != "" && style != "none" && style != "hidden" {
		width = 3
	}
	return
}

func isBorderStyle(v string) bool {
	switch v {
	case "none", "hidden", "dotted", "dashed", "solid",
		"double", "groove", "ridge", "inset", "outset":
		return true
	}
	return false
}

func parseBorderStyles(v string) (top, right, bottom, left string) {
	parts := strings.Fields(v)
	switch len(parts) {
	case 1:
		return parts[0], parts[0], parts[0], parts[0]
	case 2:
		return parts[0], parts[1], parts[0], parts[1]
	case 3:
		return parts[0], parts[1], parts[2], parts[1]
	case 4:
		return parts[0], parts[1], parts[2], parts[3]
	}
	return
}

func splitColorTokens(v string) []string {
	var tokens []string
	depth := 0
	start := -1
	for i := 0; i < len(v); i++ {
		switch v[i] {
		case '(':
			depth++
			if start < 0 {
				start = i
			}
		case ')':
			if depth > 0 {
				depth--
			}
		case ' ', '\t':
			if depth == 0 {
				if start >= 0 {
					tokens = append(tokens, strings.TrimSpace(v[start:i]))
					start = -1
				}
			} else if start < 0 {
				start = i
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		tokens = append(tokens, strings.TrimSpace(v[start:]))
	}
	return tokens
}

func parseBorderColors(v string) [4]css.Color {
	parts := splitColorTokens(v)
	var colors [4]css.Color
	switch len(parts) {
	case 1:
		c, _ := css.ParseColor(parts[0])
		return [4]css.Color{c, c, c, c}
	case 2:
		t, _ := css.ParseColor(parts[0])
		r, _ := css.ParseColor(parts[1])
		return [4]css.Color{t, r, t, r}
	case 3:
		colors[0], _ = css.ParseColor(parts[0])
		colors[1], _ = css.ParseColor(parts[1])
		colors[2], _ = css.ParseColor(parts[2])
		colors[3] = colors[1]
	case 4:
		colors[0], _ = css.ParseColor(parts[0])
		colors[1], _ = css.ParseColor(parts[1])
		colors[2], _ = css.ParseColor(parts[2])
		colors[3], _ = css.ParseColor(parts[3])
	}
	return colors
}

// parseFontShorthand reads `font: [style] [variant] [weight] size[/line-height] family`.
// Bare numbers before the size are weights, so the size is the first length,
// percentage, or absolute font-size keyword that appears.
func parseFontShorthand(cs *ComputedStyle, v string, parentFontSize float32) {
	parts := strings.Fields(v)
	sizeIdx := -1
	for i, p := range parts {
		if isFontShorthandSize(p) {
			sizeIdx = i
			break
		}
	}
	if sizeIdx < 0 {
		return
	}
	for _, p := range parts[:sizeIdx] {
		switch strings.ToLower(p) {
		case "italic":
			cs.FontStyle = FontStyleItalic
		case "oblique":
			cs.FontStyle = FontStyleOblique
		case "bold", "bolder", "normal":
			cs.FontWeight = parseFontWeight(p)
		default:
			if css.ParseValue(p).Type == css.ValueNumber {
				cs.FontWeight = parseFontWeight(p)
			}
		}
	}
	sizeTok, line := parts[sizeIdx], ""
	if j := strings.Index(sizeTok, "/"); j >= 0 {
		sizeTok, line = sizeTok[:j], sizeTok[j+1:]
	}
	cs.FontSize = clampFontSize(resolveFontSize(css.ParseValue(sizeTok), parentFontSize))
	if line != "" {
		parsed := css.ParseValue(line)
		cs.LineHeight = resolveLineHeight(parsed, cs.FontSize)
		cs.LineHeightRatio = lineHeightRatio(parsed)
		// The size is final by the time a shorthand carries a line-height, so
		// this is already right; storing the em count keeps an earlier
		// `line-height` declaration from overriding the shorthand.
		cs.LineHeightEm = lineHeightEm(parsed)
	}
	if fam := strings.TrimSpace(strings.Join(parts[sizeIdx+1:], " ")); fam != "" {
		cs.FontFamily = parseFontFamily(fam)
	}
}

func isFontShorthandSize(tok string) bool {
	if j := strings.Index(tok, "/"); j >= 0 {
		tok = tok[:j]
	}
	switch parsed := css.ParseValue(tok); parsed.Type {
	case css.ValueLength, css.ValuePercentage:
		return true
	case css.ValueKeyword:
		switch parsed.Str {
		case "xx-small", "x-small", "small", "medium", "large", "x-large", "xx-large", "smaller", "larger":
			return true
		}
	}
	return false
}

// parseBorderRadius reads the shorthand: one to four values set the corners, and
// the group after an optional "/" replaces the vertical radii.
func parseBorderRadius(cs *ComputedStyle, v string) {
	horizontal, vertical := v, ""
	if i := strings.Index(v, "/"); i >= 0 {
		horizontal, vertical = v[:i], v[i+1:]
	}
	hs := radiusGroup(cs, horizontal)
	vs := hs
	if strings.TrimSpace(vertical) != "" {
		vs = radiusGroup(cs, vertical)
	}
	for c := 0; c < 4; c++ {
		cs.BorderRadius[c][0] = hs[c]
		cs.BorderRadius[c][1] = vs[c]
	}
}

// radiusGroup expands a one-to-four value list into the four corners the way the
// shorthand's repetition rules say: 1 sets all, 2 alternates, 3 mirrors the
// second onto the bottom left.
func radiusGroup(cs *ComputedStyle, v string) [4]float32 {
	vals := make([]float32, 0, 4)
	for _, p := range strings.Fields(v) {
		vals = append(vals, resolveLengthEm(css.ParseValue(p), 0, cs.FontSize))
	}
	var out [4]float32
	if len(vals) == 0 {
		return out
	}
	at := func(i int) float32 {
		if i >= len(vals) {
			i = len(vals) - 1
		}
		return vals[i]
	}
	switch len(vals) {
	case 1:
		out = [4]float32{vals[0], vals[0], vals[0], vals[0]}
	case 2:
		out = [4]float32{vals[0], vals[1], vals[0], vals[1]}
	case 3:
		out = [4]float32{vals[0], vals[1], vals[2], vals[1]}
	default:
		out = [4]float32{at(0), at(1), at(2), at(3)}
	}
	return out
}

// parseCornerRadius sets one corner, whose value may itself carry "horizontal
// vertical" for an elliptical corner.
func parseCornerRadius(cs *ComputedStyle, corner *[2]float32, v string) {
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return
	}
	corner[0] = resolveLengthEm(css.ParseValue(parts[0]), 0, cs.FontSize)
	corner[1] = corner[0]
	if len(parts) > 1 {
		corner[1] = resolveLengthEm(css.ParseValue(parts[1]), 0, cs.FontSize)
	}
}

func parseFlexShorthand(cs *ComputedStyle, v string) {
	parts := strings.Fields(v)
	switch len(parts) {
	case 1:
		if parts[0] == "none" {
			cs.FlexGrow = 0
			cs.FlexShrink = 0
			cs.FlexBasis = -1
		} else if parts[0] == "auto" {
			cs.FlexGrow = 1
			cs.FlexShrink = 1
			cs.FlexBasis = -1
		} else {
			cs.FlexGrow = float32(css.ParseValue(parts[0]).Num)
			cs.FlexShrink = 1
			cs.FlexBasis = 0
		}
	case 2:
		cs.FlexGrow = float32(css.ParseValue(parts[0]).Num)
		cs.FlexShrink = float32(css.ParseValue(parts[1]).Num)
		cs.FlexBasis = 0
	case 3:
		cs.FlexGrow = float32(css.ParseValue(parts[0]).Num)
		cs.FlexShrink = float32(css.ParseValue(parts[1]).Num)
		cs.FlexBasis = resolveLength(css.ParseValue(parts[2]), -1)
	}
}

// parseGridTemplateShorthand expands `grid-template` into the three longhands it
// sets: the track list after the slash is the column template, and what comes
// before it is the row template, which may open with quoted area rows as in
// `"hd hd" auto "sb main" 1fr / 12rem 1fr`.
//
// The shorthand always resets areas, rows and columns, so a declaration that
// gives only rows leaves the column template none. Wikipedia's Vector 2022 shell
// declares its whole page grid through this shorthand, and an unexpanded value
// left `grid-template-columns` empty, so the container fell through to auto tracks
// and the sidebar took half the page.
func parseGridTemplateShorthand(cs *ComputedStyle, v string) {
	rows, cols := strings.TrimSpace(v), ""
	if i := strings.Index(v, "/"); i >= 0 {
		rows = strings.TrimSpace(v[:i])
		cols = strings.TrimSpace(v[i+1:])
	}
	cs.GridTemplateAreas = ""
	if strings.Contains(rows, "\"") {
		var areas, sizes []string
		for i := 0; i < len(rows); {
			if rows[i] != '"' {
				j := i
				for j < len(rows) && rows[j] != '"' {
					j++
				}
				sizes = append(sizes, strings.Fields(rows[i:j])...)
				i = j
				continue
			}
			j := strings.IndexByte(rows[i+1:], '"')
			if j < 0 {
				break
			}
			areas = append(areas, rows[i:i+j+2])
			i += j + 2
		}
		cs.GridTemplateAreas = strings.Join(areas, " ")
		rows = strings.Join(sizes, " ")
	}
	if rows == "none" {
		rows = ""
	}
	if cols == "none" {
		cols = ""
	}
	cs.GridTemplateRows = rows
	cs.GridTemplateColumns = cols
}

// parseGridSpan reduces a grid-column/grid-row value to how many tracks the
// item occupies. "span N" is the direct form; "A / B" names two lines; anything
// else (including "auto" and named lines) places a single track.
func parseGridSpan(v string) int {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return 1
	}
	if i := strings.Index(v, "/"); i >= 0 {
		a, b := gridSpanNum(strings.TrimSpace(v[:i])), gridSpanNum(strings.TrimSpace(v[i+1:]))
		if a > 0 && b > a {
			return b - a
		}
		return 1
	}
	if strings.HasPrefix(v, "span") {
		if n := gridSpanNum(strings.TrimSpace(strings.TrimPrefix(v, "span"))); n > 0 {
			return n
		}
		return 1
	}
	return 1
}

func gridSpanNum(s string) int {
	p := css.ParseValue(s)
	if p.Type != css.ValueNumber || p.Num <= 0 {
		return 0
	}
	return int(p.Num)
}
