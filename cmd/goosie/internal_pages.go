package main

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
)

// internalNavHeader returns standard top navigation for all goosie:// internal pages.
func internalNavHeader(active string) string {
	links := []struct {
		ID   string
		Name string
		URL  string
	}{
		{"version", "About", "goosie://version"},
		{"bookmarks", "Bookmarks", "goosie://bookmarks"},
		{"history", "History", "goosie://history"},
		{"downloads", "Downloads", "goosie://downloads"},
	}

	var b strings.Builder
	b.WriteString(`<div class="header">
  <div class="logo">Goosie Browser</div>
  <div class="nav-links">`)
	for _, l := range links {
		cls := "nav-link"
		if l.ID == active {
			cls += " active"
		}
		b.WriteString(fmt.Sprintf(`<a class="%s" href="%s">%s</a>`, cls, l.URL, l.Name))
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

// internalPageTemplate wraps page body inside standard Goosie styling.
func internalPageTemplate(activeTab, title, bodyHTML string) string {
	nav := internalNavHeader(activeTab)
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>%s</title>
<style>
body {
  font-family: sans-serif;
  background-color: #f8f9fa;
  color: #202124;
  margin: 0;
  padding: 0;
}
.header {
  background-color: #ffffff;
  border-bottom: 1px solid #dadce0;
  padding: 16px 32px;
  margin-bottom: 24px;
}
.logo {
  font-size: 20px;
  font-weight: 700;
  color: #1a73e8;
  margin-bottom: 12px;
}
.nav-links {
  margin-top: 4px;
}
.nav-link {
  display: inline-block;
  color: #5f6368;
  text-decoration: none;
  font-size: 14px;
  font-weight: 500;
  margin-right: 20px;
  padding-bottom: 4px;
}
.nav-link.active {
  color: #1a73e8;
  border-bottom: 2px solid #1a73e8;
  font-weight: 700;
}
.container {
  max-width: 800px;
  margin: 0 auto;
  padding: 0 24px 48px 24px;
}
.card {
  background-color: #ffffff;
  border: 1px solid #dadce0;
  border-radius: 8px;
  padding: 24px;
  margin-bottom: 20px;
}
h1 {
  font-size: 22px;
  font-weight: 600;
  margin-top: 0;
  margin-bottom: 16px;
  color: #202124;
}
h2 {
  font-size: 16px;
  font-weight: 600;
  margin-top: 20px;
  margin-bottom: 12px;
  color: #3c4043;
}
table {
  width: 100%%;
  border-collapse: collapse;
}
td {
  padding: 8px 12px;
  font-size: 14px;
  border-bottom: 1px solid #f1f3f4;
}
td.label {
  color: #5f6368;
  width: 240px;
  font-weight: 500;
}
td.val {
  color: #202124;
  font-family: monospace;
}
.list-item {
  padding: 12px;
  border-bottom: 1px solid #f1f3f4;
}
.list-title {
  font-size: 15px;
  font-weight: 600;
  margin-bottom: 4px;
}
.list-link {
  color: #1a73e8;
  text-decoration: none;
}
.list-meta {
  font-size: 12px;
  color: #5f6368;
}
.empty-state {
  text-align: center;
  padding: 40px;
  color: #5f6368;
  font-size: 15px;
}
.btn-action {
  display: inline-block;
  background-color: #1a73e8;
  color: #ffffff;
  padding: 4px 10px;
  border-radius: 4px;
  text-decoration: none;
  font-size: 12px;
  font-weight: 500;
  margin-right: 6px;
}
.btn-action.secondary {
  background-color: #f1f3f4;
  color: #3c4043;
  border: 1px solid #dadce0;
}
.btn-action.danger {
  background-color: #fce8e6;
  color: #c5221f;
  border: 1px solid #fad2cf;
}
.badge {
  display: inline-block;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
}
.badge-active { background: #e8f0fe; color: #1a73e8; }
.badge-completed { background: #e6f4ea; color: #137333; }
.badge-failed { background: #fce8e6; color: #c5221f; }
.badge-canceled { background: #f1f3f4; color: #5f6368; }
.progress-track {
  background-color: #e8eaed;
  height: 6px;
  border-radius: 3px;
  overflow: hidden;
  margin-top: 8px;
}
.progress-bar {
  background-color: #1a73e8;
  height: 100%%;
}
</style>
</head>
<body>
%s
<div class="container">
%s
</div>
</body>
</html>`, html.EscapeString(title), nav, bodyHTML)
}

// buildVersionPageHTML generates the goosie://version diagnostic report.
func buildVersionPageHTML(f *framePath) (string, string) {
	tabCount := 0
	closedCount := 0
	if f.tabMgr != nil {
		tabCount = f.tabMgr.Count()
		closedCount = f.tabMgr.ClosedTabCount()
	}
	bmCount := 0
	if f.bookmarks != nil {
		bmCount = len(f.bookmarks.List())
	}
	histCount := 0
	if f.history != nil {
		histCount = len(f.history.Entries())
	}

	body := fmt.Sprintf(`
  <div class="card">
    <h1>About Goosie</h1>
    <p style="color: #5f6368; font-size: 14px;">An independent, modern, high-performance web browser written in Go.</p>
    <h2>Environment & Runtime</h2>
    <table>
      <tr><td class="label">Version</td><td class="val">Goosie v0.2.0-dev (Phase M1)</td></tr>
      <tr><td class="label">Go Version</td><td class="val">%s</td></tr>
      <tr><td class="label">Platform / Arch</td><td class="val">%s / %s</td></tr>
      <tr><td class="label">CPU Cores</td><td class="val">%d</td></tr>
    </table>

    <h2>Engine Architecture</h2>
    <table>
      <tr><td class="label">Pipeline</td><td class="val">Parse → Style → Layout → Paint</td></tr>
      <tr><td class="label">Frame Path</td><td class="val">256px Tile Rasterizer, Damage-Blit Composer, Vsync UI Loop</td></tr>
      <tr><td class="label">Layout Arena</td><td class="val">Block, Inline, Float, Flex, Grid, Table, Positioning</td></tr>
      <tr><td class="label">JavaScript Engine</td><td class="val">Per-document Goja Runtime</td></tr>
    </table>

    <h2>Resource Caps (Modern Standard)</h2>
    <table>
      <tr><td class="label">MaxDocumentBytes</td><td class="val">%d MiB (%d bytes)</td></tr>
      <tr><td class="label">MaxDocumentNodes</td><td class="val">%s</td></tr>
      <tr><td class="label">MaxLayoutObjects</td><td class="val">%s</td></tr>
      <tr><td class="label">MaxDocumentTiles</td><td class="val">%s tiles</td></tr>
    </table>

    <h2>Active Session Metrics</h2>
    <table>
      <tr><td class="label">Open Tabs</td><td class="val">%d</td></tr>
      <tr><td class="label">Reopen Stack</td><td class="val">%d closed tab(s)</td></tr>
      <tr><td class="label">Saved Bookmarks</td><td class="val">%d</td></tr>
      <tr><td class="label">History Records</td><td class="val">%d visits</td></tr>
    </table>
  </div>
`, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(),
		engine.MaxDocumentBytes/(1024*1024), engine.MaxDocumentBytes,
		"1,000,000", "4,194,304", "524,288",
		tabCount, closedCount, bmCount, histCount)

	return "About Goosie", internalPageTemplate("version", "About Goosie", body)
}

// buildBookmarksPageHTML generates the goosie://bookmarks management page.
func buildBookmarksPageHTML(f *framePath) (string, string) {
	var b strings.Builder
	b.WriteString(`<div class="card"><h1>Bookmarks</h1>`)

	if f.bookmarks == nil || len(f.bookmarks.List()) == 0 {
		b.WriteString(`<div class="empty-state">No bookmarks saved yet.<br><small style="margin-top: 8px; display:inline-block;">Tip: Press <b>Cmd+D</b> while browsing to star your favorite pages.</small></div>`)
	} else {
		for _, bm := range f.bookmarks.List() {
			title := bm.Title
			if title == "" {
				title = bm.URL
			}
			dateStr := bm.Added.Format("Jan 02, 2006 15:04")
			b.WriteString(fmt.Sprintf(`
      <div class="list-item">
        <div class="list-title"><a class="list-link" href="%s">%s</a></div>
        <div class="list-meta">★ Starred on %s &bull; <span style="font-family: monospace;">%s</span></div>
      </div>`, html.EscapeString(bm.URL), html.EscapeString(title), dateStr, html.EscapeString(bm.URL)))
		}
	}
	b.WriteString(`</div>`)
	return "Bookmarks", internalPageTemplate("bookmarks", "Bookmarks", b.String())
}

// buildHistoryPageHTML generates the goosie://history chronological visit page.
func buildHistoryPageHTML(f *framePath) (string, string) {
	var b strings.Builder
	b.WriteString(`<div class="card"><h1>Browsing History</h1>`)

	if f.history == nil || len(f.history.Entries()) == 0 {
		b.WriteString(`<div class="empty-state">Your browsing history is currently empty.</div>`)
	} else {
		entries := f.history.Entries()
		// Show newest first
		for i := len(entries) - 1; i >= 0; i-- {
			h := entries[i]
			title := h.Title
			if title == "" {
				title = h.URL
			}
			dateStr := h.At.Format("Jan 02, 2006 15:04:05")
			b.WriteString(fmt.Sprintf(`
      <div class="list-item">
        <div class="list-title"><a class="list-link" href="%s">%s</a></div>
        <div class="list-meta">Visited at %s &bull; <span style="font-family: monospace;">%s</span></div>
      </div>`, html.EscapeString(h.URL), html.EscapeString(title), dateStr, html.EscapeString(h.URL)))
		}
	}
	b.WriteString(`</div>`)
	return "Browsing History", internalPageTemplate("history", "Browsing History", b.String())
}

// buildDownloadsPageHTML generates the goosie://downloads viewer page.
func buildDownloadsPageHTML(f *framePath, rawURL string) (string, string) {
	if u, err := url.Parse(rawURL); err == nil {
		action := u.Query().Get("action")
		switch action {
		case "reveal":
			p := u.Query().Get("path")
			if p != "" {
				_ = download.ShowInFolder(p)
			}
		case "open":
			p := u.Query().Get("path")
			if p != "" {
				_ = download.OpenFile(p)
			}
		case "clear":
			if f.downloads != nil {
				f.downloads.ClearCompleted()
			}
		case "cancel":
			id := u.Query().Get("id")
			if f.downloads != nil && id != "" {
				f.downloads.Cancel(id)
			}
		}
	}

	var b strings.Builder
	b.WriteString(`<div class="card">
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
      <h1 style="margin: 0;">Downloads</h1>
      <div>
        <a class="btn-action secondary" href="goosie://downloads?action=clear">Clear Finished</a>
      </div>
    </div>`)

	dir := f.config.downloadDir
	if dir == "" {
		dir = "~/Downloads"
	}
	b.WriteString(fmt.Sprintf(`<p style="color: #5f6368; font-size: 13px; margin-bottom: 18px;">Save directory: <code style="background: #f1f3f4; padding: 2px 6px; border-radius: 3px;">%s</code></p>`, html.EscapeString(dir)))

	renderedFiles := make(map[string]bool)
	hasItems := false

	// 1. Render items from download manager (active and recent)
	if f.downloads != nil {
		items := f.downloads.Items()
		for i := len(items) - 1; i >= 0; i-- {
			it := items[i]
			hasItems = true
			if it.LocalPath != "" {
				renderedFiles[it.LocalPath] = true
			}

			badgeClass := "badge-completed"
			switch it.Status {
			case download.StatusActive:
				badgeClass = "badge-active"
			case download.StatusFailed:
				badgeClass = "badge-failed"
			case download.StatusCanceled:
				badgeClass = "badge-canceled"
			}

			b.WriteString(`<div class="list-item">`)
			b.WriteString(fmt.Sprintf(`<div style="display: flex; justify-content: space-between; align-items: flex-start;">
        <div>
          <span class="list-title">%s</span>
          <span class="badge %s" style="margin-left: 8px;">%s</span>
        </div>`, html.EscapeString(it.Filename), badgeClass, html.EscapeString(string(it.Status))))

			// Action buttons on the right
			b.WriteString(`<div>`)
			if it.Status == download.StatusActive {
				b.WriteString(fmt.Sprintf(`<a class="btn-action danger" href="goosie://downloads?action=cancel&id=%s">Cancel</a>`, html.EscapeString(it.ID)))
			} else if it.Status == download.StatusCompleted && it.LocalPath != "" {
				b.WriteString(fmt.Sprintf(`<a class="btn-action" href="goosie://downloads?action=open&path=%s">Open</a>`, url.QueryEscape(it.LocalPath)))
				b.WriteString(fmt.Sprintf(`<a class="btn-action secondary" href="goosie://downloads?action=reveal&path=%s">Show in Finder</a>`, url.QueryEscape(it.LocalPath)))
			}
			b.WriteString(`</div></div>`)

			// Meta info
			meta := it.FormattedSize()
			if it.Status == download.StatusActive {
				if it.Speed() > 0 {
					meta += fmt.Sprintf(" &bull; %s", it.FormattedSpeed())
				}
				if pct := it.Percent(); pct >= 0 {
					meta += fmt.Sprintf(" &bull; %.0f%%", pct)
				}
			} else if !it.EndTime.IsZero() {
				meta += fmt.Sprintf(" &bull; %s", it.EndTime.Format("Jan 02, 2006 15:04"))
			}
			if it.Error != "" {
				meta += fmt.Sprintf(" &bull; <span style='color: #c5221f;'>%s</span>", html.EscapeString(it.Error))
			}
			b.WriteString(fmt.Sprintf(`<div class="list-meta">%s</div>`, meta))

			// Progress bar for active downloads
			if it.Status == download.StatusActive {
				pct := it.Percent()
				if pct < 0 {
					pct = 100 // indeterminate
				}
				b.WriteString(fmt.Sprintf(`<div class="progress-track"><div class="progress-bar" style="width: %.1f%%;"></div></div>`, pct))
			}
			b.WriteString(`</div>`)
		}
	}

	// 2. Discover preexisting files on disk in the download directory
	files, _ := os.ReadDir(dir)
	for _, fi := range files {
		if fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue
		}
		fullPath := filepath.Join(dir, fi.Name())
		if renderedFiles[fullPath] {
			continue
		}
		info, err := fi.Info()
		if err != nil {
			continue
		}
		hasItems = true
		sizeStr := formatFileSize(info.Size())
		modStr := info.ModTime().Format("Jan 02, 2006 15:04")
		b.WriteString(fmt.Sprintf(`
      <div class="list-item">
        <div style="display: flex; justify-content: space-between; align-items: flex-start;">
          <div>
            <span class="list-title">%s</span>
            <span class="badge badge-completed" style="margin-left: 8px;">Saved</span>
          </div>
          <div>
            <a class="btn-action" href="goosie://downloads?action=open&path=%s">Open</a>
            <a class="btn-action secondary" href="goosie://downloads?action=reveal&path=%s">Show in Finder</a>
          </div>
        </div>
        <div class="list-meta">%s &bull; Modified %s &bull; <span style="font-family: monospace;">%s</span></div>
      </div>`, html.EscapeString(fi.Name()), url.QueryEscape(fullPath), url.QueryEscape(fullPath), sizeStr, modStr, html.EscapeString(fullPath)))
	}

	if !hasItems {
		b.WriteString(`<div class="empty-state">No downloaded files found.</div>`)
	}
	b.WriteString(`</div>`)
	return "Downloads", internalPageTemplate("downloads", "Downloads", b.String())
}

func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// buildInternalPageHTML routes goosie:// paths to their respective HTML generators.
func (f *framePath) buildInternalPageHTML(rawURL string) (title, htmlContent string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "Error", internalPageTemplate("", "Error", `<div class="card"><h1>Invalid URL</h1><p>Malformed goosie:// URL.</p></div>`)
	}

	host := strings.ToLower(u.Host)
	path := strings.Trim(strings.ToLower(u.Path), "/")
	page := host
	if page == "" {
		page = path
	}

	switch page {
	case "version", "about":
		return buildVersionPageHTML(f)
	case "bookmarks":
		return buildBookmarksPageHTML(f)
	case "history":
		return buildHistoryPageHTML(f)
	case "downloads":
		return buildDownloadsPageHTML(f, rawURL)
	default:
		// Index page listing available internal pages
		body := `<div class="card">
      <h1>Goosie Internal Pages</h1>
      <p style="color: #5f6368;">Available system and management pages:</p>
      <ul style="line-height: 2; font-size: 15px;">
        <li><a class="list-link" href="goosie://version">goosie://version</a> &mdash; About and engine diagnostics</li>
        <li><a class="list-link" href="goosie://bookmarks">goosie://bookmarks</a> &mdash; Manage saved bookmarks</li>
        <li><a class="list-link" href="goosie://history">goosie://history</a> &mdash; Browsing history</li>
        <li><a class="list-link" href="goosie://downloads">goosie://downloads</a> &mdash; Downloaded files</li>
      </ul>
    </div>`
		return "Goosie Pages", internalPageTemplate("", "Goosie Pages", body)
	}
}

// renderInternalPage compiles a goosie:// internal page into a frame Layer and engine Session.
func (f *framePath) renderInternalPage(ctx context.Context, rawURL string, viewportW, viewportH int, scale float32) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, string, error) {
	if err := engine.ValidateViewport(viewportW, viewportH, float64(scale)); err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: viewport: %w", err)
	}
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", ctx.Err()
	}

	title, htmlStr := f.buildInternalPageHTML(rawURL)

	var opts []engine.Option
	if f.fonts != nil {
		opts = append(opts, engine.WithMetrics(f.fonts))
	}
	opts = append(opts, engine.WithViewportH(float32(viewportH)))

	sess, err := engine.NewSession(htmlStr, nil, float32(viewportW), opts...)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: build internal session: %w", err)
	}

	list, err := sess.PaintChecked(scale)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: paint internal page: %w", err)
	}

	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: size internal cache: %w", err)
	}

	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)

	_ = title // title is tracked on Tab
	return layer, paint.SceneSpec{DocHeight: extent.H()}, sess.BackgroundColor(), sess, rawURL, nil
}
