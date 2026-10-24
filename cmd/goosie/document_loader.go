package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/download"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/pdf"
	"github.com/vyquocvu/goosie/internal/perms"
	"github.com/vyquocvu/goosie/internal/platform"
	"github.com/vyquocvu/goosie/internal/raster"
	"github.com/vyquocvu/goosie/internal/userscript"
	"github.com/vyquocvu/goosie/internal/viewsource"
)

// loadURLCtx is like loadURL but respects context cancellation. With
// deferImages the session collects image references without fetching them, so
// the first frame can go out immediately and LoadDeferredImages repaints once
// they land.
func loadURLCtx(ctx context.Context, client net.HTTP, fonts *raster.Fonts, rawURL string, viewportW, viewportH int, scale float32, downloadDir string, deferImages, deferFonts bool, extra ...any) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, string, error) {
	if err := engine.ValidateViewport(viewportW, viewportH, float64(scale)); err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: viewport: %w", err)
	}
	// Check context before starting the fetch.
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", ctx.Err()
	}
	fetchURL := rawURL
	targetURL, isViewSource := viewsource.IsViewSourceURL(rawURL)
	if isViewSource {
		fetchURL = targetURL
	}
	var resp *net.Response
	var stream *net.StreamResponse
	var err error
	if streamer, ok := client.(net.StreamingHTTP); ok && isHTTPURL(fetchURL) {
		stream, err = streamer.GetStream(ctx, fetchURL)
		if err == nil {
			resp = &net.Response{StatusCode: stream.StatusCode, Headers: stream.Headers, URL: stream.URL}
		}
	} else {
		resp, err = client.Get(ctx, fetchURL)
	}
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: fetch %s: %w", net.RedactURL(fetchURL), err)
	}
	if stream != nil {
		defer stream.Close()
	}
	var mgr *download.Manager
	var storageReg *js.OriginStorageRegistry
	var sessionID string
	var appMode appearanceMode
	var userscriptMgr *userscript.Manager
	var permsMgr *perms.Manager
	var mediaBackend media.Backend
	var savePathPicker platform.SavePathPicker
	for _, ex := range extra {
		switch v := ex.(type) {
		case *download.Manager:
			mgr = v
		case *js.OriginStorageRegistry:
			storageReg = v
		case string:
			sessionID = v
		case appearanceMode:
			appMode = v
		case *userscript.Manager:
			userscriptMgr = v
		case *perms.Manager:
			permsMgr = v
		case media.Backend:
			mediaBackend = v
		case platform.SavePathPicker:
			savePathPicker = v
		}
	}
	if isViewSource && stream != nil {
		body, readErr := stream.ReadBounded(net.MaxResponseBytes)
		if readErr != nil {
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: read view-source response: %w", readErr)
		}
		resp.Body = body
	}

	if isViewSource {
		viewHTML := viewsource.RenderHTML(resp.Text(), resp.URL, appMode.dark)
		sessOpts := []engine.Option{
			engine.WithMetrics(fonts),
			engine.WithViewportH(float32(viewportH)),
			engine.WithDarkMode(appMode.dark),
			engine.WithHighContrast(appMode.highContrast),
		}
		sess, err := engine.NewSession(viewHTML, nil, float32(viewportW), sessOpts...)
		if err != nil {
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: build view-source session: %w", err)
		}
		list, err := sess.PaintChecked(scale)
		if err != nil {
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: paint view-source document: %w", err)
		}
		dl := list.Build(1)
		extent := dl.Extent()
		budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
		if err != nil {
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: size view-source tile cache: %w", err)
		}
		pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
		layer := frame.NewLayer(1, extent, budgetBytes, pool)
		layer.SetContent(dl)
		return layer, paint.SceneSpec{DocHeight: extent.H()}, sess.BackgroundColor(), sess, viewsource.WrapViewSourceURL(resp.URL), nil
	}

	contentType := resp.Headers["Content-Type"]
	contentDisposition := resp.Headers["Content-Disposition"]
	isAttachment := strings.Contains(strings.ToLower(contentDisposition), "attachment")

	var streamBody io.Reader
	streamIsPDF := false
	if stream != nil {
		prefix := make([]byte, 5)
		n, readErr := io.ReadFull(stream.Body, prefix)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: read response: %w", readErr)
		}
		prefix = prefix[:n]
		streamBody = io.MultiReader(bytes.NewReader(prefix), stream.Body)
		streamIsPDF = !isAttachment && pdf.IsPDF(prefix, contentType, resp.URL)
		if download.ShouldDownload(contentType, contentDisposition) && !isAttachment && !streamIsPDF {
			// Unknown binary content is downloaded directly to a temporary file;
			// it never passes through the 16 MiB document response buffer.
			isAttachment = true
		}
		if !isAttachment || streamIsPDF {
			if stream.ContentLength > net.MaxResponseBytes {
				return nil, paint.SceneSpec{}, frame.Color(0), nil, "", net.ErrResponseTooLarge
			}
			body, readErr := io.ReadAll(io.LimitReader(streamBody, net.MaxResponseBytes+1))
			if int64(len(body)) > net.MaxResponseBytes {
				return nil, paint.SceneSpec{}, frame.Color(0), nil, "", net.ErrResponseTooLarge
			}
			if readErr != nil {
				return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: read response: %w", readErr)
			}
			resp.Body = body
			// The body is now buffered; if PDF parsing declines the response,
			// the existing bounded Save path must use resp.Body rather than the
			// already-consumed stream reader.
			stream = nil
		}
	}
	if !isAttachment && pdf.IsPDF(resp.Body, contentType, resp.URL) {
		fallbackTitle := download.FileName(contentDisposition, resp.URL)
		pdfDoc, pdfErr := pdf.Parse(resp.Body, fallbackTitle)
		if pdfErr == nil && pdfDoc != nil {
			pdfHTML := pdf.RenderHTML(pdfDoc, resp.URL)
			rt, _ := js.New(js.Options{
				URL:          resp.URL,
				DarkMode:     appMode.dark,
				HighContrast: appMode.highContrast,
			})
			sessOpts := []engine.Option{
				engine.WithMetrics(fonts),
				engine.WithViewportH(float32(viewportH)),
				engine.WithDarkMode(appMode.dark),
				engine.WithHighContrast(appMode.highContrast),
			}
			if rt != nil {
				sessOpts = append(sessOpts, engine.WithJS(rt))
			}
			sess, err := engine.NewSession(pdfHTML, nil, float32(viewportW), sessOpts...)
			if err != nil {
				return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: build pdf session: %w", err)
			}
			list, err := sess.PaintChecked(scale)
			if err != nil {
				return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: paint pdf document: %w", err)
			}
			dl := list.Build(1)
			extent := dl.Extent()
			budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
			if err != nil {
				return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: size pdf tile cache: %w", err)
			}
			pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
			layer := frame.NewLayer(1, extent, budgetBytes, pool)
			layer.SetContent(dl)
			return layer, paint.SceneSpec{DocHeight: extent.H()}, sess.BackgroundColor(), sess, resp.URL, nil
		}
	}

	if download.ShouldDownload(contentType, contentDisposition) {
		name := download.FileName(resp.Headers["Content-Disposition"], resp.URL)
		if stream != nil {
			targetPath := ""
			if savePathPicker != nil {
				chosenPath, chosen, pickErr := savePathPicker.ChooseSavePath(downloadDir, name)
				if pickErr != nil {
					return nil, paint.SceneSpec{}, frame.Color(0), nil, "", pickErr
				}
				if ctx.Err() != nil {
					return nil, paint.SceneSpec{}, frame.Color(0), nil, "", ctx.Err()
				}
				if !chosen {
					return nil, paint.SceneSpec{}, frame.Color(0), nil, "", errDownloadSaveCanceled
				}
				if strings.TrimSpace(chosenPath) == "" {
					return nil, paint.SceneSpec{}, frame.Color(0), nil, "", errors.New("goosie: save dialog returned an empty path")
				}
				targetPath = chosenPath
				name = filepath.Base(targetPath)
			}
			downloadCtx, cancel := context.WithCancel(ctx)
			var itemID string
			if mgr != nil {
				it := mgr.CreateItem(resp.URL, name, stream.ContentLength)
				itemID = it.ID
				mgr.SetCancel(itemID, func() {
					cancel()
					stream.Cancel()
				})
			}
			lastProgressBytes := int64(0)
			lastProgressAt := time.Now()
			progress := func(downloaded int64) {
				if mgr != nil && itemID != "" && (lastProgressBytes == 0 || downloaded-lastProgressBytes >= 512*1024 || time.Since(lastProgressAt) >= 250*time.Millisecond) {
					mgr.UpdateProgress(itemID, downloaded)
					lastProgressBytes = downloaded
					lastProgressAt = time.Now()
				}
			}
			var savedPath string
			var downloaded int64
			var saveErr error
			if targetPath != "" {
				downloaded, saveErr = download.SaveStreamTo(downloadCtx, targetPath, streamBody, progress)
				savedPath, _ = filepath.Abs(targetPath)
			} else {
				savedPath, downloaded, saveErr = download.SaveStream(downloadCtx, downloadDir, name, streamBody, progress)
			}
			cancel()
			if saveErr != nil {
				if mgr != nil && itemID != "" {
					if saveErr == context.Canceled {
						mgr.Cancel(itemID)
					} else if it := mgr.Get(itemID); it != nil && it.Status != download.StatusCanceled {
						mgr.Fail(itemID, saveErr)
					}
				}
				return nil, paint.SceneSpec{}, frame.Color(0), nil, "", saveErr
			}
			if mgr != nil && itemID != "" {
				mgr.UpdateProgress(itemID, downloaded)
				mgr.Complete(itemID, savedPath)
				if it := mgr.Get(itemID); it == nil || it.Status != download.StatusCompleted {
					_ = os.Remove(savedPath)
					return nil, paint.SceneSpec{}, frame.Color(0), nil, "", context.Canceled
				}
			}
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", downloadDone{path: savedPath}
		}
		var itemID string
		if mgr != nil {
			it := mgr.CreateItem(resp.URL, name, int64(len(resp.Body)))
			itemID = it.ID
		}
		savedPath, saveErr := download.Save(downloadDir, name, resp.Body)
		if saveErr != nil {
			if mgr != nil && itemID != "" {
				mgr.Fail(itemID, saveErr)
			}
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", saveErr
		}
		if mgr != nil && itemID != "" {
			mgr.Complete(itemID, savedPath)
		}
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", downloadDone{path: savedPath}
	}
	// Family deadline contexts: the engine's fetch budgets stop new spawns past
	// their deadline, but only context aborts work already in flight. Each
	// synchronous family gets a deadline child of the navigation context, so a
	// hung host settles at the budget instead of the 30s client timeout; tab
	// close and shutdown still cancel everything through the parent. Deferred
	// families keep the plain navigation context: their work starts after this
	// function returns, and post-paint arrivals carry no total deadline by
	// design (superseded results are still dropped by serial).
	cssCtx, cssCancel := context.WithTimeout(ctx, engine.CSSFetchBudget)
	defer cssCancel()
	imgCtx := ctx
	if !deferImages {
		var imgCancel context.CancelFunc
		imgCtx, imgCancel = context.WithTimeout(ctx, engine.ImageFetchBudget)
		defer imgCancel()
	}
	fontCtx := ctx
	if !deferFonts {
		var fontCancel context.CancelFunc
		fontCtx, fontCancel = context.WithTimeout(ctx, engine.FontFetchBudget)
		defer fontCancel()
	}
	// External scripts are a synchronous family like stylesheets: the
	// document cannot run them before they arrive, so they share the
	// deadline-child shape. The engine resolves srcs before calling.
	scriptCtx, scriptCancel := context.WithTimeout(ctx, engine.ScriptFetchBudget)
	defer scriptCancel()
	origin := perms.NormalizeOrigin(resp.URL)
	jsAllowed := permsMgr == nil || permsMgr.IsAllowed(origin, perms.PermissionJavaScript)
	imgAllowed := permsMgr == nil || permsMgr.IsAllowed(origin, perms.PermissionImages)
	cookiesAllowed := permsMgr == nil || permsMgr.IsAllowed(origin, perms.PermissionCookies)

	scriptFetcher := func(base, url string) ([]byte, error) {
		if !jsAllowed {
			return nil, fmt.Errorf("goosie: scripts blocked by site permissions")
		}
		resp, err := client.Get(subresourceCtx(scriptCtx, base), url)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	// fetch()/XMLHttpRequest transport for page scripts. Unlike the
	// families above these fire after the load (event handlers, beacons),
	// so they ride the navigation context rather than a load-scoped
	// deadline: post-paint arrivals carry no total deadline by the same
	// design as deferred images and fonts.
	jsFetch := jsTransport{client: client, ctx: subresourceCtx(ctx, resp.URL)}
	// Linked style sheets are fetched with the same client and the same
	// cancellation: the linker sees absolute http(s) URLs only, because the
	// engine resolves hrefs against the final document URL before calling.
	linker := func(base, href string) (string, error) {
		resp, err := client.Get(subresourceCtx(cssCtx, base), href)
		if err != nil {
			return "", err
		}
		if refused := resp.Headers["Content-Type"]; net.StyleSheetRefused(refused) {
			return "", fmt.Errorf("goosie: %s is served as %q, not as a style sheet", net.RedactURL(href), refused)
		}
		return resp.Text(), nil
	}
	// Image subresources use the same client and cancellation. The engine hands
	// absolute http(s) URLs only, so the fetcher just reads the response bytes.
	imageFetcher := func(base, url string) ([]byte, error) {
		if !imgAllowed {
			return nil, fmt.Errorf("goosie: image loading blocked by site permissions")
		}
		resp, err := client.Get(subresourceCtx(imgCtx, base), url)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	// @font-face resources use the same client. The fetcher is the same shape
	// as the image fetcher, plus the response headers the engine's font CORS
	// check reads; the engine resolves src URLs before calling.
	fontFetcher := func(base, url string) (engine.FontResult, error) {
		resp, err := client.Get(subresourceCtx(fontCtx, base), url)
		if err != nil {
			return engine.FontResult{}, err
		}
		return engine.FontResult{Data: resp.Body, Headers: resp.Headers}, nil
	}
	// Check context after the fetch in case it was cancelled during the network call.
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", ctx.Err()
	}
	// A per-document JS runtime executes the page's classic scripts during
	// the build below. A runtime that fails to construct degrades to no
	// scripting rather than failing the load. The session owns it
	// afterwards and closes it with itself (see applyNavResult/closeTab), so
	// every error return below must close what it created.
	var cookieStore js.CookieStore
	if client != nil && cookiesAllowed {
		cookieStore = client.Jar()
	}
	var rt *js.Runtime
	if jsAllowed {
		var rtErr error
		rt, rtErr = js.New(js.Options{
			URL:          resp.URL,
			HTTPClient:   jsFetch,
			Cookies:      cookieStore,
			Storage:      storageReg,
			SessionID:    sessionID,
			DarkMode:     appMode.dark,
			HighContrast: appMode.highContrast,
		})
		if rtErr != nil {
			fmt.Fprintf(os.Stderr, "goosie: js runtime: %v\n", rtErr)
			rt = nil
		}
	}
	completed := false
	defer func() {
		if !completed && rt != nil {
			_ = rt.Close()
		}
	}()
	sessOpts := []engine.Option{
		engine.WithMetrics(fonts),
		engine.WithViewportH(float32(viewportH)),
		engine.WithLinkedCSS(resp.URL, linker),
		engine.WithScripts(resp.URL, scriptFetcher),
		engine.WithJS(rt),
		engine.WithDarkMode(appMode.dark),
		engine.WithHighContrast(appMode.highContrast),
		// Every load gets a fresh invalidation batch: JS-driven DOM
		// mutations record here so the vsync pump reflows exactly the
		// documents that changed.
		engine.WithInvalidation(&engine.Invalidation{}),
		imageOption(deferImages, resp.URL, imageFetcher),
		fontOption(deferFonts, resp.URL, fontFetcher, fonts),
	}
	if userscriptMgr != nil && jsAllowed {
		start := userscriptMgr.MatchingScripts(resp.URL, userscript.RunAtDocumentStart)
		end := userscriptMgr.MatchingScripts(resp.URL, userscript.RunAtDocumentEnd)
		matched := append(start, end...)
		if len(matched) > 0 {
			sessOpts = append(sessOpts, engine.WithUserScripts(matched))
		}
	}
	if opt := mediaOption(mediaBackend, client, resp.URL); opt != nil {
		sessOpts = append(sessOpts, opt)
	}
	sess, err := engine.NewSession(resp.Text(), nil, float32(viewportW), sessOpts...)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: build session: %w", err)
	}
	list, err := sess.PaintChecked(scale)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: paint document: %w", err)
	}
	dl := list.Build(1)
	extent := dl.Extent()
	budgetTiles, budgetBytes, err := engine.TileCacheBudget(extent)
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: size document cache: %w", err)
	}
	pool := frame.NewBitmapPool(frame.Size{W: frame.TileSize, H: frame.TileSize}, budgetTiles)
	layer := frame.NewLayer(1, extent, budgetBytes, pool)
	layer.SetContent(dl)
	completed = true
	return layer, paint.SceneSpec{DocHeight: extent.H()}, sess.BackgroundColor(), sess, resp.URL, nil
}

var errDownloadSaveCanceled = errors.New("goosie: download save dialog canceled")

func isHTTPURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
