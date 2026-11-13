package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

// GOOSIE-CSSOMCOLLECTION-001, the half that needs a fetched sheet. This engine
// rebuilt a sheet's CSSRuleList on every read, so `sheet.cssRules ===
// sheet.cssRules` was false, and the lists had no interface at all: no
// StyleSheetList / CSSRuleList / MediaList brand, a detached
// CSSRuleList.prototype.item working on any object, and own length/item
// properties where Chromium owns only the indices.
//
// The expectations are Chromium 153.0.8010.12 numbers (the Node Playwright
// 1.63.0 oracle) measured on this exact document (2026-11-13, viewport
// 1280x400, DPR 1, served over http; dumps /tmp/b287/cr_colA.json and
// cr_colB.json, probes index10.html) and re-run on the pinned 131.0.6778.33
// render oracle, which answered these two the same way, read here against the
// same rule text: LINK#L1 -> a.css with media="screen, print" and
// LINK#L2 -> b.css with no media attribute, then the two inline sheets. Only
// rule counts and selector text are compared, since the colours in the served
// sheets differ from the fixture's.

const cssomColDoc = `<html><head>
<link id="L1" rel="stylesheet" href="/a.css" media="screen, print" title="Alternate">
<link id="L2" rel="stylesheet" href="/b.css">
<style id="S1">p.a { color: rgb(10,0,0); }</style>
<style id="S2" media="print">p.b { color: rgb(11,0,0); }</style>
</head><body><p class="a" id="P">alpha</p></body></html>`

func cssomColSession(t *testing.T) *engine.Session {
	t.Helper()
	srv := newCSSOMServer()
	srv.text["https://example.test/b.css"] = ".b { color: rgb(0,10,0); }"
	rt := newCSSRuntime(t)
	return newCSSSession(t, rt, cssomColDoc, srv.link)
}

// One CSSRuleList per fetched sheet, held by the sheet and branded. Measured:
// the list is the same object on every read, a second sheet's list is a
// different object, the rule wrapper answers identically through the index and
// through item(), the two collections stringify as their interfaces, and both
// detached-member and arity failures throw the errors Chromium throws. The
// MediaList is the owner's media attribute: 2 entries for "screen, print", and
// for the link with no attribute an empty text and 0 entries - not "all".
//
// qc:BR-285
func TestFetchedSheetHoldsOneCSSRuleList(t *testing.T) {
	sess := cssomColSession(t)
	cssomJS(t, sess, `
		var s = document.styleSheets;
		var r = s[0].cssRules, r2 = s[1].cssRules;
		function err(f) { try { var v = f(); return v === undefined ? 'undefined' : String(v) } catch (e) { return e.constructor.name + ': ' + e.message } }
		[
			String(r === s[0].cssRules),
			String(r !== r2),
			String(r[0] === r.item(0)),
			Object.prototype.toString.call(r),
			Object.prototype.toString.call(s),
			String(r.length), String(r2.length),
			err(function () { return CSSRuleList.prototype.item.call({ length: 3 }, 1); }),
			err(function () { return r.item(); }),
			String(s[0].media) + ',' + s[0].media.length,
			String(s[1].media) + ',' + s[1].media.length,
			err(function () { return MediaList.prototype.toString.call(r); })
		].join(' | ')
	`, "true | true | true | [object CSSRuleList] | [object StyleSheetList] | 1 | 1 | "+
		"TypeError: Illegal invocation | "+
		"TypeError: Failed to execute 'item' on 'CSSRuleList': 1 argument required, but only 0 present. | "+
		"screen, print,2 | ,0 | TypeError: Illegal invocation")
}

