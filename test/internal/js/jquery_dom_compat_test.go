package js_test

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	js "github.com/vyquocvu/goosie/internal/js"
)

// loadJQueryFixture returns the jQuery build whose minified offsets match
// the "jQuery.Deferred exception: Cannot read property 'ownerDocument' of
// undefined" crash reported on WordPress pages (se.contains @13238,
// isAttached ie @36212, buildFragment xe @39398, S @1024).
func loadJQueryFixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/jquery-3.6.0.min.js")
	if err != nil {
		t.Fatalf("jquery fixture: %v", err)
	}
	return string(b)
}

// newJQueryRuntime builds a runtime whose timer callbacks are serialized
// with the test goroutine through a mutex, matching the browser's
// single-threaded JS execution model.
func newJQueryRuntime(t *testing.T) (*js.Runtime, *sync.Mutex) {
	t.Helper()
	rt := js.NewRuntime()
	mu := &sync.Mutex{}
	rt.SetEnqueueTask(func(f func()) {
		go func() { mu.Lock(); defer mu.Unlock(); f() }()
	})
	t.Cleanup(rt.Cleanup)
	rt.SetOrigin("https://www.blender.org")
	return rt, mu
}

// TestJQueryDOMManipulationCompat exercises the DOM manipulation APIs
// (append/clone/wrap/replaceWith/...) through jQuery's domManip and
// buildFragment paths. It guards the spec behaviors these paths depend on:
// ownerDocument on every node, parentNode bookkeeping when textContent or
// innerHTML replace children, empty textContent removing all children, and
// DocumentFragment semantics in appendChild/insertBefore.
func TestJQueryDOMManipulationCompat(t *testing.T) {
	rt, mu := newJQueryRuntime(t)
	rt.SetHTMLContent(`<html><head></head><body><div id="main" class="c"><a href="/x" class="btn" data-k="v">l</a><span class="s">t</span></div></body></html>`)

	mu.Lock()
	defer mu.Unlock()
	if _, err := rt.RunScript(loadJQueryFixture(t)); err != nil {
		t.Fatalf("jquery load: %v", err)
	}
	v, err := rt.RunScript(`(function(){
		var checks = [];
		function check(name, fn, expect) {
			var got;
			try { got = fn(); } catch(e) { got = 'THREW ' + e; }
			var ok = String(got) === String(expect);
			checks.push((ok ? 'ok  ' : 'FAIL') + ' ' + name + ':' + got + (ok ? '' : ' want=' + expect));
		}
		try {
			check('id-selector', function(){ return jQuery('#main').length; }, 1);
			check('class-selector', function(){ return jQuery('.btn').length; }, 1);
			check('attr-selector', function(){ return jQuery('[data-k="v"]').length; }, 1);
			check('find', function(){ return jQuery('#main').find('a').length; }, 1);
			check('children', function(){ return jQuery('#main').children().length; }, 2);
			check('parent', function(){ return jQuery('.btn').parent().attr('id'); }, 'main');
			check('contains', function(){ return jQuery('#main:contains("l")').length; }, 1);
			check('closest', function(){ return jQuery('.btn').closest('div').attr('id'); }, 'main');
			check('each', function(){ var n=0; jQuery('#main > *').each(function(){ n++; }); return n; }, 2);
			check('on-trigger', function(){ var hit=0; jQuery('.btn').on('click', function(){ hit++; }).trigger('click'); return hit; }, 1);
			check('append', function(){ jQuery('#main').append('<i>x</i>'); return jQuery('#main').find('i').length; }, 1);
			check('append-multi', function(){ jQuery('#main').append('<b>a</b><u>b</u>'); return jQuery('#main').find('b').length + jQuery('#main').find('u').length; }, 2);
			check('prepend', function(){ jQuery('#main').prepend('<s>p</s>'); return jQuery('#main').children().first().prop('tagName').toLowerCase(); }, 's');
			check('html', function(){ return jQuery('.s').html(); }, 't');
			check('html-set', function(){ jQuery('.s').html('<em>q</em>'); return jQuery('.s').find('em').text(); }, 'q');
			check('is', function(){ return jQuery('.btn').is('a'); }, true);
			check('first-last', function(){ return jQuery('#main > a').first().attr('class') + '|' + jQuery('#main > span').last().attr('class'); }, 'btn|s');
			check('has', function(){ return jQuery('#main').has('a').length; }, 1);
			check('siblings', function(){ return jQuery('.btn').siblings().length; }, 5);
			check('next-prev', function(){ return jQuery('.btn').next().prop('tagName').toLowerCase() + '|' + jQuery('.s').next().prop('tagName').toLowerCase(); }, 'span|i');
			check('empty', function(){ var d = jQuery('<div><p>1</p></div>'); d.empty(); return d.children().length; }, 0);
			check('remove', function(){ var d = jQuery('<div><p>1</p></div>'); d.find('p').remove(); return d.children().length; }, 0);
			check('clone-append', function(){ var c = jQuery('.btn').clone(); jQuery('#main').append(c); return jQuery('#main').find('.btn').length; }, 2);
			check('wrap', function(){ jQuery('.s').wrap('<em/>'); return jQuery('.s').parent().prop('tagName').toLowerCase(); }, 'em');
			check('wrapAll', function(){ jQuery('#main > b, #main > u').wrapAll('<strong/>'); return jQuery('#main strong').children().length; }, 2);
			check('wrapInner', function(){ jQuery('.s').wrapInner('<mark/>'); return jQuery('.s').children('mark').text(); }, 'q');
			check('unwrap', function(){ jQuery('.s').unwrap(); return jQuery('.s').parent().attr('id'); }, 'main');
			check('detach-reattach', function(){ var d = jQuery('.btn').detach(); jQuery('#main').append(d); return jQuery('#main').find('.btn').length; }, 2);
			check('text', function(){ return jQuery('.s').text(); }, 'q');
			check('replaceWith', function(){ jQuery('.s').replaceWith('<code>r</code>'); return jQuery('#main').find('code').text(); }, 'r');
			check('after-before', function(){ jQuery('#main code').before('<dfn>B</dfn>').after('<ins>A</ins>'); return jQuery('#main').find('dfn').length + jQuery('#main').find('ins').length; }, 2);
			check('replaceWith-multi', function(){ jQuery('#main code').replaceWith('<x>a</x><y>b</y>'); return jQuery('#main').find('x').length + jQuery('#main').find('y').length; }, 2);
			check('serialize-find', function(){ return document.querySelectorAll('#main i').length; }, 1);
		} catch(e) {
			checks.push('ERR: ' + e + '\n' + (e.stack || ''));
		}
		return checks.join('\n');
	})()`)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	out := v.String()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "FAIL") || strings.HasPrefix(line, "ERR:") {
			t.Error(line)
		}
	}
}

