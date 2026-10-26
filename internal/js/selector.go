package js

import (
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

// selectorCacheCap bounds the compiled-selector cache one runtime keeps. Page
// code that builds query strings in a loop must not turn the cache into a
// leak; past the cap the whole cache is dropped rather than grown, which costs
// one re-parse and never a wrong answer.
const selectorCacheCap = 256

// selectorPlan is one query string compiled once: selectors is the parsed
// list, and the remaining fields are the shape the query path needs without
// re-inspecting the parsed selectors per call.
type selectorPlan struct {
	raw       string
	selectors []css.Selector
	// syntaxErr is non-empty when raw is not a valid selector list. Chromium
	// raises a SyntaxError instead of returning an empty result, so the
	// message is kept here and thrown at every call.
	syntaxErr string
	// indexKind is the one shape the document indexes can answer: the whole
	// string is a single compound holding a single tag, class, or id
	// condition. 0 = no, 'T' = tag, '.' = class, '#' = id. Anything else -
	// a compound, a combinator, a list, a pseudo-class - declines the fast
	// path, which is the defect this predicate replaces: the old one accepted
	// every lowercase string, so "div > p" went to the tag index, found
	// nothing, and reported success.
	indexKind byte
	indexVal  string
	// usesScope reports whether :scope appears anywhere in the list,
	// including inside functional pseudo-class arguments.
	usesScope bool
}

// compileSelectorPlan parses raw the way the DOM query API requires: invalid
// input is recorded as a SyntaxError message, not repaired into a selector
// that silently matches nothing.
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

// simpleSelectorShape reports whether raw is exactly one compound with one
// tag/class/id condition, the only shape the id/tag/class indexes answer with
// the same result the general walk would produce.
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
		// The parser lowercases a tag name, but the index keys parsed
		// documents by their stored tag. An uppercase spelling must keep
		// walking (where matching is case-insensitive), which is the rule the
		// old isLowerTag predicate was reaching for.
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

// selectorPlanFor returns the compiled plan for raw, parsing it at most once
// per runtime. A runtime serves one document, and the plan is document
// independent: :scope is bound per call (see scopedSelectors), never cached.
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

// selectorArgument reads the method's selector argument, raising the TypeError
// Chromium raises when it is missing. Coercion is String(v), so undefined and
// null become the type selectors "undefined" and "null", which match nothing.
func (r *Runtime) selectorArgument(call goja.FunctionCall, method string, root *dom.Node) string {
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute '" + method + "' on '" +
			selectorContextName(root) + "': 1 argument required, but only 0 present."))
	}
	return call.Arguments[0].String()
}

// throwSelectorSyntaxError raises the exception a browser raises for a
// selector it cannot parse. Returning an empty result instead would read to
// page code as "no match", the wrong answer this API must never give.
func (r *Runtime) throwSelectorSyntaxError(method string, root *dom.Node, p *selectorPlan) {
	panic(namedError(r, "SyntaxError", "Failed to execute '"+method+"' on '"+
		selectorContextName(root)+"': "+p.syntaxErr))
}

// selectorContextName is the interface name Chromium puts in a query
// exception: the receiver's kind, not the selector's.
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

// queryRoot resolves the node a query starts from: the receiver for an
// element- or document-level call, the runtime's document when the receiver
// carries no node.
func (r *Runtime) queryRoot(v goja.Value) *dom.Node {
	if n := r.getNode(v); n != nil {
		return n
	}
	if r.domDoc == nil {
		return nil
	}
	return &r.domDoc.Node
}

// scopeForRoot answers the node :scope stands for in a query rooted at root:
// the root itself for an element, the document element for a document, and
// the root again for a fragment or a document without a document element,
// where no element can equal it and :scope therefore matches nothing.
func scopeForRoot(root *dom.Node) *dom.Node {
	if root == nil || root.Element() {
		return root
	}
	if root.Type == dom.NodeDocument && root.Doc != nil && root.Doc.HTML != nil {
		return root.Doc.HTML
	}
	return root
}

// scopedSelectors binds :scope to the query root when the plan mentions it.
// The cache holds unscoped selectors; only a :scope list pays for the copy,
// and the copy is per call, so a cached plan never carries one document's
// scope into another's query.
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

