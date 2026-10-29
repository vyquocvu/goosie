package js

import (
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

// setupStyleSheetInterfaces installs the CSSOM surface needed to reflect
// document-owned <style> elements. The collection is live; each sheet wrapper
// is cached so repeated reads preserve object identity.
func (r *Runtime) setupStyleSheetInterfaces() {
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
	_ = r.cssStyleSheetProto.DefineAccessorProperty("href", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.cssStyleSheetProto.DefineAccessorProperty("type", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue("text/css")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = r.cssStyleSheetProto.DefineAccessorProperty("disabled", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(false)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value { return goja.Undefined() }), goja.FLAG_TRUE, goja.FLAG_TRUE)
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
		r.styleSheetLists = make(map[*dom.Document]*goja.Object)
	}
	if obj := r.styleSheetLists[doc]; obj != nil {
		return obj
	}
	obj := r.newStyleSheetCollection(doc)
	r.styleSheetLists[doc] = obj
	return obj
}

func (r *Runtime) newStyleSheetCollection(doc *dom.Document) *goja.Object {
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(r.ensureCollectionProto())
	for i, node := range styleElements(doc) {
		_ = obj.Set(strconv.Itoa(i), r.wrapCSSStyleSheet(node))
	}
	_ = obj.DefineAccessorProperty("length", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(styleElements(doc)))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.Set("item", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		index := int(call.Argument(0).ToInteger())
		nodes := styleElements(doc)
		if index < 0 || index >= len(nodes) {
			return goja.Null()
		}
		return r.wrapCSSStyleSheet(nodes[index])
	})
	return obj
}

func styleElements(doc *dom.Document) []*dom.Node {
	if doc == nil || doc.IsXML {
		return nil
	}
	var nodes []*dom.Node
	var walk func(*dom.Node)
	walk = func(parent *dom.Node) {
		for node := parent.FirstChild; node != nil; node = node.NextSibling {
			if node.Element() && node.Namespace == dom.NSHTML && node.Data == "style" {
				nodes = append(nodes, node)
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

func (r *Runtime) wrapCSSStyleSheet(owner *dom.Node) *goja.Object {
	if r.cssStyleSheetObjects == nil {
		r.cssStyleSheetObjects = make(map[*dom.Node]*goja.Object)
	}
	if obj := r.cssStyleSheetObjects[owner]; obj != nil {
		return obj
	}
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(r.cssStyleSheetProto)
	_ = obj.Set("__sheetOwner", r.wrapNode(owner))
	r.cssStyleSheetObjects[owner] = obj
	return obj
}

func (r *Runtime) sheetOwner(sheet goja.Value) *dom.Node {
	obj, ok := sheet.(*goja.Object)
	if !ok {
		return nil
	}
	return r.getNode(obj.Get("__sheetOwner"))
}

func (r *Runtime) cssRuleListForSheet(owner *dom.Node) goja.Value {
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(r.ensureCollectionProto())
	rules := r.parsedCSSRules(owner)
	for i, rule := range rules {
		_ = obj.Set(strconv.Itoa(i), r.wrapCSSStyleRule(owner, rule))
	}
	_ = obj.DefineAccessorProperty("length", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(len(rules))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.Set("item", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		index := int(call.Argument(0).ToInteger())
		if index < 0 || index >= len(rules) {
			return goja.Null()
		}
		return r.wrapCSSStyleRule(owner, rules[index])
	})
	return obj
}

func (r *Runtime) parsedCSSRules(owner *dom.Node) []css.Rule {
	if owner == nil {
		return nil
	}
	return css.Parse(owner.TextContent()).Rules
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
