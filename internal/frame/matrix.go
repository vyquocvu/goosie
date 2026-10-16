// 2D affine transform matrix for CSS transforms. The matrix type is a value
// type with no locking and no allocation, matching the frame package contract.

package frame

import "math"

// Matrix3x2 is a 2D affine transform matrix:
//
//	[a  c  tx]
//	[b  d  ty]
//	[0  0  1 ]
//
// Stored as [a, b, c, d, tx, ty].
type Matrix3x2 struct {
	A, B, C, D, Tx, Ty float32
}

// MatrixIdentity is the identity transform: no translation, rotation, scale,
// or skew.
var MatrixIdentity = Matrix3x2{1, 0, 0, 1, 0, 0}

// MatrixTranslate returns a translation matrix.
func MatrixTranslate(tx, ty float32) Matrix3x2 {
	return Matrix3x2{1, 0, 0, 1, tx, ty}
}

// MatrixRotate returns a rotation matrix for the given angle in radians.
func MatrixRotate(angleRadians float32) Matrix3x2 {
	s := float32(math.Sin(float64(angleRadians)))
	c := float32(math.Cos(float64(angleRadians)))
	return Matrix3x2{c, s, -s, c, 0, 0}
}

// MatrixScale returns a scaling matrix.
func MatrixScale(sx, sy float32) Matrix3x2 {
	return Matrix3x2{sx, 0, 0, sy, 0, 0}
}

// MatrixSkew returns a skew matrix. ax and ay are the skew angles in radians
// for the X and Y axes respectively.
func MatrixSkew(ax, ay float32) Matrix3x2 {
	return Matrix3x2{1, float32(math.Tan(float64(ay))), float32(math.Tan(float64(ax))), 1, 0, 0}
}

// Multiply returns the product m * n, which is the transform that first applies
// n and then m. This matches CSS transform list order where functions are
// applied left to right: translate(10px) rotate(45deg) means first translate,
// then rotate.
func (m Matrix3x2) Multiply(n Matrix3x2) Matrix3x2 {
	return Matrix3x2{
		A:  m.A*n.A + m.C*n.B,
		B:  m.B*n.A + m.D*n.B,
		C:  m.A*n.C + m.C*n.D,
		D:  m.B*n.C + m.D*n.D,
		Tx: m.A*n.Tx + m.C*n.Ty + m.Tx,
		Ty: m.B*n.Tx + m.D*n.Ty + m.Ty,
	}
}

// TransformPoint applies the matrix to a point and returns the transformed
// coordinates.
func (m Matrix3x2) TransformPoint(x, y float32) (float32, float32) {
	return m.A*x + m.C*y + m.Tx, m.B*x + m.D*y + m.Ty
}

// TransformRect returns the axis-aligned bounding box of the rect after the
// transform is applied to all four corners. Rotation and skew can produce a
// non-axis-aligned result, so this returns the enclosing AABB.
func (m Matrix3x2) TransformRect(r RectF) RectF {
	x0, y0 := m.TransformPoint(r.X0, r.Y0)
	x1, y1 := m.TransformPoint(r.X1, r.Y0)
	x2, y2 := m.TransformPoint(r.X1, r.Y1)
	x3, y3 := m.TransformPoint(r.X0, r.Y1)

	minX := x0
	if x1 < minX {
		minX = x1
	}
	if x2 < minX {
		minX = x2
	}
	if x3 < minX {
		minX = x3
	}

	maxX := x0
	if x1 > maxX {
		maxX = x1
	}
	if x2 > maxX {
		maxX = x2
	}
	if x3 > maxX {
		maxX = x3
	}

	minY := y0
	if y1 < minY {
		minY = y1
	}
	if y2 < minY {
		minY = y2
	}
	if y3 < minY {
		minY = y3
	}

	maxY := y0
	if y1 > maxY {
		maxY = y1
	}
	if y2 > maxY {
		maxY = y2
	}
	if y3 > maxY {
		maxY = y3
	}

	return RectF{X0: minX, Y0: minY, X1: maxX, Y1: maxY}
}

// Inverse returns the inverse of the matrix, used for hit testing (mapping
// device coordinates back to element-local coordinates). If the matrix is
// singular (determinant is zero), the identity is returned.
func (m Matrix3x2) Inverse() Matrix3x2 {
	det := m.A*m.D - m.B*m.C
	if det == 0 {
		return MatrixIdentity
	}
	invDet := 1.0 / det
	return Matrix3x2{
		A:  m.D * invDet,
		B:  -m.B * invDet,
		C:  -m.C * invDet,
		D:  m.A * invDet,
		Tx: (m.C*m.Ty - m.D*m.Tx) * invDet,
		Ty: (m.B*m.Tx - m.A*m.Ty) * invDet,
	}
}

// IsIdentity reports whether the matrix is the identity transform, within a
// small tolerance for floating-point rounding.
func (m Matrix3x2) IsIdentity() bool {
	const eps float32 = 1e-6
	return abs32(m.A-1) < eps && abs32(m.B) < eps &&
		abs32(m.C) < eps && abs32(m.D-1) < eps &&
		abs32(m.Tx) < eps && abs32(m.Ty) < eps
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
