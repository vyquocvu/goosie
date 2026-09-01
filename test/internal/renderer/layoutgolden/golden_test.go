package layoutgolden_test

import (
	"github.com/vyquocvu/goosie/internal/renderer/layoutgolden"
	"flag"
	"os"
	"testing"
)

// var update is set when running tests with -update. The GOOSIE_UPDATE_GOLDEN
// environment variable is the canonical toggler used by AssertGoldenLayout,
// but we also accept the flag for convenience during local iterations.
var update = flag.Bool("update", false, "update layout golden snapshots")

// fixtures bundles the HTML+CSS+viewport dimensions for a named layout
// regression case. Keeping the declarations in a single table ensures the
// golden snapshot list, the test list, and the determinism-guard list
// stay in sync as we add new fixtures.
type fixture struct {
	name      string
	html      string
	css       string
	viewportW float32
	viewportH float32
}

func fixtures() []fixture {
	return []fixture{
		{
			name: "block_stack",
			html: `<!DOCTYPE html>
<html>
<body>
  <div class="a">A</div>
  <div class="b">B</div>
  <div class="c">C</div>
</body>
</html>`,
			css: `
.a { width: 100px; height: 30px; }
.b { width: 200px; height: 40px; }
.c { width: 50px;  height: 20px; }
`,
			viewportW: 400,
			viewportH: 300,
		},
		{
			name: "box_model",
			html: `<!DOCTYPE html>
<html>
<body>
  <div class="padded">padded</div>
  <div class="margined">margined</div>
</body>
</html>`,
			css: `
.padded  { width: 100px; padding: 10px; border: 5px solid black; }
.margined { width: 100px; margin: 15px; }
`,
			viewportW: 400,
			viewportH: 300,
		},
		{
			name: "flex_row",
			html: `<!DOCTYPE html>
<html>
<body>
  <div class="container">
    <div class="a">A</div>
    <div class="b">B</div>
    <div class="c">C</div>
  </div>
</body>
</html>`,
			css: `
.container { display: flex; flex-direction: row; width: 300px; }
.a { width: 50px;  height: 20px; }
.b { width: 100px; height: 20px; }
.c { width: 50px;  height: 20px; }
`,
			viewportW: 400,
			viewportH: 300,
		},
		{
			name: "display_none",
			html: `<!DOCTYPE html>
<html>
<body>
  <div class="visible">visible</div>
  <div class="hidden">hidden</div>
</body>
</html>`,
			css: `
.visible { width: 100px; height: 20px; }
.hidden  { display: none; }
`,
			viewportW: 400,
			viewportH: 300,
		},
		{
			name: "nested_blocks",
			html: `<!DOCTYPE html>
<html>
<body>
  <div class="outer">
    <div class="inner-a">A</div>
    <div class="inner-b">B</div>
  </div>
</body>
</html>`,
			css: `
.outer   { width: 200px; height: 80px; }
.inner-a { width: 60px;  height: 30px; margin: 5px; }
.inner-b { width: 80px;  height: 30px; margin: 5px; }
`,
			viewportW: 400,
			viewportH: 300,
		},
	}
}

// TestMain propagates the -update flag to the GOOSIE_UPDATE_GOLDEN
// environment variable so all child tests share the same update
// semantics. This makes the convention identical to the raster golden
// tests in internal/renderer/frame/golden/.
func TestMain(m *testing.M) {
	if *update {
		os.Setenv("GOOSIE_UPDATE_GOLDEN", "1")
	}
	os.Exit(m.Run())
}

// TestGoldenLayouts walks the fixture table and runs the harness for
// each entry. Each fixture produces a deterministic text snapshot and
// is compared against testdata/golden-layout/<name>.txt.
func TestGoldenLayouts(t *testing.T) {
	for _, f := range fixtures() {
		f := f // capture
		t.Run(f.name, func(t *testing.T) {
			layoutgolden.AssertGoldenLayout(t, f.name, layoutgolden.DefaultConfig(), f.viewportW, f.viewportH, f.html, f.css)
		})
	}
}

// TestGoldenLayoutDeterminism asserts that running the layout pipeline
// twice for the same fixture produces a byte-identical snapshot. This
// is a regression guard for non-determinism (e.g., accidental map
// iteration order or counter drift) in the layout engine and its
// dependencies.
func TestGoldenLayoutDeterminism(t *testing.T) {
	for _, f := range fixtures() {
		f := f // capture
		t.Run(f.name, func(t *testing.T) {
			layoutgolden.RunDeterminismGuard(t, f.name, f.viewportW, f.viewportH, f.html, f.css)
		})
	}
}
