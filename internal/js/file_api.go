package js

import (
	"bytes"
	"encoding/base64"
	"errors"
	"math"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dop251/goja"

	"github.com/vyquocvu/goosie/internal/dom"
)

const maxFileBytes = 64 << 20

type fileAPITypeError struct {
	value *goja.Object
}

func (e fileAPITypeError) Error() string { return e.value.String() }

func (r *Runtime) panicFileAPIError(err error) {
	if typeError, ok := err.(fileAPITypeError); ok {
		panic(typeError.value)
	}
	panic(err)
}

func (r *Runtime) newFileAPITypeError(message string) error {
	return fileAPITypeError{value: r.vm.NewTypeError(message)}
}

func (r *Runtime) filePropertyBag(value goja.Value) (*goja.Object, error) {
	if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
		return nil, nil
	}
	if object, ok := value.(*goja.Object); ok {
		return object, nil
	}
	return nil, r.newFileAPITypeError("Blob/File options must be an object")
}

type browserFile struct {
	data         []byte
	name         string
	typeValue    string
	lastModified int64
	isFile       bool
}

type fileReaderState struct {
	reader      *goja.Object
	readyState  int
	result      goja.Value
	error       goja.Value
	handlers    map[string]goja.Value
	listeners   map[string][]goja.Value
	readAborted bool
}

type blobPart struct {
	data   []byte
	text   string
	isText bool
}

