package js

import (
	"errors"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// This file is the element-interface half of the DOM binding. It installs the
// WebIDL prototype chain once per runtime (EventTarget > Node > Element >
// HTMLElement > per-tag interface) and the reflected attribute accessors on
// those prototypes, so a property read or write reaches the real attribute
// instead of a dead own-property snapshot.
//
// The measured Chromium answers every rule here implements are recorded in
// docs/tasks/TASK-052-ELEMENT-INTERFACES.md §4; the tables below are the
// table-driven oracle's source of truth.

// ImageLoader resolves the source a script assigned to an HTMLImageElement.
// The engine installs it so script-created images pass internal/image's
// encoded-size, dimension and pixel caps and the session's imageReservation.
// Implementations may call done from any goroutine; the runtime queues the
// state change as a microtask, so load state never changes in the middle of a
// script task.
type ImageLoader interface {
	LoadScriptImage(src string, done func(LoadedImage))
}

// LoadedImage is the outcome of one script-image load. Width and Height are
// the intrinsic dimensions of the decoded image; Src is the absolute source
// that answered (or the raw source a failed load named); Err reports why a
// load did not produce pixels.
type LoadedImage struct {
	Width, Height int
	Src           string
	Err           error
}

// SetImageLoader installs the script-image load path. The engine calls it
// before any script runs.
func (r *Runtime) SetImageLoader(l ImageLoader) {
	r.mu.Lock()
	r.opts.ImageLoader = l
	r.mu.Unlock()
}

type imageStatus uint8

const (
	imageIdle imageStatus = iota
	imageLoading
	imageLoaded
	imageFailed
)

// imageLoadState is the live state one script-created HTMLImageElement
// reflects. Parsed <img> elements have no entry and answer from their
// attributes alone.
type imageLoadState struct {
	gen        int
	status     imageStatus
	src        string
	currentSrc string
	naturalW   int
	naturalH   int
	waiters    []*promiseObj
}

// jsEventHandler is one on<type> property. The callback is registered with
// internal/dom on first assignment so dispatch order matches
// addEventListener; replacing the handler only swaps fn.
type jsEventHandler struct {
	fn         goja.Value
	registered bool
	key        *dom.Node
}

// ---------------------------------------------------------------------------
// Prototype chain and interface constructors
// ---------------------------------------------------------------------------

// setupProtoChain builds the shared interfaces. It runs once per runtime from
// setupDOM, before any wrapper exists.
func (r *Runtime) setupProtoChain() {
	r.tagProtos = make(map[string]*goja.Object)
	r.ifaceCtors = make(map[string]*goja.Object)

	r.eventTargetProto = r.vm.NewObject()
	r.nodeProto = r.newChildProto(r.eventTargetProto)
	r.elementProto = r.newChildProto(r.nodeProto)
	r.htmlElementProto = r.newChildProto(r.elementProto)
	r.documentProto = r.newChildProto(r.nodeProto)

	r.defineConstructor("EventTarget", r.eventTargetProto)
	r.defineConstructor("Node", r.nodeProto)
	r.defineConstructor("Element", r.elementProto)
	r.defineConstructor("HTMLElement", r.htmlElementProto)
	r.defineConstructor("Document", r.documentProto)

	// The interfaces whose constructors a page probes even before it creates
	// an element. Per-tag prototypes for other tags are minted lazily by
	// protoForTag, so a document pays only for the tags it actually wraps.
	for _, tag := range []string{
		"html", "head", "body", "title", "div", "span", "p", "a", "img",
		"input", "button", "form", "select", "option", "textarea", "canvas",
		"video", "script", "style", "link", "meta", "dialog",
	} {
		r.protoForTag(tag)
	}

	// The Image constructor allocates a detached <img>; the factory is an
	// internal document method so createElement and Image share the same
	// node-creation path (and the same wrapper registry).
	if doc, ok := r.vm.Get("document").(*goja.Object); ok {
		_ = doc.Set("__createImage__", func(call goja.FunctionCall) goja.Value {
			if r.domDoc == nil {
				return goja.Null()
			}
			return r.wrapNode(r.domDoc.NewElement("img"))
		})
	}

	// window.Image is a real constructor: new Image() returns a detached
	// <img>, and new Image(w,h) applies the width/height IDL setters.
	r.installImageConstructor()
}

func (r *Runtime) newChildProto(parent *goja.Object) *goja.Object {
	p := r.vm.NewObject()
	_ = p.SetPrototype(parent)
	return p
}

// defineConstructor publishes an illegal-constructor function whose prototype
// is p, which is what `instanceof` walks. Calling it throws, as the WebIDL
// [LegacyNoInterfaceObject]/illegal-constructor interfaces do in Chromium.
func (r *Runtime) defineConstructor(name string, p *goja.Object) *goja.Object {
	if c, ok := r.ifaceCtors[name]; ok {
		return c
	}
	_ = r.vm.Set("__ifaceProto__", p)
	v, err := r.vm.RunString(`(function(proto) {
		var C = function ` + name + `() { throw new TypeError("Illegal constructor"); };
		Object.defineProperty(C, 'prototype', { value: proto, writable: false, enumerable: false, configurable: false });
		Object.defineProperty(proto, 'constructor', { value: C, writable: true, enumerable: false, configurable: true });
		return C;
	})(__ifaceProto__)`)
	_ = r.vm.Set("__ifaceProto__", nil)
	if err != nil || v == nil {
		return nil
	}
	ctor := v.ToObject(r.vm)
	r.ifaceCtors[name] = ctor
	// window IS the global object (see setupWindow), so one Set reaches both
	// `Image` and `window.Image`.
	_ = r.vm.Set(name, v)
	return ctor
}

// htmlTagInterfaces maps a tag to its Chromium interface. Tags not listed
// answer HTMLElement, which is what Chromium does for an unknown tag until a
// custom element is defined for it.
var htmlTagInterfaces = map[string]string{
	"a": "HTMLAnchorElement", "area": "HTMLAreaElement", "audio": "HTMLAudioElement",
	"body": "HTMLBodyElement", "br": "HTMLBRElement", "button": "HTMLButtonElement",
	"canvas": "HTMLCanvasElement", "div": "HTMLDivElement", "form": "HTMLFormElement",
	"head": "HTMLHeadElement", "hr": "HTMLHRElement", "html": "HTMLHtmlElement",
	"iframe": "HTMLIFrameElement", "img": "HTMLImageElement", "input": "HTMLInputElement",
	"label": "HTMLLabelElement", "li": "HTMLLIElement", "link": "HTMLLinkElement",
	"meta": "HTMLMetaElement", "ol": "HTMLOListElement", "option": "HTMLOptionElement",
	"p": "HTMLParagraphElement", "pre": "HTMLPreElement", "script": "HTMLScriptElement",
	"select": "HTMLSelectElement", "span": "HTMLSpanElement", "style": "HTMLStyleElement",
	"table": "HTMLTableElement", "td": "HTMLTableCellElement", "textarea": "HTMLTextAreaElement",
	"th": "HTMLTableCellElement", "title": "HTMLTitleElement", "tr": "HTMLTableRowElement",
	"ul": "HTMLUListElement", "video": "HTMLVideoElement", "dialog": "HTMLDialogElement",
}

// protoForTag returns the per-tag interface prototype, creating it (and its
// constructor and reflected properties) on first use.
func (r *Runtime) protoForTag(tag string) *goja.Object {
	tag = strings.ToLower(tag)
	if p, ok := r.tagProtos[tag]; ok {
		return p
	}
	iface := htmlTagInterfaces[tag]
	if iface == "" {
		r.tagProtos[tag] = r.htmlElementProto
		return r.htmlElementProto
	}
	p := r.vm.NewObject()
	_ = p.SetPrototype(r.htmlElementProto)
	r.tagProtos[tag] = p
	r.installInterface(iface, p)
	r.defineConstructor(iface, p)
	return p
}

// protoForNode returns the [[Prototype]] for one wrapper.
func (r *Runtime) protoForNode(node *dom.Node) *goja.Object {
	if node.Element() {
		return r.protoForTag(node.Data)
	}
	return r.nodeProto
}

func (r *Runtime) installInterface(iface string, p *goja.Object) {
	switch iface {
	case "HTMLAnchorElement":
		r.installAnchorInterface(p)
	case "HTMLImageElement":
		r.installImageInterface(p)
	case "HTMLInputElement":
		r.installInputInterface(p)
	}
}

// ---------------------------------------------------------------------------
// Reflection helpers
// ---------------------------------------------------------------------------

// installStringReflection installs a DOMString attribute reflection: getter
// returns the attribute or "", setter stores String(value). Chromium stores
// "null" for null and "undefined" for undefined, which String already does.
func (r *Runtime) installStringReflection(p *goja.Object, prop, attr string) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.GetAttribute(attr))
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		node.SetAttribute(attr, call.Argument(0).String())
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty(prop, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// installBoolReflection installs a boolean attribute reflection: true sets
// the attribute to "", false removes it.
func (r *Runtime) installBoolReflection(p *goja.Object, prop, attr string) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.vm.ToValue(node != nil && node.HasAttribute(attr))
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		if call.Argument(0).ToBoolean() {
			node.SetAttribute(attr, "")
		} else {
			node.RemoveAttribute(attr)
		}
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty(prop, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// longSetter validates a value on its way to the attribute. A nil setter
// clamps to zero, which is what the unsigned reflections do with a negative
// number.
type longSetter func(v int64) (int64, *goja.Object)

// installUnsignedReflection installs a reflected unsigned long with def as
// the getter fallback for a missing or invalid attribute. The getter parses
// the leading HTML integer, so "12px" reads 12, and a negative attribute
// reads def. The setter truncates, then lets check clamp or throw.
func (r *Runtime) installUnsignedReflection(p *goja.Object, prop, attr string, def int64, check longSetter) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue(def)
		}
		v, ok := parseHTMLUnsigned(node.GetAttribute(attr))
		if !ok {
			return r.vm.ToValue(def)
		}
		return r.vm.ToValue(v)
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		v := call.Argument(0).ToInteger()
		if check != nil {
			var throw *goja.Object
			v, throw = check(v)
			if throw != nil {
				panic(throw)
			}
		}
		if v < 0 {
			v = 0
		}
		node.SetAttribute(attr, strconv.FormatInt(v, 10))
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty(prop, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// installLongReflection installs a signed long attribute reflection
// (tabIndex): the getter parses a leading integer and falls back to def; the
// setter stores the truncated value without a non-negative clamp.
func (r *Runtime) installLongReflection(p *goja.Object, prop, attr string, def int64) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue(def)
		}
		v, ok := parseHTMLInteger(node.GetAttribute(attr))
		if !ok {
			return r.vm.ToValue(def)
		}
		return r.vm.ToValue(v)
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		node.SetAttribute(attr, strconv.FormatInt(call.Argument(0).ToInteger(), 10))
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty(prop, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// installEnumReflection installs a "limited to only known values" attribute
// reflection. The getter lowercases a matching value and returns def
// otherwise; the setter stores the raw string, as Chromium does (setting
// `loading = 'bogus'` leaves the attribute "bogus" while the property reads
// "auto").
func (r *Runtime) installEnumReflection(p *goja.Object, prop, attr, def string, values []string) {
	allowed := make(map[string]bool, len(values))
	for _, v := range values {
		allowed[v] = true
	}
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue(def)
		}
		raw := strings.ToLower(node.GetAttribute(attr))
		if allowed[raw] {
			return r.vm.ToValue(raw)
		}
		return r.vm.ToValue(def)
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		node.SetAttribute(attr, call.Argument(0).String())
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty(prop, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// installURLReflection installs a DOMString reflected with the URL parsing
// mechanism. The getter resolves against the document base; the setter stores
// the raw string. Missing attributes answer "", and a value the URL parser
// rejects reads back exactly as written (measured Chromium: "http://[::1"
// stays "http://[::1").
func (r *Runtime) installURLReflection(p *goja.Object, prop, attr string) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.HasAttribute(attr) {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(r.resolveURLString(node.GetAttribute(attr)))
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		node.SetAttribute(attr, call.Argument(0).String())
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty(prop, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// parseHTMLInteger is the signed sibling of parseHTMLUnsigned: leading spaces
// then an optional sign and digits. Anything else, including an empty string,
// reports !ok so the caller can fall back to its default.
func parseHTMLInteger(s string) (int64, bool) {
	s = strings.TrimLeft(s, " \t\n\r\f")
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return 0, false
	}
	n, err := strconv.ParseInt(s[:i], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// parseHTMLUnsigned parses the leading non-negative HTML integer in s. It is
// deliberately lenient past the digits ("12px" -> 12) and rejects a negative
// or digitless value, matching Chromium's reflected unsigned long.
func parseHTMLUnsigned(s string) (int64, bool) {
	s = strings.TrimLeft(s, " \t\n\r\f")
	if s == "" {
		return 0, false
	}
	i := 0
	if s[0] == '+' {
		i++
	} else if s[0] == '-' {
		return 0, false
	}
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == start {
		return 0, false
	}
	n, err := strconv.ParseInt(s[start:i], 10, 64)
	if err != nil || n > math.MaxUint32 {
		return 0, false
	}
	return n, true
}

// ---------------------------------------------------------------------------
// URL helpers
// ---------------------------------------------------------------------------

// documentURL is the base every URL reflection resolves against.
func (r *Runtime) documentURL() *url.URL {
	if r.locURL != nil {
		return r.locURL
	}
	if u, err := url.Parse(r.opts.URL); err == nil {
		return u
	}
	return nil
}

// resolveURLString resolves raw against the document base and returns the
// serialised absolute URL. A value Go's parser rejects is returned verbatim,
// as Chromium does for an invalid href.
func (r *Runtime) resolveURLString(raw string) string {
	if href, _, ok := r.resolveURL(raw); ok {
		return href
	}
	return raw
}

// resolveURL resolves raw, with the URL parser's leading/trailing whitespace
// strip. It reports ok=false when the reference or the document base does not
// parse; href is then the raw value and u is nil.
func (r *Runtime) resolveURL(raw string) (href string, u *url.URL, ok bool) {
	raw = strings.Trim(raw, " \t\n\r\f")
	base := r.documentURL()
	if base == nil {
		return raw, nil, false
	}
	if raw == "" {
		// An empty reference resolves to the base; an href with no attribute
		// is handled by the caller before reaching here.
		return canonicalURLString(base), base, true
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw, nil, false
	}
	resolved := base.ResolveReference(ref)
	// Go preserves the host's case; Chromium lowercases it.
	resolved.Host = strings.ToLower(resolved.Host)
	return canonicalURLString(resolved), resolved, true
}

// canonicalURLString serialises a resolved URL. Opaque URLs (data:, mailto:,
// javascript:) keep their opaque payload untouched.
func canonicalURLString(u *url.URL) string {
	if u == nil {
		return ""
	}
	if u.Opaque != "" {
		return u.Scheme + ":" + u.Opaque
	}
	return u.String()
}

// urlPart returns one component of the anchor URL. ok=false means the source
// did not parse, in which case every part is empty except protocol, which
// Chromium answers as ":".
func urlPart(u *url.URL, part string) string {
	if u == nil {
		if part == "protocol" {
			return ":"
		}
		return ""
	}
	switch part {
	case "protocol":
		return u.Scheme + ":"
	case "hostname":
		return strings.ToLower(u.Hostname())
	case "host":
		return strings.ToLower(u.Host)
	case "port":
		return u.Port()
	case "pathname":
		if u.Opaque != "" {
			return u.Opaque
		}
		return u.EscapedPath()
	case "search":
		if u.RawQuery != "" {
			return "?" + u.RawQuery
		}
		if u.ForceQuery {
			return "?"
		}
		return ""
	case "hash":
		if u.Fragment != "" || u.RawFragment != "" {
			return "#" + u.EscapedFragment()
		}
		return ""
	case "origin":
		switch u.Scheme {
		case "http", "https":
			return u.Scheme + "://" + strings.ToLower(u.Host)
		}
		return "null"
	}
	return ""
}

// anchorURL returns the URL the href attribute resolves to, and whether it
// parsed. A missing href is the empty URL with ok=true origin "" handled by
// the caller.
func (r *Runtime) anchorURL(node *dom.Node) (*url.URL, bool) {
	if node == nil || !node.HasAttribute("href") {
		return nil, false
	}
	_, u, ok := r.resolveURL(node.GetAttribute("href"))
	return u, ok
}

// setAnchorURLPart rewrites one URL component and stores the serialisation in
// href, which is what Chromium's protocol/host/hostname/port/pathname/search/
// hash setters do (measured in probe5: six successive writes produced
// https://o.test/n?q=9#z, then http://o.test:81/n?q=9#z). The current value is
// resolved against the document base first, so an anchor without href starts
// from the page URL instead of failing. Opaque URLs (data:, mailto:) cannot
// carry these components and the write is ignored.
func (r *Runtime) setAnchorURLPart(node *dom.Node, part, value string) {
	base := r.documentURL()
	if base == nil {
		return
	}
	u := base
	if node.HasAttribute("href") {
		raw := strings.Trim(node.GetAttribute("href"), " \t\n\r\f")
		if raw != "" {
			ref, err := url.Parse(raw)
			if err != nil {
				return
			}
			u = base.ResolveReference(ref)
		}
	}
	if u.Opaque != "" {
		return
	}
	switch part {
	case "protocol":
		scheme := strings.ToLower(strings.TrimSuffix(value, ":"))
		if scheme == "" {
			return
		}
		u.Scheme = scheme
	case "host":
		if value == "" {
			return
		}
		u.Host = strings.ToLower(value)
	case "hostname":
		if value == "" {
			return
		}
		host := strings.ToLower(value)
		if port := u.Port(); port != "" {
			host += ":" + port
		}
		u.Host = host
	case "port":
		host := strings.ToLower(u.Hostname())
		if value != "" {
			host += ":" + value
		}
		u.Host = host
	case "pathname":
		if u.Scheme == "http" || u.Scheme == "https" || u.Scheme == "file" {
			if !strings.HasPrefix(value, "/") {
				value = "/" + value
			}
		}
		u.Path = value
		u.RawPath = ""
	case "search":
		u.RawQuery = strings.TrimPrefix(value, "?")
		u.ForceQuery = false
	case "hash":
		u.Fragment = strings.TrimPrefix(value, "#")
		u.RawFragment = ""
	}
	node.SetAttribute("href", canonicalURLString(u))
	r.notifyMutation()
}

// ---------------------------------------------------------------------------
// Node / Element / HTMLElement accessors
// ---------------------------------------------------------------------------

// installNodeAccessors installs textContent and the child-list reads on
// Node.prototype. Every getter resolves the receiver through __nid__, so one
// definition serves every wrapper.
func (r *Runtime) installNodeAccessors() {
	_ = r.vm.Set("__nodeProto__", r.nodeProto)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__nodeProto__, 'textContent', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getTextContent__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setTextContent__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__nodeProto__", nil)

	// nodeName for HTML elements is the uppercase tag; leaf nodes answer the
	// DOM Standard's fixed name ("#text", "#comment", ...).
	_ = r.nodeProto.DefineAccessorProperty("nodeName", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		if node.Element() {
			return r.vm.ToValue(strings.ToUpper(node.Data))
		}
		return r.vm.ToValue(nodeStaticName(node))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = r.nodeProto.DefineAccessorProperty("childNodes", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue([]goja.Value{})
		}
		kids := node.ChildNodes()
		list := make([]goja.Value, 0, len(kids))
		for _, c := range kids {
			list = append(list, r.wrapNode(c))
		}
		return r.vm.ToValue(list)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	for _, name := range []string{"firstChild", "lastChild", "nextSibling", "previousSibling"} {
		name := name
		getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Null()
			}
			switch name {
			case "firstChild":
				return r.wrapNode(node.FirstChild)
			case "lastChild":
				return r.wrapNode(node.LastChild)
			case "nextSibling":
				return r.wrapNode(node.NextSibling)
			default:
				return r.wrapNode(node.PrevSibling)
			}
		})
		_ = r.nodeProto.DefineAccessorProperty(name, getter, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}

	_ = r.nodeProto.DefineAccessorProperty("ownerDocument", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Doc == nil || node.Type == dom.NodeDocument {
			return goja.Null()
		}
		return r.wrapNode(&node.Doc.Node)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = r.nodeProto.DefineAccessorProperty("nodeValue", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		switch node.Type {
		case dom.NodeText, dom.NodeComment:
			return r.vm.ToValue(node.DataContent)
		default:
			return goja.Null()
		}
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || (node.Type != dom.NodeText && node.Type != dom.NodeComment) {
			return goja.Undefined()
		}
		if len(call.Arguments) > 0 && !goja.IsNull(call.Arguments[0]) && !goja.IsUndefined(call.Arguments[0]) {
			node.DataContent = call.Arguments[0].String()
			if node.Doc != nil {
				node.Doc.MarkQueryDirty()
			}
			r.notifyMutation()
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = r.nodeProto.DefineAccessorProperty("isConnected", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.vm.ToValue(node != nil && node.IsConnected())
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = r.nodeProto.DefineAccessorProperty("parentNode", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.wrapNode(node.Parent)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = r.nodeProto.DefineAccessorProperty("parentElement", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Parent == nil || !node.Parent.Element() {
			return goja.Null()
		}
		return r.wrapNode(node.Parent)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	// document.baseURI is the document URL every URL reflection resolves
	// against (measured in probe3 as https://example.test/docs/page.html).
	_ = r.documentProto.DefineAccessorProperty("baseURI", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.resolveURLString(""))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

// installElementAccessors installs the Element interface: tagName, id,
// className, classList, attributes, innerHTML and the child-element reads.
func (r *Runtime) installElementAccessors() {
	_ = r.elementProto.DefineAccessorProperty("tagName", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(strings.ToUpper(node.Data))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	r.installStringReflection(r.elementProto, "id", "id")
	r.installStringReflection(r.elementProto, "className", "class")
	r.installStringReflection(r.elementProto, "slot", "slot")

	_ = r.elementProto.DefineAccessorProperty("classList", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.classListObject(node)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = r.elementProto.DefineAccessorProperty("attributes", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.attributesObject(node)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = r.elementProto.DefineAccessorProperty("innerHTML", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		nid := thisNID(call)
		if nid == 0 {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(r.innerHTMLLive(nid))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		nid := thisNID(call)
		if nid != 0 {
			documentSetInnerHTML(r, nid, call.Argument(0).String())
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = r.elementProto.DefineAccessorProperty("children", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue([]goja.Value{})
		}
		var list []goja.Value
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() {
				list = append(list, r.wrapNode(c))
			}
		}
		if list == nil {
			list = []goja.Value{}
		}
		return r.vm.ToValue(list)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	for _, name := range []string{"firstElementChild", "lastElementChild"} {
		name := name
		_ = r.elementProto.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Null()
			}
			var found *dom.Node
			for c := node.FirstChild; c != nil; c = c.NextSibling {
				if c.Element() {
					found = c
					if name == "firstElementChild" {
						break
					}
				}
			}
			return r.wrapNode(found)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}

	// shadowRoot and attachShadow stay on Node.prototype (installed by
	// setupShadowDOM) and are inherited by Element.prototype.

	r.installHTMLElementAccessors()
}

// thisNID reads the registry key from the receiver without allocating a
// *dom.Node lookup; 0 means "not a wrapped node".
func thisNID(call goja.FunctionCall) int {
	obj, ok := call.This.(*goja.Object)
	if !ok || obj == nil {
		return 0
	}
	v := obj.Get("__nid__")
	if v == nil || goja.IsUndefined(v) {
		return 0
	}
	return int(v.ToInteger())
}

// innerHTMLLive and documentSetInnerHTML mirror the document helpers so the
// Element.prototype accessors keep one implementation. They are thin
// wrappers over the same node operations.
func (r *Runtime) innerHTMLLive(nid int) string {
	node := r.nodeRegistry[nid]
	if node == nil {
		return ""
	}
	return r.serializeChildren(node)
}

func documentSetInnerHTML(r *Runtime, nid int, html string) {
	if fn, ok := goja.AssertFunction(r.vm.Get("document").ToObject(r.vm).Get("__setInnerHTML__")); ok {
		// The bridge throws QuotaExceededError when admission rejects the
		// fragment. A goja call reports that as an *Exception; rethrow it so
		// the assignment the page made still fails instead of succeeding
		// silently with the old children intact.
		if _, err := fn(goja.Undefined(), r.vm.ToValue(nid), r.vm.ToValue(html)); err != nil {
			panic(err)
		}
	}
}

func (r *Runtime) installHTMLElementAccessors() {
	p := r.htmlElementProto
	_ = p.DefineAccessorProperty("style", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.styleDeclaration(node)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		r.setInlineStyleText(node, call.Argument(0).String())
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)

	// dataset is a Proxy over the live data-* attributes, as before, but the
	// accessor lives on the prototype and reads __nid__ from the receiver.
	_ = r.vm.Set("__htmlProto__", p)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__htmlProto__, 'dataset', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return {};
				var map = document.__getDataset__(nid);
				var toAttr = function(k) {
					return 'data-' + String(k).replace(/[A-Z]/g, function(c) {
						return '-' + c.toLowerCase();
					});
				};
				return new Proxy(map, {
					get: function(t, k) {
						if (typeof k !== 'string') return t[k];
						var cur = t[k];
						if (cur !== undefined) return cur;
						if (k === '__nid__') return nid;
						var v = document.__getDatasetAttr__(nid, toAttr(k));
						return v === null ? undefined : v;
					},
					set: function(t, k, v) {
						if (typeof k !== 'string' || k === '__nid__') return true;
						document.__setDatasetAttr__(map, nid, toAttr(k), String(v));
						return true;
					}
				});
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__htmlProto__", nil)

	r.installStringReflection(p, "title", "title")
	r.installStringReflection(p, "lang", "lang")
	r.installEnumReflection(p, "dir", "dir", "", []string{"ltr", "rtl", "auto"})
	r.installBoolReflection(p, "hidden", "hidden")
	r.installBoolReflection(p, "autofocus", "autofocus")
	// tabIndex is the one signed long reflection measured in probe5: negatives
	// are legal (a negative value means "not sequentially focusable"), and the
	// setter stores the truncated integer.
	r.installLongReflection(p, "tabIndex", "tabindex", -1)
}

// ---------------------------------------------------------------------------
// DOMTokenList and NamedNodeMap
// ---------------------------------------------------------------------------

// classListObject returns the per-element DOMTokenList. Identity is stable, as
// in Chromium, and every read comes from the live class attribute: a dynamic
// object is what makes `Array.prototype.join.call(classList, '+')` see fresh
// indices after setAttribute changed the class.
func (r *Runtime) classListObject(node *dom.Node) *goja.Object {
	if obj := r.classListObjects[node]; obj != nil {
		return obj
	}
	obj := r.vm.NewDynamicObject(&domTokenList{r: r, node: node})
	r.classListObjects[node] = obj
	return obj
}

type domTokenList struct {
	r    *Runtime
	node *dom.Node
}

func (t *domTokenList) tokenArg(call goja.FunctionCall) string {
	cls := call.Argument(0).String()
	if cls == "" || strings.ContainsAny(cls, " \t\n\r\f") {
		panic(t.r.domError("InvalidCharacterError", "Failed to execute 'add' on 'DOMTokenList': The token provided must not be empty and must not contain whitespace."))
	}
	return cls
}

func (t *domTokenList) Get(key string) goja.Value {
	switch key {
	case "length":
		return t.r.vm.ToValue(len(t.node.ClassList()))
	case "value":
		return t.r.vm.ToValue(strings.Join(t.node.ClassList(), " "))
	case "add":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			cls := t.tokenArg(call)
			existing := t.node.ClassList()
			for _, c := range existing {
				if c == cls {
					return goja.Undefined()
				}
			}
			t.node.SetAttribute("class", strings.Join(append(existing, cls), " "))
			t.r.notifyMutation()
			return goja.Undefined()
		})
	case "remove":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			cls := t.tokenArg(call)
			existing := t.node.ClassList()
			filtered := make([]string, 0, len(existing))
			found := false
			for _, c := range existing {
				if c != cls {
					filtered = append(filtered, c)
				} else {
					found = true
				}
			}
			if !found {
				return goja.Undefined()
			}
			if len(filtered) == 0 {
				t.node.RemoveAttribute("class")
			} else {
				t.node.SetAttribute("class", strings.Join(filtered, " "))
			}
			t.r.notifyMutation()
			return goja.Undefined()
		})
	case "toggle":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			cls := t.tokenArg(call)
			existing := t.node.ClassList()
			for i, c := range existing {
				if c == cls {
					rest := append(existing[:i], existing[i+1:]...)
					if len(rest) == 0 {
						t.node.RemoveAttribute("class")
					} else {
						t.node.SetAttribute("class", strings.Join(rest, " "))
					}
					t.r.notifyMutation()
					return t.r.vm.ToValue(false)
				}
			}
			t.node.SetAttribute("class", strings.Join(append(existing, cls), " "))
			t.r.notifyMutation()
			return t.r.vm.ToValue(true)
		})
	case "contains":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			cls := call.Argument(0).String()
			for _, c := range t.node.ClassList() {
				if c == cls {
					return t.r.vm.ToValue(true)
				}
			}
			return t.r.vm.ToValue(false)
		})
	case "item":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			i := int(call.Argument(0).ToInteger())
			list := t.node.ClassList()
			if i < 0 || i >= len(list) {
				return goja.Null()
			}
			return t.r.vm.ToValue(list[i])
		})
	case "toString":
		return t.r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return t.r.vm.ToValue(strings.Join(t.node.ClassList(), " "))
		})
	}
	if i, err := strconv.Atoi(key); err == nil {
		list := t.node.ClassList()
		if i >= 0 && i < len(list) {
			return t.r.vm.ToValue(list[i])
		}
	}
	return nil
}

func (t *domTokenList) Set(key string, value goja.Value) bool {
	if key != "value" {
		return false
	}
	t.node.SetAttribute("class", value.String())
	t.r.notifyMutation()
	return true
}

func (t *domTokenList) Has(key string) bool {
	switch key {
	case "length", "value", "add", "remove", "toggle", "contains", "item", "toString":
		return true
	}
	i, err := strconv.Atoi(key)
	return err == nil && i >= 0 && i < len(t.node.ClassList())
}

func (t *domTokenList) Delete(string) bool { return false }

func (t *domTokenList) Keys() []string {
	list := t.node.ClassList()
	keys := make([]string, 0, len(list)+2)
	for i := range list {
		keys = append(keys, strconv.Itoa(i))
	}
	return append(keys, "length", "value")
}

// attributesObject returns the per-element NamedNodeMap. A dynamic object
// keeps `attributes[0]` and `attributes.length` live while identity stays
// stable, matching Chromium.
func (r *Runtime) attributesObject(node *dom.Node) *goja.Object {
	if obj := r.attributesObjects[node]; obj != nil {
		return obj
	}
	obj := r.vm.NewDynamicObject(&namedNodeMap{r: r, node: node})
	r.attributesObjects[node] = obj
	return obj
}

type namedNodeMap struct {
	r    *Runtime
	node *dom.Node
}

func (m *namedNodeMap) attrAt(i int) goja.Value {
	if i < 0 || i >= len(m.node.Attr) {
		return nil
	}
	a := m.node.Attr[i]
	obj := m.r.vm.NewObject()
	_ = obj.Set("name", strings.ToLower(a.Name))
	_ = obj.Set("localName", strings.ToLower(a.Name))
	_ = obj.Set("value", a.Value)
	_ = obj.Set("nodeValue", a.Value)
	return obj
}

func (m *namedNodeMap) Get(key string) goja.Value {
	switch key {
	case "length":
		return m.r.vm.ToValue(len(m.node.Attr))
	case "item":
		return m.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			v := m.attrAt(int(call.Argument(0).ToInteger()))
			if v == nil {
				return goja.Null()
			}
			return v
		})
	case "getNamedItem":
		return m.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			name := call.Argument(0).String()
			for i := range m.node.Attr {
				if strings.EqualFold(m.node.Attr[i].Name, name) {
					return m.attrAt(i)
				}
			}
			return goja.Null()
		})
	}
	if i, err := strconv.Atoi(key); err == nil {
		return m.attrAt(i)
	}
	return nil
}

