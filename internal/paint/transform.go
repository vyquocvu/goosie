// Transform resolution: converts parsed CSS transform functions into a single
// 2D affine matrix, applying the transform-origin.

package paint

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
)

// ResolveTransform converts a list of TransformFunc into a single Matrix3x2.
// The origin is applied as: translate(origin) * funcs * translate(-origin).
// elementWidth and elementHeight are used for resolving percentage-based
// translate values and the transform origin.
func ResolveTransform(funcs []css.TransformFunc, origin css.TransformOrigin, elementWidth, elementHeight float32) frame.Matrix3x2 {
	if len(funcs) == 0 {
		return frame.MatrixIdentity
	}

	ox, oy := origin.ResolveOrigin(elementWidth, elementHeight)

	// Start with translate(origin).
	result := frame.MatrixTranslate(ox, oy)

	// Multiply in each transform function.
	for _, fn := range funcs {
		m := resolveOneFunc(fn)
		result = result.Multiply(m)
	}

	// End with translate(-origin).
	result = result.Multiply(frame.MatrixTranslate(-ox, -oy))

	return result
}

// resolveOneFunc converts a single TransformFunc to a Matrix3x2.
func resolveOneFunc(fn css.TransformFunc) frame.Matrix3x2 {
	switch fn.Name {
	case "translate":
		return frame.MatrixTranslate(fn.Args[0], fn.Args[1])
	case "rotate":
		return frame.MatrixRotate(fn.Args[0])
	case "scale":
		return frame.MatrixScale(fn.Args[0], fn.Args[1])
	case "skew":
		return frame.MatrixSkew(fn.Args[0], fn.Args[1])
	case "matrix":
		return frame.Matrix3x2{
			A:  fn.Args[0],
			B:  fn.Args[1],
			C:  fn.Args[2],
			D:  fn.Args[3],
			Tx: fn.Args[4],
			Ty: fn.Args[5],
		}
	}
	return frame.MatrixIdentity
}
