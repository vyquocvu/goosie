package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

const cssomPage = `<!doctype html><html><head>
<link id="L1" rel="stylesheet" href="a.css">
<link id="L2" rel="stylesheet" href="b.css" media="screen">
<link id="L3" rel="stylesheet" href="c.css" title="Alternate">
<link id="L4" rel="stylesheet" href="d.css" disabled>
<link id="L5" rel="stylesheet" href="">
<link id="L6" rel="preload" href="font.woff2">
<style id="S1">p.style1 { color: rgb(7,7,7); }</style>
<style id="S2" media="print" title="StyleTitle">p.style2 { color: rgb(8,8,8); }</style>
<template id="TPL"><style id="S3">p.tpl { color: rgb(12,12,12); }</style></template>
</head><body>
<style id="S4">p.body { color: rgb(11,11,11); }</style>
<p class="a">alpha</p>
</body></html>`

func cssomRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(cssomPage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func mustCSSOM(t *testing.T, r *js.Runtime, expr string) string {
	t.Helper()
	got, err := r.Evaluate(expr)
	if err != nil {
		t.Fatalf("Evaluate(%s): %v", expr, err)
	}
	return got
}

const ownersJS = `
	var owners = function () {
		var out = [];
		for (var i = 0; i < document.styleSheets.length; i++) {
			var n = document.styleSheets[i].ownerNode;
			out.push(n.nodeName + '#' + n.id);
		}
		return out.join(',');
	};`

// qc:BR-284
func TestStyleSheetListOrdersLinksAndStylesInDocumentOrder(t *testing.T) {
	r := cssomRuntime(t)
	got := mustCSSOM(t, r, ownersJS+`
		[document.styleSheets.length, owners()].join('|')
	`)
	want := "6|LINK#L1,LINK#L2,LINK#L3,STYLE#S1,STYLE#S2,STYLE#S4"
	if got != want {
		t.Fatalf("styleSheets = %q, want %q", got, want)
	}
	last := mustCSSOM(t, r, `
		var s = document.styleSheets[document.styleSheets.length - 1];
		[s.ownerNode.id, s.cssRules.length, s.cssRules[0].selectorText].join(':')
	`)
	if last != "S4:1:p.body" {
		t.Fatalf("last sheet = %q, want the body <style> holding the p.body rule", last)
	}
}

// qc:BR-284
func TestStyleSheetHrefIsAbsoluteForLinksAndNullForStyles(t *testing.T) {
	r := cssomRuntime(t)
	got := mustCSSOM(t, r, `
		var s0 = document.styleSheets[0], s3 = document.styleSheets[3];
		[s0.href, String(s3.href), s0.href === document.getElementById('L1').href,
		 document.styleSheets[1].href === document.getElementById('L2').href].join('|')
	`)
	want := "https://example.test/dir/a.css|null|true|true"
	if got != want {
		t.Fatalf("sheet hrefs = %q, want %q", got, want)
	}
}

// qc:BR-284
func TestStyleSheetMediaReportsTheAttributeAndStaysLive(t *testing.T) {
	r := cssomRuntime(t)
	got := mustCSSOM(t, r, `
		var plain = document.styleSheets[0].media, screened = document.styleSheets[1].media;
		var before = [plain.mediaText, plain.length, String(plain.item(0)), String(plain[0]),
		              screened.mediaText, screened.length, String(screened.item(0)), String(screened[0]),
		              String(document.styleSheets[4].media.mediaText),
		              String(document.styleSheets[0].media)].join(',');
		var identity = document.styleSheets[0].media === document.styleSheets[0].media;
		document.getElementById('L1').setAttribute('media', 'screen and (min-width: 100px)');
		[before, identity, plain.mediaText, plain.length, String(plain[0])].join('|')
	`)
	want := ",0,null,undefined,screen,1,screen,screen,print,|true|" +
		"screen and (min-width: 100px)|1|screen and (min-width: 100px)"
	if got != want {
		t.Fatalf("media surface = %q, want %q", got, want)
	}
}

// qc:BR-284
func TestStyleSheetTitleIsNullUnlessTheAttributeIsSet(t *testing.T) {
	r := cssomRuntime(t)
	got := mustCSSOM(t, r, `
		var titled = 'absent', plain = 'absent';
		for (var i = 0; i < document.styleSheets.length; i++) {
			var s = document.styleSheets[i];
			if (s.ownerNode.id === 'L3') titled = String(s.title);
			if (s.ownerNode.id === 'L1') plain = String(s.title);
		}
		var st = document.createElement('style');
		st.setAttribute('title', 'StyleTitle');
		st.textContent = 'p.st { color: rgb(13,13,13) }';
		document.head.appendChild(st);
		var appended = 'absent';
		for (var j = 0; j < document.styleSheets.length; j++) {
			if (document.styleSheets[j].ownerNode === st) appended = String(document.styleSheets[j].title);
		}
		[titled, plain, appended, document.styleSheets[0].type, document.styleSheets[3].type].join('|')
	`)
	if got != "Alternate|null|StyleTitle|text/css|text/css" {
		t.Fatalf("title/type surface = %q", got)
	}
}

// qc:BR-284
func TestSheetDisabledFlagReadsBackWithoutTouchingTheAttribute(t *testing.T) {
	r := cssomRuntime(t)
	got := mustCSSOM(t, r, `
		var s = document.styleSheets[0], link = document.getElementById('L1');
		var initial = s.disabled;
		s.disabled = true;
		var on = [s.disabled, document.styleSheets.length, document.styleSheets[0] === s,
		          String(link.getAttribute('disabled'))].join(',');
		s.disabled = false;
		[initial, on, s.disabled, document.styleSheets[0] === s].join('|')
	`)
	want := "false|true,6,true,null|false|true"
	if got != want {
		t.Fatalf("disabled flag = %q, want %q", got, want)
	}
}

// qc:BR-284
func TestLinkAndStyleSheetIdentityMatchesTheListEntry(t *testing.T) {
	r := cssomRuntime(t)
	got := mustCSSOM(t, r, `
		var byId = function (id) {
			for (var i = 0; i < document.styleSheets.length; i++) {
				if (document.styleSheets[i].ownerNode.id === id) return document.styleSheets[i];
			}
			return 'none';
		};
		var tplStyle = document.getElementById('TPL').content.getElementById('S3');
		[document.getElementById('L1').sheet === byId('L1'),
		 document.getElementById('S1').sheet === byId('S1'),
		 document.getElementById('L2').sheet === byId('L2'),
		 String(document.getElementById('L4').sheet),
		 String(document.getElementById('L5').sheet),
		 String(tplStyle.sheet)].join(',')
	`)
	if got != "true,true,true,null,null,null" {
		t.Fatalf("element .sheet surface = %q, want three true and three null", got)
	}
}

// qc:BR-284
func TestStyleSheetListIsLiveForAppendRemoveAndDisable(t *testing.T) {
	r := cssomRuntime(t)
	got := mustCSSOM(t, r, ownersJS+`
		var start = [document.styleSheets.length, owners()].join(':');
		document.getElementById('L4').disabled = false;
		var enabled = [document.styleSheets.length, owners(),
		               Array.prototype.indexOf.call(document.styleSheets, document.getElementById('L4').sheet)].join(':');
		document.getElementById('L1').disabled = true;
		var disabled = [document.styleSheets.length, owners()].join(':');
		document.head.removeChild(document.getElementById('S1'));
		var removed = [document.styleSheets.length, owners()].join(':');
		[start, enabled, disabled, removed].join('|')
	`)
	want := "6:LINK#L1,LINK#L2,LINK#L3,STYLE#S1,STYLE#S2,STYLE#S4" +
		"|7:LINK#L1,LINK#L2,LINK#L3,LINK#L4,STYLE#S1,STYLE#S2,STYLE#S4:3" +
		"|6:LINK#L2,LINK#L3,LINK#L4,STYLE#S1,STYLE#S2,STYLE#S4" +
		"|5:LINK#L2,LINK#L3,LINK#L4,STYLE#S2,STYLE#S4"
	if got != want {
		t.Fatalf("live list = %q, want %q", got, want)
	}
}

// qc:BR-284
func TestStyleSheetAndRuleObjectsKeepIdentityAcrossReads(t *testing.T) {
	r := cssomRuntime(t)
	got := mustCSSOM(t, r, `
		var s = document.styleSheets[3];
		var rule = s.cssRules[0];
		[document.styleSheets[3] === document.styleSheets.item(3),
		 s.cssRules[0] === s.cssRules.item(0),
		 s.cssRules[0] === document.styleSheets[3].cssRules[0],
		 rule.parentStyleSheet === s,
		 rule.parentStyleSheet === document.styleSheets[3],
		 String(document.styleSheets.item(-1)),
		 String(document.styleSheets[document.styleSheets.length]),
		 rule.type, s.cssRules.length].join(',')
	`)
	want := "true,true,true,true,true,null,undefined,1,1"
	if got != want {
		t.Fatalf("identity surface = %q, want %q", got, want)
	}
}

// qc:BR-284
func TestAdoptedStyleSheetIsNotListed(t *testing.T) {
	r := cssomRuntime(t)
	got := mustCSSOM(t, r, `
		var before = document.styleSheets.length;
		var cs = new CSSStyleSheet();
		cs.replaceSync('p.adopted { color: rgb(9,9,9); }');
		document.adoptedStyleSheets = [cs];
		[before, document.adoptedStyleSheets.length, document.styleSheets.length].join(',')
	`)
	if got != "6,1,6" {
		t.Fatalf("adopted sheet surface = %q, want the list unchanged at 6", got)
	}
}
