package layout_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/layout"
)

type firstLineWord struct {
	text       string
	x, y, w, h float32
	fontSize   float32
	lineHeight float32
}

func firstLineWords(t *testing.T, html string) ([]firstLineWord, *layout.Object) {
	t.Helper()
	arena, doc := sessionWithSheets(t, html, 800)
	layout.Inline(arena, layout.ObjectID(1))
	target := doc.ElementByID("target")
	if target == nil {
		t.Fatal("target element not found")
	}
	id, ok := arena.ForNode(target.ID)
	if !ok {
		t.Fatal("target layout object not found")
	}
	var words []firstLineWord
	var walk func(layout.ObjectID)
	walk = func(current layout.ObjectID) {
		obj := arena.Get(current)
		if obj.Node != nil && obj.Node.Text() && strings.TrimSpace(obj.Node.DataContent) != "" {
			word := firstLineWord{text: obj.Node.DataContent, x: obj.X, y: obj.Y, w: obj.W, h: obj.H}
			if obj.Style != nil {
				word.fontSize = obj.Style.FontSize
				word.lineHeight = obj.Style.LineHeight
			}
			words = append(words, word)
		}
		for kid := obj.FirstKid; kid != 0; kid = arena.Get(kid).NextSibling {
			walk(kid)
		}
	}
	walk(id)
	return words, arena.Get(id)
}

func firstLineWordRows(words []firstLineWord) [][]string {
	var rows [][]string
	var previousX, previousY float32
	for i, word := range words {
		startsLine := i == 0 || word.x < previousX-0.5 ||
			(word.x <= previousX+0.5 && word.y > previousY+0.5)
		if startsLine {
			rows = append(rows, nil)
		}
		rows[len(rows)-1] = append(rows[len(rows)-1], word.text)
		previousX, previousY = word.x, word.y
	}
	return rows
}

func assertFirstLineWordRows(t *testing.T, words []firstLineWord, want ...[]string) {
	t.Helper()
	got := firstLineWordRows(words)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("word rows = %q, want Chromium rows %q", got, want)
	}
}

func TestFirstLineFontSizeGrowthUsesPseudoMetricsForBreaking(t *testing.T) {
	words, _ := firstLineWords(t, `<html><head><style>
		body { margin: 0; }
		p { width: 150px; margin: 0; font: 16px/20px Arial, sans-serif; }
		p::first-line { font-size: 32px; }
	</style></head><body><p id="target">Alpha beta gamma delta</p></body></html>`)
	assertFirstLineWordRows(t, words, []string{"Alpha"}, []string{"beta", "gamma", "delta"})
	if words[0].fontSize != 32 || words[1].fontSize != 16 {
		t.Fatalf("font sizes = %v, %v, want 32, 16", words[0].fontSize, words[1].fontSize)
	}
}

func TestFirstLineFontSizeShrinkUsesPseudoMetricsForBreaking(t *testing.T) {
	words, _ := firstLineWords(t, `<html><head><style>
		body { margin: 0; }
		p { width: 95px; margin: 0; font: 16px/20px Arial, sans-serif; }
		p::first-line { font-size: 8px; }
	</style></head><body><p id="target">Alpha beta gamma delta</p></body></html>`)
	assertFirstLineWordRows(t, words, []string{"Alpha", "beta", "gamma", "delta"})
	if words[0].fontSize != 8 {
		t.Fatalf("first-word font size = %v, want 8", words[0].fontSize)
	}
}

func TestFirstLineNestedOwnFontSizeBlocksPseudoOverride(t *testing.T) {
	words, _ := firstLineWords(t, `<html><head><style>
		body { margin: 0; }
		p { width: 150px; margin: 0; font: 16px/20px Arial, sans-serif; }
		p::first-line { font-size: 32px; }
		p span { font-size: 16px; }
	</style></head><body><p id="target"><span>Alpha</span> beta gamma</p></body></html>`)
	assertFirstLineWordRows(t, words, []string{"Alpha", "beta"}, []string{"gamma"})
	if words[0].fontSize != 16 || words[1].fontSize != 32 {
		t.Fatalf("font sizes = %v, %v, want nested 16, pseudo 32", words[0].fontSize, words[1].fontSize)
	}
}

func TestFirstLineNestedInlineAncestorFontBlocksPseudoOverride(t *testing.T) {
	words, _ := firstLineWords(t, `<html><head><style>
		body { margin: 0; }
		p { width: 150px; margin: 0; font: 16px/20px Arial, sans-serif; }
		p::first-line { font-size: 32px; }
		p span { font-size: 16px; }
	</style></head><body><p id="target"><span><b>Alpha</b></span> beta gamma</p></body></html>`)
	assertFirstLineWordRows(t, words, []string{"Alpha", "beta"}, []string{"gamma"})
	if words[0].fontSize != 16 || words[1].fontSize != 32 {
		t.Fatalf("font sizes = %v, %v, want nested ancestor 16, pseudo 32", words[0].fontSize, words[1].fontSize)
	}
}

