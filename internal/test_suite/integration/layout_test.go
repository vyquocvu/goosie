package integration

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestFlexboxLayout(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	r := renderer.NewRenderer(800, 600)

	html := `
		<html>
		<head>
			<style>
				body { margin: 0; padding: 0; }
				.container {
					display: flex;
					flex-direction: row;
					justify-content: space-between;
					width: 300px;
					height: 100px;
				}
				.item {
					width: 50px;
					height: 50px;
				}
			</style>
		</head>
		<body>
			<div class="container" id="flex-container">
				<div class="item" id="item1">1</div>
				<div class="item" id="item2">2</div>
				<div class="item" id="item3">3</div>
			</div>
		</body>
		</html>
	`

	_, err := r.RenderHTML(html)
	assert.NoError(t, err)

	// Hit test logic

	// Item 1: 0..50 (center 25, 25)
	node1, box1 := r.HitTest(25, 25)
	if assert.NotNil(t, box1) {
		assert.InDelta(t, float32(0), box1.Box.X, 1.0)
		if node1 != nil {
			id, _ := node1.GetAttribute("id")
			if id == "" && node1.Parent != nil {
				id, _ = node1.Parent.GetAttribute("id")
			}
			assert.Equal(t, "item1", id)
		}
	}

	// Item 2: (300-50)/2 = 125..175 (center 150, 25)
	node2, box2 := r.HitTest(150, 25)
	if assert.NotNil(t, box2) {
		assert.InDelta(t, float32(125), box2.Box.X, 1.0)
		if node2 != nil {
			id, _ := node2.GetAttribute("id")
			if id == "" && node2.Parent != nil {
				id, _ = node2.Parent.GetAttribute("id")
			}
			assert.Equal(t, "item2", id)
		}
	}

	// Item 3: 250..300 (center 275, 25)
	node3, box3 := r.HitTest(275, 25)
	if assert.NotNil(t, box3) {
		assert.InDelta(t, float32(250), box3.Box.X, 1.0)
		if node3 != nil {
			id, _ := node3.GetAttribute("id")
			if id == "" && node3.Parent != nil {
				id, _ = node3.Parent.GetAttribute("id")
			}
			assert.Equal(t, "item3", id)
		}
	}
}

func TestGridLayout(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	r := renderer.NewRenderer(800, 600)

	html := `
		<html>
		<head>
			<style>
				body { margin: 0; padding: 0; }
				.grid {
					display: grid;
					grid-template-columns: 100px 100px;
					gap: 10px;
					width: 210px;
				}
				.cell {
					height: 50px;
				}
			</style>
		</head>
		<body>
			<div class="grid">
				<div class="cell" id="c1">1</div>
				<div class="cell" id="c2">2</div>
				<div class="cell" id="c3">3</div>
				<div class="cell" id="c4">4</div>
			</div>
		</body>
		</html>
	`

	_, err := r.RenderHTML(html)
	assert.NoError(t, err)

	// Cell 1: 0,0 -> 50,25 (center)
	node1, box1 := r.HitTest(50, 25)
	if assert.NotNil(t, box1) {
		assert.InDelta(t, float32(0), box1.Box.X, 1.0)
		assert.InDelta(t, float32(0), box1.Box.Y, 1.0)
		if node1 != nil {
			id, _ := node1.GetAttribute("id")
			if id == "" && node1.Parent != nil {
				id, _ = node1.Parent.GetAttribute("id")
			}
			assert.Equal(t, "c1", id)
		}
	}

	// Cell 2: 110,0 -> 160,25 (center)
	node2, box2 := r.HitTest(160, 25)
	if assert.NotNil(t, box2) {
		assert.InDelta(t, float32(110), box2.Box.X, 1.0)
		assert.InDelta(t, float32(0), box2.Box.Y, 1.0)
		if node2 != nil {
			id, _ := node2.GetAttribute("id")
			if id == "" && node2.Parent != nil {
				id, _ = node2.Parent.GetAttribute("id")
			}
			assert.Equal(t, "c2", id)
		}
	}

	// Cell 3: 0,60 -> 50,85 (center)
	node3, box3 := r.HitTest(50, 85)
	if assert.NotNil(t, box3) {
		assert.InDelta(t, float32(0), box3.Box.X, 1.0)
		assert.InDelta(t, float32(60), box3.Box.Y, 1.0)
		if node3 != nil {
			id, _ := node3.GetAttribute("id")
			if id == "" && node3.Parent != nil {
				id, _ = node3.Parent.GetAttribute("id")
			}
			assert.Equal(t, "c3", id)
		}
	}
}
