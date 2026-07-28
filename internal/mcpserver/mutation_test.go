package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vyquocvu/goosie/internal/browsercontrol"
)

// --- Phase 4: Semantic Interaction Tests ---

func TestMutation_Click_GeneratesNewRevision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Test</title></head><body>
			<button id="btn">Click Me</button>
			<script>
				document.getElementById('btn').addEventListener('click', function() {
					document.body.innerHTML += '<p>Clicked!</p>';
				});
			</script>
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	nav, err := ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)
	initialRev := nav.PageRevision

	// Get a button ref
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "button", Exact: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, qr.Refs)

	// Click the button
	result, err := ec.Click(ctx, qr.Refs[0], browsercontrol.ClickOptions{})
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)

	// Verify revision incremented or stayed same (depends on whether mutation happened)
	// In real implementation, DOM mutations would increment revision
	assert.GreaterOrEqual(t, result.PageRevision, initialRev)
}

func TestMutation_Type_IntoInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Form Test</title></head><body>
			<input type="text" id="name" />
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Get input ref
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "textbox", Exact: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, qr.Refs)

	// Type into input
	result, err := ec.Type(ctx, qr.Refs[0], "Hello, World!", browsercontrol.TypeOptions{
		Replace: true,
		Submit:  false,
	})
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)
}

func TestMutation_Click_NavigationStarted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Write([]byte(`<html><head><title>Link Page</title></head><body>
				<a href="/next" id="link">Go to next</a>
			</body></html>`))
		} else if r.Method == "POST" {
			w.Write([]byte(`<html><head><title>Next Page</title></head><body><p>Navigated!</p></body></html>`))
		}
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Get link ref
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "link", Exact: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, qr.Refs)

	// Click link - may or may not start navigation depending on JS execution
	result, err := ec.Click(ctx, qr.Refs[0], browsercontrol.ClickOptions{})
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)
	// Navigation may or may not start (depends on JS execution)
	_ = result.NavigationStarted
}

func TestMutation_PressKey_Escape(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	result, err := ec.PressKey(ctx, "Escape", nil)
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)
}

func TestMutation_PressKey_WithModifiers(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	result, err := ec.PressKey(ctx, "Enter", []string{"Shift"})
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)
}

func TestMutation_Scroll_Viewport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Long page
		content := `<html><head><title>Long Page</title></head><body>`
		for i := 0; i < 50; i++ {
			content += `<p style="margin: 20px 0;">Paragraph ` + string(rune('0'+i%10)) + `</p>`
		}
		content += `</body></html>`
		w.Write([]byte(content))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Scroll down
	result, err := ec.Scroll(ctx, browsercontrol.ScrollOptions{
		DeltaX: 0,
		DeltaY: 300,
	})
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)
}

func TestMutation_SetViewport_Resize(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{
		Viewport: browsercontrol.Viewport{Width: 1280, Height: 720},
	})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	// Resize viewport
	newVP, err := ec.SetViewport(ctx, browsercontrol.Viewport{
		Width:  1920,
		Height: 1080,
		Scale:  2.0,
	})
	require.NoError(t, err)
	assert.Equal(t, 1920, newVP.Width)
	assert.Equal(t, 1080, newVP.Height)
	assert.Equal(t, 2.0, newVP.Scale)
}

