package renderer

import (
	"net/url"
	"strings"
)

// This file owns click-activation semantics shared by the widget-tree canvas
// (canvas.go) and the single-surface raster canvas (internal/ui). Hit testing
// returns the deepest layout box — usually a text node inside the clickable
// element — so every activation path must walk up the ancestor chain instead
// of inspecting only the hit node itself.

// FindLinkAncestor walks up from node (inclusive) and returns the nearest
// <a> ancestor carrying a non-empty href. It returns nil when the click did
// not land inside a link.
func FindLinkAncestor(node *RenderNode) *RenderNode {
	for n := node; n != nil; n = n.Parent {
		if n.TagName == "a" {
			if href, ok := n.GetAttribute("href"); ok && strings.TrimSpace(href) != "" {
				return n
			}
			return nil
		}
	}
	return nil
}

// FindButtonAncestor walks up from node (inclusive) and returns the nearest
// <button> or <input> ancestor that activates on click (button, submit,
// image, reset, checkbox, radio). Text-like inputs are not click-activated;
// they are focus-activated (see IsTextInput).
func FindButtonAncestor(node *RenderNode) *RenderNode {
	for n := node; n != nil; n = n.Parent {
		switch n.TagName {
		case "button":
			return n
		case "input":
			t, _ := n.GetAttribute("type")
			switch strings.ToLower(strings.TrimSpace(t)) {
			case "", "submit", "button", "image", "reset", "checkbox", "radio":
				// "" defaults to submit per the HTML spec.
				return n
			}
			return nil
		}
	}
	return nil
}

// FindFormAncestor walks up the parent tree to find the containing form
// element. It returns nil when the node is not inside a form.
func FindFormAncestor(node *RenderNode) *RenderNode {
	for n := node; n != nil; n = n.Parent {
		if n.TagName == "form" {
			return n
		}
	}
	return nil
}

// InputType returns the lower-cased type of an <input> node ("text" when the
// attribute is absent). It returns "" for non-input nodes.
func InputType(node *RenderNode) string {
	if node == nil || node.TagName != "input" {
		return ""
	}
	t, ok := node.GetAttribute("type")
	if !ok || strings.TrimSpace(t) == "" {
		return "text"
	}
	return strings.ToLower(strings.TrimSpace(t))
}

// IsTextInput reports whether the node is an editable text control that
// should receive keyboard focus on click (<input> text variants, <textarea>).
func IsTextInput(node *RenderNode) bool {
	if node == nil {
		return false
	}
	if node.TagName == "textarea" {
		return true
	}
	if node.TagName != "input" {
		return false
	}
	switch InputType(node) {
	case "text", "password", "search", "url", "tel", "email", "number":
		return true
	}
	return false
}

// IsDisabled reports whether the node (or its form-control ancestors)
// carries a disabled attribute.
func IsDisabled(node *RenderNode) bool {
	for n := node; n != nil; n = n.Parent {
		if _, ok := n.GetAttribute("disabled"); ok {
			return true
		}
		if n.TagName == "form" || n.TagName == "body" || n.TagName == "html" {
			break
		}
	}
	return false
}

// CollectFormDataValues gathers name/value pairs from a form subtree using
// only RenderNode attributes (no Fyne widget state). The widget-tree canvas
// prefers live widget state via findWidgetByNodeID; the raster canvas and
// headless paths use this. Disabled controls are skipped. Checkbox/radio
// contribute only when checked (defaulting to "on" like browsers do).
func CollectFormDataValues(formNode *RenderNode) map[string]string {
	data := make(map[string]string)
	var collect func(*RenderNode)
	collect = func(n *RenderNode) {
		if n == nil {
			return
		}
		if n.Type == NodeTypeElement {
			if _, disabled := n.GetAttribute("disabled"); !disabled {
				if name, ok := n.GetAttribute("name"); ok && name != "" {
					switch n.TagName {
					case "input":
						switch InputType(n) {
						case "checkbox", "radio":
							if _, checked := n.GetAttribute("checked"); checked {
								if val, ok := n.GetAttribute("value"); ok {
									data[name] = val
								} else {
									data[name] = "on"
								}
							}
						case "submit", "button", "image", "reset":
							// Submitter values are handled by the caller.
						default:
							val, _ := n.GetAttribute("value")
							data[name] = val
						}
					case "textarea":
						data[name] = textareaValue(n)
					case "select":
						if val, ok := selectedOptionValue(n); ok {
							data[name] = val
						}
					}
				}
			}
		}
		for _, child := range n.Children {
			collect(child)
		}
	}
	collect(formNode)
	return data
}

// textareaValue extracts the current value of a <textarea> node: the value
// attribute when set by editing, otherwise its text content.
func textareaValue(n *RenderNode) string {
	if v, ok := n.GetAttribute("value"); ok {
		return v
	}
	var sb strings.Builder
	var text func(*RenderNode)
	text = func(c *RenderNode) {
		if c == nil {
			return
		}
		if c.Type == NodeTypeText {
			sb.WriteString(c.Text)
		}
		for _, child := range c.Children {
			text(child)
		}
	}
	for _, child := range n.Children {
		text(child)
	}
	return sb.String()
}

