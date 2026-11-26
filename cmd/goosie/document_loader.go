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

type historySeed struct {
	URLs         []string
	States       [][]byte
	SameDocument []bool
	Index        int
	Window       windowSeed
}

func documentDeviceViewport(viewportW, viewportH int, scale float32) frame.Size {
	return frame.Size{
		W: int32(float64(viewportW)*float64(scale) + 0.5),
		H: int32(float64(viewportH)*float64(scale) + 0.5),
	}
}

func loadURLCtx(ctx context.Context, client net.HTTP, fonts *raster.Fonts, rawURL string, viewportW, viewportH int, scale float32, downloadDir string, deferImages, deferFonts bool, extra ...any) (*frame.Layer, paint.SceneSpec, frame.Color, *engine.Session, string, error) {
	if err := engine.ValidateViewport(viewportW, viewportH, float64(scale)); err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: viewport: %w", err)
	}
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", ctx.Err()
	}
	fetchURL := rawURL
	targetURL, isViewSource := viewsource.IsViewSourceURL(rawURL)
	if isViewSource {
		fetchURL = targetURL
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
	var policy scriptPolicy
	var request navRequest
	var seed historySeed
	var window windowSeed
	var contexts *browsingContextRegistry
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
		case scriptPolicy:
			policy = v
		case navRequest:
			request = v
		case historySeed:
			seed = v
			window = v.Window
		case windowSeed:
			window = v
		case *browsingContextRegistry:
			contexts = v
		}
	}
	if window.OpaqueID == "" && contexts != nil {
		window.OpaqueID = contexts.newOpaqueID()
	}
	if permsMgr != nil {
		ctx = net.WithCookiePolicy(ctx, func(u *url.URL) bool {
			return permsMgr.IsAllowed(perms.NormalizeOrigin(u.String()), perms.PermissionCookies)
		})
	}
	fetchCtx := subresourceCtx(ctx, fetchURL)
	var resp *net.Response
	var stream *net.StreamResponse
	var err error
	if strings.EqualFold(request.Method, "POST") {
		resp, err = client.Post(fetchCtx, fetchURL, request.ContentType, strings.NewReader(request.Body))
	} else if streamer, ok := client.(net.StreamingHTTP); ok && isHTTPURL(fetchURL) {
		stream, err = streamer.GetStream(fetchCtx, fetchURL)
		if err == nil {
			resp = &net.Response{StatusCode: stream.StatusCode, Headers: stream.Headers, HeaderValues: stream.HeaderValues, URL: stream.URL}
		}
	} else {
		resp, err = client.Get(fetchCtx, fetchURL)
	}
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: fetch %s: %w", net.RedactURL(fetchURL), err)
	}
	if stream != nil {
		defer stream.Close()
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
			engine.WithDevicePixelRatio(scale),
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
		extent, err := sess.DocumentLayerBounds(dl.Extent(), scale, documentDeviceViewport(viewportW, viewportH, scale))
		if err != nil {
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: size view-source document bounds: %w", err)
		}
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
				engine.WithDevicePixelRatio(scale),
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
			extent, err := sess.DocumentLayerBounds(dl.Extent(), scale, documentDeviceViewport(viewportW, viewportH, scale))
			if err != nil {
				return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: size pdf document bounds: %w", err)
			}
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
			downloadCtx, cancel := context.WithCancelCause(ctx)
			var itemID string
			if mgr != nil {
				it := mgr.CreateItem(resp.URL, name, stream.ContentLength)
				itemID = it.ID
				mgr.SetCancel(itemID, func() {
					cancel(errDownloadCanceled)
					stream.Cancel()
				})
				mgr.SetPause(itemID, func() {
					cancel(download.ErrPaused)
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
			var result download.StreamResult
			var saveErr error
			if targetPath != "" {
				result, saveErr = download.SaveStreamToPausable(downloadCtx, targetPath, streamBody, progress)
			} else {
				result, saveErr = download.SaveStreamPausable(downloadCtx, downloadDir, name, streamBody, progress)
			}
			cancel(nil)
			if errors.Is(saveErr, download.ErrPaused) {
				if mgr != nil && itemID != "" {
					plan := &resumeDownloadPlan{
						mgr:        mgr,
						client:     client,
						itemID:     itemID,
						url:        resp.URL,
						tempPath:   result.TempPath,
						received:   result.Bytes,
						targetPath: targetPath,
						dir:        downloadDir,
						name:       name,
					}
					plan.register()
				}
				return nil, paint.SceneSpec{}, frame.Color(0), nil, "", downloadPaused{id: itemID}
			}
			savedPath := result.Path
			downloaded := result.Bytes
			digest := result.Digest
			if saveErr != nil {
				if mgr != nil && itemID != "" {
					if errors.Is(saveErr, context.Canceled) {
						mgr.Cancel(itemID)
					} else if it := mgr.Get(itemID); it != nil && it.Status != download.StatusCanceled {
						mgr.Fail(itemID, saveErr)
					}
				}
				return nil, paint.SceneSpec{}, frame.Color(0), nil, "", saveErr
			}
			if mgr != nil && itemID != "" {
				mgr.UpdateProgress(itemID, downloaded)
				mgr.CompleteWithDigest(itemID, savedPath, digest.String())
				if it := mgr.Get(itemID); it == nil || it.Status != download.StatusCompleted {
					_ = os.Remove(savedPath)
					return nil, paint.SceneSpec{}, frame.Color(0), nil, "", context.Canceled
				}
			}
			return nil, paint.SceneSpec{}, frame.Color(0), nil, "", downloadDone{path: savedPath, url: resp.URL, contentType: contentType, contentDisposition: contentDisposition}
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
			mgr.CompleteWithDigest(itemID, savedPath, download.Sum(resp.Body).String())
		}
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", downloadDone{path: savedPath, url: resp.URL, contentType: contentType, contentDisposition: contentDisposition}
	}
	csp := documentCSP(resp)
	cssCtx, cssCancel := context.WithTimeout(ctx, engine.CSSFetchBudget)
	defer cssCancel()
	imgCtx := ctx
	if !deferImages {
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
		resolvedURL := resolveHref(base, url)
		if !csp.AllowsScript(resolvedURL, resp.URL) {
			return nil, fmt.Errorf("goosie: script blocked by Content-Security-Policy")
		}
		resp, err := client.Get(subresourceCtx(scriptCtx, base), resolvedURL)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	jsFetch := jsTransport{client: client, ctx: subresourceCtx(ctx, resp.URL), csp: csp, documentURL: resp.URL}
	linker := func(base, href string) (string, error) {
		resolvedURL := resolveHref(base, href)
		if !csp.AllowsStyle(resolvedURL, resp.URL) {
			return "", fmt.Errorf("goosie: stylesheet blocked by Content-Security-Policy")
		}
		resp, err := client.Get(subresourceCtx(cssCtx, base), resolvedURL)
		if err != nil {
			return "", err
		}
		if refused := resp.Headers["Content-Type"]; net.StyleSheetRefused(refused) {
			return "", fmt.Errorf("goosie: %s is served as %q, not as a style sheet", net.RedactURL(href), refused)
		}
		return resp.Text(), nil
	}
	imageFetcher := func(base, url string) ([]byte, error) {
		if !imgAllowed {
			return nil, fmt.Errorf("goosie: image loading blocked by site permissions")
		}
		resolvedURL := resolveHref(base, url)
		if !csp.AllowsImage(resolvedURL, resp.URL) {
			return nil, fmt.Errorf("goosie: image loading blocked by Content-Security-Policy")
		}
		resp, err := client.Get(subresourceCtx(imgCtx, base), resolvedURL)
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
		if !csp.AllowsFrame(frameURL.String(), resp.URL) {
			return nil, fmt.Errorf("goosie: iframe blocked by Content-Security-Policy")
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
		if !csp.AllowsFrame(formURL.String(), resp.URL) {
			return nil, fmt.Errorf("goosie: iframe form blocked by Content-Security-Policy")
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
	fontFetcher := func(base, url string) (engine.FontResult, error) {
		resolvedURL := resolveHref(base, url)
		if !csp.AllowsFont(resolvedURL, resp.URL) {
			return engine.FontResult{}, fmt.Errorf("goosie: font blocked by Content-Security-Policy")
		}
		resp, err := client.Get(subresourceCtx(fontCtx, base), resolvedURL)
		if err != nil {
			return engine.FontResult{}, err
		}
		return engine.FontResult{Data: resp.Body, Headers: resp.Headers}, nil
	}
	if ctx.Err() != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", ctx.Err()
	}
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
	if jsAllowed && !policy.disabled {
		var rtErr error
		rt, rtErr = js.New(js.Options{
			URL:                      resp.URL,
			HistoryURLs:              seed.URLs,
			HistoryStates:            seed.States,
			HistoryIndex:             seed.Index,
			BroadcastHub:             broadcastHub,
			HTTPClient:               jsFetch,
			Cookies:                  cookieStore,
			Storage:                  storageReg,
			ClipboardProvider:        clipboardProvider,
			ClipboardPermissionState: clipboardPermissionState,
			SessionID:                sessionID,
			WindowHandle:             window.Handle,
			WindowName:               window.Name,
			WindowOrigin:             window.Origin,
			WindowOpaqueID:           window.OpaqueID,
			OpenerHandle:             window.OpenerHandle,
			OpenerName:               window.OpenerName,
			DarkMode:                 appMode.dark,
			HighContrast:             appMode.highContrast,
			Timeout:                  policy.timeout,
			CSP:                      csp,
		})
		if contexts != nil {
			rt.SetWindowMessageSink(contexts.deliver)
			rt.SetWindowClosed(contexts.closed)
			rt.SetWindowNameSink(contexts.rename)
		}
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
		engine.WithDevicePixelRatio(scale),
		engine.WithLinkedCSS(resp.URL, linker),
		engine.WithScripts(resp.URL, scriptFetcher),
		engine.WithIFrameDocumentLoader(iframeFetcher),
		engine.WithIFrameFormSubmitter(iframeFormSubmitter),
		engine.WithJS(rt),
		engine.WithDarkMode(appMode.dark),
		engine.WithHighContrast(appMode.highContrast),
		engine.WithInvalidation(&engine.Invalidation{}),
		imageOption(deferImages, resp.URL, imageFetcher),
		fontOption(deferFonts, resp.URL, fontFetcher, fonts),
	}
	if userscriptMgr != nil && jsAllowed && !policy.disabled {
		start := userscriptMgr.MatchingScripts(resp.URL, userscript.RunAtDocumentStart)
		end := userscriptMgr.MatchingScripts(resp.URL, userscript.RunAtDocumentEnd)
		matched := append(start, end...)
		if len(matched) > 0 {
			sessOpts = append(sessOpts, engine.WithUserScripts(matched))
		}
	}
	if mediaBackend != nil && client != nil {
		fetchMedia := mediaFetcher(client, resp.URL)
		guardedMedia := engine.MediaFetcher(func(ctx context.Context, source string) (engine.MediaResponse, error) {
			resolvedURL := resolveHref(resp.URL, source)
			if !csp.AllowsMedia(resolvedURL, resp.URL) {
				return engine.MediaResponse{}, fmt.Errorf("goosie: media blocked by Content-Security-Policy")
			}
			return fetchMedia(ctx, resolvedURL)
		})
		sessOpts = append(sessOpts, engine.WithMedia(mediaBackend, guardedMedia, resp.URL))
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
	extent, err := sess.DocumentLayerBounds(dl.Extent(), scale, documentDeviceViewport(viewportW, viewportH, scale))
	if err != nil {
		return nil, paint.SceneSpec{}, frame.Color(0), nil, "", fmt.Errorf("goosie: size document bounds: %w", err)
	}
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

func documentCSP(resp *net.Response) *js.CSPPolicy {
	if resp == nil {
		return nil
	}
	var policies []*js.CSPPolicy
	for _, header := range responseHeaderValues(resp, "Content-Security-Policy") {
		if strings.TrimSpace(header) != "" {
			policies = append(policies, js.ParseCSP(header))
		}
	}
	return js.ComposeCSP(policies...)
}

func responseHeaderValues(resp *net.Response, name string) []string {
	for key, values := range resp.HeaderValues {
		if strings.EqualFold(key, name) {
			return values
		}
	}
	for key, value := range resp.Headers {
		if strings.EqualFold(key, name) && value != "" {
			return []string{value}
		}
	}
	return nil
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

func reportScriptErrors(w io.Writer, rawURL string, sess *engine.Session) {
	total := sess.ScriptErrorCount()
	errs := sess.ScriptErrors()
	if total == 0 || len(errs) == 0 {
		return
	}
	fmt.Fprintf(w, "goosie: %d uncaught script error(s) in %s; first: %s\n",
		total, rawURL, errs[0])
}

func reportRefusedDocument(w io.Writer, resp *net.Response) {
	fmt.Fprintf(w, "goosie: the source refused %s with HTTP %d; rendered %d bytes of that refusal, not the page\n",
		resp.URL, resp.StatusCode, len(resp.Body))
}

var errDownloadSaveCanceled = errors.New("goosie: download save dialog canceled")

func isHTTPURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
