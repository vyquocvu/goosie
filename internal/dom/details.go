package dom

// DetailsTreeScope returns a walker and the root of the exclusive tree scope
// containing node. Shadow trees are separate scopes even though this DOM stores
// their children with the host as Parent.
func DetailsTreeScope(node *Node) (walk func(*Node, func(*Node)), root *Node) {
	if node == nil {
		return nil, nil
	}
	var scope *ShadowRoot
	for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
		if sr := ancestor.ShadowRoot; sr != nil && shadowTreeContains(sr.FirstChild, node) {
			scope = sr
			break
		}
	}
	if node.Doc != nil && scope == nil {
		var find func(*Node) bool
		find = func(current *Node) bool {
			if current == nil {
				return false
			}
			if sr := current.ShadowRoot; sr != nil && shadowTreeContains(sr.FirstChild, node) {
				scope = sr
				return true
			}
			for child := current.FirstChild; child != nil; child = child.NextSibling {
				if find(child) {
					return true
				}
			}
			if sr := current.ShadowRoot; sr != nil {
				for child := sr.FirstChild; child != nil; child = child.NextSibling {
					if find(child) {
						return true
					}
				}
			}
			return false
		}
		find(&node.Doc.Node)
	}
	walk = func(root *Node, visit func(*Node)) {
		var visitTree func(*Node)
		visitTree = func(current *Node) {
			for item := current; item != nil; item = item.NextSibling {
				visit(item)
				visitTree(item.FirstChild)
			}
		}
		if root != nil && root.Element() && root.Parent == nil {
			visit(root)
			visitTree(root.FirstChild)
			return
		}
		visitTree(root)
	}
	if scope != nil {
		return walk, scope.FirstChild
	}
	root = node
	for root.Parent != nil {
		root = root.Parent
	}
	return walk, root
}

func shadowTreeContains(root, target *Node) bool {
	for item := root; item != nil; item = item.NextSibling {
		if item == target || shadowTreeContains(item.FirstChild, target) {
			return true
		}
	}
	return false
}

// CloseOtherNamedDetails applies the details name-group exclusivity rule for
// node: every open details element in node's tree scope that shares node's
// non-empty name is closed. It returns the peers it closed, in tree order, so
// the caller can dispatch their toggle transitions. It changes tree state only;
// event semantics belong to the caller.
func CloseOtherNamedDetails(node *Node) []*Node {
	if node == nil || !node.Element() || node.Data != "details" {
		return nil
	}
	name := node.GetAttribute("name")
	if name == "" {
		return nil
	}
	walk, root := DetailsTreeScope(node)
	if walk == nil || root == nil {
		return nil
	}
	var closed []*Node
	walk(root, func(other *Node) {
		if other == node || !other.Element() || other.Data != "details" ||
			!other.HasAttribute("open") || other.GetAttribute("name") != name {
			return
		}
		other.RemoveAttribute("open")
		closed = append(closed, other)
	})
	return closed
}
