package js

import (
	"errors"
	"math"
	"math/big"
	"net/url"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// setupWindowPostMessage installs the same-window Window.postMessage path.
// It queues a task, applies targetOrigin filtering, and clones the supported
// JSON-compatible portion of structured-clone data before dispatch.
func (r *Runtime) setupWindowPostMessage(win *goja.Object) {
	_ = win.Set("postMessage", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'postMessage': 1 argument required"))
		}
		targetOrigin := "/"
		var transfer goja.Value
		if len(call.Arguments) > 1 {
			arg := call.Arguments[1]
			if arg != nil && !goja.IsNull(arg) && !goja.IsUndefined(arg) {
				if _, ok := arg.Export().(string); ok {
					targetOrigin = arg.String()
				} else {
					options := arg.ToObject(r.vm)
					if origin := options.Get("targetOrigin"); origin != nil && !goja.IsUndefined(origin) {
						targetOrigin = origin.String()
					}
					transfer = options.Get("transfer")
				}
			}
		}
		if len(call.Arguments) > 2 {
			transfer = call.Arguments[2]
		}
		data, err := cloneMessageValue(r, r, call.Arguments[0])
		if err != nil {
			panic(r.dataCloneError(err.Error()))
		}
		ports, err := r.transferPortsTo(r, transfer)
		if err != nil {
			panic(r.dataCloneError(err.Error()))
		}
		origin := r.windowOrigin()
		if !r.targetOriginMatches(targetOrigin, origin) {
			return goja.Undefined()
		}
		callback := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			event := dom.NewTrustedMessageEvent("message", false, false, data, origin, "", win, ports)
			r.fireWindowListeners(&event.Event, true)
			r.fireWindowListeners(&event.Event, false)
			return goja.Undefined()
		})
		setTimeout, ok := goja.AssertFunction(r.vm.Get("setTimeout"))
		if !ok {
			panic(r.vm.NewTypeError("Window.postMessage requires an active event loop"))
		}
		if _, err := setTimeout(win, callback, r.vm.ToValue(0)); err != nil {
			panic(err)
		}
		return goja.Undefined()
	})
}

// postMessageAcross queues a message in another runtime. The source window is
// supplied in the receiver's realm so MessageEvent.source has the right
// identity and can be used without exporting a goja object across runtimes.
func (r *Runtime) postMessageAcross(call goja.FunctionCall, target *Runtime, sourceWindow goja.Value) goja.Value {
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'postMessage': 1 argument required"))
	}
	targetOrigin, transfer := postMessageOptions(r.vm, call)
	cloned, err := cloneMessageValue(r, target, call.Arguments[0])
	if err != nil {
		panic(r.dataCloneError(err.Error()))
	}
	ports, err := r.transferPortsTo(target, transfer)
	if err != nil {
		panic(r.dataCloneError(err.Error()))
	}
	origin := r.windowOrigin()
	if !r.targetOriginMatches(targetOrigin, target.windowOrigin()) {
		return goja.Undefined()
	}
	dispatch := target.vm.ToValue(func(goja.FunctionCall) goja.Value {
		event := dom.NewTrustedMessageEvent("message", false, false, cloned, origin, "", sourceWindow, ports)
		target.fireWindowListeners(&event.Event, true)
		target.fireWindowListeners(&event.Event, false)
		return goja.Undefined()
	})
	setTimeout, ok := goja.AssertFunction(target.vm.Get("setTimeout"))
	if !ok {
		panic(r.vm.NewTypeError("Window.postMessage requires an active event loop"))
	}
	if _, err := setTimeout(target.vm.GlobalObject(), dispatch, target.vm.ToValue(0)); err != nil {
		panic(r.vm.NewGoError(err))
	}
	return goja.Undefined()
}

