package engine_test

import (
	"math"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func TestSessionRejectsUnboundedInput(t *testing.T) {
	tests := []struct {
		name, html string
		css        []string
		width      float32
		want       string
	}{
		{name: "HTML bytes", html: strings.Repeat(" ", (8<<20)+1), width: 400, want: "HTML"},
		{name: "nodes", html: strings.Repeat("<br>", engine.MaxDocumentNodes+1), width: 400, want: "node"},
		{name: "depth", html: strings.Repeat("<div>", 129), width: 400, want: "depth"},
		{name: "attributes", html: "<p " + strings.Repeat("a='b' ", 1000) + ">x", width: 400, want: "attribute"},
		{name: "CSS bytes", css: []string{strings.Repeat(" ", engine.MaxCSSBytes+1)}, width: 400, want: "CSS"},
		{name: "rules", css: []string{strings.Repeat("p{}", engine.MaxCSSRules+1)}, width: 400, want: "rule"},
		{name: "selector list", css: []string{strings.Repeat("p,", engine.MaxCSSSelectors) + "p{}"}, width: 400, want: "selector"},
		{name: "selector chain", css: []string{strings.Repeat("p > ", engine.MaxSelectorParts) + "p{}"}, width: 400, want: "selector"},
		{name: "recursive selector", css: []string{strings.Repeat(":not(", 20) + "p" + strings.Repeat(")", 20) + "{}"}, width: 400, want: "nesting"},
		{name: "font", html: `<p style="font-size:513px">x`, width: 400, want: "font"},
		{name: "geometry", html: `<p style="width:1048577px">x`, width: 400, want: "geometry"},
		{name: "aggregate geometry", html: `<div style="height:600000px"></div><div style="height:600000px"></div>`, width: 400, want: "geometry"},
		{name: "numeric overflow", html: `<p style="width:` + strings.Repeat("9", 400) + `px">x`, width: 400, want: "numeric"},
		{name: "NaN width", width: float32(math.NaN()), want: "viewport"},
		{name: "Inf width", width: float32(math.Inf(1)), want: "viewport"},
		{name: "huge width", width: math.MaxFloat32, want: "viewport"},
		{name: "zero width", width: 0, want: "viewport"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := engine.NewSession(tt.html, tt.css, tt.width)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("want %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestSessionAcceptsRepeatedTextAndRawHTML(t *testing.T) {
	for _, html := range []string{
		`<p>` + strings.Repeat("normal repeated text ", 1000) + `</p>`,
		`<script>` + strings.Repeat("<div>", 10000) + `</script><p>hello</p>`,
		`<p title="<div><div>">hello</p>`,
		strings.Repeat("<p>text</p>", 1000),
	} {
		if _, err := engine.NewSession(html, nil, 800); err != nil {
			t.Fatal(err)
		}
	}
}
