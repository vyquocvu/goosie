package ui

import (
	"fmt"
	"image/color"
	"reflect"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/vyquocvu/goosie/internal/renderer"
)

type styleProp struct {
	Name  string
	Value string
}

type ComputedStyleView struct {
	node      *renderer.RenderNode
	props     []styleProp
	filter    string
	filterBox *widget.Entry
	list      *fyne.Container
	box       fyne.CanvasObject
}

func NewComputedStyleView() *ComputedStyleView {
	v := &ComputedStyleView{
		filterBox: widget.NewEntry(),
		list:      container.NewVBox(),
	}
	v.filterBox.PlaceHolder = "Filter styles..."
	v.filterBox.OnChanged = func(s string) {
		v.filter = strings.TrimSpace(strings.ToLower(s))
		v.rebuild()
	}
	scroll := container.NewVScroll(v.list)
	v.box = container.NewBorder(v.filterBox, nil, nil, nil, scroll)
	return v
}

func (v *ComputedStyleView) CanvasObject() fyne.CanvasObject {
	return v.box
}

func (v *ComputedStyleView) SetNode(node *renderer.RenderNode) {
	v.node = node
	v.props = v.extractProps()
	v.rebuild()
}

func (v *ComputedStyleView) SetFilter(s string) {
	v.filterBox.SetText(s)
}

func (v *ComputedStyleView) VisibleCount() int {
	count := 0
	lowerFilter := v.filter
	for _, p := range v.props {
		if lowerFilter != "" && !containsLower(p.Name, lowerFilter) && !containsLower(p.Value, lowerFilter) {
			continue
		}
		count++
	}
	return count
}

func (v *ComputedStyleView) rebuild() {
	v.list.RemoveAll()
	lowerFilter := v.filter
	for _, p := range v.props {
		if lowerFilter != "" && !containsLower(p.Name, lowerFilter) && !containsLower(p.Value, lowerFilter) {
			continue
		}
		nameLabel := widget.NewLabel(p.Name)
		nameLabel.TextStyle.Monospace = true
		valLabel := widget.NewLabel(p.Value)
		valLabel.TextStyle.Monospace = true
		valLabel.Wrapping = fyne.TextWrapWord
		row := container.NewBorder(nil, nil, nameLabel, nil, valLabel)
		v.list.Add(row)
	}
	if len(v.props) == 0 {
		v.list.Add(widget.NewLabel("No element selected"))
	}
	v.list.Refresh()
}

func (v *ComputedStyleView) extractProps() []styleProp {
	if v.node == nil || v.node.ComputedStyle == nil {
		return nil
	}
	s := v.node.ComputedStyle
	rv := reflect.ValueOf(s).Elem()
	rt := rv.Type()
	var props []styleProp
	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)
		sf := rt.Field(i)
		if !sf.IsExported() || sf.Anonymous {
			continue
		}
		val := formatValue(field)
		if val == "" {
			continue
		}
		props = append(props, styleProp{
			Name:  cssFieldName(sf.Name),
			Value: val,
		})
	}
	return props
}

func cssFieldName(goName string) string {
	var result strings.Builder
	for i, ch := range goName {
		if i > 0 && ch >= 'A' && ch <= 'Z' {
			result.WriteRune('-')
		}
		result.WriteRune(ch)
	}
	return strings.ToLower(result.String())
}

func formatValue(fv reflect.Value) string {
	// Check if the value implements fmt.Stringer (covers atom types).
	// Zero-valued atoms (underlying uint8 == 0) are defaults and are
	// suppressed to keep the inspector concise.
	if fv.CanInterface() {
		if s, ok := fv.Interface().(fmt.Stringer); ok {
			if fv.Kind() >= reflect.Uint && fv.Kind() <= reflect.Uint64 && fv.Uint() == 0 {
				return ""
			}
			return s.String()
		}
	}
	switch fv.Kind() {
	case reflect.String:
		s := fv.String()
		if s == "" || s == "0" {
			return ""
		}
		return s
	case reflect.Float32, reflect.Float64:
		f := fv.Float()
		if f == 0 {
			return ""
		}
		return fmt.Sprintf("%g", f)
	case reflect.Int, reflect.Int32, reflect.Int64:
		i := fv.Int()
		if i == 0 {
			return ""
		}
		return fmt.Sprintf("%d", i)
	case reflect.Struct:
		if c, ok := fv.Interface().(color.Color); ok {
			r, g, b, a := c.RGBA()
			if a == 0 {
				return ""
			}
			if a == 0xffff {
				return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
			}
			return fmt.Sprintf("rgba(%d,%d,%d,%.2f)", r>>8, g>>8, b>>8, float64(a)/0xffff)
		}
	case reflect.Map:
		// Skip maps for now
		return ""
	}
	return ""
}

func containsLower(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), substr)
}
