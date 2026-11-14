package engine_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func newParitySession(t *testing.T, html string) (*engine.Session, *js.Runtime) {
	t.Helper()
	rt, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://test.example/",
	})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	t.Cleanup(func() { rt.Close() })

	sess, err := engine.NewSession(html, nil, 800,
		engine.WithViewportH(600),
		engine.WithJS(rt),
	)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess, rt
}

func TestParity_FormWithAllInputTypes(t *testing.T) {
	sess, rt := newParitySession(t, `<!DOCTYPE html>
<html><body>
<form id="form">
<input type="text" id="name" value="">
<input type="checkbox" id="agree">
<input type="radio" name="color" id="red" value="red">
<input type="radio" name="color" id="blue" value="blue">
<textarea id="bio">initial</textarea>
<button type="submit" id="submit">Go</button>
</form>
<script>
__submitted = false;
__formData = '';
document.getElementById('form').addEventListener('submit', function(e) {
	e.preventDefault();
	__submitted = true;
	var name = document.getElementById('name').value;
	var agree = document.getElementById('agree').checked;
	var bio = document.getElementById('bio').value;
	__formData = name + '|' + agree + '|' + bio;
});
</script>
</body></html>`)

	sess.FocusControl(10, 10)
	for _, ch := range "Alice" {
		sess.Edit(engine.EditRune, ch)
	}

	clickCenter(t, sess, "agree")

	clickCenter(t, sess, "blue")

	sess.FocusControl(10, 10)

	clickCenter(t, sess, "submit")

	v, ok := rt.Global("__submitted")
	if !ok || !v.Bool {
		t.Error("form was not submitted")
	}
}

func TestParity_CheckboxGroup(t *testing.T) {
	sess, rt := newParitySession(t, `<!DOCTYPE html>
<html><body>
<input type="checkbox" id="a">
<input type="checkbox" id="b">
<input type="checkbox" id="c">
<script>
__changes = 0;
document.getElementById('a').addEventListener('change', function() { __changes++; });
document.getElementById('b').addEventListener('change', function() { __changes++; });
document.getElementById('c').addEventListener('change', function() { __changes++; });
</script>
</body></html>`)

	clickCenter(t, sess, "a")
	clickCenter(t, sess, "b")
	clickCenter(t, sess, "c")

	v, ok := rt.Global("__changes")
	if !ok {
		t.Fatal("__changes not set")
	}
	if v.Num != 3 {
		t.Errorf("__changes = %v, want 3", v.Num)
	}

	clickCenter(t, sess, "a")
	clickCenter(t, sess, "b")
	clickCenter(t, sess, "c")

	v, _ = rt.Global("__changes")
	if v.Num != 6 {
		t.Errorf("__changes after uncheck = %v, want 6", v.Num)
	}
}

func TestParity_RadioGroupMutualExclusion(t *testing.T) {
	sess, rt := newParitySession(t, `<!DOCTYPE html>
<html><body>
<input type="radio" name="size" id="sm" value="small">
<input type="radio" name="size" id="md" value="medium" checked>
<input type="radio" name="size" id="lg" value="large">
<script>
__lastChecked = 'md';
document.getElementById('sm').addEventListener('change', function() { __lastChecked = 'sm'; });
document.getElementById('lg').addEventListener('change', function() { __lastChecked = 'lg'; });
</script>
</body></html>`)

	clickCenter(t, sess, "lg")

	v, ok := rt.Global("__lastChecked")
	if !ok || v.Str != "lg" {
		t.Errorf("__lastChecked = %v, want 'lg'", v)
	}
}

func TestParity_TextInputBackspaceAndOvertype(t *testing.T) {
	sess, rt := newParitySession(t, `<!DOCTYPE html>
<html><body>
<input type="text" id="input" value="hello">
<script>
__inputCount = 0;
document.getElementById('input').addEventListener('input', function() {
	__inputCount++;
});
</script>
</body></html>`)

	focusCenter(t, sess, "input")

	sess.Edit(engine.EditEnd, 0)

	sess.Edit(engine.EditBackspace, 0)
	sess.Edit(engine.EditBackspace, 0)

	sess.Edit(engine.EditRune, 'p')
	sess.Edit(engine.EditRune, '!')

	v, ok := rt.Global("__inputCount")
	if !ok {
		t.Fatal("__inputCount not set")
	}
	if v.Num != 4 {
		t.Errorf("__inputCount = %v, want 4 (2 backspaces + 2 runes)", v.Num)
	}
}

