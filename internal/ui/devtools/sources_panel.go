package devtools

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Empty-state and informational copy for the Sources panel.
const (
	sourcesSelectHint  = "Select a resource to view its source."
	sourcesEmptyState  = "No resources loaded.\nLoad a page to inspect its sources."
	sourcesNotCaptured = "Response body not captured by the network cache.\n" +
		"Only the main document and cache-retained bodies are viewable."
)

// sourceResource is one row of the Sources tree: the main document or a
// sub-resource observed by the network layer.
type sourceResource struct {
	URL         string
	Name        string
	Type        string // document | stylesheet | script | image | font | other
	Method      string
	Status      int
	ContentType string
	Bytes       int64
	CacheHit    bool
	Error       string
	Duration    time.Duration
	Content     string
	HasContent  bool
}

// sourceCategory groups resources in the tree, Chrome DevTools style.
type sourceCategory struct {
	Type  string
	Title string
}

func sourceCategories() []sourceCategory {
	return []sourceCategory{
		{Type: "document", Title: "Documents"},
		{Type: "stylesheet", Title: "Stylesheets"},
		{Type: "script", Title: "Scripts"},
		{Type: "image", Title: "Images"},
		{Type: "font", Title: "Fonts"},
		{Type: "other", Title: "Other"},
	}
}

func treeCategoryUID(t string) string { return "cat:" + t }
func treeResourceUID(i int) string    { return "res:" + strconv.Itoa(i) }
func treeRootUID() string             { return "" }

func parseResourceUID(uid string) int {
	if !strings.HasPrefix(uid, "res:") {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimPrefix(uid, "res:"))
	if err != nil {
		return -1
	}
	return n
}

// sourcesPanel is the interactive Sources DevTools tab: a grouped resource
// tree on the left and a monospace, line-numbered source viewer on the right.
type sourcesPanel struct {
	fyne.Container

	activeTab func() *TabContext

	all     []sourceResource
	visible []sourceResource
	filter  string

	tree        *widget.Tree
	sourceView  *widget.Label
	gutterView  *widget.Label
	detailLabel *widget.Label
	statusLabel *widget.Label
	filterEntry *widget.Entry
	refreshBtn  *widget.Button
	copyBtn     *widget.Button
	emptyLabel  *widget.Label
	treeStack   *fyne.Container

	selectedURL string
	seenCats    map[string]bool
}