func (m *namedNodeMap) Set(string, goja.Value) bool { return false }

func (m *namedNodeMap) Has(key string) bool {
	switch key {
	case "length", "item", "getNamedItem":
		return true
	}
	i, err := strconv.Atoi(key)
	return err == nil && i >= 0 && i < len(m.node.Attr)
}

func (m *namedNodeMap) Delete(string) bool { return true }

func (m *namedNodeMap) Keys() []string {
	keys := make([]string, 0, len(m.node.Attr))
	for i := range m.node.Attr {
		keys = append(keys, strconv.Itoa(i))
	}
	return keys
}

// ---------------------------------------------------------------------------
// HTMLAnchorElement
// ---------------------------------------------------------------------------

func (r *Runtime) installAnchorInterface(p *goja.Object) {
	_ = p.DefineAccessorProperty("href", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.HasAttribute("href") {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(r.resolveURLString(node.GetAttribute("href")))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		node.SetAttribute("href", call.Argument(0).String())
		r.notifyMutation()
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)

	for _, part := range []string{"protocol", "host", "hostname", "port", "pathname", "search", "hash", "origin"} {
		part := part
		getter := func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil || !node.HasAttribute("href") {
				if part == "origin" {
					return r.vm.ToValue("")
				}
				return r.vm.ToValue(urlPart(nil, part))
			}
			u, _ := r.anchorURL(node)
			return r.vm.ToValue(urlPart(u, part))
		}
		setter := goja.Undefined()
		if part != "origin" {
			setter = r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				node := r.getNode(call.This)
				if node == nil {
					return goja.Undefined()
				}
				r.setAnchorURLPart(node, part, call.Argument(0).String())
				return goja.Undefined()
			})
		}
		_ = p.DefineAccessorProperty(part, r.vm.ToValue(getter), setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}

	for _, prop := range []string{"target", "rel", "download", "hreflang", "type", "name", "ping", "referrerPolicy"} {
		r.installStringReflection(p, prop, strings.ToLower(prop))
	}

	_ = p.DefineAccessorProperty("text", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.TextContent())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		r.setNodeTextContent(node, call.Argument(0).String())
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = p.Set("toString", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.HasAttribute("href") {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(r.resolveURLString(node.GetAttribute("href")))
	})
}

