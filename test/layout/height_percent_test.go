package layout_test

import (
	"testing"
)

// TestPercentageHeightResolvesAgainstParentHeight guards CSS 2.1 §10.5: a
// percentage height speaks the containing block's *height*, and only when that
// height is definite. Reading it off the containing width instead made
// css-tricks.com's `ul.menu{height:100%}` 1240px tall in a 1280px viewport, and
// the header wrapping it painted its background over the whole page.
func TestPercentageHeightResolvesAgainstParentHeight(t *testing.T) {
	arena := sessionH(t, `<html style="height: 100%;">
<body style="margin: 0; height: 100%;">
  <div id="half" style="height: 50%;"></div>
</body></html>`, 1280, 800)

	html := findByTag(arena, "html")
	body := findByTag(arena, "body")
	half := findByTag(arena, "div")
	if html == nil || body == nil || half == nil {
		t.Fatal("html, body or div missing")
	}
	// The initial containing block is the viewport, so html:100% is 800 tall.
	if !almostEqual(html.H, 800) {
		t.Errorf("html.H = %v, want 800 (100%% of the viewport height)", html.H)
	}
	// A definite parent makes body's percentage resolvable too.
	if !almostEqual(body.H, 800) {
		t.Errorf("body.H = %v, want 800", body.H)
	}
	if !almostEqual(half.H, 400) {
		t.Errorf("div.H = %v, want 400 (50%% of body)", half.H)
	}
}

// TestPercentageHeightWithAutoParentIsAuto is the other half of §10.5: with no
// definite height above it, a percentage height behaves as auto and the box is
// as tall as its content. An auto-height ancestor stops the walk even when the
// viewport is known, because the chain has to be unbroken from the box up to the
// initial containing block.
func TestPercentageHeightWithAutoParentIsAuto(t *testing.T) {
	arena := sessionH(t, `<html><body style="margin: 0;">
<div style="height: 100%;">
  <p style="margin: 0;">One line</p>
</div>
</body></html>`, 1280, 800)

	div := findByTag(arena, "div")
	p := findByTag(arena, "p")
	if div == nil || p == nil {
		t.Fatal("div or p missing")
	}
	if !almostEqual(div.H, p.H) {
		t.Errorf("div.H = %v, want the content height %v rather than 100%% of the width", div.H, p.H)
	}
	if div.H > 2*p.H {
		t.Errorf("div.H = %v, far past its one line of content (%v)", div.H, p.H)
	}
}

// TestPercentageMinAndMaxHeightGuardTheSameAxis keeps min-height and max-height
// on the same rules as height: resolved against a definite parent height, and
// ignored when the parent height is auto.
func TestPercentageMinAndMaxHeightGuardTheSameAxis(t *testing.T) {
	arena := sessionH(t, `<html style="height: 400px;"><body style="margin: 0; height: 100%;">
<div style="min-height: 50%;"></div>
<div style="height: 100%; max-height: 25%;"></div>
</body></html>`, 1280, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	if !almostEqual(divs[0].H, 200) {
		t.Errorf("min-height div.H = %v, want 200 (50%% of html)", divs[0].H)
	}
	if !almostEqual(divs[1].H, 100) {
		t.Errorf("max-height div.H = %v, want 100 (25%% of html, capping 100%%)", divs[1].H)
	}
}
