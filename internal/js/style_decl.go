package js

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

type inlineDeclaration struct {
	name     string
	value    string
	priority string
}

func (r *Runtime) defineStyleAccessor(obj *goja.Object, node *dom.Node) {
	if node == nil || !node.Element() {
		return
	}
	getter := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.styleDeclaration(node)
	})
	setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		value := ""
		if len(call.Arguments) > 0 && call.Arguments[0] != nil {
			value = call.Arguments[0].String()
		}
		r.setInlineStyleText(node, value)
		return goja.Undefined()
	})
	_ = obj.DefineAccessorProperty("style", getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) styleDeclaration(node *dom.Node) *goja.Object {
	if r.styleObjects == nil {
		r.styleObjects = make(map[*dom.Node]*goja.Object)
	}
	if obj := r.styleObjects[node]; obj != nil {
		return obj
	}
	obj := r.vm.NewDynamicObject(&inlineStyleDeclaration{runtime: r, node: node})
	if r.cssStyleDeclarationProto != nil {
		_ = obj.SetPrototype(r.cssStyleDeclarationProto)
	}
	r.styleObjects[node] = obj
	return obj
}

func (r *Runtime) setInlineStyleText(node *dom.Node, value string) {
	if node == nil || !node.Element() {
		return
	}
	value = serializeInlineStyle(value)
	before, had := node.GetAttribute("style"), node.HasAttribute("style")
	if value == "" {
		if had {
			node.RemoveAttribute("style")
			r.notifyCustomElementAttributeChanged(node, "style", before, true, "", false, "")
			r.notifyMutation()
		}
		return
	}
	if !had || before != value {
		node.SetAttribute("style", value)
		r.notifyCustomElementAttributeChanged(node, "style", before, had, value, true, "")
		r.notifyMutation()
	}
}

type inlineStyleDeclaration struct {
	runtime *Runtime
	node    *dom.Node
}

func (s *inlineStyleDeclaration) Get(key string) goja.Value {
	if s == nil || s.runtime == nil || s.runtime.vm == nil || s.node == nil {
		return nil
	}
	if isJSIntrinsicKey(key) {
		return nil
	}
	switch key {
	case "cssText":
		return s.runtime.vm.ToValue(serializeInlineStyle(s.node.GetAttribute("style")))
	case "length":
		return s.runtime.vm.ToValue(len(parseInlineStyle(s.node.GetAttribute("style"))))
	case "parentRule":
		return goja.Null()
	case "item":
		return s.runtime.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return s.runtime.vm.ToValue("")
			}
			i, err := strconv.Atoi(call.Arguments[0].String())
			decls := parseInlineStyle(s.node.GetAttribute("style"))
			if err != nil || i < 0 || i >= len(decls) {
				return s.runtime.vm.ToValue("")
			}
			return s.runtime.vm.ToValue(decls[i].name)
		})
	case "getPropertyValue":
		return s.runtime.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return s.runtime.vm.ToValue("")
			}
			return s.runtime.vm.ToValue(inlineProperty(s.node, call.Arguments[0].String()).value)
		})
	case "getPropertyPriority":
		return s.runtime.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return s.runtime.vm.ToValue("")
			}
			return s.runtime.vm.ToValue(inlineProperty(s.node, call.Arguments[0].String()).priority)
		})
	case "setProperty":
		return s.runtime.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			priority := ""
			if len(call.Arguments) > 2 {
				priority = call.Arguments[2].String()
			}
			s.setProperty(call.Arguments[0].String(), call.Arguments[1].String(), priority)
			return goja.Undefined()
		})
	case "removeProperty":
		return s.runtime.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return s.runtime.vm.ToValue("")
			}
			name := normalizeInlineProperty(call.Arguments[0].String())
			previous := inlineProperty(s.node, name).value
			s.replaceProperty(name, "", "", true)
			return s.runtime.vm.ToValue(previous)
		})
	}
	if i, err := strconv.Atoi(key); err == nil {
		decls := parseInlineStyle(s.node.GetAttribute("style"))
		if i >= 0 && i < len(decls) {
			return s.runtime.vm.ToValue(decls[i].name)
		}
		return nil
	}
	name := idlCSSPropertyName(key)
	if name == "" {
		return nil
	}
	return s.runtime.vm.ToValue(inlineProperty(s.node, name).value)
}

func (s *inlineStyleDeclaration) Set(key string, value goja.Value) bool {
	if s == nil || s.runtime == nil || s.node == nil || value == nil {
		return false
	}
	text := value.String()
	if key == "cssText" {
		s.runtime.setInlineStyleText(s.node, text)
		return true
	}
	if key == "length" || key == "item" || key == "getPropertyValue" || key == "getPropertyPriority" || key == "setProperty" || key == "removeProperty" || key == "parentRule" {
		return false
	}
	name := idlCSSPropertyName(key)
	if name == "" {
		return false
	}
	s.setProperty(name, text, "")
	return true
}