func (r *Runtime) setupFileAPI() {
	r.fileListProto = r.vm.NewObject()
	fileListCtor := r.vm.ToValue(func(goja.ConstructorCall) *goja.Object {
		panic(r.vm.NewTypeError("Illegal constructor"))
	}).(*goja.Object)
	_ = fileListCtor.Set("prototype", r.fileListProto)
	_ = r.fileListProto.Set("constructor", fileListCtor)
	_ = r.fileListProto.Set("item", func(call goja.FunctionCall) goja.Value {
		items := r.fileListValues[r.asObject(call.This)]
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		index := int(call.Argument(0).ToInteger())
		if index < 0 || index >= len(items) {
			return goja.Null()
		}
		return items[index]
	})
	r.defineInterfaceToStringTag("FileList", r.fileListProto)
	_ = r.vm.Set("FileList", fileListCtor)
	if win, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = win.Set("FileList", fileListCtor)
	}

	_ = r.vm.Set("__goosieBlobInit", func(call goja.FunctionCall) goja.Value {
		return r.initializeBlob(call.Argument(0), call.Argument(1), call.Argument(2))
	})
	if _, err := r.vm.RunString(`globalThis.Blob = (function(init) { return class Blob { constructor(blobParts = [], options = {}) { init(this, blobParts, options); } }; })(__goosieBlobInit)`); err != nil {
		panic(err)
	}
	blobConstructor := r.vm.Get("Blob").ToObject(r.vm)
	blobProto := blobConstructor.Get("prototype").ToObject(r.vm)
	r.blobProto = blobProto
	_ = blobProto.DefineAccessorProperty("size", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if file := r.fileValues[r.asObject(call.This)]; file != nil {
			return r.vm.ToValue(len(file.data))
		}
		return r.vm.ToValue(0)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = blobProto.DefineAccessorProperty("type", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if file := r.fileValues[r.asObject(call.This)]; file != nil {
			return r.vm.ToValue(file.typeValue)
		}
		return r.vm.ToValue("")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = blobProto.Set("slice", func(call goja.FunctionCall) goja.Value {
		data := r.fileValues[r.asObject(call.This)]
		if data == nil {
			return goja.Null()
		}
		start, end := int64(0), int64(len(data.data))
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
			start = webIDLToLongLong(call.Argument(0))
		}
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
			end = webIDLToLongLong(call.Argument(1))
		}
		length := int64(len(data.data))
		if start < 0 {
			start = max(0, length+start)
		} else {
			start = min(start, length)
		}
		if end < 0 {
			end = max(0, length+end)
		} else {
			end = min(end, length)
		}
		if end < start {
			end = start
		}
		mime := ""
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
			mime = normalizeMIMEType(call.Argument(2).String())
		}
		return r.newBlob(bytes.Clone(data.data[start:end]), mime)
	})
	_ = blobProto.Set("text", func(call goja.FunctionCall) goja.Value {
		data := r.fileValues[r.asObject(call.This)]
		if data == nil {
			return goja.Null()
		}
		promise := r.newPromiseObj()
		r.resolvePromise(promise, r.vm.ToValue(decodeBlobText(data.data)))
		return promise.obj
	})
	_ = blobProto.Set("arrayBuffer", func(call goja.FunctionCall) goja.Value {
		data := r.fileValues[r.asObject(call.This)]
		if data == nil {
			return goja.Null()
		}
		promise := r.newPromiseObj()
		r.resolvePromise(promise, r.vm.ToValue(r.vm.NewArrayBuffer(bytes.Clone(data.data))))
		return promise.obj
	})
	_ = blobProto.Set("bytes", func(call goja.FunctionCall) goja.Value {
		data := r.fileValues[r.asObject(call.This)]
		if data == nil {
			return goja.Null()
		}
		promise := r.newPromiseObj()
		r.resolvePromise(promise, r.newUint8Array(data.data))
		return promise.obj
	})
	if _, err := r.vm.RunString(`
		if (typeof globalThis.ReadableStream !== "function") {
			globalThis.ReadableStream = class ReadableStream {
				constructor(source = {}) {
					if (source === null || typeof source !== "object") throw new TypeError("ReadableStream source must be an object");
					this._blobBytes = source.__goosieBlobBytes instanceof Uint8Array ? source.__goosieBlobBytes : new Uint8Array(0);
				this._blobText = typeof source.__goosieBlobText === "string" ? source.__goosieBlobText : null;
				this._blobTextRead = false;
					this._blobOffset = 0;
					this._blobLocked = false;
				}
				get locked() { return this._blobLocked; }
				getReader(options = {}) {
					if (this._blobLocked) throw new TypeError("ReadableStream is locked");
					const mode = options && options.mode;
					if (mode !== undefined && mode !== "byob") throw new RangeError("Unsupported reader mode");
					if (mode === "byob" && !this._blobByteStream) throw new TypeError("BYOB requires a byte stream");
					this._blobLocked = true;
					const stream = this;
					let released = false;
					return {
						read(view) {
							if (released) return Promise.reject(new TypeError("Reader lock was released"));
						if (stream._blobText !== null) {
							if (stream._blobTextRead || stream._blobText.length === 0) return Promise.resolve({value: undefined, done: true});
						stream._blobTextRead = true;
						return Promise.resolve({value: stream._blobText, done: false});
						}
						if (mode === "byob") {
							if (!(view instanceof Uint8Array) || view.byteLength === 0) return Promise.reject(new TypeError("BYOB read requires a non-empty Uint8Array"));
							if (stream._blobOffset >= stream._blobBytes.length) return Promise.resolve({value: undefined, done: true});
							const count = Math.min(view.byteLength, stream._blobBytes.length - stream._blobOffset);
							view.set(stream._blobBytes.subarray(stream._blobOffset, stream._blobOffset + count));
							stream._blobOffset += count;
							return Promise.resolve({value: view.subarray(0, count), done: false});
						}
						if (stream._blobOffset >= stream._blobBytes.length) return Promise.resolve({value: undefined, done: true});
						const end = Math.min(stream._blobOffset + 65536, stream._blobBytes.length);
						const value = stream._blobBytes.slice(stream._blobOffset, end);
						stream._blobOffset = end;
						return Promise.resolve({value, done: false});
						},
						cancel() { stream._blobOffset = stream._blobBytes.length; return Promise.resolve(); },
						releaseLock() { if (!released) { released = true; stream._blobLocked = false; } }
					};
				}
			};
		}
		globalThis.__goosieMakeBlobStream = function(bytes) {
			const stream = new ReadableStream({__goosieBlobBytes: bytes});
			stream._blobByteStream = true;
			return stream;
		};
		globalThis.__goosieMakeBlobTextStream = function(text) {
			return new ReadableStream({__goosieBlobText: text});
		};
	`); err != nil {
		panic(err)
	}
	blobStreamFactory, ok := goja.AssertFunction(r.vm.Get("__goosieMakeBlobStream"))
	if !ok {
		panic(r.vm.NewTypeError("Blob stream factory is unavailable"))
	}
	_ = blobProto.Set("stream", func(call goja.FunctionCall) goja.Value {
		data := r.fileValues[r.asObject(call.This)]
		if data == nil {
			return goja.Null()
		}
		stream, err := blobStreamFactory(goja.Undefined(), r.newUint8Array(data.data))
		if err != nil {
			panic(err)
		}
		return stream
	})
	textStreamFactory, ok := goja.AssertFunction(r.vm.Get("__goosieMakeBlobTextStream"))
	if !ok {
		panic(r.vm.NewTypeError("Blob text stream factory is unavailable"))
	}
	_ = blobProto.Set("textStream", func(call goja.FunctionCall) goja.Value {
		data := r.fileValues[r.asObject(call.This)]
		if data == nil {
			return goja.Null()
		}
		stream, err := textStreamFactory(goja.Undefined(), r.vm.ToValue(decodeBlobText(data.data)))
		if err != nil {
			panic(err)
		}
		return stream
	})
	r.defineInterfaceToStringTag("Blob", blobProto)
	if win, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = win.Set("Blob", blobConstructor)
	}

	_ = r.vm.Set("__goosieFileInit", func(call goja.FunctionCall) goja.Value {
		return r.initializeFile(call.Argument(0), call.Argument(1), call.Argument(2), call.Argument(3), call.Argument(4))
	})
	if _, err := r.vm.RunString(`globalThis.File = (function(init) { return class File extends Blob { constructor(fileBits, fileName, options = {}) { super(); init(this, arguments.length, fileBits, fileName, options); } }; })(__goosieFileInit)`); err != nil {
		panic(err)
	}
	fileConstructor := r.vm.Get("File").ToObject(r.vm)
	fileProto := fileConstructor.Get("prototype").ToObject(r.vm)
	r.fileProto = fileProto
	_ = fileProto.DefineAccessorProperty("name", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if file := r.fileValues[r.asObject(call.This)]; file != nil && file.isFile {
			return r.vm.ToValue(file.name)
		}
		return r.vm.ToValue("")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = fileProto.DefineAccessorProperty("lastModified", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if file := r.fileValues[r.asObject(call.This)]; file != nil && file.isFile {
			return r.vm.ToValue(file.lastModified)
		}
		return r.vm.ToValue(int64(0))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = fileProto.DefineAccessorProperty("webkitRelativePath", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue("")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	r.defineInterfaceToStringTag("File", fileProto)
	if win, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = win.Set("File", fileConstructor)
	}
	r.setupFileReader()
	_, _ = r.vm.RunString(`delete globalThis.__goosieBlobInit; delete globalThis.__goosieFileInit; delete globalThis.__goosieMakeBlobStream`)

}

func (r *Runtime) setupFileReader() {
	proto := r.vm.NewObject()
	if eventTarget, ok := r.vm.Get("EventTarget").(*goja.Object); ok {
		if eventTargetProto, ok := eventTarget.Get("prototype").(*goja.Object); ok {
			_ = proto.SetPrototype(eventTargetProto)
		}
	}
	constructor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		reader := call.This
		state := &fileReaderState{
			reader: reader, error: goja.Null(), handlers: make(map[string]goja.Value),
			listeners: make(map[string][]goja.Value),
		}
		r.fileReaderStates[reader] = state
		return nil
	}).(*goja.Object)
	_ = constructor.Set("prototype", proto)
	_ = proto.Set("constructor", constructor)
	r.defineInterfaceToStringTag("FileReader", proto)
	for name, value := range map[string]int{"EMPTY": 0, "LOADING": 1, "DONE": 2} {
		_ = constructor.Set(name, value)
		_ = proto.Set(name, value)
	}
	_ = proto.DefineAccessorProperty("readyState", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if state := r.fileReaderStates[r.asObject(call.This)]; state != nil {
			return r.vm.ToValue(state.readyState)
		}
		return r.vm.ToValue(0)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("result", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if state := r.fileReaderStates[r.asObject(call.This)]; state != nil {
			if state.readyState == 1 {
				return goja.Null()
			}
			if state.result != nil {
				return state.result
			}
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("error", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if state := r.fileReaderStates[r.asObject(call.This)]; state != nil && state.error != nil {
			return state.error
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	for _, eventName := range []string{"loadstart", "progress", "load", "abort", "error", "loadend"} {
		eventName := eventName
		property := "on" + eventName
		_ = proto.DefineAccessorProperty(property, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if state := r.fileReaderStates[r.asObject(call.This)]; state != nil {
				if handler := state.handlers[eventName]; handler != nil {
					return handler
				}
			}
			return goja.Null()
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if state := r.fileReaderStates[r.asObject(call.This)]; state != nil {
				value := call.Argument(0)
				if value == nil || goja.IsNull(value) || goja.IsUndefined(value) {
					delete(state.handlers, eventName)
				} else if _, ok := goja.AssertFunction(value); ok {
					state.handlers[eventName] = value
				}
			}
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = proto.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		state := r.fileReaderStates[r.asObject(call.This)]
		if state == nil || len(call.Arguments) < 2 || goja.IsNull(call.Argument(1)) || goja.IsUndefined(call.Argument(1)) {
			return goja.Undefined()
		}
		typ, callback := call.Argument(0).String(), call.Argument(1)
		for _, existing := range state.listeners[typ] {
			if existing == callback {
				return goja.Undefined()
			}
		}
		state.listeners[typ] = append(state.listeners[typ], callback)
		return goja.Undefined()
	})
	_ = proto.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		state := r.fileReaderStates[r.asObject(call.This)]
		if state == nil || len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		typ, callback := call.Argument(0).String(), call.Argument(1)
		listeners := state.listeners[typ]
		for i, existing := range listeners {
			if existing == callback {
				state.listeners[typ] = append(listeners[:i], listeners[i+1:]...)
				break
			}
		}
		return goja.Undefined()
	})
	_ = proto.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'dispatchEvent': 1 argument required"))
		}
		state := r.fileReaderStates[r.asObject(call.This)]
		if state == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		r.dispatchFileReaderEvent(state, call.Argument(0).ToObject(r.vm))
		return r.vm.ToValue(!call.Argument(0).ToObject(r.vm).Get("defaultPrevented").ToBoolean())
	})
	_ = proto.Set("readAsArrayBuffer", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.startFileRead(call, "arrayBuffer")
		return goja.Undefined()
	}))
	_ = proto.Set("readAsText", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.startFileRead(call, "text")
		return goja.Undefined()
	}))
	_ = proto.Set("readAsDataURL", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.startFileRead(call, "dataURL")
		return goja.Undefined()
	}))
	_ = proto.Set("readAsBinaryString", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.startFileRead(call, "binaryString")
		return goja.Undefined()
	}))
	_ = proto.Set("abort", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		state := r.fileReaderStates[r.asObject(call.This)]
		if state == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		if state.readyState != 1 {
			return goja.Undefined()
		}
		state.readAborted = true
		state.result = goja.Null()
		state.error = goja.Null()
		state.readyState = 2
		r.dispatchFileReaderProgress(state, "abort", 0, 0)
		r.dispatchFileReaderProgress(state, "loadend", 0, 0)
		return goja.Undefined()
	}))
	_ = r.vm.Set("FileReader", constructor)
	if window, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = window.Set("FileReader", constructor)
	}
}