// setNodeTextContent replaces a node's children with one text node, charging
// the insertion through the same admission path as textContent.
func (r *Runtime) setNodeTextContent(node *dom.Node, text string) {
	var children []*dom.Node
	var textNode *dom.Node
	if text != "" {
		textNode = node.Doc.NewText(text)
		children = append(children, textNode)
	}
	parentWasLive := r.nodeIsLive(node)
	r.admitInsert("text", node, children, func() {
		var old []*dom.Node
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			old = append(old, c)
		}
		for _, c := range old {
			node.RemoveChild(c)
		}
		if parentWasLive {
			r.releaseNodes(old)
		}
		if textNode != nil {
			node.AppendChild(textNode)
		}
	})
	r.notifyMutation()
}

// ---------------------------------------------------------------------------
// HTMLImageElement
// ---------------------------------------------------------------------------

func (r *Runtime) installImageInterface(p *goja.Object) {
	_ = p.DefineAccessorProperty("src", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.HasAttribute("src") {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(r.resolveURLString(node.GetAttribute("src")))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		raw := call.Argument(0).String()
		node.SetAttribute("src", raw)
		r.notifyMutation()
		r.startImageLoad(node, raw)
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = p.DefineAccessorProperty("currentSrc", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		if st := r.imageStates[node]; st != nil {
			switch st.status {
			case imageLoading:
				return r.vm.ToValue("")
			case imageLoaded, imageFailed:
				return r.vm.ToValue(st.currentSrc)
			}
		}
		if node.HasAttribute("src") {
			return r.vm.ToValue(r.resolveURLString(node.GetAttribute("src")))
		}
		return r.vm.ToValue("")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)

	// width/height reflect the attribute and fall back to the intrinsic size
	// once a load has completed. Chromium's attribute-less value is the
	// laid-out box, which this binding cannot see (see §9).
	for _, dim := range []string{"width", "height"} {
		dim := dim
		_ = p.DefineAccessorProperty(dim, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return r.vm.ToValue(0)
			}
			if node.HasAttribute(dim) {
				if v, ok := parseHTMLUnsigned(node.GetAttribute(dim)); ok {
					return r.vm.ToValue(v)
				}
				return r.vm.ToValue(0)
			}
			if st := r.imageStates[node]; st != nil && st.status == imageLoaded {
				if dim == "width" {
					return r.vm.ToValue(st.naturalW)
				}
				return r.vm.ToValue(st.naturalH)
			}
			return r.vm.ToValue(0)
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Undefined()
			}
			v := call.Argument(0).ToInteger()
			if v < 0 {
				v = 0
			}
			node.SetAttribute(dim, strconv.FormatInt(v, 10))
			r.notifyMutation()
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}

	for _, dim := range []string{"naturalWidth", "naturalHeight"} {
		dim := dim
		_ = p.DefineAccessorProperty(dim, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return r.vm.ToValue(0)
			}
			st := r.imageStates[node]
			if st == nil || st.status != imageLoaded {
				return r.vm.ToValue(0)
			}
			if dim == "naturalWidth" {
				return r.vm.ToValue(st.naturalW)
			}
			return r.vm.ToValue(st.naturalH)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}

	_ = p.DefineAccessorProperty("complete", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue(true)
		}
		st := r.imageStates[node]
		return r.vm.ToValue(st == nil || st.status != imageLoading)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)

	r.installEnumReflection(p, "loading", "loading", "auto", []string{"eager", "lazy"})
	r.installEnumReflection(p, "decoding", "decoding", "auto", []string{"sync", "async", "auto"})
	for _, prop := range []string{"alt", "srcset", "sizes", "name", "useMap", "border"} {
		r.installStringReflection(p, prop, strings.ToLower(prop))
	}
	r.installUnsignedReflection(p, "hspace", "hspace", 0, nil)
	r.installUnsignedReflection(p, "vspace", "vspace", 0, nil)
	r.installBoolReflection(p, "isMap", "ismap")
	r.installURLReflection(p, "longDesc", "longdesc")
	r.installReferrerPolicyReflection(p)

	// x/y are the image's position in the document. Layout geometry does not
	// reach internal/js (see §9), so a detached image answers 0, which is what
	// Chromium answers for the only case the binding can see.
	for _, prop := range []string{"x", "y"} {
		_ = p.DefineAccessorProperty(prop, r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return r.vm.ToValue(0)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}

	// crossOrigin is a nullable enum: absent reads null, an unknown value
	// reads "anonymous" (measured), and assigning null removes the attribute.
	_ = p.DefineAccessorProperty("crossOrigin", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.HasAttribute("crossorigin") {
			return goja.Null()
		}
		if strings.EqualFold(node.GetAttribute("crossorigin"), "use-credentials") {
			return r.vm.ToValue("use-credentials")
		}
		return r.vm.ToValue("anonymous")
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		if v := call.Argument(0); goja.IsNull(v) || goja.IsUndefined(v) {
			node.RemoveAttribute("crossorigin")
		} else {
			node.SetAttribute("crossorigin", v.String())
		}
		r.notifyMutation()
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = p.Set("decode", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			p := r.newPromiseObj()
			r.rejectPromise(p, r.encodingError("The image element is not usable."))
			return p.obj
		}
		st := r.imageStates[node]
		if st != nil {
			switch st.status {
			case imageLoaded:
				p := r.newPromiseObj()
				r.resolvePromise(p, goja.Undefined())
				return p.obj
			case imageLoading:
				p := r.newPromiseObj()
				st.waiters = append(st.waiters, p)
				return p.obj
			}
		}
		p := r.newPromiseObj()
		r.rejectPromise(p, r.encodingError("The image cannot be decoded."))
		return p.obj
	})

	// onload/onerror are handler properties backed by a real DOM listener, so
	// dispatch order follows registration like addEventListener.
	r.installEventHandler(p, "load")
	r.installEventHandler(p, "error")
}

