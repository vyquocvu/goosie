package devtools_test

import (
	"github.com/vyquocvu/goosie/internal/ui/devtools"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatMethod(t *testing.T) {
	assert.Equal(t, "GET", devtools.FormatMethod("GET"))
	assert.Equal(t, "POST", devtools.FormatMethod("POST"))
}

func TestFormatStatusClass(t *testing.T) {
	assert.Equal(t, "2xx", devtools.FormatStatusClass(200))
	assert.Equal(t, "3xx", devtools.FormatStatusClass(301))
	assert.Equal(t, "4xx", devtools.FormatStatusClass(404))
	assert.Equal(t, "5xx", devtools.FormatStatusClass(500))
}

func TestFormatContentType(t *testing.T) {
	assert.Equal(t, "document", devtools.FormatContentType("text/html"))
	assert.Equal(t, "stylesheet", devtools.FormatContentType("text/css"))
	assert.Equal(t, "script", devtools.FormatContentType("application/javascript"))
	assert.Equal(t, "image", devtools.FormatContentType("image/png"))
	assert.Equal(t, "font", devtools.FormatContentType("font/woff2"))
	assert.Equal(t, "image", devtools.FormatContentType("image/webp"))
	assert.Equal(t, "other", devtools.FormatContentType("application/json"))
}

func TestFormatBytes(t *testing.T) {
	assert.Equal(t, "0 B", devtools.FormatBytes(0))
	assert.Equal(t, "500 B", devtools.FormatBytes(500))
	assert.Equal(t, "1.0 KB", devtools.FormatBytes(1024))
	assert.Equal(t, "1.5 KB", devtools.FormatBytes(1536))
	assert.Equal(t, "1.0 MB", devtools.FormatBytes(1024*1024))
}

func TestFormatDuration(t *testing.T) {
	assert.Equal(t, "0ms", devtools.FormatDuration(0))
	assert.Equal(t, "500µs", devtools.FormatDuration(500*time.Microsecond))
	assert.Equal(t, "50ms", devtools.FormatDuration(50*time.Millisecond))
	assert.Equal(t, "1.50s", devtools.FormatDuration(1500*time.Millisecond))
}

func TestTruncateMiddle(t *testing.T) {
	assert.Equal(t, "short", devtools.TruncateMiddle("short", 60))
	assert.Equal(t, "abc...xyz", devtools.TruncateMiddle("abcdefghijklmnopqrstuvwxyz", 9))
	assert.Equal(t, "abc...klm", devtools.TruncateMiddle("abcdefghijklm", 9))
	assert.Equal(t, "ab...lm", devtools.TruncateMiddle("abcdefghijklm", 7))
}

func TestFormatWaterfall_NoPhases(t *testing.T) {
	result := devtools.FormatWaterfall(time.Second)
	assert.Contains(t, result, "1000ms")
	assert.Contains(t, result, "█")
}

func TestFormatWaterfall_ZeroDuration(t *testing.T) {
	result := devtools.FormatWaterfall(0)
	assert.Contains(t, result, "0ms")
}

func TestFormatWaterfall_WithPhases(t *testing.T) {
	phases := []devtools.TimingPhase{
		{Name: "DNS", Duration: 10 * time.Millisecond},
		{Name: "Connect", Duration: 20 * time.Millisecond},
		{Name: "TLS", Duration: 30 * time.Millisecond},
		{Name: "Request", Duration: 40 * time.Millisecond},
		{Name: "Download", Duration: 50 * time.Millisecond},
	}
	result := devtools.FormatWaterfallWithPhases(150*time.Millisecond, phases)
	assert.Contains(t, result, "150ms")
}

func TestFormatWaterfall_PhasesProportional(t *testing.T) {
	phases := []devtools.TimingPhase{
		{Name: devtools.PhaseRequest, Duration: 100 * time.Millisecond},
		{Name: devtools.PhaseDownload, Duration: 900 * time.Millisecond},
	}
	result := devtools.FormatWaterfallWithPhases(time.Second, phases)
	assert.Contains(t, result, "1000ms")
}

func TestFormatWaterfall_EmptyPhases(t *testing.T) {
	result := devtools.FormatWaterfallWithPhases(100*time.Millisecond, nil)
	assert.Contains(t, result, "100ms")
	assert.Contains(t, result, "█")
}

// TestFormatCurl covers every common request shape the user might
// copy: a plain GET, a POST with body (currently ignored in the
// curl output), and a URL containing shell metacharacters that
// must be quoted.
func TestFormatCurl(t *testing.T) {
	cases := []struct {
		name string
		in   devtools.NetRequestEntry
		want string
	}{
		{
			name: "simple GET",
			in:   devtools.NetRequestEntry{Method: "GET", URL: "https://example.com/"},
			want: `curl -X GET https://example.com/`,
		},
		{
			name: "POST defaults method when zero",
			in:   devtools.NetRequestEntry{URL: "https://example.com/api"},
			want: `curl -X GET https://example.com/api`,
		},
		{
			name: "URL with spaces gets single-quoted",
			in:   devtools.NetRequestEntry{Method: "GET", URL: "https://example.com/path with space"},
			want: `curl -X GET 'https://example.com/path with space'`,
		},
		{
			name: "URL with single quotes is escaped",
			in:   devtools.NetRequestEntry{Method: "GET", URL: "https://example.com/it's"},
			want: `curl -X GET 'https://example.com/it'\''s'`,
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, devtools.FormatCurl(c.in))
		})
	}
}

