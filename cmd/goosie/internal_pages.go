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
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/flags"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/perms"
	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/userscript"
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
		{"extensions", "Extensions", "goosie://extensions"},
		{"flags", "Flags", "goosie://flags"},
		{"settings", "Settings", "goosie://settings"},
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

// mgmtNoticeHTML renders an action-feedback banner on the management pages.
// The message is escaped; kind is "ok" or "err".
func mgmtNoticeHTML(kind, msg string) string {
	bg, border, fg := "#e6f4ea", "#34a853", "#137333"
	if kind == "err" {
		bg, border, fg = "#fce8e6", "#ea4335", "#c5221f"
	}
	return fmt.Sprintf(`<div role="status" style="background:%s;border:1px solid %s;color:%s;border-radius:6px;padding:10px 14px;margin-bottom:16px;font-size:13px;">%s</div>`,
		bg, border, fg, html.EscapeString(msg))
}

// mgmtSearchFormHTML renders the shared search box used by the bookmarks and
// history management pages. GET submission re-renders the page filtered by q.
func mgmtSearchFormHTML(page, placeholder, query string) string {
	return fmt.Sprintf(`
      <form action="goosie://%s" method="GET" style="display:flex;gap:8px;margin-bottom:16px;max-width:480px;">
        <input type="text" name="q" value="%s" placeholder="%s" aria-label="%s" style="flex:1;padding:6px 10px;border:1px solid #dadce0;border-radius:4px;font-size:13px;">
        <button type="submit" class="btn-action" style="border:none;cursor:pointer;">Search</button>
      </form>`, page, html.EscapeString(query), placeholder, placeholder)
}

// isLinkableURL reports whether a stored URL is safe to expose as a clickable
// link. Only http, https and file URLs are navigable by the shell; anything
// else (javascript:, data:, hostile schemes) is rendered as inert text.
func isLinkableURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "file":
		return true
	}
	return false
}

// mgmtEntryTitleHTML renders an entry title as a link when the target URL is
// navigable, or as inert escaped text otherwise.
func mgmtEntryTitleHTML(targetURL, title string) string {
	if isLinkableURL(targetURL) {
		return fmt.Sprintf(`<a class="list-link" href="%s">%s</a>`, html.EscapeString(targetURL), html.EscapeString(title))
	}
	return fmt.Sprintf(`<span>%s</span>`, html.EscapeString(title))
}

