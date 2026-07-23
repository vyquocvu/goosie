package devtools

import (
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

func TestSecurityPanel_InitialState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newSecurityPanel(nil).(*securityPanel)
	assert.NotNil(t, p)
	assert.Contains(t, p.label.Text, "No security information")
}

func TestSecurityPanel_EmptyContext(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := newSecurityPanel(func() *TabContext { return &TabContext{} }).(*securityPanel)
	p.RefreshFrom(&TabContext{})
	assert.Contains(t, p.label.Text, "No page loaded")
}

func TestSecurityPanel_HTTPPage(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		CurrentURL: "http://example.com/page",
	}
	p := newSecurityPanel(func() *TabContext { return ctx }).(*securityPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.label.Text, "http://example.com/page")
	assert.Contains(t, p.label.Text, "HTTP (unencrypted)")
}

func TestSecurityPanel_HTTPSPage(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		CurrentURL: "https://secure.example.com",
	}
	p := newSecurityPanel(func() *TabContext { return ctx }).(*securityPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.label.Text, "https://secure.example.com")
	assert.Contains(t, p.label.Text, "HTTPS (encrypted)")
}

func TestSecurityPanel_WithSummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		CurrentURL:      "https://example.com",
		SecuritySummary: "TLS 1.3 | AES_128_GCM",
	}
	p := newSecurityPanel(func() *TabContext { return ctx }).(*securityPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.label.Text, "TLS 1.3")
	assert.Contains(t, p.label.Text, "AES_128_GCM")
}

func TestSecurityPanel_RefreshFrom(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &TabContext{
		CurrentURL:      "https://example.com",
		SecuritySummary: "TLS 1.2 | ECDHE_RSA",
	}
	p := newSecurityPanel(func() *TabContext { return ctx }).(*securityPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.label.Text, "Certificate chain inspection")
}
