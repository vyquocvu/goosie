package main

import (
	"fmt"
	"html"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/net"
)

// clearDataRequest is the set of browsing-data scopes the settings page asked
// to clear. History has a time range because the store already supports one;
// cookies, the response cache, and the saved session are all-or-nothing.
type clearDataRequest struct {
	History      bool
	HistoryScope string
	Cookies      bool
	Cache        bool
	Session      bool
}

func clearDataRequestFromValues(values url.Values) clearDataRequest {
	req := clearDataRequest{
		History:      values.Get("history") == "1",
		HistoryScope: values.Get("historyScope"),
		Cookies:      values.Get("cookies") == "1",
		Cache:        values.Get("cache") == "1",
		Session:      values.Get("session") == "1",
	}
	switch req.HistoryScope {
	case "hour", "today", "all":
	default:
		req.HistoryScope = "all"
	}
	return req
}

func (r clearDataRequest) any() bool {
	return r.History || r.Cookies || r.Cache || r.Session
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
      <div style="margin-bottom:12px;"><label for="clear-cache"><input id="clear-cache" type="checkbox" name="cache" value="1"> Cached page resources</label></div>
      <div style="margin-bottom:12px;"><label for="clear-session"><input id="clear-session" type="checkbox" name="session" value="1"> Saved tabs and the crash-recovery journal</label></div>
      <p style="color:#5f6368;font-size:13px;">Selected data is removed from memory and from this profile's files. Bookmarks and settings are kept.</p>
      <button id="clear-browsing-data" class="btn-action danger" type="submit">Clear selected data</button>
    </form>`)
	b.WriteString(`</div>`)
	return b.String()
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
