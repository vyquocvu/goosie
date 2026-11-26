package js

import (
	"image"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

const (
	maxImageDataBytes        = 64 << 20
	maxCanvasOps             = 100000
	maxCanvasStateStack      = 64
	maxCanvasCoordinate      = 1 << 20
	maxCanvasBitmapDimension = 16384
	maxCanvasBitmapPixels    = 16 * 1024 * 1024
	canvasPaintMethod        = "canvasPaint"
)

type canvasDrawOp struct {
	Method string
	Args   []interface{}
}

type CanvasOp = canvasDrawOp

type canvasDrawState struct {
	fillStyle         string
	fillStyleObject   *goja.Object
	fillColor         [4]uint8
	strokeStyle       string
	strokeStyleObject *goja.Object
	strokeColor       [4]uint8
	globalAlpha       float64
	lineWidth         float64
	font              string
	matrix            [6]float64
}

type canvasPoint struct {
	x, y float64
}

type canvasSubpath struct {
	points []canvasPoint
	closed bool
}

type canvasPath struct {
	subpaths []canvasSubpath
}

type canvasContext2D struct {
	mu                       sync.Mutex
	width                    int
	height                   int
	fillStyle                string
	fillStyleObject          *goja.Object
	fillColor                [4]uint8
	strokeStyle              string
	strokeStyleObject        *goja.Object
	strokeColor              [4]uint8
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
	matrix                   [6]float64
	stateStack               []canvasDrawState
	path                     canvasPath
	ops                      []canvasDrawOp
	nodeID                   int
	bitmap                   []byte
	bitmapW                  int
	bitmapH                  int
	obj                      *goja.Object
}

func defaultCanvasDrawState() canvasDrawState {
	return canvasDrawState{
		fillStyle:   "#000000",
		fillColor:   [4]uint8{0, 0, 0, 255},
		strokeStyle: "#000000",
		strokeColor: [4]uint8{0, 0, 0, 255},
		globalAlpha: 1,
		lineWidth:   1,
		font:        "10px sans-serif",
		matrix:      [6]float64{1, 0, 0, 1, 0, 0},
	}
}

func (ctx *canvasContext2D) snapshotState() canvasDrawState {
	return canvasDrawState{
		fillStyle:         ctx.fillStyle,
		fillStyleObject:   ctx.fillStyleObject,
		fillColor:         ctx.fillColor,
		strokeStyle:       ctx.strokeStyle,
		strokeStyleObject: ctx.strokeStyleObject,
		strokeColor:       ctx.strokeColor,
		globalAlpha:       ctx.globalAlpha,
		lineWidth:         ctx.lineWidth,
		font:              ctx.font,
		matrix:            ctx.matrix,
	}
}

func (ctx *canvasContext2D) restoreState(state canvasDrawState) {
	ctx.fillStyle = state.fillStyle
	ctx.fillStyleObject = state.fillStyleObject
	ctx.fillColor = state.fillColor
	ctx.strokeStyle = state.strokeStyle
	ctx.strokeStyleObject = state.strokeStyleObject
	ctx.strokeColor = state.strokeColor
	ctx.globalAlpha = state.globalAlpha
	ctx.lineWidth = state.lineWidth
	ctx.font = state.font
	ctx.matrix = state.matrix
}

func (ctx *canvasContext2D) resetState() {
	ctx.restoreState(defaultCanvasDrawState())
	ctx.textAlign = "start"
	ctx.textBaseline = "alphabetic"
	ctx.imageSmoothingEnabled = true
	ctx.imageSmoothingQuality = "low"
	ctx.filter = "none"
	ctx.globalCompositeOperation = "source-over"
	ctx.shadowBlur = 0
	ctx.shadowColor = "rgba(0, 0, 0, 0)"
	ctx.shadowOffsetX = 0
	ctx.shadowOffsetY = 0
	ctx.lineCap = "butt"
	ctx.lineJoin = "miter"
	ctx.miterLimit = 10
	ctx.stateStack = nil
	ctx.path = canvasPath{}
}

func canvasFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= maxCanvasCoordinate
}

func canvasMatrixFinite(m [6]float64) bool {
	for _, v := range m {
		if !canvasFinite(v) {
			return false
		}
	}
	return true
}

func canvasTransformPoint(m [6]float64, x, y float64) canvasPoint {
	return canvasPoint{x: m[0]*x + m[2]*y + m[4], y: m[1]*x + m[3]*y + m[5]}
}

func canvasBitmapFits(width, height int) bool {
	if width <= 0 || height <= 0 || width > maxCanvasBitmapDimension || height > maxCanvasBitmapDimension {
		return false
	}
	return int64(width)*int64(height) <= maxCanvasBitmapPixels
}

func (r *Runtime) setupImageData() {
	prototype := r.vm.NewObject()
	constructor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to construct 'ImageData': 1 argument required"))
		}
		clampedArrayCtor, hasClampedArrayCtor := r.vm.Get("Uint8ClampedArray").(*goja.Object)
		if source, ok := call.Arguments[0].(*goja.Object); ok && hasClampedArrayCtor && r.vm.InstanceOf(source, clampedArrayCtor) {
			if len(call.Arguments) < 2 {
				panic(r.vm.NewTypeError("Failed to construct 'ImageData': width is required"))
			}
			width := int(call.Arguments[1].ToInteger())
			if width <= 0 {
				panic(r.domError("IndexSizeError", "ImageData width must be positive"))
			}
			length := int(source.Get("length").ToInteger())
			if length <= 0 || length%(width*4) != 0 {
				panic(r.domError("IndexSizeError", "ImageData data length does not match its dimensions"))
			}
			height := length / (width * 4)
			if len(call.Arguments) > 2 && !goja.IsUndefined(call.Arguments[2]) && int(call.Arguments[2].ToInteger()) != height {
				panic(r.domError("IndexSizeError", "ImageData data length does not match its dimensions"))
			}
			return r.newImageData(width, height, source)
		}
		width := int(call.Arguments[0].ToInteger())
		height := 1
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) {
			height = int(call.Arguments[1].ToInteger())
		}
		if width <= 0 || height <= 0 {
			panic(r.domError("IndexSizeError", "ImageData dimensions must be positive"))
		}
		if width > math.MaxInt/4 || height > maxImageDataBytes/(width*4) {
			panic(r.vm.NewTypeError("ImageData dimensions exceed the implementation limit"))
		}
		data := r.newUint8ClampedArray(width * height * 4)
		return r.newImageData(width, height, data)
	}).(*goja.Object)
	_ = constructor.Set("prototype", prototype)
	_ = prototype.Set("constructor", constructor)
	r.defineInterfaceToStringTag("ImageData", prototype)
	r.imageDataProto = prototype
	_ = r.vm.Set("ImageData", constructor)
	if window, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = window.Set("ImageData", constructor)
	}
}

func (r *Runtime) newUint8ClampedArray(length int) *goja.Object {
	if length < 0 || length > maxImageDataBytes {
		panic(r.vm.NewTypeError("ImageData data exceeds the implementation limit"))
	}
	constructor, ok := goja.AssertConstructor(r.vm.Get("Uint8ClampedArray"))
	if !ok {
		panic(r.vm.NewTypeError("Uint8ClampedArray is unavailable"))
	}
	array, err := constructor(nil, r.vm.ToValue(length))
	if err != nil {
		panic(err)
	}
	return array
}

func (r *Runtime) validateImageDataRegion(width, height int) {
	if width <= 0 || height <= 0 || width > math.MaxInt/4 || height > maxImageDataBytes/(width*4) {
		panic(r.domError("IndexSizeError", "ImageData dimensions are invalid or exceed the implementation limit"))
	}
}

