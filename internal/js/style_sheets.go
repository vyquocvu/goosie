package js

import (
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

// LinkedCSSFunc answers the CSS text the engine already fetched for one
// absolute style sheet URL. ok is false when this document owes no text for
// that URL - it was never requested, or the request failed - and the sheet then
// reports an empty rule list, which is the measured shape of a <link> whose
// fetch 404s: Chromium lists /missing.css with 0 rules and a non-null .sheet.
//
// The contract lives in internal/js because the import graph forbids this
// package from reaching internal/engine. The engine supplies it from the same
// memo the cascade fetches through, so the CSSOM and the cascade cannot
// disagree about what a sheet contains.
type LinkedCSSFunc func(url string) (string, bool)

// SetLinkedCSSProvider installs the engine's fetched style sheet reader. The
// engine calls it alongside SetComputedStyleProvider, before any script runs.
func (r *Runtime) SetLinkedCSSProvider(fn LinkedCSSFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.linkedCSS = fn
}

func (r *Runtime) linkedCSSProvider() LinkedCSSFunc {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.linkedCSS
}

// setupStyleSheetInterfaces installs the CSSOM surface needed to reflect the
// document's <style> and <link rel=stylesheet> elements. The collection is
// live; each sheet, MediaList and rule wrapper is cached so repeated reads
// preserve object identity.
func (r *Runtime) setupStyleSheetInterfaces() {
	// The collection interfaces exist whether or not a page has read a list
	// yet: Chromium answers typeof StyleSheetList "function" on a document with
	// no sheets at all.
	r.cssomCollectionProto("StyleSheetList")
	r.cssomCollectionProto("CSSRuleList")
	r.cssomCollectionProto("MediaList")
	r.cssRuleProto = r.vm.NewObject()
	for name, value := range map[string]int{
		"STYLE_RULE": 1, "IMPORT_RULE": 3, "MEDIA_RULE": 4,
		"FONT_FACE_RULE": 5, "PAGE_RULE": 6, "KEYFRAMES_RULE": 7,
	} {
		_ = r.cssRuleProto.Set(name, value)
	}
	r.cssStyleRuleProto = r.vm.NewObject()
	_ = r.cssStyleRuleProto.SetPrototype(r.cssRuleProto)
	r.cssStyleSheetProto = r.vm.NewObject()
	_ = r.cssStyleSheetProto.DefineAccessorProperty("cssRules", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.cssRuleListForSheet(r.sheetOwner(call.This))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.cssStyleSheetProto.DefineAccessorProperty("rules", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.cssRuleListForSheet(r.sheetOwner(call.This))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.cssStyleSheetProto.DefineAccessorProperty("ownerNode", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.wrapNode(r.sheetOwner(call.This))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	// An inline sheet has no URL of its own; a linked one reports the absolute
	// URL its text came from. Measured over http, a re-enabled link's sheet
	// reads "http://127.0.0.1:8931/dup.css" verbatim and every <style> reads
	// null; HTMLLinkElement.href on the same link is the same absolute URL.
	_ = r.cssStyleSheetProto.DefineAccessorProperty("href", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if url, ok := r.linkedSheetHref(r.sheetOwner(call.This)); ok {
			return r.vm.ToValue(url)
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.cssStyleSheetProto.DefineAccessorProperty("media", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.mediaListOf(r.sheetOwner(call.This))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	// The title of a sheet is its owner's title attribute. Measured: the
	// <link title="Alternate"> entry reports "Alternate", an appended
	// <style title="StyleTitle"> reports "StyleTitle", and every sheet with no
	// title attribute reports null rather than "".
	_ = r.cssStyleSheetProto.DefineAccessorProperty("title", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		owner := r.sheetOwner(call.This)
		if owner == nil || !owner.HasAttribute("title") {
			return goja.Null()
		}
		return r.vm.ToValue(owner.GetAttribute("title"))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	// disabled is the sheet's own flag rather than the element's attribute:
	// measured, setting it on a listed sheet reads back true, leaves the sheet
	// at the same index, and writes no attribute on the link.
	_ = r.cssStyleSheetProto.DefineAccessorProperty("disabled", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.sheetDisabled[r.sheetOwner(call.This)])
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		owner := r.sheetOwner(call.This)
		if owner == nil {
			return goja.Undefined()
		}
		if r.sheetDisabled == nil {
			r.sheetDisabled = make(map[*dom.Node]bool)
		}
		r.sheetDisabled[owner] = call.Argument(0).ToBoolean()
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.cssStyleSheetProto.DefineAccessorProperty("type", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue("text/css")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.cssStyleSheetProto.Set("replaceSync", func(call goja.FunctionCall) goja.Value {
		sheet, ok := call.This.(*goja.Object)
		if !ok {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		text := ""
		if len(call.Arguments) > 0 {
			text = call.Arguments[0].String()
		}
		_ = sheet.Set("__constructedCSS", text)
		return goja.Undefined()
	})

	ctor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		obj := r.vm.NewObject()
		_ = obj.SetPrototype(r.cssStyleSheetProto)
		return obj
	}).(*goja.Object)
	if ok {
		_ = ctor.Set("prototype", r.cssStyleSheetProto)
		_ = r.cssStyleSheetProto.Set("constructor", ctor)
		// String(sheet) carries the interface name: measured as
		// "[object CSSStyleSheet]" for both a linked and an inline sheet,
		// where this engine answered "[object Object]" without the brand.
		r.defineInterfaceToStringTag("CSSStyleSheet", r.cssStyleSheetProto)
		_ = r.vm.Set("CSSStyleSheet", ctor)
	}

	cssRuleCtor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object { return r.vm.NewObject() }).(*goja.Object)
	if ok {
		_ = cssRuleCtor.Set("prototype", r.cssRuleProto)
		_ = r.cssRuleProto.Set("constructor", cssRuleCtor)
		_ = r.vm.Set("CSSRule", cssRuleCtor)
	}
	cssStyleRuleCtor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object { return r.vm.NewObject() }).(*goja.Object)
	if ok {
		_ = cssStyleRuleCtor.Set("prototype", r.cssStyleRuleProto)
		_ = r.cssStyleRuleProto.Set("constructor", cssStyleRuleCtor)
		_ = r.vm.Set("CSSStyleRule", cssStyleRuleCtor)
	}

	if r.documentProto != nil {
		_ = r.documentProto.DefineAccessorProperty("styleSheets", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.This)
			if node == nil || node.Doc == nil {
				return r.newStyleSheetList(nil)
			}
			return r.newStyleSheetList(node.Doc)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
}

func (r *Runtime) installDetachedStyleSheets(obj *goja.Object, doc *dom.Document) {
	_ = obj.DefineAccessorProperty("styleSheets", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.newStyleSheetList(doc)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

func (r *Runtime) newStyleSheetList(doc *dom.Document) goja.Value {
	if doc == nil {
		doc = r.domDoc
	}
	if doc == nil {
		return r.newStyleSheetCollection(nil)
	}
	if r.styleSheetLists == nil {
		r.styleSheetLists = make(map[*dom.Document]goja.Value)
	}
	if obj, ok := r.styleSheetLists[doc]; ok {
		return obj
	}
	obj := r.newStyleSheetCollection(doc)
	r.styleSheetLists[doc] = obj
	return obj
}

// newStyleSheetCollection builds the live document.styleSheets list. Every read
// walks the document again, so a sheet a script appends, disables or removes is
// visible without any rebuild: measured on the fixture, clearing L8's disabled
// flag puts its sheet back at index 7 and takes the length from 12 to 13, and
// removing L1 takes its entry with the length down to 11 and leaves L2 first.
func (r *Runtime) newStyleSheetCollection(doc *dom.Document) goja.Value {
	owners := func() []*dom.Node { return styleSheetOwners(doc) }
	target := r.vm.NewObject()
	_ = target.SetPrototype(r.cssomCollectionProto("StyleSheetList"))
	return r.cssomCollection(r.vm.ToValue(r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		// length and the indices are answered here so the list stays live: a
		// sheet a script appends, removes or disables moves through the one
		// object the page already holds, with no rebuild and no second list.
		Get: func(proxied *goja.Object, property string, _ goja.Value) goja.Value {
			if property == "length" {
				return r.vm.ToValue(len(owners()))
			}
			if isCanonicalIndex(property) {
				index, _ := strconv.Atoi(property)
				nodes := owners()
				if index >= 0 && index < len(nodes) {
					return r.wrapCSSStyleSheet(nodes[index])
				}
				return goja.Undefined()
			}
			return proxied.Get(property)
		},
		GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value {
			nodes := owners()
			if index >= 0 && index < len(nodes) {
				return r.wrapCSSStyleSheet(nodes[index])
			}
			return goja.Undefined()
		},
		HasIdx: func(_ *goja.Object, index int) bool { return index >= 0 && index < len(owners()) },
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			nodes := owners()
			if index < 0 || index >= len(nodes) {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: r.wrapCSSStyleSheet(nodes[index]), Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(proxied *goja.Object) *goja.Object {
			nodes := owners()
			keys := make([]interface{}, 0, len(nodes)+len(proxied.Keys()))
			for index := range nodes {
				keys = append(keys, strconv.Itoa(index))
			}
			for _, key := range proxied.Keys() {
				keys = append(keys, key)
			}
			return r.vm.NewArray(keys...)
		},
	})), "StyleSheetList")
}

// styleSheetOwners lists the elements whose sheets document.styleSheets
// reports, in document order over the whole tree with template contents
// skipped. Measured against Chromium on the fixture, the 12 entries are
//
//	LINK#L1 LINK#L2 LINK#L3 LINK#L4 LINK#L5 LINK#L6 LINK#L7 STYLE#S1
//	STYLE#S2 LINK#L9 STYLE#S3 STYLE#S5
//
// which is plain document order, and the fixture was built to pin three rules
// inside it: a link inserted by a parser-blocking script lands where its
// element sits, so L9 follows the two head <style> elements and precedes the
// body ones; a link with the disabled attribute contributes no entry (L8,
// disabled from script, and L10, disabled in the markup, are both absent) while
// a <style> carrying the same attribute still does, because HTMLStyleElement
// has no disabled IDL and Chromium lists and applies S5; and neither the sheet
// inside <template id="TPL"> nor the one adopted through
// document.adoptedStyleSheets is listed.
func styleSheetOwners(doc *dom.Document) []*dom.Node {
	if doc == nil || doc.IsXML {
		return nil
	}
	var nodes []*dom.Node
	var walk func(*dom.Node)
	walk = func(parent *dom.Node) {
		for node := parent.FirstChild; node != nil; node = node.NextSibling {
			if node.Element() && node.Namespace == dom.NSHTML {
				switch node.Data {
				case "style":
					nodes = append(nodes, node)
				case "link":
					if _, ok := linkedSheetReference(node); ok {
						nodes = append(nodes, node)
					}
				}
			}
			if node.IsHTMLTemplate() {
				continue
			}
			walk(node)
		}
	}
	walk(&doc.Node)
	return nodes
}

// linkedSheetReference reports the raw reference of the style sheet a <link>
// points at, and false when the element owns no sheet at all: no stylesheet rel
// token, the disabled attribute set, or an empty href. The false cases are
// measured - an appended `<link rel=stylesheet>` with no href reports a null
// .sheet and is not listed, and a disabled link does the same even though its
// href resolves.
func linkedSheetReference(node *dom.Node) (string, bool) {
	if node == nil || node.HasAttribute("disabled") {
		return "", false
	}
	isCSS := false
	for _, tok := range strings.Fields(node.GetAttribute("rel")) {
		if strings.EqualFold(tok, "stylesheet") {
			isCSS = true
		}
	}
	if !isCSS {
		return "", false
	}
	href := strings.TrimSpace(node.GetAttribute("href"))
	if href == "" {
		return "", false
	}
	return href, true
}

// linkedSheetHref resolves a <link>'s reference the way HTMLLinkElement.href
// does, so the CSSOM and the element's own IDL cannot disagree about one URL.
// Without a document base to resolve against the raw reference answers, which
// is resolveURLString's existing fallback.
func (r *Runtime) linkedSheetHref(node *dom.Node) (string, bool) {
	href, ok := linkedSheetReference(node)
	if !ok {
		return "", false
	}
	if abs, _, ok := r.resolveURLForNode(href, node); ok {
		return abs, true
	}
	return href, true
}

// installSheetAccessor defines the read-only `.sheet` of an element that can
// own a style sheet. Measured, L1.sheet is the very object
// document.styleSheets[0] is, a link whose fetch 404s still answers a sheet,
// and S1.sheet is the entry at its index in the list.
func (r *Runtime) installSheetAccessor(p *goja.Object) {
	_ = p.DefineAccessorProperty("sheet", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.ownerStyleSheet(r.getNode(call.This))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

// ownerStyleSheet answers the CSSStyleSheet an element owns, which is the same
// object document.styleSheets hands out for it, or null when the element owns
// no sheet. Membership is decided by the list itself, so the two views cannot
// disagree: measured, L1.sheet === document.styleSheets[0], a link whose fetch
// 404s still has a sheet, a disabled link and a link with no href have none, and
// a <style> inside a template has none even though it is in the document.
func (r *Runtime) ownerStyleSheet(node *dom.Node) goja.Value {
	if node == nil || node.Doc == nil {
		return goja.Null()
	}
	for _, owner := range styleSheetOwners(node.Doc) {
		if owner == node {
			return r.wrapCSSStyleSheet(node)
		}
	}
	return goja.Null()
}

// sheetCSSText is the text one sheet's rules are parsed from: the element's own
// children for a <style>, and the text this navigation fetched for the URL a
// <link> resolves to. A linked sheet nobody fetched has no text, so its rule
// list is empty - the measured shape of the 404 entry, and the honest answer
// wherever the engine never installed a reader.
func (r *Runtime) sheetCSSText(owner *dom.Node) string {
	if owner == nil {
		return ""
	}
	if owner.Element() && owner.Data == "link" {
		url, ok := r.linkedSheetHref(owner)
		if !ok {
			return ""
		}
		if fn := r.linkedCSSProvider(); fn != nil {
			if text, ok := fn(url); ok {
				return text
			}
		}
		return ""
	}
	return owner.TextContent()
}

// cachedCSSRules is one sheet's rule wrappers together with the text they were
// built from, so a sheet whose text changed rebuilds and a sheet whose text is
// the same keeps handing back the objects the page already holds.
type cachedCSSRules struct {
	text  string
	rules []*goja.Object
}

func (r *Runtime) sheetRuleObjects(owner *dom.Node) []*goja.Object {
	text := r.sheetCSSText(owner)
	if owner == nil {
		return r.wrapCSSRules(owner, css.Parse(text).Rules)
	}
	if cached, ok := r.cssRuleLists[owner]; ok && cached.text == text {
		return cached.rules
	}
	rules := r.wrapCSSRules(owner, css.Parse(text).Rules)
	if r.cssRuleLists == nil {
		r.cssRuleLists = make(map[*dom.Node]cachedCSSRules)
	}
	r.cssRuleLists[owner] = cachedCSSRules{text: text, rules: rules}
	return rules
}

func (r *Runtime) wrapCSSRules(owner *dom.Node, parsed []css.Rule) []*goja.Object {
	rules := make([]*goja.Object, len(parsed))
	for i, rule := range parsed {
		rules[i] = r.wrapCSSStyleRule(owner, rule)
	}
	return rules
}

func (r *Runtime) wrapCSSStyleSheet(owner *dom.Node) *goja.Object {
	if owner == nil {
		return r.cssStyleSheetObject(owner)
	}
	if r.cssStyleSheetObjects == nil {
		r.cssStyleSheetObjects = make(map[*dom.Node]*goja.Object)
	}
	if obj := r.cssStyleSheetObjects[owner]; obj != nil {
		return obj
	}
	obj := r.cssStyleSheetObject(owner)
	r.cssStyleSheetObjects[owner] = obj
	return obj
}

func (r *Runtime) cssStyleSheetObject(owner *dom.Node) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(r.cssStyleSheetProto)
	_ = obj.Set("__sheetOwner", r.wrapNode(owner))
	return obj
}

func (r *Runtime) sheetOwner(sheet goja.Value) *dom.Node {
	obj, ok := sheet.(*goja.Object)
	if !ok {
		return nil
	}
	return r.getNode(obj.Get("__sheetOwner"))
}

// mediaListOf answers the one MediaList of one sheet. Measured, Chromium hands
// the same object for every read (`sheet.media === sheet.media`), the text is
// the owner's media attribute read on the spot - after
// `L3.setAttribute("media", "screen and (min-width: 100px)")` the sheet already
// in hand reports the new text - and a sheet with no media attribute reports ""
// with an empty list rather than "all".
func (r *Runtime) mediaListOf(owner *dom.Node) goja.Value {
	if owner == nil {
		return r.newMediaList(owner)
	}
	if list, ok := r.cssMediaLists[owner]; ok {
		return list
	}
	if r.cssMediaLists == nil {
		r.cssMediaLists = make(map[*dom.Node]goja.Value)
	}
	list := r.newMediaList(owner)
	r.cssMediaLists[owner] = list
	return list
}

func (r *Runtime) newMediaList(owner *dom.Node) goja.Value {
	mediaText := func() string {
		if owner == nil {
			return ""
		}
		return owner.GetAttribute("media")
	}
	// A media query list is comma-separated: measured, `media="screen"` answers
	// length 1 with item(0) and [0] both "screen", and no attribute answers
	// length 0 with item(0) null and [0] undefined.
	segments := func() []string {
		text := mediaText()
		if strings.TrimSpace(text) == "" {
			return nil
		}
		parts := strings.Split(text, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out
	}
	proto := r.cssomCollectionProto("MediaList")
	target := r.vm.NewObject()
	_ = target.SetPrototype(proto)
	return r.cssomCollection(r.vm.ToValue(r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(proxied *goja.Object, property string, _ goja.Value) goja.Value {
			// The two live reads answered here are what keeps a MediaList live:
			// the prototype's members reach the same values through `this`, but
			// only this list knows the attribute its owner carries now.
			switch property {
			case "mediaText":
				return r.vm.ToValue(mediaText())
			case "length":
				return r.vm.ToValue(len(segments()))
			}
			if isCanonicalIndex(property) {
				parts := segments()
				index, _ := strconv.Atoi(property)
				if index >= 0 && index < len(parts) {
					return r.vm.ToValue(parts[index])
				}
				return goja.Undefined()
			}
			return proxied.Get(property)
		},
		GetIdx: func(_ *goja.Object, index int, _ goja.Value) goja.Value {
			parts := segments()
			if index >= 0 && index < len(parts) {
				return r.vm.ToValue(parts[index])
			}
			return goja.Undefined()
		},
		HasIdx: func(_ *goja.Object, index int) bool { return index >= 0 && index < len(segments()) },
		GetOwnPropertyDescriptorIdx: func(_ *goja.Object, index int) goja.PropertyDescriptor {
			parts := segments()
			if index < 0 || index >= len(parts) {
				return goja.PropertyDescriptor{}
			}
			return goja.PropertyDescriptor{Value: r.vm.ToValue(parts[index]), Writable: goja.FLAG_FALSE, Enumerable: goja.FLAG_TRUE, Configurable: goja.FLAG_TRUE}
		},
		OwnKeys: func(proxied *goja.Object) *goja.Object {
			parts := segments()
			keys := make([]interface{}, 0, len(parts)+len(proxied.Keys()))
			for index := range parts {
				keys = append(keys, strconv.Itoa(index))
			}
			for _, key := range proxied.Keys() {
				keys = append(keys, key)
			}
			return r.vm.NewArray(keys...)
		},
	})), "MediaList")
}

// cssRuleListForSheet is how the sheet wrappers reach their list: one cached
// CSSRuleList per owner, rebuilt only when the sheet's text changes.
func (r *Runtime) cssRuleListForSheet(owner *dom.Node) goja.Value {
	return r.cssRuleList(owner)
}

func (r *Runtime) wrapCSSStyleRule(owner *dom.Node, rule css.Rule) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(r.cssStyleRuleProto)
	selectorParts := make([]string, len(rule.SelectorStrs))
	for i, part := range rule.SelectorStrs {
		selectorParts[i] = normalizeSelectorFunctionWhitespace(part)
	}
	selector := strings.Join(selectorParts, ", ")
	_ = obj.Set("type", 1)
	_ = obj.Set("selectorText", selector)
	declarations := make([]string, 0, len(rule.Declarations))
	for _, declaration := range rule.Declarations {
		value := declaration.Value
		if declaration.Important {
			value += " !important"
		}
		declarations = append(declarations, declaration.Property+": "+value+";")
	}
	_ = obj.Set("cssText", selector+" { "+strings.Join(declarations, " ")+" }")
	_ = obj.Set("parentStyleSheet", r.wrapCSSStyleSheet(owner))
	return obj
}

// normalizeSelectorFunctionWhitespace removes insignificant whitespace at
// the edges of functional selector arguments for CSSOM serialization.
func normalizeSelectorFunctionWhitespace(selector string) string {
	buf := make([]byte, 0, len(selector))
	type paren struct {
		start int
		quote byte
	}
	stack := make([]paren, 0, 4)
	escaped := false
	for i := 0; i < len(selector); i++ {
		ch := selector[i]
		if escaped {
			buf = append(buf, ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			buf = append(buf, ch)
			escaped = true
			continue
		}
		if len(stack) > 0 && stack[len(stack)-1].quote != 0 {
			buf = append(buf, ch)
			if ch == stack[len(stack)-1].quote {
				stack[len(stack)-1].quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			buf = append(buf, ch)
			if len(stack) > 0 {
				stack[len(stack)-1].quote = ch
			}
			continue
		}
		if ch == '(' {
			buf = append(buf, ch)
			stack = append(stack, paren{start: len(buf)})
			continue
		}
		if len(stack) > 0 && len(buf) == stack[len(stack)-1].start && (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f') {
			continue
		}
		if ch == ')' && len(stack) > 0 {
			frame := stack[len(stack)-1]
			for len(buf) > frame.start && (buf[len(buf)-1] == ' ' || buf[len(buf)-1] == '\t' || buf[len(buf)-1] == '\n' || buf[len(buf)-1] == '\r' || buf[len(buf)-1] == '\f') {
				buf = buf[:len(buf)-1]
			}
			stack = stack[:len(stack)-1]
		}
		buf = append(buf, ch)
	}
	return string(buf)
}
