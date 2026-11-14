package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestDocumentFontsExists(t *testing.T) {
	r := newRuntime(t, jsOpts())
	if err := r.Run(`var out = (typeof document.fonts) + "/" + (document.fonts === document.fonts);`, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "object/true" {
		t.Fatalf("document.fonts identity = %+v, want object/true", v)
	}
}

func TestDocumentFontsLoadResolves(t *testing.T) {
	r := newRuntime(t, jsOpts())
	src := `
var out = "pending";
document.fonts.load('400 10pt "Google Sans"').then(function(faces) {
	out = "loaded:" + (Array.isArray(faces) ? faces.length : "noarray");
}, function(e) {
	out = "rejected:" + (e && e.name);
});`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "loaded:0" {
		t.Fatalf("fonts.load result = %+v, want loaded:0", v)
	}
}

func TestDocumentFontsLoadBadSpecRejects(t *testing.T) {
	r := newRuntime(t, jsOpts())
	src := `
var out = "pending";
try {
	document.fonts.load('not-a-font-spec!!!').then(function() {
		out = "resolved";
	}, function(e) {
		out = "rejected:" + (e && e.name);
	});
} catch (e) {
	out = "sync-throw:" + e.name;
}`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "rejected:SyntaxError" {
		t.Fatalf("bad-spec result = %+v, want rejected:SyntaxError", v)
	}
}

func TestDocumentFontsCheckAndReady(t *testing.T) {
	r := newRuntime(t, jsOpts())
	src := `
var out = document.fonts.status + "/" + document.fonts.check('16px sans-serif') + "/" + (typeof document.fonts.ready.then);
var readyOut = "pending";
document.fonts.ready.then(function(s) { readyOut = (s === document.fonts) ? "self" : "other"; });`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatal(err)
	}
	if v, ok := r.Global("out"); !ok || v.Str != "loaded/true/function" {
		t.Fatalf("fonts status/check/ready = %+v, want loaded/true/function", v)
	}
	if v, ok := r.Global("readyOut"); !ok || v.Str != "self" {
		t.Fatalf("fonts.ready resolved = %+v, want self", v)
	}
}

func TestGoogleFontsBootScript(t *testing.T) {
	r := newRuntime(t, jsOpts())
	src := `(function(){var w=["Google Sans",[400,500,700]];(function(){for(var a=0;a<w.length;a+=2)for(var b=w[a],e=w[a+1],d=0,c=void 0;c=e[d];++d)b==="Noto Color Emoji"?document.fonts.load(c+" 10pt "+b,"").catch(function(){}):document.fonts.load(c+" 10pt "+b).catch(function(){})})();})();`
	if err := r.Run(src, "inline"); err != nil {
		t.Fatalf("google boot script: %v", err)
	}
}

func jsOpts() js.Options {
	return js.Options{URL: "https://www.google.com/"}
}
