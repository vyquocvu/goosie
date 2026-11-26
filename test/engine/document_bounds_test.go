package engine_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

const documentBoundsStaticPage = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">` +
	`<div style="width:400px;height:300px;margin:20px;background:rgb(200,0,0)"></div>` +
	`<div style="width:400px;height:60px;margin:20px 20px 0 20px;background:rgb(0,0,200)"></div>` +
	`</body></html>`

const documentBoundsTallPage = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0">` +
	`<div style="width:400px;height:300px;margin:20px;background:rgb(200,0,0)"></div>` +
	`<div style="width:400px;height:300px;margin:0"></div>` +
	`<div style="width:400px;height:60px;margin:20px 0 0 20px;background:rgb(0,0,200)"></div>` +
	`<div style="width:400px;height:600px;margin:0;%s"></div>` +
	`</body></html>`

func documentBoundsFixture(t *testing.T, html string, scale float32) (*engine.Session, frame.Rect) {
	t.Helper()
	sess, err := engine.NewSession(html, nil, 600,
		engine.WithViewportH(400),
		engine.WithDevicePixelRatio(scale),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)
	list, err := sess.PaintChecked(scale)
	if err != nil {
		t.Fatal(err)
	}
	return sess, list.Build(1).Extent()
}

func TestDocumentLayerBoundsIncludesUnpaintedTail(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, twin := range []struct {
			name string
			tail string
		}{
			{name: "PLAIN", tail: ""},
			{name: "PAINTED", tail: "background:rgb(250,250,250)"},
		} {
			t.Run(twin.name, func(t *testing.T) {
				html := fmt.Sprintf(documentBoundsTallPage, twin.tail)
				sess, painted := documentBoundsFixture(t, html, scale)
				wantPaintedH := int32(1300 * scale)
				if twin.name == "PLAIN" {
					wantPaintedH = 700 * int32(scale)
				}
				if painted.H() != wantPaintedH {
					t.Fatalf("painted height = %d, want %d", painted.H(), wantPaintedH)
				}
				got, err := sess.DocumentLayerBounds(painted, scale, frame.Size{W: int32(600 * scale), H: int32(400 * scale)})
				if err != nil {
					t.Fatal(err)
				}
				want := frame.Rect4(0, 0, int32(600*scale), int32(1320*scale))
				if got != want {
					t.Fatalf("DocumentLayerBounds = %v, want %v", got, want)
				}
			})
		}
	}
}

