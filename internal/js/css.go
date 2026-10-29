package js

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func (r *Runtime) setupCSS() {
	cssObj := r.vm.NewObject()

	// CSS.escape(ident)
	cssObj.Set("escape", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return r.vm.ToValue("")
		}
		s := call.Argument(0).String()
		return r.vm.ToValue(escapeCSSIdent(s))
	})

	// CSS.supports(prop, val) or CSS.supports(condition)
	cssObj.Set("supports", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return r.vm.ToValue(false)
		}
		if len(call.Arguments) == 1 {
			cond := call.Argument(0).String()
			return r.vm.ToValue(evalSupportsCondition(cond))
		}
		prop := strings.TrimSpace(call.Argument(0).String())
		val := strings.TrimSpace(call.Argument(1).String())
		return r.vm.ToValue(evalSupportsProperty(prop, val))
	})

	// CSS.registerProperty(descriptor)
	cssObj.Set("registerProperty", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("CSS.registerProperty: descriptor object required"))
		}
		descVal := call.Argument(0)
		descObj := descVal.ToObject(r.vm)
		if descObj == nil {
			panic(r.vm.NewTypeError("CSS.registerProperty: descriptor must be an object"))
		}

		nameVal := descObj.Get("name")
		if nameVal == nil || goja.IsUndefined(nameVal) {
			panic(namedError(r, "SyntaxError", "CSS.registerProperty: name descriptor is required"))
		}
		name := strings.TrimSpace(nameVal.String())
		if !strings.HasPrefix(name, "--") || len(name) <= 2 {
			panic(namedError(r, "SyntaxError", "CSS.registerProperty: invalid custom property name: "+name))
		}

		syntaxVal := descObj.Get("syntax")
		if syntaxVal == nil || goja.IsUndefined(syntaxVal) {
			panic(r.vm.NewTypeError("CSS.registerProperty: syntax descriptor is required"))
		}
		syntax := dom.CleanSyntax(syntaxVal.String())
		if err := dom.ValidatePropertySyntax(syntax); err != nil {
			panic(namedError(r, "SyntaxError", "CSS.registerProperty: invalid syntax descriptor: "+syntax))
		}

		inheritsVal := descObj.Get("inherits")
		if inheritsVal == nil || goja.IsUndefined(inheritsVal) || inheritsVal.ExportType().Kind() != reflect.Bool {
			panic(r.vm.NewTypeError("CSS.registerProperty: inherits descriptor is required and must be boolean"))
		}
		inherits := inheritsVal.ToBoolean()

		initialVal := descObj.Get("initialValue")
		initialValue := ""
		hasInitial := false
		if initialVal != nil && !goja.IsUndefined(initialVal) {
			initialValue = strings.TrimSpace(initialVal.String())
			hasInitial = true
		}

		if syntax != "*" {
			if !hasInitial {
				panic(r.vm.NewTypeError("CSS.registerProperty: initialValue is required for syntax " + syntax))
			}
			if !dom.MatchesPropertySyntax(syntax, initialValue) {
				panic(namedError(r, "SyntaxError", "CSS.registerProperty: initialValue does not match syntax "+syntax))
			}
		}

		if r.opts.DOM == nil {
			return goja.Undefined()
		}

		def := dom.PropertyDefinition{
			Name:         name,
			Syntax:       syntax,
			Inherits:     inherits,
			InitialValue: initialValue,
		}

		if !r.opts.DOM.RegisterProperty(def) {
			panic(namedError(r, "InvalidModificationError", "CSS.registerProperty: property "+name+" is already registered"))
		}

		if r.opts.OnMutation != nil {
			r.opts.OnMutation()
		}

		return goja.Undefined()
	})

	r.vm.Set("CSS", cssObj)
	if win := r.vm.Get("window"); win != nil {
		if winObj := win.ToObject(r.vm); winObj != nil {
			winObj.Set("CSS", cssObj)
		}
	}
	r.setupStyleSheetInterfaces()
}

// escapeCSSIdent escapes a string for use as a CSS identifier per CSSOM View §3.1.
func escapeCSSIdent(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if r == 0 {
			b.WriteRune('\uFFFD')
			continue
		}
		if (r >= 0x0001 && r <= 0x001F) || r == 0x007F {
			fmt.Fprintf(&b, "\\%x ", r)
			continue
		}
		if i == 0 && unicode.IsDigit(r) {
			fmt.Fprintf(&b, "\\%x ", r)
			continue
		}
		if i == 1 && r >= '0' && r <= '9' && runes[0] == '-' {
			fmt.Fprintf(&b, "\\%x ", r)
			continue
		}
		if i == 0 && r == '-' && len(runes) == 1 {
			b.WriteString("\\-")
			continue
		}
		if r >= 0x0080 || r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('\\')
			b.WriteRune(r)
		}
	}
	return b.String()
}

func evalSupportsProperty(prop, val string) bool {
	prop = strings.ToLower(strings.TrimSpace(prop))
	val = strings.TrimSpace(val)
	if prop == "" || val == "" {
		return false
	}
	if strings.HasPrefix(prop, "--") {
		return true
	}
	switch prop {
	case "display", "position", "color", "background-color", "width", "height",
		"margin", "padding", "border", "font-size", "opacity", "transform",
		"flex", "grid", "popover", "container", "overflow", "z-index":
		return true
	default:
		return true
	}
}

func evalSupportsCondition(cond string) bool {
	cond = strings.TrimSpace(cond)
	if cond == "" {
		return false
	}
	if strings.HasPrefix(cond, "(") && strings.HasSuffix(cond, ")") {
		inner := strings.TrimSpace(cond[1 : len(cond)-1])
		colon := strings.IndexByte(inner, ':')
		if colon > 0 {
			p := inner[:colon]
			v := inner[colon+1:]
			return evalSupportsProperty(p, v)
		}
	}
	return true
}
