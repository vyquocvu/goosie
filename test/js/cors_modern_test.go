package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestCORSOriginMatching(t *testing.T) {
	origin := "https://foo.example.com"

	// 1. Wildcard allow origin without credentials
	resp1 := map[string]string{"access-control-allow-origin": "*"}
	if !js.CheckCORSResponse(origin, resp1, false) {
		t.Errorf("wildcard without credentials should pass")
	}

	// 2. Wildcard allow origin WITH credentials must fail
	respWithWildcard := map[string]string{
		"Access-Control-Allow-Origin":      "*",
		"Access-Control-Allow-Credentials": "true",
	}
	if js.CheckCORSResponse(origin, respWithWildcard, true) {
		t.Errorf("wildcard '*' with credentials MUST be rejected by CORS spec")
	}

	// 3. Exact match with credentials
	respWithExact := map[string]string{
		"Access-Control-Allow-Origin":      "https://foo.example.com",
		"Access-Control-Allow-Credentials": "true",
	}
	if !js.CheckCORSResponse(origin, respWithExact, true) {
		t.Errorf("exact origin match with credentials: true must pass")
	}

	// 4. Exact match with credentials missing allow-credentials header
	respMissingCreds := map[string]string{
		"Access-Control-Allow-Origin": "https://foo.example.com",
	}
	if js.CheckCORSResponse(origin, respMissingCreds, true) {
		t.Errorf("request with credentials requires Access-Control-Allow-Credentials: true")
	}

	// 5. Mismatched origin
	respMismatched := map[string]string{
		"Access-Control-Allow-Origin": "https://other.com",
	}
	if js.CheckCORSResponse(origin, respMismatched, false) {
		t.Errorf("mismatched origin must fail")
	}
}

func TestCORSPreflightResponseValidation(t *testing.T) {
	origin := "https://app.example.com"

	// 1. Successful preflight with custom method (DELETE) and custom header (X-User-Token)
	reqHeaders := map[string]string{
		"Content-Type": "application/json",
		"X-User-Token": "secret123",
	}
	respHeaders := map[string]string{
		"access-control-allow-origin":  origin,
		"access-control-allow-methods": "GET, POST, DELETE, OPTIONS",
		"access-control-allow-headers": "Content-Type, X-User-Token",
	}
	if !js.CheckPreflightResponse(origin, "DELETE", reqHeaders, respHeaders, false) {
		t.Errorf("valid preflight response should pass")
	}

	// 2. Preflight rejecting method
	if js.CheckPreflightResponse(origin, "PUT", reqHeaders, respHeaders, false) {
		t.Errorf("preflight should fail when PUT is not in allow-methods")
	}

	// 3. Preflight rejecting unlisted non-simple header
	unlistedHeaders := map[string]string{
		"X-Unlisted-Header": "value",
	}
	if js.CheckPreflightResponse(origin, "DELETE", unlistedHeaders, respHeaders, false) {
		t.Errorf("preflight should fail when non-simple header is not in allow-headers")
	}

	// 4. Wildcard allow-headers and allow-methods
	wildcardResp := map[string]string{
		"access-control-allow-origin":  origin,
		"access-control-allow-methods": "*",
		"access-control-allow-headers": "*",
	}
	if !js.CheckPreflightResponse(origin, "PATCH", unlistedHeaders, wildcardResp, false) {
		t.Errorf("wildcard allow-methods and allow-headers should allow any method and header")
	}
}