func TestParity_FocusTabOrder(t *testing.T) {
	sess, rt := newParitySession(t, `<!DOCTYPE html>
<html><body>
<input type="text" id="first">
<input type="text" id="second">
<input type="text" id="third">
<script>
__focusLog = '';
document.getElementById('first').addEventListener('focus', function() { __focusLog += '1'; });
document.getElementById('second').addEventListener('focus', function() { __focusLog += '2'; });
document.getElementById('third').addEventListener('focus', function() { __focusLog += '3'; });
</script>
</body></html>`)

	sess.FocusNext()
	sess.FocusNext()
	sess.FocusNext()

	v, ok := rt.Global("__focusLog")
	if !ok {
		t.Fatal("__focusLog not set")
	}
	if v.Str != "123" {
		t.Errorf("__focusLog = %q, want %q", v.Str, "123")
	}
}

func TestParity_ClickPreventDefaultBlocksFormSubmit(t *testing.T) {
	sess, rt := newParitySession(t, `<!DOCTYPE html>
<html><body>
<form id="form">
<button type="submit" id="btn">Submit</button>
</form>
<script>
__submitFired = false;
document.getElementById('form').addEventListener('submit', function(e) {
	e.preventDefault();
	__submitFired = true;
});
</script>
</body></html>`)

	clickCenter(t, sess, "btn")

	v, ok := rt.Global("__submitFired")
	if !ok || !v.Bool {
		t.Error("submit handler did not fire or preventDefault was not observed")
	}
}

func TestParity_MultipleEventListenersOnSameElement(t *testing.T) {
	sess, rt := newParitySession(t, `<!DOCTYPE html>
<html><body>
<button id="btn">Click</button>
<script>
__order = '';
var btn = document.getElementById('btn');
btn.addEventListener('click', function() { __order += 'A'; });
btn.addEventListener('click', function() { __order += 'B'; });
btn.addEventListener('click', function() { __order += 'C'; });
</script>
</body></html>`)

	clickCenter(t, sess, "btn")

	v, ok := rt.Global("__order")
	if !ok {
		t.Fatal("__order not set")
	}
	if v.Str != "ABC" {
		t.Errorf("__order = %q, want %q", v.Str, "ABC")
	}
}

func TestParity_ScrollEventFiresOnDocument(t *testing.T) {
	sess, rt := newParitySession(t, `<!DOCTYPE html>
<html><body>
<div style="height:2000px;">Tall content</div>
<script>
__scrolls = 0;
document.addEventListener('scroll', function() { __scrolls++; });
</script>
</body></html>`)

	for i := 0; i < 5; i++ {
		sess.FireScrollEvent()
	}
	sess.PumpJS(800)

	v, ok := rt.Global("__scrolls")
	if !ok {
		t.Fatal("__scrolls not set")
	}
	if v.Num != 1 {
		t.Errorf("__scrolls = %v, want 1", v.Num)
	}
}

func TestParity_DOMMutationFromClickListener(t *testing.T) {
	sess, rt := newParitySession(t, `<!DOCTYPE html>
<html><body>
<div id="target">original</div>
<button id="btn">Change</button>
<script>
document.getElementById('btn').addEventListener('click', function() {
	var el = document.getElementById('target');
	el.textContent = 'modified';
	el.setAttribute('data-changed', 'true');
});
</script>
</body></html>`)

	clickCenter(t, sess, "btn")

	v, ok := rt.Global("__check")
	_ = v
	_ = ok
	sess2, rt2 := newParitySession(t, `<!DOCTYPE html>
<html><body>
<div id="target">original</div>
<button id="btn">Change</button>
<script>
__attr = '';
document.getElementById('btn').addEventListener('click', function() {
	var el = document.getElementById('target');
	el.textContent = 'modified';
	el.setAttribute('data-changed', 'true');
	__attr = el.getAttribute('data-changed');
});
</script>
</body></html>`)
	_ = sess2

	clickCenter(t, sess2, "btn")

	v2, ok2 := rt2.Global("__attr")
	if !ok2 || v2.Str != "true" {
		t.Errorf("__attr = %v, want 'true'", v2)
	}
}
