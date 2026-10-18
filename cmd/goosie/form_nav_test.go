package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/surface"
)

func TestFormNavigationOnSubmitClick(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<!DOCTYPE html><html><body><h1>Query: %s</h1></body></html>", r.URL.Query().Get("q"))
	}))
	defer server.Close()

	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	f.runCtx, f.runCancel = context.WithCancel(context.Background())
	defer f.runCancel()
	f.client = net.DefaultClient()
	defer f.client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	f.fonts = fonts

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<form id="searchform" action="%s/search" method="GET">
  <input type="text" name="q" value="goosie">
  <input type="submit" id="subbtn" value="Search">
</form>
</body></html>`, server.URL)

	sess, err := engine.NewSession(html, nil, 800, engine.WithMetrics(fonts))
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	tab := f.tabMgr.Active()
	tab.SetURL(server.URL + "/index.html")
	tab.SetTitle("Search Page")
	tab.SetSession(sess)

	// Find the submit button's center
	var subX, subY int32
	found := false
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "subbtn" {
			x0, y0, x1, y1 := obj.BorderRect()
			subX = int32((x0 + x1) / 2)
			subY = int32((y0 + y1) / 2)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("submit button not found in arena")
	}

	// Click the submit button via focusClick
	handled := f.focusClick(subX, subY)
	if !handled {
		t.Fatal("expected focusClick to handle submit button click")
	}

	// Wait for navResult from navigateTab
	select {
	case res := <-f.navResults:
		f.applyNavResult(res)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for navResult")
	}

	// Check that the tab was navigated with parameters
	currentURL := tab.URL()
	expectedURL := server.URL + "/search?q=goosie"
	if currentURL != expectedURL {
		t.Errorf("expected tab URL to be %q, got %q", expectedURL, currentURL)
	}
}

func TestFormNavigationOnEnterKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<!DOCTYPE html><html><body><h1>Query: %s</h1></body></html>", r.URL.Query().Get("q"))
	}))
	defer server.Close()

	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	f.runCtx, f.runCancel = context.WithCancel(context.Background())
	defer f.runCancel()
	f.client = net.DefaultClient()
	defer f.client.Close()
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	f.fonts = fonts

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><body>
<form id="searchform" action="%s/results" method="GET">
  <input type="text" id="queryinput" name="q" value="golang">
</form>
</body></html>`, server.URL)

	sess, err := engine.NewSession(html, nil, 800, engine.WithMetrics(fonts))
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	tab := f.tabMgr.Active()
	tab.SetURL(server.URL + "/")
	tab.SetSession(sess)

	// Focus the input
	var inpX, inpY int32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "queryinput" {
			x0, y0, x1, y1 := obj.BorderRect()
			inpX = int32((x0 + x1) / 2)
			inpY = int32((y0 + y1) / 2)
			break
		}
	}
	f.focusClick(inpX, inpY)

	// Press Enter key
	handled := f.contentKey('\r', surface.KeyMod(0))
	if !handled {
		t.Fatal("expected contentKey to handle Enter key")
	}

	// Wait for navResult
	select {
	case res := <-f.navResults:
		f.applyNavResult(res)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for navResult")
	}

	// Tab URL should be navigated
	currentURL := tab.URL()
	expectedURL := server.URL + "/results?q=golang"
	if currentURL != expectedURL {
		t.Errorf("expected tab URL to be %q, got %q", expectedURL, currentURL)
	}
}

func TestCheckboxClickToggle(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	html := `<!DOCTYPE html>
<html><body>
  <input type="checkbox" id="mychk">
</body></html>`

	sess, err := engine.NewSession(html, nil, 800)
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com/checkbox")
	tab.SetSession(sess)

	var chkNode *dom.Node
	var chkX, chkY int32
	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == "mychk" {
			chkNode = obj.Node
			x0, y0, x1, y1 := obj.BorderRect()
			chkX = int32((x0 + x1) / 2)
			chkY = int32((y0 + y1) / 2)
			break
		}
	}
	if chkNode == nil {
		t.Fatal("checkbox node not found")
	}

	if chkNode.HasAttribute("checked") {
		t.Fatal("checkbox should be initially unchecked")
	}

	// Click checkbox via focusClick
	if !f.focusClick(chkX, chkY) {
		t.Fatal("focusClick failed to handle checkbox")
	}
	if !chkNode.HasAttribute("checked") {
		t.Errorf("checkbox should be checked after click")
	}

	// Click checkbox again to toggle off
	if !f.focusClick(chkX, chkY) {
		t.Fatal("focusClick failed to handle second checkbox click")
	}
	if chkNode.HasAttribute("checked") {
		t.Errorf("checkbox should be unchecked after second click")
	}
}
