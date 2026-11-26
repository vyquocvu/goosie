package layout_test

import "testing"

func TestFlexMinHeightCentersSingleLine(t *testing.T) {
	const doc = `<html><body><div id="card" style="padding:40px;width:100%;max-width:400px"><p>hello</p></div></body></html>`
	const sheet = `*{margin:0;padding:0;box-sizing:border-box}body{min-height:100vh;display:flex;align-items:center;justify-content:center}`
	arena := layoutAtViewport(t, doc, sheet, 800, 600)
	body := findByTag(arena, "body")
	card := findByID(arena, "card")
	if body == nil || card == nil {
		t.Fatalf("body=%v card=%v", body, card)
	}
	if !almostEqual(body.H, 600) {
		t.Errorf("body.H = %v, want 600 (min-height:100vh)", body.H)
	}
	want := (body.H - card.BorderH()) / 2
	if !almostEqual(card.Y, want) {
		t.Errorf("card.Y = %v, want %v (center in the min-height:100vh cross size)", card.Y, want)
	}
}
