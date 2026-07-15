package ui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/renderer"
)

type inlineProp struct {
	Name  string
	Value string
}

type InlineStyleEditor struct {
	onChange func(style string)
	node     *renderer.RenderNode
	props    []inlineProp
	list     *fyne.Container
	box      fyne.CanvasObject

	// Add property row
	addName  *widget.Entry
	addValue *widget.Entry
	addBtn   *widget.Button
}

func NewInlineStyleEditor(onChange func(style string)) *InlineStyleEditor {
	e := &InlineStyleEditor{
		onChange: onChange,
		list:     container.NewVBox(),
	}
	e.addName = widget.NewEntry()
	e.addName.PlaceHolder = "name"
	e.addValue = widget.NewEntry()
	e.addValue.PlaceHolder = "value"
	e.addBtn = widget.NewButton("+", func() {
		name := strings.TrimSpace(e.addName.Text)
		value := strings.TrimSpace(e.addValue.Text)
		if name == "" {
			return
		}
		e.AddProp(name, value)
		e.addName.SetText("")
		e.addValue.SetText("")
	})

	addRow := container.NewBorder(nil, nil, e.addName, e.addBtn, e.addValue)
	scroll := container.NewVScroll(e.list)
	e.box = container.NewBorder(nil, nil, nil, nil,
		container.NewVSplit(scroll, addRow),
	)
	return e
}

func (e *InlineStyleEditor) CanvasObject() fyne.CanvasObject {
	return e.box
}

func (e *InlineStyleEditor) SetNode(node *renderer.RenderNode) {
	e.node = node
	e.props = e.parseStyle()
	e.rebuild()
}

func (e *InlineStyleEditor) PropCount() int {
	return len(e.props)
}

func (e *InlineStyleEditor) HasColorValue(val string) bool {
	val = strings.TrimSpace(val)
	if val == "" {
		return false
	}
	// Check named CSS colors and hex colors
	if _, ok := namedColors[strings.ToLower(val)]; ok {
		return true
	}
	if strings.HasPrefix(val, "#") {
		return true
	}
	if strings.HasPrefix(val, "rgb(") || strings.HasPrefix(val, "rgba(") {
		return true
	}
	if strings.HasPrefix(val, "hsl(") || strings.HasPrefix(val, "hsla(") {
		return true
	}
	return false
}

func (e *InlineStyleEditor) UpdatePropValue(name, value string) {
	for i := range e.props {
		if e.props[i].Name == name {
			e.props[i].Value = value
			break
		}
	}
	e.emitChange()
	e.rebuild()
}

func (e *InlineStyleEditor) ToggleProp(name string, enabled bool) {
	if enabled {
		// Re-enable: the property is already in the list
		return
	}
	e.RemoveProp(name)
}

func (e *InlineStyleEditor) AddProp(name, value string) {
	// Don't add duplicates
	for _, p := range e.props {
		if p.Name == name {
			return
		}
	}
	e.props = append(e.props, inlineProp{Name: name, Value: value})
	e.emitChange()
	e.rebuild()
}

func (e *InlineStyleEditor) RemoveProp(name string) {
	for i := range e.props {
		if e.props[i].Name == name {
			e.props = append(e.props[:i], e.props[i+1:]...)
			break
		}
	}
	e.emitChange()
	e.rebuild()
}

func (e *InlineStyleEditor) parseStyle() []inlineProp {
	if e.node == nil {
		return nil
	}
	styleStr := e.node.Attrs["style"]
	if styleStr == "" {
		return nil
	}
	var props []inlineProp
	for _, part := range strings.Split(styleStr, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.IndexByte(part, ':')
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(part[:idx])
		value := strings.TrimSpace(part[idx+1:])
		if name == "" {
			continue
		}
		props = append(props, inlineProp{Name: name, Value: value})
	}
	return props
}

func (e *InlineStyleEditor) serialize() string {
	var parts []string
	for _, p := range e.props {
		parts = append(parts, p.Name+": "+p.Value)
	}
	return strings.Join(parts, "; ")
}

func (e *InlineStyleEditor) emitChange() {
	s := e.serialize()
	if e.onChange != nil {
		e.onChange(s)
	}
	if e.node != nil {
		if s == "" {
			delete(e.node.Attrs, "style")
		} else {
			e.node.Attrs["style"] = s
		}
	}
}

func (e *InlineStyleEditor) rebuild() {
	e.list.RemoveAll()
	for _, p := range e.props {
		p := p
		row := e.buildPropRow(p)
		e.list.Add(row)
	}
	e.list.Refresh()
}

func (e *InlineStyleEditor) buildPropRow(p inlineProp) fyne.CanvasObject {
	check := widget.NewCheck("", func(checked bool) {
		if !checked {
			e.ToggleProp(p.Name, false)
		}
	})
	check.Checked = true

	valEntry := widget.NewEntry()
	valEntry.SetText(p.Value)
	valEntry.OnSubmitted = func(s string) {
		e.UpdatePropValue(p.Name, s)
	}

	var swatch *canvas.Rectangle
	var swatchBox *fyne.Container
	if e.HasColorValue(p.Value) {
		swatch = canvas.NewRectangle(parseCSSColor(p.Value))
		swatch.Resize(fyne.NewSize(12, 12))
		swatchBox = container.NewWithoutLayout(swatch)
	}

	nameLabel := widget.NewLabel(p.Name)
	nameLabel.TextStyle.Monospace = true

	content := container.NewBorder(nil, nil, nameLabel, nil, valEntry)
	if swatchBox != nil {
		content = container.NewBorder(nil, nil, nameLabel, swatchBox, valEntry)
	}

	return container.NewBorder(nil, nil, check, nil, content)
}

// namedColors is a minimal set of CSS named colors for swatch detection.
var namedColors = map[string]color.RGBA{
	"black":       {0, 0, 0, 255},
	"white":       {255, 255, 255, 255},
	"red":         {255, 0, 0, 255},
	"green":       {0, 128, 0, 255},
	"blue":        {0, 0, 255, 255},
	"yellow":      {255, 255, 0, 255},
	"orange":      {255, 165, 0, 255},
	"purple":      {128, 0, 128, 255},
	"pink":        {255, 192, 203, 255},
	"gray":        {128, 128, 128, 255},
	"grey":        {128, 128, 128, 255},
	"transparent": {0, 0, 0, 0},
}

func parseCSSColor(s string) color.Color {
	s = strings.TrimSpace(strings.ToLower(s))
	if c, ok := namedColors[s]; ok {
		return c
	}
	if strings.HasPrefix(s, "#") {
		return parseHexColor(s)
	}
	return color.RGBA{R: 255, G: 0, B: 255, A: 255}
}

func parseHexColor(s string) color.Color {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.RGBA{R: 255, G: 0, B: 255, A: 255}
	}
	var r, g, b uint8
	n, _ := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	if n != 3 {
		return color.RGBA{R: 255, G: 0, B: 255, A: 255}
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}
}
