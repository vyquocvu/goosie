package js

import (
	"errors"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
	"golang.org/x/net/html/charset"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
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
	realm      *Runtime
	registered bool
	key        *dom.Node
	content    string
	hasContent bool
	compiled   bool
}

// globalEventHandlerTypes is the GlobalEventHandlers IDL surface shared by
// HTML elements, Document, and Window in the HTML Standard.
var globalEventHandlerTypes = []string{
	"abort", "auxclick", "beforeinput", "beforematch", "beforetoggle", "blur",
	"cancel", "canplay", "canplaythrough", "change", "click", "close", "command",
	"contextlost", "contextmenu", "contextrestored", "copy", "cuechange", "cut",
	"dblclick", "drag", "dragend", "dragenter", "dragleave", "dragover", "dragstart",
	"drop", "durationchange", "emptied", "ended", "error", "focus", "formdata",
	"input", "invalid", "keydown", "keypress", "keyup", "load", "loadeddata",
	"loadedmetadata", "loadstart", "mousedown", "mouseenter", "mouseleave", "mousemove",
	"mouseout", "mouseover", "mouseup", "paste", "pause", "play", "playing", "progress",
	"ratechange", "reset", "resize", "scroll", "scrollend", "securitypolicyviolation",
	"seeked", "seeking", "select", "slotchange", "stalled", "submit", "suspend",
	"timeupdate", "toggle", "volumechange", "waiting", "webkitanimationend",
	"webkitanimationiteration", "webkitanimationstart", "webkittransitionend", "wheel",
}

var windowEventHandlerTypes = []string{
	"afterprint", "beforeprint", "beforeunload", "hashchange", "languagechange", "message",
	"messageerror", "offline", "online", "pagehide", "pagereveal", "pageshow", "pageswap",
	"popstate", "rejectionhandled", "storage", "unhandledrejection", "unload",
}