func newSourcesPanel(activeTab func() *TabContext) *sourcesPanel {
	p := &sourcesPanel{
		activeTab: activeTab,
		seenCats:  make(map[string]bool),
	}

	// --- Left: resource tree -------------------------------------------------
	p.tree = widget.NewTree(
		func(uid widget.TreeNodeID) []widget.TreeNodeID {
			if uid == treeRootUID() {
				return p.categoryUIDs()
			}
			if strings.HasPrefix(uid, "cat:") {
				return p.resourceUIDsInCategory(strings.TrimPrefix(uid, "cat:"))
			}
			return nil
		},
		func(uid widget.TreeNodeID) bool {
			return uid == treeRootUID() || strings.HasPrefix(uid, "cat:")
		},
		func(branch bool) fyne.CanvasObject {
			l := widget.NewLabel("")
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(uid widget.TreeNodeID, branch bool, obj fyne.CanvasObject) {
			l := obj.(*widget.Label)
			if branch {
				l.TextStyle = fyne.TextStyle{Bold: true}
				l.SetText(p.categoryLabel(strings.TrimPrefix(uid, "cat:")))
				return
			}
			l.TextStyle = fyne.TextStyle{}
			if idx := parseResourceUID(uid); idx >= 0 && idx < len(p.visible) {
				l.SetText(p.visible[idx].Name)
			}
		},
	)
	p.tree.OnSelected = func(uid widget.TreeNodeID) {
		if idx := parseResourceUID(uid); idx >= 0 && idx < len(p.visible) {
			p.showResource(p.visible[idx])
		}
	}

	p.emptyLabel = widget.NewLabel(sourcesEmptyState)
	p.emptyLabel.Alignment = fyne.TextAlignCenter
	p.emptyLabel.TextStyle = fyne.TextStyle{Italic: true}
	p.treeStack = container.NewMax(p.emptyLabel)

	// --- Right: detail header + line-numbered source -------------------------
	p.detailLabel = widget.NewLabel("")
	p.detailLabel.Truncation = fyne.TextTruncateEllipsis
	p.detailLabel.TextStyle = fyne.TextStyle{Monospace: true}

	p.gutterView = widget.NewLabel("")
	p.gutterView.TextStyle = fyne.TextStyle{Monospace: true}
	p.gutterView.Alignment = fyne.TextAlignTrailing

	p.sourceView = widget.NewLabel(sourcesSelectHint)
	p.sourceView.TextStyle = fyne.TextStyle{Monospace: true}
	p.sourceView.Selectable = true
	p.sourceView.Wrapping = fyne.TextWrapOff

	codeArea := container.NewBorder(nil, nil,
		container.NewVBox(p.gutterView, widget.NewSeparator()),
		nil,
		p.sourceView,
	)
	sourceScroll := container.NewScroll(codeArea)
	rightSide := container.NewBorder(
		container.NewVBox(p.detailLabel, widget.NewSeparator()),
		nil, nil, nil,
		sourceScroll,
	)

	// --- Toolbar --------------------------------------------------------------
	p.refreshBtn = widget.NewButton("Refresh", func() { p.refreshFromActiveTab() })
	p.copyBtn = widget.NewButton("Copy", func() { p.copySelection() })
	p.copyBtn.Disable()

	p.filterEntry = widget.NewEntry()
	p.filterEntry.PlaceHolder = "Filter resources by URL..."
	p.filterEntry.OnChanged = func(s string) {
		p.filter = strings.TrimSpace(s)
		p.applyFilter()
	}

	toolbar := container.NewBorder(nil, nil,
		container.NewHBox(p.refreshBtn, p.copyBtn),
		nil,
		p.filterEntry,
	)

	// --- Status bar -----------------------------------------------------------
	p.statusLabel = widget.NewLabel("No resources")
	p.statusLabel.TextStyle = fyne.TextStyle{Monospace: true}

	// --- Layout ---------------------------------------------------------------
	split := container.NewHSplit(p.treeStack, rightSide)
	split.Offset = 0.30

	content := container.NewBorder(
		toolbar,
		container.NewVBox(widget.NewSeparator(), p.statusLabel),
		nil, nil,
		split,
	)
	p.Container = *content
	return p
}

// RefreshFrom implements refreshablePanel; called by the dock on tab switch
// and by the Refresh toolbar button via refreshFromActiveTab.
func (p *sourcesPanel) RefreshFrom(ctx *TabContext) {
	if ctx == nil {
		return
	}
	p.all = p.buildResources(ctx)
	p.applyFilter()
}

func (p *sourcesPanel) refreshFromActiveTab() {
	if p.activeTab == nil {
		return
	}
	p.RefreshFrom(p.activeTab())
}

// buildResources maps live engine data (raw document source + network request
// log + HTTP cache bodies) into the panel's resource model.
func (p *sourcesPanel) buildResources(ctx *TabContext) []sourceResource {
	var out []sourceResource
	indexByURL := make(map[string]int)

	if ctx.RawSource != "" || ctx.CurrentURL != "" {
		doc := sourceResource{
			URL:         ctx.CurrentURL,
			Name:        sourceBaseName(ctx.CurrentURL),
			Type:        "document",
			ContentType: "text/html",
			Content:     ctx.RawSource,
			HasContent:  ctx.RawSource != "",
		}
		indexByURL[ctx.CurrentURL] = 0
		out = append(out, doc)
	}

	if ctx.RequestLog == nil {
		return out
	}
	for _, e := range ctx.RequestLog.Entries() {
		if e.URL == "" {
			continue
		}
		// The main document fetch enriches the document resource rather
		// than producing a duplicate tree entry.
		if idx, seen := indexByURL[e.URL]; seen {
			mergeLogEntry(&out[idx], e)
			continue
		}
		r := sourceResource{
			URL:         e.URL,
			Name:        sourceBaseName(e.URL),
			Type:        classifySourceType(e.ContentType, e.URL),
			Method:      e.Method,
			Status:      e.Status,
			ContentType: e.ContentType,
			Bytes:       e.Bytes,
			CacheHit:    e.CacheHit,
			Error:       e.Error,
			Duration:    e.Duration,
		}
		if isTextualSourceType(r.Type) && ctx.SourceCache != nil {
			if body, ok := ctx.SourceCache.CachedBody(e.URL); ok {
				r.Content = body
				r.HasContent = true
			}
		}
		indexByURL[e.URL] = len(out)
		out = append(out, r)
	}
	return out
}

func mergeLogEntry(r *sourceResource, e NetRequestEntry) {
	r.Method = e.Method
	r.Status = e.Status
	if e.ContentType != "" {
		r.ContentType = e.ContentType
	}
	r.Bytes = e.Bytes
	r.CacheHit = e.CacheHit
	r.Error = e.Error
	r.Duration = e.Duration
}

// applyFilter recomputes the visible slice from the current filter, then
// refreshes tree, status bar, and selection-dependent views.
func (p *sourcesPanel) applyFilter() {
	q := strings.ToLower(p.filter)
	p.visible = p.visible[:0]
	for _, r := range p.all {
		if q == "" ||
			strings.Contains(strings.ToLower(r.URL), q) ||
			strings.Contains(strings.ToLower(r.Name), q) {
			p.visible = append(p.visible, r)
		}
	}
	p.syncTree()
	p.updateStatus()
	p.restoreSelection()
}

func (p *sourcesPanel) syncTree() {
	if p.treeStack == nil {
		return
	}
	if len(p.visible) > 0 {
		p.treeStack.Objects = []fyne.CanvasObject{p.tree}
	} else {
		p.treeStack.Objects = []fyne.CanvasObject{p.emptyLabel}
	}
	if fyne.CurrentApp() == nil {
		return
	}
	p.openNewCategories()
	p.tree.Refresh()
	p.treeStack.Refresh()
}

// openNewCategories auto-expands category branches the first time they
// appear, while respecting branches the user collapsed afterwards.
func (p *sourcesPanel) openNewCategories() {
	for _, uid := range p.categoryUIDs() {
		if !p.seenCats[uid] {
			p.seenCats[uid] = true
			p.tree.OpenBranch(uid)
		}
	}
}

func (p *sourcesPanel) updateStatus() {
	if len(p.all) == 0 {
		p.statusLabel.SetText("No resources")
		return
	}
	var total int64
	for _, r := range p.all {
		total += r.Bytes
	}
	if p.filter != "" {
		p.statusLabel.SetText(fmt.Sprintf("%d of %d resources · %s",
			len(p.visible), len(p.all), formatBytes(total)))
		return
	}
	p.statusLabel.SetText(fmt.Sprintf("%d resources · %s", len(p.all), formatBytes(total)))
}

// restoreSelection re-renders the currently selected resource after a data
// refresh, or resets the viewer when the selection vanished (tab switch).
func (p *sourcesPanel) restoreSelection() {
	if p.selectedURL != "" {
		for _, r := range p.visible {
			if r.URL == p.selectedURL {
				p.showResource(r)
				return
			}
		}
		p.selectedURL = ""
	}
	p.showEmptySelection()
}

func (p *sourcesPanel) showEmptySelection() {
	p.detailLabel.SetText("")
	p.gutterView.SetText("")
	p.sourceView.SetText(sourcesSelectHint)
	p.copyBtn.Disable()
}

func (p *sourcesPanel) showResource(r sourceResource) {
	p.selectedURL = r.URL
	p.detailLabel.SetText(formatResourceDetail(r))

	switch {
	case r.HasContent:
		p.sourceView.SetText(r.Content)
		p.gutterView.SetText(lineNumberGutter(r.Content))
		p.copyBtn.Enable()
	case isTextualSourceType(r.Type):
		p.sourceView.SetText(sourcesNotCaptured)
		p.gutterView.SetText("")
		p.copyBtn.Disable()
	default:
		p.sourceView.SetText(binaryResourceSummary(r))
		p.gutterView.SetText("")
		p.copyBtn.Disable()
	}
}

func (p *sourcesPanel) copySelection() {
	app := fyne.CurrentApp()
	if app == nil || p.selectedURL == "" {
		return
	}
	for _, r := range p.visible {
		if r.URL == p.selectedURL && r.HasContent {
			app.Clipboard().SetContent(r.Content)
			return
		}
	}
}

// --- Tree data helpers -------------------------------------------------------

func (p *sourcesPanel) categoryUIDs() []string {
	var uids []string
	for _, c := range sourceCategories() {
		if len(p.resourceUIDsInCategory(c.Type)) > 0 {
			uids = append(uids, treeCategoryUID(c.Type))
		}
	}
	return uids
}

func (p *sourcesPanel) resourceUIDsInCategory(catType string) []string {
	var uids []string
	for i, r := range p.visible {
		if r.Type == catType {
			uids = append(uids, treeResourceUID(i))
		}
	}
	return uids
}

func (p *sourcesPanel) categoryLabel(catType string) string {
	count := len(p.resourceUIDsInCategory(catType))
	for _, c := range sourceCategories() {
		if c.Type == catType {
			return fmt.Sprintf("%s (%d)", c.Title, count)
		}
	}
	return fmt.Sprintf("%s (%d)", catType, count)
}

// --- Formatting helpers ------------------------------------------------------

// classifySourceType buckets a resource by its content type, falling back to
// the URL extension when the server sent no usable content type.
func classifySourceType(contentType, rawURL string) string {
	if ct := formatContentType(contentType); ct != "" {
		return ct
	}
	u, err := url.Parse(rawURL)
	urlPath := rawURL
	if err == nil && u.Path != "" {
		urlPath = u.Path
	}
	switch strings.ToLower(path.Ext(urlPath)) {
	case ".css":
		return "stylesheet"
	case ".js", ".mjs":
		return "script"
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".avif":
		return "image"
	case ".woff", ".woff2", ".ttf", ".otf", ".eot":
		return "font"
	case ".html", ".htm":
		return "document"
	}
	return "other"
}

func isTextualSourceType(t string) bool {
	switch t {
	case "document", "stylesheet", "script":
		return true
	}
	return false
}

// sourceBaseName derives a compact display name from a resource URL.
func sourceBaseName(rawURL string) string {
	if rawURL == "" {
		return "(current page)"
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	base := path.Base(u.Path)
	if base == "" || base == "/" || base == "." {
		if u.Host != "" {
			return u.Host
		}
		return rawURL
	}
	return base
}

// lineNumberGutter renders the 1-based line-number column shown alongside
// the source viewer. A trailing newline does not produce a phantom line.
func lineNumberGutter(content string) string {
	if content == "" {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	var b strings.Builder
	b.Grow(len(lines) * 4)
	for i := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%d", i+1)
	}
	return b.String()
}

// formatResourceDetail builds the monospace header shown above the source
// viewer for the selected resource.
func formatResourceDetail(r sourceResource) string {
	parts := []string{r.Name}
	if r.Method != "" {
		parts = append(parts, r.Method)
	}
	if r.Status != 0 {
		parts = append(parts, strconv.Itoa(r.Status))
	}
	if r.Error != "" {
		parts = append(parts, "error: "+r.Error)
	}
	if r.ContentType != "" {
		parts = append(parts, r.ContentType)
	}
	if r.Bytes > 0 {
		parts = append(parts, formatBytes(r.Bytes))
	}
	if r.Duration > 0 {
		parts = append(parts, formatDuration(r.Duration))
	}
	if r.CacheHit {
		parts = append(parts, "cache hit")
	}
	return strings.Join(parts, "  ·  ")
}

func binaryResourceSummary(r sourceResource) string {
	ct := r.ContentType
	if ct == "" {
		ct = "unknown"
	}
	return fmt.Sprintf("Binary resource — no text preview available.\n\nType: %s\nSize: %s\nURL:  %s",
		ct, formatBytes(r.Bytes), r.URL)
}

// sortedTypes is a stable ordering helper kept for future sortable columns.
func sortedTypes() []string {
	types := make([]string, 0, len(sourceCategories()))
	for _, c := range sourceCategories() {
		types = append(types, c.Type)
	}
	sort.Strings(types)
	return types
}
