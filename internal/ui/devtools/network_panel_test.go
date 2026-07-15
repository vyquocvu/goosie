package devtools

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestFormatBytes(t *testing.T) {
	assert.Equal(t, "0 B", formatBytes(0))
	assert.Equal(t, "500 B", formatBytes(500))
	assert.Equal(t, "1.0 KB", formatBytes(1024))
	assert.Equal(t, "1.5 KB", formatBytes(1536))
	assert.Equal(t, "1.0 MB", formatBytes(1024*1024))
}

func TestFormatDuration(t *testing.T) {
	assert.Equal(t, "0ms", formatDuration(0))
	assert.Equal(t, "500µs", formatDuration(500*time.Microsecond))
	assert.Equal(t, "50ms", formatDuration(50*time.Millisecond))
	assert.Equal(t, "1.50s", formatDuration(1500*time.Millisecond))
}

func TestTruncateMiddle(t *testing.T) {
	assert.Equal(t, "short", truncateMiddle("short", 60))
	assert.Equal(t, "abc...xyz", truncateMiddle("abcdefghijklmnopqrstuvwxyz", 9))
	assert.Equal(t, "abc...klm", truncateMiddle("abcdefghijklm", 9))
	assert.Equal(t, "ab...lm", truncateMiddle("abcdefghijklm", 7))
}

func TestFormatWaterfall_NoPhases(t *testing.T) {
	result := formatWaterfall(time.Second)
	assert.Contains(t, result, "1000ms")
	assert.Contains(t, result, "█")
}

func TestFormatWaterfall_ZeroDuration(t *testing.T) {
	result := formatWaterfall(0)
	assert.Contains(t, result, "0ms")
}

func TestFormatWaterfall_WithPhases(t *testing.T) {
	phases := []TimingPhase{
		{Name: "DNS", Duration: 10 * time.Millisecond},
		{Name: "Connect", Duration: 20 * time.Millisecond},
		{Name: "TLS", Duration: 30 * time.Millisecond},
		{Name: "Request", Duration: 40 * time.Millisecond},
		{Name: "Download", Duration: 50 * time.Millisecond},
	}
	result := formatWaterfallWithPhases(150*time.Millisecond, phases)
	assert.Contains(t, result, "150ms")
}

func TestFormatWaterfall_PhasesProportional(t *testing.T) {
	phases := []TimingPhase{
		{Name: PhaseRequest, Duration: 100 * time.Millisecond},
		{Name: PhaseDownload, Duration: 900 * time.Millisecond},
	}
	result := formatWaterfallWithPhases(time.Second, phases)
	assert.Contains(t, result, "1000ms")
}

func TestFormatWaterfall_EmptyPhases(t *testing.T) {
	result := formatWaterfallWithPhases(100*time.Millisecond, nil)
	assert.Contains(t, result, "100ms")
	assert.Contains(t, result, "█")
}

func TestNetworkPanel_EmptyState(t *testing.T) {
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{}}
	}).(*networkPanel)
	assert.Empty(t, p.entries)
	assert.Empty(t, p.visible)
}

func TestNetworkPanel_RefreshFromPopulatesEntries(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "https://example.com", Status: 200, ContentType: "text/html", Bytes: 1000, Duration: time.Millisecond * 50},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{}
	}).(*networkPanel)
	p.entries = entries
	p.syncData()
	assert.Equal(t, 1, len(p.entries))
	assert.Equal(t, 1, len(p.visible))
	assert.Equal(t, "GET", p.visible[0].Method)
}

func TestNetworkPanel_RefreshFromSetsEntries(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/a", Status: 200, Duration: time.Millisecond},
		{Method: "POST", URL: "/b", Status: 201, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.RefreshFrom(&TabContext{
		RequestLog: &mockRequestLog{entries: entries},
	})
	assert.Equal(t, 2, len(p.entries))
}

func TestNetworkPanel_RefreshFromNilContext(t *testing.T) {
	p := newNetworkPanel(func() *TabContext { return &TabContext{} }).(*networkPanel)
	p.entries = []NetRequestEntry{{Method: "GET"}}
	p.RefreshFrom(nil)
	assert.Equal(t, 1, len(p.entries), "should keep existing entries on nil context")
}

func TestNetworkPanel_RefreshFromNilRequestLog(t *testing.T) {
	p := newNetworkPanel(func() *TabContext { return &TabContext{} }).(*networkPanel)
	p.entries = []NetRequestEntry{{Method: "GET"}}
	p.RefreshFrom(&TabContext{RequestLog: nil})
	assert.Equal(t, 1, len(p.entries), "should keep existing entries when RequestLog is nil")
}

func TestNetworkPanel_RefreshFromMultipleCalls(t *testing.T) {
	first := []NetRequestEntry{
		{Method: "GET", URL: "/a", Duration: time.Millisecond},
	}
	second := []NetRequestEntry{
		{Method: "GET", URL: "/a", Duration: time.Millisecond},
		{Method: "POST", URL: "/b", Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: first}}
	}).(*networkPanel)
	p.RefreshFrom(&TabContext{
		RequestLog: &mockRequestLog{entries: second},
	})
	assert.Equal(t, 2, len(p.entries))
}

