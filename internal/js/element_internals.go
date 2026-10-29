package js

import (
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

// elementInternalsState is the per-element backing state for the Web-exposed
// ElementInternals object. It intentionally lives with the custom element
// instance so repeated attachInternals() calls preserve object identity.
type elementInternalsState struct {
	node              *dom.Node
	formAssociated    bool
	object            *goja.Object
	validity          *goja.Object
	validationMessage string
	validationAnchor  goja.Value
	formValue         goja.Value
	formState         goja.Value
	valid             bool
	flags             map[string]bool
	ariaValues        map[string]goja.Value
	states            *goja.Object
	shadowRoot        goja.Value
}

func (s *elementInternalsState) setARIAValue(name string, value goja.Value) {
	if s.ariaValues == nil {
		s.ariaValues = make(map[string]goja.Value)
	}
	s.ariaValues[name] = value
}

func (r *Runtime) setupElementInternals() {
	if r.elementInternals == nil {
		r.elementInternals = make(map[*dom.Node]*elementInternalsState)
	}
	constructor, ok := r.vm.Get("ElementInternals").(*goja.Object)
	if !ok {
		ctor := func(goja.ConstructorCall) *goja.Object { panic(r.vm.NewTypeError("Illegal constructor")) }
		constructor = r.vm.ToValue(ctor).(*goja.Object)
		prototype := constructor.Get("prototype").(*goja.Object)
		_ = prototype.Set("constructor", constructor)
		_ = constructor.Set("name", "ElementInternals")
		_ = r.vm.Set("ElementInternals", constructor)
		if win, ok := r.vm.Get("window").(*goja.Object); ok {
			_ = win.Set("ElementInternals", constructor)
		}
	}
	prototype := constructor.Get("prototype").(*goja.Object)
	r.setupCustomStateSet()
	if r.htmlElementProto == nil {
		return
	}
	_ = r.htmlElementProto.Set("attachInternals", func(call goja.FunctionCall) goja.Value {
		node := r.getNode(call.This)
		if node == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		def := r.customElementInstances[node]
		constructing := false
		for _, current := range r.customConstructionNodes {
			if current == node {
				constructing = true
				break
			}
		}
		if def == nil && constructing {
			def = r.customElementDefinitionForNode(node)
		}
		if def == nil || (!constructing && !r.isCustomElementInstance(node, def)) || def.extends != "" || def.disableInternals {
			panic(r.domError("NotSupportedError", "ElementInternals cannot be attached to this custom element."))
		}
		if _, exists := r.elementInternals[node]; exists {
			panic(r.domError("NotSupportedError", "ElementInternals has already been attached to this element."))
		}
		state := r.newElementInternals(node, prototype)
		r.elementInternals[node] = state
		return state.object
	})
	_ = prototype.Set("checkValidity", func(call goja.FunctionCall) goja.Value {
		state := r.elementInternalsFor(call.This)
		if state == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		if !state.formAssociated {
			panic(r.domError("NotSupportedError", "The element is not form-associated."))
		}
		if !state.valid {
			dom.DispatchEvent(state.node, dom.NewEvent("invalid", false, true))
		}
		return r.vm.ToValue(state.valid)
	})
	_ = prototype.Set("reportValidity", func(call goja.FunctionCall) goja.Value {
		state := r.elementInternalsFor(call.This)
		if state == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		if !state.formAssociated {
			panic(r.domError("NotSupportedError", "The element is not form-associated."))
		}
		if !state.valid {
			if dom.DispatchEvent(state.node, dom.NewEvent("invalid", false, true)) {
				r.focusInvalidControl(state.node)
			}
		}
		return r.vm.ToValue(state.valid)
	})
	_ = prototype.Set("setFormValue", func(call goja.FunctionCall) goja.Value {
		state := r.elementInternalsFor(call.This)
		if state == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		if !state.formAssociated {
			panic(r.domError("NotSupportedError", "The element is not form-associated."))
		}
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("1 argument required"))
		}
		state.formValue = call.Arguments[0]
		state.formState = call.Arguments[0]
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) {
			state.formState = call.Arguments[1]
		}
		_ = state.object.Set("__formValue", state.formValue)
		_ = state.object.Set("__formState", state.formState)
		return goja.Undefined()
	})
	_ = prototype.Set("setValidity", func(call goja.FunctionCall) goja.Value {
		state := r.elementInternalsFor(call.This)
		if state == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		if !state.formAssociated {
			panic(r.domError("NotSupportedError", "The element is not form-associated."))
		}
		flags := make(map[string]bool)
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) {
			flagObj, ok := call.Arguments[0].(*goja.Object)
			if !ok {
				panic(r.vm.NewTypeError("The first argument is not an object"))
			}
			for _, name := range []string{"badInput", "customError", "patternMismatch", "rangeOverflow", "rangeUnderflow", "stepMismatch", "tooLong", "tooShort", "typeMismatch", "valid", "valueMissing"} {
				if value := flagObj.Get(name); value != nil && value.ToBoolean() {
					flags[name] = true
				}
			}
		}
		invalid := false
		for name, value := range flags {
			if name != "valid" && value {
				invalid = true
				break
			}
		}
		message := ""
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) {
			message = call.Arguments[1].String()
		}
		message = strings.ReplaceAll(strings.ReplaceAll(message, "\r\n", "\n"), "\r", "\n")
		if invalid && message == "" {
			panic(r.vm.NewTypeError("The validation message must not be empty."))
		}
		if !invalid {
			message = ""
		}
		if len(call.Arguments) > 2 && !goja.IsUndefined(call.Arguments[2]) && !goja.IsNull(call.Arguments[2]) {
			anchor := r.getNode(call.Arguments[2])
			if anchor == nil || anchor.Type != dom.NodeElement || anchor.Namespace != dom.NSHTML {
				panic(r.vm.NewTypeError("The validation anchor must be an HTMLElement."))
			}
			if !r.isCustomValidationAnchor(state.node, anchor) {
				panic(r.domError("NotFoundError", "The validation anchor is not a descendant of this element."))
			}
			state.validationAnchor = call.Arguments[2]
		} else {
			state.validationAnchor = goja.Null()
		}
		state.flags = flags
		state.valid = !invalid
		state.node.CustomElementValid = state.valid
		state.validationMessage = message
		for _, name := range []string{"badInput", "customError", "patternMismatch", "rangeOverflow", "rangeUnderflow", "stepMismatch", "tooLong", "tooShort", "typeMismatch", "valid", "valueMissing"} {
			_ = state.validity.Set(name, name == "valid" && state.valid || flags[name])
		}
		_ = state.object.Set("validationAnchor", state.validationAnchor)
		return goja.Undefined()
	})
	r.installElementInternalsARIA(prototype)
	_ = prototype.DefineAccessorProperty("states", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		state := r.elementInternalsFor(call.This)
		if state == nil {
			panic(r.vm.NewTypeError("Illegal invocation"))
		}
		if state.states == nil {
			state.states = r.newCustomStateSet(state.node)
		}
		return state.states
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
}

