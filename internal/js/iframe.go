package js

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type iframePendingListener struct {
	typ      string
	callback goja.Value
	capture  bool
}

type iframeFormRequest struct {
	url         string
	contentType string
	body        []byte
}

// SetIFrameLoader supplies a document loader used when a connected iframe
// navigates. The callback receives the src attribute; base-aware hosts should
// use SetIFrameLoaderWithBase so nested frames resolve against their own URL.
func (r *Runtime) SetIFrameLoader(loader func(string) (*dom.Document, error)) {
	if loader == nil {
		r.iframeLoader = nil
		return
	}
	r.iframeLoader = func(_, src string) (*dom.Document, error) { return loader(src) }
}

// SetIFrameLoaderWithBase supplies a child-document loader that resolves each
// frame's src against the active document URL. The host owns transport policy
// and input admission; the runtime owns the browsing-context URL.
func (r *Runtime) SetIFrameLoaderWithBase(loader func(base, src string) (*dom.Document, error)) {
	r.iframeLoader = loader
}

// SetIFrameFormSubmitter supplies the host-owned transport for POST form
// navigations targeting a child browsing context. The runtime creates the
// encoded form body; the host applies origin policy and response limits.
func (r *Runtime) SetIFrameFormSubmitter(submit func(base, target, contentType string, body []byte) (*dom.Document, error)) {
	r.iframeFormSubmitter = submit
}

// SetIFrameScriptFetcher supplies the same-origin classic-script transport to
// srcdoc child realms. base is the inherited document URL and src is resolved
// by the child runtime before this callback is invoked.
func (r *Runtime) SetIFrameScriptFetcher(fetcher func(base, src string) ([]byte, error)) {
	r.iframeScriptFetcher = fetcher
}

// SetDynamicScriptFetcher supplies bounded transport for classic scripts
// inserted into a live document. Fetch and execution happen at a later event
// loop turn, followed by the element's load or error event.
func (r *Runtime) SetDynamicScriptFetcher(fetcher func(base, src string) ([]byte, error)) {
	r.dynamicScriptFetcher = fetcher
}

// SetIFrameModuleRunner supplies the host's bounded module-script executor for
// iframe document realms. The host controls dependency fetching and admission;
// the runtime invokes it after the child document's classic parser scripts.
func (r *Runtime) SetIFrameModuleRunner(run func(runtime *Runtime, doc *dom.Document, documentURL string)) {
	r.iframeModuleRunner = run
}

func (r *Runtime) installIFrameInterface(p *goja.Object) {
	r.installStringReflection(p, "name", "name")
	contentDocument := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		frame := r.getNode(call.This)
		if frame == nil || frame.Data != "iframe" || r.domDoc == nil {
			return goja.Null()
		}
		if !r.iframeIsSameOrigin(frame) {
			return goja.Null()
		}
		return r.wrapIFrameDocument(frame)
	})
	_ = p.DefineAccessorProperty("contentDocument", contentDocument, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)

	contentWindow := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		frame := r.getNode(call.This)
		if frame == nil || frame.Data != "iframe" || r.domDoc == nil {
			return goja.Null()
		}
		if frame.IsConnected() {
			if r.iframeConnectedContexts == nil {
				r.iframeConnectedContexts = make(map[*dom.Node]bool)
			}
			r.iframeConnectedContexts[frame] = true
		} else if r.iframeConnectedContexts[frame] {
			return goja.Null()
		}
		return r.iframeWindow(frame)
	})
	_ = p.DefineAccessorProperty("contentWindow", contentWindow, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.installURLReflection(p, "src", "src")
	_ = p.DefineAccessorProperty("sandbox", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		frame := r.getNode(call.This)
		if frame == nil || frame.Data != "iframe" {
			return goja.Undefined()
		}
		return r.tokenListObject(frame, "sandbox")
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		frame := r.getNode(call.This)
		if frame == nil || frame.Data != "iframe" {
			return goja.Undefined()
		}
		frame.SetAttribute("sandbox", call.Argument(0).String())
		r.notifyMutation()
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	srcdocGetter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		frame := r.getNode(call.This)
		if frame == nil || frame.Data != "iframe" || !frame.HasAttribute("srcdoc") {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(frame.GetAttribute("srcdoc"))
	})
	srcdocSetter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		frame := r.getNode(call.This)
		if frame == nil || frame.Data != "iframe" {
			return goja.Undefined()
		}
		frame.SetAttribute("srcdoc", call.Argument(0).String())
		r.notifyMutation()
		if r.nodeIsLive(frame) {
			r.queueIFrameLoad(frame)
		}
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty("srcdoc", srcdocGetter, srcdocSetter, goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.installEventHandler(p, "load")
	r.installEventHandler(p, "error")
}

func (r *Runtime) iframeIsSameOrigin(frame *dom.Node) bool {
	if frame == nil {
		return false
	}
	target := r.iframeLocations[frame]
	if target == "" || target == "about:blank" || target == "about:srcdoc" {
		return true
	}
	targetURL, err := url.Parse(target)
	if err != nil || (targetURL.Scheme != "http" && targetURL.Scheme != "https") {
		return true
	}
	if child := r.iframeChildRuntimes[frame]; child != nil {
		return sameOriginDomain(r, child)
	}
	// A live non-blank child that has not acquired a Runtime has not relaxed
	// its origin. A parent that has set document.domain is therefore not
	// same-origin-domain with it, even if their URL tuples still match.
	if r.documentDomains[r.domDoc] != "" {
		return false
	}
	base := r.documentURL()
	if base == nil || (base.Scheme != "http" && base.Scheme != "https") {
		return false
	}
	return strings.EqualFold(base.Scheme, targetURL.Scheme) &&
		strings.EqualFold(base.Hostname(), targetURL.Hostname()) && originPort(base) == originPort(targetURL)
}

// sameOriginDomain applies the same-origin-domain comparison used for
// synchronous WindowProxy and Document access. document.domain only relaxes
// this comparison when both documents set it; APIs that explicitly require
// tuple-origin equality (such as input.showPicker()) keep their own check.
func sameOriginDomain(a, b *Runtime) bool {
	if a == nil || b == nil {
		return false
	}
	urlA, urlB := a.documentURL(), b.documentURL()
	if urlA == nil || urlB == nil || urlA.Hostname() == "" || urlB.Hostname() == "" {
		return false
	}
	domainA := a.documentDomains[a.domDoc]
	domainB := b.documentDomains[b.domDoc]
	if domainA != "" || domainB != "" {
		return domainA != "" && domainB != "" &&
			strings.EqualFold(urlA.Scheme, urlB.Scheme) && domainA == domainB
	}
	return strings.EqualFold(urlA.Scheme, urlB.Scheme) &&
		strings.EqualFold(urlA.Hostname(), urlB.Hostname()) && originPort(urlA) == originPort(urlB)
}

func originPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

func (r *Runtime) iframeDocument(frame *dom.Node) *dom.Document {
	if r.iframeDocuments == nil {
		r.iframeDocuments = make(map[*dom.Node]*dom.Document)
	}
	if doc := r.iframeDocuments[frame]; doc != nil {
		return doc
	}
	doc, err := dom.ParseBounded(`<!doctype html><html><head></head><body></body></html>`, dom.ParseLimits{
		Nodes: 64, Depth: 16, Attributes: 32, AttributeBytes: 1024,
	})
	if err != nil {
		return nil
	}
	r.iframeDocuments[frame] = doc
	return doc
}

func (r *Runtime) iframeWindow(frame *dom.Node) goja.Value {
	if r.iframeWindows == nil {
		r.iframeWindows = make(map[*dom.Node]*goja.Object)
	}
	if win := r.iframeWindows[frame]; win != nil {
		return win
	}
	win := r.vm.NewObject()
	_ = win.Set("close", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if r.iframeOpenedContexts[frame] {
			r.closeIFrameContext(frame)
		}
		return goja.Undefined()
	}))
	_ = win.DefineAccessorProperty("closed", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.iframeClosedContexts[frame])
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = win.Set("focus", r.vm.ToValue(func(goja.FunctionCall) goja.Value { return goja.Undefined() }))
	_ = win.Set("blur", r.vm.ToValue(func(goja.FunctionCall) goja.Value { return goja.Undefined() }))
	if r.iframeRegistries == nil {
		r.iframeRegistries = make(map[*dom.Node]*CustomElementRegistry)
	}
	doc := r.iframeDocument(frame)
	registry, customElements := r.newCustomElementRegistry(r.vm.Get("CustomElementRegistry").(*goja.Object))
	registry.document = doc
	r.iframeRegistries[frame] = registry
	_ = win.Set("window", win)
	_ = win.Set("self", win)
	_ = win.DefineAccessorProperty("event", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if child := r.iframeChildRuntimes[frame]; child != nil {
			value := child.legacyWindowEvent
			if value == nil {
				return goja.Undefined()
			}
			return r.importChildValue(child, value, make(map[*goja.Object]*goja.Object), 0)
		}
		if pending := r.iframeWindowWrites[frame]; pending != nil {
			if value, exists := pending["event"]; exists {
				return value
			}
		}
		return goja.Undefined()
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		if child := r.iframeChildRuntimes[frame]; child != nil {
			child.legacyWindowEvent = child.importParentValue(r, value)
		} else {
			if r.iframeWindowWrites == nil {
				r.iframeWindowWrites = make(map[*dom.Node]map[string]goja.Value)
			}
			if r.iframeWindowWrites[frame] == nil {
				r.iframeWindowWrites[frame] = make(map[string]goja.Value)
			}
			r.iframeWindowWrites[frame]["event"] = value
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = win.Set("parent", r.vm.GlobalObject())
	_ = win.Set("top", r.vm.GlobalObject())
	_ = win.Set("opener", goja.Null())
	_ = win.Set("frameElement", r.wrapNode(frame))
	_ = win.Set("document", r.wrapDetachedDocumentWithRegistry(doc, registry))
	_ = win.Set("HTMLElement", r.vm.Get("HTMLElement"))
	_ = win.Set("DOMException", r.vm.Get("DOMException"))
	_ = win.Set("TypeError", r.vm.Get("TypeError"))
	_ = win.Set("Node", r.vm.Get("Node"))
	_ = win.Set("Element", r.vm.Get("Element"))
	_ = win.Set("CustomElementRegistry", r.vm.Get("CustomElementRegistry"))
	_ = win.Set("customElements", customElements)
	location := r.vm.NewObject()
	locationHref := func() string {
		if href := r.iframeLocations[frame]; href != "" {
			return href
		}
		return "about:blank"
	}
	// Location is stringifier-backed in the platform. WPT load handlers compare
	// it with "about:blank", and URL constructors consume it as a USVString.
	// Keeping the conversion tied to this live getter preserves navigation
	// updates without replacing the stable Location object.
	_ = location.DefineDataProperty("toString", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(locationHref())
	}), goja.FLAG_TRUE, goja.FLAG_FALSE, goja.FLAG_TRUE)
	locationURL := func() *url.URL {
		parsed, err := url.Parse(locationHref())
		if err != nil {
			return nil
		}
		return parsed
	}
	_ = location.DefineAccessorProperty("href", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(locationHref())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.navigateIFrameLocation(frame, call.Argument(0).String())
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = location.DefineAccessorProperty("search", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		parsed, err := url.Parse(locationHref())
		if err != nil || parsed.RawQuery == "" {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue("?" + parsed.RawQuery)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	for property, part := range map[string]func(*url.URL) string{
		"protocol": func(u *url.URL) string {
			if u == nil || u.Scheme == "" {
				return ""
			}
			return u.Scheme + ":"
		},
		"host": func(u *url.URL) string {
			if u == nil {
				return ""
			}
			return u.Host
		},
		"hostname": func(u *url.URL) string {
			if u == nil {
				return ""
			}
			return u.Hostname()
		},
		"port": func(u *url.URL) string {
			if u == nil {
				return ""
			}
			return u.Port()
		},
		"pathname": func(u *url.URL) string {
			if u == nil {
				return ""
			}
			return u.EscapedPath()
		},
		"hash": func(u *url.URL) string {
			if u == nil || u.Fragment == "" {
				return ""
			}
			return "#" + u.EscapedFragment()
		},
		"origin": func(u *url.URL) string {
			if u == nil || u.Scheme == "" || u.Host == "" {
				return "null"
			}
			return u.Scheme + "://" + u.Host
		},
	} {
		getter := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(part(locationURL()))
		})
		_ = location.DefineAccessorProperty(property, getter, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
	_ = win.Set("location", location)
	_ = win.Set("getComputedStyle", func(call goja.FunctionCall) goja.Value {
		return r.jsGetComputedStyle(call)
	})
	_ = win.Set("open", func(call goja.FunctionCall) goja.Value {
		if child := r.iframeChildRuntimes[frame]; child != nil {
			opened := child.openWindowCall(call)
			return r.importChildValue(child, opened, make(map[*goja.Object]*goja.Object), 0)
		}
		return r.iframeWindowOpenCall(frame, call)
	})
	var proxyValue *goja.Object
	r.installDOMParserConstructor(win, func() (*dom.Document, string) {
		document := r.iframeDocument(frame)
		if child := r.iframeChildRuntimes[frame]; child != nil {
			if child.domDoc == document {
				if childURL := child.documentURL(); childURL != nil {
					return document, childURL.String()
				}
			}
		}
		return document, locationHref()
	})
	_ = win.Set("postMessage", func(call goja.FunctionCall) goja.Value {
		child := r.iframeChildRuntimes[frame]
		if child == nil {
			return goja.Undefined()
		}
		return r.postMessageAcross(call, child, child.iframeParentWindow)
	})
	_ = win.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 || goja.IsNull(call.Argument(1)) || goja.IsUndefined(call.Argument(1)) {
			return goja.Undefined()
		}
		capture := len(call.Arguments) > 2 && call.Argument(2).ToBoolean()
		r.addIFrameWindowListener(frame, call.Argument(0).String(), call.Argument(1), capture)
		return goja.Undefined()
	})
	_ = win.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		r.removeIFrameWindowListener(frame, call.Argument(0).String(), call.Argument(1))
		return goja.Undefined()
	})
	_ = win.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'dispatchEvent': 1 argument required"))
		}
		child := r.iframeChildRuntimes[frame]
		if child == nil {
			child = r.ensureIFrameInitialRuntime(frame, locationHref())
		}
		if child == nil {
			return r.vm.ToValue(false)
		}
		result := child.jsWindowDispatchEvent(goja.FunctionCall{
			This:      child.vm.GlobalObject(),
			Arguments: []goja.Value{child.importParentValue(r, call.Argument(0))},
		})
		return child.importParentValue(r, result)
	})
	_ = win.DefineAccessorProperty("onmessage", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if value, ok := r.iframePendingOnMsg[frame]; ok {
			return value
		}
		child := r.iframeChildRuntimes[frame]
		if child == nil {
			return goja.Null()
		}
		return r.importChildValue(child, child.vm.Get("onmessage"), make(map[*goja.Object]*goja.Object), 0)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if r.iframePendingOnMsg == nil {
			r.iframePendingOnMsg = make(map[*dom.Node]goja.Value)
		}
		value := call.Argument(0)
		if goja.IsNull(value) || goja.IsUndefined(value) {
			delete(r.iframePendingOnMsg, frame)
		} else {
			r.iframePendingOnMsg[frame] = value
		}
		child := r.iframeChildRuntimes[frame]
		if child != nil {
			_ = child.vm.Set("onmessage", child.importParentValue(r, value))
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	functionConstructor := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		child := r.ensureIFrameInitialRuntime(frame, locationHref())
		if child == nil {
			panic(r.vm.NewTypeError("The iframe Function constructor is unavailable"))
		}
		constructor, ok := goja.AssertConstructor(child.vm.Get("Function"))
		if !ok {
			panic(r.vm.NewTypeError("The iframe Function constructor is unavailable"))
		}
		args := make([]goja.Value, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = child.vm.ToValue(arg.String())
		}
		function, err := constructor(nil, args...)
		if err != nil {
			panic(r.importCrossRealmException(child, err))
		}
		imported := r.importChildValue(child, function, make(map[*goja.Object]*goja.Object), 0)
		return imported.ToObject(r.vm)
	}).(*goja.Object)
	_ = win.Set("Function", functionConstructor)
	r.iframeWindowTargets[frame] = win
	proxy := r.vm.NewProxy(win, &goja.ProxyTrapConfig{
		Get: func(target *goja.Object, property string, _ goja.Value) goja.Value {
			if !r.iframeIsSameOrigin(frame) {
				if value, ok := r.crossOriginWindowValue(frame, proxyValue, target, property); ok {
					return value
				}
				panic(r.domError("SecurityError", "Blocked a cross-origin Window property access"))
			}
			child := r.iframeChildRuntimes[frame]
			if child == nil && property == "Node" {
				// Interface constructors belong to the iframe's realm even while
				// it is still on its initial document. Establish that realm before
				// exposing Node/Element constructors so wrappers returned from the
				// iframe's Document can share its interface prototypes.
				child = r.ensureIFrameInitialRuntime(frame, locationHref())
			}
			if child == nil && property != "Node" && r.ifaceCtors[property] != nil {
				if existing := target.Get(property); existing != nil && !goja.IsUndefined(existing) {
					return r.iframeInterfaceConstructor(frame, property)
				}
			}
			if child == nil && property == "Proxy" {
				return r.iframeProxyConstructor(frame)
			}
			if property == "navigator" {
				if child := r.iframeChildRuntimes[frame]; child != nil {
					return r.iframeNavigator(frame, child)
				}
			}
			if writes := r.iframeWindowWrites[frame]; writes != nil {
				if value, exists := writes[property]; exists {
					if value != nil && !goja.IsUndefined(value) {
						return value
					}
					if child := r.iframeChildRuntimes[frame]; child != nil {
						childValue := child.vm.GlobalObject().Get(property)
						if childValue != nil && !goja.IsUndefined(childValue) {
							return r.importChildValue(child, childValue, make(map[*goja.Object]*goja.Object), 0)
						}
					}
					return value
				}
			}
			if child != nil {
				if property == "document" {
					if document := child.vm.Get("document"); document != nil && !goja.IsUndefined(document) {
						return r.wrapIFrameDocument(frame)
					}
				}
				if property == "customElements" || property == "CustomElementRegistry" || property == "DOMException" || property == "TypeError" || r.ifaceCtors[property] != nil {
					if childValue := child.vm.GlobalObject().Get(property); childValue != nil && !goja.IsUndefined(childValue) {
						return r.importChildValue(child, childValue, make(map[*goja.Object]*goja.Object), 0)
					}
				}
			}
			value := target.Get(property)
			if value == nil || goja.IsUndefined(value) {
				child = r.iframeChildRuntimes[frame]
				if child == nil && (iframeRealmIntrinsic(property) || property == "BroadcastChannel" || property == "customElements" || r.ifaceCtors[property] != nil && !r.iframeRegistryHasDefinitions(frame)) {
					child = r.ensureIFrameInitialRuntime(frame, locationHref())
				}
				if child != nil {
					childValue := child.vm.GlobalObject().Get(property)
					if (childValue == nil || goja.IsUndefined(childValue)) && r.ifaceCtors[property] != nil {
						child.ensureHTMLInterfaceConstructor(property)
						childValue = child.vm.GlobalObject().Get(property)
					}
					if childValue != nil && !goja.IsUndefined(childValue) {
						return r.importChildValue(child, childValue, make(map[*goja.Object]*goja.Object), 0)
					}
				}
				// Before an iframe has a script Runtime, its initial about:blank
				// WindowProxy still exposes the HTML interface constructors. The
				// parent Runtime already owns the shared interface prototypes used
				// by wrappers for that initial document; use those constructors
				// until a child Runtime can provide its own globals.
				if constructor := r.ifaceCtors[property]; constructor != nil {
					return r.iframeInterfaceConstructor(frame, property)
				}
			}
			return value
		},
		GetSym: func(target *goja.Object, symbol *goja.Symbol, _ goja.Value) goja.Value {
			if !r.iframeIsSameOrigin(frame) {
				if r.isCrossOriginWindowSymbol(symbol) {
					return goja.Undefined()
				}
				panic(r.domError("SecurityError", "Blocked a cross-origin Window symbol access"))
			}
			return target.GetSymbol(symbol)
		},
		Set: func(target *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
			if !r.iframeIsSameOrigin(frame) {
				if property == "location" {
					r.navigateIFrameLocation(frame, assigned.String())
					return true
				}
				panic(r.domError("SecurityError", "Blocked a cross-origin Window property assignment"))
			}
			if err := target.Set(property, assigned); err != nil {
				return false
			}
			switch property {
			case "window", "self", "document", "parent", "top", "frameElement", "location":
				return true
			}
			if r.iframeWindowWrites == nil {
				r.iframeWindowWrites = make(map[*dom.Node]map[string]goja.Value)
			}
			if r.iframeWindowWrites[frame] == nil {
				r.iframeWindowWrites[frame] = make(map[string]goja.Value)
			}
			r.iframeWindowWrites[frame][property] = assigned
			if child := r.iframeChildRuntimes[frame]; child != nil {
				childValue := child.importParentValue(r, assigned)
				if err := child.vm.Set(property, childValue); err != nil {
					return false
				}
				if childWindow, ok := child.vm.Get("window").(*goja.Object); ok {
					_ = childWindow.Set(property, childValue)
				}
			}
			return true
		},
		SetSym: func(target *goja.Object, symbol *goja.Symbol, assigned goja.Value, _ goja.Value) bool {
			if !r.iframeIsSameOrigin(frame) {
				panic(r.domError("SecurityError", "Blocked a cross-origin Window symbol assignment"))
			}
			return target.SetSymbol(symbol, assigned) == nil
		},
		Has: func(target *goja.Object, property string) bool {
			if !r.iframeIsSameOrigin(frame) {
				if _, ok := r.crossOriginWindowValue(frame, proxyValue, target, property); ok {
					return true
				}
				panic(r.domError("SecurityError", "Blocked a cross-origin Window property check"))
			}
			if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
				return true
			}
			if child := r.iframeChildRuntimes[frame]; child != nil {
				value := child.vm.GlobalObject().Get(property)
				if value != nil && !goja.IsUndefined(value) {
					return true
				}
			}
			return r.ifaceCtors[property] != nil
		},
		HasSym: func(target *goja.Object, symbol *goja.Symbol) bool {
			if !r.iframeIsSameOrigin(frame) {
				if r.isCrossOriginWindowSymbol(symbol) {
					return true
				}
				panic(r.domError("SecurityError", "Blocked a cross-origin Window symbol check"))
			}
			return !goja.IsUndefined(target.GetSymbol(symbol))
		},
		GetOwnPropertyDescriptor: func(target *goja.Object, property string) goja.PropertyDescriptor {
			if r.iframeIsSameOrigin(frame) {
				return r.ownPropertyDescriptor(target, property)
			}
			descriptor, ok := r.crossOriginWindowDescriptor(frame, proxyValue, target, property)
			if !ok {
				panic(r.domError("SecurityError", "Blocked a cross-origin Window property descriptor access"))
			}
			return descriptor
		},
		GetOwnPropertyDescriptorSym: func(target *goja.Object, symbol *goja.Symbol) goja.PropertyDescriptor {
			if !r.iframeIsSameOrigin(frame) {
				if r.isCrossOriginWindowSymbol(symbol) {
					return crossOriginWindowSymbolDescriptor()
				}
				panic(r.domError("SecurityError", "Blocked a cross-origin Window symbol descriptor access"))
			}
			return r.ownSymbolPropertyDescriptor(target, symbol)
		},
		OwnKeys: func(target *goja.Object) *goja.Object {
			if !r.iframeIsSameOrigin(frame) {
				return r.vm.NewArray(r.crossOriginWindowKeys(frame, target)...)
			}
			keys := make([]interface{}, 0)
			for _, key := range target.GetOwnPropertyNames() {
				keys = append(keys, r.vm.ToValue(key))
			}
			for _, symbol := range target.Symbols() {
				keys = append(keys, r.vm.ToValue(symbol))
			}
			return r.vm.NewArray(keys...)
		},
		DeleteProperty: func(target *goja.Object, property string) bool {
			if !r.iframeIsSameOrigin(frame) {
				panic(r.domError("SecurityError", "Blocked a cross-origin Window property deletion"))
			}
			return target.Delete(property) == nil
		},
		DeletePropertySym: func(target *goja.Object, symbol *goja.Symbol) bool {
			if !r.iframeIsSameOrigin(frame) {
				panic(r.domError("SecurityError", "Blocked a cross-origin Window symbol deletion"))
			}
			return target.DeleteSymbol(symbol) == nil
		},
		DefineProperty: func(target *goja.Object, property string, descriptor goja.PropertyDescriptor) bool {
			if !r.iframeIsSameOrigin(frame) {
				panic(r.domError("SecurityError", "Blocked a cross-origin Window property definition"))
			}
			return r.defineOwnProperty(target, property, descriptor)
		},
		DefinePropertySym: func(target *goja.Object, symbol *goja.Symbol, descriptor goja.PropertyDescriptor) bool {
			if !r.iframeIsSameOrigin(frame) {
				panic(r.domError("SecurityError", "Blocked a cross-origin Window symbol definition"))
			}
			return r.defineOwnSymbolProperty(target, symbol, descriptor)
		},
		GetPrototypeOf: func(target *goja.Object) *goja.Object {
			if !r.iframeIsSameOrigin(frame) {
				return nil
			}
			return target.Prototype()
		},
		SetPrototypeOf: func(target, prototype *goja.Object) bool {
			if !r.iframeIsSameOrigin(frame) {
				return prototype == nil
			}
			setPrototype, ok := goja.AssertFunction(r.vm.Get("Object").ToObject(r.vm).Get("setPrototypeOf"))
			if !ok {
				return false
			}
			_, err := setPrototype(goja.Undefined(), target, prototype)
			return err == nil
		},
		IsExtensible:      func(*goja.Object) bool { return true },
		PreventExtensions: func(*goja.Object) bool { return false },
	})
	proxyValue = r.vm.ToValue(proxy).ToObject(r.vm)
	_ = win.Set("window", proxyValue)
	_ = win.Set("self", proxyValue)
	_ = win.Set("frames", proxyValue)
	r.iframeWindows[frame] = proxyValue
	if child := r.iframeChildRuntimes[frame]; child != nil {
		r.syncIFrameGlobals(frame, child, r.iframeGlobalBaseline[frame])
	}
	return proxyValue
}