// selectedOptionValue returns the value of the selected <option> child:
// the first selected option wins, falling back to the first option.
func selectedOptionValue(selectNode *RenderNode) (string, bool) {
	var first *RenderNode
	for _, child := range selectNode.Children {
		if child.TagName != "option" {
			continue
		}
		if first == nil {
			first = child
		}
		if _, ok := child.GetAttribute("selected"); ok {
			return optionValue(child), true
		}
	}
	if first != nil {
		return optionValue(first), true
	}
	return "", false
}

// optionValue returns an option's value attribute, defaulting to its text.
func optionValue(option *RenderNode) string {
	if v, ok := option.GetAttribute("value"); ok {
		return v
	}
	var sb strings.Builder
	for _, child := range option.Children {
		if child.Type == NodeTypeText {
			sb.WriteString(child.Text)
		}
	}
	return sb.String()
}

// ActivationKind identifies the default action of a click on a node.
type ActivationKind int

const (
	// ActivationNone means the click has no default action.
	ActivationNone ActivationKind = iota
	// ActivationNavigate means the click navigates to Activation.URL.
	ActivationNavigate
	// ActivationStateChanged means the click mutated node state (e.g. a
	// toggled checkbox) and the caller should repaint; Node is the
	// mutated control.
	ActivationStateChanged
	// ActivationFocus means the click focused an editable control for
	// keyboard input; Node is the focused control.
	ActivationFocus
)

// Activation is the default-action outcome of activating a node.
type Activation struct {
	Kind ActivationKind
	URL  string
	Node *RenderNode
}

// ClickActivation computes (and, for toggles, applies) the default action
// of a click on the hit node. Link and submitter URLs are resolved via
// resolve. Checkbox/radio toggles mutate the tree, so callers must hold
// the tree lock (see Renderer.ActivateClick).
//
// Disabled controls never activate. A click inside a link navigates; a
// submitter inside a form submits it; a submitter outside a form and a
// plain button have no default action here (JavaScript listeners, when
// present, run through the JS runtime instead).
func ClickActivation(node *RenderNode, resolve func(string) string) Activation {
	if node == nil || IsDisabled(node) {
		return Activation{Kind: ActivationNone}
	}
	if link := FindLinkAncestor(node); link != nil {
		href, _ := link.GetAttribute("href")
		target := strings.TrimSpace(href)
		if resolve != nil {
			target = resolve(target)
		}
		if strings.TrimSpace(target) == "" {
			return Activation{Kind: ActivationNone}
		}
		return Activation{Kind: ActivationNavigate, URL: target, Node: link}
	}
	if btn := FindButtonAncestor(node); btn != nil {
		switch btn.TagName {
		case "button":
			if t, ok := btn.GetAttribute("type"); ok && strings.EqualFold(strings.TrimSpace(t), "button") {
				return Activation{Kind: ActivationNone, Node: btn}
			}
			return submitActivation(btn, resolve)
		case "input":
			switch InputType(btn) {
			case "checkbox", "radio":
				return toggleActivation(btn)
			case "submit", "image":
				return submitActivation(btn, resolve)
			case "reset":
				return resetActivation(btn)
			case "button":
				return Activation{Kind: ActivationNone, Node: btn}
			}
		}
	}
	if IsTextInput(node) {
		return Activation{Kind: ActivationFocus, Node: node}
	}
	for n := node; n != nil; n = n.Parent {
		if n.TagName == "select" {
			return cycleSelectActivation(n)
		}
		if n.TagName == "a" || n.TagName == "button" || n.TagName == "input" {
			break
		}
	}
	return Activation{Kind: ActivationNone}
}

// submitActivation builds the navigation for a submitter. Outside a form it
// has no default action.
func submitActivation(submitter *RenderNode, resolve func(string) string) Activation {
	form := FindFormAncestor(submitter)
	if form == nil {
		return Activation{Kind: ActivationNone, Node: submitter}
	}
	data := CollectFormDataValues(form)
	if target := BuildFormSubmitURL(form, data, resolve); strings.TrimSpace(target) != "" {
		return Activation{Kind: ActivationNavigate, URL: target, Node: form}
	}
	return Activation{Kind: ActivationNone, Node: submitter}
}

// toggleActivation flips a checkbox or radio. Radios clear same-named
// siblings in the same form (or tree scope when formless), matching
// browser radio-group behavior.
func toggleActivation(btn *RenderNode) Activation {
	kind := InputType(btn)
	if _, checked := btn.GetAttribute("checked"); checked {
		if kind == "checkbox" {
			btn.RemoveAttribute("checked")
			return Activation{Kind: ActivationStateChanged, Node: btn}
		}
		// Clicking a checked radio keeps it checked.
		return Activation{Kind: ActivationNone, Node: btn}
	}
	if kind == "radio" {
		name, _ := btn.GetAttribute("name")
		scope := btn
		for scope.Parent != nil {
			scope = scope.Parent
		}
		if form := FindFormAncestor(btn); form != nil {
			scope = form
		}
		var clear func(*RenderNode)
		clear = func(n *RenderNode) {
			if n == nil {
				return
			}
			if n != btn && n.TagName == "input" && InputType(n) == "radio" {
				if nName, _ := n.GetAttribute("name"); nName == name {
					n.RemoveAttribute("checked")
				}
			}
			for _, child := range n.Children {
				clear(child)
			}
		}
		clear(scope)
	}
	btn.SetAttribute("checked", "")
	return Activation{Kind: ActivationStateChanged, Node: btn}
}

