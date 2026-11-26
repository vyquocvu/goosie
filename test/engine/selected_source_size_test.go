package engine_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

func selectedSourceObject(t *testing.T, viewportW float32) (*engine.Session, dom.NodeID) {
	t.Helper()
	html := `<!doctype html><html><body style="margin:0"><picture>` +
		`<source media="(min-width: 600px)" width="288" height="384" srcset="large.png">` +
		`<source media="(max-width: 599px)" width="100" height="200" srcset="small.png">` +
		`<img id="pic" src="fallback.png" width="543" height="724" style="width:100px">` +
		`</picture></body></html>`
	sess, err := engine.NewSession(html, nil, viewportW,
		engine.WithViewportH(600), engine.WithMetrics(&fixedMetrics{advance: 7}))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(sess.Close)
	n := sess.Doc.ElementByID("pic")
	if n == nil {
		t.Fatal("selected img was not parsed")
	}
	if _, ok := sess.Arena.ForNode(n.ID); !ok {
		t.Fatal("selected img has no layout object")
	}
	return sess, n.ID
}

func TestSelectedSourceDimensionsOverrideDecodedAndImgAttributes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		viewportW   float32
		wantW       float32
		wantH       float32
		wantLayoutH float32
	}{
		{"wide source", 1280, 288, 384, 384},
		{"narrow source", 400, 100, 200, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess, nodeID := selectedSourceObject(t, tc.viewportW)
			size, ok := sess.Arena.NaturalSizes[nodeID]
			if !ok || !size.FromSource {
				t.Fatalf("natural size = %+v, present=%v, want selected-source size", size, ok)
			}
			if size.W != tc.wantW || size.H != tc.wantH {
				t.Errorf("natural size = %vx%v, want %vx%v", size.W, size.H, tc.wantW, tc.wantH)
			}
			oid, ok := sess.Arena.ForNode(nodeID)
			if !ok {
				t.Fatal("selected img has no layout object")
			}
			obj := sess.Arena.Get(oid)
			if obj.W != 100 {
				t.Errorf("img.W = %v, want 100", obj.W)
			}
			if math.Abs(float64(obj.H-tc.wantLayoutH)) >= 0.01 {
				t.Errorf("img.H = %v, want %v from the selected source presentational hint", obj.H, tc.wantLayoutH)
			}
		})
	}
}
