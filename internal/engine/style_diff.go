package engine

import (
	"reflect"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

// layoutStyleKey is the projection of computed style consumed by layout.
// Fields omitted here affect painting or interaction only, so their changes
// can reuse the existing arena after all keys compare equal.
type layoutStyleKey struct {
	Display, Position, Float, Clear               uint8
	Overflow, OverflowX, OverflowY                uint8
	BoxSizing                                     uint8
	Visibility, ContentVisibility                 string
	ContainIntrinsicWidth, ContainIntrinsicHeight float32
	AnchorName, PositionAnchor                    string
	PositionAnchorNodeID                          dom.NodeID
	PositionArea                                  string
	TopAnchor, RightAnchor                        string
	BottomAnchor, LeftAnchor                      string
	WidthAnchor, HeightAnchor                     string
	TextWrap, ViewTransitionName                  string
	ShapeOutside                                  string
	LineClamp                                     int
	ShapeMargin                                   float32
	Width, Height, MinWidth, MinHeight            float32
	MaxWidth, MaxHeight                           float32
	WidthCalc, HeightCalc                         style.DeferredLength
	MinWidthCalc, MinHeightCalc                   style.DeferredLength
	MaxWidthCalc, MaxHeightCalc                   style.DeferredLength
	MarginTop, MarginRight                        float32
	MarginBottom, MarginLeft                      float32
	PaddingTop, PaddingRight                      float32
	PaddingBottom, PaddingLeft                    float32
	BorderTopWidth, BorderRightWidth              float32
	BorderBottomWidth, BorderLeftWidth            float32
	BorderTopStyle, BorderRightStyle              string
	BorderBottomStyle, BorderLeftStyle            string
	BorderImage                                   css.BorderImage
	Top, Right, Bottom, Left                      float32
	HasTop, HasRight, HasBottom, HasLeft          bool
	FontSlot                                      frame.FontSlot
	FontSize, LineHeight                          float32
	LineHeightRatio, LineHeightEm                 float32
	Direction                                     string
	TextAlign                                     style.TextAlign
	TextIndent                                    float32
	TextTransform, VerticalAlign                  string
	WhiteSpace                                    style.WhiteSpace
	WordSpacing, LetterSpacing                    float32
	WordBreak, Hyphens                            string
	TabSize                                       float32
	WritingMode, TextOrientation                  string
	FontVariantNumeric                            string
	FlexDirection, FlexWrap                       uint8
	FlexGrow, FlexShrink, FlexBasis               float32
	JustifyContent, AlignItems, AlignSelf         string
	AlignContent                                  string
	RowGap, ColumnGap                             float32
	Order                                         int
	GridTemplateColumns, GridTemplateRows         string
	GridTemplateAreas, GridArea                   string
	GridColumn, GridRow                           string
	GridColumnSpan, GridRowSpan                   int
	JustifyItems                                  string
	TableLayout                                   string
	BorderCollapse                                bool
	BorderSpacingH, BorderSpacingV                float32
	Content                                       string
	ContentParts                                  []style.ContentPart
	CounterReset, CounterSet                      map[string]int
	CounterIncrement                              map[string]int
	Quotes                                        []string
	AspectRatio                                   float32
	OffsetPath, OffsetRotate                      string
	OffsetDistance                                float32
	ScrollPaddingTop, ScrollPaddingRight          float32
	ScrollPaddingBottom, ScrollPaddingLeft        float32
	ScrollMarginTop, ScrollMarginRight            float32
	ScrollMarginBottom, ScrollMarginLeft          float32
	ContainerType, ContainerName                  string
	AnimationName                                 string
	AnimationDuration, AnimationDelay             time.Duration
	AnimationIterCount                            int
	AnimationDirection                            css.AnimationDirection
	AnimationFillMode                             css.AnimationFillMode
	AnimationTimeline                             string
}

func makeLayoutStyleKey(s *style.ComputedStyle) layoutStyleKey {
	if s == nil {
		return layoutStyleKey{}
	}
	return layoutStyleKey{
		Display: uint8(s.Display), Position: uint8(s.Position), Float: uint8(s.Float), Clear: uint8(s.Clear),
		Overflow: uint8(s.Overflow), OverflowX: uint8(s.OverflowX), OverflowY: uint8(s.OverflowY), BoxSizing: uint8(s.BoxSizing),
		Visibility: s.Visibility, ContentVisibility: s.ContentVisibility,
		ContainIntrinsicWidth: s.ContainIntrinsicWidth, ContainIntrinsicHeight: s.ContainIntrinsicHeight,
		AnchorName: s.AnchorName, PositionAnchor: s.PositionAnchor, PositionAnchorNodeID: s.PositionAnchorNodeID, PositionArea: s.PositionArea,
		TopAnchor: s.TopAnchor, RightAnchor: s.RightAnchor,
		BottomAnchor: s.BottomAnchor, LeftAnchor: s.LeftAnchor, WidthAnchor: s.WidthAnchor, HeightAnchor: s.HeightAnchor,
		TextWrap: s.TextWrap, ViewTransitionName: s.ViewTransitionName, ShapeOutside: s.ShapeOutside,
		LineClamp: s.LineClamp, ShapeMargin: s.ShapeMargin,
		Width: s.Width, Height: s.Height, MinWidth: s.MinWidth, MinHeight: s.MinHeight, MaxWidth: s.MaxWidth, MaxHeight: s.MaxHeight,
		WidthCalc: s.WidthCalc, HeightCalc: s.HeightCalc, MinWidthCalc: s.MinWidthCalc, MinHeightCalc: s.MinHeightCalc,
		MaxWidthCalc: s.MaxWidthCalc, MaxHeightCalc: s.MaxHeightCalc,
		MarginTop: s.MarginTop, MarginRight: s.MarginRight, MarginBottom: s.MarginBottom, MarginLeft: s.MarginLeft,
		PaddingTop: s.PaddingTop, PaddingRight: s.PaddingRight, PaddingBottom: s.PaddingBottom, PaddingLeft: s.PaddingLeft,
		BorderTopWidth: s.BorderTopWidth, BorderRightWidth: s.BorderRightWidth,
		BorderBottomWidth: s.BorderBottomWidth, BorderLeftWidth: s.BorderLeftWidth,
		BorderTopStyle: s.BorderTopStyle, BorderRightStyle: s.BorderRightStyle,
		BorderBottomStyle: s.BorderBottomStyle, BorderLeftStyle: s.BorderLeftStyle, BorderImage: s.BorderImage,
		Top: s.Top, Right: s.Right, Bottom: s.Bottom, Left: s.Left,
		HasTop: s.HasTop, HasRight: s.HasRight, HasBottom: s.HasBottom, HasLeft: s.HasLeft,
		FontSlot: s.FontSlot(), FontSize: s.FontSize, LineHeight: s.LineHeight,
		LineHeightRatio: s.LineHeightRatio, LineHeightEm: s.LineHeightEm, Direction: s.Direction,
		TextAlign: s.TextAlign, TextIndent: s.TextIndent, TextTransform: s.TextTransform, VerticalAlign: s.VerticalAlign,
		WhiteSpace: s.WhiteSpace, WordSpacing: s.WordSpacing, LetterSpacing: s.LetterSpacing,
		WordBreak: s.WordBreak, Hyphens: s.Hyphens, TabSize: s.TabSize,
		WritingMode: s.WritingMode, TextOrientation: s.TextOrientation, FontVariantNumeric: s.FontVariantNumeric,
		FlexDirection: uint8(s.FlexDirection), FlexWrap: uint8(s.FlexWrap), FlexGrow: s.FlexGrow, FlexShrink: s.FlexShrink,
		FlexBasis: s.FlexBasis, JustifyContent: s.JustifyContent, AlignItems: s.AlignItems,
		AlignSelf: s.AlignSelf, AlignContent: s.AlignContent, RowGap: s.RowGap, ColumnGap: s.ColumnGap, Order: s.Order,
		GridTemplateColumns: s.GridTemplateColumns, GridTemplateRows: s.GridTemplateRows,
		GridTemplateAreas: s.GridTemplateAreas, GridArea: s.GridArea,
		GridColumn: s.GridColumn, GridRow: s.GridRow, GridColumnSpan: s.GridColumnSpan, GridRowSpan: s.GridRowSpan,
		JustifyItems: s.JustifyItems, TableLayout: s.TableLayout, BorderCollapse: s.BorderCollapse,
		BorderSpacingH: s.BorderSpacingH, BorderSpacingV: s.BorderSpacingV,
		Content: s.Content, ContentParts: s.ContentParts, CounterReset: s.CounterReset, CounterSet: s.CounterSet,
		CounterIncrement: s.CounterIncrement, Quotes: s.Quotes, AspectRatio: s.AspectRatio,
		OffsetPath: s.OffsetPath, OffsetRotate: s.OffsetRotate, OffsetDistance: s.OffsetDistance,
		ScrollPaddingTop: s.ScrollPaddingTop, ScrollPaddingRight: s.ScrollPaddingRight,
		ScrollPaddingBottom: s.ScrollPaddingBottom, ScrollPaddingLeft: s.ScrollPaddingLeft,
		ScrollMarginTop: s.ScrollMarginTop, ScrollMarginRight: s.ScrollMarginRight,
		ScrollMarginBottom: s.ScrollMarginBottom, ScrollMarginLeft: s.ScrollMarginLeft,
		ContainerType: s.ContainerType, ContainerName: s.ContainerName,
		AnimationName: s.AnimationName, AnimationDuration: s.AnimationDuration, AnimationDelay: s.AnimationDelay,
		AnimationIterCount: s.AnimationIterCount, AnimationDirection: s.AnimationDirection,
		AnimationFillMode: s.AnimationFillMode, AnimationTimeline: s.AnimationTimeline,
	}
}

func sameLayoutStyles(before, after map[dom.NodeID]*style.ComputedStyle) bool {
	if len(before) != len(after) {
		return false
	}
	for id, oldStyle := range before {
		newStyle, ok := after[id]
		if !ok || !reflect.DeepEqual(makeLayoutStyleKey(oldStyle), makeLayoutStyleKey(newStyle)) {
			return false
		}
	}
	return true
}

func sameLayoutPseudoStyles(before, after map[style.PseudoKey]*style.ComputedStyle) bool {
	if len(before) != len(after) {
		return false
	}
	for key, oldStyle := range before {
		newStyle, ok := after[key]
		if !ok || !reflect.DeepEqual(makeLayoutStyleKey(oldStyle), makeLayoutStyleKey(newStyle)) {
			return false
		}
	}
	return true
}

func interactiveLayoutChanged(resolution style.InteractiveResolution, current map[dom.NodeID]*style.ComputedStyle, pseudos map[style.PseudoKey]*style.ComputedStyle) bool {
	for id, before := range resolution.PreviousStyles {
		after, ok := current[id]
		if !ok || !reflect.DeepEqual(makeLayoutStyleKey(before), makeLayoutStyleKey(after)) {
			return true
		}
	}
	for _, id := range resolution.Nodes {
		for _, pseudo := range []string{"before", "after", "first-letter"} {
			key := style.PseudoKey{NodeID: id, Pseudo: pseudo}
			before, hadBefore := resolution.PreviousPseudo[key]
			after, hasAfter := pseudos[key]
			// The arena owns the pseudo box's computed style pointer. Refreshing
			// ordinary DOM objects cannot update that anonymous layout object, so
			// any pseudo style change needs a fresh reflow even when geometry is
			// unchanged.
			if hadBefore != hasAfter || (hadBefore && !reflect.DeepEqual(before, after)) {
				return true
			}
		}
	}
	return false
}
