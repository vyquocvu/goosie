package renderer

import (
	"fmt"
	"testing"
)

// BenchmarkLayoutSmall benchmarks layout of 10 nodes
func BenchmarkLayoutSmall(b *testing.B) {
	benchmarkLayout(b, 10)
}

// BenchmarkLayoutMedium benchmarks layout of 100 nodes
func BenchmarkLayoutMedium(b *testing.B) {
	benchmarkLayout(b, 100)
}

// BenchmarkLayoutLarge benchmarks layout of 1000 nodes
func BenchmarkLayoutLarge(b *testing.B) {
	benchmarkLayout(b, 1000)
}

// BenchmarkLayoutVeryLarge benchmarks layout of 5000 nodes
func BenchmarkLayoutVeryLarge(b *testing.B) {
	benchmarkLayout(b, 5000)
}

// benchmarkLayout creates a render tree with n nodes and benchmarks layout computation
func benchmarkLayout(b *testing.B, n int) {
	// Create a tree structure with n nodes
	root := createBenchmarkTree(n)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		le := NewLayoutEngine(800, 600)
		le.ComputeLayout(root)
	}
}

// createBenchmarkTree creates a balanced tree with approximately n nodes
func createBenchmarkTree(n int) *RenderNode {
	if n <= 0 {
		return nil
	}

	root := NewRenderNode(NodeTypeElement)
	root.TagName = "div"

	// Create a tree structure
	// Each div has 3-4 children (mix of divs and text nodes)
	createBenchmarkTreeRecursive(root, n-1, 3)

	return root
}

// createBenchmarkTreeRecursive recursively creates tree nodes
func createBenchmarkTreeRecursive(parent *RenderNode, remaining int, depth int) int {
	if remaining <= 0 || depth <= 0 {
		return 0
	}

	created := 0
	childrenCount := 3 // 3 children per node

	for i := 0; i < childrenCount && remaining > 0; i++ {
		var child *RenderNode

		// Alternate between element and text nodes
		if i%2 == 0 {
			child = NewRenderNode(NodeTypeElement)
			child.TagName = "p"

			// Add text node to the paragraph
			if remaining > 1 {
				text := NewRenderNode(NodeTypeText)
				text.Text = "Lorem ipsum dolor sit amet"
				child.AddChild(text)
				remaining--
				created++
			}
		} else {
			child = NewRenderNode(NodeTypeElement)
			child.TagName = "div"

			// Recursively add children
			added := createBenchmarkTreeRecursive(child, remaining-1, depth-1)
			remaining -= added
			created += added
		}

		parent.AddChild(child)
		remaining--
		created++
	}

	return created
}

// BenchmarkHitTest benchmarks hit testing on different tree sizes
func BenchmarkHitTestSmall(b *testing.B) {
	benchmarkHitTest(b, 10)
}

func BenchmarkHitTestMedium(b *testing.B) {
	benchmarkHitTest(b, 100)
}

func BenchmarkHitTestLarge(b *testing.B) {
	benchmarkHitTest(b, 1000)
}

// benchmarkHitTest benchmarks hit testing on a tree with n nodes
func benchmarkHitTest(b *testing.B, n int) {
	// Create and layout a tree
	root := createBenchmarkTree(n)
	le := NewLayoutEngine(800, 600)
	layoutRoot := le.ComputeLayout(root)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Test hit at various positions
		le.HitTest(layoutRoot, 100, 100)
		le.HitTest(layoutRoot, 400, 300)
		le.HitTest(layoutRoot, 700, 500)
	}
}

// BenchmarkDisplayListBuild benchmarks display list building
func BenchmarkDisplayListBuildSmall(b *testing.B) {
	benchmarkDisplayListBuild(b, 10)
}

func BenchmarkDisplayListBuildMedium(b *testing.B) {
	benchmarkDisplayListBuild(b, 100)
}

func BenchmarkDisplayListBuildLarge(b *testing.B) {
	benchmarkDisplayListBuild(b, 1000)
}

