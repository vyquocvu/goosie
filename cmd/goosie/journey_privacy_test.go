package main

import (
	"net/url"
	"testing"

	gonet "github.com/vyquocvu/goosie/internal/net"
)

// qc:BR-058
func TestJourneyClearCookiesForOneSite(t *testing.T) {
	browser := newBrowserE2E(t)
	jar := browser.f.client.Jar()
	for _, raw := range []string{"https://example.com/", "https://other.org/"} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		jar.Store(u, []string{"sid=abc; Path=/; Max-Age=3600"})
	}

	browser.navigateViaAddressBar(t, "goosie://settings")
	browser.waitFor(t, "settings lists both cookie sites", func() bool {
		_, example := findAXNode(browser.window.AccessibilityTree(), "clear-site-example.com")
		_, other := findAXNode(browser.window.AccessibilityTree(), "clear-site-other.org")
		return browser.activeURL() == "goosie://settings" && example && other
	})

	browser.clickNode(t, "clear-site-example.com")
	browser.waitFor(t, "per-site confirmation is shown", func() bool {
		_, ok := findAXNode(browser.window.AccessibilityTree(), "confirm-clear-site-cookies")
		return ok
	})
	if got := len(jar.AllCookies()); got != 2 {
		t.Fatalf("cookies after opening the confirmation = %d, want 2 (nothing removed yet)", got)
	}

	browser.clickNode(t, "confirm-clear-site-cookies")
	browser.waitFor(t, "only other.org remains in memory and in the list", func() bool {
		cookies := jar.AllCookies()
		if len(cookies) != 1 || cookies[0].Domain != "other.org" {
			return false
		}
		_, example := findAXNode(browser.window.AccessibilityTree(), "clear-site-example.com")
		_, other := findAXNode(browser.window.AccessibilityTree(), "clear-site-other.org")
		return !example && other
	})

	reloaded := gonet.NewCookieJar()
	if err := reloaded.Load(browser.f.config.cookieFile); err != nil {
		t.Fatal(err)
	}
	persisted := reloaded.AllCookies()
	if len(persisted) != 1 || persisted[0].Domain != "other.org" {
		t.Fatalf("persisted cookies = %+v, want only other.org", persisted)
	}
}
