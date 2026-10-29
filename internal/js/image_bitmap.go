package js

import (
	"math"
	"strconv"

	"github.com/dop251/goja"
)

// ImageBitmap is represented by runtime-owned immutable dimensions and pixels.
// The current canvas implementation does not rasterize synchronously, so a
// bitmap created from a canvas without a committed bitmap starts transparent.
type imageBitmapData struct {
	width, height int
	pixels        []byte
	closed        bool
}

func (r *Runtime) setupImageBitmap() {
	prototype := r.vm.NewObject()
	constructor := r.vm.ToValue(func(goja.ConstructorCall) *goja.Object {
		panic(r.vm.NewTypeError("Illegal constructor"))
	}).(*goja.Object)
	_ = constructor.Set("prototype", prototype)
	_ = prototype.Set("constructor", constructor)
	r.defineInterfaceToStringTag("ImageBitmap", prototype)
	r.imageBitmapProto = prototype
	_ = prototype.Set("close", func(call goja.FunctionCall) goja.Value {
		if data := r.imageBitmapObjects[call.This.ToObject(r.vm)]; data != nil {
			data.closed = true
			data.pixels = nil
			data.width, data.height = 0, 0
		}
		return goja.Undefined()
	})
	for _, property := range []string{"width", "height"} {
		name := property
		getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if data := r.imageBitmapObjects[call.This.ToObject(r.vm)]; data != nil {
				if name == "width" {
					return r.vm.ToValue(data.width)
				}
				return r.vm.ToValue(data.height)
			}
			return r.vm.ToValue(0)
		})
		_ = prototype.DefineAccessorProperty(name, getter, nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	_ = r.vm.Set("ImageBitmap", constructor)
	if window, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = window.Set("ImageBitmap", constructor)
	}
	create := func(call goja.FunctionCall) goja.Value {
		promise := r.newPromiseObj()
		if len(call.Arguments) == 0 {
			r.rejectPromise(promise, r.vm.NewTypeError("Failed to execute 'createImageBitmap': source is required"))
			return promise.obj
		}
		node := r.getNode(call.Arguments[0])
		if source, ok := call.Arguments[0].(*goja.Object); ok {
			if offscreen := r.offscreenCanvases[source]; offscreen != nil && !offscreen.detached {
				var pixels []byte
				if offscreen.context != nil {
					offscreen.context.mu.Lock()
					pixels = append([]byte(nil), offscreen.context.bitmap...)
					offscreen.context.mu.Unlock()
				}
				r.resolvePromise(promise, r.newImageBitmap(offscreen.width, offscreen.height, pixels))
				return promise.obj
			}
		}
		if node == nil || node.Data != "canvas" {
			r.rejectPromise(promise, r.vm.NewTypeError("Unsupported ImageBitmap source"))
			return promise.obj
		}
		width, height := canvasDimension(node.GetAttribute("width"), 300), canvasDimension(node.GetAttribute("height"), 150)
		var pixels []byte
		if r.canvasContexts != nil {
			for _, context := range r.canvasContexts {
				if context != nil && context.nodeID == int(node.ID) {
					context.mu.Lock()
					if context.bitmapW == width && context.bitmapH == height && len(context.bitmap) == width*height*4 {
						pixels = append([]byte(nil), context.bitmap...)
					}
					context.mu.Unlock()
					break
				}
			}
		}
		if pixels == nil {
			pixels = make([]byte, width*height*4)
		}
		r.resolvePromise(promise, r.newImageBitmap(width, height, pixels))
		return promise.obj
	}
	_ = r.vm.Set("createImageBitmap", create)
	if window, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = window.Set("createImageBitmap", create)
	}
}

func canvasDimension(raw string, fallback int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	if value == 0 {
		return fallback
	}
	return value
}

func (r *Runtime) newImageBitmap(width, height int, pixels []byte) *goja.Object {
	if width <= 0 || height <= 0 || width > math.MaxInt/4 || height > maxImageDataBytes/(width*4) {
		panic(r.vm.NewTypeError("ImageBitmap dimensions exceed the implementation limit"))
	}
	length := width * height * 4
	if len(pixels) != length {
		pixels = make([]byte, length)
	} else {
		pixels = append([]byte(nil), pixels...)
	}
	object := r.vm.NewObject()
	_ = object.SetPrototype(r.imageBitmapProto)
	r.imageBitmapObjects[object] = &imageBitmapData{width: width, height: height, pixels: pixels}
	return object
}