func (r *Runtime) iframeCrossOriginLocation(frame *dom.Node) *goja.Object {
	if r.iframeCrossOriginLocations == nil {
		r.iframeCrossOriginLocations = make(map[*dom.Node]*goja.Object)
	}
	if location := r.iframeCrossOriginLocations[frame]; location != nil {
		return location
	}
	target := r.vm.NewObject()
	_ = target.DefineAccessorProperty("href", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.navigateIFrameLocation(frame, call.Argument(0).String())
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = target.Set("replace", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.navigateIFrameLocation(frame, call.Argument(0).String())
		return goja.Undefined()
	}))
	location := r.vm.ToValue(r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(object *goja.Object, property string, _ goja.Value) goja.Value {
			if property == "href" {
				panic(r.domError("SecurityError", "Blocked a cross-origin Location href read"))
			}
			if property == "replace" {
				return object.Get(property)
			}
			if property == "then" || property == "Symbol.toStringTag" {
				return goja.Undefined()
			}
			panic(r.domError("SecurityError", "Blocked a cross-origin Location property access"))
		},
		Set: func(_ *goja.Object, property string, value goja.Value, _ goja.Value) bool {
			if property != "href" {
				panic(r.domError("SecurityError", "Blocked a cross-origin Location property assignment"))
			}
			r.navigateIFrameLocation(frame, value.String())
			return true
		},
		Has: func(_ *goja.Object, property string) bool {
			if property == "href" || property == "replace" || property == "then" {
				return true
			}
			panic(r.domError("SecurityError", "Blocked a cross-origin Location property check"))
		},
		GetOwnPropertyDescriptor: func(object *goja.Object, property string) goja.PropertyDescriptor {
			switch property {
			case "then":
				return goja.PropertyDescriptor{
					Value:        goja.Undefined(),
					Writable:     goja.FLAG_FALSE,
					Enumerable:   goja.FLAG_FALSE,
					Configurable: goja.FLAG_TRUE,
				}
			case "href":
				getDescriptor, ok := goja.AssertFunction(r.vm.Get("Object").ToObject(r.vm).Get("getOwnPropertyDescriptor"))
				if !ok {
					return goja.PropertyDescriptor{}
				}
				descriptor, err := getDescriptor(goja.Undefined(), object, r.vm.ToValue("href"))
				if err != nil || goja.IsUndefined(descriptor) || goja.IsNull(descriptor) {
					return goja.PropertyDescriptor{}
				}
				return goja.PropertyDescriptor{
					Setter:       descriptor.ToObject(r.vm).Get("set"),
					Enumerable:   goja.FLAG_FALSE,
					Configurable: goja.FLAG_TRUE,
				}
			case "replace":
				return goja.PropertyDescriptor{
					Value:        object.Get("replace"),
					Writable:     goja.FLAG_FALSE,
					Enumerable:   goja.FLAG_FALSE,
					Configurable: goja.FLAG_TRUE,
				}
			default:
				panic(r.domError("SecurityError", "Blocked a cross-origin Location property descriptor access"))
			}
		},
		OwnKeys: func(*goja.Object) *goja.Object {
			keys := []interface{}{"href", "replace", "then"}
			symbol := r.vm.Get("Symbol").ToObject(r.vm)
			for _, name := range []string{"toStringTag", "hasInstance", "isConcatSpreadable"} {
				keys = append(keys, symbol.Get(name))
			}
			return r.vm.NewArray(keys...)
		},
		GetSym: func(_ *goja.Object, symbol *goja.Symbol, _ goja.Value) goja.Value {
			if r.isCrossOriginWindowSymbol(symbol) {
				return goja.Undefined()
			}
			panic(r.domError("SecurityError", "Blocked a cross-origin Location symbol access"))
		},
		HasSym: func(_ *goja.Object, symbol *goja.Symbol) bool {
			if r.isCrossOriginWindowSymbol(symbol) {
				return true
			}
			panic(r.domError("SecurityError", "Blocked a cross-origin Location symbol check"))
		},
		GetOwnPropertyDescriptorSym: func(_ *goja.Object, symbol *goja.Symbol) goja.PropertyDescriptor {
			if r.isCrossOriginWindowSymbol(symbol) {
				return crossOriginWindowSymbolDescriptor()
			}
			panic(r.domError("SecurityError", "Blocked a cross-origin Location symbol descriptor access"))
		},
		DeletePropertySym: func(_ *goja.Object, _ *goja.Symbol) bool {
			panic(r.domError("SecurityError", "Blocked a cross-origin Location symbol deletion"))
		},
		DefinePropertySym: func(_ *goja.Object, _ *goja.Symbol, _ goja.PropertyDescriptor) bool {
			panic(r.domError("SecurityError", "Blocked a cross-origin Location symbol definition"))
		},
		DeleteProperty: func(_ *goja.Object, property string) bool {
			panic(r.domError("SecurityError", "Blocked a cross-origin Location property deletion"))
		},
		DefineProperty: func(_ *goja.Object, property string, _ goja.PropertyDescriptor) bool {
			panic(r.domError("SecurityError", "Blocked a cross-origin Location property definition"))
		},
		GetPrototypeOf:    func(*goja.Object) *goja.Object { return nil },
		SetPrototypeOf:    func(_ *goja.Object, prototype *goja.Object) bool { return prototype == nil },
		IsExtensible:      func(*goja.Object) bool { return true },
		PreventExtensions: func(*goja.Object) bool { return false },
	})).ToObject(r.vm)
	r.iframeCrossOriginLocations[frame] = location
	return location
}

func (r *Runtime) crossOriginWindowValue(frame *dom.Node, proxy, target *goja.Object, property string) (goja.Value, bool) {
	if index, ok := canonicalWindowIndex(property); ok {
		child := r.iframeChildRuntimes[frame]
		if child == nil {
			return nil, false
		}
		children := child.windowFrameNodes()
		if index < 0 || index >= len(children) {
			return nil, false
		}
		return r.importChildValue(child, child.iframeWindow(children[index]), make(map[*goja.Object]*goja.Object), 0), true
	}
	switch property {
	case "window", "self", "frames":
		return proxy, true
	case "location":
		return r.iframeCrossOriginLocation(frame), true
	case "length":
		if child := r.iframeChildRuntimes[frame]; child != nil {
			return r.vm.ToValue(len(child.windowFrameNodes())), true
		}
		return r.vm.ToValue(0), true
	case "close", "closed", "focus", "blur", "top", "opener", "parent", "postMessage":
		return target.Get(property), true
	case "then":
		return goja.Undefined(), true
	default:
		return nil, false
	}
}

func canonicalWindowIndex(property string) (int, bool) {
	index, err := strconv.Atoi(property)
	if err != nil || index < 0 || strconv.Itoa(index) != property {
		return 0, false
	}
	return index, true
}

func (r *Runtime) isCrossOriginWindowSymbol(symbol *goja.Symbol) bool {
	if symbol == nil {
		return false
	}
	constructor := r.vm.Get("Symbol").ToObject(r.vm)
	for _, name := range []string{"toStringTag", "hasInstance", "isConcatSpreadable"} {
		if symbol.StrictEquals(constructor.Get(name)) {
			return true
		}
	}
	return false
}

func crossOriginWindowSymbolDescriptor() goja.PropertyDescriptor {
	return goja.PropertyDescriptor{
		Value:        goja.Undefined(),
		Writable:     goja.FLAG_FALSE,
		Enumerable:   goja.FLAG_FALSE,
		Configurable: goja.FLAG_TRUE,
	}
}