// transferPortsTo moves MessagePort wrappers into the destination runtime while
// preserving the endpoint pair and any messages already queued on the port.
func (r *Runtime) transferPortsTo(target *Runtime, transfer goja.Value) ([]any, error) {
	values := transferList(r.vm, transfer)
	ports := make([]any, 0, len(values))
	seen := make(map[*messagePort]bool, len(values))
	seenObjects := make(map[*goja.Object]bool, len(values))
	for _, value := range values {
		obj, ok := value.(*goja.Object)
		if !ok || seenObjects[obj] {
			return nil, errors.New("invalid or duplicate transferable")
		}
		seenObjects[obj] = true
		if isArrayBufferObject(obj) {
			continue
		}
		if canvas := r.offscreenCanvases[obj]; canvas != nil {
			if canvas.detached {
				return nil, errors.New("OffscreenCanvas is detached")
			}
			continue
		}
		if bitmap := r.imageBitmapObjects[obj]; bitmap != nil {
			if bitmap.closed {
				return nil, errors.New("ImageBitmap is detached")
			}
			continue
		}
		port := messagePortFromObject(r.vm, value)
		if port == nil || port.r != r {
			return nil, errors.New("object is not transferable")
		}
		port.mu.Lock()
		closed := port.closed
		port.mu.Unlock()
		if closed {
			return nil, errors.New("MessagePort is already closed")
		}
	}
	for _, value := range values {
		port := messagePortFromObject(r.vm, value)
		if port == nil {
			if obj, ok := value.(*goja.Object); ok {
				if isArrayBufferObject(obj) {
					continue
				}
				if canvas := r.offscreenCanvases[obj]; canvas != nil && !canvas.detached {
					canvas.detached = true
					canvas.context = nil
					canvas.contextObject = nil
					continue
				}
				if bitmap := r.imageBitmapObjects[obj]; bitmap != nil && !bitmap.closed {
					bitmap.closed = true
					bitmap.pixels = nil
					bitmap.width, bitmap.height = 0, 0
					continue
				}
			}
			return nil, errors.New("object is not a transferable MessagePort")
		}
		if port.r != r || seen[port] {
			return nil, errors.New("invalid or duplicate MessagePort transfer")
		}
		seen[port] = true
		port.mu.Lock()
		if port.closed {
			port.mu.Unlock()
			return nil, errors.New("MessagePort is already closed")
		}
		endpoint := port.endpoint
		queue := append([]messagePortMessage(nil), port.queue...)
		started := port.started
		port.closed = true
		port.queue = nil
		port.mu.Unlock()
		stateValue := target.vm.Get(messageChannelStateKey)
		if stateValue == nil || goja.IsUndefined(stateValue) {
			return nil, errors.New("target does not support MessagePort")
		}
		state, ok := stateValue.Export().(*messageChannelState)
		if !ok || state == nil {
			return nil, errors.New("target does not support MessagePort")
		}
		moved := target.newMessagePort(state, endpoint)
		newPort := messagePortFromObject(target.vm, moved)
		newPort.mu.Lock()
		newPort.queue = queue
		newPort.started = started
		newPort.mu.Unlock()
		ports = append(ports, moved)
	}
	return ports, nil
}

func postMessageOptions(vm *goja.Runtime, call goja.FunctionCall) (string, goja.Value) {
	targetOrigin := "/"
	var transfer goja.Value
	if len(call.Arguments) > 1 {
		arg := call.Arguments[1]
		if arg != nil && !goja.IsNull(arg) && !goja.IsUndefined(arg) {
			if _, ok := arg.Export().(string); ok {
				targetOrigin = arg.String()
			} else {
				options := arg.ToObject(vm)
				if origin := options.Get("targetOrigin"); origin != nil && !goja.IsUndefined(origin) {
					targetOrigin = origin.String()
				}
				transfer = options.Get("transfer")
			}
		}
	}
	if len(call.Arguments) > 2 {
		transfer = call.Arguments[2]
	}
	return targetOrigin, transfer
}

func (r *Runtime) dataCloneError(message string) *goja.Object {
	if constructor, ok := goja.AssertConstructor(r.vm.Get("DOMException")); ok {
		if exception, err := constructor(nil, r.vm.ToValue(message), r.vm.ToValue("DataCloneError")); err == nil {
			return exception
		}
	}
	exception := r.vm.NewGoError(errors.New(message))
	_ = exception.Set("name", "DataCloneError")
	_ = exception.Set("code", 25)
	return exception
}

