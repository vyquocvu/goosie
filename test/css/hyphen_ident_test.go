package css_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

type tokSpec struct {
	Type   css.TokenType
	Value  string
	Unit   string
	NumVal float64
}

func (s tokSpec) name() string { return typeName(s.Type) }

func typeName(t css.TokenType) string {
	switch t {
	case css.TokenIdent:
		return "Ident"
	case css.TokenNumber:
		return "Number"
	case css.TokenPercentage:
		return "Percentage"
	case css.TokenDimension:
		return "Dimension"
	case css.TokenHash:
		return "Hash"
	case css.TokenDelim:
		return "Delim"
	case css.TokenCDC:
		return "CDC"
	case css.TokenFunction:
		return "Function"
	case css.TokenLBrace:
		return "LBrace"
	}
	return "Other"
}

// qc:BR-192
func TestHyphenTokenKind(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []tokSpec
	}{
		{"digit-leading custom property", "--8e8e8e", []tokSpec{{css.TokenIdent, "--8e8e8e", "", 0}}},
		{"digit-leading short name", "--8e8", []tokSpec{{css.TokenIdent, "--8e8", "", 0}}},
		{"digit-leading colour name", "--666666", []tokSpec{{css.TokenIdent, "--666666", "", 0}}},
		{"hyphen inside the name", "--a-8e8e8", []tokSpec{{css.TokenIdent, "--a-8e8e8", "", 0}}},
		{"non-numeric tail", "--1e9e9", []tokSpec{{css.TokenIdent, "--1e9e9", "", 0}}},
		{"bare double hyphen", "--", []tokSpec{{css.TokenIdent, "--", "", 0}}},
		{"vendor prefix", "-webkit-mask", []tokSpec{{css.TokenIdent, "-webkit-mask", "", 0}}},
		{"vendor prefix with digits", "-moz-opacity2", []tokSpec{{css.TokenIdent, "-moz-opacity2", "", 0}}},
		{"negative length", "-5px", []tokSpec{{css.TokenDimension, "-5", "px", -5}}},
		{"negative fraction", "-.5em", []tokSpec{{css.TokenDimension, "-.5", "em", -0.5}}},
		{"negative scientific", "-8e8e8", []tokSpec{{css.TokenDimension, "-8e8", "e8", -8e8}}},
		{"negative percentage", "-25%", []tokSpec{{css.TokenPercentage, "-25", "", -25}}},
		{"lone hyphen", "-", []tokSpec{{css.TokenDelim, "-", "", 0}}},
		{"hyphen before brace", "-{", []tokSpec{{css.TokenDelim, "-", "", 0}, {css.TokenLBrace, "{", "", 0}}},
		{"hash colour", "#8e8e8e", []tokSpec{{css.TokenHash, "8e8e8e", "", 0}}},
		{"digit-led dimension", "8e8e8px", []tokSpec{{css.TokenDimension, "8e8", "e8px", 8e8}}},
		{"CDC", "-->", []tokSpec{{css.TokenCDC, "-->", "", 0}}},
		{"custom property function", "--a(", []tokSpec{{css.TokenFunction, "--a", "", 0}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tok := css.NewTokenizer(tc.input)
			var got []tokSpec
			for {
				tk := tok.Next()
				if tk.Type == css.TokenEOF {
					break
				}
				got = append(got, tokSpec{tk.Type, tk.Value, tk.Unit, tk.NumVal})
			}
			if len(got) != len(tc.want) {
				t.Fatalf("token stream for %q = %v (%d tokens), want %v (%d tokens)",
					tc.input, describe(got), len(got), describe(tc.want), len(tc.want))
			}
			for i, w := range tc.want {
				g := got[i]
				if g.Type != w.Type || g.Value != w.Value || g.Unit != w.Unit {
					t.Errorf("token %d for %q = %s value %q unit %q, want %s value %q unit %q",
						i, tc.input, g.name(), g.Value, g.Unit, w.name(), w.Value, w.Unit)
					continue
				}
				if math.Abs(g.NumVal-w.NumVal) > 1e-9 {
					t.Errorf("token %d for %q numval %v, want %v", i, tc.input, g.NumVal, w.NumVal)
				}
			}
		})
	}
}

// qc:BR-192
func TestTokenSpansCoverInputExactly(t *testing.T) {
	for _, in := range []string{"--8e8e8e", "--1e5x", "--5px", "-8e8e8", "1e1e1", "--a-8e8e8", "-webkit-mask"} {
		tok := css.NewTokenizer(in)
		type span struct {
			kind css.TokenType
			a, b int
		}
		var spans []span
		for {
			tk := tok.Next()
			if tk.Type == css.TokenEOF {
				break
			}
			spans = append(spans, span{tk.Type, tok.Offset(), 0})
			if len(spans) > 1 {
				spans[len(spans)-2].b = tok.Offset()
			}
		}
		if len(spans) == 0 {
			t.Fatalf("tokenizer produced no tokens for %q", in)
		}
		spans[len(spans)-1].b = len(in)
		for i, s := range spans {
			if s.b <= s.a {
				t.Fatalf("token %d of %q has empty or reversed span [%d,%d)", i, in, s.a, s.b)
			}
			if i > 0 && spans[i-1].b != s.a {
				t.Errorf("token %d of %q starts at %d but the previous token ended at %d: spans overlap or leave bytes uncovered",
					i, in, s.a, spans[i-1].b)
			}
		}
	}
}

func describe(ts []tokSpec) string {
	out := "["
	for i, t := range ts {
		if i > 0 {
			out += " "
		}
		out += t.name() + "(value=" + t.Value + " unit=" + t.Unit + ")"
	}
	return out + "]"
}
