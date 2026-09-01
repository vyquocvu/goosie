package devtools_test

import (
	"github.com/vyquocvu/goosie/internal/ui/devtools"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

func TestSecurityPanel_InitialState(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := devtools.NewSecurityPanel(nil).(*devtools.SecurityPanel)
	assert.NotNil(t, p)
	assert.Contains(t, p.Label().Text, "No security information")
}

func TestSecurityPanel_EmptyContext(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	p := devtools.NewSecurityPanel(func() *devtools.TabContext { return &devtools.TabContext{} }).(*devtools.SecurityPanel)
	p.RefreshFrom(&devtools.TabContext{})
	assert.Contains(t, p.Label().Text, "No page loaded")
}

func TestSecurityPanel_HTTPPage(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &devtools.TabContext{
		CurrentURL: "http://example.com/page",
	}
	p := devtools.NewSecurityPanel(func() *devtools.TabContext { return ctx }).(*devtools.SecurityPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.Label().Text, "http://example.com/page")
	assert.Contains(t, p.Label().Text, "HTTP (unencrypted)")
}

func TestSecurityPanel_HTTPSPage(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &devtools.TabContext{
		CurrentURL: "https://secure.example.com",
	}
	p := devtools.NewSecurityPanel(func() *devtools.TabContext { return ctx }).(*devtools.SecurityPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.Label().Text, "https://secure.example.com")
	assert.Contains(t, p.Label().Text, "HTTPS (encrypted)")
}

func TestSecurityPanel_WithSummary(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &devtools.TabContext{
		CurrentURL:      "https://example.com",
		SecuritySummary: "TLS 1.3 | AES_128_GCM",
	}
	p := devtools.NewSecurityPanel(func() *devtools.TabContext { return ctx }).(*devtools.SecurityPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.Label().Text, "TLS 1.3")
	assert.Contains(t, p.Label().Text, "AES_128_GCM")
}

func TestSecurityPanel_RefreshFrom(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &devtools.TabContext{
		CurrentURL:      "https://example.com",
		SecuritySummary: "TLS 1.2 | ECDHE_RSA",
	}
	p := devtools.NewSecurityPanel(func() *devtools.TabContext { return ctx }).(*devtools.SecurityPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.Label().Text, "TLS 1.2")
}

func TestSecurityPanel_WithCertDetails(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &devtools.TabContext{
		CurrentURL:      "https://example.com",
		SecuritySummary: "TLS CN=example.com",
		SecurityInfo: devtools.SecurityInfo{
			Subject:   "CN=example.com",
			Issuer:    "CN=CA Issuer",
			NotBefore: "Jan 1, 2026",
			NotAfter:  "Jan 1, 2027",
		},
	}
	p := devtools.NewSecurityPanel(func() *devtools.TabContext { return ctx }).(*devtools.SecurityPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.Label().Text, "CN=example.com")
	assert.Contains(t, p.Label().Text, "CN=CA Issuer")
	assert.Contains(t, p.Label().Text, "Jan 1, 2027")
}

func TestSecurityPanel_HTTPPageNoCert(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	ctx := &devtools.TabContext{
		CurrentURL:   "http://example.com",
		SecurityInfo: devtools.SecurityInfo{Scheme: "http"},
	}
	p := devtools.NewSecurityPanel(func() *devtools.TabContext { return ctx }).(*devtools.SecurityPanel)
	p.RefreshFrom(ctx)
	assert.Contains(t, p.Label().Text, "No certificate for HTTP")
}
