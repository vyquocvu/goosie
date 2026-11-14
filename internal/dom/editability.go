package dom

func ContentEditableState(n *Node) string {
	if n == nil || !n.Element() || !n.HasAttribute("contenteditable") {
		return "inherit"
	}
	switch asciiLower(n.GetAttribute("contenteditable")) {
	case "", "true":
		return "true"
	case "false":
		return "false"
	case "plaintext-only":
		return "plaintext-only"
	default:
		return "inherit"
	}
}

func IsContentEditable(n *Node) bool {
	if n == nil || !n.Element() {
		return false
	}
	for cur := n; cur != nil; cur = cur.Parent {
		if !cur.Element() || !cur.HasAttribute("contenteditable") {
			continue
		}
		switch asciiLower(cur.GetAttribute("contenteditable")) {
		case "", "true", "plaintext-only":
			return true
		case "false":
			return false
		}
	}
	return n.Doc != nil && n.Doc.DesignMode
}

func asciiLower(value string) string {
	bytes := []byte(value)
	for i, b := range bytes {
		if b >= 'A' && b <= 'Z' {
			bytes[i] = b + ('a' - 'A')
		}
	}
	return string(bytes)
}