// TestJQueryReadyNoOwnerDocumentCrash reproduces the WordPress page failure:
// a jQuery ready handler (Deferred-resolved) performing DOM manipulation
// used to crash inside buildFragment -> isAttached -> Sizzle.contains with
// "Cannot read property 'ownerDocument' of undefined", surfaced as a
// "jQuery.Deferred exception" console warning.
func TestJQueryReadyNoOwnerDocumentCrash(t *testing.T) {
	rt, mu := newJQueryRuntime(t)
	rt.SetHTMLContent(`<html><head></head><body><div id="main"><a class="btn">l</a></div></body></html>`)

	mu.Lock()
	if _, err := rt.RunScript(loadJQueryFixture(t)); err != nil {
		t.Fatalf("jquery load: %v", err)
	}
	if _, err := rt.RunScript(`jQuery(function($){
		$('.btn').wrap('<em/>');
		$('#main').append('<i>x</i><b>y</b>');
		var clone = $('.btn').clone();
		$(document.body).append(clone);
		window.__readyDone = 'yes';
	});`); err != nil {
		t.Fatalf("register ready handler: %v", err)
	}
	mu.Unlock()

	// Let the setTimeout(jQuery.ready) dispatch run under the mutex.
	done := false
	for i := 0; i < 100; i++ {
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		v, err := rt.RunScript(`String(window.__readyDone || 'no')`)
		mu.Unlock()
		if err == nil && v.String() == "yes" {
			done = true
			break
		}
	}
	if !done {
		t.Fatal("jQuery ready handler never executed")
	}

	mu.Lock()
	defer mu.Unlock()
	v, err := rt.RunScript(`(function(){
		var wrap = document.querySelectorAll('#main > em');
		var btn = document.querySelectorAll('em > .btn');
		var appended = document.querySelectorAll('#main i').length + document.querySelectorAll('#main b').length;
		var clones = document.querySelectorAll('body > .btn').length;
		return 'wrap=' + wrap.length + ' btnInWrap=' + btn.length + ' appended=' + appended + ' clones=' + clones;
	})()`)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	want := "wrap=1 btnInWrap=1 appended=2 clones=1"
	if got := v.String(); got != want {
		t.Errorf("DOM state after ready handler: got %q want %q", got, want)
	}

	for _, m := range rt.GetConsoleMessages() {
		if strings.Contains(m.Message, "jQuery.Deferred exception") || strings.Contains(m.Message, "ownerDocument") {
			t.Errorf("console %s: %s", m.Level, m.Message)
		}
	}
	for _, e := range rt.GetJavaScriptErrors() {
		if strings.Contains(e, "ownerDocument") {
			t.Errorf("js error: %s", e)
		}
	}
}

