package main

import (
	"bytes"
	"context"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/bookmarks"
	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/history"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/perms"
	"github.com/vyquocvu/goosie/internal/preferences"
	"github.com/vyquocvu/goosie/internal/raster"
)

type navigationFileLauncher struct {
	opened   []string
	revealed []string
}

func (l *navigationFileLauncher) OpenFile(path string) error {
	l.opened = append(l.opened, path)
	return nil
}

func (l *navigationFileLauncher) ShowInFolder(path string) error {
	l.revealed = append(l.revealed, path)
	return nil
}

func navigationTrustTestPath(t *testing.T) (*framePath, string, *navigationFileLauncher) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "downloads")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	f.setPreferences(preferences.Settings{
		SearchURLTemplate: preferences.Defaults().SearchURLTemplate,
		DownloadDir:       dir,
		StartupMode:       preferences.StartupNewTab,
	})
	launcher := &navigationFileLauncher{}
	f.downloads = download.NewManager(dir)
	f.downloads.SetLauncher(launcher)
	return f, root, launcher
}

func receiveNavigationResult(t *testing.T, f *framePath) navResult {
	t.Helper()
	return <-f.navResults
}

func requireBlockedInternalNavigation(t *testing.T, res navResult) {
	t.Helper()
	if !errors.Is(res.err, errInternalNavigationFromContent) {
		t.Fatalf("navigation error = %v, want %v", res.err, errInternalNavigationFromContent)
	}
	if res.layer != nil || res.session != nil {
		t.Fatal("blocked navigation produced a document")
	}
}

func TestHostileWebLinksCannotNavigateInternalPages(t *testing.T) {
	f, root, launcher := navigationTrustTestPath(t)
	bookmarkPath := filepath.Join(root, "bookmarks.json")
	bookmarkStore, err := bookmarks.Load(bookmarkPath)
	if err != nil {
		t.Fatal(err)
	}
	bookmarkStore.Toggle("https://saved.example/article", "Saved")
	if err := bookmarkStore.Save(); err != nil {
		t.Fatal(err)
	}
	f.bookmarks = bookmarkStore

	historyPath := filepath.Join(root, "history.json")
	historyStore, err := history.Load(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	historyStore.Record("https://visited.example/article", "Visited")
	if err := historyStore.Save(); err != nil {
		t.Fatal(err)
	}
	f.history = historyStore

	preferencesPath := filepath.Join(root, "preferences.json")
	f.config.preferencesFile = preferencesPath
	if err := preferences.Save(preferencesPath, f.currentPreferences()); err != nil {
		t.Fatal(err)
	}
	permissionsPath := filepath.Join(root, "permissions.json")
	f.perms = perms.NewManager(permissionsPath)

	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	bookmarkBytes, err := os.ReadFile(bookmarkPath)
	if err != nil {
		t.Fatal(err)
	}
	historyBytes, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	preferencesBytes, err := os.ReadFile(preferencesPath)
	if err != nil {
		t.Fatal(err)
	}
	tab := f.tabMgr.Active()
	tabID := tab.ID()
	settingsURL := "goosie://settings?save=1&searchURLTemplate=" + url.QueryEscape("https://changed.example/?q={query}") + "&downloadDir=" + url.QueryEscape(f.downloadDirectory()) + "&startupMode=newtab"
	inspectURL := "goosie://inspect?action=perm_set&origin=" + url.QueryEscape("https://victim.example") + "&perm=javascript&state=allow&tab=" + strconv.FormatUint(tabID, 10)

	for _, href := range []string{
		"goosie://bookmarks?action=delete&url=" + url.QueryEscape("https://saved.example/article"),
		"GOOSIE://HISTORY?action=clear&confirm=1",
		"about:history?action=clear&confirm=1",
		"view-source:goosie://history?action=clear&confirm=1",
		"VIEW-SOURCE:GOOSIE://HISTORY?action=clear&confirm=1",
		"view-source:view-source:goosie://history?action=clear&confirm=1",
		"goosie://resubmit?action=confirm",
		settingsURL,
		inspectURL,
		"goosie://downloads?action=open&path=" + url.QueryEscape(outside),
		"goosie://downloads?action=reveal&path=" + url.QueryEscape(outside),
	} {
		tab.SetURL("https://hostile.example/page")
		f.handleLinkClick(href)
		res := receiveNavigationResult(t, f)
		requireBlockedInternalNavigation(t, res)
		f.applyNavResult(res)
		if got := tab.Error(); !strings.Contains(got, "web content cannot open internal browser pages") {
			t.Fatalf("tab error = %q", got)
		}
		if got := tab.URL(); got != "https://hostile.example/page" {
			t.Fatalf("blocked navigation changed tab URL to %q", got)
		}
		if len(bookmarkStore.List()) != 1 || len(historyStore.Entries()) != 1 {
			t.Fatal("blocked link mutated profile data")
		}
		if f.currentPreferences().SearchURLTemplate != preferences.Defaults().SearchURLTemplate {
			t.Fatal("blocked link changed preferences")
		}
		if f.perms.State("https://victim.example", perms.PermissionJavaScript) != perms.StateDefault {
			t.Fatal("blocked link changed permissions")
		}
		if len(launcher.opened) != 0 || len(launcher.revealed) != 0 {
			t.Fatalf("blocked link reached file launcher: %+v", launcher)
		}
	}
	for path, want := range map[string][]byte{
		bookmarkPath:    bookmarkBytes,
		historyPath:     historyBytes,
		preferencesPath: preferencesBytes,
	} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("blocked link changed %s: %v", path, err)
		}
	}
	if _, err := os.Stat(permissionsPath); !os.IsNotExist(err) {
		t.Fatalf("blocked link wrote permissions: %v", err)
	}
}

