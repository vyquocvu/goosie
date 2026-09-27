package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// A word with no break opportunity wider than the line is a real shape (a base64
// blob, a long URL) and browsers overflow it. Refusing the whole document for it
// is the bug: one text node denied the page. The laid-out fragments must stay
// inside the guard's finite geometry and must still carry every rune.
func TestUnbreakableWordIsSoftWrapped(t *testing.T) {
	const word = "A"
	const runes = 300000
	const viewportW = 1440.0

	html := "<html><body><p>" + strings.Repeat(word, runes) + "</p></body></html>"
	sess, err := engine.NewSession(html, nil, viewportW)
	if err != nil {
		t.Fatalf("NewSession refused a document that only contains one long word: %v", err)
	}

	var fragments, total int
	for i := 1; i < len(sess.Arena.Objects); i++ {
		o := &sess.Arena.Objects[i]
		if o.Node == nil || o.Node.Type != dom.NodeText {
			continue
		}
		fragments++
		total += len([]rune(o.Node.DataContent))
		if o.W > engine.MaxGeometry {
			t.Errorf("fragment %d is %g CSS pixels wide, past the %d-pixel geometry guard: a word this wide is what used to refuse the document", i, o.W, engine.MaxGeometry)
		}
	}
	if fragments < 2 {
		t.Errorf("%d unbreakable runes became %d fragment: the word was never broken at all", runes, fragments)
	}
	if total != runes {
		t.Errorf("the fragments carry %d of the document's %d runes: breaking the word dropped text", total, runes)
	}
}
