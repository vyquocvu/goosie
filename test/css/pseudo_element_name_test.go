package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// qc:BR-322
func TestPseudoElementNameCanonicalisesTheResolvedFamily(t *testing.T) {
	cases := []struct {
		token string
		name  string
		ok    bool
	}{
		{"before", "before", true},
		{"after", "after", true},
		{"first-line", "first-line", true},
		{"first-letter", "first-letter", true},
		{"marker", "marker", true},
		{"placeholder", "placeholder", true},
		{"selection", "selection", true},
		{"backdrop", "backdrop", true},
		{"file-selector-button", "file-selector-button", true},
		{"target-text", "target-text", true},
		{"grammar-error", "grammar-error", true},
		{"spelling-error", "spelling-error", true},
		{"cue", "cue", true},
		{"view-transition", "view-transition", true},
		{"BEFORE", "before", true},
		{"First-Line", "first-line", true},
		{"part(widget)", "part", true},
		{"part( widget )", "part", true},
		{"slotted(div)", "slotted", true},
		{"highlight(x)", "highlight", true},
		{"", "", false},
		{"nope", "", false},
		{"-moz-none", "", false},
		{"hover", "", false},
		{"focus-visible", "", false},
		{"not(before)", "", false},
		{"::before", "", false},
		{":before", "", false},
		{"before extra", "", false},
		{"before ", "", false},
		{"before;", "", false},
		{"before:after", "", false},
		{"part", "", false},
		{"part()", "", false},
		{"part(widget", "", false},
		{"slotted()", "", false},
		{"first-line-", "", false},
		{"1before", "", false},
	}
	for _, c := range cases {
		name, ok := css.PseudoElementName(c.token)
		if ok != c.ok || name != c.name {
			t.Errorf("PseudoElementName(%q) = (%q, %v), want (%q, %v)", c.token, name, ok, c.name, c.ok)
		}
	}
}
