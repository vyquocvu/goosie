package js

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type WindowHandle string

type WindowMessage struct {
	Target       WindowHandle
	Source       WindowHandle
	SourceName   string
	Origin       string
	SourceKey    string
	TargetOrigin string
	Data         []byte
}

type WindowMessageSink func(WindowMessage)

type WindowClosed func(WindowHandle) bool

type WindowControlSink func(WindowHandle, string)

type WindowNameSink func(WindowHandle, string)

const (
	maxWindowHandleBytes       = 512
	maxOpenWindowsPerContext   = 64
	maxWindowControlActionSize = 16
)

func (r *Runtime) SetWindowMessageSink(sink WindowMessageSink) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.windowMessageSink = sink
	r.mu.Unlock()
}

func (r *Runtime) SetWindowClosed(closed WindowClosed) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.windowClosed = closed
	r.mu.Unlock()
}

func (r *Runtime) SetWindowControlSink(sink WindowControlSink) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.windowControlSink = sink
	r.mu.Unlock()
}

func (r *Runtime) SetWindowNameSink(sink WindowNameSink) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.windowNameSink = sink
	r.mu.Unlock()
}

func (r *Runtime) WindowHandle() WindowHandle {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.windowHandle
}

func (r *Runtime) WindowOrigin() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.windowOriginOverride != "" {
		return r.windowOriginOverride
	}
	return r.windowOrigin()
}

func (r *Runtime) WindowOpaqueID() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.windowOpaqueID
}

func (r *Runtime) WindowName() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.windowName
}

func (r *Runtime) allocateWindowHandle() (WindowHandle, bool) {
	r.mu.Lock()
	if r.openWindowCount >= maxOpenWindowsPerContext {
		r.mu.Unlock()
		return "", false
	}
	r.openWindowCount++
	count := r.openWindowCount
	base := r.windowHandle
	r.mu.Unlock()
	handle := WindowHandle(string(base) + "/open-" + strconv.Itoa(count))
	if len(handle) > maxWindowHandleBytes {
		return "", false
	}
	return handle, true
}

func clampWindowName(value string) string {
	if len(value) <= maxWindowHandleBytes {
		return value
	}
	cut := maxWindowHandleBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

func (r *Runtime) renameWindow(handle WindowHandle, value string) {
	if r == nil {
		return
	}
	value = clampWindowName(value)
	r.mu.Lock()
	if handle == "" || handle == r.windowHandle {
		r.windowName = value
	}
	sink := r.windowNameSink
	r.mu.Unlock()
	if sink != nil && handle != "" {
		sink(handle, value)
	}
}

func (r *Runtime) installWindowIdentity(win *goja.Object) {
	if r == nil || win == nil {
		return
	}
	_ = win.DefineAccessorProperty("name", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		r.mu.Lock()
		value := r.windowName
		r.mu.Unlock()
		return r.vm.ToValue(value)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := ""
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			value = call.Argument(0).String()
		}
		r.renameWindow(r.windowHandle, clampWindowName(value))
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	if r.openerHandle != "" {
		_ = win.Set("opener", r.newHostWindowProxy(r.openerHandle, r.openerName, goja.Null()))
	} else {
		_ = win.Set("opener", goja.Null())
	}
	_ = win.DefineAccessorProperty("closed", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.windowIsClosed(r.windowHandle))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	for _, action := range []string{"close", "focus", "blur"} {
		action := action
		_ = win.Set(action, func(goja.FunctionCall) goja.Value {
			r.controlWindow(r.windowHandle, action)
			return goja.Undefined()
		})
	}
}

func (r *Runtime) detachHostTransfers(transfer goja.Value) error {
	values := transferList(r.vm, transfer)
	seen := make(map[*goja.Object]bool, len(values))
	buffers := make([]goja.ArrayBuffer, 0, len(values))
	for _, value := range values {
		object, ok := value.(*goja.Object)
		if !ok || seen[object] {
			return errors.New("invalid or duplicate transferable")
		}
		seen[object] = true
		if messagePortFromObject(r.vm, value) != nil {
			return errors.New("MessagePort transfer across host windows is not supported")
		}
		if !isArrayBufferObject(r.vm, object) {
			return errors.New("object is not a supported transferable")
		}
		buffer, ok := object.Export().(goja.ArrayBuffer)
		if !ok || buffer.Detached() {
			return errors.New("ArrayBuffer is detached or invalid")
		}
		buffers = append(buffers, buffer)
	}
	for _, buffer := range buffers {
		if !buffer.Detach() {
			return errors.New("ArrayBuffer is detached or invalid")
		}
	}
	return nil
}

func (r *Runtime) newHostWindowProxy(handle WindowHandle, name string, opener goja.Value) *goja.Object {
	if proxy := r.hostWindowProxies[handle]; proxy != nil {
		return proxy
	}
	proxy := r.vm.NewObject()
	proxyName := clampWindowName(name)
	_ = proxy.DefineAccessorProperty("closed", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.windowIsClosed(handle))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proxy.DefineAccessorProperty("name", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(proxyName)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := ""
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			value = call.Argument(0).String()
		}
		proxyName = clampWindowName(value)
		r.renameWindow(handle, proxyName)
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = proxy.Set("postMessage", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'postMessage': 1 argument required"))
		}
		targetOrigin, transfer := postMessageOptions(r.vm, call)
		if !ValidTargetOrigin(targetOrigin) {
			panic(r.syntaxError("Failed to execute 'postMessage': invalid targetOrigin"))
		}
		encoded, err := serializeBroadcastMessage(r, call.Arguments[0])
		if err != nil {
			panic(r.dataCloneError(err.Error()))
		}
		if err := r.detachHostTransfers(transfer); err != nil {
			panic(r.dataCloneError(err.Error()))
		}
		r.mu.Lock()
		sink := r.windowMessageSink
		origin := r.windowOrigin()
		sourceName := r.windowName
		sourceOpaqueID := r.windowOpaqueID
		if r.windowHandle == "" {
			origin = "null"
		}
		r.mu.Unlock()
		if sink == nil {
			return goja.Undefined()
		}
		sourceKey := ""
		if origin == "null" {
			sourceKey = sourceOpaqueID
			if sourceKey == "" {
				sourceKey = string(r.windowHandle)
			}
		}
		sink(WindowMessage{
			Target:       handle,
			Source:       r.windowHandle,
			SourceName:   sourceName,
			Origin:       origin,
			SourceKey:    sourceKey,
			TargetOrigin: targetOrigin,
			Data:         encoded,
		})
		return goja.Undefined()
	})
	for _, action := range []string{"close", "focus", "blur"} {
		action := action
		_ = proxy.Set(action, func(goja.FunctionCall) goja.Value {
			r.controlWindow(handle, action)
			return goja.Undefined()
		})
	}
	if opener != nil && !goja.IsNull(opener) && !goja.IsUndefined(opener) {
		_ = proxy.Set("opener", opener)
	} else {
		_ = proxy.Set("opener", goja.Null())
	}
	if r.hostWindowProxies == nil {
		r.hostWindowProxies = make(map[WindowHandle]*goja.Object)
	}
	r.hostWindowProxies[handle] = proxy
	return proxy
}

