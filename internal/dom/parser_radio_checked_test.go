package dom

import "testing"

func TestParserCallbacksDoNotLoseCheckedAttribute(t *testing.T) {
	limits := ParseLimits{Nodes: 100, Depth: 10, Attributes: 10, AttributeBytes: 100}
	doc, err := ParseBoundedWithParserCallbacks(
		`<form><input type="radio" name="group" required checked></form>`,
		limits,
		nil,
		func(node *Node, attr Attribute) {
			// Runtime attribute callbacks can inspect checkedness while the
			// parser is still processing this start tag. Reading it for the
			// earlier type attribute must not mask the later checked attribute.
			if node.Data == "input" && attr.Name == "type" {
				_ = node.InputChecked()
			}
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var input *Node
	doc.Node.Walk(func(node *Node) bool {
		if node.Data == "input" {
			input = node
			return false
		}
		return true
	})
	if input == nil {
		t.Fatal("parser did not create the radio input")
	}
	if !input.InputChecked() {
		t.Fatal("parser callback masked the checked content attribute from live checkedness")
	}
}
