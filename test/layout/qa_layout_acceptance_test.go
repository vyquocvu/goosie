package layout_test

import (
	"testing"
)

// qc:BR-060
func TestNestedBlocksAndInlinesKeepLineOrderWithoutOverlap(t *testing.T) {
	t.Run("inline siblings and nesting stay on one line in order", func(t *testing.T) {
		arena := session(t, `<html><body style="margin:0">
<div id="line" style="width:400px;font-size:16px;line-height:20px">alpha <span>beta <b>gamma</b></span> delta</div>
<div id="tail">tail</div>
</body></html>`, 800)

		line := findByID(arena, "line")
		tail := findByID(arena, "tail")
		if line == nil || tail == nil {
			t.Fatal("fixture boxes not found")
		}
		var prev *wordAt
		for _, w := range []string{"alpha", "beta", "gamma", "delta"} {
			obj := findWordObject(arena, w)
			if obj == nil {
				t.Fatalf("word %q was not laid out", w)
			}
			cur := &wordAt{x: obj.X, y: obj.Y}
			if prev != nil {
				if !almostEqual(cur.y, prev.y) {
					t.Errorf("%q Y = %v, want %v (same line as the previous word)", w, cur.y, prev.y)
				}
				if cur.x <= prev.x {
					t.Errorf("%q X = %v, want past the previous word at %v", w, cur.x, prev.x)
				}
			}
			prev = cur
		}
		if tail.Y < line.Y+line.H {
			t.Errorf("tail.Y = %v, want >= %v (below the first block)", tail.Y, line.Y+line.H)
		}
	})

	t.Run("a block inside an inline splits the line around it", func(t *testing.T) {
		arena := session(t, `<html><body style="margin:0">
<div id="outer" style="width:300px;font-size:16px;line-height:20px">before <span id="mid" style="display:block;height:30px">middle</span> after</div>
</body></html>`, 800)

		mid := findByID(arena, "mid")
		before := findWordObject(arena, "before")
		after := findWordObject(arena, "after")
		if mid == nil || before == nil || after == nil {
			t.Fatal("fixture boxes not found")
		}
		if mid.H != 30 {
			t.Errorf("mid.H = %v, want the declared 30", mid.H)
		}
		if before.Y >= mid.Y {
			t.Errorf("before.Y = %v, want it above the block at %v", before.Y, mid.Y)
		}
		if after.Y < mid.Y+mid.H {
			t.Errorf("after.Y = %v, want >= %v (below the block, not overlapping it)", after.Y, mid.Y+mid.H)
		}
	})
}

type wordAt struct{ x, y float32 }

// qc:BR-064
func TestBoxModelSizesContentAndBorderBoxes(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div id="content" style="width:100px;height:50px;padding:10px;border:5px solid black;margin:7px"></div>
<div id="border" style="box-sizing:border-box;width:100px;height:60px;padding:10px;border:5px solid black"></div>
</body></html>`, 800)

	content := findByID(arena, "content")
	border := findByID(arena, "border")
	if content == nil || border == nil {
		t.Fatal("fixture boxes not found")
	}

	if content.W != 100 || content.H != 50 {
		t.Errorf("content box = %vx%v, want 100x50", content.W, content.H)
	}
	if x0, y0, x1, y1 := content.BorderRect(); !almostEqual(x1-x0, 130) || !almostEqual(y1-y0, 80) {
		t.Errorf("content border box = %vx%v, want 130x80", x1-x0, y1-y0)
	}
	if !almostEqual(content.X, 7) || !almostEqual(content.Y, 7) {
		t.Errorf("content origin = (%v,%v), want (7,7) from the margin", content.X, content.Y)
	}

	if border.W != 70 || border.H != 30 {
		t.Errorf("border box content = %vx%v, want 70x30", border.W, border.H)
	}
	if x0, y0, x1, y1 := border.BorderRect(); !almostEqual(x1-x0, 100) || !almostEqual(y1-y0, 60) {
		t.Errorf("border-box outer = %vx%v, want 100x60", x1-x0, y1-y0)
	}
}

// qc:BR-065
func TestFontSizeWeightAndLineHeightDriveWrappingAndHeight(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div id="small" style="width:120px;font-size:16px;line-height:20px">One two three four five six seven eight</div>
<div id="large" style="width:120px;font-size:24px;line-height:30px;font-weight:700">One two three four five six seven eight</div>
</body></html>`, 800)

	small := findByID(arena, "small")
	large := findByID(arena, "large")
	if small == nil || large == nil {
		t.Fatal("fixture boxes not found")
	}
	if small.H <= 20 {
		t.Errorf("small.H = %v, want more than one 20px line (the text wraps at 120px)", small.H)
	}
	if large.H <= 30 {
		t.Errorf("large.H = %v, want more than one 30px line", large.H)
	}
	if large.H <= small.H {
		t.Errorf("large.H = %v, want taller than small.H = %v (bigger font and line-height)", large.H, small.H)
	}
	for _, tc := range []struct {
		name string
		w    float32
	}{{"small", small.W}, {"large", large.W}} {
		if !almostEqual(tc.w, 120) {
			t.Errorf("%s.W = %v, want the declared 120", tc.name, tc.w)
		}
	}
}