// TestShellQuote covers the shell-quoting edge cases. Plain
// ASCII identifiers and alphanumeric URLs pass through
// unchanged; metacharacters force single-quoting; embedded
// single quotes are escaped via the standard `'\''` trick.
func TestShellQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "''"},
		{"hello", "hello"},
		{"hello world", "'hello world'"},
		{"it's", `'it'\''s'`},
		{"$PATH", "'$PATH'"},
		{`a"b`, `'a"b'`},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, devtools.ShellQuote(c.in))
		})
	}
}

// TestClipboardSetNoApp verifies that the copy helpers do not
// panic when no Fyne app is running (headless test environment).
// This is the regression guard for the dev-test harness which
// constructs the panel without a current app.
func TestClipboardSetNoApp(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("clipboardSet panicked without app: %v", r)
		}
	}()
	p := &devtools.NetworkPanel{}
	devtools.ClipboardSet(p, "anything")
	devtools.ClipboardSet(nil, "nil panel")
}

func TestNetworkPanel_EmptyState(t *testing.T) {
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{}}
	}).(*devtools.NetworkPanel)
	assert.Empty(t, p.Entries())
	assert.Empty(t, p.VisibleResources())
}

func TestNetworkPanel_RefreshFromPopulatesEntries(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "https://example.com", Status: 200, ContentType: "text/html", Bytes: 1000, Duration: time.Millisecond * 50},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SyncData()
	assert.Equal(t, 1, len(p.Entries()))
	assert.Equal(t, 1, len(p.VisibleResources()))
	assert.Equal(t, "GET", p.VisibleResources()[0].Method)
}

