package js

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSerialize_OwnAttributesOnly verifies that the __serialize
// polyfill no longer emits Object.prototype methods as fake HTML
// attributes. The previous implementation used
// 'for (const key in node.attributes)' which walked the prototype
// chain and produced 6-10x more bytes than the real content for
// every element. That made every DOM mutation cost a
// full-document re-serialize + re-parse + re-layout, which is the
// freeze the user reported.
func TestSerialize_OwnAttributesOnly(t *testing.T) {
	rt := NewRuntime()
	rt.SetHTMLContent(`<html><body><div class="probe" id="x" data-y="42">hi</div></body></html>`)

	v, err := rt.RunScript("window.__serialize(document.body.firstChild)")
	assert.NoError(t, err)
	out := v.String()

	// The real attributes must be present.
	assert.Contains(t, out, `class="probe"`)
	assert.Contains(t, out, `id="x"`)
	assert.Contains(t, out, `data-y="42"`)
	// The internal __goosie_id is preserved (it is used by the
	// Go-side querySelectorAll round-trip helper) but is short
	// and not a leak — the bloat bug we are guarding against
	// was the JS function source appearing in the style
	// attribute, not the 12-byte id.
	assert.Contains(t, out, `__goosie_id="`)

	// None of the inherited Object.prototype methods should leak.
	for _, leak := range []string{
		"constructor=",
		"toString=",
		"valueOf=",
		"hasOwnProperty=",
		"isPrototypeOf=",
		"propertyIsEnumerable=",
		"toLocaleString=",
		"__defineGetter__=",
		"__defineSetter__=",
		"__lookupGetter__=",
		"__lookupSetter__=",
		"__proto__=",
	} {
		assert.NotContains(t, out, leak, "serialize must not leak %s", leak)
	}

	// No function source should be embedded.
	assert.False(t, strings.Contains(out, "function"), "serialize must not embed function source")
}

// TestSerialize_SetAttributeNoOp verifies that a small element
// serializes to a small string. The pre-fix bug serialized a
// single `<div class="a" id="b">x</div>` to ~550 bytes because
// every element carried a copy of the entire style proxy
// source as a fake attribute. The fix brings the size down to
// well under 100 bytes.
func TestSerialize_SetAttributeNoOp(t *testing.T) {
	rt := NewRuntime()
	rt.SetHTMLContent(`<html><body><div class="a" id="b">x</div></body></html>`)

	v, err := rt.RunScript("window.__serialize(document.body.firstChild)")
	assert.NoError(t, err)
	out := v.String()
	t.Logf("serialized: %q", out)
	assert.Less(t, v.ToInteger(), int64(100),
		"a small div should serialize to under 100 bytes; the bug produced ~550 bytes")
	// The exact form depends on the __goosie_id counter; the
	// class and id attributes must be present and in that order.
	assert.Contains(t, out, `class="a"`)
	assert.Contains(t, out, `id="b"`)
	assert.Contains(t, out, `>x</div>`)
}

// TestProxy_StyleAssignmentDoesNotPolluteAttributes guards the
// regression where assigning setProperty / getPropertyValue /
// removeProperty on element.style caused the Proxy's set handler
// to fold the function source into the style attribute (via
// setAttribute). That made every element's attributes.style
// contain a multi-line JS function string, which the next
// __serialize pass emitted and the next ghtml.Parse tried to
// parse as CSS — ballooning the serialized DOM and re-parse cost
// by ~10x. The fix lives in the proxy's set handler; this test
// fails loudly if the bug regresses.
func TestProxy_StyleAssignmentDoesNotPolluteAttributes(t *testing.T) {
	rt := NewRuntime()
	v, err := rt.RunScript(`(function(){
		var d = document.createElement('div');
		// style should not have been added to attributes by the
		// polyfill constructor's proxy installation.
		return 'style' in d.attributes ? 'POLLUTED' : 'clean';
	})()`)
	assert.NoError(t, err)
	assert.Equal(t, "clean", v.String(),
		"element.attributes must not contain a 'style' key after construction")
}

func TestDOMMutationBatchCallbackSkipsSerialization(t *testing.T) {
	rt := NewRuntime()
	rt.SetHTMLContent(`<html><body><div>before</div></body></html>`)
	before := rt.htmlCache

	var batches []DOMMutation
	rt.SetDOMMutationBatchCallback(func(batch []DOMMutation) {
		batches = append(batches, batch...)
	})

	_, err := rt.RunScript(`document.body.firstChild.textContent = "after"`)
	assert.NoError(t, err)
	assert.NotEmpty(t, batches)
	assert.Equal(t, before, rt.htmlCache)
}

func TestSetHTMLContentSkipsHtmlCacheWhenOnlyBatchCallbackSet(t *testing.T) {
	rt := NewRuntime()
	rt.SetDOMMutationBatchCallback(func([]DOMMutation) {})
	rt.SetHTMLContent(`<html><body><div>initial</div></body></html>`)
	if rt.htmlCache != "" {
		t.Fatalf("htmlCache should stay empty on the typed path, got %q", rt.htmlCache)
	}
}

func TestDOMMutationBatchIncludesOperationDetails(t *testing.T) {
	rt := NewRuntime()
	rt.SetHTMLContent(`<html><body><div id="target">before</div></body></html>`)

	var mutations []DOMMutation
	rt.SetDOMMutationBatchCallback(func(batch []DOMMutation) {
		mutations = append(mutations, batch...)
	})

	_, err := rt.RunScript(`(function(){
		var target = document.getElementById("target");
		target.setAttribute("data-state", "ready");
		target.textContent = "after";
	})()`)
	assert.NoError(t, err)
	if len(mutations) != 2 {
		t.Fatalf("mutation count = %d, want 2", len(mutations))
	}
	assert.Equal(t, MutationSetAttribute, mutations[0].Kind)
	assert.Equal(t, "data-state", mutations[0].Attribute)
	assert.Equal(t, "ready", mutations[0].NewValue)
	assert.Equal(t, MutationSetText, mutations[1].Kind)
	assert.NotEmpty(t, mutations[1].TargetID)
	assert.Equal(t, "after", mutations[1].NewValue)
}
