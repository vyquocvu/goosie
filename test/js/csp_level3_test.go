package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestCSPNonceVerification(t *testing.T) {
	policy := js.ParseCSP("script-src 'nonce-abc123XYZ'; style-src 'nonce-cssSecret'")

	if !policy.AllowsScriptWithNonce("https://evil.com/bad.js", "abc123XYZ", "https://example.com") {
		t.Errorf("script with matching nonce should be allowed")
	}
	if policy.AllowsScriptWithNonce("https://evil.com/bad.js", "wrongNonce", "https://example.com") {
		t.Errorf("script with wrong nonce should be rejected")
	}
	if !policy.AllowsInlineScriptWithNonce("abc123XYZ") {
		t.Errorf("inline script with valid nonce should be allowed")
	}
	if policy.AllowsInlineScriptWithNonce("random") {
		t.Errorf("inline script with invalid nonce should be rejected")
	}

	if !policy.AllowsStyleWithNonce("https://evil.com/style.css", "cssSecret", "https://example.com") {
		t.Errorf("style with valid nonce should be allowed")
	}
	if policy.AllowsStyleWithNonce("https://evil.com/style.css", "wrong", "https://example.com") {
		t.Errorf("style with wrong nonce should be rejected")
	}
	if !policy.AllowsInlineStyleWithNonce("cssSecret") {
		t.Errorf("inline style with valid nonce should be allowed")
	}
	if policy.AllowsInlineStyleWithNonce("invalid") {
		t.Errorf("inline style with invalid nonce should be rejected")
	}
}

func TestCSPSchemeSources(t *testing.T) {
	policy := js.ParseCSP("img-src data: https:; script-src blob: https://cdn.example.com")

	if !policy.AllowsImage("data:image/png;base64,iVBORw0KGgo...", "https://example.com") {
		t.Errorf("data: scheme should be allowed for img-src")
	}
	if !policy.AllowsImage("https://trusted.com/pic.png", "https://example.com") {
		t.Errorf("https: scheme should allow any https image")
	}
	if policy.AllowsImage("http://insecure.com/pic.png", "https://example.com") {
		t.Errorf("http: scheme should be refused when only data: and https: are permitted")
	}

	if !policy.AllowsScript("blob:https://example.com/uuid-here", "https://example.com") {
		t.Errorf("blob: scheme should be allowed for script-src")
	}
	if policy.AllowsScript("data:text/javascript,alert(1)", "https://example.com") {
		t.Errorf("data: scheme should NOT be allowed for script-src")
	}
}

func TestCSPHostPortAndPathMatching(t *testing.T) {
	policy := js.ParseCSP("connect-src https://*.example.com:8443/api/ http://localhost:3000")

	if !policy.AllowsConnect("https://api.example.com:8443/api/v1/users", "https://example.com") {
		t.Errorf("connect to matching subdomain, port, and path should be allowed")
	}
	if !policy.AllowsConnect("https://sub.deep.example.com:8443/api/v2/items", "https://example.com") {
		t.Errorf("deep subdomain should match *.example.com")
	}
	if policy.AllowsConnect("https://api.example.com:443/api/v1/users", "https://example.com") {
		t.Errorf("connect with mismatched port should be rejected")
	}
	if policy.AllowsConnect("https://api.example.com:8443/admin/secret", "https://example.com") {
		t.Errorf("connect with mismatched path prefix should be rejected")
	}
	if policy.AllowsConnect("http://api.example.com:8443/api/v1/users", "https://example.com") {
		t.Errorf("connect with mismatched scheme should be rejected")
	}
	if !policy.AllowsConnect("http://localhost:3000/data", "http://localhost") {
		t.Errorf("localhost:3000 should be allowed")
	}
}

func TestCSPDirectiveFallbackHierarchy(t *testing.T) {
	policy1 := js.ParseCSP("default-src https://trusted.com")
	if !policy1.AllowsFrame("https://trusted.com/frame.html", "https://example.com") {
		t.Errorf("frame-src should fall back to child-src then default-src")
	}
	if policy1.AllowsFrame("https://other.com/frame.html", "https://example.com") {
		t.Errorf("untrusted frame should be rejected by default-src")
	}

	policy2 := js.ParseCSP("default-src 'none'; child-src https://child.com; frame-src https://frame.com")
	if !policy2.AllowsFrame("https://frame.com/page", "https://example.com") {
		t.Errorf("frame-src should be allowed")
	}
	if policy2.AllowsFrame("https://child.com/page", "https://example.com") {
		t.Errorf("child-src should not allow frame when frame-src is explicitly defined")
	}

	policyWorker := js.ParseCSP("default-src 'none'; script-src https://scripts.com")
	if !policyWorker.AllowsWorker("https://scripts.com/worker.js", "https://example.com") {
		t.Errorf("worker-src should fall back to script-src if child-src is absent")
	}
	if policyWorker.AllowsWorker("https://other.com/worker.js", "https://example.com") {
		t.Errorf("worker-src should reject other domains")
	}
}

func TestCSPCompositionIntersectsEnforcingPolicies(t *testing.T) {
	first := js.ParseCSP("default-src 'self'; script-src https://scripts.example; connect-src https://api.example")
	second := js.ParseCSP("default-src 'none'; script-src https://scripts.example https://extra.example; connect-src https://api.example https://extra.example")
	policy := js.ComposeCSP(first, second)

	if !policy.AllowsScript("https://scripts.example/app.js", "https://document.example") {
		t.Error("a source allowed by both enforcing policies was rejected")
	}
	if policy.AllowsScript("https://extra.example/app.js", "https://document.example") {
		t.Error("a source allowed by only one enforcing policy was accepted")
	}
	if policy.AllowsScript("https://document.example/app.js", "https://document.example") {
		t.Error("default-src from one policy overrode script-src from another")
	}
	if !policy.AllowsConnect("https://api.example/data", "https://document.example") {
		t.Error("a connection allowed by both enforcing policies was rejected")
	}
	if policy.AllowsConnect("https://extra.example/data", "https://document.example") {
		t.Error("a connection allowed by only one enforcing policy was accepted")
	}
	if policy.AllowsImage("https://document.example/image.png", "https://document.example") {
		t.Error("default-src intersection did not apply to fetch directives")
	}
}

func TestCSPCompositionIgnoresReportOnlyPolicies(t *testing.T) {
	reportOnly := js.ParseCSP("default-src 'none'")
	reportOnly.ReportOnly = true
	policy := js.ComposeCSP(reportOnly, js.ParseCSP("img-src 'self'"))
	if !policy.AllowsImage("https://document.example/image.png", "https://document.example") {
		t.Error("report-only policy participated in enforcement")
	}
	if got := js.ComposeCSP(reportOnly); got != nil {
		t.Error("a report-only-only header produced an enforcing policy")
	}
}

func TestCSPSelfAllowsRelativeSources(t *testing.T) {
	policy := js.ParseCSP("default-src 'self'")
	if !policy.AllowsFrame("/frame.html", "https://document.example/page") {
		t.Error("a relative same-origin frame source was rejected")
	}
	if policy.AllowsFrame("https://other.example/frame.html", "https://document.example/page") {
		t.Error("a cross-origin frame source was accepted")
	}
}