func cloneArrayBufferToRuntime(sourceRuntime, destination *Runtime, source goja.ArrayBuffer) (goja.Value, error) {
	if sourceRuntime == nil || destination == nil || source.Detached() {
		return nil, errBroadcastDataClone
	}
	if source.Shared() && (!sourceRuntime.crossOriginIsolated || !destination.crossOriginIsolated) {
		return nil, errBroadcastDataClone
	}
	data := source.Bytes()
	maxByteLength := source.MaxByteLength()
	if len(data) > maxBroadcastCloneBytes || maxByteLength < len(data) || maxByteLength > maxBroadcastCloneBytes {
		return nil, errBroadcastDataClone
	}
	var cloned goja.ArrayBuffer
	switch {
	case source.Shared() && source.Resizable():
		cloned = destination.vm.NewGrowableSharedArrayBuffer(append([]byte(nil), data...), maxByteLength)
	case source.Shared():
		cloned = destination.vm.NewSharedArrayBuffer(append([]byte(nil), data...))
	case source.Resizable():
		cloned = destination.vm.NewResizableArrayBuffer(append([]byte(nil), data...), maxByteLength)
	default:
		cloned = destination.vm.NewArrayBuffer(append([]byte(nil), data...))
	}
	return destination.vm.ToValue(cloned), nil
}

func hasTransferItems(value goja.Value, vm *goja.Runtime) bool {
	if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
		return false
	}
	length := value.ToObject(vm).Get("length")
	return length != nil && !goja.IsUndefined(length) && length.ToInteger() > 0
}

func cloneMessageValue(source, destination *Runtime, value goja.Value) (goja.Value, error) {
	if source == nil || destination == nil {
		return nil, errBroadcastDataClone
	}
	cloner := messageValueCloner{source: source, destination: destination, seen: make(map[*goja.Object]goja.Value)}
	return cloner.clone(value, 0)
}

type messageValueCloner struct {
	source      *Runtime
	destination *Runtime
	seen        map[*goja.Object]goja.Value
	transfers   map[*goja.Object]goja.Value
	nodes       int
}

