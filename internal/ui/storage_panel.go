package ui

import (
	"fmt"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type StoragePanel struct {
	container *fyne.Container
	label     *widget.Label
}

func NewStoragePanel() *StoragePanel {
	panel := &StoragePanel{label: widget.NewLabel("No storage data")}
	panel.label.Wrapping = fyne.TextWrapWord
	panel.container = container.NewBorder(widget.NewLabel("Storage"), nil, nil, nil, panel.label)
	return panel
}
func (p *StoragePanel) SetSnapshot(snapshot map[string]map[string]string) {
	origins := make([]string, 0, len(snapshot))
	for origin := range snapshot {
		origins = append(origins, origin)
	}
	sort.Strings(origins)
	var lines strings.Builder
	for _, origin := range origins {
		lines.WriteString(origin + "\n")
		keys := make([]string, 0, len(snapshot[origin]))
		for key := range snapshot[origin] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&lines, "  %s = %s\n", key, snapshot[origin][key])
		}
	}
	text := lines.String()
	if text == "" {
		text = "No storage data"
	}
	p.label.SetText(text)
}
func (p *StoragePanel) CanvasObject() fyne.CanvasObject { return p.container }
