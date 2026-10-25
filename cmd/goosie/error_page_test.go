package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/raster"
)

// qc:BR-012
func TestCategorizeError(t *testing.T) {
	tests := []struct {
		err         error
		wantCode    string
		wantHeading string
	}{
		{
			err:         errors.New("dial tcp: lookup non-existent-website.test: no such host"),
			wantCode:    "ERR_NAME_NOT_RESOLVED",
			wantHeading: "Server Not Found",
		},
		{
			err:         errors.New("dial tcp 127.0.0.1:9999: connect: connection refused"),
			wantCode:    "ERR_CONNECTION_REFUSED",
			wantHeading: "Unable to Connect",
		},
		{
			err:         errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"),
			wantCode:    "ERR_CERT_COMMON_NAME_INVALID",
			wantHeading: "Your Connection is Not Private",
		},
		{
			err:         errors.New("context deadline exceeded (Client.Timeout or fetch budget exceeded)"),
			wantCode:    "ERR_CONNECTION_TIMED_OUT",
			wantHeading: "Connection Timed Out",
		},
		{
			err:         errors.New("dial tcp: connect: network is unreachable"),
			wantCode:    "ERR_INTERNET_DISCONNECTED",
			wantHeading: "No Internet Connection",
		},
		{
			err:         errors.New("net: request blocked by ad/tracker filter: tracker.com"),
			wantCode:    "ERR_BLOCKED_BY_CLIENT",
			wantHeading: "Blocked by Shield",
		},
		{
			err:         errors.New("some generic network pipe broken error"),
			wantCode:    "ERR_CONNECTION_FAILED",
			wantHeading: "This Site Can’t Be Reached",
		},
	}

	for _, tt := range tests {
		code, heading, summary := categorizeError(tt.err)
		if code != tt.wantCode {
			t.Errorf("categorizeError(%v) code = %q, want %q", tt.err, code, tt.wantCode)
		}
		if heading != tt.wantHeading {
			t.Errorf("categorizeError(%v) heading = %q, want %q", tt.err, heading, tt.wantHeading)
		}
		if summary == "" {
			t.Errorf("categorizeError(%v) summary is empty", tt.err)
		}
	}
}

func TestFormatErrorPageHTMLSanitization(t *testing.T) {
	maliciousURL := "https://example.com/<script>alert(1)</script>"
	maliciousErr := errors.New("error with <b>bold</b> & special characters")

	htmlContent := formatErrorPageHTML(maliciousURL, maliciousErr)
	if strings.Contains(htmlContent, "<script>") {
		t.Fatal("HTML should escape script tags in URL")
	}
	if !strings.Contains(htmlContent, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatal("HTML should contain properly escaped URL")
	}
	if strings.Contains(htmlContent, "<b>bold</b>") {
		t.Fatal("HTML should escape tags in error message")
	}
	if !strings.Contains(htmlContent, "Try Again") {
		t.Fatal("HTML should contain Try Again action button")
	}
	if !strings.Contains(htmlContent, "Diagnostics") {
		t.Fatal("HTML should contain Diagnostics link to goosie://version")
	}
}

func TestBuildErrorPageRender(t *testing.T) {
	fonts, err := raster.NewFonts()
	if err != nil {
		t.Skipf("fonts unavailable: %v", err)
	}

	f := newNavTestPath(t)
	f.fonts = fonts
	f.config = config{width: 800, height: 600, dpr: 1}

	targetErr := errors.New("dial tcp: lookup fail.example.com: no such host")
	layer, sess, bg := f.buildErrorPage("https://fail.example.com", targetErr)
	if layer == nil {
		t.Fatal("buildErrorPage produced nil layer")
	}
	if sess == nil {
		t.Fatal("buildErrorPage produced nil session")
	}
	if bg == 0 {
		t.Fatal("buildErrorPage produced zero background color")
	}

	// Apply nav result with error to active tab
	tab := f.tabMgr.Active()
	tab.Nav.Serial = 10
	f.applyNavResult(navResult{
		tabID:  tab.ID(),
		serial: 10,
		url:    "https://fail.example.com",
		err:    targetErr,
	})

	if tab.Error() != targetErr.Error() {
		t.Errorf("tab.Error() = %q, want %q", tab.Error(), targetErr.Error())
	}
	if tab.Layer() == nil {
		t.Error("tab.Layer() should not be nil after error page is applied")
	}
	if tab.Title() != "Problem loading page" {
		t.Errorf("tab.Title() = %q, want %q", tab.Title(), "Problem loading page")
	}
	if f.sched.PlanStats().Published == 0 {
		t.Error("scheduler plan was not published for error page")
	}
}