func (c *messageValueCloner) clone(value goja.Value, depth int) (goja.Value, error) {
	if depth > maxBroadcastCloneDepth {
		return nil, errBroadcastDataClone
	}
	if value == nil || goja.IsUndefined(value) {
		return goja.Undefined(), nil
	}
	if goja.IsNull(value) {
		return goja.Null(), nil
	}
	if _, ok := value.(*goja.Symbol); ok {
		return nil, errBroadcastDataClone
	}
	if _, callable := goja.AssertFunction(value); callable {
		return nil, errBroadcastDataClone
	}
	object, isObject := value.(*goja.Object)
	if !isObject {
		// Primitive goja values are immutable and can be shared between realms.
		// Returning them directly preserves NaN, infinities, -0, BigInt, and
		// UTF-16 strings containing unpaired surrogates without a lossy JSON hop.
		return value, nil
	}
	if transferred := c.transfers[object]; transferred != nil {
		return transferred, nil
	}
	if messagePortFromObject(c.source.vm, object) != nil {
		return nil, errBroadcastDataClone
	}
	// Response has no structured-serialize steps in the HTML Standard. Check
	// prototype identity (including subclasses) without reading user getters.
	if c.source.responseProto != nil {
		for prototype := object; prototype != nil; prototype = prototype.Prototype() {
			if prototype == c.source.responseProto {
				return nil, errBroadcastDataClone
			}
		}
	}
	if prior := c.seen[object]; prior != nil {
		return prior, nil
	}
	c.nodes++
	if c.nodes > maxBroadcastCloneNodes {
		return nil, errBroadcastDataClone
	}
	// ArrayBuffer and SharedArrayBuffer are branded platform objects rather
	// than ordinary objects. Preserve their native slots and resize mode when
	// cloning them into the destination runtime.
	if buffer, ok := object.Export().(goja.ArrayBuffer); ok {
		if buffer.Detached() {
			return nil, errBroadcastDataClone
		}
		cloned, err := cloneArrayBufferToRuntime(c.source, c.destination, buffer)
		if err != nil {
			return nil, errBroadcastDataClone
		}
		c.seen[object] = cloned
		return cloned, nil
	}
	// ArrayBuffer views must be detected from their internal slots. Exported
	// Go slices lose the distinction between fixed-length and length-tracking
	// views and do not reveal whether a view is currently out of bounds.
	if viewInfo, ok := c.source.vm.ArrayBufferViewInfo(object); ok {
		if viewInfo.OutOfBounds || viewInfo.ByteOffset < 0 || viewInfo.ByteLength < 0 || viewInfo.ByteOffset > maxBroadcastCloneBytes || viewInfo.ByteLength > maxBroadcastCloneBytes || viewInfo.ElementSize <= 0 {
			return nil, errBroadcastDataClone
		}
		bufferValue, err := c.clone(viewInfo.Buffer, depth+1)
		if err != nil {
			return nil, err
		}
		bufferObject, ok := bufferValue.(*goja.Object)
		if !ok {
			return nil, errBroadcastDataClone
		}
		name := ""
		constructor := c.destination.vm.Get("DataView")
		args := []goja.Value{bufferObject, c.destination.vm.ToValue(viewInfo.ByteOffset)}
		if !viewInfo.DataView {
			name = viewInfo.InterfaceName
			constructor = c.destination.vm.Get(name)
			if constructor == nil || goja.IsUndefined(constructor) || goja.IsNull(constructor) {
				return nil, errBroadcastDataClone
			}
			if !viewInfo.LengthTracking {
				if viewInfo.ByteLength%viewInfo.ElementSize != 0 {
					return nil, errBroadcastDataClone
				}
				args = append(args, c.destination.vm.ToValue(viewInfo.ByteLength/viewInfo.ElementSize))
			}
		} else if !viewInfo.LengthTracking {
			args = append(args, c.destination.vm.ToValue(viewInfo.ByteLength))
		}
		cloned, err := c.destination.vm.New(constructor, args...)
		if err != nil {
			return nil, errBroadcastDataClone
		}
		c.seen[object] = cloned
		return cloned, nil
	}
	if files, ok := c.source.fileListValues[object]; ok {
		if c.destination.fileListProto == nil {
			return nil, errBroadcastDataClone
		}
		clonedList := c.destination.newFileList(nil)
		c.seen[object] = clonedList
		clonedFiles := make([]*goja.Object, 0, len(files))
		for _, file := range files {
			cloned, err := c.clone(file, depth+1)
			if err != nil {
				return nil, err
			}
			clonedFile, ok := cloned.(*goja.Object)
			if !ok {
				return nil, errBroadcastDataClone
			}
			clonedFiles = append(clonedFiles, clonedFile)
		}
		c.destination.setFileListFiles(clonedList, clonedFiles)
		return clonedList, nil
	}
	if bitmap := c.source.imageBitmapObjects[object]; bitmap != nil {
		if bitmap.closed || c.destination.imageBitmapProto == nil || bitmap.width <= 0 || bitmap.height <= 0 || bitmap.width > math.MaxInt/4 || bitmap.height > maxImageDataBytes/(bitmap.width*4) || len(bitmap.pixels) != bitmap.width*bitmap.height*4 {
			return nil, errBroadcastDataClone
		}
		cloned := c.destination.newImageBitmap(bitmap.width, bitmap.height, bitmap.pixels)
		c.seen[object] = cloned
		return cloned, nil
	}
	if canvas := c.source.offscreenCanvases[object]; canvas != nil {
		if canvas.detached || c.destination.offscreenCanvasProto == nil || canvas.width < 0 || canvas.height < 0 || canvas.width > math.MaxInt/4 || (canvas.width > 0 && canvas.height > maxImageDataBytes/(canvas.width*4)) {
			return nil, errBroadcastDataClone
		}
		cloned := c.source.cloneOffscreenCanvas(c.destination, canvas)
		c.seen[object] = cloned
		return cloned, nil
	}

	class := object.ClassName()
	if _, isImageData := c.source.imageDataObjects[object]; isImageData {
		width := int(object.Get("width").ToInteger())
		height := int(object.Get("height").ToInteger())
		sourceData, ok := object.Get("data").(*goja.Object)
		clampedArrayCtor, hasClampedArrayCtor := c.source.vm.Get("Uint8ClampedArray").(*goja.Object)
		if !ok || !hasClampedArrayCtor || !c.source.vm.InstanceOf(sourceData, clampedArrayCtor) || width <= 0 || height <= 0 {
			return nil, errBroadcastDataClone
		}
		length := int(sourceData.Get("length").ToInteger())
		if length != width*height*4 || length > maxImageDataBytes {
			return nil, errBroadcastDataClone
		}
		clonedData := c.destination.newUint8ClampedArray(length)
		for index := 0; index < length; index++ {
			if err := clonedData.Set(strconv.Itoa(index), sourceData.Get(strconv.Itoa(index))); err != nil {
				return nil, errBroadcastDataClone
			}
		}
		cloned := c.destination.newImageData(width, height, clonedData)
		c.seen[object] = cloned
		return cloned, nil
	}
	if file := c.source.fileValues[object]; file != nil {
		if len(file.data) > maxFileBytes {
			return nil, errBroadcastDataClone
		}
		if (!file.isFile && c.destination.blobProto == nil) || (file.isFile && c.destination.fileProto == nil) {
			return nil, errBroadcastDataClone
		}
		cloned := c.destination.makeBlob(nil, file.data, file.typeValue, file.isFile, file.name, file.lastModified)
		c.seen[object] = cloned
		return cloned, nil
	}
	if bigint, ok := object.Export().(*big.Int); ok {
		boxed, err := c.destination.vm.New(c.destination.vm.Get("Object"), c.destination.vm.ToValue(bigint))
		if err != nil {
			return nil, errBroadcastDataClone
		}
		c.seen[object] = boxed
		return boxed, nil
	}
	switch class {
	case "Boolean", "Number", "String", "BigInt":
		valueOf, ok := goja.AssertFunction(object.Get("valueOf"))
		if !ok {
			return nil, errBroadcastDataClone
		}
		primitive, err := valueOf(object)
		if err != nil {
			return nil, errBroadcastDataClone
		}
		cloned, err := c.clone(primitive, depth+1)
		if err != nil {
			return nil, err
		}
		constructor := c.destination.vm.Get(class)
		if class == "BigInt" {
			constructor = c.destination.vm.Get("Object")
		}
		wrapper, err := c.destination.vm.New(constructor, cloned)
		if err != nil {
			return nil, errBroadcastDataClone
		}
		c.seen[object] = wrapper
		return wrapper, nil
	case "Date":
		valueOf, ok := goja.AssertFunction(object.Get("valueOf"))
		if !ok {
			return nil, errBroadcastDataClone
		}
		milliseconds, err := valueOf(object)
		if err != nil {
			return nil, errBroadcastDataClone
		}
		cloned, err := c.clone(milliseconds, depth+1)
		if err != nil {
			return nil, err
		}
		date, err := c.destination.vm.New(c.destination.vm.Get("Date"), cloned)
		if err != nil {
			return nil, errBroadcastDataClone
		}
		c.seen[object] = date
		return date, nil
	case "RegExp":
		source, flags := object.Get("source"), object.Get("flags")
		// Goja serializes escaped slashes as a Unicode escape in the source
		// getter, while RegExp.prototype.source uses the short escaped-slash
		// spelling required by the web platform.
		sourceText := strings.NewReplacer("\\u002f", "\\/", "\\u002F", "\\/").Replace(source.String())
		regexp, err := c.destination.vm.New(c.destination.vm.Get("RegExp"), c.destination.vm.ToValue(sourceText), flags)
		if err != nil {
			return nil, errBroadcastDataClone
		}
		c.seen[object] = regexp
		return regexp, nil
	case "Error":
		return c.cloneError(object, depth)
	case "Array":
		length := object.Get("length").ToInteger()
		if length < 0 || length > maxBroadcastCloneNodes {
			return nil, errBroadcastDataClone
		}
		array := c.destination.vm.NewArray()
		c.seen[object] = array
		for _, key := range object.Keys() {
			cloned, err := c.clone(object.Get(key), depth+1)
			if err != nil {
				return nil, err
			}
			if err := array.Set(key, cloned); err != nil {
				return nil, errBroadcastDataClone
			}
		}
		if err := array.Set("length", length); err != nil {
			return nil, errBroadcastDataClone
		}
		return array, nil
	case "Object":
		cloned := c.destination.vm.NewObject()
		c.seen[object] = cloned
		for _, key := range object.Keys() {
			item, err := c.clone(object.Get(key), depth+1)
			if err != nil {
				return nil, err
			}
			if err := cloned.DefineDataProperty(key, item, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
				return nil, errBroadcastDataClone
			}
		}
		return cloned, nil
	default:
		return nil, errBroadcastDataClone
	}
}