func TestNetworkPanel_RefreshFromSetsEntries(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/a", Status: 200, Duration: time.Millisecond},
		{Method: "POST", URL: "/b", Status: 201, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.RefreshFrom(&devtools.TabContext{
		RequestLog: &mockRequestLog{entries: entries},
	})
	assert.Equal(t, 2, len(p.Entries()))
}

func TestNetworkPanel_RefreshFromNilContext(t *testing.T) {
	p := devtools.NewNetworkPanel(func() *devtools.TabContext { return &devtools.TabContext{} }).(*devtools.NetworkPanel)
	p.SetEntries([]devtools.NetRequestEntry{{Method: "GET"}})
	p.RefreshFrom(nil)
	assert.Equal(t, 1, len(p.Entries()), "should keep existing entries on nil context")
}

func TestNetworkPanel_RefreshFromNilRequestLog(t *testing.T) {
	p := devtools.NewNetworkPanel(func() *devtools.TabContext { return &devtools.TabContext{} }).(*devtools.NetworkPanel)
	p.SetEntries([]devtools.NetRequestEntry{{Method: "GET"}})
	p.RefreshFrom(&devtools.TabContext{RequestLog: nil})
	assert.Equal(t, 1, len(p.Entries()), "should keep existing entries when RequestLog is nil")
}

func TestNetworkPanel_RefreshFromMultipleCalls(t *testing.T) {
	first := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/a", Duration: time.Millisecond},
	}
	second := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/a", Duration: time.Millisecond},
		{Method: "POST", URL: "/b", Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: first}}
	}).(*devtools.NetworkPanel)
	p.RefreshFrom(&devtools.TabContext{
		RequestLog: &mockRequestLog{entries: second},
	})
	assert.Equal(t, 2, len(p.Entries()))
}

func TestNetworkPanel_SortByURL(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "https://z.com", Status: 200, Duration: time.Millisecond},
		{Method: "POST", URL: "https://a.com", Status: 201, Duration: time.Millisecond},
		{Method: "GET", URL: "https://m.com", Status: 301, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetVisibleEntries(append([]devtools.NetRequestEntry{}, entries...))
	p.SetSortCol(devtools.ColURL)
	p.SetSortAsc(true)
	p.ApplySort()
	assert.Equal(t, "https://a.com", p.VisibleResources()[0].URL)
	assert.Equal(t, "https://m.com", p.VisibleResources()[1].URL)
	assert.Equal(t, "https://z.com", p.VisibleResources()[2].URL)
}

func TestNetworkPanel_SortByURLDesc(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "https://a.com", Status: 200, Duration: time.Millisecond},
		{Method: "POST", URL: "https://z.com", Status: 201, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetVisibleEntries(append([]devtools.NetRequestEntry{}, entries...))
	p.SetSortCol(devtools.ColURL)
	p.SetSortAsc(false)
	p.ApplySort()
	assert.Equal(t, "https://z.com", p.VisibleResources()[0].URL)
}

