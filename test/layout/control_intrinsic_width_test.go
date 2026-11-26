package layout_test

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func assertControlIntrinsicWidth(t *testing.T, arena *layout.Arena, id string, want float32) {
	t.Helper()
	obj := findByID(arena, id)
	if obj == nil {
		t.Fatalf("#%s has no layout object", id)
	}
	_, _, x1, _ := obj.BorderRect()
	got := x1 - obj.X
	if math.Abs(float64(got-want)) > 1.0/64 {
		t.Errorf("border box width = %g, want %g within 1/64", got, want)
	}
}

func TestControlIntrinsicInputWidth(t *testing.T) {
	var cases strings.Builder
	cases.WriteString(`<!doctype html><style>body{margin:0}input{display:inline-block;font-family:Arial;font-weight:400;font-style:normal;letter-spacing:0;word-spacing:0}.s13\.3333{font-size:13.3333px}.s16{font-size:16px}.s20{font-size:20px}.s24{font-size:24px}.s32{font-size:32px}</style>`)
	for _, fontSize := range []float32{13.3333, 16, 20, 24, 32} {
		for _, size := range []string{"", "10", "30"} {
			id := fmt.Sprintf("i%g-%s", fontSize, size)
			if size == "" {
				id = fmt.Sprintf("i%g", fontSize)
			}
			attr := ""
			if size != "" {
				attr = ` size="` + size + `"`
			}
			fmt.Fprintf(&cases, `<input id="%s" class="s%s" type="text"%s>`, id, strconv.FormatFloat(float64(fontSize), 'f', -1, 32), attr)
		}
	}
	arena := controlMetricsSession(t, cases.String(), 800)
	for _, c := range []struct {
		fontSize float32
		widths   [3]float32
	}{
		{13.3333, [3]float32{153, 83, 223}},
		{16, [3]float32{174, 94, 254}},
		{20, [3]float32{216, 116, 316}},
		{24, [3]float32{258, 138, 378}},
		{32, [3]float32{341, 181, 501}},
	} {
		for i, size := range []string{"", "-10", "-30"} {
			id := fmt.Sprintf("i%g%s", c.fontSize, size)
			t.Run(id, func(t *testing.T) {
				assertControlIntrinsicWidth(t, arena, id, c.widths[i])
			})
		}
	}
}

func TestControlIntrinsicInputSizeAttributeFallback(t *testing.T) {
	arena := controlMetricsSession(t, `<!doctype html><style>body{margin:0}input{display:inline-block;font-family:Arial;font-size:16px}</style>
<input id="default" type="text"><input id="ten" type="text" size="10"><input id="invalid" type="text" size="bogus"><input id="zero" type="text" size="0"><input id="trailing" type="text" size="10x">`, 800)
	assertControlIntrinsicWidth(t, arena, "default", 174)
	assertControlIntrinsicWidth(t, arena, "ten", 94)
	for _, id := range []string{"invalid", "zero"} {
		t.Run(id, func(t *testing.T) {
			assertControlIntrinsicWidth(t, arena, id, 174)
		})
	}
	t.Run("trailing", func(t *testing.T) {
		assertControlIntrinsicWidth(t, arena, "trailing", 94)
	})
}

func TestControlIntrinsicWidthPreservesAuthorBox(t *testing.T) {
	arena := controlMetricsSession(t, `<!doctype html><style>body{margin:0}input{display:inline-block;font-family:Arial;font-size:16px}.content{width:100px;padding:10px;border:3px solid;box-sizing:content-box}.border{width:100px;padding:10px;border:3px solid;box-sizing:border-box}</style><input id="content" class="content" type="text" size="30"><input id="border" class="border" type="text" size="30">`, 800)
	assertControlIntrinsicWidth(t, arena, "content", 126)
	assertControlIntrinsicWidth(t, arena, "border", 100)
	if got := findByID(arena, "content").W; got != 100 {
		t.Errorf("content-box input content width = %g, want 100", got)
	}
	if got := findByID(arena, "border").W; got != 74 {
		t.Errorf("border-box input content width = %g, want 74", got)
	}
}

