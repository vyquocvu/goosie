package engine_test

// Regression tests for superlinear algorithms that used to hang the browser.
// They assert CORRECTNESS on adversarial shapes (which implicitly bounds
// runtime through the suite timeout); no durations are asserted.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/style"
)

func advDoc(t *testing.T, html string) *dom.Document {
	t.Helper()
	doc, err := dom.ParseBounded(html, dom.ParseLimits{
		Nodes: 100000, Depth: 256, Attributes: 64, AttributeBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestWideNthChildStripes pins the sibling-index path: 4000 siblings under
// one parent with li:nth-child(2n) used to cost O(siblings²) (~50ms and ~8s
// extrapolated at the node limit); exactly the even items must win.
func TestWideNthChildStripes(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("<html><body><ul>")
	for i := 0; i < 4000; i++ {
		fmt.Fprintf(&sb, "<li>item %d</li>", i)
	}
	sb.WriteString("</ul></body></html>")
	doc := advDoc(t, sb.String())
	sheets := []*css.Stylesheet{css.ParseForViewport("li:nth-child(2n){color:red}", 1440)}
	styles := style.ResolveViewport(doc, sheets, style.Viewport{W: 1440, H: 900}, nil)
	// Count element styles only: text nodes inherit (and share) their li's
	// color, so counting the whole map would double-count.
	red, total := 0, 0
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n.Element() && n.Data == "li" {
			total++
			if st := styles[n.ID]; st != nil &&
				st.Color.R == 255 && st.Color.G == 0 && st.Color.B == 0 && st.Color.A == 255 {
				red++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&doc.Node)
	if total != 4000 || red != 2000 {
		t.Fatalf("li total = %d (want 4000), red = %d (want 2000)", total, red)
	}
}

// TestManyQuerySelectorAllPins the wrapper cache: 5000 hits used to compile
// accessors per hit (seconds); the result must still list every node once.
// qc:BR-164
func TestManyQuerySelectorAll(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("<html><body><ul>")
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&sb, "<li>item %d</li>", i)
	}
	sb.WriteString("</ul></body></html>")
	doc := advDoc(t, sb.String())
	// Race instrumentation slows wrapper construction beyond the production
	// default even though this case's contract is result correctness, not a
	// five-second performance budget. Keep the larger bound scoped to this test.
	r, err := js.New(js.Options{Timeout: qsaTimeout, URL: "https://example.test/", DOM: doc})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Run(`var out = "" + document.querySelectorAll("li").length;`, "inline"); err != nil {
		t.Fatal(err)
	}
	v, ok := r.Global("out")
	if !ok || v.Str != "5000" {
		t.Fatalf("qsa length = %+v, want 5000", v)
	}
	// Identity across calls (one wrapper per node).
	if err := r.Run(`var out = (document.querySelectorAll("li")[0] === document.querySelectorAll("li")[0]) ? "same" : "diff";`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "same" {
		t.Fatalf("wrapper identity = %+v, want same", v)
	}
}

// TestPumpJSNoopRAFDoesNotRepaint guards against a common animation pattern:
// a callback continually schedules another RAF but does not change output.
func TestPumpJSNoopRAFDoesNotRepaint(t *testing.T) {
	html := `<html><body><canvas id="c" width="10" height="10"></canvas></body></html>`
	sess, rt, _ := pumpSession(t, html, true)
	if err := rt.Run(`function tick(){requestAnimationFrame(tick);} requestAnimationFrame(tick);`, "inline"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if sess.PumpJS(800) {
			t.Fatalf("PumpJS reported repaint work for no-op RAF turn %d", i+1)
		}
	}
}
