package main

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/media"
	"github.com/vyquocvu/goosie/internal/raster"
)

func TestDocumentCSPBlocksAndAllowsResourceFetches(t *testing.T) {
	const documentURL = "https://document.example/page"
	const sameOriginAPIURL = "https://document.example/api"
	const xhrURL = "https://api.example/xhr"
	const extraURL = "https://extra.example/data"
	client := &securityClient{responses: map[string]securityResponse{
		documentURL: cspDocumentResponse(
			[]string{
				"default-src 'self'; script-src 'unsafe-inline' https://scripts.example; connect-src 'self' https://api.example; frame-src 'none'; media-src 'none'",
				"default-src 'self'; script-src 'unsafe-inline' https://scripts.example https://extra.example; connect-src 'self' https://api.example https://extra.example; frame-src 'none'; media-src 'none'",
			},
			nil,
			`<html><head>
				<link rel="stylesheet" href="/allowed.css">
				<link rel="stylesheet" href="https://styles.example/blocked.css">
			</head><body>
				<img src="/allowed.png">
				<img src="https://images.example/blocked.png">
				<script src="/blocked.js"></script>
				<script src="https://scripts.example/allowed.js"></script>
				<script src="https://extra.example/blocked.js"></script>
				<iframe src="/blocked-frame.html"></iframe>
				<video id="media" src="/clip.mp4"></video>
				<script>
					fetch("/api").then(function(r) { document.body.setAttribute("data-fetch", String(r.status)); });
					fetch("https://extra.example/data").catch(function() { document.body.setAttribute("data-fetch-blocked", "blocked"); });
					var allowed = new XMLHttpRequest();
					allowed.open("GET", "https://api.example/xhr");
					allowed.onload = function() { document.body.setAttribute("data-xhr", allowed.responseText); };
					allowed.send();
					var blocked = new XMLHttpRequest();
					blocked.open("GET", "https://extra.example/xhr");
					blocked.onerror = function() { document.body.setAttribute("data-xhr-blocked", "blocked"); };
					blocked.send();
				</script>
			</body></html>`,
		),
		"https://document.example/allowed.css":        cspResourceResponse("text/css", "@font-face { font-family: boundary; src: url('/allowed.woff2') format('woff2'); } body { font-family: boundary; }"),
		"https://document.example/allowed.woff2":      cspResourceResponse("font/woff2", "font"),
		"https://document.example/allowed.png":        cspResourceResponse("image/png", "image"),
		"https://scripts.example/allowed.js":          cspResourceResponse("application/javascript", "document.body.setAttribute('data-script', 'yes');"),
		xhrURL:                                        cspResourceResponse("text/plain", "ok"),
		sameOriginAPIURL:                              cspResourceResponse("text/plain", "ok"),
		"https://document.example/blocked.js":         cspResourceResponse("application/javascript", "document.body.setAttribute('data-bad-script', 'yes');"),
		"https://extra.example/blocked.js":            cspResourceResponse("application/javascript", "document.body.setAttribute('data-bad-script', 'yes');"),
		"https://styles.example/blocked.css":          cspResourceResponse("text/css", "body { color: red }"),
		"https://images.example/blocked.png":          cspResourceResponse("image/png", "image"),
		"https://document.example/blocked-frame.html": cspResourceResponse("text/html", "<html><body>blocked frame</body></html>"),
		"https://extra.example/xhr":                   cspResourceResponse("text/plain", "blocked"),
		extraURL:                                      cspResourceResponse("text/plain", "blocked"),
		"https://document.example/clip.mp4":           cspResourceResponse("video/mp4", stubMP4Body),
	}}
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Fatal(err)
	}
	backend := &stubBackend{}
	_, _, _, session, _, err := loadURLCtx(context.Background(), client, fonts, documentURL, 800, 600, 1, t.TempDir(), false, false, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	deadline := time.Now().Add(3 * time.Second)
	for {
		session.PumpJS(8)
		body := session.Doc.Body
		if body != nil && body.GetAttribute("data-script") == "yes" && body.GetAttribute("data-fetch") == "200" && body.GetAttribute("data-xhr") == "ok" && body.GetAttribute("data-fetch-blocked") == "blocked" {
			break
		}
		if time.Now().After(deadline) {
			if body == nil {
				t.Fatal("CSP-controlled scripts and fetch/XHR did not settle: no body")
			}
			t.Fatalf("CSP-controlled scripts and fetch/XHR did not settle: script=%q fetch=%q xhr=%q fetch-blocked=%q xhr-blocked=%q api-requested=%v errors=%v", body.GetAttribute("data-script"), body.GetAttribute("data-fetch"), body.GetAttribute("data-xhr"), body.GetAttribute("data-fetch-blocked"), body.GetAttribute("data-xhr-blocked"), client.requested(sameOriginAPIURL), session.ScriptErrors())
		}
		runtime.Gosched()
	}
	for _, raw := range []string{
		"https://document.example/allowed.css",
		"https://document.example/allowed.woff2",
		"https://document.example/allowed.png",
		"https://scripts.example/allowed.js",
		xhrURL,
		sameOriginAPIURL,
	} {
		if !client.requested(raw) {
			t.Errorf("CSP-allowed resource %q was not fetched", raw)
		}
	}
	for _, raw := range []string{
		"https://document.example/blocked.js",
		"https://extra.example/blocked.js",
		"https://styles.example/blocked.css",
		"https://images.example/blocked.png",
		"https://document.example/blocked-frame.html",
		"https://extra.example/xhr",
		extraURL,
	} {
		if client.requested(raw) {
			t.Errorf("CSP-blocked resource %q was fetched", raw)
		}
	}

	controller := session.MediaController(videoElement(t, session))
	if controller == nil {
		t.Fatal("CSP fixture produced no media controller")
	}
	controller.Load()
	waitForSecurityCondition(t, func() bool {
		_ = session.PumpMedia()
		return controller.Snapshot().State == media.StateFailed
	}, "CSP-blocked media did not fail")
	if err := controller.Snapshot().Err; err == nil || !strings.Contains(err.Error(), "Content-Security-Policy") {
		t.Fatalf("blocked media error = %v", err)
	}
	if client.requested("https://document.example/clip.mp4") {
		t.Fatal("CSP-blocked media was fetched")
	}
}