func (r *Runtime) crossOriginWindowKeys(frame *dom.Node, target *goja.Object) []interface{} {
	keys := make([]interface{}, 0, 18)
	if child := r.iframeChildRuntimes[frame]; child != nil {
		for i := range child.windowFrameNodes() {
			keys = append(keys, r.vm.ToValue(strconv.Itoa(i)))
		}
	}
	for _, property := range []string{"window", "self", "location", "close", "closed", "focus", "blur", "frames", "length", "top", "opener", "parent", "postMessage", "then"} {
		keys = append(keys, r.vm.ToValue(property))
	}
	symbol := r.vm.Get("Symbol").ToObject(r.vm)
	for _, name := range []string{"toStringTag", "hasInstance", "isConcatSpreadable"} {
		keys = append(keys, symbol.Get(name))
	}
	return keys
}

func (r *Runtime) crossOriginWindowDescriptor(frame *dom.Node, proxy, target *goja.Object, property string) (goja.PropertyDescriptor, bool) {
	value, ok := r.crossOriginWindowValue(frame, proxy, target, property)
	if !ok {
		return goja.PropertyDescriptor{}, false
	}
	desc := goja.PropertyDescriptor{Configurable: goja.FLAG_TRUE, Enumerable: goja.FLAG_FALSE}
	if _, indexed := canonicalWindowIndex(property); indexed {
		desc.Enumerable = goja.FLAG_TRUE
	}
	if property == "window" || property == "self" || property == "location" || property == "closed" || property == "frames" || property == "length" || property == "top" || property == "opener" || property == "parent" {
		desc.Getter = r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			current, exists := r.crossOriginWindowValue(frame, proxy, target, property)
			if !exists {
				return goja.Undefined()
			}
			return current
		})
		if property == "location" {
			desc.Setter = r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				r.navigateIFrameLocation(frame, call.Argument(0).String())
				return goja.Undefined()
			})
		}
		return desc, true
	}
	desc.Value = value
	desc.Writable = goja.FLAG_FALSE
	return desc, true
}

func (r *Runtime) ownPropertyDescriptor(target *goja.Object, property string) goja.PropertyDescriptor {
	objectConstructor := r.vm.Get("Object").ToObject(r.vm)
	getDescriptor, ok := goja.AssertFunction(objectConstructor.Get("getOwnPropertyDescriptor"))
	if !ok {
		return goja.PropertyDescriptor{}
	}
	value, err := getDescriptor(goja.Undefined(), target, r.vm.ToValue(property))
	if err != nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return goja.PropertyDescriptor{}
	}
	descriptor := value.ToObject(r.vm)
	result := goja.PropertyDescriptor{}
	if v := descriptor.Get("value"); v != nil && !goja.IsUndefined(v) {
		result.Value = v
	}
	if v := descriptor.Get("get"); v != nil && !goja.IsUndefined(v) {
		result.Getter = v
	}
	if v := descriptor.Get("set"); v != nil && !goja.IsUndefined(v) {
		result.Setter = v
	}
	if v := descriptor.Get("writable"); v != nil && !goja.IsUndefined(v) {
		result.Writable = goja.ToFlag(v.ToBoolean())
	}
	if v := descriptor.Get("enumerable"); v != nil && !goja.IsUndefined(v) {
		result.Enumerable = goja.ToFlag(v.ToBoolean())
	}
	if v := descriptor.Get("configurable"); v != nil && !goja.IsUndefined(v) {
		result.Configurable = goja.ToFlag(v.ToBoolean())
	}
	return result
}

func (r *Runtime) ownSymbolPropertyDescriptor(target *goja.Object, symbol *goja.Symbol) goja.PropertyDescriptor {
	objectConstructor := r.vm.Get("Object").ToObject(r.vm)
	getDescriptor, ok := goja.AssertFunction(objectConstructor.Get("getOwnPropertyDescriptor"))
	if !ok {
		return goja.PropertyDescriptor{}
	}
	value, err := getDescriptor(goja.Undefined(), target, symbol)
	if err != nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return goja.PropertyDescriptor{}
	}
	descriptor := value.ToObject(r.vm)
	result := goja.PropertyDescriptor{}
	if v := descriptor.Get("value"); v != nil && !goja.IsUndefined(v) {
		result.Value = v
	}
	if v := descriptor.Get("get"); v != nil && !goja.IsUndefined(v) {
		result.Getter = v
	}
	if v := descriptor.Get("set"); v != nil && !goja.IsUndefined(v) {
		result.Setter = v
	}
	if v := descriptor.Get("writable"); v != nil && !goja.IsUndefined(v) {
		result.Writable = goja.ToFlag(v.ToBoolean())
	}
	if v := descriptor.Get("enumerable"); v != nil && !goja.IsUndefined(v) {
		result.Enumerable = goja.ToFlag(v.ToBoolean())
	}
	if v := descriptor.Get("configurable"); v != nil && !goja.IsUndefined(v) {
		result.Configurable = goja.ToFlag(v.ToBoolean())
	}
	return result
}

func (r *Runtime) defineOwnProperty(target *goja.Object, property string, descriptor goja.PropertyDescriptor) bool {
	objectConstructor := r.vm.Get("Object").ToObject(r.vm)
	define, ok := goja.AssertFunction(objectConstructor.Get("defineProperty"))
	if !ok {
		return false
	}
	desc := r.vm.NewObject()
	if descriptor.Value != nil {
		_ = desc.Set("value", descriptor.Value)
	}
	if descriptor.Getter != nil {
		_ = desc.Set("get", descriptor.Getter)
	}
	if descriptor.Setter != nil {
		_ = desc.Set("set", descriptor.Setter)
	}
	if descriptor.Writable != goja.FLAG_NOT_SET {
		_ = desc.Set("writable", descriptor.Writable.Bool())
	}
	if descriptor.Enumerable != goja.FLAG_NOT_SET {
		_ = desc.Set("enumerable", descriptor.Enumerable.Bool())
	}
	if descriptor.Configurable != goja.FLAG_NOT_SET {
		_ = desc.Set("configurable", descriptor.Configurable.Bool())
	}
	if _, err := define(goja.Undefined(), target, r.vm.ToValue(property), desc); err != nil {
		return false
	}
	return true
}

func (r *Runtime) defineOwnSymbolProperty(target *goja.Object, symbol *goja.Symbol, descriptor goja.PropertyDescriptor) bool {
	objectConstructor := r.vm.Get("Object").ToObject(r.vm)
	define, ok := goja.AssertFunction(objectConstructor.Get("defineProperty"))
	if !ok {
		return false
	}
	desc := r.vm.NewObject()
	if descriptor.Value != nil {
		_ = desc.Set("value", descriptor.Value)
	}
	if descriptor.Getter != nil {
		_ = desc.Set("get", descriptor.Getter)
	}
	if descriptor.Setter != nil {
		_ = desc.Set("set", descriptor.Setter)
	}
	if descriptor.Writable != goja.FLAG_NOT_SET {
		_ = desc.Set("writable", descriptor.Writable.Bool())
	}
	if descriptor.Enumerable != goja.FLAG_NOT_SET {
		_ = desc.Set("enumerable", descriptor.Enumerable.Bool())
	}
	if descriptor.Configurable != goja.FLAG_NOT_SET {
		_ = desc.Set("configurable", descriptor.Configurable.Bool())
	}
	if _, err := define(goja.Undefined(), target, symbol, desc); err != nil {
		return false
	}
	return true
}

// iframeInterfaceConstructor gives an initial about:blank WindowProxy its own
// HTML interface constructor/prototype identity without replacing the frame's
// already exposed CustomElementRegistry. Its prototype extends the embedding
// realm's native interface prototype, so existing interface checks and methods
// remain available while NewTarget fallback identity stays per browsing
// context.
func (r *Runtime) iframeInterfaceConstructor(frame *dom.Node, name string) goja.Value {
	constructor := r.ifaceCtors[name]
	if constructor == nil {
		return goja.Undefined()
	}
	if r.iframeInterfaceConstructors == nil {
		r.iframeInterfaceConstructors = make(map[*dom.Node]map[string]*goja.Object)
	}
	if byName := r.iframeInterfaceConstructors[frame]; byName != nil {
		if existing := byName[name]; existing != nil {
			return existing
		}
	}
	canonicalPrototype, ok := constructor.Get("prototype").(*goja.Object)
	if !ok {
		return constructor
	}
	framePrototype := r.vm.NewObject()
	_ = framePrototype.SetPrototype(canonicalPrototype)
	wrapper := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		construct, ok := goja.AssertConstructor(constructor)
		if !ok {
			panic(r.vm.NewTypeError("Illegal constructor"))
		}
		instance, err := construct(call.NewTarget, call.Arguments...)
		if err != nil {
			panic(err)
		}
		return instance
	}).ToObject(r.vm)
	_ = wrapper.Set("prototype", framePrototype)
	_ = framePrototype.Set("constructor", wrapper)
	if r.iframeInterfaceConstructors[frame] == nil {
		r.iframeInterfaceConstructors[frame] = make(map[string]*goja.Object)
	}
	r.iframeInterfaceConstructors[frame][name] = wrapper
	return wrapper
}

// iframeProxyConstructor exposes Proxy on an initial WindowProxy. GetFunctionRealm
// follows a Proxy's target, so constructing through this wrapper preserves the
// target function's realm while keeping the WindowProxy property distinct.
func (r *Runtime) iframeProxyConstructor(frame *dom.Node) goja.Value {
	if r.iframeInterfaceConstructors == nil {
		r.iframeInterfaceConstructors = make(map[*dom.Node]map[string]*goja.Object)
	}
	if existing := r.iframeInterfaceConstructors[frame]["Proxy"]; existing != nil {
		return existing
	}
	native := r.vm.Get("Proxy")
	construct, ok := goja.AssertConstructor(native)
	if !ok {
		return goja.Undefined()
	}
	wrapper := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		instance, err := construct(call.NewTarget, call.Arguments...)
		if err != nil {
			panic(err)
		}
		return instance
	}).ToObject(r.vm)
	_ = wrapper.Set("prototype", goja.Undefined())
	if r.iframeInterfaceConstructors[frame] == nil {
		r.iframeInterfaceConstructors[frame] = make(map[string]*goja.Object)
	}
	r.iframeInterfaceConstructors[frame]["Proxy"] = wrapper
	return wrapper
}

func (r *Runtime) iframeInterfacePrototype(doc *dom.Document, name string) *goja.Object {
	for frame, frameDocument := range r.iframeDocuments {
		if frameDocument != doc {
			continue
		}
		if child := r.iframeChildRuntimes[frame]; child != nil {
			child.ensureHTMLInterfaceConstructor(name)
			if constructor := child.ifaceCtors[name]; constructor != nil {
				if prototype, ok := constructor.Get("prototype").(*goja.Object); ok {
					if imported, ok := r.importChildValue(child, prototype, make(map[*goja.Object]*goja.Object), 0).(*goja.Object); ok {
						return imported
					}
				}
			}
			continue
		}
		if constructor, ok := r.iframeInterfaceConstructor(frame, name).(*goja.Object); ok {
			if prototype, ok := constructor.Get("prototype").(*goja.Object); ok {
				return prototype
			}
		}
	}
	return nil
}

func (r *Runtime) ensureIFrameInitialRuntime(frame *dom.Node, childURL string) *Runtime {
	if child := r.iframeChildRuntimes[frame]; child != nil {
		return child
	}
	doc := r.iframeDocument(frame)
	if doc == nil {
		return nil
	}
	child, baseline, err := r.newIFrameChildRuntime(frame, doc, childURL)
	if err != nil {
		return nil
	}
	if r.iframeChildRuntimes == nil {
		r.iframeChildRuntimes = make(map[*dom.Node]*Runtime)
	}
	if r.iframeGlobalBaseline == nil {
		r.iframeGlobalBaseline = make(map[*dom.Node]map[string]goja.Value)
	}
	r.iframeChildRuntimes[frame] = child
	r.iframeGlobalBaseline[frame] = baseline
	r.alignIFrameWrapperPrototypes(r.iframeDocument(frame), child)
	if writes := r.iframeWindowWrites[frame]; writes != nil {
		for property, value := range writes {
			if property == "window" || property == "self" || property == "document" || property == "parent" || property == "top" || property == "frameElement" || property == "location" || property == "close" {
				continue
			}
			_ = child.vm.Set(property, child.importParentValue(r, value))
		}
	}
	r.installPendingIFrameWindowListeners(frame, child)
	r.syncIFrameGlobals(frame, child, baseline)
	return child
}

func (r *Runtime) iframeRegistryHasDefinitions(frame *dom.Node) bool {
	registry := r.iframeRegistries[frame]
	if registry == nil {
		return false
	}
	registry.mu.Lock()
	defined := len(registry.definitions) != 0
	registry.mu.Unlock()
	return defined
}

// alignIFrameWrapperPrototypes updates parent-runtime wrappers that escaped
// before a lazily-created child Runtime existed. Those wrappers still represent
// nodes in the same active Document, so their interface prototype must follow
// the newly established realm just as newly-created wrappers do.
func (r *Runtime) alignIFrameWrapperPrototypes(doc *dom.Document, child *Runtime) {
	if doc == nil || child == nil {
		return
	}
	prototypeInterfaces := make(map[*goja.Object]string, len(r.ifaceCtors))
	for name, constructor := range r.ifaceCtors {
		if prototype, ok := constructor.Get("prototype").(*goja.Object); ok {
			prototypeInterfaces[prototype] = name
		}
	}
	// Nodes can be wrapped before their child Runtime exists. In that window,
	// iframeInterfacePrototype gives them a per-frame prototype that inherits
	// from the parent interface prototype. Recognize those prototypes too so a
	// later child Runtime can align escaped wrappers to the active realm.
	for _, constructors := range r.iframeInterfaceConstructors {
		for name, constructor := range constructors {
			if name != "HTMLHtmlElement" {
				continue
			}
			if prototype, ok := constructor.Get("prototype").(*goja.Object); ok {
				prototypeInterfaces[prototype] = name
			}
		}
	}
	for id, node := range r.nodeRegistry {
		if node == nil || node.Doc != doc {
			continue
		}
		if child.customElementInstances[node] != nil || child.isFailedCustomElement(node) {
			// Custom-element construction owns its prototype, including the
			// destination interface retained after a failed constructor.
			continue
		}
		wrapper := r.nodeWrappers[id]
		if wrapper == nil {
			continue
		}
		name := prototypeInterfaces[wrapper.Prototype()]
		if name == "" {
			// A custom constructor can leave an element with its own prototype;
			// only native interface prototypes have an equivalent in this realm.
			continue
		}
		child.ensureHTMLInterfaceConstructor(name)
		constructor, _ := child.vm.Get(name).(*goja.Object)
		if constructor == nil {
			continue
		}
		prototype, _ := constructor.Get("prototype").(*goja.Object)
		if prototype == nil {
			continue
		}
		imported := r.importChildValue(child, prototype, make(map[*goja.Object]*goja.Object), 0)
		if importedPrototype, ok := imported.(*goja.Object); ok {
			_ = wrapper.SetPrototype(importedPrototype)
		}
	}
}

func iframeRealmIntrinsic(name string) bool {
	switch name {
	case "Object", "Function", "Array", "Number", "Boolean", "String", "Symbol", "BigInt", "Date", "Promise", "RegExp", "Error", "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError", "AggregateError", "MutationObserver", "MutationRecord":
		return true
	default:
		return false
	}
}

func (r *Runtime) iframeWindowOpenCall(frame *dom.Node, call goja.FunctionCall) goja.Value {
	urlText := "about:blank"
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) {
		if raw := strings.TrimSpace(call.Argument(0).String()); raw != "" {
			urlText = r.resolveURLString(raw)
		}
	}
	target := ""
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		target = strings.ToLower(strings.TrimSpace(call.Argument(1).String()))
	}
	switch target {
	case "_self":
		if urlText != "about:blank" {
			r.navigateIFrameLocation(frame, urlText)
		}
		return r.iframeWindow(frame)
	case "_parent", "_top":
		return r.openWindowCall(goja.FunctionCall{Arguments: []goja.Value{r.vm.ToValue(urlText), r.vm.ToValue(target)}})
	default:
		if strings.EqualFold(target, "_blank") {
			target = ""
		}
		return r.openBrowsingContext(urlText, target, r.iframeWindow(frame))
	}
}