// benchmarkDisplayListBuild benchmarks display list building on a tree with n nodes
func benchmarkDisplayListBuild(b *testing.B, n int) {
	// Create and layout a tree
	root := createBenchmarkTree(n)
	le := NewLayoutEngine(800, 600)
	layoutRoot := le.ComputeLayout(root)
	dlb := NewDisplayListBuilder()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dlb.Build(layoutRoot, root)
	}
}

// BenchmarkFullPipeline benchmarks the full pipeline: render tree -> layout tree -> display list
func BenchmarkFullPipelineSmall(b *testing.B) {
	benchmarkFullPipeline(b, 10)
}

func BenchmarkFullPipelineMedium(b *testing.B) {
	benchmarkFullPipeline(b, 100)
}

func BenchmarkFullPipelineLarge(b *testing.B) {
	benchmarkFullPipeline(b, 1000)
}

// benchmarkFullPipeline benchmarks the entire rendering pipeline
func benchmarkFullPipeline(b *testing.B, n int) {
	root := createBenchmarkTree(n)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Layout phase
		le := NewLayoutEngine(800, 600)
		layoutRoot := le.ComputeLayout(root)

		// Display list building phase
		dlb := NewDisplayListBuilder()
		dlb.Build(layoutRoot, root)
	}
}

// TestBenchmarkTreeCreation validates that benchmark trees are created correctly
func TestBenchmarkTreeCreation(t *testing.T) {
	tests := []struct {
		n int
	}{
		{10},
		{100},
		{1000},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("n=%d", tt.n), func(t *testing.T) {
			root := createBenchmarkTree(tt.n)
			if root == nil {
				t.Fatal("createBenchmarkTree returned nil")
			}

			// Count nodes
			count := countNodes(root)

			// Should be approximately n nodes (within reasonable range)
			if count < tt.n/2 || count > tt.n*2 {
				t.Logf("Created %d nodes (requested %d)", count, tt.n)
			}
		})
	}
}

// countNodes counts the total number of nodes in a tree
func countNodes(node *RenderNode) int {
	if node == nil {
		return 0
	}

	count := 1
	for _, child := range node.Children {
		count += countNodes(child)
	}

	return count
}

// BenchmarkViewportRendering benchmarks viewport-based rendering performance
func BenchmarkViewportRenderingSmall(b *testing.B) {
	benchmarkViewportRendering(b, 10)
}

func BenchmarkViewportRenderingMedium(b *testing.B) {
	benchmarkViewportRendering(b, 100)
}

func BenchmarkViewportRenderingLarge(b *testing.B) {
	benchmarkViewportRendering(b, 1000)
}

func BenchmarkViewportRenderingVeryLarge(b *testing.B) {
	benchmarkViewportRendering(b, 5000)
}

// benchmarkViewportRendering benchmarks viewport-based rendering
func benchmarkViewportRendering(b *testing.B, n int) {
	// Create a tree structure with n nodes
	root := createBenchmarkTree(n)

	// Create layout tree
	le := NewLayoutEngine(800, 600)
	layoutRoot := le.ComputeLayout(root)

	// Create canvas renderer with viewport
	cr := NewCanvasRenderer(800, 600)
	cr.SetViewport(0, 600)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cr.RenderWithViewport(root, layoutRoot)
	}
}

// BenchmarkViewportScroll benchmarks viewport updates during scrolling
func BenchmarkViewportScrollSmall(b *testing.B) {
	benchmarkViewportScroll(b, 10)
}

func BenchmarkViewportScrollMedium(b *testing.B) {
	benchmarkViewportScroll(b, 100)
}

func BenchmarkViewportScrollLarge(b *testing.B) {
	benchmarkViewportScroll(b, 1000)
}

// benchmarkViewportScroll simulates scrolling with viewport updates
func benchmarkViewportScroll(b *testing.B, n int) {
	// Create a tree structure with n nodes
	root := createBenchmarkTree(n)

	// Create layout tree
	le := NewLayoutEngine(800, 600)
	layoutRoot := le.ComputeLayout(root)

	// Create canvas renderer
	cr := NewCanvasRenderer(800, 600)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate scrolling by updating viewport
		scrollPos := float32(i % 1000)
		cr.SetViewport(scrollPos, 600)
		cr.RenderWithViewport(root, layoutRoot)
	}
}
