package style_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/style"
)

func TestQuotesParsing(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	q1 := doc.NewElement("q")
	q1.SetAttribute("id", "q1")
	body.AppendChild(q1)

	q2 := doc.NewElement("q")
	q2.SetAttribute("id", "q2")
	body.AppendChild(q2)

	q3 := doc.NewElement("q")
	q3.SetAttribute("id", "q3")
	body.AppendChild(q3)

	sheet := css.Parse(`
		#q1 { quotes: "«" "»" "‹" "›"; }
		#q2 { quotes: none; }
		#q3 { quotes: auto; }
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)

	s1 := styles[q1.ID]
	if len(s1.Quotes) != 4 || s1.Quotes[0] != "«" || s1.Quotes[1] != "»" || s1.Quotes[2] != "‹" || s1.Quotes[3] != "›" {
		t.Fatalf("unexpected quotes for q1: %v", s1.Quotes)
	}

	s2 := styles[q2.ID]
	if len(s2.Quotes) != 0 {
		t.Fatalf("expected empty quotes for q2 (none), got: %v", s2.Quotes)
	}

	s3 := styles[q3.ID]
	if len(s3.Quotes) != 4 || s3.Quotes[0] != "“" || s3.Quotes[1] != "”" {
		t.Fatalf("expected default quotes for q3 (auto), got: %v", s3.Quotes)
	}
}

func TestEvaluateContentQuotes(t *testing.T) {
	partsOpen, ok := style.ParseContentParts("open-quote")
	if !ok || len(partsOpen) != 1 || partsOpen[0].Kind != style.ContentOpenQuote {
		t.Fatalf("failed to parse open-quote: %+v, ok=%v", partsOpen, ok)
	}

	partsClose, ok := style.ParseContentParts("close-quote")
	if !ok || len(partsClose) != 1 || partsClose[0].Kind != style.ContentCloseQuote {
		t.Fatalf("failed to parse close-quote: %+v, ok=%v", partsClose, ok)
	}

	depth := 0
	quotes := []string{"«", "»", "“", "”"}

	// Depth 0: open-quote -> «, depth becomes 1
	got1 := style.EvaluateContentWithQuotes(partsOpen, nil, quotes, &depth)
	if got1 != "«" || depth != 1 {
		t.Fatalf("got1=%q depth=%d, want « and 1", got1, depth)
	}

	// Depth 1: open-quote -> “, depth becomes 2
	got2 := style.EvaluateContentWithQuotes(partsOpen, nil, quotes, &depth)
	if got2 != "“" || depth != 2 {
		t.Fatalf("got2=%q depth=%d, want “ and 2", got2, depth)
	}

	// Depth 2: close-quote -> ”, depth becomes 1
	got3 := style.EvaluateContentWithQuotes(partsClose, nil, quotes, &depth)
	if got3 != "”" || depth != 1 {
		t.Fatalf("got3=%q depth=%d, want ” and 1", got3, depth)
	}

	// Depth 1: close-quote -> », depth becomes 0
	got4 := style.EvaluateContentWithQuotes(partsClose, nil, quotes, &depth)
	if got4 != "»" || depth != 0 {
		t.Fatalf("got4=%q depth=%d, want » and 0", got4, depth)
	}
}

func TestFilterAndBackdropFilter(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)

	div := doc.NewElement("div")
	div.SetAttribute("id", "box")
	body.AppendChild(div)

	sheet := css.Parse(`
		#box {
			filter: blur(5px) grayscale(50%);
			backdrop-filter: blur(10px);
			width: 100px;
			height: 100px;
			background-color: red;
		}
	`)

	styles := style.Resolve(doc, []*css.Stylesheet{sheet}, nil)
	s := styles[div.ID]
	if len(s.Filter) != 2 {
		t.Fatalf("expected 2 filter funcs, got %v", s.Filter)
	}
	if s.Filter[0].Name != "blur" || s.Filter[0].Arg != 5 {
		t.Fatalf("unexpected filter 0: %+v", s.Filter[0])
	}
	if s.Filter[1].Name != "grayscale" || s.Filter[1].Arg != 0.5 {
		t.Fatalf("unexpected filter 1: %+v", s.Filter[1])
	}

	if len(s.BackdropFilter) != 1 {
		t.Fatalf("expected 1 backdrop-filter func, got %v", s.BackdropFilter)
	}
	if s.BackdropFilter[0].Name != "blur" || s.BackdropFilter[0].Arg != 10 {
		t.Fatalf("unexpected backdrop-filter: %+v", s.BackdropFilter[0])
	}

	// Verify paint list emits DisplayCmd with Filter and BackdropFilter set via Session
	htmlStr := `<!DOCTYPE html>
<html>
<head>
<style>
#box {
	filter: blur(5px) grayscale(50%);
	backdrop-filter: blur(10px);
	width: 100px;
	height: 100px;
	background-color: red;
}
</style>
</head>
<body>
<div id="box">Content</div>
</body>
</html>`
	sess, err := engine.NewSession(htmlStr, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}
	list := dl.Build(1)
	if list == nil {
		t.Fatal("expected non-nil display list")
	}

	found := false
	for _, cmd := range list.All() {
		if cmd.Filter != "" || cmd.BackdropFilter != "" {
			found = true
			if cmd.Filter != "blur(5px) grayscale(50%)" && cmd.Filter != "blur(5) grayscale(0.5)" {
				t.Fatalf("unexpected cmd.Filter: %q", cmd.Filter)
			}
			if cmd.BackdropFilter != "blur(10px)" && cmd.BackdropFilter != "blur(10)" {
				t.Fatalf("unexpected cmd.BackdropFilter: %q", cmd.BackdropFilter)
			}
		}
	}
	if !found {
		t.Fatal("expected at least one command with Filter and BackdropFilter set")
	}
}

func TestPseudoElementQuotesIntegration(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
<html>
<head>
<style>
body { quotes: "«" "»" "‹" "›"; }
</style>
</head>
<body>
<p><q>Outer <q>Inner</q> Quote</q></p>
</body>
</html>`
	sess, err := engine.NewSession(htmlStr, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	dl, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}
	list := dl.Build(1)
	if list == nil {
		t.Fatal("expected non-nil display list")
	}

	// Verify that the rendered glyphs contain « and ‹ and › and »
	var textRunes []rune
	for _, cmd := range list.All() {
		for _, g := range cmd.Text.Glyphs {
			textRunes = append(textRunes, g.Rune)
		}
	}
	textStr := string(textRunes)
	if !strings.Contains(textStr, "«") || !strings.Contains(textStr, "‹") || !strings.Contains(textStr, "›") || !strings.Contains(textStr, "»") {
		t.Fatalf("expected nested quotes « ... ‹ ... › ... » in rendered text, got runes: %q", textStr)
	}
}