func (r *Runtime) iframeNavigator(frame *dom.Node, child *Runtime) goja.Value {
	if r.iframeNavigators == nil {
		r.iframeNavigators = make(map[*dom.Node]*goja.Object)
	}
	if navigator := r.iframeNavigators[frame]; navigator != nil {
		return navigator
	}
	childNavigator, ok := child.vm.Get("navigator").(*goja.Object)
	if !ok {
		return goja.Undefined()
	}
	proxy := r.vm.NewProxy(r.vm.NewObject(), &goja.ProxyTrapConfig{
		Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
			value := childNavigator.Get(property)
			if property == "userActivation" {
				activation, ok := value.(*goja.Object)
				if !ok {
					return r.importChildValue(child, value, make(map[*goja.Object]*goja.Object), 0)
				}
				if r.iframeUserActivations == nil {
					r.iframeUserActivations = make(map[*dom.Node]*goja.Object)
				}
				if existing := r.iframeUserActivations[frame]; existing != nil {
					return existing
				}
				activationProxy := r.vm.NewProxy(r.vm.NewObject(), &goja.ProxyTrapConfig{
					Get: func(_ *goja.Object, member string, _ goja.Value) goja.Value {
						return r.importChildValue(child, activation.Get(member), make(map[*goja.Object]*goja.Object), 0)
					},
					Has: func(_ *goja.Object, member string) bool {
						memberValue := activation.Get(member)
						return memberValue != nil && !goja.IsUndefined(memberValue)
					},
				})
				activationObject := r.vm.ToValue(activationProxy).ToObject(r.vm)
				r.iframeUserActivations[frame] = activationObject
				return activationObject
			}
			return r.importChildValue(child, value, make(map[*goja.Object]*goja.Object), 0)
		},
		Set: func(_ *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
			return childNavigator.Set(property, child.importParentValue(r, assigned)) == nil
		},
		Has: func(_ *goja.Object, property string) bool {
			value := childNavigator.Get(property)
			return value != nil && !goja.IsUndefined(value)
		},
	})
	navigator := r.vm.ToValue(proxy).ToObject(r.vm)
	r.iframeNavigators[frame] = navigator
	return navigator
}

func (r *Runtime) iframeWindowTarget(frame *dom.Node) *goja.Object {
	_ = r.iframeWindow(frame)
	return r.iframeWindowTargets[frame]
}

func (r *Runtime) iframeForDocument(doc *dom.Document) *dom.Node {
	for frame, candidate := range r.iframeDocuments {
		if candidate == doc {
			return frame
		}
	}
	return r.iframeFormerDocuments[doc]
}

// installWindowFrameProperties exposes the child browsing contexts through
// Window.frames and its indexed properties. The accessors resolve the current
// frame tree on each read, so removing or reordering an iframe is observable.
func (r *Runtime) installWindowFrameProperties() {
	if r.domDoc == nil || r.vm == nil {
		return
	}
	frames := r.windowFrameNodes()
	win := r.vm.GlobalObject()
	_ = win.Set("frames", win)
	_ = win.DefineAccessorProperty("length", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(r.windowFrameNodes()))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	for index := range frames {
		i := index
		_ = win.DefineAccessorProperty(strconv.Itoa(i), r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			current := r.windowFrameNodes()
			if i >= len(current) {
				return goja.Undefined()
			}
			return r.iframeWindow(current[i])
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
}

func (r *Runtime) windowFrameNodes() []*dom.Node {
	var frames []*dom.Node
	if r.domDoc == nil {
		return frames
	}
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() && node.Data == "iframe" {
			frames = append(frames, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(&r.domDoc.Node)
	return frames
}

func (r *Runtime) wrapIFrameDocument(frame *dom.Node) goja.Value {
	// Creating a same-origin Document also creates its initial about:blank
	// Window and per-document custom element registry.
	_ = r.iframeWindow(frame)
	doc := r.iframeDocument(frame)
	if child := r.iframeChildRuntimes[frame]; child != nil {
		if childDocument := child.vm.Get("document"); childDocument != nil && !goja.IsUndefined(childDocument) {
			if registry := r.iframeRegistries[frame]; registry != nil {
				registry.document = doc
			}
			return r.wrapDetachedDocumentWithRegistry(doc, r.iframeRegistries[frame])
		}
	}
	if registry := r.iframeRegistries[frame]; registry != nil {
		registry.document = doc
		return r.wrapDetachedDocumentWithRegistry(doc, registry)
	}
	return r.wrapDetachedDocument(doc)
}

// setIFrameDocument installs a new active Document and the per-document
// CustomElementRegistry exposed by its browsing context's stable WindowProxy.
// Keep the prior association for wrappers that still reference the old
// Document; navigation replaces the active registry without changing Window.
func (r *Runtime) setIFrameDocument(frame *dom.Node, doc *dom.Document) {
	if frame == nil || doc == nil {
		return
	}
	if r.iframeDocuments[frame] == doc {
		return
	}
	if r.iframeRegistries[frame] == nil {
		_ = r.iframeWindow(frame)
	}
	if previous := r.iframeDocuments[frame]; previous != nil {
		if r.iframeFormerDocuments == nil {
			r.iframeFormerDocuments = make(map[*dom.Document]*dom.Node)
		}
		r.iframeFormerDocuments[previous] = frame
		if r.customElementDocumentRegistries == nil {
			r.customElementDocumentRegistries = make(map[*dom.Document]*CustomElementRegistry)
		}
		r.customElementDocumentRegistries[previous] = r.iframeRegistries[frame]
	}
	constructor, _ := r.vm.Get("CustomElementRegistry").(*goja.Object)
	registry, customElements := r.newCustomElementRegistry(constructor)
	registry.document = doc
	delete(r.iframeFormerDocuments, doc)
	r.iframeDocuments[frame] = doc
	r.iframeRegistries[frame] = registry
	if target := r.iframeWindowTargets[frame]; target != nil {
		_ = target.Set("customElements", customElements)
	}
	if child := r.iframeChildRuntimes[frame]; child != nil && child.domDoc == doc {
		r.syncIFrameGlobals(frame, child, r.iframeGlobalBaseline[frame])
	}
}

func (r *Runtime) parseSrcdoc(frame *dom.Node, markup string) (*dom.Document, error) {
	var child *Runtime
	var childErr error
	var baseline map[string]goja.Value
	limits := dom.ParseLimits{Nodes: 100000, Depth: 128, Attributes: 100000, AttributeBytes: 1 << 20}
	scriptingEnabled := !r.opts.ScriptingDisabled && iframeAllowsScripts(frame)
	if !scriptingEnabled {
		return dom.ParseBoundedWithHandlersAndScripting(markup, limits, false, nil, nil, nil)
	}
	doc, err := dom.ParseBoundedWithParserCallbacksAndScripting(markup, limits, true,
		func(node *dom.Node, attrs []dom.Attribute) *dom.Node {
			if child != nil {
				return child.UpgradeParserElementWithAttributes(node, attrs)
			}
			return node
		},
		func(node *dom.Node, attr dom.Attribute) {
			if child != nil {
				child.ParserElementAttributeChanged(node, attr)
			}
		},
		func(node *dom.Node) {
			if child != nil {
				child.ParserNodeInserted(node)
			}
		},
		func(doc *dom.Document, script *dom.Node) {
			if child == nil {
				child, baseline, childErr = r.newIFrameChildRuntime(frame, doc, r.opts.URL)
				if childErr != nil {
					return
				}
				if win := r.iframeWindows[frame]; win != nil {
					_ = win.Set("document", r.wrapIFrameDocument(frame))
				}
			}
			child.RefreshDocumentTreeReferences()
			if !classicScript(script) {
				r.syncIFrameGlobals(frame, child, baseline)
				return
			}
			src := strings.TrimSpace(script.GetAttribute("src"))
			if src != "" && (script.HasAttribute("async") || script.HasAttribute("defer")) {
				r.syncIFrameGlobals(frame, child, baseline)
				return
			}
			source, name := script.TextContent(), "about:srcdoc"
			if src != "" {
				if child.iframeScriptFetcher == nil || (child.opts.CSP != nil && !child.opts.CSP.AllowsScript(src, child.opts.URL)) {
					r.syncIFrameGlobals(frame, child, baseline)
					return
				}
				name = child.resolveURLString(src)
				data, fetchErr := child.iframeScriptFetcher(child.opts.URL, name)
				if fetchErr != nil {
					r.syncIFrameGlobals(frame, child, baseline)
					return
				}
				source = string(data)
			}
			if source != "" {
				_ = child.RunParserScript(source, name)
			}
			r.syncIFrameGlobals(frame, child, baseline)
		},
	)
	if err != nil {
		if child != nil {
			_ = child.Close()
		}
		return nil, err
	}
	if childErr != nil {
		return nil, childErr
	}
	if child != nil {
		child.associateNullRegistryAttributesInDocument(doc)
	} else {
		r.associateNullRegistryAttributesInDocument(doc)
	}
	if child != nil {
		child.RefreshDocumentTreeReferences()
		if child.iframeModuleRunner != nil {
			child.iframeModuleRunner(child, doc, child.opts.URL)
		}
		child.FireDocumentLifecycle()
		r.iframeChildRuntimes[frame] = child
		r.iframeGlobalBaseline[frame] = baseline
		r.installPendingIFrameWindowListeners(frame, child)
		// The caller installs doc as the frame's active Document immediately
		// after parseSrcdoc returns. setIFrameDocument then synchronizes globals
		// with the final document mapping in place, so child Node values nested
		// in exported objects share identity with contentDocument wrappers.
	}
	return doc, nil
}

func (r *Runtime) newIFrameChildRuntime(frame *dom.Node, doc *dom.Document, childURL string) (*Runtime, map[string]goja.Value, error) {
	opts := r.opts
	opts.DOM = doc
	if childURL != "" {
		opts.URL = childURL
	}
	opts.ScriptingDisabled = false
	opts.OnMutation = nil
	opts.OnFocusChange = nil
	opts.OnFocusRestore = nil
	opts.ActiveElement = nil
	opts.ImageLoader = nil
	child, err := New(opts)
	if err != nil {
		return nil, nil, err
	}
	if r.testUserActivationHook {
		child.EnableTestUserActivationHook()
	}
	child.iframeLoader = r.iframeLoader
	child.iframeFormSubmitter = r.iframeFormSubmitter
	child.SetIFrameScriptFetcher(r.iframeScriptFetcher)
	child.SetDynamicScriptFetcher(r.dynamicScriptFetcher)
	child.SetDynamicScriptLimit(r.dynamicScriptLimit)
	child.SetIFrameModuleRunner(r.iframeModuleRunner)
	baseline := make(map[string]goja.Value)
	for _, key := range child.vm.GlobalObject().Keys() {
		baseline[key] = child.vm.GlobalObject().Get(key)
	}
	parent := r.vm.GlobalObject()
	proxyTarget := child.vm.NewObject()
	child.iframeParent = r
	child.iframeParentFrame = frame
	child.iframeParentObjects = make(map[*goja.Object]goja.Value)
	for _, key := range parent.Keys() {
		k := key
		get := child.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return child.importParentValue(r, parent.Get(k))
		})
		set := child.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) != 0 {
				_ = parent.Set(k, r.importChildValue(child, call.Argument(0), make(map[*goja.Object]*goja.Object), 0))
			}
			return goja.Undefined()
		})
		_ = proxyTarget.DefineAccessorProperty(k, get, set, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = proxyTarget.DefineDataProperty("postMessage", child.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return child.postMessageAcross(call, r, r.iframeWindow(frame))
	}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE)
	parentProxy := child.vm.NewProxy(proxyTarget, &goja.ProxyTrapConfig{
		Get: func(target *goja.Object, property string, _ goja.Value) goja.Value {
			if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
				return value
			}
			return child.importParentValue(r, parent.Get(property))
		},
		Set: func(target *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
			if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
				return target.Set(property, assigned) == nil
			}
			return parent.Set(property, r.importChildValue(child, assigned, make(map[*goja.Object]*goja.Object), 0)) == nil
		},
		Has: func(target *goja.Object, property string) bool {
			if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
				return true
			}
			value := parent.Get(property)
			return value != nil && !goja.IsUndefined(value)
		},
	})
	child.iframeParentWindow = child.vm.ToValue(parentProxy).ToObject(child.vm)
	childGlobal := child.vm.GlobalObject()
	_ = childGlobal.Set("parent", child.iframeParentWindow)
	top := child.iframeParentWindow.ToObject(child.vm).Get("top")
	if top == nil || goja.IsUndefined(top) {
		top = child.iframeParentWindow
	}
	_ = childGlobal.Set("top", top)
	_ = childGlobal.Set("frameElement", child.wrapNode(frame))
	return child, baseline, nil
}

func (r *Runtime) installExternalIFrameRuntime(frame *dom.Node, doc *dom.Document, childURL string) {
	if r.opts.ScriptingDisabled || !iframeAllowsScripts(frame) {
		return
	}
	delete(r.iframeNavigators, frame)
	delete(r.iframeUserActivations, frame)
	if previous := r.iframeChildRuntimes[frame]; previous != nil {
		_ = previous.Close()
		delete(r.iframeChildRuntimes, frame)
	}
	child, baseline, err := r.newIFrameChildRuntime(frame, doc, childURL)
	if err != nil {
		return
	}
	r.iframeChildRuntimes[frame] = child
	r.iframeGlobalBaseline[frame] = baseline
	contextName := frame.GetAttribute("name")
	if openedName, exists := r.iframeContextNames[frame]; exists {
		contextName = openedName
	}
	_ = child.vm.GlobalObject().Set("name", child.vm.ToValue(contextName))
	_ = child.vm.Set("close", child.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if r.iframeOpenedContexts[frame] {
			r.closeIFrameContext(frame)
		}
		return goja.Undefined()
	}))
	if r.iframeOpenedContexts[frame] {
		childWindow := child.vm.GlobalObject()
		_ = childWindow.Set("parent", childWindow)
		_ = childWindow.Set("top", childWindow)
		_ = childWindow.Set("frameElement", goja.Null())
		if opener := r.iframeContextOpeners[frame]; opener != nil && !goja.IsNull(opener) && !goja.IsUndefined(opener) {
			_ = childWindow.Set("opener", child.importParentValue(r, opener))
		} else {
			_ = childWindow.Set("opener", goja.Null())
		}
	}
	if writes := r.iframeWindowWrites[frame]; writes != nil {
		for property, value := range writes {
			if property == "window" || property == "self" || property == "document" || property == "parent" || property == "top" || property == "frameElement" || property == "location" || property == "close" {
				continue
			}
			_ = child.vm.Set(property, child.importParentValue(r, value))
		}
	}
	r.installPendingIFrameWindowListeners(frame, child)
	if doc.IsXML && doc.ParserInput != "" {
		// The host loader admits and validates the XML source before handing us
		// the document. Reparse it into the same document now that its realm and
		// custom-element registry exist, preserving XML parser pauses.
		for doc.Node.FirstChild != nil {
			doc.Node.RemoveChild(doc.Node.FirstChild)
		}
		doc.HTML, doc.Head, doc.Body, doc.DocType = nil, nil, nil, nil
		limits := dom.ParseLimits{Nodes: 100000, Depth: 128, Attributes: 100000, AttributeBytes: 1 << 20}
		parseErr := ParseXMLDocumentIntoBoundedWithParserCallbacks(doc, doc.ParserInput, doc.ContentType, limits,
			func(node *dom.Node, attrs []dom.Attribute) {
				child.UpgradeParserElementWithAttributesInDocument(node, attrs, doc)
			},
			func(node *dom.Node) {
				child.ParserNodeInsertedInDocument(node, doc)
			},
			func(_ *dom.Document, script *dom.Node) {
				child.RefreshDocumentTreeReferences()
				if !classicScript(script) || script.HasAttribute("async") || script.HasAttribute("defer") {
					return
				}
				source, name := script.TextContent(), childURL
				if src := strings.TrimSpace(script.GetAttribute("src")); src != "" {
					if child.iframeScriptFetcher == nil || (child.opts.CSP != nil && !child.opts.CSP.AllowsScript(src, child.opts.URL)) {
						return
					}
					name = child.resolveURLString(src)
					data, fetchErr := child.iframeScriptFetcher(child.opts.URL, name)
					if fetchErr != nil {
						return
					}
					source = string(data)
				}
				if source != "" {
					_ = child.RunParserScript(source, name)
					r.syncIFrameGlobals(frame, child, baseline)
				}
			},
		)
		if parseErr == nil {
			child.RefreshDocumentTreeReferences()
		} else {
			// The source was already admitted by the host parser. Keep the
			// validated document if a parser callback unexpectedly rejects it.
			return
		}
	}
	var runScripts func(*dom.Node)
	runScripts = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() && node.Data == "script" && classicScript(node) {
			src := strings.TrimSpace(node.GetAttribute("src"))
			if src == "" || (!node.HasAttribute("async") && !node.HasAttribute("defer")) {
				source, name := node.TextContent(), childURL
				if src != "" {
					name = child.resolveURLString(src)
					if child.iframeScriptFetcher == nil || (child.opts.CSP != nil && !child.opts.CSP.AllowsScript(src, child.opts.URL)) {
						source = ""
					} else if data, fetchErr := child.iframeScriptFetcher(child.opts.URL, name); fetchErr == nil {
						source = string(data)
					} else {
						source = ""
					}
				}
				if source != "" {
					_ = child.RunParserScript(source, name)
					r.syncIFrameGlobals(frame, child, baseline)
				}
			}
		}
		for childNode := node.FirstChild; childNode != nil; childNode = childNode.NextSibling {
			runScripts(childNode)
		}
	}
	runScripts(&doc.Node)
	if child.iframeModuleRunner != nil {
		child.iframeModuleRunner(child, doc, childURL)
	}
	child.RefreshDocumentTreeReferences()
	child.QueueExistingIFrames()
	child.FlushIFrameLoads()
	child.FireDocumentLifecycle()
	r.syncIFrameGlobals(frame, child, baseline)
}

