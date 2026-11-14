package js

import (
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

	ctor, ok := r.vm.ToValue(func(call goja.ConstructorCall) *goja.Object {
		obj := r.vm.NewObject()
		_ = obj.SetPrototype(r.cssStyleSheetProto)
		return obj
	}).(*goja.Object)
	if ok {
		_ = ctor.Set("prototype", r.cssStyleSheetProto)
		_ = r.cssStyleSheetProto.Set("constructor", ctor)
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

func (r *Runtime) newMediaList(owner *dom.Node) goja.Value {
	mediaText := func() string {
		if owner == nil {
			return ""
		}
		return owner.GetAttribute("media")
	}
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

func cssRuleSelectorText(rule css.Rule) string {
	parts := make([]string, len(rule.SelectorStrs))
	for i, part := range rule.SelectorStrs {
		parts[i] = normalizeSelectorFunctionWhitespace(part)
	}
	return strings.Join(parts, ", ")
}

func cssRuleStyleText(rule css.Rule) string {
	selector := cssRuleSelectorText(rule)
	declarations := make([]string, 0, len(rule.Declarations))
	for _, declaration := range rule.Declarations {
		value := declaration.Value
		if declaration.Important {
			value += " !important"
		}
		declarations = append(declarations, declaration.Property+": "+value+";")
	}
	return selector + " { " + strings.Join(declarations, " ") + " }"
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