func (r *Runtime) newImageData(width, height int, data *goja.Object) *goja.Object {
	r.validateImageDataRegion(width, height)
	if data == nil {
		data = r.newUint8ClampedArray(width * height * 4)
	}
	obj := r.vm.NewObject()
	if r.imageDataProto != nil {
		_ = obj.SetPrototype(r.imageDataProto)
	}
	_ = obj.DefineDataProperty("width", r.vm.ToValue(width), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineDataProperty("height", r.vm.ToValue(height), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineDataProperty("data", data, goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	if r.imageDataObjects == nil {
		r.imageDataObjects = make(map[*goja.Object]struct{})
	}
	r.imageDataObjects[obj] = struct{}{}
	return obj
}

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

func (r *Runtime) CanvasVisualStamp() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	var stamp uint64
	for nodeID, ctx := range r.canvasContexts {
		if ctx == nil {
			continue
		}
		ctx.mu.Lock()
		width, height, opCount := ctx.width, ctx.height, len(ctx.ops)
		ctx.mu.Unlock()

		value := uint64(uint32(nodeID))<<32 | uint64(uint32(width))
		value ^= uint64(uint32(height)) * 0x9e3779b185ebca87
		value ^= uint64(opCount) * 0xc2b2ae3d27d4eb4f
		value ^= value >> 30
		value *= 0xbf58476d1ce4e5b9
		value ^= value >> 27
		value *= 0x94d049bb133111eb
		value ^= value >> 31
		stamp += value
	}
	return stamp
}

func (r *Runtime) CanvasDimensions(nodeID int) (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.canvasContexts == nil {
		return 0, 0
	}
	for _, ctx := range r.canvasContexts {
		if ctx.nodeID == nodeID {
			ctx.mu.Lock()
			width, height := ctx.width, ctx.height
			ctx.mu.Unlock()
			return width, height
		}
	}
	return 0, 0
}

func (r *Runtime) BindCanvasContext(nodeID int, ctx *canvasContext2D) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.canvasContexts == nil {
		r.canvasContexts = make(map[int]*canvasContext2D)
	}
	ctx.nodeID = nodeID
	r.canvasContexts[nodeID] = ctx
}

func (r *Runtime) SetCanvasBitmap(nodeID int, rgba []byte, w, h int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.canvasContexts == nil {
		return
	}
	ctx, ok := r.canvasContexts[nodeID]
	if !ok || ctx == nil {
		return
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.bitmap = make([]byte, len(rgba))
	copy(ctx.bitmap, rgba)
	ctx.bitmapW = w
	ctx.bitmapH = h
}

const (
	defaultCanvasWidth  = 300
	defaultCanvasHeight = 150
)

func canvasNodeDimension(node *dom.Node, attr string, def int) int {
	if v, ok := parseHTMLUnsigned(node.GetAttribute(attr)); ok {
		return int(v)
	}
	return def
}

func isCanvasDimensionAttribute(node *dom.Node, name string) bool {
	if node == nil || node.Data != "canvas" {
		return false
	}
	local := domAttributeLocalName(name)
	return strings.EqualFold(local, "width") || strings.EqualFold(local, "height")
}

func (r *Runtime) resetCanvasForAttribute(node *dom.Node) {
	if node == nil || node.Data != "canvas" {
		return
	}
	width := canvasNodeDimension(node, "width", defaultCanvasWidth)
	height := canvasNodeDimension(node, "height", defaultCanvasHeight)

	r.mu.Lock()
	var ctx *canvasContext2D
	if r.canvasContexts != nil {
		for _, candidate := range r.canvasContexts {
			if candidate != nil && candidate.nodeID == int(node.ID) {
				ctx = candidate
				break
			}
		}
	}
	r.mu.Unlock()
	if ctx == nil {
		return
	}

	ctx.mu.Lock()
	ctx.width, ctx.height = width, height
	ctx.ops = nil
	ctx.bitmap = nil
	ctx.bitmapW, ctx.bitmapH = 0, 0
	ctx.resetState()
	ctx.mu.Unlock()
}

func (r *Runtime) newCanvasContextObject(width, height int, canvas goja.Value) (*canvasContext2D, *goja.Object) {
	ctx := &canvasContext2D{width: width, height: height}
	ctx.resetState()
	obj := r.vm.NewObject()
	ctx.obj = obj
	r.setupCanvasContextObject(obj, ctx, canvas)
	return ctx, obj
}

func (r *Runtime) setupCanvasContextObject(obj *goja.Object, ctx *canvasContext2D, canvas goja.Value) {
	_ = obj.Set("canvas", canvas)
	r.defineCanvasAccessor(obj, "fillStyle", func() goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		if ctx.fillStyleObject != nil {
			return ctx.fillStyleObject
		}
		return r.vm.ToValue(ctx.fillStyle)
	}, func(value goja.Value) {
		r.setCanvasFillStyle(ctx, value)
	})
	r.defineCanvasAccessor(obj, "strokeStyle", func() goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		if ctx.strokeStyleObject != nil {
			return ctx.strokeStyleObject
		}
		return r.vm.ToValue(ctx.strokeStyle)
	}, func(value goja.Value) {
		r.setCanvasStrokeStyle(ctx, value)
	})
	r.defineCanvasAccessor(obj, "globalAlpha", func() goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		return r.vm.ToValue(ctx.globalAlpha)
	}, func(value goja.Value) {
		alpha := value.ToFloat()
		if !canvasFinite(alpha) || alpha < 0 || alpha > 1 {
			return
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		ctx.globalAlpha = alpha
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "globalAlpha", Args: []interface{}{alpha}})
	})
	r.defineCanvasAccessor(obj, "lineWidth", func() goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		return r.vm.ToValue(ctx.lineWidth)
	}, func(value goja.Value) {
		width := value.ToFloat()
		if !canvasFinite(width) || width <= 0 {
			return
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		ctx.lineWidth = width
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "lineWidth", Args: []interface{}{width}})
	})
	r.defineCanvasAccessor(obj, "font", func() goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		return r.vm.ToValue(ctx.font)
	}, func(value goja.Value) {
		font := value.String()
		if !canvasFinite(float64(len(font))) || len(font) > 4096 {
			return
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		ctx.font = font
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "font", Args: []interface{}{font}})
	})
	r.defineCanvasAccessor(obj, "textAlign", func() goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		return r.vm.ToValue(ctx.textAlign)
	}, func(value goja.Value) {
		ctx.mu.Lock()
		ctx.textAlign = value.String()
		ctx.mu.Unlock()
	})
	r.defineCanvasAccessor(obj, "textBaseline", func() goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		return r.vm.ToValue(ctx.textBaseline)
	}, func(value goja.Value) {
		ctx.mu.Lock()
		ctx.textBaseline = value.String()
		ctx.mu.Unlock()
	})
	_ = obj.Set("imageSmoothingEnabled", true)
	_ = obj.Set("imageSmoothingQuality", "low")
	_ = obj.Set("filter", "none")
	_ = obj.Set("globalCompositeOperation", "source-over")
	_ = obj.Set("shadowBlur", 0.0)
	_ = obj.Set("shadowColor", "rgba(0, 0, 0, 0)")
	_ = obj.Set("shadowOffsetX", 0.0)
	_ = obj.Set("shadowOffsetY", 0.0)
	_ = obj.Set("lineCap", "butt")
	_ = obj.Set("lineJoin", "miter")
	_ = obj.Set("miterLimit", 10.0)
	_ = obj.Set("lineDashOffset", 0.0)
	r.setupCanvasContextMethods(obj, ctx)
}

func (r *Runtime) defineCanvasAccessor(obj *goja.Object, name string, get func() goja.Value, set func(goja.Value)) {
	_ = obj.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return get()
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			set(call.Arguments[0])
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) setCanvasFillStyle(ctx *canvasContext2D, value goja.Value) {
	style, object, rgba, ok := r.canvasStyleValue(value)
	if !ok {
		return
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.fillStyle = style
	ctx.fillStyleObject = object
	ctx.fillColor = rgba
	args := []interface{}{float64(rgba[0]), float64(rgba[1]), float64(rgba[2]), float64(rgba[3])}
	if object != nil {
		args = []interface{}{"unsupported"}
	}
	r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "fillStyle", Args: args})
}

func (r *Runtime) setCanvasStrokeStyle(ctx *canvasContext2D, value goja.Value) {
	style, object, rgba, ok := r.canvasStyleValue(value)
	if !ok {
		return
	}
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.strokeStyle = style
	ctx.strokeStyleObject = object
	ctx.strokeColor = rgba
	args := []interface{}{float64(rgba[0]), float64(rgba[1]), float64(rgba[2]), float64(rgba[3])}
	if object != nil {
		args = []interface{}{"unsupported"}
	}
	r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "strokeStyle", Args: args})
}

func (r *Runtime) canvasStyleValue(value goja.Value) (string, *goja.Object, [4]uint8, bool) {
	if object, ok := value.(*goja.Object); ok {
		if gradient := object.Get("__is_gradient__"); gradient != nil && gradient.ToBoolean() {
			return "", object, [4]uint8{}, true
		}
		if pattern := object.Get("__is_pattern__"); pattern != nil && pattern.ToBoolean() {
			return "", object, [4]uint8{}, true
		}
	}
	style := value.String()
	parsed, ok := css.ParseColor(style)
	if !ok {
		return "", nil, [4]uint8{}, false
	}
	return style, nil, [4]uint8{parsed.R, parsed.G, parsed.B, parsed.A}, true
}