func TestDocumentLayerBoundsExcludesNonScrollableMargins(t *testing.T) {
	for _, tc := range []struct {
		name string
		html string
		want frame.Rect
	}{
		{
			name: "static collapsed margins",
			html: documentBoundsStaticPage,
			want: frame.Rect4(0, 0, 600, 400),
		},
		{
			name: "in-flow block right margin",
			html: `<!doctype html><style>body{margin:0}</style><body><div style="width:590px;height:100px;margin-right:20px"></div></body>`,
			want: frame.Rect4(0, 0, 600, 400),
		},
		{
			name: "absolute trailing margins",
			html: `<!doctype html><style>body{margin:0}</style><body><div style="position:absolute;left:590px;top:390px;width:100px;height:100px;margin:20px"></div></body>`,
			want: frame.Rect4(0, 0, 710, 510),
		},
		{
			name: "inline-block margin box",
			html: `<!doctype html><style>body{margin:0}</style><body><span style="display:inline-block;width:590px;height:100px;margin:20px"></span></body>`,
			want: frame.Rect4(0, 0, 630, 400),
		},
		{
			name: "in-flow trailing bottom margin",
			html: `<!doctype html><style>body{margin:0}</style><body><div style="width:100px;height:500px;margin:20px"></div></body>`,
			want: frame.Rect4(0, 0, 600, 540),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess, painted := documentBoundsFixture(t, tc.html, 1)
			got, err := sess.DocumentLayerBounds(painted, 1, frame.Size{W: 600, H: 400})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("DocumentLayerBounds = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDocumentLayerBoundsIncludesDeviceViewportForShortDocuments(t *testing.T) {
	sess, _ := documentBoundsFixture(t, `<!doctype html><html><body style="margin:0"><div style="width:100px;height:50px"></div></body></html>`, 1)
	got, err := sess.DocumentLayerBounds(frame.Rect{}, 1, frame.Size{W: 600, H: 400})
	if err != nil {
		t.Fatal(err)
	}
	if want := frame.Rect4(0, 0, 600, 400); got != want {
		t.Fatalf("DocumentLayerBounds = %v, want %v", got, want)
	}
}

func TestDocumentLayerBoundsUsesScrollableOverflow(t *testing.T) {
	for _, tc := range []struct {
		name string
		html string
		want frame.Rect
	}{
		{
			name: "absolute descendant",
			html: `<!doctype html><html><body style="margin:0"><div style="position:absolute;left:500px;top:900px;width:80px;height:40px"></div></body></html>`,
			want: frame.Rect4(0, 0, 600, 940),
		},
		{
			name: "fixed descendant",
			html: `<!doctype html><html><body style="margin:0"><div style="position:fixed;left:500px;top:900px;width:80px;height:40px"></div></body></html>`,
			want: frame.Rect4(0, 0, 600, 400),
		},
		{
			name: "nested overflow clip",
			html: `<!doctype html><html><body style="margin:0"><div style="height:100px;overflow:hidden"><div style="height:1000px"></div></div><div style="height:500px"></div></body></html>`,
			want: frame.Rect4(0, 0, 600, 600),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess, _ := documentBoundsFixture(t, tc.html, 1)
			got, err := sess.DocumentLayerBounds(frame.Rect{}, 1, frame.Size{W: 600, H: 400})
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("DocumentLayerBounds = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDocumentLayerBoundsUsesRootMarginBoxAndClampsOrigins(t *testing.T) {
	sess, err := engine.NewSession(`<!doctype html><html style="margin:10px 20px 30px 40px"><body style="margin:0"></body></html>`, nil, 200, engine.WithViewportH(20))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.Close)
	root := sess.Arena.Get(layout.ObjectID(1))
	root.X, root.Y, root.W, root.H = 40, 10, 100, 0
	root.MarginLeft, root.MarginRight, root.MarginTop, root.MarginBottom = 60, 80, 15, 60
	got, err := sess.DocumentLayerBounds(frame.Rect{}, 1, frame.Size{W: 200, H: 20})
	if err != nil {
		t.Fatal(err)
	}
	if want := frame.Rect4(0, 0, 220, 70); got != want {
		t.Fatalf("DocumentLayerBounds = %v, want %v", got, want)
	}
}

func TestDocumentLayerBoundsValidatesInputsAndExtent(t *testing.T) {
	sess, _ := documentBoundsFixture(t, `<!doctype html><html><body></body></html>`, 1)
	for _, scale := range []float32{0, -1, 9, float32(math.NaN()), float32(math.Inf(1))} {
		if _, err := sess.DocumentLayerBounds(frame.Rect{}, scale, frame.Size{W: 600, H: 400}); err == nil {
			t.Errorf("DocumentLayerBounds accepted scale %v", scale)
		}
	}
	for _, viewport := range []frame.Size{{W: 0, H: 400}, {W: 600, H: -1}, {W: engine.MaxDeviceDimension + 1, H: 1}, {W: 16384, H: 16384}} {
		if _, err := sess.DocumentLayerBounds(frame.Rect{}, 1, viewport); err == nil {
			t.Errorf("DocumentLayerBounds accepted viewport %v", viewport)
		}
	}
	if _, err := sess.DocumentLayerBounds(frame.Rect4(0, 0, 131073, 262144), 1, frame.Size{W: 600, H: 400}); err == nil {
		t.Error("DocumentLayerBounds accepted an unsafe painted extent")
	}
}
