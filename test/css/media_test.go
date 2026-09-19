package css_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func rulesText(t *testing.T, src string) []string {
	t.Helper()
	sheet := css.Parse(src)
	var out []string
	for _, r := range sheet.Rules {
		for _, s := range r.SelectorStrs {
			out = append(out, s)
		}
	}
	return out
}

func TestMediaRulesGateOnViewport(t *testing.T) {
	css.SetMediaViewportWidth(1280)
	src := `
		a{}
		@media (max-width: 600px) { .mobile{} }
		@media screen and (min-width: 1200px) { .wide{} }
		@media print { .paper{} }
		@media (min-width: 30em) { .emwide{} }
		@media (orientation: portrait) { .portrait{} }
		@media not all and (max-width: 600px) { .notmobile{} }
	`
	got := rulesText(t, src)
	want := []string{"a", ".wide", ".emwide", ".notmobile"}
	if len(got) != len(want) {
		t.Fatalf("got rules %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got rules %v, want %v", got, want)
		}
	}
}

func TestMediaNestedInsideMedia(t *testing.T) {
	css.SetMediaViewportWidth(1280)
	got := rulesText(t, `@media screen { @media (min-width: 100px) { .deep{} } }`)
	if len(got) != 1 || got[0] != ".deep" {
		t.Fatalf("got %v, want [.deep]", got)
	}
}

func TestMediaOrderPreserved(t *testing.T) {
	css.SetMediaViewportWidth(1280)
	got := rulesText(t, `.before{} @media all { .mid{} } .after{}`)
	if len(got) != 3 || got[0] != ".before" || got[1] != ".mid" || got[2] != ".after" {
		t.Fatalf("got %v, want [.before .mid .after]", got)
	}
}

// TestMediaRangeSyntaxAndOr covers the Media Queries 4 forms modern sites write
// instead of min-/max-width. A parenthesised expression the parser could not
// read fell through as satisfied, so MDN's mobile-only
// `@media (width <= 769px) or (width < calc(1rem * 2 + 15rem + 2rem + 31rem))`
// hid the page banner at 1280px and shifted every block below it up. `or` also
// read as `and`, so one failing alternative killed a group that should match.
func TestMediaRangeSyntaxAndOr(t *testing.T) {
	css.SetMediaViewportWidth(1280)
	src := `
		@media (width <= 769px) { .narrow{} }
		@media (width < calc(1rem * 2 + 15rem + 2rem + 31rem)) { .under800{} }
		@media (width <= 769px) or (width >= 1000px) { .either{} }
		@media (400px <= width < 1200px) { .band{} }
		@media (769px >= width) { .reversed{} }
		@media (width >= 1280px) { .atleast{} }
	`
	got := rulesText(t, src)
	want := []string{".either", ".atleast"}
	if len(got) != len(want) {
		t.Fatalf("got rules %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got rules %v, want %v", got, want)
		}
	}
}

// TestMediaPreferenceFeatures covers the preference and resolution features
// modern themes wrap around dark palettes and retina sprites. None of them
// parsed as anything, and an unrecognised feature read as satisfied, so goosie
// painted `@media (prefers-color-scheme: dark)` backgrounds onto pages a light
// Chromium renders white.
func TestMediaPreferenceFeatures(t *testing.T) {
	css.SetMediaViewportWidth(1280)
	src := `
		@media (prefers-color-scheme: dark) { .dark{} }
		@media (prefers-color-scheme: light) { .light{} }
		@media (prefers-color-scheme) { .any{} }
		@media (prefers-reduced-motion: reduce) { .motion{} }
		@media (prefers-reduced-motion: no-preference) { .still{} }
		@media (prefers-contrast: more) { .contrast{} }
		@media (min-device-pixel-ratio: 2) { .retina{} }
		@media (-webkit-min-device-pixel-ratio: 1.5) { .webkit2x{} }
		@media (min-device-pixel-ratio: 1) { .onex{} }
	`
	got := rulesText(t, src)
	want := []string{".light", ".any", ".still", ".onex"}
	if len(got) != len(want) {
		t.Fatalf("got rules %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got rules %v, want %v", got, want)
		}
	}
}