// installReferrerPolicyReflection is the enumerated referrer policy shared by
// <img> and <a>. Unknown and missing values read "".
func (r *Runtime) installReferrerPolicyReflection(p *goja.Object) {
	r.installEnumReflection(p, "referrerPolicy", "referrerpolicy", "", []string{
		"no-referrer", "no-referrer-when-downgrade", "origin",
		"origin-when-cross-origin", "same-origin", "strict-origin",
		"strict-origin-when-cross-origin", "unsafe-url",
	})
}

// installImageConstructor publishes window.Image. The constructor returns a
// detached <img>, applies width/height when arguments are supplied, and shares
// HTMLImageElement.prototype so `new Image() instanceof HTMLImageElement`.
func (r *Runtime) installImageConstructor() {
	imgProto := r.protoForTag("img")
	_ = r.vm.Set("__imgProto__", imgProto)
	v, err := r.vm.RunString(`(function(proto) {
		function Image() {
			var el = document.__createImage__();
			if (arguments.length > 0) el.width = arguments[0];
			if (arguments.length > 1) el.height = arguments[1];
			return el;
		}
		Object.defineProperty(Image, 'prototype', { value: proto, writable: false, enumerable: false, configurable: false });
		return Image;
	})(__imgProto__)`)
	_ = r.vm.Set("__imgProto__", nil)
	if err == nil && v != nil {
		_ = r.vm.Set("Image", v)
	}
}

