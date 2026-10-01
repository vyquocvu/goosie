package css_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestParseTransformTranslate(t *testing.T) {
	funcs := css.ParseTransform("translate(10px, 20px)")
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].Name != "translate" {
		t.Errorf("Name = %q, want %q", funcs[0].Name, "translate")
	}
	if len(funcs[0].Args) != 2 {
		t.Fatalf("got %d args, want 2", len(funcs[0].Args))
	}
	if funcs[0].Args[0] != 10 || funcs[0].Args[1] != 20 {
		t.Errorf("Args = %v, want [10, 20]", funcs[0].Args)
	}
}

func TestParseTransformTranslateX(t *testing.T) {
	funcs := css.ParseTransform("translateX(5px)")
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].Name != "translate" {
		t.Errorf("Name = %q, want %q", funcs[0].Name, "translate")
	}
	if funcs[0].Args[0] != 5 || funcs[0].Args[1] != 0 {
		t.Errorf("Args = %v, want [5, 0]", funcs[0].Args)
	}
}

func TestParseTransformRotate(t *testing.T) {
	funcs := css.ParseTransform("rotate(45deg)")
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].Name != "rotate" {
		t.Errorf("Name = %q, want %q", funcs[0].Name, "rotate")
	}
	want := float32(math.Pi / 4)
	if diff := funcs[0].Args[0] - want; diff > 0.01 || diff < -0.01 {
		t.Errorf("Args[0] = %g, want %g (pi/4)", funcs[0].Args[0], want)
	}
}

func TestParseTransformScale(t *testing.T) {
	funcs := css.ParseTransform("scale(2, 3)")
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].Name != "scale" {
		t.Errorf("Name = %q, want %q", funcs[0].Name, "scale")
	}
	if funcs[0].Args[0] != 2 || funcs[0].Args[1] != 3 {
		t.Errorf("Args = %v, want [2, 3]", funcs[0].Args)
	}
}

func TestParseTransformScaleUniform(t *testing.T) {
	funcs := css.ParseTransform("scale(2)")
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].Name != "scale" {
		t.Errorf("Name = %q, want %q", funcs[0].Name, "scale")
	}
	if funcs[0].Args[0] != 2 || funcs[0].Args[1] != 2 {
		t.Errorf("Args = %v, want [2, 2]", funcs[0].Args)
	}
}

func TestParseTransformSkew(t *testing.T) {
	funcs := css.ParseTransform("skew(30deg, 0deg)")
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].Name != "skew" {
		t.Errorf("Name = %q, want %q", funcs[0].Name, "skew")
	}
	want := float32(30 * math.Pi / 180)
	if diff := funcs[0].Args[0] - want; diff > 0.01 || diff < -0.01 {
		t.Errorf("Args[0] = %g, want %g (30deg in rad)", funcs[0].Args[0], want)
	}
	if funcs[0].Args[1] != 0 {
		t.Errorf("Args[1] = %g, want 0", funcs[0].Args[1])
	}
}

func TestParseTransformMatrix(t *testing.T) {
	funcs := css.ParseTransform("matrix(1, 0, 0, 1, 10, 20)")
	if len(funcs) != 1 {
		t.Fatalf("got %d funcs, want 1", len(funcs))
	}
	if funcs[0].Name != "matrix" {
		t.Errorf("Name = %q, want %q", funcs[0].Name, "matrix")
	}
	want := []float32{1, 0, 0, 1, 10, 20}
	if len(funcs[0].Args) != 6 {
		t.Fatalf("got %d args, want 6", len(funcs[0].Args))
	}
	for i, w := range want {
		if funcs[0].Args[i] != w {
			t.Errorf("Args[%d] = %g, want %g", i, funcs[0].Args[i], w)
		}
	}
}

func TestParseTransformMultiple(t *testing.T) {
	funcs := css.ParseTransform("translate(10px, 20px) rotate(45deg)")
	if len(funcs) != 2 {
		t.Fatalf("got %d funcs, want 2", len(funcs))
	}
	if funcs[0].Name != "translate" {
		t.Errorf("funcs[0].Name = %q, want %q", funcs[0].Name, "translate")
	}
	if funcs[1].Name != "rotate" {
		t.Errorf("funcs[1].Name = %q, want %q", funcs[1].Name, "rotate")
	}
}

func TestParseTransformNone(t *testing.T) {
	funcs := css.ParseTransform("none")
	if funcs != nil {
		t.Errorf("got %v, want nil", funcs)
	}

	funcs = css.ParseTransform("")
	if funcs != nil {
		t.Errorf("empty: got %v, want nil", funcs)
	}
}

func TestParseTransformOrigin(t *testing.T) {
	tests := []struct {
		input    string
		wantX    float32
		wantY    float32
		wantXPct bool
		wantYPct bool
	}{
		{"left top", 0, 0, true, true},
		{"center", 50, 50, true, true},
		{"50% 50%", 50, 50, true, true},
		{"10px 20px", 10, 20, false, false},
		{"right bottom", 100, 100, true, true},
		{"center center", 50, 50, true, true},
	}
	for _, tt := range tests {
		o := css.ParseTransformOrigin(tt.input)
		if o.X != tt.wantX || o.Y != tt.wantY {
			t.Errorf("ParseTransformOrigin(%q) = (%g, %g), want (%g, %g)",
				tt.input, o.X, o.Y, tt.wantX, tt.wantY)
		}
		if o.XIsPct != tt.wantXPct {
			t.Errorf("ParseTransformOrigin(%q).XIsPct = %v, want %v",
				tt.input, o.XIsPct, tt.wantXPct)
		}
		if o.YIsPct != tt.wantYPct {
			t.Errorf("ParseTransformOrigin(%q).YIsPct = %v, want %v",
				tt.input, o.YIsPct, tt.wantYPct)
		}
	}
}