func TestControlIntrinsicTextareaWidth(t *testing.T) {
	var cases strings.Builder
	cases.WriteString(`<!doctype html><style>body{margin:0}textarea{display:inline-block;font-family:Arial;font-size:13.3333px}.s16{font-size:16px}.s20{font-size:20px}.s24{font-size:24px}.s32{font-size:32px}</style>`)
	for _, fontSize := range []string{"13.3333", "16", "20", "24", "32"} {
		fmt.Fprintf(&cases, `<textarea id="t%s" class="s%s" cols="20"></textarea>`, fontSize, fontSize)
	}
	arena := controlMetricsSession(t, cases.String(), 800)
	for _, c := range []struct {
		id    string
		width float32
	}{
		{"t13.3333", 162},
		{"t16", 182},
		{"t20", 222},
		{"t24", 262},
		{"t32", 342},
	} {
		t.Run(c.id, func(t *testing.T) {
			assertControlIntrinsicWidth(t, arena, c.id, c.width)
		})
	}
}

func TestControlIntrinsicButtonLabelWidth(t *testing.T) {
	var cases strings.Builder
	cases.WriteString(`<!doctype html><style>body{margin:0}input{display:inline-block;font-family:Arial;letter-spacing:0;word-spacing:0}.s13\.3333{font-size:13.3333px}.s16{font-size:16px}.s20{font-size:20px}.s24{font-size:24px}.s32{font-size:32px}</style>`)
	for _, fontSize := range []string{"13.3333", "16", "20", "24", "32"} {
		fmt.Fprintf(&cases, `<input id="b%s" class="s%s" type="button" value="Go"><input id="s%s" class="s%s" type="submit" value="Go"><input id="r%s" class="s%s" type="reset" value="Go">`, fontSize, fontSize, fontSize, fontSize, fontSize, fontSize)
	}
	arena := controlMetricsSession(t, cases.String(), 800)
	for _, c := range []struct {
		fontSize string
		width    float32
	}{
		{"13.3333", 33.796875},
		{"16", 37.34375},
		{"20", 42.6875},
		{"24", 48.015625},
		{"32", 58.6875},
	} {
		for _, prefix := range []string{"b", "s", "r"} {
			id := prefix + c.fontSize
			t.Run(id, func(t *testing.T) {
				assertControlIntrinsicWidth(t, arena, id, c.width)
			})
		}
	}
}

func TestControlIntrinsicClosedSelectWidth(t *testing.T) {
	arena := controlMetricsSession(t, `<!doctype html><style>body{margin:0}select{display:inline-block;font-family:Arial;letter-spacing:0;word-spacing:0}.s16{font-size:16px}.s24{font-size:24px}</style>
<select id="short"><option>Hi</option></select><select id="short16" class="s16"><option>Hi</option></select><select id="short24" class="s24"><option>Hi</option></select><select id="long"><option>A much longer option</option></select><select id="longest"><option>Hi</option><option>A much longer option</option></select><select id="attribute"><option label="A much longer option">Hi</option></select>`, 800)
	for _, c := range []struct {
		id    string
		width float32
	}{
		{"short", 35},
		{"short16", 38},
		{"short24", 45},
		{"long", 148},
		{"longest", 148},
		{"attribute", 148},
	} {
		t.Run(c.id, func(t *testing.T) {
			assertControlIntrinsicWidth(t, arena, c.id, c.width)
		})
	}
}

func TestControlIntrinsicTextareaWidthWithoutMetrics(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><style>body{margin:0}textarea{display:inline-block;font-family:Arial;font-size:16px}</style><textarea id="plain" cols="20"></textarea>`)
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
	layout.Block(arena, layout.ObjectID(1), 800, 0)
	assertControlIntrinsicWidth(t, arena, "plain", 182)
}
