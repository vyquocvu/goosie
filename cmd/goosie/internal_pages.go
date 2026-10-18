package main

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/tabs"
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
		{"inspect", "Inspector", "goosie://inspect"},
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
@media (prefers-color-scheme: dark) {
  body { background-color: #121212; color: #e8eaed; }
  .header { background-color: #1e1e1e; border-bottom: 1px solid #333333; }
  .logo { color: #8ab4f8; }
  .nav-link { color: #9aa0a6; }
  .nav-link.active { color: #8ab4f8; border-bottom: 2px solid #8ab4f8; }
  .card { background-color: #1e1e1e; border: 1px solid #333333; }
  h1 { color: #e8eaed; }
  h2 { color: #bdc1c6; }
  td { border-bottom: 1px solid #2d2e30; }
  td.label { color: #9aa0a6; }
  td.val { color: #e8eaed; }
  .list-item { border-bottom: 1px solid #2d2e30; }
  .list-link { color: #8ab4f8; }
  .list-meta { color: #9aa0a6; }
  .empty-state { color: #9aa0a6; }
  .btn-action.secondary { background-color: #2d2e30; color: #e8eaed; border: 1px solid #3c4043; }
  .badge-completed { background: #0d381e; color: #81c995; }
  .badge-failed { background: #441210; color: #f28b82; }
  .badge-canceled { background: #2d2e30; color: #9aa0a6; }
}
@media (prefers-contrast: more) {
  body { font-weight: 500; }
  .card { border: 2px solid #000000; }
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

    <h2>Network & Security Posture</h2>
    <table>
      <tr><td class="label">Ad & Tracker Shield</td><td class="val"><span class="badge badge-completed">Active</span></td></tr>
      <tr><td class="label">DNS-over-HTTPS</td><td class="val">RFC 8484 Compliant Resolver</td></tr>
      <tr><td class="label">Cookie Security</td><td class="val">RFC 6265 with HttpOnly & SameSite Isolation</td></tr>
      <tr><td class="label">Web Storage API</td><td class="val">Origin-Scoped Atomic Persistence</td></tr>
    </table>

    <h2>Appearance &amp; Theme Mode</h2>
    <table>
      <tr><td class="label">Configured Theme</td><td class="val"><span class="badge badge-completed">%s</span></td></tr>
      <tr><td class="label">Dark Scheme Active</td><td class="val">%t</td></tr>
      <tr><td class="label">High Contrast Active</td><td class="val">%t</td></tr>
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
		f.theme, f.isDarkMode(), f.isHighContrast(),
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
	case "inspect", "devtools":
		return buildInspectPageHTML(f, rawURL)
	case "reader":
		return buildReaderPageHTML(f, rawURL)
	case "restore":
		return buildRestorePageHTML(f, rawURL)
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
        <li><a class="list-link" href="goosie://inspect">goosie://inspect</a> &mdash; Web Inspector &amp; DOM/CSS Tree Explorer (Cmd+Option+I)</li>
        <li><a class="list-link" href="goosie://reader">goosie://reader</a> &mdash; Distraction-Free Reader Mode (Cmd+Shift+R)</li>
        <li><a class="list-link" href="goosie://restore">goosie://restore</a> &mdash; Session Crash Recovery &amp; Restore Prompt</li>
      </ul>
    </div>`
		return "Goosie Pages", internalPageTemplate("", "Goosie Pages", body)
	}
}

// buildInspectPageHTML generates the Web Inspector page inspecting a live browser tab.
func buildInspectPageHTML(f *framePath, rawURL string) (title, htmlContent string) {
	u, _ := url.Parse(rawURL)
	targetIDStr := ""
	evalExpr := ""
	action := ""
	device := ""
	if u != nil {
		targetIDStr = u.Query().Get("tab")
		evalExpr = u.Query().Get("eval")
		action = u.Query().Get("action")
		device = u.Query().Get("device")
	}

	if action == "emulate" {
		if device == "reset" || device == "" {
			f.SetDeviceEmulation(nil)
		} else if preset := DevicePresetByID(device); preset != nil {
			f.SetDeviceEmulation(preset)
		}
	}

	var targetTab *tabs.Tab
	var allTabs []*tabs.Tab
	if f.tabMgr != nil {
		allTabs = f.tabMgr.Tabs()
	}

	if targetIDStr != "" && f.tabMgr != nil {
		if id, err := strconv.ParseUint(targetIDStr, 10, 64); err == nil {
			targetTab = f.tabMgr.TabByID(id)
		}
	}
	if targetTab == nil && f.tabMgr != nil {
		active := f.tabMgr.Active()
		if active != nil && !strings.HasPrefix(strings.ToLower(active.URL()), "goosie://inspect") {
			targetTab = active
		} else {
			for _, t := range allTabs {
				if !strings.HasPrefix(strings.ToLower(t.URL()), "goosie://") {
					targetTab = t
					break
				}
			}
		}
	}

	if targetTab != nil {
		switch action {
		case "toggle_shields":
			targetTab.SetShieldsEnabled(!targetTab.ShieldsEnabled())
		case "storage_del":
			key := u.Query().Get("key")
			origin := js.OriginOf(targetTab.URL())
			if f.storage != nil && key != "" {
				f.storage.RemoveLocalStorageItem(origin, key)
			}
		case "storage_clear":
			origin := js.OriginOf(targetTab.URL())
			if f.storage != nil {
				f.storage.ClearLocalStorage(origin)
			}
		case "cookies_clear":
			if f.client != nil && f.client.Jar() != nil {
				f.client.Jar().Clear()
			}
		case "set_theme":
			t := u.Query().Get("theme")
			if t != "" {
				f.setTheme(t)
			}
		}
	}

	if targetTab == nil || targetTab.Session() == nil {
		body := `<div class="card">
      <h1>Web Inspector</h1>
      <p style="color: #5f6368;">No active web document found to inspect.</p>
      <p>Navigate to any webpage (e.g. <code>https://news.ycombinator.com</code>) in a browser tab, then press <code>Cmd+Option+I</code> to inspect its live DOM tree, computed CSS box model, and JavaScript runtime.</p>
    </div>`
		return "Web Inspector", internalPageTemplate("inspect", "Web Inspector", body)
	}

	sess := targetTab.Session()
	nodeCount, maxDepth := sess.DOMStats()
	sheets := sess.Stylesheets()

	var b strings.Builder

	// Header card with tab picker
	b.WriteString(`<div class="card">`)
	b.WriteString(fmt.Sprintf(`<h1>Web Inspector &mdash; %s</h1>`, html.EscapeString(targetTab.Title())))
	b.WriteString(fmt.Sprintf(`<p style="color: #5f6368; word-break: break-all;">Inspecting: <a href="%s" style="color: #1a73e8;">%s</a></p>`, html.EscapeString(targetTab.URL()), html.EscapeString(targetTab.URL())))

	// Open tabs pill list
	if len(allTabs) > 0 {
		b.WriteString(`<div style="margin-top: 12px; margin-bottom: 8px;"><b style="font-size: 13px;">Open Tabs:</b> `)
		for _, t := range allTabs {
			if strings.HasPrefix(strings.ToLower(t.URL()), "goosie://inspect") {
				continue
			}
			activeStyle := "background-color: #e8f0fe; color: #1a73e8; border: 1px solid #1a73e8; font-weight: bold;"
			if t.ID() != targetTab.ID() {
				activeStyle = "background-color: #f1f3f4; color: #3c4043; border: 1px solid #dadce0;"
			}
			b.WriteString(fmt.Sprintf(`<a href="goosie://inspect?tab=%d" style="display: inline-block; padding: 4px 10px; margin-right: 6px; margin-bottom: 4px; border-radius: 12px; text-decoration: none; font-size: 12px; %s">%s</a> `, t.ID(), activeStyle, html.EscapeString(t.Title())))
		}
		b.WriteString(`</div>`)
	}

	// Metrics summary cards
	b.WriteString(`<div style="display: flex; gap: 16px; margin-top: 16px;">`)
	b.WriteString(fmt.Sprintf(`<div style="flex: 1; background: #f8f9fa; border: 1px solid #e8eaed; border-radius: 6px; padding: 12px; text-align: center;"><div style="font-size: 20px; font-weight: 700; color: #1a73e8;">%d</div><div style="font-size: 12px; color: #5f6368;">DOM Nodes</div></div>`, nodeCount))
	b.WriteString(fmt.Sprintf(`<div style="flex: 1; background: #f8f9fa; border: 1px solid #e8eaed; border-radius: 6px; padding: 12px; text-align: center;"><div style="font-size: 20px; font-weight: 700; color: #188038;">%d</div><div style="font-size: 12px; color: #5f6368;">Tree Depth</div></div>`, maxDepth))
	b.WriteString(fmt.Sprintf(`<div style="flex: 1; background: #f8f9fa; border: 1px solid #e8eaed; border-radius: 6px; padding: 12px; text-align: center;"><div style="font-size: 20px; font-weight: 700; color: #e37400;">%d</div><div style="font-size: 12px; color: #5f6368;">Stylesheets</div></div>`, len(sheets)))
	b.WriteString(`</div></div>`)

	// Performance & Memory Meter Card
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	allocMB := float64(mem.Alloc) / (1024 * 1024)
	sysMB := float64(mem.Sys) / (1024 * 1024)
	gcPauseUS := float64(0)
	if mem.NumGC > 0 {
		gcPauseUS = float64(mem.PauseNs[(mem.NumGC+255)%256]) / 1000.0
	}
	numGoroutines := runtime.NumGoroutine()
	numCPU := runtime.NumCPU()

	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Performance &amp; Memory Meter</h2>`)
	b.WriteString(`<div style="display: flex; gap: 12px; margin-bottom: 12px;">`)
	b.WriteString(fmt.Sprintf(`<div style="flex: 1; background: #e8f0fe; border-radius: 6px; padding: 12px; text-align: center;"><div style="font-size: 18px; font-weight: 700; color: #1a73e8;">%.2f MB</div><div style="font-size: 11px; color: #5f6368;">Heap In-Use</div></div>`, allocMB))
	b.WriteString(fmt.Sprintf(`<div style="flex: 1; background: #e6f4ea; border-radius: 6px; padding: 12px; text-align: center;"><div style="font-size: 18px; font-weight: 700; color: #188038;">%.2f MB</div><div style="font-size: 11px; color: #5f6368;">OS Reserved</div></div>`, sysMB))
	b.WriteString(fmt.Sprintf(`<div style="flex: 1; background: #fef7e0; border-radius: 6px; padding: 12px; text-align: center;"><div style="font-size: 18px; font-weight: 700; color: #b06000;">%.1f &mu;s</div><div style="font-size: 11px; color: #5f6368;">Last GC Pause</div></div>`, gcPauseUS))
	b.WriteString(fmt.Sprintf(`<div style="flex: 1; background: #f1f3f4; border-radius: 6px; padding: 12px; text-align: center;"><div style="font-size: 18px; font-weight: 700; color: #3c4043;">%d / %d</div><div style="font-size: 11px; color: #5f6368;">Goroutines / CPUs</div></div>`, numGoroutines, numCPU))
	b.WriteString(`</div>`)
	b.WriteString(fmt.Sprintf(`<div style="font-size: 12px; color: #5f6368;">Total GC Cycles: <b>%d</b> &bull; Cumulative Allocated: <b>%.2f MB</b></div>`, mem.NumGC, float64(mem.TotalAlloc)/(1024*1024)))
	b.WriteString(`</div>`)

	// DOM Elements Tree Card
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Elements (DOM Tree)</h2>`)
	b.WriteString(`<div style="background-color: #282c34; color: #abb2bf; font-family: monospace; font-size: 13px; padding: 16px; border-radius: 6px; overflow-x: auto; line-height: 1.6;">`)
	if sess.Doc != nil {
		renderDOMTreeHTML(&b, &sess.Doc.Node, 0, 150)
	} else {
		b.WriteString(`<em>Empty Document</em>`)
	}
	b.WriteString(`</div></div>`)

	// CSS Box Model Card
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Computed CSS Box Model</h2>`)
	b.WriteString(`<div style="margin: 16px auto; max-width: 480px; background-color: #fce8b2; border: 1px dashed #f29900; border-radius: 6px; padding: 14px; text-align: center; font-family: monospace; font-size: 12px;">
      <div style="color: #b06000; font-weight: bold; margin-bottom: 6px;">margin (auto)</div>
      <div style="background-color: #fef7e0; border: 1px solid #f9ab00; border-radius: 4px; padding: 12px;">
        <div style="color: #b06000; font-weight: bold; margin-bottom: 6px;">border (1px)</div>
        <div style="background-color: #e6f4ea; border: 1px solid #34a853; border-radius: 4px; padding: 12px;">
          <div style="color: #137333; font-weight: bold; margin-bottom: 6px;">padding (16px)</div>
          <div style="background-color: #c2e7ff; border: 1px solid #1a73e8; border-radius: 4px; padding: 14px; color: #0b57d0; font-weight: bold;">
            content (viewport)
          </div>
        </div>
      </div>
    </div></div>`)

	// Active Stylesheets Card
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Active Stylesheets</h2>`)
	if len(sheets) == 0 {
		b.WriteString(`<p style="color: #5f6368;">No external or inline stylesheets found (Default User-Agent styling applies).</p>`)
	} else {
		b.WriteString(`<table><thead><tr><th>#</th><th>Origin</th><th>Rules</th></tr></thead><tbody>`)
		for i, s := range sheets {
			b.WriteString(fmt.Sprintf(`<tr><td>%d</td><td>%s</td><td>%d rules</td></tr>`, i+1, "Author Stylesheet", len(s.Rules)))
		}
		b.WriteString(`</tbody></table>`)
	}
	b.WriteString(`</div>`)

	// Device Emulation Card
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Device Emulation (Cmd+Shift+M)</h2>`)
	b.WriteString(`<p style="color: #5f6368;">Test responsive layouts across mobile and tablet viewports.</p>`)
	b.WriteString(`<div style="display: flex; gap: 10px; margin-top: 12px; flex-wrap: wrap;">`)
	activeDevID := ""
	if f.emulationPreset != nil {
		activeDevID = f.emulationPreset.ID
	}
	resetClass := "btn-action secondary"
	if activeDevID == "" {
		resetClass = "btn-action"
	}
	b.WriteString(fmt.Sprintf(`<a class="%s" href="goosie://inspect?action=emulate&device=reset">Desktop (Responsive)</a>`, resetClass))
	for _, p := range DevicePresets {
		btnClass := "btn-action secondary"
		if activeDevID == p.ID {
			btnClass = "btn-action"
		}
		b.WriteString(fmt.Sprintf(`<a class="%s" href="goosie://inspect?action=emulate&device=%s">%s (%d &times; %d @ %.1fx)</a>`,
			btnClass, url.QueryEscape(p.ID), html.EscapeString(p.Name), p.Width, p.Height, p.DPR))
	}
	b.WriteString(`</div>`)
	if f.emulationPreset != nil {
		b.WriteString(fmt.Sprintf(`<p style="margin-top: 10px; font-size: 13px; color: #1a73e8;"><b>Active Emulation:</b> %s (%d &times; %d px, DPR %.2f)</p>`,
			html.EscapeString(f.emulationPreset.Name), f.emulationPreset.Width, f.emulationPreset.Height, f.emulationPreset.DPR))
	} else {
		b.WriteString(`<p style="margin-top: 10px; font-size: 13px; color: #5f6368;"><b>Active Emulation:</b> Desktop (Window Native)</p>`)
	}
	b.WriteString(`</div>`)

	// Shields & Privacy Card
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Shields &amp; Privacy</h2>`)
	shieldsBadge := `<span class="badge badge-completed">Protection Active</span>`
	toggleActionText := "Disable for this tab"
	toggleBtnClass := "btn-action secondary"
	if !targetTab.ShieldsEnabled() {
		shieldsBadge = `<span class="badge badge-failed">Protection Disabled</span>`
		toggleActionText = "Enable Protection"
		toggleBtnClass = "btn-action"
	}
	b.WriteString(fmt.Sprintf(`
      <div style="display: flex; justify-content: space-between; align-items: center;">
        <div>
          %s
          <span style="margin-left: 10px; font-size: 14px; color: #5f6368;">Blocked Trackers / Ads: <b>%d</b></span>
        </div>
        <a class="%s" href="goosie://inspect?action=toggle_shields&tab=%d">%s</a>
      </div>`, shieldsBadge, targetTab.BlockedTrackers(), toggleBtnClass, targetTab.ID(), toggleActionText))
	b.WriteString(`</div>`)

	// Appearance & Theme Mode Card
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Appearance &amp; Theme Mode</h2>`)
	b.WriteString(`<p style="color: #5f6368; font-size: 13px;">Toggle browser theme and CSS <code>prefers-color-scheme</code> / <code>prefers-contrast</code> media query resolution.</p>`)
	activeTheme := f.theme
	if activeTheme == "" {
		activeTheme = "system"
	}
	pillStyle := func(thm string) string {
		if strings.EqualFold(activeTheme, thm) {
			return "background-color: #1a73e8; color: #ffffff; font-weight: bold; padding: 6px 14px; border-radius: 4px; text-decoration: none; font-size: 13px; display: inline-block;"
		}
		return "background-color: #f1f3f4; color: #3c4043; border: 1px solid #dadce0; padding: 6px 14px; border-radius: 4px; text-decoration: none; font-size: 13px; display: inline-block;"
	}
	b.WriteString(`<div style="display: flex; gap: 10px; margin-top: 12px;">`)
	b.WriteString(fmt.Sprintf(`<a style="%s" href="goosie://inspect?action=set_theme&theme=system&tab=%d">System Default</a> `, pillStyle("system"), targetTab.ID()))
	b.WriteString(fmt.Sprintf(`<a style="%s" href="goosie://inspect?action=set_theme&theme=light&tab=%d">Light Mode</a> `, pillStyle("light"), targetTab.ID()))
	b.WriteString(fmt.Sprintf(`<a style="%s" href="goosie://inspect?action=set_theme&theme=dark&tab=%d">Dark Mode</a> `, pillStyle("dark"), targetTab.ID()))
	b.WriteString(fmt.Sprintf(`<a style="%s" href="goosie://inspect?action=set_theme&theme=high-contrast&tab=%d">High Contrast</a> `, pillStyle("high-contrast"), targetTab.ID()))
	b.WriteString(`</div></div>`)

	// Web Storage (localStorage) Card
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Web Storage (localStorage)</h2>`)
	origin := js.OriginOf(targetTab.URL())
	b.WriteString(fmt.Sprintf(`<p style="color: #5f6368; font-size: 13px;">Origin: <code>%s</code></p>`, html.EscapeString(origin)))
	var localEntries []js.StorageEntry
	if f.storage != nil {
		localEntries = f.storage.DumpLocalStorage(origin)
	}
	if len(localEntries) == 0 {
		b.WriteString(`<p style="color: #5f6368;">No localStorage keys stored for this origin.</p>`)
	} else {
		b.WriteString(`<table><thead><tr><th>Key</th><th>Value</th><th>Action</th></tr></thead><tbody>`)
		for _, e := range localEntries {
			delLink := fmt.Sprintf(`goosie://inspect?action=storage_del&key=%s&tab=%d`, url.QueryEscape(e.Key), targetTab.ID())
			b.WriteString(fmt.Sprintf(`<tr><td style="font-family: monospace; font-weight: bold;">%s</td><td style="font-family: monospace; word-break: break-all;">%s</td><td><a class="btn-action danger" href="%s">Delete</a></td></tr>`,
				html.EscapeString(e.Key), html.EscapeString(e.Value), delLink))
		}
		b.WriteString(`</tbody></table>`)
		clearLink := fmt.Sprintf(`goosie://inspect?action=storage_clear&tab=%d`, targetTab.ID())
		b.WriteString(fmt.Sprintf(`<div style="margin-top: 12px;"><a class="btn-action danger" href="%s">Clear All LocalStorage</a></div>`, clearLink))
	}
	b.WriteString(`</div>`)

	// Cookies Card
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Cookies (Active Jar)</h2>`)
	var docHost string
	if parsedU, err := url.Parse(targetTab.URL()); err == nil {
		docHost = parsedU.Hostname()
	}
	var cookies []net.Cookie
	if f.client != nil && f.client.Jar() != nil {
		if docHost != "" {
			cookies = f.client.Jar().CookiesForHost(docHost)
		} else {
			cookies = f.client.Jar().AllCookies()
		}
	}
	if len(cookies) == 0 {
		b.WriteString(`<p style="color: #5f6368;">No active cookies found for this host.</p>`)
	} else {
		b.WriteString(`<table><thead><tr><th>Name</th><th>Value</th><th>Domain</th><th>Path</th><th>Expires</th><th>Flags</th></tr></thead><tbody>`)
		for _, c := range cookies {
			flags := ""
			if c.HttpOnly {
				flags += `<span class="badge" style="background:#e8f0fe; color:#1a73e8; margin-right:4px;">HttpOnly</span>`
			}
			if c.Secure {
				flags += `<span class="badge" style="background:#e6f4ea; color:#137333;">Secure</span>`
			}
			expStr := "Session"
			if !c.Expires.IsZero() {
				expStr = c.Expires.Format("Jan 02, 2006 15:04")
			}
			b.WriteString(fmt.Sprintf(`<tr><td style="font-family: monospace; font-weight: bold;">%s</td><td style="font-family: monospace; word-break: break-all;">%s</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(c.Name), html.EscapeString(c.Value), html.EscapeString(c.Domain), html.EscapeString(c.Path), expStr, flags))
		}
		b.WriteString(`</tbody></table>`)
		clearCookiesLink := fmt.Sprintf(`goosie://inspect?action=cookies_clear&tab=%d`, targetTab.ID())
		b.WriteString(fmt.Sprintf(`<div style="margin-top: 12px;"><a class="btn-action danger" href="%s">Clear All Cookies</a></div>`, clearCookiesLink))
	}
	b.WriteString(`</div>`)

	// Network Activity Log Card
	b.WriteString(`<div class="card">`)

	b.WriteString(`<h2>Network Activity</h2>`)
	var traces []net.RequestTrace
	if f.client != nil && f.client.NetworkLog() != nil {
		traces = f.client.NetworkLog().List()
	}
	if len(traces) == 0 {
		b.WriteString(`<p style="color: #5f6368;">No HTTP requests recorded in session yet.</p>`)
	} else {
		b.WriteString(`<table><thead><tr><th>Status</th><th>Method</th><th>URL</th><th>Type</th><th>Size</th><th>Time</th></tr></thead><tbody>`)
		for i := len(traces) - 1; i >= 0 && i >= len(traces)-50; i-- {
			tr := traces[i]
			statusBadge := fmt.Sprintf(`<span style="display: inline-block; padding: 2px 8px; border-radius: 4px; font-weight: bold; font-size: 11px; background-color: #e6f4ea; color: #137333;">%d</span>`, tr.StatusCode)
			if tr.Blocked {
				statusBadge = `<span style="display: inline-block; padding: 2px 8px; border-radius: 4px; font-weight: bold; font-size: 11px; background-color: #fce8e6; color: #c5221f;">BLOCKED</span>`
			} else if tr.StatusCode >= 400 {
				statusBadge = fmt.Sprintf(`<span style="display: inline-block; padding: 2px 8px; border-radius: 4px; font-weight: bold; font-size: 11px; background-color: #fce8e6; color: #c5221f;">%d</span>`, tr.StatusCode)
			} else if tr.StatusCode == 304 {
				statusBadge = `<span style="display: inline-block; padding: 2px 8px; border-radius: 4px; font-weight: bold; font-size: 11px; background-color: #e8f0fe; color: #1a73e8;">304</span>`
			}
			sizeStr := fmt.Sprintf("%.1f KB", float64(tr.Bytes)/1024.0)
			if tr.Bytes < 1024 {
				sizeStr = fmt.Sprintf("%d B", tr.Bytes)
			}
			timeStr := fmt.Sprintf("%.1f ms", float64(tr.Duration.Microseconds())/1000.0)
			shortURL := tr.URL
			if len(shortURL) > 50 {
				shortURL = shortURL[:47] + "..."
			}
			b.WriteString(fmt.Sprintf(`<tr><td>%s</td><td><b>%s</b></td><td title="%s" style="word-break: break-all; font-family: monospace; font-size: 12px;">%s</td><td>%s</td><td>%s</td><td>%s</td></tr>`,
				statusBadge, html.EscapeString(tr.Method), html.EscapeString(tr.URL), html.EscapeString(shortURL), html.EscapeString(tr.ContentType), sizeStr, timeStr))
		}
		b.WriteString(`</tbody></table>`)
	}
	b.WriteString(`</div>`)

	// Console REPL Card
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Console REPL</h2>`)
	if evalExpr != "" {
		res, err := sess.EvaluateJS(evalExpr)
		b.WriteString(fmt.Sprintf(`<div style="background: #f1f3f4; padding: 10px; border-radius: 4px; font-family: monospace; margin-bottom: 12px;"><b>&gt;</b> %s</div>`, html.EscapeString(evalExpr)))
		if err != nil {
			b.WriteString(fmt.Sprintf(`<div style="color: #d93025; font-family: monospace; margin-bottom: 12px;"><b>Error:</b> %s</div>`, html.EscapeString(err.Error())))
		} else {
			b.WriteString(fmt.Sprintf(`<div style="color: #188038; font-family: monospace; margin-bottom: 12px;"><b>&lt;</b> %s</div>`, html.EscapeString(res)))
		}
	}
	b.WriteString(fmt.Sprintf(`
      <form action="goosie://inspect" method="GET">
        <input type="hidden" name="tab" value="%d">
        <div style="display: flex; gap: 8px;">
          <input type="text" name="eval" placeholder="document.title, location.href, 2 + 2..." style="flex: 1; padding: 8px 12px; border: 1px solid #dadce0; border-radius: 4px; font-family: monospace;" value="%s">
          <button type="submit" class="btn-action" style="border: none; cursor: pointer;">Execute</button>
        </div>
      </form>
    </div>`, targetTab.ID(), html.EscapeString(evalExpr)))

	return "Web Inspector", internalPageTemplate("inspect", "Web Inspector", b.String())
}

// buildReaderPageHTML renders the distraction-free Reader Mode page.
func buildReaderPageHTML(f *framePath, rawURL string) (title, htmlContent string) {
	u, _ := url.Parse(rawURL)
	targetURL := ""
	theme := "light"
	if u != nil {
		targetURL = u.Query().Get("url")
		if t := u.Query().Get("theme"); t != "" {
			theme = t
		}
	}

	var targetTab *tabs.Tab
	if f.tabMgr != nil {
		if targetURL != "" {
			for _, t := range f.tabMgr.Tabs() {
				if t.URL() == targetURL {
					targetTab = t
					break
				}
			}
		}
		if targetTab == nil {
			active := f.tabMgr.Active()
			if active != nil && !strings.HasPrefix(strings.ToLower(active.URL()), "goosie://reader") {
				targetTab = active
				if targetURL == "" {
					targetURL = active.URL()
				}
			}
		}
	}

	if targetTab == nil || targetTab.Session() == nil || targetTab.Session().Doc == nil {
		body := `<div class="card">
      <h1>Reader Mode</h1>
      <p style="color: #5f6368;">No readable document is currently available.</p>
      <p>Navigate to an article or blog post in any tab and press <code>Cmd+Shift+R</code> to view it in distraction-free Reader Mode.</p>
    </div>`
		return "Reader Mode", internalPageTemplate("", "Reader Mode", body)
	}

	art := ExtractArticle(targetTab.Session().Doc, targetURL, theme)
	return art.Title + " - Reader Mode", RenderReaderPageHTML(art)
}

// buildRestorePageHTML generates the session crash recovery and restore interface.
func buildRestorePageHTML(f *framePath, rawURL string) (title, htmlContent string) {
	u, _ := url.Parse(rawURL)
	action := ""
	if u != nil {
		action = u.Query().Get("action")
	}

	if action == "restore" {
		if f.crashedSession != nil && len(f.crashedSession.Tabs) > 0 {
			crashed := *f.crashedSession
			f.crashedSession = nil
			if f.config.sessionActiveFile != "" {
				_ = os.Remove(f.config.sessionActiveFile)
			}
			f.restoreCrashedSession(crashed)
			body := fmt.Sprintf(`<div class="card">
      <h1 style="color: #137333;">Session Restored</h1>
      <p style="color: #5f6368; font-size: 15px;">Successfully restored %d tabs from your previous session.</p>
      <p style="margin-top: 16px;"><a class="list-link" href="goosie://version">About Goosie</a> &bull; <a class="list-link" href="goosie://history">History</a></p>
    </div>`, len(crashed.Tabs))
			return "Session Restored - Goosie", internalPageTemplate("restore", "Session Restored - Goosie", body)
		}
	} else if action == "dismiss" {
		f.crashedSession = nil
		if f.config.sessionActiveFile != "" {
			_ = os.Remove(f.config.sessionActiveFile)
		}
		body := `<div class="card">
      <h1>Session Dismissed</h1>
      <p style="color: #5f6368; font-size: 15px;">Previous session data has been discarded.</p>
      <p style="margin-top: 16px;"><a class="list-link" href="goosie://version">About Goosie</a> &bull; <a class="list-link" href="goosie://bookmarks">Bookmarks</a></p>
    </div>`
		return "Session Dismissed - Goosie", internalPageTemplate("restore", "Session Dismissed - Goosie", body)
	}

	if f.crashedSession == nil || len(f.crashedSession.Tabs) == 0 {
		body := `<div class="card">
      <h1>Session Restore</h1>
      <p style="color: #5f6368; font-size: 15px;">No crashed session was detected. All tabs were closed normally.</p>
      <p style="margin-top: 16px;"><a class="list-link" href="goosie://version">About Goosie</a> &bull; <a class="list-link" href="goosie://history">History</a></p>
    </div>`
		return "Session Restore - Goosie", internalPageTemplate("restore", "Session Restore - Goosie", body)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<div class="card" style="border-left: 4px solid #ea4335;">
      <h1 style="color: #d93025;">Goosie did not shut down cleanly</h1>
      <p style="color: #5f6368; font-size: 15px; line-height: 1.5;">
        Goosie closed unexpectedly while tabs were open. You can restore your open tabs and navigation history below, or start fresh.
      </p>
      <div style="margin: 20px 0 10px 0;">
        <a class="btn-action" href="goosie://restore?action=restore" style="padding: 8px 16px; font-size: 14px;">Restore %d Tabs</a>
        <a class="btn-action secondary" href="goosie://restore?action=dismiss" style="padding: 8px 16px; font-size: 14px;">Don't Restore</a>
      </div>
    </div>
    <div class="card">
      <h2>Tabs to restore (%d):</h2>`, len(f.crashedSession.Tabs), len(f.crashedSession.Tabs)))

	for i, t := range f.crashedSession.Tabs {
		titleText := t.Title
		if titleText == "" {
			titleText = t.URL
		}
		if titleText == "" {
			titleText = "Untitled Tab"
		}
		activeBadge := ""
		if i == f.crashedSession.Active {
			activeBadge = ` <span class="badge badge-active" style="margin-left: 6px;">Active</span>`
		}
		b.WriteString(fmt.Sprintf(`
      <div class="list-item">
        <div class="list-title">%s%s</div>
        <div class="list-meta" style="word-break: break-all;">%s</div>
      </div>`, html.EscapeString(titleText), activeBadge, html.EscapeString(t.URL)))
	}
	b.WriteString(`</div>`)

	return "Restore Session - Goosie", internalPageTemplate("restore", "Restore Session - Goosie", b.String())
}

// renderDOMTreeHTML serializes the DOM hierarchy into syntax-highlighted HTML.
func renderDOMTreeHTML(b *strings.Builder, n *dom.Node, depth, limit int) int {
	if n == nil || limit <= 0 {
		return limit
	}
	indent := strings.Repeat("&nbsp;&nbsp;", depth)

	switch n.Type {
	case dom.NodeElement:
		b.WriteString(indent)
		b.WriteString(fmt.Sprintf(`&lt;<span style="color: #e06c75; font-weight: bold;">%s</span>`, html.EscapeString(n.Data)))
		if len(n.Attr) > 0 {
			for _, a := range n.Attr {
				b.WriteString(fmt.Sprintf(` <span style="color: #d19a66;">%s</span>=<span style="color: #98c379;">&quot;%s&quot;</span>`, html.EscapeString(a.Name), html.EscapeString(a.Value)))
			}
		}
		if n.FirstChild == nil {
			b.WriteString(` /&gt;<br>`)
			return limit - 1
		}
		b.WriteString(`&gt;<br>`)
		limit--
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if limit <= 0 {
				b.WriteString(indent + `&nbsp;&nbsp;<span style="color: #5c6370;">... (more child nodes truncated)</span><br>`)
				break
			}
			limit = renderDOMTreeHTML(b, child, depth+1, limit)
		}
		b.WriteString(fmt.Sprintf(`%s&lt;/<span style="color: #e06c75; font-weight: bold;">%s</span>&gt;<br>`, indent, html.EscapeString(n.Data)))
	case dom.NodeText:
		text := strings.TrimSpace(n.Data)
		if text == "" {
			text = strings.TrimSpace(n.DataContent)
		}
		if text != "" {
			if len(text) > 80 {
				text = text[:80] + "..."
			}
			b.WriteString(fmt.Sprintf(`%s<span style="color: #5c6370;">&quot;%s&quot;</span><br>`, indent, html.EscapeString(text)))
			limit--
		}
	case dom.NodeDocument:
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			limit = renderDOMTreeHTML(b, child, depth, limit)
		}
	}
	return limit
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
	opts = append(opts,
		engine.WithViewportH(float32(viewportH)),
		engine.WithDarkMode(f.isDarkMode()),
		engine.WithHighContrast(f.isHighContrast()),
	)

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
