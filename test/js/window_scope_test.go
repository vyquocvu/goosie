package js_test

import (
	"testing"
)

func TestWindowIsGlobalScope(t *testing.T) {
	r := newRuntime(t, jsOpts())
	src := `
window.google = {kEI: 'x'};
var out = google.kEI + "/" + (window.google === google);`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatalf("window->global publish: %v", err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "x/true" {
		t.Fatalf("window.publish = %+v, want x/true", v)
	}
}

func TestGlobalIsWindowScope(t *testing.T) {
	r := newRuntime(t, jsOpts())
	if err := r.Run(`var appBooted = 1; var out = window.appBooted === 1;`, "inline"); err != nil {
		t.Fatalf("global->window read: %v", err)
	}
	if v, ok := r.Global("out"); !ok || !v.Bool {
		t.Fatalf("global.publish = %+v, want true", v)
	}
}

func TestWindowSelfReferences(t *testing.T) {
	r := newRuntime(t, jsOpts())
	src := `var out = (window.window === window) + "/" + (self === window) + "/" + (globalThis === window) + "/" + (window.location.href === location.href);`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatalf("self references: %v", err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "true/true/true/true" {
		t.Fatalf("self references = %+v, want true/true/true/true", v)
	}
}

func TestMatchMediaUsesDefaultLightColorScheme(t *testing.T) {
	r := newRuntime(t, jsOpts())
	src := `var out = window.matchMedia('(prefers-color-scheme: light)').matches + "/" + window.matchMedia('(prefers-color-scheme: dark)').matches;`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "true/false" {
		t.Fatalf("matchMedia color scheme = %+v, want true/false", v)
	}
}

func TestGoogleNamespaceBootstrap(t *testing.T) {
	r := newRuntime(t, jsOpts())
	part1 := `(function(){var _g={kEI:'IkfOasPwI-uu4-EPrv3E-QM'};(function(){var a;((a=window.google)==null?0:a.stvsc)?google.kEI=_g.kEI:window.google=_g;}).call(this);})();`
	if err := r.Run(part1, "inline"); err != nil {
		t.Fatalf("bootstrap publish: %v", err)
	}
	part2 := `(function(){google.sn='webhp';google.kHL='vi';})();`
	if err := r.Run(part2, "inline"); err != nil {
		t.Fatalf("bootstrap readback: %v", err)
	}
	if err := r.Run(`var out = google.sn + "/" + google.kEI;`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "webhp/IkfOasPwI-uu4-EPrv3E-QM" {
		t.Fatalf("google namespace = %+v", v)
	}
}
