package js

import (
	"math"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

type LinkedCSSFunc func(url string) (string, bool)

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

func (r *Runtime) setupStyleSheetInterfaces() {
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
	_ = r.cssRuleProto.DefineAccessorProperty("type", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.requireCSSRuleInfo(call.This)
		return r.vm.ToValue(1)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.cssRuleProto.DefineAccessorProperty("cssText", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(cssRuleStyleText(r.requireCSSRuleInfo(call.This).rule))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.cssRuleProto.DefineAccessorProperty("parentStyleSheet", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.wrapCSSStyleSheet(r.requireCSSRuleInfo(call.This).owner)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.cssRuleProto.DefineAccessorProperty("parentRule", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.requireCSSRuleInfo(call.This)
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = r.cssStyleRuleProto.DefineAccessorProperty("selectorText", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue(cssRuleSelectorText(r.requireCSSRuleInfo(call.This).rule))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.cssStyleDeclarationProto = r.vm.NewObject()
	r.defineInterfaceToStringTag("CSSStyleDeclaration", r.cssStyleDeclarationProto)
	_ = r.cssStyleRuleProto.DefineAccessorProperty("style", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		r.requireCSSRuleInfo(call.This)
		obj, _ := call.This.(*goja.Object)
		return r.ruleStyleDeclaration(obj)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
	r.defineInterfaceToStringTag("CSSRule", r.cssRuleProto)
	r.defineInterfaceToStringTag("CSSStyleRule", r.cssStyleRuleProto)
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
	_ = r.cssStyleSheetProto.DefineAccessorProperty("href", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if url, ok := r.linkedSheetHref(r.sheetOwner(call.This)); ok {
			return r.vm.ToValue(url)
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.cssStyleSheetProto.DefineAccessorProperty("media", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.mediaListOf(r.sheetOwner(call.This))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.cssStyleSheetProto.DefineAccessorProperty("title", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		owner := r.sheetOwner(call.This)
		if owner == nil || !owner.HasAttribute("title") {
			return goja.Null()
		}
		return r.vm.ToValue(owner.GetAttribute("title"))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
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
	_ = r.cssStyleSheetProto.Set("insertRule", r.cssomMethod("insertRule", true, func(call goja.FunctionCall) goja.Value {
		owner := r.requireOwnedSheetRules(call.This)
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("Failed to execute 'insertRule' on 'CSSStyleSheet': 1 argument required, but only 0 present."))
		}
		source := r.cssomRuleArgument(call.Argument(0))
		parsed := css.Parse(source)
		if len(parsed.Rules) != 1 {
			panic(r.domError("SyntaxError", "Failed to execute 'insertRule' on 'CSSStyleSheet': Failed to parse the rule '"+source+"'."))
		}
		index := uint32(0)
		if len(call.Arguments) >= 2 {
			index = r.cssomUnsignedLong(call.Argument(1))
		}
		view := r.sheetRuleView(owner)
		max := uint32(len(view.rules))
		if index > max {
			panic(r.domError("IndexSizeError", "Failed to execute 'insertRule' on 'CSSStyleSheet': The index provided ("+strconv.FormatUint(uint64(index), 10)+") is larger than the maximum index ("+strconv.FormatUint(uint64(max), 10)+")."))
		}
		rule := r.wrapCSSStyleRule(owner, parsed.Rules[0])
		out := make([]*goja.Object, 0, len(view.rules)+1)
		out = append(out, view.rules[:index]...)
		out = append(out, rule)
		out = append(out, view.rules[index:]...)
		view.rules = out
		return r.vm.ToValue(index)
	}))
	_ = r.cssStyleSheetProto.Set("deleteRule", r.cssomMethod("deleteRule", true, func(call goja.FunctionCall) goja.Value {
		owner := r.requireOwnedSheetRules(call.This)
		if len(call.Arguments) < 1 {
			panic(r.vm.NewTypeError("Failed to execute 'deleteRule' on 'CSSStyleSheet': 1 argument required, but only 0 present."))
		}
		index := r.cssomUnsignedLong(call.Argument(0))
		view := r.sheetRuleView(owner)
		max := int64(len(view.rules)) - 1
		if int64(index) > max {
			panic(r.domError("IndexSizeError", "Failed to execute 'deleteRule' on 'CSSStyleSheet': The index provided ("+strconv.FormatUint(uint64(index), 10)+") is larger than the maximum index ("+strconv.FormatInt(max, 10)+")."))
		}
		out := make([]*goja.Object, 0, len(view.rules)-1)
		out = append(out, view.rules[:index]...)
		out = append(out, view.rules[index+1:]...)
		view.rules = out
		return goja.Undefined()
	}))

	r.defineInterfaceToStringTag("CSSStyleSheet", r.cssStyleSheetProto)
	_ = r.vm.Set("__cssSheetProto__", r.cssStyleSheetProto)
	ctorValue, ctorErr := r.vm.RunString(`(function(proto) {
		var C = function CSSStyleSheet() {
			if (!new.target) throw new TypeError("Failed to construct 'CSSStyleSheet': Please use the 'new' operator, this DOM object constructor cannot be called as a function.");
		};
		Object.defineProperty(C, 'prototype', { value: proto, writable: false, enumerable: false, configurable: false });
		Object.defineProperty(proto, 'constructor', { value: C, writable: true, enumerable: false, configurable: true });
		return C;
	})(__cssSheetProto__)`)
	_ = r.vm.Set("__cssSheetProto__", nil)
	if ctorErr == nil && ctorValue != nil {
		_ = r.vm.Set("CSSStyleSheet", ctorValue)
	}

	r.defineConstructor("CSSRule", r.cssRuleProto)
	r.defineConstructor("CSSStyleRule", r.cssStyleRuleProto)
	r.defineConstructor("CSSStyleDeclaration", r.cssStyleDeclarationProto)

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

func (r *Runtime) newStyleSheetCollection(doc *dom.Document) goja.Value {
	owners := func() []*dom.Node { return styleSheetOwners(doc) }
	target := r.vm.NewObject()
	_ = target.SetPrototype(r.cssomCollectionProto("StyleSheetList"))
	return r.cssomCollection(r.vm.ToValue(r.vm.NewProxy(target, &goja.ProxyTrapConfig{
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

func (r *Runtime) installSheetAccessor(p *goja.Object) {
	_ = p.DefineAccessorProperty("sheet", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.ownerStyleSheet(r.getNode(call.This))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

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

type cachedCSSRules struct {
	text  string
	rules []*goja.Object
}

func (r *Runtime) sheetRuleView(owner *dom.Node) *cachedCSSRules {
	text := r.sheetCSSText(owner)
	if owner == nil {
		return &cachedCSSRules{text: text, rules: r.wrapCSSRules(owner, css.Parse(text).Rules)}
	}
	if view, ok := r.cssRuleLists[owner]; ok && view.text == text {
		return view
	}
	view := &cachedCSSRules{text: text, rules: r.wrapCSSRules(owner, css.Parse(text).Rules)}
	if r.cssRuleLists == nil {
		r.cssRuleLists = make(map[*dom.Node]*cachedCSSRules)
	}
	r.cssRuleLists[owner] = view
	return view
}

func (r *Runtime) sheetRuleObjects(owner *dom.Node) []*goja.Object {
	return r.sheetRuleView(owner).rules
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
	if r.cssSheetOwners == nil {
		r.cssSheetOwners = make(map[*goja.Object]*dom.Node)
	}
	r.cssSheetOwners[obj] = owner
	return obj
}

func (r *Runtime) sheetOwner(sheet goja.Value) *dom.Node {
	obj, ok := sheet.(*goja.Object)
	if !ok {
		return nil
	}
	return r.cssSheetOwners[obj]
}

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

type mediaListState struct {
	owner    *dom.Node
	override []string
	base     string
}

func (st *mediaListState) attributeText() string {
	if st == nil || st.owner == nil {
		return ""
	}
	return st.owner.GetAttribute("media")
}

func (st *mediaListState) text() string {
	attr := st.attributeText()
	if st.override != nil && attr == st.base {
		return strings.Join(st.override, ", ")
	}
	st.override = nil
	return attr
}

func (st *mediaListState) segments() []string {
	attr := st.attributeText()
	if st.override != nil && attr == st.base {
		return st.override
	}
	st.override = nil
	return mediaSegments(attr)
}

func (r *Runtime) newMediaList(owner *dom.Node) goja.Value {
	state := &mediaListState{owner: owner}
	mediaText := func() string { return state.text() }
	segments := func() []string { return state.segments() }
	proto := r.cssomCollectionProto("MediaList")
	target := r.vm.NewObject()
	_ = target.SetPrototype(proto)
	proxy := r.vm.NewProxy(target, &goja.ProxyTrapConfig{
		Get: func(proxied *goja.Object, property string, _ goja.Value) goja.Value {
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
	})
	value := r.vm.ToValue(proxy)
	if r.cssMediaListStates == nil {
		r.cssMediaListStates = map[*goja.Object]*mediaListState{}
	}
	if obj, ok := value.(*goja.Object); ok {
		r.cssMediaListStates[obj] = state
	}
	return r.cssomCollection(value, "MediaList")
}

func (r *Runtime) mediaStateOf(obj *goja.Object) *mediaListState {
	if obj == nil {
		return nil
	}
	return r.cssMediaListStates[obj]
}

func mediaSegments(text string) []string {
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

func (st *mediaListState) setMediaText(text string) {
	segs := mediaSegments(text)
	if segs == nil {
		segs = []string{}
	}
	st.override = segs
	st.base = st.attributeText()
}

func (st *mediaListState) appendMedium(medium string) {
	if medium == "" {
		return
	}
	segs := st.segments()
	for _, existing := range segs {
		if strings.EqualFold(existing, medium) {
			return
		}
	}
	st.override = append(append([]string{}, segs...), medium)
	st.base = st.attributeText()
}

func (st *mediaListState) deleteMedium(medium string) bool {
	if medium == "" {
		return true
	}
	segs := st.segments()
	for i, existing := range segs {
		if strings.EqualFold(existing, medium) {
			next := append(append([]string{}, segs[:i]...), segs[i+1:]...)
			st.override = next
			st.base = st.attributeText()
			return true
		}
	}
	return false
}

func (r *Runtime) cssRuleListForSheet(owner *dom.Node) goja.Value {
	return r.cssRuleList(owner)
}

func (r *Runtime) wrapCSSStyleRule(owner *dom.Node, rule css.Rule) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(r.cssStyleRuleProto)
	if r.cssRuleWrappers == nil {
		r.cssRuleWrappers = make(map[*goja.Object]cssRuleInfo)
	}
	r.cssRuleWrappers[obj] = cssRuleInfo{owner: owner, rule: rule}
	return obj
}

type cssRuleInfo struct {
	owner *dom.Node
	rule  css.Rule
}

func (r *Runtime) cssRuleInfoOf(value goja.Value) (cssRuleInfo, bool) {
	obj, ok := value.(*goja.Object)
	if !ok {
		return cssRuleInfo{}, false
	}
	info, ok := r.cssRuleWrappers[obj]
	return info, ok
}

func (r *Runtime) requireCSSRuleInfo(value goja.Value) cssRuleInfo {
	info, ok := r.cssRuleInfoOf(value)
	if !ok {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	return info
}

func (r *Runtime) requireOwnedSheetRules(value goja.Value) *dom.Node {
	obj, ok := value.(*goja.Object)
	if !ok {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	owner, registered := r.cssSheetOwners[obj]
	if !registered || owner == nil {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	return owner
}

func (r *Runtime) cssomRuleArgument(value goja.Value) string {
	if value == nil {
		return "undefined"
	}
	if _, isSymbol := value.(*goja.Symbol); isSymbol {
		panic(r.vm.NewTypeError("Cannot convert a Symbol value to a string"))
	}
	return value.String()
}

func (r *Runtime) cssomUnsignedLong(value goja.Value) uint32 {
	if value == nil {
		return 0
	}
	if _, isSymbol := value.(*goja.Symbol); isSymbol {
		panic(r.vm.NewTypeError("Cannot convert a Symbol value to a number"))
	}
	return uint32(value.ToInteger())
}

func (r *Runtime) ruleStyleDeclaration(ruleWrapper *goja.Object) *goja.Object {
	if r.ruleStyleObjects == nil {
		r.ruleStyleObjects = make(map[*goja.Object]*goja.Object)
	}
	if obj := r.ruleStyleObjects[ruleWrapper]; obj != nil {
		return obj
	}
	obj := r.vm.NewDynamicObject(&ruleStyleDeclaration{runtime: r, ruleWrapper: ruleWrapper})
	if r.cssStyleDeclarationProto != nil {
		_ = obj.SetPrototype(r.cssStyleDeclarationProto)
	}
	r.ruleStyleObjects[ruleWrapper] = obj
	return obj
}

func cssRuleSelectorText(rule css.Rule) string {
	parts := make([]string, len(rule.SelectorStrs))
	for i, part := range rule.SelectorStrs {
		parts[i] = normalizeSelectorFunctionWhitespace(part)
	}
	return strings.Join(parts, ", ")
}

func cssRuleDeclarationText(declarations []css.Declaration) string {
	parts := make([]string, 0, len(declarations))
	for _, important := range []bool{false, true} {
		for _, declaration := range declarations {
			if declaration.Important != important {
				continue
			}
			value := serializeDeclarationValue(declaration.Property, declaration.Value)
			if declaration.Important {
				value += " !important"
			}
			parts = append(parts, declaration.Property+": "+value+";")
		}
	}
	return strings.Join(parts, " ")
}

func normalizeDeclarationValue(value string) string {
	out := make([]byte, 0, len(value)+8)
	var quote byte
	escaped := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if escaped {
			out = append(out, ch)
			escaped = false
			continue
		}
		if quote != 0 {
			out = append(out, ch)
			if ch == '\\' {
				escaped = true
			} else if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			out = append(out, ch)
			continue
		}
		out = append(out, ch)
		if ch == ',' && i+1 < len(value) {
			next := value[i+1]
			if next != ' ' && next != '\t' && next != '\n' && next != '\r' && next != '\f' {
				out = append(out, ' ')
			}
		}
	}
	return string(out)
}

func serializeDeclarationValue(property, value string) string {
	if strings.HasPrefix(property, "--") {
		return normalizeDeclarationValue(value)
	}
	return normalizeDeclarationValue(normalizeNumbers(normalizeHexColors(normalizeUrls(value))))
}

func normalizeUrls(value string) string {
	out := make([]byte, 0, len(value)+8)
	var quote byte
	escaped := false
	for i := 0; i < len(value); {
		ch := value[i]
		if escaped {
			out = append(out, ch)
			escaped = false
			i++
			continue
		}
		if quote != 0 {
			out = append(out, ch)
			if ch == '\\' {
				escaped = true
			} else if ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			out = append(out, ch)
			i++
			continue
		}
		if (ch == 'u' || ch == 'U') && strings.EqualFold(value[i:min(i+4, len(value))], "url(") && (i == 0 || !isIdentByte(value[i-1])) {
			if text, consumed, ok := canonicalURLToken(value[i:]); ok {
				out = append(out, text...)
				i += consumed
				continue
			}
			rp := strings.IndexByte(value[i:], ')')
			if rp < 0 {
				out = append(out, value[i:]...)
				break
			}
			end := i + rp + 1
			out = append(out, value[i:end]...)
			i = end
			continue
		}
		out = append(out, ch)
		i++
	}
	return string(out)
}

func canonicalURLToken(s string) (string, int, bool) {
	pos := 4
	for pos < len(s) && isURLSpace(s[pos]) {
		pos++
	}
	if pos >= len(s) {
		return "", 0, false
	}
	var decoded []byte
	switch s[pos] {
	case '"', '\'':
		q := s[pos]
		pos++
		closed := false
		for pos < len(s) {
			c := s[pos]
			if c == '\\' {
				b, n, ok := decodeCSSEscape(s[pos:])
				if !ok {
					pos++
					continue
				}
				decoded = append(decoded, b...)
				pos += n
				continue
			}
			if c == q {
				pos++
				closed = true
				break
			}
			decoded = append(decoded, c)
			pos++
		}
		if !closed {
			return "", 0, false
		}
		for pos < len(s) && isURLSpace(s[pos]) {
			pos++
		}
		if pos >= len(s) || s[pos] != ')' {
			return "", 0, false
		}
		pos++
	default:
		for pos < len(s) {
			c := s[pos]
			if c == '\\' {
				b, n, ok := decodeCSSEscape(s[pos:])
				if !ok {
					pos++
					continue
				}
				decoded = append(decoded, b...)
				pos += n
				continue
			}
			if c == ')' || isURLSpace(c) {
				break
			}
			if c == '"' || c == '\'' || c == '(' {
				return "", 0, false
			}
			decoded = append(decoded, c)
			pos++
		}
		for pos < len(s) && isURLSpace(s[pos]) {
			pos++
		}
		if pos >= len(s) || s[pos] != ')' {
			return "", 0, false
		}
		pos++
	}
	encoded := make([]byte, 0, len(decoded)+2)
	for _, c := range decoded {
		if c == '"' || c == '\\' {
			encoded = append(encoded, '\\')
		}
		encoded = append(encoded, c)
	}
	return "url(\"" + string(encoded) + "\")", pos, true
}

func isURLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func decodeCSSEscape(s string) ([]byte, int, bool) {
	if len(s) < 2 || s[0] != '\\' {
		return nil, 0, false
	}
	c := s[1]
	if c >= 0x80 {
		return []byte{c}, 2, true
	}
	hexDigits := 0
	for hexDigits < len(s)-1 && isHexByte(s[1+hexDigits]) {
		hexDigits++
	}
	if hexDigits > 0 {
		if hexDigits > 6 {
			hexDigits = 6
		}
		cp, err := strconv.ParseUint(s[1:1+hexDigits], 16, 32)
		j := 1 + hexDigits
		if j < len(s) && s[j] == ' ' {
			j++
		}
		if err != nil {
			return []byte{c}, 2, true
		}
		return []byte(string(rune(cp))), j, true
	}
	if c == '\n' || c == '\r' || c == '\f' {
		return nil, 0, false
	}
	return []byte{c}, 2, true
}

func normalizeHexColors(value string) string {
	out := make([]byte, 0, len(value)+8)
	var quote byte
	escaped := false
	for i := 0; i < len(value); {
		ch := value[i]
		if escaped {
			out = append(out, ch)
			escaped = false
			i++
			continue
		}
		if quote != 0 {
			out = append(out, ch)
			if ch == '\\' {
				escaped = true
			} else if ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			out = append(out, ch)
			i++
			continue
		}
		if (ch == 'u' || ch == 'U') && strings.EqualFold(value[i:min(i+4, len(value))], "url(") {
			rp := strings.IndexByte(value[i:], ')')
			if rp < 0 {
				out = append(out, value[i:]...)
				break
			}
			end := i + rp + 1
			out = append(out, value[i:end]...)
			i = end
			continue
		}
		if ch == '#' && (i == 0 || !isIdentByte(value[i-1])) {
			if consumed, css, ok := parseHexColorToken(value[i:]); ok {
				out = append(out, css...)
				i += consumed
				continue
			}
		}
		out = append(out, ch)
		i++
	}
	return string(out)
}

func parseHexColorToken(s string) (int, string, bool) {
	body := s[1:]
	n := 0
	for n < len(body) && isHexByte(body[n]) {
		n++
	}
	if n != 3 && n != 4 && n != 6 && n != 8 {
		return 0, "", false
	}
	if n < len(body) && isIdentByte(body[n]) {
		return 0, "", false
	}
	var r, g, b, a uint8
	switch n {
	case 3:
		r, g, b = hexVal(body[0])*17, hexVal(body[1])*17, hexVal(body[2])*17
		a = 255
	case 4:
		r, g, b, a = hexVal(body[0])*17, hexVal(body[1])*17, hexVal(body[2])*17, hexVal(body[3])*17
	case 6:
		r, g, b = hexPair(body[0], body[1]), hexPair(body[2], body[3]), hexPair(body[4], body[5])
		a = 255
	case 8:
		r, g, b, a = hexPair(body[0], body[1]), hexPair(body[2], body[3]), hexPair(body[4], body[5]), hexPair(body[6], body[7])
	}
	channels := strconv.Itoa(int(r)) + ", " + strconv.Itoa(int(g)) + ", " + strconv.Itoa(int(b))
	if a == 255 {
		return 1 + n, "rgb(" + channels + ")", true
	}
	return 1 + n, "rgba(" + channels + ", " + formatHexAlpha(a) + ")", true
}

func formatHexAlpha(b uint8) string {
	v := float64(b) / 255
	for d := 1; d <= 4; d++ {
		s := strconv.FormatFloat(v, 'f', d, 64)
		x, err := strconv.ParseFloat(s, 64)
		if err != nil {
			continue
		}
		if int(math.Floor(x*255+0.5)) == int(b) {
			for len(s) > 1 && s[len(s)-1] == '0' {
				s = s[:len(s)-1]
			}
			return strings.TrimSuffix(s, ".")
		}
	}
	return strconv.FormatFloat(v, 'f', 3, 64)
}

func hexVal(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func hexPair(hi, lo byte) uint8 {
	return hexVal(hi)<<4 | hexVal(lo)
}

func normalizeNumbers(value string) string {
	out := make([]byte, 0, len(value))
	var quote byte
	escaped := false
	for i := 0; i < len(value); {
		ch := value[i]
		if escaped {
			out = append(out, ch)
			escaped = false
			i++
			continue
		}
		if quote != 0 {
			if ch == '\\' {
				escaped = true
			} else if ch == quote {
				quote = 0
			}
			out = append(out, ch)
			i++
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			out = append(out, ch)
			i++
			continue
		}
		if numberStart(value, i) && (i == 0 || !isIdentByte(value[i-1])) {
			end, normalized, ok := scanNumberLiteral(value, i)
			if ok {
				out = append(out, normalized...)
				i = end
				continue
			}
		}
		out = append(out, ch)
		i++
	}
	return string(out)
}

func numberStart(value string, i int) bool {
	c := value[i]
	if isDigitByte(c) {
		return true
	}
	if c == '.' {
		return i+1 < len(value) && isDigitByte(value[i+1])
	}
	if c == '+' || c == '-' {
		if i+1 < len(value) && isDigitByte(value[i+1]) {
			return true
		}
		return i+2 < len(value) && value[i+1] == '.' && isDigitByte(value[i+2])
	}
	return false
}

func scanNumberLiteral(value string, start int) (int, []byte, bool) {
	i := start
	if value[i] == '+' || value[i] == '-' {
		i++
	}
	for i < len(value) && isDigitByte(value[i]) {
		i++
	}
	if i < len(value) && value[i] == '.' {
		j := i + 1
		for j < len(value) && isDigitByte(value[j]) {
			j++
		}
		if j > i+1 {
			i = j
		}
	}
	if i >= len(value) {
		return i, []byte(normalizeMantissa(value[start:i])), true
	}
	if value[i] == 'e' || value[i] == 'E' {
		return start, nil, false
	}
	return i, []byte(normalizeMantissa(value[start:i])), true
}

func normalizeMantissa(mantissa string) string {
	negative := false
	digits := mantissa
	if digits[0] == '+' {
		digits = digits[1:]
	} else if digits[0] == '-' {
		negative = true
		digits = digits[1:]
	}
	intPart := digits
	fracPart := ""
	if dot := strings.IndexByte(digits, '.'); dot >= 0 {
		intPart = digits[:dot]
		fracPart = digits[dot+1:]
	}
	if intPart == "" {
		intPart = "0"
	}
	for len(intPart) > 1 && intPart[0] == '0' {
		intPart = intPart[1:]
	}
	for len(fracPart) > 0 && fracPart[len(fracPart)-1] == '0' {
		fracPart = fracPart[:len(fracPart)-1]
	}
	if intPart == "0" && fracPart == "" {
		return "0"
	}
	result := intPart
	if fracPart != "" {
		result += "." + fracPart
	}
	if negative {
		result = "-" + result
	}
	return result
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

func isIdentByte(c byte) bool {
	return isDigitByte(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '-' || c >= 0x80
}

func cssRuleStyleText(rule css.Rule) string {
	decls := cssRuleDeclarationText(rule.Declarations)
	if decls == "" {
		return cssRuleSelectorText(rule) + " { }"
	}
	return cssRuleSelectorText(rule) + " { " + decls + " }"
}

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
