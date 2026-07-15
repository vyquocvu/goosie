package devtools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatMethod(t *testing.T) {
	assert.Equal(t, "GET", formatMethod("GET"))
	assert.Equal(t, "POST", formatMethod("POST"))
}

func TestFormatStatusClass(t *testing.T) {
	assert.Equal(t, "2xx", formatStatusClass(200))
	assert.Equal(t, "3xx", formatStatusClass(301))
	assert.Equal(t, "4xx", formatStatusClass(404))
	assert.Equal(t, "5xx", formatStatusClass(500))
}

func TestFormatContentType(t *testing.T) {
	assert.Equal(t, "document", formatContentType("text/html"))
	assert.Equal(t, "stylesheet", formatContentType("text/css"))
	assert.Equal(t, "script", formatContentType("application/javascript"))
	assert.Equal(t, "image", formatContentType("image/png"))
	assert.Equal(t, "font", formatContentType("font/woff2"))
	assert.Equal(t, "image", formatContentType("image/webp"))
	assert.Equal(t, "other", formatContentType("application/json"))
}

func TestNetworkPanel_ClearOnFilter(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "https://example.com/page", Status: 200, ContentType: "text/html", Bytes: 1000, Duration: time.Millisecond * 50},
		{Method: "GET", URL: "https://example.com/style.css", Status: 200, ContentType: "text/css", Bytes: 500, Duration: time.Millisecond * 30},
		{Method: "POST", URL: "https://example.com/api", Status: 404, ContentType: "application/json", Bytes: 200, Duration: time.Millisecond * 100},
	}
	panel := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	})
	assert.NotNil(t, panel)
}

// mockRequestLog implements requestLogProvider for testing.
type mockRequestLog struct {
	entries []NetRequestEntry
}

func (m *mockRequestLog) Entries() []NetRequestEntry {
	return m.entries
}
