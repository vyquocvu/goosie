package dom

import (
	"strconv"
	"strings"
	"sync"
)

// NodeType identifies the kind of node in the DOM tree.
type NodeType uint8

const (
	NodeDocument NodeType = iota
	NodeElement
	NodeText
	NodeComment
	NodeDoctype
	NodeDocumentFragment
	NodeCDATASection
	NodeProcessingInstruction
)

// NodeID is a stable, unique identifier for a node within a document. It is
// assigned at creation time and never reused, so style and layout can index
// their per-node data by it without a map.
type NodeID uint32

// Namespace is the XML namespace of an element.
type Namespace uint8

const (
	NSHTML Namespace = iota
	NSSVG
	NSMathML
	NSOther
	// NSXML marks attributes in the XML namespace, which only
	// setAttributeNS can create: the HTML parser never puts xml:lang there,
	// and :lang() only honors the namespaced form.
	NSXML
)

// Attribute is one name-value pair on an element.
type Attribute struct {
	Namespace    Namespace
	NamespaceURI string
	Name         string
	NameAtom     AttrAtom
	Value        string
}

// Node is one node in the DOM tree. Elements, text, comments, and the document
// itself are all nodes; NodeType selects which fields are meaningful.
type Node struct {
	ID           NodeID
	Type         NodeType
	Namespace    Namespace
	NamespaceURI string
	Prefix       string

	// Element fields.
	Data     string
	DataAtom Atom
	Attr     []Attribute

	// Text / comment / doctype fields.
	DataContent string
	// DataUTF16 preserves code units that Go strings cannot represent, such as
	// an isolated UTF-16 surrogate. Nil means DataContent encodes the full value.
	DataUTF16   []uint16
	DoctypeName string
	PublicID    string
	SystemID    string

	// Tree pointers.
	Parent      *Node
	FirstChild  *Node
	LastChild   *Node
	NextSibling *Node
	PrevSibling *Node

	// Shadow DOM.
	ShadowRoot          *ShadowRoot
	TemplateContentNode *Node
	// TemplateHost links a template's inert content fragment back to its host
	// for host-including ancestor checks. Ordinary ParentNode traversal still
	// stops at the fragment, preserving the disconnected DOM tree boundary.
	TemplateHost *Node
	// FormOwner records parser-assigned form ownership for controls whose
	// owner comes from the tree builder's form element pointer rather than
	// ancestor traversal.
	FormOwner *Node
	// SVGScriptReady is set by the HTML parser only when an SVG script is
	// properly ended or uses the foreign-content self-closing flag. An SVG
	// script popped by foreign-content breakout or EOF must not execute.
	SVGScriptReady bool
	// ScriptAlreadyStarted tracks the HTML script element's one-shot execution
	// state across insertion, adoption, and cloning.
	ScriptAlreadyStarted bool

	// Dialog element state.
	IsModal     bool
	ReturnValue string

	// Popover element state.
	IsPopoverOpen bool

	// CSS Scoping state.
	IsScopeRoot bool

	// Form-associated custom-element validity is stored on the DOM node so
	// form algorithms and CSS pseudo-class matching share the same state.
	FormAssociatedCustomElement bool
	CustomElementValid          bool
	// CustomElementUndefined remains true while an HTML custom-element
	// candidate is unresolved and throughout its constructor.
	CustomElementUndefined bool
	CustomStates           map[string]bool
	// ValueChangedByUser tracks the last-value-change mode used by maxlength
	// constraint validation. Script changes to the value attribute clear it.
	ValueChangedByUser bool
	// InputValue is the live value for input states where the value IDL
	// attribute is in value mode. It is separate from the value content
	// attribute, which remains the default value.
	InputValue            string
	InputValueType        string
	InputValueMultiple    bool
	InputValueInitialized bool
	InputValueDirty       bool
	// InputUserEditValue preserves the text displayed while a user is editing
	// controls whose live DOM value is sanitized (currently email inputs).
	InputUserEditValue       string
	InputUserEditValueActive bool
	// InputBadInput records user-entered text that cannot be represented by
	// the control's value (for example, an incomplete number exponent).
	InputBadInput bool
	// InputCheckedness is the current checked state, separate from the checked
	// content attribute (defaultChecked) once script or user interaction makes
	// the state dirty.
	InputCheckedness            bool
	InputCheckednessInitialized bool
	InputCheckednessDirty       bool
	InputIndeterminate          bool
	// OptionSelectedness is the live selected state of an option, separate
	// from its selected content attribute (defaultSelected) after it is dirtied.
	OptionSelectedness            bool
	OptionSelectednessInitialized bool
	OptionSelectednessDirty       bool
	// SelectNoSelection preserves an explicitly cleared selection even for a
	// display-size-one select, whose initial default would otherwise be option 0.
	SelectNoSelection bool
	// OutputDefaultValueOverride records HTMLOutputElement's default value
	// override. The corresponding current value lives in descendant text.
	OutputDefaultValueOverride    string
	OutputDefaultValueOverrideSet bool
	// CustomValidityMessage stores the native form control custom error string.
	CustomValidityMessage string
	// PatternMismatch caches the latest native input pattern check for CSS
	// constraint pseudo-classes, which depend on the same form-control state.
	PatternMismatch bool
	// TypeMismatch caches native email/URL syntax validity for CSS selectors.
	TypeMismatch bool
	// Numeric constraint flags are cached so CSS pseudo-classes share the
	// ValidityState result used by native form validation.
	RangeUnderflow bool
	RangeOverflow  bool
	StepMismatch   bool
	RangeLimited   bool

	// Document back-pointer.
	Doc *Document
}

// ShadowRoot is the root of a shadow tree attached to a host element.
type ShadowRoot struct {
	Host                      *Node
	FirstChild                *Node
	LastChild                 *Node
	Mode                      string // "open" or "closed"
	Declarative               bool
	DelegatesFocus            bool
	SlotAssignment            string // "named" or "manual"
	Clonable                  bool
	CustomElementRegistryNull bool
	// CustomElementRegistryNullDeclarative distinguishes the permanent null
	// registry boundary created by the declarative shadow-root attribute from
	// an imperative null association that can be recomputed after adoption.
	CustomElementRegistryNullDeclarative bool
	AdoptedStyleSheetTexts               []string
	ManualSlots                          map[*Node][]*Node
}

// Element reports whether the node is an element.
func (n *Node) Element() bool { return n.Type == NodeElement }

// InputValueMode reports whether an input type keeps a live value separate
// from its value content attribute.
func InputValueMode(typ string) bool {
	switch strings.ToLower(typ) {
	case "hidden", "checkbox", "radio", "file", "submit", "image", "reset", "button":
		return false
	default:
		return true
	}
}

// inputTypeAttributeChanged applies the value-mode transitions required when
// an input's type state changes. The caller has already written or removed the
// type attribute and captured the previous live value before that mutation.
func (n *Node) inputTypeAttributeChanged(oldType, newType, oldValue string) {
	if n == nil || n.Data != "input" {
		return
	}
	wasValueMode := InputValueMode(oldType)
	isValueMode := InputValueMode(newType)
	switch {
	case wasValueMode && !isValueMode && !strings.EqualFold(newType, "file") && oldValue != "":
		// The current value becomes the default when leaving value mode for a
		// default/default-on state. Keep the dirty flag; the reflected property
		// in the new state reads the attribute directly.
		n.SetAttribute("value", oldValue)
	case !wasValueMode && isValueMode:
		// Re-entering value mode starts from the content attribute and clears
		// dirtiness, after which the new state's sanitizer is applied by JS.
		n.InputValueDirty = false
		n.SyncInputControlValueFromAttribute()
	case !strings.EqualFold(oldType, "file") && strings.EqualFold(newType, "file"):
		// The filename state always starts with an empty value. Runtime-owned
		// FileList state is cleared by RefreshInputPatternMismatch.
		n.InputValue = ""
	}
}

