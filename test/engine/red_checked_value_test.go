package engine_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// RED regression: JS bridge must expose checked/value properties on form
// controls, matching Chromium's behavior. These tests FAIL before the fix
// and PASS after.
// ---------------------------------------------------------------------------

func redSession(t *testing.T, html string) (*engine.Session, *js.Runtime) {
	t.Helper()
	rt, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://test.example/"})
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

// TestRED_CheckboxCheckedGetter verifies el.checked returns correct boolean
// for a checkbox before and after click activation.
func TestRED_CheckboxCheckedGetter(t *testing.T) {
	sess, rt := redSession(t, `<!DOCTYPE html>
<html><body>
<input type="checkbox" id="cb" checked>
<script>
__before = document.getElementById('cb').checked;
</script>
</body></html>`)

	// Read initial state (should be true since checked attribute is set).
	v, ok := rt.Global("__before")
	if !ok {
		t.Fatal("__before not set")
	}
	if !v.Bool {
		t.Errorf("initial checked = %v, want true", v.Bool)
	}

	// Click to uncheck, then read via a script that sets a global.
	clickCenter(t, sess, "cb")
	if err := rt.Run("__after = document.getElementById('cb').checked", ""); err != nil {
		t.Fatalf("Run: %v", err)
	}
	after, ok := rt.Global("__after")
	if !ok {
		t.Fatal("__after not set")
	}
	if after.Bool {
		t.Errorf("after click checked = %v, want false", after.Bool)
	}
}

// TestRED_RadioValueGetter verifies el.value returns correct value for radio
// buttons.
func TestRED_RadioValueGetter(t *testing.T) {
	_, rt := redSession(t, `<!DOCTYPE html>
<html><body>
<input type="radio" name="r" id="r1" value="alpha">
<input type="radio" name="r" id="r2" value="beta" checked>
<script>
__v1 = document.getElementById('r1').value;
__v2 = document.getElementById('r2').value;
</script>
</body></html>`)

	v1, ok := rt.Global("__v1")
	if !ok || v1.Str != "alpha" {
		t.Errorf("r1.value = %q, want %q", v1.Str, "alpha")
	}
	v2, ok := rt.Global("__v2")
	if !ok || v2.Str != "beta" {
		t.Errorf("r2.value = %q, want %q", v2.Str, "beta")
	}
}

// TestRED_CheckboxCheckedSetter verifies el.checked = true/false changes state.
func TestRED_CheckboxCheckedSetter(t *testing.T) {
	_, rt := redSession(t, `<!DOCTYPE html>
<html><body>
<input type="checkbox" id="cb">
<script>
var cb = document.getElementById('cb');
cb.checked = true;
__afterSet = cb.checked;
cb.checked = false;
__afterUnset = cb.checked;
</script>
</body></html>`)

	v1, ok := rt.Global("__afterSet")
	if !ok || !v1.Bool {
		t.Errorf("after checked=true: %v, want true", v1)
	}
	v2, ok := rt.Global("__afterUnset")
	if !ok || v2.Bool {
		t.Errorf("after checked=false: %v, want false", v2)
	}
}

// TestRED_TextInputValueGetterSetter verifies el.value on text inputs.
func TestRED_TextInputValueGetterSetter(t *testing.T) {
	_, rt := redSession(t, `<!DOCTYPE html>
<html><body>
<input type="text" id="txt" value="initial">
<script>
__initial = document.getElementById('txt').value;
document.getElementById('txt').value = 'changed';
__changed = document.getElementById('txt').value;
</script>
</body></html>`)

	v1, ok := rt.Global("__initial")
	if !ok || v1.Str != "initial" {
		t.Errorf("initial value = %q, want %q", v1.Str, "initial")
	}
	v2, ok := rt.Global("__changed")
	if !ok || v2.Str != "changed" {
		t.Errorf("after set value = %q, want %q", v2.Str, "changed")
	}
}

// TestRED_CheckboxValueDefault verifies that a checkbox without explicit value
// attribute returns "on" as its default value (per HTML spec).
func TestRED_CheckboxValueDefault(t *testing.T) {
	_, rt := redSession(t, `<!DOCTYPE html>
<html><body>
<input type="checkbox" id="cb">
<script>
__val = document.getElementById('cb').value;
</script>
</body></html>`)

	v, ok := rt.Global("__val")
	if !ok || v.Str != "on" {
		t.Errorf("checkbox.value = %q, want %q (HTML spec default)", v.Str, "on")
	}
}
