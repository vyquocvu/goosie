package js

import (
	"sync"

	"github.com/dop251/goja"
)

// Canvas 2D API implementation.

// canvasDrawOp represents one drawing operation.
type canvasDrawOp struct {
	Method string
	Args   []interface{}
}

// canvasContext2D holds the internal state of one 2D rendering context.
type canvasContext2D struct {
	mu           sync.Mutex
	width        int
	height       int
	fillStyle    string
	strokeStyle  string
	lineWidth    float64
	font         string
	textAlign    string
	textBaseline string
	ops          []canvasDrawOp
}

// setupCanvas installs the Canvas 2D API.
func (r *Runtime) setupCanvas() {
	constructor := func(call goja.ConstructorCall) *goja.Object {
		width := 300
		height := 150

		if len(call.Arguments) >= 1 {
			width = int(call.Arguments[0].ToInteger())
		}
		if len(call.Arguments) >= 2 {
			height = int(call.Arguments[1].ToInteger())
		}

		ctx := &canvasContext2D{
			width:        width,
			height:       height,
			fillStyle:    "#000000",
			strokeStyle:  "#000000",
			lineWidth:    1.0,
			font:         "10px sans-serif",
			textAlign:    "start",
			textBaseline: "alphabetic",
		}

		obj := call.This

		// Properties
		_ = obj.Set("canvas", goja.Null())
		_ = obj.Set("fillStyle", ctx.fillStyle)
		_ = obj.Set("strokeStyle", ctx.strokeStyle)
		_ = obj.Set("lineWidth", ctx.lineWidth)
		_ = obj.Set("font", ctx.font)
		_ = obj.Set("textAlign", ctx.textAlign)
		_ = obj.Set("textBaseline", ctx.textBaseline)

		// Rect methods
		_ = obj.Set("fillRect", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0)
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "fillRect",
				Args:   []interface{}{args[0], args[1], args[2], args[3]},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("strokeRect", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0)
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "strokeRect",
				Args:   []interface{}{args[0], args[1], args[2], args[3]},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("clearRect", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0)
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "clearRect",
				Args:   []interface{}{args[0], args[1], args[2], args[3]},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		// Path methods
		_ = obj.Set("beginPath", func(call goja.FunctionCall) goja.Value {
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{Method: "beginPath"})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("closePath", func(call goja.FunctionCall) goja.Value {
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{Method: "closePath"})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("moveTo", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0)
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "moveTo",
				Args:   []interface{}{args[0], args[1]},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("lineTo", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0)
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "lineTo",
				Args:   []interface{}{args[0], args[1]},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("arc", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0, 0)
			counterclockwise := false
			if len(call.Arguments) >= 6 {
				counterclockwise = call.Arguments[5].ToBoolean()
			}
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "arc",
				Args:   []interface{}{args[0], args[1], args[2], args[3], args[4], counterclockwise},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("rect", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0)
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "rect",
				Args:   []interface{}{args[0], args[1], args[2], args[3]},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		// Drawing methods
		_ = obj.Set("fill", func(call goja.FunctionCall) goja.Value {
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{Method: "fill"})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("stroke", func(call goja.FunctionCall) goja.Value {
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{Method: "stroke"})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		// Text methods
		_ = obj.Set("fillText", func(call goja.FunctionCall) goja.Value {
			text := ""
			if len(call.Arguments) >= 1 {
				text = call.Arguments[0].String()
			}
			x, y := 0.0, 0.0
			if len(call.Arguments) >= 2 {
				x = call.Arguments[1].ToFloat()
			}
			if len(call.Arguments) >= 3 {
				y = call.Arguments[2].ToFloat()
			}
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "fillText",
				Args:   []interface{}{text, x, y},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("strokeText", func(call goja.FunctionCall) goja.Value {
			text := ""
			if len(call.Arguments) >= 1 {
				text = call.Arguments[0].String()
			}
			x, y := 0.0, 0.0
			if len(call.Arguments) >= 2 {
				x = call.Arguments[1].ToFloat()
			}
			if len(call.Arguments) >= 3 {
				y = call.Arguments[2].ToFloat()
			}
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "strokeText",
				Args:   []interface{}{text, x, y},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		// Transform methods
		_ = obj.Set("save", func(call goja.FunctionCall) goja.Value {
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{Method: "save"})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("restore", func(call goja.FunctionCall) goja.Value {
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{Method: "restore"})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("translate", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0)
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "translate",
				Args:   []interface{}{args[0], args[1]},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("rotate", func(call goja.FunctionCall) goja.Value {
			angle := 0.0
			if len(call.Arguments) >= 1 {
				angle = call.Arguments[0].ToFloat()
			}
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "rotate",
				Args:   []interface{}{angle},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("scale", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 1, 1)
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "scale",
				Args:   []interface{}{args[0], args[1]},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		// Image methods (stubs)
		_ = obj.Set("drawImage", func(call goja.FunctionCall) goja.Value {
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "drawImage",
				Args:   []interface{}{"image"},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		// Pixel manipulation (stubs)
		_ = obj.Set("createImageData", func(call goja.FunctionCall) goja.Value {
			w, h := 1, 1
			if len(call.Arguments) >= 1 {
				w = int(call.Arguments[0].ToInteger())
			}
			if len(call.Arguments) >= 2 {
				h = int(call.Arguments[1].ToInteger())
			}
			imgData := r.vm.NewObject()
			_ = imgData.Set("width", w)
			_ = imgData.Set("height", h)
			data := make([]int, w*h*4)
			_ = imgData.Set("data", data)
			return imgData
		})

		_ = obj.Set("getImageData", func(call goja.FunctionCall) goja.Value {
			args := parseIntArgs(call, 0, 0, 0, 0)
			x, y, w, h := args[0], args[1], args[2], args[3]
			imgData := r.vm.NewObject()
			_ = imgData.Set("width", w)
			_ = imgData.Set("height", h)
			data := make([]int, w*h*4)
			_ = imgData.Set("data", data)
			_ = imgData.Set("x", x)
			_ = imgData.Set("y", y)
			return imgData
		})

		_ = obj.Set("putImageData", func(call goja.FunctionCall) goja.Value {
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{Method: "putImageData"})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		return nil
	}

	ctor := r.vm.ToValue(constructor)
	_ = r.vm.Set("CanvasRenderingContext2D", ctor)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("CanvasRenderingContext2D", ctor)
		}
	}
}

func parseFloatArgs(call goja.FunctionCall, defaults ...float64) []float64 {
	result := make([]float64, len(defaults))
	copy(result, defaults)
	for i := range result {
		if i < len(call.Arguments) {
			result[i] = call.Arguments[i].ToFloat()
		}
	}
	return result
}

func parseIntArgs(call goja.FunctionCall, defaults ...int) []int {
	result := make([]int, len(defaults))
	copy(result, defaults)
	for i := range result {
		if i < len(call.Arguments) {
			result[i] = int(call.Arguments[i].ToInteger())
		}
	}
	return result
}