// qc:BR-066
func TestFlexRowColumnGapAlignAndWrap(t *testing.T) {
	t.Run("row gap and centred alignment", func(t *testing.T) {
		arena := session(t, `<html><body style="margin:0">
<div id="row" style="display:flex;gap:10px;width:330px;height:100px;align-items:center">
<div id="a" style="width:100px;height:20px"></div>
<div id="b" style="width:100px;height:40px"></div>
</div></body></html>`, 800)

		a, b := findByID(arena, "a"), findByID(arena, "b")
		if a == nil || b == nil {
			t.Fatal("flex items not found")
		}
		if !almostEqual(b.X, 110) {
			t.Errorf("b.X = %v, want 110 (100px item + 10px gap)", b.X)
		}
		if !almostEqual(a.Y, 40) {
			t.Errorf("a.Y = %v, want 40 (centred 20px item in a 100px row)", a.Y)
		}
		if !almostEqual(b.Y, 30) {
			t.Errorf("b.Y = %v, want 30 (centred 40px item in a 100px row)", b.Y)
		}
	})

	t.Run("wrap moves the third item to a second line", func(t *testing.T) {
		arena := session(t, `<html><body style="margin:0">
<div id="wrap" style="display:flex;flex-wrap:wrap;gap:10px;width:210px">
<div id="c" style="width:100px;height:20px"></div>
<div id="d" style="width:100px;height:20px"></div>
<div id="e" style="width:100px;height:20px"></div>
</div></body></html>`, 800)

		c, d, e := findByID(arena, "c"), findByID(arena, "d"), findByID(arena, "e")
		if c == nil || d == nil || e == nil {
			t.Fatal("flex items not found")
		}
		if !almostEqual(c.Y, 0) || !almostEqual(d.Y, 0) {
			t.Errorf("first row Y = %v,%v, want 0,0", c.Y, d.Y)
		}
		if !almostEqual(e.X, 0) {
			t.Errorf("e.X = %v, want 0 (start of the second line)", e.X)
		}
		if !almostEqual(e.Y, 30) {
			t.Errorf("e.Y = %v, want 30 (20px line + 10px row gap)", e.Y)
		}
	})

	t.Run("column direction stacks with the row gap", func(t *testing.T) {
		arena := session(t, `<html><body style="margin:0">
<div id="col" style="display:flex;flex-direction:column;gap:8px;height:200px">
<div id="f" style="height:30px"></div>
<div id="g" style="height:30px"></div>
</div></body></html>`, 800)

		f, g := findByID(arena, "f"), findByID(arena, "g")
		if f == nil || g == nil {
			t.Fatal("flex items not found")
		}
		if !almostEqual(f.Y, 0) || !almostEqual(g.Y, 38) {
			t.Errorf("column item Y = %v,%v, want 0,38", f.Y, g.Y)
		}
		if !almostEqual(f.X, 0) || !almostEqual(g.X, 0) {
			t.Errorf("column item X = %v,%v, want 0,0", f.X, g.X)
		}
	})
}

