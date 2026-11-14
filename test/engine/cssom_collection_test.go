package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

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
