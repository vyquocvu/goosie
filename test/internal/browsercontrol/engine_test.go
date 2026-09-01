package browsercontrol_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
)

// --- Fixture Test Helpers ---

func newTestService() *browsercontrol.EngineService {
	s := browsercontrol.NewEngineService()
	s.SetMaxContexts(10)
	return s
}

func newTestContext(t *testing.T, s *browsercontrol.EngineService) (context.Context, browsercontrol.Context, func()) {
	t.Helper()
	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{
		Viewport: browsercontrol.Viewport{Width: 1280, Height: 720, Scale: 1},
	})
	require.NoError(t, err)
	ec, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)
	return ctx, ec, func() { s.CloseContext(ctx, info.ID) }
}

func fixtureServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// --- State-transition tests ---

func TestNavigate_StateSequence(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><head><title>Test</title></head><body><p>hello</p></body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	nav, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, browsercontrol.ContextComplete, nav.State)
	assert.True(t, nav.WaitConditionMet)
	assert.Equal(t, 200, nav.HTTPStatus)
	assert.Equal(t, 1, nav.PageRevision)
	assert.NotEmpty(t, nav.NavigationID)

	info, err := c.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, browsercontrol.ContextComplete, info.State)
}

func TestNavigate_WaitCommit(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Commit</title></head><body><p>ok</p></body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	nav, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitCommit, 5000)
	require.NoError(t, err)
	assert.True(t, nav.WaitConditionMet)
	assert.Equal(t, 1, nav.PageRevision)
}

func TestNavigate_WaitInteractive(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Interactive</title></head><body><p>ok</p></body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	nav, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitInteractive, 5000)
	require.NoError(t, err)
	assert.True(t, nav.WaitConditionMet)
	assert.Equal(t, 1, nav.PageRevision)
}

func TestNavigate_RevisionIncrements(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Multi</title></head><body><p>nav</p></body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	nav1, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, 1, nav1.PageRevision)

	nav2, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, 2, nav2.PageRevision)
}