// InputControlValue returns the live value of input and textarea controls,
// including the checkbox and radio default of "on".
func (n *Node) InputControlValue() string {
	if n == nil {
		return ""
	}
	if n.Data == "textarea" {
		if n.InputValueInitialized {
			return n.InputValue
		}
		return normalizeTextareaValue(n.TextContent())
	}
	if n.Data != "input" {
		return n.GetAttribute("value")
	}
	typ := strings.ToLower(n.GetAttribute("type"))
	if !InputValueMode(typ) {
		if n.HasAttribute("value") {
			return n.GetAttribute("value")
		}
		if typ == "checkbox" || typ == "radio" {
			return "on"
		}
		return ""
	}
	if n.InputValueInitialized {
		return n.InputValue
	}
	return n.GetAttribute("value")
}

// InputChecked returns the current checkedness for an input. The checked
// content attribute supplies the initial state until checkedness is changed.
func (n *Node) InputChecked() bool {
	if n == nil || n.Data != "input" {
		return false
	}
	if !n.InputCheckednessInitialized {
		n.InputCheckedness = n.HasAttribute("checked")
		n.InputCheckednessInitialized = true
	}
	return n.InputCheckedness
}

// SetInputChecked stores current checkedness and marks it dirty when the
// change comes from script or user interaction.
func (n *Node) SetInputChecked(checked, dirty bool) {
	if n == nil || n.Data != "input" {
		return
	}
	if !n.InputCheckednessInitialized {
		n.InputCheckedness = n.HasAttribute("checked")
		n.InputCheckednessInitialized = true
	}
	n.InputCheckedness = checked
	if dirty {
		n.InputCheckednessDirty = true
	}
}

// ResetInputChecked restores current checkedness from the checked content
// attribute and clears the dirty-checkedness flag.
func (n *Node) ResetInputChecked() {
	if n == nil || n.Data != "input" {
		return
	}
	n.InputCheckedness = n.HasAttribute("checked")
	n.InputCheckednessInitialized = true
	n.InputCheckednessDirty = false
}

func (n *Node) syncInputCheckednessFromAttribute() {
	if n != nil && n.Data == "input" && !n.InputCheckednessDirty {
		n.InputCheckedness = n.HasAttribute("checked")
		n.InputCheckednessInitialized = true
	}
}

// OptionSelected returns an option's current selectedness. The selected
// content attribute supplies the initial state until selectedness is changed.
func (n *Node) OptionSelected() bool {
	if n == nil || n.Data != "option" {
		return false
	}
	if !n.OptionSelectednessInitialized {
		n.OptionSelectedness = n.HasAttribute("selected")
		n.OptionSelectednessInitialized = true
	}
	return n.OptionSelectedness
}

// SetOptionSelected updates current selectedness and marks it dirty when the
// change comes from script or user interaction. A selected option clears the
// other options in its single-select owner without changing their attributes.
func (n *Node) SetOptionSelected(selected, dirty bool) {
	if n == nil || n.Data != "option" {
		return
	}
	if !n.OptionSelectednessInitialized {
		n.OptionSelectedness = n.HasAttribute("selected")
		n.OptionSelectednessInitialized = true
	}
	n.OptionSelectedness = selected
	if dirty {
		n.OptionSelectednessDirty = true
	}
	if selected {
		if owner := NearestSelect(n); owner != nil {
			owner.SelectNoSelection = false
		}
		if owner := NearestSelect(n); owner != nil && !owner.HasAttribute("multiple") {
			for _, option := range SelectOptions(owner) {
				if option != n {
					option.setOptionSelectedness(false)
				}
			}
		}
	} else if dirty {
		if owner := NearestSelect(n); owner != nil {
			anySelected := false
			for _, option := range SelectOptions(owner) {
				anySelected = anySelected || option.OptionSelected()
			}
			owner.SelectNoSelection = !anySelected
		}
	}
}

func (n *Node) setOptionSelectedness(selected bool) {
	if n != nil && n.Data == "option" {
		n.OptionSelectedness = selected
		n.OptionSelectednessInitialized = true
	}
}

// ResetOptionSelectedness restores an option's current state from its
// selected content attribute and clears its dirty flag.
func (n *Node) ResetOptionSelectedness() {
	if n == nil || n.Data != "option" {
		return
	}
	n.OptionSelectedness = n.HasAttribute("selected")
	n.OptionSelectednessInitialized = true
	n.OptionSelectednessDirty = false
}

func (n *Node) syncOptionSelectednessFromAttribute() {
	if n != nil && n.Data == "option" && !n.OptionSelectednessDirty {
		n.OptionSelectedness = n.HasAttribute("selected")
		n.OptionSelectednessInitialized = true
		if owner := NearestSelect(n); owner != nil {
			if n.OptionSelectedness {
				owner.SelectNoSelection = false
			}
			NormalizeSelectSelectedness(owner)
		}
	}
}

// OptionDisabled reports whether the option is disabled directly or through
// an ancestor optgroup.
func (n *Node) OptionDisabled() bool {
	if n == nil || n.Data != "option" {
		return false
	}
	for parent := n.Parent; parent != nil; parent = parent.Parent {
		if parent.Data == "optgroup" && parent.HasAttribute("disabled") {
			return true
		}
		if parent.Data == "select" {
			break
		}
	}
	return n.HasAttribute("disabled")
}

// IsDisabledByFieldset reports inherited disabledness for disableable HTML
// elements, honoring the first-legend exception for each disabled fieldset.
func IsDisabledByFieldset(node *Node) bool {
	if node == nil || !node.Element() || node.Namespace != NSHTML {
		return false
	}
	if !node.FormAssociatedCustomElement {
		switch node.Data {
		case "button", "fieldset", "input", "select", "textarea":
		default:
			return false
		}
	}
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Namespace != NSHTML || parent.Data != "fieldset" || !parent.HasAttribute("disabled") {
			continue
		}
		var firstLegend *Node
		for child := parent.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && child.Namespace == NSHTML && child.Data == "legend" {
				firstLegend = child
				break
			}
		}
		insideFirstLegend := false
		for current := node; current != nil && current != parent; current = current.Parent {
			if current == firstLegend {
				insideFirstLegend = true
				break
			}
		}
		if !insideFirstLegend {
			return true
		}
	}
	return false
}

// IsFormControlDisabled reports whether a form-associated control is disabled
// by its own attribute or by an ancestor fieldset.
func IsFormControlDisabled(node *Node) bool {
	return node != nil && (node.HasAttribute("disabled") || IsDisabledByFieldset(node))
}

func (n *Node) selectAttributeChanged(name string) {
	if n == nil {
		return
	}
	if n.Data == "select" {
		switch strings.ToLower(name) {
		case "multiple":
			if !n.HasAttribute("multiple") {
				keptOne := false
				for _, option := range SelectOptions(n) {
					if option.OptionSelected() {
						if keptOne {
							option.setOptionSelectedness(false)
						} else {
							keptOne = true
						}
					}
				}
			}
			NormalizeSelectSelectedness(n)
		case "size":
			NormalizeSelectSelectedness(n)
		}
	}
	if n.Data == "option" && strings.EqualFold(name, "disabled") {
		if owner := NearestSelect(n); owner != nil {
			NormalizeSelectSelectedness(owner)
		}
	}
}

// NearestSelect returns the closest ancestor select for an option.
func NearestSelect(option *Node) *Node {
	for parent := optionParent(option); parent != nil; parent = parent.Parent {
		if parent.Data == "select" {
			return parent
		}
	}
	return nil
}

func optionParent(option *Node) *Node {
	if option == nil {
		return nil
	}
	return option.Parent
}

// SelectOptions returns option descendants in tree order.
func SelectOptions(selectNode *Node) []*Node {
	if selectNode == nil || selectNode.Data != "select" {
		return nil
	}
	var options []*Node
	var walk func(*Node)
	walk = func(node *Node) {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if child.Data == "select" {
				continue
			}
			if child.Data == "option" {
				options = append(options, child)
			}
			walk(child)
		}
	}
	walk(selectNode)
	return options
}

