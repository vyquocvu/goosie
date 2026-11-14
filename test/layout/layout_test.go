package layout_test

import (
	"math"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func almostEqual(a, b float32) bool {
	return math.Abs(float64(a-b)) < 0.01
}

func session(t *testing.T, html string, viewportW float32) *layout.Arena {
	t.Helper()
	return sessionH(t, html, viewportW, 0)
}

func sessionH(t *testing.T, html string, viewportW, viewportH float32) *layout.Arena {
	t.Helper()
	doc := domtest.Parse(html)
	styles := style.Resolve(doc, nil, nil)
	arena := layout.Build(doc, styles, nil)
	layout.Block(arena, layout.ObjectID(1), viewportW, viewportH)
	return arena
}

func findByTag(arena *layout.Arena, tag string) *layout.Object {
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node != nil && obj.Node.Data == tag && obj.Style != nil {
			return obj
		}
	}
	return nil
}

func findAllByTag(arena *layout.Arena, tag string) []*layout.Object {
	var result []*layout.Object
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node != nil && obj.Node.Data == tag && obj.Style != nil {
			result = append(result, obj)
		}
	}
	return result
}

func TestBlockLayout(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0; padding: 10px;">
<div style="width: 200px; height: 100px;"></div>
</body></html>`, 800)

	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	if div.W != 200 {
		t.Errorf("div.W = %v, want 200", div.W)
	}
	if div.H != 100 {
		t.Errorf("div.H = %v, want 100", div.H)
	}
	if div.X != 10 {
		t.Errorf("div.X = %v, want 10", div.X)
	}
	if div.Y != 10 {
		t.Errorf("div.Y = %v, want 10", div.Y)
	}
}

func TestAutoWidth(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0; padding: 10px;">
<div style="height: 50px;"></div>
</body></html>`, 800)

	body := findByTag(arena, "body")
	if body == nil {
		t.Fatal("body not found")
	}
	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	if div.W != 780 {
		t.Errorf("div.W = %v, want 780 (auto width in 800px viewport with 10px padding)", div.W)
	}
}

func TestAutoHeight(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0; padding: 10px;">
<div style="height: 100px;"></div>
</body></html>`, 800)

	body := findByTag(arena, "body")
	if body == nil {
		t.Fatal("body not found")
	}
	if body.H != 100 {
		t.Errorf("body.H = %v, want 100 (auto content height from children)", body.H)
	}
}

func TestNestedPositioning(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="padding: 10px;">
  <div style="padding: 5px;">
    <div style="width: 50px; height: 20px;"></div>
  </div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) < 3 {
		t.Fatalf("found %d divs, want 3", len(divs))
	}
	inner := divs[2]

	if inner.X != 15 {
		t.Errorf("inner.X = %v, want 15", inner.X)
	}
	if inner.Y != 15 {
		t.Errorf("inner.Y = %v, want 15", inner.Y)
	}
}

func TestMarginCollapsing(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="margin: 20px; height: 50px;"></div>
<div style="margin: 30px; height: 50px;"></div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) < 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	div1, div2 := divs[0], divs[1]

	if div1.Y != 20 {
		t.Errorf("div1.Y = %v, want 20", div1.Y)
	}

	if div2.Y != 100 {
		t.Errorf("div2.Y = %v, want 100 (margin collapsing)", div2.Y)
	}
}

func TestMarginCollapsingThroughAWrapper(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<p style="margin: 0 0 16px; height: 18px">one</p>
<section>
<h3 style="margin: 20px 0; height: 21px">two</h3>
<p style="margin: 16px 0; height: 18px">three</p>
</section>
<p style="margin: 0 0 40px; height: 10px">four</p>
</body></html>`, 800)

	ps := findAllByTag(arena, "p")
	if len(ps) != 3 {
		t.Fatalf("found %d paragraphs, want 3", len(ps))
	}
	section := findByTag(arena, "section")
	h3 := findByTag(arena, "h3")
	if section == nil || h3 == nil {
		t.Fatal("section or h3 not found")
	}

	if want := float32(38); !almostEqual(h3.Y, want) {
		t.Errorf("h3.Y = %v, want %v", h3.Y, want)
	}
	if want := float32(38); !almostEqual(section.Y, want) {
		t.Errorf("section.Y = %v, want %v", section.Y, want)
	}
	if want := float32(59); !almostEqual(section.H, want) {
		t.Errorf("section.H = %v, want %v", section.H, want)
	}
	if want := float32(113); !almostEqual(ps[2].Y, want) {
		t.Errorf("last p.Y = %v, want %v", ps[2].Y, want)
	}
}

