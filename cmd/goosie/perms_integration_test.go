package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/perms"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/tabs"
)

func TestSitePermissions_JSRuntimeBlocked(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Original Title</title></head><body><h1 id="headline">Original</h1><script>document.title = "Executed Title"; document.getElementById("headline").textContent = "Executed";</script></body></html>`)
	}))
	defer ts.Close()

	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	mgr := perms.NewManager(filepath.Join(tmpDir, "permissions.json"))

	// 1. Load with permissions allowed by default: script executes.
	_, _, _, sessAllowed, _, err := loadURLCtx(context.Background(), client, fonts, ts.URL, 800, 600, 1.0, tmpDir, false, false, mgr)
	if err != nil {
		t.Fatalf("loadURLCtx failed: %v", err)
	}
	defer sessAllowed.Close()

	if got := sessAllowed.JSTitle(); got != "Executed Title" {
		t.Errorf("expected JSTitle to be 'Executed Title', got %q", got)
	}
	elemAllowed := sessAllowed.Doc.ElementByID("headline")
	if elemAllowed == nil || elemAllowed.FirstChild == nil || elemAllowed.FirstChild.DataContent != "Executed" {
		t.Errorf("expected script to execute and set headline to 'Executed', got %+v", elemAllowed)
	}

	// 2. Block JavaScript for this origin: script does NOT execute.
	mgr.Set(ts.URL, perms.PermissionJavaScript, perms.StateBlock)

	_, _, _, sessBlocked, _, err := loadURLCtx(context.Background(), client, fonts, ts.URL, 800, 600, 1.0, tmpDir, false, false, mgr)
	if err != nil {
		t.Fatalf("loadURLCtx with blocked JS failed: %v", err)
	}
	defer sessBlocked.Close()

	if got := sessBlocked.JSTitle(); got != "" {
		t.Errorf("expected JSTitle to be empty when JS blocked, got %q", got)
	}
	elemBlocked := sessBlocked.Doc.ElementByID("headline")
	if elemBlocked == nil || elemBlocked.FirstChild == nil || elemBlocked.FirstChild.DataContent != "Original" {
		t.Errorf("expected script to be blocked and headline to remain 'Original', got %+v", elemBlocked)
	}
}

func TestSitePermissions_ImagesBlocked(t *testing.T) {
	imageFetches := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".png") {
			imageFetches++
			w.Header().Set("Content-Type", "image/png")
			// 1x1 transparent PNG bytes
			pngBytes := []byte{
				0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
				0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
				0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
				0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
				0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
				0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
			}
			w.Write(pngBytes)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><body><h1>Photo</h1><img src="/photo.png"></body></html>`)
	}))
	defer ts.Close()

	client := net.DefaultClient()
	defer client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	mgr := perms.NewManager(filepath.Join(tmpDir, "permissions.json"))

	// Block images for this origin
	mgr.Set(ts.URL, perms.PermissionImages, perms.StateBlock)

	_, _, _, sess, _, err := loadURLCtx(context.Background(), client, fonts, ts.URL, 800, 600, 1.0, tmpDir, false, false, mgr)
	if err != nil {
		t.Fatalf("loadURLCtx with blocked images failed: %v", err)
	}
	defer sess.Close()

	if imageFetches != 0 {
		t.Errorf("expected 0 image fetches when blocked, got %d", imageFetches)
	}
}

func TestSitePermissions_ClipboardReadStatusAndDenialReachRuntime(t *testing.T) {
	const pageURL = "https://clipboard-permission.example.test/"
	client := &recordingClient{docs: map[string]string{
		pageURL: `<!doctype html><html><body><button id="read">Read clipboard</button><button id="write">Write clipboard</button><script>
			document.getElementById('read').addEventListener('click', function() {
				navigator.permissions.query({name: 'clipboard-read'}).then(function(status) {
					document.body.setAttribute('data-permission', status.state);
				});
				navigator.clipboard.readText().catch(function(error) {
					document.body.setAttribute('data-error', error.name);
				});
			});
			document.getElementById('write').addEventListener('click', function() {
				navigator.permissions.query({name: 'clipboard-write'}).then(function(status) {
					document.body.setAttribute('data-write-permission', status.state);
				});
				navigator.clipboard.writeText('overwrite').catch(function(error) {
					document.body.setAttribute('data-write-error', error.name);
				});
			});
		</script></body></html>`,
	}}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatalf("create fonts: %v", err)
	}
	mgr := perms.NewManager(filepath.Join(t.TempDir(), "permissions.json"))
	mgr.Set(pageURL, perms.PermissionClipboardRead, perms.StateBlock)
	mgr.Set(pageURL, perms.PermissionClipboardWrite, perms.StateBlock)
	clipboard := &testClipboardProvider{text: "private"}
	_, _, _, sess, _, err := loadURLCtx(context.Background(), client, fonts, pageURL, 800, 600, 1, t.TempDir(), false, false, mgr, clipboard)
	if err != nil {
		t.Fatalf("loadURLCtx: %v", err)
	}
	defer sess.Close()
	var clickX, clickY float32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "read" {
			x0, y0, x1, y1 := obj.BorderRect()
			clickX, clickY = (x0+x1)/2, (y0+y1)/2
			break
		}
	}
	if !sess.ClickAt(clickX, clickY) {
		t.Fatal("clipboard permission button click was not handled")
	}
	sess.PumpJS(800)
	got, err := sess.EvaluateJS(`document.body.getAttribute('data-permission') + '|' + document.body.getAttribute('data-error')`)
	if err != nil {
		t.Fatalf("read clipboard permission result: %v", err)
	}
	if got != "denied|NotAllowedError" {
		t.Fatalf("permission query/access result = %q, want denied|NotAllowedError", got)
	}
	var writeX, writeY float32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "write" {
			x0, y0, x1, y1 := obj.BorderRect()
			writeX, writeY = (x0+x1)/2, (y0+y1)/2
			break
		}
	}
	if !sess.ClickAt(writeX, writeY) {
		t.Fatal("clipboard write permission button click was not handled")
	}
	sess.PumpJS(800)
	got, err = sess.EvaluateJS(`document.body.getAttribute('data-write-permission') + '|' + document.body.getAttribute('data-write-error')`)
	if err != nil {
		t.Fatalf("read clipboard write permission result: %v", err)
	}
	if got != "denied|NotAllowedError" {
		t.Fatalf("write permission/access result = %q, want denied|NotAllowedError", got)
	}
	if clipboard.text != "private" {
		t.Fatalf("blocked read changed clipboard text to %q", clipboard.text)
	}
}

