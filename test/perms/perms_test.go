package perms_test

import (
	"path/filepath"
	"testing"

	"github.com/vyquocvu/goosie/internal/perms"
)

func TestNormalizeOrigin(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com/page?query=1#frag", "https://example.com"},
		{"http://localhost:8080/app", "http://localhost:8080"},
		{"HTTPS://MYAPP.IO:443/", "https://myapp.io:443"},
		{"", ""},
	}

	for _, tc := range tests {
		got := perms.NormalizeOrigin(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeOrigin(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestPermsManager_DefaultsAndModifications(t *testing.T) {
	mgr := perms.NewManager("")
	origin := "https://example.com"

	// Defaults: all allowed
	if !mgr.IsAllowed(origin, perms.PermissionJavaScript) {
		t.Errorf("expected JS allowed by default")
	}
	if !mgr.IsAllowed(origin, perms.PermissionImages) {
		t.Errorf("expected Images allowed by default")
	}
	if mgr.State(origin, perms.PermissionJavaScript) != perms.StateDefault {
		t.Errorf("expected StateDefault")
	}

	// Block JS
	mgr.Set(origin, perms.PermissionJavaScript, perms.StateBlock)
	if mgr.IsAllowed(origin, perms.PermissionJavaScript) {
		t.Errorf("expected JS blocked after Set StateBlock")
	}
	if mgr.State(origin, perms.PermissionJavaScript) != perms.StateBlock {
		t.Errorf("expected StateBlock")
	}

	// Images remain allowed
	if !mgr.IsAllowed(origin, perms.PermissionImages) {
		t.Errorf("expected Images still allowed")
	}

	// Set back to default
	mgr.Set(origin, perms.PermissionJavaScript, perms.StateDefault)
	if !mgr.IsAllowed(origin, perms.PermissionJavaScript) {
		t.Errorf("expected JS allowed after resetting to StateDefault")
	}
}

func TestPermsManager_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "permissions.json")

	mgr1 := perms.NewManager(filePath)
	mgr1.Set("https://blocked-js.com", perms.PermissionJavaScript, perms.StateBlock)
	mgr1.Set("https://blocked-images.com", perms.PermissionImages, perms.StateBlock)

	if err := mgr1.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	mgr2 := perms.NewManager(filePath)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if mgr2.IsAllowed("https://blocked-js.com", perms.PermissionJavaScript) {
		t.Errorf("expected blocked-js to remain blocked after Load")
	}
	if mgr2.IsAllowed("https://blocked-images.com", perms.PermissionImages) {
		t.Errorf("expected blocked-images to remain blocked after Load")
	}
	if !mgr2.IsAllowed("https://other.com", perms.PermissionJavaScript) {
		t.Errorf("expected other origins to remain allowed")
	}
}
