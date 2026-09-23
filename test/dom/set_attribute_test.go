package dom_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestSetAttributeNew(t *testing.T) {
	n := &dom.Node{Type: dom.NodeElement, Data: "input"}
	n.SetAttribute("value", "hello")
	if got := n.GetAttribute("value"); got != "hello" {
		t.Fatalf("value = %q, want %q", got, "hello")
	}
}

func TestSetAttributeOverwriteCaseInsensitive(t *testing.T) {
	n := &dom.Node{Type: dom.NodeElement, Data: "input"}
	n.SetAttribute("Value", "one")
	n.SetAttribute("value", "two")
	if got := n.GetAttribute("value"); got != "two" {
		t.Fatalf("value = %q, want %q", got, "two")
	}
	if len(n.Attr) != 1 {
		t.Fatalf("got %d attributes, want 1 (case-insensitive overwrite)", len(n.Attr))
	}
}

func TestSetAttributePreservesOthers(t *testing.T) {
	n := &dom.Node{Type: dom.NodeElement, Data: "input"}
	n.SetAttribute("type", "text")
	n.SetAttribute("maxlength", "5")
	n.SetAttribute("value", "ab")
	if got := n.GetAttribute("type"); got != "text" {
		t.Fatalf("type = %q, want %q", got, "text")
	}
	if got := n.GetAttribute("maxlength"); got != "5" {
		t.Fatalf("maxlength = %q, want %q", got, "5")
	}
	if len(n.Attr) != 3 {
		t.Fatalf("got %d attributes, want 3", len(n.Attr))
	}
}
