package layout_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

type imageInputMetrics struct{}

func (imageInputMetrics) GlyphAdvance(sizePx int32, r rune, slot frame.FontSlot) int32 {
	return imageInputMetrics{}.GlyphAdvanceFixed(sizePx, r, slot) / 64
}

func (imageInputMetrics) GlyphAdvanceFixed(_ int32, r rune, _ frame.FontSlot) int32 {
	switch r {
	case 'S':
		return 555
	case 'u', 'b':
		return 463
	case 'm':
		return 695
	case 'i':
		return 185
	case 't':
		return 228
	case 'G':
		return 644
	case 'o':
		return 466
	default:
		return 6 * 64
	}
}

func (imageInputMetrics) LineMetrics(_ int32, _ frame.FontSlot) (int32, int32, int32) {
	return 12, 3, 15
}

func (imageInputMetrics) XHeightFixed(sizePx int32, _ frame.FontSlot) int32 {
	return sizePx * 32
}

func TestInlineReplacedBoxModelKeepsContentDimensions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		boxSizing  string
		width      string
		wantW      float32
		wantH      float32
		wantBorder float32
	}{
		{name: "content-box", boxSizing: "content-box", width: `width="40" height="20"`, wantW: 40, wantH: 20, wantBorder: 54},
		{name: "border-box", boxSizing: "border-box", width: `width="40" height="20"`, wantW: 26, wantH: 6, wantBorder: 40},
		{name: "border-box-width-only", boxSizing: "border-box", width: `width="40"`, wantW: 26, wantH: 13, wantBorder: 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := domtest.Parse(`<!doctype html><style>body{margin:0}img,input[type=image]{display:inline;padding:5px;border:2px solid;box-sizing:` + tc.boxSizing + `}</style><img id="img" ` + tc.width + `><input id="input" type="image" ` + tc.width + `>`)
			var sheets []*css.Stylesheet
			var walk func(*dom.Node)
			walk = func(n *dom.Node) {
				if n.Element() && n.Data == "style" {
					sheets = append(sheets, css.ParseForViewport(n.TextContent(), 800))
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
			}
			walk(&doc.Node)
			arena := layout.Build(doc, style.Resolve(doc, sheets, nil), nil)
			arena.NaturalSizes = map[dom.NodeID]layout.NaturalSize{
				doc.ElementByID("img").ID:   {W: 60, H: 30},
				doc.ElementByID("input").ID: {W: 60, H: 30},
			}
			layout.Block(arena, layout.ObjectID(1), 800, 0)
			for _, id := range []string{"img", "input"} {
				obj := findByID(arena, id)
				if obj == nil {
					t.Fatalf("#%s has no layout object", id)
				}
				if !almostEqual(obj.W, tc.wantW) || !almostEqual(obj.H, tc.wantH) {
					t.Errorf("#%s content box = %gx%g, want %gx%g", id, obj.W, obj.H, tc.wantW, tc.wantH)
				}
				_, _, x1, y1 := obj.BorderRect()
				if !almostEqual(x1-obj.X, tc.wantBorder) {
					t.Errorf("#%s border box width = %g, want %g", id, x1-obj.X, tc.wantBorder)
				}
				if tc.name != "border-box-width-only" && !almostEqual(y1-obj.Y, tc.wantH+14) {
					t.Errorf("#%s border box height = %g, want %g", id, y1-obj.Y, tc.wantH+14)
				}
			}
		})
	}
}

func TestImageInputContentSize(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><style>
		body { margin: 0; }
		input { display: block; margin: 10px; }
	</style>
	<input id="natural" type="image">
	<input id="attrs" type="image" width="40" height="20">
	<input id="widthonly" type="image" width="40">
	<input id="missing" type="image">
	<input id="alt" type="image" alt="Go">
	<input id="emptyalt" type="image" alt="">
	<input id="css" type="image" style="width:80px;height:25px">`)
	var sheets []*css.Stylesheet
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n.Element() && n.Data == "style" {
			sheets = append(sheets, css.ParseForViewport(n.TextContent(), 800))
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	styles := style.Resolve(doc, sheets, nil)
	arena := layout.Build(doc, styles, nil)
	arena.Metrics = imageInputMetrics{}
	arena.NaturalSizes = map[dom.NodeID]layout.NaturalSize{
		doc.ElementByID("natural").ID:   {W: 60, H: 30},
		doc.ElementByID("widthonly").ID: {W: 60, H: 30},
	}
	layout.Block(arena, layout.ObjectID(1), 800, 0)

	for _, tc := range []struct {
		id    string
		wantW float32
		wantH float32
	}{
		{id: "natural", wantW: 60, wantH: 30},
		{id: "attrs", wantW: 40, wantH: 20},
		{id: "widthonly", wantW: 40, wantH: 20},
		{id: "missing", wantW: 57.5, wantH: 16},
		{id: "alt", wantW: 33.796875, wantH: 16},
		{id: "emptyalt", wantW: 0, wantH: 0},
		{id: "css", wantW: 80, wantH: 25},
	} {
		t.Run(tc.id, func(t *testing.T) {
			obj := findByID(arena, tc.id)
			if obj == nil {
				t.Fatalf("#%s has no layout object", tc.id)
			}
			if math.Abs(float64(obj.W-tc.wantW)) > 1.0/64 || math.Abs(float64(obj.H-tc.wantH)) > 1.0/64 {
				t.Errorf("#%s = %gx%g, want %gx%g within 1/64", tc.id, obj.W, obj.H, tc.wantW, tc.wantH)
			}
		})
	}
}
