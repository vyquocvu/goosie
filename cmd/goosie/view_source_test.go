package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/surface"
)

func TestViewSource_Navigation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, `<!DOCTYPE html><html><head><title>Test Title</title></head><body><h1>Hello World</h1></body></html>`)
	}))
	defer ts.Close()

	f := newPDFTestPath(t)
	viewURL := "view-source:" + ts.URL + "/sample.html"

	layer, _, _, sess, finalURL, err := f.loadDocumentCtx(context.Background(), f.client, viewURL, 800, 600, 1.0, t.TempDir(), false, false)
	if err != nil {
		t.Fatalf("loadDocumentCtx failed: %v", err)
	}
	if layer == nil || sess == nil {
		t.Fatal("expected non-nil layer and session for view-source document")
	}
	defer sess.Close()

	if !strings.HasPrefix(finalURL, "view-source:") {
		t.Fatalf("expected finalURL to start with view-source:, got %q", finalURL)
	}

	// Verify the session painted without errors
	list, err := sess.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked failed: %v", err)
	}
	if list == nil {
		t.Fatal("expected non-nil display list")
	}
}

func TestViewSource_Shortcut(t *testing.T) {
	called := false
	cw := &chromeWindow{
		onViewSource: func() { called = true },
	}

	// Cmd+Option+U
	ev := surface.Event{
		Kind: surface.EvKey,
		Key:  'u',
		Mods: surface.ModCommand | surface.ModOption,
	}

	if !cw.handleTabShortcut(ev.Key, ev.Mods) {
		t.Fatal("expected Cmd+Option+U to be intercepted")
	}
	if !called {
		t.Fatal("expected onViewSource to be called on Cmd+Option+U")
	}

	// Uppercase 'U'
	called = false
	evUpper := surface.Event{
		Kind: surface.EvKey,
		Key:  'U',
		Mods: surface.ModCommand | surface.ModOption,
	}
	if !cw.handleTabShortcut(evUpper.Key, evUpper.Mods) {
		t.Fatal("expected Cmd+Option+Shift+U to be intercepted")
	}
	if !called {
		t.Fatal("expected onViewSource to be called on Cmd+Option+U uppercase")
	}
}

func TestViewSource_ContextMenuAndAction(t *testing.T) {
	items := BuildContextMenuItems("https://example.com/link", "", "", "", false, true, true)
	hasViewSource := false
	for _, item := range items {
		if item.Action == "view_source" && item.Label == "View Page Source" {
			hasViewSource = true
			break
		}
	}
	if !hasViewSource {
		t.Fatal("expected 'View Page Source' item in context menu")
	}

	f := newPDFTestPath(t)
	active := f.tabMgr.Active()
	active.SetURL("https://example.com/my-page")

	initialTabCount := f.tabMgr.Count()
	f.handleContextMenuAction("view_source", "")

	if f.tabMgr.Count() != initialTabCount+1 {
		t.Fatalf("expected tab count %d after view_source, got %d", initialTabCount+1, f.tabMgr.Count())
	}

	newActive := f.tabMgr.Active()
	newActive.Nav.Mu.Lock()
	isLoading := newActive.Nav.Loading
	serial := newActive.Nav.Serial
	newActive.Nav.Mu.Unlock()

	if !isLoading {
		t.Error("expected new tab to be in loading state for view-source navigation")
	}

	expectedURL := "view-source:https://example.com/my-page"
	f.applyNavResult(navResult{
		tabID:  newActive.ID(),
		serial: serial,
		url:    expectedURL,
	})

	if newActive.URL() != expectedURL {
		t.Errorf("newActive.URL = %q, want %q", newActive.URL(), expectedURL)
	}
}

func TestViewSource_NormalizeURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"view-source:example.com", "view-source:https://example.com"},
		{"view-source:https://news.ycombinator.com", "view-source:https://news.ycombinator.com"},
		{"VIEW-SOURCE:http://localhost:3000/app", "view-source:http://localhost:3000/app"},
	}

	for _, tc := range tests {
		got := normalizeURL(tc.input)
		if got != tc.want {
			t.Errorf("normalizeURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
