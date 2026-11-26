package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

func newScriptNavigationTestPath(t *testing.T) *framePath {
	t.Helper()
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	f.setPreferences(preferences.Settings{
		SearchURLTemplate: preferences.Defaults().SearchURLTemplate,
		DownloadDir:       t.TempDir(),
		StartupMode:       preferences.StartupNewTab,
	})
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	f.fonts = fonts
	client := goosienet.DefaultClient()
	f.client = client
	t.Cleanup(func() { _ = client.Close() })
	tab := f.tabMgr.Active()
	_, layer := blankLayer(f.devSize())
	tab.SetLayer(layer)
	sess, err := engine.NewSession(`<!doctype html><html><body>old document</body></html>`, nil, float32(f.contentWidth()), engine.WithMetrics(fonts))
	if err != nil {
		t.Fatal(err)
	}
	tab.SetSession(sess)
	t.Cleanup(sess.Close)
	return f
}

// qc:BR-207
func TestReplaceNavigationChangesHistoryOnlyAfterSuccess(t *testing.T) {
	f := newScriptNavigationTestPath(t)
	tab := f.tabMgr.Active()
	tab.History().PushRequest("https://example.test/old", toolbar.Request{Method: "POST", Body: "a=1"})
	tab.History().Push("https://example.test/forward")
	if !tab.History().Step(-1, 1) {
		t.Fatal("failed to move to old history entry")
	}
	f.applyNavResult(navResult{
		tabID:          tab.ID(),
		serial:         tab.Nav.Serial,
		url:            "https://example.test/failed",
		replaceHistory: true,
		err:            context.Canceled,
	})
	entries, index := tab.History().Entries()
	if len(entries) != 2 || index != 0 || entries[0] != "https://example.test/old" || entries[1] != "https://example.test/forward" {
		t.Fatalf("failed replace history = %v at %d", entries, index)
	}
	f.applyNavResult(navResult{
		tabID:          tab.ID(),
		serial:         tab.Nav.Serial,
		url:            "https://example.test/new",
		replaceHistory: true,
	})
	entries, index = tab.History().Entries()
	if len(entries) != 1 || index != 0 || entries[0] != "https://example.test/new" {
		t.Fatalf("successful replace history = %v at %d", entries, index)
	}
	current, request, ok := tab.History().CurrentRequest()
	if !ok || current != "https://example.test/new" || request.IsPost() {
		t.Fatalf("replaced current request = %q, %+v, %t", current, request, ok)
	}
}

// qc:BR-207
func TestScriptNavigationUsesUntrustedInternalPageBoundary(t *testing.T) {
	f := newScriptNavigationTestPath(t)
	tab := f.tabMgr.Active()
	tab.SetURL("https://hostile.example/page")
	tab.History().Push(tab.URL())
	for _, request := range []js.NavigationRequest{
		{Kind: js.NavigationAssign, URL: "goosie://history?action=clear&confirm=1"},
		{Kind: js.NavigationReplace, URL: "view-source:goosie://bookmarks"},
	} {
		f.handleScriptNavigation(tab, request)
		result := receiveNavigationResult(t, f)
		if !errors.Is(result.err, errInternalNavigationFromContent) {
			t.Fatalf("%v navigation error = %v, want internal-page boundary error", request.Kind, result.err)
		}
		if request.Kind == js.NavigationReplace && !result.replaceHistory {
			t.Fatal("replace navigation lost its history mode")
		}
		f.applyNavResult(result)
		entries, _ := tab.History().Entries()
		if len(entries) != 1 || entries[0] != "https://hostile.example/page" {
			t.Fatalf("blocked script navigation changed history to %v", entries)
		}
	}
	tab.SetURL("goosie://version")
	tab.History().ReplaceCurrent("goosie://version")
	f.resubmitMu.Lock()
	if f.resubmits == nil {
		f.resubmits = make(map[uint64]*pendingResubmit)
	}
	f.resubmits[tab.ID()] = &pendingResubmit{}
	f.resubmitMu.Unlock()
	f.handleScriptNavigation(tab, js.NavigationRequest{Kind: js.NavigationAssign, URL: "goosie://resubmit?action=confirm"})
	result := receiveNavigationResult(t, f)
	if !errors.Is(result.err, errInternalNavigationFromContent) {
		t.Fatalf("script resubmit action error = %v, want untrusted boundary error", result.err)
	}
	f.handleScriptNavigation(tab, js.NavigationRequest{Kind: js.NavigationReload, URL: tab.URL()})
	result = receiveNavigationResult(t, f)
	if !errors.Is(result.err, errInternalNavigationFromContent) {
		t.Fatalf("script reload error = %v, want untrusted boundary error", result.err)
	}
}

