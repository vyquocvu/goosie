package domtest

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// TestFixtureLimitsTrackEngineCaps is why this package spells the caps as literals rather
// than importing them: internal/engine depends on internal/style, so a fixture helper that
// depends back on engine makes internal/style's own test binary an import cycle. The
// equality is what has to hold, so the equality is what is asserted.
func TestFixtureLimitsTrackEngineCaps(t *testing.T) {
	got := fixtureLimits()
	want := dom.ParseLimits{
		Nodes:          engine.MaxDocumentNodes,
		Depth:          engine.MaxDocumentDepth,
		Attributes:     engine.MaxAttributes,
		AttributeBytes: engine.MaxAttributeBytes,
	}
	if got != want {
		t.Errorf("fixture limits are %+v but the engine caps a loaded document at %+v; fixtures are then not the shape the engine retains", got, want)
	}
}
