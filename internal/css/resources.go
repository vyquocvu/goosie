package css

import (
	"regexp"
	"strings"
)

// ResourceKind categorises a secondary resource discovered inside a
// stylesheet. The documentloader coordinator routes each kind to the
// matching callback (OnStylesheet, OnFont, OnImage).
//
// Per plan.md M7: "Nested @import/@font-face/url() in CSS must be
// discovered and scheduled by the same coordinator." The Kind is the
// discriminator that drives which coordinator callback fires.
type ResourceKind uint8

const (
	// ResourceStylesheet is a nested stylesheet discovered via
	// @import url(...). The bytes returned by the fetcher are
	// themselves a stylesheet and may yield further secondaries.
	ResourceStylesheet ResourceKind = iota

	// ResourceFont is a font file discovered via @font-face src:
	// url(...) or a font-face declaration elsewhere. Fetched as
	// bytes (the renderer does its own font decode).
	ResourceFont

	// ResourceImage is an image URL discovered via url(...) in any
	// declaration (background-image, list-style-image, etc.). The
	// property name is preserved in the Property field so callers
	// can correlate it to the CSS rule that referenced it.
	ResourceImage
)

// String returns a stable lowercase identifier for logging and for
// tests that round-trip through string keys.
func (k ResourceKind) String() string {
	switch k {
	case ResourceStylesheet:
		return "stylesheet"
	case ResourceFont:
		return "font"
	case ResourceImage:
		return "image"
	default:
		return "unknown"
	}
}

// CSSResource is one nested resource discovered inside a stylesheet.
// URL is the raw string as it appears in the CSS source; callers
// resolve it against the parent stylesheet's URL.
//
// Property is set only for ResourceImage and names the CSS property
// (background-image, list-style-image, etc.) where the url() appeared.
// For ResourceStylesheet and ResourceFont, Property is empty.
type CSSResource struct {
	Kind     ResourceKind
	URL      string
	Property string
}

// urlPattern matches url(...) with a string or unquoted identifier
// inside. We deliberately do not parse CSS values deeply — the goal
// is discovery, not full CSS value parsing. Nested parens are not
// supported (rare in CSS); if encountered, the regex stops at the
// first closing paren. Quoted strings may contain escapes; we accept
// the basic case without escape decoding.
var urlPattern = regexp.MustCompile(`url\(\s*(?:"([^"]+)"|'([^']+)'|([^)\s]+))\s*\)`)

// importPattern matches @import directives in the prelude. The CSS
// grammar allows @import to appear only at the top of a stylesheet
// (before any other rules), but the parser does not enforce that
// strictly — we discover @import wherever the parser recorded it as
// an AtRule with Name == "import".
//
// importURLPattern captures the URL token from the prelude. The
// prelude is free-form text; we accept both url(...) and bare
// string/identifier forms.
var importURLPattern = regexp.MustCompile(`(?i)(?:url\(\s*(?:"([^"]+)"|'([^']+)'|([^)\s]+))\s*\)|"([^"]+)"|'([^']+)'|([^\s;]+))`)

// ExtractResources walks a parsed StyleSheet and returns all nested
// resource URLs. The returned slice is in source order. Duplicate
// URLs are kept (the coordinator dedupes; consumers may want to
// deduplicate themselves).
//
// Rules:
//   - @import in AtRules at any level produces ResourceStylesheet.
//   - @font-face rule bodies produce ResourceFont for each url() in
//     the rule's declarations.
//   - Every Declaration.Value is scanned for url(...). Matches
//     produce ResourceImage with the declaration's Property name
//     (e.g. "background-image").
//
// url() matches use the regex above; this is intentionally lenient.
// A correct CSS parser would respect strings/comments; we accept
// the trade-off (false positives are rare in practice and only
// trigger an extra fetch attempt, which fails harmlessly).
func ExtractResources(sheet *StyleSheet) []CSSResource {
	if sheet == nil {
		return nil
	}
	out := make([]CSSResource, 0, 8)
	// Top-level @import.
	for _, ar := range sheet.AtRules {
		if strings.EqualFold(ar.Name, "import") {
			if u := extractImportURL(ar.Prelude); u != "" {
				out = append(out, CSSResource{
					Kind: ResourceStylesheet, URL: u,
				})
			}
		}
		walkAtRuleResources(ar, &out)
	}
	// Top-level rules: scan declarations for url().
	for _, r := range sheet.Rules {
		scanRuleForURLs(r, &out)
	}
	return out
}

// walkAtRuleResources recurses into at-rule substructures (@media
// @supports @keyframes etc.) to discover font-face and image URLs.
func walkAtRuleResources(ar AtRule, out *[]CSSResource) {
	if strings.EqualFold(ar.Name, "font-face") {
		// font-face declarations: each url() in any declaration value
		// is a font file.
		for _, d := range ar.Declarations {
			for _, u := range extractURLs(d.Value) {
				*out = append(*out, CSSResource{
					Kind: ResourceFont, URL: u,
				})
			}
		}
		// Also check nested rules (some fonts use src inside @font-face).
		for _, r := range ar.Rules {
			scanRuleForURLs(r, out)
		}
	}
	// Nested at-rules.
	for _, nested := range ar.AtRules {
		walkAtRuleResources(nested, out)
	}
	// Nested rules.
	for _, r := range ar.Rules {
		scanRuleForURLs(r, out)
	}
}

// scanRuleForURLs walks a rule's declarations and appends any
// url() matches as ResourceImage with the declaration's Property.
func scanRuleForURLs(r Rule, out *[]CSSResource) {
	for _, d := range r.Declarations {
		for _, u := range extractURLs(d.Value) {
			*out = append(*out, CSSResource{
				Kind:     ResourceImage,
				URL:      u,
				Property: d.Property,
			})
		}
	}
}

// extractURLs returns the bare URLs (strings) inside url(...) calls
// in a CSS value. Strings without a url(...) wrapper are ignored.
func extractURLs(value string) []string {
	matches := urlPattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		for i := 1; i <= 3; i++ {
			if m[i] != "" {
				out = append(out, m[i])
				break
			}
		}
	}
	return out
}

// extractImportURL extracts the URL from an @import prelude. Returns
// the empty string if no URL can be found.
func extractImportURL(prelude string) string {
	m := importURLPattern.FindStringSubmatch(prelude)
	if m == nil {
		return ""
	}
	for i := 1; i <= 6; i++ {
		if m[i] != "" {
			return m[i]
		}
	}
	return ""
}