//go:build e2e

package e2e

import (
	"log"
	"os"
	"testing"

	"github.com/playwright-community/playwright-go"
)

var (
	pw      *playwright.Playwright
	browser playwright.Browser
)

func TestMain(m *testing.M) {
	var err error

	// Initialize Playwright
	log.Println("Starting Playwright driver...")
	pw, err = playwright.Run()
	if err != nil {
		log.Fatalf("could not launch playwright: %v", err)
	}

	// Launch Browser
	// Default to headless=true unless HEADLESS=false is set
	headless := os.Getenv("HEADLESS") != "false"

	launchOptions := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
	}

	browserType := os.Getenv("BROWSER")
	if browserType == "" {
		browserType = "chromium"
	}

	log.Printf("Launching browser: %s (Headless: %v)", browserType, headless)

	switch browserType {
	case "firefox":
		browser, err = pw.Firefox.Launch(launchOptions)
	case "webkit":
		browser, err = pw.WebKit.Launch(launchOptions)
	default:
		browser, err = pw.Chromium.Launch(launchOptions)
	}

	if err != nil {
		log.Fatalf("could not launch browser: %v", err)
	}

	// Run Tests
	code := m.Run()

	// Cleanup
	log.Println("Cleaning up Playwright resources...")
	if err := browser.Close(); err != nil {
		log.Printf("could not close browser: %v", err)
	}
	if err := pw.Stop(); err != nil {
		log.Printf("could not stop Playwright: %v", err)
	}

	os.Exit(code)
}

// newPage is a helper to create a new page context for tests
func newPage(t *testing.T) playwright.Page {
	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("could not create page: %v", err)
	}
	return page
}