func TestFirstLineFontSizeAdvancesByTheRealLineHeightPastFloat(t *testing.T) {
	words, target := firstLineWords(t, `<html><head><style>
		body { margin: 0; }
		p { width: 200px; margin: 0; font: 16px/20px Arial, sans-serif; }
		p::first-line { font-size: 32px; line-height: 40px; }
		p .float { float: right; width: 60px; height: 40px; }
	</style></head><body><p id="target"><span class="float"></span>Alpha beta gamma delta</p></body></html>`)
	assertFirstLineWordRows(t, words, []string{"Alpha"}, []string{"beta", "gamma", "delta"})
	if target.H != 60 {
		t.Fatalf("target height = %v, want Chromium 60", target.H)
	}
}

func TestFirstLineLeadingBreakUsesPseudoLineHeight(t *testing.T) {
	words, target := firstLineWords(t, `<html><head><style>
		body { margin: 0; }
		p { width: 200px; margin: 0; font: 16px/20px Arial, sans-serif; }
		p::first-line { font-size: 32px; line-height: 48px; }
	</style></head><body><p id="target"><br>Alpha beta</p></body></html>`)
	assertFirstLineWordRows(t, words, []string{"Alpha", "beta"})
	if target.H != 68 {
		t.Fatalf("target height = %v, want Chromium 68", target.H)
	}
	if words[0].y < 49 || words[0].y > 50 {
		t.Fatalf("first word y = %v, want Chromium ink top 49", words[0].y)
	}
}

func TestFirstLineSpacingParticipatesInBreaking(t *testing.T) {
	cases := []struct {
		name        string
		width       int
		declaration string
	}{
		{"letter-spacing", 150, "letter-spacing: 4px;"},
		{"word-spacing", 130, "word-spacing: 12px;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := fmt.Sprintf(`<html><head><style>
				body { margin: 0; }
				p { width: %dpx; margin: 0; font: 16px/20px Arial, sans-serif; }
				p::first-line { %s }
			</style></head><body><p id="target">Alpha beta gamma delta</p></body></html>`, tc.width, tc.declaration)
			words, _ := firstLineWords(t, html)
			assertFirstLineWordRows(t, words, []string{"Alpha", "beta"}, []string{"gamma", "delta"})
		})
	}
}

func TestFirstLinePreservesFirstLetterFont(t *testing.T) {
	words, _ := firstLineWords(t, `<html><head><style>
		body { margin: 0; }
		p { width: 150px; margin: 0; font: 16px/20px Arial, sans-serif; }
		p::first-line { font-size: 32px; }
		p::first-letter { font-size: 48px; }
	</style></head><body><p id="target">Alpha beta gamma</p></body></html>`)
	assertFirstLineWordRows(t, words, []string{"A", "lpha"}, []string{"beta", "gamma"})
	if words[0].fontSize != 48 || words[1].fontSize != 32 {
		t.Fatalf("first-letter/remainder font sizes = %v/%v, want 48/32", words[0].fontSize, words[1].fontSize)
	}
}

func TestFirstLineBreakingFallsBackForBalanceAndSpecialSplitting(t *testing.T) {
	cases := []struct {
		name  string
		style string
		text  string
	}{
		{"balance", "text-wrap: balance;", "Alpha beta gamma delta epsilon zeta eta theta"},
		{"break-all", "word-break: break-all;", "AlphaBetaGamma"},
		{"cjk", "", "漢字漢字漢字"},
		{"soft-hyphen", "", "Alpha\u00adBetaGamma"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			head := `<html><head><style>body { margin: 0; } p { width: 70px; margin: 0; font: 16px/20px Arial, sans-serif; ` + tc.style + ` }`
			plain, _ := firstLineWords(t, head+`</style></head><body><p id="target">`+tc.text+`</p></body></html>`)
			gated, _ := firstLineWords(t, head+`p::first-line { font-size: 32px; }</style></head><body><p id="target">`+tc.text+`</p></body></html>`)
			if fmt.Sprint(firstLineWordRows(plain)) != fmt.Sprint(firstLineWordRows(gated)) {
				t.Fatalf("fallback rows = %q, want base rows %q", firstLineWordRows(gated), firstLineWordRows(plain))
			}
			if gated[0].fontSize != 16 {
				t.Fatalf("fallback font size = %v, want base 16", gated[0].fontSize)
			}
		})
	}
}

func TestFirstLineLineHeightRatioUsesFinalPseudoFontSize(t *testing.T) {
	words, target := firstLineWords(t, `<html><head><style>
		body { margin: 0; }
		p { width: 400px; margin: 0; font: 16px/20px Arial, sans-serif; }
		p::first-line { line-height: 1.5; font-size: 32px; }
	</style></head><body><p id="target">Alpha beta<br>gamma</p></body></html>`)
	assertFirstLineWordRows(t, words, []string{"Alpha", "beta"}, []string{"gamma"})
	if target.H != 68 {
		t.Fatalf("target height = %v, want Chromium 68", target.H)
	}
	if words[0].lineHeight != 48 || words[2].lineHeight != 20 {
		t.Fatalf("line heights = %v, %v, want first-line 48 and second-line 20", words[0].lineHeight, words[2].lineHeight)
	}
}
