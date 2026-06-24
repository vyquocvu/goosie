package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	goosienet "github.com/vyquocvu/goosie/internal/net"
)

type NetworkPanel struct {
	container *fyne.Container
	list      *widget.List
	entries   []goosienet.RequestLogEntry
}

func NewNetworkPanel() *NetworkPanel {
	panel := &NetworkPanel{entries: []goosienet.RequestLogEntry{}}
	panel.list = widget.NewList(func() int { return len(panel.entries) }, func() fyne.CanvasObject { return widget.NewLabel("") }, func(id widget.ListItemID, object fyne.CanvasObject) {
		entry := panel.entries[id]
		object.(*widget.Label).SetText(fmt.Sprintf("%s %d %s %dB", entry.Method, entry.Status, entry.URL, entry.Bytes))
	})
	panel.container = container.NewBorder(widget.NewLabel("Network"), nil, nil, nil, panel.list)
	return panel
}

func (p *NetworkPanel) SetEntries(entries []goosienet.RequestLogEntry) {
	p.entries = append([]goosienet.RequestLogEntry(nil), entries...)
	p.list.Refresh()
}
func (p *NetworkPanel) CanvasObject() fyne.CanvasObject { return p.container }