func TestBlockInInline(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div>
<p style="margin: 0; height: 18px">first</p>
<span><p style="margin: 0; height: 18px">nested</p></span>
</div>
</body></html>`, 800)
	layout.Inline(arena, layout.ObjectID(1))

	ps := findAllByTag(arena, "p")
	if len(ps) != 2 {
		t.Fatalf("found %d paragraphs, want 2", len(ps))
	}
	var nested *layout.Object
	for _, p := range ps {
		if p.Y > 0 {
			nested = p
		}
	}
	if nested == nil {
		t.Fatal("the paragraph inside the span was never placed below the first one")
	}
	if want := float32(800); !almostEqual(nested.W, want) {
		t.Errorf("nested p.W = %v, want %v", nested.W, want)
	}
	if want := float32(18); !almostEqual(nested.Y, want) {
		t.Errorf("nested p.Y = %v, want %v", nested.Y, want)
	}
	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	if want := float32(36); !almostEqual(div.H, want) {
		t.Errorf("div.H = %v, want %v", div.H, want)
	}
}

func TestFixedPositioning(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="position: fixed; top: 0; left: 0; right: 0; height: 40px">bar</div>
<p>in flow</p>
<div style="position: fixed; bottom: 0; left: 0; width: 100px">foot</div>
</body></html>`, 800)
	layout.Inline(arena, layout.ObjectID(1))
	layout.Positioning(arena, layout.ObjectID(1), 800, 600)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	bar, foot := divs[0], divs[1]
	if bar.X != 0 || bar.Y != 0 {
		t.Errorf("bar origin = %v,%v, want 0,0", bar.X, bar.Y)
	}
	if want := float32(800); !almostEqual(bar.W, want) {
		t.Errorf("bar.W = %v, want %v", bar.W, want)
	}
	if want := float32(40); !almostEqual(bar.H, want) {
		t.Errorf("bar.H = %v, want %v", bar.H, want)
	}
	p := findByTag(arena, "p")
	if p == nil {
		t.Fatal("p not found")
	}
	if want := float32(16); !almostEqual(p.Y, want) {
		t.Errorf("p.Y = %v, want %v", p.Y, want)
	}
	if want := float32(100); !almostEqual(foot.W, want) {
		t.Errorf("foot.W = %v, want %v", foot.W, want)
	}
	if want := float32(600); !almostEqual(foot.Y+foot.H, want) {
		t.Errorf("foot bottom edge = %v, want %v", foot.Y+foot.H, want)
	}
}

