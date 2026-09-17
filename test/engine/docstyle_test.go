package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

// TestDocumentStyleApplied verifies that <style> blocks inside the document
// reach the style resolver: a background-color rule must produce a fill
// command in the display list. Without this, the -url path renders documents
// with no author colors at all.
func TestDocumentStyleApplied(t *testing.T) {
	sess, err := engine.NewSession(`<html><head>
<style>
div { background-color: #336699; }
</style>
</head><body style="margin: 0;">
<div style="width: 50px; height: 50px;">x</div>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	list := sess.Paint(1)
	dl := list.Build(1)
	fills := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color.R() == 0x33 && c.Color.G() == 0x66 && c.Color.B() == 0x99 {
			fills++
		}
	}
	if fills == 0 {
		t.Fatal("no fill command with the stylesheet color found in display list")
	}
}
