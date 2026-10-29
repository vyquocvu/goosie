package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
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
	var broadcastHub *js.BroadcastHub
	var clipboardProvider js.ClipboardProvider
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
		case *js.BroadcastHub:
			broadcastHub = v
		case js.ClipboardProvider:
			clipboardProvider = v
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
		// Deliberately not deferred. A parsed <img> fetches and decodes inside
		// this call, but a page that runs script can create an image after it
		// has returned (`new Image(); img.src = ...`), and the loader starts
		// that fetch on its own goroutine as soon as the script assigns src.
		// Canceling on return aborted every one of those before the request
		// reached the network (GOOSIE-PUMP-001). The lifetime is instead the
		// engine's ImageFetchBudget deadline below - the same window the
		// parse-time image pool gets - and the parent navigation context, which
		// tab close and shutdown cancel. Do not widen that budget.
		var imgCancel context.CancelFunc
		imgCtx, imgCancel = context.WithTimeout(ctx, engine.ImageFetchBudget)
		_ = imgCancel
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
	iframeFetcher := func(base, source string) (*dom.Document, error) {
		if client == nil {
			return nil, fmt.Errorf("goosie: iframe transport unavailable")
		}
		baseURL, err := url.Parse(base)
		if err != nil {
			return nil, err
		}
		reference, err := url.Parse(source)
		if err != nil {
			return nil, err
		}
		frameURL := baseURL.ResolveReference(reference)
		if frameURL.Scheme != "http" && frameURL.Scheme != "https" {
			return nil, fmt.Errorf("iframe source must use HTTP(S)")
		}
		if perms.NormalizeOrigin(frameURL.String()) != perms.NormalizeOrigin(base) {
			return nil, fmt.Errorf("goosie: cross-origin iframe document access is not available")
		}
		frameResponse, err := client.Get(subresourceCtx(ctx, base), frameURL.String())
		if err != nil {
			return nil, err
		}
		if frameResponse.StatusCode < 200 || frameResponse.StatusCode >= 400 {
			return nil, fmt.Errorf("iframe response status %d", frameResponse.StatusCode)
		}
		text, encoding, _ := frameResponse.TextWithEncoding()
		if len(text) > engine.MaxDocumentBytes {
			return nil, fmt.Errorf("iframe document exceeds the engine byte limit after decoding")
		}
		frameDoc, err := dom.ParseBounded(text, dom.ParseLimits{
			Nodes: engine.MaxDocumentNodes, Depth: engine.MaxDocumentDepth,
			Attributes: engine.MaxAttributes, AttributeBytes: engine.MaxAttributeBytes,
		})
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(encoding, "utf-8") {
			encoding = "UTF-8"
		} else if strings.HasPrefix(strings.ToLower(encoding), "iso-8859-") {
			encoding = strings.ToUpper(encoding)
		}
		frameDoc.CharacterSet = encoding
		if frameResponse.Headers != nil {
			frameDoc.ContentLanguage = frameResponse.Headers["Content-Language"]
		}
		return frameDoc, nil
	}
	iframeFormSubmitter := func(base, target, contentType string, body []byte) (*dom.Document, error) {
		if client == nil {
			return nil, fmt.Errorf("goosie: iframe form transport unavailable")
		}
		baseURL, err := url.Parse(base)
		if err != nil {
			return nil, err
		}
		reference, err := url.Parse(target)
		if err != nil {
			return nil, err
		}
		formURL := baseURL.ResolveReference(reference)
		if (formURL.Scheme != "http" && formURL.Scheme != "https") || perms.NormalizeOrigin(formURL.String()) != perms.NormalizeOrigin(base) {
			return nil, fmt.Errorf("goosie: cross-origin iframe form submission is not available")
		}
		formResponse, err := client.Post(subresourceCtx(ctx, base), formURL.String(), contentType, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		if formResponse.StatusCode < 200 || formResponse.StatusCode >= 400 {
			return nil, fmt.Errorf("iframe form response status %d", formResponse.StatusCode)
		}
		if formResponse.URL != "" && perms.NormalizeOrigin(formResponse.URL) != perms.NormalizeOrigin(base) {
			return nil, fmt.Errorf("goosie: iframe form redirect is outside the local origin")
		}
		text, encoding, _ := formResponse.TextWithEncoding()
		if len(text) > engine.MaxDocumentBytes {
			return nil, fmt.Errorf("iframe form response exceeds the engine byte limit after decoding")
		}
		responseType, _, _ := mime.ParseMediaType(formResponse.Headers["Content-Type"])
		var frameDoc *dom.Document
		if responseType == "text/plain" {
			frameDoc = makeTextDocument(text)
		} else {
			frameDoc, err = dom.ParseBounded(text, dom.ParseLimits{
				Nodes: engine.MaxDocumentNodes, Depth: engine.MaxDocumentDepth,
				Attributes: engine.MaxAttributes, AttributeBytes: engine.MaxAttributeBytes,
			})
			if err != nil {
				return nil, err
			}
		}
		if strings.EqualFold(encoding, "utf-8") {
			encoding = "UTF-8"
		} else if strings.HasPrefix(strings.ToLower(encoding), "iso-8859-") {
			encoding = strings.ToUpper(encoding)
		}
		frameDoc.CharacterSet = encoding
		if formResponse.Headers != nil {
			frameDoc.ContentLanguage = formResponse.Headers["Content-Language"]
		}
		return frameDoc, nil
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
	var clipboardPermissionState func(documentURL, name string) string
	if permsMgr != nil {
		clipboardPermissionState = func(documentURL, name string) string {
			permissionOrigin := perms.NormalizeOrigin(documentURL)
			var permission perms.Permission
			switch name {
			case "clipboard-read":
				permission = perms.PermissionClipboardRead
			case "clipboard-write":
				permission = perms.PermissionClipboardWrite
			default:
				return "prompt"
			}
			switch permsMgr.State(permissionOrigin, permission) {
			case perms.StateAllow:
				return "granted"
			case perms.StateBlock:
				return "denied"
			default:
				return "prompt"
			}
		}
	}
	var rt *js.Runtime
	if jsAllowed {
		var rtErr error
		rt, rtErr = js.New(js.Options{
			URL:                      resp.URL,
			BroadcastHub:             broadcastHub,
			HTTPClient:               jsFetch,
			Cookies:                  cookieStore,
			Storage:                  storageReg,
			ClipboardProvider:        clipboardProvider,
			ClipboardPermissionState: clipboardPermissionState,
			SessionID:                sessionID,
			DarkMode:                 appMode.dark,
			HighContrast:             appMode.highContrast,
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
		engine.WithIFrameDocumentLoader(iframeFetcher),
		engine.WithIFrameFormSubmitter(iframeFormSubmitter),
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
	documentText, documentEncoding, _ := resp.TextWithEncoding()
	sessOpts = append(sessOpts, engine.WithDocumentEncoding(documentEncoding))
	sess, err := engine.NewSession(documentText, nil, float32(viewportW), sessOpts...)
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
	if resp.StatusCode >= 400 {
		reportRefusedDocument(os.Stderr, resp)
	}
	reportScriptErrors(os.Stderr, resp.URL, sess)
	return layer, paint.SceneSpec{DocHeight: extent.H()}, sess.BackgroundColor(), sess, resp.URL, nil
}

func makeTextDocument(text string) *dom.Document {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	head := doc.NewElement("head")
	body := doc.NewElement("body")
	pre := doc.NewElement("pre")
	pre.AppendChild(doc.NewText(text))
	body.AppendChild(pre)
	html.AppendChild(head)
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Head, doc.Body = html, head, body
	doc.ContentType = "text/plain"
	return doc
}

// reportScriptErrors writes one line naming the document's uncaught script
// errors, and nothing at all when there are none.
//
// It exists because the engine used to throw these errors away, so every page
// of GOOSIE-LIVE-001 - nine real sites, 0 ink pixels - reported rc 0 with an
// empty stderr: no reason for the sweep to record, the operator to read, or the
// bug to cite. The reason was there all along (21 uncaught script errors on
// www.gov.vn, the first one naming the page's jQuery bundle); it was just
// discarded one layer down. The session bounds how many are kept and how
// long each message can be, so a hostile document cannot grow this line.
// Nothing here changes what is rendered: it is a report, after the paint. The
// destination is a parameter so a test can read the line without taking over
// the process's stderr.
func reportScriptErrors(w io.Writer, rawURL string, sess *engine.Session) {
	total := sess.ScriptErrorCount()
	errs := sess.ScriptErrors()
	if total == 0 || len(errs) == 0 {
		return
	}
	fmt.Fprintf(w, "goosie: %d uncaught script error(s) in %s; first: %s\n",
		total, rawURL, errs[0])
}

// reportRefusedDocument writes one line naming the status the source answered
// with and how many bytes of that refusal Goosie rendered instead of a page.
//
// A 401/403 body is an ordinary document to the pipeline: it parses, it lays
// out, and it paints nothing, because a bot-management interstitial shows its
// text only after its own script runs. Nothing else separates "the server
// refused us and we rendered the refusal" from "this page is genuinely empty",
// so both end up rc 0 with an empty stderr and the same sweep label. Six of the
// eight zero-output pages of GOOSIE-LIVE-001 are this branch, measured with the
// client the browser itself uses: 403 / 5,581 B apnews.com, 403 / 771 B
// nytimes.com, 403 / 5,613 B japantimes.co.jp, 401 / 774 B reuters.com,
// 403 / 779 B etsy.com, 403 / 5,607 B newegg.com. The media path already
// refuses a non-200 answer (internal/engine/media_download.go); the document
// path renders it, so the status is the one fact that makes the two cases
// tellable apart. Like the line above it, this is a report written after the
// paint - it changes nothing about what is rendered.
func reportRefusedDocument(w io.Writer, resp *net.Response) {
	fmt.Fprintf(w, "goosie: the source refused %s with HTTP %d; rendered %d bytes of that refusal, not the page\n",
		resp.URL, resp.StatusCode, len(resp.Body))
}

var errDownloadSaveCanceled = errors.New("goosie: download save dialog canceled")

func isHTTPURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
