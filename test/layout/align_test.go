package layout_test

import (
	"testing"
)

func TestAlignDivInputs(t *testing.T) {
	arena := session(t, `<html><body style="margin:0"><form><div align="center" style="width:1000px"><input type="text" name="q" id="q" style="width:200px"><input type="submit" value="Search" style="width:100px"></div></form></body></html>`, 1000)
	div := findByTag(arena, "div")
	if div == nil {
		t.Fatal("div not found")
	}
	inp := findByTag(arena, "input")
	if inp == nil {
		t.Fatal("input not found")
	}
	// Total inputs width: 200 + 100 = 300. In 1000px wide centered div, expected X around (1000-300)/2 = 350.
	if inp.X < 300 || inp.X > 400 {
		t.Fatalf("expected inp.X around 350, got %v", inp.X)
	}
}
