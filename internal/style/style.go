package style

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
)

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
	DisplayInlineTable
)

type Position uint8

const (
	PositionStatic Position = iota
	PositionRelative
	PositionAbsolute
	PositionFixed
	PositionSticky
)

type Overflow uint8

const (
	OverflowVisible Overflow = iota
	OverflowHidden
	OverflowScroll
	OverflowAuto
)

type Float uint8

const (
	FloatNone Float = iota
	FloatLeft
	FloatRight
)

type Clear uint8

const (
	ClearNone Clear = iota
	ClearLeft
	ClearRight
	ClearBoth
)

type TextAlign uint8

const (
	TextAlignStart TextAlign = iota
	TextAlignLeft
	TextAlignRight
	TextAlignCenter
	TextAlignJustify
)

type WhiteSpace uint8

const (
	WhiteSpaceNormal WhiteSpace = iota
	WhiteSpaceNowrap
	WhiteSpacePre
	WhiteSpacePrewrite
	WhiteSpacePreline
)

type FontWeight uint16

const (
	WeightNormal FontWeight = 400
	WeightBold   FontWeight = 700
)

type FontStyle uint8

const (
	FontStyleNormal FontStyle = iota
	FontStyleItalic
	FontStyleOblique
)

type TextDecoration uint8

const (
	TextDecorationNone      TextDecoration = 0
	TextDecorationUnderline TextDecoration = 1 << iota
	TextDecorationOverline
	TextDecorationLineThrough
)

type BgRepeat uint8

const (
	BgRepeatRepeat BgRepeat = iota
	BgRepeatRepeatX
	BgRepeatRepeatY
	BgRepeatNoRepeat
	BgRepeatRound
	BgRepeatSpace
)

type BgBox uint8

const (
	BgBoxBorder BgBox = iota
	BgBoxPadding
	BgBoxContent
	BgBoxText
	BgBoxBorderArea
)

type BgSize uint8

const (
	BgSizeAuto BgSize = iota
	BgSizeContain
	BgSizeCover
	BgSizeLength
)

type BgPosMode uint8

const (
	BgPosStart BgPosMode = iota
	BgPosCenter
	BgPosEnd
	BgPosLength
	BgPosStartOffset
	BgPosEndOffset
)

type BoxSizing uint8

const (
	BoxSizingContentBox BoxSizing = iota
	BoxSizingBorderBox
)

type ListStyleType uint8

const (
	ListStyleNone ListStyleType = iota
	ListStyleDisc
	ListStyleCircle
	ListStyleSquare
	ListStyleDecimal
	ListStyleDecimalLeadingZero
	ListStyleUpperRoman
	ListStyleLowerRoman
	ListStyleUpperAlpha
	ListStyleLowerAlpha
	ListStyleGeorgian
	ListStyleArmenian
)

type FlexDirection uint8

const (
	FlexRow FlexDirection = iota
	FlexRowReverse
	FlexColumn
	FlexColumnReverse
)

type FlexWrap uint8

const (
	FlexNowrap FlexWrap = iota
	FlexWrapValue
	FlexWrapReverse
)

type StartingStyle struct {
	Opacity    float32
	HasOpacity bool
	Display    Display
	Values     map[string]string
}

type ComputedStyle struct {
	Custom      map[string]string
	Starting    *StartingStyle
	Display     Display
	FlowRoot    bool
	Position    Position
	Float       Float
	Clear       Clear
	Overflow    Overflow
	OverflowX   Overflow
	OverflowY   Overflow
	BoxSizing   BoxSizing
	Visibility  string
	ColorScheme string
	isDark      bool

	ContentVisibility      string
	ContainIntrinsicWidth  float32
	ContainIntrinsicHeight float32
	AnchorName             string
	PositionAnchor         string
	PositionAnchorNodeID   dom.NodeID
	PositionArea           string
	TopAnchor              string
	RightAnchor            string
	BottomAnchor           string
	LeftAnchor             string
	WidthAnchor            string
	HeightAnchor           string

	TextWrap           string
	LineClamp          int
	ViewTransitionName string

	ScrollTimelineName string
	ScrollTimelineAxis string
	ViewTimelineName   string
	ViewTimelineAxis   string
	ViewTimelineInset  string
	AnimationTimeline  string

	FontPalette    string
	OffsetPath     string
	OffsetDistance float32
	OffsetRotate   string

	ShapeOutside string
	ShapeMargin  float32

	HasClip    bool
	ClipTop    float32
	ClipRight  float32
	ClipBottom float32
	ClipLeft   float32
	Opacity    float32
	ZIndex     int32
	HasZIndex  bool

	Width     float32
	Height    float32
	MinWidth  float32
	MinHeight float32
	MaxWidth  float32
	MaxHeight float32

	MinWidthAuto bool

	WidthCalc     DeferredLength
	HeightCalc    DeferredLength
	MinWidthCalc  DeferredLength
	MinHeightCalc DeferredLength
	MaxWidthCalc  DeferredLength
	MaxHeightCalc DeferredLength

	MarginTop       float32
	MarginRight     float32
	MarginBottom    float32
	MarginLeft      float32
	MarginLeftAuto  bool
	MarginRightAuto bool

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

	OutlineWidth  float32
	OutlineStyle  string
	OutlineColor  css.Color
	OutlineOffset float32

	BorderImage css.BorderImage

	BorderRadius [4][2]float32

	Top                                  float32
	Right                                float32
	Bottom                               float32
	Left                                 float32
	HasTop, HasRight, HasBottom, HasLeft bool

	Color                                  css.Color
	BackgroundColor                        css.Color
	BackgroundGradient                     Gradient
	BackgroundRadialGradient               RadialGradient
	BackgroundImage                        string
	MaskImage                              string
	MaskRepeat                             string
	MaskSize                               string
	MaskPosition                           string
	BackgroundRepeat                       BgRepeat
	BackgroundRepeatY                      BgRepeat
	BackgroundSize                         BgSize
	BackgroundOrigin                       BgBox
	BackgroundClip                         BgBox
	BgSizeW, BgSizeH                       float32
	BgSizeWPct, BgSizeHPct                 bool
	BackgroundPosXMode, BackgroundPosYMode BgPosMode
	BackgroundPosX, BackgroundPosY         float32
	BgPosXPct, BgPosYPct                   bool

	FontFamily      frame.FontFamily
	CustomFamily    string
	CustomFontIdx   uint16
	FontSize        float32
	rootFontSize    float32
	FontWeight      FontWeight
	FontStyle       FontStyle
	LineHeight      float32
	LineHeightRatio float32
	LineHeightEm    float32
	Direction       string
	UnicodeBidi     string
	TextAlign       TextAlign
	TextIndent      float32
	TextTransform   string
	TextDecoration  TextDecoration
	WhiteSpace      WhiteSpace
	WordSpacing     float32
	LetterSpacing   float32

	VerticalAlign string

	ListStyleType ListStyleType

	FlexDirection  FlexDirection
	FlexWrap       FlexWrap
	JustifyContent string
	JustifySelf    string
	AlignItems     string
	AlignSelf      string
	FlexGrow       float32
	FlexShrink     float32
	FlexBasis      float32
	RowGap         float32
	ColumnGap      float32
	ObjectFit      string
	Order          int

	GridTemplateColumns string
	GridTemplateRows    string
	GridTemplateAreas   string
	GridArea            string
	GridColumnSpan      int
	GridRowSpan         int
	GridColumn          string
	GridRow             string
	JustifyItems        string
	AlignContent        string

	TableLayout    string
	BorderCollapse bool
	BorderSpacingH float32
	BorderSpacingV float32

	Content          string
	ContentParts     []ContentPart
	CounterReset     map[string]int
	CounterSet       map[string]int
	CounterIncrement map[string]int

	BoxShadow  []css.BoxShadow
	TextShadow []css.TextShadow

	Transitions []css.Transition

	Transform       []css.TransformFunc
	TransformOrigin css.TransformOrigin

	Filter         []css.FilterFunc
	BackdropFilter []css.FilterFunc

	Quotes []string

	AspectRatio float32

	AnimationName      string
	AnimationDuration  time.Duration
	AnimationTiming    css.TimingFunc
	AnimationDelay     time.Duration
	AnimationIterCount int
	AnimationDirection css.AnimationDirection
	AnimationFillMode  css.AnimationFillMode

	AccentColor css.Color

	ContainerType string
	ContainerName string

	ScrollSnapType       string
	ScrollSnapStrictness string
	ScrollSnapAlign      string
	ScrollSnapStop       string
	ScrollPaddingTop     float32
	ScrollPaddingRight   float32
	ScrollPaddingBottom  float32
	ScrollPaddingLeft    float32
	ScrollMarginTop      float32
	ScrollMarginRight    float32
	ScrollMarginBottom   float32
	ScrollMarginLeft     float32

	TextUnderlineOffset     float32
	TextDecorationThickness float32

	PointerEvents string
	UserSelect    string
	CaretColor    css.Color

	OverscrollBehaviorX string
	OverscrollBehaviorY string

	WordBreak string
	Hyphens   string
	TabSize   float32

	WritingMode        string
	TextOrientation    string
	FontVariantNumeric string
}

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
	s.MinWidthAuto = true
	s.MarginTop, s.MarginRight, s.MarginBottom, s.MarginLeft = 0, 0, 0, 0
	s.MarginLeftAuto, s.MarginRightAuto = false, false
	s.PaddingTop, s.PaddingRight, s.PaddingBottom, s.PaddingLeft = 0, 0, 0, 0
	s.BorderTopWidth, s.BorderRightWidth, s.BorderBottomWidth, s.BorderLeftWidth = 0, 0, 0, 0
	s.OutlineWidth, s.OutlineOffset = 0, 0
	s.OutlineStyle = ""
	s.OutlineColor = css.Color{}
	s.BackgroundColor = css.Color{}
	s.BackgroundImage = ""
	s.BackgroundGradient = Gradient{}
	s.BackgroundRadialGradient = RadialGradient{}
	s.BoxShadow = nil
	s.BorderRadius = [4][2]float32{}
	s.MaskImage = ""
	s.MaskRepeat = ""
	s.MaskSize = ""
	s.MaskPosition = ""
	s.BorderImage = css.BorderImage{}
	return &s
}

func AnonymousTableStyle(parent *ComputedStyle, d Display) *ComputedStyle {
	var s ComputedStyle
	if parent == nil {
		s = DefaultStyle()
	} else {
		s = *parent
	}
	s.Display = d
	s.Position = PositionStatic
	s.Width = -1
	s.Height = -1
	s.MinWidth, s.MinHeight, s.MaxWidth, s.MaxHeight = 0, 0, -1, -1
	s.MinWidthAuto = true
	s.MarginTop, s.MarginRight, s.MarginBottom, s.MarginLeft = 0, 0, 0, 0
	s.MarginLeftAuto, s.MarginRightAuto = false, false
	s.PaddingTop, s.PaddingRight, s.PaddingBottom, s.PaddingLeft = 0, 0, 0, 0
	s.BorderTopWidth, s.BorderRightWidth, s.BorderBottomWidth, s.BorderLeftWidth = 0, 0, 0, 0
	s.BorderRadius = [4][2]float32{}
	s.OutlineWidth, s.OutlineOffset = 0, 0
	s.OutlineStyle = ""
	s.OutlineColor = css.Color{}
	s.BackgroundColor = css.Color{}
	s.BackgroundImage = ""
	s.MaskImage = ""
	s.MaskRepeat = ""
	s.MaskSize = ""
	s.MaskPosition = ""
	s.BackgroundGradient = Gradient{}
	s.BackgroundRadialGradient = RadialGradient{}
	s.BoxShadow = nil
	s.BorderImage = css.BorderImage{}
	return &s
}

func DefaultStyle() ComputedStyle {
	return ComputedStyle{
		Display:                DisplayInline,
		Position:               PositionStatic,
		Direction:              "ltr",
		Opacity:                1.0,
		Width:                  -1,
		Height:                 -1,
		MinWidth:               0,
		MinHeight:              0,
		MaxWidth:               -1,
		MaxHeight:              -1,
		MinWidthAuto:           true,
		Color:                  css.Color{R: 0, G: 0, B: 0, A: 255},
		FontSize:               16,
		rootFontSize:           16,
		FontWeight:             WeightNormal,
		LineHeight:             -1,
		FontFamily:             frame.FontTimes,
		FlexShrink:             1,
		Visibility:             "visible",
		WhiteSpace:             WhiteSpaceNormal,
		FlexBasis:              -1,
		Content:                "normal",
		BackgroundOrigin:       BgBoxPadding,
		BackgroundClip:         BgBoxBorder,
		ContainerType:          "normal",
		ContainerName:          "",
		ContentVisibility:      "visible",
		ContainIntrinsicWidth:  -1,
		ContainIntrinsicHeight: -1,
		TextWrap:               "wrap",
		LineClamp:              0,
		ViewTransitionName:     "none",
		ScrollTimelineName:     "none",
		ScrollTimelineAxis:     "block",
		ViewTimelineName:       "none",
		ViewTimelineAxis:       "block",
		ViewTimelineInset:      "auto",
		AnimationTimeline:      "auto",
		FontPalette:            "normal",
		OffsetPath:             "none",
		OffsetDistance:         0,
		OffsetRotate:           "auto",
		ShapeOutside:           "none",
		ShapeMargin:            0,
		Quotes:                 []string{"“", "”", "‘", "’"},
		AccentColor:            css.Color{},
		AspectRatio:            0,
		ScrollSnapType:         "none",
		ScrollSnapStrictness:   "proximity",
		ScrollSnapAlign:        "none",
		ScrollSnapStop:         "normal",
		PointerEvents:          "auto",
		UserSelect:             "auto",
		CaretColor:             css.Color{},
		OverscrollBehaviorX:    "auto",
		OverscrollBehaviorY:    "auto",
		WordBreak:              "normal",
		Hyphens:                "manual",
		TabSize:                8,
		WritingMode:            "horizontal-tb",
		TextOrientation:        "mixed",
		FontVariantNumeric:     "normal",
	}
}

func (s *ComputedStyle) UsedBorderWidths() [4]float32 {
	w := [4]float32{s.BorderTopWidth, s.BorderRightWidth, s.BorderBottomWidth, s.BorderLeftWidth}
	st := [4]string{s.BorderTopStyle, s.BorderRightStyle, s.BorderBottomStyle, s.BorderLeftStyle}
	for i, t := range st {
		if t == "" || t == "none" || t == "hidden" {
			w[i] = 0
		}
	}
	return w
}

func (cs *ComputedStyle) FontSlot() frame.FontSlot {
	if cs == nil {
		return frame.FontSlot{}
	}
	slot := frame.FontSlot{
		Family:    cs.FontFamily,
		Bold:      cs.FontWeight >= WeightBold,
		Italic:    cs.FontStyle != FontStyleNormal,
		Light:     cs.FontWeight > 0 && cs.FontWeight < WeightNormal,
		CustomIdx: cs.CustomFontIdx,
	}
	return slot
}

func UserAgentStylesheet() *css.Stylesheet { return uaSheet }

var uaSheet = css.Parse(uaCSS)

