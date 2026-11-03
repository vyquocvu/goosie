package paint

import (
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/layout"
)

// ListBox is a <select> whose option rows a pointer can pick: the panel and its
// rows arrive in document CSS px, already resolved by the engine from the
// control's live box and option list.
//
// The engine owns that geometry (internal/engine/listbox.go) and hands the same
// rows to the builder it uses for hit testing, so a row cannot be drawn where
// the pointer lands on a different one. Paint only turns the model into ink.
type ListBox struct {
	// Obj is the control's layout box, whose style the rows share.
	Obj *layout.Object
	// Panel is the listbox background, border included.
	Panel frame.RectF
	// Rows are the option rows in tree order, inset inside Panel and tiling it.
	Rows []ListBoxRow
}

// ListBoxRow is one option: where it sits and what it says.
type ListBoxRow struct {
	Rect     frame.RectF
	Label    string
	Selected bool
	Disabled bool
}

// SetSelectListBox marks the open listbox to paint. It is emitted after the
// document walk, so no ancestor's overflow or later box can cover a row.
func (b *Builder) SetSelectListBox(lb *ListBox) {
	b.listbox = lb
}

// The listbox palette. A closed <select> opens a native popup whose colours no
// page script can read, so these are the engine's own UA colours, chosen to read
// like the in-page listbox Chromium paints for <select size> - a light panel, a
// grey hairline, one blue row marking the selection.
//
// They are declared, never assigned: internal/archtest treats a package var that
// nothing writes as configuration, not state.
var (
	ListboxPanelColor        = frame.RGB(252, 252, 252)
	ListboxBorderColor       = frame.RGB(118, 118, 118)
	ListboxHighlightColor    = frame.RGB(0, 120, 215)
	ListboxTextColor         = frame.RGB(0, 0, 0)
	ListboxSelectedTextColor = frame.RGB(255, 255, 255)
	ListboxDisabledTextColor = frame.RGB(148, 148, 148)
)

// listboxRowPadX is the gap between a row's left edge and its first glyph, in
// CSS px. The row rect itself is what the pointer hits; this only insets the
// label so text never touches the panel border.
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

// paintListBoxBorder draws the panel's one-pixel frame as four strips, so the
// rows inside keep their exact rects: insetting the rows is the engine's job and
// the pointer's, and a border must not shift either.
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