func (r *Runtime) addIFrameWindowListener(frame *dom.Node, typ string, callback goja.Value, capture bool) {
	if child := r.iframeChildRuntimes[frame]; child != nil {
		r.installIFrameWindowListener(child, typ, callback, capture)
		return
	}
	if r.iframePendingEvents == nil {
		r.iframePendingEvents = make(map[*dom.Node][]iframePendingListener)
	}
	for _, listener := range r.iframePendingEvents[frame] {
		if listener.typ == typ && listener.capture == capture && listener.callback.StrictEquals(callback) {
			return
		}
	}
	r.iframePendingEvents[frame] = append(r.iframePendingEvents[frame], iframePendingListener{typ: typ, callback: callback, capture: capture})
}

func (r *Runtime) removeIFrameWindowListener(frame *dom.Node, typ string, callback goja.Value) {
	listeners := r.iframePendingEvents[frame]
	for i, listener := range listeners {
		if listener.typ == typ && listener.callback.StrictEquals(callback) {
			r.iframePendingEvents[frame] = append(listeners[:i], listeners[i+1:]...)
			break
		}
	}
	if child := r.iframeChildRuntimes[frame]; child != nil {
		childCallback := child.importParentValue(r, callback)
		fn, ok := goja.AssertFunction(child.vm.Get("removeEventListener"))
		if ok {
			_, _ = fn(child.vm.GlobalObject(), child.vm.ToValue(typ), childCallback)
		}
	}
}

func (r *Runtime) installIFrameWindowListener(child *Runtime, typ string, callback goja.Value, capture bool) {
	childCallback := child.importParentValue(r, callback)
	fn, ok := goja.AssertFunction(child.vm.Get("addEventListener"))
	if !ok {
		return
	}
	_, _ = fn(child.vm.GlobalObject(), child.vm.ToValue(typ), childCallback, child.vm.ToValue(capture))
}

func (r *Runtime) installPendingIFrameWindowListeners(frame *dom.Node, child *Runtime) {
	for _, listener := range r.iframePendingEvents[frame] {
		r.installIFrameWindowListener(child, listener.typ, listener.callback, listener.capture)
	}
	if onmessage, ok := r.iframePendingOnMsg[frame]; ok {
		_ = child.vm.Set("onmessage", child.importParentValue(r, onmessage))
	}
}

func iframeAllowsScripts(frame *dom.Node) bool {
	if frame == nil || !frame.HasAttribute("sandbox") {
		return true
	}
	for _, token := range strings.FieldsFunc(frame.GetAttribute("sandbox"), func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
	}) {
		if strings.EqualFold(token, "allow-scripts") {
			return true
		}
	}
	return false
}

func classicScript(node *dom.Node) bool {
	if node == nil {
		return false
	}
	t := strings.ToLower(strings.TrimSpace(node.GetAttribute("type")))
	switch t {
	case "", "text/javascript", "application/javascript", "application/ecmascript", "application/x-ecmascript", "application/x-javascript", "text/ecmascript", "text/jscript", "text/livescript":
		return true
	default:
		return false
	}
}

func (r *Runtime) syncIFrameGlobals(frame *dom.Node, child *Runtime, baseline map[string]goja.Value) {
	if frame == nil || child == nil {
		return
	}
	twin := r.iframeWindowTarget(frame)
	if twin == nil {
		return
	}
	// `open` is installed when the child Runtime is created, so it is part of
	// the child's baseline and the ordinary changed-global sync below skips it.
	// WindowProxy still needs to expose it to callers in the parent realm.
	_ = twin.Set("open", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		opened := child.openWindowCall(call)
		return r.importChildValue(child, opened, make(map[*goja.Object]*goja.Object), 0)
	}))
	global := child.vm.GlobalObject()
	for _, key := range global.Keys() {
		// The WindowProxy's onmessage accessor reads the child's current value
		// and forwards writes from the parent into the child. Writing a child
		// handler back through that setter would import it into the parent and
		// then re-import it into the child, wrapping MessageEvent.ports in plain
		// cross-realm proxies.
		if key == "document" || key == "window" || key == "self" || key == "parent" || key == "top" || key == "frameElement" || key == "opener" || key == "close" || key == "onmessage" || key == "event" {
			continue
		}
		value := global.Get(key)
		if original, existed := baseline[key]; existed && value.StrictEquals(original) {
			continue
		}
		_ = twin.Set(key, r.importChildValue(child, value, make(map[*goja.Object]*goja.Object), 0))
	}
	_ = twin.Set("document", r.wrapIFrameDocument(frame))
}

func (r *Runtime) importChildNode(child *Runtime, source *goja.Object, node *dom.Node, seen map[*goja.Object]*goja.Object, depth int) goja.Value {
	if child == nil || source == nil || node == nil {
		return goja.Undefined()
	}
	if wrapper := r.existingParentNodeWrapper(child, source, node); wrapper != nil {
		return wrapper
	}
	if prior := r.iframeChildObjects[source]; prior != nil {
		if r.iframeImportedChildNodes == nil {
			r.iframeImportedChildNodes = make(map[*goja.Object]*dom.Node)
		}
		r.iframeImportedChildNodes[prior] = node
		return prior
	}
	if seen == nil {
		seen = make(map[*goja.Object]*goja.Object)
	}
	if prior := seen[source]; prior != nil {
		return prior
	}
	target := r.vm.NewObject()
	proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		GetPrototypeOf: func(*goja.Object) *goja.Object {
			prototype := r.importChildValue(child, source.Prototype(), make(map[*goja.Object]*goja.Object), depth+1)
			if prototype == nil || goja.IsNull(prototype) || goja.IsUndefined(prototype) {
				return nil
			}
			return prototype.ToObject(r.vm)
		},
		Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
			if property == "ownerDocument" && node.Type != dom.NodeDocument && node.Doc != nil {
				if frame := r.iframeForDocument(node.Doc); frame != nil {
					return r.wrapIFrameDocument(frame)
				}
			}
			if property == "attributes" && node.Element() {
				// NamedNodeMap is a dynamic object whose indexed and length
				// properties are supplied by goja.DynamicObject. Copying the
				// child object through importChildValue loses those dynamic keys;
				// expose the same DOM node's live collection in this realm instead.
				return r.attributesObject(node)
			}
			return r.importChildValue(child, source.Get(property), make(map[*goja.Object]*goja.Object), depth+1)
		},
		GetSym: func(_ *goja.Object, property *goja.Symbol, _ goja.Value) goja.Value {
			return r.importChildValue(child, source.GetSymbol(property), make(map[*goja.Object]*goja.Object), depth+1)
		},
		Set: func(_ *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
			if err := source.Set(property, child.importParentValue(r, assigned)); err != nil {
				if exception, ok := err.(*goja.Exception); ok {
					panic(child.importParentValue(r, exception.Value()))
				}
				panic(r.vm.NewGoError(err))
			}
			return true
		},
		Has: func(_ *goja.Object, property string) bool {
			return source.Get(property) != nil && !goja.IsUndefined(source.Get(property))
		},
	})
	proxyObject := r.vm.ToValue(proxy).ToObject(r.vm)
	seen[source] = proxyObject
	if r.iframeChildObjects == nil {
		r.iframeChildObjects = make(map[*goja.Object]*goja.Object)
	}
	r.iframeChildObjects[source] = proxyObject
	if r.iframeImportedChildNodes == nil {
		r.iframeImportedChildNodes = make(map[*goja.Object]*dom.Node)
	}
	r.iframeImportedChildNodes[proxyObject] = node
	if child.iframeImportedParentValues == nil {
		child.iframeImportedParentValues = make(map[*goja.Object]*goja.Object)
	}
	child.iframeImportedParentValues[proxyObject] = source
	if prototype := source.Prototype(); prototype != nil {
		importedPrototype := r.importChildValue(child, prototype, seen, depth+1)
		if importedPrototype != nil && !goja.IsUndefined(importedPrototype) && !goja.IsNull(importedPrototype) {
			_ = target.SetPrototype(importedPrototype.ToObject(r.vm))
		}
	}
	for _, key := range source.Keys() {
		if key == "__nid__" || key == "nodeType" {
			_ = target.Set(key, source.Get(key))
		}
	}
	return proxyObject
}

// existingParentNodeWrapper keeps one parent-visible wrapper for a DOM node
// reached through both contentDocument and the same-origin WindowProxy. The
// wrapper's prototype is aligned to the child realm when that Runtime exists,
// and its bridged operations translate the receiver back to the child node.
func (r *Runtime) existingParentNodeWrapper(child *Runtime, source *goja.Object, node *dom.Node) goja.Value {
	if child == nil || source == nil || node == nil {
		return nil
	}
	if r.iframeForDocument(node.Doc) == nil {
		return nil
	}
	nid, exists := r.wrapCache[node]
	var wrapper *goja.Object
	if exists {
		wrapper = r.nodeWrappers[nid]
	}
	if wrapper == nil {
		// Native nodes with no per-instance JS state can use the parent
		// Runtime's wrapper. This makes `contentWindow.node.parentNode` and
		// `contentDocument.head` return the same object. Keep custom elements
		// and wrappers with own state on the source-backed proxy path so their
		// child-realm prototype and per-instance behavior remain intact.
		if child.customElementInstances[node] != nil || child.isFailedCustomElement(node) {
			return nil
		}
		if source.Prototype() != child.protoForNode(node) {
			// Script may replace a node's prototype without registering it as a
			// custom element. Preserve the source-backed proxy in that case so
			// later prototype changes remain observable through WindowProxy.
			return nil
		}
		value, ok := r.wrapNodeLocal(node).(*goja.Object)
		if !ok {
			return nil
		}
		wrapper = value
	}
	if r.iframeChildObjects == nil {
		r.iframeChildObjects = make(map[*goja.Object]*goja.Object)
	}
	r.iframeChildObjects[source] = wrapper
	if r.iframeImportedChildNodes == nil {
		r.iframeImportedChildNodes = make(map[*goja.Object]*dom.Node)
	}
	r.iframeImportedChildNodes[wrapper] = node
	if child.iframeImportedParentValues == nil {
		child.iframeImportedParentValues = make(map[*goja.Object]*goja.Object)
	}
	child.iframeImportedParentValues[wrapper] = source
	return wrapper
}

func (r *Runtime) syncImportedNodePrototype(child *Runtime, value goja.Value) {
	if child == nil || value == nil {
		return
	}
	wrapper, ok := value.(*goja.Object)
	if !ok {
		return
	}
	source := child.iframeImportedParentValues[wrapper]
	if source == nil {
		return
	}
	node := child.getNode(source)
	if node == nil || r.getNode(wrapper) != node {
		return
	}
	prototype := r.importChildValue(child, source.Prototype(), make(map[*goja.Object]*goja.Object), 0)
	if imported, ok := prototype.(*goja.Object); ok {
		_ = wrapper.SetPrototype(imported)
	}
}

func inheritedSymbolValue(object *goja.Object, property *goja.Symbol) goja.Value {
	for current := object; current != nil; current = current.Prototype() {
		if value := current.GetSymbol(property); value != nil && !goja.IsUndefined(value) {
			return value
		}
	}
	return goja.Undefined()
}

// importedObjectRealm returns the Window realm that originally owns object.
// Imported iframe wrappers retain this separately from the realm of methods
// stored on the object, which may themselves come from another frame.
func (r *Runtime) importedObjectRealm(object *goja.Object) *Runtime {
	if object == nil {
		return r
	}
	if realm := r.iframeImportedObjectRealms[object]; realm != nil {
		return realm
	}
	if realm := r.iframeImportedFunctionRealms[object]; realm != nil {
		return realm
	}
	return r
}

func (r *Runtime) rememberImportedObjectRealm(imported, source *goja.Object, sourceRuntime *Runtime) {
	if imported == nil || source == nil || sourceRuntime == nil {
		return
	}
	if r.iframeImportedObjectRealms == nil {
		r.iframeImportedObjectRealms = make(map[*goja.Object]*Runtime)
	}
	r.iframeImportedObjectRealms[imported] = sourceRuntime.importedObjectRealm(source)
}

func (r *Runtime) withEventListenerInvocation(run func()) {
	root := r
	for root.iframeParent != nil {
		root = root.iframeParent
	}
	root.iframeEventListenerInvocationDepth++
	defer func() { root.iframeEventListenerInvocationDepth-- }()
	run()
}

// importRuntimeValue moves a value through the shared browsing-context tree,
// preserving the identity bridges installed at each WindowProxy boundary.
func importRuntimeValue(source, destination *Runtime, value goja.Value) goja.Value {
	if source == nil || destination == nil || source == destination || value == nil {
		return value
	}
	ancestors := func(runtime *Runtime) []*Runtime {
		var result []*Runtime
		for current := runtime; current != nil; current = current.iframeParent {
			result = append(result, current)
		}
		return result
	}
	sourcePath := ancestors(source)
	destinationPath := ancestors(destination)
	commonIndex, destinationCommonIndex := -1, -1
	for i, current := range sourcePath {
		for j, candidate := range destinationPath {
			if current == candidate {
				commonIndex, destinationCommonIndex = i, j
				break
			}
		}
		if commonIndex >= 0 {
			break
		}
	}
	if commonIndex < 0 {
		return value
	}
	currentValue := value
	for i := 0; i < commonIndex; i++ {
		child, parent := sourcePath[i], sourcePath[i+1]
		currentValue = parent.importChildValue(child, currentValue, make(map[*goja.Object]*goja.Object), 0)
	}
	for i := destinationCommonIndex - 1; i >= 0; i-- {
		child, parent := destinationPath[i], destinationPath[i+1]
		currentValue = child.importParentValue(parent, currentValue)
	}
	return currentValue
}