func (r *Runtime) startFileRead(call goja.FunctionCall, kind string) {
	state := r.fileReaderStates[r.asObject(call.This)]
	if state == nil {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	if state.readyState == 1 {
		panic(r.domError("InvalidStateError", "The FileReader is already loading"))
	}
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Blob argument is required"))
	}
	file := r.fileValues[r.asObject(call.Argument(0))]
	if file == nil {
		panic(r.vm.NewTypeError("The argument is not a Blob"))
	}
	data := bytes.Clone(file.data)
	contentType := file.typeValue
	encoding := ""
	if kind == "text" && len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		encoding = call.Argument(1).String()
	}
	state.readyState = 1
	state.result = nil
	state.error = goja.Null()
	state.readAborted = false
	r.QueueTask(func() {
		if state.readAborted {
			return
		}
		total := len(data)
		r.dispatchFileReaderProgress(state, "loadstart", 0, total)
		var result goja.Value
		switch kind {
		case "arrayBuffer":
			result = r.vm.ToValue(r.vm.NewArrayBuffer(data))
		case "dataURL":
			mime := contentType
			if mime == "" {
				mime = "application/octet-stream"
			}
			result = r.vm.ToValue("data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data))
		case "binaryString":
			var binary strings.Builder
			for _, b := range data {
				binary.WriteRune(rune(b))
			}
			result = r.vm.ToValue(binary.String())
		default:
			result = r.vm.ToValue(decodeBlobTextWithEncoding(data, encoding))
		}
		state.result = result
		state.readyState = 2
		r.dispatchFileReaderProgress(state, "progress", total, total)
		r.dispatchFileReaderProgress(state, "load", total, total)
		r.dispatchFileReaderProgress(state, "loadend", total, total)
	})
}

