package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestBorderAreaClipPaintsRingOnly(t *testing.T) {
	s, err := engine.NewSession(`<html><body style="margin:0;"><div style="border: 20px solid transparent; background: green; background-clip: border-area; padding: 10px; width: 100px; height: 50px;">x</div></body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}
	dl, err := s.PaintChecked(1)
	if err != nil {
		t.Fatal(err)
	}
	var fills []string
	for _, cmd := range dl.All() {
		if cmd.Kind == paint.CmdFill {
			fills = append(fills, cmd.Rect.String())
		}
	}
	want := map[string]bool{
		"Rect(0,0 160x20)": true, "Rect(0,90 160x20)": true,
		"Rect(0,20 20x70)": true, "Rect(140,20 20x70)": true,
	}
	if len(fills) != len(want) {
		t.Fatalf("fills = %v, want 4 ring strips", fills)
	}
	for _, f := range fills {
		if !want[f] {
			t.Errorf("unexpected fill %s (paints inside the ring)", f)
		}
	}
}