func (r *Runtime) importChildValue(child *Runtime, value goja.Value, seen map[*goja.Object]*goja.Object, depth int) goja.Value {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return value
	}
	if object, ok := value.(*goja.Object); ok {
		if parentValue, exists := child.iframeParentObjects[object]; exists {
			return parentValue
		}
		if _, isCustomElementRegistry := child.customElementRegistryObjects[object]; isCustomElementRegistry {
			if proxy := r.iframeChildObjects[object]; proxy != nil {
				return proxy
			}
			proxy := r.vm.NewProxy(r.vm.NewObject(), &goja.ProxyTrapConfig{
				Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
					return r.importChildValue(child, object.Get(property), make(map[*goja.Object]*goja.Object), 0)
				},
				Set: func(_ *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
					return object.Set(property, child.importParentValue(r, assigned)) == nil
				},
				Has: func(_ *goja.Object, property string) bool {
					return object.Get(property) != nil && !goja.IsUndefined(object.Get(property))
				},
			})
			proxyValue := r.vm.ToValue(proxy).ToObject(r.vm)
			if r.iframeChildObjects == nil {
				r.iframeChildObjects = make(map[*goja.Object]*goja.Object)
			}
			r.iframeChildObjects[object] = proxyValue
			if child.iframeParentObjects == nil {
				child.iframeParentObjects = make(map[*goja.Object]goja.Value)
			}
			child.iframeParentObjects[proxyValue] = object
			return proxyValue
		}
		if node := child.getNode(value); node != nil {
			if wrapper := r.existingParentNodeWrapper(child, object, node); wrapper != nil {
				return wrapper
			}
		}
		if existing := r.iframeChildObjects[object]; existing != nil {
			return existing
		}
		if broadcastChannelForTarget(child, object) != nil {
			return r.importChildBroadcastChannel(child, object, seen, depth)
		}
	}
	if value.StrictEquals(child.vm.GlobalObject()) && child.iframeParent == r {
		return r.iframeWindow(child.iframeParentFrame)
	}
	if object, ok := value.(*goja.Object); ok {
		if node := child.getNode(value); node != nil {
			if node.Type == dom.NodeDocument && node.Doc != nil {
				if documentURL := child.detachedDocumentURLs[node.Doc]; documentURL != nil {
					if r.detachedDocumentURLs == nil {
						r.detachedDocumentURLs = make(map[*dom.Document]*url.URL)
					}
					copyURL := *documentURL
					r.detachedDocumentURLs[node.Doc] = &copyURL
				}
			}
			return r.importChildNode(child, object, node, seen, depth)
		}
		// Same-origin WindowProxy access preserves an exception's source realm.
		// In particular, an exception thrown by a child document must remain an
		// instance of that document's DOMException, not the receiving window's.
		if code := object.Get("code"); code != nil && !goja.IsUndefined(code) && code.ToInteger() != 0 {
			target := r.vm.NewObject()
			if prototype := r.importChildValue(child, object.Prototype(), seen, depth+1); prototype != nil && !goja.IsUndefined(prototype) && !goja.IsNull(prototype) {
				_ = target.SetPrototype(prototype.ToObject(r.vm))
			}
			proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
				Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
					return r.importChildValue(child, object.Get(property), make(map[*goja.Object]*goja.Object), 0)
				},
				Has: func(_ *goja.Object, property string) bool {
					return object.Get(property) != nil && !goja.IsUndefined(object.Get(property))
				},
			})
			proxyValue := r.vm.ToValue(proxy).ToObject(r.vm)
			if r.iframeChildObjects == nil {
				r.iframeChildObjects = make(map[*goja.Object]*goja.Object)
			}
			r.iframeChildObjects[object] = proxyValue
			return proxyValue
		}
		// Error constructors also have a `name` and a `message` property. Only
		// convert Error instances here; callable Error/TypeError/etc. constructors
		// must reach the function bridge below so WindowProxy keeps their
		// [[Call]]/[[Construct]] behavior.
		if _, callable := goja.AssertFunction(value); !callable && object.ClassName() == "Error" {
			if errorValue := object.Get("name"); errorValue != nil && !goja.IsUndefined(errorValue) {
				switch errorValue.String() {
				case "Error", "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError", "AggregateError":
					if message := object.Get("message"); message != nil && !goja.IsUndefined(message) {
						target := r.vm.NewObject()
						if prototype := r.importChildValue(child, object.Prototype(), seen, depth+1); prototype != nil && !goja.IsUndefined(prototype) && !goja.IsNull(prototype) {
							_ = target.SetPrototype(prototype.ToObject(r.vm))
						}
						proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
							Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
								return r.importChildValue(child, object.Get(property), make(map[*goja.Object]*goja.Object), 0)
							},
							Has: func(_ *goja.Object, property string) bool {
								return object.Get(property) != nil && !goja.IsUndefined(object.Get(property))
							},
						})
						proxyValue := r.vm.ToValue(proxy).ToObject(r.vm)
						if r.iframeChildObjects == nil {
							r.iframeChildObjects = make(map[*goja.Object]*goja.Object)
						}
						r.iframeChildObjects[object] = proxyValue
						return proxyValue
					}
				}
			}
		}
		if object.ClassName() == "Array" {
			if prior := seen[object]; prior != nil {
				return prior
			}
			array := r.vm.NewArray()
			seen[object] = array
			r.importChildOwnProperties(child, object, array, seen, depth+1)
			if length := object.Get("length"); length != nil && !goja.IsUndefined(length) {
				_ = array.Set("length", length.ToInteger())
			}
			return array
		}
		if getRange(object) != nil {
			if proxy := r.iframeChildObjects[object]; proxy != nil {
				return proxy
			}
			target := r.vm.NewObject()
			if constructor, ok := r.vm.Get("Range").(*goja.Object); ok {
				if prototype, ok := constructor.Get("prototype").(*goja.Object); ok {
					_ = target.SetPrototype(prototype)
				}
			}
			proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
				Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
					return r.importChildValue(child, object.Get(property), make(map[*goja.Object]*goja.Object), 0)
				},
				Set: func(_ *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
					return object.Set(property, child.importParentValue(r, assigned)) == nil
				},
				Has: func(_ *goja.Object, property string) bool {
					return object.Get(property) != nil && !goja.IsUndefined(object.Get(property))
				},
			})
			proxyValue := r.vm.ToValue(proxy).ToObject(r.vm)
			if r.iframeChildObjects == nil {
				r.iframeChildObjects = make(map[*goja.Object]*goja.Object)
			}
			r.iframeChildObjects[object] = proxyValue
			return proxyValue
		}
		if fn, ok := goja.AssertFunction(value); ok {
			if prior := seen[object]; prior != nil {
				return prior
			}
			var bridgedFunction *goja.Object
			invoke := func(this goja.Value, arguments []goja.Value) goja.Value {
				previousEvent := child.legacyWindowEvent
				inheritedEvent := r.legacyWindowEvent
				if r.iframeEventContext != nil && !goja.IsUndefined(r.iframeEventContext) {
					inheritedEvent = r.iframeEventContext
				}
				if inheritedEvent == nil {
					child.legacyWindowEvent = goja.Undefined()
				} else {
					child.legacyWindowEvent = child.importParentValue(r, inheritedEvent)
				}
				defer func() { child.legacyWindowEvent = previousEvent }()
				if child.iframeParent == r && child.iframeParentFrame != nil {
					for key, assigned := range r.iframeWindowWrites[child.iframeParentFrame] {
						childValue := child.importParentValue(r, assigned)
						if err := child.vm.Set(key, childValue); err != nil {
							panic(r.vm.NewGoError(err))
						}
						if childWindow, ok := child.vm.Get("window").(*goja.Object); ok {
							_ = childWindow.Set(key, childValue)
						}
					}
				}
				args := make([]goja.Value, len(arguments))
				for i, arg := range arguments {
					args[i] = child.importParentValue(r, arg)
				}
				childThis := child.importParentValue(r, this)
				result, err := fn(childThis, args...)
				if err != nil {
					if sourceRealm := r.iframeAccessorFunctions[bridgedFunction]; sourceRealm != nil {
						panic(r.importCrossRealmException(sourceRealm, err))
					}
					errorRealm := child
					if bridgedFunction != nil {
						if sourceRealm := r.iframeImportedFunctionRealms[bridgedFunction]; sourceRealm != nil {
							errorRealm = sourceRealm
						}
					}
					if r.iframeEventListenerInvocationDepth > 0 {
						if exception, ok := err.(*goja.Exception); ok {
							panic(importRuntimeValue(errorRealm, r, exception.Value()))
						}
						panic(r.vm.NewGoError(err))
					}
					errorRealm.reportWindowException(err)
					return goja.Undefined()
				}
				// A child-realm operation may change a DOM wrapper's prototype.
				// When both access paths share the parent-visible wrapper, mirror
				// that change before returning from the WindowProxy call.
				for _, argument := range arguments {
					r.syncImportedNodePrototype(child, argument)
				}
				r.syncImportedNodePrototype(child, this)
				if child.iframeParent == r && child.iframeParentFrame != nil {
					r.syncIFrameGlobals(child.iframeParentFrame, child, r.iframeGlobalBaseline[child.iframeParentFrame])
				}
				return r.importChildValue(child, result, make(map[*goja.Object]*goja.Object), 0)
			}
			// A WindowProxy import must preserve both [[Call]] and [[Construct]].
			// In particular, Function-created constructors are valid arguments to
			// CustomElementRegistry.define(). A callable and constructible native
			// Function is used only as the proxy target; all operations execute in
			// the source realm and values cross through the existing import bridge.
			functionConstructor, ok := goja.AssertConstructor(r.vm.Get("Function"))
			if !ok {
				return goja.Undefined()
			}
			target, err := functionConstructor(nil, r.vm.ToValue("return function(){}"))
			if err != nil {
				return goja.Undefined()
			}
			proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
				GetPrototypeOf: func(*goja.Object) *goja.Object {
					sourcePrototype := object.Prototype()
					if sourcePrototype == nil {
						return nil
					}
					imported := r.importChildValue(child, sourcePrototype, make(map[*goja.Object]*goja.Object), 0)
					prototype, _ := imported.(*goja.Object)
					return prototype
				},
				Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
					if property == "prototype" {
						// Keep the proxy result identical to its own fixed target
						// property. The imported source prototype was installed when
						// the bridge was created; re-importing it can allocate a
						// second wrapper and violate Proxy [[Get]] invariants.
						return target.Get(property)
					}
					// Read other function properties through the source realm while
					// keeping the restricted legacy properties pinned to the target.
					if property == "arguments" || property == "caller" {
						return target.Get(property)
					}
					return r.importChildValue(child, object.Get(property), make(map[*goja.Object]*goja.Object), 0)
				},
				GetSym: func(_ *goja.Object, property *goja.Symbol, _ goja.Value) goja.Value {
					if property != goja.SymHasInstance {
						return r.importChildValue(child, inheritedSymbolValue(object, property), make(map[*goja.Object]*goja.Object), 0)
					}
					childFunctionPrototype := child.vm.Get("Function").ToObject(child.vm).Get("prototype").ToObject(child.vm)
					hasInstance := inheritedSymbolValue(object, property)
					if !hasInstance.StrictEquals(childFunctionPrototype.GetSymbol(property)) {
						return r.importChildValue(child, hasInstance, make(map[*goja.Object]*goja.Object), 0)
					}
					return r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
						constructor, ok := call.This.(*goja.Object)
						if !ok {
							return r.vm.ToValue(false)
						}
						prototype, ok := constructor.Get("prototype").(*goja.Object)
						if !ok {
							panic(r.vm.NewTypeError("Function has non-object prototype in instanceof check"))
						}
						instance, ok := call.Argument(0).(*goja.Object)
						if !ok {
							return r.vm.ToValue(false)
						}
						for current := instance.Prototype(); current != nil; current = current.Prototype() {
							if current == prototype {
								return r.vm.ToValue(true)
							}
						}
						return r.vm.ToValue(false)
					})
				},
				Apply: func(_ *goja.Object, this goja.Value, arguments []goja.Value) goja.Value {
					return invoke(this, arguments)
				},
				Set: func(_ *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
					if property == "prototype" {
						childValue := child.importParentValue(r, assigned)
						if err := object.Set(property, childValue); err != nil {
							return false
						}
					}
					return target.Set(property, assigned) == nil
				},
				Construct: func(_ *goja.Object, arguments []goja.Value, newTarget *goja.Object) *goja.Object {
					constructor, ok := goja.AssertConstructor(value)
					if !ok {
						panic(r.vm.NewTypeError("Cross-realm value is not a constructor"))
					}
					args := make([]goja.Value, len(arguments))
					for i, arg := range arguments {
						args[i] = child.importParentValue(r, arg)
					}
					importedNewTarget := child.importParentNewTarget(r, newTarget)
					instance, err := constructor(importedNewTarget, args...)
					if err != nil {
						if len(r.customConstructionCrossRealmException) != 0 {
							index := len(r.customConstructionCrossRealmException) - 1
							r.customConstructionCrossRealmException[index] = true
							if index < len(r.customConstructionCrossRealmErrors) {
								r.customConstructionCrossRealmErrors[index] = err
							}
							if index < len(r.customConstructionCrossRealmErrorRuntimes) {
								r.customConstructionCrossRealmErrorRuntimes[index] = child
							}
						} else {
							child.reportWindowException(err)
						}
						return child.vm.NewObject()
					}
					imported := r.importChildValue(child, instance, make(map[*goja.Object]*goja.Object), 0)
					return imported.ToObject(r.vm)
				},
			})
			proxyValue := r.vm.ToValue(proxy)
			if proxyObject, ok := proxyValue.(*goja.Object); ok {
				bridgedFunction = proxyObject
				seen[object] = proxyObject
				if r.iframeChildObjects == nil {
					r.iframeChildObjects = make(map[*goja.Object]*goja.Object)
				}
				r.iframeChildObjects[object] = proxyObject
				if child.iframeImportedParentValues == nil {
					child.iframeImportedParentValues = make(map[*goja.Object]*goja.Object)
				}
				child.iframeImportedParentValues[proxyObject] = object
				prototype := r.importChildValue(child, object.Get("prototype"), seen, depth+1)
				if err := target.Set("prototype", prototype); err != nil {
					return goja.Undefined()
				}
				if r.iframeImportedFunctionRealms == nil {
					r.iframeImportedFunctionRealms = make(map[*goja.Object]*Runtime)
				}
				realm := child.iframeImportedFunctionRealms[object]
				if realm == nil {
					realm = child
				}
				r.iframeImportedFunctionRealms[proxyObject] = realm
			}
			return proxyValue
		}
		if depth >= 8 {
			return goja.Undefined()
		}
		if prior := seen[object]; prior != nil {
			return prior
		}
		copy := r.vm.NewObject()
		seen[object] = copy
		r.rememberImportedObjectRealm(copy, object, child)
		if r.iframeChildObjects == nil {
			r.iframeChildObjects = make(map[*goja.Object]*goja.Object)
		}
		r.iframeChildObjects[object] = copy
		if child.iframeImportedParentValues == nil {
			child.iframeImportedParentValues = make(map[*goja.Object]*goja.Object)
		}
		child.iframeImportedParentValues[copy] = object
		if sourcePrototype := object.Prototype(); sourcePrototype != nil {
			importedPrototype := r.importChildValue(child, sourcePrototype, seen, depth+1)
			if prototype, ok := importedPrototype.(*goja.Object); ok {
				_ = copy.SetPrototype(prototype)
			}
		}
		r.importChildOwnProperties(child, object, copy, seen, depth+1)
		// Error.name is inherited from the realm's Error prototype and is
		// therefore absent from Object.keys(). Preserve the standard error
		// identity when crossing a WindowProxy boundary.
		if object.ClassName() == "Error" {
			name := object.Get("name")
			if name != nil && !goja.IsUndefined(name) {
				switch name.String() {
				case "Error", "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError", "AggregateError":
					_ = copy.Set("name", name.String())
					if message := object.Get("message"); message != nil && !goja.IsUndefined(message) {
						_ = copy.Set("message", message.String())
					}
				}
			}
		}
		return copy
	}
	return r.vm.ToValue(value.Export())
}

func (r *Runtime) importChildBroadcastChannel(child *Runtime, source *goja.Object, seen map[*goja.Object]*goja.Object, depth int) goja.Value {
	if child == nil || source == nil || depth > 8 {
		return goja.Undefined()
	}
	if prior := r.iframeChildObjects[source]; prior != nil {
		return prior
	}
	if seen == nil {
		seen = make(map[*goja.Object]*goja.Object)
	}
	if prior := seen[source]; prior != nil {
		return prior
	}
	target := r.vm.NewObject()
	proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		GetPrototypeOf: func(*goja.Object) *goja.Object {
			prototype := r.importChildValue(child, source.Prototype(), make(map[*goja.Object]*goja.Object), depth+1)
			if prototype == nil || goja.IsNull(prototype) || goja.IsUndefined(prototype) {
				return nil
			}
			return prototype.ToObject(r.vm)
		},
		Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
			return r.importChildValue(child, source.Get(property), make(map[*goja.Object]*goja.Object), depth+1)
		},
		GetSym: func(_ *goja.Object, property *goja.Symbol, _ goja.Value) goja.Value {
			return r.importChildValue(child, source.GetSymbol(property), make(map[*goja.Object]*goja.Object), depth+1)
		},
		Set: func(_ *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
			return source.Set(property, child.importParentValue(r, assigned)) == nil
		},
		SetSym: func(_ *goja.Object, property *goja.Symbol, assigned goja.Value, _ goja.Value) bool {
			return source.SetSymbol(property, child.importParentValue(r, assigned)) == nil
		},
		Has: func(_ *goja.Object, property string) bool {
			value := source.Get(property)
			return value != nil && !goja.IsUndefined(value)
		},
		HasSym: func(_ *goja.Object, property *goja.Symbol) bool {
			value := source.GetSymbol(property)
			return value != nil && !goja.IsUndefined(value)
		},
	})
	proxyObject := r.vm.ToValue(proxy).ToObject(r.vm)
	seen[source] = proxyObject
	if r.iframeChildObjects == nil {
		r.iframeChildObjects = make(map[*goja.Object]*goja.Object)
	}
	r.iframeChildObjects[source] = proxyObject
	if child.iframeImportedParentValues == nil {
		child.iframeImportedParentValues = make(map[*goja.Object]*goja.Object)
	}
	child.iframeImportedParentValues[proxyObject] = source
	return proxyObject
}

func (r *Runtime) importChildAccessorFunction(child *Runtime, value goja.Value, seen map[*goja.Object]*goja.Object, depth int) goja.Value {
	imported := r.importChildValue(child, value, seen, depth)
	if object, ok := imported.(*goja.Object); ok {
		if _, callable := goja.AssertFunction(object); !callable {
			return imported
		}
		if r.iframeAccessorFunctions == nil {
			r.iframeAccessorFunctions = make(map[*goja.Object]*Runtime)
		}
		r.iframeAccessorFunctions[object] = child
	}
	return imported
}