func (r *Runtime) installGlobalEventHandlerAttributes() {
	for _, typ := range globalEventHandlerTypes {
		r.installEventHandler(r.htmlElementProto, typ)
		r.installEventHandler(r.svgElementProto, typ)
		r.installEventHandler(r.documentProto, typ)
	}
	r.installEventHandler(r.documentProto, "readystatechange")
	r.installEventHandler(r.documentProto, "visibilitychange")
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
	r.attrProto = r.newChildProto(r.nodeProto)
	r.elementProto = r.newChildProto(r.nodeProto)
	r.svgElementProto = r.newChildProto(r.elementProto)
	r.htmlElementProto = r.newChildProto(r.elementProto)
	r.unknownElementProto = r.newChildProto(r.htmlElementProto)
	r.documentProto = r.newChildProto(r.nodeProto)
	_ = r.documentProto.Set("createEvent", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'createEvent': 1 argument required"))
		}
		return r.jsCreateLegacyEvent(call.Arguments[0].String())
	})
	_ = r.documentProto.Set("getSelection", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Doc == nil {
			return goja.Null()
		}
		return r.wrapSelection(r.selectionForDocument(node.Doc))
	})
	_ = r.documentProto.Set("execCommand", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Doc == nil {
			return r.vm.ToValue(false)
		}
		return r.execCommand(node.Doc, call.Arguments)
	})
	_ = r.documentProto.Set("queryCommandSupported", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return r.vm.ToValue(false)
		}
		return r.vm.ToValue(supportedEditCommand(call.Arguments[0].String()))
	})
	_ = r.documentProto.Set("queryCommandEnabled", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if len(call.Arguments) == 0 || node == nil || node.Doc == nil || !supportedEditCommand(call.Arguments[0].String()) {
			return r.vm.ToValue(false)
		}
		if node.Doc.DesignMode {
			return r.vm.ToValue(true)
		}
		selection := r.selectionForDocument(node.Doc)
		if selection == nil || len(selection.ranges) == 0 {
			return r.vm.ToValue(false)
		}
		return r.vm.ToValue(editingHostFor(selection.ranges[0].start.container) != nil)
	})
	_ = r.documentProto.Set("queryCommandIndeterm", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeDocument || node.Doc == nil || len(call.Arguments) == 0 {
			return r.vm.ToValue(false)
		}
		_, indeterminate := r.queryEditingFormatState(node.Doc, call.Arguments[0].String())
		return r.vm.ToValue(indeterminate)
	})
	_ = r.documentProto.Set("queryCommandState", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeDocument || node.Doc == nil || len(call.Arguments) == 0 {
			return r.vm.ToValue(false)
		}
		state, _ := r.queryEditingFormatState(node.Doc, call.Arguments[0].String())
		return r.vm.ToValue(state)
	})
	_ = r.documentProto.Set("queryCommandValue", func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue("")
	})
	_ = r.documentProto.DefineAccessorProperty("customElementRegistry", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeDocument || node.Doc == nil {
			return goja.Null()
		}
		return r.customElementRegistryObject(r.customElementRegistryForDocument(node.Doc))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.documentProto.DefineAccessorProperty("readyState", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Doc == nil || node.Doc != r.domDoc {
			return r.vm.ToValue("complete")
		}
		return r.vm.ToValue(r.readyState())
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.documentProto.DefineAccessorProperty("compatMode", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Doc == nil || node.Doc.QuirksMode {
			return r.vm.ToValue("BackCompat")
		}
		return r.vm.ToValue("CSS1Compat")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.documentProto.DefineAccessorProperty("characterSet", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Doc == nil || node.Doc.CharacterSet == "" {
			return r.vm.ToValue("UTF-8")
		}
		return r.vm.ToValue(node.Doc.CharacterSet)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	for _, name := range []string{"charset", "inputEncoding"} {
		_ = r.documentProto.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil || node.Doc == nil || node.Doc.CharacterSet == "" {
				return r.vm.ToValue("UTF-8")
			}
			return r.vm.ToValue(node.Doc.CharacterSet)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = r.documentProto.DefineAccessorProperty("contentType", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Doc == nil || node.Doc.ContentType == "" {
			return r.vm.ToValue("text/html")
		}
		return r.vm.ToValue(node.Doc.ContentType)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.documentProto.DefineAccessorProperty("domain", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeDocument || node.Doc == nil {
			return r.vm.ToValue("")
		}
		if domain := r.documentDomains[node.Doc]; domain != "" {
			return r.vm.ToValue(domain)
		}
		if node.Doc != r.domDoc {
			return r.vm.ToValue("")
		}
		if documentURL := r.documentURL(); documentURL != nil {
			return r.vm.ToValue(strings.ToLower(documentURL.Hostname()))
		}
		return r.vm.ToValue("")
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeDocument || node.Doc == nil || !r.documentHasBrowsingContext(node.Doc) || r.documentDomainSandboxed() {
			panic(r.domError("SecurityError", "document.domain cannot be set in this document"))
		}
		current := r.documentURL()
		if node.Doc != r.domDoc || current == nil || current.Hostname() == "" {
			panic(r.domError("SecurityError", "document.domain requires a tuple origin"))
		}
		effectiveDomain := strings.ToLower(current.Hostname())
		if relaxed := r.documentDomains[node.Doc]; relaxed != "" {
			effectiveDomain = relaxed
		}
		candidate, ok := parseDocumentDomain(call.Argument(0).String())
		if !ok || !validDocumentDomainSuffix(candidate, effectiveDomain) {
			panic(r.domError("SecurityError", "The supplied domain is not a registrable suffix of this document's host"))
		}
		if r.documentDomains == nil {
			r.documentDomains = make(map[*dom.Document]string)
		}
		r.documentDomains[node.Doc] = candidate
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.documentProto.DefineAccessorProperty("designMode", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node != nil && node.Doc != nil && node.Doc.DesignMode {
			return r.vm.ToValue("on")
		}
		return r.vm.ToValue("off")
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Doc == nil {
			return goja.Undefined()
		}
		node.Doc.DesignMode = asciiEqualFold(call.Argument(0).String(), "on")
		r.notifyMutation()
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.documentProto.DefineAccessorProperty("location", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeDocument || node.Doc == nil {
			return goja.Null()
		}
		return r.locationForDocument(node.Doc)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeDocument || node.Doc == nil {
			return goja.Undefined()
		}
		location := r.locationForDocument(node.Doc)
		if object, ok := location.(*goja.Object); ok {
			_ = object.Set("href", call.Argument(0).String())
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_FALSE)
	r.documentFragmentProto = r.newChildProto(r.nodeProto)
	r.documentTypeProto = r.newChildProto(r.nodeProto)
	_ = r.documentTypeProto.DefineAccessorProperty("name", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeDoctype {
			return goja.Undefined()
		}
		return r.vm.ToValue(node.DoctypeName)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	for name, read := range map[string]func(*dom.Node) string{
		"publicId": func(node *dom.Node) string { return node.PublicID },
		"systemId": func(node *dom.Node) string { return node.SystemID },
	} {
		name, read := name, read
		_ = r.documentTypeProto.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil || node.Type != dom.NodeDoctype {
				return goja.Undefined()
			}
			return r.vm.ToValue(read(node))
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
	r.characterDataProto = r.newChildProto(r.nodeProto)
	r.textProto = r.newChildProto(r.characterDataProto)
	r.commentProto = r.newChildProto(r.characterDataProto)
	r.processingInstructionProto = r.newChildProto(r.characterDataProto)
	_ = r.processingInstructionProto.DefineAccessorProperty("target", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Type != dom.NodeProcessingInstruction {
			return goja.Undefined()
		}
		return r.vm.ToValue(node.Data)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	r.defineEventTargetConstructor()
	nodeConstructor := r.defineConstructor("Node", r.nodeProto)
	r.defineConstructor("Attr", r.attrProto)
	for name, value := range map[string]int{
		"ELEMENT_NODE": 1, "ATTRIBUTE_NODE": 2, "TEXT_NODE": 3,
		"CDATA_SECTION_NODE": 4, "ENTITY_REFERENCE_NODE": 5, "ENTITY_NODE": 6,
		"PROCESSING_INSTRUCTION_NODE": 7, "COMMENT_NODE": 8, "DOCUMENT_NODE": 9,
		"DOCUMENT_TYPE_NODE": 10, "DOCUMENT_FRAGMENT_NODE": 11, "NOTATION_NODE": 12,
	} {
		v := r.vm.ToValue(value)
		_ = nodeConstructor.Set(name, v)
		_ = r.nodeProto.Set(name, v)
	}
	for name, value := range map[string]int{
		"DOCUMENT_POSITION_DISCONNECTED":            1,
		"DOCUMENT_POSITION_PRECEDING":               2,
		"DOCUMENT_POSITION_FOLLOWING":               4,
		"DOCUMENT_POSITION_CONTAINS":                8,
		"DOCUMENT_POSITION_CONTAINED_BY":            16,
		"DOCUMENT_POSITION_IMPLEMENTATION_SPECIFIC": 32,
	} {
		v := r.vm.ToValue(value)
		_ = nodeConstructor.Set(name, v)
		_ = r.nodeProto.Set(name, v)
	}
	r.defineConstructor("Element", r.elementProto)
	r.defineConstructor("SVGElement", r.svgElementProto)
	r.defineConstructor("HTMLElement", r.htmlElementProto)
	r.defineConstructor("HTMLUnknownElement", r.unknownElementProto)
	r.defineConstructor("Document", r.documentProto)
	r.defineConstructor("DocumentType", r.documentTypeProto)
	r.xmlDocumentProto = r.newChildProto(r.documentProto)
	r.defineConstructor("XMLDocument", r.xmlDocumentProto)
	if r.domDoc != nil && r.domDoc.IsXML {
		if document, ok := r.vm.Get("document").(*goja.Object); ok {
			_ = document.SetPrototype(r.xmlDocumentProto)
		}
	}
	r.defineConstructor("DocumentFragment", r.documentFragmentProto)
	r.defineConstructor("CharacterData", r.characterDataProto)
	r.defineConstructor("ProcessingInstruction", r.processingInstructionProto)
	r.defineConstructor("Text", r.textProto)
	r.defineConstructor("Comment", r.commentProto)
	r.installCharacterDataAccessors()

	// The interfaces whose constructors a page probes even before it creates
	// an element. Per-tag prototypes for other tags are minted lazily by
	// protoForTag, so a document pays only for the tags it actually wraps.
	for _, tag := range []string{
		"html", "head", "body", "title", "div", "span", "p", "a", "img",
		"input", "button", "form", "select", "option", "textarea", "canvas",
		"video", "script", "style", "link", "meta", "dialog", "h1", "col", "del", "dir", "font", "td",
	} {
		r.protoForTag(tag)
	}
	r.setupHTMLFormCollectionInterfaces()

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

func (r *Runtime) installCharacterDataAccessors() {
	p := r.characterDataProto
	_ = p.DefineAccessorProperty("data", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		return goja.StringFromUTF16(characterDataCodeUnits(node))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node != nil {
			value := call.Argument(0)
			data := characterDataValueUnits(value)
			if goja.IsNull(value) {
				data = nil
			}
			r.replaceCharacterDataUnits(node, 0, len(characterDataCodeUnits(node)), data)
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = p.DefineAccessorProperty("length", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue(0)
		}
		return r.vm.ToValue(len(characterDataCodeUnits(node)))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = p.Set("substringData", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.requireCharacterData(call.This, "substringData")
		requireCharacterDataArgs(r, call, "substringData", 2)
		units := characterDataCodeUnits(node)
		length := len(units)
		offset := int64(webIDLUnsignedLong(call.Argument(0)))
		count := int64(webIDLUnsignedLong(call.Argument(1)))
		if offset < 0 || offset > int64(length) {
			panic(r.domError("IndexSizeError", "The offset is greater than the node length"))
		}
		start := int(offset)
		end := length
		if count < int64(length-start) {
			end = start + int(count)
		}
		return goja.StringFromUTF16(units[start:end])
	}))
	_ = p.Set("appendData", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.requireCharacterData(call.This, "appendData")
		requireCharacterDataArgs(r, call, "appendData", 1)
		r.replaceCharacterDataUnits(node, len(characterDataCodeUnits(node)), 0, characterDataValueUnits(call.Argument(0)))
		return goja.Undefined()
	}))
	_ = p.Set("insertData", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.requireCharacterData(call.This, "insertData")
		requireCharacterDataArgs(r, call, "insertData", 2)
		offset := webIDLUnsignedLong(call.Argument(0))
		if offset > len(characterDataCodeUnits(node)) {
			panic(r.domError("IndexSizeError", "The offset is greater than the node length"))
		}
		r.replaceCharacterDataUnits(node, offset, 0, characterDataValueUnits(call.Argument(1)))
		return goja.Undefined()
	}))
	_ = p.Set("deleteData", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.requireCharacterData(call.This, "deleteData")
		requireCharacterDataArgs(r, call, "deleteData", 2)
		r.replaceCharacterDataUnits(node, webIDLUnsignedLong(call.Argument(0)), webIDLUnsignedLong(call.Argument(1)), nil)
		return goja.Undefined()
	}))
	_ = p.Set("replaceData", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.requireCharacterData(call.This, "replaceData")
		requireCharacterDataArgs(r, call, "replaceData", 3)
		r.replaceCharacterDataUnits(node, webIDLUnsignedLong(call.Argument(0)), webIDLUnsignedLong(call.Argument(1)), characterDataValueUnits(call.Argument(2)))
		return goja.Undefined()
	}))
}

func requireCharacterDataArgs(r *Runtime, call goja.FunctionCall, operation string, count int) {
	if len(call.Arguments) < count {
		panic(r.vm.NewTypeError("Failed to execute '" + operation + "' on 'CharacterData': " + strconv.Itoa(count) + " arguments required"))
	}
}

func (r *Runtime) requireCharacterData(value goja.Value, operation string) *dom.Node {
	if node := r.getNode(value); node != nil && (node.Type == dom.NodeText || node.Type == dom.NodeComment || node.Type == dom.NodeCDATASection || node.Type == dom.NodeProcessingInstruction) {
		return node
	}
	panic(r.vm.NewTypeError("Failed to execute '" + operation + "' on 'CharacterData': Illegal invocation"))
}

func characterDataLength(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func characterDataCodeUnits(node *dom.Node) []uint16 {
	if node == nil {
		return nil
	}
	if node.DataUTF16 != nil {
		return node.DataUTF16
	}
	return utf16.Encode([]rune(node.DataContent))
}

func characterDataValueUnits(value goja.Value) []uint16 {
	str, ok := value.(goja.String)
	if !ok {
		return utf16.Encode([]rune(value.ToString().String()))
	}
	units := make([]uint16, str.Length())
	for i := range units {
		units[i] = str.CharAt(i)
	}
	return units
}

func setCharacterDataCodeUnits(node *dom.Node, units []uint16) {
	if node == nil {
		return
	}
	decoded := string(utf16.Decode(units))
	node.DataContent = decoded
	node.SyncTextareaValueFromTextMutation()
	if unitsEqual(utf16.Encode([]rune(decoded)), units) {
		node.DataUTF16 = nil
		return
	}
	node.DataUTF16 = append([]uint16(nil), units...)
}

func unitsEqual(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *Runtime) replaceCharacterData(node *dom.Node, offset, count int, inserted string) {
	r.replaceCharacterDataUnits(node, offset, count, utf16.Encode([]rune(inserted)))
}

func (r *Runtime) replaceCharacterDataUnits(node *dom.Node, offset, count int, insertedUnits []uint16) {
	units := characterDataCodeUnits(node)
	oldValue := string(utf16.Decode(units))
	if offset < 0 || offset > len(units) {
		panic(r.domError("IndexSizeError", "The offset is greater than the node length"))
	}
	if count < 0 {
		count = 0
	}
	if count > len(units)-offset {
		count = len(units) - offset
	}
	end := offset + count
	updated := make([]uint16, 0, len(units)-count+len(insertedUnits))
	updated = append(updated, units[:offset]...)
	updated = append(updated, insertedUnits...)
	updated = append(updated, units[end:]...)
	r.adjustLiveRangesForCharacterData(node, offset, end, len(insertedUnits))
	setCharacterDataCodeUnits(node, updated)
	r.queueMutationRecord(MutationRecord{Type: "characterData", Target: node, OldValue: &oldValue})
	if node.Doc != nil {
		node.Doc.MarkQueryDirty()
	}
	r.notifyMutation()
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
	r.defineInterfaceToStringTag(name, p)
	if c, ok := r.ifaceCtors[name]; ok {
		return c
	}
	if name == "HTMLElement" || r.isHTMLTagInterface(name) {
		// HTML element interface constructors remain illegal on their own, but
		// serve as the base constructor for autonomous and customized built-ins.
		ctor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
			definition, owner := r.customElementDefinitionForConstructor(call.NewTarget)
			if call.NewTarget == nil || call.NewTarget == r.ifaceCtors[name] {
				panic(r.vm.NewTypeError("Illegal constructor"))
			}
			if definition == nil {
				panic(r.vm.NewTypeError("Illegal constructor"))
			}
			isHTMLElement, mismatch := r.customElementConstructorMatchesInterface(definition, name)
			if mismatch {
				panic(r.vm.NewTypeError("Custom element constructor does not match its element interface"))
			}
			if !isHTMLElement && r.customHTMLElementConstruction == 0 {
				newTargetPrototype, known := r.htmlConstructorNewTargetPrototypes[call.NewTarget]
				primitivePrototype := !known || !isJSObject(newTargetPrototype)
				if name != "HTMLElement" && !primitivePrototype {
					panic(r.vm.NewTypeError("Custom element constructor must inherit the matching HTML element interface"))
				}
				// Reflect.construct() can call an HTML constructor with a
				// registered NewTarget whose prototype is not in the matching HTML
				// interface chain. Return the ordinary object from ECMAScript
				// construction with HTMLConstructor's realm-specific fallback proto.
				r.applyHTMLConstructorFallbackPrototype(call.This, definition, r.functionRealm(call.NewTarget, definition.constructorRealm), primitivePrototype)
				return call.This
			}
			if !isHTMLElement {
				panic(r.vm.NewTypeError("Custom element constructor must inherit the matching HTML element interface"))
			}
			if r.customHTMLElementConstruction == 0 {
				newTargetPrototype, known := r.htmlConstructorNewTargetPrototypes[call.NewTarget]
				if !known || !isJSObject(newTargetPrototype) {
					r.applyHTMLConstructorFallbackPrototype(call.This, definition, r.functionRealm(call.NewTarget, definition.constructorRealm), true)
				}
				if !r.allocateDirectCustomElement(call.This, definition, owner) {
					panic(r.vm.NewTypeError("Illegal constructor"))
				}
				return call.This
			}
			if len(r.customConstructionNodes) > 0 {
				construction := len(r.customConstructionNodes) - 1
				if construction < len(r.customConstructionAlready) && r.customConstructionAlready[construction] {
					if construction < len(r.customConstructionAllowAdditional) && r.customConstructionAllowAdditional[construction] {
						if !r.allocateDirectCustomElement(call.This, definition, owner) {
							panic(r.vm.NewTypeError("Illegal constructor"))
						}
						return call.This
					}
					panic(r.vm.NewTypeError("This custom element has already been constructed"))
				}
				if construction < len(r.customConstructionAlready) {
					r.customConstructionAlready[construction] = true
				}
				node := r.customConstructionNodes[construction]
				if nid, exists := r.wrapCache[node]; exists {
					if existing := r.nodeWrappers[nid]; existing != nil {
						_ = existing.SetPrototype(call.This.Prototype())
						return existing
					}
					_ = call.This.Set("__nid__", nid)
					r.nodeWrappers[nid] = call.This
				}
			}
			return call.This
		}).(*goja.Object)
		if !ok {
			return nil
		}
		_ = r.vm.Set("__htmlElementConstructor__", ctor)
		_ = r.vm.Set("__htmlElementConstructorName__", name)
		_, _ = r.vm.RunString(`Object.defineProperty(__htmlElementConstructor__, "name", { value: __htmlElementConstructorName__, configurable: true })`)
		_ = r.vm.Set("__htmlElementConstructor__", nil)
		_ = r.vm.Set("__htmlElementConstructorName__", nil)
		_ = ctor.Set("prototype", p)
		_ = p.Set("constructor", ctor)
		var htmlConstructor *goja.Object
		functionPrototype := r.vm.Get("Function").ToObject(r.vm).Get("prototype").ToObject(r.vm)
		defaultHasInstance := functionPrototype.GetSymbol(goja.SymHasInstance)
		ordinaryHasInstance := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
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
		proxyValue := r.vm.ToValue(r.vm.NewProxy(ctor, &goja.ProxyTrapConfig{
			GetSym: func(target *goja.Object, property *goja.Symbol, _ goja.Value) goja.Value {
				if property != goja.SymHasInstance {
					return target.GetSymbol(property)
				}
				hasInstance := inheritedSymbolValue(target, property)
				if hasInstance == nil || goja.IsUndefined(hasInstance) || !hasInstance.StrictEquals(defaultHasInstance) {
					return hasInstance
				}
				return ordinaryHasInstance
			},
			Construct: func(target *goja.Object, arguments []goja.Value, newTarget *goja.Object) *goja.Object {
				// ECMAScript normally reads newTarget.prototype while allocating
				// `this`, before entering an HTML element constructor. The HTML
				// constructor has registration and interface checks that must run
				// first, so guard construction at the Proxy boundary and only then
				// delegate allocation to Goja's ordinary construct operation.
				if newTarget == nil || newTarget == htmlConstructor {
					panic(r.vm.NewTypeError("Illegal constructor"))
				}
				definition, _ := r.customElementDefinitionForConstructor(newTarget)
				if definition == nil {
					panic(r.vm.NewTypeError("Illegal constructor"))
				}
				matches, mismatch := r.customElementConstructorMatchesInterface(definition, name)
				if mismatch {
					panic(r.vm.NewTypeError("Custom element constructor does not match its element interface"))
				}
				prototypeValue := newTarget.Get("prototype")
				primitivePrototype := !isJSObject(prototypeValue)
				if !matches && !(r.customHTMLElementConstruction == 0 && (name == "HTMLElement" || definition.extends != "" && primitivePrototype)) {
					panic(r.vm.NewTypeError("Custom element constructor must inherit the matching HTML element interface"))
				}
				constructor, ok := goja.AssertConstructor(target)
				if !ok {
					panic(r.vm.NewTypeError("Illegal constructor"))
				}
				allocationTarget := r.prepareHTMLConstructorNewTarget(newTarget, prototypeValue, definition)
				defer r.clearHTMLConstructorNewTarget(allocationTarget)
				instance, err := constructor(allocationTarget, arguments...)
				if err != nil {
					panic(err)
				}
				if primitivePrototype {
					r.applyHTMLConstructorFallbackPrototype(instance, definition, r.functionRealm(newTarget, definition.constructorRealm), true)
				}
				return instance
			},
		})).ToObject(r.vm)
		htmlConstructor = proxyValue
		_ = p.Set("constructor", htmlConstructor)
		r.ifaceCtors[name] = htmlConstructor
		_ = r.vm.Set(name, htmlConstructor)
		return htmlConstructor
	}
	if name == "Document" {
		ctor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
			doc := dom.NewDocument()
			doc.IsXML = true
			value := r.wrapDetachedDocument(doc)
			if object, ok := value.(*goja.Object); ok {
				return object
			}
			return r.vm.NewObject()
		}).(*goja.Object)
		if !ok {
			return nil
		}
		_ = ctor.Set("prototype", p)
		_ = p.Set("constructor", ctor)
		r.ifaceCtors[name] = ctor
		_ = r.vm.Set(name, ctor)
		return ctor
	}
	if name == "DocumentFragment" {
		ctor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
			doc := r.domDoc
			if doc == nil {
				doc = dom.NewDocument()
				doc.IsXML = true
			}
			value := r.wrapNode(doc.NewDocumentFragment())
			if object, ok := value.(*goja.Object); ok {
				return object
			}
			return r.vm.NewObject()
		}).(*goja.Object)
		if !ok {
			return nil
		}
		_ = ctor.Set("prototype", p)
		_ = p.Set("constructor", ctor)
		r.ifaceCtors[name] = ctor
		_ = r.vm.Set(name, ctor)
		return ctor
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

func isJSObject(value goja.Value) bool {
	_, ok := value.(*goja.Object)
	return ok
}

func (r *Runtime) prepareHTMLConstructorNewTarget(newTarget *goja.Object, prototypeValue goja.Value, definition *customElementDefinition) *goja.Object {
	allocationTarget := r.vm.ToValue(r.vm.NewProxy(newTarget, &goja.ProxyTrapConfig{
		Get: func(_ *goja.Object, property string, _ goja.Value) goja.Value {
			if property == "prototype" {
				return prototypeValue
			}
			return newTarget.Get(property)
		},
		GetSym: func(_ *goja.Object, property *goja.Symbol, _ goja.Value) goja.Value {
			return inheritedSymbolValue(newTarget, property)
		},
		Set: func(_ *goja.Object, property string, value goja.Value, _ goja.Value) bool {
			return newTarget.Set(property, value) == nil
		},
		Has: func(_ *goja.Object, property string) bool {
			return newTarget.Get(property) != nil && !goja.IsUndefined(newTarget.Get(property))
		},
	})).ToObject(r.vm)
	if r.iframeImportedFunctionAliases == nil {
		r.iframeImportedFunctionAliases = make(map[*goja.Object]*goja.Object)
	}
	r.iframeImportedFunctionAliases[allocationTarget] = newTarget
	if r.iframeImportedFunctionRealms == nil {
		r.iframeImportedFunctionRealms = make(map[*goja.Object]*Runtime)
	}
	r.iframeImportedFunctionRealms[allocationTarget] = r.functionRealm(newTarget, definition.constructorRealm)
	if r.htmlConstructorNewTargetPrototypes == nil {
		r.htmlConstructorNewTargetPrototypes = make(map[*goja.Object]goja.Value)
	}
	r.htmlConstructorNewTargetPrototypes[allocationTarget] = prototypeValue
	return allocationTarget
}

func (r *Runtime) clearHTMLConstructorNewTarget(allocationTarget *goja.Object) {
	delete(r.iframeImportedFunctionAliases, allocationTarget)
	delete(r.iframeImportedFunctionRealms, allocationTarget)
	delete(r.htmlConstructorNewTargetPrototypes, allocationTarget)
}

func (r *Runtime) isHTMLTagInterface(name string) bool {
	for _, iface := range htmlTagInterfaces {
		if iface == name {
			return true
		}
	}
	return false
}

// defineInterfaceToStringTag implements Web IDL's @@toStringTag property on
// interface prototypes. Object.prototype.toString consults this property, so
// platform objects stringify with their interface brand (for example,
// [object HTMLStyleElement]) instead of [object Object].
func (r *Runtime) defineInterfaceToStringTag(name string, proto *goja.Object) {
	if proto == nil || name == "" {
		return
	}
	_ = r.vm.Set("__goosieInterfacePrototype__", proto)
	_ = r.vm.Set("__goosieInterfaceName__", name)
	_, _ = r.vm.RunString(`Object.defineProperty(__goosieInterfacePrototype__, Symbol.toStringTag, {
		value: __goosieInterfaceName__, configurable: true
	});`)
	_ = r.vm.Set("__goosieInterfacePrototype__", nil)
	_ = r.vm.Set("__goosieInterfaceName__", nil)
}

// htmlTagInterfaces maps a tag to its Chromium interface. Tags not listed
// answer HTMLElement, which is what Chromium does for an unknown tag until a
// custom element is defined for it.
var htmlTagInterfaces = map[string]string{
	"a": "HTMLAnchorElement", "area": "HTMLAreaElement", "audio": "HTMLAudioElement", "base": "HTMLBaseElement",
	"body": "HTMLBodyElement", "br": "HTMLBRElement", "button": "HTMLButtonElement",
	"canvas": "HTMLCanvasElement", "caption": "HTMLTableCaptionElement", "data": "HTMLDataElement", "datalist": "HTMLDataListElement", "del": "HTMLModElement", "details": "HTMLDetailsElement", "div": "HTMLDivElement", "dl": "HTMLDListElement", "embed": "HTMLEmbedElement", "form": "HTMLFormElement",
	"head": "HTMLHeadElement", "hr": "HTMLHRElement", "html": "HTMLHtmlElement",
	"h1": "HTMLHeadingElement", "h2": "HTMLHeadingElement", "h3": "HTMLHeadingElement", "h4": "HTMLHeadingElement", "h5": "HTMLHeadingElement", "h6": "HTMLHeadingElement",
	"iframe": "HTMLIFrameElement", "img": "HTMLImageElement", "input": "HTMLInputElement", "ins": "HTMLModElement",
	"label": "HTMLLabelElement", "legend": "HTMLLegendElement", "li": "HTMLLIElement", "link": "HTMLLinkElement", "fieldset": "HTMLFieldSetElement",
	"map": "HTMLMapElement", "menu": "HTMLMenuElement", "meta": "HTMLMetaElement", "meter": "HTMLMeterElement", "ol": "HTMLOListElement", "optgroup": "HTMLOptGroupElement", "option": "HTMLOptionElement",
	"object": "HTMLObjectElement", "output": "HTMLOutputElement", "p": "HTMLParagraphElement", "picture": "HTMLPictureElement", "pre": "HTMLPreElement", "script": "HTMLScriptElement",
	"param": "HTMLParamElement", "progress": "HTMLProgressElement", "q": "HTMLQuoteElement", "blockquote": "HTMLQuoteElement",
	"select": "HTMLSelectElement", "source": "HTMLSourceElement", "span": "HTMLSpanElement", "style": "HTMLStyleElement", "template": "HTMLTemplateElement", "time": "HTMLTimeElement",
	"table": "HTMLTableElement", "tbody": "HTMLTableSectionElement", "td": "HTMLTableCellElement", "textarea": "HTMLTextAreaElement", "col": "HTMLTableColElement", "colgroup": "HTMLTableColElement",
	"tfoot": "HTMLTableSectionElement", "th": "HTMLTableCellElement", "thead": "HTMLTableSectionElement", "title": "HTMLTitleElement", "tr": "HTMLTableRowElement", "track": "HTMLTrackElement",
	"ul": "HTMLUListElement", "video": "HTMLVideoElement", "dialog": "HTMLDialogElement",
	"frame":    "HTMLFrameElement",
	"frameset": "HTMLFrameSetElement",
	"slot":     "HTMLSlotElement",
	"dir":      "HTMLDirectoryElement",
	"font":     "HTMLFontElement",
}

// protoForTag returns the per-tag interface prototype, creating it (and its
// constructor and reflected properties) on first use.
func (r *Runtime) protoForTag(tag string) *goja.Object {
	if p, ok := r.tagProtos[tag]; ok {
		return p
	}
	iface := htmlTagInterfaces[tag]
	if iface == "" {
		if tag != strings.ToLower(tag) || !isHTMLStandardElementName(tag) && dom.Lookup(tag) == dom.AtomUnknown && !strings.Contains(tag, "-") {
			r.tagProtos[tag] = r.unknownElementProto
			return r.unknownElementProto
		}
		r.tagProtos[tag] = r.htmlElementProto
		return r.htmlElementProto
	}
	if constructor := r.ifaceCtors[iface]; constructor != nil {
		if prototype, ok := constructor.Get("prototype").(*goja.Object); ok {
			r.tagProtos[tag] = prototype
			return prototype
		}
	}
	p := r.vm.NewObject()
	_ = p.SetPrototype(r.htmlElementProto)
	r.tagProtos[tag] = p
	r.installInterface(iface, p)
	r.defineConstructor(iface, p)
	return p
}

func (r *Runtime) ensureHTMLInterfaceConstructor(name string) {
	if r.ifaceCtors[name] != nil {
		return
	}
	for tag, interfaceName := range htmlTagInterfaces {
		if interfaceName == name {
			r.protoForTag(tag)
			if r.ifaceCtors[name] != nil {
				return
			}
		}
	}
}

// protoForNode returns the [[Prototype]] for one wrapper.
func (r *Runtime) protoForNode(node *dom.Node) *goja.Object {
	if node.Element() && node.Namespace == dom.NSSVG {
		return r.svgElementProto
	}
	if node.Element() && node.Namespace == dom.NSHTML {
		localName := node.Data
		if colon := strings.IndexByte(localName, ':'); colon >= 0 {
			localName = localName[colon+1:]
		}
		interfaceName := htmlTagInterfaces[localName]
		if interfaceName == "" {
			if isValidCustomElementName(localName) {
				interfaceName = "HTMLElement"
			} else {
				interfaceName = "HTMLUnknownElement"
			}
		}
		if prototype := r.iframeInterfacePrototype(node.Doc, interfaceName); prototype != nil {
			return prototype
		}
		return r.protoForTag(localName)
	}
	if node.Element() && (node.Namespace != dom.NSHTML || node.Doc != nil && node.Doc.IsXML) {
		return r.elementProto
	}
	if node.Type == dom.NodeDocument {
		if node.Doc != nil && node.Doc.IsXML && !node.Doc.IsDOMParserDocument && r.xmlDocumentProto != nil {
			return r.xmlDocumentProto
		}
		return r.documentProto
	}
	if node.Type == dom.NodeDoctype {
		return r.documentTypeProto
	}
	if node.Type == dom.NodeDocumentFragment {
		return r.documentFragmentProto
	}
	if node.Type == dom.NodeText || node.Type == dom.NodeCDATASection {
		return r.textProto
	}
	if node.Type == dom.NodeComment {
		return r.commentProto
	}
	if node.Type == dom.NodeProcessingInstruction {
		return r.processingInstructionProto
	}
	if node.Element() {
		return r.protoForTag(node.Data)
	}
	return r.nodeProto
}

func (r *Runtime) installInterface(iface string, p *goja.Object) {
	switch iface {
	case "HTMLDirectoryElement":
		r.installBoolReflection(p, "compact", "compact")
	case "HTMLFontElement":
		r.installStringReflection(p, "color", "color")
		r.installStringReflection(p, "face", "face")
		r.installStringReflection(p, "size", "size")
	case "HTMLScriptElement":
		r.installURLReflection(p, "src", "src")
		r.installStringReflection(p, "type", "type")
		r.installBoolReflection(p, "async", "async")
		r.installBoolReflection(p, "defer", "defer")
		r.installBoolReflection(p, "noModule", "nomodule")
	case "HTMLBodyElement", "HTMLFrameSetElement":
		r.installWindowReflectingBodyHandlers(p)
	case "HTMLAnchorElement":
		r.installAnchorInterface(p)
	case "HTMLAreaElement":
		r.installAreaInterface(p)
	case "HTMLImageElement":
		r.installImageInterface(p)
	case "HTMLCanvasElement":
		r.installUnsignedReflection(p, "width", "width", 300, nil)
		r.installUnsignedReflection(p, "height", "height", 150, nil)
	case "HTMLStyleElement":
		r.installStringReflection(p, "media", "media")
		r.installStringReflection(p, "type", "type")
	case "HTMLTitleElement":
		_ = p.DefineAccessorProperty("text", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return r.vm.ToValue("")
			}
			return r.vm.ToValue(node.TextContent())
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node != nil {
				r.setNodeTextContent(node, call.Argument(0).String())
			}
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	case "HTMLBaseElement":
		r.installURLReflection(p, "href", "href")
		r.installStringReflection(p, "target", "target")
	case "HTMLDataElement":
		r.installStringReflection(p, "value", "value")
	case "HTMLDetailsElement":
		r.installBoolReflection(p, "open", "open")
		r.installStringReflection(p, "name", "name")
	case "HTMLEmbedElement":
		r.installStringReflection(p, "name", "name")
		r.installURLReflection(p, "src", "src")
		r.installStringReflection(p, "type", "type")
		r.installUnsignedReflection(p, "width", "width", 0, nil)
		r.installUnsignedReflection(p, "height", "height", 0, nil)
	case "HTMLLabelElement":
		r.installStringReflection(p, "htmlFor", "for")
		r.installLabelInterface(p)
	case "HTMLOutputElement":
		r.installFormOwnerReflection(p)
		r.installOutputInterface(p)
	case "HTMLLIElement":
		r.installLongReflection(p, "value", "value", 0)
	case "HTMLMapElement":
		r.installStringReflection(p, "name", "name")
	case "HTMLMetaElement":
		r.installStringReflection(p, "name", "name")
		r.installStringReflection(p, "content", "content")
		r.installStringReflection(p, "httpEquiv", "http-equiv")
		r.installStringReflection(p, "charSet", "charset")
	case "HTMLMeterElement":
		r.installDoubleReflection(p, "value", "value", 0)
		r.installDoubleReflection(p, "min", "min", 0)
		r.installDoubleReflection(p, "max", "max", 1)
		r.installDoubleReflection(p, "low", "low", 0.25)
		r.installDoubleReflection(p, "high", "high", 0.75)
		r.installDoubleReflection(p, "optimum", "optimum", 0.5)
	case "HTMLOListElement":
		r.installBoolReflection(p, "reversed", "reversed")
		r.installLongReflection(p, "start", "start", 1)
		r.installEnumReflection(p, "type", "type", "1", []string{"1", "a", "A", "i", "I"})
	case "HTMLModElement":
		r.installURLReflection(p, "cite", "cite")
		r.installStringReflection(p, "dateTime", "datetime")
	case "HTMLOptGroupElement":
		r.installBoolReflection(p, "disabled", "disabled")
		r.installStringReflection(p, "label", "label")
	case "HTMLParamElement":
		r.installStringReflection(p, "name", "name")
		r.installStringReflection(p, "value", "value")
		r.installStringReflection(p, "type", "type")
		r.installStringReflection(p, "valueType", "valuetype")
	case "HTMLProgressElement":
		r.installStringReflection(p, "max", "max")
		r.installStringReflection(p, "value", "value")
	case "HTMLQuoteElement":
		r.installURLReflection(p, "cite", "cite")
	case "HTMLSourceElement":
		r.installURLReflection(p, "src", "src")
		r.installStringReflection(p, "srcset", "srcset")
		r.installStringReflection(p, "sizes", "sizes")
		r.installStringReflection(p, "type", "type")
		r.installStringReflection(p, "media", "media")
	case "HTMLTimeElement":
		r.installStringReflection(p, "dateTime", "datetime")
	case "HTMLTableColElement":
		r.installUnsignedReflection(p, "span", "span", 1, nil)
		r.installStringReflection(p, "width", "width")
	case "HTMLAudioElement", "HTMLVideoElement":
		r.installMediaElementInterface(p)
	case "HTMLFormElement":
		r.installHTMLFormElementInterface(p)
	case "HTMLButtonElement":
		r.installButtonInterface(p)
	case "HTMLFieldSetElement", "HTMLObjectElement", "HTMLSelectElement", "HTMLTextAreaElement":
		r.installFormOwnerReflection(p)
		if iface == "HTMLObjectElement" {
			r.installStringReflection(p, "name", "name")
		}
		if iface == "HTMLSelectElement" {
			r.installSelectSelectednessInterface(p)
			r.installSelectCollectionInterface(p)
			_ = p.Set("showPicker", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				node := r.getNode(call.This)
				if node == nil || node.Data != "select" {
					panic(r.vm.NewTypeError("Illegal invocation"))
				}
				r.showPicker(node, true)
				return goja.Undefined()
			}))
		}
		if iface == "HTMLFieldSetElement" {
			r.installStringReflection(p, "name", "name")
			r.installBoolReflection(p, "disabled", "disabled")
			r.installFieldsetInterface(p)
		}
		if iface == "HTMLSelectElement" || iface == "HTMLTextAreaElement" {
			r.installAutocompleteReflection(p, false)
		}
		if iface == "HTMLSelectElement" {
			r.installFormControlValidation(p)
		}
		if iface == "HTMLTextAreaElement" {
			r.installFormControlValidation(p)
			r.installTextControlLengthReflections(p, "HTMLTextAreaElement")
			r.installTextareaValueAccessors(p)
			r.installTextControlSelection(p, false)
			r.installTextareaInterface(p)
		}
	case "HTMLInputElement":
		r.installInputInterface(p)
		r.installAutocompleteReflection(p, false)
	case "HTMLOptionElement":
		r.installOptionInterface(p)
	case "HTMLIFrameElement":
		r.installIFrameInterface(p)
	case "HTMLFrameElement":
		r.installStringReflection(p, "name", "name")
	case "HTMLSlotElement":
		r.installSlotInterface(p)
	case "HTMLTemplateElement":
		r.installTemplateInterface(p)
	case "HTMLTableElement":
		r.installTableInterface(p)
	case "HTMLTableSectionElement":
		r.installTableSectionInterface(p)
	case "HTMLTableRowElement":
		r.installTableRowInterface(p)
	case "HTMLTableCellElement":
		r.installUnsignedReflection(p, "colSpan", "colspan", 1, nil)
		r.installUnsignedReflection(p, "rowSpan", "rowspan", 1, nil)
		r.installStringReflection(p, "headers", "headers")
		r.installStringReflection(p, "scope", "scope")
		r.installStringReflection(p, "abbr", "abbr")
	}
}

