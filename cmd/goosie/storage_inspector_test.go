package main

import (
	"net/url"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/net"
)

func TestStorageRegistryInspection(t *testing.T) {
	reg := js.NewOriginStorageRegistry("", false)
	origin := "https://example.com:443"

	// Initially empty
	entries := reg.DumpLocalStorage(origin)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries initially, got %d", len(entries))
	}

	// Add items
	reg.SetLocalStorageItem(origin, "token", "abc123secret")
	reg.SetLocalStorageItem(origin, "theme", "dark")

	entries = reg.DumpLocalStorage(origin)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Key != "token" || entries[0].Value != "abc123secret" {
		t.Errorf("entry 0 mismatch: %+v", entries[0])
	}
	if entries[1].Key != "theme" || entries[1].Value != "dark" {
		t.Errorf("entry 1 mismatch: %+v", entries[1])
	}

	// Remove item
	reg.RemoveLocalStorageItem(origin, "token")
	entries = reg.DumpLocalStorage(origin)
	if len(entries) != 1 || entries[0].Key != "theme" {
		t.Fatalf("expected 1 entry (theme) after remove, got %+v", entries)
	}

	// Clear all
	reg.ClearLocalStorage(origin)
	entries = reg.DumpLocalStorage(origin)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", len(entries))
	}
}

func TestInspectPageStorageAndCookies(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com/app")
	sess, err := engine.NewSession("<html><body>Storage Inspector</body></html>", nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	tab.SetSession(sess)

	// Set up storage
	origin := js.OriginOf("https://example.com/app")
	storageReg := js.NewOriginStorageRegistry("", false)
	storageReg.SetLocalStorageItem(origin, "user_pref", "compact_mode")
	storageReg.SetLocalStorageItem(origin, "app_lang", "vi")
	f.storage = storageReg

	// Set up cookies
	netClient := net.DefaultClient()
	defer netClient.Close()
	u, _ := url.Parse("https://example.com/app")
	netClient.Jar().Store(u, []string{"session_token=secret999; Path=/; Secure; HttpOnly"})
	f.client = netClient

	// 1. Verify inspect page displays localStorage and cookies
	title, htmlContent := f.buildInternalPageHTML("goosie://inspect")
	if !strings.Contains(title, "Web Inspector") {
		t.Fatalf("expected title Web Inspector, got %q", title)
	}
	if !strings.Contains(htmlContent, "Web Storage (localStorage)") {
		t.Error("expected html to contain Web Storage card")
	}
	if !strings.Contains(htmlContent, "user_pref") || !strings.Contains(htmlContent, "compact_mode") {
		t.Error("expected html to contain user_pref storage key and value")
	}
	if !strings.Contains(htmlContent, "Cookies (Active Jar)") {
		t.Error("expected html to contain Cookies card")
	}
	if !strings.Contains(htmlContent, "session_token") {
		t.Error("expected html to contain session_token cookie")
	}
	if !strings.Contains(htmlContent, "HttpOnly") || !strings.Contains(htmlContent, "Secure") {
		t.Error("expected html to contain HttpOnly and Secure cookie badges")
	}

	// 2. Action: delete single localStorage item
	f.buildInternalPageHTML("goosie://inspect?action=storage_del&key=user_pref")
	if len(f.storage.DumpLocalStorage(origin)) != 1 {
		t.Fatalf("expected 1 item after delete user_pref, got %d", len(f.storage.DumpLocalStorage(origin)))
	}

	// 3. Action: clear all localStorage items
	f.buildInternalPageHTML("goosie://inspect?action=storage_clear")
	if len(f.storage.DumpLocalStorage(origin)) != 0 {
		t.Fatalf("expected 0 items after storage_clear, got %d", len(f.storage.DumpLocalStorage(origin)))
	}

	// 4. Action: clear cookies
	f.buildInternalPageHTML("goosie://inspect?action=cookies_clear")
	if len(f.client.Jar().AllCookies()) != 0 {
		t.Fatalf("expected 0 cookies after cookies_clear, got %d", len(f.client.Jar().AllCookies()))
	}
}
