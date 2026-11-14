package domtest

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

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
