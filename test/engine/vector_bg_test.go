package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// TestVectorBackgroundRasterizesAtTile pins lazy vector backgrounds: an SVG
// without intrinsic dimensions rasterizes at its laid-out tile (not at a
// guessed viewport) and attaches flagged, so paint negotiates the same tile.
func TestVectorBackgroundRasterizesAtTile(t *testing.T) {
	svg := `%3Csvg xmlns='http://www.w3.org/2000/svg' height='50%25'%3E%3Crect y='0' width='100%25' height='50%25' fill='lime'/%3E%3C/svg%3E`
	html := `<html><head><style>div{background-image: url('data:image/svg+xml,` + svg + `'); background-repeat: no-repeat; background-size: auto; height: 768px; width: 256px;}</style></head><body style="margin:0;"><div>x</div></body></html>`
	s, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i := range s.Arena.Objects {
		o := &s.Arena.Objects[i]
		if o.Node != nil && o.Node.Data == "div" {
			found = true
			if o.BgImage == nil {
				t.Fatal("vector background not attached")
			}
			if !o.BgVector {
				t.Error("vector background attached without the vector flag")
			}
		}
	}
	if !found {
		t.Fatal("div not laid out")
	}
}
