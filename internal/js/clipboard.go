package js

import (
	"bytes"
	"errors"
	"mime"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/dop251/goja"
)

// ClipboardProvider is the interface a platform clipboard must satisfy. The
// browser host installs one through js.Options or SetClipboardProvider;
// without it, the runtime keeps an in-memory clipboard for headless use.
type ClipboardProvider interface {
	Read() string
	Write(string)
}

// RichClipboardProvider optionally transports the text/html clipboard flavor.
// Providers that implement only ClipboardProvider still support text/plain.
type RichClipboardProvider interface {
	ClipboardProvider
	ReadHTML() string
	WriteRich(text, html string)
}

type clipboardRepresentation struct {
	data      []byte
	typeValue string
	promise   *goja.Object
}

type clipboardItemData struct {
	representations map[string]clipboardRepresentation
	types           []string
}

// memoryClipboard is the in-memory fallback used when no platform clipboard
// is available (headless, tests). It is per-runtime, so two tabs cannot
// read each other's clipboard through this path.
type memoryClipboard struct {
	mu   sync.Mutex
	text string
	html string
}

func (m *memoryClipboard) Read() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.text
}

func (m *memoryClipboard) Write(s string) {
	m.WriteRich(s, "")
}

func (m *memoryClipboard) ReadHTML() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.html
}

func (m *memoryClipboard) WriteRich(text, html string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.text, m.html = text, html
}

// SetClipboardProvider installs a platform clipboard on the runtime.
// Passing nil reverts to the in-memory fallback.
func (r *Runtime) SetClipboardProvider(cp ClipboardProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cp != nil {
		r.clipboardProvider = cp
		r.hasPlatformClipboard = true
	} else {
		r.clipboardProvider = &memoryClipboard{}
		r.hasPlatformClipboard = false
	}
}