func (r *Runtime) installSelectSelectednessInterface(proto *goja.Object) {
	r.installBoolReflection(proto, "disabled", "disabled")
	r.installBoolReflection(proto, "multiple", "multiple")
	r.installBoolReflection(proto, "required", "required")
	r.installStringReflection(proto, "name", "name")
	r.installUnsignedReflection(proto, "size", "size", 0, nil)
	_ = proto.DefineAccessorProperty("type", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node != nil && node.HasAttribute("multiple") {
			return r.vm.ToValue("select-multiple")
		}
		return r.vm.ToValue("select-one")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("selectedIndex", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		selectNode := r.getNode(call.This)
		if selectNode == nil {
			return r.vm.ToValue(-1)
		}
		dom.NormalizeSelectSelectedness(selectNode)
		for i, option := range dom.SelectOptions(selectNode) {
			if option.OptionSelected() {
				return r.vm.ToValue(i)
			}
		}
		return r.vm.ToValue(-1)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		selectNode := r.getNode(call.This)
		if selectNode == nil {
			return goja.Undefined()
		}
		index := call.Argument(0).ToInteger()
		options := dom.SelectOptions(selectNode)
		for i, option := range options {
			option.SetOptionSelected(int64(i) == index, int64(i) == index)
		}
		selectNode.SelectNoSelection = index < 0 || index >= int64(len(options))
		r.updateSelectSelectedContent(selectNode)
		r.notifyMutation()
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = proto.DefineAccessorProperty("value", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		selectNode := r.getNode(call.This)
		if selectNode == nil {
			return r.vm.ToValue("")
		}
		dom.NormalizeSelectSelectedness(selectNode)
		for _, option := range dom.SelectOptions(selectNode) {
			if option.OptionSelected() {
				return r.vm.ToValue(optionValue(option))
			}
		}
		return r.vm.ToValue("")
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		selectNode := r.getNode(call.This)
		if selectNode == nil {
			return goja.Undefined()
		}
		value := call.Argument(0).String()
		var match *dom.Node
		for _, option := range dom.SelectOptions(selectNode) {
			option.SetOptionSelected(false, false)
			if match == nil && optionValue(option) == value {
				match = option
			}
		}
		if match != nil {
			match.SetOptionSelected(true, true)
		} else {
			selectNode.SelectNoSelection = true
		}
		r.updateSelectSelectedContent(selectNode)
		r.notifyMutation()
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = proto.Set("item", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		selectNode := r.getNode(call.This)
		if selectNode == nil {
			return goja.Null()
		}
		index := int(call.Argument(0).ToInteger())
		options := dom.SelectOptions(selectNode)
		if index < 0 || index >= len(options) {
			return goja.Null()
		}
		return r.wrapNode(options[index])
	}))
}

func optionValue(option *dom.Node) string {
	if option == nil {
		return ""
	}
	if option.HasAttribute("value") {
		return option.GetAttribute("value")
	}
	return normalizeHTMLOptionText(option.TextContent())
}

func normalizeHTMLOptionText(text string) string {
	var result strings.Builder
	result.Grow(len(text))
	pendingSpace := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' ' {
			pendingSpace = true
			continue
		}
		if pendingSpace && result.Len() > 0 {
			result.WriteByte(' ')
		}
		pendingSpace = false
		result.WriteByte(c)
	}
	return result.String()
}