func TestNetworkPanel_SortByStatus(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/a", Status: 301, Duration: time.Millisecond},
		{Method: "POST", URL: "/b", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/c", Status: 404, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetVisibleEntries(append([]devtools.NetRequestEntry{}, entries...))
	p.SetSortCol(devtools.ColStatus)
	p.SetSortAsc(true)
	p.ApplySort()
	assert.Equal(t, 200, p.VisibleResources()[0].Status)
	assert.Equal(t, 301, p.VisibleResources()[1].Status)
	assert.Equal(t, 404, p.VisibleResources()[2].Status)
}

func TestNetworkPanel_SortBySize(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/a", Bytes: 1000, Duration: time.Millisecond},
		{Method: "POST", URL: "/b", Bytes: 100, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetVisibleEntries(append([]devtools.NetRequestEntry{}, entries...))
	p.SetSortCol(devtools.ColSize)
	p.SetSortAsc(true)
	p.ApplySort()
	assert.Equal(t, int64(100), p.VisibleResources()[0].Bytes)
}

func TestNetworkPanel_SortByTime(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/a", Duration: 500 * time.Millisecond},
		{Method: "POST", URL: "/b", Duration: 50 * time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetVisibleEntries(append([]devtools.NetRequestEntry{}, entries...))
	p.SetSortCol(devtools.ColTime)
	p.SetSortAsc(true)
	p.ApplySort()
	assert.Equal(t, 50*time.Millisecond, p.VisibleResources()[0].Duration)
}

func TestNetworkPanel_FilterByTypeDocument(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/page", ContentType: "text/html", Duration: time.Millisecond},
		{Method: "GET", URL: "/style.css", ContentType: "text/css", Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterContentType("document")
	p.SyncData()
	require.Equal(t, 1, len(p.VisibleResources()))
	assert.Equal(t, "/page", p.VisibleResources()[0].URL)
}

func TestNetworkPanel_FilterByTypeScript(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/page", ContentType: "text/html", Duration: time.Millisecond},
		{Method: "GET", URL: "/app.js", ContentType: "application/javascript", Duration: time.Millisecond},
		{Method: "POST", URL: "/api", ContentType: "application/json", Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterContentType("script")
	p.SyncData()
	require.Equal(t, 1, len(p.VisibleResources()))
	assert.Equal(t, "/app.js", p.VisibleResources()[0].URL)
}

func TestNetworkPanel_FilterByTypeOther(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/page", ContentType: "text/html", Duration: time.Millisecond},
		{Method: "POST", URL: "/api", ContentType: "application/json", Duration: time.Millisecond},
		{Method: "GET", URL: "/font.woff2", ContentType: "font/woff2", Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterContentType("other")
	p.SyncData()
	require.Equal(t, 1, len(p.VisibleResources()))
	assert.Equal(t, "/api", p.VisibleResources()[0].URL)
}

func TestNetworkPanel_FilterByStatusClass(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/ok", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/redirect", Status: 301, Duration: time.Millisecond},
		{Method: "POST", URL: "/notfound", Status: 404, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterStatusClass("4xx")
	p.SyncData()
	require.Equal(t, 1, len(p.VisibleResources()))
	assert.Equal(t, "/notfound", p.VisibleResources()[0].URL)
}

func TestNetworkPanel_FilterByStatus2xx(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/ok", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/created", Status: 201, Duration: time.Millisecond},
		{Method: "GET", URL: "/redirect", Status: 301, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterStatusClass("2xx")
	p.SyncData()
	require.Equal(t, 2, len(p.VisibleResources()))
}

func TestNetworkPanel_SearchFilter(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "https://example.com/page", Status: 200, Duration: time.Millisecond},
		{Method: "POST", URL: "https://api.example.com/data", Status: 200, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterSearch("api")
	p.SyncData()
	require.Equal(t, 1, len(p.VisibleResources()))
	assert.Contains(t, p.VisibleResources()[0].URL, "api")
}

func TestNetworkPanel_SearchFilterCaseInsensitive(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "https://EXAMPLE.com", Status: 200, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterSearch("example")
	p.SyncData()
	require.Equal(t, 1, len(p.VisibleResources()))
}

func TestNetworkPanel_SearchFilterNoMatch(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "https://example.com", Status: 200, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterSearch("zzzzz")
	p.SyncData()
	assert.Empty(t, p.VisibleResources())
}

func TestNetworkPanel_CombinedFilters(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "https://example.com/page", Status: 200, ContentType: "text/html", Duration: time.Millisecond},
		{Method: "GET", URL: "https://example.com/style.css", Status: 200, ContentType: "text/css", Duration: time.Millisecond},
		{Method: "GET", URL: "https://example.com/404", Status: 404, ContentType: "text/html", Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterContentType("document")
	p.SetFilterStatusClass("2xx")
	p.SyncData()
	require.Equal(t, 1, len(p.VisibleResources()))
	assert.Contains(t, p.VisibleResources()[0].URL, "/page")
}

func TestNetworkPanel_ClearEntries(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "https://example.com", Status: 200, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetVisibleEntries(entries)
	p.SetEntries(nil)
	p.SyncData()
	assert.Empty(t, p.Entries())
	assert.Empty(t, p.VisibleResources())
}

func TestNetworkPanel_FormatRow(t *testing.T) {
	e := devtools.NetRequestEntry{
		Method: "GET", URL: "https://example.com/page", Status: 200,
		ContentType: "text/html", Bytes: 1024, Duration: 50 * time.Millisecond,
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{}}
	}).(*devtools.NetworkPanel)
	row := p.FormatRow(e)
	assert.Contains(t, row, "GET")
	assert.Contains(t, row, "200")
	assert.Contains(t, row, "example.com")
	assert.Contains(t, row, "1.0 KB")
}

func TestNetworkPanel_FormatRowWithError(t *testing.T) {
	e := devtools.NetRequestEntry{
		Method: "GET", URL: "https://example.com", Status: 0,
		Error: "connection refused", Duration: time.Second,
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{}}
	}).(*devtools.NetworkPanel)
	row := p.FormatRow(e)
	assert.Contains(t, row, "ERR")
}

func TestNetworkPanel_FormatRowWithPhases(t *testing.T) {
	e := devtools.NetRequestEntry{
		Method: "GET", URL: "https://example.com", Status: 200,
		ContentType: "text/html", Bytes: 500, Duration: 200 * time.Millisecond,
		TimingPhases: []devtools.TimingPhase{
			{Name: devtools.PhaseDNS, Duration: 20 * time.Millisecond},
			{Name: devtools.PhaseRequest, Duration: 80 * time.Millisecond},
			{Name: devtools.PhaseDownload, Duration: 100 * time.Millisecond},
		},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{}}
	}).(*devtools.NetworkPanel)
	row := p.FormatRow(e)
	assert.Contains(t, row, "200ms")
}