func normalizeSelectsAfterTreeMutation(changed, owner *Node) {
	seen := make(map[*Node]bool)
	if owner != nil {
		seen[owner] = true
	}
	if changed != nil {
		visited := make(map[*Node]bool)
		var selectedOptions []*Node
		var walk func(*Node)
		walk = func(node *Node) {
			if node == nil || visited[node] {
				return
			}
			visited[node] = true
			if node.Data == "select" {
				seen[node] = true
			}
			if node.Data == "option" {
				if node.OptionSelected() {
					selectedOptions = append(selectedOptions, node)
				}
				if selectNode := NearestSelect(node); selectNode != nil {
					seen[selectNode] = true
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		walk(changed)
		// An option inserted with selectedness already true wins over the
		// current selection in a single-select control, as required by the
		// option insertion steps. This also handles a selected option moved
		// back into a select after being detached.
		for _, option := range selectedOptions {
			if selectNode := NearestSelect(option); selectNode != nil && !selectNode.HasAttribute("multiple") {
				option.SetOptionSelected(true, false)
				seen[selectNode] = true
			}
		}
	}
	for selectNode := range seen {
		NormalizeSelectSelectedness(selectNode)
	}
}

// NormalizeSelectSelectedness enforces single-select exclusivity and applies
// the display-size-one first-enabled-option default when no option is selected.
func NormalizeSelectSelectedness(selectNode *Node) {
	if selectNode == nil || selectNode.Data != "select" || selectNode.HasAttribute("multiple") {
		return
	}
	options := SelectOptions(selectNode)
	last := -1
	firstEnabled := -1
	for i, option := range options {
		if firstEnabled < 0 && !option.OptionDisabled() {
			firstEnabled = i
		}
		if option.OptionSelected() {
			last = i
		}
	}
	if last < 0 && !selectNode.SelectNoSelection && selectDisplaySize(selectNode) == 1 {
		last = firstEnabled
	}
	for i, option := range options {
		option.setOptionSelectedness(i == last && last >= 0)
	}
}

// SelectValueMissing reports the required-value-missing state for a select.
// A required select is missing a value when it has no selected option, or
// when its only selected option is the placeholder label option of a
// single-line, single-select control.
func SelectValueMissing(selectNode *Node) bool {
	if selectNode == nil || selectNode.Data != "select" || !selectNode.HasAttribute("required") {
		return false
	}
	options := SelectOptions(selectNode)
	var selected *Node
	selectedCount := 0
	for _, option := range options {
		if option.OptionSelected() {
			selected = option
			selectedCount++
		}
	}
	if selectedCount == 0 {
		return true
	}
	if selectedCount != 1 || selectNode.HasAttribute("multiple") || selectDisplaySize(selectNode) != 1 {
		return false
	}
	placeholder := selectPlaceholderLabelOption(selectNode, options)
	return placeholder != nil && selected == placeholder
}

func selectPlaceholderLabelOption(selectNode *Node, options []*Node) *Node {
	if len(options) == 0 || selectOptionValue(options[0]) != "" {
		return nil
	}
	for parent := options[0].Parent; parent != nil && parent != selectNode; parent = parent.Parent {
		if parent.Data == "optgroup" {
			return nil
		}
	}
	return options[0]
}

func selectOptionValue(option *Node) string {
	if option == nil {
		return ""
	}
	if option.HasAttribute("value") {
		return option.GetAttribute("value")
	}
	text := option.TextContent()
	var result strings.Builder
	result.Grow(len(text))
	pendingSpace := false
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\t', '\n', '\f', '\r', ' ':
			pendingSpace = true
		default:
			if pendingSpace && result.Len() > 0 {
				result.WriteByte(' ')
			}
			pendingSpace = false
			result.WriteByte(text[i])
		}
	}
	return result.String()
}

func selectDisplaySize(selectNode *Node) int {
	defaultSize := 1
	if selectNode.HasAttribute("multiple") {
		defaultSize = 4
	}
	raw := strings.TrimSpace(selectNode.GetAttribute("size"))
	if raw == "" {
		return defaultSize
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return defaultSize
	}
	return value
}

// TextareaSubmissionValue returns a textarea's API value after the hard-wrap
// transformation used for form submission. It deliberately leaves the live
// value untouched. This implementation inserts LF after at most cols Unicode
// scalar values on each line; the form encoding path later normalizes LF to
// CRLF as required by the form submission algorithms.
func (n *Node) TextareaSubmissionValue() string {
	if n == nil || n.Data != "textarea" {
		return ""
	}
	value := n.InputControlValue()
	if !strings.EqualFold(n.GetAttribute("wrap"), "hard") {
		return value
	}
	runes := []rune(value)
	width := parseTextareaPositiveAttribute(n.GetAttribute("cols"), 20, len(runes)+1)
	var out strings.Builder
	out.Grow(len(value) + len(runes)/width)
	column := 0
	for i, r := range runes {
		out.WriteRune(r)
		if r == '\n' {
			column = 0
			continue
		}
		column++
		if column == width && i+1 < len(runes) && runes[i+1] != '\n' {
			out.WriteByte('\n')
			column = 0
		}
	}
	return out.String()
}

// TextareaCols and TextareaRows return the visual dimensions with HTML's
// defaults and forgiving integer parsing. Layout caps extreme dimensions to
// keep untrusted attributes from creating unbounded geometry; reflected IDL
// values retain their separate Web IDL bounds.
func (n *Node) TextareaCols() int {
	if n == nil || n.Data != "textarea" {
		return 20
	}
	return parseTextareaPositiveAttribute(n.GetAttribute("cols"), 20, 4096)
}

func (n *Node) TextareaRows() int {
	if n == nil || n.Data != "textarea" {
		return 2
	}
	return parseTextareaPositiveAttribute(n.GetAttribute("rows"), 2, 4096)
}

func parseTextareaPositiveAttribute(attribute string, fallback, ceiling int) int {
	input := strings.TrimLeft(attribute, " \t\n\r\f")
	if input == "" {
		return fallback
	}
	if input[0] == '+' {
		input = input[1:]
	}
	if input == "" || input[0] < '0' || input[0] > '9' {
		return fallback
	}
	value := 0
	for i := 0; i < len(input) && input[i] >= '0' && input[i] <= '9'; i++ {
		digit := int(input[i] - '0')
		if value > (ceiling-digit)/10 {
			return ceiling
		}
		value = value*10 + digit
	}
	if value == 0 {
		return fallback
	}
	if value > ceiling {
		return ceiling
	}
	return value
}

// SetInputControlValue updates an input's live value without changing its
// default-value content attribute.
func (n *Node) SetInputControlValue(value string, dirty bool) {
	if n == nil || (n.Data != "input" && n.Data != "textarea") ||
		(n.Data == "input" && !InputValueMode(n.GetAttribute("type"))) {
		return
	}
	if n.Data == "textarea" {
		value = normalizeTextareaValue(value)
	}
	n.InputValue = value
	n.InputBadInput = false
	n.InputUserEditValue = ""
	n.InputUserEditValueActive = false
	n.InputValueType = strings.ToLower(n.GetAttribute("type"))
	n.InputValueMultiple = n.HasAttribute("multiple")
	n.InputValueInitialized = true
	if dirty {
		n.InputValueDirty = true
	}
}

// SyncInputControlValueFromAttribute applies the current default-value
// attribute to an input whose live value is not dirty.
func (n *Node) SyncInputControlValueFromAttribute() {
	if n == nil || n.InputValueDirty {
		return
	}
	if n.Data == "textarea" {
		n.InputValue = normalizeTextareaValue(n.TextContent())
		n.InputValueInitialized = true
		return
	}
	if n.Data != "input" {
		return
	}
	typ := strings.ToLower(n.GetAttribute("type"))
	value := n.GetAttribute("value")
	if !n.HasAttribute("value") && (typ == "checkbox" || typ == "radio") {
		value = "on"
	}
	n.InputValue = value
	n.InputValueType = typ
	n.InputValueMultiple = n.HasAttribute("multiple")
	n.InputValueInitialized = true
}

// ResetInputControlValue clears the dirty flag and restores the input's
// current value from its value content attribute.
func (n *Node) ResetInputControlValue() {
	if n == nil || (n.Data != "input" && n.Data != "textarea") {
		return
	}
	n.InputValueDirty = false
	n.ValueChangedByUser = false
	n.SyncInputControlValueFromAttribute()
}

// OutputDefaultValue returns the output element's default value, which is an
// override when present and otherwise its descendant text content.
func (n *Node) OutputDefaultValue() string {
	if n == nil || n.Data != "output" {
		return ""
	}
	if n.OutputDefaultValueOverrideSet {
		return n.OutputDefaultValueOverride
	}
	return n.TextContent()
}

// ResetOutputControlValue returns the output's reset value and clears its
// default-value override. The caller replaces the descendant text with the
// returned value as part of the reset algorithm.
func (n *Node) ResetOutputControlValue() string {
	if n == nil || n.Data != "output" {
		return ""
	}
	value := n.OutputDefaultValue()
	n.OutputDefaultValueOverride = ""
	n.OutputDefaultValueOverrideSet = false
	return value
}

func normalizeTextareaValue(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\r", "\n")
}

// SyncTextareaValueFromTextMutation propagates a descendant CharacterData
// change into a clean textarea's live value. Dirty textareas keep their edited
// value while their child text continues to define defaultValue.
func (n *Node) SyncTextareaValueFromTextMutation() {
	for parent := n.Parent; parent != nil; parent = parent.Parent {
		if parent.Data == "textarea" {
			if !parent.InputValueDirty {
				parent.SyncInputControlValueFromAttribute()
			}
			return
		}
	}
}

// Text reports whether the node is a text node.
func (n *Node) Text() bool { return n.Type == NodeText }

// DocumentFragment reports whether the node is a document fragment.
func (n *Node) DocumentFragment() bool { return n.Type == NodeDocumentFragment }

// IsHTMLTemplate distinguishes the HTML template element from foreign-content
// elements that happen to have the same local name (for example SVG <template>).
func (n *Node) IsHTMLTemplate() bool {
	return n != nil && n.Type == NodeElement && n.Namespace == NSHTML && n.Data == "template"
}

// CloneNode returns a duplicate of node n. If deep is true, descendants are also recursively cloned.
func (n *Node) CloneNode(deep bool) *Node {
	if n == nil {
		return nil
	}
	clone := &Node{
		Type:                   n.Type,
		Namespace:              n.Namespace,
		NamespaceURI:           n.NamespaceURI,
		Prefix:                 n.Prefix,
		DoctypeName:            n.DoctypeName,
		PublicID:               n.PublicID,
		SystemID:               n.SystemID,
		Data:                   n.Data,
		DataAtom:               n.DataAtom,
		DataContent:            n.DataContent,
		DataUTF16:              append([]uint16(nil), n.DataUTF16...),
		SVGScriptReady:         n.SVGScriptReady,
		ScriptAlreadyStarted:   n.ScriptAlreadyStarted || (n.Type == NodeElement && n.Namespace == NSHTML && n.Data == "script"),
		CustomElementUndefined: n.CustomElementUndefined,
		Doc:                    n.Doc,
		SelectNoSelection:      n.SelectNoSelection,
	}
	if n.Doc != nil {
		clone.ID = n.Doc.allocID()
	}
	if len(n.Attr) > 0 {
		clone.Attr = make([]Attribute, len(n.Attr))
		copy(clone.Attr, n.Attr)
	}
	if n.Data == "input" {
		clone.InputValue = n.InputValue
		clone.InputValueType = n.InputValueType
		clone.InputValueMultiple = n.InputValueMultiple
		clone.InputValueInitialized = n.InputValueInitialized
		clone.InputValueDirty = n.InputValueDirty
		clone.ValueChangedByUser = n.ValueChangedByUser
		clone.InputIndeterminate = n.InputIndeterminate
		clone.InputCheckedness = n.InputChecked()
		clone.InputCheckednessInitialized = true
		clone.InputCheckednessDirty = n.InputCheckednessDirty
	}
	if n.Data == "textarea" {
		clone.InputValue = n.InputValue
		clone.InputValueInitialized = n.InputValueInitialized
		clone.InputValueDirty = n.InputValueDirty
		clone.ValueChangedByUser = n.ValueChangedByUser
	}
	if n.Data == "option" {
		clone.OptionSelectedness = n.OptionSelected()
		clone.OptionSelectednessInitialized = true
		clone.OptionSelectednessDirty = n.OptionSelectednessDirty
	}
	if deep {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			childClone := c.CloneNode(true)
			clone.AppendChild(childClone)
		}
		if n.ShadowRoot != nil && n.ShadowRoot.Clonable {
			shadowClone := &ShadowRoot{
				Host:                                 clone,
				Mode:                                 n.ShadowRoot.Mode,
				Declarative:                          n.ShadowRoot.Declarative,
				DelegatesFocus:                       n.ShadowRoot.DelegatesFocus,
				SlotAssignment:                       n.ShadowRoot.SlotAssignment,
				Clonable:                             true,
				CustomElementRegistryNull:            n.ShadowRoot.CustomElementRegistryNull,
				CustomElementRegistryNullDeclarative: n.ShadowRoot.CustomElementRegistryNullDeclarative,
			}
			clone.ShadowRoot = shadowClone
			for child := n.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				shadowClone.AppendChild(child.CloneNode(true))
			}
		}
		if n.TemplateContentNode != nil {
			contentDoc := clone.Doc
			if contentDoc != nil {
				contentDoc = contentDoc.TemplateContentsOwnerDocument()
			}
			content := &Node{Type: NodeDocumentFragment, Doc: contentDoc, TemplateHost: clone}
			if contentDoc != nil {
				content.ID = contentDoc.allocID()
			}
			for c := n.TemplateContentNode.FirstChild; c != nil; c = c.NextSibling {
				content.AppendChild(c.CloneNode(true))
			}
			clone.TemplateContentNode = content
		}
	}
	return clone
}