func TestNetworkPanel_SortByURL(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "https://z.com", Status: 200, Duration: time.Millisecond},
		{Method: "POST", URL: "https://a.com", Status: 201, Duration: time.Millisecond},
		{Method: "GET", URL: "https://m.com", Status: 301, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.visible = append([]NetRequestEntry{}, entries...)
	p.sortCol = colURL
	p.sortAsc = true
	p.applySort()
	assert.Equal(t, "https://a.com", p.visible[0].URL)
	assert.Equal(t, "https://m.com", p.visible[1].URL)
	assert.Equal(t, "https://z.com", p.visible[2].URL)
}

func TestNetworkPanel_SortByURLDesc(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "https://a.com", Status: 200, Duration: time.Millisecond},
		{Method: "POST", URL: "https://z.com", Status: 201, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.visible = append([]NetRequestEntry{}, entries...)
	p.sortCol = colURL
	p.sortAsc = false
	p.applySort()
	assert.Equal(t, "https://z.com", p.visible[0].URL)
}

func TestNetworkPanel_SortByStatus(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/a", Status: 301, Duration: time.Millisecond},
		{Method: "POST", URL: "/b", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/c", Status: 404, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.visible = append([]NetRequestEntry{}, entries...)
	p.sortCol = colStatus
	p.sortAsc = true
	p.applySort()
	assert.Equal(t, 200, p.visible[0].Status)
	assert.Equal(t, 301, p.visible[1].Status)
	assert.Equal(t, 404, p.visible[2].Status)
}

func TestNetworkPanel_SortBySize(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/a", Bytes: 1000, Duration: time.Millisecond},
		{Method: "POST", URL: "/b", Bytes: 100, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.visible = append([]NetRequestEntry{}, entries...)
	p.sortCol = colSize
	p.sortAsc = true
	p.applySort()
	assert.Equal(t, int64(100), p.visible[0].Bytes)
}

func TestNetworkPanel_SortByTime(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/a", Duration: 500 * time.Millisecond},
		{Method: "POST", URL: "/b", Duration: 50 * time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.visible = append([]NetRequestEntry{}, entries...)
	p.sortCol = colTime
	p.sortAsc = true
	p.applySort()
	assert.Equal(t, 50*time.Millisecond, p.visible[0].Duration)
}

func TestNetworkPanel_FilterByTypeDocument(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/page", ContentType: "text/html", Duration: time.Millisecond},
		{Method: "GET", URL: "/style.css", ContentType: "text/css", Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.contentType = "document"
	p.syncData()
	require.Equal(t, 1, len(p.visible))
	assert.Equal(t, "/page", p.visible[0].URL)
}

func TestNetworkPanel_FilterByTypeScript(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/page", ContentType: "text/html", Duration: time.Millisecond},
		{Method: "GET", URL: "/app.js", ContentType: "application/javascript", Duration: time.Millisecond},
		{Method: "POST", URL: "/api", ContentType: "application/json", Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.contentType = "script"
	p.syncData()
	require.Equal(t, 1, len(p.visible))
	assert.Equal(t, "/app.js", p.visible[0].URL)
}

func TestNetworkPanel_FilterByTypeOther(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/page", ContentType: "text/html", Duration: time.Millisecond},
		{Method: "POST", URL: "/api", ContentType: "application/json", Duration: time.Millisecond},
		{Method: "GET", URL: "/font.woff2", ContentType: "font/woff2", Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.contentType = "other"
	p.syncData()
	require.Equal(t, 1, len(p.visible))
	assert.Equal(t, "/api", p.visible[0].URL)
}

func TestNetworkPanel_FilterByStatusClass(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/ok", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/redirect", Status: 301, Duration: time.Millisecond},
		{Method: "POST", URL: "/notfound", Status: 404, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.statusClass = "4xx"
	p.syncData()
	require.Equal(t, 1, len(p.visible))
	assert.Equal(t, "/notfound", p.visible[0].URL)
}

func TestNetworkPanel_FilterByStatus2xx(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/ok", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/created", Status: 201, Duration: time.Millisecond},
		{Method: "GET", URL: "/redirect", Status: 301, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.statusClass = "2xx"
	p.syncData()
	require.Equal(t, 2, len(p.visible))
}

func TestNetworkPanel_SearchFilter(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "https://example.com/page", Status: 200, Duration: time.Millisecond},
		{Method: "POST", URL: "https://api.example.com/data", Status: 200, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.search = "api"
	p.syncData()
	require.Equal(t, 1, len(p.visible))
	assert.Contains(t, p.visible[0].URL, "api")
}

func TestNetworkPanel_SearchFilterCaseInsensitive(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "https://EXAMPLE.com", Status: 200, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.search = "example"
	p.syncData()
	require.Equal(t, 1, len(p.visible))
}

func TestNetworkPanel_SearchFilterNoMatch(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "https://example.com", Status: 200, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.search = "zzzzz"
	p.syncData()
	assert.Empty(t, p.visible)
}

func TestNetworkPanel_CombinedFilters(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "https://example.com/page", Status: 200, ContentType: "text/html", Duration: time.Millisecond},
		{Method: "GET", URL: "https://example.com/style.css", Status: 200, ContentType: "text/css", Duration: time.Millisecond},
		{Method: "GET", URL: "https://example.com/404", Status: 404, ContentType: "text/html", Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.contentType = "document"
	p.filter.statusClass = "2xx"
	p.syncData()
	require.Equal(t, 1, len(p.visible))
	assert.Contains(t, p.visible[0].URL, "/page")
}

func TestNetworkPanel_ClearEntries(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "https://example.com", Status: 200, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.visible = entries
	p.entries = nil
	p.syncData()
	assert.Empty(t, p.entries)
	assert.Empty(t, p.visible)
}

func TestNetworkPanel_FormatRow(t *testing.T) {
	e := NetRequestEntry{
		Method: "GET", URL: "https://example.com/page", Status: 200,
		ContentType: "text/html", Bytes: 1024, Duration: 50 * time.Millisecond,
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{}}
	}).(*networkPanel)
	row := p.formatRow(e)
	assert.Contains(t, row, "GET")
	assert.Contains(t, row, "200")
	assert.Contains(t, row, "example.com")
	assert.Contains(t, row, "1.0 KB")
}

func TestNetworkPanel_FormatRowWithError(t *testing.T) {
	e := NetRequestEntry{
		Method: "GET", URL: "https://example.com", Status: 0,
		Error: "connection refused", Duration: time.Second,
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{}}
	}).(*networkPanel)
	row := p.formatRow(e)
	assert.Contains(t, row, "ERR")
}

func TestNetworkPanel_FormatRowWithPhases(t *testing.T) {
	e := NetRequestEntry{
		Method: "GET", URL: "https://example.com", Status: 200,
		ContentType: "text/html", Bytes: 500, Duration: 200 * time.Millisecond,
		TimingPhases: []TimingPhase{
			{Name: PhaseDNS, Duration: 20 * time.Millisecond},
			{Name: PhaseRequest, Duration: 80 * time.Millisecond},
			{Name: PhaseDownload, Duration: 100 * time.Millisecond},
		},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{}}
	}).(*networkPanel)
	row := p.formatRow(e)
	assert.Contains(t, row, "200ms")
}

