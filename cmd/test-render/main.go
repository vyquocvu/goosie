// Command test-render verifies the rendering pipeline with debug instrumentation.
//
// It loads an HTML file, runs it through parse → style → layout → paint, and
// reports timing, render counts, and phase offsets without opening a window.
// Useful for CI and automated testing of rendering performance.
//
// Usage:
//
//	test-render [flags] <html-file>
//
// Flags:
//
//	-profile        Print pipeline profiling stats
//	-phase          Enable phase tracking (render counts per node)
//	-iterations N   Run N iterations (default 1)
//	-width N        Viewport width (default 1024)
//	-height N       Viewport height (default 768)
//	-dpr N          Device pixel ratio (default 2.0)
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vyquocvu/goosie/internal/debug"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/raster"
)

var (
	flagProfile    = flag.Bool("profile", false, "Print pipeline profiling stats")
	flagPhase      = flag.Bool("phase", false, "Enable phase tracking (render counts per node)")
	flagIterations = flag.Int("iterations", 1, "Run N iterations")
	flagWidth      = flag.Int("width", 1024, "Viewport width")
	flagHeight     = flag.Int("height", 768, "Viewport height")
	flagDPR        = flag.Float64("dpr", 2.0, "Device pixel ratio")
)

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: test-render [flags] <html-file>")
		flag.PrintDefaults()
		os.Exit(2)
	}

	// Initialize debug tools
	profiler := debug.NewProfiler(*flagProfile)
	phaseTracker := debug.NewPhaseTracker(*flagPhase)

	// Load HTML
	htmlBytes, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "test-render: %v\n", err)
		os.Exit(1)
	}
	html := string(htmlBytes)

	// Initialize fonts
	fonts, err := raster.NewFonts()
	if err != nil {
		fmt.Fprintf(os.Stderr, "test-render: fonts: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "test-render: testing %s (%d bytes)\n", flag.Arg(0), len(html))

	// Run iterations
	var totalStyle, totalLayout, totalPaint, totalDamage time.Duration

	for i := 0; i < *flagIterations; i++ {
		iterStart := time.Now()

		// Parse + Style
		profiler.Begin(debug.StageStyle)
		opts := []engine.Option{
			engine.WithMetrics(fonts),
			engine.WithViewportH(float32(*flagHeight)),
		}
		if *flagPhase {
			opts = append(opts, engine.WithPhaseTracker(phaseTracker))
		}
		sess, err := engine.NewSession(html, nil, float32(*flagWidth), opts...)
		profiler.End()
		if err != nil {
			fmt.Fprintf(os.Stderr, "test-render: session: %v\n", err)
			os.Exit(1)
		}
		styleTime := time.Since(iterStart)
		totalStyle += styleTime

		// Layout
		profiler.Begin(debug.StageLayout)
		if err := sess.Reflow(float32(*flagWidth)); err != nil {
			fmt.Fprintf(os.Stderr, "test-render: reflow: %v\n", err)
			os.Exit(1)
		}
		profiler.End()
		layoutTime := time.Since(iterStart) - styleTime
		totalLayout += layoutTime

		// Paint
		profiler.Begin(debug.StagePaint)
		scale := float32(*flagDPR)
		list, err := sess.PaintChecked(scale)
		if err != nil {
			fmt.Fprintf(os.Stderr, "test-render: paint: %v\n", err)
			os.Exit(1)
		}
		dl := list.Build(1)
		profiler.End()
		paintTime := time.Since(iterStart) - styleTime - layoutTime
		totalPaint += paintTime

		// Take snapshot before interaction
		if *flagPhase {
			phaseTracker.Snapshot()
		}

		// Simulate hover interaction
		profiler.Begin(debug.StageRestyle)
		sess.SetHover(100, 100)
		if sess.HasPseudoClassRule("hover") {
			// Use optimized hover path that skips reflow when possible
			if sess.IsHoverGeometryNeutral() {
				sess.RestyleForHover()
			} else {
				sess.RestyleAndReflow(float32(*flagWidth))
			}
		}
		profiler.End()

		// Repaint after interaction
		profiler.Begin(debug.StagePaint)
		list2, _ := sess.PaintChecked(scale)
		dl2 := list2.Build(1)
		profiler.End()

		// Compute damage
		profiler.Begin(debug.StageContentDamage)
		damage := paint.ContentDamage(dl, dl2)
		profiler.End()
		damageTime := time.Since(iterStart) - styleTime - layoutTime - paintTime
		totalDamage += damageTime

		if i == 0 {
			fmt.Fprintf(os.Stderr, "test-render: iteration 0:\n")
			fmt.Fprintf(os.Stderr, "  nodes: %d\n", countNodes(sess))
			fmt.Fprintf(os.Stderr, "  layout objects: %d\n", len(sess.Arena.Objects))
			fmt.Fprintf(os.Stderr, "  paint commands: %d\n", len(dl.All()))
			fmt.Fprintf(os.Stderr, "  style: %v\n", styleTime)
			fmt.Fprintf(os.Stderr, "  layout: %v\n", layoutTime)
			fmt.Fprintf(os.Stderr, "  paint: %v\n", paintTime)
			fmt.Fprintf(os.Stderr, "  damage: %v\n", damageTime)
			fmt.Fprintf(os.Stderr, "  damage rect: %v\n", damage)
		}

		// Detect phase offset after interaction
		if *flagPhase {
			deltas := phaseTracker.DetectPhaseOffset(0, "hover")
			if i == 0 && len(deltas) > 0 {
				fmt.Fprintf(os.Stderr, "\n  Phase offset detected after hover:\n")
				fmt.Fprintf(os.Stderr, "    nodes changed: %d\n", len(deltas))
				if len(deltas) > 0 {
					fmt.Fprintf(os.Stderr, "    most changed: node %d (%s.%s) style=%+d layout=%+d paint=%+d\n",
						deltas[0].NodeID, deltas[0].NodeTag, deltas[0].NodeClass,
						deltas[0].StyleDelta, deltas[0].LayoutDelta, deltas[0].PaintDelta)
				}
			}
		}
	}

	// Report averages
	if *flagIterations > 1 {
		n := time.Duration(*flagIterations)
		fmt.Fprintf(os.Stderr, "\ntest-render: averages over %d iterations:\n", *flagIterations)
		fmt.Fprintf(os.Stderr, "  style: %v\n", totalStyle/n)
		fmt.Fprintf(os.Stderr, "  layout: %v\n", totalLayout/n)
		fmt.Fprintf(os.Stderr, "  paint: %v\n", totalPaint/n)
		fmt.Fprintf(os.Stderr, "  damage: %v\n", totalDamage/n)
		fmt.Fprintf(os.Stderr, "  total: %v\n", (totalStyle+totalLayout+totalPaint+totalDamage)/n)
	}

	// Print profiling stats
	if *flagProfile {
		profiler.Print()
	}

	// Print phase tracking report
	if *flagPhase {
		phaseTracker.PrintReport()
	}

	fmt.Fprintf(os.Stderr, "\ntest-render: OK\n")
}

// countNodes counts the number of nodes in a session's DOM tree.
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