// TemplateContent returns the inert document fragment associated with a template.
func (n *Node) TemplateContent() *Node {
	if !n.IsHTMLTemplate() {
		return nil
	}
	if n.TemplateContentNode == nil {
		contentDoc := n.Doc
		if contentDoc != nil {
			contentDoc = contentDoc.TemplateContentsOwnerDocument()
		}
		frag := &Node{Type: NodeDocumentFragment, Doc: contentDoc, TemplateHost: n}
		if contentDoc != nil {
			frag.ID = contentDoc.allocID()
		}
		n.TemplateContentNode = frag
	}
	return n.TemplateContentNode
}

// TemplateContentsOwnerDocument returns the inert document shared by this
// document's template contents. It has no browsing context and is separate
// from the document that owns the <template> elements.
func (d *Document) TemplateContentsOwnerDocument() *Document {
	if d == nil {
		return nil
	}
	d.propertiesMu.Lock()
	defer d.propertiesMu.Unlock()
	if d.templateContentsDocument == nil {
		d.templateContentsDocument = NewDocument()
		d.templateContentsDocument.templateContentsOwner = true
	}
	return d.templateContentsDocument
}

// IsTemplateContentsOwnerDocument reports whether d is the inert owner
// document shared by HTML template contents.
func (d *Document) IsTemplateContentsOwnerDocument() bool {
	return d != nil && d.templateContentsOwner
}

// ImportNode creates a copy of a node from an external document and adopts it into d.
func (d *Document) ImportNode(n *Node, deep bool) *Node {
	if n == nil || d == nil {
		return nil
	}
	clone := n.CloneNode(deep)
	clone.adopt(d)
	return clone
}

// adopt assigns targetDoc and allocates new NodeIDs throughout the subtree
// rooted at n if n comes from another document or lacks a valid NodeID.
func (n *Node) adopt(targetDoc *Document) {
	if n == nil || targetDoc == nil {
		return
	}
	var walk func(curr *Node, owner *Document)
	walk = func(curr *Node, owner *Document) {
		if curr.Doc != owner || curr.ID == 0 {
			curr.Doc = owner
			curr.ID = owner.allocID()
		}
		for k := curr.FirstChild; k != nil; k = k.NextSibling {
			walk(k, owner)
		}
		if curr.ShadowRoot != nil {
			for k := curr.ShadowRoot.FirstChild; k != nil; k = k.NextSibling {
				walk(k, owner)
			}
		}
		if curr.TemplateContentNode != nil {
			walk(curr.TemplateContentNode, owner.TemplateContentsOwnerDocument())
		}
	}
	walk(n, targetDoc)
}

// AdoptInto changes the node's owner document and assigns target-document IDs
// throughout its ordinary, shadow, and template-content subtrees.
func (n *Node) AdoptInto(targetDoc *Document) { n.adopt(targetDoc) }

