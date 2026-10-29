package js

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestInputShowPickerOriginAndActivationOrder(t *testing.T) {
	limits := dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 16, AttributeBytes: 1024}
	parentDoc, err := dom.ParseBounded(`<!doctype html><html><body></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := New(Options{URL: "https://top.example/page", DOM: parentDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()

	childDoc, err := dom.ParseBounded(`<!doctype html><html><body></body></html>`, limits)
	if err != nil {
		t.Fatal(err)
	}
	child, err := New(Options{URL: "https://other.example/frame", DOM: childDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	child.iframeParent = parent

	for _, tc := range []struct{ script, want string }{
		{`(function(){var i=document.createElement('input');i.type='button';try{i.showPicker();return 'none'}catch(e){return e.name}})()`, "SecurityError"},
		{`(function(){var s=document.createElement('select');try{s.showPicker();return 'none'}catch(e){return e.name}})()`, "SecurityError"},
		{`(function(){var i=document.createElement('input');i.type='file';try{i.showPicker();return 'none'}catch(e){return e.name}})()`, "NotAllowedError"},
		{`(function(){var i=document.createElement('input');i.type='button';i.disabled=true;try{i.showPicker();return 'none'}catch(e){return e.name}})()`, "InvalidStateError"},
	} {
		got, err := child.Evaluate(tc.script)
		if err != nil {
			t.Fatalf("Evaluate(%q): %v", tc.script, err)
		}
		if got != tc.want {
			t.Errorf("Evaluate(%q) = %q, want %q", tc.script, got, tc.want)
		}
	}

	child.SetTransientUserActivation(true)
	got, err := child.Evaluate(`(function(){var i=document.createElement('input');i.type='file';i.showPicker();return navigator.userActivation.isActive+'|'+navigator.userActivation.hasBeenActive})()`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "false|true" {
		t.Fatalf("file picker with activation = %q, want false|true", got)
	}

	for _, inputType := range []string{"color", "date", "datetime-local", "file", "month", "time", "week"} {
		child.SetTransientUserActivation(true)
		got, err := child.Evaluate(`(function(){var i=document.createElement('input');i.type='` + inputType + `';i.click();return navigator.userActivation.isActive})()`)
		if err != nil {
			t.Fatalf("click() for %s: %v", inputType, err)
		}
		if got != "false" {
			t.Errorf("click() for %s left transient activation active: %q", inputType, got)
		}
	}
}
