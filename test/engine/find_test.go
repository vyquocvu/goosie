package engine_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
)

type findBox struct {
	text           string
	x0, y0, x1, y1 float32
}

// findWordBoxes walks the arena in tree order and returns every text box with
// geometry, the same population Session.Find searches.
func findWordBoxes(sess *engine.Session) []findBox {
	objs := sess.Arena.Objects
	var out []findBox
	var walk func(layout.ObjectID)
	walk = func(id layout.ObjectID) {
		if id == 0 || int(id) >= len(objs) {
			return
		}
		o := &objs[id]
		if o.Node != nil && o.Node.Type == dom.NodeText {
			if x0, y0, x1, y1 := o.BorderRect(); x0 < x1 && y0 < y1 {
				out = append(out, findBox{text: o.Node.DataContent, x0: x0, y0: y0, x1: x1, y1: y1})
			}
		}
		for k := o.FirstKid; k != 0; k = objs[k].NextSibling {
			walk(k)
		}
	}
	walk(1)
	return out
}

func rectsNear(m engine.Match, x0, y0, x1, y1 float32) bool {
	const eps = 0.05
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < eps }
	return near(m.X0, x0) && near(m.Y0, y0) && near(m.X1, x1) && near(m.Y1, y1)
}

func TestFindFindsWord(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin: 0;"><p>hello world hello</p></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	var world *findBox
	for i := range findWordBoxes(sess) {
		b := findWordBoxes(sess)[i]
		if b.text == "world" {
			world = &b
			break
		}
	}
	if world == nil {
		t.Fatal("word box for \"world\" not found")
	}

	got := sess.Find("world")
	if len(got) != 1 {
		t.Fatalf("Find(\"world\") = %d matches, want 1", len(got))
	}
	if !rectsNear(got[0], world.x0, world.y0, world.x1, world.y1) {
		t.Fatalf("Find(\"world\")[0] = %+v, want the world box rect %+v", got[0], *world)
	}
}

func TestFindCaseInsensitive(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin: 0;"><p>hello world hello</p></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	var hellos []findBox
	for _, b := range findWordBoxes(sess) {
		if b.text == "hello" {
			hellos = append(hellos, b)
		}
	}
	if len(hellos) != 2 {
		t.Fatalf("expected 2 hello boxes, got %d", len(hellos))
	}

	got := sess.Find("HELLO")
	if len(got) != 2 {
		t.Fatalf("Find(\"HELLO\") = %d matches, want 2", len(got))
	}
	for i, b := range hellos {
		if !rectsNear(got[i], b.x0, b.y0, b.x1, b.y1) {
			t.Fatalf("Find(\"HELLO\")[%d] = %+v, want %+v", i, got[i], b)
		}
	}
}

func TestFindSpansWords(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin: 0;"><p>hello world hello</p></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	var boxes []findBox
	for _, b := range findWordBoxes(sess) {
		if b.text == "hello" || b.text == "world" {
			boxes = append(boxes, b)
		}
	}
	if len(boxes) != 3 {
		t.Fatalf("expected hello/world/hello boxes, got %d", len(boxes))
	}

	got := sess.Find("hello world")
	if len(got) != 1 {
		t.Fatalf("Find(\"hello world\") = %d matches, want 1", len(got))
	}
	want := boxes[0]
	if !rectsNear(got[0], want.x0, want.y0, boxes[1].x1, want.y1) {
		t.Fatalf("Find(\"hello world\")[0] = %+v, want span from hello x0 %v to world x1 %v", got[0], want.x0, boxes[1].x1)
	}
}

func TestFindAcrossParagraphs(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin: 0;"><p>needle one</p><p>two needle</p></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	got := sess.Find("needle")
	if len(got) != 2 {
		t.Fatalf("Find(\"needle\") = %d matches, want 2", len(got))
	}
	if got[0].Y0 >= got[1].Y0 {
		t.Fatalf("matches not in document order: first y %v, second y %v", got[0].Y0, got[1].Y0)
	}
}

func TestFindNoMatch(t *testing.T) {
	sess, err := engine.NewSession(`<html><body style="margin: 0;"><p>hello world</p></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.Find("xyzzy"); len(got) != 0 {
		t.Fatalf("Find(\"xyzzy\") = %d matches, want 0", len(got))
	}
	if got := sess.Find(""); len(got) != 0 {
		t.Fatalf("Find(\"\") = %d matches, want 0", len(got))
	}
}

func TestFindEmptyArena(t *testing.T) {
	sess, err := engine.NewSession(`<html><body>hello</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	sess.Arena = &layout.Arena{}
	if got := sess.Find("hello"); len(got) != 0 {
		t.Fatalf("Find with empty arena = %d matches, want 0", len(got))
	}
}