func collectArenaTexts(a *layout.Arena, id layout.ObjectID, out *[]string) {
	for kid := id; kid != 0; kid = a.Get(kid).NextSibling {
		obj := a.Get(kid)
		if obj.Node != nil && obj.Node.Type == dom.NodeText {
			*out = append(*out, obj.Node.DataContent)
		}
		if obj.FirstKid != 0 {
			collectArenaTexts(a, obj.FirstKid, out)
		}
	}
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestFlexRow(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="display: flex; height: 40px;">
  <div style="flex: 1; padding: 0 8px;"></div>
  <div style="flex: 1; padding: 0 8px;"></div>
  <div style="flex: 1; padding: 0 8px;"></div>
</div>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 4 {
		t.Fatalf("found %d divs, want 4 (row + 3 cells)", len(divs))
	}
	row, c1, c2, c3 := divs[0], divs[1], divs[2], divs[3]

	share := float32(800) / 3
	if c1.X != 0 {
		t.Errorf("c1.X = %v, want 0", c1.X)
	}
	if !almostEqual(c2.X, share) {
		t.Errorf("c2.X = %v, want %v", c2.X, share)
	}
	if !almostEqual(c3.X, 2*share) {
		t.Errorf("c3.X = %v, want %v", c3.X, 2*share)
	}
	if !almostEqual(c1.W, share-16) {
		t.Errorf("c1.W = %v, want %v", c1.W, share-16)
	}
	if c1.Y != 0 || c2.Y != 0 || c3.Y != 0 {
		t.Errorf("cells Y = %v %v %v, want 0 0 0", c1.Y, c2.Y, c3.Y)
	}
	if c1.H != 40 || c2.H != 40 || c3.H != 40 {
		t.Errorf("cells H = %v %v %v, want 40 (stretched)", c1.H, c2.H, c3.H)
	}
	if row.H != 40 {
		t.Errorf("row.H = %v, want 40 (explicit height)", row.H)
	}
}

func TestFlexRowItemWithBlockChild(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<ul style="display: flex; justify-content: space-between; width: 600px; margin: 0; padding: 0; list-style: none;">
  <li><span style="display: block;">Alpha</span></li>
  <li><span style="display: block;">Beta</span></li>
  <li><span style="display: block;">Gamma</span></li>
</ul>
</body></html>`, 800)

	lis := findAllByTag(arena, "li")
	if len(lis) != 3 {
		t.Fatalf("found %d li, want 3", len(lis))
	}
	first, mid, last := lis[0], lis[1], lis[2]
	for i, it := range []*layout.Object{first, mid, last} {
		if it.W <= 0 {
			t.Errorf("li[%d].W = %v, want > 0 (block content must size the item)", i, it.W)
		}
	}
	if !(first.X < mid.X && mid.X < last.X) {
		t.Errorf("items not spread: X = %v %v %v", first.X, mid.X, last.X)
	}
	if right := last.X + last.W; right < 560 {
		t.Errorf("last item right edge = %v, want near 600 (space-between)", right)
	}
}

func findWordObject(a *layout.Arena, word string) *layout.Object {
	for i := range a.Objects {
		obj := &a.Objects[i]
		if obj.Node != nil && obj.Node.Type == dom.NodeText && obj.Node.DataContent == word {
			return obj
		}
	}
	return nil
}

func TestLineHeightCentersText(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="line-height: 40px; margin: 0;">hello world</div>
<div style="margin: 0;">plain</div>
</body></html>`, 800)
	layout.Inline(arena, layout.ObjectID(1))

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	hello := findWordObject(arena, "hello")
	plain := findWordObject(arena, "plain")
	if hello == nil || plain == nil {
		t.Fatal("word objects not found")
	}

	wantY := (float32(40) - 16) / 2
	if !almostEqual(hello.Y, wantY) {
		t.Errorf("hello.Y = %v, want %v (centered in the 40px line)", hello.Y, wantY)
	}
	if !almostEqual(divs[0].H, 40) {
		t.Errorf("div.H = %v, want 40 (one line of the declared line-height)", divs[0].H)
	}
	if !almostEqual(plain.Y, 40+(19.2-16)/2) {
		t.Errorf("plain.Y = %v, want %v (half-leading below the 40px first div)", plain.Y, 40+(19.2-16)/2)
	}
	if !almostEqual(divs[1].H, 16*1.2) {
		t.Errorf("second div.H = %v, want %v (default 1.2 line-height)", divs[1].H, 16*1.2)
	}
}