// importChildOwnProperties preserves property descriptors without evaluating
// source getters. This includes non-enumerable DOM IDL attributes on interface
// prototypes; copying only Object.keys() drops members such as Node.childNodes
// from iframe-owned elements.
func (r *Runtime) importChildOwnProperties(child *Runtime, source, target *goja.Object, seen map[*goja.Object]*goja.Object, depth int) {
	if child == nil || source == nil || target == nil {
		return
	}
	objectConstructor := child.vm.Get("Object").ToObject(child.vm)
	getDescriptor, ok := goja.AssertFunction(objectConstructor.Get("getOwnPropertyDescriptor"))
	if !ok {
		return
	}
	copyProperty := func(sourceKey goja.Value, targetKey goja.Value, targetSymbol *goja.Symbol) {
		descriptorValue, err := getDescriptor(goja.Undefined(), source, sourceKey)
		if err != nil || goja.IsUndefined(descriptorValue) || goja.IsNull(descriptorValue) {
			return
		}
		sourceDescriptor := descriptorValue.ToObject(child.vm)
		descriptor := goja.PropertyDescriptor{}
		if value := sourceDescriptor.Get("get"); value != nil && !goja.IsUndefined(value) {
			descriptor.Getter = r.importChildAccessorFunction(child, value, seen, depth+1)
		}
		if value := sourceDescriptor.Get("set"); value != nil && !goja.IsUndefined(value) {
			descriptor.Setter = r.importChildAccessorFunction(child, value, seen, depth+1)
		}
		if descriptor.Getter == nil && descriptor.Setter == nil {
			value := sourceDescriptor.Get("value")
			if value == nil {
				value = goja.Undefined()
			}
			descriptor.Value = r.importChildValue(child, value, seen, depth+1)
		}
		if value := sourceDescriptor.Get("writable"); value != nil && !goja.IsUndefined(value) {
			descriptor.Writable = goja.ToFlag(value.ToBoolean())
		}
		if value := sourceDescriptor.Get("enumerable"); value != nil && !goja.IsUndefined(value) {
			descriptor.Enumerable = goja.ToFlag(value.ToBoolean())
		}
		if value := sourceDescriptor.Get("configurable"); value != nil && !goja.IsUndefined(value) {
			descriptor.Configurable = goja.ToFlag(value.ToBoolean())
		}
		if targetSymbol != nil {
			r.defineOwnSymbolProperty(target, targetSymbol, descriptor)
		} else if targetKey != nil {
			r.defineOwnProperty(target, targetKey.String(), descriptor)
		}
	}
	for _, key := range source.GetOwnPropertyNames() {
		name := child.vm.ToValue(key)
		copyProperty(name, r.vm.ToValue(key), nil)
	}
	// Live collection proxies expose indexed own keys through a custom ownKeys
	// trap that omits their non-enumerable length property. Preserve that
	// descriptor explicitly for them and arrays.
	copyProperty(child.vm.ToValue("length"), r.vm.ToValue("length"), nil)
	// Preserve well-known symbols used by platform objects and collections.
	// Symbols from separate realms are distinct objects, but the standard's
	// well-known symbols are shared by name across realms.
	childSymbols := child.vm.Get("Symbol").ToObject(child.vm)
	parentSymbols := r.vm.Get("Symbol").ToObject(r.vm)
	for _, symbol := range source.Symbols() {
		var targetSymbol *goja.Symbol
		for _, name := range []string{"iterator", "asyncIterator", "toStringTag", "hasInstance", "isConcatSpreadable", "match", "matchAll", "replace", "search", "species", "split", "toPrimitive", "unscopables"} {
			childSymbol := childSymbols.Get(name)
			if childSymbol != nil && childSymbol.StrictEquals(symbol) {
				targetSymbol, _ = parentSymbols.Get(name).(*goja.Symbol)
				break
			}
		}
		if targetSymbol != nil {
			copyProperty(child.vm.ToValue(symbol), nil, targetSymbol)
		}
	}
}

func (child *Runtime) importParentValue(parent *Runtime, value goja.Value) goja.Value {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return value
	}
	// A proxy imported from this child carries the child's __nid__ property.
	// Resolve that established bridge before consulting the parent's node ID
	// map, where the same integer may name an unrelated node.
	if object, ok := value.(*goja.Object); ok {
		if imported := child.iframeImportedParentValues[object]; imported != nil {
			return imported
		}
	}
	var parentDocument *dom.Document
	if node := parent.getNode(value); node != nil {
		if node.Type != dom.NodeDocument || node.Doc == nil {
			return child.wrapNode(node)
		}
		parentDocument = node.Doc
	}
	if child.iframeParent == parent && child.iframeParentFrame != nil {
		if window := parent.iframeWindows[child.iframeParentFrame]; window != nil && value.StrictEquals(window) {
			return child.vm.GlobalObject()
		}
	}
	if value.StrictEquals(parent.vm.GlobalObject()) && child.iframeParent == parent && child.iframeParentWindow != nil {
		return child.iframeParentWindow
	}
	if source, ok := value.(*goja.Object); ok {
		// A function imported from a sibling iframe is represented in the
		// parent realm by a callable proxy. Re-import it through the generic
		// constructor bridge would recursively wrap that proxy's synthetic
		// Function prototype, which can fail before the receiving API is called.
		// Preserve the original callback realm and bridge invocation values via
		// the shared browsing-context tree instead.
		if _, callable := goja.AssertFunction(source); callable {
			if sourceRealm, original := findImportedFunctionSource(parent, source); sourceRealm != nil {
				if _, originalCallable := goja.AssertFunction(original); originalCallable {
					if sourceRealm == child {
						return original
					}
					return child.importSiblingFunction(sourceRealm, original)
				}
			}
		}
		if _, constructible := goja.AssertConstructor(source); constructible {
			return child.importParentConstructor(parent, source)
		}
	}
	if fn, ok := goja.AssertFunction(value); ok {
		invoke := func(this goja.Value, arguments []goja.Value) goja.Value {
			previousContext := parent.iframeEventContext
			previousLegacyEvent := parent.legacyWindowEvent
			preserveErrorEvent := false
			eventContext := child.legacyWindowEvent
			if child.iframeEventContext != nil && !goja.IsUndefined(child.iframeEventContext) {
				eventContext = child.iframeEventContext
			}
			if eventContext != nil && !goja.IsUndefined(eventContext) {
				importedEvent := parent.importChildValue(child, eventContext, make(map[*goja.Object]*goja.Object), 0)
				parent.iframeEventContext = importedEvent
				parent.legacyWindowEvent = importedEvent
				if importedObject, ok := importedEvent.(*goja.Object); ok {
					preserveErrorEvent = importedObject.Get("type") != nil && importedObject.Get("type").String() == "error"
				}
				defer func() {
					parent.iframeEventContext = previousContext
					if !preserveErrorEvent {
						parent.legacyWindowEvent = previousLegacyEvent
					}
				}()
			}
			args := make([]goja.Value, len(arguments))
			seen := make(map[*goja.Object]*goja.Object)
			for i, arg := range arguments {
				args[i] = parent.importChildValue(child, arg, seen, 0)
			}
			if preserveErrorEvent && len(args) > 0 {
				if event, ok := args[0].(*goja.Object); ok && event.Get("type") != nil && event.Get("type").String() == "error" {
					parent.legacyWindowEvent = args[0]
				}
			}
			importedThis := parent.importChildValue(child, this, make(map[*goja.Object]*goja.Object), 0)
			result, err := fn(importedThis, args...)
			if err != nil {
				if exception, ok := err.(*goja.Exception); ok {
					panic(importRuntimeValue(parent, child, exception.Value()))
				}
				panic(child.vm.NewGoError(err))
			}
			return child.importParentValue(parent, result)
		}
		importedFunction := child.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			return invoke(call.This, call.Arguments)
		})
		if importedObject, ok := importedFunction.(*goja.Object); ok {
			if sourceObject, ok := value.(*goja.Object); ok {
				if child.iframeImportedParentValues == nil {
					child.iframeImportedParentValues = make(map[*goja.Object]*goja.Object)
				}
				child.iframeImportedParentValues[sourceObject] = importedObject
				if child.iframeParentObjects == nil {
					child.iframeParentObjects = make(map[*goja.Object]goja.Value)
				}
				child.iframeParentObjects[importedObject] = value
			}
			if child.iframeImportedFunctionRealms == nil {
				child.iframeImportedFunctionRealms = make(map[*goja.Object]*Runtime)
			}
			realm := parent
			if sourceObject, ok := value.(*goja.Object); ok {
				if sourceRealm := parent.iframeImportedFunctionRealms[sourceObject]; sourceRealm != nil {
					realm = sourceRealm
				}
			}
			child.iframeImportedFunctionRealms[importedObject] = realm
		}
		return importedFunction
	}

	if object, ok := value.(*goja.Object); ok {
		if imported := child.iframeImportedParentValues[object]; imported != nil {
			return imported
		}
		target := child.vm.NewObject()
		if parentDocument != nil {
			if wrapped := child.wrapDetachedDocument(parentDocument); wrapped != nil && !goja.IsNull(wrapped) && !goja.IsUndefined(wrapped) {
				target = wrapped.ToObject(child.vm)
			}
		} else if prototype := object.Prototype(); prototype != nil {
			// The target's prototype must mirror the source object's chain. A
			// plain proxy target would otherwise make imported class prototypes
			// inherit from the receiving realm's Object.prototype, breaking
			// interface checks such as customized-built-in constructor matching.
			if importedPrototype, ok := child.importParentValue(parent, prototype).(*goja.Object); ok {
				_ = target.SetPrototype(importedPrototype)
			}
		}
		proxy := child.vm.NewProxy(target, &goja.ProxyTrapConfig{
			Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
				return child.importParentValue(parent, object.Get(property))
			},
			Set: func(_ *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
				return object.Set(property, parent.importChildValue(child, assigned, make(map[*goja.Object]*goja.Object), 0)) == nil
			},
			Has: func(_ *goja.Object, property string) bool {
				return object.Get(property) != nil && !goja.IsUndefined(object.Get(property))
			},
		})
		proxyValue := child.vm.ToValue(proxy)
		if proxyObject, ok := proxyValue.(*goja.Object); ok {
			if child.iframeImportedParentValues == nil {
				child.iframeImportedParentValues = make(map[*goja.Object]*goja.Object)
			}
			child.iframeImportedParentValues[object] = proxyObject
			child.iframeParentObjects[proxyObject] = value
			child.rememberImportedObjectRealm(proxyObject, object, parent)
		}
		return proxyValue
	}
	return child.vm.ToValue(value.Export())
}

func findImportedFunctionSource(start *Runtime, imported *goja.Object) (*Runtime, *goja.Object) {
	if start == nil || imported == nil {
		return nil, nil
	}
	root := start
	for root.iframeParent != nil {
		root = root.iframeParent
	}
	seen := make(map[*Runtime]bool)
	var find func(*Runtime) (*Runtime, *goja.Object)
	find = func(runtime *Runtime) (*Runtime, *goja.Object) {
		if runtime == nil || seen[runtime] {
			return nil, nil
		}
		seen[runtime] = true
		for _, child := range runtime.iframeChildRuntimes {
			if original := child.iframeImportedParentValues[imported]; original != nil {
				return child, original
			}
			if sourceRuntime, original := find(child); sourceRuntime != nil {
				return sourceRuntime, original
			}
		}
		return nil, nil
	}
	return find(root)
}

func (child *Runtime) importSiblingFunction(sourceRuntime *Runtime, source *goja.Object) goja.Value {
	if sourceRuntime == nil || source == nil {
		return goja.Undefined()
	}
	if imported := child.iframeSiblingFunctions[source]; imported != nil {
		return imported
	}
	call, ok := goja.AssertFunction(source)
	if !ok {
		return goja.Undefined()
	}
	invoke := func(this goja.Value, arguments []goja.Value) goja.Value {
		importedArguments := make([]goja.Value, len(arguments))
		for i, argument := range arguments {
			importedArguments[i] = importRuntimeValue(child, sourceRuntime, argument)
		}
		importedThis := importRuntimeValue(child, sourceRuntime, this)
		result, err := call(importedThis, importedArguments...)
		if err != nil {
			if exception, ok := err.(*goja.Exception); ok {
				panic(importRuntimeValue(sourceRuntime, child, exception.Value()))
			}
			panic(child.vm.NewGoError(err))
		}
		return importRuntimeValue(sourceRuntime, child, result)
	}
	var importedValue goja.Value
	if constructor, constructible := goja.AssertConstructor(source); constructible {
		functionConstructor, ok := goja.AssertConstructor(child.vm.Get("Function"))
		if !ok {
			return goja.Undefined()
		}
		target, err := functionConstructor(nil, child.vm.ToValue("return function(){}"))
		if err != nil {
			return goja.Undefined()
		}
		if prototype := importRuntimeValue(sourceRuntime, child, source.Get("prototype")); prototype != nil && !goja.IsUndefined(prototype) && !goja.IsNull(prototype) {
			_ = target.Set("prototype", prototype)
		}
		var proxy *goja.Object
		proxyValue := child.vm.NewProxy(target, &goja.ProxyTrapConfig{
			GetPrototypeOf: func(*goja.Object) *goja.Object {
				prototype := importRuntimeValue(sourceRuntime, child, source.Prototype())
				if prototype == nil || goja.IsNull(prototype) || goja.IsUndefined(prototype) {
					return nil
				}
				return prototype.ToObject(child.vm)
			},
			Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
				if property == "prototype" || property == "arguments" || property == "caller" {
					return target.Get(property)
				}
				return importRuntimeValue(sourceRuntime, child, source.Get(property))
			},
			GetSym: func(_ *goja.Object, property *goja.Symbol, _ goja.Value) goja.Value {
				return importRuntimeValue(sourceRuntime, child, inheritedSymbolValue(source, property))
			},
			Apply: func(_ *goja.Object, this goja.Value, arguments []goja.Value) goja.Value {
				return invoke(this, arguments)
			},
			Construct: func(_ *goja.Object, arguments []goja.Value, newTarget *goja.Object) *goja.Object {
				importedArguments := make([]goja.Value, len(arguments))
				for i, argument := range arguments {
					importedArguments[i] = importRuntimeValue(child, sourceRuntime, argument)
				}
				var sourceNewTarget *goja.Object
				if newTarget == proxy {
					sourceNewTarget = source
				} else if value := importRuntimeValue(child, sourceRuntime, newTarget); value != nil {
					sourceNewTarget, _ = value.(*goja.Object)
				}
				instance, constructErr := constructor(sourceNewTarget, importedArguments...)
				if constructErr != nil {
					if exception, ok := constructErr.(*goja.Exception); ok {
						panic(importRuntimeValue(sourceRuntime, child, exception.Value()))
					}
					panic(child.vm.NewGoError(constructErr))
				}
				return importRuntimeValue(sourceRuntime, child, instance).ToObject(child.vm)
			},
		})
		proxy = child.vm.ToValue(proxyValue).ToObject(child.vm)
		importedValue = proxy
	} else {
		importedValue = child.vm.ToValue(func(invocation goja.FunctionCall) goja.Value {
			return invoke(invocation.This, invocation.Arguments)
		})
	}
	if imported, ok := importedValue.(*goja.Object); ok {
		if child.iframeSiblingFunctions == nil {
			child.iframeSiblingFunctions = make(map[*goja.Object]goja.Value)
		}
		child.iframeSiblingFunctions[source] = imported
		if child.iframeImportedFunctionRealms == nil {
			child.iframeImportedFunctionRealms = make(map[*goja.Object]*Runtime)
		}
		child.iframeImportedFunctionRealms[imported] = sourceRuntime
	}
	return importedValue
}

