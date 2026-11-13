package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// GOOSIE-CSSOMCOLLECTION-001: the three CSSOM collections had no interface of
// their own. document.styleSheets, sheet.cssRules and sheet.media answered as
// anonymous indexed objects with own `length` and `item` properties, so
// Object.prototype.toString.call(list) named nothing, `list instanceof
// StyleSheetList` was false, a detached CSSRuleList.prototype.item worked on any
// object at all, and sheet.cssRules was rebuilt on every read, so
// `sheet.cssRules === sheet.cssRules` was false.
//
// Every expectation is read off the Chromium 153.0.8010.12 measurements (the
// Node Playwright 1.63.0 oracle; 2026-11-13, one http fixture, viewport
// 1280x400, DPR 1, dumps /tmp/b287/cr_col.json through cr_col12.txt plus
// cr_colA/B/C) quoted in internal/js/cssom_collections.go; re-running the same
// 14 probes on the pinned 131.0.6778.33 render oracle agreed on 12 of them. The
// page below is the same document order measured there - two links, then the
// head's style elements, then the body's - with one rule per style element, and it is
// what the dumps measured: the C1 link is the one carrying media="screen, print"
// (MediaList of 2), C2 has no media attribute (the empty list, not "all"), and
// A1/A2/A3 each own a one-rule sheet.

const cssomColPage = `<!doctype html><html><head>
<link id="C1" rel="stylesheet" href="a.css" media="screen, print" title="Alternate">
<link id="C2" rel="stylesheet" href="b.css">
<style id="A1">p.a1 { color: rgb(10,0,0); }</style>
<style id="A2" media="print">p.a2 { color: rgb(11,0,0); }</style>
</head><body>
<style id="A3">p.a3 { color: rgb(12,0,0); }</style>
<p class="a1">alpha</p>
</body></html>`

func cssomColRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	r, err := js.New(js.Options{
		DOM:     domtest.Parse(cssomColPage),
		URL:     "https://example.test/dir/index.html",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// shapeJS declares the probes several of these oracles share. This runtime has
// no network, so the linked sheets own no rules; the rule-list laws read the
// inline A1 sheet, whose one rule Chromium measured the same way.
const shapeJS = `
	var list = document.styleSheets;
	var c1 = list[0], c2 = list[1], a1 = list[2];
	var rules = a1.cssRules, media = c1.media;
	function err(f) { try { var v = f(); return v === undefined ? 'undefined' : String(v) } catch (e) { return e.constructor.name + ': ' + e.message } }
	function getter(iface, prop) { return Object.getOwnPropertyDescriptor(window[iface].prototype, prop).get }
	function names(o) { return Object.getOwnPropertyNames(o).join('+') }
	function tagged(name) { var fake = { length: 5 }; fake[1] = 'x'; fake[Symbol.toStringTag] = name; return fake }
`

// The three interfaces exist as globals, brand their lists, and sit one link
// above Object.prototype. Measured: Object.prototype.toString.call gives
// [object StyleSheetList] / [object CSSRuleList] / [object MediaList]; instanceof
// and isPrototypeOf answer true against each global; the globals are functions of
// length 0 whose call is `TypeError: Illegal constructor`; the own symbols of a
// prototype are Symbol.toStringTag and Symbol.iterator.
//
// Only the call form is asserted. Measured just after (cr_colC.txt), Chromium
// answers `new CSSRuleList()` with `TypeError: Failed to construct 'CSSRuleList':
// Illegal constructor` - the bare message is reserved for a call - while this
// engine's house style is the bare message both ways, in every interface in
// internal/js. That wording is GOOSIE-CTORMESSAGE-001, so it is deliberately not
// pinned here.
//
// qc:BR-285
func TestCSSOMCollectionsBrandTheirInterfaces(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		[
			Object.prototype.toString.call(list),
			Object.prototype.toString.call(rules),
			Object.prototype.toString.call(media),
			String(list instanceof StyleSheetList),
			String(rules instanceof CSSRuleList),
			String(media instanceof MediaList),
			String(StyleSheetList.prototype.isPrototypeOf(list)),
			String(CSSRuleList.prototype.isPrototypeOf(rules)),
			String(MediaList.prototype.isPrototypeOf(media)),
			[typeof StyleSheetList, StyleSheetList.length, typeof CSSRuleList, CSSRuleList.length, typeof MediaList, MediaList.length].join(','),
			err(function () { return CSSRuleList(); }),
			err(function () { return StyleSheetList(); }),
			Object.getPrototypeOf(Object.getPrototypeOf(rules)).constructor.name,
			Object.getOwnPropertySymbols(MediaList.prototype).map(function (x) { return x.description; }).join('+'),
			[typeof list[Symbol.iterator], typeof rules[Symbol.iterator], typeof media[Symbol.iterator]].join(',')
		].join(' | ')
	`)
	want := "[object StyleSheetList] | [object CSSRuleList] | [object MediaList] | true | true | true | true | true | true | function,0,function,0,function,0 | TypeError: Illegal constructor | TypeError: Illegal constructor | Object | Symbol.toStringTag+Symbol.iterator | function,function,function"
	if got != want {
		t.Errorf("CSSOM interface brands\n got: %s\nwant: %s", got, want)
	}
}

// The prototypes own exactly the members Chromium's do, with the measured
// descriptors: length is a getter-only enumerable configurable accessor with no
// setter, item a data function of arity 1, MediaList adds a mediaText accessor
// and a toString of arity 0, and the constructor stays non-enumerable. The
// methods keep one identity per interface, so `rules.item` is
// CSSRuleList.prototype.item on every read.
//
// qc:BR-285
func TestCSSOMCollectionPrototypesOwnTheMeasuredMembers(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		var ld = Object.getOwnPropertyDescriptor(CSSRuleList.prototype, 'length');
		var id = Object.getOwnPropertyDescriptor(CSSRuleList.prototype, 'item');
		var md = Object.getOwnPropertyDescriptor(MediaList.prototype, 'mediaText');
		var td = Object.getOwnPropertyDescriptor(MediaList.prototype, 'toString');
		var it = Object.getOwnPropertyDescriptor(CSSRuleList.prototype, Symbol.iterator);
		var cd = Object.getOwnPropertyDescriptor(StyleSheetList.prototype, 'constructor');
		[
			names(CSSRuleList.prototype),
			names(StyleSheetList.prototype),
			names(MediaList.prototype),
			Object.keys(MediaList.prototype).join('+'),
			[typeof ld.get, String(ld.set), String(ld.enumerable), String(ld.configurable)].join(','),
			[typeof id.value, id.value.length, String(id.enumerable)].join(','),
			[typeof md.get, String(md.enumerable), String(md.configurable)].join(','),
			[typeof td.value, td.value.length, String(td.enumerable)].join(','),
			[it ? String(it.enumerable) + ',' + String(it.writable) + ',' + String(it.configurable) : 'none'],
			[String(cd.enumerable), String(cd.writable), String(cd.configurable)].join(','),
			String(rules.item === CSSRuleList.prototype.item),
			String(rules.item === rules.item),
			String(list.item === StyleSheetList.prototype.item),
			String(media.item === MediaList.prototype.item),
			String(media.toString === MediaList.prototype.toString)
		].join(' | ')
	`)
	want := "length+item+constructor | length+item+constructor | length+mediaText+item+toString+constructor | length+mediaText+item+toString | function,undefined,true,true | function,1,true | function,true,true | function,0,true | false,true,true | false,true,true | true | true | true | true | true"
	if got != want {
		t.Errorf("CSSOM collection prototypes\ngot: %s\nwant: %s", got, want)
	}
}

// A collection owns its indices and nothing else: measured, the StyleSheetList
// of three sheets gives own names "0+1+2", a one-rule list "0", a two-entry
// MediaList "0+1", and Object.keys answers the same because length and item are
// inherited. for-in still walks indices first, then the enumerable prototype
// members, and for-of yields the members.
//
// qc:BR-285
func TestCSSOMCollectionsOwnOnlyTheirIndices(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		var forIn = [];
		for (var k in list) forIn.push(k);
		var forRules = [];
		for (var k2 in rules) forRules.push(k2);
		var spread = [];
		for (var x of list) spread.push(x.ownerNode.id);
		var spreadRules = [];
		for (var r2 of rules) spreadRules.push(r2.type);
		var spreadMedia = [];
		for (var q of media) spreadMedia.push(q);
		[
			names(list),
			Object.keys(list).join('+'),
			names(rules),
			names(media),
			String('length' in list),
			String(list.hasOwnProperty('length')),
			String(list.hasOwnProperty('0')),
			forIn.join('+'),
			forRules.join('+'),
			spread.join(','),
			spreadRules.join(','),
			spreadMedia.join('|')
		].join(' | ')
	`)
	want := "0+1+2+3+4 | 0+1+2+3+4 | 0 | 0+1 | true | false | true | 0+1+2+3+4+length+item | 0+length+item | C1,C2,A1,A2,A3 | 1 | screen|print"
	if got != want {
		t.Errorf("CSSOM collection own names\ngot: %s\nwant: %s", got, want)
	}
}

// The receiver test is an identity test. Measured: the length getter, item,
// mediaText and toString all answer `TypeError: Illegal invocation` for `{}`,
// `[]`, `{ length: 3 }`, `{ length: 3, 1: 'x' }`, an object carrying the
// interface's own @@toStringTag, and a list handed to a *different* interface's
// member - while called on a real list the same detached members answer live
// values: the length getter gives the rule count and the media text is readable.
//
// qc:BR-285
func TestCSSOMCollectionMembersRefuseForeignReceivers(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		[
			err(function () { return getter('CSSRuleList', 'length').call({}); }),
			err(function () { return getter('CSSRuleList', 'length').call([]); }),
			err(function () { return getter('CSSRuleList', 'length').call({ length: 5 }); }),
			err(function () { return getter('CSSRuleList', 'length').call(tagged('CSSRuleList')); }),
			err(function () { return getter('CSSRuleList', 'length').call(media); }),
			err(function () { return getter('StyleSheetList', 'length').call(rules); }),
			err(function () { return CSSRuleList.prototype.item.call({}); }),
			err(function () { return CSSRuleList.prototype.item.call({ length: 3 }, 1); }),
			err(function () { return CSSRuleList.prototype.item.call(tagged('CSSRuleList'), 1); }),
			err(function () { return CSSRuleList.prototype.item.call(); }),
			err(function () { return StyleSheetList.prototype.item.call(media, 0); }),
			err(function () { return getter('MediaList', 'mediaText').call({ length: 0 }); }),
			err(function () { return MediaList.prototype.toString.call({ mediaText: 'screen' }); }),
			err(function () { return MediaList.prototype.toString.call(rules); }),
			err(function () { return getter('StyleSheetList', 'length').call(list); }),
			err(function () { return getter('CSSRuleList', 'length').call(rules); }),
			err(function () { return getter('MediaList', 'length').call(media); }),
			err(function () { return getter('MediaList', 'mediaText').call(media); }),
			err(function () { return CSSRuleList.prototype.item.call(rules, 1); }),
			err(function () { return CSSRuleList.prototype.item.call(rules, 0).selectorText }),
			err(function () { return CSSRuleList.prototype.item.call(null, 0); }),
			err(function () { return CSSRuleList.prototype.item.call('abc', 1); }),
			err(function () { return getter('CSSRuleList', 'length').call(); }),
			err(function () { return MediaList.prototype.toString.call(); })
		].join(' | ')
	`)
	want := "TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | 5 | 1 | 2 | screen, print | null | p.a1 | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation | TypeError: Illegal invocation"
	if got != want {
		t.Errorf("CSSOM collection receiver law\ngot: %s\nwant: %s", got, want)
	}
}

