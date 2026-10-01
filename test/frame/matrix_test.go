package frame_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
)

func TestMatrixIdentity(t *testing.T) {
	m := frame.MatrixIdentity
	x, y := m.TransformPoint(10, 20)
	if x != 10 || y != 20 {
		t.Fatalf("identity transform: got (%g, %g), want (10, 20)", x, y)
	}
	if !m.IsIdentity() {
		t.Fatal("identity matrix not reported as identity")
	}
}

func TestMatrixTranslate(t *testing.T) {
	m := frame.MatrixTranslate(5, 10)
	x, y := m.TransformPoint(3, 7)
	if x != 8 || y != 17 {
		t.Fatalf("translate(5,10) * (3,7) = (%g, %g), want (8, 17)", x, y)
	}
	if m.IsIdentity() {
		t.Fatal("non-trivial translate reported as identity")
	}
}

func TestMatrixScale(t *testing.T) {
	m := frame.MatrixScale(2, 3)
	x, y := m.TransformPoint(5, 10)
	if x != 10 || y != 30 {
		t.Fatalf("scale(2,3) * (5,10) = (%g, %g), want (10, 30)", x, y)
	}
}

func TestMatrixRotate(t *testing.T) {
	// Rotate 90 degrees: (1, 0) -> (0, 1)
	m := frame.MatrixRotate(math.Pi / 2)
	x, y := m.TransformPoint(1, 0)
	if !approx(x, 0) || !approx(y, 1) {
		t.Fatalf("rotate(90deg) * (1,0) = (%g, %g), want ~(0, 1)", x, y)
	}

	// Rotate 180 degrees: (1, 0) -> (-1, 0)
	m = frame.MatrixRotate(math.Pi)
	x, y = m.TransformPoint(1, 0)
	if !approx(x, -1) || !approx(y, 0) {
		t.Fatalf("rotate(180deg) * (1,0) = (%g, %g), want ~(-1, 0)", x, y)
	}
}

func TestMatrixMultiply(t *testing.T) {
	// translate(10, 20) * scale(2, 3):
	// First scale (2,3), then translate (10,20).
	// Point (5, 5) -> scale -> (10, 15) -> translate -> (20, 35)
	tr := frame.MatrixTranslate(10, 20)
	sc := frame.MatrixScale(2, 3)
	combined := tr.Multiply(sc)
	x, y := combined.TransformPoint(5, 5)
	if !approx(x, 20) || !approx(y, 35) {
		t.Fatalf("translate*scale * (5,5) = (%g, %g), want (20, 35)", x, y)
	}
}

func TestMatrixTransformRect(t *testing.T) {
	// Translate a rect.
	m := frame.MatrixTranslate(10, 20)
	r := frame.RectF4(0, 0, 100, 50)
	got := m.TransformRect(r)
	want := frame.RectF4(10, 20, 110, 70)
	if !rectFApprox(got, want) {
		t.Fatalf("translate transform rect = %v, want %v", got, want)
	}

	// Scale a rect.
	m = frame.MatrixScale(2, 3)
	got = m.TransformRect(r)
	want = frame.RectF4(0, 0, 200, 150)
	if !rectFApprox(got, want) {
		t.Fatalf("scale transform rect = %v, want %v", got, want)
	}

	// Rotate 90 degrees: rect (0,0,100,50) -> AABB should be (-50,0,0,100)
	m = frame.MatrixRotate(math.Pi / 2)
	got = m.TransformRect(r)
	// After 90deg rotation: (0,0)->(0,0), (100,0)->(0,100), (100,50)->(-50,100), (0,50)->(-50,0)
	// AABB: (-50, 0, 0, 100)
	want = frame.RectF4(-50, 0, 0, 100)
	if !rectFApprox(got, want) {
		t.Fatalf("rotate 90 transform rect = %v, want %v", got, want)
	}
}

func TestMatrixInverse(t *testing.T) {
	// Translate inverse.
	m := frame.MatrixTranslate(10, 20)
	inv := m.Inverse()
	x, y := inv.TransformPoint(15, 25)
	if !approx(x, 5) || !approx(y, 5) {
		t.Fatalf("inverse translate * (15,25) = (%g, %g), want (5, 5)", x, y)
	}

	// Scale inverse.
	m = frame.MatrixScale(2, 4)
	inv = m.Inverse()
	x, y = inv.TransformPoint(10, 20)
	if !approx(x, 5) || !approx(y, 5) {
		t.Fatalf("inverse scale * (10,20) = (%g, %g), want (5, 5)", x, y)
	}

	// Combined: M * M^-1 should be identity.
	m = frame.MatrixTranslate(10, 20).Multiply(frame.MatrixScale(2, 3))
	inv = m.Inverse()
	product := m.Multiply(inv)
	if !product.IsIdentity() {
		t.Fatalf("M * M^-1 is not identity: %v", product)
	}
}

func TestMatrixIsIdentity(t *testing.T) {
	if !frame.MatrixIdentity.IsIdentity() {
		t.Fatal("MatrixIdentity.IsIdentity() = false")
	}
	if frame.MatrixTranslate(1, 0).IsIdentity() {
		t.Fatal("translate(1,0).IsIdentity() = true")
	}
	if frame.MatrixScale(1, 1).IsIdentity() != true {
		t.Fatal("scale(1,1).IsIdentity() = false, want true")
	}
}

func approx(a, b float32) bool {
	const eps float32 = 0.01
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}

func rectFApprox(a, b frame.RectF) bool {
	return approx(a.X0, b.X0) && approx(a.Y0, b.Y0) &&
		approx(a.X1, b.X1) && approx(a.Y1, b.Y1)
}