func (r *Runtime) appendCanvasOpLocked(ctx *canvasContext2D, op canvasDrawOp) {
	if len(ctx.ops) >= maxCanvasOps {
		panic(r.vm.NewTypeError("Canvas operation limit exceeded"))
	}
	ctx.ops = append(ctx.ops, op)
}

func (r *Runtime) appendCanvasOpsLocked(ctx *canvasContext2D, ops ...canvasDrawOp) {
	if len(ops) > maxCanvasOps-len(ctx.ops) {
		panic(r.vm.NewTypeError("Canvas operation limit exceeded"))
	}
	ctx.ops = append(ctx.ops, ops...)
}

func canvasRoundRectRadius(call goja.FunctionCall, index int) (float64, bool) {
	if index >= len(call.Arguments) {
		return 0, true
	}
	radius := call.Arguments[index].ToFloat()
	if values, ok := call.Arguments[index].Export().([]interface{}); ok {
		if len(values) == 0 {
			return 0, true
		}
		switch value := values[0].(type) {
		case float64:
			radius = value
		case int64:
			radius = float64(value)
		default:
			return 0, false
		}
	}
	if !canvasFinite(radius) {
		return 0, false
	}
	if radius < 0 {
		radius = 0
	}
	return radius, true
}

func parseCanvasFloatArgs(call goja.FunctionCall, count int) ([]float64, bool) {
	args := make([]float64, count)
	for i := range args {
		if i < len(call.Arguments) {
			args[i] = call.Arguments[i].ToFloat()
		}
		if !canvasFinite(args[i]) {
			return nil, false
		}
	}
	return args, true
}

func (r *Runtime) jsGetContext(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		return goja.Null()
	}
	contextType := call.Arguments[0].String()
	if contextType != "2d" {
		return goja.Null()
	}

	thisObj, ok := call.This.(*goja.Object)
	if !ok {
		return goja.Null()
	}
	nidVal := thisObj.Get("__nid__")
	if nidVal == nil || goja.IsUndefined(nidVal) {
		return goja.Null()
	}
	nodeID := int(nidVal.ToInteger())

	r.mu.Lock()
	node := r.nodeRegistry[nodeID]
	r.mu.Unlock()
	if node == nil || node.Data != "canvas" {
		return goja.Null()
	}

	r.mu.Lock()
	if ctx, exists := r.canvasContexts[nodeID]; exists {
		obj := ctx.obj
		r.mu.Unlock()
		if obj != nil {
			return obj
		}
		return goja.Null()
	}
	if node.ID != 0 {
		if ctx, exists := r.canvasContexts[int(node.ID)]; exists {
			obj := ctx.obj
			r.mu.Unlock()
			if obj != nil {
				return obj
			}
			return goja.Null()
		}
	}
	r.mu.Unlock()

	width := canvasNodeDimension(node, "width", defaultCanvasWidth)
	height := canvasNodeDimension(node, "height", defaultCanvasHeight)
	ctx, obj := r.newCanvasContextObject(width, height, call.This)

	r.BindCanvasContext(nodeID, ctx)
	if node.ID != 0 && int(node.ID) != nodeID {
		r.BindCanvasContext(int(node.ID), ctx)
	}
	return obj
}

func (r *Runtime) unsupportedCanvasMethod(name string) {
	panic(r.domError("NotSupportedError", "CanvasRenderingContext2D."+name+" is not supported"))
}