func decodeBlobTextWithEncoding(data []byte, encoding string) string {
	if encoding == "" || strings.EqualFold(encoding, "utf-8") || strings.EqualFold(encoding, "utf8") {
		return decodeBlobText(data)
	}
	return strings.ToValidUTF8(string(data), "\uFFFD")
}

func (r *Runtime) dispatchFileReaderProgress(state *fileReaderState, typ string, loaded, total int) {
	constructor, ok := goja.AssertConstructor(r.vm.Get("Event"))
	if !ok {
		return
	}
	event, err := constructor(nil, r.vm.ToValue(typ))
	if err != nil {
		return
	}
	_ = event.Set("target", state.reader)
	_ = event.Set("currentTarget", state.reader)
	_ = event.Set("lengthComputable", true)
	_ = event.Set("loaded", loaded)
	_ = event.Set("total", total)
	r.dispatchFileReaderEvent(state, event)
}

func (r *Runtime) dispatchFileReaderEvent(state *fileReaderState, event *goja.Object) {
	typ := event.Get("type").String()
	r.withCurrentEvent(event, func() {
		if handler := state.handlers[typ]; handler != nil {
			if fn, ok := goja.AssertFunction(handler); ok {
				if _, err := fn(state.reader, event); err != nil {
					r.reportFileReaderError(err)
				}
			}
		}
		for _, listener := range append([]goja.Value(nil), state.listeners[typ]...) {
			if fn, ok := goja.AssertFunction(listener); ok {
				if _, err := fn(state.reader, event); err != nil {
					r.reportFileReaderError(err)
				}
			} else if object, ok := listener.(*goja.Object); ok {
				if fn, ok := goja.AssertFunction(object.Get("handleEvent")); ok {
					if _, err := fn(object, event); err != nil {
						r.reportFileReaderError(err)
					}
				}
			}
		}
	})
}

func (r *Runtime) reportFileReaderError(err error) {
	if err == nil {
		return
	}
	console, ok := r.vm.Get("console").(*goja.Object)
	if !ok {
		return
	}
	if logger, ok := goja.AssertFunction(console.Get("error")); ok {
		_, _ = logger(console, r.vm.ToValue(err.Error()))
	}
}

