package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

func layoutObjectWithID(arena *layout.Arena, id string) *layout.Object {
	for i := range arena.Objects {
		obj := &arena.Objects[i]
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			return obj
		}
	}
	return nil
}

func TestInlineBlockMeasuresReplacedChild(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<span id="first" style="display:inline-block;margin:3px 0 4px 4px"><span style="display:block"><input id="firstButton" type="submit" value="Search"></span></span>
<span id="second" style="display:inline-block;margin:3px 0 4px 4px"><span style="display:block"><input id="secondButton" type="submit" value="Lucky"></span></span>
</body></html>`, 800)

	first := layoutObjectWithID(arena, "first")
	second := layoutObjectWithID(arena, "second")
	firstButton := layoutObjectWithID(arena, "firstButton")
	secondButton := layoutObjectWithID(arena, "secondButton")
	if first == nil || second == nil || firstButton == nil || secondButton == nil {
		t.Fatalf("missing layout boxes: first=%v second=%v firstButton=%v secondButton=%v",
			first != nil, second != nil, firstButton != nil, secondButton != nil)
	}
	firstButtonOuter := firstButton.W + firstButton.PaddingLeft + firstButton.PaddingRight + firstButton.BorderLeft + firstButton.BorderRight
	secondButtonOuter := secondButton.W + secondButton.PaddingLeft + secondButton.PaddingRight + secondButton.BorderLeft + secondButton.BorderRight
	if first.W+0.01 < firstButtonOuter || second.W+0.01 < secondButtonOuter {
		t.Errorf("wrapper widths = %v/%v, want at least button outer widths %v/%v",
			first.W, second.W, firstButtonOuter, secondButtonOuter)
	}
	if !almostEqual(first.Y, second.Y) {
		t.Errorf("button wrappers wrapped: first Y=%v second Y=%v", first.Y, second.Y)
	}
	if !almostEqual(firstButton.X, first.X) || !almostEqual(secondButton.X, second.X) {
		t.Errorf("buttons escaped their wrappers: first %v/%v second %v/%v",
			firstButton.X, first.X, secondButton.X, second.X)
	}
	if second.X+0.01 < first.X+first.W+first.MarginLeft+first.MarginRight {
		t.Errorf("second wrapper X=%v did not clear first wrapper", second.X)
	}
}

func TestAtomicInlineBlockRelayoutsBlockChildrenAfterShrink(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">prefix <span id="wrapper" style="display:inline-block"><span id="block" style="display:block"><input id="button" type="submit" value="Search"></span></span></body></html>`, 800)

	wrapper := layoutObjectWithID(arena, "wrapper")
	block := layoutObjectWithID(arena, "block")
	button := layoutObjectWithID(arena, "button")
	if wrapper == nil || block == nil || button == nil {
		t.Fatalf("missing layout boxes: wrapper=%v block=%v button=%v", wrapper != nil, block != nil, button != nil)
	}
	if block.W > wrapper.W+0.01 {
		t.Errorf("block child W = %v, want no wider than shrunk wrapper %v", block.W, wrapper.W)
	}
	if !almostEqual(button.X, wrapper.X) {
		t.Errorf("button X = %v, want wrapper X %v after shrink", button.X, wrapper.X)
	}
}

func TestFlexItemMeasuresAllInlineBlockChildren(t *testing.T) {
	arena := session(t, `<html><body style="margin:0;font:13px Arial">
<div id="bar" style="display:flex;justify-content:flex-end;align-items:center;float:right;height:48px;padding-left:32px">
  <div id="links" style="flex:0 1 auto;line-height:normal;padding-right:15px">
    <div id="first" style="display:inline-block;padding-left:15px"><a>Gmail</a></div>
    <div id="second" style="display:inline-block;padding-left:15px"><a>Hình ảnh</a></div>
  </div>
  <div id="account" style="position:relative;float:right">
    <div style="display:inline"><div id="apps" style="display:inline-block;vertical-align:middle;width:40px;height:40px;padding:4px"><div style="position:relative"><a style="display:inline-block;box-sizing:border-box;width:40px;height:40px;padding:8px"></a></div></div></div>
    <a id="signin" style="display:inline-block;box-sizing:border-box;min-width:85px;height:40px;padding:10px 12px;margin:12px 16px 12px 10px">Sign in</a>
  </div>
</div>
</body></html>`, 800)

	bar := layoutObjectWithID(arena, "bar")
	links := layoutObjectWithID(arena, "links")
	first := layoutObjectWithID(arena, "first")
	second := layoutObjectWithID(arena, "second")
	account := layoutObjectWithID(arena, "account")
	apps := layoutObjectWithID(arena, "apps")
	signin := layoutObjectWithID(arena, "signin")
	if bar == nil || links == nil || first == nil || second == nil || account == nil || apps == nil || signin == nil {
		t.Fatalf("missing layout boxes: bar=%v links=%v first=%v second=%v account=%v apps=%v signin=%v",
			bar != nil, links != nil, first != nil, second != nil, account != nil, apps != nil, signin != nil)
	}
	firstOuter := first.W + first.PaddingLeft + first.PaddingRight + first.BorderLeft + first.BorderRight
	secondOuter := second.W + second.PaddingLeft + second.PaddingRight + second.BorderLeft + second.BorderRight
	if links.W+0.01 < firstOuter+secondOuter {
		t.Errorf("links.W = %v, want at least %v for both inline-block children", links.W, firstOuter+secondOuter)
	}
	if !almostEqual(first.Y, second.Y) {
		t.Errorf("inline-block links wrapped: first Y=%v second Y=%v", first.Y, second.Y)
	}
	if second.X+0.01 < first.X+firstOuter {
		t.Errorf("second link X=%v did not clear first outer edge %v", second.X, first.X+firstOuter)
	}
	linksOuter := links.W + links.PaddingLeft + links.PaddingRight + links.BorderLeft + links.BorderRight
	appsOuter := apps.W + apps.PaddingLeft + apps.PaddingRight + apps.BorderLeft + apps.BorderRight
	signinOuter := signin.W + signin.PaddingLeft + signin.PaddingRight + signin.BorderLeft + signin.BorderRight + signin.MarginLeft + signin.MarginRight
	if account.W+0.01 < appsOuter+signinOuter {
		t.Errorf("account content W = %v, want at least %v for its atomic inline children", account.W, appsOuter+signinOuter)
	}
	if apps.X < account.X || signin.X < apps.X+appsOuter {
		t.Errorf("account children escaped: account X=%v apps X=%v signin X=%v apps outer=%v", account.X, apps.X, signin.X, appsOuter)
	}
	if !almostEqual(account.H, 64) {
		t.Errorf("account.H = %v, want 64 (48px apps plus the sign-in control's 12px margins)", account.H)
	}
	if !almostEqual(account.Y, -8) {
		t.Errorf("account.Y = %v, want -8 (the 64px item centers in the 48px flex line)", account.Y)
	}
	if apps.Y < account.Y || apps.Y > account.Y+16 || signin.Y <= account.Y {
		t.Errorf("account children Y = %v/%v, want both inside the centered item at Y=%v", apps.Y, signin.Y, account.Y)
	}
	accountOuter := account.W + account.PaddingLeft + account.PaddingRight + account.BorderLeft + account.BorderRight
	barContent := bar.W
	if barContent+0.01 < linksOuter+accountOuter {
		t.Errorf("flex container content W = %v, want at least %v for both items", barContent, linksOuter+accountOuter)
	}
}