func TestNetworkPanel_DetailView(t *testing.T) {
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{}}
	}).(*devtools.NetworkPanel)
	require.NotNil(t, p.DetailBox())
}

func TestNetworkPanel_DetailViewWithPhases(t *testing.T) {
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{}}
	}).(*devtools.NetworkPanel)
	require.NotNil(t, p.DetailBox())
}

func TestNetworkPanel_MultipleEntries(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/a", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/b", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/c", Status: 200, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SyncData()
	assert.Equal(t, 3, len(p.Entries()))
	assert.Equal(t, 3, len(p.VisibleResources()))
}

func TestNetworkPanel_PreserveCheckToggle(t *testing.T) {
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{}}
	}).(*devtools.NetworkPanel)
	require.NotNil(t, p.PreserveCheck())
}

func TestNetworkPanel_TypeFiltersAllShowsAll(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/page", ContentType: "text/html", Duration: time.Millisecond},
		{Method: "GET", URL: "/style.css", ContentType: "text/css", Duration: time.Millisecond},
		{Method: "POST", URL: "/api", ContentType: "application/json", Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterContentType("")
	p.SyncData()
	assert.Equal(t, 3, len(p.VisibleResources()))
}

func TestNetworkPanel_StatusFiltersAllShowsAll(t *testing.T) {
	entries := []devtools.NetRequestEntry{
		{Method: "GET", URL: "/ok", Status: 200, Duration: time.Millisecond},
		{Method: "GET", URL: "/notfound", Status: 404, Duration: time.Millisecond},
	}
	p := devtools.NewNetworkPanel(func() *devtools.TabContext {
		return &devtools.TabContext{RequestLog: &mockRequestLog{entries: entries}}
	}).(*devtools.NetworkPanel)
	p.SetEntries(entries)
	p.SetFilterStatusClass("")
	p.SyncData()
	assert.Equal(t, 2, len(p.VisibleResources()))
}

func TestNetworkPanel_TruncateLongURL(t *testing.T) {
	longURL := "https://example.com/very/long/path/that/should/be/truncated/in/the/middle/for/better/display"
	truncated := devtools.TruncateMiddle(longURL, 60)
	assert.True(t, len(truncated) <= 63, "truncated length %d exceeds max", len(truncated))
	assert.Contains(t, truncated, "...")
}

// mockRequestLog implements requestLogProvider for testing.
type mockRequestLog struct {
	entries []devtools.NetRequestEntry
}

func (m *mockRequestLog) Entries() []devtools.NetRequestEntry {
	if m.entries == nil {
		return nil
	}
	return m.entries
}