// qc:BR-067
func TestGridTracksAndGapsPlaceItemsInTheirCells(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div id="grid" style="display:grid;grid-template-columns:100px 180px;gap:20px;width:300px">
<div id="g1" style="height:30px"></div>
<div id="g2" style="height:30px"></div>
<div id="g3" style="height:30px"></div>
<div id="g4" style="height:30px"></div>
</div></body></html>`, 800)

	g1, g2, g3, g4 := findByID(arena, "g1"), findByID(arena, "g2"), findByID(arena, "g3"), findByID(arena, "g4")
	if g1 == nil || g2 == nil || g3 == nil || g4 == nil {
		t.Fatal("grid items not found")
	}
	if !almostEqual(g1.X, 0) || !almostEqual(g1.W, 100) {
		t.Errorf("g1 = x%v w%v, want x0 w100", g1.X, g1.W)
	}
	if !almostEqual(g2.X, 120) || !almostEqual(g2.W, 180) {
		t.Errorf("g2 = x%v w%v, want x120 w180", g2.X, g2.W)
	}
	if !almostEqual(g3.X, 0) || !almostEqual(g3.Y, 50) {
		t.Errorf("g3 = (%v,%v), want (0,50)", g3.X, g3.Y)
	}
	if !almostEqual(g4.X, 120) || !almostEqual(g4.Y, 50) {
		t.Errorf("g4 = (%v,%v), want (120,50)", g4.X, g4.Y)
	}
	if right := g2.X + g2.W; right > 300 {
		t.Errorf("last column ends at %v, want <= 300", right)
	}
}

// qc:BR-068
func TestTableColumnsRowsAndBordersStayAligned(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<table id="t" style="width:300px;border-collapse:collapse">
<thead><tr>
<th id="h1" style="width:100px;border:1px solid black">H1</th>
<th id="h2" style="border:1px solid black">H2</th>
</tr></thead>
<tbody>
<tr><td id="c1" style="border:1px solid black">A</td><td id="c2" style="border:1px solid black">B</td></tr>
<tr><td id="c3" style="border:1px solid black">C</td><td id="c4" style="border:1px solid black">D</td></tr>
</tbody></table>
</body></html>`, 800)

	cells := map[string]*struct{ x, y, w, h float32 }{}
	boxes := map[string]bool{}
	for _, id := range []string{"h1", "h2", "c1", "c2", "c3", "c4"} {
		obj := findByID(arena, id)
		if obj == nil {
			t.Fatalf("cell %q not laid out", id)
		}
		cells[id] = &struct{ x, y, w, h float32 }{obj.X, obj.Y, obj.W, obj.H}
		if obj.BorderTop != 1 || obj.BorderLeft != 1 {
			t.Errorf("%s borders = %v/%v, want 1px", id, obj.BorderTop, obj.BorderLeft)
		}
		boxes[id] = true
	}
	if len(boxes) != 6 {
		t.Fatalf("found %d cells, want 6", len(boxes))
	}

	for _, pair := range [][2]string{{"h1", "c1"}, {"h1", "c3"}, {"h2", "c2"}, {"h2", "c4"}} {
		a, b := cells[pair[0]], cells[pair[1]]
		if !almostEqual(a.x, b.x) {
			t.Errorf("%s x = %v, want %s x = %v", pair[1], b.x, pair[0], a.x)
		}
		if !almostEqual(a.w, b.w) {
			t.Errorf("%s w = %v, want %s w = %v", pair[1], b.w, pair[0], a.w)
		}
	}
	if cells["h1"].y >= cells["c1"].y || cells["c1"].y >= cells["c3"].y {
		t.Errorf("row Y = %v,%v,%v, want strictly increasing", cells["h1"].y, cells["c1"].y, cells["c3"].y)
	}
	for _, w := range []string{"H1", "H2", "A", "B", "C", "D"} {
		if findWordObject(arena, w) == nil {
			t.Errorf("word %q missing from the laid-out table", w)
		}
	}
}

