package layout_test

import (
	"testing"
)

// TestGridMinMaxFlexTrackTakesItsShare guards the fr distribution of CSS Grid
// §12.6.1. A `minmax(<length>, <Nfr>)` track's minimum used to be subtracted
// from the space the fractional tracks share *and* applied again as a floor on
// that track's own share, so it was billed twice: `minmax(100px,1fr)` across
// 720px resolved to 620px, and a three-column auto-fill card grid came out 62px
// per column narrower than Chromium with dead space to the right of the row.
func TestGridMinMaxFlexTrackTakesItsShare(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="display: grid; width: 720px; grid-template-columns: minmax(100px, 1fr);"><div>a</div></section>
<section style="display: grid; width: 720px; grid-template-columns: minmax(100px, 1fr) 1fr;"><div>a</div><div>b</div></section>
<section style="display: grid; width: 720px; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); grid-gap: 32px;"><div>a</div><div>b</div><div>c</div></section>
<section style="display: grid; width: 720px; grid-template-columns: minmax(400px, 1fr) 1fr;"><div>a</div><div>b</div></section>
</body></html>`, 800)

	divs := findAllByTag(arena, "div")
	if len(divs) != 8 {
		t.Fatalf("found %d divs, want 8 (one per item)", len(divs))
	}
	for i, want := range []float32{720, 360, 360, 344, 344, 344, 400, 320} {
		if !almostEqual(divs[i].W, want) {
			t.Errorf("item %d W = %v, want %v", i, divs[i].W, want)
		}
	}
	if !almostEqual(divs[7].X, 400) {
		t.Errorf("item 7 X = %v, want 400 (the frozen track leaves 320 for its neighbour)", divs[7].X)
	}
}

// An `auto` column is minmax(min-content, max-content): it takes the width its
// items ask for before §12.6 hands what is left to the flexible tracks. Billing
// the whole container to the fr track instead left a Wikipedia-style sidebar
// column at nothing and moved the article body into its place.
func TestGridAutoColumnSizesBeforeTheFlexTrack(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="display: grid; width: 1000px; gap: 24px; grid-template-columns: auto 1fr;">
<aside style="width: 200px;"></aside><div>content</div>
</section>
</body></html>`, 1200)

	aside := findByTag(arena, "aside")
	if aside == nil {
		t.Fatal("aside not found")
	}
	if !almostEqual(aside.W, 200) {
		t.Errorf("aside.W = %v, want the 200 it declares", aside.W)
	}
	content := findByTag(arena, "div")
	if content == nil {
		t.Fatal("content div not found")
	}
	if !almostEqual(content.W, 776) {
		t.Errorf("content div.W = %v, want 776 (1000 - 24 gap - the 200 auto column)", content.W)
	}
	if !almostEqual(content.X, 224) {
		t.Errorf("content div.X = %v, want 224", content.X)
	}
}

// `grid-template` is the shorthand for all three longhands, and the track list
// after the slash is the column template. Wikipedia's page shell declares its
// grid that way, so an unexpanded value left grid-template-columns empty and the
// container fell through to two auto tracks that split the page down the middle.
// The rem column is the other half of the same rule: 12.25rem is the 196px
// sidebar Chromium gives that column, and a track list that only understood px
// sized it as auto.
func TestGridTemplateShorthandSetsColumnsAndRemTracks(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="display: grid; width: 1192px; gap: 24px; grid-template: auto auto / 12.25rem minmax(0, 1fr);">
<aside></aside><div>content</div>
</section>
</body></html>`, 1280)

	aside := findByTag(arena, "aside")
	if aside == nil {
		t.Fatal("aside not found")
	}
	if !almostEqual(aside.W, 196) {
		t.Errorf("aside.W = %v, want 196 (12.25rem)", aside.W)
	}
	content := findByTag(arena, "div")
	if content == nil {
		t.Fatal("content div not found")
	}
	if !almostEqual(content.W, 972) {
		t.Errorf("content div.W = %v, want 972 (1192 - 24 gap - the 196 column)", content.W)
	}
}

// A percentage track is a share of the grid's own content box, which the cascade
// has no way to know, so it resolves in layout. A page that lays a card row out
// in percentages otherwise got nothing but auto tracks.
func TestGridPercentageTracksResolveAgainstTheContainer(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="display: grid; width: 800px; grid-template-columns: 25% 50%;">
<div>a</div><div>b</div>
</section>
</body></html>`, 1000)

	divs := findAllByTag(arena, "div")
	if len(divs) != 2 {
		t.Fatalf("found %d divs, want 2", len(divs))
	}
	if !almostEqual(divs[0].W, 200) {
		t.Errorf("first column W = %v, want 200 (25%% of 800)", divs[0].W)
	}
	if !almostEqual(divs[1].W, 400) {
		t.Errorf("second column W = %v, want 400 (50%% of 800)", divs[1].W)
	}
}

// `column-gap` spaces only the columns. A page that declares the axes separately
// - Wikipedia's shell sets column-gap and never row-gap - must not have the same
// value spent down the rows too, which moved every row of the page down.
func TestGridColumnGapLeavesTheRowAxisAlone(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0;">
<section style="display: grid; width: 1192px; column-gap: 24px; grid-template-columns: 12.25rem minmax(0, 1fr); grid-template-rows: 40px 40px;">
<aside></aside><div>a</div><div>b</div><div>c</div>
</section>
</body></html>`, 1280)

	divs := findAllByTag(arena, "div")
	if len(divs) != 3 {
		t.Fatalf("found %d divs, want 3", len(divs))
	}
	if !almostEqual(divs[1].Y, 40) {
		t.Errorf("second row Y = %v, want 40 (no row-gap was declared)", divs[1].Y)
	}
	if !almostEqual(divs[0].W, 972) {
		t.Errorf("flex column W = %v, want 972 (1192 - 24 - the 196 rem column)", divs[0].W)
	}
}
