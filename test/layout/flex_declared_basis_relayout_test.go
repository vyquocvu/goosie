package layout_test

import "testing"

func TestDeclaredBasisFlexItemLaysSubtreeOutAtFinalOrigin(t *testing.T) {
	arena := session(t, `<html><body style="margin: 0; height: 600px; display: flex; align-items: center; justify-content: center;">
<div style="width: 100%; max-width: 400px; padding: 40px; box-sizing: border-box;">
  <h1 style="text-align: center; font-size: 28px;">Welcome Back</h1>
</div>
</body></html>`, 800)

	card := findByTag(arena, "div")
	if card == nil {
		t.Fatal("card not found")
	}
	if !almostEqual(card.X, 200) {
		t.Errorf("card.X = %v, want 200 (justify-content: center)", card.X)
	}
	if !almostEqual(card.W, 320) {
		t.Errorf("card.W = %v, want 320 (400px border box minus 40px padding a side)", card.W)
	}

	h1 := findByTag(arena, "h1")
	if h1 == nil {
		t.Fatal("heading not found")
	}
	if !almostEqual(h1.X, 240) || !almostEqual(h1.W, 320) {
		t.Errorf("h1 box = X %v W %v, want X 240 W 320 (the card's content box)", h1.X, h1.W)
	}
	if h1.H <= 0 {
		t.Errorf("h1.H = %v; a heading with one text line must carry its height", h1.H)
	}

	var minX, maxX float32
	seen := false
	for kid := h1.FirstKid; kid != 0; kid = arena.Get(kid).NextSibling {
		k := arena.Get(kid)
		if k.Node == nil || !k.Node.Text() || k.W <= 0 {
			continue
		}
		if !seen || k.X < minX {
			minX = k.X
		}
		if !seen || k.X+k.W > maxX {
			maxX = k.X + k.W
		}
		seen = true
	}
	if !seen {
		t.Fatal("heading carries no text runs")
	}
	if minX < h1.X || maxX > h1.X+h1.W {
		t.Errorf("heading text spans [%v,%v], outside its box [%v,%v]", minX, maxX, h1.X, h1.X+h1.W)
	}
	if want := h1.X + h1.W/2; !almostEqual((minX+maxX)/2, want) {
		t.Errorf("heading text centre = %v, want %v (text-align: center inside its own box)", (minX+maxX)/2, want)
	}
}
