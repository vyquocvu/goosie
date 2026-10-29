package js

import (
	"strings"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
)

func innerTextSetterValue(call goja.FunctionCall) string {
	if len(call.Arguments) == 0 || goja.IsNull(call.Argument(0)) {
		return ""
	}
	return call.Argument(0).String()
}

func innerTextSetterNodes(doc *dom.Document, value string) []*dom.Node {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	children := make([]*dom.Node, 0, len(lines)*2-1)
	for i, line := range lines {
		if line != "" {
			children = append(children, doc.NewText(line))
		}
		if i+1 < len(lines) {
			children = append(children, doc.NewElement("br"))
		}
	}
	return children
}

func (r *Runtime) setInnerText(node *dom.Node, value string) {
	if node == nil || node.Namespace != dom.NSHTML {
		return
	}
	children := innerTextSetterNodes(node.Doc, value)
	old := make([]*dom.Node, 0)
	wasConnected := make(map[*dom.Node]bool)
	parentWasLive := r.nodeIsLive(node)
	r.admitInsert("innerText", node, children, func() {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			old = append(old, child)
			wasConnected[child] = child.IsConnected()
		}
		for _, child := range old {
			node.RemoveChild(child)
		}
		if parentWasLive {
			r.releaseNodes(old)
		}
		for _, child := range children {
			node.AppendChild(child)
		}
	})
	r.markRadioGroupsDirtyForNodes(append(old, children...))
	r.notifyDisconnected(old, wasConnected)
	r.notifyMutation()
	for _, child := range children {
		r.activateDOMInsertedSubtree(child)
	}
}

func (r *Runtime) setOuterText(node *dom.Node, value string) {
	if node == nil || node.Namespace != dom.NSHTML {
		return
	}
	if node.Parent == nil {
		panic(r.domError("NoModificationAllowedError", "The element has no modifiable parent."))
	}
	parent := node.Parent
	previous := node.PrevSibling
	next := node.NextSibling
	replacements := innerTextSetterNodes(node.Doc, value)
	if len(replacements) == 0 {
		// Unlike innerText, outerText replaces the element with a Text node even
		// for the empty string. Adjacent Text nodes are merged below.
		replacements = []*dom.Node{node.Doc.NewText("")}
	}
	fragment := node.Doc.NewDocumentFragment()
	for _, child := range replacements {
		fragment.AppendChild(child)
	}
	r.jsReplaceChild(goja.FunctionCall{
		This:      r.wrapNode(parent),
		Arguments: []goja.Value{r.wrapNode(fragment), r.wrapNode(node)},
	})
	r.mergeOuterTextAdjacentNodes(parent, previous, replacements, next)
}

// mergeOuterTextAdjacentNodes merges only the Text nodes touching the
// replacement sequence. Adjacent text elsewhere in the parent is left intact,
// as required by the outerText setter's replacement algorithm.
func (r *Runtime) mergeOuterTextAdjacentNodes(parent, previous *dom.Node, replacements []*dom.Node, next *dom.Node) {
	if parent == nil || len(replacements) == 0 {
		return
	}
	first := replacements[0]
	last := replacements[len(replacements)-1]
	if first.Type != dom.NodeText || first.Parent != parent {
		first = nil
	}
	if last.Type != dom.NodeText || last.Parent != parent {
		last = nil
	}
	if previous != nil && previous.Parent == parent && previous.Type == dom.NodeText && first != nil {
		r.appendOuterTextNode(previous, first)
		r.removeOuterTextNode(parent, first)
		if len(replacements) == 1 {
			first = previous
			last = previous
		} else if last == first {
			last = previous
		}
	}
	if next != nil && next.Parent == parent && next.Type == dom.NodeText && last != nil {
		r.appendOuterTextNode(last, next)
		r.removeOuterTextNode(parent, next)
	}
}

func (r *Runtime) appendOuterTextNode(target, source *dom.Node) {
	if target == nil || source == nil {
		return
	}
	units := characterDataCodeUnits(source)
	r.replaceCharacterDataUnits(target, len(characterDataCodeUnits(target)), 0, units)
}

func (r *Runtime) removeOuterTextNode(parent, child *dom.Node) {
	if parent == nil || child == nil || child.Parent != parent {
		return
	}
	r.jsRemoveChild(goja.FunctionCall{This: r.wrapNode(parent), Arguments: []goja.Value{r.wrapNode(child)}})
}
