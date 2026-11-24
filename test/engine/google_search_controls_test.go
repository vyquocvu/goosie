package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
)

func TestTextControlAccessibleNameFallsBackToTitle(t *testing.T) {
	s := newFocusSession(t, `<html><body><input name="q" title="Search" placeholder="Query"></body></html>`)
	field := axFind(s.AccessibilityTree(), ax.RoleTextField, "Search")
	if field == nil {
		t.Fatalf("text control with title-based name not found: %+v", s.AccessibilityTree())
	}
	if field.Value != "" {
		t.Fatalf("field value = %q, want empty", field.Value)
	}
}

func TestDocumentLoadFocusesNamedFormControl(t *testing.T) {
	s, _ := newJSSession(t, `<html><body>
<form name="f"><input name="q"></form>
<script>document.body.onload = function () { document.f.q.focus(); };</script>
</body></html>`)
	focused := s.Focused()
	if focused == nil || focused.GetAttribute("name") != "q" {
		t.Fatalf("focused node = %+v, want the named form query control", focused)
	}
}
