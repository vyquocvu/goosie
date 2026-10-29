package dom

import (
	"strings"

	"golang.org/x/text/unicode/bidi"
)

// Directionality returns the HTML directionality of an element. It follows
// explicit dir attributes and the first-strong character rule used by dir=auto
// and bdi, then inherits through the DOM tree. With no declaration the value is
// ltr, as required by HTML's directionality algorithm.
func (n *Node) Directionality() string {
	for current := n; current != nil; current = current.Parent {
		if !current.Element() {
			continue
		}
		dir := strings.ToLower(strings.TrimSpace(current.GetAttribute("dir")))
		switch dir {
		case "ltr", "rtl":
			return dir
		case "auto":
			return current.AutoDirectionality()
		}
		if current.Data == "bdi" {
			return current.AutoDirectionality()
		}
	}
	return "ltr"
}

// AutoDirectionality computes the direction of a dir=auto element or bdi by
// finding its first strong directional character. Descendant elements with a
// dir attribute establish their own direction and are excluded from the scan.
func (n *Node) AutoDirectionality() string {
	if n == nil || !n.Element() {
		return "ltr"
	}
	if n.Data == "input" || n.Data == "textarea" {
		value := n.InputControlValue()
		if dir := firstStrongDirection(value); dir != "" {
			return dir
		}
		return "ltr"
	}
	if n.Data == "slot" {
		for _, assigned := range n.AssignedNodes() {
			if dir := firstStrongIn(n, assigned); dir != "" {
				return dir
			}
		}
		return "ltr"
	}
	if dir := firstStrongIn(n, n); dir != "" {
		return dir
	}
	return "ltr"
}

func firstStrongIn(root, current *Node) string {
	if current == nil {
		return ""
	}
	if current != root && current.Element() && current.HasAttribute("dir") {
		return ""
	}
	if current != root && current.Element() && current.Data == "bdi" {
		return ""
	}
	if current != root && current.Element() && current.Data == "slot" {
		return current.slotDirectionality()
	}
	if current.Text() {
		return firstStrongDirection(current.DataContent)
	}
	for child := current.FirstChild; child != nil; child = child.NextSibling {
		if dir := firstStrongIn(root, child); dir != "" {
			return dir
		}
	}
	return ""
}

func (n *Node) slotDirectionality() string {
	if n == nil || !n.Element() || n.Data != "slot" {
		return "ltr"
	}
	dir := strings.ToLower(strings.TrimSpace(n.GetAttribute("dir")))
	switch dir {
	case "ltr", "rtl":
		return dir
	case "auto":
		return n.AutoDirectionality()
	}
	if host, _ := n.ShadowHost(); host != nil {
		return host.Directionality()
	}
	return "ltr"
}

// ShadowHost returns the host and root containing a slot element.
func (n *Node) ShadowHost() (*Node, *ShadowRoot) {
	if n == nil || !n.Element() || n.Data != "slot" {
		return nil, nil
	}
	for p := n.Parent; p != nil; p = p.Parent {
		if p.ShadowRoot != nil && containsShadowNode(p.ShadowRoot.FirstChild, n) {
			return p, p.ShadowRoot
		}
	}
	return nil, nil
}

func containsShadowNode(root, target *Node) bool {
	for n := root; n != nil; n = n.NextSibling {
		if n == target || containsShadowNode(n.FirstChild, target) {
			return true
		}
	}
	return false
}

// AssignedNodes returns the slotables assigned to a slot using the host's
// named or manual slot assignment mode. It does not return fallback children.
func (n *Node) AssignedNodes() []*Node {
	host, shadow := n.ShadowHost()
	if host == nil || shadow == nil {
		return nil
	}
	if shadow.SlotAssignment == "manual" {
		return shadow.ManualSlots[n]
	}
	name := n.GetAttribute("name")
	if first := findSlotByName(shadow.FirstChild, name); first != n {
		return nil
	}
	var assigned []*Node
	for child := host.FirstChild; child != nil; child = child.NextSibling {
		childName := ""
		if child.Element() {
			childName = child.GetAttribute("slot")
		}
		if childName == name {
			assigned = append(assigned, child)
		}
	}
	return assigned
}

// AssignedSlot returns the slot that receives an element or text node through
// named or manual slot assignment.
func (n *Node) AssignedSlot() *Node {
	if n == nil || n.Parent == nil {
		return nil
	}
	host := n.Parent
	shadow := host.ShadowRoot
	if shadow == nil {
		return nil
	}
	var found *Node
	var walk func(*Node)
	walk = func(current *Node) {
		for slot := current; slot != nil && found == nil; slot = slot.NextSibling {
			if slot.Element() && slot.Data == "slot" {
				for _, assigned := range slot.AssignedNodes() {
					if assigned == n {
						found = slot
						return
					}
				}
			}
			walk(slot.FirstChild)
		}
	}
	walk(shadow.FirstChild)
	return found
}

func findSlotByName(root *Node, name string) *Node {
	for n := root; n != nil; n = n.NextSibling {
		if n.Element() && n.Data == "slot" && n.GetAttribute("name") == name {
			return n
		}
		if slot := findSlotByName(n.FirstChild, name); slot != nil {
			return slot
		}
	}
	return nil
}

func firstStrongDirection(text string) string {
	for _, r := range text {
		properties, _ := bidi.LookupRune(r)
		class := properties.Class()
		switch class {
		case bidi.L:
			return "ltr"
		case bidi.R, bidi.AL:
			return "rtl"
		}
	}
	return ""
}
