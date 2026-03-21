package renderer

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestStyleResolver(t *testing.T) {
	parser := css.NewParser("div { color: red; }")
	stylesheet, _ := parser.Parse()
	resolver := NewStyleResolver(stylesheet)

	node := NewRenderNode(NodeTypeElement)
	node.TagName = "div"

	resolver.Resolve(node)

	if node.Styles["color"] != "red" {
		t.Errorf("expected color red, got %s", node.Styles["color"])
	}
}
