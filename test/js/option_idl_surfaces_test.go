package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func optionIDLRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(`<!DOCTYPE html><html><body>
<select id="s">
  <option id="plain" selected>
    Charcoal
  </option>
  <option id="runs" value="v-runs">  Hi   there </option>
  <option id="lab" label="  My Label  ">text content</option>
  <option id="labempty" label="">empty label attr</option>
  <option id="labonly" label="Label only"></option>
  <option id="blank" value="b">   </option>
</select></body></html>`)
	return newRuntime(t, js.Options{URL: "https://example.test/opt.html", DOM: doc})
}

// qc:BR-101
func TestOptionTextIsNormalisedWhileLabelReflectsTheAttribute(t *testing.T) {
	rt := optionIDLRuntime(t)
	runJSCases(t, rt, []jsCase{
		{"plain text", `document.getElementById('plain').text`, "Charcoal"},
		{"plain label", `document.getElementById('plain').label`, "Charcoal"},
		{"plain value", `document.getElementById('plain').value`, "Charcoal"},
		{"runs text", `document.getElementById('runs').text`, "Hi there"},
		{"runs label", `document.getElementById('runs').label`, "Hi there"},
		{"runs value", `document.getElementById('runs').value`, "v-runs"},
		{"lab label", `document.getElementById('lab').label`, "  My Label  "},
		{"lab text", `document.getElementById('lab').text`, "text content"},
		{"lab value", `document.getElementById('lab').value`, "text content"},
		{"labempty label", `document.getElementById('labempty').label`, ""},
		{"labempty text", `document.getElementById('labempty').text`, "empty label attr"},
		{"labonly label", `document.getElementById('labonly').label`, "Label only"},
		{"labonly text", `document.getElementById('labonly').text`, ""},
		{"labonly value", `document.getElementById('labonly').value`, ""},
		{"blank text", `document.getElementById('blank').text`, ""},
		{"blank label", `document.getElementById('blank').label`, ""},
		{"blank value", `document.getElementById('blank').value`, "b"},
		{"indexes", `[0,1,2,3,4,5].map(function(i){return document.getElementById('s').options[i].index}).join(',')`, "0,1,2,3,4,5"},
	})
}

// qc:BR-101
func TestOptionLabelAndValueAssignmentWritesContentAttributes(t *testing.T) {
	rt := optionIDLRuntime(t)
	got := evalString(t, rt, `(function(){
		var o = document.getElementById('runs');
		o.label = '  set  label  ';
		o.value = '  set  value  ';
		return [o.getAttribute('label'), o.label, o.getAttribute('value'), o.value, o.text].join('|');
	})()`)
	if want := "  set  label  |  set  label  |  set  value  |  set  value  |Hi there"; got != want {
		t.Fatalf("IDL-SET = %q, want %q", got, want)
	}
}

// qc:BR-101
func TestOptgroupFlattensIntoOptionsWithoutTouchingOptionLabels(t *testing.T) {
	doc := domtest.Parse(`<!DOCTYPE html><html><body>
<select id="s">
  <option id="top" value="top">Top</option>
  <optgroup id="g" label="  Group  Label  ">
    <option id="in1" value="in1">In one</option>
    <option id="in2" selected value="in2">In two</option>
  </optgroup>
  <option id="tail" value="tail">Tail</option>
</select></body></html>`)
	rt := newRuntime(t, js.Options{URL: "https://example.test/opt.html", DOM: doc})
	runJSCases(t, rt, []jsCase{
		{"flattened length", `document.getElementById('s').options.length`, "4"},
		{"flattened ids", `[].map.call(document.getElementById('s').options, function(o){return o.id}).join(',')`, "top,in1,in2,tail"},
		{"group label verbatim", `document.getElementById('g').label`, "  Group  Label  "},
		{"option label ignores the group", `document.getElementById('in1').label`, "In one"},
		{"option text of every flattened option", `[].map.call(document.getElementById('s').options, function(o){return o.text}).join(',')`, "Top,In one,In two,Tail"},
		{"select value follows the selected option", `document.getElementById('s').value`, "in2"},
	})
}

// qc:BR-101
func TestMultipleSelectReportsEverySelectionAndValuesTheFirst(t *testing.T) {
	doc := domtest.Parse(`<!DOCTYPE html><html><body><form id="f">
<select id="s" name="m" multiple>
  <option id="a" value="a">A</option>
  <option id="b" value="b" selected>B</option>
  <option id="c" value="c" selected>C</option>
</select></form></body></html>`)
	rt := newRuntime(t, js.Options{URL: "https://example.test/opt.html", DOM: doc})
	runJSCases(t, rt, []jsCase{
		{"initial", `(function(){var s=document.getElementById('s');
			return [s.value, s.selectedIndex, [].map.call(s.selectedOptions, function(o){return o.id}).join(','),
				[].map.call(s.options, function(o){return o.selected}).join(',')].join('|')})()`,
			"b|1|b,c|false,true,true"},
		{"formdata carries every authored selection", `new FormData(document.getElementById('f')).getAll('m').join(',')`, "b,c"},
		{"after a third selection", `(function(){var s=document.getElementById('s');
			s.options[0].selected = true;
			return [s.value, s.selectedIndex, [].map.call(s.selectedOptions, function(o){return o.id}).join(',')].join('|')})()`,
			"a|0|a,b,c"},
	})
}