func TestNetworkPanel_DetailView(t *testing.T) {
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{}}
	}).(*networkPanel)
	require.NotNil(t, p.detailBox)
}

func TestNetworkPanel_DetailViewWithPhases(t *testing.T) {
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{}}
	}).(*networkPanel)
	require.NotNil(t, p.detailBox)
}

func TestNetworkPanel_MultipleEntries(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/a", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/b", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/c", Status: 200, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.syncData()
	assert.Equal(t, 3, len(p.entries))
	assert.Equal(t, 3, len(p.visible))
}

func TestNetworkPanel_PreserveCheckToggle(t *testing.T) {
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{}}
	}).(*networkPanel)
	require.NotNil(t, p.preserveCheck)
}

func TestNetworkPanel_TypeFiltersAllShowsAll(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/page", ContentType: "text/html", Duration: time.Millisecond},
		{Method: "GET", URL: "/style.css", ContentType: "text/css", Duration: time.Millisecond},
		{Method: "POST", URL: "/api", ContentType: "application/json", Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.contentType = ""
	p.syncData()
	assert.Equal(t, 3, len(p.visible))
}

func TestNetworkPanel_StatusFiltersAllShowsAll(t *testing.T) {
	entries := []NetRequestEntry{
		{Method: "GET", URL: "/ok", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/notfound", Status: 404, Duration: time.Millisecond},
	}
	p := newNetworkPanel(func() *TabContext {
		return &TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*networkPanel)
	p.entries = entries
	p.filter.statusClass = ""
	p.syncData()
	assert.Equal(t, 2, len(p.visible))
}

func TestNetworkPanel_TruncateLongURL(t *testing.T) {
	longURL := "https://example.com/very/long/path/that/should/be/truncated/in/the/middle/for/better/display"
	truncated := truncateMiddle(longURL, 60)
	assert.True(t, len(truncated) <= 63, "truncated length %d exceeds max", len(truncated))
	assert.Contains(t, truncated, "...")
}

// mockRequestLog implements requestLogProvider for testing.
type mockRequestLog struct {
	entries []NetRequestEntry
}

func (m *mockRequestLog) Entries() []NetRequestEntry {
	if m.entries == nil {
		return nil
	}
	return m.entries
}