// A CSSRuleList is a snapshot of the sheet's text while a StyleSheetList and a
// MediaList stay live through one object. Measured: the handle read twice is one
// object and holds 1 rule, and after `S1.textContent` is replaced with two rules
// `s[2].cssRules === before` is false and the fresh read reports 2 rules with the
// new selectors - while the handle the page already holds still reports its 1
// old rule and `before[0] === after[0]` is false. The untouched link's list keeps
// its identity and its 1 rule in the same task.
//
// The `#P` colour is not part of this oracle and not evidence about anyone's
// cascade: on this document a.css matches `p.a` too, so both engines answer
// rgb(10, 0, 0) before and after the replacement for that reason. Measured on a
// link-free document (/tmp/b287/index12.html, cr_col12.txt vs go_col12.txt)
// Chromium drops a removed rule synchronously - `#P` reads rgb(0, 0, 0) and a
// newly added one computes rgb(9, 9, 9) for a matching element - where this
// engine keeps rgb(10, 0, 0) and answers rgb(0, 0, 0). That is the cascade half of
// GOOSIE-STYLETEXTRULES-001, registered rather than asserted here.
//
// qc:BR-285
func TestFetchedSheetRuleListSurvivesAStyleChange(t *testing.T) {
	sess := cssomColSession(t)
	cssomJS(t, sess, `
		var s = document.styleSheets;
		var before = s[2].cssRules;
		var sameTwice = String(before === s[2].cssRules);
		var beforeDesc = before.length + ',' + String(before[0].selectorText);
		document.getElementById('S1').textContent = 'p.z { color: rgb(20,0,0); } q.z { color: rgb(21,0,0); }';
		var after = s[2].cssRules;
		[
			sameTwice,
			beforeDesc,
			String(after === before),
			String(after.length),
			String(after[0].selectorText),
			String(after[1].selectorText),
			before.length + ',' + String(before[0] === after[0]) + ',' + String(before[0].selectorText),
			String(s[0].cssRules === s[0].cssRules) + ',' + s[0].cssRules.length
		].join(' | ')
	`, "true | 1,p.a | false | 2 | p.z | q.z | 1,false,p.a | true,1")
}

// The same law across the four shapes of "edit a <style>'s text", measured on a
// document with no links at all so the sheet under edit is `styleSheets[0]`
// (/tmp/b287/index12.html; cr_col12.txt against go_col12.txt, 2026-11-13). In
// every shape a read made after the edit sees the edited rules and the handle
// held across the edit keeps the rules it was built from, whether the edit added
// a rule (`append`) or dropped one (`shrink`).
//
// The two probes Chromium also answered colours for - `#P` after a replacement
// and a freshly appended element matching a newly added rule - are left out of
// these expectations: this engine reports rgb(10, 0, 0) and rgb(0, 0, 0) where
// Chromium reports rgb(0, 0, 0) and rgb(9, 9, 9), which is the cascade half of
// GOOSIE-STYLETEXTRULES-001.
//
// qc:BR-285
func TestStyleTextEditReparseSucceedingReads(t *testing.T) {
	const doc = `<html><head><style id="S1">p.a { color: rgb(10,0,0); } p.b { color: rgb(0,10,0); }</style></head><body><p class="a" id="P">alpha</p></body></html>`
	shapes := []struct{ name, probe, want string }{
		{"replace, no prior read", `
			document.getElementById('S1').textContent = 'p.z { color: rgb(20,0,0); } q.z { color: rgb(21,0,0); }';
			var s = document.styleSheets;
			[s[0].cssRules.length, String(s[0].cssRules[0].selectorText)].join(',')`,
			"2,p.z"},
		{"append after a read", `
			var s = document.styleSheets;
			var before = s[0].cssRules;
			var first = before.length + ',' + String(before[0].selectorText);
			document.getElementById('S1').textContent += ' p.d { color: rgb(9,9,9); }';
			var after = s[0].cssRules;
			[first, String(after.length), String(after === before), String(before.length)].join(' | ')`,
			"2,p.a | 3 | false | 2"},
		{"append, no prior read", `
			document.getElementById('S1').textContent += ' p.d { color: rgb(9,9,9); }';
			var s = document.styleSheets;
			[s[0].cssRules.length, String(s[0].cssRules[0].selectorText)].join(',')`,
			"3,p.a"},
		{"shrink after a read", `
			var s = document.styleSheets;
			var before = s[0].cssRules;
			var first = before.length + ',' + String(before[0].selectorText);
			document.getElementById('S1').textContent = 'p.z { color: rgb(20,0,0); }';
			var after = s[0].cssRules;
			[first, String(after === before), String(after.length), String(after[0].selectorText),
			 String(before.length), String(after[0] === before[0])].join(' | ')`,
			"2,p.a | false | 1 | p.z | 2 | false"},
	}
	for _, shape := range shapes {
		rt := newCSSRuntime(t)
		sess := newCSSSession(t, rt, doc, newCSSOMServer().link)
		cssomJS(t, sess, shape.probe, shape.want)
	}
}