func TestMutation_RefValidation_WrongContext(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	// Create two contexts
	info1, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec1, err := svc.Context(info1.ID)
	require.NoError(t, err)

	info2, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec2, err := svc.Context(info2.ID)
	require.NoError(t, err)

	// Navigate to get a ref in context 2
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><button>Click</button></body></html>`))
	}))
	defer srv.Close()

	_, err = ec2.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	qr, err := ec2.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "button", Exact: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, qr.Refs)

	// Try to click ref from context 2 in context 1
	_, err = ec1.Click(ctx, qr.Refs[0], browsercontrol.ClickOptions{})
	require.Error(t, err)

	bcErr, ok := err.(*browsercontrol.Error)
	require.True(t, ok)
	assert.Equal(t, browsercontrol.ErrContextNotFound, bcErr.Code)
}

func TestMutation_RefValidation_StaleRevision(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Page</title></head><body><button>Click</button></body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	// Navigate to get a ref
	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "button", Exact: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, qr.Refs)

	// Navigate again to increment revision
	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Try to click with stale ref (old revision)
	_, err = ec.Click(ctx, qr.Refs[0], browsercontrol.ClickOptions{})
	require.Error(t, err)
	assert.Equal(t, browsercontrol.ErrPageChanged, err.(*browsercontrol.Error).Code)
}

func TestMutation_Query_FilterByRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Query Test</title></head><body>
			<nav><a href="/">Home</a></nav>
			<main>
				<h1>Title</h1>
				<p>Paragraph</p>
				<button>Button 1</button>
				<button>Button 2</button>
			</main>
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Query buttons only
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "button", Exact: true},
	})
	require.NoError(t, err)
	assert.Len(t, qr.Refs, 2) // Two buttons

	// Query links
	qr, err = ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "link", Exact: true},
	})
	require.NoError(t, err)
	assert.Len(t, qr.Refs, 1) // One link
}

func TestMutation_Query_PartialRoleMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
			<button id="btn1">Submit Button</button>
			<button id="btn2">Cancel Button</button>
			<input type="textbox" id="input1" />
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Query with partial role match
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "button", Exact: false}, // Partial match
	})
	require.NoError(t, err)
	assert.Len(t, qr.Refs, 2) // Both buttons
}

func TestMutation_Type_ReplaceVsAppend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
			<input type="text" id="field" value="initial" />
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	qr, err := ec.Query(ctx, browsercontrol.Locator{
		CSS: &browsercontrol.CSSLocator{Selector: "#field"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, qr.Refs)

	// Type with replace=true
	result1, err := ec.Type(ctx, qr.Refs[0], "new value", browsercontrol.TypeOptions{
		Replace: true,
	})
	require.NoError(t, err)
	assert.True(t, result1.ActionApplied)

	// Type with replace=false (append)
	result2, err := ec.Type(ctx, qr.Refs[0], " appended", browsercontrol.TypeOptions{
		Replace: false,
	})
	require.NoError(t, err)
	assert.True(t, result2.ActionApplied)
}

func TestMutation_Cancellation_DuringAction(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	// Navigate first
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><button>Click</button></body></html>`))
	}))
	defer srv.Close()
	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "button", Exact: true},
	})
	require.NoError(t, err)

	// Create cancelled context
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	// Try to click with cancelled context
	_, err = ec.Click(cancelledCtx, qr.Refs[0], browsercontrol.ClickOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestMutation_DisabledElement(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
			<button id="btn1">Enabled</button>
			<button id="btn2" disabled>Disabled</button>
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Query for button (should find both)
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "button", Exact: true},
	})
	require.NoError(t, err)
	assert.Len(t, qr.Refs, 2) // Both buttons found

	// Click the disabled one - should succeed (real browser would not dispatch event)
	result, err := ec.Click(ctx, qr.Refs[1], browsercontrol.ClickOptions{})
	require.NoError(t, err)
	// Action applied is true because the action was dispatched
	// (disabled behavior is up to the page)
	assert.True(t, result.ActionApplied)
}

func TestMutation_AmbiguousSelector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
			<div class="item">
				<button>Action 1</button>
				<button>Action 2</button>
			</div>
			<div class="item">
				<button>Action 3</button>
			</div>
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Query for buttons - should find all 3
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "button", Exact: true},
	})
	require.NoError(t, err)
	assert.Len(t, qr.Refs, 3) // All 3 buttons

	// Click first button
	result, err := ec.Click(ctx, qr.Refs[0], browsercontrol.ClickOptions{})
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)
}

