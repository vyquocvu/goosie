package engine_test

import (
	"image/color"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

func imageInputObject(t *testing.T, sess *engine.Session, id string) *layout.Object {
	t.Helper()
	n := sess.Doc.ElementByID(id)
	if n == nil {
		t.Fatalf("image input %q was not parsed", id)
	}
	oid, ok := sess.Arena.ForNode(n.ID)
	if !ok {
		t.Fatalf("image input %q has no layout object", id)
	}
	return sess.Arena.Get(oid)
}

func imageInputCommandRect(t *testing.T, obj *layout.Object, commands []paint.DisplayCmd) (frame.Rect, bool) {
	t.Helper()
	x0, y0, x1, y1 := obj.ContentRect()
	want := frame.Rect4(int32(x0), int32(y0), int32(x1), int32(y1))
	for _, cmd := range commands {
		if cmd.Kind == paint.CmdImage && cmd.Rect == want && cmd.Image.Src != nil {
			return cmd.Rect, true
		}
	}
	return frame.Rect{}, false
}

func TestImageInputDataURINaturalSizeAndAuthoredDimensions(t *testing.T) {
	src := dataURL(solidPNG(t, 60, 30, color.RGBA{R: 200, G: 30, B: 120, A: 255}))
	html := `<!doctype html><html><body style="margin:0">` +
		`<input id="natural" type="image" src="` + src + `">` +
		`<input id="attrs" type="image" src="` + src + `" width="40" height="20">` +
		`</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)

	natural := imageInputObject(t, sess, "natural")
	if natural.W != 60 || natural.H != 30 {
		t.Errorf("natural image input = %vx%v, want 60x30", natural.W, natural.H)
	}
	if natural.Image == nil {
		t.Fatal("natural image input has no decoded pixels")
	}
	attrs := imageInputObject(t, sess, "attrs")
	if attrs.W != 40 || attrs.H != 20 {
		t.Errorf("authored image input = %vx%v, want 40x20", attrs.W, attrs.H)
	}
	if attrs.Image == nil {
		t.Fatal("authored image input has no decoded pixels")
	}

	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	commands := list.All()
	if _, ok := imageInputCommandRect(t, natural, commands); !ok {
		t.Error("natural image input emitted no CmdImage for its content box")
	}
	if _, ok := imageInputCommandRect(t, attrs, commands); !ok {
		t.Error("authored image input emitted no CmdImage for its 40x20 content box")
	}
}

func TestImageInputDynamicSourceAssignment(t *testing.T) {
	rt, err := js.New(js.Options{URL: "https://example.test/page"})
	if err != nil {
		t.Fatalf("js.New: %v", err)
	}
	src := dataURL(solidPNG(t, 60, 30, color.RGBA{R: 200, G: 30, B: 120, A: 255}))
	html := `<!doctype html><html><body><input id="dynamic" type="image"><script>
		setTimeout(function(){
			var input = document.getElementById('dynamic');
			input.onload = function(){ window.done = 'yes'; };
			input.src = '` + src + `';
		}, 0);
	</script></body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithMetrics(&fixedMetrics{advance: 7}),
	)
	if err != nil {
		rt.Close()
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)

	pumpUntilSet(t, sess, rt, "window.done")
	obj := imageInputObject(t, sess, "dynamic")
	if obj.W != 60 || obj.H != 30 {
		t.Errorf("dynamic image input = %vx%v, want 60x30", obj.W, obj.H)
	}
	if obj.Image == nil {
		t.Fatal("dynamic image input has no decoded pixels")
	}
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	if _, ok := imageInputCommandRect(t, obj, list.All()); !ok {
		t.Error("dynamic image input emitted no CmdImage")
	}
}