func cloneMessageValueWithTransfers(source, destination *Runtime, value goja.Value, transfers map[*goja.Object]goja.Value) (goja.Value, error) {
	if source == nil || destination == nil {
		return nil, errBroadcastDataClone
	}
	cloner := messageValueCloner{source: source, destination: destination, seen: make(map[*goja.Object]goja.Value), transfers: transfers}
	return cloner.clone(value, 0)
}

func (c *messageValueCloner) cloneError(source *goja.Object, depth int) (goja.Value, error) {
	nameValue := source.Get("name")
	name := "Error"
	if nameValue != nil && !goja.IsUndefined(nameValue) {
		name = nameValue.String()
	}
	constructor := c.destination.vm.Get(name)
	if _, ok := goja.AssertConstructor(constructor); !ok {
		name = "Error"
		constructor = c.destination.vm.Get("Error")
	}
	message := source.Get("message")
	if message == nil {
		message = goja.Undefined()
	}
	args := []goja.Value(nil)
	if hasOwnPropertyName(source, "message") {
		args = []goja.Value{message}
	}
	cloned, err := c.destination.vm.New(constructor, args...)
	if err != nil {
		return nil, errBroadcastDataClone
	}
	c.seen[source] = cloned
	// Error cause is an own structured-serializable data member. AggregateError
	// additionally carries its errors sequence. Neither the stack nor arbitrary
	// custom own properties are part of Error serialization.
	for _, property := range []string{"cause", "errors"} {
		if !hasOwnPropertyName(source, property) {
			continue
		}
		value, err := c.clone(source.Get(property), depth+1)
		if err != nil {
			return nil, err
		}
		if err := cloned.Set(property, value); err != nil {
			return nil, errBroadcastDataClone
		}
	}
	if hasOwnPropertyName(source, "name") {
		if err := cloned.Set("name", nameValue); err != nil {
			return nil, errBroadcastDataClone
		}
	}
	return cloned, nil
}

func hasOwnPropertyName(object *goja.Object, name string) bool {
	for _, property := range object.GetOwnPropertyNames() {
		if property == name {
			return true
		}
	}
	return false
}

func (r *Runtime) windowOrigin() string {
	u, err := url.Parse(r.opts.URL)
	if err != nil || u.Host == "" {
		return "null"
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ws", "wss":
		return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
	default:
		return "null"
	}
}

func (r *Runtime) targetOriginMatches(targetOrigin, origin string) bool {
	if targetOrigin == "*" {
		return true
	}
	if targetOrigin == "/" {
		return r.windowOrigin() == origin
	}
	u, err := url.Parse(targetOrigin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	target := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
	return target == origin
}
