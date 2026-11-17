package dom

import (
	"strconv"
	"strings"
	"sync"
)

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

type NodeID uint32

type Namespace uint8

const (
	NSHTML Namespace = iota
	NSSVG
	NSMathML
	NSOther
	NSXML
)

type Attribute struct {
	Namespace    Namespace
	NamespaceURI string
	Name         string
	NameAtom     AttrAtom
	Value        string
}

type Node struct {
	ID           NodeID
	Type         NodeType
	Namespace    Namespace
	NamespaceURI string
	Prefix       string

	Data     string
	DataAtom Atom
	Attr     []Attribute

	DataContent string
	DataUTF16   []uint16
	DoctypeName string
	PublicID    string
	SystemID    string

	Parent      *Node
	FirstChild  *Node
	LastChild   *Node
	NextSibling *Node
	PrevSibling *Node

	ShadowRoot           *ShadowRoot
	TemplateContentNode  *Node
	TemplateHost         *Node
	FormOwner            *Node
	SVGScriptReady       bool
	ScriptAlreadyStarted bool

	IsModal     bool
	ReturnValue string

	IsPopoverOpen bool

	IsScopeRoot bool

	FormAssociatedCustomElement   bool
	CustomElementValid            bool
	CustomElementUndefined        bool
	CustomStates                  map[string]bool
	ValueChangedByUser            bool
	InputValue                    string
	InputValueType                string
	InputValueMultiple            bool
	InputValueInitialized         bool
	InputValueDirty               bool
	InputUserEditValue            string
	InputUserEditValueActive      bool
	InputBadInput                 bool
	InputCheckedness              bool
	InputCheckednessInitialized   bool
	InputCheckednessDirty         bool
	InputIndeterminate            bool
	OptionSelectedness            bool
	OptionSelectednessInitialized bool
	OptionSelectednessDirty       bool
	SelectNoSelection             bool
	OutputDefaultValueOverride    string
	OutputDefaultValueOverrideSet bool
	CustomValidityMessage         string
	UserInteracted                bool
	InputFileCount                int
	PatternMismatch               bool
	TypeMismatch                  bool
	RangeUnderflow                bool
	RangeOverflow                 bool
	StepMismatch                  bool
	RangeLimited                  bool

	Doc *Document
}

type ShadowRoot struct {
	Host                                 *Node
	FirstChild                           *Node
	LastChild                            *Node
	Mode                                 string
	Declarative                          bool
	DelegatesFocus                       bool
	SlotAssignment                       string
	Clonable                             bool
	CustomElementRegistryNull            bool
	CustomElementRegistryNullDeclarative bool
	AdoptedStyleSheetTexts               []string
	ManualSlots                          map[*Node][]*Node
}

func (n *Node) Element() bool { return n.Type == NodeElement }

func InputValueMode(typ string) bool {
	switch strings.ToLower(typ) {
	case "hidden", "checkbox", "radio", "file", "submit", "image", "reset", "button":
		return false
	default:
		return true
	}
}

func (n *Node) inputTypeAttributeChanged(oldType, newType, oldValue string) {
	if n == nil || n.Data != "input" {
		return
	}
	wasValueMode := InputValueMode(oldType)
	isValueMode := InputValueMode(newType)
	switch {
	case wasValueMode && !isValueMode && !strings.EqualFold(newType, "file") && oldValue != "":
		n.SetAttribute("value", oldValue)
	case !wasValueMode && isValueMode:
		n.InputValueDirty = false
		n.SyncInputControlValueFromAttribute()
	case !strings.EqualFold(oldType, "file") && strings.EqualFold(newType, "file"):
		n.InputValue = ""
	}
}

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

func IsFormControlDisabled(node *Node) bool {
	return node != nil && (node.HasAttribute("disabled") || IsDisabledByFieldset(node))
}