// setupClipboard installs navigator.clipboard with writeText and readText.
// Both methods return Promises. Without a platform clipboard the runtime
// uses an in-memory fallback so the API always resolves.
func (r *Runtime) setupClipboard() {
	// Default to the in-memory clipboard when the host has no platform provider.
	r.clipboardProvider = &memoryClipboard{}
	if r.opts.ClipboardProvider != nil {
		r.clipboardProvider = r.opts.ClipboardProvider
		r.hasPlatformClipboard = true
	}
	r.setupClipboardItem()

	navigator := r.vm.Get("navigator")
	if navigator == nil || goja.IsUndefined(navigator) {
		navigatorObj := r.vm.NewObject()
		_ = r.vm.Set("navigator", navigatorObj)
		navigator = navigatorObj
	}
	navigatorObj, ok := navigator.(*goja.Object)
	if !ok {
		return
	}

	clipboard := r.vm.NewObject()

	_ = clipboard.Set("writeText", func(call goja.FunctionCall) goja.Value {
		text := ""
		if len(call.Arguments) >= 1 {
			text = call.Arguments[0].String()
		}
		p := r.newPromiseObj()
		if !r.clipboardAccessAllowed("clipboard-write") {
			r.rejectPromise(p, namedError(r, "NotAllowedError", "Clipboard access requires a secure context and transient user activation"))
			return p.obj
		}
		r.mu.Lock()
		cp := r.clipboardProvider
		r.mu.Unlock()
		cp.Write(text)
		r.resolvePromise(p, goja.Undefined())
		return p.obj
	})

	_ = clipboard.Set("readText", func(call goja.FunctionCall) goja.Value {
		p := r.newPromiseObj()
		if !r.clipboardAccessAllowed("clipboard-read") {
			r.rejectPromise(p, namedError(r, "NotAllowedError", "Clipboard access requires a secure context and transient user activation"))
			return p.obj
		}
		r.mu.Lock()
		cp := r.clipboardProvider
		r.mu.Unlock()
		text := cp.Read()
		r.resolvePromise(p, r.vm.ToValue(text))
		return p.obj
	})

	_ = clipboard.Set("write", func(call goja.FunctionCall) goja.Value {
		p := r.newPromiseObj()
		if !r.clipboardAccessAllowed("clipboard-write") {
			r.rejectPromise(p, namedError(r, "NotAllowedError", "Clipboard access requires a secure context and transient user activation"))
			return p.obj
		}
		items, err := r.readClipboardItemSequence(call.Argument(0))
		if err != nil && !goja.IsNull(err) && !goja.IsUndefined(err) {
			r.rejectPromise(p, err)
			return p.obj
		}
		if len(items) != 1 {
			r.rejectPromise(p, namedError(r, "NotSupportedError", "The host clipboard supports one clipboard item at a time"))
			return p.obj
		}
		r.mu.Lock()
		cp := r.clipboardProvider
		r.mu.Unlock()
		r.writeClipboardItem(items[0], cp, p)
		return p.obj
	})

	_ = clipboard.Set("read", func(call goja.FunctionCall) goja.Value {
		p := r.newPromiseObj()
		if !r.clipboardAccessAllowed("clipboard-read") {
			r.rejectPromise(p, namedError(r, "NotAllowedError", "Clipboard access requires a secure context and transient user activation"))
			return p.obj
		}
		r.mu.Lock()
		cp := r.clipboardProvider
		r.mu.Unlock()
		plain := cp.Read()
		formats := map[string]clipboardRepresentation{
			"text/plain": {data: []byte(plain), typeValue: "text/plain"},
		}
		if rich, ok := cp.(interface{ ReadHTML() string }); ok {
			if html := rich.ReadHTML(); html != "" {
				formats["text/html"] = clipboardRepresentation{data: []byte(html), typeValue: "text/html"}
			}
		}
		item := r.newClipboardItem(formats)
		r.resolvePromise(p, r.vm.ToValue(r.vm.NewArray(item)))
		return p.obj
	})

	_ = navigatorObj.Set("clipboard", clipboard)
	permissions := r.vm.NewObject()
	_ = permissions.Set("query", func(call goja.FunctionCall) goja.Value {
		p := r.newPromiseObj()
		if len(call.Arguments) == 0 {
			r.rejectPromise(p, r.vm.NewTypeError("Permission descriptor is required"))
			return p.obj
		}
		descriptor, ok := call.Argument(0).(*goja.Object)
		if !ok || descriptor == nil {
			r.rejectPromise(p, r.vm.NewTypeError("Permission descriptor must be an object"))
			return p.obj
		}
		name := descriptor.Get("name").String()
		state := r.clipboardPermissionState(name)
		if state == "" {
			r.rejectPromise(p, namedError(r, "NotSupportedError", "Permission name is not supported"))
			return p.obj
		}
		status := r.vm.NewObject()
		_ = status.DefineAccessorProperty("name", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(name)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
		_ = status.DefineAccessorProperty("state", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(r.clipboardPermissionState(name))
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
		_ = status.Set("onchange", goja.Null())
		r.resolvePromise(p, status)
		return p.obj
	})
	_ = navigatorObj.Set("permissions", permissions)
}

func (r *Runtime) setupClipboardItem() {
	if r.clipboardItems == nil {
		r.clipboardItems = make(map[*goja.Object]*clipboardItemData)
	}
	proto := r.vm.NewObject()
	ctor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("ClipboardItem data is required"))
		}
		input, ok := call.Argument(0).(*goja.Object)
		if !ok || input == nil {
			panic(r.vm.NewTypeError("ClipboardItem data must be a record"))
		}
		representations := make(map[string]clipboardRepresentation)
		types := make([]string, 0, len(input.Keys()))
		for _, key := range input.Keys() {
			typeName, _, err := mime.ParseMediaType(key)
			if err != nil {
				panic(r.vm.NewTypeError("ClipboardItem type must be a valid MIME type"))
			}
			mimeType := strings.ToLower(typeName)
			value := input.Get(key)
			object, ok := value.(*goja.Object)
			if !ok || object == nil {
				panic(r.vm.NewTypeError("ClipboardItem representations must be Blob objects or Promises"))
			}
			blob := r.fileValues[object]
			representation := clipboardRepresentation{typeValue: mimeType}
			if blob != nil {
				if len(blob.data) > 1<<20 {
					panic(r.newRangeError("ClipboardItem representation exceeds the clipboard size limit"))
				}
				representation.data = bytes.Clone(blob.data)
			} else if getInternalPromise(object) != nil {
				representation.promise = object
			} else {
				panic(r.vm.NewTypeError("ClipboardItem representations must be Blob objects or Promises"))
			}
			if _, exists := representations[mimeType]; !exists {
				types = append(types, mimeType)
			}
			representations[mimeType] = representation
		}
		if len(representations) == 0 || len(representations) > 16 {
			panic(r.vm.NewTypeError("ClipboardItem must contain between 1 and 16 representations"))
		}
		obj := call.This
		if obj == nil {
			obj = r.vm.NewObject()
		}
		_ = obj.SetPrototype(proto)
		r.clipboardItems[obj] = &clipboardItemData{representations: representations, types: types}
		return obj
	})
	_ = proto.Set("constructor", ctor)
	_ = proto.DefineAccessorProperty("types", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		data := r.clipboardItems[r.asObject(call.This)]
		if data == nil {
			panic(r.vm.NewTypeError("Illegal ClipboardItem receiver"))
		}
		values := make([]interface{}, len(data.types))
		for i, typ := range data.types {
			values[i] = typ
		}
		return r.vm.ToValue(r.vm.NewArray(values...))
	}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	_ = proto.Set("getType", func(call goja.FunctionCall) goja.Value {
		data := r.clipboardItems[r.asObject(call.This)]
		if data == nil {
			panic(r.vm.NewTypeError("Illegal ClipboardItem receiver"))
		}
		mimeType, _, err := mime.ParseMediaType(call.Argument(0).String())
		if err != nil {
			panic(r.vm.NewTypeError("ClipboardItem.getType requires a valid MIME type"))
		}
		representation, ok := data.representations[strings.ToLower(mimeType)]
		p := r.newPromiseObj()
		if !ok {
			r.rejectPromise(p, namedError(r, "NotFoundError", "ClipboardItem does not contain the requested type"))
		} else if representation.promise != nil {
			r.forwardClipboardBlobPromise(p, representation.promise, representation.typeValue)
		} else {
			r.resolvePromise(p, r.newBlob(representation.data, representation.typeValue))
		}
		return p.obj
	})
	_ = proto.DefineAccessorProperty("presentationStyle", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if r.clipboardItems[r.asObject(call.This)] == nil {
			panic(r.vm.NewTypeError("Illegal ClipboardItem receiver"))
		}
		return r.vm.ToValue("unspecified")
	}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_TRUE)
	r.defineInterfaceToStringTag("ClipboardItem", proto)
	_ = r.vm.Set("ClipboardItem", ctor)
	if constructor, ok := r.vm.Get("ClipboardItem").(*goja.Object); ok {
		_ = constructor.Set("prototype", proto)
		_ = constructor.Set("supports", func(call goja.FunctionCall) goja.Value {
			mimeType, _, err := mime.ParseMediaType(call.Argument(0).String())
			if err != nil {
				return r.vm.ToValue(false)
			}
			switch strings.ToLower(mimeType) {
			case "text/plain", "text/html":
				return r.vm.ToValue(true)
			default:
				return r.vm.ToValue(false)
			}
		})
	}
	if win := r.vm.Get("window"); win != nil && !goja.IsUndefined(win) {
		_ = win.ToObject(r.vm).Set("ClipboardItem", r.vm.Get("ClipboardItem"))
	}
}