// buildBookmarksPageHTML generates the goosie://bookmarks management page:
// search, rename, delete and open for every saved bookmark. Mutations are
// applied to the store and persisted immediately; failures surface as a
// banner on the re-rendered page.
func buildBookmarksPageHTML(f *framePath, rawURL string) (string, string) {
	u, _ := url.Parse(rawURL)
	action, targetURL, newTitle, query := "", "", "", ""
	if u != nil {
		action = u.Query().Get("action")
		targetURL = u.Query().Get("url")
		newTitle = u.Query().Get("title")
		query = strings.TrimSpace(u.Query().Get("q"))
	}

	var b strings.Builder
	b.WriteString(`<div class="card"><h1>Bookmarks</h1>`)

	if f.bookmarks == nil {
		b.WriteString(`<div class="empty-state">Bookmark storage is unavailable in this session.<br><small style="margin-top: 8px; display:inline-block;">Private sessions never read or write your profile's bookmarks.</small></div></div>`)
		return "Bookmarks", internalPageTemplate("bookmarks", "Bookmarks", b.String())
	}

	notice := ""
	switch action {
	case "delete":
		if targetURL == "" {
			notice = mgmtNoticeHTML("err", "Delete failed: no bookmark URL was provided.")
		} else if f.bookmarks.Delete(targetURL) {
			if err := f.bookmarks.Save(); err != nil {
				notice = mgmtNoticeHTML("err", fmt.Sprintf("Bookmark removed for this session, but saving to disk failed: %v", err))
			} else {
				notice = mgmtNoticeHTML("ok", "Bookmark removed and saved.")
			}
		} else {
			notice = mgmtNoticeHTML("err", "Bookmark not found; it may already have been removed.")
		}
	case "rename":
		trimmed := strings.TrimSpace(newTitle)
		if targetURL == "" || trimmed == "" {
			notice = mgmtNoticeHTML("err", "Rename failed: a bookmark URL and a non-empty title are required.")
		} else if f.bookmarks.Rename(targetURL, trimmed) {
			if err := f.bookmarks.Save(); err != nil {
				notice = mgmtNoticeHTML("err", fmt.Sprintf("Title updated for this session, but saving to disk failed: %v", err))
			} else {
				notice = mgmtNoticeHTML("ok", "Bookmark title updated and saved.")
			}
		} else {
			notice = mgmtNoticeHTML("err", "Bookmark not found; it may already have been removed.")
		}
	}
	b.WriteString(notice)

	b.WriteString(mgmtSearchFormHTML("bookmarks", "Search bookmarks by title or URL...", query))
	if query != "" {
		b.WriteString(fmt.Sprintf(`<p style="font-size:13px;color:#5f6368;margin:-8px 0 14px 0;">Filtered by "%s" &bull; <a class="list-link" href="goosie://bookmarks">Clear search</a></p>`, html.EscapeString(query)))
	}

	entries := f.bookmarks.Search(query)
	if len(entries) == 0 {
		if query != "" {
			b.WriteString(`<div class="empty-state">No bookmarks match your search.</div>`)
		} else {
			b.WriteString(`<div class="empty-state">No bookmarks saved yet.<br><small style="margin-top: 8px; display:inline-block;">Tip: Press <b>Cmd+D</b> while browsing to star your favorite pages.</small></div>`)
		}
	}

	qSuffix := ""
	if query != "" {
		qSuffix = "&q=" + url.QueryEscape(query)
	}
	for _, bm := range entries {
		title := bm.Title
		if title == "" {
			title = bm.URL
		}
		dateStr := bm.Added.Format("Jan 02, 2006 15:04")
		delLink := fmt.Sprintf(`goosie://bookmarks?action=delete&url=%s%s`, url.QueryEscape(bm.URL), qSuffix)
		qHidden := ""
		if query != "" {
			qHidden = fmt.Sprintf(`<input type="hidden" name="q" value="%s">`, html.EscapeString(query))
		}
		b.WriteString(fmt.Sprintf(`
      <div class="list-item">
        <div style="display:flex;justify-content:space-between;align-items:flex-start;gap:12px;">
          <div style="min-width:0;flex:1;">
            <div class="list-title">%s</div>
            <div class="list-meta" style="word-break:break-all;">★ Starred on %s &bull; <span style="font-family: monospace;">%s</span></div>
            <form action="goosie://bookmarks" method="GET" style="display:flex;gap:6px;margin-top:8px;max-width:420px;">
              <input type="hidden" name="action" value="rename">
              <input type="hidden" name="url" value="%s">
              %s
              <input type="text" name="title" value="%s" aria-label="Title for %s" style="flex:1;padding:4px 8px;border:1px solid #dadce0;border-radius:4px;font-size:12px;">
              <button type="submit" class="btn-action secondary" style="border:1px solid #dadce0;cursor:pointer;">Rename</button>
            </form>
          </div>
          <div style="white-space:nowrap;"><a class="btn-action danger" href="%s">Delete</a></div>
        </div>
      </div>`,
			mgmtEntryTitleHTML(bm.URL, title),
			dateStr, html.EscapeString(bm.URL),
			html.EscapeString(bm.URL),
			qHidden,
			html.EscapeString(bm.Title), html.EscapeString(bm.URL),
			delLink))
	}
	b.WriteString(`</div>`)
	return "Bookmarks", internalPageTemplate("bookmarks", "Bookmarks", b.String())
}

