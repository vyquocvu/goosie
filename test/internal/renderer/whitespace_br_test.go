package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// layoutLineTexts runs the full parse→style→layout pipeline and returns the
// line-box texts for every box that has inline content, in document order.
func layoutLineTexts(t *testing.T, htmlSrc string, vw, vh float32) [][]string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(htmlSrc))
	if err != nil {
		t.Fatal(err)
	}
	stylesheet := extractAndParseCSS(doc)
	body := findBodyNode(doc)
	renderTree := renderer.BuildRenderTree(body)
	if stylesheet != nil && len(stylesheet.Rules) > 0 {
		sm := renderer.NewStyleManagerWithViewport(stylesheet, vw, vh)
		sm.ApplyStyles(renderTree)
	}
	le := renderer.NewLayoutEngine(vw, vh)
	root := le.ComputeLayout(renderTree)

	var out [][]string
	var walk func(b *renderer.LayoutBox)
	walk = func(b *renderer.LayoutBox) {
		if len(b.LineBoxes) > 0 {
			texts := make([]string, 0, len(b.LineBoxes))
			for _, lb := range b.LineBoxes {
				var sb strings.Builder
				for _, ib := range lb.InlineBoxes {
					sb.WriteString(ib.Text)
				}
				texts = append(texts, sb.String())
			}
			out = append(out, texts)
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

func TestBRCreatesLineBreaks(t *testing.T) {
	lines := layoutLineTexts(t, `<!DOCTYPE html><html><body>
		<p>first line<br>second line<br>third line</p>
		</body></html>`, 800, 400)

	if len(lines) == 0 {
		t.Fatal("no inline content found")
	}
	p := lines[0]
	if len(p) != 3 {
		t.Fatalf("expected 3 lines for a<br>b<br>c, got %d: %q", len(p), p)
	}
	if strings.TrimSpace(p[0]) != "first line" || strings.TrimSpace(p[1]) != "second line" || strings.TrimSpace(p[2]) != "third line" {
		t.Fatalf("unexpected line contents: %q", p)
	}
}

func TestConsecutiveBRsCreateBlankLine(t *testing.T) {
	lines := layoutLineTexts(t, `<!DOCTYPE html><html><body>
		<p>one<br><br>two</p>
		</body></html>`, 800, 400)

	if len(lines) == 0 {
		t.Fatal("no inline content found")
	}
	p := lines[0]
	if len(p) != 3 {
		t.Fatalf("expected 3 line boxes (one, blank, two), got %d: %q", len(p), p)
	}
	if strings.TrimSpace(p[1]) != "" {
		t.Fatalf("middle line should be blank, got %q", p[1])
	}
}

func TestWhiteSpaceNoWrapCSS(t *testing.T) {
	lines := layoutLineTexts(t, `<!DOCTYPE html><html><head><style>
		p { white-space: nowrap; }
		</style></head><body>
		<p>`+strings.Repeat("word ", 60)+`</p>
		</body></html>`, 300, 400)

	if len(lines) == 0 {
		t.Fatal("no inline content found")
	}
	if len(lines[0]) != 1 {
		t.Fatalf("white-space: nowrap must produce exactly one line, got %d", len(lines[0]))
	}
}

func TestWhiteSpacePreCSSHonorsNewlines(t *testing.T) {
	lines := layoutLineTexts(t, `<!DOCTYPE html><html><head><style>
		div { white-space: pre; }
		</style></head><body>
		<div>alpha
beta</div>
		</body></html>`, 800, 400)

	if len(lines) == 0 {
		t.Fatal("no inline content found")
	}
	if len(lines[0]) < 2 {
		t.Fatalf("white-space: pre must break at newlines, got %d line(s): %q", len(lines[0]), lines[0])
	}
}

func TestCodeWrapsByDefault(t *testing.T) {
	// Browsers default <code> to white-space: normal — long code must wrap,
	// not overflow on one line.
	lines := layoutLineTexts(t, `<!DOCTYPE html><html><body>
		<code>`+strings.Repeat("abcdefgh ", 60)+`</code>
		</body></html>`, 300, 400)

	if len(lines) == 0 {
		t.Fatal("no inline content found")
	}
	if len(lines[0]) < 2 {
		t.Fatalf("<code> should wrap by default, got %d line(s)", len(lines[0]))
	}
}

func TestWhiteSpaceInherits(t *testing.T) {
	// white-space is inherited: nowrap on the container applies to the span.
	lines := layoutLineTexts(t, `<!DOCTYPE html><html><head><style>
		div { white-space: nowrap; }
		</style></head><body>
		<div><span>`+strings.Repeat("word ", 60)+`</span></div>
		</body></html>`, 300, 400)

	if len(lines) == 0 {
		t.Fatal("no inline content found")
	}
	if len(lines[0]) != 1 {
		t.Fatalf("inherited nowrap must keep one line, got %d", len(lines[0]))
	}
}