var uaCSS = `
table {
	box-sizing: border-box;
}
html { display: block; }
head { display: none; }
style, script, title, meta, link, template, noscript { display: none; }
[hidden] { display: none; }
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
tfoot { display: table-footer-group; }
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
search { display: block; }
section { display: block; }
article { display: block; }
aside { display: block; }
figure { display: block; margin: 1em 40px; }
figcaption { display: block; }
details { display: block; }
summary { display: block; }
/* A closed <details> hides its remaining content without removing its layout
   boxes: CSSOM geometry APIs must still report the closed subtree's box sizes. */
details:not([open]) > *:not(summary) { content-visibility: hidden; }
dialog { display: none; position: absolute; left: 0; right: 0; margin: auto; border: solid 1px; padding: 1em; background: white; color: black; }
dialog[open] { display: block; }
[popover] { display: none; position: fixed; inset: auto; margin: auto; border: solid 1px; padding: 0.25em; background: white; color: black; }
[popover]:popover-open { display: block; }
fieldset { display: block; margin: 0 2px; padding: 0.35em 0.75em 0.625em; border: 2px groove; }
legend { display: block; padding: 0 0.25em; }
form { display: block; }
label { display: inline; }
select { display: inline-block; }
progress { display: inline-block; width: 10em; height: 1em; vertical-align: -0.2em; }
meter { display: inline-block; width: 5em; height: 1em; vertical-align: -0.2em; }
datalist { display: none; }
/* A select is a replaced control: its option list is widget data, not content.
   Without this every option paints into the page's normal flow. */
option, optgroup { display: none; }
ruby { display: inline; }
rt { display: inline-block; font-size: 50%; line-height: 1; vertical-align: super; }
rp { display: none; }
picture { display: inline; }
source { display: none; }
/* MathML Core Foundation - basic display rules for MathML elements */
math { display: inline-block; }
mrow { display: inline; }
mfrac { display: inline-flex; flex-direction: column; vertical-align: middle; text-align: center; }
msup, msub { display: inline; font-size: 0.8em; }
msqrt { display: inline-block; }

/* A text or tick control is a replaced box: Chrome gives it a box of its own
   whose size comes from the control rather than from content. The date, time
   and colour controls are replaced boxes too even though the engine has no
   picker behind them: keeping them inline collapsed each one to 0x0, so the
   border and padding a page sets on it painted nothing and the row it sat in
   lost its height (five blank bands in forms/04-advanced-inputs). */
input { display: inline-block; background-color: #ffffff; }
/* Chromium lays every control out in its own font rather than the document's.
   Measured with getComputedStyle 2026-11-12 (standards mode, body at 16px
   sans-serif): 13.3333px weight 400 in Arial for the fields, buttons,
   selects and the colour swatch, the same size in monospace for textarea and
   the date family, and line-height normal for all of them. It belongs in the
   cascade, not in a magic height in layout, because that is the only origin an
   author can beat: a declared font-size wins here exactly as it does in the
   reference (measured 24px on a field: 258x34 there, 153x34 here - the width
   is GOOSIE-CONTROLWIDTH-001). A CSS-wide keyword in the shorthand does not
   win yet, which is GOOSIE-FONTSHORTHANDWIDE-001. */
input, select, button { font: 400 13.3333px Arial; font-style: normal; }
textarea, input[type=date], input[type=time], input[type=month],
input[type=week], input[type=datetime-local] { font: 400 13.3333px monospace; font-style: normal; }
/* The chrome each control carries: measured 2026-11-12 with getComputedStyle,
   the style, the width and the box-sizing of every side plus the background, and
   it is what turns one line of the control font into the box the reference
   draws. A text field's 21px is a 15px line with 1px of padding and a 2px
   border either side; a select's 19px is its own 17px interior inside a 1px
   solid frame; a file, tick or range control carries no border and no padding,
   and the tick and file controls are transparent because the reference is.
   inset and outset are the reference's styles, and Goosie paints both as a
   solid strip in the border colour (GOOSIE-BEVELPAINT-001): the field frame's
   rgb(118,118,118) is what the reference computes and so reads correctly as a
   flat strip, while a button's border computes to currentColor because the
   reference derives its bevel from the face colour - so the #767676 below is
   the mid-grey Goosie has always painted a button frame with, standing in for
   a bevel, not a measured colour. */
input { box-sizing: content-box; border: 2px inset rgb(118, 118, 118); padding: 1px 2px; }
input[type=date], input[type=time], input[type=month],
input[type=week], input[type=datetime-local] { border: 2px inset rgb(118, 118, 118); padding: 0 0 0 1px; }
input[type=search], input[type=button], input[type=submit], input[type=reset],
input[type=checkbox], input[type=radio], input[type=color], select, button { box-sizing: border-box; }
input[type=file], input[type=range], input[type=checkbox],
input[type=radio], input[type=image] { border: 0; padding: 0; }
input[type=file], input[type=checkbox], input[type=radio], input[type=image] { background-color: transparent; }
input[type=button], input[type=submit], input[type=reset], button { background-color: #efefef; padding: 1px 6px; border: 2px outset #767676; }
input[type=color] { background-color: #efefef; border: 1px solid; padding: 1px 2px; }
textarea { display: inline-block; background-color: #ffffff; border: 1px solid rgb(118, 118, 118); padding: 2px; white-space: pre-wrap; }
textarea[wrap="off" i] { white-space: pre; }
select { display: inline-block; background-color: #ffffff; border: 1px solid rgb(118, 118, 118); padding: 0; }
/* Chrome lays button-like inputs out as inline-level boxes that shrink-wrap
   their value label. display:inline would send them down the generic inline
   descent with no children to measure, collapsing every submit button to 0x0,
   so they are inline-block like <button> below and sized by replacedSize. */
input[type=submit], input[type=reset], input[type=button] { display: inline-block; }
input[type=hidden] { display: none; }
/* The appearance-only widgets carry UA margins, and they are not decoration:
   a checkbox's baseline sits on its bottom *border* edge, so the 3px above and
   3px below are what lift a 13px box out of a 16px strut into a 20px line at
   offset 3 (Chromium computed margins, measured 2026-11-12: checkbox 3/3/3/4,
   radio 3/3/0/5, range 2 all round). They live in the cascade rather than in
   layout because an author margin: 0 has to win, as it does in the reference. */
input[type=checkbox] { margin: 3px 3px 3px 4px; }
input[type=radio] { margin: 3px 3px 0 5px; }
input[type=range] { margin: 2px; }
/* Chrome lays a button out as an inline-level box that shrinks to its label, so
   a row of buttons sits side by side and the label centres in it. The control
   font and the platform chrome it draws that box with are the shared rules
   above, where an author font-size or border can beat them. */
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
video, canvas { display: inline-block; }
audio { display: inline-block; }
audio:not([controls]) { display: none; }
/* SVG elements: <svg> is a replaced inline box; container and definition elements
   are not rendered directly; shape and text elements are inline within the SVG
   coordinate system. */
svg { display: inline-block; overflow: hidden; }
g, defs, symbol, clipPath, mask, pattern, marker { display: none; }
linearGradient, radialGradient, stop, filter,
feBlend, feColorMatrix, feComponentTransfer, feComposite,
feConvolveMatrix, feDiffuseLighting, feDisplacementMap, feFlood,
feGaussianBlur, feImage, feMerge, feMergeNode, feMorphology,
feOffset, feSpecularLighting, feTile, feTurbulence,
animate, animateTransform, animateMotion, set, mpath,
title, desc, metadata { display: none; }
rect, circle, ellipse, line, polyline, polygon, path { display: inline; }
text, tspan, textPath { display: inline; }
foreignObject { display: block; }
image { display: inline; }
q::before { content: open-quote; }
q::after { content: close-quote; }
`

type Viewport struct {
	W, H     float32
	DarkMode bool
}

func usedColorScheme(supported string, prefersDark bool) string {
	light, dark := false, false
	for _, token := range strings.Fields(strings.ToLower(supported)) {
		switch token {
		case "light":
			light = true
		case "dark":
			dark = true
		}
	}
	if light && dark {
		if prefersDark {
			return "dark"
		}
		return "light"
	}
	if dark {
		return "dark"
	}
	if light {
		return "light"
	}
	if prefersDark {
		return "dark"
	}
	return "light"
}

func (cs *ComputedStyle) effectiveColorScheme(vp Viewport) string {
	if cs != nil {
		return usedColorScheme(cs.ColorScheme, vp.DarkMode)
	}
	if vp.DarkMode {
		return "dark"
	}
	return "light"
}

func (cs *ComputedStyle) scheme() string {
	if cs != nil && cs.isDark {
		return "dark"
	}
	return "light"
}

func inViewport(d css.Declaration, vp Viewport) css.Declaration {
	if vp.W <= 0 || vp.H <= 0 {
		return d
	}
	v := d.Parsed
	if v.Type == css.ValueLength {
		switch v.Unit {
		case "vw", "vh", "vmin", "vmax", "svw", "svh", "dvw", "dvh", "lvw", "lvh", "vi", "vb", "svmin", "svmax", "dvmin", "dvmax", "lvmin", "lvmax":
		default:
			return d
		}
		d.Parsed = css.Value{Type: css.ValueLength, Num: float64(v.ToLengthWithContext(0, vp.W, vp.H)), Unit: "px"}
		return d
	}
	if strings.IndexAny(d.Value, "0123456789") < 0 {
		return d
	}
	text, ok := css.ResolveViewportUnits(d.Value, vp.W, vp.H)
	if !ok {
		return d
	}
	d.Value = text
	d.Parsed = css.ParseValue(text)
	return d
}

func inContainerUnits(d css.Declaration, cqW, cqH float32) css.Declaration {
	if cqW <= 0 {
		cqW = 1440
	}
	if cqH <= 0 {
		cqH = 900
	}
	v := d.Parsed
	if v.Type == css.ValueLength {
		switch v.Unit {
		case "cqw", "cqi", "cqh", "cqb", "cqmin", "cqmax":
			px := v.ToLengthWithContainerContext(16, 16, cqW, cqH, cqW, cqH)
			d.Parsed = css.Value{Type: css.ValueLength, Num: float64(px), Unit: "px"}
			return d
		}
	}
	if strings.IndexAny(d.Value, "0123456789") < 0 {
		return d
	}
	text, ok := css.ResolveContainerUnits(d.Value, cqW, cqH)
	if !ok {
		return d
	}
	d.Value = text
	d.Parsed = css.ParseValue(text)
	return d
}

func findNearestContainerDimensions(n *dom.Node, result map[dom.NodeID]*ComputedStyle, vp Viewport) (float32, float32) {
	contNode, contStyle := findQueryContainer(n, "", false, result)
	if contNode == nil || contStyle == nil {
		vw, vh := vp.W, vp.H
		if vw <= 0 {
			vw = 1440
		}
		if vh <= 0 {
			vh = 900
		}
		return vw, vh
	}
	return getContainerDimensions(contNode, contStyle, result, vp)
}

func findQueryContainer(n *dom.Node, name string, needsBlock bool, result map[dom.NodeID]*ComputedStyle) (*dom.Node, *ComputedStyle) {
	if n == nil {
		return nil, nil
	}
	for p := n.Parent; p != nil; p = p.Parent {
		cs := result[p.ID]
		if cs == nil {
			continue
		}
		if name != "" {
			matchedName := false
			if strings.EqualFold(cs.ContainerName, name) {
				matchedName = true
			} else {
				for _, f := range strings.Fields(cs.ContainerName) {
					if strings.EqualFold(f, name) {
						matchedName = true
						break
					}
				}
			}
			if !matchedName {
				continue
			}
		}
		if needsBlock {
			if cs.ContainerType == "size" {
				return p, cs
			}
		} else {
			if cs.ContainerType == "inline-size" || cs.ContainerType == "size" {
				return p, cs
			}
		}
	}
	return nil, nil
}

func getContainingBlockWidth(n *dom.Node, result map[dom.NodeID]*ComputedStyle, vp Viewport) float32 {
	if n == nil {
		if vp.W > 0 {
			return vp.W
		}
		return 1280
	}
	for p := n.Parent; p != nil; p = p.Parent {
		cs := result[p.ID]
		if cs == nil {
			continue
		}
		if cs.Width >= 0 {
			return cs.Width
		}
		if cs.Width < -2 {
			pct := -2 - cs.Width
			base := getContainingBlockWidth(p, result, vp)
			return pct * base / 100
		}
	}
	if vp.W > 0 {
		return vp.W
	}
	return 1280
}

func getContainingBlockHeight(n *dom.Node, result map[dom.NodeID]*ComputedStyle, vp Viewport) float32 {
	if n == nil {
		if vp.H > 0 {
			return vp.H
		}
		return 900
	}
	for p := n.Parent; p != nil; p = p.Parent {
		cs := result[p.ID]
		if cs == nil {
			continue
		}
		if cs.Height >= 0 {
			return cs.Height
		}
		if cs.Height < -2 {
			pct := -2 - cs.Height
			base := getContainingBlockHeight(p, result, vp)
			return pct * base / 100
		}
	}
	if vp.H > 0 {
		return vp.H
	}
	return 900
}

func getContainerDimensions(n *dom.Node, cs *ComputedStyle, result map[dom.NodeID]*ComputedStyle, vp Viewport) (float32, float32) {
	var w, h float32
	if cs.Width >= 0 {
		w = cs.Width
	} else if cs.Width < -2 {
		pct := -2 - cs.Width
		baseW := getContainingBlockWidth(n, result, vp)
		w = pct * baseW / 100
	} else {
		w = getContainingBlockWidth(n, result, vp)
		if cs.BoxSizing == BoxSizingContentBox {
			w -= (cs.PaddingLeft + cs.PaddingRight + cs.BorderLeftWidth + cs.BorderRightWidth)
			if w < 0 {
				w = 0
			}
		}
	}

	if cs.Height >= 0 {
		h = cs.Height
	} else if cs.Height < -2 {
		pct := -2 - cs.Height
		baseH := getContainingBlockHeight(n, result, vp)
		h = pct * baseH / 100
	} else {
		h = 0
	}
	return w, h
}

func matchesContainerQuery(n *dom.Node, query string, result map[dom.NodeID]*ComputedStyle, vp Viewport) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	clauses := css.SplitOutsideParens(query, " and ")
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		name := ""
		cond := clause
		if idx := strings.IndexByte(clause, '('); idx > 0 {
			name = strings.TrimSpace(clause[:idx])
			cond = strings.TrimSpace(clause[idx:])
		}
		condLower := strings.ToLower(cond)
		needsBlock := strings.Contains(condLower, "height") || strings.Contains(condLower, "block-size")
		contNode, contStyle := findQueryContainer(n, name, needsBlock, result)
		if contNode == nil || contStyle == nil {
			return false
		}
		w, h := getContainerDimensions(contNode, contStyle, result, vp)
		if !evalContainerCondition(cond, w, h) {
			return false
		}
	}
	return true
}

func evalContainerCondition(cond string, w, h float32) bool {
	cond = strings.TrimSpace(cond)
	if cond == "" {
		return true
	}
	for _, group := range css.SplitTopLevel(cond, ',') {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if evalContainerGroup(group, w, h) {
			return true
		}
	}
	return false
}