func (s *inlineStyleDeclaration) Has(key string) bool {
	switch key {
	case "cssText", "length", "item", "getPropertyValue", "getPropertyPriority", "setProperty", "removeProperty", "parentRule":
		return true
	}
	if i, err := strconv.Atoi(key); err == nil {
		return i >= 0 && i < len(parseInlineStyle(s.node.GetAttribute("style")))
	}
	return idlCSSPropertyName(key) != ""
}

func (s *inlineStyleDeclaration) Delete(key string) bool {
	if key == "cssText" || key == "length" {
		return false
	}
	if i, err := strconv.Atoi(key); err == nil {
		decls := parseInlineStyle(s.node.GetAttribute("style"))
		if i >= 0 && i < len(decls) {
			s.replaceProperty(decls[i].name, "", "", true)
		}
		return true
	}
	name := idlCSSPropertyName(key)
	if name != "" {
		s.replaceProperty(name, "", "", true)
	}
	return true
}

func (s *inlineStyleDeclaration) Keys() []string {
	decls := parseInlineStyle(s.node.GetAttribute("style"))
	keys := make([]string, len(decls))
	for i := range decls {
		keys[i] = strconv.Itoa(i)
	}
	return keys
}

func (s *inlineStyleDeclaration) setProperty(name, value, priority string) {
	name = normalizeInlineProperty(name)
	priority = strings.ToLower(strings.TrimSpace(priority))
	if name == "" || priority != "" && priority != "important" {
		return
	}
	value = strings.TrimSpace(value)
	if i := strings.LastIndex(value, "!"); i >= 0 && strings.EqualFold(strings.TrimSpace(value[i:]), "!important") {
		value = strings.TrimSpace(value[:i])
		priority = "important"
	}
	s.replaceProperty(name, value, priority, value == "")
}

func (s *inlineStyleDeclaration) replaceProperty(name, value, priority string, remove bool) {
	decls := parseInlineStyle(s.node.GetAttribute("style"))
	updated := make([]inlineDeclaration, 0, len(decls)+1)
	inserted := false
	for _, decl := range decls {
		if sameInlineProperty(decl.name, name) {
			if !remove && !inserted {
				updated = append(updated, inlineDeclaration{name: name, value: value, priority: priority})
				inserted = true
			}
			continue
		}
		updated = append(updated, decl)
	}
	if !remove && !inserted {
		updated = append(updated, inlineDeclaration{name: name, value: value, priority: priority})
	}
	parts := make([]string, 0, len(updated))
	for _, decl := range updated {
		part := decl.name + ": " + decl.value
		if decl.priority != "" {
			part += " !important"
		}
		parts = append(parts, part)
	}
	s.runtime.setInlineStyleText(s.node, strings.Join(parts, "; ")+";")
}

func parseInlineStyle(text string) []inlineDeclaration {
	var decls []inlineDeclaration
	for _, part := range splitInlineDeclarations(text) {
		colon := topLevelColon(part)
		if colon < 0 {
			continue
		}
		name := normalizeInlineProperty(strings.TrimSpace(part[:colon]))
		value := strings.TrimSpace(part[colon+1:])
		if name == "" || value == "" {
			continue
		}
		priority := ""
		if i := strings.LastIndex(value, "!"); i >= 0 && strings.EqualFold(strings.TrimSpace(value[i:]), "!important") {
			value = strings.TrimSpace(value[:i])
			priority = "important"
		}
		if !validInlineStyleValue(name, value) {
			continue
		}
		decls = append(decls, inlineDeclaration{name: name, value: value, priority: priority})
	}
	return decls
}

func validInlineStyleValue(name, value string) bool {
	if value == "" || strings.HasPrefix(value, ":") {
		return false
	}
	if strings.HasPrefix(name, "--") {
		return true
	}
	if name == "color" || strings.HasSuffix(name, "-color") {
		_, ok := css.ParseColor(value)
		return ok
	}
	return true
}

func serializeInlineStyle(text string) string {
	decls := parseInlineStyle(text)
	parts := make([]string, 0, len(decls))
	for _, decl := range decls {
		part := decl.name + ": " + decl.value
		if decl.priority != "" {
			part += " !important"
		}
		parts = append(parts, part+";")
	}
	return strings.Join(parts, " ")
}

