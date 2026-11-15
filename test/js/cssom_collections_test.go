package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

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

const shapeJS = `
	var list = document.styleSheets;
	var c1 = list[0], c2 = list[1], a1 = list[2];
	var rules = a1.cssRules, media = c1.media;
	function err(f) { try { var v = f(); return v === undefined ? 'undefined' : String(v) } catch (e) { return e.constructor.name + ': ' + e.message } }
	function getter(iface, prop) { return Object.getOwnPropertyDescriptor(window[iface].prototype, prop).get }
	function names(o) { return Object.getOwnPropertyNames(o).join('+') }
	function tagged(name) { var fake = { length: 5 }; fake[1] = 'x'; fake[Symbol.toStringTag] = name; return fake }
`

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
	want := "length+item+constructor | length+item+constructor | length+mediaText+appendMedium+deleteMedium+item+toString+constructor | length+mediaText+appendMedium+deleteMedium+item+toString | function,undefined,true,true | function,1,true | function,true,true | function,0,true | false,true,true | false,true,true | true | true | true | true | true"
	if got != want {
		t.Errorf("CSSOM collection prototypes\ngot: %s\nwant: %s", got, want)
	}
}

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

// qc:BR-304
func TestMediaListAppendDeleteMediumMutateTheirOwnView(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		var attr = document.getElementById('C1');
		var out = [];
		out.push('start:' + media.mediaText + '|' + media.length);
		out.push('append_ret:' + String(media.appendMedium('handheld')));
		out.push('after_append:' + media.mediaText + '|' + media.length + '|' + media.item(2) + '|attr=' + attr.getAttribute('media'));
		media.appendMedium('screen');
		out.push('dup:' + media.mediaText + '|' + media.length);
		media.appendMedium('');
		out.push('empty_append:' + media.mediaText + '|' + media.length);
		out.push('del_ret:' + String(media.deleteMedium('print')));
		out.push('after_del:' + media.mediaText + '|' + media.length);
		try { media.deleteMedium('nope'); out.push('missing:nothrow'); } catch (e) { out.push('missing:' + e.name); }
		media.deleteMedium('SCREEN');
		out.push('del_case:' + media.mediaText + '|' + media.length + '|attr=' + attr.getAttribute('media'));
		out.push('del_empty:' + String(media.deleteMedium('')) + '|' + media.mediaText);
		out.join(' || ')
	`)
	want := "start:screen, print|2 || append_ret:undefined || after_append:screen, print, handheld|3|handheld|attr=screen, print || dup:screen, print, handheld|3 || empty_append:screen, print, handheld|3 || del_ret:undefined || after_del:screen, handheld|2 || missing:NotFoundError || del_case:handheld|1|attr=screen, print || del_empty:undefined|handheld"
	if got != want {
		t.Errorf("MediaList appendMedium/deleteMedium\ngot: %s\nwant: %s", got, want)
	}
}

// qc:BR-304
func TestMediaListMediumMutationsMatchChromiumShape(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		var attr = document.getElementById('C1');
		var a2 = list[3].media;
		var out = [];
		media.appendMedium('tv');
		out.push('stringify:' + String(media));
		attr.setAttribute('media', 'aural');
		out.push('after_setattr:' + media.mediaText + '|' + media.length + '|names=' + names(media));
		out.push('a2_start:' + a2.mediaText + '|' + a2.length);
		a2.appendMedium('print');
		out.push('a2_dup:' + a2.mediaText + '|' + a2.length);
		out.push('proto_names:' + names(MediaList.prototype));
		out.push('arity:' + MediaList.prototype.appendMedium.length + ',' + MediaList.prototype.deleteMedium.length + ',' + MediaList.prototype.appendMedium.name);
		out.push(err(function () { return MediaList.prototype.appendMedium.call({}); }));
		out.push(err(function () { return media.appendMedium(); }));
		out.push(err(function () { return MediaList.prototype.deleteMedium.call(rules, 'x'); }));
		out.join(' || ')
	`)
	want := "stringify:screen, print, tv || after_setattr:aural|1|names=0 || a2_start:print|1 || a2_dup:print|1 || proto_names:length+mediaText+appendMedium+deleteMedium+item+toString+constructor || arity:1,1,appendMedium || TypeError: Illegal invocation || TypeError: Failed to execute 'appendMedium' on 'MediaList': 1 argument required, but only 0 present. || TypeError: Illegal invocation"
	if got != want {
		t.Errorf("MediaList mutation shape\ngot: %s\nwant: %s", got, want)
	}
}

// qc:BR-305
func TestMediaListMediaTextSetterWritesAnOverrideNotTheAttribute(t *testing.T) {
	r := cssomColRuntime(t)
	got := mustCSSOM(t, r, shapeJS+`
		var attr = document.getElementById('C1');
		var d = Object.getOwnPropertyDescriptor(MediaList.prototype, 'mediaText');
		var out = [];
		out.push('desc:' + typeof d.get + ',' + typeof d.set + ',' + d.enumerable + ',' + d.configurable);
		media.mediaText = 'aural, print';
		out.push('set_spaced:' + media.mediaText + '|' + media.length + '|' + media.item(1) + '|attr=' + attr.getAttribute('media'));
		media.mediaText = 'tv,radio,print';
		out.push('set_nospace:' + media.mediaText + '|' + media.length);
		media.mediaText = '   ';
		out.push('set_ws:' + media.mediaText + '|' + media.length);
		media.mediaText = '';
		out.push('set_empty:' + media.mediaText + '|' + media.length);
		media.mediaText = 'screen, print';
		media.appendMedium('tv');
		out.push('append_after_set:' + media.mediaText + '|' + media.length + '|attr=' + attr.getAttribute('media'));
		attr.setAttribute('media', 'all');
		out.push('attr_clears:' + media.mediaText + '|' + media.length);
		out.push(err(function () { return Object.getOwnPropertyDescriptor(MediaList.prototype, 'mediaText').set.call({}, 'x'); }));
		out.join(' || ')
	`)
	want := "desc:function,function,true,true || set_spaced:aural, print|2|print|attr=screen, print || set_nospace:tv, radio, print|3 || set_ws:|0 || set_empty:|0 || append_after_set:screen, print, tv|3|attr=screen, print || attr_clears:all|1 || TypeError: Illegal invocation"
	if got != want {
		t.Errorf("MediaList mediaText setter\ngot: %s\nwant: %s", got, want)
	}
}