func (r *Runtime) imageState(node *dom.Node) *imageLoadState {
	st := r.imageStates[node]
	if st == nil {
		st = &imageLoadState{status: imageIdle}
		r.imageStates[node] = st
	}
	return st
}

// startImageLoad begins a script-image load. The state flips to loading
// synchronously (so `complete` reads false for the rest of the task, as
// Chromium does) and settles in a microtask, because a loader may answer from
// another goroutine.
func (r *Runtime) startImageLoad(node *dom.Node, raw string) {
	st := r.imageState(node)
	st.gen++
	gen := st.gen
	st.status = imageLoading
	st.src = raw
	st.currentSrc = ""
	st.naturalW, st.naturalH = 0, 0

	if raw == "" {
		r.queueImageResult(node, st, gen, LoadedImage{Src: "", Err: errImageEmpty})
		return
	}
	abs, _, ok := r.resolveURL(raw)
	if !ok {
		r.queueImageResult(node, st, gen, LoadedImage{Src: raw, Err: errImageUnparseable})
		return
	}
	r.mu.Lock()
	loader := r.opts.ImageLoader
	r.mu.Unlock()
	if loader == nil {
		r.queueImageResult(node, st, gen, LoadedImage{Src: abs, Err: errImageNoLoader})
		return
	}
	loader.LoadScriptImage(abs, func(res LoadedImage) {
		r.queueMicrotask(func() error {
			if st.gen != gen {
				return nil
			}
			r.applyImageResult(node, st, res)
			return nil
		})
	})
}

