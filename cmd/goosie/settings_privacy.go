package main

import (
	"fmt"
	"html"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/net"
)

// clearDataRequest is the set of browsing-data scopes the settings page asked
// to clear. History has a time range because the store already supports one;
// cookies, local storage, the response cache, and the saved session are all
// all-or-nothing.
type clearDataRequest struct {
	History      bool
	HistoryScope string
	Cookies      bool
	Cache        bool
	Session      bool
	Storage      bool
}

func clearDataRequestFromValues(values url.Values) clearDataRequest {
	req := clearDataRequest{
		History:      values.Get("history") == "1",
		HistoryScope: values.Get("historyScope"),
		Cookies:      values.Get("cookies") == "1",
		Cache:        values.Get("cache") == "1",
		Session:      values.Get("session") == "1",
		Storage:      values.Get("storage") == "1",
	}
	switch req.HistoryScope {
	case "hour", "today", "all":
	default:
		req.HistoryScope = "all"
	}
	return req
}

func (r clearDataRequest) any() bool {
	return r.History || r.Cookies || r.Cache || r.Session || r.Storage
}

func (r clearDataRequest) historyScopeLabel() string {
	switch r.HistoryScope {
	case "hour":
		return "last hour"
	case "today":
		return "today"
	default:
		return "all time"
	}
}

func (r clearDataRequest) labels() []string {
	var labels []string
	if r.History {
		labels = append(labels, "browsing history ("+r.historyScopeLabel()+")")
	}
	if r.Cookies {
		labels = append(labels, "cookies for all sites")
	}
	if r.Storage {
		labels = append(labels, "local storage for all sites")
	}
	if r.Cache {
		labels = append(labels, "cached page resources")
	}
	if r.Session {
		labels = append(labels, "saved tabs and the crash-recovery journal")
	}
	return labels
}

// buildPrivacySettingsCard renders the browsing-data controls. Private sessions
// never write this data to the profile, so the card explains that instead of
// offering a destructive action that would have nothing to clear.
func (f *framePath) buildPrivacySettingsCard(values url.Values) string {
	var b strings.Builder
	b.WriteString(`<div class="card"><h1>Privacy &amp; browsing data</h1>`)
	if f.config.private {
		b.WriteString(`<p class="empty-state">Private sessions keep browsing data in memory only; they never write browsing data to this profile. There is nothing to clear here.</p></div>`)
		return b.String()
	}

	b.WriteString(`<p>Clear data stored in this profile. Bookmarks and settings are kept.</p>`)
	req := clearDataRequestFromValues(values)
	if values.Get("action") == "clear-data" {
		switch {
		case !req.any():
			b.WriteString(mgmtNoticeHTML("err", "Select at least one type of data to clear."))
		case values.Get("confirm") == "1":
			removed, problems := f.clearProfileData(req)
			switch {
			case len(problems) > 0 && len(removed) > 0:
				b.WriteString(mgmtNoticeHTML("err", "Cleared: "+strings.Join(removed, ", ")+". Could not clear: "+strings.Join(problems, "; ")+"."))
			case len(problems) > 0:
				b.WriteString(mgmtNoticeHTML("err", "Could not clear: "+strings.Join(problems, "; ")+"."))
			default:
				b.WriteString(mgmtNoticeHTML("ok", "Cleared: "+strings.Join(removed, ", ")+"."))
			}
		default:
			b.WriteString(privacyConfirmHTML(req))
		}
	}

	if values.Get("action") == "clear-site-cookies" {
		site := strings.TrimSpace(values.Get("site"))
		switch {
		case site == "":
			b.WriteString(mgmtNoticeHTML("err", "No site was selected."))
		case values.Get("confirm") == "1":
			removed, problem := f.clearSiteCookies(site)
			switch {
			case problem != "":
				b.WriteString(mgmtNoticeHTML("err", problem))
			case removed == 0:
				b.WriteString(mgmtNoticeHTML("err", "No cookies were found for "+site+"."))
			default:
				b.WriteString(mgmtNoticeHTML("ok", fmt.Sprintf("Cleared %d cookie(s) for %s.", removed, site)))
			}
		default:
			if count := siteCookieCount(f.client, site); count == 0 {
				b.WriteString(mgmtNoticeHTML("err", "No cookies were found for "+site+"."))
			} else {
				b.WriteString(siteCookieConfirmHTML(site, count))
			}
		}
	}

	b.WriteString(`<form method="GET" action="goosie://settings" style="max-width:640px;margin-top:16px;">
      <input type="hidden" name="action" value="clear-data">
      <div style="margin-bottom:12px;">
        <label for="clear-history"><input id="clear-history" type="checkbox" name="history" value="1"> Browsing history</label>
        <span style="margin-left:8px;">from</span>
        <select id="history-scope" name="historyScope" aria-label="History time range">
          <option value="hour">last hour</option>
          <option value="today">today</option>
          <option value="all" selected>all time</option>
        </select>
      </div>
      <div style="margin-bottom:12px;"><label for="clear-cookies"><input id="clear-cookies" type="checkbox" name="cookies" value="1"> Cookies for all sites</label></div>
      <div style="margin-bottom:12px;"><label for="clear-storage"><input id="clear-storage" type="checkbox" name="storage" value="1"> Local storage for all sites</label></div>
      <div style="margin-bottom:12px;"><label for="clear-cache"><input id="clear-cache" type="checkbox" name="cache" value="1"> Cached page resources</label></div>
      <div style="margin-bottom:12px;"><label for="clear-session"><input id="clear-session" type="checkbox" name="session" value="1"> Saved tabs and the crash-recovery journal</label></div>
      <p style="color:#5f6368;font-size:13px;">Selected data is removed from memory and from this profile's files. Bookmarks and settings are kept.</p>
      <button id="clear-browsing-data" class="btn-action danger" type="submit">Clear selected data</button>
    </form>`)
	b.WriteString(f.buildCookieSitesHTML())
	b.WriteString(`</div>`)
	return b.String()
}

