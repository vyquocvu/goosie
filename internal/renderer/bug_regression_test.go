package renderer

import (
	"strings"
	"testing"
)

// TestBugFixDuplicateRendering is a regression test for the bug where HTML content
// was rendered multiple times due to duplicate LayoutBox instances for inline content.
//
// The original issue: when text wrapped across multiple lines, each word got its own
// LayoutBox with the same NodeID, causing the display list to render the text multiple
// times. The fix ensures each text node produces exactly one InlineBox per word/run,
// not one per character.
//
// This test also covers vw/vh viewport-unit resolution: the test HTML uses
// `width:60vw` and `margin:15vh auto`. Before the fix, parseLength returned 0 for
// these units, collapsing the available width to 0 and triggering per-character layout.
func TestBugFixDuplicateRendering(t *testing.T) {
	htmlContent := `<!doctype html>
<html lang="en">
<head>
  <title>Example Domain</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>
    body{background:#eee;width:60vw;margin:15vh auto;font-family:system-ui,sans-serif}
    h1{font-size:1.5em}
    div{opacity:0.8}
    a:link,a:visited{color:#348}
  </style>
<body>
  <div>
    <h1>Example Domain</h1>
    <p>This domain is for use in documentation examples without needing permission. Avoid use in operations.
    <p><a href="https://iana.org/domains/example">Learn more</a>
  </div>
</body>
</html>`

	r := NewRenderer(800, 600)
	r.SetViewport(0, 100000)
	_, err := r.RenderHTML(htmlContent)
	if err != nil {
		t.Fatalf("RenderHTML error: %v", err)
	}

	dl := r.canvasRenderer.cachedDisplayList
	if dl == nil {
		t.Fatal("no display list produced")
	}

	// Collect all PaintText commands
	var textCmds []string
	for _, cmd := range dl.Commands {
		if cmd.Type == PaintText {
			textCmds = append(textCmds, cmd.Text)
		}
	}

	// h1 "Example Domain" must appear as a single run (fits on one line at 800px viewport)
	h1Found := false
	for _, cmd := range dl.Commands {
		if cmd.Type == PaintText && cmd.Text == "Example Domain" {
			h1Found = true
		}
	}
	if !h1Found {
		t.Errorf("expected a single PaintText with text %q; got: %v", "Example Domain", textCmds)
	}

	// "Learn more" link must appear exactly once
	learnMoreCount := 0
	for _, cmd := range dl.Commands {
		if cmd.Type == PaintText && strings.TrimSpace(cmd.Text) == "Learn more" {
			learnMoreCount++
		}
	}
	if learnMoreCount != 1 {
		t.Errorf("expected exactly 1 PaintText for %q, got %d; commands: %v", "Learn more", learnMoreCount, textCmds)
	}

	// No single text node should produce more PaintText commands than the number of
	// wrapped lines (<=10 is generous). Before the fix, a single node produced 100+
	// commands -- one per character -- due to vw/vh units resolving to 0.
	nodeIDCount := make(map[int64]int)
	for _, cmd := range dl.Commands {
		if cmd.Type == PaintText {
			nodeIDCount[cmd.NodeID]++
		}
	}
	for nodeID, count := range nodeIDCount {
		if count > 10 {
			t.Errorf("nodeID %d produced %d PaintText commands -- per-character rendering regression", nodeID, count)
		}
	}

	// Total PaintText commands must be word-level, not character-level (<=10 for this HTML)
	if len(textCmds) > 10 {
		t.Errorf("expected <=10 PaintText commands (word-level), got %d", len(textCmds))
	}
}
