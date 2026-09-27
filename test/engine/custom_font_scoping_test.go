package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// Two documents, two font tables. Each engine Session is given its own
// rasterizer-side FontRegistry, so the 1-based indices it hands out only mean
// anything inside that registry - which is exactly why a family table shared by
// the whole process resolves the wrong face: document B's registrations replace
// the map document A resolves against, and A's declared family stops matching
// even though A's own table still holds it.
type fakeRegistry struct{ n uint16 }

func (r *fakeRegistry) Register([]byte) (uint16, error) {
	r.n++
	return r.n, nil
}

func fontFaceSession(t *testing.T, family string, reg engine.FontRegistry) *engine.Session {
	t.Helper()
	html := `<html><body><p id="p" style="font-family: '` + family + `'">hi</p></body></html>`
	css := `@font-face { font-family: '` + family + `'; src: url(/face.ttf); }`
	fetch := func(base, url string) ([]byte, error) { return []byte("font bytes"), nil }
	s, err := engine.NewSession(html, []string{css}, 800,
		engine.WithCustomFontLoading("https://example.com/", fetch, reg))
	if err != nil {
		t.Fatalf("build session for %q: %v", family, err)
	}
	return s
}

func customIdx(t *testing.T, s *engine.Session) uint16 {
	t.Helper()
	st := s.Styles[s.Doc.ElementByID("p").ID]
	if st == nil {
		t.Fatal("the paragraph has no computed style")
	}
	return st.FontSlot().CustomIdx
}

func TestCustomFontResolutionIsPerSession(t *testing.T) {
	regA, regB := &fakeRegistry{}, &fakeRegistry{}
	a := fontFaceSession(t, "AlphaFace", regA)
	if got := customIdx(t, a); got != 1 {
		t.Fatalf("document A alone resolves AlphaFace to index %d, want 1: the @font-face never reached the cascade", got)
	}
	b := fontFaceSession(t, "BetaFace", regB)
	if got := customIdx(t, b); got != 1 {
		t.Fatalf("document B resolves BetaFace to index %d, want 1", got)
	}
	// B's load is what the tab-switch or navigation is: a second document
	// registering its own faces in its own table. A must keep what it had.
	if got := customIdx(t, a); got != 1 {
		t.Errorf("after document B registered its own @font-face set, document A resolves AlphaFace to index %d: "+
			"the family table is process-global, so B's registrations replaced A's", got)
	}
}