func (r *Runtime) setupCanvasContextMethods(obj *goja.Object, ctx *canvasContext2D) {
	_ = obj.Set("fillRect", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 4)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		if ctx.fillStyleObject != nil {
			r.unsupportedCanvasMethod("fillRect with a gradient or pattern")
		}
		path := canvasPath{subpaths: []canvasSubpath{{points: canvasRectPoints(args[0], args[1], args[2], args[3]), closed: true}}}
		path.transform(ctx.matrix)
		r.appendCanvasOpsLocked(ctx,
			canvasDrawOp{Method: "fillRect", Args: []interface{}{args[0], args[1], args[2], args[3]}},
			canvasDrawOp{Method: canvasPaintMethod, Args: r.canvasPaintArgs("fill", path, "nonzero", ctx.fillColor, ctx.globalAlpha, ctx.matrix, ctx.font, ctx.lineWidth)},
		)
		return goja.Undefined()
	})

	_ = obj.Set("strokeRect", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 4)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		if ctx.strokeStyleObject != nil {
			r.unsupportedCanvasMethod("strokeRect with a gradient or pattern")
		}
		path := canvasPath{subpaths: []canvasSubpath{{points: canvasRectPoints(args[0], args[1], args[2], args[3]), closed: true}}}
		outline := canvasStrokePath(path, ctx.lineWidth, ctx.matrix)
		r.appendCanvasOpsLocked(ctx,
			canvasDrawOp{Method: "strokeRect", Args: []interface{}{args[0], args[1], args[2], args[3]}},
			canvasDrawOp{Method: canvasPaintMethod, Args: r.canvasPaintArgs("fill", outline, "nonzero", ctx.strokeColor, ctx.globalAlpha, ctx.matrix, ctx.font, ctx.lineWidth)},
		)
		return goja.Undefined()
	})

	_ = obj.Set("clearRect", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 4)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		path := canvasPath{subpaths: []canvasSubpath{{points: canvasRectPoints(args[0], args[1], args[2], args[3]), closed: true}}}
		path.transform(ctx.matrix)
		r.appendCanvasOpsLocked(ctx,
			canvasDrawOp{Method: "clearRect", Args: []interface{}{args[0], args[1], args[2], args[3]}},
			canvasDrawOp{Method: canvasPaintMethod, Args: r.canvasPaintArgs("clear", path, "nonzero", [4]uint8{}, 1, ctx.matrix, ctx.font, ctx.lineWidth)},
		)
		return goja.Undefined()
	})

	_ = obj.Set("beginPath", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		ctx.path = canvasPath{}
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "beginPath"})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("closePath", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		ctx.path.close()
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "closePath"})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("moveTo", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 2)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		ctx.path.move(canvasTransformPoint(ctx.matrix, args[0], args[1]))
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "moveTo", Args: []interface{}{args[0], args[1]}})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("lineTo", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 2)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		ctx.path.line(canvasTransformPoint(ctx.matrix, args[0], args[1]))
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "lineTo", Args: []interface{}{args[0], args[1]}})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("arc", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 6)
		if !ok {
			return goja.Undefined()
		}
		if args[2] < 0 {
			panic(r.domError("IndexSizeError", "Canvas arc radius must not be negative"))
		}
		counterclockwise := false
		if len(call.Arguments) >= 6 {
			counterclockwise = call.Arguments[5].ToBoolean()
		}
		ctx.mu.Lock()
		ctx.path.arc(args[0], args[1], args[2], args[3], args[4], counterclockwise, ctx.matrix)
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "arc", Args: []interface{}{args[0], args[1], args[2], args[3], args[4], counterclockwise}})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("rect", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 4)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		ctx.path.rect(canvasRectPoints(args[0], args[1], args[2], args[3]), ctx.matrix)
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "rect", Args: []interface{}{args[0], args[1], args[2], args[3]}})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("roundRect", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 4)
		if !ok {
			return goja.Undefined()
		}
		radius, ok := canvasRoundRectRadius(call, 4)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		ctx.path.roundRect(args[0], args[1], args[2], args[3], []float64{radius}, ctx.matrix)
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "roundRect", Args: []interface{}{args[0], args[1], args[2], args[3], radius}})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("bezierCurveTo", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 6)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		ctx.path.bezier(args, ctx.matrix)
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "bezierCurveTo", Args: []interface{}{args[0], args[1], args[2], args[3], args[4], args[5]}})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("quadraticCurveTo", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 4)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		ctx.path.quadratic(args, ctx.matrix)
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "quadraticCurveTo", Args: []interface{}{args[0], args[1], args[2], args[3]}})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("ellipse", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 8)
		if !ok {
			return goja.Undefined()
		}
		if args[2] < 0 || args[3] < 0 {
			panic(r.domError("IndexSizeError", "Canvas ellipse radii must not be negative"))
		}
		counterclockwise := false
		if len(call.Arguments) >= 8 {
			counterclockwise = call.Arguments[7].ToBoolean()
		}
		ctx.mu.Lock()
		ctx.path.ellipse(args, counterclockwise, ctx.matrix)
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "ellipse", Args: []interface{}{args[0], args[1], args[2], args[3], args[4], args[5], args[6], counterclockwise}})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("fill", func(call goja.FunctionCall) goja.Value {
		pathOps, rule := r.canvasPathArgument(call)
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		if ctx.fillStyleObject != nil {
			r.unsupportedCanvasMethod("fill with a gradient or pattern")
		}
		path := ctx.path
		if pathOps != nil {
			path = canvasPathFromOps(pathOps, ctx.matrix)
		}
		ops := make([]canvasDrawOp, 0, len(pathOps)+2)
		ops = append(ops, pathOps...)
		ops = append(ops, canvasDrawOp{Method: "fill", Args: []interface{}{rule}})
		if len(path.subpaths) > 0 {
			ops = append(ops, canvasDrawOp{Method: canvasPaintMethod, Args: r.canvasPaintArgs("fill", path, rule, ctx.fillColor, ctx.globalAlpha, ctx.matrix, ctx.font, ctx.lineWidth)})
		}
		r.appendCanvasOpsLocked(ctx, ops...)
		return goja.Undefined()
	})

	_ = obj.Set("stroke", func(call goja.FunctionCall) goja.Value {
		pathOps, _ := r.canvasPathArgument(call)
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		if ctx.strokeStyleObject != nil {
			r.unsupportedCanvasMethod("stroke with a gradient or pattern")
		}
		path := ctx.path
		if pathOps != nil {
			path = canvasPathFromOps(pathOps, ctx.matrix)
		}
		outline := canvasStrokePath(path, ctx.lineWidth, ctx.matrix)
		ops := make([]canvasDrawOp, 0, len(pathOps)+2)
		ops = append(ops, pathOps...)
		ops = append(ops, canvasDrawOp{Method: "stroke"})
		if len(outline.subpaths) > 0 {
			ops = append(ops, canvasDrawOp{Method: canvasPaintMethod, Args: r.canvasPaintArgs("fill", outline, "nonzero", ctx.strokeColor, ctx.globalAlpha, ctx.matrix, ctx.font, ctx.lineWidth)})
		}
		r.appendCanvasOpsLocked(ctx, ops...)
		return goja.Undefined()
	})

	_ = obj.Set("clip", func(call goja.FunctionCall) goja.Value {
		pathOps, rule := r.canvasPathArgument(call)
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		path := ctx.path
		if pathOps != nil {
			path = canvasPathFromOps(pathOps, ctx.matrix)
		}
		ops := make([]canvasDrawOp, 0, len(pathOps)+2)
		ops = append(ops, pathOps...)
		ops = append(ops, canvasDrawOp{Method: "clip", Args: []interface{}{rule}})
		if len(path.subpaths) > 0 {
			ops = append(ops, canvasDrawOp{Method: canvasPaintMethod, Args: r.canvasPaintArgs("clip", path, rule, [4]uint8{}, 1, ctx.matrix, ctx.font, ctx.lineWidth)})
		}
		r.appendCanvasOpsLocked(ctx, ops...)
		return goja.Undefined()
	})

	_ = obj.Set("fillText", func(call goja.FunctionCall) goja.Value {
		r.unsupportedCanvasMethod("fillText")
		return goja.Undefined()
	})
	_ = obj.Set("strokeText", func(call goja.FunctionCall) goja.Value {
		r.unsupportedCanvasMethod("strokeText")
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
		default:
			left = 0.0
			right = width
		}
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

	_ = obj.Set("createLinearGradient", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 4)
		if !ok {
			return goja.Null()
		}
		return r.newCanvasGradient("linear", args)
	})
	_ = obj.Set("createRadialGradient", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 6)
		if !ok {
			return goja.Null()
		}
		return r.newCanvasGradient("radial", args)
	})
	_ = obj.Set("createConicGradient", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 4)
		if !ok {
			return goja.Null()
		}
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

	_ = obj.Set("save", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		if len(ctx.stateStack) >= maxCanvasStateStack {
			panic(r.vm.NewTypeError("Canvas state stack limit exceeded"))
		}
		ctx.stateStack = append(ctx.stateStack, ctx.snapshotState())
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "save"})
		return goja.Undefined()
	})
	_ = obj.Set("restore", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		if len(ctx.stateStack) > 0 {
			last := len(ctx.stateStack) - 1
			ctx.restoreState(ctx.stateStack[last])
			ctx.stateStack = ctx.stateStack[:last]
		}
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "restore"})
		return goja.Undefined()
	})
	_ = obj.Set("translate", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 2)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		m := ctx.matrix
		next := m
		next[4] = m[0]*args[0] + m[2]*args[1] + m[4]
		next[5] = m[1]*args[0] + m[3]*args[1] + m[5]
		if canvasMatrixFinite(next) {
			ctx.matrix = next
			r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "translate", Args: []interface{}{args[0], args[1]}})
		}
		return goja.Undefined()
	})
	_ = obj.Set("rotate", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 1)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		m := ctx.matrix
		sin, cos := math.Sin(args[0]), math.Cos(args[0])
		next := [6]float64{
			m[0]*cos + m[2]*sin,
			m[1]*cos + m[3]*sin,
			-m[0]*sin + m[2]*cos,
			-m[1]*sin + m[3]*cos,
			m[4], m[5],
		}
		if canvasMatrixFinite(next) {
			ctx.matrix = next
			r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "rotate", Args: []interface{}{args[0]}})
		}
		return goja.Undefined()
	})
	_ = obj.Set("scale", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 2)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		m := ctx.matrix
		next := [6]float64{m[0] * args[0], m[1] * args[0], m[2] * args[1], m[3] * args[1], m[4], m[5]}
		if canvasMatrixFinite(next) {
			ctx.matrix = next
			r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "scale", Args: []interface{}{args[0], args[1]}})
		}
		return goja.Undefined()
	})
	_ = obj.Set("transform", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 6)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		m := ctx.matrix
		next := [6]float64{
			m[0]*args[0] + m[2]*args[1],
			m[1]*args[0] + m[3]*args[1],
			m[0]*args[2] + m[2]*args[3],
			m[1]*args[2] + m[3]*args[3],
			m[0]*args[4] + m[2]*args[5] + m[4],
			m[1]*args[4] + m[3]*args[5] + m[5],
		}
		if canvasMatrixFinite(next) {
			ctx.matrix = next
			r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "transform", Args: []interface{}{args[0], args[1], args[2], args[3], args[4], args[5]}})
		}
		return goja.Undefined()
	})
	_ = obj.Set("setTransform", func(call goja.FunctionCall) goja.Value {
		args, ok := parseCanvasFloatArgs(call, 6)
		if !ok {
			return goja.Undefined()
		}
		ctx.mu.Lock()
		defer ctx.mu.Unlock()
		next := [6]float64{args[0], args[1], args[2], args[3], args[4], args[5]}
		ctx.matrix = next
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "setTransform", Args: []interface{}{args[0], args[1], args[2], args[3], args[4], args[5]}})
		return goja.Undefined()
	})
	_ = obj.Set("resetTransform", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		ctx.matrix = [6]float64{1, 0, 0, 1, 0, 0}
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "resetTransform"})
		ctx.mu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("reset", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		ctx.resetState()
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "reset"})
		ctx.mu.Unlock()
		_ = obj.Set("imageSmoothingEnabled", true)
		_ = obj.Set("imageSmoothingQuality", "low")
		_ = obj.Set("filter", "none")
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

	var lineDash []float64
	_ = obj.Set("setLineDash", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			if arr, ok := call.Arguments[0].Export().([]interface{}); ok {
				lineDash = make([]float64, 0, len(arr))
				for _, v := range arr {
					switch n := v.(type) {
					case int64:
						if canvasFinite(float64(n)) && n >= 0 {
							lineDash = append(lineDash, float64(n))
						}
					case float64:
						if canvasFinite(n) && n >= 0 {
							lineDash = append(lineDash, n)
						}
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

	_ = obj.Set("drawImage", func(call goja.FunctionCall) goja.Value {
		r.unsupportedCanvasMethod("drawImage")
		return goja.Undefined()
	})
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
		return r.newImageData(w, h, nil)
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
		r.validateImageDataRegion(w, h)
		data := make([]byte, w*h*4)
		ctx.mu.Lock()
		width, height := ctx.width, ctx.height
		var bitmap []byte
		if canvasBitmapFits(width, height) {
			bitmap = make([]byte, width*height*4)
			replayCanvasPixels(bitmap, width, height, ctx.ops)
			ctx.bitmap = bitmap
			ctx.bitmapW, ctx.bitmapH = width, height
			copyCanvasRegion(data, w, h, x, y, bitmap, width, height)
		}
		ctx.mu.Unlock()
		imgData := r.newImageData(w, h, nil)
		pixels := imgData.Get("data").ToObject(r.vm)
		for index, value := range data {
			_ = pixels.Set(strconv.Itoa(index), value)
		}
		return imgData
	})
	_ = obj.Set("putImageData", func(call goja.FunctionCall) goja.Value {
		ctx.mu.Lock()
		r.appendCanvasOpLocked(ctx, canvasDrawOp{Method: "putImageData"})
		ctx.mu.Unlock()
		return goja.Undefined()
	})
}

func (r *Runtime) canvasPaintArgs(kind string, path canvasPath, rule string, rgba [4]uint8, alpha float64, matrix [6]float64, font string, lineWidth float64) []interface{} {
	subpaths := make([][]float64, 0, len(path.subpaths))
	for _, subpath := range path.subpaths {
		if len(subpath.points) == 0 {
			continue
		}
		points := make([]float64, 0, len(subpath.points)*2)
		for _, point := range subpath.points {
			if !canvasFinite(point.x) || !canvasFinite(point.y) {
				continue
			}
			points = append(points, point.x, point.y)
		}
		if len(points) >= 4 {
			subpaths = append(subpaths, points)
		}
	}
	stateMatrix := make([]float64, len(matrix))
	copy(stateMatrix, matrix[:])
	return []interface{}{kind, subpaths, []float64{float64(rgba[0]), float64(rgba[1]), float64(rgba[2]), float64(rgba[3])}, alpha, rule, stateMatrix, font, lineWidth}
}

func (r *Runtime) canvasPathArgument(call goja.FunctionCall) ([]canvasDrawOp, string) {
	rule := "nonzero"
	if len(call.Arguments) == 0 {
		return nil, rule
	}
	if text, ok := call.Arguments[0].Export().(string); ok {
		rule = strings.ToLower(text)
		if rule != "evenodd" {
			rule = "nonzero"
		}
		return nil, rule
	}
	pathObj, ok := call.Arguments[0].(*goja.Object)
	if !ok {
		return nil, rule
	}
	if len(call.Arguments) > 1 {
		rule = strings.ToLower(call.Arguments[1].String())
		if rule != "evenodd" {
			rule = "nonzero"
		}
	}
	pOpsVal := pathObj.Get("__path_ops__")
	if pOpsVal == nil || goja.IsUndefined(pOpsVal) {
		return nil, rule
	}
	fn, ok := goja.AssertFunction(pOpsVal)
	if !ok {
		return nil, rule
	}
	res, err := fn(pathObj)
	if err != nil {
		return nil, rule
	}
	ops, _ := res.Export().([]canvasDrawOp)
	return append([]canvasDrawOp{}, ops...), rule
}

func canvasRectPoints(x, y, width, height float64) []canvasPoint {
	return []canvasPoint{{x, y}, {x + width, y}, {x + width, y + height}, {x, y + height}, {x, y}}
}

func (path *canvasPath) transform(matrix [6]float64) {
	for i := range path.subpaths {
		for j := range path.subpaths[i].points {
			path.subpaths[i].points[j] = canvasTransformPoint(matrix, path.subpaths[i].points[j].x, path.subpaths[i].points[j].y)
		}
	}
}

func (path *canvasPath) move(point canvasPoint) {
	path.subpaths = append(path.subpaths, canvasSubpath{points: []canvasPoint{point}})
}

func (path *canvasPath) line(point canvasPoint) {
	if len(path.subpaths) == 0 {
		path.subpaths = append(path.subpaths, canvasSubpath{points: []canvasPoint{{0, 0}, point}})
		return
	}
	last := &path.subpaths[len(path.subpaths)-1]
	last.points = append(last.points, point)
}

func (path *canvasPath) close() {
	if len(path.subpaths) == 0 {
		return
	}
	last := &path.subpaths[len(path.subpaths)-1]
	if len(last.points) == 0 {
		return
	}
	last.closed = true
	first := last.points[0]
	lastPoint := last.points[len(last.points)-1]
	if first != lastPoint {
		last.points = append(last.points, first)
	}
}

func (path *canvasPath) rect(points []canvasPoint, matrix [6]float64) {
	for i := range points {
		points[i] = canvasTransformPoint(matrix, points[i].x, points[i].y)
	}
	path.subpaths = append(path.subpaths, canvasSubpath{points: points, closed: true})
}

func (path *canvasPath) arc(x, y, radius, start, end float64, counterclockwise bool, matrix [6]float64) {
	delta := end - start
	if !counterclockwise && delta < 0 {
		delta += 2 * math.Pi
	} else if counterclockwise && delta > 0 {
		delta -= 2 * math.Pi
	}
	scale := math.Max(math.Abs(matrix[0]), math.Abs(matrix[1]))
	scale = math.Max(scale, math.Max(math.Abs(matrix[2]), math.Abs(matrix[3])))
	steps := int(math.Ceil(math.Abs(delta) * math.Max(radius*scale, 1) / 0.5))
	if steps < 16 {
		steps = 8
	}
	if steps > 512 {
		steps = 512
	}
	if len(path.subpaths) > 0 {
		last := &path.subpaths[len(path.subpaths)-1]
		if len(last.points) > 0 {
			first := canvasTransformPoint(matrix, x+radius*math.Cos(start), y+radius*math.Sin(start))
			last.points = append(last.points, first)
		}
	}
	startPoint := canvasTransformPoint(matrix, x+radius*math.Cos(start), y+radius*math.Sin(start))
	if len(path.subpaths) == 0 || len(path.subpaths[len(path.subpaths)-1].points) == 0 {
		path.move(startPoint)
	}
	for i := 1; i <= steps; i++ {
		angle := start + delta*float64(i)/float64(steps)
		point := canvasPoint{x + radius*math.Cos(angle), y + radius*math.Sin(angle)}
		path.line(canvasTransformPoint(matrix, point.x, point.y))
	}
}

func (path *canvasPath) ellipse(args []float64, counterclockwise bool, matrix [6]float64) {
	cx, cy, rx, ry, rotation, start, end := args[0], args[1], args[2], args[3], args[4], args[5], args[6]
	delta := end - start
	if !counterclockwise && delta < 0 {
		delta += 2 * math.Pi
	} else if counterclockwise && delta > 0 {
		delta -= 2 * math.Pi
	}
	steps := 64
	cosRot, sinRot := math.Cos(rotation), math.Sin(rotation)
	for i := 0; i <= steps; i++ {
		angle := start + delta*float64(i)/float64(steps)
		x, y := rx*math.Cos(angle), ry*math.Sin(angle)
		point := canvasTransformPoint(matrix, cx+x*cosRot-y*sinRot, cy+x*sinRot+y*cosRot)
		if i == 0 {
			path.move(point)
		} else {
			path.line(point)
		}
	}
}

func (path *canvasPath) bezier(args []float64, matrix [6]float64) {
	if len(path.subpaths) == 0 {
		path.move(canvasPoint{})
	}
	last := path.subpaths[len(path.subpaths)-1]
	start := last.points[len(last.points)-1]
	for i := 1; i <= 16; i++ {
		t := float64(i) / 16
		it := 1 - t
		x := it*it*it*start.x + 3*it*it*t*canvasTransformPoint(matrix, args[0], args[1]).x + 3*it*t*t*canvasTransformPoint(matrix, args[2], args[3]).x + t*t*t*canvasTransformPoint(matrix, args[4], args[5]).x
		y := it*it*it*start.y + 3*it*it*t*canvasTransformPoint(matrix, args[0], args[1]).y + 3*it*t*t*canvasTransformPoint(matrix, args[2], args[3]).y + t*t*t*canvasTransformPoint(matrix, args[4], args[5]).y
		path.line(canvasPoint{x, y})
	}
}

func (path *canvasPath) quadratic(args []float64, matrix [6]float64) {
	if len(path.subpaths) == 0 {
		path.move(canvasPoint{})
	}
	last := path.subpaths[len(path.subpaths)-1]
	start := last.points[len(last.points)-1]
	control := canvasTransformPoint(matrix, args[0], args[1])
	end := canvasTransformPoint(matrix, args[2], args[3])
	for i := 1; i <= 16; i++ {
		t := float64(i) / 16
		it := 1 - t
		path.line(canvasPoint{it*it*start.x + 2*it*t*control.x + t*t*end.x, it*it*start.y + 2*it*t*control.y + t*t*end.y})
	}
}

func (path *canvasPath) roundRect(x, y, width, height float64, radii []float64, matrix [6]float64) {
	r := 0.0
	if len(radii) > 0 {
		r = radii[0]
	}
	if r < 0 {
		r = 0
	}
	r = math.Min(r, math.Min(math.Abs(width)/2, math.Abs(height)/2))
	points := []canvasPoint{{x + r, y}, {x + width - r, y}, {x + width, y + r}, {x + width, y + height - r}, {x + width - r, y + height}, {x + r, y + height}, {x, y + height - r}, {x, y + r}, {x + r, y}}
	for i := range points {
		points[i] = canvasTransformPoint(matrix, points[i].x, points[i].y)
	}
	path.subpaths = append(path.subpaths, canvasSubpath{points: points, closed: true})
}

func canvasPathFromOps(ops []canvasDrawOp, matrix [6]float64) canvasPath {
	var path canvasPath
	for _, op := range ops {
		args := make([]float64, 0, len(op.Args))
		flag := false
		valid := true
		for _, arg := range op.Args {
			switch value := arg.(type) {
			case float64:
				if !canvasFinite(value) {
					valid = false
				} else {
					args = append(args, value)
				}
			case bool:
				flag = value
			}
		}
		if !valid {
			continue
		}
		switch op.Method {
		case "beginPath":
			path = canvasPath{}
		case "moveTo":
			if len(args) >= 2 {
				path.move(canvasTransformPoint(matrix, args[0], args[1]))
			}
		case "lineTo":
			if len(args) >= 2 {
				path.line(canvasTransformPoint(matrix, args[0], args[1]))
			}
		case "closePath":
			path.close()
		case "rect":
			if len(args) >= 4 {
				path.rect(canvasRectPoints(args[0], args[1], args[2], args[3]), matrix)
			}
		case "arc":
			if len(args) >= 5 && args[2] >= 0 {
				path.arc(args[0], args[1], args[2], args[3], args[4], flag, matrix)
			}
		case "ellipse":
			if len(args) >= 7 && args[2] >= 0 && args[3] >= 0 {
				path.ellipse(args[:7], flag, matrix)
			}
		case "bezierCurveTo":
			if len(args) >= 6 {
				path.bezier(args[:6], matrix)
			}
		case "quadraticCurveTo":
			if len(args) >= 4 {
				path.quadratic(args[:4], matrix)
			}
		case "roundRect":
			if len(args) >= 5 {
				path.roundRect(args[0], args[1], args[2], args[3], []float64{args[4]}, matrix)
			}
		}
	}
	return path
}

func canvasStrokePath(path canvasPath, lineWidth float64, matrix [6]float64) canvasPath {
	if lineWidth <= 0 || !canvasFinite(lineWidth) {
		return canvasPath{}
	}
	user := cloneCanvasPath(path)
	if determinant := matrix[0]*matrix[3] - matrix[1]*matrix[2]; math.Abs(determinant) > 1e-12 {
		inverse := [6]float64{matrix[3] / determinant, -matrix[1] / determinant, -matrix[2] / determinant, matrix[0] / determinant, 0, 0}
		inverse[4] = -(inverse[0]*matrix[4] + inverse[2]*matrix[5])
		inverse[5] = -(inverse[1]*matrix[4] + inverse[3]*matrix[5])
		user.transform(inverse)
	}
	half := lineWidth / 2
	var outline canvasPath
	for _, subpath := range user.subpaths {
		points := subpath.points
		closed := subpath.closed
		if closed && len(points) > 1 && points[0] == points[len(points)-1] {
			points = points[:len(points)-1]
		}
		if len(points) < 2 {
			continue
		}
		segmentCount := len(points) - 1
		if closed {
			segmentCount = len(points)
		}
		if segmentCount == 0 {
			continue
		}
		segments := make([][2]canvasPoint, segmentCount)
		for i := 0; i < segmentCount; i++ {
			segments[i] = [2]canvasPoint{points[i], points[(i+1)%len(points)]}
		}
		left := make([]canvasPoint, segmentCount)
		right := make([]canvasPoint, segmentCount)
		leftEnd := make([]canvasPoint, segmentCount)
		rightEnd := make([]canvasPoint, segmentCount)
		for i, segment := range segments {
			dx, dy := segment[1].x-segment[0].x, segment[1].y-segment[0].y
			length := math.Hypot(dx, dy)
			if length == 0 {
				left[i], right[i] = segment[0], segment[0]
				leftEnd[i], rightEnd[i] = segment[1], segment[1]
				continue
			}
			nx, ny := -dy/length*half, dx/length*half
			left[i] = canvasPoint{segment[0].x + nx, segment[0].y + ny}
			right[i] = canvasPoint{segment[0].x - nx, segment[0].y - ny}
			leftEnd[i] = canvasPoint{segment[1].x + nx, segment[1].y + ny}
			rightEnd[i] = canvasPoint{segment[1].x - nx, segment[1].y - ny}
		}
		leftSide := make([]canvasPoint, 0, segmentCount+1)
		rightSide := make([]canvasPoint, 0, segmentCount+1)
		if closed {
			for i := 0; i < segmentCount; i++ {
				prev := (i + segmentCount - 1) % segmentCount
				leftSide = append(leftSide, canvasOffsetJoin(left[prev], leftEnd[prev], left[i], leftEnd[i], half))
				rightSide = append(rightSide, canvasOffsetJoin(right[prev], rightEnd[prev], right[i], rightEnd[i], half))
			}
		} else {
			leftSide = append(leftSide, left[0])
			rightSide = append(rightSide, right[0])
			for i := 1; i < segmentCount; i++ {
				leftSide = append(leftSide, canvasOffsetJoin(left[i-1], leftEnd[i-1], left[i], leftEnd[i], half))
				rightSide = append(rightSide, canvasOffsetJoin(right[i-1], rightEnd[i-1], right[i], rightEnd[i], half))
			}
			leftSide = append(leftSide, left[segmentCount-1])
			rightSide = append(rightSide, right[segmentCount-1])
			last := segments[segmentCount-1]
			dx, dy := last[1].x-last[0].x, last[1].y-last[0].y
			length := math.Hypot(dx, dy)
			if length > 0 {
				nx, ny := -dy/length*half, dx/length*half
				leftSide = append(leftSide, canvasPoint{last[1].x + nx, last[1].y + ny})
				rightSide = append(rightSide, canvasPoint{last[1].x - nx, last[1].y - ny})
			}
		}
		polygon := append([]canvasPoint(nil), leftSide...)
		for i := len(rightSide) - 1; i >= 0; i-- {
			polygon = append(polygon, rightSide[i])
		}
		if canvasPolygonArea(polygon) < 0 {
			for i, j := 0, len(polygon)-1; i < j; i, j = i+1, j-1 {
				polygon[i], polygon[j] = polygon[j], polygon[i]
			}
		}
		outline.subpaths = append(outline.subpaths, canvasSubpath{points: polygon, closed: true})
	}
	outline.transform(matrix)
	return outline
}

func canvasOffsetJoin(a, b, c, d canvasPoint, limit float64) canvasPoint {
	v1x, v1y := b.x-a.x, b.y-a.y
	v2x, v2y := d.x-c.x, d.y-c.y
	denominator := v1x*v2y - v1y*v2x
	if math.Abs(denominator) < 1e-12 {
		return c
	}
	t := ((c.x-a.x)*v2y - (c.y-a.y)*v2x) / denominator
	point := canvasPoint{a.x + t*v1x, a.y + t*v1y}
	if math.Hypot(point.x-c.x, point.y-c.y) > limit*10 {
		return c
	}
	return point
}

func canvasPolygonArea(points []canvasPoint) float64 {
	var area float64
	for i := 0; i+1 < len(points); i++ {
		area += points[i].x*points[i+1].y - points[i+1].x*points[i].y
	}
	return area / 2
}

func (r *Runtime) setupCanvas() {
	r.setupImageData()
	constructor := func(call goja.ConstructorCall) *goja.Object {
		width, height := defaultCanvasWidth, defaultCanvasHeight
		if len(call.Arguments) >= 1 {
			width = int(call.Arguments[0].ToInteger())
		}
		if len(call.Arguments) >= 2 {
			height = int(call.Arguments[1].ToInteger())
		}
		if width < 0 {
			width = 0
		}
		if height < 0 {
			height = 0
		}
		_, obj := r.newCanvasContextObject(width, height, goja.Null())
		return obj
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
	kind  string
	args  []float64
	stops []gradientStop
}

type gradientStop struct {
	offset float64
	color  string
}

func (r *Runtime) newCanvasGradient(kind string, args []float64) *goja.Object {
	gData := &canvasGradientData{kind: kind, args: args}
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
	pData := &canvasPatternData{image: image, repetition: repetition}
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

func (r *Runtime) appendPath2DOp(data *path2DData, op canvasDrawOp) {
	if len(data.ops) >= maxCanvasOps {
		panic(r.vm.NewTypeError("Canvas path operation limit exceeded"))
	}
	data.ops = append(data.ops, op)
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
			pData.ops = nil
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("closePath", func(call goja.FunctionCall) goja.Value {
			pData.mu.Lock()
			r.appendPath2DOp(pData, canvasDrawOp{Method: "closePath"})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("moveTo", func(call goja.FunctionCall) goja.Value {
			args, ok := parseCanvasFloatArgs(call, 2)
			if !ok {
				return goja.Undefined()
			}
			pData.mu.Lock()
			r.appendPath2DOp(pData, canvasDrawOp{Method: "moveTo", Args: []interface{}{args[0], args[1]}})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("lineTo", func(call goja.FunctionCall) goja.Value {
			args, ok := parseCanvasFloatArgs(call, 2)
			if !ok {
				return goja.Undefined()
			}
			pData.mu.Lock()
			r.appendPath2DOp(pData, canvasDrawOp{Method: "lineTo", Args: []interface{}{args[0], args[1]}})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("arc", func(call goja.FunctionCall) goja.Value {
			args, ok := parseCanvasFloatArgs(call, 6)
			if !ok {
				return goja.Undefined()
			}
			if args[2] < 0 {
				panic(r.domError("IndexSizeError", "Canvas arc radius must not be negative"))
			}
			counterclockwise := false
			if len(call.Arguments) >= 6 {
				counterclockwise = call.Arguments[5].ToBoolean()
			}
			pData.mu.Lock()
			r.appendPath2DOp(pData, canvasDrawOp{Method: "arc", Args: []interface{}{args[0], args[1], args[2], args[3], args[4], counterclockwise}})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("rect", func(call goja.FunctionCall) goja.Value {
			args, ok := parseCanvasFloatArgs(call, 4)
			if !ok {
				return goja.Undefined()
			}
			pData.mu.Lock()
			r.appendPath2DOp(pData, canvasDrawOp{Method: "rect", Args: []interface{}{args[0], args[1], args[2], args[3]}})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("roundRect", func(call goja.FunctionCall) goja.Value {
			args, ok := parseCanvasFloatArgs(call, 4)
			if !ok {
				return goja.Undefined()
			}
			radius, ok := canvasRoundRectRadius(call, 4)
			if !ok {
				return goja.Undefined()
			}
			pData.mu.Lock()
			r.appendPath2DOp(pData, canvasDrawOp{Method: "roundRect", Args: []interface{}{args[0], args[1], args[2], args[3], radius}})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("bezierCurveTo", func(call goja.FunctionCall) goja.Value {
			args, ok := parseCanvasFloatArgs(call, 6)
			if !ok {
				return goja.Undefined()
			}
			pData.mu.Lock()
			r.appendPath2DOp(pData, canvasDrawOp{Method: "bezierCurveTo", Args: []interface{}{args[0], args[1], args[2], args[3], args[4], args[5]}})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("quadraticCurveTo", func(call goja.FunctionCall) goja.Value {
			args, ok := parseCanvasFloatArgs(call, 4)
			if !ok {
				return goja.Undefined()
			}
			pData.mu.Lock()
			r.appendPath2DOp(pData, canvasDrawOp{Method: "quadraticCurveTo", Args: []interface{}{args[0], args[1], args[2], args[3]}})
			pData.mu.Unlock()
			return goja.Undefined()
		})
		_ = obj.Set("ellipse", func(call goja.FunctionCall) goja.Value {
			args, ok := parseCanvasFloatArgs(call, 8)
			if !ok {
				return goja.Undefined()
			}
			if args[2] < 0 || args[3] < 0 {
				panic(r.domError("IndexSizeError", "Canvas ellipse radii must not be negative"))
			}
			counterclockwise := false
			if len(call.Arguments) >= 8 {
				counterclockwise = call.Arguments[7].ToBoolean()
			}
			pData.mu.Lock()
			r.appendPath2DOp(pData, canvasDrawOp{Method: "ellipse", Args: []interface{}{args[0], args[1], args[2], args[3], args[4], args[5], args[6], counterclockwise}})
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
									for _, op := range ops {
										r.appendPath2DOp(pData, op)
									}
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

type canvasClipSpan struct {
	x0, x1 int
	alpha  uint8
}

type canvasClipMask struct {
	rows [][]canvasClipSpan
}

type canvasReplay struct {
	width, height int
	mask          *image.Alpha
	clip          *canvasClipMask
	stack         []*canvasClipMask
}

func newCanvasReplay(width, height int) *canvasReplay {
	return &canvasReplay{
		width: width, height: height,
		mask: image.NewAlpha(image.Rect(0, 0, width, height)),
	}
}

func replayCanvasPixels(pixels []byte, width, height int, ops []canvasDrawOp) {
	if width <= 0 || height <= 0 || len(pixels) < width*height*4 {
		return
	}
	clear(pixels)
	replay := newCanvasReplay(width, height)
	for _, op := range ops {
		switch op.Method {
		case "save":
			if len(replay.stack) < maxCanvasStateStack {
				replay.stack = append(replay.stack, replay.clip)
			}
		case "restore":
			if len(replay.stack) > 0 {
				last := len(replay.stack) - 1
				replay.clip = replay.stack[last]
				replay.stack = replay.stack[:last]
			}
		case "reset":
			clear(pixels)
			replay.clip = nil
			replay.stack = nil
		case canvasPaintMethod:
			args := op.Args
			if len(args) < 8 {
				continue
			}
			kind, _ := args[0].(string)
			subpaths, _ := args[1].([][]float64)
			rgba, _ := args[2].([]float64)
			alpha, _ := args[3].(float64)
			if len(rgba) < 4 || !canvasFinite(alpha) {
				continue
			}
			coverage, bounds := replay.coverage(subpaths)
			if bounds.Empty() {
				continue
			}
			switch kind {
			case "clip":
				replay.clip = intersectCanvasClip(replay.clip, canvasClipFromCoverage(coverage, bounds, width, height))
			case "clear":
				canvasClearCoverage(pixels, width, coverage, bounds, replay.clip)
			case "fill":
				source := canvasPackedColor(rgba, alpha)
				canvasFillCoverage(pixels, width, source, coverage, bounds, replay.clip)
			}
		}
	}
}

type canvasIntersection struct {
	x     float64
	delta int
}

func (replay *canvasReplay) coverage(subpaths [][]float64) ([]uint8, image.Rectangle) {
	clear(replay.mask.Pix)
	bounds := image.Rectangle{}
	validPaths := make([][]float64, 0, len(subpaths))
	for _, points := range subpaths {
		if len(points) < 6 || len(points)%2 != 0 {
			continue
		}
		valid := true
		for i := 0; i+1 < len(points); i += 2 {
			if !canvasFinite(points[i]) || !canvasFinite(points[i+1]) {
				valid = false
				break
			}
			x0, y0 := int(math.Floor(points[i])), int(math.Floor(points[i+1]))
			x1, y1 := int(math.Ceil(points[i]))+1, int(math.Ceil(points[i+1]))+1
			bounds = bounds.Union(image.Rect(x0, y0, x1, y1))
		}
		if valid {
			validPaths = append(validPaths, points)
		}
	}
	if bounds.Empty() {
		return nil, bounds
	}
	bounds = bounds.Intersect(image.Rect(0, 0, replay.width, replay.height))
	if bounds.Empty() {
		return nil, bounds
	}
	var intersections []canvasIntersection
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for subY := 0; subY < 4; subY++ {
			sampleY := float64(y) + (float64(subY)+0.5)/4
			intersections = intersections[:0]
			for _, points := range validPaths {
				count := len(points) / 2
				for i := 0; i < count; i++ {
					next := (i + 1) % count
					x1, y1 := points[i*2], points[i*2+1]
					x2, y2 := points[next*2], points[next*2+1]
					if y1 <= sampleY && y2 > sampleY {
						x := x1 + (sampleY-y1)*(x2-x1)/(y2-y1)
						intersections = append(intersections, canvasIntersection{x: x, delta: 1})
					} else if y2 <= sampleY && y1 > sampleY {
						x := x1 + (sampleY-y1)*(x2-x1)/(y2-y1)
						intersections = append(intersections, canvasIntersection{x: x, delta: -1})
					}
				}
			}
			if len(intersections) < 2 {
				continue
			}
			sort.Slice(intersections, func(i, j int) bool { return intersections[i].x < intersections[j].x })
			winding := 0
			previous := intersections[0].x
			for i := 0; i < len(intersections); {
				x := intersections[i].x
				if winding != 0 && x > previous {
					canvasAccumulateSpan(replay.mask.Pix, replay.width, y, previous, x)
				}
				for i < len(intersections) && intersections[i].x == x {
					winding += intersections[i].delta
					i++
				}
				previous = x
			}
		}
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			offset := y*replay.width + x
			replay.mask.Pix[offset] = byte((int(replay.mask.Pix[offset])*255 + 8) / 16)
		}
	}
	return replay.mask.Pix, bounds
}

func canvasAccumulateSpan(mask []byte, width, y int, left, right float64) {
	if left >= right {
		return
	}
	x0, x1 := max(0, int(math.Floor(left))), min(width, int(math.Ceil(right)))
	for x := x0; x < x1; x++ {
		sx0 := max(0, int(math.Ceil((left-float64(x))*4-0.5)))
		sx1 := min(4, int(math.Ceil((right-float64(x))*4-0.5)))
		if sx0 < sx1 {
			offset := y*width + x
			mask[offset] = byte(int(mask[offset]) + sx1 - sx0)
		}
	}
}

func canvasPackedColor(rgba []float64, alpha float64) uint32 {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	a := uint8(math.Round(math.Max(0, math.Min(255, rgba[3])) * alpha))
	r := uint8(math.Max(0, math.Min(255, rgba[0])))
	g := uint8(math.Max(0, math.Min(255, rgba[1])))
	b := uint8(math.Max(0, math.Min(255, rgba[2])))
	return uint32(uint32(r)*uint32(a)/255)<<24 |
		uint32(uint32(g)*uint32(a)/255)<<16 |
		uint32(uint32(b)*uint32(a)/255)<<8 |
		uint32(a)
}

func canvasFillCoverage(pixels []byte, width int, source uint32, coverage []uint8, bounds image.Rectangle, clip *canvasClipMask) {
	sa := uint8(source)
	sr, sg, sb := uint8(source>>24), uint8(source>>16&0xff), uint8(source>>8&0xff)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			cov := coverage[y*width+x]
			if clip != nil {
				cov = uint8((uint32(cov)*uint32(canvasClipAlpha(clip, x, y)) + 127) / 255)
			}
			if cov == 0 {
				continue
			}
			scaled := uint32(uint32(sr)*uint32(cov)+127)/255<<24 |
				uint32(uint32(sg)*uint32(cov)+127)/255<<16 |
				uint32(uint32(sb)*uint32(cov)+127)/255<<8 |
				uint32(uint32(sa)*uint32(cov)+127)/255
			offset := (y*width + x) * 4
			dst := uint32(pixels[offset])<<24 | uint32(pixels[offset+1])<<16 | uint32(pixels[offset+2])<<8 | uint32(pixels[offset+3])
			out := canvasBlendOver(dst, scaled)
			pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = byte(out>>24), byte(out>>16&0xff), byte(out>>8&0xff), byte(out)
		}
	}
}

func canvasClearCoverage(pixels []byte, width int, coverage []uint8, bounds image.Rectangle, clip *canvasClipMask) {
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			cov := coverage[y*width+x]
			if clip != nil {
				cov = uint8((uint32(cov)*uint32(canvasClipAlpha(clip, x, y)) + 127) / 255)
			}
			if cov == 0 {
				continue
			}
			keep := uint32(255 - cov)
			offset := (y*width + x) * 4
			for channel := 0; channel < 4; channel++ {
				pixels[offset+channel] = byte((uint32(pixels[offset+channel])*keep + 127) / 255)
			}
		}
	}
}

func canvasBlendOver(dst, src uint32) uint32 {
	sa := uint32(uint8(src))
	if sa == 255 {
		return src
	}
	if sa == 0 {
		return dst
	}
	inv := 255 - sa
	r := (uint32(uint8(src>>24))*255 + uint32(uint8(dst>>24))*inv + 127) / 255
	g := (uint32(uint8(src>>16&0xff))*255 + uint32(uint8(dst>>16&0xff))*inv + 127) / 255
	b := (uint32(uint8(src>>8&0xff))*255 + uint32(uint8(dst>>8&0xff))*inv + 127) / 255
	a := (sa*255 + uint32(uint8(dst))*inv + 127) / 255
	return r<<24 | g<<16 | b<<8 | a
}

func canvasClipFromCoverage(coverage []uint8, bounds image.Rectangle, width, height int) *canvasClipMask {
	clip := &canvasClipMask{rows: make([][]canvasClipSpan, height)}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; {
			alpha := coverage[y*width+x]
			if alpha == 0 {
				x++
				continue
			}
			start := x
			for x < bounds.Max.X && coverage[y*width+x] == alpha {
				x++
			}
			clip.rows[y] = append(clip.rows[y], canvasClipSpan{x0: start, x1: x, alpha: alpha})
		}
	}
	return clip
}

func intersectCanvasClip(base, next *canvasClipMask) *canvasClipMask {
	if base == nil {
		return next
	}
	if next == nil {
		return base
	}
	out := &canvasClipMask{rows: make([][]canvasClipSpan, len(base.rows))}
	for y, a := range base.rows {
		b := next.rows[y]
		var spans []canvasClipSpan
		for i, j := 0, 0; i < len(a) && j < len(b); {
			x0, x1 := max(a[i].x0, b[j].x0), min(a[i].x1, b[j].x1)
			if x0 < x1 {
				alpha := uint8((uint32(a[i].alpha)*uint32(b[j].alpha) + 127) / 255)
				if alpha > 0 {
					spans = append(spans, canvasClipSpan{x0: x0, x1: x1, alpha: alpha})
				}
			}
			if a[i].x1 <= b[j].x1 {
				i++
			} else {
				j++
			}
		}
		out.rows[y] = spans
	}
	return out
}

func canvasClipAlpha(clip *canvasClipMask, x, y int) uint8 {
	if y < 0 || y >= len(clip.rows) {
		return 0
	}
	spans := clip.rows[y]
	lo, hi := 0, len(spans)
	for lo < hi {
		mid := (lo + hi) / 2
		if x < spans[mid].x0 {
			hi = mid
		} else if x >= spans[mid].x1 {
			lo = mid + 1
		} else {
			return spans[mid].alpha
		}
	}
	return 0
}

func copyCanvasRegion(dst []byte, dstWidth, dstHeight, x, y int, src []byte, srcWidth, srcHeight int) {
	for row := 0; row < dstHeight; row++ {
		srcY := int64(y) + int64(row)
		if srcY < 0 || srcY >= int64(srcHeight) {
			continue
		}
		for col := 0; col < dstWidth; col++ {
			srcX := int64(x) + int64(col)
			if srcX < 0 || srcX >= int64(srcWidth) {
				continue
			}
			srcOffset := int(srcY)*srcWidth*4 + int(srcX)*4
			dstOffset := (row*dstWidth + col) * 4
			a := int(src[srcOffset+3])
			if a == 0 {
				continue
			}
			for channel := 0; channel < 3; channel++ {
				value := (int(src[srcOffset+channel])*255 + a/2) / a
				if value > 255 {
					value = 255
				}
				dst[dstOffset+channel] = byte(value)
			}
			dst[dstOffset+3] = byte(a)
		}
	}
}
