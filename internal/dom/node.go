package dom

import "strings"

// NodeType identifies the kind of node in the DOM tree.
type NodeType uint8

const (
	NodeDocument   NodeType = iota
	NodeElement
	NodeText
	NodeComment
	NodeDoctype
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

	// Document back-pointer.
	Doc *Document
}

// Element reports whether the node is an element.
func (n *Node) Element() bool { return n.Type == NodeElement }

// Text reports whether the node is a text node.
func (n *Node) Text() bool { return n.Type == NodeText }

// AppendChild adds c as the last child of n.
func (n *Node) AppendChild(c *Node) {
	if c.Parent != nil {
		c.Parent.RemoveChild(c)
	}
	c.Parent = n
	c.Doc = n.Doc
	if n.LastChild != nil {
		n.LastChild.NextSibling = c
		c.PrevSibling = n.LastChild
		n.LastChild = c
	} else {
		n.FirstChild = c
		n.LastChild = c
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
}

// InsertBefore inserts newChild before refChild among n's children.
func (n *Node) InsertBefore(newChild, refChild *Node) {
	if refChild == nil {
		n.AppendChild(newChild)
		return
	}
	if newChild.Parent != nil {
		newChild.Parent.RemoveChild(newChild)
	}
	newChild.Parent = n
	newChild.Doc = n.Doc
	newChild.NextSibling = refChild
	newChild.PrevSibling = refChild.PrevSibling
	if refChild.PrevSibling != nil {
		refChild.PrevSibling.NextSibling = newChild
	} else {
		n.FirstChild = newChild
	}
	refChild.PrevSibling = newChild
}

// GetAttribute returns the value of the named attribute, or "" if not present.
func (n *Node) GetAttribute(name string) string {
	lower := strings.ToLower(name)
	for _, a := range n.Attr {
		if strings.ToLower(a.Name) == lower {
			return a.Value
		}
	}
	return ""
}

// HasAttribute reports whether the named attribute is present.
func (n *Node) HasAttribute(name string) bool {
	lower := strings.ToLower(name)
	for _, a := range n.Attr {
		if strings.ToLower(a.Name) == lower {
			return true
		}
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
	if n.Type == NodeText {
		sb.WriteString(n.DataContent)
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		c.collectText(sb)
	}
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

// ElementByID returns the first element with the given ID, or nil.
func (d *Document) ElementByID(id string) *Node {
	return findByID(&d.Node, id)
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

// ElementsByTagName returns all elements with the given tag name.
func (d *Document) ElementsByTagName(tag string) []*Node {
	atom := Lookup(tag)
	var result []*Node
	collectByTag(&d.Node, tag, atom, &result)
	return result
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
