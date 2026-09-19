package css_test

import "testing"

// An @supports body splices in only when the engine lays out the tested feature,
// so a grid/flex progressive-enhancement rule reaches the cascade while anything
// we cannot judge - subgrid, selector(), another property - stays out.
func TestSupportsGateOnFeature(t *testing.T) {
	got := rulesText(t, `
		a{}
		@supports (display: grid) { .gridcards{} }
		@supports (display: subgrid) { .subgrid{} }
		@supports (display: flex) { .flexcards{} }
		@supports not (display: grid) { .nogrid{} }
		@supports (backdrop-filter: blur(4px)) { .blur{} }
		@supports selector(:has(*)) { .has{} }
		@supports (display: grid) and (display: flex) { .both{} }
		@supports (display: subgrid) or (display: grid) { .either{} }
	`)
	want := []string{"a", ".gridcards", ".flexcards", ".both", ".either"}
	if len(got) != len(want) {
		t.Fatalf("got rules %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got rules %v, want %v", got, want)
		}
	}
}

// An @supports body keeps its place in the cascade relative to the rules around
// it, exactly as @media does.
func TestSupportsOrderPreserved(t *testing.T) {
	got := rulesText(t, `.before{} @supports (display: grid) { .mid{} } .after{}`)
	if len(got) != 3 || got[0] != ".before" || got[1] != ".mid" || got[2] != ".after" {
		t.Fatalf("got %v, want [.before .mid .after]", got)
	}
}