func (r *Runtime) installOptionInterface(proto *goja.Object) {
	r.installBoolReflection(proto, "disabled", "disabled")
	r.installBoolReflection(proto, "defaultSelected", "selected")
	_ = proto.DefineAccessorProperty("form", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.wrapNode(r.formOwner(dom.NearestSelect(node)))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("label", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		if node.HasAttribute("label") {
			return r.vm.ToValue(node.GetAttribute("label"))
		}
		return r.vm.ToValue(normalizeHTMLOptionText(node.TextContent()))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node != nil {
			node.SetAttribute("label", call.Argument(0).String())
			r.notifyMutation()
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = proto.DefineAccessorProperty("selected", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue(false)
		}
		if selectNode := dom.NearestSelect(node); selectNode != nil {
			dom.NormalizeSelectSelectedness(selectNode)
		}
		return r.vm.ToValue(node.OptionSelected())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node != nil {
			node.SetOptionSelected(call.Argument(0).ToBoolean(), true)
			if selectNode := dom.NearestSelect(node); selectNode != nil {
				r.updateSelectSelectedContent(selectNode)
			}
			r.notifyMutation()
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = proto.DefineAccessorProperty("value", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		if node.HasAttribute("value") {
			return r.vm.ToValue(node.GetAttribute("value"))
		}
		return r.vm.ToValue(normalizeHTMLOptionText(node.TextContent()))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node != nil {
			node.SetAttribute("value", call.Argument(0).String())
			r.notifyMutation()
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = proto.DefineAccessorProperty("index", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		selectNode := dom.NearestSelect(node)
		for i, option := range dom.SelectOptions(selectNode) {
			if option == node {
				return r.vm.ToValue(i)
			}
		}
		return r.vm.ToValue(0)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = proto.DefineAccessorProperty("text", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.TextContent())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node != nil {
			r.setNodeTextContent(node, call.Argument(0).String())
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) installAreaInterface(p *goja.Object) {
	r.installHyperlinkURLInterface(p)
	r.installStringReflection(p, "alt", "alt")
	r.installStringReflection(p, "coords", "coords")
	r.installStringReflection(p, "target", "target")
	r.installStringReflection(p, "download", "download")
	r.installStringReflection(p, "ping", "ping")
	r.installStringReflection(p, "rel", "rel")
	r.installStringReflection(p, "referrerPolicy", "referrerpolicy")
	r.installStringReflection(p, "hreflang", "hreflang")
	r.installStringReflection(p, "type", "type")
	r.installEnumReflection(p, "shape", "shape", "rect", []string{"rect", "circle", "poly", "default"})
}

func (r *Runtime) installMediaElementInterface(p *goja.Object) {
	r.installURLReflection(p, "src", "src")
	r.installNullableStringReflection(p, "crossOrigin", "crossorigin")
	r.installEnumReflection(p, "preload", "preload", "auto", []string{"none", "metadata", "auto"})
	r.installBoolReflection(p, "autoplay", "autoplay")
	r.installBoolReflection(p, "loop", "loop")
	r.installBoolReflection(p, "controls", "controls")
	r.installBoolReflection(p, "defaultMuted", "muted")
}

func (r *Runtime) installButtonInterface(p *goja.Object) {
	r.installFormControlValidation(p)
	r.installFormOwnerReflection(p)
	r.installBoolReflection(p, "autofocus", "autofocus")
	r.installBoolReflection(p, "disabled", "disabled")
	r.installBoolReflection(p, "formNoValidate", "formnovalidate")
	r.installStringReflection(p, "name", "name")
	r.installStringReflection(p, "value", "value")
	r.installStringReflection(p, "formTarget", "formtarget")
	r.installURLReflection(p, "formAction", "formaction")
	r.installEnumReflection(p, "type", "type", "submit", []string{"submit", "reset", "button"})
	r.installEnumReflection(p, "formEnctype", "formenctype", "application/x-www-form-urlencoded", []string{
		"application/x-www-form-urlencoded", "multipart/form-data", "text/plain",
	})
	r.installEnumReflection(p, "formMethod", "formmethod", "get", []string{"get", "post", "dialog"})
}

func (r *Runtime) installNullableStringReflection(p *goja.Object, prop, attr string) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.HasAttribute(attr) {
			return goja.Null()
		}
		return r.vm.ToValue(node.GetAttribute(attr))
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		value := call.Argument(0)
		if goja.IsNull(value) || goja.IsUndefined(value) {
			r.setReflectedAttribute(node, attr, "", false)
		} else {
			r.setReflectedAttribute(node, attr, value.String(), true)
		}
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty(prop, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) installDoubleReflection(p *goja.Object, prop, attr string, def float64) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.HasAttribute(attr) {
			return r.vm.ToValue(def)
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(node.GetAttribute(attr)), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return r.vm.ToValue(def)
		}
		return r.vm.ToValue(value)
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		value := call.Argument(0).ToFloat()
		raw := strconv.FormatFloat(value, 'f', -1, 64)
		r.setReflectedAttribute(node, attr, raw, true)
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty(prop, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) installWindowReflectingBodyHandlers(p *goja.Object) {
	// HTMLBodyElement and HTMLFrameSetElement reflect the legacy body/window
	// handlers onto Window. The six GlobalEventHandlers entries listed here
	// are the body-specific reflected handlers; WindowEventHandlers are also
	// exposed through the same legacy interface surface.
	reflected := map[string]bool{
		"blur": true, "error": true, "focus": true, "load": true,
		"resize": true, "scroll": true,
	}
	for _, typ := range windowEventHandlerTypes {
		reflected[typ] = true
	}
	for typ := range reflected {
		name := "on" + typ
		typ := typ
		_ = p.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil || !r.nodeHasBrowsingContext(node) {
				return goja.Null()
			}
			return r.vm.GlobalObject().Get("on" + typ)
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node != nil && r.nodeHasBrowsingContext(node) {
				_ = r.vm.GlobalObject().Set("on"+typ, call.Argument(0))
			}
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
}

func (r *Runtime) installTemplateInterface(p *goja.Object) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.wrapNode(node.TemplateContent())
	})
	_ = p.DefineAccessorProperty("content", getter, goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) installSlotInterface(p *goja.Object) {
	r.installStringReflection(p, "name", "name")
	_ = p.Set("assignedNodes", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Data != "slot" {
			return newJSArray(r.vm, nil)
		}
		assigned := node.AssignedNodes()
		flatten := len(call.Arguments) > 0 && call.Arguments[0] != nil && !goja.IsNull(call.Arguments[0]) && call.Arguments[0].ToObject(r.vm).Get("flatten").ToBoolean()
		if flatten {
			assigned = r.flattenAssignedNodes(node, assigned)
		}
		return r.wrapNodeList(assigned)
	})
	_ = p.Set("assignedElements", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Data != "slot" {
			return newJSArray(r.vm, nil)
		}
		assigned := node.AssignedNodes()
		flatten := len(call.Arguments) > 0 && call.Arguments[0] != nil && !goja.IsNull(call.Arguments[0]) && call.Arguments[0].ToObject(r.vm).Get("flatten").ToBoolean()
		if flatten {
			assigned = r.flattenAssignedNodes(node, assigned)
		}
		var elements []*dom.Node
		for _, n := range assigned {
			if n.Element() {
				elements = append(elements, n)
			}
		}
		return r.wrapNodeList(elements)
	})
	_ = p.Set("assign", func(call goja.FunctionCall) goja.Value {
		slot := r.getNode(call.This)
		if slot == nil || slot.Data != "slot" {
			return goja.Undefined()
		}
		host, shadow := slot.ShadowHost()
		if host == nil || shadow == nil || shadow.SlotAssignment != "manual" {
			panic(r.vm.NewTypeError("slot.assign is only available for manual slot assignment"))
		}
		assigned := make([]*dom.Node, 0, len(call.Arguments))
		seen := make(map[*dom.Node]bool, len(call.Arguments))
		for _, argument := range call.Arguments {
			n := r.getNode(argument)
			if n == nil || n.Parent != host || seen[n] {
				panic(r.vm.NewTypeError("slot.assign arguments must be unique direct children of the host"))
			}
			seen[n] = true
			assigned = append(assigned, n)
		}
		if shadow.ManualSlots == nil {
			shadow.ManualSlots = make(map[*dom.Node][]*dom.Node)
		}
		shadow.ManualSlots[slot] = assigned
		r.notifyMutation()
		return goja.Undefined()
	})
}

func (r *Runtime) wrapNodeList(nodes []*dom.Node) goja.Value {
	values := make([]goja.Value, len(nodes))
	for i, node := range nodes {
		values[i] = r.wrapNode(node)
	}
	return newJSArray(r.vm, values)
}

func (r *Runtime) flattenAssignedNodes(slot *dom.Node, assigned []*dom.Node) []*dom.Node {
	if len(assigned) == 0 {
		assigned = slot.ChildNodes()
	}
	var result []*dom.Node
	for _, n := range assigned {
		if n.Element() && n.Data == "slot" {
			nested := n.AssignedNodes()
			if len(nested) == 0 {
				nested = n.ChildNodes()
			}
			result = append(result, r.flattenAssignedNodes(n, nested)...)
		} else {
			result = append(result, n)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Reflection helpers
// ---------------------------------------------------------------------------

func (r *Runtime) setReflectedAttribute(node *dom.Node, attr, value string, present bool) {
	if node == nil {
		return
	}
	oldValue, hadOld := node.GetAttribute(attr), node.HasAttribute(attr)
	beforeDetailsOpen := node.Data == "details" && attr == "open" && hadOld
	if present {
		node.SetAttribute(attr, value)
	} else {
		node.RemoveAttribute(attr)
	}
	r.RefreshInputPatternMismatch(node)
	newValue, hasNew := node.GetAttribute(attr), node.HasAttribute(attr)
	if node.Data == "details" && attr == "open" && beforeDetailsOpen != hasNew {
		r.detailsOpenChanged(node, beforeDetailsOpen, hasNew, nil)
	}
	if node.Data == "details" && attr == "name" && (oldValue != newValue || hadOld != hasNew) {
		r.detailsNameChanged(node)
	}
	r.notifyCustomElementAttributeChanged(node, attr, oldValue, hadOld, newValue, hasNew, "")
	var old *string
	if hadOld {
		old = &oldValue
	}
	if present {
		r.notifyAttributeMutation(node, "", attr, old)
	} else if hadOld {
		r.notifyAttributeMutation(node, "", attr, old)
	}
	if hadOld != hasNew || oldValue != newValue {
		r.notifyMutation()
	}
}

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
		value := call.Argument(0).String()
		r.setReflectedAttribute(node, attr, value, true)
		if attr == "id" {
			r.installWindowNamedProperty(call.Argument(0).String())
		}
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
		r.setReflectedAttribute(node, attr, "", call.Argument(0).ToBoolean())
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
		r.setReflectedAttribute(node, attr, strconv.FormatInt(v, 10), true)
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
		r.setReflectedAttribute(node, attr, strconv.FormatInt(call.Argument(0).ToInteger(), 10), true)
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
		raw := asciiLowerString(node.GetAttribute(attr))
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
		r.setReflectedAttribute(node, attr, call.Argument(0).String(), true)
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty(prop, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func asciiLowerString(value string) string {
	bytes := []byte(value)
	for i, b := range bytes {
		if b >= 'A' && b <= 'Z' {
			bytes[i] = b + ('a' - 'A')
		}
	}
	return string(bytes)
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
		r.setReflectedAttribute(node, attr, call.Argument(0).String(), true)
		if node.Data == "iframe" && attr == "src" && r.nodeIsLive(node) {
			r.queueIFrameLoad(node)
		}
		if node.Data == "script" && attr == "src" && node.IsConnected() {
			r.queueDynamicScript(node)
		}
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

func (r *Runtime) iframeFrameForDocument(node *dom.Node) *dom.Node {
	if node == nil {
		return nil
	}
	if node.Type != dom.NodeDocument {
		if node.Doc == nil {
			return nil
		}
		node = &node.Doc.Node
	}
	for frame, doc := range r.iframeDocuments {
		if doc != nil && &doc.Node == node {
			return frame
		}
	}
	return nil
}

func (r *Runtime) documentURLForNode(node *dom.Node) *url.URL {
	if node == nil || node.Doc == nil {
		return r.documentURL()
	}
	owner, frame := r.browsingContextForDocument(node.Doc)
	if owner == nil {
		if parsed := r.detachedDocumentURLs[node.Doc]; parsed != nil {
			copyURL := *parsed
			return &copyURL
		}
		return r.documentURL()
	}
	if frame == nil {
		return owner.documentURL()
	}
	if child := owner.iframeChildRuntimes[frame]; child != nil {
		return child.documentURL()
	}
	location := owner.iframeLocations[frame]
	if location == "" {
		location = "about:blank"
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return nil
	}
	return parsed
}

func (r *Runtime) documentBaseURLForNode(node *dom.Node) *url.URL {
	if node == nil || node.Doc == nil {
		return r.documentBaseURL()
	}
	owner, frame := r.browsingContextForDocument(node.Doc)
	if owner == nil {
		return baseURLForDocument(node.Doc, r.documentURLForNode(node))
	}
	if frame == nil {
		return owner.documentBaseURL()
	}
	if child := owner.iframeChildRuntimes[frame]; child != nil {
		return child.documentBaseURL()
	}
	if frame.HasAttribute("srcdoc") {
		return owner.documentBaseURLForNode(frame)
	}
	return baseURLForDocument(node.Doc, r.documentURLForNode(node))
}

func (r *Runtime) documentBaseURL() *url.URL {
	return baseURLForDocument(r.domDoc, r.documentURL())
}

func baseURLForDocument(doc *dom.Document, documentURL *url.URL) *url.URL {
	if documentURL == nil || doc == nil {
		return documentURL
	}
	var baseHref string
	doc.Node.Walk(func(node *dom.Node) bool {
		if node.Element() && node.Namespace == dom.NSHTML && node.Data == "base" && node.HasAttribute("href") {
			baseHref = node.GetAttribute("href")
			return false
		}
		return true
	})
	if baseHref == "" {
		return documentURL
	}
	ref, err := url.Parse(strings.Trim(baseHref, " \t\n\r\f"))
	if err != nil {
		return documentURL
	}
	return documentURL.ResolveReference(ref)
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
	return r.resolveURLAgainst(raw, r.documentBaseURL(), "UTF-8")
}

func (r *Runtime) resolveURLForNode(raw string, node *dom.Node) (href string, u *url.URL, ok bool) {
	encoding := "UTF-8"
	if node != nil && node.Doc != nil && node.Doc.CharacterSet != "" {
		encoding = node.Doc.CharacterSet
	}
	return r.resolveURLAgainst(raw, r.documentBaseURLForNode(node), encoding)
}

func (r *Runtime) resolveURLAgainst(raw string, base *url.URL, encoding string) (href string, u *url.URL, ok bool) {
	// The WHATWG URL parser removes ASCII tab/newline characters throughout
	// input, then trims leading/trailing C0 controls and spaces. In particular,
	// data URLs used as form actions often contain line breaks around markup.
	raw = strings.ReplaceAll(raw, "\t", "")
	raw = strings.ReplaceAll(raw, "\n", "")
	raw = strings.ReplaceAll(raw, "\r", "")
	raw = strings.Trim(raw, " \t\n\r\f")
	if base == nil {
		return raw, nil, false
	}
	if raw == "" {
		// An empty reference resolves to the base; an href with no attribute
		// is handled by the caller before reaching here.
		return canonicalURLStringWithEncoding(base, encoding), base, true
	}
	ref, err := url.Parse(raw)
	if err != nil {
		// WHATWG URL parsing treats a colon in the first path segment as a
		// scheme only when the preceding bytes form a valid ASCII scheme. Go's
		// parser rejects any such colon in a relative first segment, including
		// when that segment starts with Unicode whitespace; force that case into
		// path-reference parsing while keeping malformed authorities rejected.
		if strings.Contains(err.Error(), "first path segment in URL cannot contain colon") {
			ref, err = url.Parse("./" + raw)
		}
		if err != nil {
			return raw, nil, false
		}
	}
	resolved := base.ResolveReference(ref)
	// Go preserves the host's case; Chromium lowercases it.
	resolved.Host = strings.ToLower(resolved.Host)
	resolved.RawQuery = percentEncodeURLQueryWithEncoding(resolved.RawQuery, encoding)
	return canonicalURLStringWithEncoding(resolved, encoding), resolved, true
}

// canonicalURLString serialises a resolved URL. Opaque URLs (data:, mailto:,
// javascript:) keep their opaque payload untouched.
func canonicalURLString(u *url.URL) string {
	return canonicalURLStringWithEncoding(u, "UTF-8")
}

func canonicalURLStringWithEncoding(u *url.URL, encoding string) string {
	if u == nil {
		return ""
	}
	if u.Opaque != "" {
		return u.Scheme + ":" + u.Opaque
	}
	copy := *u
	if u.Path != "" {
		path := u.RawPath
		if path == "" {
			path = u.Path
		}
		copy.RawPath = percentEncodeURLPath(path)
	}
	if u.RawQuery != "" {
		copy.RawQuery = percentEncodeURLQueryWithEncoding(u.RawQuery, encoding)
	}
	return copy.String()
}

// percentEncodeURLPath applies the path percent-encode set to a URL path while
// preserving existing percent escapes. Go's URL serializer leaves Unicode
// path bytes literal; browsers serialize them as UTF-8 percent escapes.
func percentEncodeURLPath(path string) string {
	const hex = "0123456789ABCDEF"
	var out strings.Builder
	for i := 0; i < len(path); i++ {
		b := path[i]
		if b == '%' && i+2 < len(path) && isHexByte(path[i+1]) && isHexByte(path[i+2]) {
			out.WriteString(path[i : i+3])
			i += 2
			continue
		}
		if b <= 0x20 || b >= 0x7f || b == '"' || b == '#' || b == '<' || b == '>' ||
			b == '?' || b == '^' || b == '`' || b == '{' || b == '}' {
			out.WriteByte('%')
			out.WriteByte(hex[b>>4])
			out.WriteByte(hex[b&0x0f])
			continue
		}
		out.WriteByte(b)
	}
	return out.String()
}

func isHexByte(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}

// percentEncodeURLQuery applies the URL query percent-encode set. url.URL
// intentionally preserves RawQuery verbatim, but HTMLAnchorElement.href
// serializes non-ASCII code points and spaces as UTF-8 percent escapes.
func percentEncodeURLQuery(query string) string {
	return percentEncodeURLQueryWithEncoding(query, "UTF-8")
}

func percentEncodeURLQueryWithEncoding(query, label string) string {
	const hex = "0123456789ABCDEF"
	if !strings.EqualFold(label, "UTF-8") {
		if encoding, _ := charset.Lookup(label); encoding != nil {
			if encoded, err := encoding.NewEncoder().Bytes([]byte(query)); err == nil {
				query = string(encoded)
			}
		}
	}
	var out strings.Builder
	for i := 0; i < len(query); i++ {
		b := query[i]
		if b <= 0x20 || b >= 0x7f || b == '"' || b == '#' || b == '<' || b == '>' {
			out.WriteByte('%')
			out.WriteByte(hex[b>>4])
			out.WriteByte(hex[b&0x0f])
			continue
		}
		out.WriteByte(b)
	}
	return out.String()
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
	case "username":
		if u.User != nil {
			userinfo := u.User.String()
			username, _, _ := strings.Cut(userinfo, ":")
			return username
		}
		return ""
	case "password":
		if u.User != nil {
			userinfo := u.User.String()
			_, password, hasPassword := strings.Cut(userinfo, ":")
			if hasPassword {
				return password
			}
		}
		return ""
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
	_, u, ok := r.resolveURLForNode(node.GetAttribute("href"), node)
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
	base := r.documentBaseURLForNode(node)
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
	case "username", "password":
		if u.Host == "" {
			return
		}
		username, password, hasPassword := "", "", false
		if u.User != nil {
			username = u.User.Username()
			password, hasPassword = u.User.Password()
		}
		if part == "username" {
			username = value
		} else {
			password = value
			hasPassword = true
		}
		if hasPassword {
			u.User = url.UserPassword(username, password)
		} else {
			u.User = url.User(username)
		}
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
	encoding := "UTF-8"
	if node.Doc != nil && node.Doc.CharacterSet != "" {
		encoding = node.Doc.CharacterSet
	}
	node.SetAttribute("href", canonicalURLStringWithEncoding(u, encoding))
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

	// nodeName for HTML elements is the uppercase tag. Foreign-content and XML
	// elements preserve their qualified name; leaf nodes use the DOM Standard's
	// fixed name ("#text", "#comment", ...).
	_ = r.nodeProto.DefineAccessorProperty("nodeName", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		if node.Element() {
			if (node.Doc != nil && node.Doc.IsXML) || node.Namespace != dom.NSHTML {
				return r.vm.ToValue(node.Data)
			}
			return r.vm.ToValue(strings.ToUpper(node.Data))
		}
		return r.vm.ToValue(nodeStaticName(node))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = r.nodeProto.DefineAccessorProperty("childNodes", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.newLiveNodeList(nil)
		}
		return r.newLiveNodeList(node)
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
		if node.Doc == r.domDoc {
			return r.wrapNode(&node.Doc.Node)
		}
		return r.wrapDetachedDocument(node.Doc)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = r.nodeProto.DefineAccessorProperty("nodeValue", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		switch node.Type {
		case dom.NodeText, dom.NodeCDATASection, dom.NodeComment, dom.NodeProcessingInstruction:
			return goja.StringFromUTF16(characterDataCodeUnits(node))
		default:
			return goja.Null()
		}
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || (node.Type != dom.NodeText && node.Type != dom.NodeCDATASection && node.Type != dom.NodeComment && node.Type != dom.NodeProcessingInstruction) {
			return goja.Undefined()
		}
		if len(call.Arguments) > 0 && !goja.IsNull(call.Arguments[0]) && !goja.IsUndefined(call.Arguments[0]) {
			r.replaceCharacterDataUnits(node, 0, len(characterDataCodeUnits(node)), characterDataValueUnits(call.Arguments[0]))
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
	_ = r.documentProto.DefineAccessorProperty("URL", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if documentURL := r.documentURLForNode(node); documentURL != nil {
			return r.vm.ToValue(documentURL.String())
		}
		return r.vm.ToValue("")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.documentProto.DefineAccessorProperty("documentURI", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if documentURL := r.documentURLForNode(node); documentURL != nil {
			return r.vm.ToValue(documentURL.String())
		}
		return r.vm.ToValue("")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.documentProto.DefineAccessorProperty("baseURI", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if baseURL := r.documentBaseURLForNode(node); baseURL != nil {
			return r.vm.ToValue(baseURL.String())
		}
		return r.vm.ToValue("")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

// installElementAccessors installs the Element interface: tagName, id,
// className, classList, attributes, innerHTML and the child-element reads.
func (r *Runtime) installElementAccessors() {
	_ = r.elementProto.DefineAccessorProperty("customElementRegistry", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.Element() {
			return goja.Null()
		}
		if owner := r.runtimeForDocument(node.Doc); owner != nil && owner != r {
			registry := owner.customElementRegistryForNode(node)
			return r.importChildValue(owner, owner.customElementRegistryObject(registry), make(map[*goja.Object]*goja.Object), 0)
		}
		return r.customElementRegistryObject(r.customElementRegistryForNode(node))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.elementProto.DefineAccessorProperty("prefix", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.Element() {
			return goja.Null()
		}
		if node.Prefix != "" {
			return r.vm.ToValue(node.Prefix)
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.elementProto.DefineAccessorProperty("localName", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.Element() {
			return goja.Null()
		}
		localName := node.Data
		if node.Prefix != "" && strings.HasPrefix(localName, node.Prefix+":") {
			localName = strings.TrimPrefix(localName, node.Prefix+":")
		}
		return r.vm.ToValue(localName)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.elementProto.DefineAccessorProperty("tagName", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		// XML documents preserve tag-name case; HTML documents uppercase it.
		if (node.Doc != nil && node.Doc.IsXML) || node.Namespace != dom.NSHTML {
			return r.vm.ToValue(node.Data)
		}
		return r.vm.ToValue(strings.ToUpper(node.Data))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.elementProto.DefineAccessorProperty("namespaceURI", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		if uri := elementNamespaceURI(node); uri != "" {
			return r.vm.ToValue(uri)
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	r.installStringReflection(r.elementProto, "id", "id")
	r.installStringReflection(r.elementProto, "className", "class")
	r.installStringReflection(r.elementProto, "slot", "slot")
	r.installARIAMixin(r.elementProto)

	_ = r.elementProto.DefineAccessorProperty("classList", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.classListObject(node)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if node := r.getNode(call.This); node != nil {
			r.classListObject(node).Set("value", call.Argument(0))
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = r.elementProto.DefineAccessorProperty("attributes", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.attributesObject(node)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)

	_ = r.elementProto.DefineAccessorProperty("innerHTML", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		if node.Doc != nil && node.Doc.IsXML {
			markup, err := serializeXMLChildren(node)
			if err != nil {
				panic(r.domError("InvalidStateError", err.Error()))
			}
			return r.vm.ToValue(markup)
		}
		return r.vm.ToValue(r.innerHTMLLive(thisNID(call)))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		nid := thisNID(call)
		if nid != 0 {
			documentSetInnerHTML(r, nid, htmlMarkupString(call.Argument(0)))
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.elementProto.DefineAccessorProperty("outerHTML", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		var serialized strings.Builder
		r.serializeNode(&serialized, node)
		return r.vm.ToValue(serialized.String())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		nid := thisNID(call)
		if nid != 0 {
			r.setOuterHTML(nid, htmlMarkupString(call.Argument(0)))
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)

	// shadowRoot and attachShadow stay on Node.prototype (installed by
	// setupShadowDOM) and are inherited by Element.prototype.

	r.installElementCollectionMethods()

	r.installHTMLElementAccessors()
}

// installARIAMixin exposes the reflected string-valued ARIA IDL attributes
// shared by Element and its HTML element subclasses. Using the normal
// reflected-attribute path keeps custom-element reactions and mutation
// observation aligned with setAttribute().
func (r *Runtime) installARIAMixin(prototype *goja.Object) {
	properties := []string{
		"role", "ariaAtomic", "ariaAutoComplete", "ariaBrailleLabel", "ariaBrailleRoleDescription", "ariaBusy", "ariaChecked", "ariaColCount", "ariaColIndex", "ariaColIndexText", "ariaColSpan", "ariaCurrent", "ariaDescription", "ariaDisabled", "ariaExpanded", "ariaHasPopup", "ariaHidden", "ariaInvalid", "ariaKeyShortcuts", "ariaLabel", "ariaLevel", "ariaLive", "ariaModal", "ariaMultiLine", "ariaMultiSelectable", "ariaOrientation", "ariaPlaceholder", "ariaPosInSet", "ariaPressed", "ariaReadOnly", "ariaRelevant", "ariaRequired", "ariaRoleDescription", "ariaRowCount", "ariaRowIndex", "ariaRowIndexText", "ariaRowSpan", "ariaSelected", "ariaSetSize", "ariaSort", "ariaValueMax", "ariaValueMin", "ariaValueNow", "ariaValueText",
	}
	for _, property := range properties {
		attribute := "role"
		if strings.HasPrefix(property, "aria") {
			attribute = "aria-" + strings.ToLower(strings.TrimPrefix(property, "aria"))
		}
		r.installStringReflection(prototype, property, attribute)
	}
	for _, property := range []string{"ariaActiveDescendantElement"} {
		name := property
		attribute := "aria-activedescendant"
		_ = prototype.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Null()
			}
			if value, ok := r.ariaElementReferences[node][name]; ok {
				return value
			}
			return r.elementReferenceFromAttribute(node, attribute)
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Undefined()
			}
			value := call.Argument(0)
			if goja.IsNull(value) || goja.IsUndefined(value) {
				delete(r.ariaElementReferences[node], name)
				r.setReflectedAttribute(node, attribute, "", false)
				return goja.Undefined()
			}
			if referenced := r.getNode(value); referenced == nil || !referenced.Element() {
				panic(r.vm.NewTypeError("The value is not an Element."))
			}
			if r.ariaElementReferences[node] == nil {
				r.ariaElementReferences[node] = make(map[string]goja.Value)
			}
			r.ariaElementReferences[node][name] = value
			r.setReflectedAttribute(node, attribute, "", true)
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	listAttributes := map[string]string{
		"ariaControlsElements": "aria-controls", "ariaDescribedByElements": "aria-describedby", "ariaDetailsElements": "aria-details", "ariaErrorMessageElements": "aria-errormessage", "ariaFlowToElements": "aria-flowto", "ariaLabelledByElements": "aria-labelledby", "ariaOwnsElements": "aria-owns",
	}
	for property, attribute := range listAttributes {
		name, attr := property, attribute
		_ = prototype.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Null()
			}
			if value, ok := r.ariaElementListReferences[node][name]; ok {
				return value
			}
			return r.elementReferenceListFromAttribute(node, attr)
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Undefined()
			}
			value := call.Argument(0)
			if goja.IsNull(value) || goja.IsUndefined(value) {
				delete(r.ariaElementListReferences[node], name)
				r.setReflectedAttribute(node, attr, "", false)
				return goja.Undefined()
			}
			array := value.ToObject(r.vm)
			length := int(array.Get("length").ToInteger())
			for i := 0; i < length; i++ {
				item := array.Get(strconv.Itoa(i))
				if referenced := r.getNode(item); referenced == nil || !referenced.Element() {
					panic(r.vm.NewTypeError("The value contains a non-Element item."))
				}
			}
			if r.ariaElementListReferences[node] == nil {
				r.ariaElementListReferences[node] = make(map[string]goja.Value)
			}
			r.ariaElementListReferences[node][name] = value
			r.setReflectedAttribute(node, attr, "", true)
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
}

func (r *Runtime) elementReferenceFromAttribute(node *dom.Node, attribute string) goja.Value {
	if node == nil || node.Doc == nil {
		return goja.Null()
	}
	for _, id := range strings.Fields(node.GetAttribute(attribute)) {
		if target := node.Doc.ElementByID(id); target != nil {
			return r.wrapNode(target)
		}
	}
	return goja.Null()
}

func (r *Runtime) elementReferenceListFromAttribute(node *dom.Node, attribute string) goja.Value {
	values := make([]goja.Value, 0)
	if node != nil && node.Doc != nil {
		for _, id := range strings.Fields(node.GetAttribute(attribute)) {
			if target := node.Doc.ElementByID(id); target != nil {
				values = append(values, r.wrapNode(target))
			}
		}
	}
	return newJSArray(r.vm, values)
}

func htmlMarkupString(value goja.Value) string {
	if goja.IsNull(value) {
		return ""
	}
	return value.String()
}

func (r *Runtime) setOuterHTML(nid int, markup string) {
	node := r.nodeRegistry[nid]
	if node == nil || node.Parent == nil {
		return
	}
	parent := node.Parent
	if parent.Type == dom.NodeDocument {
		panic(r.domError("NoModificationAllowedError", "The element has a document parent."))
	}
	parseContext := parent
	if parent.DocumentFragment() {
		parseContext = &dom.Node{Type: dom.NodeElement, Namespace: dom.NSHTML, Data: "body", Doc: parent.Doc}
	}
	if !parseContext.Element() {
		return
	}
	parsed, err := dom.ParseFragmentBounded(parseContext, markup, dom.ParseLimits{
		Nodes: 1024, Depth: 128, Attributes: 256, AttributeBytes: 4096,
	})
	if err != nil || parsed == nil {
		return
	}
	newChildren := parsed.ChildNodes()
	oldWasLive := r.nodeIsLive(node)
	wasConnected := node.IsConnected()
	removal := snapshotChildRemovals([]*dom.Node{node})[0]
	r.admitInsert("outerHTML", parent, newChildren, func() {
		if len(newChildren) == 0 {
			parent.RemoveChild(node)
		} else {
			parent.ReplaceNode(parsed, node)
		}
	})
	r.queueMutationRecord(MutationRecord{Type: "childList", Target: parent, AddedNodes: newChildren, RemovedNodes: []*dom.Node{node}, PreviousSibling: removal.previous, NextSibling: removal.next})
	r.notifyDisconnected([]*dom.Node{node}, map[*dom.Node]bool{node: wasConnected})
	r.markRadioGroupsDirtyForNodes(append(newChildren, node))
	if oldWasLive {
		r.releaseNodes([]*dom.Node{node})
	}
	for _, child := range newChildren {
		r.activateInsertedSubtree(child)
	}
	if r.domDoc != nil && node == r.domDoc.Body {
		var newBody *dom.Node
		for _, child := range newChildren {
			if child.Element() && (child.Data == "body" || child.Data == "frameset") {
				newBody = child
				break
			}
		}
		r.domDoc.Body = newBody
		r.RefreshDocumentTreeReferences()
	}
	r.notifyMutation()
}

func (r *Runtime) activateInsertedSubtree(node *dom.Node) {
	r.activateInsertedSubtreeWithDetailsPolicy(node, false)
}

// activateDOMInsertedSubtree applies connection effects for nodes inserted by
// DOM mutation APIs. If an inserted details element is already open and a
// group peer is open, the previously connected peer wins even when the new
// element was inserted earlier in tree order.
func (r *Runtime) activateDOMInsertedSubtree(node *dom.Node) {
	r.activateInsertedSubtreeWithDetailsPolicy(node, true)
}

func (r *Runtime) activateInsertedSubtreeWithDetailsPolicy(node *dom.Node, domInsertion bool) {
	if node == nil {
		return
	}
	if node.Element() {
		if domInsertion && node.Data == "iframe" && node.IsConnected() {
			r.queueIFrameLoad(node)
		}
		if domInsertion && node.Data == "script" && node.IsConnected() {
			r.queueDynamicScript(node)
		}
		if node.Data == "details" && node.HasAttribute("open") {
			if domInsertion {
				r.normalizeDOMInsertedDetailsGroup(node)
			} else {
				r.normalizeInsertedDetailsGroup(node)
			}
		}
		if id := node.GetAttribute("id"); id != "" {
			r.installWindowNamedProperty(id)
		}
	}
	for _, attr := range node.Attr {
		if attr.Namespace == dom.NSHTML && attr.NamespaceURI == "" {
			r.syncEventHandlerContentAttribute(node, attr.Name, attr.Value, true)
		}
	}
	r.notifyConnected(node)
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		r.activateInsertedSubtreeWithDetailsPolicy(child, domInsertion)
	}
}

// thisNID reads the registry key from the receiver without allocating a
// *dom.Node lookup; 0 means "not a wrapped node".
// installParentNodeAccessors implements the ParentNode mixin on its three
// specified interfaces. `children` is a live HTMLCollection, while the
// element-child endpoints and count are reflected reads over the live tree.
func (r *Runtime) installParentNodeAccessors() {
	prototypes := []*goja.Object{r.elementProto, r.documentProto, r.documentFragmentProto}
	for _, proto := range prototypes {
		_ = proto.Set("replaceChildren", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			return r.jsReplaceChildren(call)
		}))
		_ = proto.DefineAccessorProperty("children", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return r.newLiveHTMLCollectionMatching(func() []*dom.Node { return nil })
			}
			return r.newLiveHTMLCollectionMatching(func() []*dom.Node {
				var result []*dom.Node
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Element() {
						result = append(result, child)
					}
				}
				return result
			})
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

		for _, name := range []string{"firstElementChild", "lastElementChild"} {
			name := name
			_ = proto.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				node := r.getNode(call.This)
				if node == nil {
					return goja.Null()
				}
				var found *dom.Node
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if !child.Element() {
						continue
					}
					found = child
					if name == "firstElementChild" {
						break
					}
				}
				return r.wrapNode(found)
			}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
		}

		_ = proto.DefineAccessorProperty("childElementCount", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return r.vm.ToValue(0)
			}
			count := 0
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Element() {
					count++
				}
			}
			return r.vm.ToValue(count)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
	// NonDocumentTypeChildNode exposes these live sibling reads on Element.
	for _, name := range []string{"nextElementSibling", "previousElementSibling"} {
		name := name
		_ = r.elementProto.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Null()
			}
			sibling := node.NextSibling
			if name == "previousElementSibling" {
				sibling = node.PrevSibling
			}
			for sibling != nil && !sibling.Element() {
				if name == "nextElementSibling" {
					sibling = sibling.NextSibling
				} else {
					sibling = sibling.PrevSibling
				}
			}
			return r.wrapNode(sibling)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
}

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
	if node.IsHTMLTemplate() {
		node = node.TemplateContent()
	}
	return r.serializeChildren(node)
}

func documentSetInnerHTML(r *Runtime, nid int, html string, forceHTMLFragment ...bool) {
	if fn, ok := goja.AssertFunction(r.vm.Get("document").ToObject(r.vm).Get("__setInnerHTML__")); ok {
		// The bridge throws QuotaExceededError when admission rejects the
		// fragment. A goja call reports that as an *Exception; rethrow it so
		// the assignment the page made still fails instead of succeeding
		// silently with the old children intact.
		args := []goja.Value{r.vm.ToValue(nid), r.vm.ToValue(html)}
		if len(forceHTMLFragment) > 0 && forceHTMLFragment[0] {
			args = append(args, r.vm.ToValue(true))
		}
		if _, err := fn(goja.Undefined(), args...); err != nil {
			panic(err)
		}
	}
}

func (r *Runtime) installHTMLElementAccessors() {
	p := r.htmlElementProto
	datasetProto := r.vm.NewObject()
	r.defineConstructor("DOMStringMap", datasetProto)
	_ = r.vm.Set("__datasetProto__", datasetProto)
	_ = p.DefineAccessorProperty("draggable", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.vm.ToValue(dom.IsDraggable(node))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		value := "false"
		if call.Argument(0).ToBoolean() {
			value = "true"
		}
		r.setReflectedAttribute(node, "draggable", value, true)
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = p.DefineAccessorProperty("contentEditable", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.vm.ToValue(dom.ContentEditableState(node))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		oldValue, hadOld := node.GetAttributeNS("", "contenteditable")
		value := call.Argument(0).String()
		switch {
		case value == "" || asciiEqualFold(value, "true"):
			node.SetAttribute("contenteditable", "true")
		case asciiEqualFold(value, "false"):
			node.SetAttribute("contenteditable", "false")
		case asciiEqualFold(value, "plaintext-only"):
			node.SetAttribute("contenteditable", "plaintext-only")
		case asciiEqualFold(value, "inherit"):
			node.RemoveAttribute("contenteditable")
		default:
			panic(r.domError("SyntaxError", "The value provided is not a valid contentEditable value."))
		}
		newValue, hasNew := node.GetAttributeNS("", "contenteditable")
		r.notifyCustomElementAttributeChanged(node, "contenteditable", oldValue, hadOld, newValue, hasNew, "")
		if oldValue != newValue || hadOld != hasNew {
			r.notifyMutation()
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = p.DefineAccessorProperty("isContentEditable", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(dom.IsContentEditable(r.getNode(call.This)))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
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
	_ = r.vm.Set("__htmlProto__", r.elementProto)
	_, _ = r.vm.RunString(`(function(datasetProto) {
		Object.defineProperty(__htmlProto__, 'dataset', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return {};
				if (!document.__datasetSupported__(nid)) return undefined;
				var map = document.__getDataset__(nid);
				Object.setPrototypeOf(map, datasetProto);
				var toAttr = function(k) {
					return 'data-' + String(k).replace(/[A-Z]/g, function(c) {
						return '-' + c.toLowerCase();
					});
				};
				var toKey = function(attr) {
					var suffix = attr.slice(5), out = '';
					for (var i = 0; i < suffix.length; i++) {
						var c = suffix.charAt(i);
						if (c === '-' && i + 1 < suffix.length && /[a-z]/.test(suffix.charAt(i + 1))) {
							out += suffix.charAt(++i).toUpperCase();
						} else {
							out += c;
						}
					}
					return out;
				};
				return new Proxy(map, {
					get: function(t, k) {
						if (typeof k !== 'string') return t[k];
						var cur = t[k];
						if (cur !== undefined) return cur;
						if (k === '__nid__') return nid;
						var attr = toAttr(k);
						if (toKey(attr) !== k) return undefined;
						var v = document.__getDatasetAttr__(nid, attr);
						return v === null ? undefined : v;
					},
					ownKeys: function(t) {
						return Reflect.ownKeys(t).filter(function(k) { return k !== '__nid__'; });
					},
					getOwnPropertyDescriptor: function(t, k) {
						if (k === '__nid__') return undefined;
						return Reflect.getOwnPropertyDescriptor(t, k);
					},
					set: function(t, k, v) {
						if (typeof k !== 'string' || k === '__nid__') return true;
						if (/-[a-z]/.test(k)) {
							throw new DOMException("The dataset property name contains a hyphen followed by a lowercase ASCII letter.", 'SyntaxError');
						}
						if (/[\u0000\t\n\f\r ]/.test(k)) {
							throw new DOMException("The dataset property name produces an invalid attribute name.", 'InvalidCharacterError');
						}
						document.__setDatasetAttr__(map, nid, toAttr(k), String(v));
						return true;
					},
					deleteProperty: function(t, k) {
						if (typeof k !== 'string' || k === '__nid__') return Reflect.deleteProperty(t, k);
						var attr = toAttr(k);
						if (toKey(attr) !== k) return Reflect.deleteProperty(t, k);
						document.__deleteDatasetAttr__(nid, attr);
						return Reflect.deleteProperty(t, k);
					}
				});
			},
			configurable: true,
			enumerable: true
		});
	})(__datasetProto__);`)
	_ = r.vm.Set("__htmlProto__", nil)
	_ = r.vm.Set("__datasetProto__", nil)

	r.installStringReflection(p, "title", "title")
	r.installStringReflection(p, "lang", "lang")
	r.installEnumReflection(p, "dir", "dir", "", []string{"ltr", "rtl", "auto"})
	_ = p.DefineAccessorProperty("translate", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		for curr := node; curr != nil; curr = curr.Parent {
			if !curr.Element() || !curr.HasAttribute("translate") {
			} else {
				value := curr.GetAttribute("translate")
				if value == "" || asciiEqualFold(value, "yes") {
					return r.vm.ToValue(true)
				}
				if asciiEqualFold(value, "no") {
					return r.vm.ToValue(false)
				}
			}
			if curr.Parent != nil && curr.Parent.ShadowRoot != nil {
				for child := curr.Parent.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
					if child == curr {
						return r.vm.ToValue(true)
					}
				}
			}
		}
		return r.vm.ToValue(true)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		value := "no"
		if call.Argument(0).ToBoolean() {
			value = "yes"
		}
		r.setReflectedAttribute(node, "translate", value, true)
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.installStringReflection(p, "accessKey", "accesskey")
	_ = p.DefineAccessorProperty("spellcheck", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue(true)
		}
		value := strings.ToLower(node.GetAttribute("spellcheck"))
		return r.vm.ToValue(value != "false")
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if node := r.getNode(call.This); node != nil {
			value := "false"
			if call.Argument(0).ToBoolean() {
				value = "true"
			}
			r.setReflectedAttribute(node, "spellcheck", value, true)
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = p.DefineAccessorProperty("innerText", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(r.innerText(node))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		r.setInnerText(node, innerTextSetterValue(call))
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = p.DefineAccessorProperty("outerText", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(r.innerText(node))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		r.setOuterText(node, innerTextSetterValue(call))
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.installBoolReflection(p, "hidden", "hidden")
	r.installBoolReflection(p, "autofocus", "autofocus")
	r.installBoolReflection(p, "inert", "inert")
	// tabIndex is the one signed long reflection measured in probe5: negatives
	// are legal (a negative value means "not sequentially focusable"), and the
	// setter stores the truncated integer.
	r.installLongReflection(p, "tabIndex", "tabindex", -1)
}

func asciiEqualFold(value, keyword string) bool {
	if len(value) != len(keyword) {
		return false
	}
	for i := range value {
		c := value[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != keyword[i] {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// DOMTokenList and NamedNodeMap
// ---------------------------------------------------------------------------

// classListObject returns the per-element DOMTokenList. Identity is stable, as
// in Chromium, and every read comes from the live class attribute: a dynamic
// object is what makes `Array.prototype.join.call(classList, '+')` see fresh
// indices after setAttribute changed the class.
func (r *Runtime) classListObject(node *dom.Node) *goja.Object {
	return r.tokenListObject(node, "class")
}

type domTokenListKey struct {
	node      *dom.Node
	attribute string
}

func (r *Runtime) tokenListObject(node *dom.Node, attribute string) *goja.Object {
	key := domTokenListKey{node: node, attribute: attribute}
	if obj := r.tokenListObjects[key]; obj != nil {
		return obj
	}
	obj := r.vm.NewDynamicObject(&domTokenList{r: r, node: node, attribute: attribute})
	r.tokenListObjects[key] = obj
	return obj
}

type domTokenList struct {
	r         *Runtime
	node      *dom.Node
	attribute string
}

func (t *domTokenList) tokens() []string {
	value := t.node.GetAttribute(t.attribute)
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == '\t' || r == '\n' || r == '\f' || r == '\r' || r == ' '
	})
}

func (t *domTokenList) setTokens(tokens []string) {
	oldValue, hadOld := t.node.GetAttribute(t.attribute), t.node.HasAttribute(t.attribute)
	if len(tokens) == 0 {
		t.node.RemoveAttribute(t.attribute)
	} else {
		t.node.SetAttribute(t.attribute, strings.Join(tokens, " "))
	}
	newValue, hasNew := t.node.GetAttribute(t.attribute), t.node.HasAttribute(t.attribute)
	changed := oldValue != newValue || hadOld != hasNew
	// DOMTokenList mutators run the associated attribute setter even when the
	// serialized value is unchanged. That setter still enqueues a CEReactions
	// attributeChanged callback for an observed attribute.
	t.r.notifyCustomElementAttributeChanged(t.node, t.attribute, oldValue, hadOld, newValue, hasNew, "")
	var old *string
	if hadOld {
		old = &oldValue
	}
	t.r.notifyAttributeMutation(t.node, "", t.attribute, old)
	if changed {
		t.r.notifyMutation()
	}
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
		return t.r.vm.ToValue(len(t.tokens()))
	case "value":
		return t.r.vm.ToValue(strings.Join(t.tokens(), " "))
	case "add":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			var requested []string
			for _, value := range call.Arguments {
				cls := value.String()
				if cls == "" || strings.ContainsAny(cls, " \t\n\r\f") {
					panic(t.r.domError("InvalidCharacterError", "Failed to execute 'add' on 'DOMTokenList': The token provided must not be empty and must not contain whitespace."))
				}
				requested = append(requested, cls)
			}
			existing := t.tokens()
			seen := make(map[string]bool, len(existing)+len(requested))
			for _, c := range existing {
				seen[c] = true
			}
			changed := false
			for _, cls := range requested {
				if !seen[cls] {
					existing = append(existing, cls)
					seen[cls] = true
					changed = true
				}
			}
			if changed || len(requested) > 0 {
				t.setTokens(existing)
			}
			return goja.Undefined()
		})
	case "remove":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			requested := make(map[string]bool, len(call.Arguments))
			for _, value := range call.Arguments {
				cls := value.String()
				if cls == "" || strings.ContainsAny(cls, " \t\n\r\f") {
					panic(t.r.domError("InvalidCharacterError", "Failed to execute 'remove' on 'DOMTokenList': The token provided must not be empty and must not contain whitespace."))
				}
				requested[cls] = true
			}
			existing := t.tokens()
			filtered := make([]string, 0, len(existing))
			found := false
			for _, c := range existing {
				if !requested[c] {
					filtered = append(filtered, c)
				} else {
					found = true
				}
			}
			if found {
				t.setTokens(filtered)
			} else if t.node.HasAttribute(t.attribute) {
				// remove() still runs the token list's attribute update step when
				// the requested token is absent but the attribute is present.
				t.setTokens(existing)
			}
			return goja.Undefined()
		})
	case "replace":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			oldToken := t.tokenArg(call)
			newToken := call.Argument(1).String()
			if newToken == "" || strings.ContainsAny(newToken, " \t\n\r\f") {
				panic(t.r.domError("InvalidCharacterError", "Failed to execute 'replace' on 'DOMTokenList': The token provided must not be empty and must not contain whitespace."))
			}
			existing := t.tokens()
			oldIndex := -1
			newExists := false
			for i, token := range existing {
				if token == oldToken && oldIndex < 0 {
					oldIndex = i
				}
				if token == newToken {
					newExists = true
				}
			}
			if oldIndex < 0 {
				return t.r.vm.ToValue(false)
			}
			if oldToken == newToken {
				t.setTokens(existing)
				return t.r.vm.ToValue(true)
			}
			if newExists {
				existing = append(existing[:oldIndex], existing[oldIndex+1:]...)
			} else {
				existing[oldIndex] = newToken
			}
			t.setTokens(existing)
			return t.r.vm.ToValue(true)
		})
	case "toggle":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			cls := t.tokenArg(call)
			existing := t.tokens()
			for i, c := range existing {
				if c == cls {
					if len(call.Arguments) < 2 || goja.IsUndefined(call.Argument(1)) || call.Argument(1).ToBoolean() == false {
						rest := append(existing[:i], existing[i+1:]...)
						t.setTokens(rest)
						return t.r.vm.ToValue(false)
					}
					return t.r.vm.ToValue(true)
				}
			}
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !call.Argument(1).ToBoolean() {
				return t.r.vm.ToValue(false)
			}
			t.setTokens(append(existing, cls))
			return t.r.vm.ToValue(true)
		})
	case "contains":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			cls := call.Argument(0).String()
			for _, c := range t.tokens() {
				if c == cls {
					return t.r.vm.ToValue(true)
				}
			}
			return t.r.vm.ToValue(false)
		})
	case "item":
		return t.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			i := int(call.Argument(0).ToInteger())
			list := t.tokens()
			if i < 0 || i >= len(list) {
				return goja.Null()
			}
			return t.r.vm.ToValue(list[i])
		})
	case "toString":
		return t.r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			return t.r.vm.ToValue(strings.Join(t.tokens(), " "))
		})
	}
	if i, err := strconv.Atoi(key); err == nil {
		list := t.tokens()
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
	oldValue, hadOld := t.node.GetAttribute(t.attribute), t.node.HasAttribute(t.attribute)
	t.node.SetAttribute(t.attribute, value.String())
	newValue := t.node.GetAttribute(t.attribute)
	t.r.notifyCustomElementAttributeChanged(t.node, t.attribute, oldValue, hadOld, newValue, true, "")
	if oldValue != newValue || !hadOld {
		var old *string
		if hadOld {
			old = &oldValue
		}
		t.r.notifyAttributeMutation(t.node, "", t.attribute, old)
		t.r.notifyMutation()
	}
	return true
}

func (t *domTokenList) Has(key string) bool {
	switch key {
	case "length", "value", "add", "remove", "replace", "toggle", "contains", "item", "toString":
		return true
	}
	i, err := strconv.Atoi(key)
	return err == nil && i >= 0 && i < len(t.tokens())
}

func (t *domTokenList) Delete(string) bool { return false }

func (t *domTokenList) Keys() []string {
	list := t.tokens()
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
	target := r.vm.NewDynamicObject(&namedNodeMap{r: r, node: node})
	proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		OwnKeys: func(target *goja.Object) *goja.Object {
			keys := target.GetOwnPropertyNames()
			values := make([]interface{}, 0, len(keys))
			for _, key := range keys {
				values = append(values, key)
			}
			return r.vm.NewArray(values...)
		},
		GetOwnPropertyDescriptor: func(target *goja.Object, property string) goja.PropertyDescriptor {
			if _, err := strconv.Atoi(property); err == nil {
				return r.ownPropertyDescriptor(target, property)
			}
			if isNamedNodeMapBuiltin(property) {
				return r.ownPropertyDescriptor(target, property)
			}
			if value := target.Get(property); value != nil && !goja.IsUndefined(value) {
				return goja.PropertyDescriptor{Value: value, Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_FALSE, Configurable: goja.FLAG_TRUE}
			}
			return r.ownPropertyDescriptor(target, property)
		},
	})
	obj := r.vm.ToValue(proxy).(*goja.Object)
	r.attributesObjects[node] = obj
	return obj
}

func isNamedNodeMapBuiltin(property string) bool {
	switch property {
	case "length", "item", "getNamedItem", "getNamedItemNS", "setNamedItem", "setNamedItemNS", "removeNamedItem", "removeNamedItemNS":
		return true
	default:
		return false
	}
}

type namedNodeMap struct {
	r    *Runtime
	node *dom.Node
}

func (m *namedNodeMap) attrAt(i int) goja.Value {
	if i < 0 || i >= len(m.node.Attr) {
		return nil
	}
	return m.r.jsAttributeForNode(m.node, m.node.Attr[i])
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
	case "getNamedItemNS":
		return m.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(m.r.vm.NewTypeError("Failed to execute 'getNamedItemNS': 2 arguments required"))
			}
			namespace := ""
			if value := call.Argument(0); value != nil && !goja.IsNull(value) && !goja.IsUndefined(value) {
				namespace = value.String()
			}
			localName := call.Argument(1).String()
			for i, attr := range m.node.Attr {
				if attr.NamespaceURI == namespace && domAttributeLocalName(attr.Name) == localName {
					return m.attrAt(i)
				}
			}
			return goja.Null()
		})
	case "removeNamedItem":
		return m.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(m.r.vm.NewTypeError("Failed to execute 'removeNamedItem': 1 argument required"))
			}
			name := call.Argument(0).String()
			for i, attr := range m.node.Attr {
				if strings.EqualFold(attr.Name, name) {
					return m.r.jsRemoveAttributeNode(goja.FunctionCall{This: m.r.wrapNode(m.node), Arguments: []goja.Value{m.attrAt(i)}})
				}
			}
			panic(m.r.domError("NotFoundError", "The attribute was not found"))
		})
	case "removeNamedItemNS":
		return m.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				panic(m.r.vm.NewTypeError("Failed to execute 'removeNamedItemNS': 2 arguments required"))
			}
			namespace := ""
			if value := call.Argument(0); value != nil && !goja.IsNull(value) && !goja.IsUndefined(value) {
				namespace = value.String()
			}
			localName := call.Argument(1).String()
			for i, attr := range m.node.Attr {
				if attr.NamespaceURI == namespace && domAttributeLocalName(attr.Name) == localName {
					return m.r.jsRemoveAttributeNode(goja.FunctionCall{This: m.r.wrapNode(m.node), Arguments: []goja.Value{m.attrAt(i)}})
				}
			}
			panic(m.r.domError("NotFoundError", "The attribute was not found"))
		})
	case "setNamedItem", "setNamedItemNS":
		return m.r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(m.r.vm.NewTypeError("Failed to execute 'setNamedItem': 1 argument required"))
			}
			obj, ok := call.Argument(0).(*goja.Object)
			if !ok || m.r.attributeValues[obj] == nil {
				panic(m.r.vm.NewTypeError("parameter 1 is not of type 'Attr'"))
			}
			attribute := m.r.attributeValues[obj]
			if attribute.owner != nil && attribute.owner != m.node {
				panic(m.r.domError("InUseAttributeError", "The attribute is already in use by another element"))
			}
			oldValue, hadOld := "", false
			for _, existing := range m.node.Attr {
				if existing.NamespaceURI == attribute.namespace && strings.EqualFold(domAttributeLocalName(existing.Name), domAttributeLocalName(attribute.name)) {
					oldValue, hadOld = existing.Value, true
					break
				}
			}
			m.node.SetAttributeNS(attribute.namespace, attribute.name, attribute.value)
			attribute.owner = m.node
			var old *string
			if hadOld {
				old = &oldValue
			}
			m.r.notifyAttributeMutation(m.node, attribute.namespace, domAttributeLocalName(attribute.name), old)
			m.r.notifyCustomElementAttributeChanged(m.node, domAttributeLocalName(attribute.name), oldValue, hadOld, attribute.value, true, attribute.namespace)
			m.r.notifyMutation()
			if !hadOld {
				return goja.Null()
			}
			return m.r.newJSAttribute(attribute.name, attribute.namespace)
		})
	}
	if i, err := strconv.Atoi(key); err == nil {
		return m.attrAt(i)
	}
	for i, attr := range m.node.Attr {
		if strings.EqualFold(attr.Name, key) {
			return m.attrAt(i)
		}
	}
	return nil
}