var (
	errImageEmpty       = errors.New("js: empty image source")
	errImageUnparseable = errors.New("js: image source is not a valid URL")
	errImageNoLoader    = errors.New("js: no image loader is installed")
)

func (r *Runtime) queueImageResult(node *dom.Node, st *imageLoadState, gen int, res LoadedImage) {
	r.queueMicrotask(func() error {
		if st.gen != gen {
			return nil
		}
		r.applyImageResult(node, st, res)
		return nil
	})
}

func (r *Runtime) applyImageResult(node *dom.Node, st *imageLoadState, res LoadedImage) {
	if res.Err != nil {
		st.status = imageFailed
		st.currentSrc = res.Src
		st.naturalW, st.naturalH = 0, 0
		reason := r.encodingError(res.Err.Error())
		for _, p := range st.waiters {
			r.rejectPromise(p, reason)
		}
		st.waiters = nil
		dom.DispatchEvent(node, dom.NewEvent("error", false, false))
		return
	}
	st.status = imageLoaded
	st.currentSrc = res.Src
	st.naturalW, st.naturalH = res.Width, res.Height
	for _, p := range st.waiters {
		r.resolvePromise(p, goja.Undefined())
	}
	st.waiters = nil
	dom.DispatchEvent(node, dom.NewEvent("load", false, false))
}