func (r *Runtime) newUint8Array(data []byte) *goja.Object {
	constructor, ok := goja.AssertConstructor(r.vm.Get("Uint8Array"))
	if !ok {
		panic(r.vm.NewTypeError("Uint8Array is unavailable"))
	}
	value, err := constructor(nil, r.vm.ToValue(len(data)))
	if err != nil {
		panic(err)
	}
	for i, b := range data {
		if err := value.Set(newIndex(int64(i)), int(b)); err != nil {
			panic(err)
		}
	}
	return value
}

func decodeBlobText(data []byte) string {
	var decoded strings.Builder
	decoded.Grow(len(data))
	for len(data) > 0 {
		runeValue, size := utf8.DecodeRune(data)
		if runeValue == utf8.RuneError && size == 1 {
			decoded.WriteRune(utf8.RuneError)
		} else {
			decoded.WriteRune(runeValue)
		}
		data = data[size:]
	}
	text := decoded.String()
	return strings.TrimPrefix(text, "\uFEFF")
}

func (r *Runtime) asObject(value goja.Value) *goja.Object {
	obj, _ := value.(*goja.Object)
	return obj
}

func (r *Runtime) initializeBlob(receiver, partsValue, optionsValue goja.Value) goja.Value {
	receiverObject, ok := receiver.(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("Illegal Blob receiver"))
	}
	parts, err := r.readBlobParts(partsValue)
	if err != nil {
		r.panicFileAPIError(err)
	}
	endings, mime := "transparent", ""
	options, err := r.filePropertyBag(optionsValue)
	if err != nil {
		r.panicFileAPIError(err)
	}
	if options != nil {
		if value := options.Get("endings"); value != nil && !goja.IsUndefined(value) {
			endings = value.String()
		}
		if endings != "transparent" && endings != "native" {
			panic(r.vm.NewTypeError("Blob/File endings must be 'transparent' or 'native'"))
		}
		if value := options.Get("type"); value != nil && !goja.IsUndefined(value) {
			mime = normalizeMIMEType(value.String())
		}
	}
	data, err := r.joinBlobParts(parts, endings)
	if err != nil {
		r.panicFileAPIError(err)
	}
	r.makeBlob(receiverObject, data, mime, false, "", 0)
	return goja.Undefined()
}

func (r *Runtime) initializeFile(receiver, argumentCountValue, bits, fileName, optionsValue goja.Value) goja.Value {
	receiverObject, ok := receiver.(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("Illegal File receiver"))
	}
	if argumentCountValue.ToInteger() < 2 {
		panic(r.vm.NewTypeError("File constructor requires fileBits and fileName"))
	}
	parts, err := r.readBlobParts(bits)
	if err != nil {
		r.panicFileAPIError(err)
	}
	name, err := r.filePartString(fileName)
	if err != nil {
		panic(err)
	}
	mime := ""
	lastModified := time.Now().UnixMilli()
	endings := "transparent"
	options, err := r.filePropertyBag(optionsValue)
	if err != nil {
		r.panicFileAPIError(err)
	}
	if options != nil {
		if value := options.Get("endings"); value != nil && !goja.IsUndefined(value) {
			endings = value.String()
		}
		if endings != "transparent" && endings != "native" {
			panic(r.vm.NewTypeError("Blob/File endings must be 'transparent' or 'native'"))
		}
		if value := options.Get("lastModified"); value != nil && !goja.IsUndefined(value) {
			lastModified = webIDLToLongLong(value)
		}
		if value := options.Get("type"); value != nil && !goja.IsUndefined(value) {
			mime = normalizeMIMEType(value.String())
		}
	}
	data, err := r.joinBlobParts(parts, endings)
	if err != nil {
		r.panicFileAPIError(err)
	}
	r.makeBlob(receiverObject, data, mime, true, name, lastModified)
	return goja.Undefined()
}