func (r *Runtime) setupCustomStateSet() {
	constructor, ok := r.vm.Get("CustomStateSet").(*goja.Object)
	if !ok {
		ctor := func(goja.ConstructorCall) *goja.Object { panic(r.vm.NewTypeError("Illegal constructor")) }
		constructor = r.vm.ToValue(ctor).(*goja.Object)
		_ = constructor.Set("name", "CustomStateSet")
		_ = r.vm.Set("CustomStateSet", constructor)
		if win, ok := r.vm.Get("window").(*goja.Object); ok {
			_ = win.Set("CustomStateSet", constructor)
		}
		if result, err := r.vm.RunString(`Object.setPrototypeOf(CustomStateSet.prototype, Set.prototype); Object.defineProperty(CustomStateSet.prototype, "constructor", {value: CustomStateSet, writable: true, configurable: true}); Object.defineProperty(CustomStateSet.prototype, Symbol.toStringTag, {value: "CustomStateSet", configurable: true});`); err == nil {
			_ = result
		}
	}
	prototype := constructor.Get("prototype").(*goja.Object)
	setProto := r.vm.Get("Set").(*goja.Object).Get("prototype").(*goja.Object)
	add, _ := goja.AssertFunction(setProto.Get("add"))
	has, _ := goja.AssertFunction(setProto.Get("has"))
	del, _ := goja.AssertFunction(setProto.Get("delete"))
	clearSet, _ := goja.AssertFunction(setProto.Get("clear"))
	_ = prototype.Set("add", func(call goja.FunctionCall) goja.Value {
		token := call.Argument(0).String()
		_, err := add(call.This, r.vm.ToValue(token))
		if err != nil {
			panic(err)
		}
		if state := r.elementInternalsForStateSet(call.This); state != nil {
			if state.node.CustomStates == nil {
				state.node.CustomStates = make(map[string]bool)
			}
			state.node.CustomStates[token] = true
			r.notifyMutation()
		}
		return call.This
	})
	_ = prototype.Set("has", func(call goja.FunctionCall) goja.Value {
		result, err := has(call.This, r.vm.ToValue(call.Argument(0).String()))
		if err != nil {
			panic(err)
		}
		return result
	})
	_ = prototype.Set("delete", func(call goja.FunctionCall) goja.Value {
		token := call.Argument(0).String()
		result, err := del(call.This, r.vm.ToValue(token))
		if err != nil {
			panic(err)
		}
		if result.ToBoolean() {
			if state := r.elementInternalsForStateSet(call.This); state != nil {
				delete(state.node.CustomStates, token)
				r.notifyMutation()
			}
		}
		return result
	})
	_ = prototype.Set("clear", func(call goja.FunctionCall) goja.Value {
		oldSize := call.This.(*goja.Object).Get("size").ToInteger()
		_, err := clearSet(call.This)
		if err != nil {
			panic(err)
		}
		if oldSize > 0 {
			if state := r.elementInternalsForStateSet(call.This); state != nil {
				clear(state.node.CustomStates)
				r.notifyMutation()
			}
		}
		return goja.Undefined()
	})
	_ = prototype.Set("supports", func(call goja.FunctionCall) goja.Value {
		token := call.Argument(0).String()
		if !strings.HasPrefix(token, "--") || len(token) < 3 || strings.ContainsAny(token, " \t\n\r\f") {
			panic(r.vm.NewTypeError("The custom state name is not a valid custom identifier."))
		}
		return r.vm.ToValue(true)
	})
}

