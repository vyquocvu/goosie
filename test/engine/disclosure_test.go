package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

func TestDetailsSummary_ClickToggle(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<body style="margin: 0;">
  <details id="det">
    <summary id="sum">Toggle Details</summary>
    <p id="desc">Hidden body text</p>
  </details>
</body>
</html>`

	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	det := s.Doc.ElementByID("det")
	sum := s.Doc.ElementByID("sum")
	if det == nil || sum == nil {
		t.Fatal("det or sum not found in document")
	}

	// Initial state: details does not have open attribute
	if det.HasAttribute("open") {
		t.Fatal("details should not have open attribute initially")
	}

	toggleFiredCount := 0
	dom.AddEventListener(det, "toggle", func(e *dom.Event) {
		toggleFiredCount++
	}, false)

	// Click summary
	x, y := clickPoint(t, s, "sum")
	handled := s.ClickAt(x, y)
	if !handled {
		t.Fatal("expected ClickAt on summary to return handled=true")
	}

	if !det.HasAttribute("open") {
		t.Fatal("expected details to have open attribute after first click on summary")
	}
	if toggleFiredCount != 1 {
		t.Fatalf("expected 1 toggle event, got %d", toggleFiredCount)
	}

	// Click summary again to collapse
	s.ClickAt(x, y)
	if det.HasAttribute("open") {
		t.Fatal("expected details to NOT have open attribute after second click on summary")
	}
	if toggleFiredCount != 2 {
		t.Fatalf("expected 2 toggle events, got %d", toggleFiredCount)
	}
}
