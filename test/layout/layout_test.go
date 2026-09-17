package layout_test

import (
	"math"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func almostEqual(a, b float32) bool {
	return math.Abs(float64(a-b)) < 0.01
}

func session(t *testing.T, html string, viewportW float32) *layout.Arena {
	t.Helper()
	doc := dom.Parse(html)
	styles := style.Resolve(doc, nil)
	arena := layout.Build(doc, styles)
	layout.Block(arena, layout.ObjectID(1), viewportW)
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
	// Div should be positioned inside body's padding
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
	// Div auto width should fill body's content area: 800 - 10 - 10 = 780
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
	// Body auto height should be: padding-top (10) + child height (100) + padding-bottom (10) = 120
	if body.H != 120 {
		t.Errorf("body.H = %v, want 120 (auto height from children + padding)", body.H)
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

	// The innermost div must sit at the accumulated content origin of its
	// ancestors: outer padding 10 + middle padding 5.
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

	// First div: margin-top 20px, positioned inside body (no padding)
	if div1.Y != 20 {
		t.Errorf("div1.Y = %v, want 20", div1.Y)
	}

	// Second div: margins collapse between siblings.
	// div1 bottom margin = 20, div2 top margin = 30, collapsed = max(20, 30) = 30
	// div2.Y = div1.Y + div1.H + collapsed margin = 20 + 50 + 30 = 100
	if div2.Y != 100 {
		t.Errorf("div2.Y = %v, want 100 (margin collapsing)", div2.Y)
	}
}

// collectArenaTexts walks the arena kid chain from id and returns every text
// payload still reachable through FirstKid/NextSibling links. Paint walks the
// same links, so this is exactly what the display list will contain.
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
	// The gate fixture's row shape: a fixed-height flex container with three
	// equal cells padded 0 8px. Cells must sit side by side, one third of the
	// row each, and stretch to the row's content height.
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
	// Content width is the share minus the 8px padding on each side.
	if !almostEqual(c1.W, share-16) {
		t.Errorf("c1.W = %v, want %v", c1.W, share-16)
	}
	// All cells start at the row's content top.
	if c1.Y != 0 || c2.Y != 0 || c3.Y != 0 {
		t.Errorf("cells Y = %v %v %v, want 0 0 0", c1.Y, c2.Y, c3.Y)
	}
	// align-items: stretch — auto-height cells fill the row's content height.
	if c1.H != 40 || c2.H != 40 || c3.H != 40 {
		t.Errorf("cells H = %v %v %v, want 40 (stretched)", c1.H, c2.H, c3.H)
	}
	if row.H != 40 {
		t.Errorf("row.H = %v, want 40 (explicit height)", row.H)
	}
}

// findWordObject returns the inline-pass word object whose text payload is
// word, or nil. Word objects exist only after the inline pass splits a text
// node, and paint walks exactly these objects to emit text commands.
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

	// Half-leading: the 19.2px glyph box centers in the 40px line box, so the
	// word's top sits (40 - 19.2) / 2 below the line's top.
	wantY := (float32(40) - 16*1.2) / 2
	if !almostEqual(hello.Y, wantY) {
		t.Errorf("hello.Y = %v, want %v (centered in the 40px line)", hello.Y, wantY)
	}
	if !almostEqual(divs[0].H, 40) {
		t.Errorf("div.H = %v, want 40 (one line of the declared line-height)", divs[0].H)
	}
	// Without a declaration the 1.2 multiplier still applies.
	if plain.Y != 0 {
		t.Errorf("plain.Y = %v, want 0 (default line box starts at the top)", plain.Y)
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

	// Every word of every text node must remain reachable after the inline
	// pass splits text nodes into word objects, including words that come
	// after an inline element sibling.
	for _, want := range []string{"alpha", "beta", "gamma", "delta"} {
		if !containsStr(texts, want) {
			t.Errorf("word %q not reachable from div kid chain; got %v", want, texts)
		}
	}
	// The original text nodes must have been replaced by words, not kept.
	for _, got := range texts {
		if got == "alpha beta " || got == " delta" {
			t.Errorf("original text node %q still in the chain", got)
		}
	}
}

func TestInlineWordRelinkLarge(t *testing.T) {
	// Many text nodes spread over enough words that inlineInto's Alloc calls
	// force the arena slice to grow partway through, which invalidates any
	// object pointer held across an Alloc.
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