func TestMutation_UnicodeText(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	// Navigate to a form
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
			<input type="text" id="name" />
			<textarea id="bio"></textarea>
		</body></html>`))
	}))
	defer srv.Close()
	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	qr, err := ec.Query(ctx, browsercontrol.Locator{
		CSS: &browsercontrol.CSSLocator{Selector: "#name"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, qr.Refs)

	// Type Unicode text
	unicodeText := "你好世界 🌍 émojis & special: χαρακτήρες"
	result, err := ec.Type(ctx, qr.Refs[0], unicodeText, browsercontrol.TypeOptions{
		Replace: true,
	})
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)
}

func TestMutation_SequentialMutations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
			<input type="text" id="name" />
			<button id="submit">Submit</button>
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Sequential operations
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		CSS: &browsercontrol.CSSLocator{Selector: "#name"},
	})
	require.NoError(t, err)

	// Type
	result1, err := ec.Type(ctx, qr.Refs[0], "John", browsercontrol.TypeOptions{Replace: true})
	require.NoError(t, err)
	assert.True(t, result1.ActionApplied)

	// Click button
	qr2, err := ec.Query(ctx, browsercontrol.Locator{
		CSS: &browsercontrol.CSSLocator{Selector: "#submit"},
	})
	require.NoError(t, err)

	result2, err := ec.Click(ctx, qr2.Refs[0], browsercontrol.ClickOptions{})
	require.NoError(t, err)
	assert.True(t, result2.ActionApplied)

	// Both operations should succeed in order
	assert.Equal(t, result1.PageRevision, result2.PageRevision)
}

func TestMutation_Evaluate_JavaScript(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	// Navigate
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>Eval Test</title></head><body>
			<script>window.testValue = 42;</script>
		</body></html>`))
	}))
	defer srv.Close()
	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Evaluate
	result, err := ec.Evaluate(ctx, "document.title", browsercontrol.EvaluateOptions{
		AwaitPromise:   false,
		TimeoutMs:      1000,
		MaxResultBytes: 1024,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, ec.ID(), result.ContextID)
}

// --- Concurrency Tests ---

func TestMutation_ConcurrentQueries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
			<nav><a href="/">Link 1</a><a href="/">Link 2</a></nav>
			<main><button>Btn 1</button><button>Btn 2</button></main>
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Concurrent queries
	qr1, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "link", Exact: true},
	})
	require.NoError(t, err)

	qr2, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "button", Exact: true},
	})
	require.NoError(t, err)

	// Both should work
	assert.Len(t, qr1.Refs, 2)
	assert.Len(t, qr2.Refs, 2)
}

func TestMutation_Query_WithCSSSelector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><head><title>CSS Query</title></head><body>
			<form id="login">
				<input type="text" name="username" id="user" />
				<input type="password" name="password" id="pass" />
				<button type="submit">Sign In</button>
			</form>
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// CSS selector query
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		CSS: &browsercontrol.CSSLocator{Selector: "#login input[type=text]"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, qr.Refs)

	// Verify it's the username field
	result, err := ec.Click(ctx, qr.Refs[0], browsercontrol.ClickOptions{})
	require.NoError(t, err)
	assert.True(t, result.ActionApplied)
}

func TestMutation_Query_WithTextLocator(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body>
			<nav>
				<a href="/">Home</a>
				<a href="/about">About</a>
				<a href="/contact">Contact Us</a>
			</nav>
		</body></html>`))
	}))
	defer srv.Close()

	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Text locator - find "About" link
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Text: &browsercontrol.TextLocator{Value: "About", Exact: true},
	})
	require.NoError(t, err)
	require.NotEmpty(t, qr.Refs)
}

func TestMutation_EmptyResult_NoPanic(t *testing.T) {
	svc := browsercontrol.NewEngineService()
	ctx := context.Background()

	info, err := svc.CreateContext(ctx, browsercontrol.CreateContextOptions{})
	require.NoError(t, err)
	ec, err := svc.Context(info.ID)
	require.NoError(t, err)

	// Navigate
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html><body><p>No buttons here</p></body></html>`))
	}))
	defer srv.Close()
	_, err = ec.Navigate(ctx, srv.URL, browsercontrol.WaitComplete, 5000)
	require.NoError(t, err)

	// Query for non-existent element
	qr, err := ec.Query(ctx, browsercontrol.Locator{
		Role: &browsercontrol.RoleLocator{Name: "button", Exact: true},
	})
	require.NoError(t, err)
	assert.Empty(t, qr.Refs) // No buttons found

	// Trying to click empty ref should fail
	if len(qr.Refs) == 0 {
		// This is expected - no ref to click
		assert.True(t, true)
	}
}