func evalContainerGroup(group string, w, h float32) bool {
	if alts := css.SplitOutsideParens(group, " or "); len(alts) > 1 {
		for _, alt := range alts {
			if evalContainerGroup(strings.TrimSpace(alt), w, h) {
				return true
			}
		}
		return false
	}
	if parts := css.SplitOutsideParens(group, " and "); len(parts) > 1 {
		for _, part := range parts {
			if !evalContainerGroup(strings.TrimSpace(part), w, h) {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(strings.ToLower(group), "not ") {
		return !evalContainerGroup(strings.TrimSpace(group[4:]), w, h)
	}
	test := strings.TrimSpace(group)
	if strings.HasPrefix(test, "(") && strings.HasSuffix(test, ")") {
		test = strings.TrimSpace(test[1 : len(test)-1])
	}
	if strings.ContainsAny(test, "<>=") {
		return evalContainerRange(test, w, h)
	}
	colon := strings.IndexByte(test, ':')
	if colon < 0 {
		return false
	}
	feature := strings.ToLower(strings.TrimSpace(test[:colon]))
	val := strings.TrimSpace(test[colon+1:])
	return evalSingleContainerFeature(feature, val, w, h)
}

func evalSingleContainerFeature(feature, val string, w, h float32) bool {
	switch feature {
	case "min-width", "min-inline-size":
		if px, ok := parseCSSPx(val); ok {
			return w >= px
		}
	case "max-width", "max-inline-size":
		if px, ok := parseCSSPx(val); ok {
			return w <= px
		}
	case "width", "inline-size":
		if px, ok := parseCSSPx(val); ok {
			return float32(math.Abs(float64(w-px))) < 0.5
		}
	case "min-height", "min-block-size":
		if px, ok := parseCSSPx(val); ok {
			return h >= px
		}
	case "max-height", "max-block-size":
		if px, ok := parseCSSPx(val); ok {
			return h <= px
		}
	case "height", "block-size":
		if px, ok := parseCSSPx(val); ok {
			return float32(math.Abs(float64(h-px))) < 0.5
		}
	case "orientation":
		v := strings.ToLower(strings.TrimSpace(val))
		if v == "portrait" {
			return h > w
		} else if v == "landscape" {
			return w >= h
		}
	case "aspect-ratio":
		parts := strings.Split(val, "/")
		if len(parts) == 2 {
			rW, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 32)
			rH, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 32)
			if err1 == nil && err2 == nil && rH > 0 && h > 0 {
				ratio := float32(rW / rH)
				actual := w / h
				return float32(math.Abs(float64(actual-ratio))) < 0.05
			}
		}
	}
	return false
}

func evalContainerRange(expr string, w, h float32) bool {
	var tokens []string
	var ops []string
	start := 0
	i := 0
	for i < len(expr) {
		if expr[i] == '<' || expr[i] == '>' || expr[i] == '=' {
			token := strings.TrimSpace(expr[start:i])
			if token != "" {
				tokens = append(tokens, token)
			}
			opStart := i
			for i < len(expr) && (expr[i] == '<' || expr[i] == '>' || expr[i] == '=') {
				i++
			}
			ops = append(ops, expr[opStart:i])
			start = i
			continue
		}
		i++
	}
	if start < len(expr) {
		token := strings.TrimSpace(expr[start:])
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	if len(tokens) == 2 && len(ops) == 1 {
		featIdx, valIdx := -1, -1
		if isContainerFeatureName(tokens[0]) {
			featIdx, valIdx = 0, 1
		} else if isContainerFeatureName(tokens[1]) {
			featIdx, valIdx = 1, 0
		} else {
			return false
		}
		valPx, ok := parseCSSPx(tokens[valIdx])
		if !ok {
			return false
		}
		valDim := getFeatureDim(tokens[featIdx], w, h)
		op := ops[0]
		if featIdx == 1 {
			op = flipContainerOp(op)
		}
		return compareDim(valDim, op, valPx)
	} else if len(tokens) == 3 && len(ops) == 2 {
		if !isContainerFeatureName(tokens[1]) {
			return false
		}
		loPx, okLo := parseCSSPx(tokens[0])
		hiPx, okHi := parseCSSPx(tokens[2])
		if !okLo || !okHi {
			return false
		}
		valDim := getFeatureDim(tokens[1], w, h)
		op1 := flipContainerOp(ops[0])
		op2 := ops[1]
		return compareDim(valDim, op1, loPx) && compareDim(valDim, op2, hiPx)
	}
	return false
}

func isContainerFeatureName(name string) bool {
	switch strings.ToLower(name) {
	case "width", "inline-size", "height", "block-size":
		return true
	}
	return false
}

func getFeatureDim(name string, w, h float32) float32 {
	switch strings.ToLower(name) {
	case "height", "block-size":
		return h
	default:
		return w
	}
}

func flipContainerOp(op string) string {
	switch op {
	case "<":
		return ">"
	case "<=":
		return ">="
	case ">":
		return "<"
	case ">=":
		return "<="
	}
	return op
}

func compareDim(dim float32, op string, target float32) bool {
	switch op {
	case "<":
		return dim < target
	case "<=":
		return dim <= target
	case ">":
		return dim > target
	case ">=":
		return dim >= target
	case "=", "==":
		return float32(math.Abs(float64(dim-target))) < 0.5
	}
	return false
}

func parseCSSPx(val string) (float32, bool) {
	val = strings.TrimSpace(val)
	if val == "" {
		return 0, false
	}
	v := css.ParseValue(val)
	if v.Type == css.ValueLength || v.Type == css.ValueNumber {
		return v.ToLength(), true
	}
	return 0, false
}

func Resolve(doc *dom.Document, sheets []*css.Stylesheet, fonts *CustomFonts) map[dom.NodeID]*ComputedStyle {
	return ResolveViewport(doc, sheets, Viewport{}, fonts)
}

type Resolver struct {
	mu                         sync.Mutex
	index                      *ruleIndex
	incrementalInteractiveSafe bool
	hasSnapshot                bool
	snapshotViewport           Viewport
	snapshotInputs             treeInputSnapshot
	snapshotStates             map[dom.NodeID]interactiveSnapshot
	snapshotProperties         map[string]dom.PropertyDefinition
}

func NewResolver(sheets []*css.Stylesheet) *Resolver {
	allSheets := make([]*css.Stylesheet, 0, len(sheets)+1)
	allSheets = append(allSheets, UserAgentStylesheet())
	allSheets = append(allSheets, sheets...)
	index := buildRuleIndex(allSheets)
	interactiveSafe, _ := interactiveSelectorSupport(index.selectors)
	return &Resolver{index: index, incrementalInteractiveSafe: interactiveSafe}
}

func (r *Resolver) ResolveViewport(doc *dom.Document, vp Viewport, fonts *CustomFonts) map[dom.NodeID]*ComputedStyle {
	if r == nil || r.index == nil || doc == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.index.loadDocumentProperties(doc)
	result := make(map[dom.NodeID]*ComputedStyle)
	six := css.NewSiblingIndex()
	for c := doc.Node.FirstChild; c != nil; c = c.NextSibling {
		resolveNode(c, r.index, result, vp, fonts, six)
	}
	r.snapshotInteractiveState(doc, vp)
	return result
}

func (r *Resolver) ResolvePseudoElements(doc *dom.Document, elementStyles map[dom.NodeID]*ComputedStyle, fonts *CustomFonts) map[PseudoKey]*ComputedStyle {
	if r == nil || r.index == nil || doc == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return resolvePseudoElements(doc, r.index, elementStyles, fonts)
}

func ResolveViewport(doc *dom.Document, sheets []*css.Stylesheet, vp Viewport, fonts *CustomFonts) map[dom.NodeID]*ComputedStyle {
	return NewResolver(sheets).ResolveViewport(doc, vp, fonts)
}

func resolveNode(n *dom.Node, index *ruleIndex, result map[dom.NodeID]*ComputedStyle, vp Viewport, fonts *CustomFonts, six *css.SiblingIndex) {
	if n == nil {
		return
	}
	if n.Element() {
		parentStyle := findParentStyle(n, result)
		cs := computeStyle(n, index, parentStyle, vp, fonts, six, result)
		result[n.ID] = cs
	} else if n.Text() {
		parentStyle := findParentStyle(n, result)
		if parentStyle != nil {
			textStyle := inheritStyle(parentStyle)
			result[n.ID] = textStyle
		}
	}
	if n.ShadowRoot != nil {
		var shadowSheets []*css.Stylesheet
		collectShadowStyleSheets(n.ShadowRoot.FirstChild, &shadowSheets)
		partSheet := &css.Stylesheet{}
		for _, indexed := range index.part {
			rule := *indexed.rule
			rule.Selectors = []css.Selector{indexed.sel}
			partSheet.Rules = append(partSheet.Rules, rule)
		}
		if len(partSheet.Rules) > 0 {
			shadowSheets = append(shadowSheets, partSheet)
		}
		for _, text := range n.ShadowRoot.AdoptedStyleSheetTexts {
			if strings.TrimSpace(text) != "" {
				shadowSheets = append(shadowSheets, css.Parse(text))
			}
		}
		shadowIndex := index
		if len(shadowSheets) > 0 {
			allShadowSheets := []*css.Stylesheet{UserAgentStylesheet()}
			allShadowSheets = append(allShadowSheets, shadowSheets...)
			shadowIndex = buildRuleIndex(allShadowSheets)
			if n.Doc != nil {
				shadowIndex.loadDocumentProperties(n.Doc)
			}

			pSize := float32(16)
			if pStyle := findParentStyle(n, result); pStyle != nil {
				pSize = pStyle.FontSize
			}
			applyHostStyles(n, shadowSheets, result[n.ID], pSize, vp, fonts, six)
		}
		for sc := n.ShadowRoot.FirstChild; sc != nil; sc = sc.NextSibling {
			resolveNode(sc, shadowIndex, result, vp, fonts, six)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		resolveNode(c, index, result, vp, fonts, six)
	}
}

func collectShadowStyleSheets(root *dom.Node, sheets *[]*css.Stylesheet) {
	for c := root; c != nil; c = c.NextSibling {
		if c.Element() && c.Data == "style" {
			text := c.TextContent()
			if strings.TrimSpace(text) != "" {
				*sheets = append(*sheets, css.Parse(text))
			}
		}
		if c.FirstChild != nil {
			collectShadowStyleSheets(c.FirstChild, sheets)
		}
	}
}

func applyHostStyles(n *dom.Node, shadowSheets []*css.Stylesheet, cs *ComputedStyle, parentFontSize float32, vp Viewport, fonts *CustomFonts, six *css.SiblingIndex) {
	if cs == nil || n == nil {
		return
	}
	var normal, important []cascadeEntry
	order := 0
	for _, sheet := range shadowSheets {
		for i := range sheet.Rules {
			rule := &sheet.Rules[i]
			matches := false
			for _, sel := range rule.Selectors {
				if sel.MatchesIndexed(n, six) {
					matches = true
					break
				}
			}
			if matches {
				for _, d := range rule.Declarations {
					e := cascadeEntry{
						decl:       inViewport(d, vp),
						origin:     2,
						layerOrder: 0,
						order:      order,
					}
					order++
					if d.Important {
						important = append(important, e)
					} else {
						normal = append(normal, e)
					}
				}
			}
		}
	}
	sortCascade(normal)
	sortCascade(important)
	vars := expanderFor(cs.Custom, normal, important, nil)
	for _, e := range normal {
		applyCascadeEntry(cs, e, parentFontSize, vars, fonts)
	}
	for _, e := range important {
		applyCascadeEntry(cs, e, parentFontSize, vars, fonts)
	}
}

func findParentStyle(n *dom.Node, result map[dom.NodeID]*ComputedStyle) *ComputedStyle {
	if slot := n.AssignedSlot(); slot != nil {
		if cs, ok := result[slot.ID]; ok {
			return cs
		}
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if cs, ok := result[p.ID]; ok {
			return cs
		}
	}
	return nil
}

func inheritStyle(parent *ComputedStyle) *ComputedStyle {
	if parent.Display == DisplayInline {
		return parent
	}
	cs := *parent
	cs.Display = DisplayInline
	return &cs
}

type cascadeEntry struct {
	decl       css.Declaration
	origin     int
	layerOrder int
	scopeDist  int
	a, b, c    int
	order      int
}

func layerPrecedenceRank(layerOrder int, important bool) int {
	if !important {
		if layerOrder == 0 {
			return 1_000_000_000
		}
		return layerOrder
	}
	if layerOrder == 0 {
		return 0
	}
	return 1_000_000_000 - layerOrder
}

func (e cascadeEntry) before(o cascadeEntry) bool {
	if e.origin != o.origin {
		return e.origin < o.origin
	}
	if e.origin > 0 {
		rE := layerPrecedenceRank(e.layerOrder, e.decl.Important)
		rO := layerPrecedenceRank(o.layerOrder, o.decl.Important)
		if rE != rO {
			return rE < rO
		}
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
	if e.scopeDist != o.scopeDist {
		if e.scopeDist == 0 && o.scopeDist > 0 {
			return true
		}
		if e.scopeDist > 0 && o.scopeDist == 0 {
			return false
		}
		return e.scopeDist > o.scopeDist
	}
	return e.order < o.order
}

func sortCascade(entries []cascadeEntry) {
	if len(entries) < 2 {
		return
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].before(entries[j]) })
}

func computeStyle(n *dom.Node, index *ruleIndex, parent *ComputedStyle, vp Viewport, fonts *CustomFonts, six *css.SiblingIndex, result map[dom.NodeID]*ComputedStyle) *ComputedStyle {
	cs := DefaultStyle()
	if parent != nil {
		inheritFromParent(&cs, parent)
	}

	parentFontSize := float32(16)
	if parent != nil {
		parentFontSize = parent.FontSize
	}

	var normal, important []cascadeEntry
	order := index.declCount
	cands := index.candidates(n)
	matched := cands[:0]
	var startingMatched []*indexedSelector
	ruleDecls, ruleImportant := 0, 0
	for _, cand := range cands {
		if cand.rule.StartingStyle {
			startingMatched = append(startingMatched, cand)
			continue
		}
		if cand.rule.ScopeRoot != "" || cand.rule.ScopeLimit != "" {
			var rootNode *dom.Node
			if cand.rule.ScopeRoot == "" {
				if n.Doc != nil {
					rootNode = &n.Doc.Node
				}
			} else {
				rootSel := css.ParseSelector(cand.rule.ScopeRoot)
				if rootSel.Matches(n) {
					rootNode = n
				} else {
					for cur := n.Parent; cur != nil; cur = cur.Parent {
						if cur.Element() && rootSel.Matches(cur) {
							rootNode = cur
							break
						}
					}
				}
			}
			if rootNode == nil {
				continue
			}
			if cand.rule.ScopeLimit != "" {
				limitSel := css.ParseSelector(cand.rule.ScopeLimit)
				blocked := false
				for cur := n; cur != nil && cur != rootNode; cur = cur.Parent {
					if cur.Element() && limitSel.Matches(cur) {
						blocked = true
						break
					}
				}
				if blocked {
					continue
				}
			}
			rootNode.IsScopeRoot = true
			matchedSel := cand.sel.MatchesIndexed(n, six)
			rootNode.IsScopeRoot = false
			if !matchedSel {
				continue
			}
		} else {
			matchedSelector := cand.sel.MatchesIndexed(n, six)
			if !matchedSelector && selectorHasPart(cand.sel) {
				for host := n.Parent; host != nil; host = host.Parent {
					if host.ShadowRoot != nil && isShadowDescendant(host, n) && cand.sel.MatchesPart(host, n) {
						matchedSelector = true
						break
					}
				}
			}
			if !matchedSelector {
				continue
			}
		}
		if cand.rule.ContainerQuery != "" && !matchesContainerQuery(n, cand.rule.ContainerQuery, result, vp) {
			continue
		}
		matched = append(matched, cand)
		for _, d := range cand.rule.Declarations {
			ruleDecls++
			if d.Important {
				ruleImportant++
			}
		}
	}

	var inlineDecls []css.Declaration
	if inline := n.GetAttribute("style"); inline != "" {
		inlineDecls = parseInlineDeclarations(inline)
	}

	hintDecls := presentationalHints(n)

	if cap := ruleDecls - ruleImportant + len(inlineDecls) + len(hintDecls); cap > 0 {
		normal = make([]cascadeEntry, 0, cap)
	}
	if cap := ruleImportant + len(inlineDecls); cap > 0 {
		important = make([]cascadeEntry, 0, cap)
	}

	cqW, cqH := findNearestContainerDimensions(n, result, vp)
	for _, cand := range matched {
		dist := computeScopeDist(n, cand.rule.ScopeRoot)
		for _, d := range cand.rule.Declarations {
			d = inContainerUnits(inViewport(d, vp), cqW, cqH)
			e := cascadeEntry{decl: d, origin: cand.origin, layerOrder: cand.layerOrder, scopeDist: dist, a: cand.a, b: cand.b, c: cand.c, order: cand.rank}
			if d.Important {
				important = append(important, e)
			} else {
				normal = append(normal, e)
			}
		}
	}

	for _, d := range inlineDecls {
		d = inContainerUnits(inViewport(d, vp), cqW, cqH)
		e := cascadeEntry{decl: d, origin: 2, a: 1, b: 1, c: 1, order: order}
		order++
		if d.Important {
			important = append(important, e)
		} else {
			normal = append(normal, e)
		}
	}

	for _, d := range hintDecls {
		normal = append(normal, cascadeEntry{decl: d, origin: 1, a: 0, b: 0, c: 0, order: -1})
	}

	sortCascade(normal)
	sortCascade(important)
	cs.Custom = resolveCustom(parent, normal, important, index.properties)
	cs.ColorScheme = resolveColorScheme(parent, normal, important)
	cs.isDark = usedColorScheme(cs.ColorScheme, vp.DarkMode) == "dark"
	vars := expanderFor(cs.Custom, normal, important, index.properties)
	for _, e := range normal {
		applyCascadeEntry(&cs, e, parentFontSize, vars, fonts)
	}
	for _, e := range important {
		applyCascadeEntry(&cs, e, parentFontSize, vars, fonts)
	}
	if len(startingMatched) > 0 {
		cs.Starting = resolveStartingStyle(startingMatched, vp, fonts, parentFontSize, vars)
	}

	for _, e := range normal {
		if againstOwnFont(e.decl.Property) {
			applyCascadeEntry(&cs, e, parentFontSize, vars, fonts)
		}
	}
	for _, e := range important {
		if againstOwnFont(e.decl.Property) {
			applyCascadeEntry(&cs, e, parentFontSize, vars, fonts)
		}
	}

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

	if cs.Float != FloatNone {
		switch cs.Display {
		case DisplayInline, DisplayInlineBlock, DisplayInlineFlex:
			cs.Display = DisplayBlock
		case DisplayInlineGrid:
			cs.Display = DisplayGrid
		}
	}

	if cs.LineHeightRatio > 0 {
		cs.LineHeight = cs.LineHeightRatio * cs.FontSize
	}
	if cs.LineHeightEm > 0 {
		cs.LineHeight = cs.LineHeightEm * cs.FontSize
	}

	resolveCurrentColor(&cs, matched, inlineDecls, hintDecls)
	resolveShadowColors(&cs, parent, normal, important)
	resolveCustomFont(&cs, fonts)
	if parent == nil {
		cs.rootFontSize = cs.FontSize
	}

	return &cs
}

func isShadowDescendant(host, n *dom.Node) bool {
	for child := n; child != nil; child = child.Parent {
		if child.Parent != host {
			continue
		}
		for rootChild := host.ShadowRoot.FirstChild; rootChild != nil; rootChild = rootChild.NextSibling {
			if rootChild == child {
				return true
			}
		}
		return false
	}
	return false
}

func computeScopeDist(n *dom.Node, scopeRoot string) int {
	if scopeRoot == "" || n == nil {
		return 0
	}
	rootSel := css.ParseSelector(scopeRoot)
	hops := 1
	if rootSel.Matches(n) {
		return hops
	}
	for cur := n.Parent; cur != nil; cur = cur.Parent {
		hops++
		if cur.Element() && rootSel.Matches(cur) {
			return hops
		}
	}
	return 0
}

func resolveStartingStyle(matched []*indexedSelector, vp Viewport, fonts *CustomFonts, parentFontSize float32, vars *varExpander) *StartingStyle {
	var normal, important []cascadeEntry
	for _, cand := range matched {
		for _, d := range cand.rule.Declarations {
			d = inViewport(d, vp)
			e := cascadeEntry{decl: d, origin: cand.origin, layerOrder: cand.layerOrder, a: cand.a, b: cand.b, c: cand.c, order: cand.rank}
			if d.Important {
				important = append(important, e)
			} else {
				normal = append(normal, e)
			}
		}
	}
	sortCascade(normal)
	sortCascade(important)

	st := &StartingStyle{
		Values: make(map[string]string),
	}
	for _, es := range [][]cascadeEntry{normal, important} {
		for _, e := range es {
			val := e.decl.Value
			if vars != nil && strings.Contains(val, "var(") {
				val = vars.expand(val)
			}
			st.Values[e.decl.Property] = val
			switch e.decl.Property {
			case "opacity":
				if op, err := strconv.ParseFloat(val, 32); err == nil {
					st.Opacity = float32(op)
					st.HasOpacity = true
				}
			case "display":
				st.Display = parseDisplay(val)
			}
		}
	}
	return st
}

func resolveShadowColors(cs *ComputedStyle, parent *ComputedStyle, normal, important []cascadeEntry) {
	if v, ok := winningDeclValue("box-shadow", normal, important); ok && strings.EqualFold(strings.TrimSpace(v), "inherit") {
		cs.BoxShadow = nil
		if parent != nil && len(parent.BoxShadow) > 0 {
			cs.BoxShadow = append([]css.BoxShadow(nil), parent.BoxShadow...)
		}
	}
	if v, ok := winningDeclValue("text-shadow", normal, important); ok {
		v = strings.TrimSpace(v)
		if strings.EqualFold(v, "inherit") || strings.EqualFold(v, "unset") {
			cs.TextShadow = nil
			if parent != nil && len(parent.TextShadow) > 0 {
				cs.TextShadow = append([]css.TextShadow(nil), parent.TextShadow...)
			}
		}
	}
}

func winningDeclValue(prop string, normal, important []cascadeEntry) (string, bool) {
	for i := len(important) - 1; i >= 0; i-- {
		if important[i].decl.Property == prop {
			return important[i].decl.Value, true
		}
	}
	for i := len(normal) - 1; i >= 0; i-- {
		if normal[i].decl.Property == prop {
			return normal[i].decl.Value, true
		}
	}
	return "", false
}

func resolveCustomFont(cs *ComputedStyle, fonts *CustomFonts) {
	if cs.CustomFamily == "" {
		cs.CustomFontIdx = 0
		return
	}
	cs.CustomFontIdx = fonts.index(cs.CustomFamily, cs.FontWeight >= WeightBold, cs.FontStyle != FontStyleNormal, cs.FontWeight > 0 && cs.FontWeight < WeightNormal)
}

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
	dir := strings.ToLower(strings.TrimSpace(n.GetAttribute("dir")))
	if dir == "auto" || n.Data == "bdi" && dir == "" {
		dir = n.Directionality()
	}
	if dir == "ltr" || dir == "rtl" {
		out = append(out, css.Declaration{Property: "direction", Value: dir, Parsed: css.ParseValue(dir)})
	}
	addLen("width", n.GetAttribute("width"))
	addLen("height", n.GetAttribute("height"))
	addLen("border-spacing", n.GetAttribute("cellspacing"))
	switch n.Data {
	case "div", "p", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "tbody", "thead", "tfoot":
		switch v := strings.ToLower(strings.TrimSpace(n.GetAttribute("align"))); v {
		case "left", "right", "center", "justify":
			out = append(out, css.Declaration{Property: "text-align", Value: v, Parsed: css.ParseValue(v)})
		}
	case "td", "th":
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

func resolveCustom(parent *ComputedStyle, normal, important []cascadeEntry, registered map[string]dom.PropertyDefinition) map[string]string {
	custom := 0
	for _, es := range [][]cascadeEntry{normal, important} {
		for _, e := range es {
			if strings.HasPrefix(e.decl.Property, "--") {
				custom++
			}
		}
	}
	if custom == 0 && len(registered) == 0 {
		if parent != nil {
			return parent.Custom
		}
		return nil
	}
	n := custom + len(registered)
	if parent != nil {
		n += len(parent.Custom)
	}
	m := make(map[string]string, n)
	if parent != nil {
		for k, v := range parent.Custom {
			if reg, ok := registered[k]; ok && !reg.Inherits {
				continue
			}
			m[k] = v
		}
	}
	for _, es := range [][]cascadeEntry{normal, important} {
		for _, e := range es {
			if strings.HasPrefix(e.decl.Property, "--") {
				val := e.decl.Value
				if reg, ok := registered[e.decl.Property]; ok && reg.Syntax != "*" {
					if !css.MatchesPropertySyntax(reg.Syntax, val) {
						if reg.InitialValue != "" {
							m[e.decl.Property] = reg.InitialValue
						} else {
							delete(m, e.decl.Property)
						}
						continue
					}
				}
				m[e.decl.Property] = val
			}
		}
	}
	for k, reg := range registered {
		if _, ok := m[k]; !ok && reg.InitialValue != "" {
			if !reg.Inherits || parent == nil {
				m[k] = reg.InitialValue
			}
		}
	}
	return m
}

func resolveColorScheme(parent *ComputedStyle, normal, important []cascadeEntry) string {
	scheme := ""
	if parent != nil {
		scheme = parent.ColorScheme
	}
	for _, es := range [][]cascadeEntry{normal, important} {
		for _, e := range es {
			if e.decl.Property == "color-scheme" {
				val := strings.ToLower(strings.TrimSpace(e.decl.Value))
				if val != "" {
					scheme = val
				}
			}
		}
	}
	return scheme
}

func applyCascadeEntry(cs *ComputedStyle, e cascadeEntry, parentFontSize float32, vars *varExpander, fonts *CustomFonts) {
	if strings.HasPrefix(e.decl.Property, "--") {
		return
	}
	value, parsed := e.decl.Value, e.decl.Parsed
	if vars != nil && strings.Contains(value, "var(") {
		vars.failed = false
		value = vars.expand(value)
		if vars.failed || strings.TrimSpace(value) == "" {
			return
		}
		parsed = css.ParseValue(value)
	}
	applyProperty(cs, e.decl.Property, value, parsed, parentFontSize, fonts)
}

const maxVarSubstitutions = 4096

type varExpander struct {
	custom     map[string]string
	registered map[string]dom.PropertyDefinition
	done       map[string]string
	visiting   map[string]bool
	budget     int
	failed     bool
}

func expanderFor(custom map[string]string, normal, important []cascadeEntry, registered map[string]dom.PropertyDefinition) *varExpander {
	for _, es := range [][]cascadeEntry{normal, important} {
		for _, e := range es {
			if strings.Contains(e.decl.Value, "var(") {
				return newVarExpander(custom, registered)
			}
		}
	}
	return nil
}

func newVarExpander(custom map[string]string, registered map[string]dom.PropertyDefinition) *varExpander {
	return &varExpander{custom: custom, registered: registered, budget: maxVarSubstitutions}
}

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
			e.failed = true
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
		} else {
			e.failed = true
		}
		value = value[:i] + repl + value[j+1:]
	}
	return value
}

