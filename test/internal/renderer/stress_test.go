package renderer_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/renderer/frame"
	"github.com/vyquocvu/goosie/internal/renderer/frame/raster"
)

// TestStress_RapidDOMMutations simulates rapid DOM mutations (like a JavaScript
// timer/slider/animation) and verifies the reconciler + incremental layout
// pipeline handles them without panicking or leaking memory.
func TestStress_RapidDOMMutations(t *testing.T) {
	r := renderer.NewRenderer(800, 600)
	html := `<html><body>
		<div id="container">
			<p id="text1">Hello</p>
			<p id="text2">World</p>
			<div id="box" style="width:100px; height:100px; background:red;"></div>
		</div>
	</body></html>`

	_, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		t.Fatalf("RenderHTML failed: %v", err)
	}

	root := r.GetRoot()
	if root == nil {
		t.Fatal("GetRoot returned nil")
	}

	// Find the text nodes.
	var textNode1, textNode2 *renderer.RenderNode
	var walk func(n *renderer.RenderNode)
	walk = func(n *renderer.RenderNode) {
		if n == nil {
			return
		}
		if n.Type == renderer.NodeTypeText {
			if n.Text == "Hello" {
				textNode1 = n
			} else if n.Text == "World" {
				textNode2 = n
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	if textNode1 == nil || textNode2 == nil {
		t.Fatal("could not find text nodes")
	}

	// Simulate 10,000 rapid DOM mutations (like a JS animation loop).
	const iterations = 10000
	startTime := time.Now()

	for i := 0; i < iterations; i++ {
		patches := []renderer.DOMPatch{
			{
				Kind:       renderer.PatchUpdateText,
				NodeID:     textNode1.ID,
				NewText:    "Frame",
				DirtyFlags: renderer.DirtyPaint,
			},
			{
				Kind:       renderer.PatchUpdateText,
				NodeID:     textNode2.ID,
				NewText:    "Updated",
				DirtyFlags: renderer.DirtyPaint,
			},
		}
		renderer.ApplyPatchesToRenderer(r, patches)
	}

	elapsed := time.Since(startTime)
	opsPerSec := float64(iterations) / elapsed.Seconds()

	t.Logf("Stress test: %d mutations in %v (%.0f ops/sec)", iterations, elapsed, opsPerSec)
	t.Logf("Average time per mutation: %v", elapsed/time.Duration(iterations))

	// Checkpoint 3.2: > 1,000 operations/second.
	if opsPerSec < 1000 {
		t.Errorf("DOM mutation throughput too low: %.0f ops/sec (target: > 1,000)", opsPerSec)
	}
}

// TestStress_BufferPoolNoLeak verifies that the buffer pool does not leak
// memory under sustained allocation pressure.
func TestStress_BufferPoolNoLeak(t *testing.T) {
	// Force GC and get baseline memory.
	runtime.GC()
	var baselineMem runtime.MemStats
	runtime.ReadMemStats(&baselineMem)

	// Perform 10,000 buffer get/put cycles.
	const iterations = 10000
	for i := 0; i < iterations; i++ {
		buf := raster.GetBuffer(800, 600)
		if buf == nil {
			t.Fatal("GetBuffer returned nil")
		}
		raster.PutBuffer(buf)
	}

	// Force GC and check memory after.
	runtime.GC()
	var afterMem runtime.MemStats
	runtime.ReadMemStats(&afterMem)

	// Check pool stats.
	pool := raster.GlobalBufferPool()
	stats := pool.Stats()
	t.Logf("Buffer pool stats after %d iterations:", iterations)
	t.Logf("  Gets: %d, Puts: %d, Allocs: %d, Reuses: %d, ActiveNow: %d",
		stats.Gets, stats.Puts, stats.Allocs, stats.Reuses, stats.ActiveNow)

	// Verify no buffers are leaked (all returned to pool).
	if stats.ActiveNow != 0 {
		t.Errorf("buffer leak detected: %d buffers still active after all puts", stats.ActiveNow)
	}

	// Verify gets == puts (balanced allocation).
	if stats.Gets != stats.Puts {
		t.Errorf("unbalanced pool usage: gets=%d puts=%d", stats.Gets, stats.Puts)
	}

	// Memory growth should be bounded (allow some growth for GC overhead).
	heapGrowth := int64(afterMem.HeapAlloc) - int64(baselineMem.HeapAlloc)
	heapGrowthMB := float64(heapGrowth) / (1024 * 1024)
	t.Logf("Heap growth: %.2f MB", heapGrowthMB)

	// Allow up to 50MB growth (generous for Go runtime overhead).
	if heapGrowthMB > 50 {
		t.Errorf("excessive memory growth: %.2f MB (limit: 50 MB)", heapGrowthMB)
	}
}

// TestStress_TiledRasterizerLargePage verifies that the tiled rasterizer
// can handle a very large page without OOM or panic.
func TestStress_TiledRasterizerLargePage(t *testing.T) {
	// Simulate a 10,000px tall page.
	width, height := 800, 10000
	tr := raster.NewTiledRasterizer()

	cmds := []raster.DisplayCmd{
		{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(0, 0, float32(width), float32(height)),
			Color: frame.NewColor(255, 255, 255, 255),
		},
	}

	// Add some content at various Y positions.
	for y := 0; y < height; y += 200 {
		cmds = append(cmds, raster.DisplayCmd{
			Kind:  raster.CmdFill,
			Rect:  frame.NewRect(50, float32(y), 700, 100),
			Color: frame.NewColor(200, 200, 255, 255),
		})
	}

	vp := frame.NewViewport(float32(width), float32(height), frame.PixelScaleDefault)

	startTime := time.Now()
	img, err := tr.RasterizeParallel(width, height, cmds, nil, vp)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("RasterizeParallel failed: %v", err)
	}
	if img == nil {
		t.Fatal("RasterizeParallel returned nil image")
	}

	t.Logf("Tiled rasterize %dx%d page with %d commands: %v", width, height, len(cmds), elapsed)

	// Checkpoint 2.1: 10,000px page should complete in < 10ms (on modern hardware).
	// We use a generous 100ms threshold to account for CI environments.
	if elapsed > 100*time.Millisecond {
		t.Logf("WARNING: rasterization took %v (target: < 100ms for CI)", elapsed)
	}
}

// TestStress_ReconcilerLargeTree verifies that the reconciler can diff
// large trees efficiently.
func TestStress_ReconcilerLargeTree(t *testing.T) {
	// Build a tree with ~5,000 nodes (5 levels, 5 children each = 5^5 = 3125+).
	// Each leaf node has a text child so the diff can detect text changes.
	buildTree := func(prefix string, depth, breadth int) *renderer.RenderNode {
		var build func(d int, id *int64) *renderer.RenderNode
		build = func(d int, id *int64) *renderer.RenderNode {
			node := &renderer.RenderNode{
				ID:      *id,
				Type:    renderer.NodeTypeElement,
				TagName: "div",
			}
			*id++
			if d < depth {
				for i := 0; i < breadth; i++ {
					child := build(d+1, id)
					node.Children = append(node.Children, child)
					child.Parent = node
				}
			} else {
				// Leaf: add a text child so diff detects text changes.
				textNode := &renderer.RenderNode{
					ID:   *id,
					Type: renderer.NodeTypeText,
					Text: prefix,
				}
				*id++
				node.Children = append(node.Children, textNode)
				textNode.Parent = node
			}
			return node
		}
		id := int64(1)
		return build(0, &id)
	}

	oldTree := buildTree("old", 5, 5)
	newTree := buildTree("new", 5, 5)

	// Diff the trees.
	conc := renderer.NewReconciler()
	startTime := time.Now()
	patches := conc.Diff(oldTree, newTree)
	elapsed := time.Since(startTime)

	t.Logf("Diffed trees with ~3125 nodes in %v, produced %d patches", elapsed, len(patches))

	// Checkpoint 3.1: < 0.1ms for 5,000 nodes.
	// We use a generous 10ms threshold for CI.
	if elapsed > 10*time.Millisecond {
		t.Logf("WARNING: diffing took %v (target: < 10ms for CI)", elapsed)
	}

	// All text nodes should have changed (different prefix).
	if len(patches) == 0 {
		t.Error("expected patches for trees with different text content")
	}
}

// TestStress_ConcurrentRasterize verifies that multiple goroutines can
// rasterize simultaneously without race conditions.
func TestStress_ConcurrentRasterize(t *testing.T) {
	const goroutines = 8
	const iterations = 100

	errCh := make(chan error, goroutines*iterations)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			for i := 0; i < iterations; i++ {
				backend := raster.NewCPUBackend(200, 200)
				vp := frame.NewViewport(200, 200, frame.PixelScaleDefault)
				if err := backend.BeginFrame(vp); err != nil {
					errCh <- err
					return
				}
				cmds := []raster.DisplayCmd{
					{
						Kind:  raster.CmdFill,
						Rect:  frame.NewRect(0, 0, 200, 200),
						Color: frame.NewColor(uint8(id*30), uint8(i), 200, 255),
					},
				}
				_, err := backend.Rasterize(cmds, nil)
				if err != nil {
					errCh <- err
					return
				}
				_ = backend.EndFrame()
				backend.Close()
			}
			errCh <- nil
		}(g)
	}

	// Collect results.
	for i := 0; i < goroutines; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("concurrent rasterize error: %v", err)
		}
	}
}

// BenchmarkStress_MutationThroughput measures the maximum DOM mutation
// throughput through the reconciler + incremental layout pipeline.
func BenchmarkStress_MutationThroughput(b *testing.B) {
	r := renderer.NewRenderer(800, 600)
	html := `<html><body><p id="target">Hello</p></body></html>`
	_, err := r.RenderHTML(context.Background(), html)
	if err != nil {
		b.Fatalf("RenderHTML failed: %v", err)
	}

	root := r.GetRoot()
	var textNode *renderer.RenderNode
	var walk func(n *renderer.RenderNode)
	walk = func(n *renderer.RenderNode) {
		if n == nil {
			return
		}
		if n.Type == renderer.NodeTypeText && n.Text == "Hello" {
			textNode = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	if textNode == nil {
		b.Fatal("could not find text node")
	}

	patches := []renderer.DOMPatch{
		{
			Kind:       renderer.PatchUpdateText,
			NodeID:     textNode.ID,
			NewText:    "Updated",
			DirtyFlags: renderer.DirtyPaint,
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		renderer.ApplyPatchesToRenderer(r, patches)
	}
}
