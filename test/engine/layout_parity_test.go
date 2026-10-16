package engine_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/vyquocvu/goosie/internal/archtest"
	"github.com/vyquocvu/goosie/internal/engine"
)

// layoutTolerancePx is the allowed per-edge difference between our border
// box and Chromium's getBoundingClientRect. Fixtures use fixed pixel sizes
// with no text metrics in play, so geometry should agree up to rounding;
// the tolerance exists for half-pixel rounding only, and any systematic
// drift is a layout bug, not font noise.
const layoutTolerancePx = 1

type layoutRefBox struct {
	X, Y, W, H int
}

type layoutReference struct {
	Viewport [2]int                  `json:"viewport"`
	Boxes    map[string]layoutRefBox `json:"boxes"`
}

// TestLayoutParityWithChromium lays every testdata/layout-parity fixture out
// and compares each id-bearing element's border box against the Chromium
// reference (testdata/gen_layout_refs.py regenerates them; never hand-edit).
// A missing reference fails rather than skips: an unscored fixture is not
// a passing one.
func TestLayoutParityWithChromium(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	root, err := archtest.RepoRoot(file)
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	dir := filepath.Join(root, "testdata", "layout-parity")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("layout-parity dir: %v", err)
	}
	var fixtures []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".html" {
			fixtures = append(fixtures, e.Name())
		}
	}
	sort.Strings(fixtures)
	if len(fixtures) == 0 {
		t.Fatal("no layout-parity fixtures")
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			html, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			refData, err := os.ReadFile(filepath.Join(dir, name[:len(name)-len(".html")]+".reference.json"))
			if err != nil {
				t.Fatalf("missing reference for %s (run testdata/gen_layout_refs.py): %v", name, err)
			}
			var ref layoutReference
			if err := json.Unmarshal(refData, &ref); err != nil {
				t.Fatalf("bad reference for %s: %v", name, err)
			}
			if ref.Viewport != [2]int{800, 600} {
				t.Fatalf("%s reference viewport = %v, want [800 600]", name, ref.Viewport)
			}
			sess, err := engine.NewSession(string(html), nil, 800, engine.WithViewportH(600))
			if err != nil {
				t.Fatalf("NewSession: %v", err)
			}
			defer sess.Close()
			ids := make([]string, 0, len(ref.Boxes))
			for id := range ref.Boxes {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			for _, id := range ids {
				node := sess.Doc.ElementByID(id)
				if node == nil {
					t.Errorf("%s: element #%s missing from our DOM", name, id)
					continue
				}
				objID, ok := sess.Arena.ForNode(node.ID)
				if !ok {
					t.Errorf("%s: #%s has no layout object", name, id)
					continue
				}
				x0, y0, x1, y1 := sess.Arena.Get(objID).BorderRect()
				got := layoutRefBox{
					X: int(math.Round(float64(x0))), Y: int(math.Round(float64(y0))),
					W: int(math.Round(float64(x1 - x0))), H: int(math.Round(float64(y1 - y0))),
				}
				want := ref.Boxes[id]
				for _, axis := range []struct {
					name string
					got  int
					want int
				}{
					{"x", got.X, want.X}, {"y", got.Y, want.Y},
					{"w", got.W, want.W}, {"h", got.H, want.H},
				} {
					d := axis.got - axis.want
					if d < 0 {
						d = -d
					}
					if d > layoutTolerancePx {
						t.Errorf("%s #%s %s = %d, want %d (Chromium)", name, id, axis.name, axis.got, axis.want)
					}
				}
			}
		})
	}
}
