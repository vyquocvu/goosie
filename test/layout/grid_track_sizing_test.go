package layout_test

import (
	"testing"
)

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
