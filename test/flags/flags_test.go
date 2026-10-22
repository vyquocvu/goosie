package flags_test

import (
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/flags"
)

func TestFlagsManager_DefaultsAndQueries(t *testing.T) {
	mgr := flags.NewManager("")
	if !mgr.IsEnabled("smooth-scrolling") {
		t.Errorf("expected smooth-scrolling default enabled")
	}
	if mgr.IsEnabled("doh-dns") {
		t.Errorf("expected doh-dns default disabled")
	}
	if mgr.IsEnabled("non-existent-flag") {
		t.Errorf("expected non-existent flag disabled")
	}

	if mgr.HasNonDefault() {
		t.Errorf("expected no non-defaults initially")
	}

	mgr.SetEnabled("doh-dns", true)
	if !mgr.IsEnabled("doh-dns") {
		t.Errorf("expected doh-dns enabled after SetEnabled")
	}
	if !mgr.HasNonDefault() {
		t.Errorf("expected HasNonDefault() = true after modifying flag")
	}

	mgr.ResetAll()
	if mgr.IsEnabled("doh-dns") {
		t.Errorf("expected doh-dns disabled after ResetAll")
	}
	if mgr.HasNonDefault() {
		t.Errorf("expected HasNonDefault() = false after ResetAll")
	}
}

func TestFlagsManager_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "flags.json")

	mgr1 := flags.NewManager(filePath)
	mgr1.SetEnabled("doh-dns", true)
	mgr1.SetEnabled("smooth-scrolling", false)

	if err := mgr1.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	mgr2 := flags.NewManager(filePath)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if !mgr2.IsEnabled("doh-dns") {
		t.Errorf("expected loaded doh-dns = true")
	}
	if mgr2.IsEnabled("smooth-scrolling") {
		t.Errorf("expected loaded smooth-scrolling = false")
	}
}
