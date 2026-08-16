package renderer

import (
	"strings"
	"testing"
	"time"
)

// Regression: github.com crashed with a stack overflow through
// minContentSize ↔ widestInlineSegment mutual recursion. A button or
// textarea carrying inline children made widestInlineSegment's
// replaced-element special case call minContentSize, which called straight
// back into widestInlineSegment via the has-inline-content branch.
func TestMinContentSizeNoRecursionOnFormControlsWithText(t *testing.T) {
	html := `<!DOCTYPE html><html><body>
		<div style="display:flex; width:500px">
			<div style="flex-grow:1; flex-basis:0">
				<button>Sign in to GitHub</button>
				<textarea rows="2" cols="12">account text</textarea>
				<p>` + strings.Repeat("long paragraph words here ", 8) + `</p>
			</div>
		</div>
		</body></html>`

	done := make(chan struct{}, 1)
	go func() {
		_, _, err := LayoutHTML(html, 1280, 800)
		if err != nil {
			t.Errorf("LayoutHTML: %v", err)
		}
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("layout did not terminate (minContentSize/widestInlineSegment recursion regression)")
	}
}

// The min-content path itself must terminate for text-bearing controls and
// produce a sane, positive width for a button (its widest word + box).
func TestMinContentSizeButtonTerminates(t *testing.T) {
	tree, _, err := LayoutHTML(`<!DOCTYPE html><html><body><button id="b">Sign in</button></body></html>`, 800, 600)
	if err != nil || tree == nil {
		t.Fatalf("LayoutHTML: %v", err)
	}
	var btn *RenderNode
	var find func(*RenderNode)
	find = func(n *RenderNode) {
		if n == nil || btn != nil {
			return
		}
		if n.TagName == "button" {
			btn = n
			return
		}
		for _, c := range n.Children {
			find(c)
		}
	}
	find(tree)
	if btn == nil {
		t.Fatal("button not found")
	}
	le := NewLayoutEngine(800, 600)
	done := make(chan float32, 1)
	go func() { done <- le.minContentSize(btn) }()
	select {
	case w := <-done:
		if w <= 0 {
			t.Errorf("button min-content width = %v, want > 0", w)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("minContentSize(button) did not terminate")
	}
}
