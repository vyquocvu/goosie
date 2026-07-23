package browsercontrol

import (
	"time"
)

// WaitCondition represents the document lifecycle stage to wait for
// during or after navigation.
type WaitCondition string

const (
	WaitCommit      WaitCondition = "commit"
	WaitInteractive WaitCondition = "interactive"
	WaitComplete    WaitCondition = "complete"
)

// ContextState describes the current state of a browser context.
type ContextState string

const (
	ContextCreated    ContextState = "created"
	ContextNavigating ContextState = "navigating"
	ContextParsing    ContextState = "parsing"
	ContextInteractive ContextState = "interactive"
	ContextComplete   ContextState = "complete"
	ContextFailed     ContextState = "failed"
	ContextCancelled  ContextState = "cancelled"
	ContextClosed     ContextState = "closed"
)

// LocatorKind identifies which locator strategy to use in a query.
type LocatorKind string

const (
	LocatorRole LocatorKind = "role"
	LocatorCSS  LocatorKind = "css"
	LocatorText LocatorKind = "text"
)

// Viewport represents a browser viewport size and scale.
type Viewport struct {
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Scale  float64 `json:"scale"`
}

// ContextInfo describes a browser context.
type ContextInfo struct {
	ID           string        `json:"contextId"`
	State        ContextState  `json:"state"`
	PageRevision int           `json:"pageRevision"`
	Viewport     Viewport      `json:"viewport"`
	CreatedAt    time.Time     `json:"-"`
}

// NavigationResult is returned after a navigate call.
type NavigationResult struct {
	ContextID         string        `json:"contextId"`
	NavigationID      string        `json:"navigationId,omitempty"`
	URL               string        `json:"url"`
	State             ContextState  `json:"state"`
	WaitConditionMet  bool          `json:"waitConditionMet"`
	PageRevision      int           `json:"pageRevision"`
	HTTPStatus        int           `json:"httpStatus"`
}

// SnapshotFormat controls the output shape of a page snapshot.
type SnapshotFormat string

const (
	SnapshotSemantic SnapshotFormat = "semantic"
)

// SnapshotOptions configures a page snapshot.
type SnapshotOptions struct {
	Format       SnapshotFormat
	MaxDepth     int
	MaxNodes     int
	IncludeHidden bool
}

// SemanticNode is a single node in a semantic page snapshot.
type SemanticNode struct {
	Role        string   `json:"role"`
	Name        string   `json:"name,omitempty"`
	Level       int      `json:"level,omitempty"`
	Ref         string   `json:"ref,omitempty"`
	Children    []SemanticNode `json:"children,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
	Checked     *bool    `json:"checked,omitempty"`
	Disabled    bool     `json:"disabled,omitempty"`
	Hidden      bool     `json:"hidden,omitempty"`
}

// PageSnapshot is a bounded semantic view of the current page.
type PageSnapshot struct {
	ContextID    string         `json:"contextId"`
	PageRevision int            `json:"pageRevision"`
	URL          string         `json:"url"`
	Title        string         `json:"title"`
	Viewport     Viewport       `json:"viewport"`
	Nodes        []SemanticNode `json:"nodes"`
	Truncated    bool           `json:"truncated"`
}

// ElementRef is an opaque reference to a DOM element, valid only for its
// originating context and page revision.
type ElementRef struct {
	Ref          string `json:"ref"`
	ContextID    string `json:"contextId"`
	PageRevision int    `json:"pageRevision"`
}

// RoleLocator finds elements by semantic role and accessible name.
type RoleLocator struct {
	Name    string `json:"name"`
	Exact   bool   `json:"exact"`
}

// CSSLocator finds elements via CSS selector.
type CSSLocator struct {
	Selector string `json:"selector"`
}

// TextLocator finds elements by visible text content.
type TextLocator struct {
	Value string `json:"value"`
	Exact bool   `json:"exact"`
}

// Locator is a union of one locator strategy (exactly one field set).
type Locator struct {
	Role *RoleLocator `json:"role,omitempty"`
	CSS  *CSSLocator  `json:"css,omitempty"`
	Text *TextLocator `json:"text,omitempty"`
}

// QueryResult contains element references matching a locator.
type QueryResult struct {
	ContextID    string        `json:"contextId"`
	PageRevision int           `json:"pageRevision"`
	Refs         []ElementRef  `json:"refs"`
}

// ClickOptions configures a click action.
type ClickOptions struct {
	Button    string
	TimeoutMs int
}

// ActionResult describes the outcome of a page mutation.
type ActionResult struct {
	ContextID          string `json:"contextId"`
	PageRevision       int    `json:"pageRevision"`
	ActionApplied      bool   `json:"actionApplied"`
	NavigationStarted  bool   `json:"navigationStarted"`
	NavigationID       string `json:"navigationId,omitempty"`
}

// TypeOptions configures a type action.
type TypeOptions struct {
	Replace bool
	Submit  bool
}

// ScrollOptions configures a scroll operation.
type ScrollOptions struct {
	Target    string
	DeltaX    int
	DeltaY    int
}

// EvaluateOptions configures JavaScript evaluation.
type EvaluateOptions struct {
	AwaitPromise  bool
	TimeoutMs     int
	MaxResultBytes int
}

// EvaluationResult contains the result of JS evaluation.
type EvaluationResult struct {
	ContextID    string      `json:"contextId"`
	PageRevision int         `json:"pageRevision"`
	Type         string      `json:"type"`
	Value        interface{} `json:"value,omitempty"`
	IsError      bool        `json:"isError"`
	ErrorText    string      `json:"errorText,omitempty"`
}

// ScreenshotOptions configures a page screenshot.
type ScreenshotOptions struct {
	Scope          string
	OmitBackground bool
}

// ScreenshotResult contains an encoded page screenshot.
type ScreenshotResult struct {
	ContextID    string `json:"contextId"`
	PageRevision int    `json:"pageRevision"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Data         []byte `json:"-"` // PNG-encoded image data
	MIMEType     string `json:"mimeType"`
	Truncated    bool   `json:"truncated"`
}

