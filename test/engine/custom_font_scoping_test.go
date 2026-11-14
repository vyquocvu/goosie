package engine_test

import (
	"errors"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
)

type fakeRegistry struct{ n uint16 }

func (r *fakeRegistry) Register([]byte) (uint16, error) {
	r.n++
	return r.n, nil
}

func fontFaceSession(t *testing.T, family string, reg engine.FontRegistry) *engine.Session {
	t.Helper()
	html := `<html><body><p id="p" style="font-family: '` + family + `'">hi</p></body></html>`
	css := `@font-face { font-family: '` + family + `'; src: url(/face.ttf); }`
	fetch := func(base, url string) (engine.FontResult, error) {
		return engine.FontResult{Data: []byte("font bytes")}, nil
	}
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

// qc:BR-083
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
	if got := customIdx(t, a); got != 1 {
		t.Errorf("after document B registered its own @font-face set, document A resolves AlphaFace to index %d: "+
			"the family table is process-global, so B's registrations replaced A's", got)
	}
}

type failingRegistry struct{}

func (failingRegistry) Register([]byte) (uint16, error) {
	return 0, errors.New("corrupt face")
}

// qc:BR-083
func TestFontFailureKeepsFallbackAndPageVisible(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fetch engine.FontFetcher
		reg   engine.FontRegistry
	}{
		{
			name: "fetch_error",
			fetch: func(string, string) (engine.FontResult, error) {
				return engine.FontResult{}, errors.New("network down")
			},
			reg: &fakeRegistry{},
		},
		{
			name: "corrupt_face",
			fetch: func(string, string) (engine.FontResult, error) {
				return engine.FontResult{Data: []byte("not a font")}, nil
			},
			reg: failingRegistry{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := `<html><body><p id="p" style="font-family: 'BrokenFace'">fallback words</p></body></html>`
			css := `@font-face { font-family: 'BrokenFace'; src: url(/face.ttf); }`
			s, err := engine.NewSession(html, []string{css}, 800,
				engine.WithCustomFontLoading("https://example.com/", tc.fetch, tc.reg))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(s.Close)
			if got := customIdx(t, s); got != 0 {
				t.Fatalf("failed face resolved to custom index %d, want fallback 0", got)
			}
			id, ok := s.Arena.ForNode(s.Doc.ElementByID("p").ID)
			if !ok {
				t.Fatal("paragraph has no layout box")
			}
			if h := s.Arena.Get(id).H; h <= 0 {
				t.Fatalf("paragraph height = %v after the font failure, want a visible line box", h)
			}
			text := 0
			for _, cmd := range s.Paint(1).All() {
				if cmd.Kind == paint.CmdText && len(cmd.Text.Glyphs) > 0 {
					text++
				}
			}
			if text == 0 {
				t.Fatal("font failure left the page with no painted text")
			}
			if err := s.Reflow(800); err != nil {
				t.Fatalf("reflow after font failure: %v", err)
			}
			if id, ok := s.Arena.ForNode(s.Doc.ElementByID("p").ID); !ok || s.Arena.Get(id).H <= 0 {
				t.Fatal("reflow after font failure collapsed the fallback line box")
			}
		})
	}
}
