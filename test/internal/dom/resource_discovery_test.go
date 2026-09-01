package dom_test

import (
	"context"
	"strings"
	"testing"
	"github.com/vyquocvu/goosie/internal/dom"
)

// TestResourceDiscovery_ScriptMode — M2: the parser reports the script
// mode observed in the markup. Classic (default), async, defer, and
// module are all distinct.
func TestResourceDiscovery_ScriptMode(t *testing.T) {
	cases := []struct {
		name string
		tag  string
		want dom.ScriptMode
	}{
		{"classic", `<script src="x.js"></script>`, dom.ScriptModeClassic},
		{"async", `<script src="x.js" async></script>`, dom.ScriptModeAsync},
		{"defer", `<script src="x.js" defer></script>`, dom.ScriptModeDefer},
		{"module", `<script type="module" src="x.js"></script>`, dom.ScriptModeModule},
		{"async+defer precedence: async wins", `<script src="x.js" async defer></script>`, dom.ScriptModeAsync},
		{"inline classic", `<script>var x = 1;</script>`, dom.ScriptModeClassic},
		{"inline module", `<script type="module">import "./x";</script>`, dom.ScriptModeModule},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seen := captureScripts(t, tc.tag)
			if len(seen) != 1 {
				t.Fatalf("expected 1 script resource, got %d", len(seen))
			}
			if seen[0].ScriptMode != tc.want {
				t.Errorf("ScriptMode = %v, want %v", seen[0].ScriptMode, tc.want)
			}
		})
	}
}

// TestResourceDiscovery_Inline — M2: <script> with no src is reported
// with Inline=true and an empty URL.
func TestResourceDiscovery_Inline(t *testing.T) {
	seen := captureScripts(t, `<script>console.log(1)</script>`)
	if len(seen) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(seen))
	}
	r := seen[0]
	if !r.Inline {
		t.Error("Inline = false, want true")
	}
	if r.URL != "" {
		t.Errorf("URL = %q, want empty for inline", r.URL)
	}
	if r.Kind != dom.ResourceScript {
		t.Errorf("Kind = %v, want ResourceScript", r.Kind)
	}
}

// TestResourceDiscovery_Position — M2: discovered resources carry a
// document-order Position. Position is assigned even when no resource
// is yielded by a tag, so document order is preserved across tags
// that do and do not produce resources.
func TestResourceDiscovery_Position(t *testing.T) {
	input := `<html><head>
		<link rel="stylesheet" href="a.css">
		<meta charset="utf-8">
		<link rel="stylesheet" href="b.css">
		<p>hi</p>
		<img src="c.png">
		<link rel="stylesheet" href="d.css">
	</head></html>`

	var got []dom.Resource
	_, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(input),
		dom.ParseConfig{OnResource: func(r dom.Resource) { got = append(got, r) }})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 resources, got %d", len(got))
	}
	// Positions are monotonic and unique.
	seen := map[int]bool{}
	for _, r := range got {
		if r.Position < 0 {
			t.Errorf("negative position: %d", r.Position)
		}
		if seen[r.Position] {
			t.Errorf("duplicate position %d", r.Position)
		}
		seen[r.Position] = true
	}
	// Resources in source order.
	wantOrder := []dom.ResourceKind{dom.ResourceCSS, dom.ResourceCSS, dom.ResourceImage, dom.ResourceCSS}
	for i, r := range got {
		if r.Kind != wantOrder[i] {
			t.Errorf("resource[%d].Kind = %v, want %v", i, r.Kind, wantOrder[i])
		}
	}
}

// TestResourceDiscovery_IntegrityCrossOrigin — M2: SRI integrity and
// crossorigin attributes are surfaced on the Resource. M5+ will act on
// them; M2 only captures.
func TestResourceDiscovery_IntegrityCrossOrigin(t *testing.T) {
	html := `<link rel="stylesheet" href="a.css" integrity="sha384-x" crossorigin="anonymous">`
	var got []dom.Resource
	_, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(html),
		dom.ParseConfig{OnResource: func(r dom.Resource) { got = append(got, r) }})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(got))
	}
	r := got[0]
	if r.Integrity != "sha384-x" {
		t.Errorf("Integrity = %q, want sha384-x", r.Integrity)
	}
	if r.CrossOrigin != "anonymous" {
		t.Errorf("CrossOrigin = %q, want anonymous", r.CrossOrigin)
	}
}

// TestResourceDiscovery_NoResourceTags — non-resource tags do not
// trigger OnResource, and the position counter is still incremented.
func TestResourceDiscovery_NoResourceTags(t *testing.T) {
	html := `<html><body><div><span>hi</span></div></body></html>`
	var got []dom.Resource
	_, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(html),
		dom.ParseConfig{OnResource: func(r dom.Resource) { got = append(got, r) }})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 resources, got %d", len(got))
	}
}

// TestResourceDiscovery_LinkNotStylesheet — <link rel="icon"> etc. are
// not reported as stylesheets.
func TestResourceDiscovery_LinkNotStylesheet(t *testing.T) {
	html := `<link rel="icon" href="favicon.ico">`
	var got []dom.Resource
	_, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(html),
		dom.ParseConfig{OnResource: func(r dom.Resource) { got = append(got, r) }})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 resources for rel=icon, got %d", len(got))
	}
}

// TestScriptModeString — ScriptMode.String covers all known modes.
func TestScriptModeString(t *testing.T) {
	cases := map[dom.ScriptMode]string{
		dom.ScriptModeClassic: "classic",
		dom.ScriptModeAsync:   "async",
		dom.ScriptModeDefer:   "defer",
		dom.ScriptModeModule:  "module",
		dom.ScriptMode(99):    "unknown",
	}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("ScriptMode(%d).String() = %q, want %q", m, got, want)
		}
	}
}

// TestResourceDiscovery_CrossOriginOnImg — crossorigin on <img> is
// captured.
func TestResourceDiscovery_CrossOriginOnImg(t *testing.T) {
	html := `<img src="x.png" crossorigin="use-credentials">`
	var got []dom.Resource
	_, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(html),
		dom.ParseConfig{OnResource: func(r dom.Resource) { got = append(got, r) }})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(got))
	}
	if got[0].CrossOrigin != "use-credentials" {
		t.Errorf("CrossOrigin = %q, want use-credentials", got[0].CrossOrigin)
	}
}

// TestResourceDiscovery_NoOnResourceCallback — parsing without
// OnResource works (the parser doesn't crash).
func TestResourceDiscovery_NoOnResourceCallback(t *testing.T) {
	html := `<link rel="stylesheet" href="x.css"><script src="y.js"></script>`
	if _, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(html), dom.ParseConfig{}); err != nil {
		t.Fatalf("parse: %v", err)
	}
}

// captureScripts parses one snippet of HTML and returns the discovered
// script resources. Helper to keep table-driven tests concise.
func captureScripts(t *testing.T, snippet string) []dom.Resource {
	t.Helper()
	html := "<html><head>" + snippet + "</head></html>"
	var got []dom.Resource
	_, err := dom.NewParser().ParseDocumentCtx(context.Background(),
		strings.NewReader(html),
		dom.ParseConfig{OnResource: func(r dom.Resource) {
			if r.Kind == dom.ResourceScript {
				got = append(got, r)
			}
		}})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return got
}
