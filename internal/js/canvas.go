package js

import (
	"strconv"
	"strings"
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
	mu                       sync.Mutex
	width                    int
	height                   int
	fillStyle                string
	strokeStyle              string
	lineWidth                float64
	font                     string
	textAlign                string
	textBaseline             string
	imageSmoothingEnabled    bool
	imageSmoothingQuality    string
	filter                   string
	globalAlpha              float64
	globalCompositeOperation string
	shadowBlur               float64
	shadowColor              string
	shadowOffsetX            float64
	shadowOffsetY            float64
	lineCap                  string
	lineJoin                 string
	miterLimit               float64
	ops                      []canvasDrawOp
	nodeID                   int // DOM node ID of the canvas element, 0 if unbound
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
	if node.ID != 0 {
		if ctx, exists := r.canvasContexts[int(node.ID)]; exists {
			r.mu.Unlock()
			return r.vm.ToValue(ctx)
		}
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
	if node.ID != 0 && int(node.ID) != nodeID {
		r.BindCanvasContext(int(node.ID), ctx)
	}

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

	_ = obj.Set("roundRect", func(call goja.FunctionCall) goja.Value {
		args := parseFloatArgs(call, 0, 0, 0, 0)
		var radii []interface{}
		if len(call.Arguments) >= 5 {
			arg := call.Arguments[4]
			if arr, ok := arg.Export().([]interface{}); ok {
				radii = arr
			} else {
				radii = []interface{}{arg.ToFloat()}
			}
		} else {
			radii = []interface{}{0.0}
		}
		ctx.mu.Lock()
		ctx.ops = append(ctx.ops, canvasDrawOp{
			Method: "roundRect",
			Args:   []interface{}{args[0], args[1], args[2], args[3], radii},
		})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("bezierCurveTo", func(call goja.FunctionCall) goja.Value {
		args := parseFloatArgs(call, 0, 0, 0, 0, 0, 0)
		ctx.mu.Lock()
		ctx.ops = append(ctx.ops, canvasDrawOp{
			Method: "bezierCurveTo",
			Args:   []interface{}{args[0], args[1], args[2], args[3], args[4], args[5]},
		})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("quadraticCurveTo", func(call goja.FunctionCall) goja.Value {
		args := parseFloatArgs(call, 0, 0, 0, 0)
		ctx.mu.Lock()
		ctx.ops = append(ctx.ops, canvasDrawOp{
			Method: "quadraticCurveTo",
			Args:   []interface{}{args[0], args[1], args[2], args[3]},
		})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("ellipse", func(call goja.FunctionCall) goja.Value {
		args := parseFloatArgs(call, 0, 0, 0, 0, 0, 0, 0)
		ccw := false
		if len(call.Arguments) >= 8 {
			ccw = call.Arguments[7].ToBoolean()
		}
		ctx.mu.Lock()
		ctx.ops = append(ctx.ops, canvasDrawOp{
			Method: "ellipse",
			Args:   []interface{}{args[0], args[1], args[2], args[3], args[4], args[5], args[6], ccw},
		})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	// Drawing methods
	_ = obj.Set("fill", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		if len(call.Arguments) > 0 {
			if pathObj, ok := call.Arguments[0].(*goja.Object); ok {
				if pOpsVal := pathObj.Get("__path_ops__"); pOpsVal != nil && !goja.IsUndefined(pOpsVal) {
					if fn, ok := goja.AssertFunction(pOpsVal); ok {
						if res, err := fn(pathObj); err == nil {
							if pOps, ok := res.Export().([]canvasDrawOp); ok {
								ctx.ops = append(ctx.ops, pOps...)
							}
						}
					}
				}
			}
		}
		ctx.ops = append(ctx.ops, canvasDrawOp{Method: "fill"})
		return goja.Undefined()
	})

	_ = obj.Set("stroke", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		if len(call.Arguments) > 0 {
			if pathObj, ok := call.Arguments[0].(*goja.Object); ok {
				if pOpsVal := pathObj.Get("__path_ops__"); pOpsVal != nil && !goja.IsUndefined(pOpsVal) {
					if fn, ok := goja.AssertFunction(pOpsVal); ok {
						if res, err := fn(pathObj); err == nil {
							if pOps, ok := res.Export().([]canvasDrawOp); ok {
								ctx.ops = append(ctx.ops, pOps...)
							}
						}
					}
				}
			}
		}
		ctx.ops = append(ctx.ops, canvasDrawOp{Method: "stroke"})
		return goja.Undefined()
	})

	_ = obj.Set("clip", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		rule := "nonzero"
		args := []interface{}{}
		if len(call.Arguments) > 0 {
			if pathObj, ok := call.Arguments[0].(*goja.Object); ok {
				if pOpsVal := pathObj.Get("__path_ops__"); pOpsVal != nil && !goja.IsUndefined(pOpsVal) {
					if fn, ok := goja.AssertFunction(pOpsVal); ok {
						if res, err := fn(pathObj); err == nil {
							if pOps, ok := res.Export().([]canvasDrawOp); ok {
								ctx.ops = append(ctx.ops, pOps...)
							}
						}
					}
				}
				if len(call.Arguments) > 1 {
					rule = call.Arguments[1].String()
				}
			} else {
				rule = call.Arguments[0].String()
			}
		}
		args = append(args, rule)
		ctx.ops = append(ctx.ops, canvasDrawOp{Method: "clip", Args: args})
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
		args := []interface{}{text, x, y}
		if len(call.Arguments) >= 4 {
			args = append(args, call.Arguments[3].ToFloat())
		}
		ctx.mu.Lock()
		ctx.ops = append(ctx.ops, canvasDrawOp{
			Method: "fillText",
			Args:   args,
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
		args := []interface{}{text, x, y}
		if len(call.Arguments) >= 4 {
			args = append(args, call.Arguments[3].ToFloat())
		}
		ctx.mu.Lock()
		ctx.ops = append(ctx.ops, canvasDrawOp{
			Method: "strokeText",
			Args:   args,
		})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("measureText", func(call goja.FunctionCall) goja.Value {
		text := ""
		if len(call.Arguments) >= 1 {
			text = call.Arguments[0].String()
		}
		fontSize := 10.0
		if fontVal := obj.Get("font"); fontVal != nil && !goja.IsUndefined(fontVal) {
			fs := fontVal.String()
			for _, part := range strings.Fields(fs) {
				if strings.HasSuffix(part, "px") {
					if n, err := strconv.ParseFloat(strings.TrimSuffix(part, "px"), 64); err == nil && n > 0 {
						fontSize = n
						break
					}
				}
			}
		}
		width := float64(len([]rune(text))) * (fontSize * 0.6)
		res := r.vm.NewObject()
		_ = res.Set("width", width)

		align := "start"
		if alignVal := obj.Get("textAlign"); alignVal != nil && !goja.IsUndefined(alignVal) {
			align = strings.ToLower(alignVal.String())
		}
		var left, right float64
		switch align {
		case "center":
			left = -width / 2.0
			right = width / 2.0
		case "right", "end":
			left = -width
			right = 0.0
		default: // "left", "start"
			left = 0.0
			right = width
		}

		// Standard TextMetrics font and bounding box metrics
		_ = res.Set("actualBoundingBoxAscent", fontSize*0.8)
		_ = res.Set("actualBoundingBoxDescent", fontSize*0.2)
		_ = res.Set("fontBoundingBoxAscent", fontSize*0.85)
		_ = res.Set("fontBoundingBoxDescent", fontSize*0.25)
		_ = res.Set("actualBoundingBoxLeft", left)
		_ = res.Set("actualBoundingBoxRight", right)
		_ = res.Set("emHeightAscent", fontSize*0.8)
		_ = res.Set("emHeightDescent", fontSize*0.2)
		_ = res.Set("alphabeticBaseline", 0.0)
		return res
	})

	// Gradient methods
	_ = obj.Set("createLinearGradient", func(call goja.FunctionCall) goja.Value {
		args := parseFloatArgs(call, 0, 0, 0, 0)
		return r.newCanvasGradient("linear", args)
	})

	_ = obj.Set("createRadialGradient", func(call goja.FunctionCall) goja.Value {
		args := parseFloatArgs(call, 0, 0, 0, 0, 0, 0)
		return r.newCanvasGradient("radial", args)
	})

	_ = obj.Set("createConicGradient", func(call goja.FunctionCall) goja.Value {
		args := parseFloatArgs(call, 0, 0, 0)
		return r.newCanvasGradient("conic", args)
	})

	_ = obj.Set("createPattern", func(call goja.FunctionCall) goja.Value {
		repetition := "repeat"
		if len(call.Arguments) >= 2 {
			repetition = strings.ToLower(call.Arguments[1].String())
		}
		var imageObj interface{}
		if len(call.Arguments) >= 1 {
			imageObj = call.Arguments[0].Export()
		}
		return r.newCanvasPattern(imageObj, repetition)
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

	_ = obj.Set("reset", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		ctx.fillStyle = "#000000"
		ctx.strokeStyle = "#000000"
		ctx.lineWidth = 1.0
		ctx.font = "10px sans-serif"
		ctx.textAlign = "start"
		ctx.textBaseline = "alphabetic"
		ctx.imageSmoothingEnabled = true
		ctx.imageSmoothingQuality = "low"
		ctx.filter = "none"
		ctx.globalAlpha = 1.0
		ctx.globalCompositeOperation = "source-over"
		ctx.shadowBlur = 0.0
		ctx.shadowColor = "rgba(0, 0, 0, 0)"
		ctx.shadowOffsetX = 0.0
		ctx.shadowOffsetY = 0.0
		ctx.lineCap = "butt"
		ctx.lineJoin = "miter"
		ctx.miterLimit = 10.0
		ctx.ops = append(ctx.ops, canvasDrawOp{Method: "reset"})
		ctx.mu.Unlock()

		_ = obj.Set("fillStyle", "#000000")
		_ = obj.Set("strokeStyle", "#000000")
		_ = obj.Set("lineWidth", 1.0)
		_ = obj.Set("font", "10px sans-serif")
		_ = obj.Set("textAlign", "start")
		_ = obj.Set("textBaseline", "alphabetic")
		_ = obj.Set("imageSmoothingEnabled", true)
		_ = obj.Set("imageSmoothingQuality", "low")
		_ = obj.Set("filter", "none")
		_ = obj.Set("globalAlpha", 1.0)
		_ = obj.Set("globalCompositeOperation", "source-over")
		_ = obj.Set("shadowBlur", 0.0)
		_ = obj.Set("shadowColor", "rgba(0, 0, 0, 0)")
		_ = obj.Set("shadowOffsetX", 0.0)
		_ = obj.Set("shadowOffsetY", 0.0)
		_ = obj.Set("lineCap", "butt")
		_ = obj.Set("lineJoin", "miter")
		_ = obj.Set("miterLimit", 10.0)
		_ = obj.Set("lineDashOffset", 0.0)
		return goja.Undefined()
	})

	// State and smoothing properties
	_ = obj.Set("imageSmoothingEnabled", true)
	_ = obj.Set("imageSmoothingQuality", "low")
	_ = obj.Set("filter", "none")
	_ = obj.Set("globalAlpha", 1.0)
	_ = obj.Set("globalCompositeOperation", "source-over")
	_ = obj.Set("shadowBlur", 0.0)
	_ = obj.Set("shadowColor", "rgba(0, 0, 0, 0)")
	_ = obj.Set("shadowOffsetX", 0.0)
	_ = obj.Set("shadowOffsetY", 0.0)
	_ = obj.Set("lineCap", "butt")
	_ = obj.Set("lineJoin", "miter")
	_ = obj.Set("miterLimit", 10.0)
	_ = obj.Set("lineDashOffset", 0.0)

	var lineDash []float64
	_ = obj.Set("setLineDash", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			if arr, ok := call.Arguments[0].Export().([]interface{}); ok {
				lineDash = make([]float64, 0, len(arr))
				for _, v := range arr {
					switch n := v.(type) {
					case int64:
						lineDash = append(lineDash, float64(n))
					case float64:
						lineDash = append(lineDash, n)
					}
				}
			}
		}
		return goja.Undefined()
	})

	_ = obj.Set("getLineDash", func(call goja.FunctionCall) goja.Value {
		res := make([]float64, len(lineDash))
		copy(res, lineDash)
		return r.vm.ToValue(res)
	})

	// Image drawing
	_ = obj.Set("drawImage", func(call goja.FunctionCall) goja.Value {
		args := make([]interface{}, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}
		ctx.mu.Lock()
		ctx.ops = append(ctx.ops, canvasDrawOp{
			Method: "drawImage",
			Args:   args,
		})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	// Pixel manipulation
	_ = obj.Set("createImageData", func(call goja.FunctionCall) goja.Value {
		w, h := 1, 1
		if len(call.Arguments) >= 2 {
			w = int(call.Arguments[0].ToInteger())
			h = int(call.Arguments[1].ToInteger())
		} else if len(call.Arguments) == 1 {
			if o, ok := call.Arguments[0].(*goja.Object); ok {
				if wv := o.Get("width"); wv != nil {
					w = int(wv.ToInteger())
				}
				if hv := o.Get("height"); hv != nil {
					h = int(hv.ToInteger())
				}
			}
		}
		if w <= 0 {
			w = 1
		}
		if h <= 0 {
			h = 1
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
		if w <= 0 {
			w = 1
		}
		if h <= 0 {
			h = 1
		}
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
		_ = obj.Set("imageSmoothingEnabled", true)
		_ = obj.Set("imageSmoothingQuality", "low")
		_ = obj.Set("filter", "none")
		_ = obj.Set("globalAlpha", 1.0)
		_ = obj.Set("globalCompositeOperation", "source-over")
		_ = obj.Set("shadowBlur", 0.0)
		_ = obj.Set("shadowColor", "rgba(0, 0, 0, 0)")
		_ = obj.Set("shadowOffsetX", 0.0)
		_ = obj.Set("shadowOffsetY", 0.0)
		_ = obj.Set("lineCap", "butt")
		_ = obj.Set("lineJoin", "miter")
		_ = obj.Set("miterLimit", 10.0)
		_ = obj.Set("lineDashOffset", 0.0)

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

		_ = obj.Set("roundRect", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0)
			var radii []interface{}
			if len(call.Arguments) >= 5 {
				arg := call.Arguments[4]
				if arr, ok := arg.Export().([]interface{}); ok {
					radii = arr
				} else {
					radii = []interface{}{arg.ToFloat()}
				}
			} else {
				radii = []interface{}{0.0}
			}
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "roundRect",
				Args:   []interface{}{args[0], args[1], args[2], args[3], radii},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("bezierCurveTo", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0, 0, 0)
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "bezierCurveTo",
				Args:   []interface{}{args[0], args[1], args[2], args[3], args[4], args[5]},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("quadraticCurveTo", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0)
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "quadraticCurveTo",
				Args:   []interface{}{args[0], args[1], args[2], args[3]},
			})
			ctx.mu.Unlock()
			return goja.Undefined()
		})

		_ = obj.Set("ellipse", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0, 0, 0, 0)
			ccw := false
			if len(call.Arguments) >= 8 {
				ccw = call.Arguments[7].ToBoolean()
			}
			ctx.mu.Lock()
			ctx.ops = append(ctx.ops, canvasDrawOp{
				Method: "ellipse",
				Args:   []interface{}{args[0], args[1], args[2], args[3], args[4], args[5], args[6], ccw},
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

		_ = obj.Set("clip", func(call goja.FunctionCall) goja.Value {
			ctx.mu.Lock()
			defer ctx.mu.Unlock()
			rule := "nonzero"
			args := []interface{}{}
			if len(call.Arguments) > 0 {
				if pathObj, ok := call.Arguments[0].(*goja.Object); ok {
					if pOpsVal := pathObj.Get("__path_ops__"); pOpsVal != nil && !goja.IsUndefined(pOpsVal) {
						if fn, ok := goja.AssertFunction(pOpsVal); ok {
							if res, err := fn(pathObj); err == nil {
								if pOps, ok := res.Export().([]canvasDrawOp); ok {
									ctx.ops = append(ctx.ops, pOps...)
								}
							}
						}
					}
					if len(call.Arguments) > 1 {
						rule = call.Arguments[1].String()
					}
				} else {
					rule = call.Arguments[0].String()
				}
			}
			args = append(args, rule)
			ctx.ops = append(ctx.ops, canvasDrawOp{Method: "clip", Args: args})
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

		_ = obj.Set("reset", func(call goja.FunctionCall) goja.Value {
			ctx.mu.Lock()
			ctx.fillStyle = "#000000"
			ctx.strokeStyle = "#000000"
			ctx.lineWidth = 1.0
			ctx.font = "10px sans-serif"
			ctx.textAlign = "start"
			ctx.textBaseline = "alphabetic"
			ctx.imageSmoothingEnabled = true
			ctx.imageSmoothingQuality = "low"
			ctx.filter = "none"
			ctx.globalAlpha = 1.0
			ctx.globalCompositeOperation = "source-over"
			ctx.shadowBlur = 0.0
			ctx.shadowColor = "rgba(0, 0, 0, 0)"
			ctx.shadowOffsetX = 0.0
			ctx.shadowOffsetY = 0.0
			ctx.lineCap = "butt"
			ctx.lineJoin = "miter"
			ctx.miterLimit = 10.0
			ctx.ops = append(ctx.ops, canvasDrawOp{Method: "reset"})
			ctx.mu.Unlock()

			_ = obj.Set("fillStyle", "#000000")
			_ = obj.Set("strokeStyle", "#000000")
			_ = obj.Set("lineWidth", 1.0)
			_ = obj.Set("font", "10px sans-serif")
			_ = obj.Set("textAlign", "start")
			_ = obj.Set("textBaseline", "alphabetic")
			_ = obj.Set("imageSmoothingEnabled", true)
			_ = obj.Set("imageSmoothingQuality", "low")
			_ = obj.Set("filter", "none")
			_ = obj.Set("globalAlpha", 1.0)
			_ = obj.Set("globalCompositeOperation", "source-over")
			_ = obj.Set("shadowBlur", 0.0)
			_ = obj.Set("shadowColor", "rgba(0, 0, 0, 0)")
			_ = obj.Set("shadowOffsetX", 0.0)
			_ = obj.Set("shadowOffsetY", 0.0)
			_ = obj.Set("lineCap", "butt")
			_ = obj.Set("lineJoin", "miter")
			_ = obj.Set("miterLimit", 10.0)
			_ = obj.Set("lineDashOffset", 0.0)
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

		var lineDash []float64
		_ = obj.Set("setLineDash", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) > 0 {
				if arr, ok := call.Arguments[0].Export().([]interface{}); ok {
					lineDash = make([]float64, 0, len(arr))
					for _, v := range arr {
						switch n := v.(type) {
						case int64:
							lineDash = append(lineDash, float64(n))
						case float64:
							lineDash = append(lineDash, n)
						}
					}
				}
			}
			return goja.Undefined()
		})

		_ = obj.Set("getLineDash", func(call goja.FunctionCall) goja.Value {
			res := make([]float64, len(lineDash))
			copy(res, lineDash)
			return r.vm.ToValue(res)
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

	r.setupPath2D()
	r.setupCanvasGradient()
	r.setupCanvasPattern()
}

type canvasGradientData struct {
	mu    sync.Mutex
	kind  string // "linear", "radial", "conic"
	args  []float64
	stops []gradientStop
}

type gradientStop struct {
	offset float64
	color  string
}

func (r *Runtime) newCanvasGradient(kind string, args []float64) *goja.Object {
	gData := &canvasGradientData{
		kind: kind,
		args: args,
	}
	obj := r.vm.NewObject()
	_ = obj.Set("__is_gradient__", true)
	_ = obj.Set("__kind__", kind)
	_ = obj.Set("addColorStop", func(call goja.FunctionCall) goja.Value {
		offset := 0.0
		color := ""
		if len(call.Arguments) >= 1 {
			offset = call.Arguments[0].ToFloat()
		}
		if len(call.Arguments) >= 2 {
			color = call.Arguments[1].String()
		}
		gData.mu.Lock()
		gData.stops = append(gData.stops, gradientStop{offset: offset, color: color})
		gData.mu.Unlock()
		return goja.Undefined()
	})
	return obj
}

func (r *Runtime) setupCanvasGradient() {
	ctor := func(call goja.ConstructorCall) *goja.Object {
		return r.newCanvasGradient("linear", nil)
	}
	ctorVal := r.vm.ToValue(ctor)
	_ = r.vm.Set("CanvasGradient", ctorVal)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("CanvasGradient", ctorVal)
		}
	}
}

type canvasPatternData struct {
	mu         sync.Mutex
	image      interface{}
	repetition string
	matrix     []float64
}

func (r *Runtime) newCanvasPattern(image interface{}, repetition string) *goja.Object {
	pData := &canvasPatternData{
		image:      image,
		repetition: repetition,
	}
	obj := r.vm.NewObject()
	_ = obj.Set("__is_pattern__", true)
	_ = obj.Set("setTransform", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 1 {
			if domMatrix, ok := call.Arguments[0].Export().([]float64); ok {
				pData.mu.Lock()
				pData.matrix = domMatrix
				pData.mu.Unlock()
			}
		}
		return goja.Undefined()
	})
	return obj
}

func (r *Runtime) setupCanvasPattern() {
	ctor := func(call goja.ConstructorCall) *goja.Object {
		return r.newCanvasPattern(nil, "repeat")
	}
	ctorVal := r.vm.ToValue(ctor)
	_ = r.vm.Set("CanvasPattern", ctorVal)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("CanvasPattern", ctorVal)
		}
	}
}

type path2DData struct {
	mu  sync.Mutex
	ops []canvasDrawOp
}

func (r *Runtime) setupPath2D() {
	path2DCtor := func(call goja.ConstructorCall) *goja.Object {
		pData := &path2DData{}
		if len(call.Arguments) > 0 {
			if existing, ok := call.Arguments[0].(*goja.Object); ok {
				if pOpsVal := existing.Get("__path_ops__"); pOpsVal != nil && !goja.IsUndefined(pOpsVal) {
					if fn, ok := goja.AssertFunction(pOpsVal); ok {
						if res, err := fn(existing); err == nil {
							if existingOps, ok := res.Export().([]canvasDrawOp); ok {
								pData.ops = append(pData.ops, existingOps...)
							}
						}
					}
				}
			}
		}

		obj := call.This
		_ = obj.Set("__is_path2d__", true)
		_ = obj.Set("__path_ops__", func() []canvasDrawOp {
			pData.mu.Lock()
			defer pData.mu.Unlock()
			cp := make([]canvasDrawOp, len(pData.ops))
			copy(cp, pData.ops)
			return cp
		})

		_ = obj.Set("beginPath", func(call goja.FunctionCall) goja.Value {
			pData.mu.Lock()
			pData.ops = append(pData.ops, canvasDrawOp{Method: "beginPath"})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("closePath", func(call goja.FunctionCall) goja.Value {
			pData.mu.Lock()
			pData.ops = append(pData.ops, canvasDrawOp{Method: "closePath"})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("moveTo", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0)
			pData.mu.Lock()
			pData.ops = append(pData.ops, canvasDrawOp{
				Method: "moveTo",
				Args:   []interface{}{args[0], args[1]},
			})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("lineTo", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0)
			pData.mu.Lock()
			pData.ops = append(pData.ops, canvasDrawOp{
				Method: "lineTo",
				Args:   []interface{}{args[0], args[1]},
			})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("arc", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0, 0)
			ccw := false
			if len(call.Arguments) >= 6 {
				ccw = call.Arguments[5].ToBoolean()
			}
			pData.mu.Lock()
			pData.ops = append(pData.ops, canvasDrawOp{
				Method: "arc",
				Args:   []interface{}{args[0], args[1], args[2], args[3], args[4], ccw},
			})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("rect", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0)
			pData.mu.Lock()
			pData.ops = append(pData.ops, canvasDrawOp{
				Method: "rect",
				Args:   []interface{}{args[0], args[1], args[2], args[3]},
			})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("roundRect", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0)
			var radii []interface{}
			if len(call.Arguments) >= 5 {
				arg := call.Arguments[4]
				if arr, ok := arg.Export().([]interface{}); ok {
					radii = arr
				} else {
					radii = []interface{}{arg.ToFloat()}
				}
			} else {
				radii = []interface{}{0.0}
			}
			pData.mu.Lock()
			pData.ops = append(pData.ops, canvasDrawOp{
				Method: "roundRect",
				Args:   []interface{}{args[0], args[1], args[2], args[3], radii},
			})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("bezierCurveTo", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0, 0, 0)
			pData.mu.Lock()
			pData.ops = append(pData.ops, canvasDrawOp{
				Method: "bezierCurveTo",
				Args:   []interface{}{args[0], args[1], args[2], args[3], args[4], args[5]},
			})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("quadraticCurveTo", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0)
			pData.mu.Lock()
			pData.ops = append(pData.ops, canvasDrawOp{
				Method: "quadraticCurveTo",
				Args:   []interface{}{args[0], args[1], args[2], args[3]},
			})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("ellipse", func(call goja.FunctionCall) goja.Value {
			args := parseFloatArgs(call, 0, 0, 0, 0, 0, 0, 0)
			ccw := false
			if len(call.Arguments) >= 8 {
				ccw = call.Arguments[7].ToBoolean()
			}
			pData.mu.Lock()
			pData.ops = append(pData.ops, canvasDrawOp{
				Method: "ellipse",
				Args:   []interface{}{args[0], args[1], args[2], args[3], args[4], args[5], args[6], ccw},
			})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("addPath", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) > 0 {
				if other, ok := call.Arguments[0].(*goja.Object); ok {
					if pOpsVal := other.Get("__path_ops__"); pOpsVal != nil && !goja.IsUndefined(pOpsVal) {
						if fn, ok := goja.AssertFunction(pOpsVal); ok {
							if res, err := fn(other); err == nil {
								if ops, ok := res.Export().([]canvasDrawOp); ok {
									pData.mu.Lock()
									pData.ops = append(pData.ops, ops...)
									pData.mu.Unlock()
								}
							}
						}
					}
				}
			}
			return goja.Undefined()
		})

		return nil
	}

	ctorVal := r.vm.ToValue(path2DCtor)
	_ = r.vm.Set("Path2D", ctorVal)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("Path2D", ctorVal)
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
