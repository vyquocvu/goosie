package dom

import (
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
	// NSXML marks attributes in the XML namespace, which only
	// setAttributeNS can create: the HTML parser never puts xml:lang there,
	// and :lang() only honors the namespaced form.
	NSXML
)

// Attribute is one name-value pair on an element.
type Attribute struct {
	Namespace Namespace
	Name      string
	NameAtom  AttrAtom
	Value     string
}

// Node is one node in the DOM tree. Elements, text, comments, and the document
// itself are all nodes; NodeType selects which fields are meaningful.
type Node struct {
	ID        NodeID
	Type      NodeType
	Namespace Namespace

	// Element fields.
	Data     string
	DataAtom Atom
	Attr     []Attribute

	// Text / comment / doctype fields.
	DataContent string

	// Tree pointers.
	Parent      *Node
	FirstChild  *Node
	LastChild   *Node
	NextSibling *Node
	PrevSibling *Node

	// Shadow DOM.
	ShadowRoot *ShadowRoot

	// Dialog element state.
	IsModal     bool
	ReturnValue string

	// Popover element state.
	IsPopoverOpen bool

	// CSS Scoping state.
	IsScopeRoot bool

	// Document back-pointer.
	Doc *Document
}

// ShadowRoot is the root of a shadow tree attached to a host element.
type ShadowRoot struct {
	Host       *Node
	FirstChild *Node
	LastChild  *Node
	Mode       string // "open" or "closed"
}

// Element reports whether the node is an element.
func (n *Node) Element() bool { return n.Type == NodeElement }

// Text reports whether the node is a text node.
func (n *Node) Text() bool { return n.Type == NodeText }

// DocumentFragment reports whether the node is a document fragment.
func (n *Node) DocumentFragment() bool { return n.Type == NodeDocumentFragment }

// CloneNode returns a duplicate of node n. If deep is true, descendants are also recursively cloned.
func (n *Node) CloneNode(deep bool) *Node {
	if n == nil {
		return nil
	}
	clone := &Node{
		Type:        n.Type,
		Namespace:   n.Namespace,
		Data:        n.Data,
		DataAtom:    n.DataAtom,
		DataContent: n.DataContent,
		Doc:         n.Doc,
	}
	if n.Doc != nil {
		clone.ID = n.Doc.allocID()
	}
	if len(n.Attr) > 0 {
		clone.Attr = make([]Attribute, len(n.Attr))
		copy(clone.Attr, n.Attr)
	}
	if deep {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			childClone := c.CloneNode(true)
			clone.AppendChild(childClone)
		}
	}
	return clone
}

// TemplateContent returns the document fragment holding the template's cloned children.
func (n *Node) TemplateContent() *Node {
	if n == nil || n.DataAtom != AtomTemplate {
		return nil
	}
	frag := &Node{
		Type: NodeDocumentFragment,
		Doc:  n.Doc,
	}
	if n.Doc != nil {
		frag.ID = n.Doc.allocID()
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		frag.AppendChild(c.CloneNode(true))
	}
	return frag
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
	var walk func(curr *Node)
	walk = func(curr *Node) {
		if curr.Doc != targetDoc || curr.ID == 0 {
			curr.Doc = targetDoc
			curr.ID = targetDoc.allocID()
		}
		for k := curr.FirstChild; k != nil; k = k.NextSibling {
			walk(k)
		}
		if curr.ShadowRoot != nil {
			for k := curr.ShadowRoot.FirstChild; k != nil; k = k.NextSibling {
				walk(k)
			}
		}
	}
	walk(n)
}

