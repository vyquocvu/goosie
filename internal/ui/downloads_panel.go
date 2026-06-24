package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	goosienet "github.com/vyquocvu/goosie/internal/net"
)

type DownloadsPanel struct {
	container *fyne.Container
	list      *widget.List
	records   []goosienet.DownloadRecord
}

func NewDownloadsPanel() *DownloadsPanel {
	panel := &DownloadsPanel{records: []goosienet.DownloadRecord{}}
	panel.list = widget.NewList(func() int { return len(panel.records) }, func() fyne.CanvasObject { return widget.NewLabel("") }, func(id widget.ListItemID, object fyne.CanvasObject) {
		record := panel.records[id]
		object.(*widget.Label).SetText(fmt.Sprintf("%s %s %dB", record.Status, record.TargetPath, record.BytesWritten))
	})
	panel.container = container.NewBorder(widget.NewLabel("Downloads"), nil, nil, nil, panel.list)
	return panel
}
func (p *DownloadsPanel) SetRecords(records []goosienet.DownloadRecord) {
	p.records = append([]goosienet.DownloadRecord(nil), records...)
	p.list.Refresh()
}
func (p *DownloadsPanel) CanvasObject() fyne.CanvasObject { return p.container }