const varInvalid = "\x00cycle"

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
		if e.registered != nil {
			if reg, rok := e.registered[name]; rok && reg.InitialValue != "" {
				return reg.InitialValue, true
			}
		}
		return "", false
	}
	if e.visiting[name] {
		e.done[name] = varInvalid
		return "", false
	}
	e.visiting[name] = true
	prevFailed := e.failed
	e.failed = false
	v := e.expand(raw)
	hadFailure := e.failed
	e.failed = prevFailed
	delete(e.visiting, name)
	if existing, bad := e.done[name]; bad && existing == varInvalid {
		return "", false
	}
	if hadFailure {
		e.done[name] = varInvalid
		return "", false
	}
	e.done[name] = v
	return v, true
}

func clampFontSize(f float32) float32 {
	if !(f > 0) {
		return 16
	}
	return f
}

func clampOpacity(f float64) float32 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return float32(f)
}

func inheritFromParent(cs *ComputedStyle, parent *ComputedStyle) {
	cs.Color = parent.Color
	cs.FontSize = parent.FontSize
	cs.rootFontSize = parent.rootFontSize
	cs.FontFamily = parent.FontFamily
	cs.CustomFamily = parent.CustomFamily
	cs.FontWeight = parent.FontWeight
	cs.FontStyle = parent.FontStyle
	cs.LineHeight = parent.LineHeight
	cs.LineHeightRatio = parent.LineHeightRatio
	cs.TextAlign = parent.TextAlign
	if parent.Direction != "" {
		cs.Direction = parent.Direction
	} else {
		cs.Direction = "ltr"
	}
	cs.Visibility = parent.Visibility
	cs.WhiteSpace = parent.WhiteSpace
	cs.WordSpacing = parent.WordSpacing
	cs.LetterSpacing = parent.LetterSpacing
	cs.TextIndent = parent.TextIndent
	cs.TextTransform = parent.TextTransform
	cs.TextDecoration = parent.TextDecoration
	cs.TextShadow = parent.TextShadow
	cs.VerticalAlign = parent.VerticalAlign
	cs.ListStyleType = parent.ListStyleType
	cs.ColorScheme = parent.ColorScheme
	if len(parent.Quotes) > 0 {
		cs.Quotes = parent.Quotes
	}
	cs.AccentColor = parent.AccentColor
	cs.TextUnderlineOffset = parent.TextUnderlineOffset
	cs.TextDecorationThickness = parent.TextDecorationThickness
	if parent.PointerEvents != "" {
		cs.PointerEvents = parent.PointerEvents
	}
	if parent.UserSelect == "none" || parent.UserSelect == "all" || parent.UserSelect == "contain" {
		cs.UserSelect = parent.UserSelect
	}
	if parent.CaretColor.A > 0 {
		cs.CaretColor = parent.CaretColor
	}
	if parent.WordBreak != "" {
		cs.WordBreak = parent.WordBreak
	}
	if parent.Hyphens != "" {
		cs.Hyphens = parent.Hyphens
	}
	if parent.TabSize > 0 {
		cs.TabSize = parent.TabSize
	}
	if parent.WritingMode != "" {
		cs.WritingMode = parent.WritingMode
	}
	if parent.TextOrientation != "" {
		cs.TextOrientation = parent.TextOrientation
	}
	if parent.FontVariantNumeric != "" {
		cs.FontVariantNumeric = parent.FontVariantNumeric
	}
}

func parseInlineDeclaration(s string) css.Declaration {
	colonIdx := strings.Index(s, ":")
	if colonIdx < 0 {
		return css.Declaration{}
	}
	prop := strings.TrimSpace(s[:colonIdx])
	val := strings.TrimSpace(s[colonIdx+1:])
	important := false
	if strings.Contains(val, "!") {
		if idx := strings.Index(strings.ToLower(val), "!important"); idx >= 0 {
			important = true
			val = strings.TrimSpace(val[:idx])
		}
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
	for _, part := range css.SplitDeclarations(s) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		decls = append(decls, parseInlineDeclaration(css.StripComments(part)))
	}
	return decls
}

type specificity struct {
	a, b, c int
	inline  bool
}

func mergeBorderImage(dst *css.BorderImage, src css.BorderImage) {
	if src.HasSlice {
		dst.Slice, dst.SlicePct, dst.HasSlice, dst.Fill = src.Slice, src.SlicePct, src.HasSlice, src.Fill
	}
	if src.HasWidth {
		dst.Width, dst.WidthU, dst.HasWidth = src.Width, src.WidthU, src.HasWidth
	}
	if src.HasOutset {
		dst.Outset, dst.OutsetN, dst.HasOutset = src.Outset, src.OutsetN, src.HasOutset
	}
	if src.HasRepeat {
		dst.RepeatX, dst.RepeatY, dst.HasRepeat = src.RepeatX, src.RepeatY, src.HasRepeat
	}
}

func applyBorderImageShorthand(cs *ComputedStyle, value string) {
	source, slice, width, outset, repeat, ok := css.ParseBorderImageShorthand(value)
	if !ok {
		return
	}
	var bi css.BorderImage
	bi.Source = css.ParseBorderImageSource(source)
	if slice != "" {
		s, ok := css.ParseBorderImageSlice(slice)
		if !ok {
			return
		}
		mergeBorderImage(&bi, s)
	}
	if width != "" {
		w, ok := css.ParseBorderImageWidth(width)
		if !ok {
			return
		}
		mergeBorderImage(&bi, w)
	}
	if outset != "" {
		o, ok := css.ParseBorderImageOutset(outset)
		if !ok {
			return
		}
		mergeBorderImage(&bi, o)
	}
	if repeat != "" {
		r, ok := css.ParseBorderImageRepeat(repeat)
		if !ok {
			return
		}
		mergeBorderImage(&bi, r)
	}
	cs.BorderImage = bi
}

