package layout

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/frame"
)

// qc:BR-087
func TestSplitSoftBreaksLeavesAFittingWordAlone(t *testing.T) {
	word := "aa\u00ADbb"
	got := splitSoftBreaks([]string{word}, 200, 16, nil, frame.FontSlot{}, 0)
	if len(got) != 1 || got[0] != word {
		t.Fatalf("fitting SHY word = %#v, want the original single token", got)
	}
}

// qc:BR-087
func TestSplitSoftBreaksClosesATokenAfterTheOpportunity(t *testing.T) {
	as, bs := strings.Repeat("a", 42), strings.Repeat("b", 48)
	word := as + "\u00AD" + bs
	got := splitSoftBreaks([]string{word}, 200, 16, nil, frame.FontSlot{}, 0)
	if len(got) != 2 {
		t.Fatalf("overlong SHY word tokens = %#v, want two (prefix including SHY, then the b-run)", got)
	}
	if got[0] != as+"\u00AD" {
		t.Errorf("first token = %q, want the a-run closed by U+00AD", got[0])
	}
	if got[1] != bs {
		t.Errorf("second token = %q, want the b-run", got[1])
	}
}

// qc:BR-087
func TestSplitSoftBreaksClosesATokenAfterZWSP(t *testing.T) {
	as, bs := strings.Repeat("a", 42), strings.Repeat("b", 48)
	word := as + "\u200B" + bs
	got := splitSoftBreaks([]string{word}, 200, 16, nil, frame.FontSlot{}, 0)
	if len(got) != 2 || got[0] != as+"\u200B" || got[1] != bs {
		t.Fatalf("overlong ZWSP word tokens = %#v, want [a-run+U+200B, b-run]", got)
	}
}
