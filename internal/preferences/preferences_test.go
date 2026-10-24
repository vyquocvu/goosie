package preferences

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchURLTemplateValidatesAndEscapesQuery(t *testing.T) {
	got, err := SearchURL("https://search.example/find?q={query}", "cà phê & tea")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://search.example/find?q=c%C3%A0+ph%C3%AA+%26+tea" {
		t.Fatalf("SearchURL() = %q", got)
	}
	for _, bad := range []string{
		"https://search.example/find?q=plain",
		"https://search.example/find?q={query}&again={query}",
		"https://{query}.search.example/find?q=x",
		"https://search.example/find?q=x#{query}",
		"file:///tmp/search?q={query}",
		"https://user:pass@search.example/?q={query}",
		"https:///find?q={query}",
	} {
		if err := ValidateSearchURLTemplate(bad); err == nil {
			t.Errorf("ValidateSearchURLTemplate(%q) accepted invalid template", bad)
		}
	}
}

func TestLoadSaveAndRejectInvalidSavedSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile", "preferences.json")
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.SearchURLTemplate != DefaultSearchURLTemplate {
		t.Fatalf("missing file defaults = %+v", got)
	}
	want := Settings{SearchURLTemplate: "https://search.example/?term={query}", StartupMode: StartupRestore}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err != nil || got != want {
		t.Fatalf("Load() = %+v, %v; want %+v", got, err, want)
	}
	if err := os.WriteFile(path, []byte(`{"searchURLTemplate":"javascript:alert(1)?q={query}"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = Load(path)
	if err == nil || got != Defaults() {
		t.Fatalf("invalid saved setting = %+v, %v; want defaults and error", got, err)
	}
}

func TestLoadLegacySettingsDefaultsStartupMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	legacy := `{"searchURLTemplate":"https://search.example/?q={query}","downloadDir":"/tmp/goosie-downloads"}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.StartupMode != StartupRestore {
		t.Fatalf("legacy startup mode = %q, want default %q", got.StartupMode, StartupRestore)
	}
}

func TestValidateDownloadDirRequiresAbsolutePath(t *testing.T) {
	if err := ValidateDownloadDir(filepath.Join(t.TempDir(), "files")); err != nil {
		t.Fatalf("absolute path rejected: %v", err)
	}
	for _, dir := range []string{"", "Downloads", "../downloads", "/tmp/bad\npath", "/tmp/bad\x00path"} {
		if err := ValidateDownloadDir(dir); err == nil {
			t.Errorf("ValidateDownloadDir(%q) accepted invalid path", dir)
		}
	}
}

func TestValidateStartupMode(t *testing.T) {
	for _, mode := range []string{StartupRestore, StartupNewTab} {
		if err := ValidateStartupMode(mode); err != nil {
			t.Errorf("ValidateStartupMode(%q): %v", mode, err)
		}
	}
	for _, mode := range []string{"", "resume", "javascript:alert(1)"} {
		if err := ValidateStartupMode(mode); err == nil {
			t.Errorf("ValidateStartupMode(%q) accepted invalid mode", mode)
		}
	}
}
