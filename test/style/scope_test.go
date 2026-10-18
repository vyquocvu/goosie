package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestStyle_ScopeRootAndDescendant(t *testing.T) {
	html := `
  <div class="card" id="card1">
    <p id="p1">Inside card</p>
  </div>
  <div id="other">
    <p id="p2">Outside card</p>
  </div>`

	cssText := `
@scope (.card) {
	:scope {
		border-top-width: 5px;
	}
	p {
		color: #0000ff;
	}
}
p {
	color: #000000;
}`

	doc, styles := parseAndResolve(html, cssText)
	card1 := doc.ElementByID("card1")
	p1 := doc.ElementByID("p1")
	p2 := doc.ElementByID("p2")

	sCard := styles[card1.ID]
	if sCard == nil {
		t.Fatal("no style for card1")
	}
	if sCard.BorderTopWidth != 5 {
		t.Errorf("card1 border-top-width = %v, want 5", sCard.BorderTopWidth)
	}

	sP1 := styles[p1.ID]
	if sP1 == nil {
		t.Fatal("no style for p1")
	}
	// Blue: #0000ff -> r:0, g:0, b:255
	if sP1.Color != (css.Color{R: 0, G: 0, B: 255, A: 255}) {
		t.Errorf("p1 color = %v, want blue", sP1.Color)
	}

	sP2 := styles[p2.ID]
	if sP2 == nil {
		t.Fatal("no style for p2")
	}
	// Black: #000000 -> r:0, g:0, b:0
	if sP2.Color != (css.Color{R: 0, G: 0, B: 0, A: 255}) {
		t.Errorf("p2 color = %v, want black", sP2.Color)
	}
}

func TestStyle_ScopeDonutLimit(t *testing.T) {
	html := `
  <div class="article" id="art">
    <p id="p-art">Article body</p>
    <div class="comments" id="comm">
      <p id="p-comm">Comment text</p>
    </div>
  </div>`

	cssText := `
@scope (.article) to (.comments) {
	p {
		color: #008000; /* green */
	}
}
p {
	color: #000000; /* black fallback */
}`

	doc, styles := parseAndResolve(html, cssText)
	pArt := doc.ElementByID("p-art")
	pComm := doc.ElementByID("p-comm")

	sArt := styles[pArt.ID]
	if sArt == nil {
		t.Fatal("no style for p-art")
	}
	// Green: #008000 -> R:0, G:128, B:0, A:255
	if sArt.Color != (css.Color{R: 0, G: 128, B: 0, A: 255}) {
		t.Errorf("p-art color = %v, want green", sArt.Color)
	}

	sComm := styles[pComm.ID]
	if sComm == nil {
		t.Fatal("no style for p-comm")
	}
	// Inside comments limit: should be black (scoping limit blocked green)
	if sComm.Color != (css.Color{R: 0, G: 0, B: 0, A: 255}) {
		t.Errorf("p-comm color = %v, want black (excluded by donut scope)", sComm.Color)
	}
}
