package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/history"
	gonet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/session"
)

func newPrivacyTestPath(t *testing.T) (*framePath, string) {
	t.Helper()
	f := newNavTestPath(t)
	dir := t.TempDir()
	f.config = config{
		cookieFile:        filepath.Join(dir, "cookies.json"),
		historyFile:       filepath.Join(dir, "history.json"),
		sessionFile:       filepath.Join(dir, "session.json"),
		sessionActiveFile: filepath.Join(dir, "session_active.json"),
		bookmarkFile:      filepath.Join(dir, "bookmarks.json"),
	}
	f.client = gonet.DefaultClient()
	t.Cleanup(func() { _ = f.client.Close() })
	return f, dir
}

func writeHistoryFile(t *testing.T, path string, entries []history.Entry) {
	t.Helper()
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsPrivacyCardListsSupportedScopes(t *testing.T) {
	f, _ := newPrivacyTestPath(t)
	_, body := f.buildInternalPageHTML("goosie://settings")

	for _, want := range []string{
		"Privacy &amp; browsing data",
		"Browsing history",
		"last hour",
		"today",
		"all time",
		"Cookies for all sites",
		"Cached page resources",
		"Saved tabs and the crash-recovery journal",
		"Bookmarks and settings are kept",
		`name="action" value="clear-data"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("settings page missing %q", want)
		}
	}
}

func TestSettingsClearDataWaitsForConfirmation(t *testing.T) {
	f, _ := newPrivacyTestPath(t)
	writeHistoryFile(t, f.config.historyFile, []history.Entry{
		{URL: "https://keep.example/", Title: "Keep", At: time.Now()},
	})
	st, err := history.Load(f.config.historyFile)
	if err != nil {
		t.Fatal(err)
	}
	f.history = st

	_, body := f.buildInternalPageHTML("goosie://settings?action=clear-data&history=1&historyScope=all")
	if !strings.Contains(body, "Clear selected browsing data?") {
		t.Fatalf("clear request did not ask for confirmation:\n%s", body)
	}
	if got := len(f.history.Entries()); got != 1 {
		t.Fatalf("history entries after unconfirmed clear = %d, want 1", got)
	}
}

func TestSettingsClearDataRemovesSelectedDataOnly(t *testing.T) {
	f, _ := newPrivacyTestPath(t)
	now := time.Now()
	writeHistoryFile(t, f.config.historyFile, []history.Entry{
		{URL: "https://old.example/", Title: "Old", At: now.Add(-2 * time.Hour)},
		{URL: "https://new.example/", Title: "New", At: now},
	})
	st, err := history.Load(f.config.historyFile)
	if err != nil {
		t.Fatal(err)
	}
	f.history = st

	cookieURL, _ := url.Parse("https://cookie.example/")
	f.client.Jar().Store(cookieURL, []string{"sid=abc; Path=/; Max-Age=3600"})
	if got := len(f.client.Jar().AllCookies()); got == 0 {
		t.Fatal("test cookie was not stored")
	}

	if err := os.WriteFile(f.config.sessionFile, []byte(`{"tabs":[{"url":"https://saved.example/"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.config.sessionActiveFile, []byte(`{"tabs":[{"url":"https://crash.example/"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f.setCrashedSession(&session.State{Tabs: []session.TabState{{URL: "https://crash.example/"}}})

	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Cache-Control", "public, max-age=60")
		_, _ = w.Write([]byte("cache payload"))
	}))
	defer srv.Close()
	for i := 0; i < 2; i++ {
		if _, err := f.client.Get(context.Background(), srv.URL); err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt64(&hits); got != 1 {
		t.Fatalf("cache priming hits = %d, want 1", got)
	}

	_, body := f.buildInternalPageHTML("goosie://settings?action=clear-data&confirm=1&history=1&historyScope=hour&cookies=1&cache=1&session=1")
	if !strings.Contains(body, "Cleared:") {
		t.Fatalf("clear did not report what was erased:\n%s", body)
	}

	remaining := f.history.Entries()
	if len(remaining) != 1 || remaining[0].URL != "https://old.example/" {
		t.Fatalf("history after last-hour clear = %+v, want only the 2h-old visit", remaining)
	}
	reloaded, err := history.Load(f.config.historyFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Entries(); len(got) != 1 || got[0].URL != "https://old.example/" {
		t.Fatalf("persisted history = %+v, want only the 2h-old visit", got)
	}

	if got := f.client.Jar().AllCookies(); len(got) != 0 {
		t.Fatalf("cookies after clear = %+v, want none", got)
	}
	if _, err := os.Stat(f.config.cookieFile); !os.IsNotExist(err) {
		t.Fatalf("cookie file still exists after clear: %v", err)
	}

	for _, path := range []string{f.config.sessionFile, f.config.sessionActiveFile} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("session file %s still exists after clear: %v", path, err)
		}
	}
	if !f.sessionDisabled.Load() {
		t.Fatal("clearing the session did not stop this run from saving it again")
	}
	if f.crashedSessionState() != nil {
		t.Fatal("crash journal survived clearing the session")
	}
	f.tabMgr.Active().SetURL("https://still-open.example/")
	f.saveActiveSession()
	if _, err := os.Stat(f.config.sessionActiveFile); !os.IsNotExist(err) {
		t.Fatalf("snapshot wrote %s after the session was cleared: %v", f.config.sessionActiveFile, err)
	}

	if _, err := f.client.Get(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt64(&hits); got != 2 {
		t.Fatalf("server hits after cache clear = %d, want 2", got)
	}
}

func TestPrivateSettingsDoNotOfferProfileClear(t *testing.T) {
	f, _ := newPrivacyTestPath(t)
	f.config.private = true
	if err := os.WriteFile(f.config.sessionFile, []byte(`{"tabs":[{"url":"https://keep.example/"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, body := f.buildInternalPageHTML("goosie://settings?action=clear-data&confirm=1&history=1&cookies=1&cache=1&session=1")
	if strings.Contains(body, `value="clear-data"`) {
		t.Fatalf("private settings offered to clear the profile:\n%s", body)
	}
	if !strings.Contains(body, "never write browsing data") {
		t.Fatalf("private settings did not explain why there is nothing to clear:\n%s", body)
	}
	if _, err := os.Stat(f.config.sessionFile); err != nil {
		t.Fatalf("private clear request touched the profile session file: %v", err)
	}
}
