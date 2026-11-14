package paint

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/frame"
)

func ResolveTransform(funcs []css.TransformFunc, origin css.TransformOrigin, elementWidth, elementHeight float32) frame.Matrix3x2 {
	if len(funcs) == 0 {
		return frame.MatrixIdentity
	}

	ox, oy := origin.ResolveOrigin(elementWidth, elementHeight)

	result := frame.MatrixTranslate(ox, oy)

	for _, fn := range funcs {
		m := resolveOneFunc(fn)
		result = result.Multiply(m)
	}

	result = result.Multiply(frame.MatrixTranslate(-ox, -oy))

	return result
}

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
