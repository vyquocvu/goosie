package js

import (
	"math"

	"github.com/dop251/goja"
)

// offscreenCanvasData keeps the small, bounded subset of OffscreenCanvas used
// by scripts and structured clone. Pixels are copied with the canvas.
type offscreenCanvasData struct {
	width, height int
	context       *canvasContext2D
	contextObject *goja.Object
	detached      bool
}

func (r *Runtime) setupOffscreenCanvas() {
	proto := r.vm.NewObject()
	ctor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		w, h := 0, 0
		if len(call.Arguments) > 0 {
			w = int(call.Arguments[0].ToInteger())
		}
		if len(call.Arguments) > 1 {
			h = int(call.Arguments[1].ToInteger())
		}
		if w < 0 || h < 0 || w > math.MaxInt/4 || (w > 0 && h > maxImageDataBytes/(w*4)) {
			panic(r.vm.NewTypeError("OffscreenCanvas dimensions exceed the implementation limit"))
		}
		o := call.This
		_ = o.SetPrototype(proto)
		data := &offscreenCanvasData{width: w, height: h}
		r.offscreenCanvases[o] = data
		return o
	}).(*goja.Object)
	_ = ctor.Set("prototype", proto)
	_ = proto.Set("constructor", ctor)
	r.defineInterfaceToStringTag("OffscreenCanvas", proto)
	r.offscreenCanvasProto = proto
	for _, key := range []string{"width", "height"} {
		name := key
		get := r.vm.ToValue(func(c goja.FunctionCall) goja.Value {
			d := r.offscreenCanvases[c.This.ToObject(r.vm)]
			if d == nil || d.detached {
				return r.vm.ToValue(0)
			}
			if name == "width" {
				return r.vm.ToValue(d.width)
			}
			return r.vm.ToValue(d.height)
		})
		set := r.vm.ToValue(func(c goja.FunctionCall) goja.Value {
			d := r.offscreenCanvases[c.This.ToObject(r.vm)]
			if d == nil || d.detached {
				return goja.Undefined()
			}
			n := int(c.Argument(0).ToInteger())
			if n < 0 {
				n = 0
			}
			if n > 0 && (n > math.MaxInt/4 || d.height > maxImageDataBytes/(n*4)) {
				panic(r.vm.NewTypeError("OffscreenCanvas dimensions exceed the implementation limit"))
			}
			if name == "width" {
				d.width = n
			} else {
				d.height = n
			}
			d.context = nil
			d.contextObject = nil
			return goja.Undefined()
		})
		_ = proto.DefineAccessorProperty(key, get, set, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = proto.Set("getContext", func(c goja.FunctionCall) goja.Value {
		d := r.offscreenCanvases[c.This.ToObject(r.vm)]
		if d == nil || d.detached || len(c.Arguments) == 0 || c.Argument(0).String() != "2d" {
			return goja.Null()
		}
		if d.contextObject != nil {
			return d.contextObject
		}
		ctx := &canvasContext2D{width: d.width, height: d.height, fillStyle: "#000000", strokeStyle: "#000000", lineWidth: 1, font: "10px sans-serif", textAlign: "start", textBaseline: "alphabetic"}
		o := r.vm.NewObject()
		_ = o.Set("canvas", c.This)
		_ = o.Set("fillStyle", ctx.fillStyle)
		_ = o.Set("strokeStyle", ctx.strokeStyle)
		_ = o.Set("lineWidth", ctx.lineWidth)
		_ = o.Set("font", ctx.font)
		_ = o.Set("textAlign", ctx.textAlign)
		_ = o.Set("textBaseline", ctx.textBaseline)
		r.setupCanvasContextMethods(o, ctx)
		d.context = ctx
		d.contextObject = o
		return o
	})
	_ = r.vm.Set("OffscreenCanvas", ctor)
	if w, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = w.Set("OffscreenCanvas", ctor)
	}
}

func (r *Runtime) cloneOffscreenCanvas(dest *Runtime, d *offscreenCanvasData) *goja.Object {
	o, err := dest.vm.New(dest.vm.Get("OffscreenCanvas"), dest.vm.ToValue(d.width), dest.vm.ToValue(d.height))
	if err != nil {
		panic(err)
	}
	cloned := o
	target := dest.offscreenCanvases[cloned]
	if d.context != nil {
		d.context.mu.Lock()
		cp := &canvasContext2D{
			width:                    d.context.width,
			height:                   d.context.height,
			fillStyle:                d.context.fillStyle,
			strokeStyle:              d.context.strokeStyle,
			lineWidth:                d.context.lineWidth,
			font:                     d.context.font,
			textAlign:                d.context.textAlign,
			textBaseline:             d.context.textBaseline,
			imageSmoothingEnabled:    d.context.imageSmoothingEnabled,
			imageSmoothingQuality:    d.context.imageSmoothingQuality,
			filter:                   d.context.filter,
			globalAlpha:              d.context.globalAlpha,
			globalCompositeOperation: d.context.globalCompositeOperation,
			shadowBlur:               d.context.shadowBlur,
			shadowColor:              d.context.shadowColor,
			shadowOffsetX:            d.context.shadowOffsetX,
			shadowOffsetY:            d.context.shadowOffsetY,
			lineCap:                  d.context.lineCap,
			lineJoin:                 d.context.lineJoin,
			miterLimit:               d.context.miterLimit,
			ops:                      append([]canvasDrawOp(nil), d.context.ops...),
			nodeID:                   d.context.nodeID,
			bitmap:                   append([]byte(nil), d.context.bitmap...),
			bitmapW:                  d.context.bitmapW,
			bitmapH:                  d.context.bitmapH,
		}
		d.context.mu.Unlock()
		target.context = cp
		wrapper := dest.vm.NewObject()
		_ = wrapper.Set("canvas", cloned)
		_ = wrapper.Set("fillStyle", cp.fillStyle)
		_ = wrapper.Set("strokeStyle", cp.strokeStyle)
		_ = wrapper.Set("lineWidth", cp.lineWidth)
		_ = wrapper.Set("font", cp.font)
		_ = wrapper.Set("textAlign", cp.textAlign)
		_ = wrapper.Set("textBaseline", cp.textBaseline)
		dest.setupCanvasContextMethods(wrapper, cp)
		target.contextObject = wrapper
	}
	return cloned
}