func (r *Runtime) newCustomStateSet(node *dom.Node) *goja.Object {
	constructor, ok := goja.AssertConstructor(r.vm.Get("Set"))
	if !ok {
		return r.vm.NewObject()
	}
	set, err := constructor(nil)
	if err != nil {
		return r.vm.NewObject()
	}
	if stateSetConstructor, ok := r.vm.Get("CustomStateSet").(*goja.Object); ok {
		if proto, ok := stateSetConstructor.Get("prototype").(*goja.Object); ok {
			_ = set.SetPrototype(proto)
		}
	}
	return set
}

func (r *Runtime) elementInternalsForStateSet(value goja.Value) *elementInternalsState {
	for _, state := range r.elementInternals {
		if state.states == value {
			return state
		}
	}
	return nil
}

func (r *Runtime) installElementInternalsARIA(prototype *goja.Object) {
	stringProperties := []string{
		"role", "ariaAtomic", "ariaAutoComplete", "ariaBrailleLabel", "ariaBrailleRoleDescription", "ariaBusy", "ariaChecked", "ariaColCount", "ariaColIndex", "ariaColIndexText", "ariaColSpan", "ariaCurrent", "ariaDescription", "ariaDisabled", "ariaExpanded", "ariaHasPopup", "ariaHidden", "ariaInvalid", "ariaKeyShortcuts", "ariaLabel", "ariaLevel", "ariaLive", "ariaModal", "ariaMultiLine", "ariaMultiSelectable", "ariaOrientation", "ariaPlaceholder", "ariaPosInSet", "ariaPressed", "ariaReadOnly", "ariaRelevant", "ariaRequired", "ariaRoleDescription", "ariaRowCount", "ariaRowIndex", "ariaRowIndexText", "ariaRowSpan", "ariaSelected", "ariaSetSize", "ariaSort", "ariaValueMax", "ariaValueMin", "ariaValueNow", "ariaValueText",
	}
	elementProperties := []string{"ariaActiveDescendantElement"}
	elementListProperties := []string{"ariaControlsElements", "ariaDescribedByElements", "ariaDetailsElements", "ariaErrorMessageElements", "ariaFlowToElements", "ariaLabelledByElements", "ariaOwnsElements"}
	for _, property := range stringProperties {
		name := property
		_ = prototype.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			state := r.elementInternalsFor(call.This)
			if state == nil {
				panic(r.vm.NewTypeError("Illegal invocation"))
			}
			if value, ok := state.ariaValues[name]; ok {
				return value
			}
			return goja.Null()
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			state := r.elementInternalsFor(call.This)
			if state == nil {
				panic(r.vm.NewTypeError("Illegal invocation"))
			}
			value := call.Argument(0)
			if goja.IsNull(value) {
				state.setARIAValue(name, goja.Null())
			} else {
				state.setARIAValue(name, r.vm.ToValue(value.String()))
			}
			r.notifyMutation()
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	for _, property := range elementProperties {
		name := property
		_ = prototype.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			state := r.elementInternalsFor(call.This)
			if state == nil {
				panic(r.vm.NewTypeError("Illegal invocation"))
			}
			if value, ok := state.ariaValues[name]; ok {
				return value
			}
			return goja.Null()
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			state := r.elementInternalsFor(call.This)
			if state == nil {
				panic(r.vm.NewTypeError("Illegal invocation"))
			}
			value := call.Argument(0)
			if goja.IsNull(value) {
				state.setARIAValue(name, goja.Null())
				r.notifyMutation()
				return goja.Undefined()
			}
			if r.getNode(value) == nil || !r.getNode(value).Element() {
				panic(r.vm.NewTypeError("The value is not an Element."))
			}
			state.setARIAValue(name, value)
			r.notifyMutation()
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	for _, property := range elementListProperties {
		name := property
		_ = prototype.DefineAccessorProperty(name, r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			state := r.elementInternalsFor(call.This)
			if state == nil {
				panic(r.vm.NewTypeError("Illegal invocation"))
			}
			if value, ok := state.ariaValues[name]; ok {
				return value
			}
			return goja.Null()
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			state := r.elementInternalsFor(call.This)
			if state == nil {
				panic(r.vm.NewTypeError("Illegal invocation"))
			}
			value := call.Argument(0)
			if goja.IsNull(value) {
				state.setARIAValue(name, goja.Null())
				r.notifyMutation()
				return goja.Undefined()
			}
			arrayFrom := r.vm.Get("Array").(*goja.Object).Get("from")
			from, ok := goja.AssertFunction(arrayFrom)
			if !ok {
				panic(r.vm.NewTypeError("Array.from is unavailable"))
			}
			arrayValue, err := from(r.vm.Get("Array"), value)
			if err != nil {
				panic(err)
			}
			array, ok := arrayValue.(*goja.Object)
			if !ok {
				panic(r.vm.NewTypeError("The value is not a sequence."))
			}
			length := array.Get("length").ToInteger()
			values := make([]goja.Value, 0, length)
			for i := int64(0); i < length; i++ {
				item := array.Get(strconv.FormatInt(i, 10))
				node := r.getNode(item)
				if node == nil || !node.Element() {
					panic(r.vm.NewTypeError("The sequence contains a value that is not an Element."))
				}
				values = append(values, item)
			}
			arrayValues := make([]interface{}, len(values))
			for i, item := range values {
				arrayValues[i] = item
			}
			result := r.vm.NewArray(arrayValues...)
			freeze, ok := goja.AssertFunction(r.vm.Get("Object").(*goja.Object).Get("freeze"))
			if ok {
				_, _ = freeze(r.vm.Get("Object"), result)
			}
			state.setARIAValue(name, result)
			r.notifyMutation()
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
}

func (r *Runtime) newElementInternals(node *dom.Node, prototype *goja.Object) *elementInternalsState {
	obj := r.vm.NewObject()
	_ = obj.SetPrototype(prototype)
	def := r.customElementInstances[node]
	if def == nil {
		def = r.customElementDefinitionForNode(node)
	}
	formAssociated := def != nil && def.formAssociated
	var validity *goja.Object
	if formAssociated {
		validity = r.vm.NewObject()
		for _, name := range []string{"badInput", "customError", "patternMismatch", "rangeOverflow", "rangeUnderflow", "stepMismatch", "tooLong", "tooShort", "typeMismatch", "valueMissing"} {
			_ = validity.Set(name, false)
		}
		_ = validity.Set("valid", true)
	}
	state := &elementInternalsState{node: node, formAssociated: formAssociated, object: obj, validity: validity, valid: true, validationAnchor: goja.Null(), formValue: goja.Null(), formState: goja.Null()}
	state.shadowRoot = r.customElementShadowRootsDuringConstruction[node]
	if state.shadowRoot == nil && node.ShadowRoot != nil && (node.ShadowRoot.Mode == "open" || node.ShadowRoot.Declarative) {
		r.mu.Lock()
		nodeID, wrapped := r.wrapCache[node]
		r.mu.Unlock()
		if wrapped {
			state.shadowRoot = r.jsGetShadowRoot(goja.FunctionCall{Arguments: []goja.Value{r.vm.ToValue(nodeID), r.vm.ToValue(node.ShadowRoot.Declarative)}})
		}
	}
	if state.formAssociated {
		node.FormAssociatedCustomElement = true
		node.CustomElementValid = true
	}
	_ = obj.DefineAccessorProperty("validity", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if !state.formAssociated {
			panic(r.domError("NotSupportedError", "The element is not form-associated."))
		}
		return validity
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineAccessorProperty("validationMessage", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if !state.formAssociated {
			panic(r.domError("NotSupportedError", "The element is not form-associated."))
		}
		return r.vm.ToValue(state.validationMessage)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.Set("validationAnchor", goja.Null())
	_ = obj.DefineAccessorProperty("form", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if !state.formAssociated {
			panic(r.domError("NotSupportedError", "The element is not form-associated."))
		}
		return r.wrapNode(r.customFormOwner(node))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineAccessorProperty("labels", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if !state.formAssociated {
			panic(r.domError("NotSupportedError", "The element is not form-associated."))
		}
		return r.newLiveNodeListMatching(func() []*dom.Node {
			var labelNodes []*dom.Node
			if node.Doc != nil {
				node.Doc.Node.Walk(func(candidate *dom.Node) bool {
					if candidate.Element() && candidate.Data == "label" && r.labelControl(candidate) == node {
						labelNodes = append(labelNodes, candidate)
					}
					return true
				})
			}
			return labelNodes
		})
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineAccessorProperty("willValidate", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if !state.formAssociated {
			panic(r.domError("NotSupportedError", "The element is not form-associated."))
		}
		return r.vm.ToValue(r.formControlWillValidate(node))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = obj.DefineAccessorProperty("shadowRoot", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if state.shadowRoot != nil {
			return state.shadowRoot
		}
		return goja.Null()
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	return state
}

func (r *Runtime) isCustomValidationAnchor(host, anchor *dom.Node) bool {
	return containsNodeIncludingShadow(host, anchor)
}

func containsNodeIncludingShadow(root, target *dom.Node) bool {
	if root == nil {
		return false
	}
	if root == target {
		return true
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if containsNodeIncludingShadow(child, target) {
			return true
		}
	}
	if root.ShadowRoot != nil {
		for child := root.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
			if containsNodeIncludingShadow(child, target) {
				return true
			}
		}
	}
	return false
}

func (r *Runtime) elementInternalsFor(value goja.Value) *elementInternalsState {
	for node, state := range r.elementInternals {
		if state.object == value {
			return state
		}
		_ = node
	}
	return nil
}

// ElementInternalsARIA returns the string-backed ARIA property supplied by
// ElementInternals for node. The engine uses this when deriving accessibility
// semantics; content attributes remain a separate source of ARIA state.
func (r *Runtime) ElementInternalsARIA(node *dom.Node, property string) string {
	if r == nil || node == nil {
		return ""
	}
	state := r.elementInternals[node]
	if state == nil {
		return ""
	}
	value, ok := state.ariaValues[property]
	if !ok || goja.IsNull(value) || goja.IsUndefined(value) {
		return ""
	}
	return value.String()
}

// ElementInternalsAccessibleName resolves host ARIA and ElementInternals name
// sources. The bool reports whether an author-provided name source was set,
// including an explicitly empty or unresolved reference list.
func (r *Runtime) ElementInternalsAccessibleName(node *dom.Node) (string, bool) {
	if r == nil || node == nil {
		return "", false
	}
	state := r.elementInternals[node]
	// ARIA on the host takes precedence over ElementInternals. The IDL
	// Element-reference list is kept separately from its reflected content
	// attribute so an explicit empty or invalid list still overrides internals.
	if node.HasAttribute("aria-labelledby") && node.GetAttribute("aria-labelledby") != "" {
		ids := strings.Fields(node.GetAttribute("aria-labelledby"))
		refs := make([]interface{}, 0, len(ids))
		if node.Doc != nil {
			for _, id := range ids {
				var found *dom.Node
				node.Doc.Node.Walk(func(candidate *dom.Node) bool {
					if candidate.Element() && candidate.GetAttribute("id") == id {
						found = candidate
						return false
					}
					return true
				})
				if found == nil {
					return "", true
				}
				refs = append(refs, r.wrapNode(found))
			}
		}
		return r.accessibleNameFromElementReferences(node, r.vm.NewArray(refs...)), true
	}
	if refs, ok := r.ariaElementListReferences[node]["ariaLabelledByElements"]; ok {
		return r.accessibleNameFromElementReferences(node, refs), true
	}
	if node.HasAttribute("aria-labelledby") {
		return "", true
	}
	if node.HasAttribute("aria-label") {
		return strings.TrimSpace(node.GetAttribute("aria-label")), true
	}
	if state == nil {
		return "", false
	}
	if value, ok := state.ariaValues["ariaLabelledByElements"]; ok && !goja.IsNull(value) && !goja.IsUndefined(value) {
		return r.accessibleNameFromElementReferences(node, value), true
	}
	if value, ok := state.ariaValues["ariaLabel"]; ok && !goja.IsNull(value) && !goja.IsUndefined(value) {
		return value.String(), true
	}
	return "", false
}

func (r *Runtime) accessibleNameFromElementReferences(owner *dom.Node, value goja.Value) string {
	list, ok := value.(*goja.Object)
	if !ok {
		return ""
	}
	ownerRoot := r.shadowRootForNode(owner)
	parts := make([]string, 0, list.Get("length").ToInteger())
	for i, length := int64(0), list.Get("length").ToInteger(); i < length; i++ {
		if ref := r.getNode(list.Get(strconv.FormatInt(i, 10))); ref != nil {
			if ref.Doc != owner.Doc || r.shadowRootForNode(ref) != ownerRoot {
				return ""
			}
			if text := strings.TrimSpace(ref.TextContent()); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, " ")
}

func (r *Runtime) customFormOwner(node *dom.Node) *dom.Node {
	if node == nil {
		return nil
	}
	if id, ok := nullNamespaceAttribute(node, "form"); ok && node.Doc != nil && id != "" {
		var found *dom.Node
		r.walkRadioGroupTreeScope(node, func(candidate *dom.Node) bool {
			if candidate.Element() && candidate.Namespace == dom.NSHTML && candidate.Data == "form" {
				if candidateID, hasID := nullNamespaceAttribute(candidate, "id"); hasID && candidateID == id {
					found = candidate
					return false
				}
			}
			return true
		})
		return found
	}
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Element() && parent.Namespace == dom.NSHTML && parent.Data == "form" {
			return parent
		}
	}
	return nil
}

func (r *Runtime) isFormAssociatedCustomElement(node *dom.Node) bool {
	if node == nil || !node.Element() || node.Namespace != dom.NSHTML {
		return false
	}
	for _, constructing := range r.customConstructionNodes {
		if constructing == node {
			return false
		}
	}
	def := r.customElementInstances[node]
	if def == nil {
		def = r.customElementDefinitionForNode(node)
	}
	return def != nil && def.formAssociated
}
