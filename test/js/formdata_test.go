package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestFormDataTextareaHardWrap(t *testing.T) {
	doc := domtest.Parse(`<!doctype html><form id="f"><textarea name="bio" cols="4" wrap="hard"></textarea></form>`)
	r := newDOMRuntime(t, doc)
	if _, err := r.Evaluate(`document.querySelector('textarea').value='abcdefghij\nxyz'`); err != nil {
		t.Fatalf("set textarea value: %v", err)
	}
	got, err := r.Evaluate(`(function(){var e=document.querySelector('textarea'),fd=new FormData(e.form);return [e.value,fd.get('bio')].join('|')})()`)
	if err != nil {
		t.Fatalf("read FormData: %v", err)
	}
	if got != "abcdefghij\nxyz|abcd\nefgh\nij\nxyz" {
		t.Fatalf("textarea FormData = %q, want unchanged API value and hard-wrapped entry", got)
	}
}

func TestFormDataSelectMultipleCreatesOneEntryPerEnabledSelectedOption(t *testing.T) {
	doc := domtest.Parse(`<form id="f"><select name="color" multiple>
		<option value="red" selected>Red</option><option selected>Blue Sky</option>
		<option value="disabled" selected disabled>Disabled</option>
		<optgroup disabled><option value="group-disabled" selected>Unavailable</option></optgroup>
		<option value="green">Green</option>
	</select></form>`)
	r := newDOMRuntime(t, doc)
	got, err := r.Evaluate(`(function(){var fd=new FormData(document.getElementById('f'));return fd.getAll('color').join(',')})()`)
	if err != nil {
		t.Fatalf("construct FormData: %v", err)
	}
	if got != "red,Blue Sky" {
		t.Fatalf("selected option entries = %q, want red,Blue Sky", got)
	}
}

func TestFormDataCharsetKeywordUsesASCIICaseOnHiddenControls(t *testing.T) {
	doc := domtest.Parse(`<form id="f">
		<input type="hidden" name="_charset_" value="lower">
		<input type="hidden" name="_CHARSET_" value="upper">
		<input type="hidden" name="_ChArSeT_" value="mixed">
		<input type="hidden" name="_charſet_" value="unicode">
		<input type="text" name="_charset_" value="text">
	</form>`)
	r := newDOMRuntime(t, doc)
	got, err := r.Evaluate(`(function(){var d=new FormData(document.getElementById('f'));return [d.getAll('_charset_').join(','),d.get('_CHARSET_'),d.get('_ChArSeT_'),d.get('_charſet_')].join('|')})()`)
	if err != nil {
		t.Fatalf("construct FormData: %v", err)
	}
	if got != "UTF-8,text|UTF-8|UTF-8|unicode" {
		t.Fatalf("charset keyword entries = %q, want UTF-8,text|UTF-8|UTF-8|unicode", got)
	}
}

func TestFormDataConstructionGuardPreventsRecursiveFormSubmission(t *testing.T) {
	doc := domtest.Parse(`<iframe name="target"></iframe><form id="f" target="target"><input name="value" value="x"></form>`)
	r := newDOMRuntime(t, doc)
	got, err := r.Evaluate(`(function() {
		const form = document.getElementById('f');
		let events = 0;
		let submits = 0;
		form.addEventListener('submit', () => submits++);
		form.addEventListener('formdata', () => {
			events++;
			form.requestSubmit();
			form.submit();
		});
		form.submit();
		const afterSubmission = events;
		new FormData(form);
		return afterSubmission + ',' + events + ',' + submits;
	})()`)
	if err != nil {
		t.Fatalf("construct and submit FormData: %v", err)
	}
	if got != "1,2,0" {
		t.Fatalf("formdata and submit events after nested submission = %q, want 1,2,0", got)
	}
}

func TestFormDataAppend(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('name', 'John');
		fd.append('age', '30');
		var name = fd.get('name');
		var age = fd.get('age');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("name")
	if val.Str != "John" {
		t.Errorf("name = %q, want %q", val.Str, "John")
	}
	val, _ = r.Global("age")
	if val.Str != "30" {
		t.Errorf("age = %q, want %q", val.Str, "30")
	}
}

func TestFormDataSet(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('key', 'old');
		fd.set('key', 'new');
		var val = fd.get('key');
		var all = fd.getAll('key');
		var len = all.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("val")
	if val.Str != "new" {
		t.Errorf("val = %q, want %q", val.Str, "new")
	}
	val, _ = r.Global("len")
	if val.Num != 1 {
		t.Errorf("getAll length = %v, want 1 (set replaces all)", val.Num)
	}
}

func TestFormDataHas(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('exists', 'yes');
		var hasExists = fd.has('exists');
		var hasMissing = fd.has('missing');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("hasExists")
	if !val.Bool {
		t.Error("has('exists') should be true")
	}
	val, _ = r.Global("hasMissing")
	if val.Bool {
		t.Error("has('missing') should be false")
	}
}

func TestFormDataDelete(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('a', '1');
		fd.append('b', '2');
		fd.delete('a');
		var a = fd.get('a');
		var b = fd.get('b');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("a")
	if val.Kind != js.KindNull {
		t.Errorf("a should be null after delete, got %v", val.Kind)
	}
	val, _ = r.Global("b")
	if val.Str != "2" {
		t.Errorf("b = %q, want %q", val.Str, "2")
	}
}

func TestFormDataGetAll(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('tag', 'a');
		fd.append('tag', 'b');
		fd.append('tag', 'c');
		var all = fd.getAll('tag');
		var len = all.length;
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("len")
	if val.Num != 3 {
		t.Errorf("getAll length = %v, want 3", val.Num)
	}
}

func TestFormDataForEach(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new FormData();
		fd.append('x', '1');
		fd.append('y', '2');
		var count = 0;
		fd.forEach(function(value, key) {
			count++;
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("count")
	if val.Num != 2 {
		t.Errorf("forEach count = %v, want 2", val.Num)
	}
}

func TestFormDataOnWindow(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var fd = new window.FormData();
		fd.append('test', 'value');
		var val = fd.get('test');
	`, "test.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	val, _ := r.Global("val")
	if val.Str != "value" {
		t.Errorf("val = %q, want %q", val.Str, "value")
	}
}
