package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestAspectRatioParsing(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	d1 := doc.NewElement("div")
	d1.SetAttribute("id", "d1")
	body.AppendChild(d1)

	d2 := doc.NewElement("div")
	d2.SetAttribute("id", "d2")
	body.AppendChild(d2)

	d3 := doc.NewElement("div")
	d3.SetAttribute("id", "d3")
	body.AppendChild(d3)

	d4 := doc.NewElement("div")
	d4.SetAttribute("id", "d4")
	body.AppendChild(d4)

	sheet := css.Parse(`
		#d1 { aspect-ratio: 16 / 9; }
		#d2 { aspect-ratio: 2; }
		#d3 { aspect-ratio: auto 4 / 3; }
		#d4 { aspect-ratio: auto; }
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s1 := styles[d1.ID]
	expected1 := float32(16.0 / 9.0)
	if s1 == nil || s1.AspectRatio < expected1-0.001 || s1.AspectRatio > expected1+0.001 {
		t.Fatalf("d1 AspectRatio = %v, want %v", s1.AspectRatio, expected1)
	}

	s2 := styles[d2.ID]
	if s2 == nil || s2.AspectRatio != 2.0 {
		t.Fatalf("d2 AspectRatio = %v, want 2.0", s2.AspectRatio)
	}

	s3 := styles[d3.ID]
	expected3 := float32(4.0 / 3.0)
	if s3 == nil || s3.AspectRatio < expected3-0.001 || s3.AspectRatio > expected3+0.001 {
		t.Fatalf("d3 AspectRatio = %v, want %v", s3.AspectRatio, expected3)
	}

	s4 := styles[d4.ID]
	if s4 == nil || s4.AspectRatio != 0 {
		t.Fatalf("d4 AspectRatio = %v, want 0", s4.AspectRatio)
	}
}

func TestAspectRatioLayoutResolution(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
<html>
<head>
<style>
#card {
	width: 160px;
	aspect-ratio: 16 / 9;
}
#square {
	display: inline-block;
	height: 100px;
	aspect-ratio: 1 / 1;
}
</style>
</head>
<body>
<div id="card">Card</div>
<div id="square">Square</div>
</body>
</html>`

	sess, err := engine.NewSession(htmlStr, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	doc := sess.Doc
	cardNode := doc.ElementByID("card")
	if cardNode == nil {
		t.Fatal("card node not found")
	}

	squareNode := doc.ElementByID("square")
	if squareNode == nil {
		t.Fatal("square node not found")
	}

	arena := sess.Arena
	cardID, ok := arena.ForNode(cardNode.ID)
	if !ok {
		t.Fatal("card layout object not found")
	}
	cardObj := arena.Get(cardID)
	if cardObj.W != 160 {
		t.Errorf("card width = %v, want 160", cardObj.W)
	}
	// 160 / (16/9) = 90
	if cardObj.H != 90 {
		t.Errorf("card height = %v, want 90", cardObj.H)
	}

	squareID, ok := arena.ForNode(squareNode.ID)
	if !ok {
		t.Fatal("square layout object not found")
	}
	squareObj := arena.Get(squareID)
	if squareObj.H != 100 {
		t.Errorf("square height = %v, want 100", squareObj.H)
	}
	if squareObj.W != 100 {
		t.Errorf("square width = %v, want 100", squareObj.W)
	}
}