func newClipboardItemData(representations map[string]clipboardRepresentation) *clipboardItemData {
	data := &clipboardItemData{representations: representations, types: make([]string, 0, len(representations))}
	for _, typ := range []string{"text/plain", "text/html"} {
		if _, ok := representations[typ]; ok {
			data.types = append(data.types, typ)
		}
	}
	return data
}

func (r *Runtime) newClipboardItem(representations map[string]clipboardRepresentation) *goja.Object {
	obj := r.vm.NewObject()
	if constructor, ok := r.vm.Get("ClipboardItem").(*goja.Object); ok {
		if prototype, ok := constructor.Get("prototype").(*goja.Object); ok {
			_ = obj.SetPrototype(prototype)
		}
	}
	if r.clipboardItems == nil {
		r.clipboardItems = make(map[*goja.Object]*clipboardItemData)
	}
	r.clipboardItems[obj] = newClipboardItemData(representations)
	return obj
}

func (r *Runtime) readClipboardItemSequence(value goja.Value) ([]*clipboardItemData, goja.Value) {
	array, ok := value.(*goja.Object)
	if !ok || array.ClassName() != "Array" {
		return nil, r.vm.NewTypeError("Clipboard.write requires a sequence of ClipboardItem objects")
	}
	length := int(array.Get("length").ToInteger())
	if length < 0 || length > 16 {
		return nil, namedError(r, "DataError", "Clipboard.write accepts at most 16 items")
	}
	items := make([]*clipboardItemData, 0, length)
	for i := 0; i < length; i++ {
		item, ok := array.Get(strconv.Itoa(i)).(*goja.Object)
		if !ok || item == nil || r.clipboardItems[item] == nil {
			return nil, r.vm.NewTypeError("Clipboard.write sequence contains a non-ClipboardItem")
		}
		items = append(items, r.clipboardItems[item])
	}
	return items, goja.Null()
}

func clipboardTextFormats(item *clipboardItemData) (plain, html string, hasPlain, hasHTML bool, err error) {
	for typ, representation := range item.representations {
		switch typ {
		case "text/plain":
			plain, hasPlain = string(representation.data), true
		case "text/html":
			html, hasHTML = string(representation.data), true
		default:
			return "", "", false, false, errors.New("Clipboard.write supports only text/plain and text/html")
		}
	}
	return plain, html, hasPlain, hasHTML, nil
}

