package style

import (
	"reflect"
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

const (
	stateHover uint8 = 1 << iota
	stateFocus
	stateFocusVisible
	stateActive
)

// InteractiveResolution keeps the old immutable style pointers for the nodes
// that were recomputed. The engine uses these to decide whether layout inputs
// changed without comparing the whole document.
type InteractiveResolution struct {
	Nodes          []dom.NodeID
	PreviousStyles map[dom.NodeID]*ComputedStyle
	PreviousPseudo map[PseudoKey]*ComputedStyle
	Visited        int
}

type interactiveSnapshot struct {
	hover, focus, focusVisible, active bool
}

type nodeStyleInput struct {
	typeOf      dom.NodeType
	namespace   dom.Namespace
	data        string
	dataAtom    dom.Atom
	dataContent string
	attrStart   int
	attrCount   int
	parent      dom.NodeID
	firstChild  dom.NodeID
	lastChild   dom.NodeID
	nextSibling dom.NodeID
	prevSibling dom.NodeID
	shadowFirst dom.NodeID
	shadowLast  dom.NodeID
	shadowMode  string
	hasShadow   bool
	modal       bool
	popoverOpen bool
	scopeRoot   bool
}

type treeInputSnapshot struct {
	nodes      map[dom.NodeID]nodeStyleInput
	attributes []dom.Attribute
}

// ResolveInteractiveStates updates only the changed node subtrees when the
// stylesheet consists of local interaction selectors. More complex selector
// dependencies, DOM changes, and viewport changes return false so callers can
// use the complete document resolver.
func (r *Resolver) ResolveInteractiveStates(doc *dom.Document, vp Viewport, fonts *CustomFonts, styles map[dom.NodeID]*ComputedStyle, pseudos map[PseudoKey]*ComputedStyle) (InteractiveResolution, bool) {
	var out InteractiveResolution
	if r == nil || r.index == nil || doc == nil || styles == nil || pseudos == nil || !r.incrementalInteractiveSafe || !r.hasSnapshot {
		return out, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.incrementalInteractiveSafe || !r.hasSnapshot || vp != r.snapshotViewport {
		return out, false
	}

	nodes, inputs, states := captureInteractiveState(doc)
	if !sameTreeInputs(r.snapshotInputs, inputs) || !reflect.DeepEqual(r.snapshotProperties, doc.RegisteredProperties()) || len(r.snapshotStates) != len(states) {
		return out, false
	}
	changed := make(map[dom.NodeID]uint8)
	for id, before := range r.snapshotStates {
		now, ok := states[id]
		if !ok {
			return out, false
		}
		if before != now {
			changed[id] = interactiveStateDelta(before, now)
		}
	}
	if len(changed) == 0 {
		return out, false
	}

	roots := make([]*dom.Node, 0, len(changed))
	for id, delta := range changed {
		n := nodes[id]
		if n == nil || !n.Element() {
			return out, false
		}
		matchedStateRule := false
		for _, candidate := range r.index.candidates(n) {
			if selectorUsesState(candidate.sel, delta) {
				matchedStateRule = true
				break
			}
		}
		if !matchedStateRule {
			continue
		}
		hasChangedAncestor := false
		for p := n.Parent; p != nil; p = p.Parent {
			if parentDelta, ok := changed[p.ID]; ok {
				parentNode := nodes[p.ID]
				parentMatches := false
				for _, candidate := range r.index.candidates(parentNode) {
					if selectorUsesState(candidate.sel, parentDelta) {
						parentMatches = true
						break
					}
				}
				if !parentMatches {
					continue
				}
				hasChangedAncestor = true
				break
			}
		}
		if !hasChangedAncestor {
			roots = append(roots, n)
		}
	}
	if len(roots) == 0 {
		r.snapshotInputs, r.snapshotStates = inputs, states
		r.snapshotProperties = doc.RegisteredProperties()
		r.snapshotViewport = vp
		return out, true
	}

	affected := make(map[dom.NodeID]struct{})
	for _, root := range roots {
		collectStyleSubtree(root, affected)
	}
	out.Nodes = make([]dom.NodeID, 0, len(affected))
	out.PreviousStyles = make(map[dom.NodeID]*ComputedStyle, len(affected))
	out.PreviousPseudo = make(map[PseudoKey]*ComputedStyle)
	for id := range affected {
		out.Nodes = append(out.Nodes, id)
		out.PreviousStyles[id] = styles[id]
	}
	for key, cs := range pseudos {
		if _, ok := affected[key.NodeID]; ok {
			out.PreviousPseudo[key] = cs
		}
	}
	six := css.NewSiblingIndex()
	for _, root := range roots {
		resolveNode(root, r.index, styles, vp, fonts, six)
	}
	reResolvePseudoSubtree(roots, r.index, styles, fonts, affected, pseudos)
	out.Visited = len(affected)
	r.snapshotInputs, r.snapshotStates = inputs, states
	r.snapshotProperties = doc.RegisteredProperties()
	r.snapshotViewport = vp
	return out, true
}

func (r *Resolver) snapshotInteractiveState(doc *dom.Document, vp Viewport) {
	if r == nil || doc == nil {
		return
	}
	_, inputs, states := captureInteractiveState(doc)
	r.snapshotInputs, r.snapshotStates = inputs, states
	r.snapshotProperties = doc.RegisteredProperties()
	r.snapshotViewport = vp
	r.hasSnapshot = true
}

func captureInteractiveState(doc *dom.Document) (map[dom.NodeID]*dom.Node, treeInputSnapshot, map[dom.NodeID]interactiveSnapshot) {
	nodes := make(map[dom.NodeID]*dom.Node)
	inputs := treeInputSnapshot{nodes: make(map[dom.NodeID]nodeStyleInput)}
	states := make(map[dom.NodeID]interactiveSnapshot)
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		nodes[n.ID] = n
		input := nodeStyleInput{
			typeOf: n.Type, namespace: n.Namespace, data: n.Data, dataAtom: n.DataAtom,
			dataContent: n.DataContent, attrStart: len(inputs.attributes), attrCount: len(n.Attr),
			parent: nodeID(n.Parent), firstChild: nodeID(n.FirstChild), lastChild: nodeID(n.LastChild),
			nextSibling: nodeID(n.NextSibling), prevSibling: nodeID(n.PrevSibling),
			modal: n.IsModal, popoverOpen: n.IsPopoverOpen, scopeRoot: n.IsScopeRoot,
		}
		inputs.attributes = append(inputs.attributes, n.Attr...)
		if n.ShadowRoot != nil {
			input.hasShadow = true
			input.shadowFirst = nodeID(n.ShadowRoot.FirstChild)
			input.shadowLast = nodeID(n.ShadowRoot.LastChild)
			input.shadowMode = n.ShadowRoot.Mode
		}
		inputs.nodes[n.ID] = input
		states[n.ID] = interactiveSnapshot{
			hover: css.IsHovered(n), focus: css.IsFocused(n),
			focusVisible: css.IsFocusVisible(n), active: css.IsActive(n),
		}
		if n.ShadowRoot != nil {
			for c := n.ShadowRoot.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	return nodes, inputs, states
}

func nodeID(n *dom.Node) dom.NodeID {
	if n == nil {
		return 0
	}
	return n.ID
}

func sameTreeInputs(a, b treeInputSnapshot) bool {
	if len(a.nodes) != len(b.nodes) || len(a.attributes) != len(b.attributes) {
		return false
	}
	for id, before := range a.nodes {
		after, ok := b.nodes[id]
		if !ok || !sameNodeStyleInput(before, after) {
			return false
		}
		for i := 0; i < before.attrCount; i++ {
			if a.attributes[before.attrStart+i] != b.attributes[after.attrStart+i] {
				return false
			}
		}
	}
	return true
}

func sameNodeStyleInput(a, b nodeStyleInput) bool {
	if a.typeOf != b.typeOf || a.namespace != b.namespace || a.data != b.data || a.dataAtom != b.dataAtom ||
		a.dataContent != b.dataContent || a.parent != b.parent || a.firstChild != b.firstChild ||
		a.lastChild != b.lastChild || a.nextSibling != b.nextSibling || a.prevSibling != b.prevSibling ||
		a.shadowFirst != b.shadowFirst || a.shadowLast != b.shadowLast || a.shadowMode != b.shadowMode ||
		a.hasShadow != b.hasShadow || a.modal != b.modal || a.popoverOpen != b.popoverOpen || a.scopeRoot != b.scopeRoot ||
		a.attrStart != b.attrStart || a.attrCount != b.attrCount {
		return false
	}
	return true
}

func collectStyleSubtree(root *dom.Node, ids map[dom.NodeID]struct{}) {
	if root == nil {
		return
	}
	ids[root.ID] = struct{}{}
	if root.ShadowRoot != nil {
		for c := root.ShadowRoot.FirstChild; c != nil; c = c.NextSibling {
			collectStyleSubtree(c, ids)
		}
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		collectStyleSubtree(c, ids)
	}
}

func interactiveSelectorSupport(selectors []css.Selector) (bool, uint8) {
	var mask uint8
	unsafe := false
	for _, selector := range selectors {
		usesState := false
		localMask := uint8(0)
		for _, part := range selector.Parts {
			for _, cond := range part.Conditions {
				if cond.Type != css.CondPseudoClass {
					continue
				}
				bit := interactiveStateBit(cond.Value)
				if bit != 0 {
					usesState = true
					localMask |= bit
					continue
				}
				if changesDuringInteraction(cond.Value) {
					unsafe = true
				}
				if isFunctionalPseudo(cond.Value) && strings.Contains(cond.Pseudo, ":") {
					if containsInteractiveState(cond.Pseudo) {
						usesState = true
					}
					unsafe = true
				}
			}
		}
		if !usesState {
			continue
		}
		mask |= localMask
		if len(selector.Parts) != 1 || localMask == 0 {
			unsafe = true
		}
	}
	return mask != 0 && !unsafe, mask
}

func interactiveStateBit(name string) uint8 {
	switch strings.ToLower(name) {
	case "hover":
		return stateHover
	case "focus":
		return stateFocus
	case "focus-visible":
		return stateFocusVisible
	case "active":
		return stateActive
	default:
		return 0
	}
}

func interactiveStateDelta(before, after interactiveSnapshot) uint8 {
	var delta uint8
	if before.hover != after.hover {
		delta |= stateHover
	}
	if before.focus != after.focus {
		delta |= stateFocus
	}
	if before.focusVisible != after.focusVisible {
		delta |= stateFocusVisible
	}
	if before.active != after.active {
		delta |= stateActive
	}
	return delta
}

func selectorUsesState(selector css.Selector, states uint8) bool {
	for _, part := range selector.Parts {
		for _, cond := range part.Conditions {
			if cond.Type == css.CondPseudoClass && interactiveStateBit(cond.Value)&states != 0 {
				return true
			}
		}
	}
	return false
}

func changesDuringInteraction(name string) bool {
	switch strings.ToLower(name) {
	case "focus-within", "checked", "placeholder-shown", "valid", "invalid", "user-valid", "user-invalid",
		"enabled", "disabled", "required", "optional", "in-range", "out-of-range", "read-only", "read-write",
		"default", "indeterminate", "target":
		return true
	default:
		return false
	}
}

func isFunctionalPseudo(name string) bool {
	switch strings.ToLower(name) {
	case "not", "is", "where", "has", "host", "host-context":
		return true
	default:
		return false
	}
}

func containsInteractiveState(arg string) bool {
	arg = strings.ToLower(arg)
	for _, name := range []string{":hover", ":focus", ":focus-visible", ":active"} {
		if strings.Contains(arg, name) {
			return true
		}
	}
	return false
}
