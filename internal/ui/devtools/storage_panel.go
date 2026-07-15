package devtools

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type storagePanel struct {
	fyne.Container
	searchEntry *widget.Entry
	list        *widget.List
	entries     []storageRow
	onRefresh   func()
	mu          sync.Mutex
}

type storageRow struct {
	Origin string
	Key    string
	Value  string
}

func newStoragePanelContent(activeTab func() *TabContext) fyne.CanvasObject {
	p := &storagePanel{}

	p.searchEntry = widget.NewEntry()
	p.searchEntry.PlaceHolder = "Search origin or key..."
	searchBtn := widget.NewButton("Search", func() {
		p.filterEntries()
	})
	p.searchEntry.OnSubmitted = func(s string) {
		p.filterEntries()
	}

	clearBtn := widget.NewButton("Clear", func() {
		if cb := p.onRefresh; cb != nil {
			cb()
		}
	})

	p.list = widget.NewList(
		func() int { return len(p.entries) },
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id >= len(p.entries) {
				return
			}
			e := p.entries[id]
			label := obj.(*widget.Label)
			if e.Key == "" {
				// Origin header
				label.SetText(fmt.Sprintf("📁 %s  (%d keys)", e.Origin, countKeys(e)))
				label.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				label.SetText(fmt.Sprintf("    %s: %s", e.Key, truncateValue(e.Value, 60)))
				label.TextStyle = fyne.TextStyle{}
			}
		},
	)

	topBar := container.NewBorder(nil, nil,
		container.NewHBox(clearBtn, widget.NewLabel("Storage")),
		searchBtn,
		p.searchEntry,
	)

	content := container.NewBorder(topBar, nil, nil, nil, container.NewScroll(p.list))
	p.Container = *content
	p.onRefresh = func() {
		if activeTab != nil {
			ctx := activeTab()
			if ctx != nil && ctx.Storage != nil {
				p.refreshFrom(ctx.Storage)
			}
		}
	}

	return p
}

func (p *storagePanel) RefreshFrom(ctx *TabContext) {
	if ctx == nil || ctx.Storage == nil {
		return
	}
	p.refreshFrom(ctx.Storage)
}

func (p *storagePanel) refreshFrom(store storageProvider) {
	snapshot := store.Snapshot()
	p.buildRows(snapshot)
	p.filterEntries()
}

func (p *storagePanel) buildRows(snapshot map[string]map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var origins []string
	for origin := range snapshot {
		origins = append(origins, origin)
	}
	sort.Strings(origins)

	p.entries = nil
	for _, origin := range origins {
		kv := snapshot[origin]
		p.entries = append(p.entries, storageRow{Origin: origin})
		var keys []string
		for k := range kv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p.entries = append(p.entries, storageRow{Origin: origin, Key: k, Value: kv[k]})
		}
	}
}

func (p *storagePanel) filterEntries() {
	query := strings.TrimSpace(strings.ToLower(p.searchEntry.Text))
	p.mu.Lock()
	defer p.mu.Unlock()

	if query == "" {
		p.list.Refresh()
		return
	}

	// Show only matching origins
	var filtered []storageRow
	var originHasMatch = make(map[string]bool)
	for _, e := range p.entries {
		if e.Key == "" {
			continue // Skip origin headers, we'll add them back if they have matches
		}
		if strings.Contains(strings.ToLower(e.Origin), query) ||
			strings.Contains(strings.ToLower(e.Key), query) ||
			strings.Contains(strings.ToLower(e.Value), query) {
			originHasMatch[e.Origin] = true
		}
	}

	for _, e := range p.entries {
		if e.Key == "" {
			if originHasMatch[e.Origin] {
				filtered = append(filtered, e)
			}
		} else if originHasMatch[e.Origin] {
			filtered = append(filtered, e)
		}
	}

	p.entries = filtered
	p.list.Refresh()
}

func countKeys(e storageRow) int {
	return 0 // Placeholder — overridden in refresh
}

func truncateValue(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func collectOrigins(snapshot map[string]map[string]string) []string {
	var origins []string
	for o := range snapshot {
		origins = append(origins, o)
	}
	sort.Strings(origins)
	return origins
}

func filterSnapshot(snapshot map[string]map[string]string, query string) map[string]map[string]string {
	q := strings.ToLower(query)
	out := make(map[string]map[string]string)
	for origin, kv := range snapshot {
		if strings.Contains(strings.ToLower(origin), q) {
			out[origin] = kv
			continue
		}
		for k, v := range kv {
			if strings.Contains(strings.ToLower(k), q) || strings.Contains(strings.ToLower(v), q) {
				out[origin] = kv
				break
			}
		}
	}
	return out
}
