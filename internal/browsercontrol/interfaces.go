package browsercontrol

import "context"

// Service manages a set of browser contexts. It is the top-level entry point
// for browser automation, independent of any protocol (MCP, CLI, etc.).
type Service interface {
	// CreateContext creates a new private ephemeral browser context.
	CreateContext(ctx context.Context, opts CreateContextOptions) (ContextInfo, error)

	// Context returns the live Context handle for an existing
	// context ID. The returned value is ready to receive method
	// calls (Navigate, Snapshot, etc.). Returns ErrContextNotFound
	// when no context with the given ID is open.
	Context(ctx context.Context, id string) (Context, error)

	// ListContexts returns all contexts owned by this service instance.
	ListContexts(ctx context.Context) ([]ContextInfo, error)

	// CloseContext idempotently closes a context and releases all resources.
	CloseContext(ctx context.Context, id string) error
}

// Context represents a single browser tab or page. All methods accept
// context.Context for cancellation and deadlines.
type Context interface {
	// ID returns the unique context identifier.
	ID() string

	// Info returns current context metadata.
	Info(ctx context.Context) (ContextInfo, error)

	// Navigate loads a URL and optionally waits for a lifecycle condition.
	Navigate(ctx context.Context, url string, waitUntil WaitCondition, timeoutMs int) (NavigationResult, error)

	// Wait blocks until a condition is met, cancelled, or the deadline passes.
	Wait(ctx context.Context, opts WaitOptions) (WaitResult, error)

	// Snapshot returns a bounded semantic page snapshot with opaque element refs.
	Snapshot(ctx context.Context, opts SnapshotOptions) (PageSnapshot, error)

	// Screenshot captures the current viewport as a PNG.
	Screenshot(ctx context.Context, opts ScreenshotOptions) (ScreenshotResult, error)

	// Query resolves a locator to element references on the current page.
	Query(ctx context.Context, locator Locator) (QueryResult, error)

	// Click activates an element identified by reference.
	Click(ctx context.Context, ref ElementRef, opts ClickOptions) (ActionResult, error)

	// Type enters text into an element identified by reference.
	Type(ctx context.Context, ref ElementRef, text string, opts TypeOptions) (ActionResult, error)

	// PressKey dispatches a key chord.
	PressKey(ctx context.Context, key string, modifiers []string) (ActionResult, error)

	// Scroll scrolls the viewport or a referenced element.
	Scroll(ctx context.Context, opts ScrollOptions) (ActionResult, error)

	// SetViewport resizes and rescales the viewport.
	SetViewport(ctx context.Context, vp Viewport) (Viewport, error)

	// Evaluate runs JavaScript in the page context.
	Evaluate(ctx context.Context, source string, opts EvaluateOptions) (EvaluationResult, error)

	// Console returns a bounded page of recent console entries.
	Console(ctx context.Context, cursor string, limit int) (ConsolePage, error)

	// Network returns a bounded page of recent network entries.
	Network(ctx context.Context, cursor string, limit int) (NetworkPage, error)

	// Security returns the current TLS and security summary.
	Security(ctx context.Context) (SecuritySummary, error)
}
