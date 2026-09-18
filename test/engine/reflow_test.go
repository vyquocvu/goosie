package engine_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

type fixedMetrics struct{ advance int32 }

func (m *fixedMetrics) GlyphAdvance(_ int32, _ rune) int32 { return m.advance }

func reflow(t *testing.T, s *engine.Session, w float32) error {
	t.Helper()
	r, ok := any(s).(interface{ Reflow(float32) error })
	if !ok {
		t.Fatal("Session is missing layout-only Reflow")
	}
	return r.Reflow(w)
}

func TestReflowRetainsDocumentStylesAndMetrics(t *testing.T) {
	metrics := &fixedMetrics{advance: 7}
	s, err := engine.NewSession(`<html><head><style>p {font-size:23px; color:red}</style></head><body><p id="p">WWWW</p></body></html>`, nil, 400, engine.WithMetrics(metrics))
	if err != nil {
		t.Fatal(err)
	}
	doc, styles, arena := s.Doc, s.Styles, s.Arena
	st := styles[doc.ElementByID("p").ID]
	for _, w := range []float32{200, 600, 400} {
		if err := reflow(t, s, w); err != nil {
			t.Fatal(err)
		}
		if s.Doc != doc || reflect.ValueOf(s.Styles).Pointer() != reflect.ValueOf(styles).Pointer() || s.Styles[doc.ElementByID("p").ID] != st {
			t.Fatal("reflow replaced retained document/styles")
		}
		if s.Arena == arena || s.Arena.Metrics != metrics {
			t.Fatal("arena not replaced or metrics lost")
		}
		id, _ := s.Arena.ForNode(doc.ElementByID("p").ID)
		if got := s.Arena.Get(id).W; got != w-16 {
			t.Fatalf("resized width = %v", got)
		}
		found := false
		for _, o := range s.Arena.Objects {
			if o.Node != nil && o.Node.DataContent == "WWWW" {
				found = true
				if o.W != 28 || o.Style.FontSize != 23 {
					t.Fatalf("lost metrics/embedded style: %+v", o)
				}
			}
		}
		if !found {
			t.Fatal("missing text")
		}
	}
}

func TestReflowFailurePreservesArena(t *testing.T) {
	m := &fixedMetrics{advance: 7}
	s, err := engine.NewSession(`<p>WWWW</p>`, nil, 400, engine.WithMetrics(m))
	if err != nil {
		t.Fatal(err)
	}
	old := s.Arena
	before := append([]layout.Object(nil), old.Objects...)
	// Valid viewport, but the candidate geometry exceeds the safe range.
	m.advance = 1 << 29
	if err := reflow(t, s, 200); err == nil {
		t.Fatal("accepted unsafe candidate geometry")
	}
	if s.Arena != old || !reflect.DeepEqual(before, old.Objects) {
		t.Fatal("failed candidate changed old arena")
	}
	for _, w := range []float32{0, -1, float32(math.NaN()), float32(math.Inf(1)), math.MaxFloat32} {
		if err := reflow(t, s, w); err == nil {
			t.Fatalf("accepted %v", w)
		}
		if s.Arena != old {
			t.Fatal("invalid viewport changed arena")
		}
	}
}

func TestPaintCheckedRejectsUnsafeScaleAndArena(t *testing.T) {
	s, err := engine.NewSession(`<p>safe</p>`, nil, 400)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := any(s).(interface {
		PaintChecked(float32) (*paint.List, error)
	})
	if !ok {
		t.Fatal("Session is missing PaintChecked")
	}
	for _, scale := range []float32{0, -1, 9, float32(math.NaN()), float32(math.Inf(1))} {
		if list, err := p.PaintChecked(scale); err == nil || list != nil {
			t.Fatalf("accepted scale %v", scale)
		}
	}
	if _, err := p.PaintChecked(2); err != nil {
		t.Fatal(err)
	}
	s.Arena.Objects[1].X = float32(math.Inf(1))
	if _, err := p.PaintChecked(1); err == nil {
		t.Fatal("accepted unsafe geometry")
	}
}