func (r *Runtime) readBlobParts(value goja.Value) ([]blobPart, error) {
	if value == nil || goja.IsUndefined(value) {
		return nil, nil
	}
	if goja.IsNull(value) {
		return nil, r.newFileAPITypeError("Blob/File parts must be a sequence")
	}
	obj, ok := value.(*goja.Object)
	if !ok {
		return nil, r.newFileAPITypeError("Blob/File parts must be an iterable object")
	}
	iteratorMethod := obj.GetSymbol(goja.SymIterator)
	iteratorFn, ok := goja.AssertFunction(iteratorMethod)
	if !ok {
		return nil, r.newFileAPITypeError("Blob/File parts must be iterable")
	}
	iteratorValue, err := iteratorFn(obj)
	if err != nil {
		return nil, err
	}
	iterator, ok := iteratorValue.(*goja.Object)
	if !ok {
		return nil, r.newFileAPITypeError("Blob/File iterator must return an object")
	}
	nextFn, ok := goja.AssertFunction(iterator.Get("next"))
	if !ok {
		return nil, r.newFileAPITypeError("Blob/File iterator.next must be callable")
	}
	parts := make([]blobPart, 0)
	byteCount := 0
	for count := 0; ; count++ {
		if count >= 1<<20 {
			return nil, errors.New("File/Blob parts exceed the admitted part count")
		}
		stepValue, err := nextFn(iterator)
		if err != nil {
			return nil, err
		}
		step, ok := stepValue.(*goja.Object)
		if !ok {
			return nil, r.newFileAPITypeError("Blob/File iterator result must be an object")
		}
		if step.Get("done").ToBoolean() {
			break
		}
		part := step.Get("value")
		converted := blobPart{}
		if partObject, ok := part.(*goja.Object); ok {
			if file := r.fileValues[partObject]; file != nil {
				converted.data = bytes.Clone(file.data)
			} else if r.isBufferSource(partObject) {
				if !r.bufferSourceDetached(partObject) {
					if err := r.vm.ExportTo(partObject, &converted.data); err != nil {
						return nil, err
					}
					converted.data = bytes.Clone(converted.data)
				}
			} else {
				converted.text, err = r.filePartString(part)
				if err != nil {
					return nil, err
				}
				converted.isText = true
			}
		} else {
			converted.text, err = r.filePartString(part)
			if err != nil {
				return nil, err
			}
			converted.isText = true
		}
		size := len(converted.data)
		if converted.isText {
			size = len(converted.text)
		}
		if size > maxFileBytes-byteCount {
			return nil, errors.New("File/Blob data exceeds the admitted byte limit")
		}
		byteCount += size
		parts = append(parts, converted)
	}
	return parts, nil
}

func (r *Runtime) joinBlobParts(parts []blobPart, endings string) ([]byte, error) {
	if endings != "transparent" && endings != "native" {
		return nil, r.newFileAPITypeError("Blob/File endings must be 'transparent' or 'native'")
	}
	data := make([]byte, 0)
	for _, part := range parts {
		bytesPart := part.data
		if part.isText {
			text := part.text
			if endings == "native" {
				text = convertNativeLineEndings(text)
			}
			bytesPart = []byte(text)
		}
		if len(bytesPart) > maxFileBytes-len(data) {
			return nil, errors.New("File/Blob data exceeds the admitted byte limit")
		}
		data = append(data, bytesPart...)
	}
	return data, nil
}

func (r *Runtime) filePartString(value goja.Value) (string, error) {
	object, ok := value.(*goja.Object)
	if !ok {
		if _, isSymbol := value.(*goja.Symbol); isSymbol {
			return "", r.newFileAPITypeError("Cannot convert a Symbol value to a string")
		}
		return value.String(), nil
	}
	if r.getNode(object) != nil {
		customToString := false
		for _, name := range object.GetOwnPropertyNames() {
			if name == "toString" {
				customToString = true
				break
			}
		}
		if !customToString {
			if tag := object.GetSymbol(goja.SymToStringTag); tag != nil && !goja.IsUndefined(tag) {
				return "[object " + tag.String() + "]", nil
			}
		}
	}
	for _, methodName := range []string{"toString", "valueOf"} {
		method := object.Get(methodName)
		callable, ok := goja.AssertFunction(method)
		if !ok {
			continue
		}
		result, err := callable(object)
		if err != nil {
			return "", err
		}
		if _, isObject := result.(*goja.Object); !isObject {
			if _, isSymbol := result.(*goja.Symbol); isSymbol {
				continue
			}
			return result.String(), nil
		}
	}
	return "", r.newFileAPITypeError("Cannot convert object to a primitive value")
}

func (r *Runtime) isBufferSource(obj *goja.Object) bool {
	if obj == nil {
		return false
	}
	names := []string{"ArrayBuffer", "DataView", "Int8Array", "Uint8Array", "Uint8ClampedArray", "Int16Array", "Uint16Array", "Int32Array", "Uint32Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array"}
	for proto := obj.Prototype(); proto != nil; proto = proto.Prototype() {
		for _, name := range names {
			constructor := r.vm.Get(name)
			if constructor == nil || goja.IsUndefined(constructor) {
				continue
			}
			prototype := constructor.ToObject(r.vm).Get("prototype")
			if object, ok := prototype.(*goja.Object); ok && object == proto {
				return true
			}
		}
	}
	return false
}

func (r *Runtime) bufferSourceDetached(obj *goja.Object) bool {
	if obj == nil {
		return false
	}
	buffer := obj
	if !isArrayBufferObject(obj) {
		var ok bool
		buffer, ok = obj.Get("buffer").(*goja.Object)
		if !ok {
			return false
		}
	}
	value, ok := buffer.Export().(goja.ArrayBuffer)
	return ok && value.Detached()
}

