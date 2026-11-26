package layout_test

import "testing"

func TestBrokenImageUsesOnlyDefiniteDimension(t *testing.T) {
	arena := session(t, `<html><body><img id="height-only" src="/missing.svg" style="height:32px">`+
		`<img id="width-only" src="/missing.svg" style="width:100px">`+
		`<img id="both" src="/missing.svg" style="width:100px;height:32px"></body></html>`, 800)

	for _, tc := range []struct {
		id    string
		wantW float32
		wantH float32
	}{
		{"height-only", 0, 32},
		{"width-only", 100, 0},
		{"both", 100, 32},
	} {
		obj := findByID(arena, tc.id)
		if obj == nil {
			t.Fatalf("missing box for %s", tc.id)
		}
		if !almostEqual(obj.W, tc.wantW) || !almostEqual(obj.H, tc.wantH) {
			t.Errorf("%s = %vx%v, want %vx%v", tc.id, obj.W, obj.H, tc.wantW, tc.wantH)
		}
	}
}