func IsDisableableFormControl(node *Node) bool {
	if node == nil || !node.Element() || node.Namespace != NSHTML {
		return false
	}
	if node.FormAssociatedCustomElement {
		return true
	}
	switch node.Data {
	case "button", "input", "select", "textarea":
		return true
	}
	return false
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

func (n *Node) SelectedOption() *Node {
	if n == nil || n.Data != "select" {
		return nil
	}
	NormalizeSelectSelectedness(n)
	for _, option := range SelectOptions(n) {
		if option.OptionSelected() {
			return option
		}
	}
	return nil
}

func (n *Node) OptionLabel() string {
	if n == nil {
		return ""
	}
	if label := n.GetAttribute("label"); label != "" {
		return NormalizeOptionText(label)
	}
	return NormalizeOptionText(n.TextContent())
}

func NormalizeOptionText(text string) string {
	var result strings.Builder
	result.Grow(len(text))
	pendingSpace := false
	for i := 0; i < len(text); i++ {
		if isOptionSpace(text[i]) {
			pendingSpace = true
			continue
		}
		if pendingSpace && result.Len() > 0 {
			result.WriteByte(' ')
		}
		pendingSpace = false
		result.WriteByte(text[i])
	}
	return result.String()
}

func isOptionSpace(b byte) bool {
	switch b {
	case '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

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
	return NormalizeOptionText(option.TextContent())
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

func SelectIsPopupStyle(selectNode *Node) bool {
	if selectNode == nil || selectNode.Data != "select" {
		return false
	}
	return !selectNode.HasAttribute("multiple") && selectDisplaySize(selectNode) == 1
}

func SelectAnnouncesListbox(selectNode *Node) bool {
	if selectNode == nil || selectNode.Data != "select" {
		return false
	}
	return selectDisplaySize(selectNode) > 1
}

func (n *Node) OptionAccessibleName() string {
	if n == nil {
		return ""
	}
	if label := n.GetAttribute("label"); label != "" {
		return collapseOptionNameWhitespace(label)
	}
	return NormalizeOptionText(n.TextContent())
}

func (n *Node) OptionDisplayStyle() string {
	if n == nil {
		return ""
	}
	if label := n.GetAttribute("label"); label != "" {
		return trimOptionDisplayEdges(label)
	}
	return trimOptionDisplayEdges(NormalizeOptionText(n.TextContent()))
}

func collapseOptionNameWhitespace(text string) string {
	lead := len(text) > 0 && isOptionSpace(text[0])
	trail := len(text) > 0 && isOptionSpace(text[len(text)-1])
	core := text
	if lead {
		for len(core) > 0 && isOptionSpace(core[0]) {
			core = core[1:]
		}
	}
	if trail {
		for len(core) > 0 && isOptionSpace(core[len(core)-1]) {
			core = core[:len(core)-1]
		}
	}
	var out strings.Builder
	out.Grow(len(text))
	if lead {
		out.WriteByte(' ')
	}
	pending := false
	for i := 0; i < len(core); i++ {
		if isOptionSpace(core[i]) {
			pending = true
			continue
		}
		if pending {
			out.WriteByte(' ')
			pending = false
		}
		out.WriteByte(core[i])
	}
	if trail {
		out.WriteByte(' ')
	}
	return out.String()
}

func trimOptionDisplayEdges(text string) string {
	isEdge := func(b byte) bool { return isOptionSpace(b) || b == '\v' }
	start, end := 0, len(text)
	for start < end && isEdge(text[start]) {
		start++
	}
	for end > start && isEdge(text[end-1]) {
		end--
	}
	return text[start:end]
}

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

func (n *Node) ResetInputControlValue() {
	if n == nil || (n.Data != "input" && n.Data != "textarea") {
		return
	}
	n.InputValueDirty = false
	n.ValueChangedByUser = false
	n.SyncInputControlValueFromAttribute()
}

func (n *Node) OutputDefaultValue() string {
	if n == nil || n.Data != "output" {
		return ""
	}
	if n.OutputDefaultValueOverrideSet {
		return n.OutputDefaultValueOverride
	}
	return n.TextContent()
}

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

func (n *Node) Text() bool { return n.Type == NodeText }

func (n *Node) DocumentFragment() bool { return n.Type == NodeDocumentFragment }

func (n *Node) IsHTMLTemplate() bool {
	return n != nil && n.Type == NodeElement && n.Namespace == NSHTML && n.Data == "template"
}

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

func (d *Document) IsTemplateContentsOwnerDocument() bool {
	return d != nil && d.templateContentsOwner
}

func (d *Document) ImportNode(n *Node, deep bool) *Node {
	if n == nil || d == nil {
		return nil
	}
	clone := n.CloneNode(deep)
	clone.adopt(d)
	return clone
}

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

func (n *Node) AdoptInto(targetDoc *Document) { n.adopt(targetDoc) }

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

func (n *Node) InsertBefore(newChild, refChild *Node) {
	if refChild == nil {
		n.AppendChild(newChild)
		return
	}
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

func (n *Node) InsertNode(newChild, refChild *Node) {
	if newChild == nil {
		return
	}
	if newChild.DocumentFragment() {
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

func (n *Node) ChildNodes() []*Node {
	var out []*Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, c)
	}
	return out
}

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

func (n *Node) GetAttribute(name string) string {
	for _, a := range n.Attr {
		if a.Name == name || strings.EqualFold(a.Name, name) {
			return a.Value
		}
	}
	return ""
}

func (n *Node) GetAttributeNS(namespaceURI, qualifiedName string) (string, bool) {
	for _, a := range n.Attr {
		if a.NamespaceURI == namespaceURI && strings.EqualFold(a.Name, qualifiedName) {
			return a.Value, true
		}
	}
	return "", false
}

func isQueryAttr(name string) bool {
	return name == "id" || name == "class" ||
		strings.EqualFold(name, "id") || strings.EqualFold(name, "class")
}

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

func (n *Node) HasAttribute(name string) bool {
	for _, a := range n.Attr {
		if a.Name == name || strings.EqualFold(a.Name, name) {
			return true
		}
	}
	return false
}

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

func (n *Node) ClassList() []string {
	v := n.GetAttribute("class")
	if v == "" {
		return nil
	}
	return strings.Fields(v)
}

func (n *Node) ChildElements() []*Node {
	var els []*Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Element() {
			els = append(els, c)
		}
	}
	return els
}

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

type Document struct {
	Node                     Node
	nextID                   NodeID
	removalCallbacksMu       sync.RWMutex
	beforeNodeRemoval        map[uint64]func(parent, child *Node)
	nextRemovalCallbackID    uint64
	templateContentsDocument *Document
	templateContentsOwner    bool
	ContentLanguage          string
	CharacterSet             string
	ContentType              string
	ParserInput              string
	HTML                     *Node
	Head                     *Node
	Body                     *Node
	DocType                  *Node
	QuirksMode               bool
	LimitedQuirksMode        bool
	DesignMode               bool
	IsXML                    bool
	IsDOMParserDocument      bool
	idIdx                    map[string]*Node
	tagIdx                   map[Atom][]*Node
	tagStrIdx                map[string][]*Node
	classIdx                 map[string][]*Node
	qValid                   bool
	propertiesMu             sync.RWMutex
	properties               map[string]PropertyDefinition
	pointerCaptureMu         sync.Mutex
	pointerPressActive       bool
	pendingPointerCapture    *Node
	activePointerCapture     *Node
	releasedPointerCapture   *Node
}

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

func (d *Document) MarkQueryDirty() {
	d.qValid = false
}

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

func (d *Document) queryIntact() bool {
	return d.Node.Doc == d
}

func (d *Document) ensureQueryIndex() {
	if !d.qValid {
		d.rebuildQueryIndex()
	}
}

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

func (d *Document) AllocateNodeID() NodeID {
	return d.allocID()
}

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

func (d *Document) NewText(data string) *Node {
	return &Node{
		ID:          d.allocID(),
		Type:        NodeText,
		DataContent: data,
		Doc:         d,
	}
}

func (d *Document) NewCDATASection(data string) *Node {
	return &Node{ID: d.allocID(), Type: NodeCDATASection, DataContent: data, Doc: d}
}

func (d *Document) NewProcessingInstruction(target, data string) *Node {
	return &Node{ID: d.allocID(), Type: NodeProcessingInstruction, Data: target, DataContent: data, Doc: d}
}

func (d *Document) NewDocumentFragment() *Node {
	return &Node{
		ID:   d.allocID(),
		Type: NodeDocumentFragment,
		Doc:  d,
	}
}

func (d *Document) NewComment(data string) *Node {
	return &Node{
		ID:          d.allocID(),
		Type:        NodeComment,
		DataContent: data,
		Doc:         d,
	}
}

func (d *Document) NewDoctype(data string) *Node {
	return &Node{
		ID:          d.allocID(),
		Type:        NodeDoctype,
		DataContent: data,
		Doc:         d,
	}
}

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

func (d *Document) ElementByID(id string) *Node {
	if id == "" {
		return findByID(&d.Node, id)
	}
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

func (n *Node) DatalistOptions() []string {
	if n == nil || n.Doc == nil {
		return nil
	}
	return n.Doc.DatalistOptions(n)
}

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

type PropertyDefinition struct {
	Name         string
	Syntax       string
	Inherits     bool
	InitialValue string
}

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

func (d *Document) GetProperty(name string) (PropertyDefinition, bool) {
	d.propertiesMu.RLock()
	defer d.propertiesMu.RUnlock()
	if d.properties == nil {
		return PropertyDefinition{}, false
	}
	def, ok := d.properties[name]
	return def, ok
}

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
