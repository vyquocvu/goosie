package layout_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func TestWordBreakBreakAllLayout(t *testing.T) {
	// A narrow container with break-all should break long continuous text into multiple lines
	html := `<!DOCTYPE html>
<html><body style="margin: 0;">
<div style="width: 50px; font-size: 16px; word-break: break-all;">
abcdefghijklmnopqrstuvwxyz
</div>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	objs := sess.Arena.Objects
	textWords := 0
	for i := range objs {
		if objs[i].Node != nil && objs[i].Node.Text() {
			textWords++
		}
	}

	// Under break-all, 26 runes become 26 character tokens instead of 1 long word
	if textWords < 10 {
		t.Errorf("expected many split text word tokens under break-all, got %d", textWords)
	}
}

func TestTextTransformFullWidth(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body style="margin: 0;">
<div id="fw" style="text-transform: full-width;">abc 123!</div>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	foundFullWidth := false
	for i := range sess.Arena.Objects {
		obj := &sess.Arena.Objects[i]
		if obj.Node != nil && obj.Node.Text() {
			// Check for full-width 'ａ' (U+FF41) or '！' (U+FF01)
			for _, r := range obj.Node.DataContent {
				if r == 0xFF41 || r == 0xFF01 {
					foundFullWidth = true
					break
				}
			}
		}
	}
	if !foundFullWidth {
		t.Errorf("expected text-transform: full-width to convert ASCII characters to full-width variants")
	}
}

func TestPositionStickyClamping(t *testing.T) {
	html := `<!DOCTYPE html>
<html><body style="margin: 0;">
<div id="parent" style="position: relative; width: 400px; height: 300px; margin: 0; padding: 0;">
	<div id="sticky" style="position: sticky; top: 50px; height: 100px; width: 200px;">sticky box</div>
</div>
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	foundSticky := false
	for i := range sess.Arena.Objects {
		obj := &sess.Arena.Objects[i]
		if obj.Node != nil && obj.Node.GetAttribute("id") == "sticky" {
			foundSticky = true
			if obj.Y < 50 {
				t.Errorf("expected sticky element Y >= 50, got %f", obj.Y)
			}
			break
		}
	}
	if !foundSticky {
		t.Fatalf("sticky element not found in arena")
	}
}
