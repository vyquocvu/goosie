package devtools

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// column index for sorting
const (
	colMethod = iota
	colStatus
	colURL
	colType
	colSize
	colTime
)

type filterConfig struct {
	statusClass string // "", "2xx", "3xx", "4xx", "5xx"
	contentType string // "", "document", "stylesheet", "script", "image", "font", "other"
	search      string
}

type networkPanel struct {
	fyne.Container
	entries       []NetRequestEntry
	visible       []NetRequestEntry
	list          *widget.List
	detailLabel   *widget.Label
	detailBox     *fyne.Container
	clearBtn      *widget.Button
	preserveCheck *widget.Check

	filter filterConfig

	// Sort state
	sortCol    int
	sortAsc    bool

	// Type filter buttons (toggle group style)
	typeButtons map[string]*widget.Button
	statusBtns  map[string]*widget.Button
	searchEntry *widget.Entry
}

func (p *networkPanel) build() {
	p.typeButtons = make(map[string]*widget.Button)
	p.statusBtns = make(map[string]*widget.Button)

	p.list = widget.NewList(
		func() int { return len(p.visible) },
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(p.visible) {
				return
			}
			e := p.visible[id]
			obj.(*widget.Label).SetText(p.formatRow(e))
			obj.(*widget.Label).TextStyle.Monospace = true
		},
	)

	p.list.OnSelected = func(id widget.ListItemID) {
		if id >= len(p.visible) {
			return
		}
		p.showDetail(p.visible[id])
	}

	p.detailLabel = widget.NewLabel("")
	p.detailLabel.Wrapping = fyne.TextWrapWord
	p.detailBox = container.NewVBox()
	p.detailBox.Hide()

	filterBar := p.buildFilterBar()

	header := p.buildHeaderRow()
	listContent := container.NewBorder(header, nil, nil, nil, p.list)

	split := container.NewVSplit(
		container.NewBorder(nil, nil, nil, nil, listContent),
		container.NewBorder(nil, nil, nil, nil,
			container.NewVScroll(p.detailBox),
		),
	)
	split.Offset = 0.7

	contentBox := container.NewBorder(
		container.NewVBox(filterBar, widget.NewSeparator()),
		nil, nil, nil, split,
	)
	p.Container = *contentBox
}

func (p *networkPanel) buildHeaderRow() fyne.CanvasObject {
	headers := []struct {
		name string
		col  int
	}{
		{"Method", colMethod},
		{"Status", colStatus},
		{"URL", colURL},
		{"Type", colType},
		{"Size", colSize},
		{"Time", colTime},
	}

	var headerChildren []fyne.CanvasObject
	for _, h := range headers {
		h := h
		btn := widget.NewButton(h.name, func() {
			if p.sortCol == h.col {
				p.sortAsc = !p.sortAsc
			} else {
				p.sortCol = h.col
				p.sortAsc = true
			}
			p.applySort()
			p.list.Refresh()
		})
		btn.Importance = widget.LowImportance
		headerChildren = append(headerChildren, btn)
	}
	return container.NewBorder(nil, nil, nil, nil, container.NewGridWithColumns(len(headers), headerChildren...))
}

func (p *networkPanel) buildFilterBar() fyne.CanvasObject {
	p.clearBtn = widget.NewButton("Clear", func() {
		p.entries = nil
		p.rebuild()
	})

	p.preserveCheck = widget.NewCheck("Preserve log", nil)

	// Type filter buttons
	types := []string{"All", "document", "stylesheet", "script", "image", "font", "other"}
	for _, t := range types {
		t := t
		btn := widget.NewButton(t, func() {
			if t == "All" {
				p.filter.contentType = ""
			} else {
				p.filter.contentType = t
			}
			p.rebuild()
		})
		btn.Importance = widget.LowImportance
		p.typeButtons[t] = btn
	}

	// Status class filter buttons
	statuses := []string{"All", "2xx", "3xx", "4xx", "5xx"}
	for _, s := range statuses {
		s := s
		btn := widget.NewButton(s, func() {
			if s == "All" {
				p.filter.statusClass = ""
			} else {
				p.filter.statusClass = s
			}
			p.rebuild()
		})
		btn.Importance = widget.LowImportance
		p.statusBtns[s] = btn
	}

	p.searchEntry = widget.NewEntry()
	p.searchEntry.PlaceHolder = "Filter URL..."
	p.searchEntry.OnSubmitted = func(s string) {
		p.filter.search = strings.TrimSpace(s)
		p.rebuild()
	}
	searchBtn := widget.NewButton("Search", func() {
		p.filter.search = strings.TrimSpace(p.searchEntry.Text)
		p.rebuild()
	})

	typeRow := container.NewBorder(nil, nil, widget.NewLabel("Type:"), nil, container.NewHBox(p.typeButtons["All"], p.typeButtons["document"], p.typeButtons["stylesheet"], p.typeButtons["script"], p.typeButtons["image"], p.typeButtons["font"], p.typeButtons["other"]))
	statusRow := container.NewBorder(nil, nil, widget.NewLabel("Status:"), nil, container.NewHBox(p.statusBtns["All"], p.statusBtns["2xx"], p.statusBtns["3xx"], p.statusBtns["4xx"], p.statusBtns["5xx"]))

	topRow := container.NewBorder(nil, nil, container.NewHBox(p.clearBtn, p.preserveCheck), searchBtn, p.searchEntry)

	return container.NewVBox(topRow, typeRow, statusRow)
}

