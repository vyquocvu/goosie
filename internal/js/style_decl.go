package js

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

type inlineDeclaration struct {
	name     string
	value    string
	priority string
}

// defineStyleAccessor exposes the element's live inline style declaration.
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
	r.styleObjects[node] = obj
	return obj
}

func (r *Runtime) setInlineStyleText(node *dom.Node, value string) {
	if node == nil || !node.Element() {
		return
	}
	before, had := node.GetAttribute("style"), node.HasAttribute("style")
	if value == "" {
		if had {
			node.RemoveAttribute("style")
			r.notifyMutation()
		}
		return
	}
	if !had || before != value {
		node.SetAttribute("style", value)
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
	switch key {
	case "cssText":
		return s.runtime.vm.ToValue(s.node.GetAttribute("style"))
	case "length":
		return s.runtime.vm.ToValue(len(parseInlineStyle(s.node.GetAttribute("style"))))
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
	name := cssNameFromJS(key)
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
	if key == "length" || key == "item" || key == "getPropertyValue" || key == "getPropertyPriority" || key == "setProperty" || key == "removeProperty" {
		return false
	}
	name := cssNameFromJS(key)
	if name == "" {
		return false
	}
	s.setProperty(name, text, "")
	return true
}

func (s *inlineStyleDeclaration) Has(key string) bool {
	switch key {
	case "cssText", "length", "item", "getPropertyValue", "getPropertyPriority", "setProperty", "removeProperty":
		return true
	}
	if i, err := strconv.Atoi(key); err == nil {
		return i >= 0 && i < len(parseInlineStyle(s.node.GetAttribute("style")))
	}
	return cssNameFromJS(key) != ""
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
	name := cssNameFromJS(key)
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
	s.runtime.setInlineStyleText(s.node, strings.Join(parts, "; "))
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
		decls = append(decls, inlineDeclaration{name: name, value: value, priority: priority})
	}
	return decls
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
