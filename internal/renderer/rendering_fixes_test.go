package renderer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverflowHiddenDoesNotPanic(t *testing.T) {
	htmlStr := `<html><head><style>
		.clip { overflow: hidden; width: 100px; height: 50px; }
	</style></head><body><div class="clip"><p>Overflowing content here</p></div></body></html>`
	r := NewRenderer(800, 600)
	r.SetTestingMode(true)
	obj, err := r.RenderHTML(context.Background(), htmlStr)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
}

func TestCollectFixed(t *testing.T) {
	htmlStr := `<html><head><style>
		.fixed { position: fixed; top: 0; left: 0; width: 100%; height: 50px; }
	</style></head><body>
		<div class="fixed">header</div>
		<p>content</p>
	</body></html>`
	doc := mustParseHTML(t, htmlStr)
	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManagerWithViewport(stylesheet, 800, 600)
	sm.ApplyStyles(renderTree)
	le := NewLayoutEngine(800, 600)
	layoutTree := le.ComputeLayout(renderTree)

	var fixedBoxes []*LayoutBox
	collectFixed(layoutTree, &fixedBoxes)
	assert.Greater(t, len(fixedBoxes), 0, "should find at least one fixed-position box")
}

func TestZIndexSort(t *testing.T) {
	htmlStr := `<html><head><style>
		.under { position: relative; z-index: 1; background: blue; width:100px; height:100px; }
		.over  { position: relative; z-index: 10; background: red;  width:50px;  height:50px;  }
	</style></head><body>
		<div class="under"></div>
		<div class="over"></div>
	</body></html>`
	doc := mustParseHTML(t, htmlStr)
	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManagerWithViewport(stylesheet, 800, 600)
	sm.ApplyStyles(renderTree)
	le := NewLayoutEngine(800, 600)
	layoutTree := le.ComputeLayout(renderTree)

	dlb := NewDisplayListBuilder()
	dl := dlb.Build(layoutTree, renderTree)
	SortByZIndex(dl)

	var blueIdx, redIdx int = -1, -1
	for i, cmd := range dl.Commands {
		if cmd.Type == PaintRect && cmd.FillColor != nil {
			r, g, b, _ := cmd.FillColor.RGBA()
			if b > r && b > g && blueIdx == -1 {
				blueIdx = i
			} else if r > g && r > b && redIdx == -1 {
				redIdx = i
			}
		}
	}
	if blueIdx != -1 && redIdx != -1 {
		assert.Greater(t, redIdx, blueIdx, "z-index:10 (red) should paint after z-index:1 (blue)")
	}
}
