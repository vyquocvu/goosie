package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// SelectableText is a custom widget that displays text that can be selected and copied
// It extends widget.Entry but appears as read-only text with selection capabilities
type SelectableText struct {
	*widget.Entry
	text string
}

// NewSelectableText creates a new selectable text widget
func NewSelectableText(text string) *SelectableText {
	st := &SelectableText{
		Entry: widget.NewMultiLineEntry(),
		text:  text,
	}

	// Set the text
	st.Entry.SetText(text)

	// Make it appear as read-only but still selectable
	st.Entry.Disable()

	// Configure appearance to look more like a label
	st.Entry.Wrapping = fyne.TextWrapWord

	return st
}

// SetText updates the text content
func (st *SelectableText) SetText(text string) {
	st.text = text
	st.Entry.SetText(text)
}

// GetText returns the current text content
func (st *SelectableText) GetText() string {
	return st.text
}

// SelectedText returns the currently selected text
func (st *SelectableText) SelectedText() string {
	return st.Entry.SelectedText()
}

// CopySelectedText copies the selected text to clipboard
func (st *SelectableText) CopySelectedText(clipboard fyne.Clipboard) {
	selected := st.SelectedText()
	if selected != "" {
		clipboard.SetContent(selected)
	}
}

// CopyAllText copies all text to clipboard
func (st *SelectableText) CopyAllText(clipboard fyne.Clipboard) {
	clipboard.SetContent(st.text)
}

// TappedSecondary shows a popup menu to copy text
func (st *SelectableText) TappedSecondary(e *fyne.PointEvent) {
	copyItem := fyne.NewMenuItem("Copy", func() {
		st.CopySelectedText(fyne.CurrentApp().Clipboard())
	})
	if st.SelectedText() == "" {
		copyItem.Disabled = true
	}

	copyAllItem := fyne.NewMenuItem("Copy All", func() {
		st.CopyAllText(fyne.CurrentApp().Clipboard())
	})

	canvas := fyne.CurrentApp().Driver().CanvasForObject(st)

	// Create and show the popup menu
	popup := widget.NewPopUpMenu(fyne.NewMenu("", copyItem, copyAllItem), canvas)
	popup.ShowAtPosition(e.AbsolutePosition)
}

// SetWrapping sets the text wrapping mode
func (st *SelectableText) SetWrapping(wrap fyne.TextWrap) {
	st.Entry.Wrapping = wrap
}

// SetTextStyle sets the text style (bold, italic, etc.)
func (st *SelectableText) SetTextStyle(style fyne.TextStyle) {
	st.Entry.TextStyle = style
}

// MinSize returns the minimum size of the widget
func (st *SelectableText) MinSize() fyne.Size {
	return st.Entry.MinSize()
}

// Resize resizes the widget
func (st *SelectableText) Resize(size fyne.Size) {
	st.Entry.Resize(size)
}

// Move moves the widget to the specified position
func (st *SelectableText) Move(pos fyne.Position) {
	st.Entry.Move(pos)
}

// Position returns the current position of the widget
func (st *SelectableText) Position() fyne.Position {
	return st.Entry.Position()
}

// Refresh refreshes the widget
func (st *SelectableText) Refresh() {
	st.Entry.Refresh()
}

// Show shows the widget
func (st *SelectableText) Show() {
	st.Entry.Show()
}

// Hide hides the widget
func (st *SelectableText) Hide() {
	st.Entry.Hide()
}

// Visible returns whether the widget is visible
func (st *SelectableText) Visible() bool {
	return st.Entry.Visible()
}
