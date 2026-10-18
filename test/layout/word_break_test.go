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