// resetActivation clears user-editable state in the containing form:
// text values, checkables, selected options, and textarea overrides.
func resetActivation(btn *RenderNode) Activation {
	form := FindFormAncestor(btn)
	if form == nil {
		return Activation{Kind: ActivationNone, Node: btn}
	}
	changed := false
	var reset func(*RenderNode)
	reset = func(n *RenderNode) {
		if n == nil {
			return
		}
		switch n.TagName {
		case "input":
			switch InputType(n) {
			case "checkbox", "radio":
				if _, ok := n.GetAttribute("checked"); ok {
					n.RemoveAttribute("checked")
					changed = true
				}
			case "submit", "button", "image", "reset":
			default:
				if _, ok := n.GetAttribute("value"); ok {
					n.RemoveAttribute("value")
					changed = true
				}
			}
		case "textarea":
			if _, ok := n.GetAttribute("value"); ok {
				n.RemoveAttribute("value")
				changed = true
			}
		case "option":
			if _, ok := n.GetAttribute("selected"); ok {
				n.RemoveAttribute("selected")
				changed = true
			}
		}
		for _, child := range n.Children {
			reset(child)
		}
	}
	reset(form)
	if changed {
		return Activation{Kind: ActivationStateChanged, Node: form}
	}
	return Activation{Kind: ActivationNone, Node: btn}
}

// cycleSelectActivation advances the selected option to the next one,
// wrapping around. It powers tap-to-cycle for <select> on canvases that
// cannot host a native dropdown.
func cycleSelectActivation(selectNode *RenderNode) Activation {
	var options []*RenderNode
	for _, child := range selectNode.Children {
		if child.TagName == "option" {
			options = append(options, child)
		}
	}
	if len(options) == 0 {
		return Activation{Kind: ActivationNone, Node: selectNode}
	}
	idx := -1
	for i, opt := range options {
		if _, ok := opt.GetAttribute("selected"); ok {
			idx = i
			break
		}
	}
	next := 0
	if idx >= 0 {
		options[idx].RemoveAttribute("selected")
		next = (idx + 1) % len(options)
	}
	options[next].SetAttribute("selected", "")
	return Activation{Kind: ActivationStateChanged, Node: selectNode}
}

// SetControlValue overwrites the editable value of a text input or
// textarea node. It reports whether the node accepted the value.
func SetControlValue(node *RenderNode, value string) bool {
	if !IsTextInput(node) {
		return false
	}
	node.SetAttribute("value", value)
	return true
}

// ControlValue returns the current editable value of a text input or
// textarea node.
func ControlValue(node *RenderNode) string {
	if node == nil {
		return ""
	}
	if node.TagName == "textarea" {
		return textareaValue(node)
	}
	if node.TagName == "input" {
		v, _ := node.GetAttribute("value")
		return v
	}
	return ""
}

// ActivateClick resolves the click activation for node under the tree lock:
// it walks ancestors from the (usually deep) hit node, applies toggle
// mutations, and resolves URLs against the current page.
func (r *Renderer) ActivateClick(node *RenderNode) Activation {
	r.treeMu.Lock()
	defer r.treeMu.Unlock()
	return ClickActivation(node, r.resolveURL)
}

// SetControlValue sets the editable value of a text control under the tree
// lock. It reports whether the node accepted the value.
func (r *Renderer) SetControlValue(node *RenderNode, value string) bool {
	if node == nil {
		return false
	}
	r.treeMu.Lock()
	defer r.treeMu.Unlock()
	return SetControlValue(node, value)
}

// BuildFormSubmitURL resolves a form's submission target: the action URL
// (resolved against base via resolve) with data encoded as a query string
// for GET (the default). POST targets resolve the action without a query;
// the caller transports the data. It returns "" when there is nothing to
// navigate to.
func BuildFormSubmitURL(formNode *RenderNode, data map[string]string, resolve func(string) string) string {
	if formNode == nil {
		return ""
	}
	action, _ := formNode.GetAttribute("action")
	resolved := action
	if resolve != nil {
		resolved = resolve(action)
	}
	method, _ := formNode.GetAttribute("method")
	if strings.ToUpper(strings.TrimSpace(method)) == "POST" {
		return resolved
	}
	if len(data) == 0 {
		return resolved
	}
	parsed, err := url.Parse(resolved)
	if err != nil {
		return resolved
	}
	query := parsed.Query()
	for k, v := range data {
		query.Set(k, v)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