func TestHostileWebFormsCannotNavigateInternalPages(t *testing.T) {
	f, root, _ := navigationTrustTestPath(t)
	historyStore, err := history.Load(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	historyStore.Record("https://visited.example/article", "Visited")
	if err := historyStore.Save(); err != nil {
		t.Fatal(err)
	}
	f.history = historyStore
	tab := f.tabMgr.Active()
	tab.SetURL("https://hostile.example/form")

	for _, method := range []string{"GET", "POST"} {
		f.handleFormSubmission(tab, &engine.FormSubmission{
			Action:  "goosie://history?action=clear&confirm=1",
			Method:  method,
			Encoded: "source=hostile",
		})
		res := receiveNavigationResult(t, f)
		requireBlockedInternalNavigation(t, res)
		f.applyNavResult(res)
		if len(historyStore.Entries()) != 1 {
			t.Fatalf("hostile %s form cleared history", method)
		}
		if got := tab.URL(); got != "https://hostile.example/form" {
			t.Fatalf("hostile %s form changed tab URL to %q", method, got)
		}
	}

	f.handleFormSubmission(tab, &engine.FormSubmission{
		Action: "view-source:goosie://history?action=clear&confirm=1",
		Method: "GET",
	})
	res := receiveNavigationResult(t, f)
	requireBlockedInternalNavigation(t, res)
	if len(historyStore.Entries()) != 1 {
		t.Fatal("hostile view-source form cleared history")
	}
}

func TestEmbeddedContentCannotNavigateInternalPages(t *testing.T) {
	f, root, _ := navigationTrustTestPath(t)
	historyStore, err := history.Load(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	historyStore.Record("https://visited.example/article", "Visited")
	f.history = historyStore
	tab := f.tabMgr.Active()

	for _, sourceURL := range []string{
		"goosie://reader?url=https%3A%2F%2Fhostile.example%2Farticle",
		"view-source:goosie://bookmarks",
	} {
		tab.SetURL(sourceURL)
		f.handleLinkClick("goosie://history?action=clear&confirm=1")
		res := receiveNavigationResult(t, f)
		requireBlockedInternalNavigation(t, res)
		if len(historyStore.Entries()) != 1 {
			t.Fatalf("content from %s cleared history", sourceURL)
		}
	}
}

func TestNavigationTrustContextBoundary(t *testing.T) {
	f, root, _ := navigationTrustTestPath(t)
	historyStore, err := history.Load(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	f.history = historyStore
	target := "goosie://history?action=clear&confirm=1"
	historyStore.Record("https://visited.example/article", "Visited")

	_, _, _, sess, _, err := f.loadDocumentCtx(context.Background(), nil, target, 800, 600, 1, f.downloadDirectory(), true, true)
	if err != nil {
		t.Fatalf("direct host navigation: %v", err)
	}
	if sess != nil {
		defer sess.Close()
	}
	if len(historyStore.Entries()) != 0 {
		t.Fatal("direct host navigation did not execute trusted action")
	}

	historyStore.Record("https://visited.example/article", "Visited")
	untrusted := withNavigationTrust(context.Background(), untrustedNavigation)
	for _, rawURL := range []string{target, "view-source:" + target} {
		_, _, _, sess, _, err := f.loadDocumentCtx(untrusted, nil, rawURL, 800, 600, 1, f.downloadDirectory(), true, true)
		if !errors.Is(err, errInternalNavigationFromContent) {
			t.Fatalf("untrusted loadDocumentCtx(%q) error = %v", rawURL, err)
		}
		if sess != nil {
			t.Fatal("untrusted loadDocumentCtx returned a session")
		}
		if len(historyStore.Entries()) != 1 {
			t.Fatal("untrusted loadDocumentCtx executed internal action")
		}
	}
}

func TestTrustedInternalNavigationPaths(t *testing.T) {
	f, root, _ := navigationTrustTestPath(t)
	bookmarkStore, err := bookmarks.Load(filepath.Join(root, "bookmarks.json"))
	if err != nil {
		t.Fatal(err)
	}
	bookmarkStore.Toggle("https://saved.example/article", "Saved")
	f.bookmarks = bookmarkStore
	historyStore, err := history.Load(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	historyStore.Record("https://visited.example/article", "Visited")
	f.history = historyStore
	tab := f.tabMgr.Active()

	f.navigateTab("goosie://version")
	res := receiveNavigationResult(t, f)
	if res.err != nil || res.layer == nil {
		t.Fatalf("typed internal navigation: %v", res.err)
	}
	f.applyNavResult(res)
	if tab.URL() != "goosie://version" {
		t.Fatalf("typed URL = %q", tab.URL())
	}

	f.reloadTab()
	res = receiveNavigationResult(t, f)
	if res.err != nil || res.layer == nil {
		t.Fatalf("internal reload: %v", res.err)
	}
	f.applyNavResult(res)

	tab.SetURL("goosie://bookmarks")
	f.handleLinkClick("goosie://history?action=clear&confirm=1")
	res = receiveNavigationResult(t, f)
	if res.err != nil || res.layer == nil {
		t.Fatalf("trusted internal link: %v", res.err)
	}
	f.applyNavResult(res)
	if len(historyStore.Entries()) != 0 {
		t.Fatal("trusted internal link did not execute action")
	}

	tab.SetURL("goosie://bookmarks")
	f.handleFormSubmission(tab, &engine.FormSubmission{
		Action: "goosie://bookmarks?action=rename&url=" + url.QueryEscape("https://saved.example/article") + "&title=" + url.QueryEscape("Renamed"),
		Method: "GET",
	})
	res = receiveNavigationResult(t, f)
	if res.err != nil || res.layer == nil {
		t.Fatalf("trusted internal form: %v", res.err)
	}
	f.applyNavResult(res)
	if got := bookmarkStore.List()[0].Title; got != "Renamed" {
		t.Fatalf("bookmark title = %q", got)
	}

	f.handleContextMenuAction("downloads", "")
	res = receiveNavigationResult(t, f)
	if res.err != nil || res.layer == nil {
		t.Fatalf("browser menu navigation: %v", res.err)
	}
	f.applyNavResult(res)
	if tab.URL() != "goosie://downloads" {
		t.Fatalf("menu URL = %q", tab.URL())
	}

	f.navigateTab("goosie://version")
	res = receiveNavigationResult(t, f)
	if res.err != nil {
		t.Fatalf("history setup navigation: %v", res.err)
	}
	f.applyNavResult(res)
	f.navigateTab("goosie://bookmarks")
	res = receiveNavigationResult(t, f)
	if res.err != nil {
		t.Fatalf("history setup navigation: %v", res.err)
	}
	f.applyNavResult(res)
	f.traverseTab(-1)
	res = receiveNavigationResult(t, f)
	if res.err != nil || res.layer == nil {
		t.Fatalf("internal history traversal: %v", res.err)
	}
	f.applyNavResult(res)
	if tab.URL() != "goosie://version" {
		t.Fatalf("traversal URL = %q", tab.URL())
	}
}

func TestHostileContextMenuLinksCannotNavigateInternalPages(t *testing.T) {
	f, root, _ := navigationTrustTestPath(t)
	historyStore, err := history.Load(filepath.Join(root, "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	historyStore.Record("https://visited.example/article", "Visited")
	f.history = historyStore

	for _, action := range []string{"open_new_tab", "open_image_tab"} {
		tab := f.tabMgr.Active()
		tab.SetURL("https://hostile.example/page")
		f.handleContextMenuAction(action, "goosie://history?action=clear&confirm=1")
		res := receiveNavigationResult(t, f)
		requireBlockedInternalNavigation(t, res)
		f.applyNavResult(res)
		if len(historyStore.Entries()) != 1 {
			t.Fatalf("hostile %s context action cleared history", action)
		}
	}
}

func TestTrustedInternalContextMenuLinkOpens(t *testing.T) {
	f, _, _ := navigationTrustTestPath(t)
	f.tabMgr.Active().SetURL("goosie://bookmarks")

	f.handleContextMenuAction("open_new_tab", "goosie://history")
	res := receiveNavigationResult(t, f)
	if res.err != nil || res.layer == nil {
		t.Fatalf("trusted internal context link: %v", res.err)
	}
	f.applyNavResult(res)
	if got := f.tabMgr.Active().URL(); got != "goosie://history" {
		t.Fatalf("trusted context URL = %q", got)
	}
}

func TestReaderNavigationLinksKeepWorking(t *testing.T) {
	f, _, _ := navigationTrustTestPath(t)
	tab := f.tabMgr.Active()
	tab.SetURL("goosie://reader?url=https%3A%2F%2Fexample.com%2Farticle&theme=light")

	f.handleLinkClick("goosie://reader?url=https%3A%2F%2Fexample.com%2Farticle&theme=dark")
	res := receiveNavigationResult(t, f)
	if res.err != nil || res.layer == nil {
		t.Fatalf("reader navigation link: %v", res.err)
	}
	f.applyNavResult(res)
	if tab.URL() != "goosie://reader?url=https%3A%2F%2Fexample.com%2Farticle&theme=dark" {
		t.Fatalf("reader URL = %q", tab.URL())
	}
}

func TestTrustedNavigationPromptsKeepWorking(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("fonts unavailable: %v", err)
	}
	f, _, _ := navigationTrustTestPath(t)
	f.fonts = fonts
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>reloaded</body></html>"))
	}))
	defer server.Close()
	client := goosienet.DefaultClient()
	defer client.Close()
	f.client = client
	tab := f.tabMgr.Active()
	target := server.URL + "/page"
	tab.SetURL(target)
	f.markTrustedNavigationPrompt(tab.ID())
	if got := f.navigationTrustForTab(tab); got != trustedNavigation {
		t.Fatalf("trusted prompt navigation trust = %v", got)
	}

	f.handleLinkClick("goosie://unresponsive?action=stop&url=" + url.QueryEscape(target))
	res := receiveNavigationResult(t, f)
	if res.err != nil || res.layer == nil {
		t.Fatalf("trusted prompt navigation: %v", res.err)
	}
	f.applyNavResult(res)
	if tab.URL() != target {
		t.Fatalf("trusted prompt target = %q, want %q", tab.URL(), target)
	}
	if f.hasTrustedNavigationPrompt(tab.ID()) {
		t.Fatal("trusted prompt marker survived navigation")
	}

	f.handleLinkClick("goosie://history?action=clear&confirm=1")
	res = receiveNavigationResult(t, f)
	requireBlockedInternalNavigation(t, res)
}

func TestDownloadsPageActionsValidateManagedPaths(t *testing.T) {
	f, root, launcher := navigationTrustTestPath(t)
	dir := f.downloadDirectory()
	inside := filepath.Join(dir, "managed.txt")
	if err := os.WriteFile(inside, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	unmanaged := filepath.Join(dir, "unmanaged.txt")
	if err := os.WriteFile(unmanaged, []byte("unmanaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	item := f.downloads.CreateItem("https://example.com/managed.txt", "managed.txt", 6)
	f.downloads.Complete(item.ID, inside)
	outsideItem := f.downloads.CreateItem("https://example.com/outside.txt", "outside.txt", 7)
	f.downloads.Complete(outsideItem.ID, outside)

	_, body := f.buildInternalPageHTML("goosie://downloads?action=open&id=" + url.QueryEscape(item.ID))
	if !strings.Contains(body, "Downloaded file opened") || len(launcher.opened) != 1 {
		t.Fatalf("managed open did not launch: %s", body)
	}
	_, body = f.buildInternalPageHTML("goosie://downloads?action=reveal&id=" + url.QueryEscape(item.ID))
	if !strings.Contains(body, "Downloaded file revealed") || len(launcher.revealed) != 1 {
		t.Fatalf("managed reveal did not launch: %s", body)
	}
	for _, actionURL := range []string{
		"goosie://downloads?action=open&path=" + url.QueryEscape(outside),
		"goosie://downloads?action=reveal&path=" + url.QueryEscape(outside),
		"goosie://downloads?action=open&id=" + url.QueryEscape(outsideItem.ID),
		"goosie://downloads?action=reveal&id=" + url.QueryEscape(outsideItem.ID),
	} {
		_, body = f.buildInternalPageHTML(actionURL)
		if !strings.Contains(body, "Download action rejected") {
			t.Fatalf("arbitrary path was not rejected: %s", body)
		}
	}
	if len(launcher.opened) != 1 || len(launcher.revealed) != 1 {
		t.Fatalf("rejected download actions reached launcher: %+v", launcher)
	}

	_, body = f.buildInternalPageHTML("goosie://downloads?action=open&path=" + url.QueryEscape(unmanaged))
	if !strings.Contains(body, "Downloaded file opened") || len(launcher.opened) != 2 {
		t.Fatalf("managed directory file did not open: %s", body)
	}
	_, body = f.buildInternalPageHTML("goosie://downloads?action=reveal&path=" + url.QueryEscape(unmanaged))
	if !strings.Contains(body, "Downloaded file revealed") || len(launcher.revealed) != 2 {
		t.Fatalf("managed directory file was not revealed: %s", body)
	}

	_, body = f.buildInternalPageHTML("goosie://downloads")
	if !strings.Contains(body, "action=open&id="+url.QueryEscape(item.ID)) || !strings.Contains(body, "action=reveal&id="+url.QueryEscape(item.ID)) {
		t.Fatalf("managed download links do not use item IDs: %s", body)
	}
	if strings.Contains(body, "action=open&path="+url.QueryEscape(inside)) {
		t.Fatal("managed download link still exposes a path")
	}

	f.navigateTab("goosie://downloads?action=open&id=" + url.QueryEscape(item.ID))
	res := receiveNavigationResult(t, f)
	if res.err != nil || res.layer == nil {
		t.Fatalf("trusted download navigation: %v", res.err)
	}
	f.applyNavResult(res)
	if len(launcher.opened) != 3 {
		t.Fatalf("trusted download action launched %d times, want once", len(launcher.opened)-2)
	}
	if got := f.tabMgr.Active().Title(); got != "Downloads" {
		t.Fatalf("downloads title = %q", got)
	}
}
