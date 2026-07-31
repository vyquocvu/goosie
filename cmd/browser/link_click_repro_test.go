package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	fynetest "fyne.io/fyne/v2/test"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine/session"
	goosienet "github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/renderer"
	"github.com/vyquocvu/goosie/internal/ui"
)

func TestRepro_PathOnlyLinkClickResolution(t *testing.T) {
	app := fynetest.NewApp()
	defer app.Quit()
	window := app.NewWindow("link click test")

	navSession := session.New()
	defer navSession.Close()
	networkService := goosienet.NewService(goosienet.ServiceOptions{Client: navSession.HTTPClient()})
	defer networkService.Close()
	fetcher := goosienet.NewFetcherWithService(networkService)
	parser := dom.NewParser()

	browser := ui.NewBrowserWithDependencies(ui.BrowserDependencies{
		App:        app,
		Window:     window,
		Headless:   true,
		NavSession: navSession,
		Network:    networkService,
	})
	browser.RendererFactory = func() ui.HTMLRenderer { return renderer.NewRenderer(1000, 600) }

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><a href="/next">Next</a></body></html>`))
	}))
	defer srv.Close()

	// First navigation on the initial tab.
	firstLoad, firstCtx := navSession.Navigate(context.Background(), srv.URL+"/dir1/page1")
	browser.NavigateTo(firstLoad.URL)
	done := make(chan struct{}, 1)
	loadPageAsyncWithCoordinator(browser, fetcher, parser, firstLoad, firstCtx, navSession, networkService, func() { done <- struct{}{} })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("first load did not complete")
	}

	tab := browser.ActiveTab()
	rend := tab.GetRenderer()
	if rend == nil {
		t.Fatal("renderer missing")
	}
	if got := rend.ResolveURL("/next"); got != srv.URL+"/next" {
		t.Fatalf("after page1: ResolveURL(/next) = %q, want %q", got, srv.URL+"/next")
	}

	secondLoad, secondCtx := navSession.Navigate(context.Background(), srv.URL+"/dir2/page2")
	browser.NavigateTo(secondLoad.URL)
	done2 := make(chan struct{}, 1)
	loadPageAsyncWithCoordinator(browser, fetcher, parser, secondLoad, secondCtx, navSession, networkService, func() { done2 <- struct{}{} })
	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("second load did not complete")
	}
	if got := rend.ResolveURL("/next"); got != srv.URL+"/next" {
		t.Fatalf("after page2: ResolveURL(/next) = %q, want %q", got, srv.URL+"/next")
	}
	if got := rend.ResolveURL("sibling.html"); got != srv.URL+"/dir2/sibling.html" {
		t.Fatalf("after page2: ResolveURL(sibling.html) = %q, want %q (renderer base URL not refreshed across navigations)", got, srv.URL+"/dir2/sibling.html")
	}
}