// cookieSite is one domain that currently holds cookies, as listed by the
// per-site clear control. Counts are exact-domain, matching RemoveForDomain.
type cookieSite struct {
	domain string
	count  int
}

func siteCookieCount(client net.HTTP, site string) int {
	if client == nil || client.Jar() == nil {
		return 0
	}
	count := 0
	for _, c := range client.Jar().AllCookies() {
		if strings.EqualFold(c.Domain, strings.TrimSpace(site)) {
			count++
		}
	}
	return count
}

func cookieSites(client net.HTTP) []cookieSite {
	if client == nil || client.Jar() == nil {
		return nil
	}
	counts := make(map[string]int)
	for _, c := range client.Jar().AllCookies() {
		if c.Domain == "" {
			continue
		}
		counts[strings.ToLower(c.Domain)]++
	}
	sites := make([]cookieSite, 0, len(counts))
	for domain, count := range counts {
		sites = append(sites, cookieSite{domain: domain, count: count})
	}
	sort.Slice(sites, func(i, j int) bool { return sites[i].domain < sites[j].domain })
	return sites
}

func (f *framePath) buildCookieSitesHTML() string {
	sites := cookieSites(f.client)
	if len(sites) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<h2 style="font-size:16px;margin-top:24px;">Cookies by site</h2>
      <p style="color:#5f6368;font-size:13px;">Clear one site without touching cookies stored by the others.</p>
      <table><thead><tr><th>Site</th><th>Cookies</th><th>Action</th></tr></thead><tbody>`)
	for _, site := range sites {
		link := "goosie://settings?action=clear-site-cookies&site=" + url.QueryEscape(site.domain)
		b.WriteString(fmt.Sprintf(`<tr><td style="font-family:monospace;word-break:break-all;">%s</td><td>%d</td><td><a id="clear-site-%s" class="btn-action danger" href="%s">Clear</a></td></tr>`,
			html.EscapeString(site.domain), site.count, html.EscapeString(site.domain), html.EscapeString(link)))
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func siteCookieConfirmHTML(site string, count int) string {
	var b strings.Builder
	b.WriteString(`<div role="alertdialog" aria-label="Confirm clearing site cookies" style="background:#fef7e0;border:1px solid #f9ab00;border-radius:6px;padding:12px 16px;margin:16px 0;font-size:13px;color:#b06000;">
      <b>Clear cookies for <code>`)
	b.WriteString(html.EscapeString(site))
	b.WriteString(`</code>?</b> This permanently removes `)
	b.WriteString(fmt.Sprintf("%d cookie(s)", count))
	b.WriteString(` from memory and this profile. Cookies for other sites are kept.
      <form method="GET" action="goosie://settings" style="margin-top:10px;">
        <input type="hidden" name="action" value="clear-site-cookies">
        <input type="hidden" name="site" value="`)
	b.WriteString(html.EscapeString(site))
	b.WriteString(`">
        <input type="hidden" name="confirm" value="1">
        <button id="confirm-clear-site-cookies" class="btn-action danger" type="submit">Yes, clear cookies for this site</button>
        <a class="btn-action secondary" href="goosie://settings">Cancel</a>
      </form>
    </div>`)
	return b.String()
}

// clearSiteCookies removes one exact domain's cookies and rewrites the profile
// file so the deletion survives a restart. Other sites stay in memory and on
// disk; a failed save is reported instead of being hidden.
func (f *framePath) clearSiteCookies(site string) (removed int, problem string) {
	if f.client == nil || f.client.Jar() == nil {
		return 0, "cookies are unavailable in this session"
	}
	removed = f.client.Jar().RemoveForDomain(site)
	if removed == 0 {
		return 0, ""
	}
	if !f.config.private && f.config.cookieFile != "" {
		if err := f.client.Jar().Save(f.config.cookieFile); err != nil {
			return removed, fmt.Sprintf("cookies for %s were cleared in memory but the profile file could not be updated (%v)", site, err)
		}
	}
	return removed, ""
}

