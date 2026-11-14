package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

type ListBox struct {
	Obj   *layout.Object
	Panel frame.RectF
	Rows  []ListBoxRow
}

type ListBoxRow struct {
	Rect     frame.RectF
	Index    int
	Label    string
	Selected bool
	Disabled bool
}

func (b *Builder) SetSelectListBox(lb *ListBox) {
	b.listbox = lb
}

var (
	ListboxPanelColor        = frame.RGB(252, 252, 252)
	ListboxBorderColor       = frame.RGB(118, 118, 118)
	ListboxHighlightColor    = frame.RGB(0, 120, 215)
	ListboxTextColor         = frame.RGB(0, 0, 0)
	ListboxSelectedTextColor = frame.RGB(255, 255, 255)
	ListboxDisabledTextColor = frame.RGB(148, 148, 148)
)

const listboxRowPadX = 2

func (b *Builder) paintSelectListBox() {
	lb := b.listbox
	if lb == nil || lb.Obj == nil || lb.Obj.Style == nil || len(lb.Rows) == 0 {
		return
	}
	panel := lb.Panel.ToDevice(b.scale)
	if panel.Empty() {
		return
	}
	b.list.Append(DisplayCmd{Kind: CmdFill, Rect: panel, Color: ListboxPanelColor, Opacity: 1})
	b.paintListBoxBorder(panel)

	s := lb.Obj.Style
	for _, row := range lb.Rows {
		r := row.Rect.ToDevice(b.scale)
		if r.Empty() {
			continue
		}
		if row.Selected {
			b.list.Append(DisplayCmd{Kind: CmdFill, Rect: r, Color: ListboxHighlightColor, Opacity: 1})
		}
		if row.Label == "" {
			continue
		}
		color := ListboxTextColor
		if row.Selected {
			color = ListboxSelectedTextColor
		}
		if row.Disabled {
			color = ListboxDisabledTextColor
		}
		textRect := r
		textRect.X0 += int32(listboxRowPadX * b.scale)
		b.appendRun(row.Label, textRect, s, color, 1)
	}
}

func (b *Builder) paintListBoxBorder(p frame.Rect) {
	strips := []frame.Rect{
		frame.Rect4(p.X0, p.Y0, p.X1, p.Y0+1),
		frame.Rect4(p.X0, p.Y1-1, p.X1, p.Y1),
		frame.Rect4(p.X0, p.Y0+1, p.X0+1, p.Y1-1),
		frame.Rect4(p.X1-1, p.Y0+1, p.X1, p.Y1-1),
	}
	for _, s := range strips {
		if s.Empty() {
			continue
		}
		b.list.Append(DisplayCmd{Kind: CmdFill, Rect: s, Color: ListboxBorderColor, Opacity: 1})
	}
}