func TestResolveTransformIdentity(t *testing.T) {
	origin := css.DefaultTransformOrigin()
	m := paint.ResolveTransform(nil, origin, 100, 50)
	if !m.IsIdentity() {
		t.Fatalf("no funcs: got %v, want identity", m)
	}
}

func TestResolveTransformTranslate(t *testing.T) {
	funcs := css.ParseTransform("translate(10px, 20px)")
	origin := css.TransformOrigin{X: 0, Y: 0}
	m := paint.ResolveTransform(funcs, origin, 100, 50)
	// With origin at (0,0), translate(10,20) should give tx=10, ty=20.
	if m.Tx != 10 || m.Ty != 20 {
		t.Errorf("translate resolve: Tx=%g Ty=%g, want 10, 20", m.Tx, m.Ty)
	}
	if m.A != 1 || m.D != 1 {
		t.Errorf("translate resolve: A=%g D=%g, want 1, 1", m.A, m.D)
	}
}

func TestResolveTransformScale(t *testing.T) {
	funcs := css.ParseTransform("scale(2)")
	origin := css.TransformOrigin{X: 0, Y: 0}
	m := paint.ResolveTransform(funcs, origin, 100, 50)
	if m.A != 2 || m.D != 2 {
		t.Errorf("scale resolve: A=%g D=%g, want 2, 2", m.A, m.D)
	}
}

func TestResolveTransformWithOrigin(t *testing.T) {
	// scale(2) with origin at center (50, 25) of a 100x50 element.
	funcs := css.ParseTransform("scale(2)")
	origin := css.DefaultTransformOrigin() // 50% 50%
	m := paint.ResolveTransform(funcs, origin, 100, 50)

	// The center point (50,25) should map to itself.
	x, y := m.TransformPoint(50, 25)
	if !fApprox(x, 50) || !fApprox(y, 25) {
		t.Errorf("center after scale with center origin: (%g, %g), want (50, 25)", x, y)
	}

	// The matrix should not be identity.
	if m.IsIdentity() {
		t.Fatal("scale(2) with origin should not be identity")
	}

	// A should be 2, D should be 2 (scale is preserved regardless of origin).
	if !fApprox(m.A, 2) || !fApprox(m.D, 2) {
		t.Errorf("A=%g D=%g, want 2, 2", m.A, m.D)
	}

	// Tx = ox - sx*ox = 50 - 2*50 = -50
	// Ty = oy - sy*oy = 25 - 2*25 = -25
	if !fApprox(m.Tx, -50) || !fApprox(m.Ty, -25) {
		t.Errorf("Tx=%g Ty=%g, want -50, -25", m.Tx, m.Ty)
	}
}

func TestParseTransformAngleUnits(t *testing.T) {
	// Test different angle units.
	tests := []struct {
		input string
		want  float32 // radians
	}{
		{"rotate(180deg)", float32(math.Pi)},
		{"rotate(3.14159rad)", float32(math.Pi)},
		{"rotate(200grad)", float32(math.Pi)},
		{"rotate(0.5turn)", float32(math.Pi)},
	}
	for _, tt := range tests {
		funcs := css.ParseTransform(tt.input)
		if len(funcs) != 1 {
			t.Errorf("ParseTransform(%q): got %d funcs, want 1", tt.input, len(funcs))
			continue
		}
		if !fApprox(funcs[0].Args[0], tt.want) {
			t.Errorf("ParseTransform(%q): angle = %g, want %g",
				tt.input, funcs[0].Args[0], tt.want)
		}
	}
}

func TestResolveTransformMatrix(t *testing.T) {
	funcs := css.ParseTransform("matrix(1, 0, 0, 1, 10, 20)")
	origin := css.TransformOrigin{X: 0, Y: 0}
	m := paint.ResolveTransform(funcs, origin, 100, 50)
	if !fApprox(m.Tx, 10) || !fApprox(m.Ty, 20) {
		t.Errorf("matrix resolve: Tx=%g Ty=%g, want 10, 20", m.Tx, m.Ty)
	}
	if !fApprox(m.A, 1) || !fApprox(m.D, 1) {
		t.Errorf("matrix resolve: A=%g D=%g, want 1, 1", m.A, m.D)
	}
}

func TestResolveTransformProducesCorrectAABB(t *testing.T) {
	// Verify that the resolved transform, when applied to a rect, gives the
	// correct AABB.
	funcs := css.ParseTransform("translate(10px, 20px)")
	origin := css.TransformOrigin{X: 0, Y: 0}
	m := paint.ResolveTransform(funcs, origin, 100, 50)
	r := frame.RectF4(0, 0, 100, 50)
	got := m.TransformRect(r)
	want := frame.RectF4(10, 20, 110, 70)
	if !rectFApprox(got, want) {
		t.Errorf("translated rect = %v, want %v", got, want)
	}
}

func fApprox(a, b float32) bool {
	const eps float32 = 0.01
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}

func rectFApprox(a, b frame.RectF) bool {
	return fApprox(a.X0, b.X0) && fApprox(a.Y0, b.Y0) &&
		fApprox(a.X1, b.X1) && fApprox(a.Y1, b.Y1)
}
