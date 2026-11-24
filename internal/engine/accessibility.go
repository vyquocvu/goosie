package engine

import (
	"sort"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func (s *Session) AccessibilityTree() []ax.Node {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.accessibilityTreeLocked()
}

func (s *Session) accessibilityTreeLocked() []ax.Node {
	if s.Arena == nil || len(s.Arena.Objects) < 2 {
		return nil
	}
	b := &axBuilder{session: s, objs: s.Arena.Objects, focused: s.focus}
	root, bubble := b.visit(1)
	if root == nil {
		if bubble.text == "" {
			return nil
		}
		root = &ax.Node{Role: ax.RoleStaticText, Label: bubble.text,
			X0: bubble.x0, Y0: bubble.y0, X1: bubble.x1, Y1: bubble.y1}
	}
	doc := ax.Node{
		Role: ax.RoleDocument, RoleName: ax.RoleDocument.String(),
		ID: "goosie.document", DOMNodeID: uint64(s.Doc.Node.ID),
		X0: root.X0, Y0: root.Y0, X1: root.X1, Y1: root.Y1,
	}
	if root.Role == ax.RoleStaticText {
		doc.Children = append(doc.Children, *root)
	} else {
		doc.Children = append(doc.Children, root.Children...)
	}
	s.appendAccessibilityNodesWithoutLayout(&doc)
	s.appendValidationBubbleAX(&doc)
	return []ax.Node{doc}
}

func (s *Session) appendValidationBubbleAX(doc *ax.Node) {
	if s.validationAnchor == nil || s.validationMessage == "" {
		return
	}
	node := ax.Node{
		Role: ax.RoleStaticText, RoleName: ax.RoleStaticText.String(),
		ID: "goosie.validation-message", DOMNodeID: uint64(s.validationAnchor.ID),
		Label: s.validationMessage,
	}
	if obj := s.objectFor(s.validationAnchor); obj != nil {
		x0, y0, x1, y1 := obj.BorderRect()
		node.X0, node.Y0, node.X1, node.Y1 = x0, y0, x1, y1
	}
	doc.Children = append(doc.Children, node)
}

func (s *Session) appendAccessibilityNodesWithoutLayout(root *ax.Node) {
	if s == nil || root == nil || s.Doc == nil || s.jsRT == nil {
		return
	}
	known := make(map[uint64]bool)
	var index func([]ax.Node)
	index = func(nodes []ax.Node) {
		for i := range nodes {
			node := &nodes[i]
			if node.DOMNodeID != 0 {
				known[node.DOMNodeID] = true
			}
			index(node.Children)
		}
	}
	index([]ax.Node{*root})
	order := make(map[dom.NodeID]int)
	ordinal := 0
	var walk func(*dom.Node)
	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		order[node.ID] = ordinal
		ordinal++
		if node.ShadowRoot != nil {
			for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(&s.Doc.Node)

	walk = func(node *dom.Node) {
		if node == nil {
			return
		}
		if node.Element() && !known[uint64(node.ID)] && node.GetAttribute("aria-hidden") != "true" {
			roleValue := s.jsRT.ElementInternalsARIA(node, "role")
			if strings.TrimSpace(roleValue) == "" {
				roleValue = node.GetAttribute("role")
			}
			role, standardName, hasRole := elementInternalsAXRole(roleValue)
			name, hasName := s.jsRT.ElementInternalsAccessibleName(node)
			if hasRole || hasName {
				computed := s.Styles[node.ID]
				if computed == nil || computed.Display != style.DisplayNone && computed.Visibility == "visible" {
					if !hasRole {
						role, standardName = ax.RoleGroup, "generic"
					}
					item := ax.Node{
						Role: role, RoleName: standardName, DOMNodeID: uint64(node.ID),
						ID: node.GetAttribute("id"), Label: name,
					}
					if role == ax.RoleHeading {
						item.Level = headingLevel(node)
					}
					parentAX := root
					for parent := node.Parent; parent != nil; parent = parent.Parent {
						if found := findAccessibilityNode(root, uint64(parent.ID)); found != nil {
							parentAX = found
							break
						}
					}
					parentAX.Children = append(parentAX.Children, item)
					known[uint64(node.ID)] = true
				}
			}
		}
		if node.ShadowRoot != nil {
			for child := node.ShadowRoot.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(&s.Doc.Node)
	var sortChildren func([]ax.Node)
	sortChildren = func(nodes []ax.Node) {
		sort.SliceStable(nodes, func(i, j int) bool {
			left, leftOK := order[dom.NodeID(nodes[i].DOMNodeID)]
			right, rightOK := order[dom.NodeID(nodes[j].DOMNodeID)]
			if leftOK && rightOK {
				return left < right
			}
			return leftOK
		})
		for i := range nodes {
			sortChildren(nodes[i].Children)
		}
	}
	sortChildren(root.Children)
}

func findAccessibilityNode(root *ax.Node, id uint64) *ax.Node {
	if root == nil {
		return nil
	}
	if root.DOMNodeID == id {
		return root
	}
	for i := range root.Children {
		if found := findAccessibilityNode(&root.Children[i], id); found != nil {
			return found
		}
	}
	return nil
}

func (s *Session) AccessibilityInfoForNode(node *dom.Node) (role, label string, ok bool) {
	if s == nil || node == nil {
		return "", "", false
	}
	var find func([]ax.Node) bool
	find = func(nodes []ax.Node) bool {
		for _, current := range nodes {
			if current.DOMNodeID == uint64(node.ID) {
				role, label = current.RoleName, current.Label
				if role == "" {
					role = current.Role.String()
				}
				ok = true
				return true
			}
			if find(current.Children) {
				return true
			}
		}
		return false
	}
	find(s.AccessibilityTree())
	return role, label, ok
}

type axBuilder struct {
	session *Session
	objs    []layout.Object
	focused *dom.Node
}

type axBubble struct {
	text           string
	x0, y0, x1, y1 float32
}

func (b *axBuilder) visit(id layout.ObjectID) (*ax.Node, axBubble) {
	if id == 0 || int(id) >= len(b.objs) {
		return nil, axBubble{}
	}
	obj := &b.objs[id]
	if obj.Node == nil {
		kids, _ := b.collect(obj)
		if len(kids) == 1 && kids[0].Role == ax.RoleStaticText {
			k := kids[0]
			return nil, axBubble{text: k.Label, x0: k.X0, y0: k.Y0, x1: k.X1, y1: k.Y1}
		}
		if len(kids) == 0 {
			return nil, axBubble{}
		}
		x0, y0, x1, y1 := b.bounds(obj, kids)
		return &ax.Node{Role: ax.RoleGroup, X0: x0, Y0: y0, X1: x1, Y1: y1, Children: kids}, axBubble{}
	}
	if obj.Node.Type == dom.NodeText {
		t := strings.TrimSpace(obj.Node.DataContent)
		x0, y0, x1, y1 := obj.BorderRect()
		if t == "" || !(x0 < x1 && y0 < y1) {
			return nil, axBubble{}
		}
		return nil, axBubble{text: t, x0: x0, y0: y0, x1: x1, y1: y1}
	}

	kids, label := b.collect(obj)
	x0, y0, x1, y1 := b.bounds(obj, kids)
	visible := x0 < x1 && y0 < y1
	emit := func(role ax.Role, lab, val, href string) *ax.Node {
		if b.session.jsRT != nil {
			if authorName, set := b.session.jsRT.ElementInternalsAccessibleName(obj.Node); set {
				lab = authorName
			}
		}
		return &ax.Node{Role: role, RoleName: role.String(), DOMNodeID: uint64(obj.Node.ID),
			ID: obj.Node.GetAttribute("id"), Label: lab, Value: val, Href: href,
			Focused: obj.Node == b.focused,
			X0:      x0, Y0: y0, X1: x1, Y1: y1, Children: kids}
	}

	n := obj.Node
	if b.session.jsRT != nil {
		roleName := b.session.jsRT.ElementInternalsARIA(n, "role")
		if strings.TrimSpace(roleName) == "" {
			roleName = n.GetAttribute("role")
		}
		if role, standardName, ok := elementInternalsAXRole(roleName); ok {
			name, authorName := b.session.jsRT.ElementInternalsAccessibleName(n)
			if !authorName {
				name = label
			}
			if n.GetAttribute("aria-hidden") == "true" || obj.Style != nil && obj.Style.Visibility != "visible" || !visible && !authorName {
				return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
			}
			accessible := emit(role, name, "", "")
			accessible.RoleName = standardName
			if role == ax.RoleHeading {
				accessible.Level = headingLevel(n)
			}
			return accessible, axBubble{}
		}
	}
	if n.GetAttribute("role") == "search" {
		if !visible {
			return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
		}
		return emit(ax.RoleSearch, label, "", ""), axBubble{}
	}
	switch n.DataAtom {
	case dom.AtomSearch:
		if !visible {
			return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
		}
		return emit(ax.RoleSearch, label, "", ""), axBubble{}
	case dom.AtomProgress:
		if !visible {
			return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
		}
		val := n.GetAttribute("value")
		max := n.GetAttribute("max")
		if max == "" {
			max = "1"
		}
		return emit(ax.RoleProgressBar, label, val+"/"+max, ""), axBubble{}
	case dom.AtomMeter:
		if !visible {
			return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
		}
		val := n.GetAttribute("value")
		return emit(ax.RoleMeter, label, val, ""), axBubble{}
	case dom.AtomH1, dom.AtomH2, dom.AtomH3, dom.AtomH4, dom.AtomH5, dom.AtomH6:
		if !visible {
			return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
		}
		if text := optionTextValue(n.TextContent()); text != "" {
			label = text
		}
		node := emit(ax.RoleHeading, label, "", "")
		node.Level = headingLevel(n)
		return node, axBubble{}
	case dom.AtomA:
		if href := n.GetAttribute("href"); href != "" && visible {
			return emit(ax.RoleLink, label, "", href), axBubble{}
		}
		return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
	case dom.AtomImg:
		if !visible {
			return nil, axBubble{}
		}
		return emit(ax.RoleImage, n.GetAttribute("alt"), "", ""), axBubble{}
	case dom.AtomButton:
		if !visible {
			return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
		}
		return emit(ax.RoleButton, label, "", ""), axBubble{}
	case dom.AtomInput:
		typ := strings.ToLower(n.GetAttribute("type"))
		if !visible || typ == "hidden" {
			return nil, axBubble{}
		}
		switch typ {
		case "button", "submit", "reset":
			return emit(ax.RoleButton, n.InputControlValue(), "", ""), axBubble{}
		case "checkbox", "radio", "range", "file", "color",
			"date", "time", "datetime-local", "month", "week":
			return nil, axBubble{}
		default:
			return emit(ax.RoleTextField, b.textControlLabel(n), n.InputControlValue(), ""), axBubble{}
		}
	case dom.AtomTextarea:
		if !visible {
			return nil, axBubble{}
		}
		node := emit(ax.RoleTextField, b.textControlLabel(n), b.session.controlValue(n), "")
		node.Children = nil
		return node, axBubble{}
	case dom.AtomSelect:
		if !visible {
			return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
		}
		selected := SelectedOption(n)
		controlDisabled := dom.IsFormControlDisabled(n)
		options := make([]ax.Node, 0)
		groupIdx := make(map[*dom.Node]int)
		for _, option := range dom.SelectOptions(n) {
			node := ax.Node{
				Role: ax.RoleOption, RoleName: ax.RoleOption.String(), DOMNodeID: uint64(option.ID),
				Label:    option.OptionAccessibleName(),
				Selected: option.OptionSelected(), Disabled: option.OptionDisabled(),
				Props: optionAXProps(option, controlDisabled),
				X0:    x0, Y0: y0, X1: x1, Y1: y1,
			}
			group := option.Parent
			if group == nil || group.DataAtom != dom.AtomOptgroup {
				options = append(options, node)
				continue
			}
			idx, ok := groupIdx[group]
			if !ok {
				options = append(options, ax.Node{
					Role: ax.RoleGroup, RoleName: ax.RoleGroup.String(), DOMNodeID: uint64(group.ID),
					ID: group.GetAttribute("id"), Label: dom.NormalizeOptionText(group.GetAttribute("label")),
					X0: x0, Y0: y0, X1: x1, Y1: y1,
				})
				idx = len(options) - 1
				groupIdx[group] = idx
			}
			options[idx].Children = append(options[idx].Children, node)
		}
		selectLabel := strings.TrimSpace(n.GetAttribute("aria-label"))
		if selectLabel == "" {
			selectLabel = b.associatedLabel(n)
		}
		if selectLabel == "" {
			selectLabel = strings.TrimSpace(label)
		}
		listbox := dom.SelectAnnouncesListbox(n)
		display := ""
		if !listbox {
			if selected != nil {
				display = selected.OptionDisplayStyle()
			} else if opts := dom.SelectOptions(n); len(opts) > 0 {
				display = opts[0].OptionDisplayStyle()
			}
		}
		role := ax.RoleComboBox
		if listbox {
			role = ax.RoleListBox
		}
		combo := emit(role, selectLabel, display, "")
		combo.Disabled = controlDisabled
		combo.Children = options
		activeDescendant := selected
		if !listbox && !combo.Focused {
			activeDescendant = nil
		}
		combo.Props = selectAXProps(n, listbox, controlDisabled, combo.Focused, activeDescendant)
		return combo, axBubble{}
	}

	hasKids := false
	for _, k := range kids {
		if k.Role != ax.RoleStaticText {
			hasKids = true
			break
		}
	}
	if !hasKids && b.inlineBox(obj) {
		return nil, axBubble{text: label, x0: x0, y0: y0, x1: x1, y1: y1}
	}
	if len(kids) == 0 {
		return nil, axBubble{}
	}
	return emit(ax.RoleGroup, "", "", ""), axBubble{}
}

func optionAXProps(option *dom.Node, controlDisabled bool) []ax.Prop {
	optionDisabled := option.OptionDisabled()
	var props []ax.Prop
	if optionDisabled || controlDisabled {
		props = append(props, ax.BoolProp("disabled", true))
	}
	if !optionDisabled {
		props = append(props, ax.BoolProp("focusable", true))
	}
	if !optionDisabled && !controlDisabled {
		props = append(props, ax.BoolProp("selected", option.OptionSelected()))
	}
	return props
}

func selectAXProps(n *dom.Node, listbox, disabled, focused bool, activeDescendant *dom.Node) []ax.Prop {
	var props []ax.Prop
	if disabled {
		props = append(props, ax.BoolProp("disabled", true))
	} else {
		props = append(props, ax.BoolProp("focusable", true))
	}
	if focused {
		props = append(props, ax.BoolProp("focused", true))
	}
	if invalid, ok := selectInvalidProp(n); ok {
		props = append(props, ax.StringProp("invalid", invalid))
	}
	if listbox {
		props = append(props, ax.BoolProp("multiselectable", n.HasAttribute("multiple")))
		props = append(props, ax.StringProp("orientation", ariaOrientationToken(n, "vertical")))
		props = append(props, ax.BoolProp("required",
			n.HasAttribute("required") || ariaSaysTrue(n, "aria-required")))
	} else {
		props = append(props, ax.BoolProp("expanded", false))
		props = append(props, ax.StringProp("hasPopup", ariaHasPopupToken(n)))
		if orientation := ariaOrientationToken(n, ""); orientation != "" {
			props = append(props, ax.StringProp("orientation", orientation))
		}
	}
	if activeDescendant != nil {
		props = append(props, ax.RefProp("activedescendant", uint64(activeDescendant.ID)))
	}
	return props
}

func selectInvalidProp(n *dom.Node) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(n.GetAttribute("aria-invalid"))) {
	case "":
		if selectConstraintFails(n) {
			return "true", true
		}
		return "false", true
	case "false":
		return "false", true
	case "grammar", "spelling":
		return "", false
	default:
		return "true", true
	}
}

func selectConstraintFails(n *dom.Node) bool {
	return n.CustomValidityMessage != "" || dom.SelectValueMissing(n)
}

func ariaHasPopupToken(n *dom.Node) string {
	token := strings.ToLower(strings.TrimSpace(n.GetAttribute("aria-haspopup")))
	switch token {
	case "menu", "listbox", "tree", "grid", "dialog":
		return token
	}
	return "menu"
}

func ariaOrientationToken(n *dom.Node, fallback string) string {
	token := strings.ToLower(strings.TrimSpace(n.GetAttribute("aria-orientation")))
	switch token {
	case "horizontal", "vertical":
		return token
	}
	return fallback
}

func ariaSaysTrue(n *dom.Node, name string) bool {
	return strings.EqualFold(strings.TrimSpace(n.GetAttribute(name)), "true")
}

func elementInternalsAXRole(role string) (ax.Role, string, bool) {
	for _, token := range strings.Fields(strings.ToLower(role)) {
		switch token {
		case "group":
			return ax.RoleGroup, token, true
		case "region":
			return ax.RoleGroup, token, true
		case "link":
			return ax.RoleLink, token, true
		case "button":
			return ax.RoleButton, token, true
		case "img":
			return ax.RoleImage, "image", true
		case "image":
			return ax.RoleImage, token, true
		case "textbox":
			return ax.RoleTextField, token, true
		case "search":
			return ax.RoleSearch, token, true
		case "progressbar":
			return ax.RoleProgressBar, token, true
		case "meter":
			return ax.RoleMeter, token, true
		case "combobox":
			return ax.RoleComboBox, token, true
		case "option":
			return ax.RoleOption, token, true
		case "listbox":
			return ax.RoleListBox, token, true
		case "heading":
			return ax.RoleHeading, token, true
		case "directory":
			return ax.RoleGroup, "list", true
		case "alert", "alertdialog", "application", "article", "banner", "blockquote", "caption", "cell", "checkbox", "code", "columnheader", "complementary", "contentinfo", "definition", "deletion", "dialog", "document", "emphasis", "feed", "figure", "form", "generic", "grid", "gridcell", "insertion", "list", "listitem", "log", "main", "marquee", "math", "menu", "menubar", "menuitem", "menuitemcheckbox", "menuitemradio", "navigation", "none", "note", "presentation", "paragraph", "radio", "radiogroup", "range", "row", "rowgroup", "rowheader", "scrollbar", "searchbox", "separator", "slider", "spinbutton", "status", "strong", "subscript", "superscript", "switch", "tab", "table", "tablist", "tabpanel", "term", "time", "timer", "toolbar", "tooltip", "tree", "treegrid", "treeitem", "window":
			return ax.RoleGroup, token, true
		}
	}
	return 0, "", false
}

func headingLevel(n *dom.Node) int {
	implicit := 2
	switch n.DataAtom {
	case dom.AtomH1:
		implicit = 1
	case dom.AtomH2:
		implicit = 2
	case dom.AtomH3:
		implicit = 3
	case dom.AtomH4:
		implicit = 4
	case dom.AtomH5:
		implicit = 5
	case dom.AtomH6:
		implicit = 6
	}
	if raw := strings.TrimSpace(n.GetAttribute("aria-level")); raw != "" {
		if level, err := strconv.Atoi(raw); err == nil && level >= 1 && level <= 6 {
			return level
		}
	}
	return implicit
}

func (b *axBuilder) textControlLabel(n *dom.Node) string {
	if n == nil {
		return ""
	}
	if label := strings.TrimSpace(n.GetAttribute("aria-label")); label != "" {
		return label
	}
	if label := b.associatedLabel(n); label != "" {
		return label
	}
	if label := strings.TrimSpace(n.GetAttribute("title")); label != "" {
		return label
	}
	return strings.TrimSpace(n.GetAttribute("placeholder"))
}

func (b *axBuilder) associatedLabel(n *dom.Node) string {
	if n == nil {
		return ""
	}
	if id := n.GetAttribute("id"); id != "" {
		var find func(*dom.Node) string
		find = func(current *dom.Node) string {
			if current == nil {
				return ""
			}
			if current.DataAtom == dom.AtomLabel && current.GetAttribute("for") == id {
				return strings.TrimSpace(current.TextContent())
			}
			for child := current.FirstChild; child != nil; child = child.NextSibling {
				if label := find(child); label != "" {
					return label
				}
			}
			return ""
		}
		if label := find(&b.session.Doc.Node); label != "" {
			return label
		}
	}
	for parent := n.Parent; parent != nil; parent = parent.Parent {
		if parent.DataAtom == dom.AtomLabel {
			return strings.TrimSpace(parent.TextContent())
		}
	}
	return ""
}

func (b *axBuilder) collect(obj *layout.Object) ([]ax.Node, string) {
	var kids []ax.Node
	var words []string
	lastWasText := false
	appendText := func(t string, bx0, by0, bx1, by1 float32) {
		if n := len(kids); lastWasText && n > 0 && kids[n-1].Role == ax.RoleStaticText {
			k := &kids[n-1]
			k.Label += " " + t
			if bx0 < k.X0 {
				k.X0 = bx0
			}
			if by0 < k.Y0 {
				k.Y0 = by0
			}
			if bx1 > k.X1 {
				k.X1 = bx1
			}
			if by1 > k.Y1 {
				k.Y1 = by1
			}
			return
		}
		kids = append(kids, ax.Node{Role: ax.RoleStaticText, Label: t,
			X0: bx0, Y0: by0, X1: bx1, Y1: by1})
		lastWasText = true
	}
	for k := obj.FirstKid; k != 0 && int(k) < len(b.objs); k = b.objs[k].NextSibling {
		ck := &b.objs[k]
		if ck.Node != nil && ck.Node.Type == dom.NodeText {
			if t := strings.TrimSpace(ck.Node.DataContent); t != "" {
				if rx0, ry0, rx1, ry1 := ck.BorderRect(); rx0 < rx1 && ry0 < ry1 {
					words = append(words, t)
					appendText(t, rx0, ry0, rx1, ry1)
				}
			}
			continue
		}
		node, bubble := b.visit(k)
		if node != nil {
			kids = append(kids, *node)
			lastWasText = false
			continue
		}
		if bubble.text != "" {
			words = append(words, bubble.text)
			appendText(bubble.text, bubble.x0, bubble.y0, bubble.x1, bubble.y1)
		}
	}
	return kids, strings.Join(words, " ")
}

func (b *axBuilder) bounds(obj *layout.Object, kids []ax.Node) (x0, y0, x1, y1 float32) {
	x0, y0, x1, y1 = obj.BorderRect()
	if x0 < x1 && y0 < y1 {
		return
	}
	for _, k := range kids {
		if k.X1 <= k.X0 && k.Y1 <= k.Y0 {
			continue
		}
		if x1 <= x0 && y1 <= y0 {
			x0, y0, x1, y1 = k.X0, k.Y0, k.X1, k.Y1
			continue
		}
		if k.X0 < x0 {
			x0 = k.X0
		}
		if k.Y0 < y0 {
			y0 = k.Y0
		}
		if k.X1 > x1 {
			x1 = k.X1
		}
		if k.Y1 > y1 {
			y1 = k.Y1
		}
	}
	return
}

func (b *axBuilder) inlineBox(obj *layout.Object) bool {
	return obj.Style != nil && obj.Style.Display == style.DisplayInline
}