func TestInlineWordRelink(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="margin: 0;">alpha beta <b>gamma</b> delta</div>
</body></html>`, 800)
	layout.Inline(arena, layout.ObjectID(1))

	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	var texts []string
	collectArenaTexts(arena, div.FirstKid, &texts)

	for _, want := range []string{"alpha", "beta", "gamma", "delta"} {
		if !containsStr(texts, want) {
			t.Errorf("word %q not reachable from div kid chain; got %v", want, texts)
		}
	}
	for _, got := range texts {
		if got == "alpha beta " || got == " delta" {
			t.Errorf("original text node %q still in the chain", got)
		}
	}
}

func TestInlineWordRelinkLarge(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 40; i++ {
		sb.WriteString("<b>x</b> lorem ipsum ")
	}
	arena := session(t, `<html><body style="margin: 0;">
<div style="margin: 0;">`+sb.String()+`</div>
</body></html>`, 800)
	layout.Inline(arena, layout.ObjectID(1))

	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	var texts []string
	collectArenaTexts(arena, div.FirstKid, &texts)

	lorem, ipsum := 0, 0
	for _, s := range texts {
		switch s {
		case "lorem":
			lorem++
		case "ipsum":
			ipsum++
		}
	}
	if lorem != 40 || ipsum != 40 {
		t.Errorf("reachable words lorem=%d ipsum=%d, want 40 each (got %d texts)", lorem, ipsum, len(texts))
	}
}

func TestMarginAutoCentering(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 600px; margin: 0 auto;"></div>
</body></html>`, 1000)

	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	if !almostEqual(div.MarginLeft, 200) {
		t.Errorf("div.MarginLeft = %v, want 200", div.MarginLeft)
	}
	if !almostEqual(div.MarginRight, 200) {
		t.Errorf("div.MarginRight = %v, want 200", div.MarginRight)
	}
	if !almostEqual(div.X, 200) {
		t.Errorf("div.X = %v, want 200", div.X)
	}
}

func TestMarginAutoCenteringAfterMaxWidthClamp(t *testing.T) {
	for _, tc := range []struct {
		name  string
		style string
	}{
		{"auto width", `max-width: 600px; margin: 0 auto`},
		{"width past the cap", `width: 100%; max-width: 600px; margin: 0 auto`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arena := session(t, `<html><body style="margin: 0;">
<div style="`+tc.style+`;"></div>
</body></html>`, 1000)
			div := findByTag(arena, "div")
			if div == nil {
				t.Fatal("div not found")
			}
			if !almostEqual(div.W, 600) {
				t.Errorf("div.W = %v, want 600", div.W)
			}
			if !almostEqual(div.MarginLeft, 200) || !almostEqual(div.MarginRight, 200) {
				t.Errorf("div margins = %v/%v, want 200/200", div.MarginLeft, div.MarginRight)
			}
			if !almostEqual(div.X, 200) {
				t.Errorf("div.X = %v, want 200", div.X)
			}
		})
	}
}