// AppendChild adds c as the last child of n.
func (n *Node) AppendChild(c *Node) {
	if c.Parent != nil {
		if root := c.Parent.ShadowRoot; root == nil || !root.RemoveChild(c) {
			c.Parent.RemoveChild(c)
		}
	}
	c.Parent = n
	oldDoc := c.Doc
	if n.Doc != nil && (oldDoc != n.Doc || c.ID == 0) {
		c.adopt(n.Doc)
	} else {
		c.Doc = n.Doc
	}
	if n.LastChild != nil {
		n.LastChild.NextSibling = c
		c.PrevSibling = n.LastChild
		n.LastChild = c
	} else {
		n.FirstChild = c
		n.LastChild = c
	}
	if n.Doc != nil {
		n.Doc.MarkQueryDirty()
	}
	if oldDoc != nil && oldDoc != n.Doc {
		oldDoc.MarkQueryDirty()
	}
	if n.Data == "textarea" && !n.InputValueDirty {
		n.SyncInputControlValueFromAttribute()
	}
	owner := NearestSelect(n)
	if n.Data == "select" {
		owner = n
	}
	normalizeSelectsAfterTreeMutation(c, owner)
}

// RemoveChild removes c from n's children.
func (n *Node) RemoveChild(c *Node) {
	if c.Parent != n {
		return
	}
	if n.Doc != nil {
		n.Doc.runBeforeNodeRemoval(n, c)
	}
	owner := NearestSelect(c)
	if c.Data == "select" {
		owner = c
	}
	if c.PrevSibling != nil {
		c.PrevSibling.NextSibling = c.NextSibling
	} else {
		n.FirstChild = c.NextSibling
	}
	if c.NextSibling != nil {
		c.NextSibling.PrevSibling = c.PrevSibling
	} else {
		n.LastChild = c.PrevSibling
	}
	c.Parent = nil
	c.PrevSibling = nil
	c.NextSibling = nil
	if n.Doc != nil {
		n.Doc.MarkQueryDirty()
	}
	if n.Data == "textarea" && !n.InputValueDirty {
		n.SyncInputControlValueFromAttribute()
	}
	normalizeSelectsAfterTreeMutation(nil, owner)
}

// InsertBefore inserts newChild before refChild among n's children.
func (n *Node) InsertBefore(newChild, refChild *Node) {
	if refChild == nil {
		n.AppendChild(newChild)
		return
	}
	// Inserting a node before itself is a no-op everywhere else in the tree,
	// and letting it through would splice the node's own pointers into a
	// cycle: PrevSibling/NextSibling would point back at the node and every
	// later walk (style, layout, serialization) would loop forever. Script
	// callers reach this with `el.insertBefore(el.firstChild, el.firstChild)`
	// and worse, so the guard belongs in the primitive, not only at the
	// binding.
	if newChild == refChild {
		return
	}
	if newChild.Parent != nil {
		if root := newChild.Parent.ShadowRoot; root == nil || !root.RemoveChild(newChild) {
			newChild.Parent.RemoveChild(newChild)
		}
	}
	newChild.Parent = n
	oldDoc := newChild.Doc
	if n.Doc != nil && (oldDoc != n.Doc || newChild.ID == 0) {
		newChild.adopt(n.Doc)
	} else {
		newChild.Doc = n.Doc
	}
	newChild.NextSibling = refChild
	newChild.PrevSibling = refChild.PrevSibling
	if refChild.PrevSibling != nil {
		refChild.PrevSibling.NextSibling = newChild
	} else {
		n.FirstChild = newChild
	}
	refChild.PrevSibling = newChild
	if n.Doc != nil {
		n.Doc.MarkQueryDirty()
	}
	if oldDoc != nil && oldDoc != n.Doc {
		oldDoc.MarkQueryDirty()
	}
	if n.Data == "textarea" && !n.InputValueDirty {
		n.SyncInputControlValueFromAttribute()
	}
	owner := NearestSelect(n)
	if n.Data == "select" {
		owner = n
	}
	normalizeSelectsAfterTreeMutation(newChild, owner)
}

// InsertNode runs the DOM Standard's "pre-insert" steps: insert newChild before
// refChild, where a DocumentFragment contributes its own children in order and
// is left empty afterwards, so the same fragment can be filled and inserted
// again. refChild nil appends, as InsertBefore already does.
//
// InsertBefore keeps its single-node behaviour on purpose: the HTML parser,
// layout and every internal caller already passes a real element, and making
// the primitive move fragment children would silently change the node count
// those paths reason about. Only the script-facing mutation APIs need the
// fragment rule, because only they can be handed a fragment by a page.
func (n *Node) InsertNode(newChild, refChild *Node) {
	if newChild == nil {
		return
	}
	if newChild.DocumentFragment() {
		// Snapshot the children first: each move unlinks the node from the
		// fragment, so walking NextSibling would skip every other child.
		var kids []*Node
		for c := newChild.FirstChild; c != nil; c = c.NextSibling {
			kids = append(kids, c)
		}
		for _, c := range kids {
			n.InsertBefore(c, refChild)
		}
		return
	}
	n.InsertBefore(newChild, refChild)
}

// ReplaceNode runs the DOM Standard's "replace one" steps: newNode goes where
// oldChild was and oldChild is detached. A DocumentFragment newNode inserts all
// of its children and removes oldChild once, matching browsers.
func (n *Node) ReplaceNode(newChild, oldChild *Node) {
	if newChild == nil || oldChild == nil {
		return
	}
	if newChild == oldChild {
		return
	}
	if newChild.DocumentFragment() {
		var kids []*Node
		for c := newChild.FirstChild; c != nil; c = c.NextSibling {
			kids = append(kids, c)
		}
		if len(kids) == 0 {
			n.RemoveChild(oldChild)
			return
		}
		ref := oldChild.NextSibling
		for _, c := range kids {
			n.InsertBefore(c, ref)
		}
		n.RemoveChild(oldChild)
		return
	}
	if oldChild.Parent != n {
		return
	}
	next := oldChild.NextSibling
	n.InsertBefore(newChild, next)
	n.RemoveChild(oldChild)
}

// ChildNodes returns n's children in document order as a snapshot slice. The
// binding hands pages a plain array rather than a live NodeList: see
// internal/js/node_api.go for the recorded consequence of that choice.
func (n *Node) ChildNodes() []*Node {
	var out []*Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, c)
	}
	return out
}

// Contains reports whether other is n itself or a descendant of it, which is
// the DOM Standard's Node.contains rule. jQuery's `:has` and `$.contains` both
// walk up Parent instead of down, so this only serves the API itself.
func (n *Node) Contains(other *Node) bool {
	if n == nil || other == nil {
		return false
	}
	seen := make(map[*Node]struct{})
	for o := other; o != nil; o = o.Parent {
		if o == n {
			return true
		}
		if _, exists := seen[o]; exists {
			return false
		}
		seen[o] = struct{}{}
	}
	return false
}

// IsConnected reports whether the node is attached to its document, the DOM
// Standard's isConnected. It stops at the document node so a node whose Parent
// chain dangles on a detached subtree reads false.
func (n *Node) IsConnected() bool {
	if n == nil {
		return false
	}
	seen := make(map[*Node]struct{})
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Type == NodeDocument {
			return true
		}
		if _, exists := seen[cur]; exists {
			return false
		}
		seen[cur] = struct{}{}
	}
	return false
}

// AppendChild adds c as the last child of the shadow root.
func (sr *ShadowRoot) AppendChild(c *Node) {
	if c.Parent != nil {
		c.Parent.RemoveChild(c)
	}
	c.Parent = sr.Host
	oldDoc := c.Doc
	if sr.Host != nil && sr.Host.Doc != nil && (oldDoc != sr.Host.Doc || c.ID == 0) {
		c.adopt(sr.Host.Doc)
	} else if sr.Host != nil {
		c.Doc = sr.Host.Doc
	}
	if sr.LastChild != nil {
		sr.LastChild.NextSibling = c
		c.PrevSibling = sr.LastChild
		sr.LastChild = c
	} else {
		sr.FirstChild = c
		sr.LastChild = c
	}
	if sr.Host != nil && sr.Host.Doc != nil {
		sr.Host.Doc.MarkQueryDirty()
	}
	if oldDoc != nil && (sr.Host == nil || oldDoc != sr.Host.Doc) {
		oldDoc.MarkQueryDirty()
	}
}

