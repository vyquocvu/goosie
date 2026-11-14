package js

import (
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

const selectorCacheCap = 256

type selectorPlan struct {
	raw       string
	selectors []css.Selector
	syntaxErr string
	indexKind byte
	indexVal  string
	usesScope bool
}

func compileSelectorPlan(raw string) *selectorPlan {
	p := &selectorPlan{raw: raw}
	if raw == "" {
		p.syntaxErr = "The provided selector is empty."
		return p
	}
	sels, err := css.ParseSelectorList(raw)
	if err != nil {
		p.syntaxErr = "'" + raw + "' is not a valid selector."
		return p
	}
	p.selectors = sels
	p.usesScope = css.SelectorsUseScope(sels)
	if kind, val, ok := simpleSelectorShape(raw, sels); ok {
		p.indexKind, p.indexVal = kind, val
	}
	return p
}

func simpleSelectorShape(raw string, sels []css.Selector) (kind byte, val string, ok bool) {
	if len(sels) != 1 || len(sels[0].Parts) != 1 {
		return 0, "", false
	}
	part := sels[0].Parts[0]
	if len(part.Conditions) != 1 {
		return 0, "", false
	}
	c := part.Conditions[0]
	switch c.Type {
	case css.CondType:
		if !isLowerTag(strings.TrimSpace(raw)) {
			return 0, "", false
		}
		return 'T', c.Value, true
	case css.CondClass:
		return '.', c.Value, true
	case css.CondID:
		return '#', c.Value, true
	}
	return 0, "", false
}

func (r *Runtime) selectorPlanFor(raw string) *selectorPlan {
	if p, ok := r.selectorPlans[raw]; ok {
		return p
	}
	if r.selectorPlans == nil {
		r.selectorPlans = make(map[string]*selectorPlan)
	} else if len(r.selectorPlans) >= selectorCacheCap {
		r.selectorPlans = make(map[string]*selectorPlan)
	}
	p := compileSelectorPlan(raw)
	r.selectorPlans[raw] = p
	return p
}

func (r *Runtime) selectorArgument(call goja.FunctionCall, method string, root *dom.Node) string {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute '" + method + "' on '" +
			selectorContextName(root) + "': 1 argument required, but only 0 present."))
	}
	return call.Arguments[0].String()
}

func (r *Runtime) throwSelectorSyntaxError(method string, root *dom.Node, p *selectorPlan) {
	panic(r.domError("SyntaxError", "Failed to execute '"+method+"' on '"+
		selectorContextName(root)+"': "+p.syntaxErr))
}

func selectorContextName(root *dom.Node) string {
	if root == nil || root.Type == dom.NodeDocument {
		return "Document"
	}
	if root.Element() {
		return "Element"
	}
	if root.Type == dom.NodeDocumentFragment {
		return "DocumentFragment"
	}
	return "Node"
}

func (r *Runtime) queryRoot(v goja.Value) *dom.Node {
	if n := r.getNode(v); n != nil {
		return n
	}
	if r.domDoc == nil {
		return nil
	}
	return &r.domDoc.Node
}

func scopeForRoot(root *dom.Node) *dom.Node {
	if root == nil || root.Element() {
		return root
	}
	if root.Type == dom.NodeDocument && root.Doc != nil && root.Doc.HTML != nil {
		return root.Doc.HTML
	}
	return root
}

func scopedSelectors(p *selectorPlan, root *dom.Node) []css.Selector {
	if !p.usesScope {
		return p.selectors
	}
	scope := scopeForRoot(root)
	out := make([]css.Selector, len(p.selectors))
	for i := range p.selectors {
		out[i] = p.selectors[i].WithScope(scope)
	}
	return out
}

func matchesAny(sels []css.Selector, n *dom.Node, ix *css.SiblingIndex) bool {
	for i := range sels {
		if sels[i].MatchesIndexed(n, ix) {
			return true
		}
	}
	return false
}

func (r *Runtime) selectorRootInShadowTree(root *dom.Node) bool {
	for current := root; current != nil; current = current.Parent {
		parent := current.Parent
		if parent != nil && parent.ShadowRoot != nil && shadowRootContains(parent.ShadowRoot, root) {
			return true
		}
	}
	return false
}

func (r *Runtime) querySelectorPlan(root *dom.Node, p *selectorPlan) *dom.Node {
	if root == nil {
		return nil
	}
	if root.Type == dom.NodeDocument || root.IsConnected() && !r.selectorRootInShadowTree(root) {
		if node, ok := r.querySelectorIndexed(root, p); ok {
			return node
		}
	}
	sels := scopedSelectors(p, root)
	ix := css.NewSiblingIndex()
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if found := firstSelectorMatch(c, sels, ix); found != nil {
			return found
		}
	}
	return nil
}

func firstSelectorMatch(n *dom.Node, sels []css.Selector, ix *css.SiblingIndex) *dom.Node {
	if n.Element() && matchesAny(sels, n, ix) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := firstSelectorMatch(c, sels, ix); found != nil {
			return found
		}
	}
	return nil
}

func (r *Runtime) querySelectorAllPlan(root *dom.Node, p *selectorPlan) []*dom.Node {
	if root == nil {
		return nil
	}
	if !root.DocumentFragment() && (root.Type == dom.NodeDocument || root.IsConnected() && !r.selectorRootInShadowTree(root)) {
		if nodes, ok := r.querySelectorAllIndexed(root, p); ok {
			return nodes
		}
	}
	sels := scopedSelectors(p, root)
	ix := css.NewSiblingIndex()
	var out []*dom.Node
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		collectSelectorMatches(c, sels, ix, &out)
	}
	return out
}