func TestSitePermissions_InspectPageActions(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := perms.NewManager(filepath.Join(tmpDir, "permissions.json"))

	tabMgr := tabs.NewManager(nil)
	tab := tabMgr.NewTab()
	tab.SetURL("https://example.com/blog")
	tab.SetTitle("Example Blog")

	sess, err := engine.NewSession("<html><body><h1>Hello World</h1></body></html>", nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()
	tab.SetSession(sess)

	f := &framePath{
		perms:  mgr,
		tabMgr: tabMgr,
	}

	// 1. Initial inspect page rendering: permissions are default (Allowed)
	_, initialHTML := buildInspectPageHTML(f, fmt.Sprintf("goosie://inspect?tab=%d", tab.ID()))
	if !strings.Contains(initialHTML, "Site Permissions &amp; Content Settings") {
		t.Errorf("expected Site Permissions card in inspect page")
	}
	if !strings.Contains(initialHTML, "https://example.com") {
		t.Errorf("expected normalized origin in inspect page")
	}
	if !strings.Contains(initialHTML, "Allowed (Default)") {
		t.Errorf("expected default allowed state in inspect page")
	}
	if !strings.Contains(initialHTML, "Clipboard Read") || !strings.Contains(initialHTML, "Clipboard Write") || !strings.Contains(initialHTML, "Prompt on user activation (Default)") {
		t.Errorf("expected clipboard permission controls with prompt defaults")
	}

	// 2. Block JavaScript via perm_set action
	actionURL := fmt.Sprintf("goosie://inspect?action=perm_set&origin=https://example.com&perm=javascript&state=block&tab=%d", tab.ID())
	buildInspectPageHTML(f, actionURL)

	if mgr.State("https://example.com", perms.PermissionJavaScript) != perms.StateBlock {
		t.Errorf("expected javascript permission to be StateBlock")
	}

	// 3. Re-render inspect page: should reflect Blocked state
	_, blockedHTML := buildInspectPageHTML(f, fmt.Sprintf("goosie://inspect?tab=%d", tab.ID()))
	if !strings.Contains(blockedHTML, "Blocked") {
		t.Errorf("expected 'Blocked' label in inspect page")
	}

	// 4. Reset to default via perm_set action
	resetURL := fmt.Sprintf("goosie://inspect?action=perm_set&origin=https://example.com&perm=javascript&state=default&tab=%d", tab.ID())
	buildInspectPageHTML(f, resetURL)

	if mgr.State("https://example.com", perms.PermissionJavaScript) != perms.StateDefault {
		t.Errorf("expected javascript permission to be StateDefault after reset")
	}

	clipboardURL := fmt.Sprintf("goosie://inspect?action=perm_set&origin=https://example.com&perm=clipboard-read&state=allow&tab=%d", tab.ID())
	buildInspectPageHTML(f, clipboardURL)
	if mgr.State("https://example.com", perms.PermissionClipboardRead) != perms.StateAllow {
		t.Errorf("expected clipboard-read permission to be StateAllow")
	}
	reloaded := perms.NewManager(filepath.Join(tmpDir, "permissions.json"))
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload site permission settings: %v", err)
	}
	if got := reloaded.State("https://example.com", perms.PermissionClipboardRead); got != perms.StateAllow {
		t.Errorf("persisted clipboard-read permission = %q, want allow", got)
	}
	_, clipboardAllowedHTML := buildInspectPageHTML(f, fmt.Sprintf("goosie://inspect?tab=%d", tab.ID()))
	if !strings.Contains(clipboardAllowedHTML, "Allowed (Explicit)") {
		t.Errorf("expected explicit clipboard permission state in inspector")
	}
}