// importParentConstructor creates a callable and constructible WindowProxy
// bridge for a constructor supplied by the parent realm. A plain Go function
// wrapper loses [[Construct]], which makes cross-realm customElements.define()
// reject valid class constructors.
func (child *Runtime) importParentConstructor(parent *Runtime, source *goja.Object) goja.Value {
	if imported := child.iframeImportedParentValues[source]; imported != nil {
		return imported
	}
	constructor, ok := goja.AssertConstructor(source)
	if !ok {
		return goja.Undefined()
	}
	functionConstructor, ok := goja.AssertConstructor(child.vm.Get("Function"))
	if !ok {
		return goja.Undefined()
	}
	target, err := functionConstructor(nil, child.vm.ToValue("return function(){}"))
	if err != nil {
		return goja.Undefined()
	}
	if prototype := child.importParentValue(parent, source.Get("prototype")); prototype != nil && !goja.IsUndefined(prototype) && !goja.IsNull(prototype) {
		_ = target.Set("prototype", prototype)
	}
	proxy := child.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
			if property == "arguments" || property == "caller" {
				return target.Get(property)
			}
			if property == "prototype" {
				return target.Get(property)
			}
			return child.importParentValue(parent, source.Get(property))
		},
		GetSym: func(_ *goja.Object, property *goja.Symbol, _ goja.Value) goja.Value {
			if property == goja.SymHasInstance {
				parentFunction := parent.vm.Get("Function").ToObject(parent.vm)
				parentFunctionPrototype := parentFunction.Get("prototype").ToObject(parent.vm)
				hasInstance := inheritedSymbolValue(source, property)
				if hasInstance.StrictEquals(parentFunctionPrototype.GetSymbol(property)) {
					return child.vm.ToValue(func(call goja.FunctionCall) goja.Value {
						constructorObject, ok := call.This.(*goja.Object)
						if !ok {
							return child.vm.ToValue(false)
						}
						prototype, ok := constructorObject.Get("prototype").(*goja.Object)
						if !ok {
							panic(child.vm.NewTypeError("Function has non-object prototype in instanceof check"))
						}
						instance, ok := call.Argument(0).(*goja.Object)
						if !ok {
							return child.vm.ToValue(false)
						}
						for current := instance.Prototype(); current != nil; current = current.Prototype() {
							if current == prototype {
								return child.vm.ToValue(true)
							}
						}
						return child.vm.ToValue(false)
					})
				}
			}
			return child.importParentValue(parent, inheritedSymbolValue(source, property))
		},
		Set: func(_ *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
			if err := source.Set(property, parent.importChildValue(child, assigned, make(map[*goja.Object]*goja.Object), 0)); err != nil {
				return false
			}
			return target.Set(property, assigned) == nil
		},
		Has: func(_ *goja.Object, property string) bool {
			return source.Get(property) != nil && !goja.IsUndefined(source.Get(property))
		},
		Apply: func(_ *goja.Object, this goja.Value, arguments []goja.Value) goja.Value {
			call, ok := goja.AssertFunction(source)
			if !ok {
				panic(child.vm.NewTypeError("Cross-realm value is not callable"))
			}
			args := make([]goja.Value, len(arguments))
			for i, argument := range arguments {
				args[i] = parent.importChildValue(child, argument, make(map[*goja.Object]*goja.Object), 0)
			}
			importedThis := parent.importChildValue(child, this, make(map[*goja.Object]*goja.Object), 0)
			result, callErr := call(importedThis, args...)
			if callErr != nil {
				if exception, ok := callErr.(*goja.Exception); ok {
					panic(child.importParentValue(parent, exception.Value()))
				}
				panic(child.vm.NewGoError(callErr))
			}
			return child.importParentValue(parent, result)
		},
		Construct: func(_ *goja.Object, arguments []goja.Value, newTarget *goja.Object) *goja.Object {
			args := make([]goja.Value, len(arguments))
			for i, argument := range arguments {
				args[i] = parent.importChildValue(child, argument, make(map[*goja.Object]*goja.Object), 0)
			}
			importedNewTarget := parent.importChildValue(child, newTarget, make(map[*goja.Object]*goja.Object), 0)
			parentNewTarget, ok := importedNewTarget.(*goja.Object)
			if !ok {
				panic(child.vm.NewTypeError("Cross-realm newTarget is not an object"))
			}
			instance, constructErr := constructor(parentNewTarget, args...)
			if constructErr != nil {
				if exception, ok := constructErr.(*goja.Exception); ok {
					panic(child.importParentValue(parent, exception.Value()))
				}
				panic(child.vm.NewGoError(constructErr))
			}
			importedInstance := child.importParentValue(parent, instance)
			if object, ok := importedInstance.(*goja.Object); ok {
				return object
			}
			panic(child.vm.NewTypeError("Cross-realm constructor did not return an object"))
		},
	})
	proxyObject := child.vm.ToValue(proxy).ToObject(child.vm)
	if child.iframeImportedParentValues == nil {
		child.iframeImportedParentValues = make(map[*goja.Object]*goja.Object)
	}
	child.iframeImportedParentValues[source] = proxyObject
	if child.iframeParentObjects == nil {
		child.iframeParentObjects = make(map[*goja.Object]goja.Value)
	}
	child.iframeParentObjects[proxyObject] = source
	if child.iframeImportedFunctionRealms == nil {
		child.iframeImportedFunctionRealms = make(map[*goja.Object]*Runtime)
	}
	realm := parent.iframeImportedFunctionRealms[source]
	if realm == nil {
		realm = parent
	}
	child.iframeImportedFunctionRealms[proxyObject] = realm
	return proxyObject
}

func (child *Runtime) importParentNewTarget(parent *Runtime, source *goja.Object) *goja.Object {
	if source == nil {
		return nil
	}
	targetValue := child.importParentValue(parent, source)
	target, ok := targetValue.(*goja.Object)
	if !ok {
		return nil
	}
	proxy := child.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
			if property == "arguments" || property == "caller" {
				return target.Get(property)
			}
			if property == "prototype" {
				return target.Get(property)
			}
			return child.importParentValue(parent, source.Get(property))
		},
		GetSym: func(_ *goja.Object, property *goja.Symbol, _ goja.Value) goja.Value {
			return child.importParentValue(parent, inheritedSymbolValue(source, property))
		},
		Set: func(_ *goja.Object, property string, assigned goja.Value, _ goja.Value) bool {
			if err := source.Set(property, parent.importChildValue(child, assigned, make(map[*goja.Object]*goja.Object), 0)); err != nil {
				return false
			}
			return target.Set(property, assigned) == nil
		},
		Has: func(_ *goja.Object, property string) bool {
			return source.Get(property) != nil && !goja.IsUndefined(source.Get(property))
		},
	})
	proxyObject := child.vm.ToValue(proxy).ToObject(child.vm)
	if child.iframeImportedFunctionAliases == nil {
		child.iframeImportedFunctionAliases = make(map[*goja.Object]*goja.Object)
	}
	child.iframeImportedFunctionAliases[proxyObject] = target
	if child.iframeImportedFunctionRealms == nil {
		child.iframeImportedFunctionRealms = make(map[*goja.Object]*Runtime)
	}
	realm := child.iframeImportedFunctionRealms[target]
	if realm == nil {
		realm = parent
	}
	child.iframeImportedFunctionRealms[proxyObject] = realm
	if child.iframeParentObjects == nil {
		child.iframeParentObjects = make(map[*goja.Object]goja.Value)
	}
	child.iframeParentObjects[proxyObject] = source
	return proxyObject
}

func (r *Runtime) importCrossRealmException(source *Runtime, err error) *goja.Object {
	if exception, ok := err.(*goja.Exception); ok && source != nil {
		if object, ok := exception.Value().(*goja.Object); ok {
			nameValue := object.Get("name")
			messageValue := object.Get("message")
			name := "Error"
			message := exception.Error()
			if nameValue != nil && !goja.IsUndefined(nameValue) {
				name = nameValue.String()
			}
			if messageValue != nil && !goja.IsUndefined(messageValue) {
				message = messageValue.String()
			}
			if code := object.Get("code"); code != nil && !goja.IsUndefined(code) {
				if constructor, ok := goja.AssertConstructor(r.vm.Get("DOMException")); ok {
					if transferred, callErr := constructor(nil, r.vm.ToValue(message), r.vm.ToValue(name)); callErr == nil {
						return transferred
					}
				}
			}
			if constructor, ok := goja.AssertConstructor(r.vm.Get(name)); ok {
				if transferred, callErr := constructor(nil, r.vm.ToValue(message)); callErr == nil {
					return transferred
				}
			}
		}
	}
	return r.vm.NewGoError(err)
}

// reportWindowException surfaces an uncaught exception from a callback in
// its owning window. The resulting ErrorEvent runs while the original event
// context remains available to outer realms.
func (r *Runtime) reportWindowException(err error) {
	r.reportWindowExceptionFrom(nil, err)
}

func (r *Runtime) reportWindowExceptionFrom(source *Runtime, err error) {
	if err == nil || r.reportingWindowError {
		return
	}
	r.reportingWindowError = true
	defer func() { r.reportingWindowError = false }()
	previousContext := r.iframeEventContext
	defer func() { r.iframeEventContext = previousContext }()

	var errorValue goja.Value
	message := err.Error()
	if exception, ok := err.(*goja.Exception); ok {
		errorValue = exception.Value()
		if source != nil && source != r {
			errorValue = importRuntimeValue(source, r, errorValue)
		}
		if object, ok := errorValue.(*goja.Object); ok {
			if value := object.Get("message"); value != nil && !goja.IsUndefined(value) {
				message = value.String()
			}
		}
	} else {
		errorValue = r.vm.NewGoError(err)
	}
	event := dom.NewErrorEvent("error", false, false, message, "", 0, 0, errorValue)
	wrapped := r.wrapWindowEvent(&event.Event, false)
	r.iframeEventContext = wrapped
	r.fireWindowListeners(&event.Event, true)
	if !event.Event.PropagationStopped() {
		r.fireWindowListeners(&event.Event, false)
	}
}

func (r *Runtime) queueIFrameLoad(frame *dom.Node) {
	if frame == nil || frame.Data != "iframe" {
		return
	}
	r.installWindowFrameProperties()
	source := iframeSourceKey(frame)
	if r.iframeLoadedSources[frame] == source {
		return
	}
	for _, pending := range r.pendingIFrames {
		if pending == frame {
			return
		}
	}
	r.pendingIFrames = append(r.pendingIFrames, frame)
}

func iframeSourceKey(frame *dom.Node) string {
	if frame.HasAttribute("srcdoc") {
		return "srcdoc:" + frame.GetAttribute("srcdoc")
	}
	return "src:" + strings.TrimSpace(frame.GetAttribute("src"))
}

func (r *Runtime) queueExistingIFrames() {
	if r.domDoc == nil {
		return
	}
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() && node.Data == "iframe" {
			r.queueIFrameLoad(node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(&r.domDoc.Node)
}

// QueueExistingIFrames queues loads for parser-created iframe elements after
// parsing has advanced since SetDOM. It preserves the runtime's wrapper and
// image state caches, which SetDOM intentionally rebuilds for a new document.
func (r *Runtime) QueueExistingIFrames() {
	r.queueExistingIFrames()
}

// RefreshDocumentTreeReferences updates the body/head/documentElement/doctype
// references after an incremental parser step. SetDOM normally runs only once
// per document, so these node-valued properties need refreshing as the parser
// creates the document's structural elements.
func (r *Runtime) RefreshDocumentTreeReferences() {
	if r.domDoc == nil {
		return
	}
	doc, _ := r.vm.Get("document").(*goja.Object)
	if doc == nil {
		return
	}
	for name, node := range map[string]*dom.Node{
		"body":            r.domDoc.Body,
		"head":            r.domDoc.Head,
		"documentElement": r.domDoc.HTML,
		"doctype":         r.domDoc.DocType,
	} {
		if node == nil {
			_ = doc.Set(name, goja.Null())
		} else {
			_ = doc.Set(name, r.wrapNode(node))
		}
	}
}

func (r *Runtime) flushIFrameLoads() {
	pending := r.pendingIFrames
	r.pendingIFrames = nil
	for _, frame := range pending {
		if frame == nil {
			continue
		}
		// An iframe's initial about:blank load must not win the load event
		// race against a form navigation already planned for this browsing
		// context. Keep that initial event suppressed; the queued form task will
		// install the destination and flush its real navigation below.
		if r.iframeLoadedSources[frame] != iframeSourceKey(frame) &&
			r.plannedIFrameGETURLs[frame] != "" {
			continue
		}
		if request, ok := r.iframeFormRequests[frame]; ok {
			delete(r.iframeFormRequests, frame)
			if r.iframeFormSubmitter != nil && (r.opts.CSP == nil || r.opts.CSP.AllowsFrame(request.url, r.opts.URL)) {
				if doc, err := r.iframeFormSubmitter(r.opts.URL, request.url, request.contentType, request.body); err == nil && doc != nil {
					r.iframeLocations[frame] = request.url
					r.setIFrameDocument(frame, doc)
					r.markIFrameSourceLoaded(frame)
					if win := r.iframeWindows[frame]; win != nil {
						_ = win.Set("document", r.wrapIFrameDocument(frame))
					}
					r.installExternalIFrameRuntime(frame, doc, request.url)
				}
			}
		} else if frame.HasAttribute("srcdoc") {
			// srcdoc is parsed as a child HTML document and takes precedence over
			// src. Bound both source bytes and the resulting tree because markup is
			// page supplied and can be much larger than the parent element.
			markup := frame.GetAttribute("srcdoc")
			if len(markup) <= 1<<20 {
				if doc, err := r.parseSrcdoc(frame, markup); err == nil && doc != nil {
					r.setIFrameDocument(frame, doc)
					r.markIFrameSourceLoaded(frame)
					if win := r.iframeWindows[frame]; win != nil {
						_ = win.Set("document", r.wrapIFrameDocument(frame))
					}
				}
			}
		} else if src := strings.TrimSpace(frame.GetAttribute("src")); src != "" && (r.opts.CSP == nil || r.opts.CSP.AllowsFrame(src, r.opts.URL)) {
			if r.iframeLocations == nil {
				r.iframeLocations = make(map[*dom.Node]string)
			}
			r.iframeLocations[frame] = r.resolveURLString(src)
			var doc *dom.Document
			if data, ok := r.objectURLSource(src); ok && len(data) <= maxObjectURLBytes {
				text := string(data)
				limits := dom.ParseLimits{Nodes: 100000, Depth: 128, Attributes: 100000, AttributeBytes: 1 << 20}
				if strings.HasPrefix(strings.TrimSpace(text), "<?xml") {
					doc, _ = ParseXMLDocumentBounded(text, "application/xhtml+xml", limits)
					if doc != nil {
						doc.ParserInput = text
					}
				} else {
					doc, _ = dom.ParseBounded(text, limits)
				}
			} else if r.iframeLoader != nil {
				doc, _ = r.iframeLoader(r.opts.URL, src)
			}
			if doc != nil {
				r.setIFrameDocument(frame, doc)
				r.markIFrameSourceLoaded(frame)
				if win := r.iframeWindows[frame]; win != nil {
					_ = win.Set("document", r.wrapIFrameDocument(frame))
				}
				r.installExternalIFrameRuntime(frame, doc, r.resolveURLString(src))
			}
		}
		if r.iframeLoadedSources[frame] == "" {
			r.markIFrameSourceLoaded(frame)
		}
		// Establish the stable WindowProxy for the active Document before load
		// listeners run. A listener may synchronously resolve a Promise that then
		// reads window.customElements; creating the proxy from that continuation
		// would otherwise expose a registry allocated against the previous
		// about:blank Document.
		_ = r.iframeWindow(frame)
		// Frame load events are queued until the current script task completes,
		// giving the page the opportunity to register onload after insertion.
		r.dispatchWithWindowStep(frame, dom.NewEvent("load", false, false))
	}
}

func (r *Runtime) markIFrameSourceLoaded(frame *dom.Node) {
	if r.iframeLoadedSources == nil {
		r.iframeLoadedSources = make(map[*dom.Node]string)
	}
	r.iframeLoadedSources[frame] = iframeSourceKey(frame)
}

func (r *Runtime) navigateIFrameLocation(frame *dom.Node, reference string) {
	if frame == nil || !frame.IsConnected() {
		return
	}
	// This Location object is the parent realm's WindowProxy view of the child.
	// The href setter parses against the caller's entry settings object, which
	// is this Runtime's document URL, rather than the child's current URL.
	base := r.documentURL()
	ref, err := url.Parse(reference)
	if err != nil {
		return
	}
	if !ref.IsAbs() && base != nil {
		ref = base.ResolveReference(ref)
	}
	target := ref.String()
	if target == "" {
		return
	}
	r.cacheIFrameHistoryDocument(frame)
	r.recordIFrameHistory(frame, target)
	r.scheduleIFrameNavigation(frame, target)
}

func (r *Runtime) scheduleIFrameNavigation(frame *dom.Node, target string) {
	if frame == nil || target == "" {
		return
	}
	if r.iframeLocations == nil {
		r.iframeLocations = make(map[*dom.Node]string)
	}
	r.iframeLocations[frame] = target
	delete(r.plannedIFrameGETURLs, frame)
	delete(r.iframeFormRequests, frame)
	pending := r.pendingIFrames[:0]
	for _, queued := range r.pendingIFrames {
		if queued != frame {
			pending = append(pending, queued)
		}
	}
	r.pendingIFrames = pending
	for form, formFrame := range r.formNavigationFrames {
		if formFrame == frame {
			delete(r.formNavigationFrames, form)
		}
	}
	navigationID := r.nextIFrameNavigationID(frame)
	r.QueueTask(func() {
		if r.iframeNavigationIDs[frame] != navigationID {
			return
		}
		if child := r.iframeChildRuntimes[frame]; child != nil {
			beforeUnload := dom.NewEvent("beforeunload", false, true)
			child.fireWindowListeners(beforeUnload, true)
			child.fireWindowListeners(beforeUnload, false)
		}
		if index := r.iframeHistoryIndex[frame]; index >= 0 {
			if doc := r.iframeHistoryDocs[frame][index]; doc != nil {
				frame.SetAttribute("src", target)
				r.setIFrameDocument(frame, doc)
				r.iframeLocations[frame] = target
				r.markIFrameSourceLoaded(frame)
				if win := r.iframeWindows[frame]; win != nil {
					_ = win.Set("document", r.wrapIFrameDocument(frame))
				}
				r.installExternalIFrameRuntime(frame, doc, target)
				r.dispatchWithWindowStep(frame, dom.NewEvent("load", false, false))
				return
			}
		}
		if wrapper, ok := r.wrapNode(frame).(*goja.Object); ok {
			// Once a browsing context navigates away from srcdoc, the embedding
			// attribute must not keep overriding the newly selected URL.
			frame.RemoveAttribute("srcdoc")
			_ = wrapper.Set("src", target)
			r.queueIFrameLoad(frame)
			r.flushIFrameLoads()
		}
	})
}

// FlushIFrameLoads processes queued iframe navigations and their load events.
// The engine calls this between DOMContentLoaded and document load, when iframe
// completion is a load-delaying step for the parent document.
func (r *Runtime) FlushIFrameLoads() {
	r.flushIFrameLoads()
}