// buildHistoryPageHTML generates the goosie://history management page:
// search, per-entry delete and clear-all (with a confirmation step) over the
// visit log. Mutations persist immediately; failures surface as a banner.
func buildHistoryPageHTML(f *framePath, rawURL string) (string, string) {
	u, _ := url.Parse(rawURL)
	action, targetURL, atParam, confirm, query := "", "", "", "", ""
	if u != nil {
		action = u.Query().Get("action")
		targetURL = u.Query().Get("url")
		atParam = u.Query().Get("at")
		confirm = u.Query().Get("confirm")
		query = strings.TrimSpace(u.Query().Get("q"))
	}

	var b strings.Builder
	b.WriteString(`<div class="card">
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px;">
      <h1 style="margin:0;">Browsing History</h1>`)
	if f.history != nil {
		b.WriteString(`<a class="btn-action danger" href="goosie://history?action=clear">Clear All History</a>`)
	}
	b.WriteString(`</div>`)

	if f.history == nil {
		b.WriteString(`<div class="empty-state">History storage is unavailable in this session.<br><small style="margin-top: 8px; display:inline-block;">Private sessions never read or write your profile's history.</small></div></div>`)
		return "Browsing History", internalPageTemplate("history", "Browsing History", b.String())
	}

	notice := ""
	switch action {
	case "delete":
		at, err := time.Parse(time.RFC3339Nano, atParam)
		switch {
		case targetURL == "" || atParam == "" || err != nil:
			notice = mgmtNoticeHTML("err", "Delete failed: the history entry reference was missing or malformed.")
		case f.history.DeleteEntry(targetURL, at):
			if err := f.history.Save(); err != nil {
				notice = mgmtNoticeHTML("err", fmt.Sprintf("Entry removed for this session, but saving to disk failed: %v", err))
			} else {
				notice = mgmtNoticeHTML("ok", "History entry removed and saved.")
			}
		default:
			notice = mgmtNoticeHTML("err", "History entry not found; it may already have been removed.")
		}
	case "clear":
		if confirm == "1" {
			f.history.Clear()
			if err := f.history.Save(); err != nil {
				notice = mgmtNoticeHTML("err", fmt.Sprintf("History cleared for this session, but saving to disk failed: %v", err))
			} else {
				notice = mgmtNoticeHTML("ok", "Browsing history cleared and saved.")
			}
		} else {
			count := len(f.history.Entries())
			b.WriteString(fmt.Sprintf(`<div role="alertdialog" aria-label="Confirm clearing history" style="background:#fef7e0;border:1px solid #f9ab00;border-radius:6px;padding:12px 16px;margin-bottom:16px;font-size:13px;color:#b06000;">
        <b>Clear all browsing history?</b> This permanently removes %d visit(s) from this profile. Bookmarks are kept.
        <div style="margin-top:10px;">
          <a class="btn-action danger" href="goosie://history?action=clear&confirm=1">Yes, Clear History</a>
          <a class="btn-action secondary" href="goosie://history">Cancel</a>
        </div>
      </div>`, count))
		}
	}
	b.WriteString(notice)

	b.WriteString(mgmtSearchFormHTML("history", "Search history by title or URL...", query))
	if query != "" {
		b.WriteString(fmt.Sprintf(`<p style="font-size:13px;color:#5f6368;margin:-8px 0 14px 0;">Filtered by "%s" &bull; <a class="list-link" href="goosie://history">Clear search</a></p>`, html.EscapeString(query)))
	}

	entries := f.history.Search(query)
	if len(entries) == 0 {
		if query != "" {
			b.WriteString(`<div class="empty-state">No history entries match your search.</div>`)
		} else {
			b.WriteString(`<div class="empty-state">Your browsing history is currently empty.</div>`)
		}
	}

	qSuffix := ""
	if query != "" {
		qSuffix = "&q=" + url.QueryEscape(query)
	}
	// Show newest first
	for i := len(entries) - 1; i >= 0; i-- {
		h := entries[i]
		title := h.Title
		if title == "" {
			title = h.URL
		}
		dateStr := h.At.Format("Jan 02, 2006 15:04:05")
		delLink := fmt.Sprintf(`goosie://history?action=delete&url=%s&at=%s%s`,
			url.QueryEscape(h.URL), url.QueryEscape(h.At.Format(time.RFC3339Nano)), qSuffix)
		b.WriteString(fmt.Sprintf(`
      <div class="list-item">
        <div style="display:flex;justify-content:space-between;align-items:flex-start;gap:12px;">
          <div style="min-width:0;flex:1;">
            <div class="list-title">%s</div>
            <div class="list-meta" style="word-break:break-all;">Visited at %s &bull; <span style="font-family: monospace;">%s</span></div>
          </div>
          <div style="white-space:nowrap;"><a class="btn-action danger" href="%s">Delete</a></div>
        </div>
      </div>`,
			mgmtEntryTitleHTML(h.URL, title),
			dateStr, html.EscapeString(h.URL),
			delLink))
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

	dir := f.downloadDirectory()
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

// buildSettingsPageHTML renders the settings page and handles the deliberately
// simple GET form used by other Goosie management pages. Settings are profile
// scoped; private sessions can inspect them but cannot mutate the profile.
func buildSettingsPageHTML(f *framePath, rawURL string) (title, htmlContent string) {
	settings := f.currentPreferences()
	if settings.SearchURLTemplate == "" {
		settings = preferences.Defaults()
	}
	if settings.DownloadDir == "" {
		settings.DownloadDir = f.downloadDirectory()
	}
	notice := ""
	u, err := url.Parse(rawURL)
	if err != nil {
		notice = mgmtNoticeHTML("err", "The settings request could not be parsed.")
	} else if u.Query().Get("save") == "1" {
		if f.config.private {
			notice = mgmtNoticeHTML("err", "Private sessions cannot change profile settings.")
		} else {
			candidate := preferences.Settings{
				SearchURLTemplate:   u.Query().Get("searchURLTemplate"),
				DownloadDir:         strings.TrimSpace(u.Query().Get("downloadDir")),
				StartupMode:         u.Query().Get("startupMode"),
				AskDownloadLocation: u.Query().Get("askDownloadLocation") == "1",
			}
			if candidate.DownloadDir == "" {
				candidate.DownloadDir = settings.DownloadDir
			}
			if candidate.StartupMode == "" {
				candidate.StartupMode = settings.StartupMode
			}
			if err := preferences.ValidateSearchURLTemplate(candidate.SearchURLTemplate); err != nil {
				notice = mgmtNoticeHTML("err", "Search URL was not saved: "+err.Error()+".")
			} else if err := preferences.ValidateDownloadDir(candidate.DownloadDir); err != nil {
				notice = mgmtNoticeHTML("err", "Download directory was not saved: "+err.Error()+".")
			} else if err := preferences.ValidateStartupMode(candidate.StartupMode); err != nil {
				notice = mgmtNoticeHTML("err", "Startup behavior was not saved: "+err.Error()+".")
			} else if err := preferences.Save(f.config.preferencesFile, candidate); err != nil {
				notice = mgmtNoticeHTML("err", "Search URL could not be saved: "+err.Error()+".")
			} else {
				settings = candidate
				f.setPreferences(candidate)
				notice = mgmtNoticeHTML("ok", "Settings saved for this profile.")
			}
		}
	}

	var b strings.Builder
	b.WriteString(`<div class="card"><h1>Settings</h1><p>Choose the search URL Goosie uses for address bar queries, where new downloads are saved, and what to open on startup. The URL must use <code>{query}</code> exactly once.</p>`)
	b.WriteString(notice)
	if f.config.private {
		b.WriteString(`<p class="empty-state">Profile settings are read-only in a private session.</p>`)
	} else {
		b.WriteString(`<form method="GET" action="goosie://settings" style="max-width:640px;">`)
		b.WriteString(`<input type="hidden" name="save" value="1">`)
		b.WriteString(`<div style="margin-bottom:12px;"><label for="search-url" style="display:block;margin-bottom:6px;">Search URL template</label><input id="search-url" name="searchURLTemplate" value="`)
		b.WriteString(html.EscapeString(settings.SearchURLTemplate))
		b.WriteString(`" required autocomplete="off" style="display:block;width:100%;padding:8px;border:1px solid #dadce0;border-radius:5px;"></div>`)
		b.WriteString(`<div style="margin-bottom:12px;"><label for="download-directory" style="display:block;margin-bottom:6px;">Download directory</label><input id="download-directory" name="downloadDir" value="`)
		b.WriteString(html.EscapeString(settings.DownloadDir))
		b.WriteString(`" required autocomplete="off" style="display:block;width:100%;padding:8px;border:1px solid #dadce0;border-radius:5px;"></div>`)
		if settings.AskDownloadLocation {
			b.WriteString(`<div style="margin-bottom:12px;"><label><input id="ask-download-location" type="checkbox" name="askDownloadLocation" value="1" checked> Ask where to save each download</label></div>`)
		} else {
			b.WriteString(`<div style="margin-bottom:12px;"><label><input id="ask-download-location" type="checkbox" name="askDownloadLocation" value="1"> Ask where to save each download</label></div>`)
		}
		b.WriteString(`<div style="margin-bottom:12px;"><div style="margin-bottom:6px;">On startup</div>`)
		if settings.StartupMode == preferences.StartupNewTab {
			b.WriteString(`<label style="margin-right:16px;"><input id="startup-restore" type="radio" name="startupMode" value="restore"> Continue where you left off</label><label><input id="startup-newtab" type="radio" name="startupMode" value="newtab" checked> Open a new tab</label>`)
		} else {
			b.WriteString(`<label style="margin-right:16px;"><input id="startup-restore" type="radio" name="startupMode" value="restore" checked> Continue where you left off</label><label><input id="startup-newtab" type="radio" name="startupMode" value="newtab"> Open a new tab</label>`)
		}
		b.WriteString(`</div><button id="save-search" class="btn-action" type="submit">Save</button></form>`)
	}
	b.WriteString(`<p style="color:#5f6368;font-size:13px;margin-top:12px;">Example: <code>https://www.google.com/search?q={query}</code></p></div>`)
	return "Settings", internalPageTemplate("settings", "Settings", b.String())
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
		return buildBookmarksPageHTML(f, rawURL)
	case "history":
		return buildHistoryPageHTML(f, rawURL)
	case "downloads":
		return buildDownloadsPageHTML(f, rawURL)
	case "inspect", "devtools":
		return buildInspectPageHTML(f, rawURL)
	case "reader":
		return buildReaderPageHTML(f, rawURL)
	case "restore":
		return buildRestorePageHTML(f, rawURL)
	case "extensions", "userscripts":
		return buildExtensionsPageHTML(f, rawURL)
	case "flags":
		return buildFlagsPageHTML(f, rawURL)
	case "settings":
		return buildSettingsPageHTML(f, rawURL)
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
        <li><a class="list-link" href="goosie://extensions">goosie://extensions</a> &mdash; Extensions &amp; User Scripts Manager</li>
        <li><a class="list-link" href="goosie://flags">goosie://flags</a> &mdash; Experiments &amp; Feature Flags</li>
        <li><a class="list-link" href="goosie://settings">goosie://settings</a> &mdash; Browser preferences</li>
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
		case "toggle_standalone":
			targetTab.SetStandaloneApp(!targetTab.IsStandaloneApp())
			if cw, ok := f.window.(*chromeWindow); ok {
				cw.notifyContentResize()
			}
		case "perm_set":
			p := perms.Permission(u.Query().Get("perm"))
			st := perms.PermissionState(u.Query().Get("state"))
			orig := u.Query().Get("origin")
			if orig == "" {
				orig = perms.NormalizeOrigin(targetTab.URL())
			}
			if f.perms != nil && p != "" && st != "" {
				f.perms.Set(orig, p, st)
				_ = f.perms.Save()
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

	// Progressive Web App (PWA) & Manifest Card
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Progressive Web App (PWA) &amp; Manifest</h2>`)
	m := targetTab.Manifest()
	if m == nil {
		b.WriteString(`<p style="color: #5f6368;">No W3C Web App Manifest detected for this page (no <code>&lt;link rel="manifest"&gt;</code>).</p>`)
		isStandalone := targetTab.IsStandaloneApp()
		statusText := "Browser Mode"
		actionBtnText := "Switch to Standalone Mode"
		if isStandalone {
			statusText = "Standalone App Mode (Chromeless)"
			actionBtnText = "Switch to Browser Mode"
		}
		b.WriteString(fmt.Sprintf(`<p style="font-size: 13px; color: #3c4043; margin-top: 8px;">Current Presentation: <b>%s</b></p>`, statusText))
		toggleLink := fmt.Sprintf(`goosie://inspect?action=toggle_standalone&tab=%d`, targetTab.ID())
		b.WriteString(fmt.Sprintf(`<div style="margin-top: 10px;"><a class="btn-action secondary" href="%s">%s</a></div>`, toggleLink, actionBtnText))
	} else {
		b.WriteString(`<table><tbody>`)
		b.WriteString(fmt.Sprintf(`<tr><td style="width: 140px; font-weight: bold;">App Name</td><td>%s</td></tr>`, html.EscapeString(m.EffectiveName())))
		if m.ShortName != "" {
			b.WriteString(fmt.Sprintf(`<tr><td style="font-weight: bold;">Short Name</td><td>%s</td></tr>`, html.EscapeString(m.ShortName)))
		}
		if m.Description != "" {
			b.WriteString(fmt.Sprintf(`<tr><td style="font-weight: bold;">Description</td><td>%s</td></tr>`, html.EscapeString(m.Description)))
		}
		b.WriteString(fmt.Sprintf(`<tr><td style="font-weight: bold;">Start URL</td><td><code>%s</code></td></tr>`, html.EscapeString(m.ResolveStartURL(targetTab.URL()))))
		b.WriteString(fmt.Sprintf(`<tr><td style="font-weight: bold;">Display Mode</td><td><code>%s</code></td></tr>`, html.EscapeString(string(m.Display))))
		if m.ThemeColor != "" {
			b.WriteString(fmt.Sprintf(`<tr><td style="font-weight: bold;">Theme Color</td><td><span style="display:inline-block;width:12px;height:12px;background:%s;border:1px solid #ccc;vertical-align:middle;margin-right:6px;"></span><code>%s</code></td></tr>`, html.EscapeString(m.ThemeColor), html.EscapeString(m.ThemeColor)))
		}
		if m.BackgroundColor != "" {
			b.WriteString(fmt.Sprintf(`<tr><td style="font-weight: bold;">Background Color</td><td><span style="display:inline-block;width:12px;height:12px;background:%s;border:1px solid #ccc;vertical-align:middle;margin-right:6px;"></span><code>%s</code></td></tr>`, html.EscapeString(m.BackgroundColor), html.EscapeString(m.BackgroundColor)))
		}
		if len(m.Icons) > 0 {
			b.WriteString(fmt.Sprintf(`<tr><td style="font-weight: bold;">Icons</td><td>%d icon(s) declared</td></tr>`, len(m.Icons)))
		}
		isStandalone := targetTab.IsStandaloneApp()
		statusText := "Browser Mode"
		actionBtnText := "Switch to Standalone App Mode"
		if isStandalone {
			statusText = "Standalone App Mode (Chromeless)"
			actionBtnText = "Switch to Browser Tab Mode"
		}
		b.WriteString(fmt.Sprintf(`<tr><td style="font-weight: bold;">Current Presentation</td><td><b>%s</b></td></tr>`, statusText))
		b.WriteString(`</tbody></table>`)

		toggleLink := fmt.Sprintf(`goosie://inspect?action=toggle_standalone&tab=%d`, targetTab.ID())
		b.WriteString(fmt.Sprintf(`<div style="margin-top: 12px;"><a class="btn-action" href="%s">%s</a></div>`, toggleLink, actionBtnText))
	}
	b.WriteString(`</div>`)

	// Site Permissions & Content Settings Card
	origin = perms.NormalizeOrigin(targetTab.URL())
	b.WriteString(`<div class="card">`)
	b.WriteString(`<h2>Site Permissions &amp; Content Settings</h2>`)
	b.WriteString(fmt.Sprintf(`<p style="color: #5f6368; font-size: 13px;">Permissions for origin: <code>%s</code></p>`, html.EscapeString(origin)))
	b.WriteString(`<table><thead><tr><th style="text-align: left;">Capability</th><th style="text-align: left;">Status</th><th style="text-align: right;">Action</th></tr></thead><tbody>`)

	capabilities := []struct {
		perm perms.Permission
		name string
	}{
		{perms.PermissionJavaScript, "JavaScript Execution"},
		{perms.PermissionImages, "Image Loading"},
		{perms.PermissionCookies, "Cookie Storage"},
		{perms.PermissionClipboardRead, "Clipboard Read"},
		{perms.PermissionClipboardWrite, "Clipboard Write"},
	}

	for _, cap := range capabilities {
		currState := perms.StateDefault
		if f.perms != nil {
			currState = f.perms.State(origin, cap.perm)
		}
		var statusLabel string
		var nextState perms.PermissionState
		var btnLabel string
		btnClass := "btn-action danger"

		switch currState {
		case perms.StateBlock:
			statusLabel = `<span style="color:#c5221f; font-weight:bold;">Blocked</span>`
			nextState = perms.StateAllow
			btnLabel = "Allow"
			btnClass = "btn-action secondary"
		case perms.StateAllow:
			statusLabel = `<span style="color:#137333; font-weight:bold;">Allowed (Explicit)</span>`
			nextState = perms.StateBlock
			btnLabel = "Block"
			btnClass = "btn-action danger"
		default:
			if cap.perm == perms.PermissionClipboardRead || cap.perm == perms.PermissionClipboardWrite {
				statusLabel = `<span style="color:#8a5a00;">Prompt on user activation (Default)</span>`
			} else {
				statusLabel = `<span style="color:#137333;">Allowed (Default)</span>`
			}
			nextState = perms.StateBlock
			btnLabel = "Block"
			btnClass = "btn-action danger"
		}

		toggleURL := fmt.Sprintf(`goosie://inspect?action=perm_set&origin=%s&perm=%s&state=%s&tab=%d`,
			url.QueryEscape(origin), url.QueryEscape(string(cap.perm)), url.QueryEscape(string(nextState)), targetTab.ID())
		resetURL := fmt.Sprintf(`goosie://inspect?action=perm_set&origin=%s&perm=%s&state=%s&tab=%d`,
			url.QueryEscape(origin), url.QueryEscape(string(cap.perm)), url.QueryEscape(string(perms.StateDefault)), targetTab.ID())

		actionsHTML := fmt.Sprintf(`<a class="%s" style="padding:2px 8px;font-size:11px;" href="%s">%s</a>`, btnClass, toggleURL, btnLabel)
		if currState == perms.StateDefault && (cap.perm == perms.PermissionClipboardRead || cap.perm == perms.PermissionClipboardWrite) {
			allowURL := fmt.Sprintf(`goosie://inspect?action=perm_set&origin=%s&perm=%s&state=%s&tab=%d`,
				url.QueryEscape(origin), url.QueryEscape(string(cap.perm)), url.QueryEscape(string(perms.StateAllow)), targetTab.ID())
			actionsHTML = fmt.Sprintf(`<a class="btn-action secondary" style="padding:2px 8px;font-size:11px;" href="%s">Allow</a> %s`, allowURL, actionsHTML)
		}
		if currState != perms.StateDefault {
			actionsHTML += fmt.Sprintf(` <a class="btn-action secondary" style="padding:2px 8px;font-size:11px;" href="%s">Reset</a>`, resetURL)
		}

		b.WriteString(fmt.Sprintf(`<tr><td><b>%s</b></td><td>%s</td><td style="text-align: right;">%s</td></tr>`, html.EscapeString(cap.name), statusLabel, actionsHTML))
	}
	b.WriteString(`</tbody></table></div>`)

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

// buildExtensionsPageHTML generates the user scripts & extensions management page.
func buildExtensionsPageHTML(f *framePath, rawURL string) (title, htmlContent string) {
	u, _ := url.Parse(rawURL)
	action := ""
	id := ""
	if u != nil {
		action = u.Query().Get("action")
		id = u.Query().Get("id")
	}

	if action == "toggle" && id != "" && f.userscripts != nil {
		s := f.userscripts.ScriptByID(id)
		if s != nil {
			f.userscripts.SetEnabled(id, !s.Enabled)
		}
	}

	var scripts []*userscript.Script
	dir := ""
	if f.userscripts != nil {
		scripts = f.userscripts.Scripts()
		dir = f.userscripts.Dir()
	}

	var b strings.Builder
	b.WriteString(`<div class="card">
      <h1>Extensions &amp; User Scripts</h1>
      <p style="color: #5f6368; font-size: 14px;">
        Manage custom user scripts and extensions. Drop <code>*.user.js</code> scripts into your profile's scripts folder to automatically inject custom styling and behavior.
      </p>`)

	if dir != "" {
		b.WriteString(fmt.Sprintf(`<p style="font-size: 13px; color: #5f6368; margin-top: 8px;">Folder: <code>%s</code></p>`, html.EscapeString(dir)))
	}
	b.WriteString(`</div>`)

	if len(scripts) == 0 {
		b.WriteString(`<div class="card empty-state">
      <p style="font-size: 16px; font-weight: 500; margin-bottom: 8px;">No User Scripts Installed</p>
      <p style="color: #5f6368; font-size: 13px;">Save Greasemonkey / Tampermonkey compatible <code>.user.js</code> files to your userscripts directory to customize any website.</p>
    </div>`)
		return "Extensions - Goosie", internalPageTemplate("extensions", "Extensions - Goosie", b.String())
	}

	b.WriteString(`<div class="card">
      <h2>Installed Scripts (` + strconv.Itoa(len(scripts)) + `)</h2>`)

	for _, s := range scripts {
		statusBadge := `<span class="badge badge-completed">Enabled</span>`
		toggleBtn := fmt.Sprintf(`<a class="btn-action secondary" href="goosie://extensions?action=toggle&id=%s">Disable</a>`, url.QueryEscape(s.ID))
		if !s.Enabled {
			statusBadge = `<span class="badge badge-canceled">Disabled</span>`
			toggleBtn = fmt.Sprintf(`<a class="btn-action" href="goosie://extensions?action=toggle&id=%s">Enable</a>`, url.QueryEscape(s.ID))
		}

		desc := s.Description
		if desc == "" {
			desc = "No description provided."
		}

		matchSummary := strings.Join(s.Matches, ", ")
		if matchSummary == "" {
			matchSummary = strings.Join(s.Includes, ", ")
		}
		if matchSummary == "" {
			matchSummary = "All URLs (<all_urls>)"
		}

		b.WriteString(fmt.Sprintf(`
      <div class="list-item" style="padding: 16px 0;">
        <div style="display: flex; justify-content: space-between; align-items: flex-start;">
          <div>
            <div class="list-title" style="font-size: 16px;">%s <span style="font-size: 12px; font-weight: normal; color: #5f6368;">v%s</span> %s</div>
            <div style="font-size: 13px; color: #3c4043; margin-top: 4px;">%s</div>
            <div class="list-meta" style="margin-top: 6px;">
              <strong>Matches:</strong> <code>%s</code> &bull; <strong>Run-at:</strong> <code>%s</code>
            </div>
          </div>
          <div style="margin-left: 16px; white-space: nowrap;">
            %s
          </div>
        </div>
      </div>`,
			html.EscapeString(s.Name),
			html.EscapeString(s.Version),
			statusBadge,
			html.EscapeString(desc),
			html.EscapeString(matchSummary),
			html.EscapeString(string(s.RunAt)),
			toggleBtn,
		))
	}
	b.WriteString(`</div>`)

	return "Extensions - Goosie", internalPageTemplate("extensions", "Extensions - Goosie", b.String())
}

// buildFlagsPageHTML generates the experimental feature flags configuration page.
func buildFlagsPageHTML(f *framePath, rawURL string) (title, htmlContent string) {
	u, _ := url.Parse(rawURL)
	action := ""
	id := ""
	query := ""
	if u != nil {
		action = u.Query().Get("action")
		id = u.Query().Get("id")
		query = strings.ToLower(strings.TrimSpace(u.Query().Get("q")))
	}

	if f.flags != nil {
		switch action {
		case "toggle":
			if id != "" {
				f.flags.SetEnabled(id, !f.flags.IsEnabled(id))
				_ = f.flags.Save()
			}
		case "reset":
			f.flags.ResetAll()
			_ = f.flags.Save()
		}
	}

	var allFlags []flags.Flag
	hasModified := false
	if f.flags != nil {
		allFlags = f.flags.Flags()
		hasModified = f.flags.HasNonDefault()
	}

	var b strings.Builder
	b.WriteString(`<div class="card">
      <h1>Experiments &amp; Feature Flags</h1>
      <p style="color: #5f6368; font-size: 14px; margin-top: 4px;">
        Configure advanced engine capabilities and experimental features. Use caution: experimental features may impact browser performance, stability, or web compatibility.
      </p>`)

	if hasModified {
		b.WriteString(`<div style="background-color: #fef7e0; border: 1px solid #f9ab00; border-radius: 6px; padding: 12px 16px; margin-top: 14px; color: #b06000; font-size: 13px; font-weight: 500;">
        <strong>Warning:</strong> You have modified experimental feature flags. Your changes will take full effect after restarting Goosie.
      </div>`)
	}

	b.WriteString(`<div style="display: flex; justify-content: space-between; align-items: center; margin-top: 16px;">
        <form action="goosie://flags" method="GET" style="display: flex; gap: 8px; flex: 1; max-width: 400px;">
          <input type="text" name="q" placeholder="Search flags..." value="` + html.EscapeString(query) + `" style="flex: 1; padding: 6px 10px; border: 1px solid #dadce0; border-radius: 4px; font-size: 13px;">
          <button type="submit" class="btn-action" style="cursor: pointer; border: none;">Search</button>
        </form>
        <div>
          <a class="btn-action secondary" href="goosie://flags?action=reset">Reset All to Default</a>
        </div>
      </div>
    </div>`)

	// Filter flags if query present
	var filtered []flags.Flag
	for _, fl := range allFlags {
		if query == "" || strings.Contains(strings.ToLower(fl.Name), query) || strings.Contains(strings.ToLower(fl.Description), query) || strings.Contains(strings.ToLower(fl.ID), query) {
			filtered = append(filtered, fl)
		}
	}

	if len(filtered) == 0 {
		b.WriteString(`<div class="card empty-state">
        <p style="font-size: 15px; font-weight: 500;">No flags match your search query.</p>
        <p style="color: #5f6368; font-size: 13px; margin-top: 6px;"><a href="goosie://flags" style="color: #1a73e8;">Clear search</a> to view all available feature flags.</p>
      </div>`)
		return "Experiments - Goosie", internalPageTemplate("flags", "Experiments - Goosie", b.String())
	}

	b.WriteString(`<div class="card">
      <h2>Available Feature Flags (` + strconv.Itoa(len(filtered)) + `)</h2>`)

	for _, fl := range filtered {
		statusBadge := `<span class="badge" style="background:#f1f3f4; color:#5f6368;">Default</span>`
		if fl.Enabled != fl.Default {
			if fl.Enabled {
				statusBadge = `<span class="badge badge-completed">Enabled</span>`
			} else {
				statusBadge = `<span class="badge badge-canceled">Disabled</span>`
			}
		}

		toggleText := "Disable"
		btnCls := "secondary"
		if !fl.Enabled {
			toggleText = "Enable"
			btnCls = ""
		}
		toggleLink := fmt.Sprintf(`goosie://flags?action=toggle&id=%s`, url.QueryEscape(fl.ID))
		if query != "" {
			toggleLink += "&q=" + url.QueryEscape(query)
		}

		defaultDesc := "Default: Enabled"
		if !fl.Default {
			defaultDesc = "Default: Disabled"
		}

		b.WriteString(fmt.Sprintf(`
      <div class="list-item" style="padding: 16px 0;">
        <div style="display: flex; justify-content: space-between; align-items: flex-start;">
          <div>
            <div class="list-title" style="font-size: 15px;">%s %s</div>
            <div style="font-size: 13px; color: #3c4043; margin-top: 4px;">%s</div>
            <div class="list-meta" style="margin-top: 6px;">
              <code>#%s</code> &bull; <span style="color: #5f6368;">%s</span>
            </div>
          </div>
          <div style="margin-left: 16px; white-space: nowrap;">
            <a class="btn-action %s" href="%s">%s</a>
          </div>
        </div>
      </div>`,
			html.EscapeString(fl.Name),
			statusBadge,
			html.EscapeString(fl.Description),
			html.EscapeString(fl.ID),
			defaultDesc,
			btnCls,
			toggleLink,
			toggleText,
		))
	}

	b.WriteString(`</div>`)
	return "Experiments - Goosie", internalPageTemplate("flags", "Experiments - Goosie", b.String())
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
