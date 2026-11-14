package layout_test

import (
	"testing"
)

const minifiedCardList = `<html><head><style>
@supports(display:grid){.glue-grid{display:grid;grid-column-gap:28px;grid-template-columns:repeat(4,minmax(5px,1fr))}}
@supports(display:grid){@media(min-width:600px){.glue-grid{grid-column-gap:40px;grid-template-columns:repeat(12,minmax(5px,1fr))}}}
@supports(display:grid){@media(min-width:1024px){.glue-grid{grid-column-gap:48px;row-gap:48px}}}
@supports(display:grid){.glue-grid .glue-grid__col--span-4{grid-column-end:span 4}}
@supports(display:grid){@media(min-width:600px){.glue-grid .glue-grid__col--span-4{display:block;grid-column-end:span 4}}}
.glue-cards{list-style:none;margin:0;padding:0}
</style></head><body style="margin:0">
<ul class="glue-cards glue-grid" style="width:1136px">
<li class="glue-grid__col glue-grid__col--span-4" style="height:100px">a</li>
<li class="glue-grid__col glue-grid__col--span-4" style="height:100px">b</li>
<li class="glue-grid__col glue-grid__col--span-4" style="height:100px">c</li>
<li class="glue-grid__col glue-grid__col--span-4" style="height:100px">d</li>
</ul>
</body></html>`

// qc:BR-289
func TestMinifiedAtRulePreludeReachesGridGeometry(t *testing.T) {
	arena := sessionSheets(t, minifiedCardList, 1280)
	lis := findAllByTag(arena, "li")
	if len(lis) != 4 {
		t.Fatalf("found %d li boxes, want 4", len(lis))
	}
	for i, want := range []struct{ x, y, w float32 }{
		{0, 0, 346.6667},
		{394.6667, 0, 346.6667},
		{789.3333, 0, 346.6667},
		{0, 148, 346.6667},
	} {
		if !almostEqual(lis[i].X, want.x) || !almostEqual(lis[i].Y, want.y) || !almostEqual(lis[i].W, want.w) {
			t.Errorf("card %d = x %v y %v w %v, want x %v y %v w %v (12 tracks of (1136-11*48)/12 spanned 4, plus the 48px row-gap)",
				i, lis[i].X, lis[i].Y, lis[i].W, want.x, want.y, want.w)
		}
	}
	ul := findByTag(arena, "ul")
	if !almostEqual(ul.H, 248) {
		t.Errorf("ul.H = %v, want 248 (two rows of 100 joined by the min-width:1024px branch's 48px row-gap)", ul.H)
	}
}

// qc:BR-289
func TestMinifiedMediaBranchesAreSelectedByWidthNotIgnored(t *testing.T) {
	mid := sessionSheets(t, minifiedCardList, 800)
	ul := findByTag(mid, "ul")
	if !almostEqual(ul.H, 200) {
		t.Errorf("ul.H at 800 = %v, want 200 (the min-width:1024px row-gap must not apply at 800)", ul.H)
	}
	cards := findAllByTag(mid, "li")
	if len(cards) != 4 {
		t.Fatalf("found %d cards at 800, want 4", len(cards))
	}
	if !almostEqual(cards[0].W, 352) || !almostEqual(cards[1].X, 392) {
		t.Errorf("cards at 800 = w %v x %v, want 352 and 392 (12 tracks of (1136-11*40)/12 spanned 4 with the 40px column-gap)",
			cards[0].W, cards[1].X)
	}

	base := sessionSheets(t, minifiedCardList, 500)
	below := findAllByTag(base, "li")
	if len(below) != 4 {
		t.Fatalf("found %d cards at 500, want 4", len(below))
	}
	for i, li := range below {
		if !almostEqual(li.X, 0) || !almostEqual(li.W, 1136) {
			t.Errorf("card %d at 500 = x %v w %v, want 0 and 1136 (the 4-track branch puts one span-4 card per row)", i, li.X, li.W)
		}
	}
	if !almostEqual(below[3].Y, 300) {
		t.Errorf("card 3 y at 500 = %v, want 300 (no row-gap below min-width:1024px)", below[3].Y)
	}
}