func (r *Runtime) writeClipboardItem(item *clipboardItemData, cp ClipboardProvider, result *promiseObj) {
	resolved := make(map[string]clipboardRepresentation, len(item.representations))
	pending := 0
	failed := false
	for typ, representation := range item.representations {
		if typ != "text/plain" && typ != "text/html" {
			r.rejectPromise(result, namedError(r, "NotSupportedError", "The host clipboard supports only text/plain and text/html"))
			return
		}
		if representation.promise != nil {
			pending++
		} else {
			resolved[typ] = representation
		}
	}
	if len(resolved)+pending == 0 {
		r.rejectPromise(result, namedError(r, "NotSupportedError", "ClipboardItem has no supported representation"))
		return
	}
	finish := func() {
		plain, html, hasPlain, hasHTML, err := clipboardTextFormats(&clipboardItemData{representations: resolved})
		if err != nil || (!hasPlain && !hasHTML) {
			r.rejectPromise(result, namedError(r, "NotSupportedError", "The host clipboard supports text/plain and text/html"))
			return
		}
		if rich, ok := cp.(RichClipboardProvider); ok {
			rich.WriteRich(plain, html)
		} else if hasPlain {
			cp.Write(plain)
		} else {
			r.rejectPromise(result, namedError(r, "NotSupportedError", "The host clipboard does not support text/html"))
			return
		}
		r.resolvePromise(result, goja.Undefined())
	}
	if pending == 0 {
		finish()
		return
	}
	for typ, representation := range item.representations {
		if representation.promise == nil {
			continue
		}
		then, ok := goja.AssertFunction(representation.promise.Get("then"))
		if !ok {
			failed = true
			r.rejectPromise(result, r.vm.NewTypeError("ClipboardItem representation Promise is not thenable"))
			return
		}
		onFulfilled := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if failed {
				return goja.Undefined()
			}
			blobObject, ok := call.Argument(0).(*goja.Object)
			blob := r.fileValues[blobObject]
			if !ok || blob == nil {
				failed = true
				r.rejectPromise(result, r.vm.NewTypeError("ClipboardItem Promise must resolve to a Blob"))
				return goja.Undefined()
			}
			if len(blob.data) > 1<<20 {
				failed = true
				r.rejectPromise(result, r.newRangeError("ClipboardItem representation exceeds the clipboard size limit"))
				return goja.Undefined()
			}
			resolved[typ] = clipboardRepresentation{data: bytes.Clone(blob.data), typeValue: typ}
			pending--
			if pending == 0 {
				finish()
			}
			return goja.Undefined()
		})
		onRejected := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if !failed {
				failed = true
				r.rejectPromise(result, call.Argument(0))
			}
			return goja.Undefined()
		})
		if _, err := then(representation.promise, onFulfilled, onRejected); err != nil {
			failed = true
			r.rejectPromise(result, r.vm.ToValue(err.Error()))
			return
		}
	}
}

func (r *Runtime) forwardClipboardBlobPromise(result *promiseObj, source *goja.Object, mimeType string) {
	then, ok := goja.AssertFunction(source.Get("then"))
	if !ok {
		r.rejectPromise(result, r.vm.NewTypeError("ClipboardItem representation Promise is not thenable"))
		return
	}
	onFulfilled := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		blobObject, ok := call.Argument(0).(*goja.Object)
		blob := r.fileValues[blobObject]
		if !ok || blob == nil {
			r.rejectPromise(result, r.vm.NewTypeError("ClipboardItem Promise must resolve to a Blob"))
			return goja.Undefined()
		}
		r.resolvePromise(result, r.newBlob(blob.data, mimeType))
		return goja.Undefined()
	})
	onRejected := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.rejectPromise(result, call.Argument(0))
		return goja.Undefined()
	})
	if _, err := then(source, onFulfilled, onRejected); err != nil {
		r.rejectPromise(result, r.vm.ToValue(err.Error()))
	}
}

func (r *Runtime) clipboardAccessAllowed(permissionName string) bool {
	if r == nil {
		return true
	}
	r.mu.Lock()
	hasPlatform := r.hasPlatformClipboard
	r.mu.Unlock()
	if !hasPlatform {
		return r.clipboardPermissionState(permissionName) != "denied"
	}
	if !clipboardSecureContext(r.opts.URL) || !r.HasTransientUserActivation() {
		return false
	}
	return r.clipboardPermissionState(permissionName) != "denied"
}

func (r *Runtime) clipboardPermissionState(name string) string {
	if name != "clipboard-read" && name != "clipboard-write" {
		return ""
	}
	if r == nil {
		return "prompt"
	}
	r.mu.Lock()
	hasPlatform := r.hasPlatformClipboard
	r.mu.Unlock()
	if r.opts.ClipboardPermissionState == nil {
		if !hasPlatform {
			return "granted"
		}
		return "prompt"
	}
	switch state := r.opts.ClipboardPermissionState(r.opts.URL, name); state {
	case "granted", "denied", "prompt":
		return state
	default:
		return "prompt"
	}
}

func clipboardSecureContext(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Scheme, "https") {
		return u.Hostname() != ""
	}
	if !strings.EqualFold(u.Scheme, "http") {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback()
}