// privacyConfirmHTML carries the requested scopes into a second GET request so
// clearing only happens after the user has seen exactly what will be removed.
func privacyConfirmHTML(req clearDataRequest) string {
	var b strings.Builder
	b.WriteString(`<div role="alertdialog" aria-label="Confirm clearing browsing data" style="background:#fef7e0;border:1px solid #f9ab00;border-radius:6px;padding:12px 16px;margin:16px 0;font-size:13px;color:#b06000;">
      <b>Clear selected browsing data?</b> This permanently removes:
      <ul style="margin:8px 0 8px 18px;padding:0;">`)
	for _, label := range req.labels() {
		b.WriteString("<li>" + html.EscapeString(label) + "</li>")
	}
	b.WriteString(`</ul>Bookmarks and settings are kept.
      <form method="GET" action="goosie://settings" style="margin-top:10px;">
        <input type="hidden" name="action" value="clear-data">
        <input type="hidden" name="confirm" value="1">`)
	if req.History {
		b.WriteString(`<input type="hidden" name="history" value="1"><input type="hidden" name="historyScope" value="` + html.EscapeString(req.HistoryScope) + `">`)
	}
	if req.Cookies {
		b.WriteString(`<input type="hidden" name="cookies" value="1">`)
	}
	if req.Storage {
		b.WriteString(`<input type="hidden" name="storage" value="1">`)
	}
	if req.Cache {
		b.WriteString(`<input type="hidden" name="cache" value="1">`)
	}
	if req.Session {
		b.WriteString(`<input type="hidden" name="session" value="1">`)
	}
	b.WriteString(`<button id="confirm-clear-data" class="btn-action danger" type="submit">Yes, clear selected data</button>
        <a class="btn-action secondary" href="goosie://settings">Cancel</a>
      </form>
    </div>`)
	return b.String()
}

// clearProfileData erases the requested scopes in memory and on disk. It
// reports what was fully cleared and what failed, so a partial clear is visible
// rather than silent.
func (f *framePath) clearProfileData(req clearDataRequest) (removed, problems []string) {
	if req.History {
		if f.history == nil {
			problems = append(problems, "browsing history is unavailable in this session")
		} else {
			now := time.Now()
			var count int
			switch req.HistoryScope {
			case "hour":
				count = f.history.DeleteLastHour(now)
			case "today":
				count = f.history.DeleteToday(now)
			default:
				count = len(f.history.Entries())
				f.history.Clear()
			}
			if err := f.history.Save(); err != nil {
				problems = append(problems, fmt.Sprintf("browsing history was cleared in memory but could not be saved (%v)", err))
			} else {
				removed = append(removed, fmt.Sprintf("browsing history (%s, %d visits)", req.historyScopeLabel(), count))
			}
		}
	}

	if req.Cookies {
		if f.client == nil || f.client.Jar() == nil {
			problems = append(problems, "cookies are unavailable in this session")
		} else {
			f.client.Jar().Clear()
			if !f.config.private && f.config.cookieFile != "" {
				if err := os.Remove(f.config.cookieFile); err != nil && !os.IsNotExist(err) {
					problems = append(problems, fmt.Sprintf("cookies were cleared in memory but the profile file could not be removed (%v)", err))
				} else {
					removed = append(removed, "cookies for all sites")
				}
			} else {
				removed = append(removed, "cookies")
			}
		}
	}

	if req.Storage {
		switch {
		case f.config.private:
			problems = append(problems, "private sessions do not store profile site data")
		case f.storage == nil:
			problems = append(problems, "local storage is unavailable in this session")
		default:
			origins, files, err := f.storage.ClearAllLocalStorage()
			if err != nil {
				problems = append(problems, fmt.Sprintf("local storage was cleared in memory but some profile files could not be removed (%v)", err))
			} else {
				removed = append(removed, fmt.Sprintf("local storage (%d origins, %d files)", origins, files))
			}
		}
	}

	if req.Cache {
		if clearer, ok := f.client.(net.CacheClearingHTTP); ok {
			clearer.ClearCache()
			removed = append(removed, "cached page resources")
		} else {
			problems = append(problems, "the network client cannot clear its cache")
		}
	}

	if req.Session {
		switch {
		case f.config.private:
			problems = append(problems, "private sessions do not store a profile session")
		default:
			f.sessionMu.Lock()
			f.sessionDisabled.Store(true)
			var failed []string
			for _, path := range []string{f.config.sessionFile, f.config.sessionActiveFile} {
				if path == "" {
					continue
				}
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					failed = append(failed, err.Error())
				}
			}
			f.setCrashedSession(nil)
			f.sessionMu.Unlock()
			if len(failed) > 0 {
				problems = append(problems, "saved session files could not be removed: "+strings.Join(failed, "; "))
			} else {
				removed = append(removed, "saved tabs and the crash-recovery journal")
			}
		}
	}
	return removed, problems
}