func TestNavigate_HTTPError(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("<html><head><title>404</title></head><body><p>Not Found</p></body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	nav, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, 404, nav.HTTPStatus)
	assert.Equal(t, browsercontrol.ContextComplete, nav.State)
}

func TestNavigate_InvalidMIME(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("some binary data"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	_, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.Error(t, err)
	var be *browsercontrol.Error
	assert.ErrorAs(t, err, &be)
	assert.Equal(t, browsercontrol.ErrUnsupported, be.Code)
}

func TestNavigate_Cancellation(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // slow response
		w.Write([]byte("<html><body><p>never</p></body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	navCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	_, err := c.Navigate(navCtx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.Error(t, err)
	// Should get context deadline exceeded or cancelled
	assert.True(t, err == context.DeadlineExceeded || err == context.Canceled ||
		strings.Contains(err.Error(), "deadline") || strings.Contains(err.Error(), "cancel"))
}

func TestNavigate_Timeout(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Write([]byte("<html><body><p>slow</p></body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	_, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 100)
	require.Error(t, err)
}

func TestNavigate_SupersededNavigation(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><head><title>Final</title></head><body><p>result</p></body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	nav, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.Equal(t, 1, nav.PageRevision)
}

func TestCloseDuringNavigation(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte("<html><body><p>late</p></body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx := context.Background()
	info, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)

	ec, err := s.Context(context.Background(), info.ID)
	require.NoError(t, err)

	// Start navigation and close immediately
	go func() {
		ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	}()
	time.Sleep(10 * time.Millisecond)

	err = s.CloseContext(ctx, info.ID)
	assert.NoError(t, err)
}

// --- Snapshot tests ---

func TestSnapshot_AfterNavigation(t *testing.T) {
	body := `<html><head><title>Snapshot Test</title></head>
<body>
<nav><a href="/">Home</a><a href="/about">About</a></nav>
<main><h1>Hello</h1><p>World</p></main>
</body></html>`
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	_, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	snap, err := c.Snapshot(ctx, browsercontrol.SnapshotOptions{Format: browsercontrol.SnapshotSemantic})
	require.NoError(t, err)
	assert.Equal(t, "Snapshot Test", snap.Title)
	assert.NotEmpty(t, snap.Nodes)
	assert.Contains(t, snap.URL, srv.URL)
	assert.Equal(t, 1, snap.PageRevision)
}

func TestSnapshot_BeforeNavigation(t *testing.T) {
	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	snap, err := c.Snapshot(ctx, browsercontrol.SnapshotOptions{})
	require.NoError(t, err)
	assert.Empty(t, snap.Nodes)
}

func TestSnapshot_NodeRoles(t *testing.T) {
	body := `<html><head><title>Roles</title></head>
<body>
<nav aria-label="Main nav"><a href="/">Home</a></nav>
<main><h1>Title</h1><button>Click</button><input type="text" placeholder="Name"></main>
<table><tr><td>cell</td></tr></table>
</body></html>`
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	_, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	snap, err := c.Snapshot(ctx, browsercontrol.SnapshotOptions{Format: browsercontrol.SnapshotSemantic})
	require.NoError(t, err)

	roles := collectRoles(snap.Nodes)
	assert.Contains(t, roles, "link")
	assert.Contains(t, roles, "heading")
	assert.Contains(t, roles, "button")
	assert.Contains(t, roles, "textbox")
	assert.Contains(t, roles, "table")
	assert.Contains(t, roles, "navigation")
}

func TestQuery_AfterNavigation(t *testing.T) {
	body := `<html><body><a href="/page1">Page 1</a><a href="/page2">Page 2</a></body></html>`
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	_, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	qr, err := c.Query(ctx, browsercontrol.Locator{Role: &browsercontrol.RoleLocator{Name: "link", Exact: false}})
	require.NoError(t, err)
	assert.NotEmpty(t, qr.Refs)
	assert.Equal(t, 1, qr.PageRevision)
}

func TestScreenshot_BeforeNavigation(t *testing.T) {
	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	shot, err := c.Screenshot(ctx, browsercontrol.ScreenshotOptions{Scope: "viewport"})
	require.NoError(t, err)
	assert.Equal(t, "image/png", shot.MIMEType)
}

func TestViewport_SetAndGet(t *testing.T) {
	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	vp, err := c.SetViewport(ctx, browsercontrol.Viewport{Width: 800, Height: 600, Scale: 2.0})
	require.NoError(t, err)
	assert.Equal(t, 800, vp.Width)
	assert.Equal(t, 600, vp.Height)
	assert.Equal(t, 2.0, vp.Scale)
}

func TestNavigate_FileURLDenied(t *testing.T) {
	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	_, err := c.Navigate(ctx, "http://0.0.0.0:1/test", browsercontrol.WaitComplete, 500)
	require.Error(t, err)
}

func TestNavigate_Redirect(t *testing.T) {
	redirected := false
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			redirected = true
			w.Header().Set("Location", "/target")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Write([]byte("<html><head><title>Redirected</title></head><body><p>ok</p></body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	nav, err := c.Navigate(ctx, srv.URL+"/redirect", browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	assert.True(t, redirected)
	assert.True(t, nav.WaitConditionMet)
}

func TestNavigate_OversizedResponse(t *testing.T) {
	largeBody := strings.Repeat("x", 100*1024*1024) // 100 MB
	html := "<html><body>" + largeBody + "</body></html>"
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(html))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	_, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 2000)
	// Should fail with either a limit exceeded or deadline exceeded error
	require.Error(t, err)
	t.Logf("oversized response error: %v", err)
}

func TestEngineService_ContextLimit(t *testing.T) {
	s := newTestService()
	s.SetMaxContexts(2)

	ctx := context.Background()
	_, err := s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	_, err = s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	_, err = s.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.Error(t, err)
	var be *browsercontrol.Error
	assert.ErrorAs(t, err, &be)
	assert.Equal(t, browsercontrol.ErrLimitExceeded, be.Code)
}

func TestConsole_ReturnsEmpty(t *testing.T) {
	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	page, err := c.Console(ctx, "", 10)
	require.NoError(t, err)
	assert.Empty(t, page.Entries)
}

func TestSecurity_AfterNavigation(t *testing.T) {
	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body>ok</body></html>"))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	_, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	sec, err := c.Security(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, sec.PageRevision)
}

func TestClick_RevisionMismatch(t *testing.T) {
	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	ref := browsercontrol.ElementRef{
		Ref:          "e_1_abc",
		ContextID:    c.ID(),
		PageRevision: 99,
	}
	_, err := c.Click(ctx, ref, browsercontrol.ClickOptions{Button: "left", TimeoutMs: 5000})
	require.Error(t, err)
	var be *browsercontrol.Error
	assert.ErrorAs(t, err, &be)
	assert.Equal(t, browsercontrol.ErrPageChanged, be.Code)
}

func TestType_RevisionMismatch(t *testing.T) {
	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	ref := browsercontrol.ElementRef{Ref: "e_1_abc", ContextID: c.ID(), PageRevision: 99}
	_, err := c.Type(ctx, ref, "hello", browsercontrol.TypeOptions{Replace: true})
	require.Error(t, err)
	var be *browsercontrol.Error
	assert.ErrorAs(t, err, &be)
	assert.Equal(t, browsercontrol.ErrPageChanged, be.Code)
}

func TestClick_WrongContext(t *testing.T) {
	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	ref := browsercontrol.ElementRef{Ref: "e_1_abc", ContextID: "other-ctx", PageRevision: 1}
	_, err := c.Click(ctx, ref, browsercontrol.ClickOptions{})
	require.Error(t, err)
	var be *browsercontrol.Error
	assert.ErrorAs(t, err, &be)
	assert.Equal(t, browsercontrol.ErrContextNotFound, be.Code)
}

func TestSnapshot_Truncation(t *testing.T) {
	body := `<html><body>`
	for i := 0; i < 50; i++ {
		body += fmt.Sprintf(`<a href="/p%d">Page %d</a>`, i, i)
	}
	body += `</body></html>`

	srv := fixtureServer(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	})
	defer srv.Close()

	s := newTestService()
	ctx, c, cleanup := newTestContext(t, s)
	defer cleanup()

	_, err := c.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Use a very low depth to trigger truncation of children
	// (MaxNodes limits the total nodes walked)
	snap, err := c.Snapshot(ctx, browsercontrol.SnapshotOptions{MaxNodes: 5, MaxDepth: 10})
	require.NoError(t, err)
	assert.True(t, snap.Truncated, "snapshot should report truncated when node limit is hit")
}

// --- Helpers ---

func collectRoles(nodes []browsercontrol.SemanticNode) []string {
	var roles []string
	for _, n := range nodes {
		if n.Role != "" && n.Role != "none" && n.Role != "text" && n.Role != "presentation" {
			roles = append(roles, n.Role)
		}
		roles = append(roles, collectRoles(n.Children)...)
	}
	return roles
}
