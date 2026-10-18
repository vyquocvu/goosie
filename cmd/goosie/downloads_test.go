package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
	"github.com/vyquocvu/goosie/internal/tabs"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func TestDownloadsManagerIntegration(t *testing.T) {
	dlDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="package.zip"`)
		w.Header().Set("Content-Type", "application/zip")
		w.Write([]byte("mock zip content 1234567890"))
	}))
	defer server.Close()

	mgr := download.NewManager(dlDir)
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}

	client := net.DefaultClient()
	defer client.Close()
	_, _, _, _, _, err = loadURLCtx(context.Background(), client, fonts, server.URL+"/download", 800, 600, 1.0, dlDir, false, false, mgr)
	if err == nil || !strings.Contains(err.Error(), "downloaded to") {
		t.Fatalf("expected downloadDone error, got: %v", err)
	}

	items := mgr.Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 item in download manager, got %d", len(items))
	}
	it := items[0]
	if it.Filename != "package.zip" {
		t.Errorf("filename = %q, want package.zip", it.Filename)
	}
	if it.Status != download.StatusCompleted {
		t.Errorf("status = %v, want StatusCompleted", it.Status)
	}
	if _, statErr := os.Stat(it.LocalPath); statErr != nil {
		t.Errorf("downloaded file does not exist at %q: %v", it.LocalPath, statErr)
	}
}

func TestDownloadsPageRenderingAndActions(t *testing.T) {
	dlDir := t.TempDir()
	testFile := filepath.Join(dlDir, "document.pdf")
	if err := os.WriteFile(testFile, []byte("pdf data"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := newNavTestPath(t)
	f.config.downloadDir = dlDir
	f.downloads = download.NewManager(dlDir)

	// Add an active download and a completed download
	active := f.downloads.CreateItem("https://example.com/huge.iso", "huge.iso", 1024*1024*100)
	f.downloads.UpdateProgress(active.ID, 1024*1024*25) // 25%

	comp := f.downloads.CreateItem("https://example.com/app.dmg", "app.dmg", 5000)
	f.downloads.Complete(comp.ID, filepath.Join(dlDir, "app.dmg"))

	// 1. Initial page render
	title, htmlContent := f.buildInternalPageHTML("goosie://downloads")
	if title != "Downloads" {
		t.Errorf("title = %q, want Downloads", title)
	}
	if !strings.Contains(htmlContent, "huge.iso") || !strings.Contains(htmlContent, "25%") {
		t.Errorf("expected active download in HTML: %s", htmlContent)
	}
	if !strings.Contains(htmlContent, "app.dmg") || !strings.Contains(htmlContent, "Show in Finder") {
		t.Errorf("expected completed download with Finder action: %s", htmlContent)
	}
	if !strings.Contains(htmlContent, "document.pdf") {
		t.Errorf("expected disk file document.pdf in HTML: %s", htmlContent)
	}

	// 2. Action: Cancel active download
	_, _ = f.buildInternalPageHTML("goosie://downloads?action=cancel&id=" + active.ID)
	it := f.downloads.Get(active.ID)
	if it.Status != download.StatusCanceled {
		t.Errorf("status after cancel = %v, want StatusCanceled", it.Status)
	}

	// 3. Action: Clear finished downloads
	_, _ = f.buildInternalPageHTML("goosie://downloads?action=clear")
	if f.downloads.TotalCount() != 0 {
		t.Errorf("total count after clear = %d, want 0", f.downloads.TotalCount())
	}
}

func TestToolbarDownloadsClickNavigates(t *testing.T) {
	tabMgr := tabs.NewManager(nil)
	tab := tabMgr.NewTab()

	tb := toolbar.NewState(800, nil)
	var navigatedURL string
	tb.OnDownloads = func() {
		navigatedURL = "goosie://downloads"
	}

	// Find the download button rect
	btnRect := toolbar.ButtonRect(toolbar.ButtonDownloads, 800)
	midX := (btnRect.X0 + btnRect.X1) / 2
	midY := (btnRect.Y0 + btnRect.Y1) / 2

	tb.HandleClick(frame.Point{X: midX, Y: midY}, surface.ButtonLeft)
	if navigatedURL != "goosie://downloads" {
		t.Errorf("navigatedURL = %q, want goosie://downloads", navigatedURL)
	}
	_ = tab
}
