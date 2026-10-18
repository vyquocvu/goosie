package js_test

import (
	"os"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestLocalStorageOriginScoping(t *testing.T) {
	reg := js.NewOriginStorageRegistry("", false)

	// Runtime 1: https://example.com/page1
	rt1, err := js.New(js.Options{
		URL:     "https://example.com/page1",
		Storage: reg,
	})
	if err != nil {
		t.Fatalf("New rt1 failed: %v", err)
	}
	defer rt1.Close()

	// Runtime 2: https://example.com/page2 (same origin)
	rt2, err := js.New(js.Options{
		URL:     "https://example.com/page2",
		Storage: reg,
	})
	if err != nil {
		t.Fatalf("New rt2 failed: %v", err)
	}
	defer rt2.Close()

	// Runtime 3: https://other.org/page (different origin)
	rt3, err := js.New(js.Options{
		URL:     "https://other.org/page",
		Storage: reg,
	})
	if err != nil {
		t.Fatalf("New rt3 failed: %v", err)
	}
	defer rt3.Close()

	// Set item in rt1
	if err := rt1.Run("localStorage.setItem('auth_token', 'secret123');", "test"); err != nil {
		t.Fatalf("rt1 setItem failed: %v", err)
	}

	// Verify rt2 (same origin) can read it
	v2, err := rt2.Evaluate("localStorage.getItem('auth_token')")
	if err != nil {
		t.Fatalf("rt2 getItem failed: %v", err)
	}
	if v2 != "secret123" {
		t.Errorf("rt2 expected 'secret123', got %q", v2)
	}

	// Verify rt3 (different origin) cannot read it (must be null)
	v3, err := rt3.Evaluate("localStorage.getItem('auth_token')")
	if err != nil {
		t.Fatalf("rt3 getItem failed: %v", err)
	}
	if v3 != "null" {
		t.Errorf("rt3 expected null, got %q", v3)
	}
}

func TestLocalStorageDiskPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "goosie-storage-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	reg1 := js.NewOriginStorageRegistry(tempDir, false)

	// Session 1: write items to localStorage
	rt1, err := js.New(js.Options{
		URL:     "https://shop.example.com/cart",
		Storage: reg1,
	})
	if err != nil {
		t.Fatalf("New rt1 failed: %v", err)
	}

	if err := rt1.Run(`
		localStorage.setItem('item1', 'shoes');
		localStorage.setItem('item2', 'shirt');
	`, "test"); err != nil {
		t.Fatalf("rt1 Run failed: %v", err)
	}
	_ = rt1.Close()

	// Verify file was written to disk
	files, err := os.ReadDir(tempDir)
	if err != nil || len(files) == 0 {
		t.Fatalf("expected storage file written to %s, got %v", tempDir, err)
	}

	// Session 2: simulate browser restart by creating new registry on same directory
	reg2 := js.NewOriginStorageRegistry(tempDir, false)
	rt2, err := js.New(js.Options{
		URL:     "https://shop.example.com/checkout",
		Storage: reg2,
	})
	if err != nil {
		t.Fatalf("New rt2 failed: %v", err)
	}
	defer rt2.Close()

	vItem1, err := rt2.Evaluate("localStorage.getItem('item1')")
	if err != nil {
		t.Fatalf("Evaluate item1 failed: %v", err)
	}
	if vItem1 != "shoes" {
		t.Errorf("expected 'shoes', got %q", vItem1)
	}

	vLength, err := rt2.Evaluate("localStorage.length")
	if err != nil {
		t.Fatalf("Evaluate length failed: %v", err)
	}
	if vLength != "2" {
		t.Errorf("expected length 2, got %v", vLength)
	}
}

func TestSessionStorageTabIsolation(t *testing.T) {
	reg := js.NewOriginStorageRegistry("", false)

	// Tab 1
	rt1, err := js.New(js.Options{
		URL:       "https://app.example.com/home",
		Storage:   reg,
		SessionID: "tab-1",
	})
	if err != nil {
		t.Fatalf("New rt1 failed: %v", err)
	}
	defer rt1.Close()

	// Tab 2 (same origin, different session ID)
	rt2, err := js.New(js.Options{
		URL:       "https://app.example.com/dashboard",
		Storage:   reg,
		SessionID: "tab-2",
	})
	if err != nil {
		t.Fatalf("New rt2 failed: %v", err)
	}
	defer rt2.Close()

	if err := rt1.Run("sessionStorage.setItem('scroll_y', '500');", "test"); err != nil {
		t.Fatalf("rt1 setItem failed: %v", err)
	}

	// Tab 1 sees it
	v1, _ := rt1.Evaluate("sessionStorage.getItem('scroll_y')")
	if v1 != "500" {
		t.Errorf("tab 1 expected '500', got %q", v1)
	}

	// Tab 2 does NOT see it (sessionStorage is tab-isolated)
	v2, _ := rt2.Evaluate("sessionStorage.getItem('scroll_y')")
	if v2 != "null" {
		t.Errorf("tab 2 expected null for isolated sessionStorage, got %q", v2)
	}
}

func TestIncognitoStorageNoDisk(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "goosie-incognito-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Incognito registry
	reg := js.NewOriginStorageRegistry(tempDir, true)
	rt, err := js.New(js.Options{
		URL:     "https://private.example.com/",
		Storage: reg,
	})
	if err != nil {
		t.Fatalf("New rt failed: %v", err)
	}
	defer rt.Close()

	if err := rt.Run("localStorage.setItem('temp', 'data');", "test"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify nothing was written to tempDir
	entries, _ := os.ReadDir(tempDir)
	if len(entries) != 0 {
		t.Errorf("Incognito storage must not write files to disk, found: %v", entries)
	}
}