func (p *networkPanel) RefreshFrom(ctx *TabContext) {
	if ctx == nil || ctx.RequestLog == nil {
		return
	}
	entries := ctx.RequestLog.Entries()
	p.entries = entries
	p.rebuild()
}

func (p *networkPanel) rebuild() {
	p.visible = p.filterEntries()
	p.applySort()
	p.list.Refresh()
}

func (p *networkPanel) filterEntries() []NetRequestEntry {
	var out []NetRequestEntry
	for _, e := range p.entries {
		if p.filter.statusClass != "" && p.filter.statusClass != formatStatusClass(e.Status) {
			continue
		}
		if p.filter.contentType != "" {
			ct := formatContentType(e.ContentType)
			if ct != p.filter.contentType && !(p.filter.contentType == "other" && ct == "") {
				if ct != p.filter.contentType {
					continue
				}
			}
		}
		if p.filter.search != "" && !strings.Contains(strings.ToLower(e.URL), strings.ToLower(p.filter.search)) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func (p *networkPanel) applySort() {
	if p.sortCol == colTime {
		sort.SliceStable(p.visible, func(i, j int) bool {
			if p.sortAsc {
				return p.visible[i].Duration < p.visible[j].Duration
			}
			return p.visible[i].Duration > p.visible[j].Duration
		})
		return
	}

	sort.SliceStable(p.visible, func(i, j int) bool {
		var less bool
		switch p.sortCol {
		case colMethod:
			less = p.visible[i].Method < p.visible[j].Method
		case colStatus:
			less = p.visible[i].Status < p.visible[j].Status
		case colURL:
			less = p.visible[i].URL < p.visible[j].URL
		case colType:
			less = formatContentType(p.visible[i].ContentType) < formatContentType(p.visible[j].ContentType)
		case colSize:
			less = p.visible[i].Bytes < p.visible[j].Bytes
		default:
			less = false
		}
		if !p.sortAsc {
			return !less
		}
		return less
	})
}

func (p *networkPanel) formatRow(e NetRequestEntry) string {
	statusStr := fmt.Sprintf("%d", e.Status)
	if e.Error != "" {
		statusStr = "ERR"
	}

	waterfall := formatWaterfall(e.Duration)
	return fmt.Sprintf("%-7s %5s  %-60s %-11s %8s  %s",
		e.Method,
		statusStr,
		truncateMiddle(e.URL, 60),
		formatContentType(e.ContentType),
		formatBytes(e.Bytes),
		waterfall,
	)
}

func (p *networkPanel) showDetail(e NetRequestEntry) {
	p.detailBox.RemoveAll()

	statusStr := fmt.Sprintf("%d", e.Status)
	if e.Error != "" {
		statusStr = fmt.Sprintf("%d (%s)", e.Status, e.Error)
	}

	p.detailBox.Add(widget.NewLabelWithStyle("General", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	addDetailRow(p.detailBox, "Request URL:", e.URL)
	addDetailRow(p.detailBox, "Request Method:", e.Method)
	addDetailRow(p.detailBox, "Status Code:", statusStr)
	addDetailRow(p.detailBox, "Content Type:", e.ContentType)
	addDetailRow(p.detailBox, "Size:", formatBytes(e.Bytes))
	addDetailRow(p.detailBox, "Duration:", formatDuration(e.Duration))
	addDetailRow(p.detailBox, "Cache:", map[bool]string{true: "HIT", false: "MISS"}[e.CacheHit])

	p.detailBox.Add(widget.NewSeparator())
	p.detailBox.Add(widget.NewLabelWithStyle("Timing", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

	ms := e.Duration.Seconds() * 1000
	addDetailRow(p.detailBox, "Total:", fmt.Sprintf("%.0f ms", ms))
	addDetailRow(p.detailBox, "Waterfall:", formatWaterfall(e.Duration))

	p.detailBox.Show()
	p.detailBox.Refresh()
}

func addDetailRow(parent *fyne.Container, key, value string) {
	parent.Add(container.NewBorder(nil, nil, widget.NewLabel(key), nil, widget.NewLabel(value)))
}

func formatMethod(method string) string {
	return method
}

func formatStatusClass(status int) string {
	return fmt.Sprintf("%dxx", status/100)
}

func formatContentType(contentType string) string {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "text/html"):
		return "document"
	case strings.Contains(ct, "text/css"):
		return "stylesheet"
	case strings.Contains(ct, "javascript") || strings.Contains(ct, "ecmascript"):
		return "script"
	case strings.Contains(ct, "image/"):
		return "image"
	case strings.Contains(ct, "font/"):
		return "font"
	default:
		if ct == "" {
			return ""
		}
		return "other"
	}
}

func truncateMiddle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 5 {
		return s[:maxLen]
	}
	half := (maxLen - 3) / 2
	return s[:half] + "..." + s[len(s)-half:]
}

func formatWaterfall(d time.Duration) string {
	const maxBar = 20
	ms := d.Seconds() * 1000
	bars := int(ms / 50) // 50ms per bar
	if bars > maxBar {
		bars = maxBar
	}
	if bars < 1 && ms > 0 {
		bars = 1
	}
	return strings.Repeat("█", bars) + fmt.Sprintf(" %.0fms", ms)
}
