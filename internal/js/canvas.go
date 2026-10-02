package js

import (
	"strconv"
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
	nodeID       int // DOM node ID of the canvas element, 0 if unbound
}

// CanvasOp is one recorded canvas drawing operation, exposed to the engine
// for replay into a bitmap.
type CanvasOp struct {
	Method string
	Args   []interface{}
}

// CanvasOps returns the recorded drawing operations for the canvas context
// bound to the given DOM node ID. Returns nil if no context is bound.
func (r *Runtime) CanvasOps(nodeID int) []CanvasOp {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.canvasContexts == nil {
		return nil
	}
	for _, ctx := range r.canvasContexts {
		if ctx.nodeID == nodeID {
			ctx.mu.Lock()
			ops := make([]CanvasOp, len(ctx.ops))
			for i, op := range ctx.ops {
				ops[i] = CanvasOp{Method: op.Method, Args: op.Args}
			}
			ctx.mu.Unlock()
			return ops
		}
	}
	return nil
}

// CanvasDimensions returns the width and height of the canvas context bound
// to the given DOM node ID. Returns 0, 0 if no context is bound.
func (r *Runtime) CanvasDimensions(nodeID int) (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.canvasContexts == nil {
		return 0, 0
	}
	for _, ctx := range r.canvasContexts {
		if ctx.nodeID == nodeID {
			return ctx.width, ctx.height
		}
	}
	return 0, 0
}

// BindCanvasContext associates a canvas context with a DOM node ID so the
// paint pipeline can find it. Called when getContext("2d") is invoked on a
// canvas element.
func (r *Runtime) BindCanvasContext(nodeID int, ctx *canvasContext2D) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.canvasContexts == nil {
		r.canvasContexts = make(map[int]*canvasContext2D)
	}
	ctx.nodeID = nodeID
	r.canvasContexts[nodeID] = ctx
}

// jsGetContext implements getContext(contextType) on DOM elements. For canvas
// elements with contextType="2d", it creates and returns a CanvasRenderingContext2D.
func (r *Runtime) jsGetContext(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return goja.Null()
	}
	contextType := call.Arguments[0].String()
	if contextType != "2d" {
		return goja.Null()
	}

	// Get the node ID from the this object
	thisObj, ok := call.This.(*goja.Object)
	if !ok {
		return goja.Null()
	}
	nidVal := thisObj.Get("__nid__")
	if nidVal == nil || goja.IsUndefined(nidVal) {
		return goja.Null()
	}
	nodeID := int(nidVal.ToInteger())

	// Look up the node to verify it's a canvas
	r.mu.Lock()
	node := r.nodeRegistry[nodeID]
	r.mu.Unlock()
	if node == nil || node.Data != "canvas" {
		return goja.Null()
	}

	// Check if we already have a context for this canvas
	r.mu.Lock()
	if ctx, exists := r.canvasContexts[nodeID]; exists {
		r.mu.Unlock()
		// Return the existing context wrapper
		return r.vm.ToValue(ctx)
	}
	r.mu.Unlock()

	// Create a new context
	width := 300
	height := 150
	if w := node.GetAttribute("width"); w != "" {
		if n, err := strconv.Atoi(w); err == nil && n > 0 {
			width = n
		}
	}
	if h := node.GetAttribute("height"); h != "" {
		if n, err := strconv.Atoi(h); err == nil && n > 0 {
			height = n
		}
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

	r.BindCanvasContext(nodeID, ctx)

	// Create the JS wrapper object
	obj := r.vm.NewObject()
	_ = obj.Set("canvas", call.This)
	_ = obj.Set("fillStyle", ctx.fillStyle)
	_ = obj.Set("strokeStyle", ctx.strokeStyle)
	_ = obj.Set("lineWidth", ctx.lineWidth)
	_ = obj.Set("font", ctx.font)
	_ = obj.Set("textAlign", ctx.textAlign)
	_ = obj.Set("textBaseline", ctx.textBaseline)

	// Add all the drawing methods
	r.setupCanvasContextMethods(obj, ctx)

	return obj
}

// setupCanvasContextMethods adds all the Canvas 2D API methods to a context wrapper.
func (r *Runtime) setupCanvasContextMethods(obj *goja.Object, ctx *canvasContext2D) {
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
