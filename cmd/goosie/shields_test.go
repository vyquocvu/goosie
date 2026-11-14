package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestTabShieldsToggle(t *testing.T) {
	mgr := tabs.NewManager(nil)
	tab := mgr.NewTab()

	if !tab.ShieldsEnabled() {
		t.Fatal("expected ShieldsEnabled() = true by default")
	}
	if tab.BlockedTrackers() != 0 {
		t.Fatalf("expected 0 blocked trackers initially, got %d", tab.BlockedTrackers())
	}

	tab.IncrementBlockedTrackers()
	tab.IncrementBlockedTrackers()
	if tab.BlockedTrackers() != 2 {
		t.Fatalf("expected 2 blocked trackers, got %d", tab.BlockedTrackers())
	}

	tab.ResetBlockedTrackers()
	if tab.BlockedTrackers() != 0 {
		t.Fatalf("expected 0 blocked trackers after reset, got %d", tab.BlockedTrackers())
	}

	tab.SetShieldsEnabled(false)
	if tab.ShieldsEnabled() {
		t.Fatal("expected ShieldsEnabled() = false after SetShieldsEnabled(false)")
	}

	tab.SetShieldsEnabled(true)
	if !tab.ShieldsEnabled() {
		t.Fatal("expected ShieldsEnabled() = true after SetShieldsEnabled(true)")
	}
}

func TestInspectPageShieldsToggle(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com/page")
	sess, err := engine.NewSession("<html><body>Shields Test</body></html>", nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	tab.SetSession(sess)
	tab.IncrementBlockedTrackers()
	tab.IncrementBlockedTrackers()
	tab.IncrementBlockedTrackers()

	_, htmlContent := f.buildInternalPageHTML("goosie://inspect")
	if !strings.Contains(htmlContent, "Protection Active") {
		t.Error("expected Protection Active badge")
	}
	if !strings.Contains(htmlContent, "Blocked Trackers / Ads: <b>3</b>") {
		t.Error("expected 3 blocked trackers in html")
	}
	if !strings.Contains(htmlContent, "Disable for this tab") {
		t.Error("expected Disable for this tab button")
	}

	f.buildInternalPageHTML("goosie://inspect?action=toggle_shields")
	if tab.ShieldsEnabled() {
		t.Fatal("expected tab shields to be disabled after toggle_shields")
	}
	_, htmlDisabled := f.buildInternalPageHTML("goosie://inspect")
	if !strings.Contains(htmlDisabled, "Protection Disabled") {
		t.Error("expected Protection Disabled badge")
	}
	if !strings.Contains(htmlDisabled, "Enable Protection") {
		t.Error("expected Enable Protection button")
	}

	f.buildInternalPageHTML("goosie://inspect?action=toggle_shields")
	if !tab.ShieldsEnabled() {
		t.Fatal("expected tab shields to be enabled after second toggle_shields")
	}
}

func TestShieldsDisabledBypassesFilter(t *testing.T) {
	client := net.DefaultClient()
	defer client.Close()

	ctx := context.Background()
	_, err := client.Get(ctx, "https://google-analytics.com/analytics.js")
	if !errors.Is(err, net.ErrBlockedByFilter) {
		t.Fatalf("expected ErrBlockedByFilter when shields are enabled, got: %v", err)
	}

	disabledCtx := net.WithShieldsDisabled(ctx, true)
	_, err = client.Get(disabledCtx, "https://google-analytics.com/analytics.js")
	if errors.Is(err, net.ErrBlockedByFilter) {
		t.Fatalf("did not expect ErrBlockedByFilter when shields are disabled: %v", err)
	}
}
