package navigation

import (
	"testing"
)

func TestCheckFileAccess_NonFileTarget(t *testing.T) {
	tests := []struct {
		current string
		target  string
	}{
		{"https://example.com", "https://other.com/page"},
		{"", "https://example.com"},
		{"file:///local/page.html", "https://example.com"},
		{"about:blank", "http://example.com"},
		{"https://example.com", "data:text/html,hello"},
	}
	for _, tt := range tests {
		err := CheckFileAccess(tt.current, tt.target)
		if err != nil {
			t.Errorf("CheckFileAccess(%q, %q) = %v, want nil", tt.current, tt.target, err)
		}
	}
}

func TestCheckFileAccess_FileTargetFromEmptyCurrent(t *testing.T) {
	// Initial navigation (no current page) should allow file:// URLs.
	err := CheckFileAccess("", "file:///etc/passwd")
	if err != nil {
		t.Fatalf("initial navigation to file:// should be allowed: %v", err)
	}
}

func TestCheckFileAccess_FileTargetFromFileOrigin(t *testing.T) {
	// Same-scheme: file:// pages can navigate to other file:// URLs.
	err := CheckFileAccess("file:///home/user/page.html", "file:///home/user/other.html")
	if err != nil {
		t.Fatalf("same-scheme file:// navigation should be allowed: %v", err)
	}
}

func TestCheckFileAccess_FileTargetFromHTTP(t *testing.T) {
	err := CheckFileAccess("https://example.com", "file:///etc/passwd")
	if err != ErrFileAccessDenied {
		t.Fatalf("file:// from https origin should be denied, got: %v", err)
	}
}

func TestCheckFileAccess_FileTargetFromHTTPWithPort(t *testing.T) {
	err := CheckFileAccess("http://localhost:8080/page", "file:///etc/shadow")
	if err != ErrFileAccessDenied {
		t.Fatalf("file:// from http origin should be denied, got: %v", err)
	}
}

func TestCheckFileAccess_FileTargetFromHTTPS(t *testing.T) {
	err := CheckFileAccess("https://evil.com", "file:///etc/passwd")
	if err != ErrFileAccessDenied {
		t.Fatalf("file:// from https origin should be denied, got: %v", err)
	}
}

func TestCheckFileAccess_FileTargetFromOpaqueOrigin(t *testing.T) {
	// data: and about: URLs produce opaque origins. file:// access from
	// these should be blocked (they are not local file schemes).
	err := CheckFileAccess("data:text/html,<script>malicious</script>", "file:///etc/passwd")
	if err != ErrFileAccessDenied {
		t.Fatalf("file:// from data: origin should be denied, got: %v", err)
	}

	err = CheckFileAccess("about:blank", "file:///etc/passwd")
	if err != ErrFileAccessDenied {
		t.Fatalf("file:// from about: origin should be denied, got: %v", err)
	}
}

func TestCheckFileAccess_FileTargetFromBlob(t *testing.T) {
	err := CheckFileAccess("blob:https://example.com/uuid", "file:///etc/passwd")
	if err != ErrFileAccessDenied {
		t.Fatalf("file:// from blob: origin should be denied, got: %v", err)
	}
}

func TestCheckFileAccess_TargetParseError(t *testing.T) {
	// Parse errors should be treated conservatively — allow the navigation.
	err := CheckFileAccess("https://example.com", "://invalid")
	if err != nil {
		t.Fatalf("malformed target URL should be allowed, got: %v", err)
	}
}

func TestCheckFileAccess_CurrentParseError(t *testing.T) {
	// Parse errors for the current URL should be treated conservatively.
	err := CheckFileAccess("://invalid", "file:///etc/passwd")
	if err != nil {
		t.Fatalf("malformed current URL should not cause denial, got: %v", err)
	}
}

func TestCheckFileAccess_FileTargetFromEmptyScheme(t *testing.T) {
	// URL without scheme (e.g., relative URL) — target.Scheme is "",
	// so this is not a file:// target.
	err := CheckFileAccess("https://example.com", "/relative/path")
	if err != nil {
		t.Fatalf("relative URL should be allowed, got: %v", err)
	}
}

func TestCheckFileAccess_FileTargetFromIPAddress(t *testing.T) {
	err := CheckFileAccess("https://127.0.0.1", "file:///etc/passwd")
	if err != ErrFileAccessDenied {
		t.Fatalf("file:// from IP-based origin should be denied, got: %v", err)
	}
}

func TestCheckFileAccess_FileTargetFromLocalhost(t *testing.T) {
	err := CheckFileAccess("http://localhost:3000", "file:///etc/passwd")
	if err != ErrFileAccessDenied {
		t.Fatalf("file:// from localhost origin should be denied, got: %v", err)
	}
}

func TestCheckFileAccess_ErrorIs(t *testing.T) {
	err := CheckFileAccess("https://example.com", "file:///etc/passwd")
	if err != ErrFileAccessDenied {
		t.Fatal("expected ErrFileAccessDenied")
	}
}

func BenchmarkCheckFileAccess_Allowed(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		CheckFileAccess("https://example.com", "https://other.com")
	}
}

func BenchmarkCheckFileAccess_Denied(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		CheckFileAccess("https://example.com", "file:///etc/passwd")
	}
}

func BenchmarkCheckFileAccess_SameScheme(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		CheckFileAccess("file:///home/page.html", "file:///home/other.html")
	}
}

func BenchmarkCheckFileAccess_EmptyCurrent(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		CheckFileAccess("", "file:///etc/passwd")
	}
}