func collectSelectorMatches(n *dom.Node, sels []css.Selector, ix *css.SiblingIndex, out *[]*dom.Node) {
	if n.Element() && matchesAny(sels, n, ix) {
		*out = append(*out, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectSelectorMatches(c, sels, ix, out)
	}
}

func (r *Runtime) matchesSelector(n *dom.Node, p *selectorPlan) bool {
	if n == nil || !n.Element() {
		return false
	}
	return matchesAny(scopedSelectors(p, n), n, css.NewSiblingIndex())
}

func (r *Runtime) closestSelector(n *dom.Node, p *selectorPlan) *dom.Node {
	if n == nil || !n.Element() {
		return nil
	}
	sels := scopedSelectors(p, n)
	ix := css.NewSiblingIndex()
	for cur := n; cur != nil && cur.Element(); cur = cur.Parent {
		if matchesAny(sels, cur, ix) {
			return cur
		}
	}
	return nil
}

func (r *Runtime) jsQuerySelector(call goja.FunctionCall) goja.Value {
	root := r.queryRoot(call.This)
	if r.domDoc == nil {
		return goja.Null()
	}
	raw := r.selectorArgument(call, "querySelector", root)
	p := r.selectorPlanFor(raw)
	if p.syntaxErr != "" {
		r.throwSelectorSyntaxError("querySelector", root, p)
	}
	if node := r.querySelectorPlan(root, p); node != nil {
		return r.wrapNode(node)
	}
	return goja.Null()
}

func (r *Runtime) jsQuerySelectorAll(call goja.FunctionCall) goja.Value {
	root := r.queryRoot(call.This)
	if r.domDoc == nil {
		return newJSArray(r.vm, nil)
	}
	raw := r.selectorArgument(call, "querySelectorAll", root)
	p := r.selectorPlanFor(raw)
	if p.syntaxErr != "" {
		r.throwSelectorSyntaxError("querySelectorAll", root, p)
	}
	nodes := r.querySelectorAllPlan(root, p)
	wrapped := make([]goja.Value, len(nodes))
	for i, n := range nodes {
		wrapped[i] = r.wrapNode(n)
	}
	return newJSArray(r.vm, wrapped)
}

func (r *Runtime) jsMatches(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	raw := r.selectorArgument(call, "matches", node)
	p := r.selectorPlanFor(raw)
	if p.syntaxErr != "" {
		r.throwSelectorSyntaxError("matches", node, p)
	}
	return r.vm.ToValue(r.matchesSelector(node, p))
}

func (r *Runtime) jsClosest(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	raw := r.selectorArgument(call, "closest", node)
	p := r.selectorPlanFor(raw)
	if p.syntaxErr != "" {
		r.throwSelectorSyntaxError("closest", node, p)
	}
	if found := r.closestSelector(node, p); found != nil {
		return r.wrapNode(found)
	}
	return goja.Null()
}

func (r *Runtime) querySelectorIndexed(root *dom.Node, p *selectorPlan) (*dom.Node, bool) {
	doc := r.indexDocFor(root)
	if root == nil || doc == nil || p.indexKind == 0 {
		return nil, false
	}
	switch p.indexKind {
	case '#':
		return r.querySelectorID(root, p.indexVal)
	case '.':
		for _, n := range doc.ElementsByClassName(p.indexVal) {
			if containedStrict(root, n) {
				return n, true
			}
		}
		return nil, true
	default:
		for _, n := range doc.ElementsByTagName(p.indexVal) {
			if containedStrict(root, n) {
				return n, true
			}
		}
		return nil, true
	}
}

func (r *Runtime) indexDocFor(root *dom.Node) *dom.Document {
	if root != nil && root.Doc != nil {
		return root.Doc
	}
	return r.domDoc
}

func containedStrict(root, n *dom.Node) bool {
	if n == nil || n == root {
		return false
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if p == root {
			return true
		}
	}
	return false
}

func isLowerTag(sel string) bool {
	if sel == "" {
		return false
	}
	for i := 0; i < len(sel); i++ {
		if c := sel[i]; c >= 'A' && c <= 'Z' {
			return false
		}
	}
	return true
}

func (r *Runtime) querySelectorID(root *dom.Node, id string) (node *dom.Node, ok bool) {
	doc := r.indexDocFor(root)
	if root == nil || doc == nil || id == "" {
		return nil, false
	}
	node = doc.ElementByID(id)
	if node == nil {
		return nil, true
	}
	if node == root {
		return nil, false
	}
	for p := node.Parent; p != nil; p = p.Parent {
		if p == root {
			return node, true
		}
	}
	return nil, false
}

func (r *Runtime) querySelectorAllIndexed(root *dom.Node, p *selectorPlan) ([]*dom.Node, bool) {
	doc := r.indexDocFor(root)
	if root == nil || doc == nil || p.indexKind == 0 {
		return nil, false
	}
	var all []*dom.Node
	switch p.indexKind {
	case '.':
		all = doc.ElementsByClassName(p.indexVal)
	case 'T':
		all = doc.ElementsByTagName(p.indexVal)
	default:
		return nil, false
	}
	if root.Type == dom.NodeDocument {
		return all, true
	}
	var out []*dom.Node
	for _, n := range all {
		if containedStrict(root, n) {
			out = append(out, n)
		}
	}
	return out, true
}