// qc:BR-207
func TestSameDocumentURLRequiresFragmentDifference(t *testing.T) {
	tests := []struct {
		name    string
		current string
		target  string
		want    bool
	}{
		{"exact", "https://example.test/page", "https://example.test/page", false},
		{"exact with fragment", "https://example.test/page#one", "https://example.test/page#one", false},
		{"fragment added", "https://example.test/page", "https://example.test/page#one", true},
		{"fragment changed", "https://example.test/page#one", "https://example.test/page#two", true},
		{"query changed", "https://example.test/page?a=1", "https://example.test/page?a=2#one", false},
		{"path changed", "https://example.test/one", "https://example.test/two#one", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameDocumentURL(tt.current, tt.target); got != tt.want {
				t.Fatalf("sameDocumentURL(%q, %q) = %t, want %t", tt.current, tt.target, got, tt.want)
			}
		})
	}
}

// qc:BR-207
func TestNavigationSchemePolicyIsOriginAware(t *testing.T) {
	tests := []struct {
		name   string
		source string
		target string
		want   bool
	}{
		{"http to https", "http://example.test/page", "https://example.test/next", true},
		{"http fragment", "https://example.test/page", "#section", true},
		{"http to file", "https://example.test/page", "file:///etc/passwd", false},
		{"http to data", "https://example.test/page", "data:text/html,owned", false},
		{"http to javascript", "https://example.test/page", "javascript:alert(1)", false},
		{"http to blob", "https://example.test/page", "blob:https://example.test/id", false},
		{"file to file", "file:///tmp/page.html", "file:///tmp/next.html", true},
		{"file to https", "file:///tmp/page.html", "https://example.test/next", true},
		{"file to data", "file:///tmp/page.html", "data:text/html,owned", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := navigationSchemeAllowed(tt.source, tt.target); got != tt.want {
				t.Fatalf("navigationSchemeAllowed(%q, %q) = %t, want %t", tt.source, tt.target, got, tt.want)
			}
		})
	}
}

// qc:BR-207
func TestScriptNavigationBlocksDisallowedSchemesFromHTTPContent(t *testing.T) {
	f := newScriptNavigationTestPath(t)
	tab := f.tabMgr.Active()
	source := "https://hostile.example/page"
	tab.SetURL(source)
	tab.History().Push(source)
	for _, kind := range []js.NavigationKind{js.NavigationAssign, js.NavigationReplace} {
		for _, target := range []string{
			"file:///etc/passwd",
			"data:text/html,<h1>owned</h1>",
			"javascript:alert(1)",
			"blob:https://example.test/id",
			"mailto:someone@example.test",
			"ftp://example.test/file",
		} {
			f.handleScriptNavigation(tab, js.NavigationRequest{Kind: kind, URL: target})
			result := receiveNavigationResult(t, f)
			if !errors.Is(result.err, errNavigationSchemeFromContent) {
				t.Fatalf("%v navigation to %q error = %v, want scheme boundary error", kind, target, result.err)
			}
			if result.layer != nil || result.session != nil {
				t.Fatalf("blocked navigation to %q produced a document", target)
			}
			f.applyNavResult(result)
			if got := tab.URL(); got != source {
				t.Fatalf("blocked navigation to %q changed tab URL to %q", target, got)
			}
			entries, _ := tab.History().Entries()
			if len(entries) != 1 || entries[0] != source {
				t.Fatalf("blocked navigation to %q changed history to %v", target, entries)
			}
		}
	}
}