// qc:BR-069
func TestFloatTakesSpaceAndClearsAfterItsBlock(t *testing.T) {
	arena := session(t, `<html><body style="margin:0">
<div id="wrap" style="width:300px;font-size:16px;line-height:20px">
<div id="f" style="float:left;width:100px;height:80px"></div>
Alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu</div>
<div id="clear" style="clear:left">cleared</div>
</body></html>`, 800)

	float := findByID(arena, "f")
	if float == nil {
		t.Fatal("float not laid out")
	}
	if !almostEqual(float.W, 100) || !almostEqual(float.H, 80) {
		t.Errorf("float = %vx%v, want 100x80", float.W, float.H)
	}
	first := findWordObject(arena, "Alpha")
	if first == nil {
		t.Fatal("first wrapped word not found")
	}
	if first.X < 100 {
		t.Errorf("Alpha.X = %v, want >= 100 (text flows beside the float)", first.X)
	}
	cleared := findWordObject(arena, "cleared")
	if cleared == nil {
		t.Fatal("cleared word not found")
	}
	if cleared.Y < float.Y+float.H {
		t.Errorf("cleared.Y = %v, want >= %v (clear:left releases the region below the float)", cleared.Y, float.Y+float.H)
	}
}

// qc:BR-070
func TestRelativeAbsoluteAndFixedUseTheirDeclaredContainingBlocks(t *testing.T) {
	t.Run("relative offsets without leaving a hole in the flow", func(t *testing.T) {
		arena := positionedSession(t, `<html><body style="margin:0">
<div id="rel" style="position:relative;top:10px;left:20px;width:100px;height:50px"></div>
<div id="next" style="height:10px"></div>
</body></html>`, 800)

		rel, next := findByID(arena, "rel"), findByID(arena, "next")
		if rel == nil || next == nil {
			t.Fatal("fixture boxes not found")
		}
		if !almostEqual(rel.X, 20) || !almostEqual(rel.Y, 10) {
			t.Errorf("rel origin = (%v,%v), want (20,10)", rel.X, rel.Y)
		}
		if !almostEqual(next.Y, 50) {
			t.Errorf("next.Y = %v, want 50 (the relative offset did not move the flow)", next.Y)
		}
	})

	t.Run("absolute resolves against the relative ancestor's padding box", func(t *testing.T) {
		arena := positionedSession(t, `<html><body style="margin:0">
<div id="cb" style="position:relative;margin:20px;padding:10px;width:200px;height:100px">
<div id="abs" style="position:absolute;top:5px;left:15px;width:50px;height:30px"></div>
</div></body></html>`, 800)

		abs := findByID(arena, "abs")
		if abs == nil {
			t.Fatal("absolute box not found")
		}
		if !almostEqual(abs.X, 35) || !almostEqual(abs.Y, 25) {
			t.Errorf("abs origin = (%v,%v), want (35,25): the padding box starts 20px out and the inset moves it 15/5 further", abs.X, abs.Y)
		}
	})

	t.Run("fixed resolves against the viewport and leaves the flow", func(t *testing.T) {
		arena := positionedSession(t, `<html><body style="margin:0">
<div id="fix" style="position:fixed;top:0;right:0;width:50px;height:40px"></div>
<div id="flow" style="height:10px"></div>
</body></html>`, 800)

		fix, flow := findByID(arena, "fix"), findByID(arena, "flow")
		if fix == nil || flow == nil {
			t.Fatal("fixture boxes not found")
		}
		if !almostEqual(fix.X, 750) || !almostEqual(fix.Y, 0) {
			t.Errorf("fixed origin = (%v,%v), want (750,0) right-aligned in the viewport", fix.X, fix.Y)
		}
		if !almostEqual(flow.Y, 0) {
			t.Errorf("flow.Y = %v, want 0 (the fixed box takes no flow space)", flow.Y)
		}
	})
}