func convertNativeLineEndings(value string) string {
	ending := "\n"
	if runtime.GOOS == "windows" {
		ending = "\r\n"
	}
	var result strings.Builder
	result.Grow(len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == '\r' {
			result.WriteString(ending)
			if i+1 < len(value) && value[i+1] == '\n' {
				i++
			}
		} else if value[i] == '\n' {
			result.WriteString(ending)
		} else {
			result.WriteByte(value[i])
		}
	}
	return result.String()
}

func newIndex(index int64) string {
	if index == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for index > 0 {
		i--
		buf[i] = byte('0' + index%10)
		index /= 10
	}
	return string(buf[i:])
}

func (r *Runtime) newBlob(data []byte, mime string) *goja.Object {
	return r.makeBlob(r.vm.NewObject(), data, mime, false, "", 0)
}

func (r *Runtime) makeBlob(obj *goja.Object, data []byte, mime string, isFile bool, name string, lastModified int64) *goja.Object {
	if len(data) > maxFileBytes {
		panic(r.newRangeError("File/Blob data exceeds the admitted byte limit"))
	}
	if obj == nil {
		obj = r.vm.NewObject()
	}
	proto := r.blobProto
	if isFile {
		proto = r.fileProto
	}
	if proto == nil {
		panic(r.vm.NewTypeError("Blob/File interface is unavailable"))
	}
	prototypePresent := false
	for current := obj.Prototype(); current != nil; current = current.Prototype() {
		if current == proto {
			prototypePresent = true
			break
		}
	}
	if !prototypePresent {
		_ = obj.SetPrototype(proto)
	}
	r.fileValues[obj] = &browserFile{data: bytes.Clone(data), name: name, typeValue: mime, lastModified: lastModified, isFile: isFile}
	return obj
}

func normalizeMIMEType(value string) string {
	for _, ch := range value {
		if ch < 0x20 || ch > 0x7e {
			return ""
		}
	}
	return strings.ToLower(value)
}

func webIDLToLongLong(value goja.Value) int64 {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return 0
	}
	number := value.ToFloat()
	if math.IsNaN(number) || number == 0 || math.IsInf(number, 0) {
		return 0
	}
	rounded := math.RoundToEven(number)
	if rounded >= math.MaxInt64 {
		return math.MaxInt64
	}
	if rounded <= math.MinInt64 {
		return math.MinInt64
	}
	return int64(rounded)
}

func sanitizeFileName(name string) string {
	return strings.NewReplacer("/", ":", "\\", ":").Replace(name)
}

func (r *Runtime) getFileList(value goja.Value) []*goja.Object {
	obj, _ := value.(*goja.Object)
	if obj == nil {
		return nil
	}
	return r.fileListValues[obj]
}

func (r *Runtime) updateDataTransferFiles(obj *goja.Object, dt *dom.DataTransfer, files []*goja.Object) {
	if dt != nil {
		r.ensureDataTransferItemOrder(dt)
	}
	oldFiles := r.dataTransferFiles[obj]
	oldIDs := r.dataTransferFileIDs[obj]
	available := make(map[*goja.Object][]uint64)
	for i, file := range oldFiles {
		if i < len(oldIDs) && oldIDs[i] != 0 {
			available[file] = append(available[file], oldIDs[i])
		}
	}
	ids := make([]uint64, len(files))
	for i, file := range files {
		if queue := available[file]; len(queue) > 0 {
			ids[i] = queue[0]
			available[file] = queue[1:]
			continue
		}
		r.nextDataTransferItemID++
		if r.nextDataTransferItemID == 0 {
			r.nextDataTransferItemID++
		}
		ids[i] = r.nextDataTransferItemID
	}
	r.dataTransferFiles[obj] = append([]*goja.Object(nil), files...)
	r.dataTransferFileIDs[obj] = ids
	if dt != nil {
		r.dataTransferFilesByStore[dt.StoreIdentity()] = append([]*goja.Object(nil), files...)
		r.dataTransferFileIDsByStore[dt.StoreIdentity()] = append([]uint64(nil), ids...)
		r.syncDataTransferFileOrder(dt, files, ids)
		r.refreshDataTransferItemIndicesForStore(dt.StoreIdentity())
	}
}

func (r *Runtime) setFileListFiles(list *goja.Object, files []*goja.Object) {
	if list == nil {
		return
	}
	for i := range r.fileListValues[list] {
		list.Delete(newIndex(int64(i)))
	}
	r.fileListValues[list] = append([]*goja.Object(nil), files...)
	_ = list.Set("length", len(files))
	for i, file := range files {
		_ = list.Set(newIndex(int64(i)), file)
	}
	for dt, obj := range r.dtWrappers {
		if r.dataTransferFileLists[obj] == list && (dt.Mode() == dom.DataTransferReadWrite || dt.Mode() == dom.DataTransferReadOnly) {
			r.updateDataTransferFiles(obj, dt, files)
		}
	}
	for node, inputList := range r.inputFileLists {
		if inputList == list {
			r.inputFiles[node] = append([]*goja.Object(nil), files...)
		}
	}
}