// RemoveChild detaches a direct child from sr. Shadow-root children store the
// host in Node.Parent for composed-tree traversal, so ordinary parent removal
// must first distinguish this child list from the host's light-DOM children.
func (sr *ShadowRoot) RemoveChild(c *Node) bool {
	if sr == nil || c == nil || sr.Host == nil || c.Parent != sr.Host {
		return false
	}
	found := false
	for child := sr.FirstChild; child != nil; child = child.NextSibling {
		if child == c {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	if sr.Host.Doc != nil {
		sr.Host.Doc.runBeforeNodeRemoval(sr.Host, c)
	}
	if c.PrevSibling != nil {
		c.PrevSibling.NextSibling = c.NextSibling
	} else {
		sr.FirstChild = c.NextSibling
	}
	if c.NextSibling != nil {
		c.NextSibling.PrevSibling = c.PrevSibling
	} else {
		sr.LastChild = c.PrevSibling
	}
	c.Parent, c.PrevSibling, c.NextSibling = nil, nil, nil
	if sr.Host.Doc != nil {
		sr.Host.Doc.MarkQueryDirty()
	}
	return true
}

// GetAttribute returns the value of the named attribute, or "" if not present.
// Attribute names are lowercased by the HTML parser, so the exact match below
// hits without allocating; the EqualFold fallback only serves mixed-case names
// installed via SetAttribute from script.
func (n *Node) GetAttribute(name string) string {
	for _, a := range n.Attr {
		if a.Name == name || strings.EqualFold(a.Name, name) {
			return a.Value
		}
	}
	return ""
}

// GetAttributeNS returns the value of the attribute matching namespace URI and
// qualified name. The bool distinguishes an absent attribute from an empty one.
func (n *Node) GetAttributeNS(namespaceURI, qualifiedName string) (string, bool) {
	for _, a := range n.Attr {
		if a.NamespaceURI == namespaceURI && strings.EqualFold(a.Name, qualifiedName) {
			return a.Value, true
		}
	}
	return "", false
}

// isQueryAttr reports whether an attribute write can retarget the id,
// class, or tag query caches.
func isQueryAttr(name string) bool {
	return name == "id" || name == "class" ||
		strings.EqualFold(name, "id") || strings.EqualFold(name, "class")
}

// SetAttribute sets the named attribute, replacing an existing value
// case-insensitively or appending a new pair.
func (n *Node) SetAttribute(name, value string) {
	typeChanged := n.Data == "input" && strings.EqualFold(name, "type")
	oldType, oldValue := "", ""
	if typeChanged {
		oldType = n.GetAttribute("type")
		oldValue = n.InputControlValue()
	}
	if strings.EqualFold(name, "value") {
		n.ValueChangedByUser = false
	}
	switch strings.ToLower(name) {
	case "value", "pattern", "type", "multiple", "min", "max", "step":
		n.PatternMismatch = false
		n.TypeMismatch = false
		n.RangeUnderflow = false
		n.RangeOverflow = false
		n.StepMismatch = false
		n.RangeLimited = false
	}
	for i := range n.Attr {
		if n.Attr[i].Namespace == NSHTML && n.Attr[i].NamespaceURI == "" &&
			(n.Attr[i].Name == name || strings.EqualFold(n.Attr[i].Name, name)) {
			attributeChanged := n.Attr[i].Value != value
			n.Attr[i].Value = value
			if strings.EqualFold(name, "value") {
				n.SyncInputControlValueFromAttribute()
			}
			if strings.EqualFold(name, "checked") {
				n.syncInputCheckednessFromAttribute()
			}
			if attributeChanged {
				n.selectAttributeChanged(name)
			}
			if typeChanged {
				n.inputTypeAttributeChanged(oldType, value, oldValue)
			}
			// Only id/class writes can retarget the query caches; anything
			// else leaves them valid.
			if n.Doc != nil && isQueryAttr(name) {
				n.Doc.MarkQueryDirty()
			}
			return
		}
	}
	n.Attr = append(n.Attr, Attribute{Name: name, Value: value})
	if strings.EqualFold(name, "value") {
		n.SyncInputControlValueFromAttribute()
	}
	if strings.EqualFold(name, "checked") {
		n.syncInputCheckednessFromAttribute()
	}
	if strings.EqualFold(name, "selected") {
		n.syncOptionSelectednessFromAttribute()
	}
	n.selectAttributeChanged(name)
	if typeChanged {
		n.inputTypeAttributeChanged(oldType, value, oldValue)
	}
	if n.Doc != nil && isQueryAttr(name) {
		n.Doc.MarkQueryDirty()
	}
}

// SetAttributeNS sets an attribute identified by namespace URI and local name.
// Qualified names from distinct namespaces may share the same spelling.
func (n *Node) SetAttributeNS(namespaceURI, qualifiedName, value string) bool {
	if namespaceURI == "" {
		before, had := "", false
		for _, a := range n.Attr {
			if a.Namespace == NSHTML && a.NamespaceURI == "" && strings.EqualFold(a.Name, qualifiedName) {
				before, had = a.Value, true
				break
			}
		}
		n.SetAttribute(qualifiedName, value)
		return !had || before != value
	}
	localName := qualifiedName
	if colon := strings.IndexByte(localName, ':'); colon >= 0 {
		localName = localName[colon+1:]
	}
	namespace := NSOther
	if namespaceURI == "http://www.w3.org/XML/1998/namespace" {
		namespace = NSXML
	}
	for i := range n.Attr {
		a := &n.Attr[i]
		if a.NamespaceURI == namespaceURI && attributeLocalName(a.Name) == localName {
			changed := a.Name != qualifiedName || a.Value != value
			a.Namespace = namespace
			a.Name = qualifiedName
			a.NamespaceURI = namespaceURI
			a.Value = value
			if namespaceURI == "" && strings.EqualFold(localName, "value") {
				n.SyncInputControlValueFromAttribute()
			}
			if changed && n.Doc != nil && namespace == NSHTML && isQueryAttr(localName) {
				n.Doc.MarkQueryDirty()
			}
			return changed
		}
	}
	n.Attr = append(n.Attr, Attribute{Namespace: namespace, NamespaceURI: namespaceURI, Name: qualifiedName, Value: value})
	if namespaceURI == "" && strings.EqualFold(localName, "value") {
		n.SyncInputControlValueFromAttribute()
	}
	if n.Doc != nil && namespace == NSHTML && isQueryAttr(localName) {
		n.Doc.MarkQueryDirty()
	}
	return true
}

func attributeLocalName(qualifiedName string) string {
	if colon := strings.IndexByte(qualifiedName, ':'); colon >= 0 {
		return qualifiedName[colon+1:]
	}
	return qualifiedName
}

// RemoveAttributeNS removes the attribute with the given namespace URI and
// local name, independent of its prefix.
func (n *Node) RemoveAttributeNS(namespaceURI, localName string) bool {
	for i := range n.Attr {
		if n.Attr[i].NamespaceURI == namespaceURI && attributeLocalName(n.Attr[i].Name) == localName {
			typeChanged := n.Data == "input" && namespaceURI == "" && strings.EqualFold(localName, "type")
			oldType, oldValue := "", ""
			if typeChanged {
				oldType = n.GetAttribute("type")
				oldValue = n.InputControlValue()
			}
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			if namespaceURI == "" && strings.EqualFold(localName, "checked") {
				n.syncInputCheckednessFromAttribute()
			}
			if typeChanged {
				n.inputTypeAttributeChanged(oldType, "", oldValue)
			}
			if namespaceURI == "" && strings.EqualFold(localName, "value") {
				n.SyncInputControlValueFromAttribute()
			}
			if n.Doc != nil && namespaceURI == "" && isQueryAttr(localName) {
				n.Doc.MarkQueryDirty()
			}
			return true
		}
	}
	return false
}

// HasAttribute reports whether the named attribute is present.
func (n *Node) HasAttribute(name string) bool {
	for _, a := range n.Attr {
		if a.Name == name || strings.EqualFold(a.Name, name) {
			return true
		}
	}
	return false
}

// RemoveAttribute removes the named attribute from the element.
func (n *Node) RemoveAttribute(name string) {
	typeChanged := n.Data == "input" && strings.EqualFold(name, "type")
	oldType, oldValue := "", ""
	if typeChanged {
		oldType = n.GetAttribute("type")
		oldValue = n.InputControlValue()
	}
	if strings.EqualFold(name, "value") {
		n.ValueChangedByUser = false
	}
	switch strings.ToLower(name) {
	case "value", "pattern", "type", "multiple", "min", "max", "step":
		n.PatternMismatch = false
		n.TypeMismatch = false
		n.RangeUnderflow = false
		n.RangeOverflow = false
		n.StepMismatch = false
		n.RangeLimited = false
	}
	for i := range n.Attr {
		if n.Attr[i].Name == name || strings.EqualFold(n.Attr[i].Name, name) {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			if strings.EqualFold(name, "checked") {
				n.syncInputCheckednessFromAttribute()
			}
			if strings.EqualFold(name, "selected") {
				n.syncOptionSelectednessFromAttribute()
			}
			n.selectAttributeChanged(name)
			if typeChanged {
				n.inputTypeAttributeChanged(oldType, "", oldValue)
			}
			if strings.EqualFold(name, "value") {
				n.SyncInputControlValueFromAttribute()
			}
			if n.Doc != nil && isQueryAttr(name) {
				n.Doc.MarkQueryDirty()
			}
			return
		}
	}
}

// IsDraggable reports the effective draggable state of an element: an explicit
// draggable="true"/"false" attribute always wins; without one, images and
// links with an href are draggable by default, matching the HTML spec's
// "auto" default for those elements. Non-elements are never draggable.
func IsDraggable(n *Node) bool {
	if n == nil || !n.Element() {
		return false
	}
	switch strings.ToLower(n.GetAttribute("draggable")) {
	case "true":
		return true
	case "false":
		return false
	}
	switch n.Data {
	case "img":
		return true
	case "a":
		return n.GetAttribute("href") != ""
	}
	return false
}

// ClassList returns the space-separated class names on the element.
func (n *Node) ClassList() []string {
	v := n.GetAttribute("class")
	if v == "" {
		return nil
	}
	return strings.Fields(v)
}

// ChildElements returns only the element children.
func (n *Node) ChildElements() []*Node {
	var els []*Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() {
			els = append(els, c)
		}
	}
	return els
}

// TextContent returns the concatenation of all descendant text nodes.
func (n *Node) TextContent() string {
	var sb strings.Builder
	n.collectText(&sb)
	return sb.String()
}

func (n *Node) collectText(sb *strings.Builder) {
	if n.Type == NodeText || n.Type == NodeCDATASection {
		sb.WriteString(n.DataContent)
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		c.collectText(sb)
	}
}

// Walk traverses the subtree rooted at n in pre-order depth-first order.
// If fn returns false, the walk stops early.
func (n *Node) Walk(fn func(*Node) bool) bool {
	if n == nil {
		return true
	}
	if !fn(n) {
		return false
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if !c.Walk(fn) {
			return false
		}
	}
	return true
}

// Document is the root of a DOM tree. It owns the node-ID allocator and
// provides the entry points for tree queries.
type Document struct {
	Node                     Node
	nextID                   NodeID
	removalCallbacksMu       sync.RWMutex
	beforeNodeRemoval        map[uint64]func(parent, child *Node)
	nextRemovalCallbackID    uint64
	templateContentsDocument *Document
	templateContentsOwner    bool
	// ContentLanguage is the first HTTP Content-Language value supplied by the
	// embedding document loader, used only when no element declares a language.
	ContentLanguage string
	CharacterSet    string
	ContentType     string
	// ParserInput retains admitted XML source for browsing contexts that bind
	// their runtime before parsing parser-inserted scripts and custom elements.
	ParserInput       string
	HTML              *Node
	Head              *Node
	Body              *Node
	DocType           *Node
	QuirksMode        bool
	LimitedQuirksMode bool
	DesignMode        bool
	// IsXML is true for documents parsed as XML (text/xml, application/xml,
	// application/xhtml+xml, image/svg+xml). When true, element creation
	// preserves tag name case and tagName returns the original case rather
	// than the HTML-spec uppercase form.
	IsXML bool
	// IsDOMParserDocument marks documents returned by DOMParser. They remain
	// XML documents for parsing and serialization, but expose the Document
	// interface rather than the legacy XMLDocument interface.
	IsDOMParserDocument bool
	// idIdx caches id→first-element for ElementByID, tagIdx caches
	// atom→elements in document order, tagStrIdx caches unknown-atom tags
	// by literal Data, and classIdx caches class→elements. All are rebuilt
	// together lazily on query after any structural, id, or class mutation
	// (MarkQueryDirty). Linear scans per query are O(N); scripts polling
	// these APIs turn them into O(queries×N), seconds on large documents.
	idIdx        map[string]*Node
	tagIdx       map[Atom][]*Node
	tagStrIdx    map[string][]*Node
	classIdx     map[string][]*Node
	qValid       bool
	propertiesMu sync.RWMutex
	properties   map[string]PropertyDefinition
}

// RegisterBeforeNodeRemovalCallback registers a callback run immediately
// before a child is detached from its parent. It returns a function that
// unregisters the callback. The document snapshots callbacks before invoking
// them so a callback can safely remove itself or register another callback.
func (d *Document) RegisterBeforeNodeRemovalCallback(callback func(parent, child *Node)) func() {
	if d == nil || callback == nil {
		return func() {}
	}
	d.removalCallbacksMu.Lock()
	if d.beforeNodeRemoval == nil {
		d.beforeNodeRemoval = make(map[uint64]func(parent, child *Node))
	}
	d.nextRemovalCallbackID++
	id := d.nextRemovalCallbackID
	d.beforeNodeRemoval[id] = callback
	d.removalCallbacksMu.Unlock()
	return func() {
		d.removalCallbacksMu.Lock()
		delete(d.beforeNodeRemoval, id)
		d.removalCallbacksMu.Unlock()
	}
}

func (d *Document) runBeforeNodeRemoval(parent, child *Node) {
	if d == nil {
		return
	}
	d.removalCallbacksMu.RLock()
	callbacks := make([]func(parent, child *Node), 0, len(d.beforeNodeRemoval))
	for _, callback := range d.beforeNodeRemoval {
		callbacks = append(callbacks, callback)
	}
	d.removalCallbacksMu.RUnlock()
	for _, callback := range callbacks {
		callback(parent, child)
	}
}

// MarkQueryDirty drops the id/tag/class caches after a structural edit
// (append, remove, insert) or an id/class-attribute write. Anything else
// leaves the caches valid. Exported because the JS runtime performs DOM
// writes through direct slice edits alongside the Node methods.
func (d *Document) MarkQueryDirty() {
	d.qValid = false
}

// rebuildQueryIndex walks the tree once in document order, filling every
// query cache with exactly what the linear scans returned: first element
// per id, document-order element lists per tag atom (unknown atoms by
// literal Data), and per class (once per node even with repeats).
func (d *Document) rebuildQueryIndex() {
	idm := make(map[string]*Node)
	tm := make(map[Atom][]*Node)
	sm := make(map[string][]*Node)
	cm := make(map[string][]*Node)
	var walk func(n *Node)
	walk = func(n *Node) {
		if n.Element() {
			if v := n.GetAttribute("id"); v != "" {
				if _, ok := idm[v]; !ok {
					idm[v] = n
				}
			}
			if n.DataAtom != AtomUnknown {
				tm[n.DataAtom] = append(tm[n.DataAtom], n)
			} else {
				sm[n.Data] = append(sm[n.Data], n)
			}
			if cls := n.GetAttribute("class"); cls != "" {
				var prev []string
				for _, c := range strings.Fields(cls) {
					dup := false
					for _, p := range prev {
						if p == c {
							dup = true
							break
						}
					}
					if !dup {
						prev = append(prev, c)
						cm[c] = append(cm[c], n)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&d.Node)
	d.idIdx = idm
	d.tagIdx = tm
	d.tagStrIdx = sm
	d.classIdx = cm
	d.qValid = true
}

// queryIntact reports whether the query caches are trustworthy for d: Doc
// back-pointers must be intact (see ElementByID).
func (d *Document) queryIntact() bool {
	return d.Node.Doc == d
}

// ensureQueryIndex rebuilds the caches when stale.
func (d *Document) ensureQueryIndex() {
	if !d.qValid {
		d.rebuildQueryIndex()
	}
}

// NewDocument creates an empty document with a document node.
func NewDocument() *Document {
	doc := &Document{CharacterSet: "UTF-8"}
	doc.Node = Node{
		ID:   doc.allocID(),
		Type: NodeDocument,
		Doc:  doc,
	}
	return doc
}

func (d *Document) allocID() NodeID {
	id := d.nextID
	d.nextID++
	return id
}

// AllocateNodeID reserves a unique ID in this document for a node assembled
// by a caller that needs to transfer an existing tree into a new document.
func (d *Document) AllocateNodeID() NodeID {
	return d.allocID()
}

// NewElement creates a new element node with the given tag name.
// For HTML documents the name is lowercased per the HTML spec; for XML
// documents the original case is preserved.
func (d *Document) NewElement(tagName string) *Node {
	name := tagName
	if !d.IsXML {
		name = strings.ToLower(tagName)
	}
	n := &Node{
		ID:       d.allocID(),
		Type:     NodeElement,
		Data:     name,
		DataAtom: Lookup(name),
		Doc:      d,
	}
	return n
}

// NewText creates a new text node.
func (d *Document) NewText(data string) *Node {
	return &Node{
		ID:          d.allocID(),
		Type:        NodeText,
		DataContent: data,
		Doc:         d,
	}
}

// NewCDATASection creates a CDATA section node for XML documents.
func (d *Document) NewCDATASection(data string) *Node {
	return &Node{ID: d.allocID(), Type: NodeCDATASection, DataContent: data, Doc: d}
}

// NewProcessingInstruction creates a ProcessingInstruction node. The target
// is stored in Data and the instruction data in DataContent.
func (d *Document) NewProcessingInstruction(target, data string) *Node {
	return &Node{ID: d.allocID(), Type: NodeProcessingInstruction, Data: target, DataContent: data, Doc: d}
}

// NewDocumentFragment creates an empty document fragment owned by d. Scripts
// reach it through document.createDocumentFragment().
func (d *Document) NewDocumentFragment() *Node {
	return &Node{
		ID:   d.allocID(),
		Type: NodeDocumentFragment,
		Doc:  d,
	}
}

// NewComment creates a new comment node.
func (d *Document) NewComment(data string) *Node {
	return &Node{
		ID:          d.allocID(),
		Type:        NodeComment,
		DataContent: data,
		Doc:         d,
	}
}

// NewDoctype creates a new doctype node.
func (d *Document) NewDoctype(data string) *Node {
	return &Node{
		ID:          d.allocID(),
		Type:        NodeDoctype,
		DataContent: data,
		Doc:         d,
	}
}

// ElementsByClassName returns all elements that have the given class name
// among their space-separated class list.
func (d *Document) ElementsByClassName(class string) []*Node {
	if !d.queryIntact() {
		var result []*Node
		collectByClass(&d.Node, class, &result)
		return result
	}
	d.ensureQueryIndex()
	src := d.classIdx[class]
	if len(src) == 0 {
		return nil
	}
	out := make([]*Node, len(src))
	copy(out, src)
	return out
}

func collectByClass(n *Node, class string, result *[]*Node) {
	if n.Element() {
		for _, c := range n.ClassList() {
			if c == class {
				*result = append(*result, n)
				break
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectByClass(c, class, result)
	}
}

// ElementByID returns the first element with the given ID, or nil.
func (d *Document) ElementByID(id string) *Node {
	// An empty id matched the first element under the old linear scan
	// (missing attributes read as ""); keep that path verbatim rather than
	// redefining it here.
	if id == "" {
		return findByID(&d.Node, id)
	}
	// The index is only trustworthy when Doc back-pointers are intact: a
	// zero-value Document (&Document{}, as hand-built trees use) has nil
	// Doc everywhere, so dirty marks from mutations can never reach this
	// struct and a cached index would go stale. Parsed documents always
	// carry intact pointers (NewDocument + AppendChild propagation), so the
	// real browser path stays indexed while degenerate trees keep the
	// always-correct linear scan.
	if !d.queryIntact() {
		return findByID(&d.Node, id)
	}
	d.ensureQueryIndex()
	return d.idIdx[id]
}

func findByID(n *Node, id string) *Node {
	if n.Element() && n.GetAttribute("id") == id {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

// DatalistOptions returns the suggested values associated with an input element
// via its list attribute pointing to a <datalist id="..."> element.
func (d *Document) DatalistOptions(input *Node) []string {
	if input == nil || input.Doc == nil {
		return nil
	}
	listID := input.GetAttribute("list")
	if listID == "" {
		return nil
	}
	dl := input.Doc.ElementByID(listID)
	if dl == nil || dl.Data != "datalist" {
		return nil
	}
	var options []string
	dl.Walk(func(n *Node) bool {
		if n.Element() && n.Data == "option" {
			if datalistOptionDisabled(n, dl) {
				return true
			}
			val := n.GetAttribute("value")
			if !n.HasAttribute("value") {
				val = strings.TrimSpace(n.TextContent())
			}
			if val != "" {
				options = append(options, val)
			}
		}
		return true
	})
	return options
}

func datalistOptionDisabled(option, datalist *Node) bool {
	if option == nil || option.HasAttribute("disabled") {
		return true
	}
	for parent := option.Parent; parent != nil && parent != datalist; parent = parent.Parent {
		if parent.Data == "optgroup" && parent.HasAttribute("disabled") {
			return true
		}
	}
	return false
}

// DatalistOptions returns the list options if n is an input with a valid list attribute.
func (n *Node) DatalistOptions() []string {
	if n == nil || n.Doc == nil {
		return nil
	}
	return n.Doc.DatalistOptions(n)
}

// ElementsByTagName returns all elements with the given tag name.
func (d *Document) ElementsByTagName(tag string) []*Node {
	var result []*Node
	collectByTag(&d.Node, tag, &result)
	return result
}

func collectByTag(n *Node, tag string, result *[]*Node) {
	if n.Element() {
		if matchesTagName(n, tag, n.Doc != nil && n.Doc.IsXML) {
			*result = append(*result, n)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectByTag(c, tag, result)
	}
}

func matchesTagName(node *Node, tag string, xmlDocument bool) bool {
	if node == nil || !node.Element() {
		return false
	}
	if tag == "*" {
		return true
	}
	if xmlDocument || node.Namespace != NSHTML {
		return node.Data == tag
	}
	return strings.EqualFold(node.Data, tag)
}

// PropertyDefinition describes a CSS custom property registered via @property or CSS.registerProperty.
type PropertyDefinition struct {
	Name         string
	Syntax       string
	Inherits     bool
	InitialValue string
}

// RegisterProperty registers a CSS custom property descriptor on the document.
// It returns false if the property was already registered.
func (d *Document) RegisterProperty(def PropertyDefinition) bool {
	d.propertiesMu.Lock()
	defer d.propertiesMu.Unlock()
	if d.properties == nil {
		d.properties = make(map[string]PropertyDefinition)
	}
	if _, exists := d.properties[def.Name]; exists {
		return false
	}
	d.properties[def.Name] = def
	return true
}

// GetProperty retrieves a registered CSS custom property descriptor if present.
func (d *Document) GetProperty(name string) (PropertyDefinition, bool) {
	d.propertiesMu.RLock()
	defer d.propertiesMu.RUnlock()
	if d.properties == nil {
		return PropertyDefinition{}, false
	}
	def, ok := d.properties[name]
	return def, ok
}

// RegisteredProperties returns a snapshot map of all registered custom properties.
func (d *Document) RegisteredProperties() map[string]PropertyDefinition {
	d.propertiesMu.RLock()
	defer d.propertiesMu.RUnlock()
	if len(d.properties) == 0 {
		return nil
	}
	out := make(map[string]PropertyDefinition, len(d.properties))
	for k, v := range d.properties {
		out[k] = v
	}
	return out
}
