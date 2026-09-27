package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// seqRegistry hands out 1-based indices in registration order, so a test can
// name the face a document should resolve to by the order its @font-face rules
// appear in.
type seqRegistry struct{ n uint16 }

func (r *seqRegistry) Register([]byte) (uint16, error) { r.n++; return r.n, nil }

func faceSession(t *testing.T, html, css string) *engine.Session {
	t.Helper()
	fetch := func(base, url string) ([]byte, error) { return []byte("font bytes"), nil }
	s, err := engine.NewSession(html, []string{css}, 800,
		engine.WithCustomFontLoading("https://example.com/", fetch, &seqRegistry{}))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func slotIdx(t *testing.T, s *engine.Session, id string) uint16 {
	t.Helper()
	st := s.Styles[s.Doc.ElementByID(id).ID]
	if st == nil {
		t.Fatalf("no computed style for %q", id)
	}
	return st.FontSlot().CustomIdx
}

// TestCustomFontVariantSelection pins the weight and slant contract against a
// real loaded face set: a bold run takes the bold file, and a run that asks for
// a variant the document never declares falls back to that family's regular
// face rather than to another family or to the built-in fonts.
func TestCustomFontVariantSelection(t *testing.T) {
	html := `<html><body>
		<p id="reg">a</p>
		<p id="bold" style="font-weight: bold">b</p>
		<p id="ital" style="font-style: italic">c</p>
		<p id="heavy" style="font-weight: 900">d</p>
	</body></html>`
	// @font-face only declares a face; the paragraphs ask for it by name.
	use := `p { font-family: Face; }`
	withVariants := use + `
@font-face { font-family: Face; src: url(/r.ttf); }
@font-face { font-family: Face; font-weight: bold; src: url(/b.ttf); }
@font-face { font-family: Face; font-style: italic; src: url(/i.ttf); }`

	s := faceSession(t, html, withVariants)
	for _, tc := range []struct {
		id   string
		want uint16
	}{
		{"reg", 1}, {"bold", 2}, {"ital", 3}, {"heavy", 2},
	} {
		if got := slotIdx(t, s, tc.id); got != tc.want {
			t.Errorf("%s resolved to face %d, want %d", tc.id, got, tc.want)
		}
	}

	// Only a regular face exists: every variant request must land on it.
	regularOnly := use + `
@font-face { font-family: Face; src: url(/r.ttf); }`
	s2 := faceSession(t, html, regularOnly)
	for _, tc := range []struct {
		id   string
		want uint16
	}{
		{"reg", 1}, {"bold", 1}, {"ital", 1}, {"heavy", 1},
	} {
		if got := slotIdx(t, s2, tc.id); got != tc.want {
			t.Errorf("regular-only document: %s resolved to face %d, want %d", tc.id, got, tc.want)
		}
	}

	// A family the document never declares draws with the built-in fonts.
	undeclared := faceSession(t, `<html><body><p id="reg" style="font-family: Nope">a</p></body></html>`, regularOnly)
	if got := slotIdx(t, undeclared, "reg"); got != 0 {
		t.Errorf("undeclared family resolved to face %d, want 0", got)
	}
}
