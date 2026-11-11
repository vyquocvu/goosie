// Command debug-render tests the rendering pipeline with debug instrumentation.
//
// It loads an HTML file, runs it through the full pipeline (parse → style →
// layout → paint), and reports timing and damage statistics. In bench mode,
// it simulates interactions and measures performance.
//
// Usage:
//
//	debug-render [flags] <html-file>
//
// Flags:
//
//	-profile        Print pipeline profiling stats
//	-frames N       Run N frames in bench mode (default 100)
//	-width N        Viewport width (default 1024)
//	-height N       Viewport height (default 768)
//	-dpr N          Device pixel ratio (default 2.0)
//	-scene name     Use synthetic scene instead of HTML file
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vyquocvu/goosie/internal/debug"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

var (
	flagProfile = flag.Bool("profile", false, "Print pipeline profiling stats")
	flagFrames  = flag.Int("frames", 100, "Run N frames in bench mode")
	flagWidth   = flag.Int("width", 1024, "Viewport width")
	flagHeight  = flag.Int("height", 768, "Viewport height")
	flagDPR     = flag.Float64("dpr", 2.0, "Device pixel ratio")
	flagScene   = flag.String("scene", "", "Use synthetic scene")
)

func main() {
	flag.Parse()
	if flag.NArg() < 1 && *flagScene == "" {
		fmt.Fprintln(os.Stderr, "usage: debug-render [flags] <html-file>")
		fmt.Fprintln(os.Stderr, "   or: debug-render -scene <name>")
		flag.PrintDefaults()
		os.Exit(2)
	}

	// Initialize debug tools
	debugRenderer := debug.NewRenderer(*flagProfile)
	debugProfiler := debug.NewProfiler(*flagProfile)

	// Load HTML or create synthetic scene
	var html string
	var err error
	if *flagScene != "" {
		html = syntheticScene(*flagScene)
		fmt.Fprintf(os.Stderr, "debug-render: using synthetic scene %q\n", *flagScene)
	} else {
		htmlBytes, err := os.ReadFile(flag.Arg(0))
		if err != nil {
			fmt.Fprintf(os.Stderr, "debug-render: %v\n", err)
			os.Exit(1)
		}
		html = string(htmlBytes)
		fmt.Fprintf(os.Stderr, "debug-render: loaded %s (%d bytes)\n", flag.Arg(0), len(html))
	}

	// Initialize fonts
	fonts, err := raster.NewFonts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug-render: fonts: %v\n", err)
		os.Exit(1)
	}

	// Create session with debug instrumentation
	debugProfiler.Begin(debug.StageStyle)
	sess, err := engine.NewSession(html, nil, float32(*flagWidth),
		engine.WithMetrics(fonts),
		engine.WithViewportH(float32(*flagHeight)),
	)
	debugProfiler.End()

	if err != nil {
		fmt.Fprintf(os.Stderr, "debug-render: session: %v\n", err)
		os.Exit(1)
	}

	// Layout
	debugProfiler.Begin(debug.StageLayout)
	if err := sess.Reflow(float32(*flagWidth)); err != nil {
		fmt.Fprintf(os.Stderr, "debug-render: reflow: %v\n", err)
		os.Exit(1)
	}
	debugProfiler.End()

	// Paint
	debugProfiler.Begin(debug.StagePaint)
	scale := float32(*flagDPR)
	list, err := sess.PaintChecked(scale)
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug-render: paint: %v\n", err)
		os.Exit(1)
	}
	dl := list.Build(1)
	debugProfiler.End()

	fmt.Fprintf(os.Stderr, "debug-render: parsed %d nodes, %d layout objects, %d paint commands\n",
		countNodes(sess), len(sess.Arena.Objects), len(dl.All()))

	// Run bench mode
	runBench(sess, dl, scale, *flagFrames, debugRenderer, debugProfiler)
}

// runBench runs N frames simulating interactions, collecting stats.
func runBench(sess *engine.Session, dl *paint.LayerDL, scale float32, frames int, renderer *debug.Renderer, profiler *debug.Profiler) {
	fmt.Fprintf(os.Stderr, "debug-render: running %d frames in bench mode\n", frames)

	start := time.Now()
	for i := 0; i < frames; i++ {
		renderer.BeginFrame(uint64(i), "bench")

		// Simulate hover interaction every 10 frames
		if i%10 == 0 {
			profiler.Begin(debug.StageRestyle)
			sess.SetHover(float32(i%800), float32(i%600))
			if sess.HasPseudoClassRule("hover") {
				sess.RestyleAndReflow(float32(*flagWidth))
			}
			profiler.End()
		}

		// Simulate click interaction every 20 frames
		if i%20 == 0 {
			profiler.Begin(debug.StageRestyle)
			sess.FocusControl(float32(i%800), float32(i%600))
			if sess.HasPseudoClassRule("focus") {
				sess.RestyleAndReflow(float32(*flagWidth))
			}
			profiler.End()
		}

		// Repaint
		profiler.Begin(debug.StagePaint)
		list, _ := sess.PaintChecked(scale)
		newDL := list.Build(1)
		profiler.End()

		// Compute damage
		profiler.Begin(debug.StageContentDamage)
		damage := paint.ContentDamage(dl, newDL)
		profiler.End()

		renderer.EndFrame(0, 0, 1, 0, 0, []frame.Rect{damage})
		dl = newDL
	}

	elapsed := time.Since(start)
	fmt.Fprintf(os.Stderr, "debug-render: %d frames in %v (%.1f fps)\n",
		frames, elapsed, float64(frames)/elapsed.Seconds())

	profiler.Print()
	renderer.PrintStats()
}

// countNodes counts the number of nodes in a DOM tree.
func countNodes(sess *engine.Session) int {
	if sess == nil || sess.Doc == nil {
		return 0
	}
	count := 0
	sess.Doc.Node.Walk(func(n *dom.Node) bool {
		count++
		return true
	})
	return count
}

// syntheticScene returns HTML for a synthetic test scene.
func syntheticScene(name string) string {
	switch name {
	case "checkerboard":
		return `<html><body style="margin:0">
<div style="display:grid; grid-template-columns:repeat(10,100px); grid-template-rows:repeat(10,100px)">
` + repeat(`<div style="border:1px solid black; background:#eee">Cell</div>`, 100) + `
</div></body></html>`
	case "hover":
		return `<html><body style="margin:20px">
<style>
.box { width:100px; height:100px; margin:10px; background:blue; display:inline-block; }
.box:hover { background:red; }
</style>
` + repeat(`<div class="box"></div>`, 50) + `
</body></html>`
	case "select":
		return `<html><body style="margin:20px">
<p>` + repeat("Lorem ipsum dolor sit amet. ", 100) + `</p>
</body></html>`
	case "focus":
		return `<html><body style="margin:20px">
<style>
input:focus { outline: 2px solid blue; }
</style>
` + repeat(`<input type="text" style="margin:5px"><br>`, 20) + `
</body></html>`
	default:
		return `<html><body><h1>Unknown scene: ` + name + `</h1></body></html>`
	}
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