func splitInlineDeclarations(text string) []string {
	parts := make([]string, 0, 4)
	start, depth := 0, 0
	var quote rune
	for i, r := range text {
		if quote != 0 {
			if r == quote && (i == 0 || text[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				parts = append(parts, text[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, text[start:])
}

func topLevelColon(text string) int {
	depth := 0
	var quote rune
	for i, r := range text {
		if quote != 0 {
			if r == quote && (i == 0 || text[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func inlineProperty(node *dom.Node, name string) inlineDeclaration {
	name = normalizeInlineProperty(name)
	decls := parseInlineStyle(node.GetAttribute("style"))
	for i := len(decls) - 1; i >= 0; i-- {
		if sameInlineProperty(decls[i].name, name) {
			return decls[i]
		}
	}
	return inlineDeclaration{name: name}
}

func normalizeInlineProperty(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "--") {
		return name
	}
	return strings.ToLower(name)
}

func sameInlineProperty(a, b string) bool {
	if strings.HasPrefix(a, "--") || strings.HasPrefix(b, "--") {
		return a == b
	}
	return strings.EqualFold(a, b)
}

func isJSIntrinsicKey(key string) bool {
	switch key {
	case "constructor", "hasOwnProperty", "isPrototypeOf", "propertyIsEnumerable",
		"toLocaleString", "toString", "valueOf", "__defineGetter__", "__defineSetter__",
		"__lookupGetter__", "__lookupSetter__", "__proto__":
		return true
	}
	return false
}

func idlCSSPropertyName(key string) string {
	if key == "cssFloat" {
		return "float"
	}
	return cssNameFromJS(key)
}

func cssNameFromJS(name string) string {
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "--") || strings.ContainsRune(name, '-') {
		return normalizeInlineProperty(name)
	}
	var b strings.Builder
	for _, r := range name {
		if unicode.IsUpper(r) {
			b.WriteByte('-')
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return normalizeInlineProperty(b.String())
}

type ruleStyleDeclaration struct {
	runtime     *Runtime
	ruleWrapper *goja.Object
}

func (s *ruleStyleDeclaration) declarations() []css.Declaration {
	if s == nil || s.runtime == nil {
		return nil
	}
	info, ok := s.runtime.cssRuleWrappers[s.ruleWrapper]
	if !ok {
		return nil
	}
	return info.rule.Declarations
}

func (s *ruleStyleDeclaration) store(decls []css.Declaration) {
	if s == nil || s.runtime == nil {
		return
	}
	info, ok := s.runtime.cssRuleWrappers[s.ruleWrapper]
	if !ok {
		return
	}
	info.rule.Declarations = decls
	s.runtime.cssRuleWrappers[s.ruleWrapper] = info
}

func ruleDeclarationListFromText(text string) []css.Declaration {
	parsed := css.Parse("s{" + text + "}")
	if len(parsed.Rules) == 0 {
		return nil
	}
	return parsed.Rules[0].Declarations
}

func ruleStyleArgumentText(value goja.Value) string {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return ""
	}
	return value.String()
}

func (s *ruleStyleDeclaration) mutateSet(rawName, value, priority string) {
	name := cssNameFromJS(rawName)
	if name == "" {
		return
	}
	value = strings.TrimSpace(value)
	important := strings.EqualFold(priority, "important")
	if value == "" {
		s.mutateRemove(name)
		return
	}
	decls := s.declarations()
	out := make([]css.Declaration, 0, len(decls)+1)
	replaced := false
	for _, declaration := range decls {
		if !replaced && sameInlineProperty(declaration.Property, name) {
			out = append(out, css.Declaration{Property: name, Value: value, Important: important})
			replaced = true
			continue
		}
		out = append(out, declaration)
	}
	if !replaced {
		out = append(out, css.Declaration{Property: name, Value: value, Important: important})
	}
	s.store(out)
}

func (s *ruleStyleDeclaration) mutateRemove(rawName string) string {
	name := cssNameFromJS(rawName)
	if name == "" {
		return ""
	}
	decls := s.declarations()
	previous := ""
	out := make([]css.Declaration, 0, len(decls))
	removed := false
	for _, declaration := range decls {
		if !removed && sameInlineProperty(declaration.Property, name) {
			previous = serializeDeclarationValue(declaration.Property, declaration.Value)
			removed = true
			continue
		}
		out = append(out, declaration)
	}
	if removed {
		s.store(out)
	}
	return previous
}

func ruleDeclarationValue(decls []css.Declaration, name string) (css.Declaration, bool) {
	name = normalizeInlineProperty(name)
	for i := len(decls) - 1; i >= 0; i-- {
		if sameInlineProperty(decls[i].Property, name) {
			return decls[i], true
		}
	}
	return css.Declaration{}, false
}

func (s *ruleStyleDeclaration) Get(key string) goja.Value {
	if s == nil || s.runtime == nil || s.runtime.vm == nil {
		return nil
	}
	decls := s.declarations()
	switch key {
	case "cssText":
		return s.runtime.vm.ToValue(cssRuleDeclarationText(decls))
	case "length":
		return s.runtime.vm.ToValue(len(decls))
	case "parentRule":
		if s.ruleWrapper != nil {
			return s.ruleWrapper
		}
		return goja.Null()
	case "item":
		return s.runtime.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return s.runtime.vm.ToValue("")
			}
			i, err := strconv.Atoi(call.Arguments[0].String())
			if err != nil || i < 0 || i >= len(decls) {
				return s.runtime.vm.ToValue("")
			}
			return s.runtime.vm.ToValue(decls[i].Property)
		})
	case "getPropertyValue":
		return s.runtime.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return s.runtime.vm.ToValue("")
			}
			decl, ok := ruleDeclarationValue(decls, call.Arguments[0].String())
			if !ok {
				return s.runtime.vm.ToValue("")
			}
			return s.runtime.vm.ToValue(serializeDeclarationValue(decl.Property, decl.Value))
		})
	case "getPropertyPriority":
		return s.runtime.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return s.runtime.vm.ToValue("")
			}
			decl, ok := ruleDeclarationValue(decls, call.Arguments[0].String())
			if !ok || !decl.Important {
				return s.runtime.vm.ToValue("")
			}
			return s.runtime.vm.ToValue("important")
		})
	case "setProperty":
		return s.runtime.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 2 {
				return goja.Undefined()
			}
			priority := ""
			if len(call.Arguments) > 2 {
				priority = ruleStyleArgumentText(call.Arguments[2])
			}
			s.mutateSet(ruleStyleArgumentText(call.Arguments[0]), ruleStyleArgumentText(call.Arguments[1]), priority)
			return goja.Undefined()
		})
	case "removeProperty":
		return s.runtime.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				return s.runtime.vm.ToValue("")
			}
			return s.runtime.vm.ToValue(s.mutateRemove(ruleStyleArgumentText(call.Arguments[0])))
		})
	}
	if i, err := strconv.Atoi(key); err == nil {
		if i >= 0 && i < len(decls) {
			return s.runtime.vm.ToValue(decls[i].Property)
		}
		return nil
	}
	name := idlCSSPropertyName(key)
	if name == "" {
		return nil
	}
	decl, ok := ruleDeclarationValue(decls, name)
	if !ok {
		return nil
	}
	return s.runtime.vm.ToValue(serializeDeclarationValue(decl.Property, decl.Value))
}