// qc:BR-207
func TestScriptNavigationFromFileOriginCanLoadFile(t *testing.T) {
	dir := t.TempDir()
	startPath := filepath.Join(dir, "start.html")
	nextPath := filepath.Join(dir, "next.html")
	if err := os.WriteFile(startPath, []byte(`<!doctype html><html><body>start</body></html>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nextPath, []byte(`<!doctype html><html><body>file target</body></html>`), 0o600); err != nil {
		t.Fatal(err)
	}
	startURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(startPath)}).String()
	nextURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(nextPath)}).String()
	f := newScriptNavigationTestPath(t)
	tab := f.tabMgr.Active()
	tab.SetURL(startURL)
	tab.History().Push(startURL)
	f.handleScriptNavigation(tab, js.NavigationRequest{Kind: js.NavigationAssign, URL: nextURL})
	result := receiveNavigationResult(t, f)
	if result.err != nil {
		t.Fatalf("file-origin navigation: %v", result.err)
	}
	if result.session == nil || result.session.Doc == nil || !strings.Contains(result.session.Doc.Node.TextContent(), "file target") {
		t.Fatal("file-origin navigation did not load the selected file document")
	}
	f.applyNavResult(result)
	if got := tab.URL(); got != nextURL {
		t.Fatalf("file-origin URL = %q, want %q", got, nextURL)
	}
}

// qc:BR-207
func TestScriptReloadUsesPOSTResubmissionPolicy(t *testing.T) {
	f := newScriptNavigationTestPath(t)
	tab := f.tabMgr.Active()
	target := "https://example.test/post"
	tab.SetURL(target)
	tab.History().PushRequest(target, toolbar.Request{Method: "POST", Body: "a=1"})
	f.handleScriptNavigation(tab, js.NavigationRequest{Kind: js.NavigationReload, URL: target})
	if !f.hasPendingResubmit(tab.ID()) {
		t.Fatal("script reload bypassed POST resubmission policy")
	}
	select {
	case result := <-f.navResults:
		t.Fatalf("script reload sent POST without confirmation: %+v", result)
	default:
	}
}

// qc:BR-207
func TestJourneyLocationNavigationThroughHeadlessShell(t *testing.T) {
	var mu sync.Mutex
	requests := map[string]int{}
	assignedStarted := make(chan struct{})
	assignedRelease := make(chan struct{})
	var assignedStartOnce, assignedReleaseOnce sync.Once
	defer assignedReleaseOnce.Do(func() { close(assignedRelease) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path]++
		count := requests[r.URL.Path]
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/start":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="start-doc">Start document</h1><button id="assign-button" onclick="setTimeout(function(){location.assign('/assigned')},0)">Assign</button></body></html>`)
		case "/assigned":
			assignedStartOnce.Do(func() { close(assignedStarted) })
			<-assignedRelease
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="assigned-doc">Assigned document</h1><button id="replace-button" onclick="setTimeout(function(){location.replace('/replaced')},0)">Replace</button></body></html>`)
		case "/replaced":
			fmt.Fprint(w, `<!doctype html><html><body><h1 id="replaced-doc">Replaced document</h1><button id="href-button" onclick="setTimeout(function(){location.href='/href'},0)">Href</button></body></html>`)
		case "/href":
			fmt.Fprintf(w, `<!doctype html><html><body><h1 id="href-doc">Render %d</h1><button id="reload-button" onclick="setTimeout(function(){location.reload()},0)">Reload</button></body></html>`, count)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)
	startURL := server.URL + "/start"
	assignedURL := server.URL + "/assigned"
	replacedURL := server.URL + "/replaced"
	hrefURL := server.URL + "/href"
	browser.navigateViaAddressBar(t, startURL)
	waitDocument := func(id, wantURL string) {
		browser.waitFor(t, id+" loaded at "+wantURL, func() bool {
			_, ok := findAXNode(browser.window.AccessibilityTree(), id)
			return ok && browser.activeURL() == wantURL && !navLoading(browser)
		})
		address, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
		if !ok || address.Value != wantURL {
			t.Fatalf("address bar = %q, %t; want %q", address.Value, ok, wantURL)
		}
	}
	waitDocument("start-doc", startURL)

	browser.clickNode(t, "assign-button")
	browser.waitFor(t, "assigned request is in flight", func() bool {
		select {
		case <-assignedStarted:
			return true
		default:
			return false
		}
	})
	browser.waitFor(t, "assign updates runtime href while old document remains", func() bool {
		tab := browser.f.tabMgr.Active()
		if tab == nil || tab.Session() == nil {
			return false
		}
		href, err := tab.Session().EvaluateJS(`location.href`)
		_, oldDocument := findAXNode(browser.window.AccessibilityTree(), "start-doc")
		return err == nil && href == assignedURL && oldDocument && browser.activeURL() == startURL
	})
	assignedReleaseOnce.Do(func() { close(assignedRelease) })
	waitDocument("assigned-doc", assignedURL)
	entries, index := browser.f.tabMgr.Active().History().Entries()
	if len(entries) != 2 || index != 1 || entries[0] != startURL || entries[1] != assignedURL {
		t.Fatalf("assign history = %v at %d", entries, index)
	}

	browser.clickToolbarButton(t, toolbar.ButtonBack)
	waitDocument("start-doc", startURL)
	browser.clickToolbarButton(t, toolbar.ButtonForward)
	waitDocument("assigned-doc", assignedURL)

	browser.clickNode(t, "replace-button")
	waitDocument("replaced-doc", replacedURL)
	entries, index = browser.f.tabMgr.Active().History().Entries()
	if len(entries) != 2 || index != 1 || entries[0] != startURL || entries[1] != replacedURL {
		t.Fatalf("replace history = %v at %d, want current entry replaced", entries, index)
	}
	browser.clickToolbarButton(t, toolbar.ButtonBack)
	waitDocument("start-doc", startURL)
	browser.clickToolbarButton(t, toolbar.ButtonForward)
	waitDocument("replaced-doc", replacedURL)

	browser.clickNode(t, "href-button")
	waitDocument("href-doc", hrefURL)
	entries, index = browser.f.tabMgr.Active().History().Entries()
	if len(entries) != 3 || index != 2 || entries[2] != hrefURL {
		t.Fatalf("history before href assignment = %v at %d", entries, index)
	}
	browser.clickToolbarButton(t, toolbar.ButtonBack)
	waitDocument("replaced-doc", replacedURL)
	browser.clickToolbarButton(t, toolbar.ButtonForward)
	waitDocument("href-doc", hrefURL)

	mu.Lock()
	beforeReload := requests["/href"]
	mu.Unlock()
	browser.clickNode(t, "reload-button")
	browser.waitFor(t, "script reload renders exactly one new response", func() bool {
		mu.Lock()
		count := requests["/href"]
		mu.Unlock()
		return count == beforeReload+1 && axContainsLabel(browser.window.AccessibilityTree(), fmt.Sprintf("Render %d", beforeReload+1)) && !navLoading(browser)
	})
	mu.Lock()
	afterReload := requests["/href"]
	mu.Unlock()
	if afterReload != beforeReload+1 {
		t.Fatalf("reload requests = %d, want exactly one more than %d", afterReload, beforeReload)
	}
	entries, index = browser.f.tabMgr.Active().History().Entries()
	if len(entries) != 3 || index != 2 || entries[2] != hrefURL {
		t.Fatalf("reload changed history to %v at %d", entries, index)
	}
}

// qc:BR-207
func TestJourneyLocationHashAndExactURLNavigationThroughHeadlessShell(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		count := requests
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html><body style="margin:0;padding:0">
			<h1 id="hash-doc">Render %d</h1>
			<h2 id="hash-state">none|0</h2>
			<button id="assign-hash" onclick="setTimeout(function(){location.assign('#first')},0)">Assign hash</button>
			<button id="replace-hash" onclick="setTimeout(function(){location.replace('#second')},0)">Replace hash</button>
			<button id="assign-exact" onclick="setTimeout(function(){location.assign(location.href)},0)">Assign exact</button>
			<button id="replace-exact" onclick="setTimeout(function(){location.replace(location.href)},0)">Replace exact</button>
			<div style="height:1200px">Spacer</div>
			<h2 id="first">First target</h2>
			<div style="height:1200px">Spacer</div>
			<h2 id="second">Second target</h2>
			<script>
				window.hashChangeCount = 0;
				window.addEventListener("hashchange", function(event) {
					window.hashChangeCount++;
					document.getElementById("hash-state").textContent = event.oldURL + "|" + event.newURL + "|" + String(window.hashChangeCount);
				});
			</script>
		</body></html>`, count)
	}))
	defer server.Close()

	browser := newBrowserE2E(t)
	defer browser.close(t)
	pageURL := server.URL + "/hash-page"
	firstURL := pageURL + "#first"
	secondURL := pageURL + "#second"
	browser.navigateViaAddressBar(t, pageURL)
	waitRender := func(wantURL string, wantRender int) {
		browser.waitFor(t, fmt.Sprintf("hash page render %d at %s", wantRender, wantURL), func() bool {
			_, ok := findAXNode(browser.window.AccessibilityTree(), "hash-doc")
			return ok && axContainsLabel(browser.window.AccessibilityTree(), fmt.Sprintf("Render %d", wantRender)) && browser.activeURL() == wantURL && !navLoading(browser)
		})
		address, ok := findAXNode(browser.window.AccessibilityTree(), "goosie.address")
		if !ok || address.Value != wantURL {
			t.Fatalf("address bar = %q, %t; want %q", address.Value, ok, wantURL)
		}
	}
	requestCount := func() int {
		mu.Lock()
		defer mu.Unlock()
		return requests
	}
	waitRender(pageURL, 1)

	browser.clickNode(t, "assign-hash")
	browser.waitFor(t, "hash-only assign updates URL, event, count, and scroll without reload", func() bool {
		tab := browser.f.tabMgr.Active()
		if tab == nil {
			return false
		}
		state, stateOK := findAXNode(browser.window.AccessibilityTree(), "hash-state")
		return browser.activeURL() == firstURL && stateOK && state.Label == pageURL+"|"+firstURL+"|1" && tab.ScrollY() > 0 && requestCount() == 1
	})
	entries, index := browser.f.tabMgr.Active().History().Entries()
	if len(entries) != 2 || index != 1 || entries[0] != pageURL || entries[1] != firstURL {
		t.Fatalf("hash-only assign history = %v at %d", entries, index)
	}

	browser.clickToolbarButton(t, toolbar.ButtonBack)
	waitRender(pageURL, 2)
	if !browser.f.tabMgr.Active().History().CanForward() {
		t.Fatal("hash-only assign did not leave a forward entry")
	}
	browser.clickNode(t, "replace-hash")
	browser.waitFor(t, "hash-only replace updates URL, event, count, and scroll without reload", func() bool {
		tab := browser.f.tabMgr.Active()
		if tab == nil {
			return false
		}
		state, stateOK := findAXNode(browser.window.AccessibilityTree(), "hash-state")
		return browser.activeURL() == secondURL && stateOK && state.Label == pageURL+"|"+secondURL+"|1" && tab.ScrollY() > 0 && requestCount() == 2
	})
	entries, index = browser.f.tabMgr.Active().History().Entries()
	if len(entries) != 1 || index != 0 || entries[0] != secondURL || browser.f.tabMgr.Active().History().CanForward() {
		t.Fatalf("hash-only replace history = %v at %d, want forward history truncated", entries, index)
	}

	if _, err := browser.f.tabMgr.Active().Session().EvaluateJS(`document.getElementById("assign-exact").click()`); err != nil {
		t.Fatal(err)
	}
	waitRender(secondURL, 3)
	if got := requestCount(); got != 3 {
		t.Fatalf("exact assign requests = %d, want reload request 3", got)
	}
	entriesBeforeReplace, _ := browser.f.tabMgr.Active().History().Entries()
	if _, err := browser.f.tabMgr.Active().Session().EvaluateJS(`document.getElementById("replace-exact").click()`); err != nil {
		t.Fatal(err)
	}
	waitRender(secondURL, 4)
	if got := requestCount(); got != 4 {
		t.Fatalf("exact replace requests = %d, want reload request 4", got)
	}
	entriesAfterReplace, indexAfterReplace := browser.f.tabMgr.Active().History().Entries()
	if len(entriesAfterReplace) != len(entriesBeforeReplace) || indexAfterReplace != len(entriesAfterReplace)-1 || entriesAfterReplace[indexAfterReplace] != secondURL {
		t.Fatalf("exact replace history = %v at %d, want current entry only", entriesAfterReplace, indexAfterReplace)
	}
}

// qc:BR-207
func TestScriptAssignPushesHistoryAfterSuccessfulLoad(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><body>next</body></html>`)
	}))
	defer server.Close()
	f := newScriptNavigationTestPath(t)
	tab := f.tabMgr.Active()
	startURL := server.URL + "/start"
	nextURL := server.URL + "/next"
	tab.SetURL(startURL)
	tab.History().Push(startURL)
	f.handleScriptNavigation(tab, js.NavigationRequest{Kind: js.NavigationAssign, URL: nextURL})
	result := receiveNavigationResult(t, f)
	if result.err != nil {
		t.Fatalf("assign navigation: %v", result.err)
	}
	if result.replaceHistory {
		t.Fatal("assign navigation requested replace history")
	}
	entries, _ := tab.History().Entries()
	if len(entries) != 1 {
		t.Fatalf("assign changed history before load result: %v", entries)
	}
	f.applyNavResult(result)
	entries, index := tab.History().Entries()
	if len(entries) != 2 || index != 1 || entries[1] != nextURL {
		t.Fatalf("assign history after load = %v at %d", entries, index)
	}
}