// encodingError is the DOMException-shaped rejection decode() reports, which
// Chromium names EncodingError.
func (r *Runtime) encodingError(msg string) goja.Value {
	e := r.vm.NewGoError(errors.New(msg))
	_ = e.Set("name", "EncodingError")
	return e
}

// installEventHandler wires on<typ> as a real listener on first assignment.
func (r *Runtime) installEventHandler(p *goja.Object, typ string) {
	_ = p.DefineAccessorProperty("on"+typ, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		if hs := r.onHandlers[node]; hs != nil {
			if h := hs[typ]; h != nil && h.fn != nil {
				return h.fn
			}
		}
		return goja.Null()
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		hs := r.onHandlers[node]
		if hs == nil {
			hs = make(map[string]*jsEventHandler)
			r.onHandlers[node] = hs
		}
		h := hs[typ]
		if h == nil {
			h = &jsEventHandler{key: node}
			hs[typ] = h
		}
		v := call.Argument(0)
		if goja.IsNull(v) || goja.IsUndefined(v) {
			h.fn = nil
			return goja.Undefined()
		}
		h.fn = v
		if !h.registered {
			h.registered = true
			key := h.key
			dom.AddEventListenerWithKey(node, typ, func(e *dom.Event) {
				state := r.onHandlers[key]
				if state == nil {
					return
				}
				handler := state[typ]
				if handler == nil || handler.fn == nil {
					return
				}
				fn, ok := goja.AssertFunction(handler.fn)
				if !ok {
					return
				}
				_, _ = fn(r.wrapNode(key), r.wrapEvent(e))
			}, false, key)
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// ---------------------------------------------------------------------------
// HTMLInputElement
// ---------------------------------------------------------------------------

func (r *Runtime) installInputInterface(p *goja.Object) {
	r.installInputValueAccessors(p)

	r.installStringReflection(p, "defaultValue", "value")
	r.installStringReflection(p, "name", "name")
	r.installStringReflection(p, "placeholder", "placeholder")
	r.installStringReflection(p, "alt", "alt")
	r.installStringReflection(p, "min", "min")
	r.installStringReflection(p, "max", "max")
	r.installStringReflection(p, "step", "step")
	r.installStringReflection(p, "pattern", "pattern")
	r.installURLReflection(p, "src", "src")

	r.installEnumReflection(p, "type", "type", "text", inputTypeValues)
	r.installBoolReflection(p, "defaultChecked", "checked")
	r.installBoolReflection(p, "disabled", "disabled")
	r.installBoolReflection(p, "required", "required")
	r.installBoolReflection(p, "multiple", "multiple")
	r.installBoolReflection(p, "readOnly", "readonly")

	// maxLength reads -1 when absent/invalid and throws IndexSizeError on a
	// negative assignment; size defaults to 20, throws on 0, and resets a
	// negative assignment to the default.
	r.installUnsignedReflection(p, "maxLength", "maxlength", -1, func(v int64) (int64, *goja.Object) {
		if v < 0 {
			return 0, r.domError("IndexSizeError", "Failed to set the 'maxLength' property on 'HTMLInputElement': The value provided is negative.")
		}
		return v, nil
	})
	r.installUnsignedReflection(p, "size", "size", 20, func(v int64) (int64, *goja.Object) {
		if v == 0 {
			return 0, r.domError("IndexSizeError", "Failed to set the 'size' property on 'HTMLInputElement': The value provided is 0, which is an invalid size.")
		}
		if v < 0 {
			return 20, nil
		}
		return v, nil
	})

	_ = p.DefineAccessorProperty("form", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		for p := node; p != nil; p = p.Parent {
			if p.Element() && p.Data == "form" {
				return r.wrapNode(p)
			}
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// inputTypeValues is the HTML input type keyword set. An attribute outside it
// reads back as "text", as Chromium's limited-to-known-values reflection.
var inputTypeValues = []string{
	"hidden", "text", "search", "tel", "url", "email", "password", "date",
	"month", "week", "time", "datetime-local", "number", "range", "color",
	"checkbox", "radio", "file", "submit", "image", "reset", "button",
}

// installInputValueAccessors keeps the live-value contract TASK-030/031/037
// established: `value` reads and writes the value attribute (with the
// checkbox/radio "on" fallback), distinct from defaultValue, which never
// invents a value. See §9 for the measured Chromium divergence.
func (r *Runtime) installInputValueAccessors(p *goja.Object) {
	_ = r.vm.Set("__inputProto__", p)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__inputProto__, 'checked', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return false;
				return document.__getChecked__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setChecked__(nid, !!v);
			},
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(__inputProto__, 'value', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getValue__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setValue__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__inputProto__", nil)
}

// domError builds a DOMException-shaped object with the given name, which is
// how internal/dom reports the IndexSize/InvalidCharacter errors Chromium
// throws from these setters.
func (r *Runtime) domError(name, msg string) *goja.Object {
	e := r.vm.NewTypeError(msg)
	_ = e.Set("name", name)
	return e
}