func applyProperty(cs *ComputedStyle, prop, value string, parsed css.Value, parentFontSize float32, fonts *CustomFonts) {
	switch prop {
	case "display":
		applyDisplay(cs, value)
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
	case "content-visibility":
		val := strings.ToLower(strings.TrimSpace(value))
		if val == "hidden" || val == "auto" || val == "visible" {
			cs.ContentVisibility = val
		}
	case "contain-intrinsic-size":
		fields := strings.Fields(strings.ToLower(value))
		var lens []float32
		for _, f := range fields {
			if f == "auto" || f == "none" {
				continue
			}
			v := css.ParseValue(f)
			if lengthSpecified(v) {
				lens = append(lens, resolveLengthEm(v, 0, cs.FontSize, cs.rootFontSize))
			}
		}
		if len(lens) == 1 {
			cs.ContainIntrinsicWidth = lens[0]
			cs.ContainIntrinsicHeight = lens[0]
		} else if len(lens) >= 2 {
			cs.ContainIntrinsicWidth = lens[0]
			cs.ContainIntrinsicHeight = lens[1]
		}
	case "contain-intrinsic-width":
		fields := strings.Fields(strings.ToLower(value))
		for _, f := range fields {
			if f == "auto" || f == "none" {
				continue
			}
			v := css.ParseValue(f)
			if lengthSpecified(v) {
				cs.ContainIntrinsicWidth = resolveLengthEm(v, 0, cs.FontSize, cs.rootFontSize)
				break
			}
		}
	case "contain-intrinsic-height":
		fields := strings.Fields(strings.ToLower(value))
		for _, f := range fields {
			if f == "auto" || f == "none" {
				continue
			}
			v := css.ParseValue(f)
			if lengthSpecified(v) {
				cs.ContainIntrinsicHeight = resolveLengthEm(v, 0, cs.FontSize, cs.rootFontSize)
				break
			}
		}
	case "anchor-name":
		cs.AnchorName = strings.TrimSpace(value)
	case "position-anchor":
		cs.PositionAnchor = strings.TrimSpace(value)
	case "position-area":
		cs.PositionArea = strings.ToLower(strings.TrimSpace(value))
	case "text-wrap", "text-wrap-mode", "text-wrap-style":
		val := strings.ToLower(strings.TrimSpace(value))
		if val == "wrap" || val == "nowrap" || val == "balance" || val == "pretty" {
			cs.TextWrap = val
		}
	case "line-clamp", "-webkit-line-clamp":
		val := strings.TrimSpace(value)
		if strings.EqualFold(val, "none") {
			cs.LineClamp = 0
		} else if n, err := strconv.Atoi(val); err == nil && n >= 0 {
			cs.LineClamp = n
		}
	case "view-transition-name":
		val := strings.TrimSpace(value)
		if strings.EqualFold(val, "none") {
			cs.ViewTransitionName = "none"
		} else {
			cs.ViewTransitionName = val
		}
	case "scroll-timeline-name":
		cs.ScrollTimelineName = strings.TrimSpace(value)
	case "scroll-timeline-axis":
		val := strings.ToLower(strings.TrimSpace(value))
		if val == "block" || val == "inline" || val == "x" || val == "y" {
			cs.ScrollTimelineAxis = val
		}
	case "scroll-timeline":
		fields := strings.Fields(strings.TrimSpace(value))
		for _, f := range fields {
			lower := strings.ToLower(f)
			if lower == "block" || lower == "inline" || lower == "x" || lower == "y" {
				cs.ScrollTimelineAxis = lower
			} else {
				cs.ScrollTimelineName = f
			}
		}
	case "view-timeline-name":
		cs.ViewTimelineName = strings.TrimSpace(value)
	case "view-timeline-axis":
		val := strings.ToLower(strings.TrimSpace(value))
		if val == "block" || val == "inline" || val == "x" || val == "y" {
			cs.ViewTimelineAxis = val
		}
	case "view-timeline-inset":
		cs.ViewTimelineInset = strings.TrimSpace(value)
	case "view-timeline":
		fields := strings.Fields(strings.TrimSpace(value))
		for _, f := range fields {
			lower := strings.ToLower(f)
			if lower == "block" || lower == "inline" || lower == "x" || lower == "y" {
				cs.ViewTimelineAxis = lower
			} else {
				cs.ViewTimelineName = f
			}
		}
	case "animation-timeline":
		cs.AnimationTimeline = strings.TrimSpace(value)
	case "scroll-snap-type":
		fields := strings.Fields(strings.ToLower(value))
		if len(fields) == 1 {
			if fields[0] == "none" {
				cs.ScrollSnapType = "none"
			} else if fields[0] == "mandatory" || fields[0] == "proximity" {
				cs.ScrollSnapStrictness = fields[0]
			} else {
				cs.ScrollSnapType = fields[0]
			}
		} else if len(fields) >= 2 {
			for _, f := range fields {
				switch f {
				case "none", "x", "y", "block", "inline", "both":
					cs.ScrollSnapType = f
				case "mandatory", "proximity":
					cs.ScrollSnapStrictness = f
				}
			}
		}
	case "scroll-snap-align":
		cs.ScrollSnapAlign = strings.TrimSpace(strings.ToLower(value))
	case "scroll-snap-stop":
		cs.ScrollSnapStop = strings.TrimSpace(strings.ToLower(value))
	case "scroll-padding":
		t, r, b, l := parsePaddingShorthand(value, cs.FontSize, cs.rootFontSize)
		cs.ScrollPaddingTop = t
		cs.ScrollPaddingRight = r
		cs.ScrollPaddingBottom = b
		cs.ScrollPaddingLeft = l
	case "scroll-padding-top":
		cs.ScrollPaddingTop = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "scroll-padding-right":
		cs.ScrollPaddingRight = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "scroll-padding-bottom":
		cs.ScrollPaddingBottom = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "scroll-padding-left":
		cs.ScrollPaddingLeft = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "scroll-margin":
		t, r, b, l, _, _ := parseMarginShorthand(value, cs.FontSize, cs.rootFontSize)
		cs.ScrollMarginTop = t
		cs.ScrollMarginRight = r
		cs.ScrollMarginBottom = b
		cs.ScrollMarginLeft = l
	case "scroll-margin-top":
		cs.ScrollMarginTop = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "scroll-margin-right":
		cs.ScrollMarginRight = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "scroll-margin-bottom":
		cs.ScrollMarginBottom = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "scroll-margin-left":
		cs.ScrollMarginLeft = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "font-palette":
		cs.FontPalette = strings.TrimSpace(value)
	case "offset-path":
		cs.OffsetPath = strings.TrimSpace(value)
	case "offset-distance":
		cs.OffsetDistance = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "offset-rotate":
		cs.OffsetRotate = strings.TrimSpace(value)
	case "offset":
		val := strings.TrimSpace(value)
		if strings.EqualFold(val, "none") {
			cs.OffsetPath = "none"
			cs.OffsetDistance = 0
			cs.OffsetRotate = "auto"
		} else {
			pathStr := ""
			rem := val
			if idx := strings.Index(val, "path("); idx != -1 {
				closeIdx := strings.Index(val[idx:], ")")
				if closeIdx != -1 {
					pathStr = val[idx : idx+closeIdx+1]
					rem = strings.TrimSpace(val[:idx] + " " + val[idx+closeIdx+1:])
				}
			} else if idx := strings.Index(val, "ray("); idx != -1 {
				closeIdx := strings.Index(val[idx:], ")")
				if closeIdx != -1 {
					pathStr = val[idx : idx+closeIdx+1]
					rem = strings.TrimSpace(val[:idx] + " " + val[idx+closeIdx+1:])
				}
			}
			if pathStr != "" {
				cs.OffsetPath = pathStr
			}
			fields := strings.Fields(rem)
			for _, f := range fields {
				p := css.ParseValue(f)
				if p.Type == css.ValueLength || (p.Type == css.ValueNumber && p.Num == 0) {
					cs.OffsetDistance = resolveLengthEm(p, 0, cs.FontSize, cs.rootFontSize)
				} else if strings.EqualFold(f, "auto") || strings.EqualFold(f, "reverse") || strings.HasSuffix(f, "deg") || strings.HasSuffix(f, "rad") || strings.HasSuffix(f, "turn") {
					cs.OffsetRotate = f
				} else if cs.OffsetPath == "" {
					cs.OffsetPath = f
				}
			}
		}
	case "shape-outside":
		cs.ShapeOutside = strings.TrimSpace(value)
	case "shape-margin":
		cs.ShapeMargin = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "clip":
		if parsed.Type == css.ValueFunc && strings.EqualFold(parsed.Func, "rect") && len(parsed.Parts) == 4 {
			cs.HasClip = true
			cs.ClipTop = resolveLengthEm(parsed.Parts[0], 0, cs.FontSize)
			cs.ClipRight = resolveLengthEm(parsed.Parts[1], 0, cs.FontSize)
			cs.ClipBottom = resolveLengthEm(parsed.Parts[2], 0, cs.FontSize)
			cs.ClipLeft = resolveLengthEm(parsed.Parts[3], 0, cs.FontSize)
		} else if value == "auto" {
			cs.HasClip = false
		}
	case "content":
		if parsed.Type == css.ValueString {
			cs.Content = parsed.Str
		} else if parts, ok := ParseContentParts(value); ok {
			cs.Content = ""
			cs.ContentParts = parts
		} else {
			cs.Content = value
		}
	case "counter-reset":
		cs.CounterReset = ParseCounterDecls(value, 0)
	case "counter-set":
		cs.CounterSet = ParseCounterDecls(value, 0)
	case "counter-increment":
		cs.CounterIncrement = ParseCounterDecls(value, 1)
	case "opacity":
		switch parsed.Type {
		case css.ValueNumber:
			cs.Opacity = clampOpacity(parsed.Num)
		case css.ValuePercentage:
			cs.Opacity = clampOpacity(parsed.Num / 100)
		}
	case "z-index":
		if value == "auto" {
			cs.HasZIndex = false
		} else {
			cs.HasZIndex = true
			cs.ZIndex = int32(parsed.Num)
		}

	case "container-type":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "normal", "size", "inline-size":
			cs.ContainerType = v
		default:
			cs.ContainerType = "normal"
		}
	case "container-name":
		cs.ContainerName = strings.TrimSpace(value)
	case "container":
		parts := strings.Split(value, "/")
		if len(parts) == 2 {
			name := strings.TrimSpace(parts[0])
			typ := strings.ToLower(strings.TrimSpace(parts[1]))
			if name == "none" {
				cs.ContainerName = ""
			} else {
				cs.ContainerName = name
			}
			switch typ {
			case "normal", "size", "inline-size":
				cs.ContainerType = typ
			default:
				cs.ContainerType = "normal"
			}
		} else {
			val := strings.TrimSpace(value)
			valLower := strings.ToLower(val)
			if valLower == "normal" || valLower == "size" || valLower == "inline-size" {
				cs.ContainerType = valLower
				cs.ContainerName = ""
			} else if valLower == "none" {
				cs.ContainerName = ""
				cs.ContainerType = "normal"
			} else {
				cs.ContainerName = val
				cs.ContainerType = "normal"
			}
		}

	case "width", "inline-size":
		if isAnchorSizeFunc(value) {
			cs.WidthAnchor = extractAnchorArgs(value)
		} else if d, ok := deferPercentFunc(parsed, cs.FontSize, cs.rootFontSize); ok {
			cs.WidthCalc, cs.Width = d, -1
		} else {
			cs.WidthCalc = DeferredLength{}
			cs.Width = resolveLengthEm(parsed, -1, cs.FontSize, cs.rootFontSize)
		}
	case "height", "block-size":
		if isAnchorSizeFunc(value) {
			cs.HeightAnchor = extractAnchorArgs(value)
		} else if d, ok := deferPercentFunc(parsed, cs.FontSize, cs.rootFontSize); ok {
			cs.HeightCalc, cs.Height = d, -1
		} else {
			cs.HeightCalc = DeferredLength{}
			cs.Height = resolveLengthEm(parsed, -1, cs.FontSize, cs.rootFontSize)
		}
	case "min-width", "min-inline-size":
		if d, ok := deferPercentFunc(parsed, cs.FontSize, cs.rootFontSize); ok {
			cs.MinWidthCalc, cs.MinWidth = d, 0
			cs.MinWidthAuto = false
		} else {
			cs.MinWidthCalc = DeferredLength{}
			cs.MinWidthAuto = valueIsAuto(parsed)
			cs.MinWidth = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		}
	case "min-height", "min-block-size":
		if d, ok := deferPercentFunc(parsed, cs.FontSize, cs.rootFontSize); ok {
			cs.MinHeightCalc, cs.MinHeight = d, 0
		} else {
			cs.MinHeightCalc = DeferredLength{}
			cs.MinHeight = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		}
	case "max-width", "max-inline-size":
		if d, ok := deferPercentFunc(parsed, cs.FontSize, cs.rootFontSize); ok {
			cs.MaxWidthCalc, cs.MaxWidth = d, -1
		} else {
			cs.MaxWidthCalc = DeferredLength{}
			cs.MaxWidth = resolveMaxLength(parsed, cs.FontSize, cs.rootFontSize)
		}
	case "max-height", "max-block-size":
		if d, ok := deferPercentFunc(parsed, cs.FontSize, cs.rootFontSize); ok {
			cs.MaxHeightCalc, cs.MaxHeight = d, -1
		} else {
			cs.MaxHeightCalc = DeferredLength{}
			cs.MaxHeight = resolveMaxLength(parsed, cs.FontSize, cs.rootFontSize)
		}

	case "margin":
		t, r, b, l, lAuto, rAuto := parseMarginShorthand(value, cs.FontSize, cs.rootFontSize)
		cs.MarginTop = t
		cs.MarginRight = r
		cs.MarginBottom = b
		cs.MarginLeft = l
		cs.MarginLeftAuto = lAuto
		cs.MarginRightAuto = rAuto
	case "margin-top":
		cs.MarginTop = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "margin-right":
		cs.MarginRightAuto = valueIsAuto(parsed)
		cs.MarginRight = resolveLengthEm(parsed, MarginAuto, cs.FontSize, cs.rootFontSize)
	case "margin-bottom":
		cs.MarginBottom = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "margin-left":
		cs.MarginLeftAuto = valueIsAuto(parsed)
		cs.MarginLeft = resolveLengthEm(parsed, MarginAuto, cs.FontSize, cs.rootFontSize)

	case "padding":
		t, r, b, l := parsePaddingShorthand(value, cs.FontSize, cs.rootFontSize)
		cs.PaddingTop = t
		cs.PaddingRight = r
		cs.PaddingBottom = b
		cs.PaddingLeft = l
	case "padding-top":
		cs.PaddingTop = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "padding-right":
		cs.PaddingRight = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "padding-bottom":
		cs.PaddingBottom = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "padding-left":
		cs.PaddingLeft = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)

	case "margin-block":
		a, b := logicalPair(value, 0, cs.FontSize, cs.rootFontSize)
		cs.MarginTop, cs.MarginBottom = a, b
	case "margin-inline":
		a, b := logicalPair(value, MarginAuto, cs.FontSize, cs.rootFontSize)
		cs.MarginLeft, cs.MarginRight = a, b
		parts := splitTopLevelSpace(value)
		if len(parts) == 1 {
			cs.MarginLeftAuto, cs.MarginRightAuto = strIsAuto(parts[0]), strIsAuto(parts[0])
		} else if len(parts) >= 2 {
			cs.MarginLeftAuto, cs.MarginRightAuto = strIsAuto(parts[0]), strIsAuto(parts[1])
		}
	case "margin-block-start":
		cs.MarginTop = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "margin-block-end":
		cs.MarginBottom = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "margin-inline-start":
		if cs.Direction == "rtl" {
			cs.MarginRightAuto = valueIsAuto(parsed)
			cs.MarginRight = resolveLengthEm(parsed, MarginAuto, cs.FontSize, cs.rootFontSize)
		} else {
			cs.MarginLeftAuto = valueIsAuto(parsed)
			cs.MarginLeft = resolveLengthEm(parsed, MarginAuto, cs.FontSize, cs.rootFontSize)
		}
	case "margin-inline-end":
		if cs.Direction == "rtl" {
			cs.MarginLeftAuto = valueIsAuto(parsed)
			cs.MarginLeft = resolveLengthEm(parsed, MarginAuto, cs.FontSize, cs.rootFontSize)
		} else {
			cs.MarginRightAuto = valueIsAuto(parsed)
			cs.MarginRight = resolveLengthEm(parsed, MarginAuto, cs.FontSize, cs.rootFontSize)
		}
	case "padding-block":
		a, b := logicalPair(value, 0, cs.FontSize, cs.rootFontSize)
		cs.PaddingTop, cs.PaddingBottom = a, b
	case "padding-inline":
		a, b := logicalPair(value, 0, cs.FontSize, cs.rootFontSize)
		cs.PaddingLeft, cs.PaddingRight = a, b
	case "padding-block-start":
		cs.PaddingTop = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "padding-block-end":
		cs.PaddingBottom = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "padding-inline-start":
		if cs.Direction == "rtl" {
			cs.PaddingRight = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		} else {
			cs.PaddingLeft = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		}
	case "padding-inline-end":
		if cs.Direction == "rtl" {
			cs.PaddingLeft = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		} else {
			cs.PaddingRight = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		}

	case "border-inline":
		w, s, c := parseBorderShorthand(value)
		cs.BorderLeftWidth, cs.BorderRightWidth = w, w
		cs.BorderLeftStyle, cs.BorderRightStyle = s, s
		cs.BorderLeftColor, cs.BorderRightColor = c, c
	case "border-block":
		w, s, c := parseBorderShorthand(value)
		cs.BorderTopWidth, cs.BorderBottomWidth = w, w
		cs.BorderTopStyle, cs.BorderBottomStyle = s, s
		cs.BorderTopColor, cs.BorderBottomColor = c, c
	case "border-inline-start":
		w, s, c := parseBorderShorthand(value)
		if cs.Direction == "rtl" {
			cs.BorderRightWidth, cs.BorderRightStyle, cs.BorderRightColor = w, s, c
		} else {
			cs.BorderLeftWidth, cs.BorderLeftStyle, cs.BorderLeftColor = w, s, c
		}
	case "border-inline-end":
		w, s, c := parseBorderShorthand(value)
		if cs.Direction == "rtl" {
			cs.BorderLeftWidth, cs.BorderLeftStyle, cs.BorderLeftColor = w, s, c
		} else {
			cs.BorderRightWidth, cs.BorderRightStyle, cs.BorderRightColor = w, s, c
		}
	case "border-block-start":
		w, s, c := parseBorderShorthand(value)
		cs.BorderTopWidth, cs.BorderTopStyle, cs.BorderTopColor = w, s, c
	case "border-block-end":
		w, s, c := parseBorderShorthand(value)
		cs.BorderBottomWidth, cs.BorderBottomStyle, cs.BorderBottomColor = w, s, c
	case "border-inline-width":
		w := resolveLength(parsed, 0)
		cs.BorderLeftWidth, cs.BorderRightWidth = w, w
	case "border-block-width":
		w := resolveLength(parsed, 0)
		cs.BorderTopWidth, cs.BorderBottomWidth = w, w
	case "border-inline-start-width":
		w := resolveLength(parsed, 0)
		if cs.Direction == "rtl" {
			cs.BorderRightWidth = w
		} else {
			cs.BorderLeftWidth = w
		}
	case "border-inline-end-width":
		w := resolveLength(parsed, 0)
		if cs.Direction == "rtl" {
			cs.BorderLeftWidth = w
		} else {
			cs.BorderRightWidth = w
		}
	case "border-block-start-width":
		cs.BorderTopWidth = resolveLength(parsed, 0)
	case "border-block-end-width":
		cs.BorderBottomWidth = resolveLength(parsed, 0)
	case "border-inline-style":
		cs.BorderLeftStyle, cs.BorderRightStyle = value, value
	case "border-block-style":
		cs.BorderTopStyle, cs.BorderBottomStyle = value, value
	case "border-inline-start-style":
		if cs.Direction == "rtl" {
			cs.BorderRightStyle = value
		} else {
			cs.BorderLeftStyle = value
		}
	case "border-inline-end-style":
		if cs.Direction == "rtl" {
			cs.BorderLeftStyle = value
		} else {
			cs.BorderRightStyle = value
		}
	case "border-block-start-style":
		cs.BorderTopStyle = value
	case "border-block-end-style":
		cs.BorderBottomStyle = value
	case "border-inline-color":
		clr := parseColorValueWithScheme(value, cs.scheme())
		cs.BorderLeftColor, cs.BorderRightColor = clr, clr
	case "border-block-color":
		clr := parseColorValueWithScheme(value, cs.scheme())
		cs.BorderTopColor, cs.BorderBottomColor = clr, clr
	case "border-inline-start-color":
		clr := parseColorValueWithScheme(value, cs.scheme())
		if cs.Direction == "rtl" {
			cs.BorderRightColor = clr
		} else {
			cs.BorderLeftColor = clr
		}
	case "border-inline-end-color":
		clr := parseColorValueWithScheme(value, cs.scheme())
		if cs.Direction == "rtl" {
			cs.BorderLeftColor = clr
		} else {
			cs.BorderRightColor = clr
		}
	case "border-block-start-color":
		cs.BorderTopColor = parseColorValueWithScheme(value, cs.scheme())
	case "border-block-end-color":
		cs.BorderBottomColor = parseColorValueWithScheme(value, cs.scheme())

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
	case "outline":
		w, s, c := parseBorderShorthand(value)
		cs.OutlineWidth = w
		cs.OutlineStyle = s
		cs.OutlineColor = c
	case "outline-width":
		cs.OutlineWidth = resolveLength(parsed, 0)
	case "outline-style":
		cs.OutlineStyle = strings.ToLower(value)
	case "outline-color":
		cs.OutlineColor = parseColorValueWithScheme(value, cs.scheme())
	case "outline-offset":
		cs.OutlineOffset = resolveLength(parsed, 0)
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
		colors := parseBorderColorsWithScheme(value, cs.scheme())
		cs.BorderTopColor = colors[0]
		cs.BorderRightColor = colors[1]
		cs.BorderBottomColor = colors[2]
		cs.BorderLeftColor = colors[3]
	case "border-top-color":
		cs.BorderTopColor = parseColorValueWithScheme(value, cs.scheme())
	case "border-right-color":
		cs.BorderRightColor = parseColorValueWithScheme(value, cs.scheme())
	case "border-bottom-color":
		cs.BorderBottomColor = parseColorValueWithScheme(value, cs.scheme())
	case "border-left-color":
		cs.BorderLeftColor = parseColorValueWithScheme(value, cs.scheme())
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
	case "border-start-start-radius":
		if cs.Direction == "rtl" {
			parseCornerRadius(cs, &cs.BorderRadius[1], value)
		} else {
			parseCornerRadius(cs, &cs.BorderRadius[0], value)
		}
	case "border-start-end-radius":
		if cs.Direction == "rtl" {
			parseCornerRadius(cs, &cs.BorderRadius[0], value)
		} else {
			parseCornerRadius(cs, &cs.BorderRadius[1], value)
		}
	case "border-end-start-radius":
		if cs.Direction == "rtl" {
			parseCornerRadius(cs, &cs.BorderRadius[2], value)
		} else {
			parseCornerRadius(cs, &cs.BorderRadius[3], value)
		}
	case "border-end-end-radius":
		if cs.Direction == "rtl" {
			parseCornerRadius(cs, &cs.BorderRadius[3], value)
		} else {
			parseCornerRadius(cs, &cs.BorderRadius[2], value)
		}
	case "border-image-source":
		cs.BorderImage.Source = css.ParseBorderImageSource(value)
	case "border-image-slice":
		if bi, ok := css.ParseBorderImageSlice(value); ok {
			mergeBorderImage(&cs.BorderImage, bi)
		}
	case "border-image-width":
		if bi, ok := css.ParseBorderImageWidth(value); ok {
			mergeBorderImage(&cs.BorderImage, bi)
		}
	case "border-image-outset":
		if bi, ok := css.ParseBorderImageOutset(value); ok {
			mergeBorderImage(&cs.BorderImage, bi)
		}
	case "border-image-repeat":
		if bi, ok := css.ParseBorderImageRepeat(value); ok {
			mergeBorderImage(&cs.BorderImage, bi)
		}
	case "border-image":
		applyBorderImageShorthand(cs, value)
	case "top":
		if isAnchorFunc(value) {
			cs.TopAnchor = extractAnchorArgs(value)
			cs.HasTop = true
		} else {
			cs.Top = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
			cs.HasTop = lengthSpecified(parsed)
		}
	case "right":
		if isAnchorFunc(value) {
			cs.RightAnchor = extractAnchorArgs(value)
			cs.HasRight = true
		} else {
			cs.Right = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
			cs.HasRight = lengthSpecified(parsed)
		}
	case "bottom":
		if isAnchorFunc(value) {
			cs.BottomAnchor = extractAnchorArgs(value)
			cs.HasBottom = true
		} else {
			cs.Bottom = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
			cs.HasBottom = lengthSpecified(parsed)
		}
	case "left":
		if isAnchorFunc(value) {
			cs.LeftAnchor = extractAnchorArgs(value)
			cs.HasLeft = true
		} else {
			cs.Left = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
			cs.HasLeft = lengthSpecified(parsed)
		}
	case "inset":
		parts := splitTopLevelSpace(value)
		switch len(parts) {
		case 1:
			v := css.ParseValue(parts[0])
			val := resolveLengthEm(v, 0, cs.FontSize, cs.rootFontSize)
			has := lengthSpecified(v)
			cs.Top, cs.Right, cs.Bottom, cs.Left = val, val, val, val
			cs.HasTop, cs.HasRight, cs.HasBottom, cs.HasLeft = has, has, has, has
		case 2:
			v0 := css.ParseValue(parts[0])
			v1 := css.ParseValue(parts[1])
			tb := resolveLengthEm(v0, 0, cs.FontSize, cs.rootFontSize)
			rl := resolveLengthEm(v1, 0, cs.FontSize, cs.rootFontSize)
			hasTB := lengthSpecified(v0)
			hasRL := lengthSpecified(v1)
			cs.Top, cs.Bottom = tb, tb
			cs.Right, cs.Left = rl, rl
			cs.HasTop, cs.HasBottom = hasTB, hasTB
			cs.HasRight, cs.HasLeft = hasRL, hasRL
		case 3:
			v0 := css.ParseValue(parts[0])
			v1 := css.ParseValue(parts[1])
			v2 := css.ParseValue(parts[2])
			t := resolveLengthEm(v0, 0, cs.FontSize, cs.rootFontSize)
			rl := resolveLengthEm(v1, 0, cs.FontSize, cs.rootFontSize)
			b := resolveLengthEm(v2, 0, cs.FontSize, cs.rootFontSize)
			cs.Top = t
			cs.Right, cs.Left = rl, rl
			cs.Bottom = b
			cs.HasTop = lengthSpecified(v0)
			cs.HasRight, cs.HasLeft = lengthSpecified(v1), lengthSpecified(v1)
			cs.HasBottom = lengthSpecified(v2)
		case 4:
			v0 := css.ParseValue(parts[0])
			v1 := css.ParseValue(parts[1])
			v2 := css.ParseValue(parts[2])
			v3 := css.ParseValue(parts[3])
			cs.Top = resolveLengthEm(v0, 0, cs.FontSize, cs.rootFontSize)
			cs.Right = resolveLengthEm(v1, 0, cs.FontSize, cs.rootFontSize)
			cs.Bottom = resolveLengthEm(v2, 0, cs.FontSize, cs.rootFontSize)
			cs.Left = resolveLengthEm(v3, 0, cs.FontSize, cs.rootFontSize)
			cs.HasTop = lengthSpecified(v0)
			cs.HasRight = lengthSpecified(v1)
			cs.HasBottom = lengthSpecified(v2)
			cs.HasLeft = lengthSpecified(v3)
		}
	case "inset-block":
		a, b := logicalPair(value, 0, cs.FontSize, cs.rootFontSize)
		cs.Top, cs.Bottom = a, b
		cs.HasTop, cs.HasBottom = true, true
	case "inset-inline":
		a, b := logicalPair(value, 0, cs.FontSize, cs.rootFontSize)
		if cs.Direction == "rtl" {
			cs.Right, cs.Left = a, b
		} else {
			cs.Left, cs.Right = a, b
		}
		cs.HasLeft, cs.HasRight = true, true
	case "inset-block-start":
		cs.Top = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		cs.HasTop = lengthSpecified(parsed)
	case "inset-block-end":
		cs.Bottom = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		cs.HasBottom = lengthSpecified(parsed)
	case "inset-inline-start":
		val := resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		if cs.Direction == "rtl" {
			cs.Right = val
			cs.HasRight = lengthSpecified(parsed)
		} else {
			cs.Left = val
			cs.HasLeft = lengthSpecified(parsed)
		}
	case "inset-inline-end":
		val := resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		if cs.Direction == "rtl" {
			cs.Left = val
			cs.HasLeft = lengthSpecified(parsed)
		} else {
			cs.Right = val
			cs.HasRight = lengthSpecified(parsed)
		}

	case "color-scheme":
		cs.ColorScheme = strings.ToLower(strings.TrimSpace(value))
	case "color":
		if c, ok := css.ParseColorWithScheme(value, cs.scheme()); ok {
			cs.Color = c
		}
	case "accent-color":
		v := strings.TrimSpace(strings.ToLower(value))
		if v == "auto" {
			cs.AccentColor = css.Color{}
		} else if c, ok := css.ParseColorWithScheme(value, cs.scheme()); ok {
			cs.AccentColor = c
		}
	case "background-color":
		cs.BackgroundColor = parseColorValueWithScheme(value, cs.scheme())
	case "background":
		cs.BackgroundColor, cs.BackgroundGradient, cs.BackgroundRadialGradient = parseBackgroundWithScheme(value, cs.scheme())
		cs.BackgroundImage = extractBackgroundURL(value)
		parseBackgroundShorthandExtras(cs, value)
	case "background-image":
		cs.BackgroundGradient = parseGradient(value)
		cs.BackgroundRadialGradient = parseRadialGradient(value)
		cs.BackgroundImage = extractBackgroundURL(value)
	case "mask-image", "-webkit-mask-image":
		cs.MaskImage = extractBackgroundURL(value)
	case "mask-repeat", "-webkit-mask-repeat":
		cs.MaskRepeat = strings.ToLower(strings.TrimSpace(value))
	case "mask-size", "-webkit-mask-size":
		cs.MaskSize = strings.ToLower(strings.TrimSpace(value))
	case "mask-position", "-webkit-mask-position":
		cs.MaskPosition = strings.ToLower(strings.TrimSpace(value))
	case "mask", "-webkit-mask":
		v := strings.TrimSpace(value)
		if u := extractBackgroundURL(v); u != "" {
			cs.MaskImage = u
		}
	case "background-repeat":
		cs.BackgroundRepeat, cs.BackgroundRepeatY = parseBackgroundRepeat(value)
	case "background-origin":
		cs.BackgroundOrigin = parseBackgroundBox(value, false)
	case "background-clip", "-webkit-background-clip":
		cs.BackgroundClip = parseBackgroundBox(value, true)
	case "background-size":
		parseBackgroundSize(cs, value)
	case "background-position":
		parseBackgroundPosition(cs, value)

	case "font-family":
		cs.FontFamily, cs.CustomFamily = parseFontFamily(value, fonts)
	case "font-size":
		cs.FontSize = clampFontSize(resolveFontSize(parsed, parentFontSize, cs.rootFontSize))
	case "font-weight":
		if parsed.Type == css.ValueNumber {
			cs.FontWeight = FontWeight(parsed.Num)
		} else {
			cs.FontWeight = parseFontWeight(value)
		}
	case "font-style":
		cs.FontStyle = parseFontStyle(value)
	case "font":
		parseFontShorthand(cs, value, parentFontSize, cs.rootFontSize, fonts)
	case "line-height":
		cs.LineHeight = resolveLineHeight(parsed, cs.FontSize)
		cs.LineHeightRatio = lineHeightRatio(parsed)
		cs.LineHeightEm = lineHeightEm(parsed)
	case "direction":
		v := strings.ToLower(strings.TrimSpace(value))
		if v == "rtl" || v == "ltr" {
			cs.Direction = v
		}
	case "unicode-bidi":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "normal", "embed", "isolate", "bidi-override", "plaintext":
			cs.UnicodeBidi = v
		}
	case "text-align":
		cs.TextAlign = parseTextAlign(value, cs.Direction)
	case "text-indent":
		cs.TextIndent = resolveLength(parsed, 0)
	case "text-transform":
		cs.TextTransform = strings.ToLower(strings.TrimSpace(value))
	case "text-decoration":
		cs.TextDecoration = parseTextDecoration(value)
	case "text-underline-offset":
		if strings.TrimSpace(strings.ToLower(value)) == "auto" {
			cs.TextUnderlineOffset = 0
		} else {
			cs.TextUnderlineOffset = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		}
	case "text-decoration-thickness":
		vLower := strings.TrimSpace(strings.ToLower(value))
		if vLower == "auto" || vLower == "from-font" {
			cs.TextDecorationThickness = 0
		} else {
			cs.TextDecorationThickness = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		}
	case "white-space":
		cs.WhiteSpace = parseWhiteSpace(value)
	case "word-spacing":
		if strings.TrimSpace(strings.ToLower(value)) == "normal" {
			cs.WordSpacing = 0
		} else {
			cs.WordSpacing = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		}
	case "letter-spacing":
		if strings.TrimSpace(strings.ToLower(value)) == "normal" {
			cs.LetterSpacing = 0
		} else {
			cs.LetterSpacing = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
		}

	case "pointer-events":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "auto", "none", "visiblepainted", "visiblefill", "visiblestroke", "visible", "painted", "fill", "stroke", "all":
			cs.PointerEvents = v
		case "initial", "unset":
			cs.PointerEvents = "auto"
		}
	case "user-select", "-webkit-user-select":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "auto", "text", "none", "contain", "all":
			cs.UserSelect = v
		case "initial", "unset":
			cs.UserSelect = "auto"
		}

	case "caret-color":
		v := strings.TrimSpace(strings.ToLower(value))
		if v == "auto" {
			cs.CaretColor = css.Color{}
		} else if c, ok := css.ParseColorWithScheme(value, cs.scheme()); ok {
			cs.CaretColor = c
		}

	case "overscroll-behavior":
		parts := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
		if len(parts) == 1 {
			switch parts[0] {
			case "auto", "contain", "none":
				cs.OverscrollBehaviorX = parts[0]
				cs.OverscrollBehaviorY = parts[0]
			}
		} else if len(parts) >= 2 {
			switch parts[0] {
			case "auto", "contain", "none":
				cs.OverscrollBehaviorX = parts[0]
			}
			switch parts[1] {
			case "auto", "contain", "none":
				cs.OverscrollBehaviorY = parts[1]
			}
		}
	case "overscroll-behavior-x", "overscroll-behavior-inline":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "auto", "contain", "none":
			cs.OverscrollBehaviorX = v
		}
	case "overscroll-behavior-y", "overscroll-behavior-block":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "auto", "contain", "none":
			cs.OverscrollBehaviorY = v
		}

	case "word-break":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "normal", "break-all", "keep-all", "break-word":
			cs.WordBreak = v
		case "initial", "unset":
			cs.WordBreak = "normal"
		}

	case "hyphens", "-webkit-hyphens":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "none", "manual", "auto":
			cs.Hyphens = v
		case "initial", "unset":
			cs.Hyphens = "manual"
		}

	case "tab-size":
		v := strings.ToLower(strings.TrimSpace(value))
		if v == "initial" || v == "unset" {
			cs.TabSize = 8
		} else if parsed.Type == css.ValueNumber && parsed.Num >= 0 {
			cs.TabSize = float32(parsed.Num)
		} else if parsed.Type == css.ValueLength {
			cs.TabSize = resolveLengthEm(parsed, 8, cs.FontSize, cs.rootFontSize)
		}

	case "writing-mode":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "horizontal-tb", "vertical-rl", "vertical-lr", "sideways-rl", "sideways-lr":
			cs.WritingMode = v
		case "initial", "unset":
			cs.WritingMode = "horizontal-tb"
		}

	case "text-orientation":
		v := strings.ToLower(strings.TrimSpace(value))
		switch v {
		case "mixed", "upright", "sideways":
			cs.TextOrientation = v
		case "initial", "unset":
			cs.TextOrientation = "mixed"
		}

	case "font-variant-numeric":
		v := strings.ToLower(strings.TrimSpace(value))
		if v == "initial" || v == "unset" {
			cs.FontVariantNumeric = "normal"
		} else if v != "" {
			cs.FontVariantNumeric = v
		}

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
	case "flex-flow":
		parseFlexFlow(cs, value)
	case "justify-content":
		cs.JustifyContent = value
	case "justify-self":
		cs.JustifySelf = value
	case "align-items":
		cs.AlignItems = value
	case "align-self":
		cs.AlignSelf = value
	case "place-self":
		parts := splitTopLevelSpace(value)
		if len(parts) > 0 {
			cs.AlignSelf = parts[0]
			cs.JustifySelf = parts[0]
		}
		if len(parts) > 1 {
			cs.JustifySelf = parts[1]
		}
	case "flex-grow":
		cs.FlexGrow = float32(parsed.Num)
	case "flex-shrink":
		cs.FlexShrink = float32(parsed.Num)
	case "flex-basis":
		cs.FlexBasis = resolveLength(parsed, -1)
	case "order":
		if parsed.Type == css.ValueNumber {
			cs.Order = int(parsed.Num)
		}
	case "object-fit":
		cs.ObjectFit = value
	case "gap", "grid-gap":
		parts := splitTopLevelSpace(value)
		if len(parts) == 1 {
			g := resolveLengthEm(css.ParseValue(parts[0]), 0, cs.FontSize, cs.rootFontSize)
			cs.RowGap, cs.ColumnGap = g, g
		} else if len(parts) >= 2 {
			cs.RowGap = resolveLengthEm(css.ParseValue(parts[0]), 0, cs.FontSize, cs.rootFontSize)
			cs.ColumnGap = resolveLengthEm(css.ParseValue(parts[1]), 0, cs.FontSize, cs.rootFontSize)
		}
	case "row-gap", "grid-row-gap":
		cs.RowGap = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "column-gap", "grid-column-gap":
		cs.ColumnGap = resolveLengthEm(parsed, 0, cs.FontSize, cs.rootFontSize)
	case "grid-template-columns":
		cs.GridTemplateColumns = value
	case "grid-template-rows":
		cs.GridTemplateRows = value
	case "grid-template-areas":
		cs.GridTemplateAreas = value
	case "grid-template":
		parseGridTemplateShorthand(cs, value)
	case "grid-area":
		cs.GridArea = strings.TrimSpace(value)
	case "grid-column":
		cs.GridColumn = strings.TrimSpace(value)
		cs.GridColumnSpan = parseGridSpan(cs.GridColumn)
	case "grid-column-start":
		cs.GridColumn = buildGridPlacement(strings.TrimSpace(value), extractGridEnd(cs.GridColumn))
		cs.GridColumnSpan = parseGridSpan(cs.GridColumn)
	case "grid-column-end":
		cs.GridColumn = buildGridPlacement(extractGridStart(cs.GridColumn), strings.TrimSpace(value))
		cs.GridColumnSpan = parseGridSpan(cs.GridColumn)
	case "grid-row":
		cs.GridRow = strings.TrimSpace(value)
		cs.GridRowSpan = parseGridSpan(cs.GridRow)
	case "grid-row-start":
		cs.GridRow = buildGridPlacement(strings.TrimSpace(value), extractGridEnd(cs.GridRow))
		cs.GridRowSpan = parseGridSpan(cs.GridRow)
	case "grid-row-end":
		cs.GridRow = buildGridPlacement(extractGridStart(cs.GridRow), strings.TrimSpace(value))
		cs.GridRowSpan = parseGridSpan(cs.GridRow)
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
		parts := splitTopLevelSpace(value)
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

	case "box-shadow":
		cs.BoxShadow = css.ParseBoxShadow(value)
	case "text-shadow":
		cs.TextShadow = css.ParseTextShadow(value)

	case "transition":
		cs.Transitions = css.ParseTransitionShorthand(value)
	case "transition-property":
		applyTransitionProperty(cs, value)
	case "transition-duration":
		applyTransitionDuration(cs, value)
	case "transition-timing-function":
		applyTransitionTimingFunction(cs, value)
	case "transition-delay":
		applyTransitionDelay(cs, value)

	case "transform":
		cs.Transform = css.ParseTransform(value)
	case "transform-origin":
		cs.TransformOrigin = css.ParseTransformOrigin(value)
	case "filter":
		cs.Filter = css.ParseFilter(value)

	case "animation":
		name, dur, timing, delay, iterCount, dir, fill := css.ParseAnimationShorthand(value)
		cs.AnimationName = name
		cs.AnimationDuration = dur
		cs.AnimationTiming = timing
		cs.AnimationDelay = delay
		cs.AnimationIterCount = iterCount
		cs.AnimationDirection = dir
		cs.AnimationFillMode = fill
	case "animation-name":
		cs.AnimationName = value
	case "animation-duration":
		cs.AnimationDuration = css.ParseDuration(value)
	case "animation-timing-function":
		cs.AnimationTiming = css.ParseTimingFunc(value)
	case "animation-delay":
		cs.AnimationDelay = css.ParseDuration(value)
	case "animation-iteration-count":
		if strings.ToLower(strings.TrimSpace(value)) == "infinite" {
			cs.AnimationIterCount = 0
		} else {
			p := css.ParseValue(value)
			if p.Type == css.ValueNumber {
				cs.AnimationIterCount = int(p.Num)
			}
		}
	case "animation-direction":
		cs.AnimationDirection = css.ParseAnimationDirection(value)
	case "animation-fill-mode":
		cs.AnimationFillMode = css.ParseAnimationFillMode(value)
	case "backdrop-filter", "-webkit-backdrop-filter":
		cs.BackdropFilter = css.ParseFilter(value)
	case "quotes":
		cs.Quotes = parseQuotes(value)
	case "aspect-ratio":
		cs.AspectRatio = parseAspectRatio(value)
	}
}