// item() takes its index the way Chromium's bindings do. Measured: no argument
// at all is `TypeError: Failed to execute 'item' on 'StyleSheetList': 1 argument
// required, but only 0 present.` - and the same sentence names whichever
// interface was called; a Symbol argument gives the same sentence ending
// `Cannot convert a Symbol value to a number`; the receiver is checked before
// either; undefined, null, NaN, `{}` and a non-numeric string all give the first
// member; 1.7 truncates to the second; and past the end or below zero is null.
//
// qc:BR-285
func TestCSSOMCollectionItemConvertsItsIndex(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		function which(v) { return err(function () { var x = list.item(v); return x === undefined ? 'undefined' : (x === null ? 'null' : x.ownerNode.id) }) }
		[
			which(undefined), which(null), which(NaN), which({}), which('x'),
			which('1'), which(1.7), which(-0), which(1e10), which(9), which(-1),
			which(Symbol('i')),
			String(list.item(0) === list[0]),
			String(rules.item(0) === rules[0]),
			String(media.item(0)),
			String(media.item(1)),
			String(media.item(2)),
			err(function () { return rules.item(); })
		].join(' | ')
	`)
	want := "C1 | C1 | C1 | C1 | C1 | C2 | C2 | C1 | null | null | null | TypeError: Failed to execute 'item' on 'StyleSheetList': Cannot convert a Symbol value to a number | true | true | screen | print | null | TypeError: Failed to execute 'item' on 'CSSRuleList': 1 argument required, but only 0 present."
	if got != want {
		t.Errorf("CSSOM collection item()\ngot: %s\nwant: %s", got, want)
	}
}

// A MediaList is the owner's media attribute read now. Measured: media="screen,
// print" gives length 2, [0] "screen" and item(1) "print", own names "0+1" and an
// index descriptor that is non-writable, enumerable and configurable; a sheet
// with no media attribute - link or style element alike - gives the empty text,
// length 0, undefined at [0], null at item(0) and no own index at all, not "all";
// and String(media) and media.toString() are the mediaText.
//
// qc:BR-285
func TestMediaListReportsTheOwnersAttribute(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		var plain = c2.media, none = a1.media;
		var d = Object.getOwnPropertyDescriptor(media, '0');
		[
			media.mediaText, media.length, String(media[0]), String(media.item(0)), names(media),
			String(d.writable) + ',' + String(d.enumerable) + ',' + String(d.configurable), String(media[2]),
			plain.mediaText, plain.length, String(plain[0]), String(plain.item(0)), names(plain),
			String(Object.getOwnPropertyDescriptor(plain, '0')),
			none.mediaText, none.length, names(none),
			String(media), media.toString(), String(plain), String(plain.toString())
		].join(' | ')
	`)
	want := "screen, print | 2 | screen | screen | 0+1 | false,true,true | undefined |  | 0 | undefined | null |  | undefined |  | 0 |  | screen, print | screen, print |  | "
	if got != want {
		t.Errorf("MediaList media attribute law\ngot: %s\nwant: %s", got, want)
	}
}