func (m *namedNodeMap) Set(string, goja.Value) bool { return false }

func (m *namedNodeMap) Has(key string) bool {
	switch key {
	case "length", "item", "getNamedItem", "getNamedItemNS", "setNamedItem", "setNamedItemNS", "removeNamedItem", "removeNamedItemNS":
		return true
	}
	i, err := strconv.Atoi(key)
	if err == nil {
		return i >= 0 && i < len(m.node.Attr)
	}
	for _, attr := range m.node.Attr {
		if strings.EqualFold(attr.Name, key) {
			return true
		}
	}
	return false
}

func (m *namedNodeMap) Delete(string) bool { return true }

func (m *namedNodeMap) Keys() []string {
	keys := make([]string, 0, len(m.node.Attr)*2)
	seen := make(map[string]bool, len(m.node.Attr)*2)
	for i := range m.node.Attr {
		key := strconv.Itoa(i)
		keys = append(keys, key)
		seen[key] = true
	}
	for _, attr := range m.node.Attr {
		if attr.Name != "" && !seen[attr.Name] {
			keys = append(keys, attr.Name)
			seen[attr.Name] = true
		}
	}
	return keys
}

// ---------------------------------------------------------------------------
// HTMLAnchorElement
// ---------------------------------------------------------------------------

func (r *Runtime) installAnchorInterface(p *goja.Object) {
	r.installHyperlinkURLInterface(p)

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
}