func (r *Runtime) clearInputFiles(node *dom.Node) {
	if node == nil {
		return
	}
	list := r.inputFileLists[node]
	if list == nil {
		list = r.newFileList(nil)
		r.inputFileLists[node] = list
	}
	r.setFileListFiles(list, nil)
	r.inputFiles[node] = nil
}

func (r *Runtime) newFileList(files []*goja.Object) *goja.Object {
	list := r.vm.NewObject()
	_ = list.SetPrototype(r.fileListProto)
	r.fileListValues[list] = append([]*goja.Object(nil), files...)
	_ = list.Set("length", len(files))
	for i, file := range files {
		_ = list.Set(newIndex(int64(i)), file)
	}
	_ = list.Set("item", func(call goja.FunctionCall) goja.Value {
		index := -1
		if len(call.Arguments) > 0 {
			index = int(call.Arguments[0].ToInteger())
		}
		items := r.fileListValues[list]
		if index < 0 || index >= len(items) {
			return goja.Null()
		}
		return items[index]
	})
	_ = list.SetSymbol(goja.SymIterator, func(goja.FunctionCall) goja.Value {
		index := 0
		iterator := r.vm.NewObject()
		_ = iterator.Set("next", func(goja.FunctionCall) goja.Value {
			result := r.vm.NewObject()
			if index >= len(r.fileListValues[list]) {
				_ = result.Set("done", true)
				_ = result.Set("value", goja.Undefined())
				return result
			}
			file := r.fileListValues[list][index]
			index++
			_ = result.Set("done", false)
			_ = result.Set("value", file)
			return result
		})
		_ = iterator.SetSymbol(goja.SymIterator, func(goja.FunctionCall) goja.Value { return iterator })
		return iterator
	})
	return list
}

func timeNowMillis() int64 { return time.Now().UnixMilli() }

func (r *Runtime) newRangeError(message string) *goja.Object {
	if ctor, ok := goja.AssertConstructor(r.vm.Get("RangeError")); ok {
		if value, err := ctor(nil, r.vm.ToValue(message)); err == nil {
			return value
		}
	}
	return r.vm.NewTypeError(message)
}

func (r *Runtime) installFileInputAccessors() {
	if r.tagProtos == nil {
		return
	}
	proto := r.tagProtos["input"]
	if proto == nil {
		return
	}
	_ = proto.DefineAccessorProperty("files", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Data != "input" || strings.ToLower(node.GetAttribute("type")) != "file" {
			return goja.Null()
		}
		if list := r.inputFileLists[node]; list != nil {
			return list
		}
		list := r.newFileList(r.inputFiles[node])
		r.inputFileLists[node] = list
		return list
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || strings.ToLower(node.GetAttribute("type")) != "file" {
			return goja.Undefined()
		}
		if len(call.Arguments) == 0 || goja.IsNull(call.Argument(0)) {
			return goja.Undefined()
		}
		list, ok := call.Argument(0).(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("Failed to set the 'files' property on 'HTMLInputElement': The provided value is not of type 'FileList'."))
		}
		files, ok := r.fileListValues[list]
		if !ok {
			panic(r.vm.NewTypeError("Failed to set the 'files' property on 'HTMLInputElement': The provided value is not of type 'FileList'."))
		}
		r.inputFiles[node] = append([]*goja.Object(nil), files...)
		r.inputFileLists[node] = list
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) installDataTransferFiles(obj *goja.Object, dt *dom.DataTransfer) {
	if obj == nil {
		return
	}
	r.dataTransferFiles[obj] = append([]*goja.Object(nil), r.dataTransferFilesByStore[dt.StoreIdentity()]...)
	r.dataTransferFileIDs[obj] = append([]uint64(nil), r.dataTransferFileIDsByStore[dt.StoreIdentity()]...)
	items := r.vm.NewObject()
	if r.dataTransferItemListProto != nil {
		_ = items.SetPrototype(r.dataTransferItemListProto)
	}
	r.dataTransferItemLists[items] = dataTransferItemListBinding{dt: dt, transfer: obj}
	_ = obj.Set("items", items)
	_ = obj.DefineAccessorProperty("files", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		mode := dt.Mode()
		if list := r.dataTransferFileLists[obj]; list != nil {
			previousMode := r.dataTransferFileModes[obj]
			if mode == dom.DataTransferProtected || mode == dom.DataTransferDisabled {
				if previousMode != mode {
					r.setFileListFiles(list, nil)
				}
			} else if previousMode == dom.DataTransferProtected {
				r.setFileListFiles(list, r.dataTransferFiles[obj])
			}
			r.dataTransferFileModes[obj] = mode
			return list
		}
		files := r.dataTransferFiles[obj]
		if mode == dom.DataTransferProtected || mode == dom.DataTransferDisabled {
			files = nil
		}
		list := r.newFileList(files)
		r.dataTransferFileLists[obj] = list
		r.dataTransferFileModes[obj] = mode
		return list
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}