// The lists stay live through the one object a page holds. Measured: after
// `L1.setAttribute("media", "screen and (min-width: 100px)")` the MediaList
// already in hand reports the new text, length 1 and one entry holding the whole
// query - a comma inside parentheses does not split it - with own names "0"; and
// after appending a <style> the StyleSheetList already in hand has one more
// entry, the new sheet answers a null href and an empty mediaText, and item(n)
// is the very object at [n].
//
// The two reference builds split on exactly this law: on 153.0.8010.12 the
// handle in hand answers the new text and a fresh read of `sheet.media` is the
// same object, while on the pinned 131.0.6778.33 render oracle the handle keeps
// the old two-entry text and a fresh read is a different MediaList (`=== m`
// false). This oracle pins the live reading, which this engine implements.
//
// qc:BR-285
func TestCSSOMCollectionsStayLiveThroughOneObject(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		var held = c1.media;
		c1.ownerNode.setAttribute('media', 'screen and (min-width: 100px), print');
		var afterAttr = [held.mediaText, held.length, String(held[0]), String(held[1]), String(held), names(held)].join(',');
		var st = document.createElement('style');
		st.id = 'A4'; st.textContent = 'p.a4 { color: rgb(13,13,13); }';
		document.body.appendChild(st);
		var afterAppend = [list.length, String(list[5].href), String(list[5].ownerNode.id), list[5].media.mediaText, list[5].media.length,
			String(list.item(5) === list[5]), names(list), String(list[5].cssRules.length), String(list[5].cssRules === list[5].cssRules)].join(',');
		[afterAttr, afterAppend].join(' | ')
	`)
	want := "screen and (min-width: 100px), print,2,screen and (min-width: 100px),print,screen and (min-width: 100px), print,0+1 | 6,null,A4,,0,true,0+1+2+3+4+5,1,true"
	if got != want {
		t.Errorf("CSSOM collection liveness\ngot: %s\nwant: %s", got, want)
	}
}

// One list per sheet, cached against the sheet. Measured: sheet.cssRules is the
// same object on every read while another sheet's list is a different object,
// the same holds for sheet.media, the rule a page reaches twice is the same
// object either way, and document.styleSheets hands out one list per document.
//
// qc:BR-285
func TestCSSOMCollectionsAreCachedPerOwner(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		[
			String(rules === a1.cssRules),
			String(rules !== list[3].cssRules),
			String(media === c1.media),
			String(media !== c2.media),
			String(rules[0] === rules.item(0)),
			String(rules[0] === a1.cssRules[0]),
			String(list === document.styleSheets),
			String(list[0].media === c1.media)
		].join(',')
	`)
	want := "true,true,true,true,true,true,true,true"
	if got != want {
		t.Errorf("CSSOM collection identity law\ngot: %s\nwant: %s", got, want)
	}
}
