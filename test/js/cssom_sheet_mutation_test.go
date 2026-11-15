package js_test

import (
	"testing"
)

const cssomSheetMutationJS = `
	function nerr(f) { try { return String(f()) } catch (e) { return e.name + ': ' + e.message } }
`

// qc:BR-310
func TestSheetInsertRulePositionsAndReturn(t *testing.T) {
	r := ruleStyleRuntime(t)
	got := mustCSSOM(t, r, cssomSheetMutationJS+`
		var sheet = document.styleSheets[0];
		var out = [];
		out.push('len0=' + sheet.cssRules.length);
		out.push('d_ret=' + nerr(function () { return sheet.insertRule('p.zz { color: rgb(50, 50, 50); }'); }));
		out.push('d_first=' + sheet.cssRules[0].selectorText);
		out.push('d_last=' + sheet.cssRules[sheet.cssRules.length - 1].selectorText);
		out.push('d_at2=' + nerr(function () { return sheet.insertRule('p.yy { color: rgb(60, 60, 60); }', 2); }));
		out.push('d_idx2=' + sheet.cssRules[2].selectorText);
		out.push('d_atlen=' + nerr(function () { return sheet.insertRule('p.xx { color: rgb(70, 70, 70); }', sheet.cssRules.length); }));
		out.join(' | ')
	`)
	want := "len0=1 | d_ret=0 | d_first=p.zz | d_last=p.a1 | d_at2=2 | d_idx2=p.yy | d_atlen=3"
	if got != want {
		t.Errorf("insertRule positions\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-310
func TestSheetInsertRuleErrorLaws(t *testing.T) {
	r := ruleStyleRuntime(t)
	got := mustCSSOM(t, r, cssomSheetMutationJS+`
		var sheet = document.styleSheets[0];
		sheet.insertRule('p.b { color: rgb(80, 80, 80); }');
		sheet.insertRule('p.c { color: rgb(90, 90, 90); }');
		sheet.insertRule('p.d { color: rgb(95, 95, 95); }');
		var out = [];
		out.push(nerr(function () { return sheet.insertRule('p.w { color: rgb(80, 80, 80); }', sheet.cssRules.length + 1); }));
		out.push(nerr(function () { return sheet.insertRule('p.v { color: rgb(90, 90, 90); }', -1); }));
		out.push(nerr(function () { return sheet.insertRule(); }));
		out.push(nerr(function () { return sheet.insertRule(undefined); }));
		out.push(nerr(function () { return sheet.insertRule(null); }));
		out.join(' | ')
	`)
	want := "IndexSizeError: Failed to execute 'insertRule' on 'CSSStyleSheet': The index provided (5) is larger than the maximum index (4). | " +
		"IndexSizeError: Failed to execute 'insertRule' on 'CSSStyleSheet': The index provided (4294967295) is larger than the maximum index (4). | " +
		"TypeError: Failed to execute 'insertRule' on 'CSSStyleSheet': 1 argument required, but only 0 present. | " +
		"SyntaxError: Failed to execute 'insertRule' on 'CSSStyleSheet': Failed to parse the rule 'undefined'. | " +
		"SyntaxError: Failed to execute 'insertRule' on 'CSSStyleSheet': Failed to parse the rule 'null'."
	if got != want {
		t.Errorf("insertRule errors\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-310
func TestSheetDeleteRuleAndLiveList(t *testing.T) {
	r := ruleStyleRuntime(t)
	got := mustCSSOM(t, r, cssomSheetMutationJS+`
		var sheet = document.styleSheets[0];
		sheet.insertRule('p.z1 { color: rgb(50, 50, 50); }');
		sheet.insertRule('p.z2 { color: rgb(60, 60, 60); }');
		sheet.insertRule('p.z3 { color: rgb(70, 70, 70); }');
		var out = [];
		out.push('lenBeforeDel=' + sheet.cssRules.length);
		out.push('del_ret=' + nerr(function () { return sheet.deleteRule(0); }));
		out.push('del_first=' + sheet.cssRules[0].selectorText);
		out.push('del_last_ret=' + nerr(function () { return sheet.deleteRule(sheet.cssRules.length - 1); }));
		out.push('del_last=' + sheet.cssRules[sheet.cssRules.length - 1].selectorText);
		out.push(nerr(function () { return sheet.deleteRule(sheet.cssRules.length); }));
		out.push(nerr(function () { return sheet.deleteRule(-1); }));
		out.push(nerr(function () { return sheet.deleteRule(); }));
		var live = sheet.cssRules;
		out.push('identity=' + String(live === sheet.cssRules));
		out.push('liveLenBefore=' + live.length);
		sheet.insertRule('p.qq { color: rgb(11, 11, 11); }', sheet.cssRules.length);
		out.push('liveLenAfter=' + live.length);
		out.push('ruleText=' + sheet.cssRules[sheet.cssRules.length - 1].cssText);
		out.push('styleVal=' + sheet.cssRules[sheet.cssRules.length - 1].style.getPropertyValue('color'));
		out.push('srcText=' + document.getElementById('A1').textContent);
		out.join(' | ')
	`)
	want := "lenBeforeDel=4 | del_ret=undefined | del_first=p.z2 | del_last_ret=undefined | del_last=p.z1 | " +
		"IndexSizeError: Failed to execute 'deleteRule' on 'CSSStyleSheet': The index provided (2) is larger than the maximum index (1). | " +
		"IndexSizeError: Failed to execute 'deleteRule' on 'CSSStyleSheet': The index provided (4294967295) is larger than the maximum index (1). | " +
		"TypeError: Failed to execute 'deleteRule' on 'CSSStyleSheet': 1 argument required, but only 0 present. | " +
		"identity=true | liveLenBefore=2 | liveLenAfter=3 | ruleText=p.qq { color: rgb(11, 11, 11); } | styleVal=rgb(11, 11, 11) | srcText=p.a1 { color: rgb(10,0,0); }"
	if got != want {
		t.Errorf("deleteRule and live list\ngot:  %s\nwant: %s", got, want)
	}
}

// qc:BR-310
func TestSheetMutationReceiverAndDescriptor(t *testing.T) {
	r := ruleStyleRuntime(t)
	got := mustCSSOM(t, r, cssomSheetMutationJS+`
		var out = [];
		out.push('recv=' + nerr(function () { return Object.getOwnPropertyDescriptor(CSSStyleSheet.prototype, 'insertRule').value.call({}); }));
		out.push('delRecv=' + nerr(function () { return Object.getOwnPropertyDescriptor(CSSStyleSheet.prototype, 'deleteRule').value.call({}); }));
		var ins = Object.getOwnPropertyDescriptor(CSSStyleSheet.prototype, 'insertRule');
		var del = Object.getOwnPropertyDescriptor(CSSStyleSheet.prototype, 'deleteRule');
		out.push('insDesc=' + [typeof ins.value, String(ins.writable), String(ins.enumerable), String(ins.configurable), ins.value.length, ins.value.name].join(','));
		out.push('delDesc=' + [typeof del.value, String(del.writable), String(del.enumerable), String(del.configurable), del.value.length, del.value.name].join(','));
		out.join(' | ')
	`)
	want := "recv=TypeError: Illegal invocation | delRecv=TypeError: Illegal invocation | insDesc=function,true,true,true,1,insertRule | delDesc=function,true,true,true,1,deleteRule"
	if got != want {
		t.Errorf("mutation receiver and descriptor\ngot:  %s\nwant: %s", got, want)
	}
}
