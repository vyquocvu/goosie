package ui_test

import (
	"github.com/vyquocvu/goosie/internal/ui"
	"sync"
	"testing"
)

func TestNewSettings(t *testing.T) {
	settings := ui.NewSettings()

	if settings.GetHomepage() != "https://example.com" {
		t.Errorf("Expected default homepage to be 'https://example.com', got '%s'", settings.GetHomepage())
	}

	if settings.GetDefaultSearchEngine() != "https://www.google.com/search?q=" {
		t.Errorf("Expected default search engine to be 'https://www.google.com/search?q=', got '%s'", settings.GetDefaultSearchEngine())
	}

	if !settings.GetEnableJavaScript() {
		t.Error("Expected JavaScript to be enabled by default")
	}

	if !settings.GetEnableImages() {
		t.Error("Expected images to be enabled by default")
	}
}

func TestSettingsHomepage(t *testing.T) {
	settings := ui.NewSettings()

	testURL := "https://github.com"
	settings.SetHomepage(testURL)

	if settings.GetHomepage() != testURL {
		t.Errorf("Expected homepage to be '%s', got '%s'", testURL, settings.GetHomepage())
	}
}

func TestSettingsSearchEngine(t *testing.T) {
	settings := ui.NewSettings()

	testURL := "https://duckduckgo.com/?q="
	settings.SetDefaultSearchEngine(testURL)

	if settings.GetDefaultSearchEngine() != testURL {
		t.Errorf("Expected search engine to be '%s', got '%s'", testURL, settings.GetDefaultSearchEngine())
	}
}

func TestSettingsJavaScript(t *testing.T) {
	settings := ui.NewSettings()

	settings.SetEnableJavaScript(false)
	if settings.GetEnableJavaScript() {
		t.Error("Expected JavaScript to be disabled")
	}

	settings.SetEnableJavaScript(true)
	if !settings.GetEnableJavaScript() {
		t.Error("Expected JavaScript to be enabled")
	}
}

func TestSettingsImages(t *testing.T) {
	settings := ui.NewSettings()

	settings.SetEnableImages(false)
	if settings.GetEnableImages() {
		t.Error("Expected images to be disabled")
	}

	settings.SetEnableImages(true)
	if !settings.GetEnableImages() {
		t.Error("Expected images to be enabled")
	}
}

func TestSettingsConcurrency(t *testing.T) {
	settings := ui.NewSettings()
	var wg sync.WaitGroup

	// Test concurrent access
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Concurrent reads and writes
			if idx%2 == 0 {
				settings.SetHomepage("https://test.com")
				_ = settings.GetHomepage()
			} else {
				settings.SetEnableJavaScript(idx%3 == 0)
				_ = settings.GetEnableJavaScript()
			}
		}(i)
	}

	wg.Wait()

	// Just verify we didn't crash - the exact values don't matter due to race
}
