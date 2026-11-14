package engine_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
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

// qc:BR-164
func TestManyQuerySelectorAll(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("<html><body><ul>")
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&sb, "<li>item %d</li>", i)
	}
	sb.WriteString("</ul></body></html>")
	doc := advDoc(t, sb.String())
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
	if err := r.Run(`var out = (document.querySelectorAll("li")[0] === document.querySelectorAll("li")[0]) ? "same" : "diff";`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "same" {
		t.Fatalf("wrapper identity = %+v, want same", v)
	}
}

// qc:BR-164
func TestStyleWorkBudgetClipsRulesInsteadOfHanging(t *testing.T) {
	const elements = 10000
	const rules = 1000
	selector := strings.Repeat("div ", engine.MaxSelectorSearches) + "p" + strings.Repeat(".a", 40)

	var html strings.Builder
	html.WriteString("<html><body>")
	html.WriteString(strings.Repeat("<div>", engine.MaxSelectorSearches))
	for i := 0; i < elements; i++ {
		html.WriteString(`<p class="a">x</p>`)
	}
	html.WriteString(strings.Repeat("</div>", engine.MaxSelectorSearches))
	html.WriteString("</body></html>")

	var sheet strings.Builder
	for i := 0; i < rules; i++ {
		color := "red"
		if i == rules-1 {
			color = "blue"
		}
		fmt.Fprintf(&sheet, "%s{color:%s}", selector, color)
	}
	if sheet.Len() > engine.MaxCSSBytes {
		t.Fatalf("fixture is %d bytes, already past the CSS byte cap; it would prove the wrong limit", sheet.Len())
	}

	sess, err := engine.NewSession(html.String(), []string{sheet.String()}, 400)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	var last *dom.Node
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n.Element() && n.Data == "p" {
			last = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(&sess.Doc.Node)
	if last == nil {
		t.Fatal("document built without any p element")
	}
	st := sess.Styles[last.ID]
	if st == nil {
		t.Fatal("last p element has no computed style")
	}
	if st.Color.R != 255 || st.Color.G != 0 || st.Color.B != 0 {
		t.Fatalf("last p color = rgb(%d,%d,%d), want red: the rule past MaxStyleWork was still applied (%d)", st.Color.R, st.Color.G, st.Color.B, engine.MaxStyleWork)
	}
}

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