// WaitOptions configures a wait operation.
type WaitOptions struct {
	Condition  WaitCondition
	URLPattern string
	MinRevision int
	TimeoutMs  int
}

// WaitResult contains the outcome of a wait.
type WaitResult struct {
	ContextID    string       `json:"contextId"`
	PageRevision int          `json:"pageRevision"`
	State        ContextState `json:"state"`
	URL          string       `json:"url"`
	ConditionMet bool         `json:"conditionMet"`
}

// ConsoleEntry represents one console message.
type ConsoleEntry struct {
	Level     string `json:"level"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

// ConsolePage is a bounded page of console entries.
type ConsolePage struct {
	ContextID    string         `json:"contextId"`
	PageRevision int            `json:"pageRevision"`
	Entries      []ConsoleEntry `json:"entries"`
	Dropped      int            `json:"dropped"`
	Cursor       string         `json:"cursor"`
}

// NetworkEntry represents one network request in the log.
type NetworkEntry struct {
	Method      string `json:"method"`
	URL         string `json:"url"`
	Status      int    `json:"status"`
	Type        string `json:"type"`
	Size        int64  `json:"size"`
	DurationMs  int    `json:"durationMs"`
}

// NetworkPage is a bounded page of network entries.
type NetworkPage struct {
	ContextID    string          `json:"contextId"`
	PageRevision int             `json:"pageRevision"`
	Entries      []NetworkEntry  `json:"entries"`
	Dropped      int             `json:"dropped"`
	Cursor       string          `json:"cursor"`
}

// SecuritySummary contains TLS/CSP/policy information.
type SecuritySummary struct {
	ContextID    string `json:"contextId"`
	PageRevision int    `json:"pageRevision"`
	Scheme       string `json:"scheme"`
	Subject      string `json:"subject,omitempty"`
	Issuer       string `json:"issuer,omitempty"`
	NotBefore    string `json:"notBefore,omitempty"`
	NotAfter     string `json:"notAfter,omitempty"`
	CSPEnabled   bool   `json:"cspEnabled"`
}

// CreateContextOptions configures a new browser context.
type CreateContextOptions struct {
	Viewport          Viewport
	JavaScriptEnabled bool
}

// Default max contexts for a server.
const DefaultMaxContexts = 10

// Default timeout for operations.
const DefaultTimeoutMs = 10000

// Hard limits for snapshot.
const (
	MaxSnapshotDepth = 50
	MaxSnapshotNodes = 5000
	MaxSnapshotBytes = 1 * 1024 * 1024 // 1 MiB
)

// Hard limits for screenshot.
const (
	MaxScreenshotPixels  = 16 * 1000 * 1000 // 16 MP
	MaxScreenshotEncoded = 8 * 1024 * 1024  // 8 MiB
)

// Hard limits for evaluation.
const (
	MaxSourceBytes      = 256 * 1024 // 256 KiB
	MaxEvalDurationMs   = 5000
	MaxEvalResultBytes  = 1 * 1024 * 1024 // 1 MiB
	MaxEvalResultDepth  = 20
)

// Hard limits for URL.
const MaxURLLength = 8192
