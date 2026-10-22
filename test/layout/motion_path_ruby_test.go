package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestMotionPathTranslation(t *testing.T) {
	// Ray 90deg points right (dx = distance, dy = 0)
	arena := positionedSession(t, `<html><body style="margin: 0;">
<div id="target" style="width: 50px; height: 50px; offset-path: ray(90deg); offset-distance: 60px;">Box</div>
</body></html>`, 800)

	var target *layout.Object
	for i := range arena.Objects {
		o := &arena.Objects[i]
		if o.Node != nil && o.Node.Element() && o.Node.GetAttribute("id") == "target" {
			target = o
			break
		}
	}

	if target == nil {
		t.Fatal("target object not found")
	}
	if target.X != 60 {
		t.Errorf("expected target.X to be 60 after ray(90deg) 60px offset, got %v", target.X)
	}

	// Path translation: M 0 0 L 100 0 with 40px distance
	arena2 := positionedSession(t, `<html><body style="margin: 0;">
<div id="target2" style="width: 50px; height: 50px; offset-path: path('M 0 0 L 100 0'); offset-distance: 40px;">Box2</div>
</body></html>`, 800)

	var target2 *layout.Object
	for i := range arena2.Objects {
		o := &arena2.Objects[i]
		if o.Node != nil && o.Node.Element() && o.Node.GetAttribute("id") == "target2" {
			target2 = o
			break
		}
	}

	if target2 == nil {
		t.Fatal("target2 object not found")
	}
	if target2.X != 40 {
		t.Errorf("expected target2.X to be 40 after path 40px offset, got %v", target2.X)
	}
}

func TestRubyAnnotationLayout(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0; font-size: 20px; line-height: 20px;">
<div>
  <ruby id="ruby-el">漢<rp>(</rp><rt id="rt-el">かん</rt><rp>)</rp></ruby>
</div>
</body></html>`, 800)

	var baseText, rtText, rpText *layout.Object
	for i := range arena.Objects {
		o := &arena.Objects[i]
		if o.Node != nil && o.Node.Text() {
			if o.Node.DataContent == "漢" {
				baseText = o
			} else if o.Node.DataContent == "かん" {
				rtText = o
			} else if o.Node.DataContent == "(" || o.Node.DataContent == ")" {
				rpText = o
			}
		}
	}

	if baseText == nil || rtText == nil {
		t.Fatalf("baseText=%v, rtText=%v not found", baseText, rtText)
	}

	var rpEl *layout.Object
	for i := range arena.Objects {
		o := &arena.Objects[i]
		if o.Node != nil && o.Node.Element() && o.Node.Data == "rp" {
			rpEl = o
		}
	}

	if rpEl == nil || rpEl.Style == nil || rpEl.Style.Display != style.DisplayNone {
		t.Errorf("<rp> element should have display: none, got %v", rpEl)
	}
	if rpText != nil && rpText.W > 0 {
		t.Errorf("<rp> parenthesis text should not be laid out, got W=%v", rpText.W)
	}

	// Ruby annotation text should sit above the base text (rtText.Y < baseText.Y)
	if rtText.Y >= baseText.Y {
		t.Errorf("expected ruby rtText.Y (%v) to be above baseText.Y (%v)", rtText.Y, baseText.Y)
	}
}
