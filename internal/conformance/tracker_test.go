package conformance

import (
	"os"
	"path/filepath"
	"testing"
)

// TestTrackerIsCurrent regenerates the HTML_CONFORMANCE.md tracker and
// fails when the committed copy is stale, so renderer changes that alter
// element handling must be committed together with a regenerated tracker.
// Regenerate with `make html-audit`.
func TestTrackerIsCurrent(t *testing.T) {
	if os.Getenv("HTML_AUDIT_WRITE") == "true" {
		t.Skip("HTML_AUDIT_WRITE set: this run is generating the tracker")
	}
	trackerPath := filepath.Join("..", "..", "HTML_CONFORMANCE.md")
	want, err := os.ReadFile(trackerPath)
	if err != nil {
		t.Fatalf("HTML_CONFORMANCE.md not found (run `make html-audit`): %v", err)
	}
	got := RenderTracker(AuditAll())
	if string(want) != got {
		t.Errorf("HTML_CONFORMANCE.md is stale — run `make html-audit` and commit the tracker with your renderer change")
	}
}

// TestElementAudit is the per-element reproduction entry point: run with
// -run 'TestElementAudit/<name>' to audit a single element verbosely.
func TestElementAudit(t *testing.T) {
	for _, el := range Elements {
		el := el
		t.Run(el.Name, func(t *testing.T) {
			res := AuditElement(el)
			t.Logf("parsed=%v display=%s (expected %s) textVisible=%v (expected %v) status=%s",
				res.Parsed, res.DisplayClass, el.Display, res.TextVisible, el.RendersText, res.Status())
			if !res.Parsed && el.Audit {
				t.Errorf("element %q is dropped from the render tree", el.Name)
			}
		})
	}
}
