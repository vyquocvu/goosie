package ui_test

import (
	"github.com/vyquocvu/goosie/internal/ui"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestInlineStyleEditor_ParseProps(t *testing.T) {
	ed := ui.NewInlineStyleEditor(nil)
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		Attrs:   map[string]string{"style": "color: red; font-size: 16px"},
	}
	ed.SetNode(node)
	assert.Equal(t, 2, ed.PropCount())
}

func TestInlineStyleEditor_NoInlineStyle(t *testing.T) {
	ed := ui.NewInlineStyleEditor(nil)
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
	}
	ed.SetNode(node)
	assert.Equal(t, 0, ed.PropCount())
}

func TestInlineStyleEditor_EmptyInlineStyle(t *testing.T) {
	ed := ui.NewInlineStyleEditor(nil)
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		Attrs:   map[string]string{"style": ""},
	}
	ed.SetNode(node)
	assert.Equal(t, 0, ed.PropCount())
}

func TestInlineStyleEditor_SkipsNonStyleAttrs(t *testing.T) {
	ed := ui.NewInlineStyleEditor(nil)
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		Attrs: map[string]string{
			"id":    "main",
			"class": "foo",
			"style": "display: block",
		},
	}
	ed.SetNode(node)
	assert.Equal(t, 1, ed.PropCount())
}

func TestInlineStyleEditor_NilNode(t *testing.T) {
	ed := ui.NewInlineStyleEditor(nil)
	ed.SetNode(nil)
	assert.Equal(t, 0, ed.PropCount())
}

func TestInlineStyleEditor_Serialize(t *testing.T) {
	var lastStyle string
	ed := ui.NewInlineStyleEditor(func(style string) {
		lastStyle = style
	})
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		Attrs:   map[string]string{"style": "color: red; font-size: 16px"},
	}
	ed.SetNode(node)
	assert.Equal(t, 2, ed.PropCount())

	// Update a property value
	ed.UpdatePropValue("color", "blue")
	assert.Equal(t, "color: blue; font-size: 16px", lastStyle)
}

func TestInlineStyleEditor_ToggleProp(t *testing.T) {
	var lastStyle string
	ed := ui.NewInlineStyleEditor(func(style string) {
		lastStyle = style
	})
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		Attrs:   map[string]string{"style": "color: red; font-size: 16px"},
	}
	ed.SetNode(node)

	// Disable a property
	ed.ToggleProp("color", false)
	assert.Equal(t, "font-size: 16px", lastStyle)
}

func TestInlineStyleEditor_AddProp(t *testing.T) {
	var lastStyle string
	ed := ui.NewInlineStyleEditor(func(style string) {
		lastStyle = style
	})
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		Attrs:   map[string]string{"style": "color: red"},
	}
	ed.SetNode(node)

	ed.AddProp("display", "flex")
	assert.Equal(t, "color: red; display: flex", lastStyle)
}

func TestInlineStyleEditor_RemoveProp(t *testing.T) {
	var lastStyle string
	ed := ui.NewInlineStyleEditor(func(style string) {
		lastStyle = style
	})
	node := &renderer.RenderNode{
		ID:      1,
		TagName: "div",
		Attrs:   map[string]string{"style": "color: red; font-size: 16px"},
	}
	ed.SetNode(node)

	ed.RemoveProp("color")
	assert.Equal(t, "font-size: 16px", lastStyle)
}

func TestInlineStyleEditor_ColorSwatchDetect(t *testing.T) {
	ed := ui.NewInlineStyleEditor(nil)
	assert.True(t, ed.HasColorValue("red"))
	assert.True(t, ed.HasColorValue("#ff0000"))
	assert.True(t, ed.HasColorValue("rgb(255, 0, 0)"))
	assert.True(t, ed.HasColorValue("rgba(255, 0, 0, 0.5)"))
	assert.True(t, ed.HasColorValue("blue"))
	assert.False(t, ed.HasColorValue("16px"))
	assert.False(t, ed.HasColorValue("block"))
}