func parseAspectRatio(v string) float32 {
	v = strings.TrimSpace(v)
	if v == "" || v == "none" || v == "auto" {
		return 0
	}
	lower := strings.ToLower(v)
	if strings.HasPrefix(lower, "auto ") {
		v = strings.TrimSpace(v[5:])
	}
	parts := strings.Split(v, "/")
	if len(parts) == 2 {
		w, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 32)
		h, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 32)
		if err1 == nil && err2 == nil && w > 0 && h > 0 {
			return float32(w / h)
		}
		return 0
	}
	if len(parts) == 1 {
		w, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 32)
		if err == nil && w > 0 {
			return float32(w)
		}
	}
	return 0
}

func parseQuotes(value string) []string {
	v := strings.TrimSpace(value)
	lower := strings.ToLower(v)
	if lower == "none" {
		return []string{}
	}
	if lower == "auto" {
		return []string{"“", "”", "‘", "’"}
	}
	var quotes []string
	i := 0
	for i < len(v) {
		for i < len(v) && (v[i] == ' ' || v[i] == '\t' || v[i] == '\n' || v[i] == '\r') {
			i++
		}
		if i >= len(v) {
			break
		}
		if v[i] == '"' || v[i] == '\'' {
			quoteChar := v[i]
			i++
			var sb strings.Builder
			for i < len(v) {
				if v[i] == '\\' && i+1 < len(v) {
					sb.WriteByte(v[i])
					i++
					sb.WriteByte(v[i])
					i++
					continue
				}
				if v[i] == quoteChar {
					break
				}
				sb.WriteByte(v[i])
				i++
			}
			str := css.DecodeEscapes(sb.String())
			quotes = append(quotes, str)
			if i < len(v) && v[i] == quoteChar {
				i++
			}
		} else {
			i++
		}
	}
	if len(quotes)%2 != 0 {
		quotes = quotes[:len(quotes)-1]
	}
	if len(quotes) == 0 {
		return []string{}
	}
	return quotes
}