func (s *ruleStyleDeclaration) Set(key string, value goja.Value) bool {
	if s == nil || s.runtime == nil || value == nil {
		return false
	}
	switch key {
	case "length", "item", "getPropertyValue", "getPropertyPriority", "setProperty", "removeProperty", "parentRule":
		return false
	}
	text := value.String()
	if key == "cssText" {
		s.store(ruleDeclarationListFromText(text))
		return true
	}
	name := idlCSSPropertyName(key)
	if name == "" {
		return false
	}
	s.mutateSet(name, text, "")
	return true
}

func (s *ruleStyleDeclaration) Has(key string) bool {
	switch key {
	case "cssText", "length", "item", "getPropertyValue", "getPropertyPriority", "setProperty", "removeProperty", "parentRule":
		return true
	}
	if i, err := strconv.Atoi(key); err == nil {
		return i >= 0 && i < len(s.declarations())
	}
	name := idlCSSPropertyName(key)
	if name == "" {
		return false
	}
	_, ok := ruleDeclarationValue(s.declarations(), name)
	return ok
}

func (s *ruleStyleDeclaration) Delete(key string) bool {
	if key == "cssText" || key == "length" {
		return false
	}
	if i, err := strconv.Atoi(key); err == nil {
		decls := s.declarations()
		if i < 0 || i >= len(decls) {
			return false
		}
		out := make([]css.Declaration, 0, len(decls)-1)
		out = append(out, decls[:i]...)
		out = append(out, decls[i+1:]...)
		s.store(out)
		return true
	}
	name := idlCSSPropertyName(key)
	if name == "" {
		return false
	}
	before := len(s.declarations())
	s.mutateRemove(name)
	return len(s.declarations()) < before
}

func (s *ruleStyleDeclaration) Keys() []string {
	decls := s.declarations()
	keys := make([]string, len(decls))
	for i := range decls {
		keys[i] = strconv.Itoa(i)
	}
	return keys
}