// AppendChild adds c as the last child of n.
func (n *Node) AppendChild(c *Node) {
	if c.Parent != nil {
		c.Parent.RemoveChild(c)
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
}

// RemoveChild removes c from n's children.
func (n *Node) RemoveChild(c *Node) {
	if c.Parent != n {
		return
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
		newChild.Parent.RemoveChild(newChild)
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
	for o := other; o != nil; o = o.Parent {
		if o == n {
			return true
		}
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
	for cur := n; cur != nil; cur = cur.Parent {
		if cur.Type == NodeDocument {
			return true
		}
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

// isQueryAttr reports whether an attribute write can retarget the id,
// class, or tag query caches.
func isQueryAttr(name string) bool {
	return name == "id" || name == "class" ||
		strings.EqualFold(name, "id") || strings.EqualFold(name, "class")
}

// SetAttribute sets the named attribute, replacing an existing value
// case-insensitively or appending a new pair.
func (n *Node) SetAttribute(name, value string) {
	for i := range n.Attr {
		if n.Attr[i].Name == name || strings.EqualFold(n.Attr[i].Name, name) {
			n.Attr[i].Value = value
			// Only id/class writes can retarget the query caches; anything
			// else leaves them valid.
			if n.Doc != nil && isQueryAttr(name) {
				n.Doc.MarkQueryDirty()
			}
			n.checkExclusiveDetails(name)
			return
		}
	}
	n.Attr = append(n.Attr, Attribute{Name: name, Value: value})
	if n.Doc != nil && isQueryAttr(name) {
		n.Doc.MarkQueryDirty()
	}
	n.checkExclusiveDetails(name)
}

func (n *Node) checkExclusiveDetails(attrName string) {
	if n.Element() && n.Data == "details" && strings.EqualFold(attrName, "open") {
		if nameAttr := n.GetAttribute("name"); nameAttr != "" && n.Doc != nil {
			n.Doc.Node.Walk(func(other *Node) bool {
				if other != n && other.Element() && other.Data == "details" && other.GetAttribute("name") == nameAttr {
					if other.HasAttribute("open") {
						other.RemoveAttribute("open")
						DispatchEvent(other, NewEvent("toggle", false, false))
					}
				}
				return true
			})
		}
	}
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
	for i := range n.Attr {
		if n.Attr[i].Name == name || strings.EqualFold(n.Attr[i].Name, name) {
			n.Attr = append(n.Attr[:i], n.Attr[i+1:]...)
			if n.Doc != nil && isQueryAttr(name) {
				n.Doc.MarkQueryDirty()
			}
			return
		}
	}
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
	if n.Type == NodeText {
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
	Node    Node
	nextID  NodeID
	HTML    *Node
	Head    *Node
	Body    *Node
	DocType *Node
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
	doc := &Document{}
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

// NewElement creates a new element node with the given tag name.
func (d *Document) NewElement(tagName string) *Node {
	n := &Node{
		ID:       d.allocID(),
		Type:     NodeElement,
		Data:     strings.ToLower(tagName),
		DataAtom: Lookup(tagName),
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
			val := n.GetAttribute("value")
			if val == "" {
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

// DatalistOptions returns the list options if n is an input with a valid list attribute.
func (n *Node) DatalistOptions() []string {
	if n == nil || n.Doc == nil {
		return nil
	}
	return n.Doc.DatalistOptions(n)
}

// ElementsByTagName returns all elements with the given tag name.
func (d *Document) ElementsByTagName(tag string) []*Node {
	atom := Lookup(tag)
	if !d.queryIntact() {
		var result []*Node
		collectByTag(&d.Node, tag, atom, &result)
		return result
	}
	d.ensureQueryIndex()
	var src []*Node
	if atom != AtomUnknown {
		src = d.tagIdx[atom]
	} else {
		src = d.tagStrIdx[tag]
	}
	if len(src) == 0 {
		return nil
	}
	out := make([]*Node, len(src))
	copy(out, src)
	return out
}

func collectByTag(n *Node, tag string, atom Atom, result *[]*Node) {
	if n.Element() {
		if (atom != AtomUnknown && n.DataAtom == atom) || (atom == AtomUnknown && n.Data == tag) {
			*result = append(*result, n)
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectByTag(c, tag, atom, result)
	}
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