const MarginAuto float32 = -1

func valueIsAuto(v css.Value) bool {
	return v.Type == css.ValueKeyword && (v.Str == "auto" || v.Str == "")
}

func strIsAuto(s string) bool {
	return valueIsAuto(css.ParseValue(s))
}

func logicalPair(v string, auto, fontSize float32, rootFontSize ...float32) (float32, float32) {
	parts := splitTopLevelSpace(v)
	if len(parts) == 0 {
		return auto, auto
	}
	a := resolveLengthEm(css.ParseValue(parts[0]), auto, fontSize, rootFontSize...)
	if len(parts) < 2 {
		return a, a
	}
	return a, resolveLengthEm(css.ParseValue(parts[1]), auto, fontSize, rootFontSize...)
}

func splitTopLevelSpace(v string) []string {
	var out []string
	depth, start := 0, -1
	for i := 0; i < len(v); i++ {
		switch c := v[i]; c {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ' ', '\t', '\n', '\r', '\f':
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

func againstOwnFont(prop string) bool {
	switch prop {
	case "width", "height", "min-width", "min-height", "max-width", "max-height",
		"inline-size", "block-size", "min-inline-size", "min-block-size", "max-inline-size", "max-block-size",
		"margin", "margin-top", "margin-right", "margin-bottom", "margin-left",
		"margin-block", "margin-block-start", "margin-block-end",
		"margin-inline", "margin-inline-start", "margin-inline-end",
		"padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
		"padding-block", "padding-block-start", "padding-block-end",
		"padding-inline", "padding-inline-start", "padding-inline-end",
		"top", "right", "bottom", "left",
		"inset", "inset-block", "inset-block-start", "inset-block-end",
		"inset-inline", "inset-inline-start", "inset-inline-end":
		return true
	}
	return false
}

type DeferredLength struct {
	Expr css.Value
	Em   float32
	Rem  float32
	Set  bool
}

func deferPercentFunc(v css.Value, fontSize float32, rootFontSize ...float32) (DeferredLength, bool) {
	if v.Type == css.ValueFunc && v.FuncHasPercent() {
		rem := float32(16)
		if len(rootFontSize) > 0 && rootFontSize[0] > 0 {
			rem = rootFontSize[0]
		}
		return DeferredLength{Expr: v, Em: fontSize, Rem: rem, Set: true}, true
	}
	return DeferredLength{}, false
}

func (d DeferredLength) Resolve(base, vw, vh float32) (float32, bool) {
	if !d.Set {
		return 0, false
	}
	return css.EvalFuncWithBaseAndRem(d.Expr.Func, d.Expr.Str, d.Em, d.Rem, vw, vh, base)
}

func resolveMaxLength(v css.Value, fontSize float32, rootFontSize ...float32) float32 {
	if v.Type == css.ValueKeyword && v.Str == "none" {
		return -1
	}
	return resolveLengthEm(v, -1, fontSize, rootFontSize...)
}

func resolveLength(v css.Value, auto float32) float32 {
	if v.Type == css.ValueKeyword && (v.Str == "auto" || v.Str == "") {
		return auto
	}
	return v.ToLength()
}

func resolveLengthEm(v css.Value, auto, fontSize float32, rootFontSize ...float32) float32 {
	if v.Type == css.ValueKeyword && (v.Str == "auto" || v.Str == "") {
		return auto
	}
	rem := float32(16)
	if len(rootFontSize) > 0 && rootFontSize[0] > 0 {
		rem = rootFontSize[0]
	}
	return v.ToLengthWithEmAndRem(fontSize, rem)
}

func lengthSpecified(v css.Value) bool {
	switch v.Type {
	case css.ValueLength, css.ValuePercentage:
		return true
	case css.ValueNumber:
		return v.Num == 0
	}
	return false
}

func resolveFontSize(v css.Value, parent, root float32) float32 {
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
			if root <= 0 {
				root = 16
			}
			return float32(v.Num) * root
		}
		if v.Unit == "em" || v.Unit == "ex" {
			return parent * float32(v.Num)
		}
		return v.ToLength()
	case css.ValueFunc:
		if v.FuncHasPercent() {
			if f, ok := css.EvalFuncWithBase(v.Func, v.Str, parent, 0, 0, parent); ok && f > 0 {
				return f
			}
			return parent
		}
		if f := v.ToLengthWithEm(parent); f > 0 {
			return f
		}
		return parent
	}
	return float32(v.Num)
}

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
		return v.ToLengthWithEm(fontSize)
	case css.ValueKeyword:
		if v.Str == "normal" {
			return -1
		}
	}
	return fontSize * 1.2
}

