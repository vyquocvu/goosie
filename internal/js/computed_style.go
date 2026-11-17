package js

import (
	"sort"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

type ComputedStyleFunc func(node *dom.Node) map[string]string

type PseudoStyleFunc func(node *dom.Node, pseudo string) map[string]string

func (r *Runtime) SetComputedStyleProvider(fn ComputedStyleFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.computedStyle = fn
}

func (r *Runtime) computedStyleProvider() ComputedStyleFunc {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.computedStyle
}

func (r *Runtime) SetPseudoStyleProvider(fn PseudoStyleFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pseudoStyle = fn
}

func (r *Runtime) pseudoStyleProvider() PseudoStyleFunc {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pseudoStyle
}

func (r *Runtime) computedValues(node *dom.Node) map[string]string {
	out := map[string]string{}
	if fn := r.computedStyleProvider(); fn != nil {
		if got := fn(node); got != nil {
			for k, v := range got {
				out[k] = v
			}
		}
	}
	if node.Element() {
		for _, d := range parseInlineStyle(node.GetAttribute("style")) {
			out[d.name] = d.value
		}
	}
	return out
}

type pseudoArgKind int

const (
	pseudoArgElement pseudoArgKind = iota
	pseudoArgPseudo
	pseudoArgInvalid
)

func pseudoElementArg(text string) (string, pseudoArgKind) {
	if text == "" || strings.ContainsAny(text[:1], " \t\n\r\f\v") {
		return "", pseudoArgElement
	}
	colons := 0
	for strings.HasPrefix(text, ":") {
		text = text[1:]
		colons++
	}
	if colons > 2 {
		return "", pseudoArgInvalid
	}
	name, ok := css.PseudoElementName(text)
	if !ok {
		if colons > 0 {
			return "", pseudoArgInvalid
		}
		return "", pseudoArgElement
	}
	return name, pseudoArgPseudo
}

func (r *Runtime) jsGetComputedStyle(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'getComputedStyle' on 'Window': parameter 1 is not of type 'Element'."))
	}
	node := r.getNode(call.Argument(0))
	if node == nil {
		panic(r.vm.NewTypeError("Failed to execute 'getComputedStyle' on 'Window': parameter 1 is not of type 'Element'."))
	}
	pseudo := ""
	if len(call.Arguments) >= 2 {
		arg := call.Argument(1)
		if !goja.IsNull(arg) && !goja.IsUndefined(arg) {
			name, kind := pseudoElementArg(arg.String())
			switch kind {
			case pseudoArgInvalid:
				return r.computedStyleObject(map[string]string{})
			case pseudoArgPseudo:
				pseudo = name
			}
		}
	}
	if pseudo != "" {
		if fn := r.pseudoStyleProvider(); fn != nil {
			if got := fn(node, pseudo); got != nil {
				return r.computedStyleObject(got)
			}
		}
	}
	values := r.computedValues(node)
	return r.computedStyleObject(values)
}

func (r *Runtime) computedStyleObject(values map[string]string) *goja.Object {
	obj := r.vm.NewObject()
	names := make([]string, 0, len(values))
	for name, value := range values {
		if value != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		value := values[name]
		_ = obj.Set(name, value)
		if camel := camelCSSName(name); camel != "" && camel != name {
			_ = obj.Set(camel, value)
		}
	}
	for i, name := range names {
		_ = obj.Set(strconv.Itoa(i), name)
	}
	_ = obj.Set("length", len(names))
	_ = obj.Set("item", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return r.vm.ToValue("")
		}
		i := int(call.Argument(0).ToInteger())
		if i < 0 || i >= len(names) {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(names[i])
	})
	_ = obj.Set("parentRule", goja.Null())
	_ = obj.Set("getPropertyValue", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return r.vm.ToValue("")
		}
		name := cssNameFromJS(call.Arguments[0].String())
		if v, ok := values[name]; ok {
			return r.vm.ToValue(v)
		}
		return r.vm.ToValue("")
	})
	_ = obj.Set("getPropertyPriority", func(call goja.FunctionCall) goja.Value {
		return r.vm.ToValue("")
	})
	return obj
}

func camelCSSName(name string) string {
	if name == "" || name[0] == '-' {
		return ""
	}
	var b []byte
	up := false
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '-' {
			up = true
			continue
		}
		if up {
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			up = false
		}
		b = append(b, c)
	}
	return string(b)
}

func (r *Runtime) jsDefaultView() {
	doc := r.docObj()
	_ = doc.DefineAccessorProperty("defaultView", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.GlobalObject()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_TRUE)
}