// matchesAny reports whether n matches one of sels. Selector.MatchesIndexed is
// the cascade's own matcher, so a selector the stylesheet engine applies means
// the same thing here.
func matchesAny(sels []css.Selector, n *dom.Node, ix *css.SiblingIndex) bool {
	for i := range sels {
		if sels[i].MatchesIndexed(n, ix) {
			return true
		}
	}
	return false
}

// querySelectorPlan returns the first descendant of root (in tree order,
// excluding root itself) matching one of the plan's selectors, or nil.
func (r *Runtime) querySelectorPlan(root *dom.Node, p *selectorPlan) *dom.Node {
	if root == nil {
		return nil
	}
	if node, ok := r.querySelectorIndexed(root, p); ok {
		return node
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

// firstSelectorMatch walks n's subtree in tree order and returns the first
// element matching sels.
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

// querySelectorAllPlan returns every descendant of root matching one of the
// plan's selectors. One tree-order walk yields document order and
// de-duplication together: a list never reports the same element twice, and no
// selector-list ordering can reorder the result.
func (r *Runtime) querySelectorAllPlan(root *dom.Node, p *selectorPlan) []*dom.Node {
	if root == nil {
		return nil
	}
	if nodes, ok := r.querySelectorAllIndexed(root, p); ok {
		return nodes
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

// matchesSelector implements Element.matches: the element itself against the
// selector list, never its descendants or ancestors. :scope is the element.
func (r *Runtime) matchesSelector(n *dom.Node, p *selectorPlan) bool {
	if n == nil || !n.Element() {
		return false
	}
	return matchesAny(scopedSelectors(p, n), n, css.NewSiblingIndex())
}

// closestSelector implements Element.closest: self first, then each ancestor
// element, stopping at the first match. :scope stays the element the call was
// made on, so an ancestor never satisfies :scope.
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

// jsQuerySelector implements document.querySelector(selector) and
// element.querySelector(selector).
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

// jsQuerySelectorAll implements document.querySelectorAll(selector) and
// element.querySelectorAll(selector).
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

// jsMatches implements element.matches(selector).
func (r *Runtime) jsMatches(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	raw := r.selectorArgument(call, "matches", node)
	p := r.selectorPlanFor(raw)
	if p.syntaxErr != "" {
		r.throwSelectorSyntaxError("matches", node, p)
	}
	return r.vm.ToValue(r.matchesSelector(node, p))
}

// jsClosest implements element.closest(selector).
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

// querySelectorIndexed answers a single simple selector from the document
// indexes: O(matches) instead of O(subtree). Reports ok==false when the walk
// must still run (any other shape, or an #id resolving outside the subtree
// where a deeper duplicate may still match). Results equal the walk: indexes
// enumerate in document order and containment+exclusion mirror it.
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

// indexDocFor returns the document whose id/tag/class indexes answer a query
// rooted at root.
//
// r.domDoc is the answer for every node of the live tree, which is all a page
// could ask for before this. document.implementation.createHTMLDocument puts a
// second, detached document inside the same runtime, and reading the *live*
// document's index for a query rooted in that foreign tree yields the live
// matches filtered by "is this node a descendant of that root" - always none,
// and reported as `ok`, so the walk never runs. The page cannot tell that
// apart from "the element is not here", which is a wrong answer rather than an
// unsupported one: jQuery's Sizzle support probes and its parseHTML context
// both query a detached document.
func (r *Runtime) indexDocFor(root *dom.Node) *dom.Document {
	if root != nil && root.Doc != nil {
		return root.Doc
	}
	return r.domDoc
}

// containedStrict reports whether n is a proper descendant of root.
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

// isLowerTag reports whether sel is already lowercase ASCII: only then does
// the atom-indexed tag lookup answer exactly what the case-insensitive walk
// would (parsed documents always carry lowercase tags).
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

// querySelectorID answers a bare #id query from the document index: O(depth)
// instead of O(subtree). It reports ok==false when the walk must still run:
// a hit on root itself (the walk never tests root, but a deeper duplicate
// still matches) or a hit outside an element-rooted subtree. A document-wide
// miss answers nil directly: no element carries that id, so the walk could
// only return nil too.
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

// querySelectorAllIndexed answers a single simple selector from the document
// indexes, filtering to the rooted subtree. Same ordering and membership as
// the walk; ok==false falls back to it.
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
