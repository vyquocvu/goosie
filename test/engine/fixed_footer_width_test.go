package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestOutOfFlowWidthSurvivesEveryPass guards a full refresh, which is where an
// out-of-flow box's width is resolved more than once. placeOutOfFlow measures the
// box by handing block layout a width of `resolved width + padding + margins`,
// and a percentage `width: 100%` then resolves against that - so the box picks up
// its own padding each time the document is laid out. A fixed, full-width footer
// is the shape that hits it: theuselessweb's footer grew 48px past the viewport
// and its rows overflowed with it.
func TestOutOfFlowWidthSurvivesEveryPass(t *testing.T) {
	html := `<html><head><style>
		body { margin: 0; }
		.sharing { position: fixed; bottom: 0; left: 0; width: 100%; padding: 12px;
		           display: flex; flex-direction: column; align-items: center; }
		</style></head><body><div class="sharing">
		<div id="row">the useless web</div>
		</div></body></html>`

	s, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatal(err)
	}
	row := s.Doc.ElementByID("row")
	if row == nil {
		t.Fatal("row element missing")
	}
	rid, ok := s.Arena.ForNode(row.ID)
	if !ok {
		t.Fatal("row not laid out")
	}
	footer := s.Arena.Get(s.Arena.Get(rid).Parent)
	item := s.Arena.Get(rid)
	if diff := footer.W - 800; diff > 0.5 || diff < -0.5 {
		t.Errorf("footer W = %v, want 800 (100%% of the viewport; padding sits outside a content box)", footer.W)
	}
	// Measured against Chromium in an 800px viewport: the centred row is its
	// fit-content width - Chromium gives the same markup 108px - never the whole
	// line, and its centre lands on the centre of the footer's content box, which
	// starts 12px of padding in from the viewport edge.
	if item.W >= footer.W {
		t.Errorf("row W = %v, want below the footer's %v (only `align-items: stretch` fills the line)", item.W, footer.W)
	}
	itemCentre := item.X + item.W/2
	footerCentre := footer.X + footer.BorderLeft + footer.PaddingLeft + footer.W/2
	if diff := itemCentre - footerCentre; diff > 0.5 || diff < -0.5 {
		t.Errorf("row centre = %v, want the footer's content-box centre %v", itemCentre, footerCentre)
	}
}