func TestMarginAutoWithRoomToSpareStaysAtTheStart(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="margin: 0 auto;"></div>
</body></html>`, 1000)
	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	if !almostEqual(div.W, 1000) {
		t.Errorf("div.W = %v, want 1000", div.W)
	}
	if div.MarginLeft != 0 || div.MarginRight != 0 {
		t.Errorf("div margins = %v/%v, want 0/0", div.MarginLeft, div.MarginRight)
	}
}

func TestDetailsClosedContentKeepsHiddenLayoutBoxes(t *testing.T) {
	hidden := session(t, `<html><body style="margin:0">
<details><summary>more</summary><ul><li>a</li></ul></details>
</body></html>`, 400)
	ul := findByTag(hidden, "ul")
	if ul == nil {
		t.Fatal("ul not found")
	}
	if ul.Style.Display == style.DisplayNone || ul.Style.ContentVisibility != "hidden" {
		t.Errorf("closed details: ul style = display:%v content-visibility:%q, want a laid-out hidden box", ul.Style.Display, ul.Style.ContentVisibility)
	}

	opened := session(t, `<html><body style="margin:0">
<details open><summary>more</summary><ul><li>a</li></ul></details>
</body></html>`, 400)
	if ul := findByTag(opened, "ul"); ul.Style.Display == style.DisplayNone || ul.Style.ContentVisibility == "hidden" {
		t.Error("an open details still hides its content")
	}
}

func TestInlineBlockOnALineShrinksToContent(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div style="font-size: 16px">title<span style="display: inline-block"><b>one</b> <b>two</b></span>tail</div>
</body></html>`, 400)
	span := findByTag(arena, "span")
	if span == nil {
		t.Fatal("span not found")
	}
	if span.W >= 400 {
		t.Errorf("span.W = %v, want the content extent, not the 400px line", span.W)
	}
	if span.W < 20 || span.W > 120 {
		t.Errorf("span.W = %v, want roughly the width of `one two`", span.W)
	}
}

func TestTextAlignCenter(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<div style="width: 400px; text-align: center;">word</div>
</body></html>`, 800)
	layout.Inline(arena, layout.ObjectID(1))

	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	wordObj := arena.Get(div.FirstKid)
	if wordObj == nil || wordObj.Node == nil {
		t.Fatal("word obj not found")
	}
	wantX := (400 - wordObj.W) / 2
	if !almostEqual(wordObj.X, wantX) {
		t.Errorf("wordObj.X = %v, want %v", wordObj.X, wantX)
	}
}

func TestPseudoElementLayout(t *testing.T) {
	html := `<html><body><div class="test">hello</div></body></html>`
	cssText := `.test::before { content: "\25b3"; color: red; }`

	doc := domtest.Parse(html)
	sheets := []*css.Stylesheet{css.Parse(cssText)}
	styles := style.Resolve(doc, sheets, nil)
	pseudoStyles := style.ResolvePseudoElements(doc, sheets, styles, nil)

	t.Logf("pseudoStyles count: %d", len(pseudoStyles))
	for k, v := range pseudoStyles {
		t.Logf("  key=%v content=%q", k, v.Content)
	}

	arena := layout.Build(doc, styles, pseudoStyles)

	layout.Block(arena, layout.ObjectID(1), 800, 600)
	layout.Inline(arena, layout.ObjectID(1))

	t.Logf("arena has %d objects", len(arena.Objects))
	for i, obj := range arena.Objects {
		if obj.Node != nil {
			t.Logf("  obj[%d]: node=%v type=%d data=%q content=%q", i, obj.Node.ID, obj.Node.Type, obj.Node.Data, obj.Node.DataContent)
		} else {
			t.Logf("  obj[%d]: no node", i)
		}
	}

	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}

	t.Logf("div obj: FirstKid=%v LastKid=%v", div.FirstKid, div.LastKid)

	for kid := div.FirstKid; kid != 0; kid = arena.Get(kid).NextSibling {
		k := arena.Get(kid)
		t.Logf("  child obj[%d]: node=%v type=%d content=%q X=%v Y=%v W=%v H=%v", kid, k.Node.ID, k.Node.Type, k.Node.DataContent, k.X, k.Y, k.W, k.H)
	}

	if div.FirstKid == 0 {
		t.Fatal("div has no children, pseudo-element not injected")
	}

	firstKid := arena.Get(div.FirstKid)
	if firstKid.Node == nil || firstKid.Node.Type != 2 {
		t.Fatalf("first child is not a text node: type=%d", firstKid.Node.Type)
	}
	if firstKid.Node.DataContent != "△" {
		t.Errorf("pseudo-element content = %q, want △", firstKid.Node.DataContent)
	}

	t.Logf("pseudo-element obj: X=%v Y=%v W=%v H=%v", firstKid.X, firstKid.Y, firstKid.W, firstKid.H)
}