func (r *Runtime) installHyperlinkURLInterface(p *goja.Object) {
	_ = p.DefineAccessorProperty("href", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.HasAttribute("href") {
			return r.vm.ToValue("")
		}
		href, _, ok := r.resolveURLForNode(node.GetAttribute("href"), node)
		if !ok {
			return r.vm.ToValue(node.GetAttribute("href"))
		}
		return r.vm.ToValue(href)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		node.SetAttribute("href", call.Argument(0).String())
		r.notifyMutation()
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)

	for _, part := range []string{"protocol", "host", "hostname", "port", "pathname", "search", "hash", "origin", "username", "password"} {
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

	_ = p.Set("toString", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || !node.HasAttribute("href") {
			return r.vm.ToValue("")
		}
		href, _, ok := r.resolveURLForNode(node.GetAttribute("href"), node)
		if !ok {
			return r.vm.ToValue(node.GetAttribute("href"))
		}
		return r.vm.ToValue(href)
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
	var old []*dom.Node
	wasConnected := make(map[*dom.Node]bool)
	parentWasLive := r.nodeIsLive(node)
	r.admitInsert("text", node, children, func() {
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			old = append(old, c)
			wasConnected[c] = c.IsConnected()
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
	r.markRadioGroupsDirtyForNodes(append(old, children...))
	if len(old) > 0 || len(children) > 0 {
		r.queueMutationRecord(MutationRecord{Type: "childList", Target: node, AddedNodes: children, RemovedNodes: old})
	}
	r.notifyDisconnected(old, wasConnected)
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
		r.setReflectedAttribute(node, "src", raw, true)
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
			r.setReflectedAttribute(node, dim, strconv.FormatInt(v, 10), true)
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
			r.setReflectedAttribute(node, "crossorigin", "", false)
		} else {
			r.setReflectedAttribute(node, "crossorigin", v.String(), true)
		}
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
		if (node.Data == "body" || node.Data == "frameset") && windowReflectingBodyEventHandler(typ) {
			if !r.nodeHasBrowsingContext(node) {
				return goja.Null()
			}
			return r.vm.Get("window").ToObject(r.vm).Get("on" + typ)
		}
		if hs := r.onHandlers[node]; hs != nil {
			if h := hs[typ]; h != nil && h.hasContent && !h.compiled {
				r.compileContentEventHandler(node, typ, h)
				if h.fn != nil {
					return h.fn
				}
			}
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
		if (node.Data == "body" || node.Data == "frameset") && windowReflectingBodyEventHandler(typ) {
			if r.nodeHasBrowsingContext(node) {
				_ = r.vm.Get("window").ToObject(r.vm).Set("on"+typ, call.Argument(0))
			}
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
		_, callable := goja.AssertFunction(v)
		if goja.IsNull(v) || goja.IsUndefined(v) || !callable {
			h.fn = nil
			h.realm = nil
			h.hasContent = false
			h.compiled = true
			if h.registered {
				dom.RemoveEventListenerByKey(node, typ, h.key, false)
				h.registered = false
			}
			return goja.Undefined()
		}
		h.fn = v
		if object, ok := v.(*goja.Object); ok {
			if realm := r.iframeImportedFunctionRealms[object]; realm != nil {
				h.realm = realm
			} else {
				h.realm = r.runtimeForFunctionObject(object)
			}
		} else {
			h.realm = r
		}
		h.hasContent = false
		h.compiled = true
		r.activateEventHandler(node, typ, h)
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func windowReflectingBodyEventHandler(typ string) bool {
	switch typ {
	case "blur", "error", "focus", "load", "resize", "scroll":
		return true
	}
	for _, candidate := range windowEventHandlerTypes {
		if typ == candidate {
			return true
		}
	}
	return false
}

func (r *Runtime) activateEventHandler(node *dom.Node, typ string, h *jsEventHandler) {
	if node == nil || h == nil || h.registered {
		return
	}
	h.registered = true
	key := h.key
	dom.AddEventListenerWithKey(node, typ, func(e *dom.Event) {
		state := r.onHandlers[key]
		if state == nil {
			return
		}
		handler := state[typ]
		if handler == nil {
			return
		}
		if handler.realm != nil && !handler.realm.isFullyActiveBrowsingContext() {
			return
		}
		if handler.hasContent && !handler.compiled {
			r.compileContentEventHandler(key, typ, handler)
		}
		if handler.fn == nil {
			return
		}
		fn, ok := goja.AssertFunction(handler.fn)
		if !ok {
			return
		}
		eventObj := r.wrapEvent(e)
		var result goja.Value
		var err error
		r.withCurrentEventOnNode(eventObj, node, func() { result, err = fn(r.wrapNode(key), eventObj) })
		if err == nil && result != nil {
			if value, ok := result.Export().(bool); ok && !value {
				e.PreventDefault()
			}
		}
	}, false, key)
}

// syncEventHandlerContentAttribute installs or removes an inline event
// handler's listener when its content attribute changes. Compilation is lazy,
// as required by HTML's event-handler processing model.
func (r *Runtime) syncEventHandlerContentAttribute(node *dom.Node, name string, value string, present bool) {
	if node == nil || node.Type != dom.NodeElement || node.Namespace != dom.NSHTML || !r.nodeHasBrowsingContext(node) || r.opts.ScriptingDisabled {
		return
	}
	lower := strings.ToLower(name)
	if !strings.HasPrefix(lower, "on") || len(lower) <= 2 {
		return
	}
	typ := lower[2:]
	known := false
	for _, candidate := range globalEventHandlerTypes {
		if typ == candidate {
			known = true
			break
		}
	}
	if !known {
		return
	}
	if typ == "webkitanimationend" {
		typ = "webkitAnimationEnd"
	} else if typ == "webkitanimationiteration" {
		typ = "webkitAnimationIteration"
	} else if typ == "webkitanimationstart" {
		typ = "webkitAnimationStart"
	} else if typ == "webkittransitionend" {
		typ = "webkitTransitionEnd"
	}
	if node.Data == "body" || node.Data == "frameset" {
		reflected := typ == "blur" || typ == "error" || typ == "focus" || typ == "load" || typ == "resize" || typ == "scroll"
		if !reflected {
			for _, candidate := range windowEventHandlerTypes {
				if typ == candidate {
					reflected = true
					break
				}
			}
		}
		if reflected {
			r.syncWindowContentEventHandler(node, typ, value, present)
			return
		}
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
	if !present {
		h.fn, h.realm, h.content, h.hasContent, h.compiled = nil, nil, "", false, true
		if h.registered {
			dom.RemoveEventListenerByKey(node, typ, h.key, false)
			h.registered = false
		}
		return
	}
	h.fn, h.realm, h.content, h.hasContent, h.compiled = nil, r, value, true, false
	r.activateEventHandler(node, typ, h)
}

func (r *Runtime) nodeHasBrowsingContext(node *dom.Node) bool {
	if node == nil || r.domDoc == nil {
		return false
	}
	if node.Doc != nil {
		if node.Doc == r.domDoc {
			return true
		}
		// Same-origin iframe Documents are wrapped by the parent Runtime when
		// reached through contentDocument. Their nodes still have a browsing
		// context while the corresponding frame is current and connected.
		frame := r.iframeForDocument(node.Doc)
		return frame != nil && r.iframeDocuments[frame] == node.Doc && frame.IsConnected()
	}
	// Some embedders build a Document value directly and populate its tree
	// before assigning Node.Doc. A node connected to this document still has
	// its browsing context; detached nodes with no owner document do not.
	for current := node; current != nil; current = current.Parent {
		if current.Type == dom.NodeDocument {
			return current == &r.domDoc.Node
		}
	}
	return false
}

func (r *Runtime) compileContentEventHandler(node *dom.Node, typ string, h *jsEventHandler) {
	if h == nil || h.compiled || !h.hasContent {
		return
	}
	h.compiled = true
	if r.opts.ScriptingDisabled || (r.opts.CSP != nil && !r.opts.CSP.AllowsInlineScript()) {
		return
	}
	params := "event"
	if node != nil && (node.Data == "body" || node.Data == "frameset") && typ == "error" {
		params = "event, source, lineno, colno, error"
	}
	source := "(function on" + typ + "(" + params + ") {\n" + h.content + "\n})"
	program, err := goja.Compile(r.opts.URL+"#event-handler", source, false)
	if err != nil {
		return
	}
	value, err := r.vm.RunProgram(program)
	if err != nil {
		return
	}
	if _, ok := goja.AssertFunction(value); ok {
		h.fn = value
		h.realm = r
	}
}

// ---------------------------------------------------------------------------
// HTMLInputElement
// ---------------------------------------------------------------------------

func (r *Runtime) installInputInterface(p *goja.Object) {
	_ = p.Set("showPicker", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Data != "input" {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		r.showPicker(node, false)
		return goja.Undefined()
	}))
	r.installInputValueAccessors(p)
	r.installTextControlSelection(p, true)
	r.installInputStepping(p)
	r.installInputNumericValue(p)

	r.installStringReflection(p, "defaultValue", "value")
	r.installStringReflection(p, "name", "name")
	r.installStringReflection(p, "placeholder", "placeholder")
	r.installStringReflection(p, "alt", "alt")
	r.installStringReflection(p, "min", "min")
	r.installStringReflection(p, "max", "max")
	r.installStringReflection(p, "step", "step")
	r.installStringReflection(p, "pattern", "pattern")
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
		r.setReflectedAttribute(node, "src", raw, true)
		if inputType(node) == "image" {
			r.startImageLoad(node, raw)
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	for _, dim := range []string{"width", "height"} {
		dim := dim
		_ = p.DefineAccessorProperty(dim, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil || inputType(node) != "image" {
				return r.vm.ToValue(0)
			}
			return r.vm.ToValue(r.inputImageDimension(node, dim))
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil {
				return goja.Undefined()
			}
			v := call.Argument(0).ToInteger()
			if v < 0 {
				v = 0
			}
			r.setReflectedAttribute(node, dim, strconv.FormatInt(v, 10), true)
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = p.DefineAccessorProperty("list", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil || node.Data != "input" || node.Doc == nil {
			return goja.Null()
		}
		listID := node.GetAttribute("list")
		if listID == "" {
			return goja.Null()
		}
		list := node.Doc.ElementByID(listID)
		if list == nil || list.Namespace != dom.NSHTML || list.Data != "datalist" {
			return goja.Null()
		}
		return r.wrapNode(list)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = p.DefineAccessorProperty("labels", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.newLiveNodeListMatching(func() []*dom.Node {
			if node == nil || node.Doc == nil {
				return nil
			}
			var labels []*dom.Node
			node.Doc.Node.Walk(func(candidate *dom.Node) bool {
				if candidate.Data == "label" && r.labelControl(candidate) == node {
					labels = append(labels, candidate)
				}
				return true
			})
			return labels
		})
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)

	_ = p.DefineAccessorProperty("type", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("text")
		}
		return r.vm.ToValue(inputType(node))
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		raw := call.Argument(0).String()
		r.setReflectedAttribute(node, "type", raw, true)
		if node.IsConnected() {
			r.maybeLoadImageInput(node, true)
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.installBoolReflection(p, "defaultChecked", "checked")
	r.installBoolReflection(p, "disabled", "disabled")
	r.installBoolReflection(p, "required", "required")
	r.installBoolReflection(p, "multiple", "multiple")
	r.installBoolReflection(p, "readOnly", "readonly")

	// maxLength/minLength read -1 when absent or invalid and throw IndexSizeError
	// on negative assignments; size defaults to 20, throws on 0, and resets a
	// negative assignment to the default.
	r.installTextControlLengthReflections(p, "HTMLInputElement")
	r.installUnsignedReflection(p, "size", "size", 20, func(v int64) (int64, *goja.Object) {
		if v == 0 {
			return 0, r.domError("IndexSizeError", "Failed to set the 'size' property on 'HTMLInputElement': The value provided is 0, which is an invalid size.")
		}
		if v < 0 {
			return 20, nil
		}
		return v, nil
	})

	r.installFormOwnerReflection(p)
	r.installFormControlValidation(p)
}

func inputTypeSupportsReadonly(typ string) bool {
	switch typ {
	case "text", "search", "tel", "url", "email", "password", "date", "month", "week", "time", "datetime-local", "number":
		return true
	default:
		return false
	}
}

func (r *Runtime) showPicker(node *dom.Node, isSelect bool) {
	if dom.IsFormControlDisabled(node) || (!isSelect && node.HasAttribute("readonly") && inputTypeSupportsReadonly(inputType(node))) {
		panic(r.domError("InvalidStateError", "The form control is not mutable"))
	}
	if !r.sameOriginWithTopLevel() && (isSelect || inputType(node) != "file" && inputType(node) != "color") {
		panic(r.domError("SecurityError", "The picker is not available to a cross-origin frame"))
	}
	if !r.HasTransientUserActivation() {
		panic(r.domError("NotAllowedError", "The picker requires transient user activation"))
	}
	if isSelect && !r.nodeIsRendered(node) {
		panic(r.domError("NotSupportedError", "The select element is not being rendered"))
	}
	// The current host has no picker UI callback. These are the API preconditions
	// and activation steps; native picker delivery remains unsupported.
	r.SetTransientUserActivation(false)
}

func (r *Runtime) nodeIsRendered(node *dom.Node) bool {
	if node == nil || !node.IsConnected() {
		return false
	}
	for current := node; current != nil && current.Element(); current = current.Parent {
		values := r.computedValues(current)
		if strings.EqualFold(strings.TrimSpace(values["display"]), "none") ||
			strings.EqualFold(strings.TrimSpace(values["content-visibility"]), "hidden") {
			return false
		}
	}
	return true
}

func (r *Runtime) documentDomainSandboxed() bool {
	frame := r.iframeParentFrame
	if frame == nil || !frame.HasAttribute("sandbox") {
		return false
	}
	for _, token := range strings.FieldsFunc(frame.GetAttribute("sandbox"), func(ch rune) bool {
		return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f'
	}) {
		if strings.EqualFold(token, "allow-same-origin") {
			return false
		}
	}
	return true
}

func parseDocumentDomain(value string) (string, bool) {
	if value == "" || strings.TrimSpace(value) != value {
		return "", false
	}
	u, err := url.Parse("http://" + value)
	if err != nil || u.Host == "" || u.User != nil || u.Port() != "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	if ascii, err := idna.Lookup.ToASCII(host); err == nil {
		host = strings.ToLower(ascii)
	} else {
		return "", false
	}
	return host, host != ""
}

func validDocumentDomainSuffix(candidate, host string) bool {
	if ascii, err := idna.Lookup.ToASCII(host); err == nil {
		host = strings.ToLower(ascii)
	} else {
		return false
	}
	if candidate == "" || host == "" {
		return false
	}
	if candidate == host {
		return true
	}
	if net.ParseIP(candidate) != nil || net.ParseIP(host) != nil || !strings.HasSuffix(host, "."+candidate) {
		return false
	}
	publicSuffix, _ := publicsuffix.PublicSuffix(host)
	if candidate == publicSuffix {
		return false
	}
	return strings.HasSuffix(candidate, "."+publicSuffix)
}

func inputType(node *dom.Node) string {
	if node == nil {
		return "text"
	}
	t := asciiLowerString(strings.TrimSpace(node.GetAttribute("type")))
	for _, valid := range inputTypeValues {
		if valid == t {
			return t
		}
	}
	return "text"
}

// HTML's dimension-attribute parser accepts a signed integer prefix when its
// numeric value is non-negative (so -0 is valid), unlike unsigned IDL reflection.
func parseDimensionAttribute(s string) (int64, bool) {
	n, ok := parseHTMLInteger(s)
	return n, ok && n >= 0 && n <= math.MaxUint32
}

func (r *Runtime) inputImageDimension(node *dom.Node, dim string) int64 {
	attr, attrOK := parseDimensionAttribute(node.GetAttribute(dim))
	st := r.imageStates[node]
	loaded := st != nil && st.status == imageLoaded
	w, h := int64(0), int64(0)
	if loaded {
		w, h = int64(st.naturalW), int64(st.naturalH)
	}
	// A hidden or detached image button uses the natural dimensions, with each
	// valid dimension attribute overriding its corresponding side.
	hidden := !node.IsConnected() || r.computedValues(node)["display"] == "none"
	if hidden {
		if dim == "width" {
			if attrOK {
				return attr
			}
			return w
		}
		if attrOK {
			return attr
		}
		return h
	}
	// A rendered image button preserves its image ratio when one dimension is
	// supplied. Without an available image, dimensions come only from attributes.
	if dim == "width" {
		if attrOK {
			return attr
		}
		if hv, hOK := parseDimensionAttribute(node.GetAttribute("height")); hOK && w > 0 {
			return hv * w / max(int64(1), h)
		}
		return w
	}
	if attrOK {
		return attr
	}
	if wv, wOK := parseDimensionAttribute(node.GetAttribute("width")); wOK && h > 0 {
		return wv * h / max(int64(1), w)
	}
	return h
}

func (r *Runtime) maybeLoadImageInput(node *dom.Node, force bool) {
	if node == nil || node.Data != "input" || inputType(node) != "image" || !node.HasAttribute("src") {
		return
	}
	if !force {
		if st := r.imageStates[node]; st != nil && st.status != imageIdle {
			return
		}
	}
	r.startImageLoad(node, node.GetAttribute("src"))
}

func (r *Runtime) installTextControlLengthReflections(proto *goja.Object, interfaceName string) {
	for _, property := range []struct{ name, attribute string }{
		{name: "maxLength", attribute: "maxlength"},
		{name: "minLength", attribute: "minlength"},
	} {
		property := property
		r.installUnsignedReflection(proto, property.name, property.attribute, -1, func(v int64) (int64, *goja.Object) {
			if v < 0 {
				message := "Failed to set the '" + property.name + "' property on '" + interfaceName + "': The value provided is negative."
				return 0, r.domError("IndexSizeError", message)
			}
			return v, nil
		})
	}
}

// inputTypeValues is the HTML input type keyword set. An attribute outside it
// reads back as "text", as Chromium's limited-to-known-values reflection.
var inputTypeValues = []string{
	"hidden", "text", "search", "tel", "url", "email", "password", "date",
	"month", "week", "time", "datetime-local", "number", "range", "color",
	"checkbox", "radio", "file", "submit", "image", "reset", "button",
}

// installInputValueAccessors keeps the live value separate from the value
// content attribute for states where HTMLInputElement.value is in value mode.
// Checkbox and radio values continue to use their attribute/default behavior.
func (r *Runtime) installInputValueAccessors(p *goja.Object) {
	_ = p.DefineAccessorProperty("indeterminate", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.vm.ToValue(node != nil && node.Data == "input" && node.InputIndeterminate)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if node := r.getNode(call.This); node != nil && node.Data == "input" {
			node.InputIndeterminate = call.Argument(0).ToBoolean()
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
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
				document.__setValue__(nid, v === null ? '' : String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__inputProto__", nil)
}

func (r *Runtime) installTextareaValueAccessors(p *goja.Object) {
	valueGet := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.InputControlValue())
	})
	valueSet := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		before := node.InputControlValue()
		node.SetInputControlValue(strings.ReplaceAll(call.Argument(0).String(), "\r", ""), true)
		node.ValueChangedByUser = false
		if before != node.InputControlValue() {
			r.resetTextSelectionAfterValueSet(node, node.InputControlValue())
		}
		if before != node.InputControlValue() {
			r.notifyMutation()
		}
		return goja.Undefined()
	})
	defaultGet := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.TextContent())
	})
	defaultSet := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		r.setNodeTextContent(node, call.Argument(0).String())
		node.SyncInputControlValueFromAttribute()
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty("value", valueGet, valueSet, goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = p.DefineAccessorProperty("defaultValue", defaultGet, defaultSet, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// installTextareaInterface adds the textarea-only IDL attributes that are not
// shared with inputs. cols and rows use positive-value fallback semantics;
// their setters still reflect zero as an attribute value, whose getter then
// falls back to the specified default.
func (r *Runtime) installTextareaInterface(p *goja.Object) {
	r.installStringReflection(p, "name", "name")
	_ = p.DefineAccessorProperty("type", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue("textarea")
	}), nil, goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.installBoolReflection(p, "disabled", "disabled")
	r.installBoolReflection(p, "readOnly", "readonly")
	r.installBoolReflection(p, "required", "required")
	_ = p.DefineAccessorProperty("textLength", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue(0)
		}
		return r.vm.ToValue(len(utf16.Encode([]rune(node.InputControlValue()))))
	}), nil, goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.installPositiveUnsignedReflection(p, "cols", "cols", 20)
	r.installPositiveUnsignedReflection(p, "rows", "rows", 2)
	r.installStringReflection(p, "wrap", "wrap")
}

// installOutputInterface implements the output element's value/default-value
// override model. The current value is the descendant text; setting value
// first snapshots the default value, while setting defaultValue updates the
// override only after value has been set.
func (r *Runtime) installOutputInterface(p *goja.Object) {
	r.installStringReflection(p, "name", "name")
	r.installFormControlValidation(p)
	_ = p.DefineAccessorProperty("labels", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		return r.newLiveNodeListMatching(func() []*dom.Node {
			if node == nil || node.Doc == nil {
				return nil
			}
			var labels []*dom.Node
			node.Doc.Node.Walk(func(candidate *dom.Node) bool {
				if candidate.Data == "label" && r.labelControl(candidate) == node {
					labels = append(labels, candidate)
				}
				return true
			})
			return labels
		})
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = p.DefineAccessorProperty("type", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue("output")
	}), nil, goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = p.DefineAccessorProperty("htmlFor", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Null()
		}
		return r.tokenListObject(node, "for")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	valueGet := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.TextContent())
	})
	valueSet := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		node.OutputDefaultValueOverride = node.OutputDefaultValue()
		node.OutputDefaultValueOverrideSet = true
		r.setNodeTextContent(node, call.Argument(0).String())
		return goja.Undefined()
	})
	defaultGet := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.OutputDefaultValue())
	})
	defaultSet := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		value := call.Argument(0).String()
		if node.OutputDefaultValueOverrideSet {
			node.OutputDefaultValueOverride = value
		} else {
			r.setNodeTextContent(node, value)
		}
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty("value", valueGet, valueSet, goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = p.DefineAccessorProperty("defaultValue", defaultGet, defaultSet, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// ResetOutputElement applies an output element's reset algorithm while keeping
// the runtime's mutation and reflow observers in sync.
func (r *Runtime) ResetOutputElement(node *dom.Node) {
	if r == nil || node == nil || node.Data != "output" {
		return
	}
	value := node.ResetOutputControlValue()
	r.setNodeTextContent(node, value)
}

func (r *Runtime) installPositiveUnsignedReflection(p *goja.Object, prop, attr string, fallback int64) {
	getter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return r.vm.ToValue(fallback)
		}
		value, ok := parseTextareaPositiveDimension(node.GetAttribute(attr))
		if !ok {
			return r.vm.ToValue(fallback)
		}
		return r.vm.ToValue(value)
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			return goja.Undefined()
		}
		// Web IDL's unsigned long conversion truncates and wraps modulo 2^32;
		// [ReflectPositiveWithFallback] then substitutes the default unless the
		// result is in the range 1..2147483647.
		number := call.Argument(0).ToFloat()
		var value uint64
		if !math.IsNaN(number) && !math.IsInf(number, 0) && number != 0 {
			mod := math.Mod(math.Trunc(number), 4294967296)
			if mod < 0 {
				mod += 4294967296
			}
			value = uint64(mod)
		}
		if value < 1 || value > math.MaxInt32 {
			value = uint64(fallback)
		}
		r.setReflectedAttribute(node, attr, strconv.FormatUint(value, 10), true)
		return goja.Undefined()
	})
	_ = p.DefineAccessorProperty(prop, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

// parseTextareaPositiveDimension applies HTML's non-negative-integer parser,
// then the unsigned-long and positive-fallback bounds for cols and rows.
func parseTextareaPositiveDimension(s string) (int64, bool) {
	s = strings.TrimLeft(s, " \t\n\r\f")
	if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	if s == "" || s[0] < '0' || s[0] > '9' {
		return 0, false
	}
	var value int64
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		digit := int64(s[i] - '0')
		if value > (math.MaxInt32-digit)/10 {
			return 0, false
		}
		value = value*10 + digit
	}
	if value == 0 {
		return 0, false
	}
	return value, true
}

// domError builds a DOMException-shaped object with the given name, which is
// how internal/dom reports the IndexSize/InvalidCharacter errors Chromium
// throws from these setters.
func (r *Runtime) domError(name, msg string) *goja.Object {
	if constructor, ok := goja.AssertConstructor(r.vm.Get("DOMException")); ok {
		if exception, err := constructor(nil, r.vm.ToValue(msg), r.vm.ToValue(name)); err == nil {
			return exception
		}
	}
	e := r.vm.NewTypeError(msg)
	_ = e.Set("name", name)
	return e
}