func applyDisplay(cs *ComputedStyle, value string) {
	cs.FlowRoot = value == "flow-root"
	cs.Display = parseDisplay(value)
	if cs.FlowRoot {
		cs.Display = DisplayBlock
	}
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
	case "inline-table":
		return DisplayInlineTable
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

func parseTextAlign(v, dir string) TextAlign {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "left":
		return TextAlignLeft
	case "right":
		return TextAlignRight
	case "center":
		return TextAlignCenter
	case "justify":
		return TextAlignJustify
	case "start":
		return TextAlignStart
	case "end":
		if dir == "rtl" {
			return TextAlignLeft
		}
		return TextAlignRight
	}
	return TextAlignStart
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

type CustomFonts struct {
	mu       sync.RWMutex
	families map[string]uint16
	variants map[CustomFontKey]uint16
}

func (f *CustomFonts) Register(family string, idx uint16, bold, italic, light bool) {
	if f == nil {
		return
	}
	name := strings.ToLower(family)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.families == nil {
		f.families = map[string]uint16{}
	}
	if f.variants == nil {
		f.variants = map[CustomFontKey]uint16{}
	}
	f.families[name] = idx
	f.variants[CustomFontKey{Family: name, Bold: bold, Italic: italic, Light: light}] = idx
}

func (f *CustomFonts) has(name string) bool {
	if f == nil {
		return false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	_, ok := f.families[name]
	return ok
}

func (f *CustomFonts) index(family string, bold, italic, light bool) uint16 {
	if f == nil || family == "" {
		return 0
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if idx, ok := f.variants[CustomFontKey{Family: family, Bold: bold, Italic: italic, Light: light}]; ok {
		return idx
	}
	if idx, ok := f.variants[CustomFontKey{Family: family}]; ok {
		return idx
	}
	return 0
}

type CustomFontKey struct {
	Family string
	Bold   bool
	Italic bool
	Light  bool
}

func parseFontFamily(v string, fonts *CustomFonts) (frame.FontFamily, string) {
	for _, part := range strings.Split(v, ",") {
		name := normalizeFontName(part)
		if fam, ok := fontFamilies[name]; ok {
			return fam, ""
		}
		if fonts.has(name) {
			return frame.FontGo, name
		}
	}
	return frame.FontTimes, ""
}

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
	case "decimal-leading-zero":
		return ListStyleDecimalLeadingZero
	case "upper-roman":
		return ListStyleUpperRoman
	case "lower-roman":
		return ListStyleLowerRoman
	case "upper-alpha", "upper-latin":
		return ListStyleUpperAlpha
	case "lower-alpha", "lower-latin":
		return ListStyleLowerAlpha
	case "georgian":
		return ListStyleGeorgian
	case "armenian", "upper-armenian", "lower-armenian":
		return ListStyleArmenian
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

func parseFlexFlow(cs *ComputedStyle, v string) {
	for _, part := range splitTopLevelSpace(v) {
		switch lower := strings.ToLower(strings.TrimSpace(part)); lower {
		case "row", "row-reverse", "column", "column-reverse":
			cs.FlexDirection = parseFlexDirection(lower)
		case "nowrap", "wrap", "wrap-reverse":
			cs.FlexWrap = parseFlexWrap(lower)
		}
	}
}

func parseColorValue(v string) css.Color {
	return parseColorValueWithScheme(v, "")
}

func parseColorValueWithScheme(v, scheme string) css.Color {
	c, _ := css.ParseColorWithScheme(v, scheme)
	return c
}

func isCurrentColor(v string) bool {
	v = strings.TrimSpace(v)
	const want = "currentcolor"
	if len(v) != len(want) {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != want[i] {
			return false
		}
	}
	return true
}

func resolveCurrentColor(cs *ComputedStyle, matched []*indexedSelector, inline, hints []css.Declaration) {
	var border, top, right, bottom, left, bg bool
	visit := func(prop, value string) {
		if !isCurrentColor(value) {
			switch prop {
			case "border-color":
				border, top, right, bottom, left = true, true, true, true, true
			case "border-top-color":
				top = true
			case "border-right-color":
				right = true
			case "border-bottom-color":
				bottom = true
			case "border-left-color":
				left = true
			case "background-color":
				bg = true
			}
			return
		}
		switch prop {
		case "border-color":
			if border {
				return
			}
			if !top {
				cs.BorderTopColor = cs.Color
			}
			if !right {
				cs.BorderRightColor = cs.Color
			}
			if !bottom {
				cs.BorderBottomColor = cs.Color
			}
			if !left {
				cs.BorderLeftColor = cs.Color
			}
			border, top, right, bottom, left = true, true, true, true, true
		case "border-top-color":
			if top {
				return
			}
			cs.BorderTopColor = cs.Color
			top = true
		case "border-right-color":
			if right {
				return
			}
			cs.BorderRightColor = cs.Color
			right = true
		case "border-bottom-color":
			if bottom {
				return
			}
			cs.BorderBottomColor = cs.Color
			bottom = true
		case "border-left-color":
			if left {
				return
			}
			cs.BorderLeftColor = cs.Color
			left = true
		case "background-color":
			if bg {
				return
			}
			cs.BackgroundColor = cs.Color
			bg = true
		}
	}
	for i := len(hints) - 1; i >= 0; i-- {
		visit(hints[i].Property, hints[i].Value)
	}
	for i := len(inline) - 1; i >= 0; i-- {
		visit(inline[i].Property, inline[i].Value)
	}
	for i := len(matched) - 1; i >= 0; i-- {
		decls := matched[i].rule.Declarations
		for j := len(decls) - 1; j >= 0; j-- {
			visit(decls[j].Property, decls[j].Value)
		}
	}
}

func parseBackgroundColor(v string) css.Color {
	return parseBackgroundColorWithScheme(v, "")
}

func parseBackgroundColorWithScheme(v, scheme string) css.Color {
	if c, ok := css.ParseColorWithScheme(v, scheme); ok {
		return c
	}
	s := v
	for {
		urlIdx := strings.Index(strings.ToLower(s), "url(")
		if urlIdx < 0 {
			break
		}
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
		if c, ok := css.ParseColorWithScheme(tok, scheme); ok {
			return c
		}
	}
	return css.Color{}
}

func extractBackgroundURL(v string) string {
	t, ok := css.ScanURL(v)
	if !ok {
		return ""
	}
	target := css.URLPayload(t)
	if target == "" || target == "none" {
		return ""
	}
	return target
}

func parseBackgroundRepeat(v string) (BgRepeat, BgRepeat) {
	fields := strings.Fields(v)
	if len(fields) == 0 {
		return BgRepeatRepeat, BgRepeatRepeat
	}
	one := func(tok string) BgRepeat {
		switch strings.ToLower(tok) {
		case "repeat-x":
			return BgRepeatRepeatX
		case "repeat-y":
			return BgRepeatRepeatY
		case "no-repeat":
			return BgRepeatNoRepeat
		case "round":
			return BgRepeatRound
		case "space":
			return BgRepeatSpace
		default:
			return BgRepeatRepeat
		}
	}
	x := one(fields[0])
	if len(fields) < 2 {
		return x, x
	}
	y := one(fields[1])
	if y == BgRepeatRepeatX {
		y = BgRepeatRepeat
	}
	if y == BgRepeatRepeatY {
		y = BgRepeatNoRepeat
	}
	return x, y
}

func parseBackgroundBox(v string, allowText bool) BgBox {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "padding-box":
		return BgBoxPadding
	case "content-box":
		return BgBoxContent
	case "border-area":
		return BgBoxBorderArea
	case "text":
		if allowText {
			return BgBoxText
		}
	case "border-box":
		return BgBoxBorder
	}
	if allowText {
		return BgBoxBorder
	}
	return BgBoxPadding
}

func isRepeatTok(t string) bool {
	switch strings.ToLower(t) {
	case "repeat", "repeat-x", "repeat-y", "no-repeat", "round", "space":
		return true
	}
	return false
}

func parseBackgroundShorthandExtras(cs *ComputedStyle, v string) {
	toks := firstBackgroundLayer(tokenizeBackground(v))
	var pos []string
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t == "/" {
			var size []string
			for j := i + 1; j < len(toks) && toks[j] != "/"; j++ {
				if !isSizeTok(cs, toks[j]) {
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
		if isRepeatTok(t) {
			if i+1 < len(toks) && isRepeatTok(toks[i+1]) {
				cs.BackgroundRepeat, cs.BackgroundRepeatY = parseBackgroundRepeat(t + " " + toks[i+1])
				i++
			} else {
				cs.BackgroundRepeat, cs.BackgroundRepeatY = parseBackgroundRepeat(t)
			}
			continue
		}
		switch strings.ToLower(t) {
		default:
			if isPositionTok(cs, t) {
				pos = append(pos, t)
			}
		}
	}
	if len(pos) > 0 {
		parseBackgroundPosition(cs, strings.Join(pos, " "))
	}
}

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

func firstBackgroundLayer(toks []string) []string {
	for i, t := range toks {
		depth := 0
		for j := 0; j < len(t); j++ {
			switch t[j] {
			case '(':
				depth++
			case ')':
				if depth > 0 {
					depth--
				}
			case ',':
				if depth == 0 {
					head := strings.TrimSpace(t[:j])
					out := make([]string, 0, i+1)
					out = append(out, toks[:i]...)
					if head != "" {
						out = append(out, head)
					}
					return out
				}
			}
		}
	}
	return toks
}

func isPositionTok(cs *ComputedStyle, t string) bool {
	switch strings.ToLower(t) {
	case "top", "bottom", "left", "right", "center":
		return true
	}
	_, _, ok := parseBgLen(cs, t)
	return ok
}

func isSizeTok(cs *ComputedStyle, t string) bool {
	switch strings.ToLower(t) {
	case "auto", "contain", "cover":
		return true
	}
	_, _, ok := parseBgLen(cs, t)
	return ok
}

func parseBackgroundSize(cs *ComputedStyle, v string) {
	parts := splitTopLevelSpace(v)
	if len(parts) == 0 {
		return
	}
	switch strings.ToLower(parts[0]) {
	case "auto":
		if len(parts) > 1 && strings.ToLower(parts[1]) != "auto" {
			if h, hPct, okH := parseBgLen(cs, parts[1]); okH {
				cs.BackgroundSize = BgSizeLength
				cs.BgSizeW, cs.BgSizeWPct = -1, false
				cs.BgSizeH, cs.BgSizeHPct = h, hPct
				return
			}
		}
		cs.BackgroundSize = BgSizeAuto
		return
	case "contain":
		cs.BackgroundSize = BgSizeContain
		return
	case "cover":
		cs.BackgroundSize = BgSizeCover
		return
	}
	w, wPct, okW := parseBgLen(cs, parts[0])
	if !okW {
		cs.BackgroundSize = BgSizeAuto
		return
	}
	cs.BackgroundSize = BgSizeLength
	cs.BgSizeW, cs.BgSizeWPct = w, wPct
	cs.BgSizeH = -1
	if len(parts) > 1 && strings.ToLower(parts[1]) != "auto" {
		if h, hPct, okH := parseBgLen(cs, parts[1]); okH {
			cs.BgSizeH, cs.BgSizeHPct = h, hPct
		}
	}
}

func parseBackgroundPosition(cs *ComputedStyle, v string) {
	parts := splitTopLevelSpace(v)
	if len(parts) == 0 {
		return
	}
	setX := func(tok string) {
		setPosX(cs, tok)
	}
	setY := func(tok string) {
		setPosY(cs, tok)
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
	if len(parts) >= 3 {
		if parsePositionRun(cs, parts) {
			return
		}
	}
	if isVerticalKeyword(parts[0]) {
		setY(parts[0])
		setX(parts[1])
	} else {
		setX(parts[0])
		setY(parts[1])
	}
}

func setPosX(cs *ComputedStyle, tok string) {
	switch strings.ToLower(tok) {
	case "left":
		cs.BackgroundPosXMode = BgPosStart
	case "center":
		cs.BackgroundPosXMode = BgPosCenter
	case "right":
		cs.BackgroundPosXMode = BgPosEnd
	default:
		if v, pct, ok := parseBgLen(cs, tok); ok {
			cs.BackgroundPosXMode = BgPosLength
			cs.BackgroundPosX, cs.BgPosXPct = v, pct
		}
	}
}

func setPosY(cs *ComputedStyle, tok string) {
	switch strings.ToLower(tok) {
	case "top":
		cs.BackgroundPosYMode = BgPosStart
	case "center":
		cs.BackgroundPosYMode = BgPosCenter
	case "bottom":
		cs.BackgroundPosYMode = BgPosEnd
	default:
		if v, pct, ok := parseBgLen(cs, tok); ok {
			cs.BackgroundPosYMode = BgPosLength
			cs.BackgroundPosY, cs.BgPosYPct = v, pct
		}
	}
}

func parsePositionRun(cs *ComputedStyle, parts []string) bool {
	type run struct {
		key, off string
		isX      bool
	}
	isOff := func(s string) bool {
		_, _, ok := parseBgLen(cs, s)
		return ok
	}
	var runs []run
	i := 0
	for i < len(parts) && len(runs) < 2 {
		k := parts[i]
		lk := strings.ToLower(k)
		var isX bool
		switch lk {
		case "left", "right":
			isX = true
		case "top", "bottom":
			isX = false
		case "center":
			isX = len(runs) == 0
		default:
			return false
		}
		r := run{key: k, isX: isX}
		if i+1 < len(parts) && isOff(parts[i+1]) {
			if lk == "center" {
				return false
			}
			r.off = parts[i+1]
			i++
		}
		runs = append(runs, r)
		i++
	}
	if i != len(parts) || len(runs) != 2 || runs[0].isX == runs[1].isX {
		return false
	}
	apply := func(r run) {
		lk := strings.ToLower(r.key)
		var start string
		if r.isX {
			start = "left"
		} else {
			start = "top"
		}
		if r.off == "" {
			if r.isX {
				setPosX(cs, r.key)
			} else {
				setPosY(cs, r.key)
			}
			return
		}
		v, isPct, ok := parseBgLen(cs, r.off)
		if !ok {
			if r.isX {
				setPosX(cs, r.key)
			} else {
				setPosY(cs, r.key)
			}
			return
		}
		if r.isX {
			cs.BackgroundPosX, cs.BgPosXPct = v, isPct
			if lk == start {
				cs.BackgroundPosXMode = BgPosStartOffset
			} else {
				cs.BackgroundPosXMode = BgPosEndOffset
			}
		} else {
			cs.BackgroundPosY, cs.BgPosYPct = v, isPct
			if lk == start {
				cs.BackgroundPosYMode = BgPosStartOffset
			} else {
				cs.BackgroundPosYMode = BgPosEndOffset
			}
		}
	}
	apply(runs[0])
	apply(runs[1])
	return true
}

func isVerticalKeyword(tok string) bool {
	switch strings.ToLower(tok) {
	case "top", "bottom":
		return true
	}
	return false
}

func parseBgLen(cs *ComputedStyle, tok string) (float32, bool, bool) {
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
	if v := css.ParseValue(tok); v.Type == css.ValueFunc && v.Str != "" && !strings.Contains(v.Str, "%") {
		return resolveLengthEm(v, 0, cs.FontSize), false, true
	}
	return 0, false, false
}

func parseMarginShorthand(v string, fontSize float32, rootFontSize ...float32) (top, right, bottom, left float32, leftAuto, rightAuto bool) {
	parts := splitTopLevelSpace(v)
	switch len(parts) {
	case 1:
		auto := strIsAuto(parts[0])
		val := resolveLengthEm(css.ParseValue(parts[0]), MarginAuto, fontSize, rootFontSize...)
		return val, val, val, val, auto, auto
	case 2:
		tb := resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize, rootFontSize...)
		rl := resolveLengthEm(css.ParseValue(parts[1]), MarginAuto, fontSize, rootFontSize...)
		auto := strIsAuto(parts[1])
		return tb, rl, tb, rl, auto, auto
	case 3:
		top = resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize, rootFontSize...)
		rl := resolveLengthEm(css.ParseValue(parts[1]), MarginAuto, fontSize, rootFontSize...)
		bottom = resolveLengthEm(css.ParseValue(parts[2]), 0, fontSize, rootFontSize...)
		auto := strIsAuto(parts[1])
		return top, rl, bottom, rl, auto, auto
	case 4:
		top = resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize, rootFontSize...)
		right = resolveLengthEm(css.ParseValue(parts[1]), MarginAuto, fontSize, rootFontSize...)
		bottom = resolveLengthEm(css.ParseValue(parts[2]), 0, fontSize, rootFontSize...)
		left = resolveLengthEm(css.ParseValue(parts[3]), MarginAuto, fontSize, rootFontSize...)
		return top, right, bottom, left, strIsAuto(parts[3]), strIsAuto(parts[1])
	}
	return
}

func parsePaddingShorthand(v string, fontSize float32, rootFontSize ...float32) (top, right, bottom, left float32) {
	parts := splitTopLevelSpace(v)
	switch len(parts) {
	case 1:
		val := resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize, rootFontSize...)
		return val, val, val, val
	case 2:
		tb := resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize, rootFontSize...)
		rl := resolveLengthEm(css.ParseValue(parts[1]), 0, fontSize, rootFontSize...)
		return tb, rl, tb, rl
	case 3:
		top = resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize, rootFontSize...)
		rl := resolveLengthEm(css.ParseValue(parts[1]), 0, fontSize, rootFontSize...)
		bottom = resolveLengthEm(css.ParseValue(parts[2]), 0, fontSize, rootFontSize...)
		return top, rl, bottom, rl
	case 4:
		top = resolveLengthEm(css.ParseValue(parts[0]), 0, fontSize, rootFontSize...)
		right = resolveLengthEm(css.ParseValue(parts[1]), 0, fontSize, rootFontSize...)
		bottom = resolveLengthEm(css.ParseValue(parts[2]), 0, fontSize, rootFontSize...)
		left = resolveLengthEm(css.ParseValue(parts[3]), 0, fontSize, rootFontSize...)
		return
	}
	return
}

func parseBoxShorthand(v string) (top, right, bottom, left float32) {
	parts := splitTopLevelSpace(v)
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
	widthSpecified := false
	for _, p := range splitColorTokens(v) {
		lower := strings.ToLower(p)
		if isBorderStyle(lower) {
			style = lower
		} else if c, ok := css.ParseColor(lower); ok {
			color = c
		} else {
			parsed := css.ParseValue(p)
			if parsed.Type == css.ValueNumber || parsed.Type == css.ValueLength || parsed.Type == css.ValueFunc {
				width = resolveLength(parsed, 0)
				widthSpecified = true
			}
		}
	}
	if !widthSpecified && style != "" && style != "none" && style != "hidden" {
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
	return parseBorderColorsWithScheme(v, "")
}

func parseBorderColorsWithScheme(v, scheme string) [4]css.Color {
	parts := splitColorTokens(v)
	var colors [4]css.Color
	switch len(parts) {
	case 1:
		c, _ := css.ParseColorWithScheme(parts[0], scheme)
		return [4]css.Color{c, c, c, c}
	case 2:
		t, _ := css.ParseColorWithScheme(parts[0], scheme)
		r, _ := css.ParseColorWithScheme(parts[1], scheme)
		return [4]css.Color{t, r, t, r}
	case 3:
		colors[0], _ = css.ParseColorWithScheme(parts[0], scheme)
		colors[1], _ = css.ParseColorWithScheme(parts[1], scheme)
		colors[2], _ = css.ParseColorWithScheme(parts[2], scheme)
		colors[3] = colors[1]
	case 4:
		colors[0], _ = css.ParseColorWithScheme(parts[0], scheme)
		colors[1], _ = css.ParseColorWithScheme(parts[1], scheme)
		colors[2], _ = css.ParseColorWithScheme(parts[2], scheme)
		colors[3], _ = css.ParseColorWithScheme(parts[3], scheme)
	}
	return colors
}

func parseFontShorthand(cs *ComputedStyle, v string, parentFontSize, rootFontSize float32, fonts *CustomFonts) {
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
	cs.FontSize = clampFontSize(resolveFontSize(css.ParseValue(sizeTok), parentFontSize, rootFontSize))
	if line != "" {
		parsed := css.ParseValue(line)
		cs.LineHeight = resolveLineHeight(parsed, cs.FontSize)
		cs.LineHeightRatio = lineHeightRatio(parsed)
		cs.LineHeightEm = lineHeightEm(parsed)
	} else {
		cs.LineHeight, cs.LineHeightRatio, cs.LineHeightEm = -1, 0, 0
	}
	if fam := strings.TrimSpace(strings.Join(parts[sizeIdx+1:], " ")); fam != "" {
		cs.FontFamily, cs.CustomFamily = parseFontFamily(fam, fonts)
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

func radiusGroup(cs *ComputedStyle, v string) [4]float32 {
	vals := make([]float32, 0, 4)
	for _, p := range splitTopLevelSpace(v) {
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

func parseCornerRadius(cs *ComputedStyle, corner *[2]float32, v string) {
	parts := splitTopLevelSpace(v)
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
	parts := splitTopLevelSpace(v)
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
		} else if v := css.ParseValue(parts[0]); v.Type == css.ValueNumber {
			cs.FlexGrow = float32(v.Num)
			cs.FlexShrink = 1
			cs.FlexBasis = 0
		} else {
			cs.FlexGrow = 1
			cs.FlexShrink = 1
			cs.FlexBasis = resolveLength(v, -1)
		}
	case 2:
		cs.FlexGrow = float32(css.ParseValue(parts[0]).Num)
		if second := css.ParseValue(parts[1]); second.Type == css.ValueNumber {
			cs.FlexShrink = float32(second.Num)
			cs.FlexBasis = 0
		} else {
			cs.FlexShrink = 1
			cs.FlexBasis = resolveLength(second, -1)
		}
	case 3:
		basis := css.ParseValue(parts[2])
		if basis.Type == css.ValueNumber && basis.Num != 0 {
			return
		}
		cs.FlexGrow = float32(css.ParseValue(parts[0]).Num)
		cs.FlexShrink = float32(css.ParseValue(parts[1]).Num)
		if basis.Type != css.ValueList {
			cs.FlexBasis = resolveLength(basis, -1)
		}
	}
}

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

func parseGridSpan(v string) int {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || v == "auto" {
		return 1
	}
	if i := strings.Index(v, "/"); i >= 0 {
		left := strings.TrimSpace(v[:i])
		right := strings.TrimSpace(v[i+1:])
		if strings.HasPrefix(left, "span") {
			if n := gridSpanNum(strings.TrimSpace(strings.TrimPrefix(left, "span"))); n > 0 {
				return n
			}
		}
		if strings.HasPrefix(right, "span") {
			if n := gridSpanNum(strings.TrimSpace(strings.TrimPrefix(right, "span"))); n > 0 {
				return n
			}
		}
		a, b := gridSpanNum(left), gridSpanNum(right)
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

func buildGridPlacement(start, end string) string {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if start == "" && end == "" {
		return ""
	}
	if start != "" && end != "" {
		return start + " / " + end
	}
	if start != "" {
		return start
	}
	return "auto / " + end
}

func extractGridStart(combined string) string {
	combined = strings.TrimSpace(combined)
	if i := strings.Index(combined, "/"); i >= 0 {
		return strings.TrimSpace(combined[:i])
	}
	return combined
}

func extractGridEnd(combined string) string {
	combined = strings.TrimSpace(combined)
	if i := strings.Index(combined, "/"); i >= 0 {
		return strings.TrimSpace(combined[i+1:])
	}
	return ""
}

func gridSpanNum(s string) int {
	p := css.ParseValue(s)
	if p.Type != css.ValueNumber || p.Num <= 0 {
		return 0
	}
	return int(p.Num)
}

func applyTransitionProperty(cs *ComputedStyle, value string) {
	props := css.ParseTransitionProperty(value)
	if len(cs.Transitions) == 0 {
		if len(props) == 0 {
			cs.Transitions = nil
			return
		}
		for _, p := range props {
			cs.Transitions = append(cs.Transitions, css.Transition{
				Property: p,
				Timing:   css.TimingEase,
			})
		}
		return
	}
	for i := range cs.Transitions {
		if i < len(props) {
			cs.Transitions[i].Property = props[i]
		}
	}
}

func applyTransitionDuration(cs *ComputedStyle, value string) {
	durations := css.ParseTransitionDuration(value)
	if len(cs.Transitions) == 0 && len(durations) > 0 {
		cs.Transitions = append(cs.Transitions, css.Transition{
			Property: "all",
			Timing:   css.TimingEase,
		})
	}
	for i := range cs.Transitions {
		if i < len(durations) {
			cs.Transitions[i].Duration = durations[i]
		}
	}
}

func applyTransitionTimingFunction(cs *ComputedStyle, value string) {
	funcs := css.ParseTransitionTimingFunction(value)
	for i := range cs.Transitions {
		if i < len(funcs) {
			cs.Transitions[i].Timing = funcs[i]
		}
	}
}

func applyTransitionDelay(cs *ComputedStyle, value string) {
	delays := css.ParseTransitionDelay(value)
	for i := range cs.Transitions {
		if i < len(delays) {
			cs.Transitions[i].Delay = delays[i]
		}
	}
}

func isAnchorFunc(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return strings.HasPrefix(s, "anchor(") && strings.HasSuffix(s, ")")
}

func isAnchorSizeFunc(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return strings.HasPrefix(s, "anchor-size(") && strings.HasSuffix(s, ")")
}

func extractAnchorArgs(s string) string {
	s = strings.TrimSpace(s)
	idx := strings.IndexByte(s, '(')
	if idx >= 0 && strings.HasSuffix(s, ")") {
		return strings.TrimSpace(s[idx+1 : len(s)-1])
	}
	return s
}