// TestFragmentSemantics pins the DOM-level behaviors jQuery relies on,
// independent of the jQuery fixture.
func TestFragmentSemantics(t *testing.T) {
	rt, mu := newJQueryRuntime(t)
	rt.SetHTMLContent(`<html><head></head><body><div id="host"><span>a</span></div></body></html>`)

	mu.Lock()
	defer mu.Unlock()
	v, err := rt.RunScript(`(function(){
		var out = [];
		var host = document.getElementById('host');
		var span = host.firstChild;

		// appendChild(fragment) moves the fragment's children.
		var f = document.createDocumentFragment();
		f.appendChild(document.createElement('i'));
		f.appendChild(document.createElement('b'));
		host.appendChild(f);
		out.push('appendChild: children=' + host.children.length + ' emptied=' + (f.childNodes.length === 0));

		// insertBefore(fragment, ref) moves the children before ref.
		var f2 = document.createDocumentFragment();
		f2.appendChild(document.createElement('u'));
		host.insertBefore(f2, span);
		out.push('insertBefore: first=' + host.children[0].tagName.toLowerCase() + ' emptied=' + (f2.childNodes.length === 0));

		// textContent = "" removes all children (no phantom text node).
		var d = document.createElement('div');
		d.appendChild(document.createElement('p'));
		d.textContent = '';
		out.push('textContent-empty: children=' + d.childNodes.length);
		d.textContent = 'x';
		out.push('textContent-set: children=' + d.childNodes.length + ' text=' + d.textContent);

		// replaceChild(fragment, old) substitutes the fragment's children.
		var host2 = document.createElement('div');
		var old = document.createElement('p');
		host2.appendChild(old);
		var f3 = document.createDocumentFragment();
		f3.appendChild(document.createElement('i'));
		f3.appendChild(document.createElement('b'));
		host2.replaceChild(f3, old);
		out.push('replaceChild: children=' + host2.children.length + ' tags=' + host2.children.map(function(c){return c.tagName.toLowerCase();}).join(',') + ' emptied=' + (f3.childNodes.length === 0) + ' oldDetached=' + (old.parentNode === null));

		return out.join('\n');
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"appendChild: children=3 emptied=true",
		"insertBefore: first=u emptied=true",
		"textContent-empty: children=0",
		"textContent-set: children=1 text=x",
		"replaceChild: children=2 tags=i,b emptied=true oldDetached=true",
	}
	got := strings.Split(v.String(), "\n")
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q want %q", i, got[i], want[i])
		}
	}
}
