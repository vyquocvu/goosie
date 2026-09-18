package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/style"
)

// slotFor returns the face the first element named tag resolves to.
func slotFor(t *testing.T, html, tag string) frame.FontSlot {
	t.Helper()
	doc := dom.Parse(html)
	styles := style.Resolve(doc, nil)
	var found *style.ComputedStyle
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		for c := n; c != nil && found == nil; c = c.NextSibling {
			if c.Element() && c.Data == tag {
				if s, ok := styles[c.ID]; ok {
					found = s
					return
				}
			}
			walk(c.FirstChild)
		}
	}
	walk(&doc.Node)
	if found == nil {
		t.Fatalf("no <%s> resolved in %s", tag, html)
	}
	return found.FontSlot()
}

func TestFamilyDefaultsToSerif(t *testing.T) {
	// A document that names no family draws in the engine's standard font, which
	// is what Chromium's is on macOS. The embedded Go face is a fallback, not the
	// default, and getting this wrong moves where every line on a page wraps.
	got := slotFor(t, `<html><body><p>text</p></body></html>`, "p")
	if want := (frame.FontSlot{Family: frame.FontTimes}); got != want {
		t.Errorf("default slot = %+v, want %+v", got, want)
	}
}

func TestFamilyListResolves(t *testing.T) {
	for _, tc := range []struct {
		declared string
		want     frame.FontFamily
	}{
		{"serif", frame.FontTimes},
		{"Times New Roman", frame.FontTimes},
		{"'times new roman'", frame.FontTimes},
		{"sans-serif", frame.FontArial},
		{"Helvetica, Arial, sans-serif", frame.FontArial},
		{"monospace", frame.FontCourier},
		{"'Courier New', monospace", frame.FontCourier},
		{"Georgia, serif", frame.FontGeorgia},
		{"Verdana", frame.FontVerdana},
		{"Go Regular", frame.FontGo},
		// A family this engine cannot serve is skipped, so the list keeps
		// falling through to the next candidate rather than drawing the first.
		{"Totally Made Up Font, monospace", frame.FontCourier},
		{"-apple-system, system-ui, BlinkMacSystemFont, sans-serif", frame.FontArial},
		{"Totally Made Up Font", frame.FontTimes},
	} {
		html := `<html><body><p style="font-family: ` + tc.declared + `">x</p></body></html>`
		got := slotFor(t, html, "p")
		if got.Family != tc.want {
			t.Errorf("font-family: %s resolves to %v, want %v", tc.declared, got.Family, tc.want)
		}
	}
}

func TestWeightAndSlantResolve(t *testing.T) {
	for _, tc := range []struct {
		html         string
		tag          string
		wantBold     bool
		wantItalic   bool
	}{
		{`<html><body><p style="font-weight: 700">x</p></body></html>`, "p", true, false},
		{`<html><body><p style="font-weight: bold">x</p></body></html>`, "p", true, false},
		{`<html><body><p style="font-weight: 500">x</p></body></html>`, "p", false, false},
		{`<html><body><b>x</b></body></html>`, "b", true, false},
		{`<html><body><p style="font-style: italic">x</p></body></html>`, "p", false, true},
		{`<html><body><p style="font-style: oblique">x</p></body></html>`, "p", false, true},
		{`<html><body><em>x</em></body></html>`, "em", false, true},
		{`<html><body><i style="font-weight: 800">x</i></body></html>`, "i", true, true},
	} {
		got := slotFor(t, tc.html, tc.tag)
		if got.Bold != tc.wantBold || got.Italic != tc.wantItalic {
			t.Errorf("%s slot = %+v, want bold=%v italic=%v", tc.tag, got, tc.wantBold, tc.wantItalic)
		}
	}
}

func TestMonospaceTagsResolveToCourier(t *testing.T) {
	html := `<html><body><pre>x</pre><p><code>x</code><kbd>x</kbd><samp>x</samp><tt>x</tt></p></body></html>`
	for _, tag := range []string{"pre", "code", "kbd", "samp", "tt"} {
		if got := slotFor(t, html, tag).Family; got != frame.FontCourier {
			t.Errorf("<%s> family = %v, want Courier New", tag, got)
		}
	}
}