func (r *Runtime) windowIsClosed(handle WindowHandle) bool {
	r.mu.Lock()
	closed := r.windowClosed
	r.mu.Unlock()
	if closed == nil {
		return false
	}
	return closed(handle)
}

func (r *Runtime) controlWindow(handle WindowHandle, action string) {
	if handle == "" || len(action) > maxWindowControlActionSize {
		return
	}
	r.mu.Lock()
	controlSink := r.windowControlSink
	nameSink := r.windowNameSink
	r.mu.Unlock()
	switch action {
	case "close":
		r.enqueueNavigation(NavigationRequest{Kind: NavigationCloseWindow, TargetHandle: handle})
	case "focus":
		r.enqueueNavigation(NavigationRequest{Kind: NavigationFocusWindow, TargetHandle: handle})
	case "blur":
		return
	default:
		if strings.HasPrefix(action, "rename:") {
			if nameSink != nil {
				nameSink(handle, strings.TrimPrefix(action, "rename:"))
			}
			return
		}
		if controlSink != nil {
			controlSink(handle, action)
		}
	}
}

func (r *Runtime) DeliverWindowMessage(message WindowMessage) {
	if r == nil || message.Target != r.windowHandle {
		return
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	recipientOrigin := r.windowOrigin()
	allowed := TargetOriginAllows(message.TargetOrigin, recipientOrigin, message.Origin)
	if recipientOrigin == "null" && message.TargetOrigin == "/" {
		recipientKey := r.windowOpaqueID
		if recipientKey == "" {
			recipientKey = string(r.windowHandle)
		}
		allowed = message.SourceKey != "" && message.SourceKey == recipientKey
	}
	if closed || !allowed {
		return
	}
	typ := "message"
	data, err := deserializeBroadcastMessage(r, message.Data)
	if err != nil {
		typ = "messageerror"
		data = goja.Null()
	}
	source := r.newHostWindowProxy(message.Source, message.SourceName, goja.Null())
	event := dom.NewTrustedMessageEvent(typ, false, false, data, message.Origin, "", source, nil)
	r.fireWindowListeners(&event.Event, true)
	r.fireWindowListeners(&event.Event, false)
}
