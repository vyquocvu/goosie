package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

// qc:BR-120
func TestCORSOriginMatching(t *testing.T) {
	origin := "https://foo.example.com"

	resp1 := map[string]string{"access-control-allow-origin": "*"}
	if !js.CheckCORSResponse(origin, resp1, false) {
		t.Errorf("wildcard without credentials should pass")
	}

	respWithWildcard := map[string]string{
		"Access-Control-Allow-Origin":      "*",
		"Access-Control-Allow-Credentials": "true",
	}
	if js.CheckCORSResponse(origin, respWithWildcard, true) {
		t.Errorf("wildcard '*' with credentials MUST be rejected by CORS spec")
	}

	respWithExact := map[string]string{
		"Access-Control-Allow-Origin":      "https://foo.example.com",
		"Access-Control-Allow-Credentials": "true",
	}
	if !js.CheckCORSResponse(origin, respWithExact, true) {
		t.Errorf("exact origin match with credentials: true must pass")
	}

	respMissingCreds := map[string]string{
		"Access-Control-Allow-Origin": "https://foo.example.com",
	}
	if js.CheckCORSResponse(origin, respMissingCreds, true) {
		t.Errorf("request with credentials requires Access-Control-Allow-Credentials: true")
	}

	respMismatched := map[string]string{
		"Access-Control-Allow-Origin": "https://other.com",
	}
	if js.CheckCORSResponse(origin, respMismatched, false) {
		t.Errorf("mismatched origin must fail")
	}
}

// qc:BR-120
func TestCORSPreflightResponseValidation(t *testing.T) {
	origin := "https://app.example.com"

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

	if js.CheckPreflightResponse(origin, "PUT", reqHeaders, respHeaders, false) {
		t.Errorf("preflight should fail when PUT is not in allow-methods")
	}

	unlistedHeaders := map[string]string{
		"X-Unlisted-Header": "value",
	}
	if js.CheckPreflightResponse(origin, "DELETE", unlistedHeaders, respHeaders, false) {
		t.Errorf("preflight should fail when non-simple header is not in allow-headers")
	}

	wildcardResp := map[string]string{
		"access-control-allow-origin":  origin,
		"access-control-allow-methods": "*",
		"access-control-allow-headers": "*",
	}
	if !js.CheckPreflightResponse(origin, "PATCH", unlistedHeaders, wildcardResp, false) {
		t.Errorf("wildcard allow-methods and allow-headers should allow any method and header")
	}
}
