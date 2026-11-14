package css_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

// qc:BR-289
func TestAtRuleKeywordEndsAtThePreludeOpeningParen(t *testing.T) {
	for _, src := range []string{
		`@media (min-width: 600px) { .wide{} } @media (max-width: 600px) { .narrow{} }`,
		`@media(min-width:600px){.wide{}}@media(max-width:600px){.narrow{}}`,
		`@media (min-width:600px){.wide{}} @media(max-width: 600px){.narrow{}}`,
	} {
		if got := strings.Join(rulesText(t, src), ","); got != ".wide" {
			t.Errorf("rulesText(%q) = %q, want \".wide\" — the minified prelude must reach the condition test, and the condition must still decide", src, got)
		}
	}
}

// qc:BR-289
func TestMinifiedSupportsPreludeStillGatesOnTheFeature(t *testing.T) {
	got := strings.Join(rulesText(t, `
		a{}
		@supports(display:grid){.gridcards{}}
		@supports(display:subgrid){.subgrid{}}
		@supports(display:flex){.flexcards{}}
		@supports not (display:grid){.nogrid{}}
		@supports(backdrop-filter:blur(4px)){.blur{}}
		@supports(display:grid)and(display:flex){.both{}}
	`), ",")
	want := "a,.gridcards,.flexcards"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// qc:BR-289
func TestMinifiedNestedAtRulesReachTheCascadeInSourceOrder(t *testing.T) {
	got := strings.Join(rulesText(t, `.before{} @supports(display:grid){@media(min-width:600px){.deep{}}} .after{}`), ",")
	if got != ".before,.deep,.after" {
		t.Fatalf("got %q, want \".before,.deep,.after\"", got)
	}
	if bad := rulesText(t, `.before{} @supports(display:grid){@media(min-width:1440px){.deep{}}} .after{}`); len(bad) != 2 {
		t.Fatalf("a minified @media that must not match still reached the cascade: %v", bad)
	}
}

// qc:BR-289
func TestAtRuleWithNoPreludeIsUnchangedByTheParenRule(t *testing.T) {
	got := strings.Join(rulesText(t, `@media{.all{}} @font-face{font-family:x} .keep{}`), ",")
	if got != ".all,.keep" {
		t.Fatalf("got %q, want \".all,.keep\"", got)
	}
}

// qc:BR-289
func TestUnknownAtRuleWithAMinifiedPreludeIsStillSkipped(t *testing.T) {
	for _, src := range []string{
		`@unknown(abc){.dropped{}} .kept{}`,
		`@unknown (abc) { .dropped{} } .kept{}`,
		`@nosuchthing(min-width:600px) and (x:y){.dropped{}} .kept{}`,
	} {
		if got := strings.Join(rulesText(t, src), ","); got != ".kept" {
			t.Errorf("rulesText(%q) = %q, want \".kept\" — an at-rule the engine cannot interpret must not leak its body", src, got)
		}
	}
}

// qc:BR-289
func TestMinifiedPreludeKeepsPrintAndAllMediaLists(t *testing.T) {
	if got := strings.Join(rulesText(t, `@media print{.p{}} @media(min-width:600px){.w{}} @media(all){.a{}}`), ","); got != ".w,.a" {
		t.Fatalf("got %q, want \".w,.a\"", got)
	}
}

// qc:BR-289
func TestAtRuleConditionKeepsTheTextBeforeTheFirstParen(t *testing.T) {
	if got := strings.Join(rulesText(t, `@media print and (min-width: 100px) { .p{} } @media (min-width: 100px) { .w{} }`), ","); got != ".w" {
		t.Fatalf("got %q, want \".w\" — cutting the prelude at the first '(' would drop the print modifier and match a query that must never match on screen", got)
	}
	if got := strings.Join(rulesText(t, `@media all and (min-width: 601px) { .w{} } @media all and (max-width: 600px) { .n{}}`), ","); got != ".w" {
		t.Fatalf("got %q, want \".w\"", got)
	}
	if got := strings.Join(rulesText(t, `@supports (display: grid) and (min-width: 1px) { .s{} }`), ","); got != "" {
		t.Fatalf("got %q, want nothing — a supports test on a non-display feature must stay false", got)
	}
}

// qc:BR-289
func TestMinifiedPreludeKeepsTheQueryTextOfEveryAtRuleFamily(t *testing.T) {
	for _, tc := range []struct {
		name     string
		spaced   string
		minified string
		pick     func(css.Rule) string
	}{
		{"container", `@container (min-width: 400px) { .card{} }`, `@container(min-width:400px){.card{}}`, func(r css.Rule) string { return r.ContainerQuery }},
		{"container-name", `@container sidebar (max-width: 600px) { .item{} }`, `@container sidebar(max-width:600px){.item{}}`, func(r css.Rule) string { return r.ContainerQuery }},
		{"scope-root", `@scope (.a to .b) { .s{} }`, `@scope(.a to .b){.s{}}`, func(r css.Rule) string { return r.ScopeRoot + "|" + r.ScopeLimit }},
	} {
		sheet := css.Parse(tc.spaced)
		mime := css.Parse(tc.minified)
		if len(sheet.Rules) != 1 || len(mime.Rules) != 1 {
			t.Errorf("%s: spaced rules %d, minified rules %d, want 1 and 1", tc.name, len(sheet.Rules), len(mime.Rules))
			continue
		}
		want, got := tc.pick(sheet.Rules[0]), tc.pick(mime.Rules[0])
		norm := func(s string) string { return strings.Join(strings.Fields(s), "") }
		if want == "" {
			t.Errorf("%s: the spaced form itself carries no query text (%q) — the oracle would prove nothing", tc.name, want)
			continue
		}
		if norm(want) != norm(got) {
			t.Errorf("%s: spaced query %q, minified query %q — a minified prelude must reach the same query text", tc.name, want, got)
		}
	}
}