func TestDocumentCSPAllowsResourcesAndReportOnlyDoesNotBlock(t *testing.T) {
	for _, tc := range []struct {
		name       string
		enforcing  []string
		reportOnly []string
	}{
		{
			name:      "enforcing",
			enforcing: []string{"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self'; img-src 'self'; font-src 'self'; frame-src 'self'; media-src 'self'; connect-src 'self'"},
		},
		{
			name:       "report-only",
			reportOnly: []string{"default-src 'none'"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const documentURL = "https://document.example/allowed"
			client := &securityClient{responses: map[string]securityResponse{
				documentURL: cspDocumentResponse(tc.enforcing, tc.reportOnly, `<html><head>
					<link rel="stylesheet" href="/allowed.css">
				</head><body>
					<img src="/allowed.png">
					<script src="/allowed.js"></script>
					<iframe src="/frame.html"></iframe>
					<video id="media" src="/clip.mp4"></video>
					<script>
						fetch("/api").then(function(r) { document.body.setAttribute("data-fetch", String(r.status)); });
					</script>
				</body></html>`),
				"https://document.example/allowed.css":   cspResourceResponse("text/css", "@font-face { font-family: boundary; src: url('/allowed.woff2') format('woff2'); } body { font-family: boundary; }"),
				"https://document.example/allowed.woff2": cspResourceResponse("font/woff2", "font"),
				"https://document.example/allowed.png":   cspResourceResponse("image/png", "image"),
				"https://document.example/allowed.js":    cspResourceResponse("application/javascript", "document.body.setAttribute('data-script', 'yes');"),
				"https://document.example/frame.html":    cspResourceResponse("text/html", "<html><body>frame</body></html>"),
				"https://document.example/api":           cspResourceResponse("text/plain", "ok"),
				"https://document.example/clip.mp4":      cspResourceResponse("video/mp4", stubMP4Body),
			}}
			fonts, err := raster.NewFonts()
			if err != nil {
				t.Fatal(err)
			}
			backend := &stubBackend{}
			_, _, _, session, _, err := loadURLCtx(context.Background(), client, fonts, documentURL, 800, 600, 1, t.TempDir(), false, false, backend)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			waitForSecurityCondition(t, func() bool {
				session.PumpJS(8)
				body := session.Doc.Body
				return body != nil && body.GetAttribute("data-script") == "yes" && body.GetAttribute("data-fetch") == "200"
			}, "allowed or report-only scripts and fetch did not settle")
			waitForSecurityCondition(t, func() bool {
				session.PumpJS(8)
				return client.requested("https://document.example/frame.html")
			}, "allowed or report-only iframe was not fetched")
			for _, raw := range []string{
				"https://document.example/allowed.css",
				"https://document.example/allowed.woff2",
				"https://document.example/allowed.png",
				"https://document.example/allowed.js",
				"https://document.example/api",
			} {
				if !client.requested(raw) {
					t.Errorf("%s policy did not fetch %q", tc.name, raw)
				}
			}
			controller := session.MediaController(videoElement(t, session))
			if controller == nil {
				t.Fatal("allowed media fixture produced no controller")
			}
			controller.Load()
			waitForSecurityCondition(t, func() bool {
				_ = session.PumpMedia()
				return backend.first() != nil
			}, "%s policy did not fetch allowed media", tc.name)
			if !client.requested("https://document.example/clip.mp4") {
				t.Fatalf("%s policy did not request the allowed media", tc.name)
			}
		})
	}
}

func cspDocumentResponse(enforcing, reportOnly []string, body string) securityResponse {
	values := map[string][]string{}
	headers := map[string]string{"Content-Type": "text/html; charset=utf-8"}
	if len(enforcing) > 0 {
		values["Content-Security-Policy"] = enforcing
		headers["Content-Security-Policy"] = strings.Join(enforcing, ", ")
	}
	if len(reportOnly) > 0 {
		values["Content-Security-Policy-Report-Only"] = reportOnly
		headers["Content-Security-Policy-Report-Only"] = strings.Join(reportOnly, ", ")
	}
	return securityResponse{headers: headers, headerValues: values, body: body}
}

func cspResourceResponse(contentType, body string) securityResponse {
	return securityResponse{
		headers: map[string]string{
			"Content-Type":                contentType,
			"Access-Control-Allow-Origin": "*",
		},
		body: body,
	}
}

func waitForSecurityCondition(t *testing.T, condition func() bool, message string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf(message, args...)
		}
		runtime.Gosched()
	}
}
