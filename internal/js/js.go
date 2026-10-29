// Package js executes a document's classic scripts against a host-provided
// window and document stub.
//
// A Runtime belongs to exactly one document: nothing in this package is
// process-global, so two tabs cannot read each other's bindings and tearing a
// document down releases its realm. That ownership rule is the reason the API
// takes an Options struct per runtime rather than exposing setters.
//
// Scope is roadmap gate 6, sub-project 1: <script> elements run in document
// order, each one blocking the parse until it returns; console output reaches a
// host writer; window and document are stubs. Tasks, microtasks, DOM mutation
// and Fetch belong to the later sub-projects, and calling an API outside this
// scope must fail a script loudly rather than resolve to undefined.
package js

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/media"
)

// DefaultTimeout bounds one script's execution when Options.Timeout is zero.
// A script that runs past it is stopped where it stands and reported through
// ErrInterrupted; the bound exists because a page that spins takes the tab's
// event loop with it, and the tab shares a process with every other tab.
const DefaultTimeout = 5 * time.Second

// MaxScriptBytes is the source limit for a single script. The document byte
// guard already bounds the page; this bounds the compile, so one oversized
// literal is refused before it is parsed rather than after.
const MaxScriptBytes = 1 << 20

// MaxMicrotasksPerCheckpoint bounds one microtask drain. A chain that
// schedules forever without touching the VM would otherwise spin past the
// timeout interrupt (which only lands on VM calls); the bound ends it with
// ErrInterrupted like any other runaway, consistent with every other
// admission limit in the engine.
const MaxMicrotasksPerCheckpoint = 100000

// timeout returns the bound for one user-code invocation: the configured
// timeout or the default. Run, Tick and the pump-driven callbacks all share
// it, so a hung callback wedges its frame rather than its thread.
func (r *Runtime) timeout() time.Duration {
	if r.opts.Timeout > 0 {
		return r.opts.Timeout
	}
	return DefaultTimeout
}

// callBounded invokes fn under the runtime timeout, reporting ErrInterrupted
// when the bound fires. Non-interrupt failures pass through untouched, so
// callers keep their existing error semantics and only add the stop rule.
func (r *Runtime) callBounded(fn func() (goja.Value, error)) (goja.Value, error) {
	t := time.AfterFunc(r.timeout(), func() { r.vm.Interrupt(ErrInterrupted) })
	defer t.Stop()
	r.scriptExecutionDepth++
	var v goja.Value
	var err error
	func() {
		defer func() { r.scriptExecutionDepth-- }()
		v, err = fn()
	}()
	if err == nil {
		return v, nil
	}
	var ie *goja.InterruptedError
	if errors.As(err, &ie) {
		return v, ErrInterrupted
	}
	return v, err
}

var (
	// ErrInterrupted reports a script stopped by its timeout or by Interrupt.
	ErrInterrupted = errors.New("js: script execution interrupted")
	// ErrClosed reports use of a runtime whose document has gone away.
	ErrClosed = errors.New("js: runtime is closed")
	// ErrTooLarge reports a script whose source exceeds MaxScriptBytes.
	ErrTooLarge = errors.New("js: script source exceeds the limit")
	// ErrNotImplemented is kept for compatibility but no longer returned.
	ErrNotImplemented = errors.New("js: runtime not implemented (roadmap gate 6, sub-project 1)")
)

// Options configures one Runtime.
type Options struct {
	// ScriptingDisabled makes this document's HTML scripting flag disabled.
	// It is set for sandboxed child documents that do not allow scripts.
	ScriptingDisabled bool
	// Timeout bounds a single Run. Zero selects DefaultTimeout; a negative
	// value is rejected rather than treated as unlimited, because "unlimited"
	// is not a bound this host is willing to give untrusted code.
	Timeout time.Duration
	// Console receives console.log, warn and error. nil discards: a runtime
	// never writes to a process stream the host did not hand it.
	Console io.Writer
	// URL is the document's URL, exposed as window.location.href and used to
	// attribute a script error to the file that threw.
	URL string
	// Title seeds document.title.
	Title string
	// DOM is the parsed document tree. When non-nil, document.querySelector,
	// document.querySelectorAll, document.createElement and element methods
	// are available to scripts. When nil, those APIs do not exist on the
	// document object.
	DOM *dom.Document

	// HTTPClient is the fetch backend. When non-nil, fetch() is available
	// to scripts. When nil, fetch() is not defined and calling it produces
	// a ReferenceError.
	HTTPClient HTTPFetcher

	// Cookies is the cookie jar document.cookie reads and writes. When nil,
	// the binding keeps a per-document memory jar instead, which round-trips
	// script-set values for the document's own URL.
	Cookies CookieStore
	// ClipboardProvider connects navigator.clipboard text methods to a host
	// clipboard. When nil, the runtime uses an isolated in-memory clipboard.
	ClipboardProvider ClipboardProvider
	// ClipboardPermissionState returns the effective Permissions API state for
	// clipboard-read or clipboard-write: granted, denied, or prompt. Nil means
	// an unconfigured per-runtime clipboard with no persistent site decision.
	ClipboardPermissionState func(documentURL, name string) string

	// WebSocketDialer is the WebSocket backend. When non-nil, WebSocket is
	// available to scripts. When nil, WebSocket is not defined.
	WebSocketDialer WebSocketDialer

	// WorkerDialer is the Web Worker backend. When non-nil, Worker is
	// available to scripts. When nil, Worker is not defined.
	WorkerDialer WorkerDialer
	// WorkerScriptFetcher loads a classic worker script or importScripts()
	// dependency. The host owns URL policy and must bound the returned source.
	WorkerScriptFetcher func(scriptURL string) ([]byte, error)

	// CSP is the parsed Content-Security-Policy for this document. When
	// non-nil, script/style/connect sources are checked against it.
	CSP *CSPPolicy

	// OnMutation is called when scripts mutate the DOM. The engine uses
	// this to trigger re-style and re-layout. nil means mutations are
	// not tracked (e.g., in tests).
	OnMutation func()
	// OnFocusChange receives programmatic HTMLElement.focus()/blur() calls.
	// The engine supplies this after binding the runtime to a Session.
	OnFocusChange func(*dom.Node, bool)
	// OnFocusRestore restores focus without scrolling the target into view.
	// Dialog close uses this hook for the HTML focusing steps.
	OnFocusRestore func(*dom.Node)
	// OnUserInput is an internal host hook used by trusted input adapters to
	// mark a text-control value change as user initiated.
	OnUserInput func(*dom.Node) bool
	// OnNumberInputEdit records whether host-simulated number input contains
	// unrepresentable user text. Script value assignments clear this state.
	OnNumberInputEdit func(*dom.Node, bool)
	// OnEmailInputEdit updates the unsanitized editing text displayed for an
	// email control while its DOM value remains sanitized.
	OnEmailInputEdit func(*dom.Node, string)
	// OnActiveState updates the host's :active state while a user activation is
	// held. It is exposed only to host-installed input adapters.
	OnActiveState func(*dom.Node, bool)
	// OnTabNavigation delegates a host-generated Tab action to the browsing
	// context's sequential focus navigation algorithm.
	OnTabNavigation func(backwards bool)
	// OnTabNavigationFrom optionally advances focus from a host input target.
	OnTabNavigationFrom func(from *dom.Node, backwards bool)
	// OnTestDriverKeyDefault applies the default action for a WPT testdriver
	// keyboard key after its key event has been dispatched.
	OnTestDriverKeyDefault func(key string, shift, ctrl, alt, meta bool)
	// ActiveElement returns the currently focused element for document.activeElement.
	ActiveElement func() *dom.Node
	// OnEscapeKey handles the user-agent default action for an unprevented
	// Escape key generated by a host input adapter.
	OnEscapeKey func() bool
	// OnTestDriverPointer feeds WPT adapter pointer actions into the host
	// interaction model. It is not exposed to page-authored pointer events.
	OnTestDriverPointer func(action string, x, y float32, button int) bool

	// Storage supplies origin-scoped Web Storage (localStorage, sessionStorage).
	// When nil, ephemeral in-memory storage is created per runtime.
	Storage *OriginStorageRegistry

	// BroadcastHub connects BroadcastChannel instances across runtimes (tabs).
	// When nil, a package-level default hub is used so channels within a single
	// runtime can still exchange messages.
	BroadcastHub *BroadcastHub

	// SessionID identifies the tab/browsing context for sessionStorage isolation.
	SessionID string

	// DarkMode indicates whether the host environment prefers a dark color scheme.
	DarkMode bool

	// HighContrast indicates whether the host environment prefers increased contrast.
	HighContrast bool

	// ImageLoader resolves the source a script assigned to an
	// HTMLImageElement (the `Image` constructor path). The engine installs its
	// own loader so a script-created image passes the same encoded-size,
	// dimension and decoded-pixel caps a parsed <img> passes. When nil, an
	// image that is given a source settles as an error instead of decoding:
	// the binding never decodes bytes itself, because internal/js may not
	// import internal/image (see internal/archtest/rules.go).
	ImageLoader ImageLoader

	// ImagePixelSink receives the decoded pixels of a successful script image
	// load, together with the node that asked and the raw attribute value the
	// result belongs to. It runs on the JS goroutine before the load event is
	// dispatched and is never called on a failed load. The value is opaque to
	// internal/js; only the embedder interprets it (or, as with internal/engine,
	// converts it into paint state without importing it here).
	ImagePixelSink func(node *dom.Node, rawSrc string, pixels any)
}

// Kind is the JS type of a Value read back out of a runtime.
type Kind uint8

const (
	KindUndefined Kind = iota
	KindNull
	KindBool
	KindNumber
	KindString
	KindObject
	KindFunction
)

// Value is a global's value as seen from the host side.
type Value struct {
	Kind Kind
	Num  float64
	Str  string
	Bool bool
}

// Error is an uncaught exception from one script. It names the source and line
// so a page author sees which file broke; the load itself continues.
type Error struct {
	SourceURL string
	Line      int
	Message   string
}

func (e *Error) Error() string {
	return e.SourceURL + ":" + strconv.Itoa(e.Line) + ": " + e.Message
}

// timer represents a pending setTimeout or setInterval callback.
type timer struct {
	id        int
	callback  goja.Value
	interval  time.Duration
	nextFire  time.Time
	repeat    bool // true for setInterval, false for setTimeout
	fromDefer bool
}

// Runtime executes scripts for one document.
type Runtime struct {
	vm                       *goja.Runtime
	opts                     Options
	title                    string
	closed                   bool
	workerClosed             bool
	workerParent             *Runtime
	crossOriginIsolated      bool
	userInput                bool
	userInputType            string
	transientUserActivation  bool
	hasBeenActive            bool
	transientActivationUntil time.Time
	testUserActivationHook   bool
	documentDomains          map[*dom.Document]string
	mu                       sync.Mutex
	timers                   map[int]*timer
	nextTimerID              int
	inDeferScript            bool
	scriptExecutionDepth     int
	testDriverTrustedClick   bool

	// microtasks is the FIFO promise-reaction queue. Producers on any
	// goroutine only ever append (queueMicrotask); only the Run/Tick
	// goroutine drains (drainMicrotasks), so user callbacks always run on
	// the runtime's own thread even when a network event settled.
	microtasks []func() error

	// DOM bridge state. nil when Options.DOM is nil.
	domDoc                    *dom.Document
	selection                 *jsSelection
	textControlSelections     map[*dom.Node]textControlSelection
	documentSelections        map[*dom.Document]*jsSelection
	selectionWrappers         map[*jsSelection]*goja.Object
	nodeRegistry              map[int]*dom.Node
	genericEventTargets       map[*goja.Object][]*genericEventListener
	currentEventStack         []goja.Value
	legacyWindowEvent         goja.Value
	popoverToggleTasks        map[*dom.Node][]*popoverToggleTask
	detailsToggleTasks        map[*dom.Node]*detailsToggleTask
	detailsInitialToggleDocs  map[*dom.Document]bool
	detailsInitialToggleNodes map[*dom.Node]bool
	dialogFocusReturn         map[*dom.Node]*dom.Node
	popoverTransitions        map[*dom.Node]bool
	popoverFocusTransitions   map[*dom.Node]bool
	popoverInvokers           map[*dom.Node]*dom.Node
	popoverTransitionSources  map[*dom.Node]*dom.Node
	popoverBeforeToggleDepth  int
	testDriverPointerDown     map[string]*dom.Node
	showingPopovers           map[*dom.Node]bool
	popoverReentrantClose     map[*dom.Node]bool
	attributeValues           map[*goja.Object]*jsAttribute
	nextNID                   int
	documentPositionOrder     map[*dom.Node]uint64
	nextDocumentPositionOrder uint64
	// parserWriteBuffer gathers all document.write calls made by one
	// parser-inserted script so their combined markup is tokenized together
	// when that script returns.
	parserWriteBuffer strings.Builder
	parserWriteDepth  int
	nodeProto         *goja.Object
	// eventTargetProto/elementProto/htmlElementProto/documentProto are the
	// shared WebIDL interfaces every wrapper inherits through [[Prototype]]:
	// EventTarget > Node > Element > HTMLElement > per-tag interface. tagProtos
	// caches one prototype per HTML tag lazily; ifaceCtors caches the
	// constructors that answer `instanceof`. See element_interfaces.go.
	eventTargetProto                *goja.Object
	elementProto                    *goja.Object
	svgElementProto                 *goja.Object
	htmlElementProto                *goja.Object
	unknownElementProto             *goja.Object
	htmlCollectionProto             *goja.Object
	htmlOptionsCollectionProto      *goja.Object
	htmlFormControlsCollectionProto *goja.Object
	radioNodeListProto              *goja.Object
	documentProto                   *goja.Object
	xmlDocumentProto                *goja.Object
	documentTypeProto               *goja.Object
	documentFragmentProto           *goja.Object
	shadowRootProto                 *goja.Object
	cssStyleSheetProto              *goja.Object
	cssRuleProto                    *goja.Object
	cssStyleRuleProto               *goja.Object
	fileListProto                   *goja.Object
	characterDataProto              *goja.Object
	textProto                       *goja.Object
	commentProto                    *goja.Object
	processingInstructionProto      *goja.Object
	attrProto                       *goja.Object
	tagProtos                       map[string]*goja.Object
	ifaceCtors                      map[string]*goja.Object
	// tokenListObjects and attributesObjects preserve per-element identity of
	// the live DOMTokenList and NamedNodeMap, as browsers do.
	tokenListObjects           map[domTokenListKey]*goja.Object
	attributesObjects          map[*dom.Node]*goja.Object
	childNodeLists             map[*dom.Node]goja.Value
	attributeNodes             map[*dom.Node]map[string]*goja.Object
	ariaElementReferences      map[*dom.Node]map[string]goja.Value
	ariaElementListReferences  map[*dom.Node]map[string]goja.Value
	formElementCollections     map[*dom.Node]*goja.Object
	documentFormCollections    map[*dom.Node]*goja.Object
	fieldsetElements           map[*dom.Node]*goja.Object
	selectOptionsCollections   map[*dom.Node]*goja.Object
	selectSelectedCollections  map[*dom.Node]*goja.Object
	formSubmissionInProgress   map[*dom.Node]bool
	constructingFormEntryList  map[*dom.Node]bool
	formResetInProgress        map[*dom.Node]bool
	formValidationInProgress   map[*dom.Node]bool
	radioGroupsDirty           bool
	formControlValidities      map[*dom.Node]*goja.Object
	validityControlNodes       map[*goja.Object]*dom.Node
	validityProto              *goja.Object
	imageDataProto             *goja.Object
	imageDataObjects           map[*goja.Object]struct{}
	imageBitmapProto           *goja.Object
	imageBitmapObjects         map[*goja.Object]*imageBitmapData
	indexedDBDateGetTime       goja.Callable
	offscreenCanvasProto       *goja.Object
	offscreenCanvases          map[*goja.Object]*offscreenCanvasData
	responseProto              *goja.Object
	blobProto                  *goja.Object
	fileProto                  *goja.Object
	fileValues                 map[*goja.Object]*browserFile
	objectURLMu                sync.Mutex
	objectURLs                 map[string][]byte
	objectURLBytes             int64
	formDataValues             map[*goja.Object]*formDataList
	activeEventObjects         map[*dom.Event]goja.Value
	activeSubmitter            *dom.Node
	activationFormFrames       []*dom.Node
	fileReaderStates           map[*goja.Object]*fileReaderState
	fileListValues             map[*goja.Object][]*goja.Object
	wasmMemories               map[*goja.Object]*wasmMemoryState
	inputFiles                 map[*dom.Node][]*goja.Object
	inputFileLists             map[*dom.Node]*goja.Object
	dataTransferFiles          map[*goja.Object][]*goja.Object
	dataTransferFilesByStore   map[*dom.DataTransferStore][]*goja.Object
	dataTransferFileIDs        map[*goja.Object][]uint64
	dataTransferFileIDsByStore map[*dom.DataTransferStore][]uint64
	dataTransferFileLists      map[*goja.Object]*goja.Object
	dataTransferFileModes      map[*goja.Object]dom.DataTransferMode
	dataTransferItems          map[*goja.Object]map[string]*goja.Object
	dataTransferItemOrder      map[*dom.DataTransferStore][]dataTransferItemRef
	dataTransferItemIndexCount map[*goja.Object]int
	dataTransferItemProto      *goja.Object
	dataTransferItemListProto  *goja.Object
	dataTransferItemLists      map[*goja.Object]dataTransferItemListBinding
	dataTransferTypesCache     map[*goja.Object]dataTransferTypesCacheEntry
	nextDataTransferItemID     uint64
	// imageStates holds the load state a script-created HTMLImageElement
	// reflects (complete/naturalWidth/naturalHeight/currentSrc and the
	// decode() waiters). parsed <img> elements have no entry.
	imageStates                        map[*dom.Node]*imageLoadState
	iframeDocuments                    map[*dom.Node]*dom.Document
	iframeFormerDocuments              map[*dom.Document]*dom.Node
	iframeLocations                    map[*dom.Node]string
	iframeHistoryEntries               map[*dom.Node][]string
	iframeHistoryIndex                 map[*dom.Node]int
	iframeHistoryDocs                  map[*dom.Node]map[int]*dom.Document
	iframeOpenedContexts               map[*dom.Node]bool
	iframeContextOpeners               map[*dom.Node]goja.Value
	iframeContextNames                 map[*dom.Node]string
	iframeNavigators                   map[*dom.Node]*goja.Object
	iframeCrossOriginLocations         map[*dom.Node]*goja.Object
	iframeUserActivations              map[*dom.Node]*goja.Object
	iframeClosedContexts               map[*dom.Node]bool
	detachedDocumentURLs               map[*dom.Document]*url.URL
	iframeLoadedSources                map[*dom.Node]string
	iframeWindows                      map[*dom.Node]*goja.Object
	iframeWindowTargets                map[*dom.Node]*goja.Object
	iframeInterfaceConstructors        map[*dom.Node]map[string]*goja.Object
	iframeWindowWrites                 map[*dom.Node]map[string]goja.Value
	iframeChildObjects                 map[*goja.Object]*goja.Object
	iframeImportedChildNodes           map[*goja.Object]*dom.Node
	iframeImportedFunctionRealms       map[*goja.Object]*Runtime
	iframeSiblingFunctions             map[*goja.Object]goja.Value
	iframeImportedObjectRealms         map[*goja.Object]*Runtime
	iframeAccessorFunctions            map[*goja.Object]*Runtime
	iframeImportedFunctionAliases      map[*goja.Object]*goja.Object
	htmlConstructorNewTargetPrototypes map[*goja.Object]goja.Value
	iframeRegistries                   map[*dom.Node]*CustomElementRegistry
	iframeLoader                       func(base, src string) (*dom.Document, error)
	iframeFormSubmitter                func(base, target, contentType string, body []byte) (*dom.Document, error)
	iframeFormRequests                 map[*dom.Node]iframeFormRequest
	plannedIFrameGETURLs               map[*dom.Node]string
	iframeNavigationIDs                map[*dom.Node]uint64
	formNavigationFrames               map[*dom.Node]*dom.Node
	iframeScriptFetcher                func(base, src string) ([]byte, error)
	dynamicScriptFetcher               func(base, src string) ([]byte, error)
	pendingDynamicScripts              []*dom.Node
	dynamicScriptQueued                map[*dom.Node]bool
	dynamicScriptStarted               map[*dom.Node]bool
	dynamicScriptErrors                []string
	dynamicScriptLimit                 int
	dynamicScriptCount                 int
	iframeModuleRunner                 func(runtime *Runtime, doc *dom.Document, documentURL string)
	iframeChildRuntimes                map[*dom.Node]*Runtime
	iframeConnectedContexts            map[*dom.Node]bool
	iframeGlobalBaseline               map[*dom.Node]map[string]goja.Value
	iframeParent                       *Runtime
	iframeParentFrame                  *dom.Node
	iframeParentWindow                 goja.Value
	iframeEventContext                 goja.Value
	iframeEventListenerInvocationDepth int
	preserveWindowErrorEvent           bool
	reportingWindowError               bool
	iframeParentObjects                map[*goja.Object]goja.Value
	iframeImportedParentValues         map[*goja.Object]*goja.Object
	liveRanges                         map[*jsRange]struct{}
	iframePendingEvents                map[*dom.Node][]iframePendingListener
	iframePendingOnMsg                 map[*dom.Node]goja.Value
	pendingIFrames                     []*dom.Node
	documentWrites                     map[*dom.Document]*documentWriteState
	// onHandlers holds the onload/onerror handler registered by assignment,
	// which is wired as a real DOM listener so dispatch order matches
	// addEventListener.
	onHandlers map[*dom.Node]map[string]*jsEventHandler
	// wrapCache maps a DOM node to its single wrapper nid, so repeated
	// queries over the same nodes reuse one wrapper instead of compiling
	// accessors and closures per hit. Wrapping the same node twice used to
	// cost two RunString compiles plus dozens of sets per hit and leaked a
	// registry entry per query; it also broke `===` identity the DOM spec
	// requires. Snapshots (tagName/className/type) refresh on reuse, so a
	// cached wrapper reads exactly what a fresh one would.
	wrapCache map[*dom.Node]int
	// styleObjects preserves the per-element CSSStyleDeclaration identity.
	styleObjects         map[*dom.Node]*goja.Object
	cssStyleSheetObjects map[*dom.Node]*goja.Object
	styleSheetLists      map[*dom.Document]*goja.Object
	// selectorPlans caches query selectors compiled from the strings pages
	// pass to querySelector/querySelectorAll/matches/closest. The general
	// matcher tests one selector against every candidate node, so parsing it
	// per node would make the walk O(nodes x selector length); a runtime sees
	// the same handful of strings over and over, and the plan is a pure
	// function of the string. Bounded by selectorCacheCap (see selector.go).
	selectorPlans map[string]*selectorPlan

	// Canvas contexts keyed by DOM node ID. nil until the first getContext("2d").
	canvasContexts map[int]*canvasContext2D

	// Media bridge. mediaProvider is installed by the engine before any page
	// script runs; mediaCtrls caches one controller per element so a repeated
	// property read does not ask the engine to make another one; pendingPlays
	// holds the play() promises that are waiting for a backend to say playback
	// actually started, keyed by the operation id the controller handed out.
	mediaProvider MediaProvider
	mediaCtrls    map[*dom.Node]media.Controller
	pendingPlays  map[uint64]*promiseObj

	// Custom Elements registry. nil until setupCustomElements() is called.
	customElementRegistry                      *CustomElementRegistry
	customHTMLElementConstruction              int
	customConstructionNodes                    []*dom.Node
	dynamicMarkupInsertionCounters             map[*dom.Document]int
	customConstructionAlready                  []bool
	customConstructionSynchronous              []bool
	customConstructionCrossRealmException      []bool
	customConstructionCrossRealmErrors         []error
	customConstructionCrossRealmErrorRuntimes  []*Runtime
	customConstructionAllowAdditional          []bool
	customElementInstances                     map[*dom.Node]*customElementDefinition
	customElementDisabledStates                map[*dom.Node]bool
	customElementFormOwners                    map[*dom.Node]*dom.Node
	customElementConnected                     map[*dom.Node]bool
	customElementReactionBatch                 *customElementReactionBatch
	customElementShadowRootsDuringConstruction map[*dom.Node]goja.Value
	adoptedStyleSheets                         map[*dom.ShadowRoot][]goja.Value
	customElementCandidates                    map[*dom.Node]string
	customElementFailures                      map[*dom.Node]bool
	customElementRegistryObjects               map[*goja.Object]*CustomElementRegistry
	customElementNodeRegistries                map[*dom.Node]*CustomElementRegistry
	customElementNullRegistryLocks             map[*dom.Node]bool
	customElementInitialRegistryDocs           map[*dom.Document]struct{}
	customElementDocumentRegistries            map[*dom.Document]*CustomElementRegistry
	customElementShadowRegistries              map[*dom.ShadowRoot]*CustomElementRegistry
	customElementConstructionRegistry          *CustomElementRegistry
	execCommandTypingStyles                    map[*dom.Document]map[string]execCommandTypingStyle
	elementInternals                           map[*dom.Node]*elementInternalsState
	promiseProto                               *goja.Object

	// nodeWrappers maps DOM node IDs to their JS wrapper objects for lifecycle callbacks.
	nodeWrappers map[int]*goja.Object
	// nodeIterators tracks live NodeIterator pointers so pre-remove steps can
	// retarget references before a referenced subtree is detached.
	nodeIterators        []*nodeIterator
	nodeRemovalObservers map[*dom.Document]func()

	// shadowRoots maps host node IDs to their shadow root JS wrappers.
	shadowRoots map[int]*goja.Object
	// windowNamedProperties tracks the legacy Window named properties we have
	// installed. Their getters resolve through the live document by ID.
	windowNamedProperties map[string]bool

	// dtWrappers caches the JS view of each DataTransfer object. Drag events get
	// distinct views backed by one shared store, and each view is disassociated
	// when its event dispatch ends. The cache is bounded by
	// maxDataTransferWrappers to keep a page that constructs many stores
	// from growing it without limit.
	dtWrappers map[*dom.DataTransfer]*goja.Object
	dtProto    *goja.Object

	// clipboardProvider is the platform clipboard (or in-memory fallback).
	// hasPlatformClipboard controls secure-context/user-activation checks.
	clipboardProvider    ClipboardProvider
	hasPlatformClipboard bool
	clipboardItems       map[*goja.Object]*clipboardItemData

	// Admission budgets injected by the engine before any script runs. nil
	// leaves the corresponding operations ungated (tests, non-engine hosts).
	// mutBudget gates every script-driven DOM insertion against the document's
	// node/depth/attribute caps; subBudget is the shared byte pool fetch and XHR
	// charge their responses to. See admission.go.
	mutBudget MutationBudget
	subBudget SubresourceBudget

	// computedStyle reads the engine's cascade result for one node. Installed
	// by the engine before any script runs; nil leaves getComputedStyle to the
	// element's own declarations (see computed_style.go).
	computedStyle ComputedStyleFunc
	// pseudoStyle reads resolved pseudo-element declarations for innerText.
	// The engine installs it alongside computedStyle; nil means no pseudo styles.
	pseudoStyle PseudoStyleFunc

	// layoutGeometry reads the engine's layout arena for one node's border-box
	// geometry. Installed by the engine alongside computedStyle; nil means
	// getBoundingClientRect and offset/scroll properties answer zero.
	layoutGeometry LayoutGeometryProvider
	// offsetParent returns the nearest positioned ancestor for one node. nil
	// means offsetParent always answers null.
	offsetParent OffsetParentProvider

	// scrollMu guards the per-element and per-window scroll positions below.
	scrollMu        sync.Mutex
	scrollPositions map[scrollKey]float64
	windowScrollX   float64
	windowScrollY   float64

	// Window event target. window is not a *dom.Node, so its listeners live on
	// the runtime instead of internal/dom's node registry; the DOM Standard's
	// window step of the propagation path is appended around the node dispatch
	// (see window_events.go).
	winMu        sync.Mutex
	winListeners []*windowListener
	// ready is document.readyState: "loading" while the document's own scripts
	// run, "interactive" at DOMContentLoaded, "complete" at load.
	ready string
	// lifecycle flags make each document lifecycle event fire once. Keeping
	// DOMContentLoaded separate from load lets the embedding engine finish
	// iframe loads between the parser and window-load phases.
	domContentLoadedFired bool
	loadFired             bool
	// mutations counts DOM writes user code made, so FireDocumentLifecycle can
	// tell its caller "a ready handler changed the tree" without an
	// invalidation batch being installed.
	mutations int64
	// locRaw/locURL are the document's current URL, the state window.location's
	// accessors read (see location.go). locURL is nil when the host gave a
	// string Go's parser rejects; href then reads the raw string.
	locRaw string
	locURL *url.URL
	// locObj is the location object the window.location and document.location
	// accessors both return: one object, two names, as the standard has them.
	locObj          *goja.Object
	collectionProto *goja.Object
}

type dataTransferItemRef struct {
	kind      string
	typeValue string
	id        uint64
	file      *goja.Object
}

type dataTransferItemListBinding struct {
	dt       *dom.DataTransfer
	transfer *goja.Object
}

type dataTransferTypesCacheEntry struct {
	signature string
	value     *goja.Object
}

// maxDataTransferWrappers bounds the DataTransfer wrapper cache. Past it the
// cache resets: identity sharing within one drag operation needs at most a
// handful of entries, so a reset only costs page-constructed stores their
// object identity, never correctness.
const maxDataTransferWrappers = 256

// New builds a runtime for one document.
func New(opts Options) (*Runtime, error) {
	if opts.Timeout < 0 {
		return nil, fmt.Errorf("js: negative timeout %v rejected", opts.Timeout)
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	consoleWriter := opts.Console
	if consoleWriter == nil {
		consoleWriter = io.Discard
	}

	r := &Runtime{
		opts:                       opts,
		title:                      opts.Title,
		vm:                         goja.New(),
		timers:                     make(map[int]*timer),
		fileValues:                 make(map[*goja.Object]*browserFile),
		imageDataObjects:           make(map[*goja.Object]struct{}),
		imageBitmapObjects:         make(map[*goja.Object]*imageBitmapData),
		offscreenCanvases:          make(map[*goja.Object]*offscreenCanvasData),
		objectURLs:                 make(map[string][]byte),
		formDataValues:             make(map[*goja.Object]*formDataList),
		fileReaderStates:           make(map[*goja.Object]*fileReaderState),
		fileListValues:             make(map[*goja.Object][]*goja.Object),
		inputFiles:                 make(map[*dom.Node][]*goja.Object),
		inputFileLists:             make(map[*dom.Node]*goja.Object),
		dataTransferFiles:          make(map[*goja.Object][]*goja.Object),
		dataTransferFilesByStore:   make(map[*dom.DataTransferStore][]*goja.Object),
		dataTransferFileIDs:        make(map[*goja.Object][]uint64),
		dataTransferFileIDsByStore: make(map[*dom.DataTransferStore][]uint64),
		dataTransferFileLists:      make(map[*goja.Object]*goja.Object),
		dataTransferFileModes:      make(map[*goja.Object]dom.DataTransferMode),
		dataTransferItems:          make(map[*goja.Object]map[string]*goja.Object),
		dataTransferItemOrder:      make(map[*dom.DataTransferStore][]dataTransferItemRef),
		dataTransferItemIndexCount: make(map[*goja.Object]int),
		dataTransferItemLists:      make(map[*goja.Object]dataTransferItemListBinding),
		dataTransferTypesCache:     make(map[*goja.Object]dataTransferTypesCacheEntry),
	}

	r.setupConsole(consoleWriter)
	r.setupDocument()
	r.setupWindow()
	r.setupDOMException()
	r.setupDOM()
	r.installInputEventConstructors()
	r.queueInitialDetailsToggleEvents()
	r.refreshDocumentPatternMismatches()
	r.setupDocumentWrite()
	if opts.HTTPClient != nil {
		r.setupFetch(opts.HTTPClient)
	}
	// XHR installs regardless: without a transport the constructor still
	// exists and send() fails through the error event, so capability
	// probes never see a missing constructor.
	r.setupXHR(opts.HTTPClient)
	r.setupTimers()
	r.setupPromise()
	r.setupCookies()
	r.setupFonts()
	r.setupIndexedDB()
	r.setupIntersectionObserver()
	r.setupRAF()
	r.setupAnimations()
	r.setupStorage()
	r.setupURLAPI()
	r.setupTextEncoding()
	r.setupFloat16Array()
	r.setupFileAPI()
	r.setupResponse()
	r.setupFormData()
	r.setupAbortController()
	r.setupCrypto()
	r.setupPerformance()
	r.setupHistory()
	if opts.WebSocketDialer != nil {
		r.setupWebSocket(opts.WebSocketDialer)
	}
	r.setupMutationObserver()
	r.setupResizeObserver()
	r.setupCanvas()
	r.setupImageBitmap()
	r.setupOffscreenCanvas()
	r.setupCustomElements()
	r.setupWorker(opts.WorkerDialer)
	r.setupWebAssemblyMemory()
	r.setupServiceWorker()
	r.setupClipboard()
	r.setupBroadcastChannel()
	r.setupMessageChannel()
	r.setupCSS()
	r.setupDragDrop()
	r.setupMicrodata()
	r.setupRange()
	r.setupSelection()
	r.setupDOMParser()
	r.setupXMLSerializer()
	r.setupUnsupportedAPIs()
	_ = timeout // enforced per-Run via vm.SetMaxCallStackSize or interrupt timer

	return r, nil
}

func (r *Runtime) setupConsole(w io.Writer) {
	console := r.vm.NewObject()

	logFn := func(args ...goja.Value) {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = formatConsoleArg(arg)
		}
		fmt.Fprintln(w, strings.Join(parts, " "))
	}

	_ = console.Set("log", logFn)
	_ = console.Set("warn", logFn)
	_ = console.Set("error", logFn)
	_ = r.vm.Set("console", console)
}

func formatConsoleArg(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if goja.IsNull(v) {
		return "null"
	}
	return v.String()
}

func (r *Runtime) setupDocument() {
	doc := r.vm.NewObject()
	_ = r.vm.Set("document", doc)

	_ = doc.Set("__title__", r.title)
	// document.nodeType === 9 is the first thing a jQuery/Sizzle boot reads:
	// Sizzle's setDocument gate is `9 === doc.nodeType && doc.documentElement`,
	// so an undefined nodeType silently un-configures the whole library and
	// every `$(document).ready(...)` reveal with it (GOOSIE-LIVE-001, measured
	// on gov.vn where the entire body sits in a display:none form that only
	// that reveal opens). This is the DOM Standard ordinal, not Goosie's
	// internal one - internal/dom numbers its kinds from 0, so the binding
	// must translate and can never pass the field through. It is the first
	// gate, not the whole chain: measured after this change gov.vn still paints
	// blank, because its jQuery bundle now dies one call later on
	// `createDocumentFragment`, another Node API the binding does not have.
	_ = doc.Set("nodeType", NodeTypeDocument)
	_ = doc.Set("nodeName", "#document")

	_ = doc.DefineAccessorProperty("title", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if titleNode := firstTitle(r.domDoc); titleNode != nil {
			return r.vm.ToValue(titleNode.TextContent())
		}
		return r.vm.ToValue(r.title)
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Undefined()
		}
		title := call.Arguments[0].String()
		r.title = title
		_ = doc.Set("__title__", title)
		if titleNode := firstTitle(r.domDoc); titleNode != nil {
			setNodeText(titleNode, title)
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
}

func (r *Runtime) setupWindow() {
	// The window object IS the global scope (HTML spec: Window is the
	// global object). An earlier stub installed window as a separate
	// object, so `window.google = ns` followed by bare `google` died with
	// "google is not defined" - the exact shape every dynamic page's boot
	// script takes. Aliasing keeps one identity: assignments in either
	// direction are visible in the other, and the later dual-installs
	// (global + window) below degenerate to the same store.
	win := r.vm.GlobalObject()
	_ = win.Set("event", goja.Undefined())
	// window.location, with every part pages read, is installed below by
	// setupLocationParts (see location.go) together with its document.location
	// alias; this used to be an href-only stub.
	_ = win.Set("window", win)
	_ = win.Set("self", win)
	_ = win.Set("globalThis", win)
	nav := r.vm.NewObject()
	_ = nav.Set("userAgent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Goosie/2.0")
	_ = nav.Set("platform", "MacIntel")
	_ = nav.Set("language", "en-US")
	_ = nav.Set("languages", r.vm.ToValue([]string{"en-US", "en"}))
	_ = nav.Set("cookieEnabled", true)
	_ = nav.Set("onLine", true)
	activation := r.vm.NewObject()
	_ = activation.DefineAccessorProperty("isActive", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.HasTransientUserActivation())
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = activation.DefineAccessorProperty("hasBeenActive", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(r.hasBeenActive)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = nav.Set("userActivation", activation)
	_ = win.Set("navigator", nav)

	_ = win.Set("matchMedia", func(call goja.FunctionCall) goja.Value {
		q := ""
		if len(call.Arguments) > 0 {
			q = call.Arguments[0].String()
		}
		mql := r.vm.NewObject()
		_ = mql.Set("media", q)
		matches := true
		lq := strings.ToLower(q)
		if strings.Contains(lq, "print") && !strings.Contains(lq, "not") {
			matches = false
		}
		if strings.Contains(lq, "prefers-color-scheme") {
			dark := strings.Contains(lq, "dark")
			light := strings.Contains(lq, "light")
			if r.opts.DarkMode {
				if dark {
					matches = !strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
				if light {
					matches = strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
			} else {
				if dark {
					matches = strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
				if light {
					matches = !strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
			}
		}
		if strings.Contains(lq, "prefers-contrast") {
			more := strings.Contains(lq, "more")
			noPref := strings.Contains(lq, "no-preference")
			if r.opts.HighContrast {
				if more {
					matches = !strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
				if noPref {
					matches = strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
			} else {
				if more {
					matches = strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
				if noPref {
					matches = !strings.HasPrefix(strings.TrimSpace(lq), "not ")
				}
			}
		}
		_ = mql.Set("matches", matches)
		_ = mql.Set("addListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = mql.Set("removeListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = mql.Set("addEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		_ = mql.Set("removeEventListener", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
		return mql
	})
	// window is an EventTarget and the document lifecycle is a real event, not
	// a page's imagination: four `window.addEventListener('DOMContentLoaded')`
	// registrations on the saved vnexpress boot each threw "Object has no
	// member 'addEventListener'", and jQuery's ready path needs both the window
	// listener and a readyState that still reads "loading" while its own script
	// runs. See window_events.go.
	r.setupWindowEvents()
	if window, ok := r.vm.Get("window").(*goja.Object); ok {
		_ = window.DefineAccessorProperty("event", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			if r.legacyWindowEvent == nil {
				return goja.Undefined()
			}
			return r.legacyWindowEvent
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			r.legacyWindowEvent = goja.Undefined()
			if len(call.Arguments) != 0 {
				r.legacyWindowEvent = call.Arguments[0]
			}
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	r.setupLocationParts(win)
	// The window handles a library reaches for on the way to a computed style:
	// document.defaultView (the window itself, so it carries getComputedStyle
	// and addEventListener), window.top/parent (=== window in a top-level
	// browsing context, which is what makes Sizzle's `n.top !== n` test end the
	// way it does in a browser instead of taking the frame branch), and
	// window.opener (null, so jQuery's `t && t.opener || (t = C)` falls back to
	// the window it already has).
	r.jsDefaultView()
	_ = win.Set("top", win)
	_ = win.Set("parent", win)
	_ = win.Set("opener", goja.Null())
	r.setupWindowOpen(win)
	_ = win.Set("getComputedStyle", func(call goja.FunctionCall) goja.Value {
		return r.jsGetComputedStyle(call)
	})
}

// SetTransientUserActivation updates the document's user-activation state for
// a host-identified trusted input event. The runtime retains sticky activation
// after transient activation is consumed or cleared.
func (r *Runtime) SetTransientUserActivation(active bool) {
	if r == nil {
		return
	}
	if r.iframeParent != nil {
		if r.iframeParent.iframeChildRuntimes == nil {
			r.iframeParent.iframeChildRuntimes = make(map[*dom.Node]*Runtime)
		}
		r.iframeParent.iframeChildRuntimes[r.iframeParentFrame] = r
	}
	if active {
		expires := time.Now().Add(5 * time.Second)
		mark := func(target *Runtime) {
			if target == nil {
				return
			}
			target.transientUserActivation = true
			target.hasBeenActive = true
			target.transientActivationUntil = expires
		}
		// Activation reaches every ancestor browsing context and same-origin
		// descendants of the document that received the trusted input.
		for ancestor := r.iframeParent; ancestor != nil; ancestor = ancestor.iframeParent {
			mark(ancestor)
		}
		mark(r)
		origin := runtimeActivationOrigin(r)
		var visit func(*Runtime)
		visit = func(parent *Runtime) {
			for _, child := range parent.iframeChildRuntimes {
				if child == nil {
					continue
				}
				if origin != "" && runtimeActivationOrigin(child) == origin {
					mark(child)
				}
				visit(child)
			}
		}
		visit(r)
		return
	}
	// Activation consumption affects every active window in the top-level
	// traversable, including cross-origin descendants.
	root := r
	for root.iframeParent != nil {
		root = root.iframeParent
	}
	var clear func(*Runtime)
	clear = func(target *Runtime) {
		if target == nil {
			return
		}
		target.transientUserActivation = false
		target.transientActivationUntil = time.Time{}
		for _, child := range target.iframeChildRuntimes {
			clear(child)
		}
	}
	clear(root)
}

func runtimeActivationOrigin(r *Runtime) string {
	if r == nil {
		return ""
	}
	u := r.documentURL()
	if u == nil || (u.Scheme == "about" && r.iframeParent != nil) {
		return runtimeActivationOrigin(r.iframeParent)
	}
	if u.Hostname() == "" {
		return ""
	}
	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

// HasTransientUserActivation reports whether the recent trusted input still
// grants transient activation. The bounded host window expires after five
// seconds unless an API consumes it or the host clears it explicitly.
func (r *Runtime) HasTransientUserActivation() bool {
	if r == nil || !r.transientUserActivation {
		return false
	}
	if !r.transientActivationUntil.IsZero() && time.Now().After(r.transientActivationUntil) {
		r.transientUserActivation = false
		return false
	}
	return true
}

func (r *Runtime) sameOriginWithTopLevel() bool {
	if r == nil || r.iframeParent == nil {
		return true
	}
	root := r
	for root.iframeParent != nil {
		root = root.iframeParent
	}
	currentOrigin := runtimeOrigin(r.locURL)
	if currentOrigin == "" && r.locURL != nil && strings.EqualFold(r.locURL.String(), "about:blank") {
		currentOrigin = runtimeOrigin(r.iframeParent.locURL)
	}
	topOrigin := runtimeOrigin(root.locURL)
	return currentOrigin != "" && topOrigin != "" && currentOrigin == topOrigin
}

func runtimeOrigin(u *url.URL) string {
	if u == nil || u.Scheme == "" || u.Hostname() == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	return scheme + "://" + host + ":" + port
}

// EnableTestUserActivationHook lets the in-process WPT adapter turn its
// testdriver-generated click into a transient activation. Production pages do
// not receive this hook; the native host marks trusted input through Go.
func (r *Runtime) EnableTestUserActivationHook() {
	if r == nil {
		return
	}
	r.testUserActivationHook = true
	_ = r.vm.Set("__goosieGrantUserActivation", func() { r.SetTransientUserActivation(true) })
	_ = r.vm.Set("__goosieTestDriverClick", func(call goja.FunctionCall) goja.Value {
		r.testDriverClick(call.Argument(0))
		return goja.Undefined()
	})
	_ = r.vm.Set("__goosieEscapeKey", func() bool {
		if r.opts.OnEscapeKey == nil {
			return false
		}
		return r.opts.OnEscapeKey()
	})
	_ = r.vm.Set("__goosieTestDriverKeyDefault", func(call goja.FunctionCall) goja.Value {
		if r.opts.OnTestDriverKeyDefault != nil {
			r.opts.OnTestDriverKeyDefault(call.Argument(0).String(), call.Argument(1).ToBoolean(), call.Argument(2).ToBoolean(), call.Argument(3).ToBoolean(), call.Argument(4).ToBoolean())
		}
		return goja.Undefined()
	})
	_ = r.vm.Set("__goosiePointerDown", func(call goja.FunctionCall) goja.Value {
		target := r.getNode(call.Argument(0))
		if target == nil {
			return r.vm.ToValue(false)
		}
		if r.testDriverPointerDown == nil {
			r.testDriverPointerDown = make(map[string]*dom.Node)
		}
		r.testDriverPointerDown[call.Argument(1).String()] = target
		return r.vm.ToValue(true)
	})
	_ = r.vm.Set("__goosieTestDriverPointer", func(call goja.FunctionCall) goja.Value {
		if r.opts.OnTestDriverPointer == nil || len(call.Arguments) < 4 {
			return r.vm.ToValue(false)
		}
		return r.vm.ToValue(r.opts.OnTestDriverPointer(
			call.Argument(0).String(),
			float32(call.Argument(1).ToFloat()),
			float32(call.Argument(2).ToFloat()),
			int(call.Argument(3).ToInteger()),
		))
	})
	_ = r.vm.Set("__goosiePointerUp", func(call goja.FunctionCall) goja.Value {
		target := r.getNode(call.Argument(0))
		if target == nil {
			return r.vm.ToValue(false)
		}
		button := 0
		if len(call.Arguments) > 2 {
			button = int(call.Argument(2).ToInteger())
		}
		return r.vm.ToValue(r.handleTestDriverPointerUp(target, call.Argument(1).String(), button))
	})
	for _, child := range r.iframeChildRuntimes {
		child.EnableTestUserActivationHook()
	}
}

// ChainOnEscapeKey appends a host-owned Escape default action.
func (r *Runtime) ChainOnEscapeKey(fn func() bool) {
	if r == nil || fn == nil {
		return
	}
	if previous := r.opts.OnEscapeKey; previous != nil {
		r.opts.OnEscapeKey = func() bool { return fn() || previous() }
	} else {
		r.opts.OnEscapeKey = fn
	}
}

// ChainOnTestDriverPointer installs the headless WPT pointer-action default.
func (r *Runtime) ChainOnTestDriverPointer(fn func(action string, x, y float32, button int) bool) {
	if r == nil || fn == nil {
		return
	}
	previous := r.opts.OnTestDriverPointer
	if previous == nil {
		r.opts.OnTestDriverPointer = fn
		return
	}
	r.opts.OnTestDriverPointer = func(action string, x, y float32, button int) bool {
		return fn(action, x, y, button) || previous(action, x, y, button)
	}
}

func (r *Runtime) setupDOMException() {
	_, _ = r.vm.RunString(`(function(global) {
		var codes = {
			IndexSizeError: 1, HierarchyRequestError: 3, WrongDocumentError: 4,
			InvalidCharacterError: 5, NoModificationAllowedError: 7, NotFoundError: 8,
			NotSupportedError: 9, InUseAttributeError: 10, InvalidStateError: 11,
			SyntaxError: 12, InvalidModificationError: 13, NamespaceError: 14,
			InvalidAccessError: 15, TypeMismatchError: 17, SecurityError: 18,
			NetworkError: 19, AbortError: 20, URLMismatchError: 21,
			TimeoutError: 23, InvalidNodeTypeError: 24, DataCloneError: 25
		};
		var constants = {
			INDEX_SIZE_ERR: 1, HIERARCHY_REQUEST_ERR: 3, WRONG_DOCUMENT_ERR: 4,
			INVALID_CHARACTER_ERR: 5, NO_MODIFICATION_ALLOWED_ERR: 7, NOT_FOUND_ERR: 8,
			NOT_SUPPORTED_ERR: 9, INUSE_ATTRIBUTE_ERR: 10, INVALID_STATE_ERR: 11,
			SYNTAX_ERR: 12, INVALID_MODIFICATION_ERR: 13, NAMESPACE_ERR: 14,
			INVALID_ACCESS_ERR: 15, TYPE_MISMATCH_ERR: 17, SECURITY_ERR: 18,
			NETWORK_ERR: 19, ABORT_ERR: 20, URL_MISMATCH_ERR: 21,
			QUOTA_EXCEEDED_ERR: 22, TIMEOUT_ERR: 23, INVALID_NODE_TYPE_ERR: 24,
			DATA_CLONE_ERR: 25
		};
		function DOMException(message, name) {
			message = message === undefined ? '' : String(message);
			name = name === undefined ? 'Error' : String(name);
			var exception = new Error(message);
			Object.setPrototypeOf(exception, DOMException.prototype);
			Object.defineProperties(exception, {
				name: {value: name, writable: true, configurable: true},
				message: {value: message, writable: true, configurable: true},
				code: {value: codes[name] || 0, enumerable: true}
			});
			return exception;
		}
		DOMException.prototype = Object.create(Error.prototype);
		Object.defineProperty(DOMException.prototype, 'constructor', {value: DOMException, writable: true, configurable: true});
		for (var key in constants) {
			Object.defineProperty(DOMException, key, {value: constants[key], enumerable: true});
			Object.defineProperty(DOMException.prototype, key, {value: constants[key], enumerable: true});
		}
		Object.defineProperty(global, 'DOMException', {value: DOMException, writable: true, configurable: true, enumerable: true});
	})(window);`)
}

func (r *Runtime) setupUnsupportedAPIs() {
	// Wave 2: fetch is now wired by setupFetch when an HTTPClient is
	// provided. When it is nil, fetch remains undefined and calling it
	// produces a ReferenceError that names the missing API.
}

// setupDOM installs DOM API bindings on the document object when a DOM tree
// was provided via Options.DOM. Without a DOM, none of the query or mutation
// methods exist, so scripts that call them get a clear undefined-function
// error rather than a silent no-op.
func (r *Runtime) setupDOM() {
	r.observeNodeRemoval(r.opts.DOM)
	if r.opts.DOM == nil {
		return
	}

	r.domDoc = r.opts.DOM
	if document, ok := r.vm.Get("document").(*goja.Object); ok {
		_ = document.DefineAccessorProperty("scrollingElement", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			if r.domDoc == nil {
				return goja.Null()
			}
			root := documentElementOf(r.domDoc)
			if r.domDoc.QuirksMode {
				root = documentBodyOf(r.domDoc)
			}
			return r.wrapNode(root)
		}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	}
	r.nodeRegistry = make(map[int]*dom.Node)
	r.nodeWrappers = make(map[int]*goja.Object)
	r.shadowRoots = make(map[int]*goja.Object)
	r.windowNamedProperties = make(map[string]bool)
	r.wrapCache = make(map[*dom.Node]int)
	r.styleObjects = make(map[*dom.Node]*goja.Object)
	r.tokenListObjects = make(map[domTokenListKey]*goja.Object)
	r.attributesObjects = make(map[*dom.Node]*goja.Object)
	r.childNodeLists = make(map[*dom.Node]goja.Value)
	r.attributeNodes = make(map[*dom.Node]map[string]*goja.Object)
	r.ariaElementReferences = make(map[*dom.Node]map[string]goja.Value)
	r.ariaElementListReferences = make(map[*dom.Node]map[string]goja.Value)
	r.imageStates = make(map[*dom.Node]*imageLoadState)
	r.iframeDocuments = make(map[*dom.Node]*dom.Document)
	r.iframeFormerDocuments = make(map[*dom.Document]*dom.Node)
	r.iframeWindows = make(map[*dom.Node]*goja.Object)
	r.iframeWindowTargets = make(map[*dom.Node]*goja.Object)
	r.iframeWindowWrites = make(map[*dom.Node]map[string]goja.Value)
	r.iframeChildObjects = make(map[*goja.Object]*goja.Object)
	r.iframeChildRuntimes = make(map[*dom.Node]*Runtime)
	r.iframeGlobalBaseline = make(map[*dom.Node]map[string]goja.Value)
	r.onHandlers = make(map[*dom.Node]map[string]*jsEventHandler)
	r.nextNID = 1

	// Build the shared WebIDL interface chain once: EventTarget > Node >
	// Element > HTMLElement > per-tag interface. Each wrapper stores a __nid__
	// (registry key) that inherited accessors use to look up the real
	// *dom.Node; nothing per-node is installed beyond that key and nodeType.
	r.setupProtoChain()
	for tag := range htmlTagInterfaces {
		r.protoForTag(tag)
	}
	r.installLegacyOptionConstructor()
	r.protoForTag("input")
	r.installFileInputAccessors()
	r.installGlobalEventHandlerAttributes()
	r.setupShadowDOM()
	if r.opts.OnUserInput != nil {
		_ = r.vm.Set("__goosieNotifyUserInput", func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.Argument(0))
			if node != nil && (node.Data == "input" || node.Data == "textarea") {
				node.ValueChangedByUser = true
				return r.vm.ToValue(r.opts.OnUserInput(node))
			}
			return r.vm.ToValue(false)
		})
	}
	if r.opts.OnNumberInputEdit != nil {
		_ = r.vm.Set("__goosieSetNumberBadInput", func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.Argument(0))
			if node != nil && node.Data == "input" && strings.EqualFold(node.GetAttribute("type"), "number") {
				bad := call.Argument(1).ToBoolean()
				node.InputBadInput = bad
				r.opts.OnNumberInputEdit(node, bad)
			}
			return goja.Undefined()
		})
	}
	if r.opts.OnEmailInputEdit != nil {
		_ = r.vm.Set("__goosieSetEmailEditValue", func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.Argument(0))
			if node != nil && node.Data == "input" && strings.EqualFold(node.GetAttribute("type"), "email") {
				value := call.Argument(1).String()
				node.InputUserEditValue = value
				node.InputUserEditValueActive = true
				r.opts.OnEmailInputEdit(node, value)
			}
			return goja.Undefined()
		})
	}
	if r.opts.OnActiveState != nil {
		_ = r.vm.Set("__goosieSetActiveState", func(call goja.FunctionCall) goja.Value {
			node := r.getNode(call.Argument(0))
			active := call.Argument(1).ToBoolean()
			if node != nil && node.Element() {
				r.opts.OnActiveState(node, active)
			}
			return goja.Undefined()
		})
	}
	if r.opts.OnTabNavigation != nil {
		_ = r.vm.Set("__goosieTabNavigation", func(call goja.FunctionCall) goja.Value {
			r.opts.OnTabNavigation(call.Argument(0).ToBoolean())
			return goja.Undefined()
		})
	}
	if r.opts.OnTabNavigationFrom != nil {
		_ = r.vm.Set("__goosieTabNavigationFrom", func(call goja.FunctionCall) goja.Value {
			r.opts.OnTabNavigationFrom(r.getNode(call.Argument(0)), call.Argument(1).ToBoolean())
			return goja.Undefined()
		})
	}
	_ = r.nodeProto.Set("append", func(call goja.FunctionCall) goja.Value {
		return r.jsAppend(call)
	})
	_ = r.nodeProto.Set("prepend", func(call goja.FunctionCall) goja.Value {
		return r.jsPrepend(call)
	})
	_ = r.nodeProto.Set("appendChild", func(call goja.FunctionCall) goja.Value {
		return r.jsAppendChild(call)
	})
	_ = r.nodeProto.Set("removeChild", func(call goja.FunctionCall) goja.Value {
		return r.jsRemoveChild(call)
	})
	_ = r.nodeProto.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		return r.jsAddEventListener(call)
	})
	_ = r.nodeProto.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		return r.jsRemoveEventListener(call)
	})
	_ = r.nodeProto.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		return r.jsDispatchEvent(call)
	})
	_ = r.htmlElementProto.Set("focus", func(call goja.FunctionCall) goja.Value {
		if node := r.getNode(call.This); node != nil && r.opts.OnFocusChange != nil {
			r.opts.OnFocusChange(node, true)
		}
		return goja.Undefined()
	})
	_ = r.htmlElementProto.Set("blur", func(call goja.FunctionCall) goja.Value {
		if node := r.getNode(call.This); node != nil && r.opts.OnFocusChange != nil {
			r.opts.OnFocusChange(node, false)
		}
		return goja.Undefined()
	})
	_ = r.htmlElementProto.Set("click", func(call goja.FunctionCall) goja.Value {
		return r.clickHTMLElement(call)
	})
	_ = r.elementProto.Set("getAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsGetAttribute(call)
	})
	_ = r.elementProto.Set("getAttributeNS", func(call goja.FunctionCall) goja.Value {
		return r.jsGetAttributeNS(call)
	})
	_ = r.elementProto.Set("getAttributeNode", func(call goja.FunctionCall) goja.Value {
		return r.jsGetAttributeNode(call)
	})
	_ = r.elementProto.Set("getAttributeNodeNS", func(call goja.FunctionCall) goja.Value {
		return r.jsGetAttributeNodeNS(call)
	})
	_ = r.elementProto.Set("setAttributeNode", func(call goja.FunctionCall) goja.Value {
		return r.jsSetAttributeNode(call)
	})
	_ = r.elementProto.Set("setAttributeNodeNS", func(call goja.FunctionCall) goja.Value {
		return r.jsSetAttributeNode(call)
	})
	_ = r.elementProto.Set("removeAttributeNode", func(call goja.FunctionCall) goja.Value {
		return r.jsRemoveAttributeNode(call)
	})
	_ = r.elementProto.Set("setAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsSetAttribute(call)
	})
	_ = r.elementProto.Set("setAttributeNS", func(call goja.FunctionCall) goja.Value {
		return r.jsSetAttributeNS(call)
	})
	_ = r.elementProto.Set("removeAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsRemoveAttribute(call)
	})
	_ = r.elementProto.Set("removeAttributeNS", func(call goja.FunctionCall) goja.Value {
		return r.jsRemoveAttributeNS(call)
	})
	_ = r.elementProto.Set("toggleAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsToggleAttribute(call)
	})
	_ = r.elementProto.Set("hasAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsHasAttribute(call)
	})
	_ = r.elementProto.Set("remove", func(call goja.FunctionCall) goja.Value {
		return r.jsRemove(call)
	})
	_ = r.elementProto.Set("getContext", func(call goja.FunctionCall) goja.Value {
		return r.jsGetContext(call)
	})
	// ParentNode query roots: jsQuerySelector(All) already resolve the root
	// from call.This, so the same handlers serve document and elements with
	// document-order, subtree-scoped results.
	_ = r.elementProto.Set("querySelector", func(call goja.FunctionCall) goja.Value {
		return r.jsQuerySelector(call)
	})
	_ = r.elementProto.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value {
		return r.jsQuerySelectorAll(call)
	})
	// DocumentFragment participates in the ParentNode mixin too. Template
	// contents and document.createDocumentFragment() receive this prototype;
	// text and comment nodes remain plain Node instances without query methods.
	_ = r.documentFragmentProto.Set("querySelector", func(call goja.FunctionCall) goja.Value {
		return r.jsQuerySelector(call)
	})
	_ = r.documentFragmentProto.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value {
		return r.jsQuerySelectorAll(call)
	})
	_ = r.documentFragmentProto.Set("getElementById", func(call goja.FunctionCall) goja.Value {
		root := r.getNode(call.This)
		if root == nil || !root.DocumentFragment() || len(call.Arguments) == 0 {
			return goja.Null()
		}
		wanted := call.Arguments[0].String()
		var found *dom.Node
		for child := root.FirstChild; child != nil && found == nil; child = child.NextSibling {
			child.Walk(func(node *dom.Node) bool {
				if node.Element() && node.GetAttribute("id") == wanted {
					found = node
					return false
				}
				return true
			})
		}
		return r.wrapNode(found)
	})
	// Element.matches/closest are the other two selector entry points. They
	// sit on the same prototype as the query roots because all of them share
	// one compiled-selector path in selector.go: same :scope binding, same
	// argument coercion, same SyntaxError for an invalid selector.
	_ = r.elementProto.Set("matches", func(call goja.FunctionCall) goja.Value {
		return r.jsMatches(call)
	})
	_ = r.elementProto.Set("closest", func(call goja.FunctionCall) goja.Value {
		return r.jsClosest(call)
	})
	_ = r.elementProto.Set("insertAdjacentHTML", func(call goja.FunctionCall) goja.Value {
		return r.jsInsertAdjacentHTML(call)
	})
	_ = r.elementProto.Set("insertAdjacentElement", func(call goja.FunctionCall) goja.Value {
		return r.jsInsertAdjacentElement(call)
	})
	_ = r.elementProto.Set("setHTMLUnsafe", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(r.vm.NewTypeError("Failed to execute 'setHTMLUnsafe' on 'Element': 1 argument required"))
		}
		node := r.getNode(call.This)
		if node == nil || !node.Element() {
			panic(r.vm.NewTypeError("Failed to execute 'setHTMLUnsafe' on 'Element': this is not an Element"))
		}
		documentSetInnerHTML(r, thisNID(call), call.Arguments[0].String(), true)
		return goja.Undefined()
	})
	_ = r.elementProto.Set("prepend", func(call goja.FunctionCall) goja.Value {
		return r.jsPrepend(call)
	})
	_ = r.elementProto.Set("append", func(call goja.FunctionCall) goja.Value {
		return r.jsAppend(call)
	})
	// The child-list mutation and traversal API (insertBefore, replaceChild,
	// cloneNode, contains, hasChildNodes) lives in node_api.go and goes on the
	// same prototype, so a document fragment created by
	// document.createDocumentFragment() answers them like any other node.
	r.installNodeTreeMethods()
	// textContent (Node) and innerHTML (Element) are inherited accessors, not
	// per-node copies. See element_interfaces.go.
	r.installNodeAccessors()
	r.installElementAccessors()
	r.installParentNodeAccessors()
	r.installCSSOMView()

	doc := r.vm.Get("document").(*goja.Object)
	// The document is a Node and a Document, so it inherits the tree API from
	// Node.prototype and answers `document instanceof Node`/`Document`. Its own
	// methods below shadow the inherited ones where the behaviour differs.
	_ = doc.SetPrototype(r.documentProto)

	_ = doc.Set("querySelector", func(call goja.FunctionCall) goja.Value {
		return r.jsQuerySelector(call)
	})
	_ = doc.Set("querySelectorAll", func(call goja.FunctionCall) goja.Value {
		return r.jsQuerySelectorAll(call)
	})
	_ = doc.Set("elementsFromPoint", func(call goja.FunctionCall) goja.Value {
		return r.jsElementsFromPoint(call)
	})
	_ = doc.Set("elementFromPoint", func(call goja.FunctionCall) goja.Value {
		return r.jsElementFromPoint(call)
	})
	_ = doc.Set("createElement", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateElement(call)
	})
	_ = doc.Set("createElementNS", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateElementNS(call)
	})
	_ = doc.Set("createAttribute", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateAttribute(call, r.domDoc)
	})
	_ = doc.Set("createAttributeNS", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateAttributeNS(call, r.domDoc)
	})
	_ = doc.Set("getElementById", func(call goja.FunctionCall) goja.Value {
		return r.jsGetElementByID(call)
	})
	_ = doc.Set("getElementsByClassName", func(call goja.FunctionCall) goja.Value {
		return r.jsGetElementsByClassName(call)
	})
	_ = doc.Set("getElementsByTagName", func(call goja.FunctionCall) goja.Value {
		return r.jsGetElementsByTagName(call)
	})
	_ = doc.Set("getElementsByTagNameNS", func(call goja.FunctionCall) goja.Value {
		if r.domDoc == nil || len(call.Arguments) < 2 {
			return r.newLiveHTMLCollectionNS(nil, "", "")
		}
		return r.newLiveHTMLCollectionNS(&r.domDoc.Node, call.Arguments[0].String(), call.Arguments[1].String())
	})
	_ = doc.Set("createTextNode", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateTextNode(call)
	})
	_ = doc.Set("createComment", func(call goja.FunctionCall) goja.Value {
		return r.jsCreateComment(call)
	})
	r.installCreateCDATASection(doc, r.domDoc)
	r.installCreateProcessingInstruction(doc, r.domDoc)
	_ = doc.Set("createDocumentFragment", r.jsCreateDocumentFragment)
	_ = doc.Set("importNode", func(call goja.FunctionCall) goja.Value {
		return r.jsImportNode(call, r.domDoc)
	})
	_ = doc.Set("adoptNode", func(call goja.FunctionCall) goja.Value {
		return r.jsAdoptNode(call, r.domDoc)
	})
	// The document is a Node, so it carries the Node API too. jQuery never
	// appends to a document directly, but Sizzle's support tests call
	// `document.contains(node)` and `document.cloneNode`, and a missing method
	// there reads to the library as "this document is not a DOM document".
	for _, name := range []string{"appendChild", "insertBefore", "replaceChild", "cloneNode", "contains", "hasChildNodes"} {
		if v := r.nodeProto.Get(name); v != nil {
			_ = doc.Set(name, v)
		}
	}

	// Register the document node so document.addEventListener / dispatchEvent
	// operate on the same *dom.Node the engine dispatches events on.
	docNID := r.nextNID
	r.nextNID++
	r.nodeRegistry[docNID] = &r.domDoc.Node
	_ = doc.Set("__nid__", docNID)
	r.nodeWrappers[docNID] = doc
	// The document node is registered here rather than through wrapNode, so the
	// wrap cache has to be pointed at it by hand. Without this line
	// wrapNode(&doc.Node) mints a second wrapper for the same node, and then
	// `el.ownerDocument` is an object that is *not* `document`: Sizzle's
	// setDocument compares `r != C` against window.document, would adopt the
	// look-alike as its document cache, and die on the first property only the
	// real one carries (defaultView, readyState, getElementById). Identity, not
	// shape, is what the library checks.
	r.wrapCache[&r.domDoc.Node] = docNID
	// The document is a Node, so the child-list reads work on it too:
	// `document.childNodes`, `document.firstChild` (the doctype on a real page)
	// and `document.ownerDocument`, which is null per the standard. Those are
	// inherited from Node.prototype through the Document prototype set above.
	// document.implementation.createHTMLDocument: jQuery's load-time support
	// probe calls it, so without it the library never finishes installing itself.
	r.setupDocumentImplementation()

	_ = doc.Set("addEventListener", func(call goja.FunctionCall) goja.Value {
		return r.jsAddEventListener(call)
	})
	_ = doc.Set("removeEventListener", func(call goja.FunctionCall) goja.Value {
		return r.jsRemoveEventListener(call)
	})
	_ = doc.Set("dispatchEvent", func(call goja.FunctionCall) goja.Value {
		return r.jsDispatchEvent(call)
	})

	// Internal helpers the prototype getter/setter calls back into.
	_ = doc.Set("__getTextContent__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("")
		}
		if isRangeCharacterData(node) {
			return goja.StringFromUTF16(characterDataCodeUnits(node))
		}
		return r.vm.ToValue(node.TextContent())
	})
	_ = doc.Set("__setTextContent__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		textValue := call.Arguments[1]
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		if isRangeCharacterData(node) {
			r.replaceCharacterDataUnits(node, 0, len(characterDataCodeUnits(node)), characterDataValueUnits(textValue))
			return goja.Undefined()
		}
		r.setNodeTextContent(node, textValue.String())
		return goja.Undefined()
	})
	_ = doc.Set("__getInnerHTML__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("")
		}
		if node.IsHTMLTemplate() {
			node = node.TemplateContent()
		}
		return r.vm.ToValue(r.serializeChildren(node))
	})
	_ = doc.Set("__setInnerHTML__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		html := call.Arguments[1].String()
		forceHTMLFragment := len(call.Arguments) > 2 && call.Arguments[2].ToBoolean()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		// Parse the fragment FIRST, before touching the live tree, so a parse
		// failure or an admission rejection leaves the existing children intact
		// instead of cleared. The per-call ParseLimits bound one call; the
		// mutation budget reserved below bounds the document in aggregate across
		// repeated innerHTML writes.
		templateContext := node.IsHTMLTemplate()
		var newChildren []*dom.Node
		limits := dom.ParseLimits{
			Nodes: 1024, Depth: 128, Attributes: 256, AttributeBytes: 4096,
		}
		var parsed *dom.Node
		var err error
		if node.Doc != nil && node.Doc.IsXML && !forceHTMLFragment {
			parsed, err = ParseXMLFragmentBounded(node, html, limits)
		} else {
			parsed, err = dom.ParseFragmentBounded(node, html, limits)
		}
		if err != nil {
			if node.Doc != nil && node.Doc.IsXML && !forceHTMLFragment {
				panic(r.domError("SyntaxError", "The XML fragment could not be parsed"))
			}
			// Fallback: treat the source as a text node (matches prior behavior).
			if html != "" {
				newChildren = append(newChildren, node.Doc.NewText(html))
			}
		} else {
			markParsedScriptsStarted(parsed)
			for c := parsed.FirstChild; c != nil; c = c.NextSibling {
				newChildren = append(newChildren, c)
			}
		}
		// The HTML fragment parsing steps for an HTML element context leave an
		// empty <html> element with its required body child, including when an
		// innerHTML assignment clears previously present head/body children.
		if len(newChildren) == 0 && node.Namespace == dom.NSHTML && node.Data == "html" && node.Doc != nil {
			newChildren = append(newChildren, node.Doc.NewElement("body"))
		}
		target := node
		if templateContext {
			target = node.TemplateContent()
		}
		// The old children were charged only if node was live; credit them on
		// the same condition so a detached host's writes never under-count.
		parentWasLive := r.nodeIsLive(node)
		var old []*dom.Node
		for c := target.FirstChild; c != nil; c = c.NextSibling {
			old = append(old, c)
		}
		r.admitInsert("innerHTML", node, newChildren, func() {
			// Detach and credit the existing children, then move the parsed ones
			// in. AppendChild adopts each out of the parse tree's document.
			for _, c := range old {
				target.RemoveChild(c)
			}
			if parentWasLive {
				r.releaseNodes(old)
			}
			for _, c := range newChildren {
				target.AppendChild(c)
			}
		})
		if len(old) > 0 || len(newChildren) > 0 {
			r.queueMutationRecord(MutationRecord{Type: "childList", Target: target, AddedNodes: newChildren, RemovedNodes: old})
		}
		r.markRadioGroupsDirtyForNodes(append(old, newChildren...))
		if parentWasLive {
			for _, child := range old {
				r.invokeDisconnectedCallbacks(child)
			}
		}
		parseRegistry := (*CustomElementRegistry)(nil)
		preserveNullRegistry := !templateContext
		if !templateContext {
			parseRegistry = r.customElementRegistryForNode(node)
			r.associateScopedRegistryWithDocument(parseRegistry, node.Doc)
		}
		for _, child := range newChildren {
			r.associateParsedSubtreeWithRegistry(child, parseRegistry, preserveNullRegistry)
		}
		for _, child := range newChildren {
			r.activateInsertedSubtree(child)
		}
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = doc.Set("prepend", func(call goja.FunctionCall) goja.Value {
		return r.jsPrepend(call)
	})
	_ = doc.Set("append", func(call goja.FunctionCall) goja.Value {
		return r.jsAppend(call)
	})
	_ = doc.Set("__getChildren__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue([]any{})
		}
		var list []goja.Value
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			if c.Element() {
				list = append(list, r.wrapNode(c))
			}
		}
		if list == nil {
			list = []goja.Value{}
		}
		return r.vm.ToValue(list)
	})
	_ = doc.Set("__getParentElement__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil || node.Parent == nil || !node.Parent.Element() {
			return goja.Null()
		}
		return r.wrapNode(node.Parent)
	})
	// __getParentNode__ is parentNode, which is not parentElement: the DOM
	// Standard's parentNode reaches the *document* node too, so
	// `document.documentElement.parentNode === document` and a text node under
	// <html> has a parent at all. Binding parentNode to parentElement (as this
	// file used to) silently made the root of the tree look detached, and
	// jQuery's `while ((elem = elem[dir]) && elem.nodeType === 1)` ancestor
	// walks stopped one level early.
	_ = doc.Set("__getParentNode__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Null()
		}
		return r.wrapNode(node.Parent)
	})

	// Form-control helpers for checked/value property accessors.
	_ = doc.Set("__getChecked__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue(false)
		}
		return r.vm.ToValue(node.InputChecked())
	})
	_ = doc.Set("__setChecked__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		checked := call.Arguments[1].ToBoolean()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		if r.setInputCheckedness(node, checked) {
			r.notifyMutation()
		}
		return goja.Undefined()
	})
	_ = doc.Set("__getValue__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("")
		}
		if node.Data == "input" && strings.EqualFold(node.GetAttribute("type"), "file") {
			files := r.inputFiles[node]
			if len(files) == 0 {
				return r.vm.ToValue("")
			}
			if file := r.fileValues[files[0]]; file != nil {
				return r.vm.ToValue(`C:\fakepath\` + file.name)
			}
			return r.vm.ToValue("")
		}
		if node.Data == "input" {
			value := node.InputControlValue()
			if !node.InputValueDirty {
				value = r.sanitizeInputValue(node, value)
				if value != node.InputControlValue() {
					node.SetInputControlValue(value, false)
				}
			}
			return r.vm.ToValue(value)
		}
		if node.Data == "textarea" {
			return r.vm.ToValue(node.InputControlValue())
		}
		if node.HasAttribute("value") {
			return r.vm.ToValue(node.GetAttribute("value"))
		}
		return r.vm.ToValue("")
	})
	_ = doc.Set("__setValue__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		v := call.Arguments[1].String()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		if node.Data == "input" && strings.EqualFold(node.GetAttribute("type"), "file") {
			if v != "" {
				panic(r.domError("InvalidStateError", "The value of a file input can only be set to the empty string."))
			}
			r.clearInputFiles(node)
			return goja.Undefined()
		}
		if node.Data == "input" && dom.InputValueMode(node.GetAttribute("type")) {
			before := node.InputControlValue()
			node.SetInputControlValue(r.sanitizeInputValue(node, v), true)
			node.ValueChangedByUser = false
			if before != node.InputControlValue() {
				r.resetTextSelectionAfterValueSet(node, node.InputControlValue())
			}
			r.RefreshInputPatternMismatch(node)
			if before != node.InputControlValue() {
				r.notifyMutation()
			}
			return goja.Undefined()
		}
		if node.Data == "textarea" {
			before := node.InputControlValue()
			node.SetInputControlValue(strings.ReplaceAll(v, "\r", ""), true)
			node.ValueChangedByUser = false
			if before != node.InputControlValue() {
				r.resetTextSelectionAfterValueSet(node, node.InputControlValue())
			}
			if before != node.InputControlValue() {
				r.notifyMutation()
			}
			return goja.Undefined()
		}
		before, hadValue := node.GetAttribute("value"), node.HasAttribute("value")
		node.SetAttribute("value", v)
		r.RefreshInputPatternMismatch(node)
		if !hadValue || before != v {
			r.notifyMutation()
		}
		return goja.Undefined()
	})
	_ = doc.Set("__getDialogOpen__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue(false)
		}
		return r.vm.ToValue(node.HasAttribute("open"))
	})
	_ = doc.Set("__setDialogOpen__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		open := call.Arguments[1].ToBoolean()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		if open {
			node.SetAttribute("open", "")
		} else {
			node.RemoveAttribute("open")
			node.IsModal = false
		}
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = doc.Set("__getDialogClosedBy__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("none")
		}
		if node.HasAttribute("closedby") {
			switch strings.ToLower(node.GetAttribute("closedby")) {
			case "any":
				return r.vm.ToValue("any")
			case "closerequest":
				return r.vm.ToValue("closerequest")
			case "none":
				return r.vm.ToValue("none")
			}
		}
		if node.HasAttribute("open") && node.IsModal {
			return r.vm.ToValue("closerequest")
		}
		return r.vm.ToValue("none")
	})
	_ = doc.Set("__setDialogClosedBy__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		node.SetAttribute("closedby", call.Arguments[1].String())
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = doc.Set("__getDialogReturnValue__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.ReturnValue)
	})
	_ = doc.Set("__setDialogReturnValue__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		v := call.Arguments[1].String()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		node.ReturnValue = v
		return goja.Undefined()
	})
	_ = doc.Set("__getPopover__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil || !node.HasAttribute("popover") {
			return goja.Null()
		}
		val := node.GetAttribute("popover")
		if val == "" || strings.EqualFold(val, "auto") {
			return r.vm.ToValue("auto")
		}
		if strings.EqualFold(val, "manual") {
			return r.vm.ToValue("manual")
		}
		return r.vm.ToValue(val)
	})
	_ = doc.Set("__setPopover__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		if len(call.Arguments) < 2 || goja.IsNull(call.Arguments[1]) || goja.IsUndefined(call.Arguments[1]) {
			r.preparePopoverAttributeChange(node, "popover", "", false)
			node.RemoveAttribute("popover")
			node.IsPopoverOpen = false
		} else {
			r.preparePopoverAttributeChange(node, "popover", call.Arguments[1].String(), true)
			node.SetAttribute("popover", call.Arguments[1].String())
		}
		r.notifyMutation()
		return goja.Undefined()
	})
	_ = doc.Set("__getId__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(node.GetAttribute("id"))
	})
	_ = doc.Set("__setId__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		v := call.Arguments[1].String()
		node := r.nodeRegistry[nid]
		if node == nil {
			return goja.Undefined()
		}
		node.SetAttribute("id", v)
		r.notifyMutation()
		return goja.Undefined()
	})
	r.installMediaBridge(doc)
	_ = doc.Set("__datasetSupported__", func(call goja.FunctionCall) goja.Value {
		node := r.nodeRegistry[int(call.Argument(0).ToInteger())]
		return r.vm.ToValue(node != nil && node.Element() &&
			(node.Namespace == dom.NSHTML || node.Namespace == dom.NSSVG || node.Namespace == dom.NSMathML))
	})
	_ = doc.Set("__getDataset__", func(call goja.FunctionCall) goja.Value {
		nid := int(call.Arguments[0].ToInteger())
		node := r.nodeRegistry[nid]
		m := r.vm.NewObject()
		if node == nil || !node.Element() {
			return m
		}
		for _, a := range node.Attr {
			if a.Namespace != dom.NSHTML || a.NamespaceURI != "" {
				continue
			}
			key, ok := datasetKey(strings.ToLower(a.Name))
			if !ok {
				continue
			}
			attrName := a.Name
			getter := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
				cur := r.nodeRegistry[nid]
				if cur == nil {
					return goja.Undefined()
				}
				v, _ := cur.GetAttributeNS("", attrName)
				return r.vm.ToValue(v)
			})
			setter := r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				cur := r.nodeRegistry[nid]
				if cur == nil {
					return goja.Undefined()
				}
				var v string
				if len(call.Arguments) > 0 && call.Arguments[0] != nil {
					v = call.Arguments[0].String()
				}
				r.setDatasetAttribute(cur, attrName, v)
				return goja.Undefined()
			})
			_ = m.DefineAccessorProperty(key, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
		}
		// The registry key rides along non-enumerably so Object.keys sees
		// only the data properties, as on a real DOMStringMap.
		_ = m.DefineDataProperty("__nid__", r.vm.ToValue(nid), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_FALSE)
		return m
	})
	_ = doc.Set("__getDatasetAttr__", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Null()
		}
		node := r.nodeRegistry[int(call.Arguments[0].ToInteger())]
		if node == nil || !node.Element() {
			return goja.Null()
		}
		attr := call.Arguments[1].String()
		for _, a := range node.Attr {
			if a.Namespace == dom.NSHTML && a.NamespaceURI == "" && strings.EqualFold(a.Name, attr) {
				return r.vm.ToValue(a.Value)
			}
		}
		return goja.Null()
	})
	_ = doc.Set("__deleteDatasetAttr__", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		node := r.nodeRegistry[int(call.Arguments[0].ToInteger())]
		if node == nil || !node.Element() {
			return goja.Undefined()
		}
		attr := call.Arguments[1].String()
		r.deleteDatasetAttribute(node, attr)
		return goja.Undefined()
	})
	_ = doc.Set("__setDatasetAttr__", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 4 {
			return goja.Undefined()
		}
		target, _ := call.Arguments[0].(*goja.Object)
		nid := int(call.Arguments[1].ToInteger())
		node := r.nodeRegistry[nid]
		if node == nil || !node.Element() || target == nil {
			return goja.Undefined()
		}
		attr, v := call.Arguments[2].String(), call.Arguments[3].String()
		r.setDatasetAttribute(node, attr, v)
		// Define the live pair on the map so later reads and Object.keys
		// see the new attribute exactly like a parsed one.
		if key, ok := datasetKey(strings.ToLower(attr)); ok {
			attrName := attr
			getter := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
				if cur := r.nodeRegistry[nid]; cur != nil {
					v, _ := cur.GetAttributeNS("", attrName)
					return r.vm.ToValue(v)
				}
				return goja.Undefined()
			})
			setter := r.vm.ToValue(func(setCall goja.FunctionCall) goja.Value {
				cur := r.nodeRegistry[nid]
				if cur == nil {
					return goja.Undefined()
				}
				var sv string
				if len(setCall.Arguments) > 0 && setCall.Arguments[0] != nil {
					sv = setCall.Arguments[0].String()
				}
				r.setDatasetAttribute(cur, attrName, sv)
				return goja.Undefined()
			})
			_ = target.DefineAccessorProperty(key, getter, setter, goja.FLAG_TRUE, goja.FLAG_TRUE)
		}
		return goja.Undefined()
	})

	// These HTMLDocument references follow the live Document tree. Cached Go
	// pointers become stale when script removes or replaces the html/head/body.
	_ = doc.DefineAccessorProperty("body", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(r.currentDocumentBody())
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = doc.DefineAccessorProperty("documentElement", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(r.currentDocumentElement())
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = doc.DefineAccessorProperty("head", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(r.currentDocumentChild("head"))
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = doc.DefineAccessorProperty("doctype", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.wrapNode(r.currentDocumentDoctype())
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = doc.DefineAccessorProperty("dir", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if r.domDoc == nil || r.domDoc.HTML == nil {
			return r.vm.ToValue("")
		}
		value := strings.ToLower(r.domDoc.HTML.GetAttribute("dir"))
		if value == "ltr" || value == "rtl" || value == "auto" {
			return r.vm.ToValue(value)
		}
		return r.vm.ToValue("")
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if r.domDoc != nil && r.domDoc.HTML != nil {
			r.domDoc.HTML.SetAttribute("dir", call.Argument(0).String())
			r.notifyMutation()
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	_ = doc.Set("__getActiveElement__", func(goja.FunctionCall) goja.Value {
		var active *dom.Node
		if r.opts.ActiveElement != nil {
			active = r.opts.ActiveElement()
		}
		if active != nil && !active.IsConnected() {
			active = nil
		}
		if active == nil && r.domDoc != nil {
			active = r.domDoc.Body
			if active == nil {
				active = r.domDoc.HTML
			}
		}
		return r.wrapNode(retargetDocumentActiveElement(active))
	})
	_ = r.vm.Set("__document__", doc)
	_, _ = r.vm.RunString(`Object.defineProperty(__document__, 'activeElement', {
		get: function() { return document.__getActiveElement__(); },
		configurable: true,
		enumerable: true
	});`)
	_ = r.vm.Set("__document__", nil)

	_ = doc.Set("__getShadowRoot__", func(call goja.FunctionCall) goja.Value {
		return r.jsGetShadowRoot(call)
	})

	// TreeWalker and NodeIterator traversal API, plus NodeFilter constants.
	r.installTreeWalkerAPI(doc)
	r.installInitialWindowNamedProperties()
	// Register parser-created inline handlers before scripts can attach later
	// listeners. Elements parsed after this point are handled by the parser hook.
	if r.domDoc != nil {
		var visit func(*dom.Node)
		visit = func(n *dom.Node) {
			if n == nil {
				return
			}
			for _, attr := range n.Attr {
				if attr.Namespace == dom.NSHTML && attr.NamespaceURI == "" {
					r.syncEventHandlerContentAttribute(n, attr.Name, attr.Value, true)
				}
			}
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		visit(&r.domDoc.Node)
	}
	r.installWindowFrameProperties()
}

// wrapNode creates a JS wrapper object for a DOM node. The wrapper inherits
// from nodeProto (which provides appendChild, textContent, addEventListener)
// and stores a __nid__ registry key so the prototype methods can find the
// underlying *dom.Node.
func (r *Runtime) wrapNode(node *dom.Node) goja.Value {
	if node == nil {
		return goja.Null()
	}
	// Nodes from an active same-origin child document must use the same
	// parent-visible wrapper whether they arrive through contentDocument or a
	// child WindowProxy value (for example, MutationRecord.target). For custom
	// elements that wrapper is source-backed so its child-realm prototype and
	// instance state stay observable.
	if node.Type != dom.NodeDocument && node.Doc != nil {
		if frame := r.iframeForDocument(node.Doc); frame != nil {
			if child := r.iframeChildRuntimes[frame]; child != nil && child.domDoc == node.Doc {
				if source, ok := child.wrapNode(node).(*goja.Object); ok {
					return r.importChildNode(child, source, node, make(map[*goja.Object]*goja.Object), 0)
				}
			}
		}
	}
	return r.wrapNodeLocal(node)
}

// wrapNodeLocal creates a wrapper in this runtime without routing through an
// active iframe's child realm. It is used by the iframe bridge when it needs a
// native parent-owned wrapper for nodes whose realm has not been initialized.
func (r *Runtime) wrapNodeLocal(node *dom.Node) goja.Value {
	if node == nil {
		return goja.Null()
	}
	// One wrapper per node: a repeat wrap refreshes the snapshot data
	// properties and returns the same object. This keeps `===` identity
	// across queries (per DOM spec) and avoids recompiling the
	// textContent/innerHTML/id accessors plus dozens of sets per hit —
	// querySelectorAll over thousands of nodes used to spend seconds here.
	if r.wrapCache == nil {
		r.wrapCache = make(map[*dom.Node]int)
	}
	if nid, ok := r.wrapCache[node]; ok {
		if obj := r.nodeWrappers[nid]; obj != nil {
			if node.Element() && node.Doc != nil && node.Doc != r.domDoc {
				_, upgraded := r.customElementInstances[node]
				// Adoption callbacks can wrap the node while its constructor is
				// running, before markCustomElementInstance records the definition.
				// The construction stack owns its prototype until that algorithm ends.
				constructing := false
				for _, current := range r.customConstructionNodes {
					if current == node {
						constructing = true
						break
					}
				}
				if !upgraded && !constructing && !r.isFailedCustomElement(node) {
					if prototype := r.protoForNode(node); prototype != nil && obj.Prototype() != prototype {
						_ = obj.SetPrototype(prototype)
					}
				}
			}
			// No snapshot refresh: every reflected property is an accessor on
			// the interface prototype and reads the live node, so a cached
			// wrapper answers exactly what a fresh one would.
			return obj
		}
	}
	nid := r.nextNID
	r.nextNID++
	r.nodeRegistry[nid] = node
	r.wrapCache[node] = nid

	obj := r.vm.NewObject()
	_ = obj.Set("__nid__", nid)
	r.nodeWrappers[nid] = obj
	// Every node a page can reach carries its spec ordinal. jQuery tests
	// `1 === e.nodeType` on each node it walks and `9 === doc.nodeType` on the
	// document, and reads a missing nodeType as "this is not a node", so one
	// absent property is enough to make the library do nothing at all. A node's
	// kind never changes, so this is a data property: no accessor, nothing to
	// refresh when a cached wrapper is returned.
	_ = obj.Set("nodeType", domNodeType(node.Type))
	if node.Type == dom.NodeDoctype {
		name := node.DoctypeName
		if name == "" {
			parts := strings.Fields(node.DataContent)
			if len(parts) > 0 {
				name = parts[0]
			}
		}
		_ = obj.Set("name", name)
		_ = obj.Set("publicId", node.PublicID)
		_ = obj.Set("systemId", node.SystemID)
	}

	// The wrapper's [[Prototype]] is the element's interface prototype, so
	// methods and reflected accessors are inherited once per runtime rather
	// than copied per node. nodeType stays an own data property because it is
	// read on every tree walk and never changes.
	_ = obj.SetPrototype(r.protoForNode(node))

	// Per-instance methods that are not inheritable because they capture the
	// node (media controllers, dialog state, popover state) still belong to
	// this wrapper. Input checked/value, video accessors, dialog methods and
	// popover methods are installed here only for the tags that answer them.
	if node.Element() {
		if node.Data == "input" {
			r.defineCheckedAccessor(obj)
			r.defineValueAccessor(obj)
		}
		if node.Data == "video" {
			r.defineMediaAccessors(obj)
		}
		if node.Data == "dialog" {
			r.defineDialogElement(obj, node)
		}
		r.definePopoverMethods(obj, node)
		r.defineMicrodataAccessors(obj, node)
	}
	if node.Element() && node.Namespace == dom.NSHTML && node.Data == "form" {
		obj = r.newHTMLFormElementProxy(obj, node)
		r.nodeWrappers[nid] = obj
	} else if node.Element() && node.Namespace == dom.NSHTML && node.Data == "select" {
		obj = r.newHTMLSelectElementProxy(obj, node)
		r.nodeWrappers[nid] = obj
	}

	return obj
}

func (r *Runtime) defineCheckedAccessor(obj *goja.Object) {
	_ = r.vm.Set("__chkTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__chkTarget__, 'checked', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return false;
				return document.__getChecked__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setChecked__(nid, !!v);
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__chkTarget__", nil)
}

func (r *Runtime) defineValueAccessor(obj *goja.Object) {
	_ = r.vm.Set("__valTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__valTarget__, 'value', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getValue__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setValue__(nid, v === null ? '' : String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__valTarget__", nil)
}

func (r *Runtime) defineDialogElement(obj *goja.Object, node *dom.Node) {
	_ = obj.Set("show", func(call goja.FunctionCall) goja.Value {
		if node.HasAttribute("open") {
			return goja.Undefined()
		}
		r.saveDialogFocus(node)
		node.SetAttribute("open", "")
		node.IsModal = false
		r.focusDialogInitialTarget(node, false)
		r.notifyMutation()
		return goja.Undefined()
	})

	_ = obj.Set("showModal", func(call goja.FunctionCall) goja.Value {
		if !node.IsConnected() {
			panic(r.domError("InvalidStateError", "The dialog is not connected"))
		}
		if node.HasAttribute("open") {
			panic(r.domError("InvalidStateError", "The dialog is already open"))
		}
		r.closeAllAutoAndHintPopovers()
		r.saveDialogFocus(node)
		node.SetAttribute("open", "")
		node.IsModal = true
		r.focusDialogInitialTarget(node, true)
		r.notifyMutation()
		return goja.Undefined()
	})

	_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
		if !node.HasAttribute("open") {
			return goja.Undefined()
		}
		wasOpenWithFocus := r.hasDialogFocusReturn(node)
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) {
			node.ReturnValue = call.Arguments[0].String()
		}
		node.RemoveAttribute("open")
		node.IsModal = false
		if wasOpenWithFocus {
			r.RestoreDialogFocus(node)
		}
		r.QueueTask(func() {
			dom.DispatchEvent(node, dom.NewTrustedEvent("close", false, false))
		})
		r.notifyMutation()
		return goja.Undefined()
	})

	_ = obj.Set("requestClose", func(call goja.FunctionCall) goja.Value {
		if !node.HasAttribute("open") {
			return goja.Undefined()
		}
		if !dom.DispatchEvent(node, dom.NewTrustedEvent("cancel", false, true)) {
			return goja.Undefined()
		}
		wasOpenWithFocus := r.hasDialogFocusReturn(node)
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) {
			node.ReturnValue = call.Arguments[0].String()
		}
		node.RemoveAttribute("open")
		node.IsModal = false
		if wasOpenWithFocus {
			r.RestoreDialogFocus(node)
		}
		r.QueueTask(func() {
			dom.DispatchEvent(node, dom.NewTrustedEvent("close", false, false))
		})
		r.notifyMutation()
		return goja.Undefined()
	})

	_ = r.vm.Set("__dlgTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__dlgTarget__, 'open', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return false;
				return document.__getDialogOpen__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setDialogOpen__(nid, !!v);
			},
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(__dlgTarget__, 'returnValue', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return '';
				return document.__getDialogReturnValue__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setDialogReturnValue__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
		Object.defineProperty(__dlgTarget__, 'closedBy', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return 'none';
				return document.__getDialogClosedBy__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setDialogClosedBy__(nid, String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__dlgTarget__", nil)
}

func (r *Runtime) focusDialogInitialTarget(dialog *dom.Node, modal bool) {
	if r == nil || dialog == nil || r.opts.OnFocusChange == nil {
		return
	}
	if dialog.HasAttribute("inert") {
		if modal && r.domDoc != nil && r.domDoc.Body != nil {
			r.opts.OnFocusChange(r.domDoc.Body, true)
		}
		return
	}
	var autofocus, firstTabbable *dom.Node
	var walk func(*dom.Node)
	walk = func(parent *dom.Node) {
		for child := parent.FirstChild; child != nil; child = child.NextSibling {
			if child.Element() && child.Data == "dialog" {
				continue
			}
			if child.Element() {
				if autofocus == nil && child.HasAttribute("autofocus") && dialogFocusCandidate(child, true) {
					autofocus = child
				}
				if firstTabbable == nil && dialogFocusCandidate(child, false) {
					firstTabbable = child
				}
			}
			walk(child)
		}
	}
	walk(dialog)
	target := autofocus
	if dialog.HasAttribute("autofocus") {
		target = dialog
	}
	if target == nil {
		target = firstTabbable
	}
	if target == nil {
		target = dialog
	}
	if target != nil {
		r.opts.OnFocusChange(target, true)
	}
}

// RestoreDialogFocus returns focus to the element active before showModal. The
// browser session also calls this when Escape or method=dialog closes a modal.
func (r *Runtime) RestoreDialogFocus(dialog *dom.Node) {
	if r == nil || dialog == nil || r.opts.OnFocusChange == nil || !r.hasDialogFocusReturn(dialog) {
		return
	}
	previous, _ := r.TakeDialogFocusReturn(dialog)
	if previous == nil || !previous.IsConnected() {
		if r.domDoc != nil {
			previous = r.domDoc.Body
			if previous == nil {
				previous = r.domDoc.HTML
			}
		}
	}
	if previous != nil {
		if r.opts.OnFocusRestore != nil {
			r.opts.OnFocusRestore(previous)
		} else {
			r.opts.OnFocusChange(previous, true)
		}
	}
}

// TakeDialogFocusReturn consumes the focus target saved by showModal. The
// engine uses it while holding its focus lock, where calling the JS focus hook
// would re-enter that lock.
func (r *Runtime) TakeDialogFocusReturn(dialog *dom.Node) (*dom.Node, bool) {
	if r == nil || dialog == nil {
		return nil, false
	}
	previous, exists := r.dialogFocusReturn[dialog]
	delete(r.dialogFocusReturn, dialog)
	return previous, exists
}

func (r *Runtime) saveDialogFocus(dialog *dom.Node) {
	if r == nil || dialog == nil {
		return
	}
	if r.dialogFocusReturn == nil {
		r.dialogFocusReturn = make(map[*dom.Node]*dom.Node)
	}
	var previous *dom.Node
	if r.opts.ActiveElement != nil {
		previous = r.opts.ActiveElement()
	}
	r.dialogFocusReturn[dialog] = previous
}

func (r *Runtime) hasDialogFocusReturn(dialog *dom.Node) bool {
	if r == nil || dialog == nil {
		return false
	}
	_, exists := r.dialogFocusReturn[dialog]
	return exists
}

// AutofocusDocument runs the document autofocus processing step once the
// parser-built tree and its focus provider are available.
func (r *Runtime) AutofocusDocument() bool {
	if r == nil || r.domDoc == nil || r.opts.OnFocusChange == nil {
		return false
	}
	if r.opts.ActiveElement != nil {
		active := r.opts.ActiveElement()
		if active != nil && active != r.domDoc.Body && active != r.domDoc.HTML {
			return false
		}
	}
	var target *dom.Node
	var walk func(*dom.Node)
	walk = func(parent *dom.Node) {
		for child := parent.FirstChild; child != nil && target == nil; child = child.NextSibling {
			if child.Element() && child.HasAttribute("autofocus") && dialogFocusCandidate(child, true) {
				target = child
				return
			}
			walk(child)
		}
	}
	walk(&r.domDoc.Node)
	if target == nil {
		return false
	}
	r.opts.OnFocusChange(target, true)
	return true
}

func dialogFocusCandidate(node *dom.Node, allowNegativeTabIndex bool) bool {
	if node == nil || !node.Element() || dom.IsFormControlDisabled(node) || node.HasAttribute("disabled") || node.HasAttribute("hidden") {
		return false
	}
	for ancestor := node; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.HasAttribute("inert") {
			return false
		}
	}
	if node.HasAttribute("tabindex") {
		tabIndex, err := strconv.Atoi(strings.TrimSpace(node.GetAttribute("tabindex")))
		return err == nil && (allowNegativeTabIndex || tabIndex >= 0)
	}
	switch node.Data {
	case "input":
		return !strings.EqualFold(node.GetAttribute("type"), "hidden")
	case "textarea", "select", "button", "summary":
		return true
	case "a", "area":
		return node.HasAttribute("href")
	default:
		return dom.IsContentEditable(node)
	}
}

func (r *Runtime) definePopoverMethods(obj *goja.Object, node *dom.Node) {
	if node.Data == "button" || node.Data == "input" {
		_ = obj.DefineAccessorProperty("popoverTargetElement", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
			id := node.GetAttribute("popovertarget")
			if id == "" {
				return goja.Null()
			}
			target := r.popoverTargetForInvoker(node, id)
			if target == nil {
				return goja.Null()
			}
			return r.wrapNode(target)
		}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 || goja.IsNull(call.Argument(0)) || goja.IsUndefined(call.Argument(0)) {
				if node.HasAttribute("popovertarget") {
					node.RemoveAttribute("popovertarget")
					r.notifyMutation()
				}
				return goja.Undefined()
			}
			target := r.getNode(call.Argument(0))
			if target == nil || !target.Element() {
				panic(r.vm.NewTypeError("popoverTargetElement must be an Element or null"))
			}
			id := target.GetAttribute("id")
			if id == "" {
				panic(r.domError("NotFoundError", "The popover target element must have an id."))
			}
			node.SetAttribute("popovertarget", id)
			r.notifyMutation()
			return goja.Undefined()
		}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	_ = obj.Set("showPopover", func(call goja.FunctionCall) goja.Value {
		if !node.HasAttribute("popover") {
			panic(r.domError("NotSupportedError", "The element does not have a valid popover attribute."))
		}
		if !r.isFullyActiveDocument(node.Doc) {
			panic(r.domError("InvalidStateError", "The popover's document is not fully active."))
		}
		if r.popoverTransitions[node] {
			panic(r.domError("InvalidStateError", "The popover is already being shown or hidden."))
		}
		if node.IsPopoverOpen {
			panic(r.vm.NewTypeError("Failed to execute 'showPopover' on 'HTMLElement': The popover is already open."))
		}
		var source *dom.Node
		if len(call.Arguments) > 0 && !goja.IsNull(call.Argument(0)) && !goja.IsUndefined(call.Argument(0)) {
			options := call.Argument(0).ToObject(r.vm)
			source, _ = r.popoverSourceOption(options)
		}
		if source != nil {
			if r.popoverInvokers == nil {
				r.popoverInvokers = make(map[*dom.Node]*dom.Node)
			}
			r.popoverInvokers[node] = source
		}
		doc := node.Doc
		if !r.setPopoverOpenFrom(node, true, source) {
			if source != nil {
				delete(r.popoverInvokers, node)
			}
			if !node.IsConnected() || node.Doc != doc || !node.HasAttribute("popover") {
				panic(r.domError("InvalidStateError", "The popover is no longer valid to show."))
			}
		}
		return goja.Undefined()
	})

	_ = obj.Set("hidePopover", func(call goja.FunctionCall) goja.Value {
		if !node.HasAttribute("popover") {
			panic(r.domError("NotSupportedError", "The element does not have a valid popover attribute."))
		}
		if !node.IsPopoverOpen {
			return goja.Undefined()
		}
		r.setPopoverOpen(node, false)
		return goja.Undefined()
	})

	_ = obj.Set("togglePopover", func(call goja.FunctionCall) goja.Value {
		if !node.HasAttribute("popover") {
			panic(r.domError("NotSupportedError", "The element does not have a valid popover attribute."))
		}
		if !node.IsConnected() {
			panic(r.domError("InvalidStateError", "The element is not connected."))
		}
		var force *bool
		var source *dom.Node
		hasSource := false
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) {
			arg := call.Arguments[0]
			if _, ok := arg.Export().(bool); ok {
				b := arg.ToBoolean()
				force = &b
			} else if options, ok := arg.(*goja.Object); ok {
				if value := options.Get("force"); value != nil && !goja.IsUndefined(value) {
					b := value.ToBoolean()
					force = &b
				}
				source, hasSource = r.popoverSourceOption(options)
			} else {
				b := arg.ToBoolean()
				force = &b
			}
		}
		if !node.IsPopoverOpen && !r.isFullyActiveDocument(node.Doc) {
			panic(r.domError("InvalidStateError", "The popover's document is not fully active."))
		}
		if force != nil {
			if *force && !node.IsPopoverOpen {
				if hasSource {
					if r.popoverInvokers == nil {
						r.popoverInvokers = make(map[*dom.Node]*dom.Node)
					}
					r.popoverInvokers[node] = source
				}
				r.setPopoverOpenFrom(node, true, source)
			} else if !*force && node.IsPopoverOpen {
				r.setPopoverOpen(node, false)
			}
			return r.vm.ToValue(node.IsPopoverOpen)
		}
		if node.IsPopoverOpen {
			r.setPopoverOpen(node, false)
		} else {
			if hasSource {
				if r.popoverInvokers == nil {
					r.popoverInvokers = make(map[*dom.Node]*dom.Node)
				}
				r.popoverInvokers[node] = source
			}
			r.setPopoverOpenFrom(node, true, source)
		}
		return r.vm.ToValue(node.IsPopoverOpen)
	})

	_ = r.vm.Set("__popoverTarget__", obj)
	_, _ = r.vm.RunString(`
		Object.defineProperty(__popoverTarget__, 'popover', {
			get: function() {
				var nid = this.__nid__;
				if (nid === undefined) return null;
				return document.__getPopover__(nid);
			},
			set: function(v) {
				var nid = this.__nid__;
				if (nid === undefined) return;
				document.__setPopover__(nid, v === null || v === undefined ? null : String(v));
			},
			configurable: true,
			enumerable: true
		});
	`)
	_ = r.vm.Set("__popoverTarget__", nil)
}

func (r *Runtime) popoverSourceOption(options *goja.Object) (*dom.Node, bool) {
	if options == nil {
		return nil, false
	}
	value := options.Get("source")
	if value == nil || goja.IsUndefined(value) {
		return nil, false
	}
	if goja.IsNull(value) {
		panic(r.vm.NewTypeError("The source option must be an Element"))
	}
	node := r.getNode(value)
	if node == nil || !node.Element() {
		panic(r.vm.NewTypeError("The source option must be an Element"))
	}
	return node, true
}

func (r *Runtime) closeOpenAutoPopovers(except *dom.Node) {
	closing := r.popoversToClose(except)
	for _, popover := range closing {
		if r.popoverTransitions[popover] {
			panic(r.domError("InvalidStateError", "The popover is already being shown or hidden."))
		}
	}
	for i := len(closing) - 1; i >= 0; i-- {
		r.setPopoverOpen(closing[i], false)
	}
}

func (r *Runtime) jsInsertAdjacentHTML(call goja.FunctionCall) goja.Value {
	thisObj := call.This.ToObject(r.vm)
	nidVal := thisObj.Get("__nid__")
	if nidVal == nil || goja.IsUndefined(nidVal) {
		return goja.Undefined()
	}
	nid := int(nidVal.ToInteger())
	node := r.nodeRegistry[nid]
	if node == nil {
		return goja.Undefined()
	}
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'insertAdjacentHTML': 2 arguments required"))
	}
	position := strings.ToLower(call.Arguments[0].String())
	switch position {
	case "beforebegin", "afterbegin", "beforeend", "afterend":
	default:
		panic(r.domError("SyntaxError", "The position provided is not a valid position."))
	}
	if (position == "beforebegin" || position == "afterend") &&
		(node.Parent == nil || node.Parent.Type == dom.NodeDocument) {
		panic(r.domError("NoModificationAllowedError", "The element has no modifiable parent."))
	}
	html := call.Arguments[1].String()
	if html == "" {
		return goja.Undefined()
	}

	parseContext := node
	if position == "beforebegin" || position == "afterend" {
		parseContext = node.Parent
	}
	if parseContext != nil && parseContext.Element() && parseContext.Namespace == dom.NSHTML && parseContext.Data == "html" {
		htmlRootContext := *parseContext
		htmlRootContext.Data = "body"
		parseContext = &htmlRootContext
	}
	limits := dom.ParseLimits{
		Nodes: 1024, Depth: 32, Attributes: 256, AttributeBytes: 4096,
	}
	var parsed *dom.Node
	var err error
	if parseContext != nil && parseContext.Doc != nil && parseContext.Doc.IsXML {
		parsed, err = ParseXMLFragmentBounded(parseContext, html, limits)
	} else {
		parsed, err = dom.ParseFragmentBounded(parseContext, html, limits)
	}
	if err != nil {
		if parseContext != nil && parseContext.Doc != nil && parseContext.Doc.IsXML {
			panic(r.domError("SyntaxError", "The XML fragment could not be parsed"))
		}
		return goja.Undefined()
	}
	markParsedScriptsStarted(parsed)

	var children []*dom.Node
	for c := parsed.FirstChild; c != nil; c = c.NextSibling {
		children = append(children, c)
	}

	// Resolve the insertion parent and per-position inserter first, then reserve
	// the whole fragment once so a rejection inserts nothing.
	var parent *dom.Node
	var insertChild func(child *dom.Node)
	switch position {
	case "beforebegin":
		if node.Parent == nil {
			return goja.Undefined()
		}
		parent = node.Parent
		insertChild = func(c *dom.Node) { parent.InsertBefore(c, node) }
	case "afterbegin":
		parent = node
		if node.IsHTMLTemplate() {
			parent = node.TemplateContent()
		}
		ref := node.FirstChild
		if node.IsHTMLTemplate() {
			ref = parent.FirstChild
		}
		insertChild = func(c *dom.Node) { parent.InsertBefore(c, ref) }
	case "beforeend":
		parent = node
		if node.IsHTMLTemplate() {
			parent = node.TemplateContent()
		}
		insertChild = func(c *dom.Node) { parent.AppendChild(c) }
	case "afterend":
		if node.Parent == nil {
			return goja.Undefined()
		}
		parent = node.Parent
		ref := node.NextSibling
		insertChild = func(c *dom.Node) { parent.InsertBefore(c, ref) }
	}

	if insertChild != nil {
		oldDocuments := captureOwnerDocuments(children)
		wasConnected := captureConnectedState(children)
		r.admitInsert("insertAdjacentHTML", parent, children, func() {
			for _, c := range children {
				parsed.RemoveChild(c)
				insertChild(c)
			}
		})
		r.notifyDisconnected(children, wasConnected)
		r.notifyAdopted(children, oldDocuments)
		registryContext := node
		shadowRegistryContext := (*dom.ShadowRoot)(nil)
		if position == "beforebegin" || position == "afterend" {
			registryContext = parent
			if parent != nil && parent.ShadowRoot != nil && shadowRootContains(parent.ShadowRoot, node) {
				shadowRegistryContext = parent.ShadowRoot
			}
		}
		parseRegistry := (*CustomElementRegistry)(nil)
		preserveNullRegistry := true
		if shadowRegistryContext != nil {
			parseRegistry = r.customElementRegistryForShadowRoot(shadowRegistryContext)
			r.associateScopedRegistryWithDocument(parseRegistry, parent.Doc)
		} else if registryContext != nil && registryContext.IsHTMLTemplate() {
			preserveNullRegistry = false
		} else if registryContext != nil {
			parseRegistry = r.customElementRegistryForNode(registryContext)
			r.associateScopedRegistryWithDocument(parseRegistry, registryContext.Doc)
		}
		for _, child := range children {
			r.associateParsedSubtreeWithRegistry(child, parseRegistry, preserveNullRegistry)
		}
		for _, child := range children {
			r.activateInsertedSubtree(child)
		}
	}
	r.markRadioGroupsDirtyForNodes(children)
	r.notifyMutation()
	return goja.Undefined()
}

func (r *Runtime) jsInsertAdjacentElement(call goja.FunctionCall) goja.Value {
	target := r.getNode(call.This)
	if target == nil || !target.Element() {
		return goja.Null()
	}
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'insertAdjacentElement': 2 arguments required"))
	}
	position := strings.ToLower(call.Argument(0).String())
	if position != "beforebegin" && position != "afterbegin" && position != "beforeend" && position != "afterend" {
		panic(r.domError("SyntaxError", "The position provided is not a valid position."))
	}
	element := r.getNode(call.Argument(1))
	if element == nil || !element.Element() {
		panic(r.vm.NewTypeError("The second argument must be an Element"))
	}
	var parent, reference *dom.Node
	switch position {
	case "beforebegin":
		parent = target.Parent
		reference = target
	case "afterbegin":
		parent = target
		reference = target.FirstChild
	case "beforeend":
		parent = target
	case "afterend":
		parent = target.Parent
		reference = target.NextSibling
	}
	if parent == nil || parent.Type == dom.NodeDocument {
		return goja.Null()
	}
	r.jsInsertBefore(goja.FunctionCall{
		This: r.wrapNode(parent),
		Arguments: []goja.Value{
			call.Argument(1),
			func() goja.Value {
				if reference == nil {
					return goja.Null()
				}
				return r.wrapNode(reference)
			}(),
		},
	})
	return call.Argument(1)
}

func (r *Runtime) jsPrepend(call goja.FunctionCall) goja.Value {
	thisObj := call.This.ToObject(r.vm)
	nidVal := thisObj.Get("__nid__")
	if nidVal == nil || goja.IsUndefined(nidVal) {
		return goja.Undefined()
	}
	node := r.nodeRegistry[int(nidVal.ToInteger())]
	if node == nil {
		return goja.Undefined()
	}
	// Resolve the arguments into an insertion plan BEFORE mutating, so the whole
	// prepend can be reserved atomically. The ref-advance when a node is already
	// the first child mirrors the original single-pass behavior.
	type pending struct {
		kid    *dom.Node
		before *dom.Node
	}
	var plan []pending
	var kids []*dom.Node
	ref := node.FirstChild
	for _, arg := range call.Arguments {
		if kidNode := r.getNode(arg); kidNode != nil {
			if kidNode == ref {
				ref = kidNode.NextSibling
				continue
			}
			plan = append(plan, pending{kidNode, ref})
			kids = append(kids, kidNode)
			continue
		}
		textNode := node.Doc.NewText(arg.String())
		plan = append(plan, pending{textNode, ref})
		kids = append(kids, textNode)
	}
	r.requireNoAncestorInsertion(node, kids)
	r.validateParentNodeInsertion(node, kids, true)
	oldDocuments := captureOwnerDocuments(kids)
	wasConnected := captureConnectedState(kids)
	r.admitInsert("prepend", node, kids, func() {
		for _, p := range plan {
			node.InsertBefore(p.kid, p.before)
		}
	})
	r.markRadioGroupsDirtyForNodes(kids)
	r.notifyMutation()
	r.notifyDisconnected(kids, wasConnected)
	r.notifyAdopted(kids, oldDocuments)
	for _, kid := range kids {
		r.activateDOMInsertedSubtree(kid)
	}
	return goja.Undefined()
}

func (r *Runtime) jsAppend(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil {
		return goja.Undefined()
	}
	// Resolve arguments to nodes first so the whole append can be reserved
	// atomically; strings become text nodes.
	var kids []*dom.Node
	for _, arg := range call.Arguments {
		if kidNode := r.getNode(arg); kidNode != nil {
			kids = append(kids, kidNode)
			continue
		}
		kids = append(kids, node.Doc.NewText(arg.String()))
	}
	r.requireNoAncestorInsertion(node, kids)
	r.validateParentNodeInsertion(node, kids, false)
	oldDocuments := captureOwnerDocuments(kids)
	wasConnected := captureConnectedState(kids)
	removals := snapshotChildRemovals(kids)
	r.admitInsert("append", node, kids, func() {
		for _, k := range kids {
			node.AppendChild(k)
		}
	})
	r.queueInsertionMutation(node, kids, removals)
	r.markRadioGroupsDirtyForNodes(kids)
	r.notifyMutation()
	r.notifyDisconnected(kids, wasConnected)
	r.notifyAdopted(kids, oldDocuments)
	for _, kid := range kids {
		r.activateDOMInsertedSubtree(kid)
	}
	return goja.Undefined()
}

// datasetKey converts a data-* attribute name to its dataset property key:
// data-foo-bar becomes fooBar. Only lowercase-dashed names convert; anything
// else is not exposed on the map.
func datasetKey(attr string) (string, bool) {
	if !strings.HasPrefix(attr, "data-") || len(attr) < 5 {
		return "", false
	}
	rest := attr[5:]
	var sb strings.Builder
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		switch {
		case c >= 'A' && c <= 'Z':
			// Uppercase in the attribute name never appears from the HTML
			// parser (it lowercases); reject defensively.
			return "", false
		case c == '-' && i+1 < len(rest) && rest[i+1] >= 'a' && rest[i+1] <= 'z':
			i++
			sb.WriteByte(rest[i] - ('a' - 'A'))
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String(), true
}

// datasetAttr converts a dataset property key back to its data-* attribute
// name: fooBar becomes data-foo-bar.
func datasetAttr(key string) string {
	var sb strings.Builder
	sb.WriteString("data-")
	for _, c := range key {
		if c >= 'A' && c <= 'Z' {
			sb.WriteByte('-')
			sb.WriteRune(c + ('a' - 'A'))
		} else {
			sb.WriteRune(c)
		}
	}
	return sb.String()
}

// getNode extracts the *dom.Node from a JS wrapper object via its __nid__.
func (r *Runtime) getNode(v goja.Value) *dom.Node {
	if v == nil || goja.IsNull(v) || goja.IsUndefined(v) {
		return nil
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return nil
	}
	if node := r.iframeImportedChildNodes[obj]; node != nil {
		return node
	}
	// A parent-visible iframe wrapper may be passed back into the child realm
	// (for example, MutationObserver.observe(target)). Its __nid__ belongs to
	// the wrapper's original runtime, so resolve the bridge source before using
	// this runtime's nodeRegistry, where the same integer may identify another
	// node.
	if source := r.iframeImportedParentValues[obj]; source != nil {
		if source != obj {
			if node := r.iframeImportedChildNodes[source]; node != nil {
				return node
			}
			if sourceNID := source.Get("__nid__"); sourceNID != nil && !goja.IsUndefined(sourceNID) {
				if node := r.nodeRegistry[int(sourceNID.ToInteger())]; node != nil {
					return node
				}
			}
		}
	}
	nidVal := obj.Get("__nid__")
	if nidVal == nil || goja.IsUndefined(nidVal) {
		return nil
	}
	nid := int(nidVal.ToInteger())
	node := r.nodeRegistry[nid]
	return node
}

// jsAppendChild implements element.appendChild(child).
func (r *Runtime) jsAppendChild(call goja.FunctionCall) goja.Value {
	parentNode := r.getNode(call.This)
	if parentNode == nil {
		return goja.Undefined()
	}
	if len(call.Arguments) < 1 {
		panic(r.vm.NewTypeError("Failed to execute 'appendChild' on 'Node': 1 argument required"))
	}
	childNode := r.getNode(call.Arguments[0])
	_, childShadow := r.shadowRootFromValue(call.Arguments[0])
	if childNode == nil && childShadow == nil {
		panic(r.vm.NewTypeError("Failed to execute 'appendChild' on 'Node': parameter 1 is not of type 'Node'"))
	}
	r.requireContainerNode(parentNode, "appendChild")
	if shadow := childShadow; shadow != nil {
		var children []*dom.Node
		for child := shadow.FirstChild; child != nil; child = child.NextSibling {
			children = append(children, child)
		}
		r.requireNoAncestorInsertion(parentNode, children)
		for _, child := range children {
			r.preserveCustomElementRegistryForDetachedNode(child)
		}
		oldDocuments := captureOwnerDocuments(children)
		wasConnected := captureConnectedState(children)
		removals := snapshotChildRemovals(children)
		r.admitInsert("appendChild", parentNode, children, func() {
			for _, child := range children {
				unlinkShadowRootChild(shadow, child)
				parentNode.InsertNode(child, nil)
			}
		})
		r.queueInsertionMutation(parentNode, children, removals)
		r.markRadioGroupsDirtyForNodes(children)
		r.notifyMutation()
		r.notifyDisconnected(children, wasConnected)
		r.notifyAdopted(children, oldDocuments)
		for _, child := range children {
			r.activateDOMInsertedSubtree(child)
		}
		return call.Arguments[0]
	}
	insertedNodes := insertCharge(childNode)
	r.requireNoAncestorInsertion(parentNode, insertedNodes)
	for _, node := range insertedNodes {
		switch node.Type {
		case dom.NodeDocumentFragment, dom.NodeDoctype, dom.NodeElement, dom.NodeText, dom.NodeCDATASection, dom.NodeProcessingInstruction, dom.NodeComment:
		default:
			panic(r.domError("HierarchyRequestError", "The node cannot be inserted at this position."))
		}
	}
	if parentNode.Type == dom.NodeDocument {
		moveSet := make(map[*dom.Node]struct{}, len(insertedNodes))
		for _, node := range insertedNodes {
			moveSet[node] = struct{}{}
		}
		prospective := make([]*dom.Node, 0, len(insertedNodes)+4)
		for child := parentNode.FirstChild; child != nil; child = child.NextSibling {
			if _, willMove := moveSet[child]; !willMove {
				prospective = append(prospective, child)
			}
		}
		prospective = append(prospective, insertedNodes...)
		r.validateReplaceChildren(parentNode, prospective)
	} else {
		for _, node := range insertedNodes {
			if node.Type == dom.NodeDoctype {
				panic(r.domError("HierarchyRequestError", "The node cannot be inserted at this position."))
			}
		}
	}
	connectedNodes := []*dom.Node{childNode}
	if childNode.DocumentFragment() {
		connectedNodes = childNode.ChildNodes()
	}
	oldDocuments := captureOwnerDocuments(connectedNodes)
	wasConnected := captureConnectedState(connectedNodes)
	removals := snapshotChildRemovals(insertedNodes)
	// Gate the insertion through the document's mutation budget: a child that
	// would breach the node/depth/attribute caps throws a catchable
	// QuotaExceededError and leaves the tree unchanged. A DocumentFragment is
	// charged as its children, because those are what actually land, and
	// InsertNode moves them and empties the fragment - the step every
	// `parent.appendChild(buildFragment(...))` in the wild depends on.
	r.admitInsert("appendChild", parentNode, insertedNodes, func() {
		parentNode.InsertNode(childNode, nil)
	})
	r.queueInsertionMutation(parentNode, insertedNodes, removals)
	r.markRadioGroupsDirtyForNodes(connectedNodes)
	r.notifyMutation()
	r.notifyDisconnected(connectedNodes, wasConnected)
	r.notifyMovedSubtrees(connectedNodes, oldDocuments)
	// Invoke connectedCallback for each node that landed.
	for _, inserted := range connectedNodes {
		r.activateDOMInsertedSubtree(inserted)
	}
	if childNode.Element() && childNode.Data == "iframe" {
		r.queueIFrameLoad(childNode)
	}
	// Return the child, matching the DOM spec.
	return call.Arguments[0]
}

func (r *Runtime) requireContainerNode(node *dom.Node, operation string) {
	if node != nil && (node.Type == dom.NodeDocument || node.Type == dom.NodeElement || node.Type == dom.NodeDocumentFragment) {
		return
	}
	panic(r.domError("HierarchyRequestError", "This node type cannot have children"))
}

// jsRemoveChild implements parent.removeChild(child).
func (r *Runtime) jsRemoveChild(call goja.FunctionCall) goja.Value {
	parent := r.getNode(call.This)
	if parent == nil || len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'removeChild' on 'Node': 1 argument required"))
	}
	child := r.getNode(call.Arguments[0])
	if child == nil {
		panic(r.vm.NewTypeError("Failed to execute 'removeChild' on 'Node': parameter 1 is not of type 'Node'"))
	}
	if child.Parent != parent {
		panic(r.domError("NotFoundError", "Failed to execute 'removeChild' on 'Node': the node to be removed is not a child of this node."))
	}
	// Credit the budget only if child was live before detaching; a child under
	// a detached parent was never charged, and crediting it would under-count.
	wasLive := r.nodeIsLive(child)
	wasConnected := child.IsConnected()
	removal := snapshotChildRemovals([]*dom.Node{child})
	r.adjustLiveRangesForNodeRemoval(parent, child, childIndex(parent, child))
	parent.RemoveChild(child)
	for _, snapshot := range removal {
		r.queueMutationRecord(MutationRecord{Type: "childList", Target: parent, RemovedNodes: []*dom.Node{child}, PreviousSibling: snapshot.previous, NextSibling: snapshot.next})
	}
	r.markRadioGroupsDirtyForNodes([]*dom.Node{child})
	if wasLive {
		r.releaseNodes([]*dom.Node{child})
	}
	r.notifyMutation()
	if wasConnected {
		r.invokeDisconnectedCallbacks(child)
	}
	return r.wrapNode(child)
}

// newJSArray builds a JS Array from a slice of goja values.
func newJSArray(vm *goja.Runtime, vals []goja.Value) *goja.Object {
	arr := vm.NewArray()
	for i, v := range vals {
		_ = arr.Set(fmt.Sprintf("%d", i), v)
	}
	return arr
}

// setupTimers installs setTimeout, setInterval, clearTimeout, and clearInterval
// on both the global scope and the window object, matching the browser model
// where these are properties of the Window interface and also available as bare
// globals.
func (r *Runtime) setupTimers() {
	setTimeout := func(callback goja.Value, delay goja.Value) int {
		ms := int64(0)
		if delay != nil && !goja.IsUndefined(delay) && !goja.IsNull(delay) {
			ms = delay.ToInteger()
		}
		if ms < 0 {
			ms = 0
		}
		r.nextTimerID++
		id := r.nextTimerID
		r.timers[id] = &timer{
			id:        id,
			callback:  callback,
			interval:  time.Duration(ms) * time.Millisecond,
			nextFire:  time.Now().Add(time.Duration(ms) * time.Millisecond),
			repeat:    false,
			fromDefer: r.inDeferScript,
		}
		return id
	}

	setInterval := func(callback goja.Value, delay goja.Value) int {
		ms := int64(0)
		if delay != nil && !goja.IsUndefined(delay) && !goja.IsNull(delay) {
			ms = delay.ToInteger()
		}
		if ms < 1 {
			ms = 1 // setInterval with 0 would spin; clamp to 1ms
		}
		r.nextTimerID++
		id := r.nextTimerID
		r.timers[id] = &timer{
			id:        id,
			callback:  callback,
			interval:  time.Duration(ms) * time.Millisecond,
			nextFire:  time.Now().Add(time.Duration(ms) * time.Millisecond),
			repeat:    true,
			fromDefer: r.inDeferScript,
		}
		return id
	}

	clearTimeout := func(id int) {
		delete(r.timers, id)
	}

	clearInterval := func(id int) {
		delete(r.timers, id)
	}

	// Install on the global scope (bare names).
	_ = r.vm.Set("setTimeout", setTimeout)
	_ = r.vm.Set("setInterval", setInterval)
	_ = r.vm.Set("clearTimeout", clearTimeout)
	_ = r.vm.Set("clearInterval", clearInterval)

	// Also install on the window object, matching the browser spec.
	win := r.vm.Get("window").(*goja.Object)
	_ = win.Set("setTimeout", setTimeout)
	_ = win.Set("setInterval", setInterval)
	_ = win.Set("clearTimeout", clearTimeout)
	_ = win.Set("clearInterval", clearInterval)
}

// TickDeferredTimer runs one ready timer queued while a deferred script was
// executing. The document parser uses this task boundary before queuing
// DOMContentLoaded; timers queued by this callback remain for a later Tick.
// QueueTask queues a host callback on the runtime's task queue. Host callbacks
// run with the next timer task, after the current operation and microtask
// checkpoint have finished.
func (r *Runtime) QueueTask(callback func()) int {
	if r == nil || callback == nil {
		return 0
	}
	wrapped := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		callback()
		return goja.Undefined()
	})
	r.nextTimerID++
	id := r.nextTimerID
	r.timers[id] = &timer{id: id, callback: wrapped, nextFire: time.Now()}
	return id
}

func (r *Runtime) TickDeferredTimer() bool {
	var next *timer
	for _, candidate := range r.timers {
		if !candidate.fromDefer || time.Now().Before(candidate.nextFire) {
			continue
		}
		if next == nil || candidate.nextFire.Before(next.nextFire) ||
			(candidate.nextFire.Equal(next.nextFire) && candidate.id < next.id) {
			next = candidate
		}
	}
	if next == nil {
		return false
	}
	if next.repeat {
		next.nextFire = time.Now().Add(next.interval)
		next.fromDefer = false
	} else {
		delete(r.timers, next.id)
	}
	if fn, ok := goja.AssertFunction(next.callback); ok {
		if _, err := r.callBounded(func() (goja.Value, error) { return fn(nil) }); err != nil {
			return true
		}
	}
	_ = r.drainMicrotasks()
	return true
}

// Tick processes one ready timer task in the goja runtime, after draining
// ready I/O callbacks. A timer callback is a task; timers it schedules belong
// to later event-loop turns even when their delay is zero. One-shot timers are
// removed after firing, while repeating timers are rescheduled for a later
// turn. A closed runtime is a no-op.
func (r *Runtime) Tick() bool {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return false
	}
	r.DrainFetchCallbacks()
	r.DrainWSEvents()
	r.DrainWorkerMessages()
	r.DrainServiceWorkerEvents()
	r.DrainBroadcastEvents()
	r.DrainMessagePortEvents()
	// Checkpoint after the I/O callbacks: a fetch resolving promises queues
	// reactions that must run before the next timer task, not after Tick.
	if err := r.drainMicrotasks(); err != nil {
		return false
	}
	// Promise and I/O callbacks can append iframes after the last direct Run.
	// Deliver their queued load tasks at this event-loop checkpoint so async
	// scripts observe the same frame lifecycle as parser-time insertion.
	r.flushIFrameLoads()
	if r.tickDynamicScripts() {
		return true
	}
	if r.tickIFrameChildren() {
		return true
	}
	now := time.Now()
	var next *timer
	for _, candidate := range r.timers {
		if now.Before(candidate.nextFire) {
			continue
		}
		if next == nil || candidate.nextFire.Before(next.nextFire) ||
			(candidate.nextFire.Equal(next.nextFire) && candidate.id < next.id) {
			next = candidate
		}
	}
	if next == nil {
		return false
	}
	if next.repeat {
		next.nextFire = now.Add(next.interval)
	} else {
		delete(r.timers, next.id)
	}
	if fn, ok := goja.AssertFunction(next.callback); ok {
		if _, err := r.callBounded(func() (goja.Value, error) { return fn(nil) }); err != nil {
			return true
		}
	}
	// A microtask checkpoint follows each task before the next event-loop turn.
	if err := r.drainMicrotasks(); err != nil {
		return true
	}
	r.flushIFrameLoads()
	return true
}

// tickIFrameChildren gives child browsing contexts an event-loop turn when
// the embedding engine pumps the top-level document. Each child Runtime keeps
// its own task queue and realm.
func (r *Runtime) tickIFrameChildren() bool {
	var tick func(*dom.Node) bool
	tick = func(node *dom.Node) bool {
		if node == nil {
			return false
		}
		if node.Element() && node.Data == "iframe" {
			if child := r.iframeChildRuntimes[node]; child != nil && child.Tick() {
				return true
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if tick(child) {
				return true
			}
		}
		return false
	}
	return r.domDoc != nil && tick(&r.domDoc.Node)
}

// Run evaluates one classic script to completion, returning an *Error for an
// uncaught exception, ErrInterrupted if the bound fired, or ErrClosed after
// Close.
func (r *Runtime) Run(src, sourceURL string) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return ErrClosed
	}
	r.mu.Unlock()

	if len(src) > MaxScriptBytes {
		return ErrTooLarge
	}

	timeout := r.opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	// Arm the timeout interrupt. It stays armed across the microtask
	// checkpoint below so a self-perpetuating reaction chain ends in
	// ErrInterrupted rather than a hang.
	timer := time.AfterFunc(timeout, func() {
		r.vm.Interrupt(ErrInterrupted)
	})

	// Compile first to catch syntax errors before execution.
	prog, err := goja.Compile(sourceURL, replaceGojaMaxCodePointEscape(src), false)
	if err != nil {
		timer.Stop()
		return r.mapError(err, sourceURL)
	}

	// Run the compiled program.
	outermostScript := r.scriptExecutionDepth == 0
	r.scriptExecutionDepth++
	_, runErr := r.vm.RunProgram(prog)
	r.scriptExecutionDepth--

	// Microtask checkpoint: reactions queued during the task drain before
	// Run returns, even when the program threw. A checkpoint failure
	// (interrupt) is reported unless the program already failed first.
	var drainErr error
	if outermostScript {
		drainErr = r.drainMicrotasks()
	}
	timer.Stop()
	if runErr != nil {
		return r.mapError(runErr, sourceURL)
	}
	if drainErr != nil {
		return drainErr
	}
	r.flushIFrameLoads()

	return nil
}

// Evaluate executes a JavaScript expression, returning its string representation or an error.
func (r *Runtime) Evaluate(expr string) (string, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return "", ErrClosed
	}
	r.mu.Unlock()

	if len(expr) > MaxScriptBytes {
		return "", ErrTooLarge
	}

	timeout := r.opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	timer := time.AfterFunc(timeout, func() {
		r.vm.Interrupt(ErrInterrupted)
	})
	defer timer.Stop()

	prog, err := goja.Compile("eval", replaceGojaMaxCodePointEscape(expr), false)
	if err != nil {
		return "", r.mapError(err, "eval")
	}
	val, err := r.vm.RunProgram(prog)
	_ = r.drainMicrotasks()
	if err != nil {
		return "", r.mapError(err, "eval")
	}
	if val == nil || goja.IsUndefined(val) {
		return "undefined", nil
	}
	if goja.IsNull(val) {
		return "null", nil
	}
	return val.String(), nil
}

func (r *Runtime) mapError(err error, sourceURL string) error {
	if err == nil {
		return nil
	}

	// Check for interrupt (timeout or manual).
	var gojaErr *goja.InterruptedError
	if errors.As(err, &gojaErr) {
		return ErrInterrupted
	}

	// Check if it's a goja Exception (uncaught JS exception).
	var exc *goja.Exception
	if errors.As(err, &exc) {
		msg := exc.String()
		line := extractLine(err.Error())
		return &Error{
			SourceURL: sourceURL,
			Line:      line,
			Message:   msg,
		}
	}

	// Syntax or other compile error.
	line := extractLine(err.Error())
	return &Error{
		SourceURL: sourceURL,
		Line:      line,
		Message:   err.Error(),
	}
}

func extractLine(errMsg string) int {
	// goja error messages often contain line information.
	// Try to extract it from patterns like "line N" or "(line N)".
	for i := 0; i < len(errMsg); i++ {
		if i+5 < len(errMsg) && errMsg[i:i+5] == "line " {
			j := i + 5
			for j < len(errMsg) && errMsg[j] >= '0' && errMsg[j] <= '9' {
				j++
			}
			if j > i+5 {
				if n, err := strconv.Atoi(errMsg[i+5 : j]); err == nil {
					return n
				}
			}
		}
	}
	return 1
}

// Global reads a top-level binding created by a script.
func (r *Runtime) Global(name string) (Value, bool) {
	v := r.vm.Get(name)
	if v == nil || goja.IsUndefined(v) {
		return Value{}, false
	}
	return exportValue(v), true
}

func exportValue(v goja.Value) Value {
	if goja.IsNull(v) {
		return Value{Kind: KindNull}
	}

	switch val := v.Export().(type) {
	case float64:
		return Value{Kind: KindNumber, Num: val}
	case int64:
		return Value{Kind: KindNumber, Num: float64(val)}
	case string:
		return Value{Kind: KindString, Str: val}
	case bool:
		return Value{Kind: KindBool, Bool: val}
	default:
		if v.ExportType().Kind().String() == "func" {
			return Value{Kind: KindFunction}
		}
		return Value{Kind: KindObject}
	}
}

// Title returns what the scripts left document.title at.
func (r *Runtime) Title() string {
	v := r.vm.Get("document")
	if v != nil {
		doc := v.ToObject(r.vm)
		if doc != nil {
			titleVal := doc.Get("__title__")
			if titleVal != nil && !goja.IsUndefined(titleVal) {
				return titleVal.String()
			}
		}
	}
	return r.title
}

// Interrupt stops a Run in progress, from any goroutine.
func (r *Runtime) Interrupt() {
	r.vm.Interrupt(ErrInterrupted)
}

// Close releases the realm. Run after Close reports ErrClosed.
func (r *Runtime) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	removeObservers := r.nodeRemovalObservers
	r.nodeRemovalObservers = nil
	r.mu.Unlock()
	for _, removeObserver := range removeObservers {
		removeObserver()
	}

	if state := workerStateOf(r); state != nil {
		state.mu.Lock()
		state.closed = true
		workers := append([]*workerObj(nil), state.workers...)
		sharedWorkers := make([]*sharedWorkerEntry, 0, len(state.shared))
		for _, worker := range state.shared {
			sharedWorkers = append(sharedWorkers, worker)
		}
		state.messages = nil
		state.mu.Unlock()
		for _, worker := range workers {
			worker.mu.Lock()
			worker.terminated = true
			runner := worker.runner
			worker.pending = nil
			worker.mu.Unlock()
			if runner != nil {
				_ = runner.Terminate()
			}
		}
		for _, worker := range sharedWorkers {
			r.releaseSharedWorker(worker)
		}
	}
	if state := swStateOf(r); state != nil {
		state.mu.Lock()
		runners := append([]*localWorkerRunner(nil), state.runners...)
		state.runners = nil
		state.mu.Unlock()
		for _, runner := range runners {
			_ = runner.Terminate()
		}
	}
	r.objectURLMu.Lock()
	clear(r.objectURLs)
	r.objectURLBytes = 0
	r.objectURLMu.Unlock()
	return nil
}

// SetDOM updates the DOM document reference. The engine calls this after
// parsing HTML so the runtime's document methods (getElementById, etc.)
// operate on the same nodes the engine's arena references.
func (r *Runtime) SetDOM(doc *dom.Document) {
	r.mu.Lock()
	r.opts.DOM = doc
	r.domDoc = doc
	if r.customElementRegistry != nil {
		r.customElementRegistry.document = doc
	}
	if r.nodeRegistry == nil {
		r.nodeRegistry = make(map[int]*dom.Node)
		r.nodeWrappers = make(map[int]*goja.Object)
		r.shadowRoots = make(map[int]*goja.Object)
		r.wrapCache = make(map[*dom.Node]int)
		r.styleObjects = make(map[*dom.Node]*goja.Object)
		r.nextNID = 1
	}
	r.mu.Unlock()
	// A runtime may be created before the parser has built its document. Scan
	// the document when it is bound so parser-created customelementregistry
	// associations exist before page script can mutate the attribute.
	r.associateNullRegistryAttributesInDocument(doc)
	// Add document methods (getElementById, querySelector, etc.) now that
	// the DOM reference is set. setupDOM checks r.opts.DOM internally.
	r.setupDOM()
	// setupAnimations runs during Runtime construction, before the engine has
	// parsed HTML. Rebind Element.animate after setupDOM creates Element.prototype.
	r.installAnimationElementAPI(getAnimRegistry(r))
	// CSSOM initially installs before the parser supplies a document, so its
	// Document.prototype accessors must be rebound after setupDOM constructs
	// the live prototype chain.
	r.setupStyleSheetInterfaces()
	// The engine creates a script runtime before the document has been
	// parsed; install DOM-backed element APIs again once SetDOM supplies the
	// live document and HTMLElement prototype chain.
	r.setupElementInternals()
	// Install Range and Selection APIs now that a DOM is available.
	r.setupRange()
	r.setupSelection()
	// Upgrade document.write/writeln from no-op stubs to real implementations
	// now that a DOM is available.
	r.setupDocumentWrite()
	r.queueExistingIFrames()
	// Runtime may be created before the HTML parser supplies its Document.
	// Install the parsed document's Window named properties now, before any
	// author script runs against the bound DOM.
	r.installInitialWindowNamedProperties()
	// Parser-created inline event-handler attributes must be registered before
	// any later script adds listeners to the same target.
	if doc != nil {
		var visit func(*dom.Node)
		visit = func(n *dom.Node) {
			if n == nil {
				return
			}
			for _, attr := range n.Attr {
				if attr.Namespace == dom.NSHTML && attr.NamespaceURI == "" {
					r.syncEventHandlerContentAttribute(n, attr.Name, attr.Value, true)
				}
			}
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		visit(&doc.Node)
	}
	r.queueInitialDetailsToggleEventsForDocument(doc)
}

// ---------------------------------------------------------------------------
// JS bridge: event methods
// ---------------------------------------------------------------------------

// jsAddEventListener implements element.addEventListener(type, callback).
func (r *Runtime) jsAddEventListener(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	typ := call.Arguments[0].String()
	capture, once := false, false
	passive := defaultPassiveEventType(typ) && defaultPassiveNodeTarget(node)
	var signal *goja.Object
	if len(call.Arguments) >= 3 {
		options := call.Arguments[2]
		if object, ok := options.(*goja.Object); ok && !goja.IsNull(options) {
			if value := object.Get("capture"); value != nil {
				capture = value.ToBoolean()
			}
			if value := object.Get("once"); value != nil {
				once = value.ToBoolean()
			}
			if value := object.Get("passive"); value != nil && !goja.IsUndefined(value) {
				passive = value.ToBoolean()
			}
			if value := object.Get("signal"); value != nil && !goja.IsUndefined(value) {
				if signalObject, ok := value.(*goja.Object); ok {
					signal = signalObject
				} else {
					panic(r.vm.NewTypeError("EventListenerOptions.signal must be an AbortSignal"))
				}
			}
		} else {
			capture = options.ToBoolean()
		}
	}
	if signal != nil {
		if _, callable := goja.AssertFunction(signal.Get("__registerAbortAlgorithm__")); !callable {
			panic(r.vm.NewTypeError("EventListenerOptions.signal must be an AbortSignal"))
		}
		if signal.Get("aborted").ToBoolean() {
			return goja.Undefined()
		}
	}
	listener := call.Arguments[1]
	fn, callable := goja.AssertFunction(listener)
	listenerRealm := r
	if listenerObject, ok := listener.(*goja.Object); ok {
		if callable {
			if realm := r.iframeImportedFunctionRealms[listenerObject]; realm != nil {
				listenerRealm = realm
			} else {
				listenerRealm = r.runtimeForFunctionObject(listenerObject)
			}
		} else {
			listenerRealm = r.importedObjectRealm(listenerObject)
		}
	}
	if !callable {
		if goja.IsNull(listener) || goja.IsUndefined(listener) {
			return goja.Undefined()
		}
		listenerObject, ok := listener.(*goja.Object)
		if !ok {
			return goja.Undefined()
		}
		reflectObject := r.vm.Get("Reflect").ToObject(r.vm)
		reflectGet, _ := goja.AssertFunction(reflectObject.Get("get"))
		reflectApply, _ := goja.AssertFunction(reflectObject.Get("apply"))
		fn = func(_ goja.Value, args ...goja.Value) (goja.Value, error) {
			method, err := reflectGet(goja.Undefined(), listenerObject, r.vm.ToValue("handleEvent"))
			if err != nil {
				return nil, err
			}
			arguments := make([]interface{}, len(args))
			for i, arg := range args {
				arguments[i] = arg
			}
			return reflectApply(goja.Undefined(), method, listenerObject, r.vm.NewArray(arguments...))
		}
	}
	thisValue := call.This

	// Wrap the goja listener as a Go callback that creates a JS Event object.
	callback := func(e *dom.Event) {
		if listenerRealm != nil && !listenerRealm.isFullyActiveBrowsingContext() {
			return
		}
		eventObj := r.wrapEvent(e)
		var callbackErr error
		r.withEventListenerInvocation(func() {
			r.withCurrentEventOnNode(eventObj, node, func() { _, callbackErr = fn(thisValue, eventObj) })
		})
		listenerRealm.reportWindowExceptionFrom(r, callbackErr)
	}
	key := call.Arguments[1]
	if once {
		callback = func(e *dom.Event) {
			dom.RemoveEventListenerByKey(node, typ, key, capture)
			if listenerRealm != nil && !listenerRealm.isFullyActiveBrowsingContext() {
				return
			}
			eventObj := r.wrapEvent(e)
			var callbackErr error
			r.withEventListenerInvocation(func() {
				r.withCurrentEventOnNode(eventObj, node, func() { _, callbackErr = fn(thisValue, eventObj) })
			})
			listenerRealm.reportWindowExceptionFrom(r, callbackErr)
		}
	}
	added := dom.AddEventListenerWithOptionsAndKey(node, typ, callback, capture, passive, key)
	if added && signal != nil {
		register := signal.Get("__registerAbortAlgorithm__")
		if fn, ok := goja.AssertFunction(register); ok {
			cleanup := r.vm.ToValue(func(goja.FunctionCall) goja.Value {
				dom.RemoveEventListenerByKey(node, typ, key, capture)
				return goja.Undefined()
			})
			_, _ = fn(signal, cleanup)
		}
	}
	return goja.Undefined()
}

// withCurrentEvent exposes the event currently being dispatched as the
// legacy Window.event global while running an event listener. Older pages
// (and several engines' compatibility behavior) rely on this even when their
// listener does not declare an event parameter.
func (r *Runtime) withCurrentEvent(event goja.Value, run func()) {
	previous := r.legacyWindowEvent
	if previous == nil {
		previous = goja.Undefined()
	}
	// Window.event is a replaceable legacy property. Respect an author value,
	// while still allowing a nested dispatch to temporarily expose its event.
	if previous != nil && !goja.IsUndefined(previous) && !goja.IsNull(previous) {
		isCurrent := false
		for _, current := range r.currentEventStack {
			if previous.StrictEquals(current) {
				isCurrent = true
				break
			}
		}
		if !isCurrent {
			run()
			return
		}
	}
	r.legacyWindowEvent = event
	r.currentEventStack = append(r.currentEventStack, event)
	defer func() {
		r.currentEventStack = r.currentEventStack[:len(r.currentEventStack)-1]
		// Preserve a replacement made by script during the callback.
		if r.legacyWindowEvent != nil && r.legacyWindowEvent.StrictEquals(event) {
			r.legacyWindowEvent = previous
		}
	}()
	run()
}

func (r *Runtime) withCurrentEventOnNode(event goja.Value, node *dom.Node, run func()) {
	if r.shadowRootForNode(node) != nil {
		run()
		return
	}
	r.withCurrentEvent(event, run)
}

// jsRemoveEventListener implements element.removeEventListener(type, callback, capture).
// It uses the original goja function value as the dedup key to match the
// listener that was stored by jsAddEventListener via AddEventListenerWithKey.
func (r *Runtime) jsRemoveEventListener(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	typ := call.Arguments[0].String()
	capture := false
	if len(call.Arguments) >= 3 {
		option := call.Arguments[2]
		if object, ok := option.(*goja.Object); ok && !goja.IsNull(option) {
			if value := object.Get("capture"); value != nil && !goja.IsUndefined(value) {
				capture = value.ToBoolean()
			}
		} else {
			capture = option.ToBoolean()
		}
	}
	dom.RemoveEventListenerByKey(node, typ, call.Arguments[1], capture)
	return goja.Undefined()
}

// jsDispatchEvent implements element.dispatchEvent(event).
func (r *Runtime) jsDispatchEvent(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 1 {
		return r.vm.ToValue(false)
	}
	eventObj := call.Arguments[0]
	object := eventObj.ToObject(r.vm)
	if value := object.Get("__dispatching"); value != nil && value.ToBoolean() {
		panic(r.domError("InvalidStateError", "The event is already being dispatched"))
	}
	if value := object.Get("__initialized"); value != nil && !value.ToBoolean() {
		panic(r.domError("InvalidStateError", "The event has not been initialized"))
	}
	if typ := object.Get("type"); typ == nil || goja.IsUndefined(typ) {
		panic(r.vm.NewTypeError("parameter 1 is not of type 'Event'"))
	}
	preStopped := object.Get("__propagationStopped") != nil && object.Get("__propagationStopped").ToBoolean()
	preImmediateStopped := object.Get("__immediateStopped") != nil && object.Get("__immediateStopped").ToBoolean()
	if trusted := object.Get("__isTrusted"); trusted != nil && !goja.IsUndefined(trusted) {
		_ = object.Set("__isTrusted", false)
	} else {
		_ = object.Set("isTrusted", false)
	}
	_ = object.Set("__dispatching", true)
	defer func() { _ = object.Set("__dispatching", false) }()
	typ := ""
	if t := object.Get("type"); t != nil && !goja.IsUndefined(t) {
		typ = t.String()
	}
	readBool := func(name string, fallback bool) bool {
		value := object.Get(name)
		if value == nil || goja.IsUndefined(value) {
			return fallback
		}
		return value.ToBoolean()
	}
	readString := func(name string) string {
		value := object.Get(name)
		if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
			return ""
		}
		return value.String()
	}
	readInt32 := func(name string) int32 {
		value := object.Get(name)
		if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
			return 0
		}
		return int32(value.ToInteger())
	}
	readInt := func(name string) int {
		value := object.Get(name)
		if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
			return 0
		}
		return int(value.ToInteger())
	}
	bubbles := readBool("bubbles", true)
	cancelable := readBool("cancelable", true)
	var event *dom.Event
	marker := object.Get("__goosieEventInterface")
	markerName := ""
	if marker != nil && !goja.IsUndefined(marker) && !goja.IsNull(marker) {
		markerName = marker.String()
	}
	switch markerName {
	case "FormDataEvent":
		formData := object.Get("formData")
		if formData == nil || goja.IsUndefined(formData) || goja.IsNull(formData) {
			formData = goja.Null()
		}
		wrapped := dom.NewFormDataEvent(typ, bubbles, cancelable, formData)
		event = &wrapped.Event
	case "SubmitEvent":
		var submitter *dom.Node
		if value := object.Get("submitter"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
			submitter = r.getNode(value)
		}
		wrapped := dom.NewSubmitEvent(typ, bubbles, cancelable, submitter)
		event = &wrapped.Event
	case "ToggleEvent":
		wrapped := dom.NewToggleEvent(typ, bubbles, cancelable, readString("oldState"), readString("newState"))
		if source := object.Get("source"); source != nil && !goja.IsUndefined(source) && !goja.IsNull(source) {
			wrapped.Source = r.getNode(source)
		}
		event = &wrapped.Event
	case "ErrorEvent":
		errorValue := object.Get("error")
		if errorValue == nil {
			errorValue = goja.Null()
		}
		lineno := readInt("lineno")
		colno := readInt("colno")
		if lineno < 0 {
			lineno = 0
		}
		if colno < 0 {
			colno = 0
		}
		wrapped := dom.NewErrorEvent(typ, bubbles, cancelable, readString("message"), readString("filename"), uint32(lineno), uint32(colno), errorValue)
		event = &wrapped.Event
	case "MessageEvent":
		data := object.Get("data")
		if data == nil {
			data = goja.Null()
		}
		source := object.Get("source")
		if source == nil {
			source = goja.Null()
		}
		ports := eventPorts(object.Get("ports"), r.vm)
		message := dom.NewMessageEvent(typ, bubbles, cancelable, data, readString("origin"), readString("lastEventId"), source, ports)
		event = &message.Event
	case "MouseEvent":
		mouse := dom.NewMouseEvent(typ, bubbles, cancelable, readInt32("clientX"), readInt32("clientY"), readInt32("screenX"), readInt32("screenY"), readInt("button"))
		mouse.AltKey = readBool("altKey", false)
		mouse.CtrlKey = readBool("ctrlKey", false)
		mouse.ShiftKey = readBool("shiftKey", false)
		mouse.MetaKey = readBool("metaKey", false)
		event = &mouse.Event
	case "KeyboardEvent":
		keyboard := dom.NewKeyboardEvent(typ, bubbles, cancelable, readString("key"), readString("code"))
		keyboard.AltKey = readBool("altKey", false)
		keyboard.CtrlKey = readBool("ctrlKey", false)
		keyboard.ShiftKey = readBool("shiftKey", false)
		keyboard.MetaKey = readBool("metaKey", false)
		keyboard.Repeat = readBool("repeat", false)
		event = &keyboard.Event
	case "InputEvent":
		var data *string
		if value := object.Get("data"); value != nil && !goja.IsUndefined(value) && !goja.IsNull(value) {
			value := value.String()
			data = &value
		}
		input := dom.NewInputEvent(typ, bubbles, cancelable, data, readString("inputType"), readBool("isComposing", false))
		event = &input.Event
	case "CompositionEvent":
		composition := dom.NewCompositionEvent(typ, bubbles, cancelable, readString("data"))
		event = &composition.Event
	default:
		if strings.HasPrefix(typ, "key") {
			keyboard := dom.NewKeyboardEvent(typ, bubbles, cancelable, readString("key"), readString("code"))
			event = &keyboard.Event
		} else {
			event = dom.NewEvent(typ, bubbles, cancelable)
		}
	}
	if preStopped {
		event.StopPropagation()
	}
	if preImmediateStopped {
		event.StopImmediatePropagation()
	}
	// The window step is part of the path, not an extra: an event a page
	// dispatches on an element still reaches window listeners when it bubbles,
	// and one dispatched on the document reaches them last.
	if r.activeEventObjects == nil {
		r.activeEventObjects = make(map[*dom.Event]goja.Value)
	}
	// Event listeners must observe the exact object passed to dispatchEvent().
	// Bind this dispatch's Go event state onto that object before the listener
	// wrappers reuse it.
	_ = object.Set("target", r.wrapNode(node))
	_ = object.Set("srcElement", r.wrapNode(node))
	_ = object.Set("currentTarget", goja.Null())
	_ = object.Set("stopPropagation", func() { event.StopPropagation() })
	_ = object.Set("stopImmediatePropagation", func() { event.StopImmediatePropagation() })
	_ = object.Set("preventDefault", func() { event.PreventDefault() })
	_ = object.DefineAccessorProperty("defaultPrevented", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(event.DefaultPrevented())
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	r.activeEventObjects[event] = eventObj
	defer delete(r.activeEventObjects, event)
	var result bool
	if typ == "click" && !dom.IsFormControlDisabled(node) {
		result = r.dispatchClickActivation(node, event)
	} else {
		result = r.dispatchWithWindowStep(node, event)
	}
	_ = object.Set("__propagationStopped", false)
	_ = object.Set("__immediateStopped", false)
	// Restore the Event object's script-facing methods after the dispatch-local
	// Go closures are no longer valid. Pages can call stopPropagation() between
	// dispatches and expect that flag to affect the next dispatch.
	_ = object.Set("stopPropagation", func() { _ = object.Set("__propagationStopped", true) })
	_ = object.Set("stopImmediatePropagation", func() {
		_ = object.Set("__propagationStopped", true)
		_ = object.Set("__immediateStopped", true)
	})
	_ = object.DefineAccessorProperty("cancelBubble", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(object.Get("__propagationStopped").ToBoolean())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if call.Argument(0).ToBoolean() {
			_ = object.Set("__propagationStopped", true)
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)
	// The JS Event object is the caller-visible event object. Listener wrappers
	// use a Go-backed view while dispatching, so reflect the dispatch result
	// back onto the original object before returning from dispatchEvent().
	_ = object.Set("defaultPrevented", event.DefaultPrevented())
	_ = object.Set("currentTarget", goja.Null())
	_ = object.Set("eventPhase", 0)
	if toggleEvent := event.AsToggle(); toggleEvent != nil {
		if host := shadowTreeHost(toggleEvent.Source); host != nil {
			toggleEvent.Source = host
			_ = object.DefineDataProperty("source", r.wrapNode(host), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_TRUE)
		}
	}
	if trusted := object.Get("isTrusted"); trusted != nil && trusted.ToBoolean() {
		if trusted := object.Get("__isTrusted"); trusted != nil && !goja.IsUndefined(trusted) {
			_ = object.Set("__isTrusted", false)
		} else {
			_ = object.Set("isTrusted", false)
		}
	}
	return r.vm.ToValue(result)
}

// wrapEvent creates a JS Event object from a *dom.Event. It exposes the
// standard Event properties and, for MouseEvent and KeyboardEvent subtypes,
// their additional fields.
func (r *Runtime) wrapEvent(e *dom.Event) goja.Value {
	if wrapped := r.activeEventObjects[e]; wrapped != nil {
		if object, ok := wrapped.(*goja.Object); ok {
			_ = object.Set("__passiveListener", e.InPassiveListener())
			_ = object.Set("eventPhase", int(e.Phase()))
			if target := e.Target(); target != nil {
				_ = object.Set("target", r.wrapNode(target))
				_ = object.Set("srcElement", r.wrapNode(target))
			}
			if current := e.CurrentTarget(); current != nil {
				_ = object.Set("currentTarget", r.wrapNode(current))
				if input := e.AsInput(); input != nil {
					_ = object.Set("__targetRanges", r.staticRangeArray(r.inputTargetRangesForTree(input.TargetRanges, current)))
				}
			} else {
				_ = object.Set("currentTarget", goja.Null())
			}
			_ = object.DefineAccessorProperty("cancelBubble", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
				return r.vm.ToValue(e.PropagationStopped())
			}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
				if call.Argument(0).ToBoolean() {
					e.StopPropagation()
				}
				return goja.Undefined()
			}), goja.FLAG_TRUE, goja.FLAG_TRUE)
		}
		return wrapped
	}
	obj := r.vm.NewObject()
	if ctorValue := r.vm.Get(e.InterfaceName()); ctorValue != nil && !goja.IsUndefined(ctorValue) {
		if ctor, ok := ctorValue.(*goja.Object); ok {
			if protoValue := ctor.Get("prototype"); protoValue != nil && !goja.IsUndefined(protoValue) {
				if proto, ok := protoValue.(*goja.Object); ok {
					_ = obj.SetPrototype(proto)
				}
			}
		}
	}
	_ = obj.Set("type", e.Type)
	_ = obj.Set("bubbles", e.Bubbles())
	_ = obj.Set("cancelable", e.Cancelable())
	_ = obj.Set("isTrusted", e.IsTrusted())
	_ = obj.Set("__goosieEvent", true)
	_ = obj.Set("__initialized", true)
	_ = obj.Set("__dispatching", e.CurrentTarget() != nil)
	_ = obj.Set("__composedPath", r.vm.NewArray())
	_ = obj.Set("composedPath", func() goja.Value {
		if !obj.Get("__dispatching").ToBoolean() {
			return r.vm.NewArray()
		}
		path, _ := obj.Get("__composedPath").(*goja.Object)
		if path == nil {
			return r.vm.NewArray()
		}
		length := path.Get("length").ToInteger()
		values := make([]interface{}, length)
		for i := int64(0); i < length; i++ {
			values[i] = path.Get(strconv.FormatInt(i, 10))
		}
		return r.vm.NewArray(values...)
	})
	_ = obj.Set("eventPhase", int(e.Phase()))
	if e.InterfaceName() == "PageTransitionEvent" {
		_ = obj.Set("persisted", e.Persisted())
	}
	if errorEvent := e.AsError(); errorEvent != nil {
		_ = obj.Set("message", errorEvent.Message)
		_ = obj.Set("filename", errorEvent.Filename)
		_ = obj.Set("lineno", errorEvent.Lineno)
		_ = obj.Set("colno", errorEvent.Colno)
		if value, ok := errorEvent.Error.(goja.Value); ok {
			_ = obj.Set("error", value)
		} else {
			_ = obj.Set("error", errorEvent.Error)
		}
	}
	if hashChange := e.AsHashChange(); hashChange != nil {
		_ = obj.Set("oldURL", hashChange.OldURL)
		_ = obj.Set("newURL", hashChange.NewURL)
	}
	if submitEvent := e.AsSubmit(); submitEvent != nil {
		if submitEvent.Submitter == nil {
			_ = obj.Set("submitter", goja.Null())
		} else {
			_ = obj.Set("submitter", r.wrapNode(submitEvent.Submitter))
		}
	}
	if toggleEvent := e.AsToggle(); toggleEvent != nil {
		_ = obj.Set("oldState", toggleEvent.OldState)
		_ = obj.Set("newState", toggleEvent.NewState)
		if toggleEvent.Source == nil {
			_ = obj.Set("source", goja.Null())
		} else {
			_ = obj.Set("source", r.wrapNode(toggleEvent.Source))
		}
	}
	if formDataEvent := e.AsFormData(); formDataEvent != nil {
		if value, ok := formDataEvent.FormData.(goja.Value); ok {
			_ = obj.Set("formData", value)
		} else if formDataEvent.FormData == nil {
			_ = obj.Set("formData", goja.Null())
		} else {
			_ = obj.Set("formData", formDataEvent.FormData)
		}
	}
	if messageEvent := e.AsMessage(); messageEvent != nil {
		if value, ok := messageEvent.Data.(goja.Value); ok {
			_ = obj.Set("data", value)
		} else {
			_ = obj.Set("data", messageEvent.Data)
		}
		_ = obj.Set("origin", messageEvent.Origin)
		_ = obj.Set("lastEventId", messageEvent.LastEventID)
		if value, ok := messageEvent.Source.(goja.Value); ok {
			_ = obj.Set("source", value)
		} else {
			_ = obj.Set("source", messageEvent.Source)
		}
		ports := r.vm.NewArray()
		for i, port := range messageEvent.Ports {
			value, ok := port.(goja.Value)
			if !ok {
				value = r.vm.ToValue(port)
			}
			_ = ports.Set(strconv.Itoa(i), value)
		}
		if object := r.vm.Get("Object").ToObject(r.vm); object != nil {
			if freeze, ok := goja.AssertFunction(object.Get("freeze")); ok {
				if frozen, err := freeze(object, ports); err == nil {
					ports, _ = frozen.(*goja.Object)
				}
			}
		}
		_ = obj.Set("ports", ports)
	}
	_ = obj.Set("stopPropagation", func() { e.StopPropagation() })
	_ = obj.Set("stopImmediatePropagation", func() { e.StopImmediatePropagation() })
	_ = obj.Set("preventDefault", func() { e.PreventDefault() })
	_ = obj.Set("__passiveListener", e.InPassiveListener())
	_ = obj.DefineAccessorProperty("returnValue", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		return r.vm.ToValue(!e.DefaultPrevented())
	}), r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		if !call.Argument(0).ToBoolean() {
			e.PreventDefault()
		}
		return goja.Undefined()
	}), goja.FLAG_TRUE, goja.FLAG_TRUE)

	// defaultPrevented must be read dynamically: the callback may call
	// preventDefault() before reading this property. We expose a helper
	// method and define a JS getter that calls it.
	_ = obj.Set("__getDP__", func() bool { return e.DefaultPrevented() })
	_ = r.vm.Set("__goosieWrappedEvent__", obj)
	_ = r.vm.Set("__goosieEventInterfaceName__", e.InterfaceName())
	_, _ = r.vm.RunString(`Object.defineProperty(__goosieWrappedEvent__, Symbol.toStringTag, {
		value: __goosieEventInterfaceName__, configurable: true
	});`)
	_ = r.vm.Set("__goosieWrappedEvent__", nil)
	_ = r.vm.Set("__goosieEventInterfaceName__", nil)
	_ = r.vm.Set("__ev__", obj)
	_, _ = r.vm.RunString(`
		(function() {
			var o = __ev__;
			var getter = o.__getDP__;
			Object.defineProperty(o, 'defaultPrevented', {
				get: getter,
				configurable: true,
				enumerable: true
			});
		})();
	`)
	_ = r.vm.Set("__ev__", nil)

	if e.Target() != nil {
		_ = obj.Set("target", r.wrapNode(e.Target()))
	}
	if e.CurrentTarget() != nil {
		_ = obj.Set("currentTarget", r.wrapNode(e.CurrentTarget()))
	}

	// MouseEvent fields (only populated for a dispatched *MouseEvent, recovered
	// through the payload back-pointer since listeners receive a *Event).
	if me := e.AsMouse(); me != nil {
		_ = obj.Set("clientX", me.ClientX)
		_ = obj.Set("clientY", me.ClientY)
		_ = obj.Set("screenX", me.ScreenX)
		_ = obj.Set("screenY", me.ScreenY)
		if me.RelatedTarget == nil {
			_ = obj.Set("relatedTarget", goja.Null())
		} else {
			_ = obj.Set("relatedTarget", r.wrapNode(me.RelatedTarget))
		}
		_ = obj.Set("button", me.Button)
		_ = obj.Set("altKey", me.AltKey)
		_ = obj.Set("ctrlKey", me.CtrlKey)
		_ = obj.Set("shiftKey", me.ShiftKey)
		_ = obj.Set("metaKey", me.MetaKey)
	}

	// KeyboardEvent fields.
	if ke := e.AsKeyboard(); ke != nil {
		_ = obj.Set("key", ke.Key)
		_ = obj.Set("code", ke.Code)
		_ = obj.Set("altKey", ke.AltKey)
		_ = obj.Set("ctrlKey", ke.CtrlKey)
		_ = obj.Set("shiftKey", ke.ShiftKey)
		_ = obj.Set("metaKey", ke.MetaKey)
		_ = obj.Set("repeat", ke.Repeat)
	}
	if input := e.AsInput(); input != nil {
		if input.Data == nil {
			_ = obj.Set("data", goja.Null())
		} else {
			_ = obj.Set("data", *input.Data)
		}
		_ = obj.Set("inputType", input.InputType)
		_ = obj.Set("isComposing", input.IsComposing)
		_ = obj.Set("__targetRanges", r.staticRangeArray(r.inputTargetRangesForTree(input.TargetRanges, e.CurrentTarget())))
		_ = obj.Set("getTargetRanges", func(call goja.FunctionCall) goja.Value {
			if !obj.Get("__dispatching").ToBoolean() {
				return r.vm.NewArray()
			}
			targetShadowRoot := r.shadowRootContainingNode(e.Target())
			if targetShadowRoot != nil {
				currentShadowRoot := r.shadowRootContainingNode(r.getNode(obj.Get("currentTarget")))
				if _, shadow := r.shadowRootFromValue(obj.Get("currentTarget")); shadow != nil {
					currentShadowRoot = shadow
				}
				if currentShadowRoot != targetShadowRoot {
					return r.vm.NewArray()
				}
			}
			ranges := input.TargetRanges
			current := obj.Get("currentTarget")
			node := r.getNode(current)
			if node == nil && r.domDoc != nil && current != nil && !goja.IsNull(current) && !goja.IsUndefined(current) {
				if object, ok := current.(*goja.Object); ok && object.Get("nodeType") != nil && object.Get("nodeType").ToInteger() == int64(dom.NodeDocument) {
					node = &r.domDoc.Node
				}
			}
			if node != nil {
				ranges = r.inputTargetRangesForTree(ranges, node)
			} else if _, shadow := r.shadowRootFromValue(current); shadow != nil {
				ranges = r.inputTargetRangesForShadowRoot(ranges, shadow)
			}
			return r.staticRangeArray(ranges)
		})
		if input.DataTransfer == nil {
			_ = obj.Set("__dataTransfer", goja.Null())
		} else {
			_ = obj.Set("__dataTransfer", r.wrapDataTransfer(input.DataTransfer))
		}
	}
	if clipboard := e.AsClipboard(); clipboard != nil {
		if clipboard.ClipboardData == nil {
			_ = obj.Set("__clipboardData", goja.Null())
		} else {
			_ = obj.Set("__clipboardData", r.wrapDataTransfer(clipboard.ClipboardData))
		}
	}
	if composition := e.AsComposition(); composition != nil {
		_ = obj.Set("data", composition.Data)
	}

	// DragEvent fields. AsMouse does not see a DragEvent payload, so the
	// inherited mouse fields are read from the DragEvent itself.
	if de := e.AsDrag(); de != nil {
		_ = obj.Set("clientX", de.ClientX)
		_ = obj.Set("clientY", de.ClientY)
		_ = obj.Set("screenX", de.ScreenX)
		_ = obj.Set("screenY", de.ScreenY)
		if de.RelatedTarget == nil {
			_ = obj.Set("relatedTarget", goja.Null())
		} else {
			_ = obj.Set("relatedTarget", r.wrapNode(de.RelatedTarget))
		}
		_ = obj.Set("button", de.Button)
		_ = obj.Set("altKey", de.AltKey)
		_ = obj.Set("ctrlKey", de.CtrlKey)
		_ = obj.Set("shiftKey", de.ShiftKey)
		_ = obj.Set("metaKey", de.MetaKey)
		_ = obj.Set("dataTransfer", r.wrapDataTransfer(de.DataTransfer))
	}

	return obj
}

func (r *Runtime) ensureDataTransferItemOrder(dt *dom.DataTransfer) []dataTransferItemRef {
	if dt == nil || dt.StoreIdentity() == nil {
		return nil
	}
	store := dt.StoreIdentity()
	if order, ok := r.dataTransferItemOrder[store]; ok {
		return order
	}
	order := make([]dataTransferItemRef, 0, len(r.dataTransferFilesByStore[store])+len(dt.Types()))
	files := r.dataTransferFilesByStore[store]
	ids := r.dataTransferFileIDsByStore[store]
	for i, file := range files {
		if i < len(ids) {
			order = append(order, dataTransferItemRef{kind: "file", id: ids[i], file: file})
		}
	}
	for _, typeValue := range dt.Types() {
		if id, ok := dt.ItemID(typeValue); ok {
			order = append(order, dataTransferItemRef{kind: "string", typeValue: typeValue, id: id})
		}
	}
	r.dataTransferItemOrder[store] = order
	return order
}

func canonicalDataTransferFormat(format string) string {
	switch strings.ToLower(strings.Trim(format, "\t\n\f\r ")) {
	case "text":
		return "text/plain"
	case "url":
		return "text/uri-list"
	default:
		return strings.ToLower(format)
	}
}

func (r *Runtime) dataTransferTypesValue(dt *dom.DataTransfer, obj *goja.Object) *goja.Object {
	order := r.ensureDataTransferItemOrder(dt)
	var signature strings.Builder
	if dt.Mode() == dom.DataTransferDisabled {
		signature.WriteString("disabled")
	} else {
		for _, item := range order {
			if item.kind == "file" {
				signature.WriteString("f:")
				signature.WriteString(strconv.FormatUint(item.id, 10))
			} else {
				signature.WriteString("s:")
				signature.WriteString(strconv.FormatUint(item.id, 10))
			}
			signature.WriteByte(';')
		}
	}
	key := signature.String()
	if cached := r.dataTransferTypesCache[obj]; cached.value != nil && cached.signature == key {
		return cached.value
	}
	array := r.vm.NewArray()
	if dt.Mode() != dom.DataTransferDisabled {
		for i, typeValue := range dt.Types() {
			_ = array.Set(strconv.Itoa(i), typeValue)
		}
		if len(r.dataTransferFiles[obj]) > 0 {
			_ = array.Set(strconv.Itoa(len(dt.Types())), "Files")
		}
	}
	r.dataTransferTypesCache[obj] = dataTransferTypesCacheEntry{signature: key, value: array}
	return array
}

func (r *Runtime) replaceDataTransferStringOrder(dt *dom.DataTransfer, typeValue string) {
	r.removeDataTransferStringOrder(dt, typeValue)
	if id, ok := dt.ItemID(typeValue); ok {
		store := dt.StoreIdentity()
		order := r.ensureDataTransferItemOrder(dt)
		r.dataTransferItemOrder[store] = append(order, dataTransferItemRef{kind: "string", typeValue: typeValue, id: id})
	}
}

// An empty type removes all string items. File entries remain until items.clear
// or their individual list index is removed.
func (r *Runtime) removeDataTransferStringOrder(dt *dom.DataTransfer, typeValue string, removeAll ...bool) {
	if dt == nil || dt.StoreIdentity() == nil {
		return
	}
	store := dt.StoreIdentity()
	order := r.ensureDataTransferItemOrder(dt)
	all := len(removeAll) > 0 && removeAll[0]
	kept := order[:0]
	for _, item := range order {
		if item.kind == "string" && (all || item.typeValue == typeValue) {
			continue
		}
		kept = append(kept, item)
	}
	r.dataTransferItemOrder[store] = kept
}

func (r *Runtime) syncDataTransferFileOrder(dt *dom.DataTransfer, files []*goja.Object, ids []uint64) {
	if dt == nil || dt.StoreIdentity() == nil {
		return
	}
	store := dt.StoreIdentity()
	order := r.ensureDataTransferItemOrder(dt)
	newIDs := make(map[uint64]*goja.Object, len(ids))
	for i, id := range ids {
		if i < len(files) {
			newIDs[id] = files[i]
		}
	}
	kept := order[:0]
	seen := make(map[uint64]bool, len(ids))
	for _, item := range order {
		if item.kind != "file" {
			kept = append(kept, item)
			continue
		}
		file, ok := newIDs[item.id]
		if ok {
			item.file = file
			kept = append(kept, item)
			seen[item.id] = true
		}
	}
	for i, id := range ids {
		if i < len(files) && !seen[id] {
			kept = append(kept, dataTransferItemRef{kind: "file", id: id, file: files[i]})
		}
	}
	r.dataTransferItemOrder[store] = kept
}

func (r *Runtime) dataTransferItemObject(dt *dom.DataTransfer, item dataTransferItemRef) *goja.Object {
	if item.kind == "file" {
		return r.dataTransferFileItem(dt, item.file, item.id)
	}
	return r.dataTransferStringItem(dt, item.typeValue)
}

func (r *Runtime) refreshDataTransferItemIndices(dt *dom.DataTransfer, items *goja.Object) {
	if dt == nil || items == nil {
		return
	}
	order := r.ensureDataTransferItemOrder(dt)
	oldCount := r.dataTransferItemIndexCount[items]
	for index, item := range order {
		_ = items.DefineDataProperty(strconv.Itoa(index), r.dataTransferItemObject(dt, item), goja.FLAG_FALSE, goja.FLAG_TRUE, goja.FLAG_TRUE)
	}
	for index := len(order); index < oldCount; index++ {
		items.Delete(strconv.Itoa(index))
	}
	r.dataTransferItemIndexCount[items] = len(order)
}

func (r *Runtime) refreshDataTransferItemIndicesForStore(store *dom.DataTransferStore) {
	if store == nil {
		return
	}
	for dt, obj := range r.dtWrappers {
		if dt.StoreIdentity() != store {
			continue
		}
		items, _ := obj.Get("items").(*goja.Object)
		r.refreshDataTransferItemIndices(dt, items)
	}
}

func (r *Runtime) wrapDataTransfer(dt *dom.DataTransfer) *goja.Object {
	if dt == nil {
		dt = dom.NewDataTransfer()
	}
	if r.dtWrappers == nil {
		r.dtWrappers = make(map[*dom.DataTransfer]*goja.Object)
	}
	if obj, ok := r.dtWrappers[dt]; ok {
		return obj
	}
	if len(r.dtWrappers) >= maxDataTransferWrappers {
		r.dtWrappers = make(map[*dom.DataTransfer]*goja.Object)
	}
	obj := r.vm.NewObject()
	if r.dtProto != nil {
		_ = obj.SetPrototype(r.dtProto)
	}
	r.dtWrappers[dt] = obj
	r.dataTransferFiles[obj] = append([]*goja.Object(nil), r.dataTransferFilesByStore[dt.StoreIdentity()]...)
	r.dataTransferFileIDs[obj] = append([]uint64(nil), r.dataTransferFileIDsByStore[dt.StoreIdentity()]...)
	r.ensureDataTransferItemOrder(dt)
	r.dataTransferItems[obj] = make(map[string]*goja.Object)
	r.dataTransferFileModes[obj] = dt.Mode()
	r.installDataTransferFiles(obj, dt)
	items, _ := obj.Get("items").(*goja.Object)

	_ = obj.Set("setData", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 2 && dt.Mode() == dom.DataTransferReadWrite {
			format := call.Arguments[0].String()
			dt.SetData(format, call.Arguments[1].String())
			if normalized := canonicalDataTransferFormat(format); normalized != "" {
				r.replaceDataTransferStringOrder(dt, normalized)
			}
			r.refreshDataTransferItemIndices(dt, items)
		}
		return goja.Undefined()
	})
	_ = obj.Set("getData", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) >= 1 {
			return r.vm.ToValue(dt.GetData(call.Arguments[0].String()))
		}
		return r.vm.ToValue("")
	})
	_ = obj.Set("clearData", func(call goja.FunctionCall) goja.Value {
		if dt.Mode() != dom.DataTransferReadWrite {
			return goja.Undefined()
		}
		if len(call.Arguments) >= 1 {
			format := call.Arguments[0].String()
			dt.ClearData(format)
			r.removeDataTransferStringOrder(dt, canonicalDataTransferFormat(format))
		} else {
			dt.ClearData()
			r.removeDataTransferStringOrder(dt, "", true)
		}
		r.refreshDataTransferItemIndices(dt, items)
		return goja.Undefined()
	})
	// setDragImage is accepted for compatibility; the headless engine draws no
	// drag image, so the call is a no-op and never throws.
	_ = obj.Set("setDragImage", func(call goja.FunctionCall) goja.Value {
		return goja.Undefined()
	})

	// Internal store readers the accessors below call back into. __getTypes__
	// builds a native JS array: Object.freeze cannot freeze a host-backed
	// goja array, and the types getter returns a frozen array.
	_ = obj.Set("__getTypes__", func() goja.Value { return r.dataTransferTypesValue(dt, obj) })
	_ = obj.Set("__getDropEffect__", func() string { return dt.DropEffect() })
	_ = obj.Set("__setDropEffect__", func(v string) { dt.SetDropEffect(v) })
	_ = obj.Set("__getEffectAllowed__", func() string { return dt.EffectAllowed() })
	_ = obj.Set("__setEffectAllowed__", func(v string) { dt.SetEffectAllowed(v) })
	_ = obj.Set("__getItemCount__", func() int {
		if dt.Mode() == dom.DataTransferDisabled {
			return 0
		}
		return len(r.ensureDataTransferItemOrder(dt))
	})

	// Accessor properties must be re-defined per wrapper: a plain Set would
	// snapshot the value, while dropEffect/effectAllowed/types have to read
	// and write the live Go store.
	_ = r.vm.Set("__dtTarget__", obj)
	_ = r.vm.Set("__dtItems__", items)
	_, _ = r.vm.RunString(`
		(function() {
			var o = __dtTarget__;
			var it = __dtItems__;
			Object.defineProperty(o, 'dropEffect', {
				get: function() { return o.__getDropEffect__(); },
				set: function(v) { o.__setDropEffect__(String(v)); },
				configurable: true,
				enumerable: true
			});
			Object.defineProperty(o, 'effectAllowed', {
				get: function() { return o.__getEffectAllowed__(); },
				set: function(v) { o.__setEffectAllowed__(String(v)); },
				configurable: true,
				enumerable: true
			});
			Object.defineProperty(o, 'types', {
				get: function() { return Object.freeze(o.__getTypes__()); },
				configurable: true,
				enumerable: true
			});
		})();
	`)
	_ = r.vm.Set("__dtTarget__", nil)
	_ = r.vm.Set("__dtItems__", nil)
	r.refreshDataTransferItemIndices(dt, items)
	return obj
}

func (r *Runtime) dataTransferStringItem(dt *dom.DataTransfer, format string) *goja.Object {
	id, ok := dt.ItemID(format)
	if !ok {
		return r.vm.NewObject()
	}
	cache := r.dataTransferItems[r.dtWrappers[dt]]
	if cache == nil {
		cache = make(map[string]*goja.Object)
		r.dataTransferItems[r.dtWrappers[dt]] = cache
	}
	key := "string:" + strconv.FormatUint(id, 10)
	if item := cache[key]; item != nil {
		return item
	}
	entry := r.vm.NewObject()
	if r.dataTransferItemProto != nil {
		_ = entry.SetPrototype(r.dataTransferItemProto)
	}
	_ = entry.DefineAccessorProperty("kind", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if dt.Mode() == dom.DataTransferDisabled || !dt.HasItemID(format, id) {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue("string")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = entry.DefineAccessorProperty("type", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if dt.Mode() == dom.DataTransferDisabled || !dt.HasItemID(format, id) {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue(format)
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = entry.Set("getAsFile", func(goja.FunctionCall) goja.Value { return goja.Null() })
	_ = entry.Set("getAsString", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || goja.IsNull(call.Arguments[0]) || goja.IsUndefined(call.Arguments[0]) {
			return goja.Undefined()
		}
		if (dt.Mode() != dom.DataTransferReadWrite && dt.Mode() != dom.DataTransferReadOnly) || !dt.HasItemID(format, id) {
			return goja.Undefined()
		}
		callback, ok := goja.AssertFunction(call.Arguments[0])
		if !ok {
			return goja.Undefined()
		}
		data := dt.GetData(format)
		r.QueueTask(func() { _, _ = callback(goja.Undefined(), r.vm.ToValue(data)) })
		return goja.Undefined()
	})
	cache[key] = entry
	return entry
}

func (r *Runtime) dataTransferFileItem(dt *dom.DataTransfer, file *goja.Object, id uint64) *goja.Object {
	cache := r.dataTransferItems[r.dtWrappers[dt]]
	if cache == nil {
		cache = make(map[string]*goja.Object)
		r.dataTransferItems[r.dtWrappers[dt]] = cache
	}
	key := "file:" + strconv.FormatUint(id, 10)
	if item := cache[key]; item != nil {
		return item
	}
	entry := r.vm.NewObject()
	if r.dataTransferItemProto != nil {
		_ = entry.SetPrototype(r.dataTransferItemProto)
	}
	containsFile := func() bool {
		files := r.dataTransferFilesByStore[dt.StoreIdentity()]
		ids := r.dataTransferFileIDsByStore[dt.StoreIdentity()]
		for i, candidate := range files {
			if candidate == file && i < len(ids) && ids[i] == id {
				return true
			}
		}
		return false
	}
	_ = entry.DefineAccessorProperty("kind", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if dt.Mode() == dom.DataTransferDisabled || !containsFile() {
			return r.vm.ToValue("")
		}
		return r.vm.ToValue("file")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = entry.DefineAccessorProperty("type", r.vm.ToValue(func(goja.FunctionCall) goja.Value {
		if dt.Mode() == dom.DataTransferDisabled || !containsFile() {
			return r.vm.ToValue("")
		}
		if value := r.fileValues[file]; value != nil {
			return r.vm.ToValue(value.typeValue)
		}
		return r.vm.ToValue("")
	}), goja.Undefined(), goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = entry.Set("getAsString", func(goja.FunctionCall) goja.Value { return goja.Undefined() })
	_ = entry.Set("getAsFile", func(goja.FunctionCall) goja.Value {
		if (dt.Mode() != dom.DataTransferReadWrite && dt.Mode() != dom.DataTransferReadOnly) || !containsFile() {
			return goja.Null()
		}
		value := r.fileValues[file]
		if value == nil || !value.isFile {
			return goja.Null()
		}
		return r.makeBlob(r.vm.NewObject(), value.data, value.typeValue, true, value.name, value.lastModified)
	})
	cache[key] = entry
	return entry
}

func (r *Runtime) setupDragDrop() {
	itemProto := r.vm.NewObject()
	r.dataTransferItemProto = itemProto
	itemCtor := func(call goja.ConstructorCall) *goja.Object {
		panic(r.vm.NewTypeError("Illegal constructor"))
	}
	itemCtorVal := r.vm.ToValue(itemCtor)
	_ = itemCtorVal.(*goja.Object).DefineDataProperty("prototype", itemProto, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
	_ = itemProto.DefineDataProperty("constructor", itemCtorVal, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	r.defineInterfaceToStringTag("DataTransferItem", itemProto)
	_ = r.vm.Set("DataTransferItem", itemCtorVal)
	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("DataTransferItem", itemCtorVal)
		}
	}

	itemsProto := r.vm.NewObject()
	r.dataTransferItemListProto = itemsProto
	itemListCtor := func(call goja.ConstructorCall) *goja.Object {
		panic(r.vm.NewTypeError("Illegal constructor"))
	}
	itemListCtorVal := r.vm.ToValue(itemListCtor)
	itemListCtorObj := itemListCtorVal.(*goja.Object)
	_ = itemListCtorObj.DefineDataProperty("prototype", itemsProto, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
	_ = itemsProto.DefineDataProperty("constructor", itemListCtorVal, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = itemsProto.DefineDataProperty("add", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.dataTransferItemListAdd(call)
	}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = itemsProto.DefineDataProperty("remove", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.dataTransferItemListRemove(call)
	}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = itemsProto.DefineDataProperty("clear", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		return r.dataTransferItemListClear(call)
	}), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	_ = itemsProto.DefineAccessorProperty("length", r.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		binding, ok := r.dataTransferItemLists[r.asObject(call.This)]
		if !ok || binding.dt == nil || binding.dt.Mode() == dom.DataTransferDisabled {
			return r.vm.ToValue(0)
		}
		return r.vm.ToValue(len(r.ensureDataTransferItemOrder(binding.dt)))
	}), goja.Undefined(), goja.FLAG_FALSE, goja.FLAG_FALSE)
	r.defineInterfaceToStringTag("DataTransferItemList", itemsProto)
	_ = r.vm.Set("DataTransferItemList", itemListCtorVal)

	dtProto := r.vm.NewObject()
	r.dtProto = dtProto
	dtCtor := func(call goja.ConstructorCall) *goja.Object {
		dt := dom.NewDataTransfer()
		return r.wrapDataTransfer(dt)
	}
	dtVal := r.vm.ToValue(dtCtor)
	dtCtorObj := dtVal.(*goja.Object)
	_ = dtCtorObj.DefineDataProperty("prototype", dtProto, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
	_ = dtProto.DefineDataProperty("constructor", dtVal, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE)
	r.defineInterfaceToStringTag("DataTransfer", dtProto)
	_ = r.vm.Set("DataTransfer", dtVal)

	dragEventCtor := func(call goja.ConstructorCall) *goja.Object {
		eventType := ""
		if len(call.Arguments) > 0 {
			eventType = call.Arguments[0].String()
		}
		dt := dom.NewDataTransfer()
		de := dom.NewDragEvent(eventType, dom.MouseEvent{}, dt)
		return r.wrapEvent(de).(*goja.Object)
	}
	deVal := r.vm.ToValue(dragEventCtor)
	_ = r.vm.Set("DragEvent", deVal)

	if win := r.vm.Get("window"); win != nil {
		if winObj, ok := win.(*goja.Object); ok {
			_ = winObj.Set("DataTransferItemList", itemListCtorVal)
			_ = winObj.Set("DataTransfer", dtVal)
			_ = winObj.Set("DragEvent", deVal)
		}
	}
}

func (r *Runtime) dataTransferItemListAdd(call goja.FunctionCall) goja.Value {
	items := r.asObject(call.This)
	binding, ok := r.dataTransferItemLists[items]
	if !ok || binding.dt == nil {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'add': 1 argument required."))
	}
	dt, obj := binding.dt, binding.transfer
	if dt.Mode() != dom.DataTransferReadWrite {
		return goja.Null()
	}
	file, isObject := call.Argument(0).(*goja.Object)
	if isObject && r.fileValues[file] != nil && r.fileValues[file].isFile {
		files := append(append([]*goja.Object(nil), r.dataTransferFiles[obj]...), file)
		r.updateDataTransferFiles(obj, dt, files)
		if list := r.dataTransferFileLists[obj]; list != nil {
			r.setFileListFiles(list, r.dataTransferFiles[obj])
		}
		return r.dataTransferItemObject(dt, r.ensureDataTransferItemOrder(dt)[len(r.ensureDataTransferItemOrder(dt))-1])
	}
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'add': 2 arguments required for string data."))
	}
	typeValue := strings.ToLower(call.Argument(1).String())
	if dt.HasTypeString(typeValue) {
		panic(r.domError("NotSupportedError", "An item with this type is already present in the drag data store."))
	}
	if !dt.AddStringItem(typeValue, call.Argument(0).String()) {
		return goja.Null()
	}
	r.replaceDataTransferStringOrder(dt, typeValue)
	r.refreshDataTransferItemIndices(dt, items)
	return r.dataTransferStringItem(dt, typeValue)
}

func (r *Runtime) dataTransferItemListRemove(call goja.FunctionCall) goja.Value {
	items := r.asObject(call.This)
	binding, ok := r.dataTransferItemLists[items]
	if !ok || binding.dt == nil {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	if len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'remove': 1 argument required."))
	}
	dt, obj := binding.dt, binding.transfer
	if dt.Mode() != dom.DataTransferReadWrite {
		panic(r.domError("InvalidStateError", "The DataTransferItemList is not in read/write mode."))
	}
	i := int(call.Argument(0).ToInteger())
	order := r.ensureDataTransferItemOrder(dt)
	if i < 0 || i >= len(order) {
		return goja.Undefined()
	}
	item := order[i]
	if item.kind == "file" {
		files := make([]*goja.Object, 0, len(r.dataTransferFiles[obj])-1)
		for j, id := range r.dataTransferFileIDs[obj] {
			if id != item.id && j < len(r.dataTransferFiles[obj]) {
				files = append(files, r.dataTransferFiles[obj][j])
			}
		}
		r.updateDataTransferFiles(obj, dt, files)
		if list := r.dataTransferFileLists[obj]; list != nil {
			r.setFileListFiles(list, r.dataTransferFiles[obj])
		}
	} else {
		dt.RemoveStringItem(item.typeValue)
		r.removeDataTransferStringOrder(dt, item.typeValue)
	}
	items = binding.transfer.Get("items").(*goja.Object)
	r.refreshDataTransferItemIndices(dt, items)
	return goja.Undefined()
}

func (r *Runtime) dataTransferItemListClear(call goja.FunctionCall) goja.Value {
	items := r.asObject(call.This)
	binding, ok := r.dataTransferItemLists[items]
	if !ok || binding.dt == nil {
		panic(r.vm.NewTypeError("Illegal invocation"))
	}
	dt, obj := binding.dt, binding.transfer
	if dt.Mode() != dom.DataTransferReadWrite {
		return goja.Undefined()
	}
	r.updateDataTransferFiles(obj, dt, nil)
	if list := r.dataTransferFileLists[obj]; list != nil {
		r.setFileListFiles(list, nil)
	}
	dt.ClearData()
	r.removeDataTransferStringOrder(dt, "", true)
	items = binding.transfer.Get("items").(*goja.Object)
	r.refreshDataTransferItemIndices(dt, items)
	return goja.Undefined()
}

// ---------------------------------------------------------------------------
// JS bridge: attribute methods
// ---------------------------------------------------------------------------

// jsGetAttribute implements element.getAttribute(name).
func (r *Runtime) jsGetAttribute(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 1 {
		return goja.Null()
	}
	name := call.Arguments[0].String()
	if !node.HasAttribute(name) {
		return goja.Null()
	}
	return r.vm.ToValue(node.GetAttribute(name))
}

func (r *Runtime) jsGetAttributeNS(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'getAttributeNS': 2 arguments required"))
	}
	namespace := ""
	if value := call.Arguments[0]; value != nil && !goja.IsNull(value) && !goja.IsUndefined(value) {
		namespace = value.String()
	}
	localName := call.Arguments[1].String()
	for _, attr := range node.Attr {
		if attr.NamespaceURI == namespace && domAttributeLocalName(attr.Name) == localName {
			return r.vm.ToValue(attr.Value)
		}
	}
	return goja.Null()
}

func domAttributeLocalName(name string) string {
	if colon := strings.IndexByte(name, ':'); colon >= 0 {
		return name[colon+1:]
	}
	return name
}

// jsSetAttribute implements element.setAttribute(name, value).
func (r *Runtime) jsSetAttribute(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	name := call.Arguments[0].String()
	value := call.Arguments[1].String()
	before, hadValue := node.GetAttribute(name), node.HasAttribute(name)
	beforeDetailsOpen := node.Data == "details" && strings.EqualFold(name, "open") && node.HasAttribute("open")
	r.preparePopoverAttributeChange(node, name, value, true)
	node.SetAttribute(name, value)
	if node.Data == "details" && strings.EqualFold(name, "open") {
		r.detailsOpenChanged(node, beforeDetailsOpen, node.HasAttribute("open"), nil)
	} else if node.Data == "details" && strings.EqualFold(name, "name") {
		r.detailsNameChanged(node)
	}
	r.RefreshInputPatternMismatch(node)
	if !hadValue || before != value {
		callbackName := name
		if node.Namespace == dom.NSHTML {
			callbackName = strings.ToLower(name)
		}
		r.notifyCustomElementAttributeChanged(node, callbackName, before, hadValue, value, true, "")
	}
	if strings.HasPrefix(strings.ToLower(name), "on") {
		r.syncEventHandlerContentAttribute(node, name, value, true)
	}
	if node.Data == "input" && (strings.EqualFold(name, "src") || strings.EqualFold(name, "type")) && node.IsConnected() {
		r.maybeLoadImageInput(node, true)
	}
	if strings.EqualFold(name, "id") && value != "" {
		r.installWindowNamedProperty(value)
	}
	if !hadValue || before != value || node.Data == "details" && strings.EqualFold(name, "open") {
		r.notifyMutation()
	}
	var old *string
	if hadValue {
		old = &before
	}
	attrName := domAttributeLocalName(name)
	if node.Namespace == dom.NSHTML {
		attrName = strings.ToLower(attrName)
	}
	r.notifyAttributeMutation(node, "", attrName, old)
	return goja.Undefined()
}

// jsSetAttributeNS implements element.setAttributeNS(ns, name, value). A null
// namespace behaves exactly like setAttribute. The XML namespace stores the
// qualified name marked NSXML, which is the only xml:lang :lang() honors -
// the HTML parser never creates namespaced attributes, so static xml:lang
// stays invisible to language matching exactly like the reference engine.
func (r *Runtime) jsSetAttributeNS(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 3 {
		return goja.Undefined()
	}
	var ns string
	if nsArg := call.Arguments[0]; nsArg != nil && !goja.IsNull(nsArg) && !goja.IsUndefined(nsArg) {
		ns = nsArg.String()
	}
	name := call.Arguments[1].String()
	value := call.Arguments[2].String()
	before, hadAttribute := node.GetAttributeNS(ns, name)
	beforeOpen := node.HasAttribute(name) && node.GetAttribute(name) != ""
	beforeDetailsOpen := ns == "" && node.Data == "details" && strings.EqualFold(name, "open") && node.HasAttribute("open")
	changed := node.SetAttributeNS(ns, name, value)
	if ns == "" && node.Data == "details" && strings.EqualFold(name, "open") {
		r.detailsOpenChanged(node, beforeDetailsOpen, node.HasAttribute("open"), nil)
	} else if ns == "" && node.Data == "details" && strings.EqualFold(name, "name") {
		r.detailsNameChanged(node)
	}
	r.RefreshInputPatternMismatch(node)
	if changed {
		localName := name
		if colon := strings.IndexByte(localName, ':'); colon >= 0 {
			localName = localName[colon+1:]
		}
		r.notifyCustomElementAttributeChanged(node, localName, before, hadAttribute, value, true, ns)
	}
	if ns == "" && strings.HasPrefix(strings.ToLower(name), "on") {
		r.syncEventHandlerContentAttribute(node, name, value, true)
	}
	if ns == "" && node.Data == "details" && strings.EqualFold(name, "open") && beforeOpen != node.HasAttribute(name) {
		changed = true
	}
	var old *string
	if hadAttribute {
		old = &before
	}
	r.notifyAttributeMutation(node, ns, domAttributeLocalName(name), old)
	if changed {
		r.notifyMutation()
	}
	return goja.Undefined()
}

// jsRemoveAttribute implements element.removeAttribute(name).
func (r *Runtime) jsRemoveAttribute(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 1 {
		return goja.Undefined()
	}
	name := call.Arguments[0].String()
	lower := strings.ToLower(name)
	changed := node.HasAttribute(name)
	if !changed {
		return goja.Undefined()
	}
	oldValue := node.GetAttribute(name)
	beforeDetailsOpen := node.Data == "details" && strings.EqualFold(name, "open") && node.HasAttribute("open")
	r.preparePopoverAttributeChange(node, name, "", false)
	node.RemoveAttribute(name)
	if node.Data == "details" && strings.EqualFold(name, "open") {
		r.detailsOpenChanged(node, beforeDetailsOpen, node.HasAttribute("open"), nil)
	} else if node.Data == "details" && strings.EqualFold(name, "name") {
		r.detailsNameChanged(node)
	}
	r.RefreshInputPatternMismatch(node)
	r.notifyCustomElementAttributeChanged(node, lower, oldValue, true, "", false, "")
	old := oldValue
	r.notifyAttributeMutation(node, "", domAttributeLocalName(name), &old)
	if strings.HasPrefix(lower, "on") {
		r.syncEventHandlerContentAttribute(node, name, "", false)
	}
	r.notifyMutation()
	return goja.Undefined()
}

func (r *Runtime) jsRemoveAttributeNS(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 2 {
		return goja.Undefined()
	}
	namespace := ""
	if value := call.Arguments[0]; value != nil && !goja.IsNull(value) && !goja.IsUndefined(value) {
		namespace = value.String()
	}
	localName := call.Arguments[1].String()
	beforeDetailsOpen := namespace == "" && node.Data == "details" && strings.EqualFold(localName, "open") && node.HasAttribute("open")
	oldValue, hadAttribute := "", false
	for _, attr := range node.Attr {
		name := attr.Name
		if colon := strings.IndexByte(name, ':'); colon >= 0 {
			name = name[colon+1:]
		}
		if attr.NamespaceURI == namespace && strings.EqualFold(name, localName) {
			oldValue, hadAttribute = attr.Value, true
			break
		}
	}
	if node.RemoveAttributeNS(namespace, localName) {
		if namespace == "" && node.Data == "details" && strings.EqualFold(localName, "open") {
			r.detailsOpenChanged(node, beforeDetailsOpen, node.HasAttribute("open"), nil)
		} else if namespace == "" && node.Data == "details" && strings.EqualFold(localName, "name") {
			r.detailsNameChanged(node)
		}
		r.RefreshInputPatternMismatch(node)
		r.notifyCustomElementAttributeChanged(node, localName, oldValue, hadAttribute, "", false, namespace)
		old := oldValue
		r.notifyAttributeMutation(node, namespace, localName, &old)
		r.notifyMutation()
	}
	return goja.Undefined()
}

func (r *Runtime) jsToggleAttribute(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) == 0 {
		panic(r.vm.NewTypeError("Failed to execute 'toggleAttribute': 1 argument required"))
	}
	name := call.Arguments[0].String()
	has := node.HasAttribute(name)
	beforeDetailsOpen := node.Data == "details" && strings.EqualFold(name, "open") && has
	forcePresent := !has
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) {
		forcePresent = call.Arguments[1].ToBoolean()
	}
	if forcePresent == has {
		return r.vm.ToValue(has)
	}
	callbackName := name
	if node.Namespace == dom.NSHTML {
		callbackName = strings.ToLower(name)
	}
	if forcePresent {
		r.preparePopoverAttributeChange(node, name, "", true)
		node.SetAttribute(name, "")
		r.notifyCustomElementAttributeChanged(node, callbackName, "", false, "", true, "")
		r.notifyAttributeMutation(node, "", domAttributeLocalName(name), nil)
	} else {
		oldValue := node.GetAttribute(name)
		r.preparePopoverAttributeChange(node, name, "", false)
		node.RemoveAttribute(name)
		r.notifyCustomElementAttributeChanged(node, callbackName, oldValue, true, "", false, "")
		r.notifyAttributeMutation(node, "", domAttributeLocalName(name), &oldValue)
	}
	if node.Data == "details" && strings.EqualFold(name, "open") {
		r.detailsOpenChanged(node, beforeDetailsOpen, node.HasAttribute("open"), nil)
	} else if node.Data == "details" && strings.EqualFold(name, "name") {
		r.detailsNameChanged(node)
	}
	r.RefreshInputPatternMismatch(node)
	r.notifyMutation()
	return r.vm.ToValue(forcePresent)
}

// jsHasAttribute implements element.hasAttribute(name).
func (r *Runtime) jsHasAttribute(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || len(call.Arguments) < 1 {
		return r.vm.ToValue(false)
	}
	return r.vm.ToValue(node.HasAttribute(call.Arguments[0].String()))
}

func (r *Runtime) currentDocumentElement() *dom.Node {
	return documentElementOf(r.domDoc)
}

func (r *Runtime) currentDocumentChild(name string) *dom.Node {
	return documentChildOf(r.domDoc, name)
}

func (r *Runtime) currentDocumentBody() *dom.Node {
	return documentBodyOf(r.domDoc)
}

func (r *Runtime) currentDocumentDoctype() *dom.Node {
	return documentDoctypeOf(r.domDoc)
}

// ---------------------------------------------------------------------------
// JS bridge: mutation methods
// ---------------------------------------------------------------------------

// jsRemove implements element.remove(), detaching the node from its parent.
func (r *Runtime) jsRemove(call goja.FunctionCall) goja.Value {
	node := r.getNode(call.This)
	if node == nil || node.Parent == nil {
		return goja.Undefined()
	}
	// Credit only if node was live before detaching; a node under a detached
	// parent was never charged, and crediting it would under-count the budget.
	wasLive := r.nodeIsLive(node)
	parent := node.Parent
	r.preserveCustomElementRegistryForDetachedNode(node)
	r.adjustLiveRangesForNodeRemoval(parent, node, childIndex(parent, node))
	if root := parent.ShadowRoot; root == nil || !root.RemoveChild(node) {
		parent.RemoveChild(node)
	}
	r.markRadioGroupsDirtyForNodes([]*dom.Node{node})
	if wasLive {
		r.releaseNodes([]*dom.Node{node})
	}
	r.notifyMutation()
	r.invokeDisconnectedCallbacks(node)
	return goja.Undefined()
}

// jsGetElementByID implements document.getElementById(id).
func (r *Runtime) jsGetElementByID(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return goja.Null()
	}
	id := call.Arguments[0].String()
	node := r.domDoc.ElementByID(id)
	if node == nil {
		return goja.Null()
	}
	return r.wrapNode(node)
}

// jsGetElementsByClassName implements document.getElementsByClassName(class).
func (r *Runtime) jsGetElementsByClassName(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return r.newLiveHTMLCollection(nil, "", "")
	}
	return r.newLiveHTMLCollection(&r.domDoc.Node, "", call.Arguments[0].String())
}

// jsGetElementsByTagName implements document.getElementsByTagName(tag).
func (r *Runtime) jsGetElementsByTagName(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return r.newLiveHTMLCollection(nil, "", "")
	}
	return r.newLiveHTMLCollection(&r.domDoc.Node, call.Arguments[0].String(), "")
}

// jsCreateElement implements document.createElement(tag).
func (r *Runtime) jsCreateElement(call goja.FunctionCall) goja.Value {
	return r.jsCreateElementInDocument(call, r.domDoc, r.customElementRegistryForDocument(r.domDoc))
}

func (r *Runtime) jsCreateElementInDocument(call goja.FunctionCall, doc *dom.Document, defaultRegistry *CustomElementRegistry) goja.Value {
	if doc == nil || len(call.Arguments) < 1 {
		return goja.Null()
	}
	tag := call.Arguments[0].String()
	if !doc.IsXML {
		tag = strings.ToLower(tag)
	}
	node := doc.NewElement(tag)
	if doc.IsXML {
		if strings.EqualFold(doc.ContentType, "application/xhtml+xml") {
			node.Namespace = dom.NSHTML
			node.NamespaceURI = "http://www.w3.org/1999/xhtml"
		} else {
			node.Namespace = dom.NSOther
			node.NamespaceURI = ""
		}
	}
	if node.Namespace == dom.NSHTML && dom.IsValidCustomElementName(tag) {
		node.CustomElementUndefined = true
	}
	var is string
	customizedBuiltInRequested := false
	registry := defaultRegistry
	explicitNullRegistry := false
	if len(call.Arguments) > 1 {
		if options, ok := call.Arguments[1].(*goja.Object); ok && options != nil {
			if registryValue := options.Get("customElementRegistry"); registryValue != nil && !goja.IsUndefined(registryValue) {
				if goja.IsNull(registryValue) {
					registry = nil
					explicitNullRegistry = true
				} else if registryObject, ok := registryValue.(*goja.Object); ok {
					registry = r.customElementRegistryObjects[registryObject]
					if registry == nil {
						panic(r.vm.NewTypeError("The customElementRegistry option is not a CustomElementRegistry"))
					}
				} else {
					panic(r.vm.NewTypeError("The customElementRegistry option is not a CustomElementRegistry"))
				}
			}
			if isValue := options.Get("is"); isValue != nil && !goja.IsUndefined(isValue) && !goja.IsNull(isValue) {
				customizedBuiltInRequested = true
				is = isValue.String()
			}
		}
	}
	if customizedBuiltInRequested {
		node.CustomElementUndefined = true
	}
	if r.customElementNodeRegistries == nil {
		r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
	}
	r.customElementNodeRegistries[node] = registry
	if explicitNullRegistry {
		r.customElementNullRegistryLocks[node] = true
	}
	r.associateScopedRegistryWithDocument(registry, doc)
	wrapped := r.wrapNode(node)
	if node.Namespace == dom.NSHTML {
		if def := r.customElementDefinitionIn(registry, tag); def != nil && def.extends == "" {
			r.upgradeElementInRegistry(node, def, true, true, registry)
			return wrapped
		}
	}
	if is != "" && node.Namespace == dom.NSHTML {
		if dom.IsValidCustomElementName(is) {
			node.CustomElementUndefined = true
		}
		if r.customElementCandidates == nil {
			r.customElementCandidates = make(map[*dom.Node]string)
		}
		r.customElementCandidates[node] = is
		if def := r.customElementDefinitionIn(registry, is); def != nil && def.extends == tag {
			node.SetAttribute("is", is)
			r.upgradeElementInRegistry(node, def, true, false, registry)
		}
	}
	return wrapped
}

func (r *Runtime) jsCreateElementNS(call goja.FunctionCall) goja.Value {
	return r.jsCreateElementNSInDocument(call, r.domDoc)
}

func (r *Runtime) jsCreateElementNSInDocument(call goja.FunctionCall, doc *dom.Document) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'createElementNS': 2 arguments required"))
	}
	if doc == nil {
		return goja.Null()
	}
	namespace := ""
	if value := call.Argument(0); value != nil && !goja.IsNull(value) && !goja.IsUndefined(value) {
		namespace = value.String()
	}
	qualifiedName := call.Arguments[1].String()
	prefix, localName, exception := validateDOMElementQName(namespace, qualifiedName)
	if exception != "" {
		panic(r.domError(exception, "The namespace and qualified name are not valid for an element"))
	}
	node := doc.NewElement(qualifiedName)
	switch namespace {
	case "http://www.w3.org/1999/xhtml":
		node.Namespace = dom.NSHTML
	case "http://www.w3.org/2000/svg":
		node.Namespace = dom.NSSVG
	case "http://www.w3.org/1998/Math/MathML":
		node.Namespace = dom.NSMathML
	default:
		node.Namespace = dom.NSOther
	}
	node.NamespaceURI = namespace
	if node.Namespace == dom.NSHTML && dom.IsValidCustomElementName(localName) {
		node.CustomElementUndefined = true
	}
	// createElementNS preserves the requested qualified name, including case,
	// even when the document itself uses HTML's createElement lowercasing rule.
	// For an autonomous custom element in the HTML namespace, however, the DOM
	// algorithm runs its constructor before assigning the namespace prefix.
	// Keep the unprefixed local name observable throughout construction, then
	// install the qualified name once upgrade has returned.
	var customDefinition *customElementDefinition
	if namespace == "http://www.w3.org/1999/xhtml" {
		customDefinition = r.customElementDefinition(localName)
	}
	if customDefinition != nil {
		node.Data = localName
		node.Prefix = ""
	} else {
		node.Data = qualifiedName
		node.Prefix = prefix
	}
	node.DataAtom = dom.Lookup(localName)
	registry := r.customElementRegistryForDocument(doc)
	explicitNullRegistry := false
	if len(call.Arguments) > 2 {
		if options, ok := call.Arguments[2].(*goja.Object); ok && options != nil {
			if registryValue := options.Get("customElementRegistry"); registryValue != nil && !goja.IsUndefined(registryValue) {
				if goja.IsNull(registryValue) {
					registry = nil
					explicitNullRegistry = true
				} else if registryObject, ok := registryValue.(*goja.Object); ok {
					registry = r.customElementRegistryObjects[registryObject]
					if registry == nil {
						panic(r.vm.NewTypeError("The customElementRegistry option is not a CustomElementRegistry"))
					}
				} else {
					panic(r.vm.NewTypeError("The customElementRegistry option is not a CustomElementRegistry"))
				}
			}
		}
	}
	if r.customElementNodeRegistries == nil {
		r.customElementNodeRegistries = make(map[*dom.Node]*CustomElementRegistry)
	}
	r.customElementNodeRegistries[node] = registry
	if explicitNullRegistry {
		r.customElementNullRegistryLocks[node] = true
	}
	r.associateScopedRegistryWithDocument(registry, doc)
	if namespace == "http://www.w3.org/1999/xhtml" {
		customDefinition = r.customElementDefinitionIn(registry, localName)
		if customDefinition != nil && customDefinition.extends == "" {
			r.upgradeElementInRegistry(node, customDefinition, true, true, registry)
			node.Data = qualifiedName
			node.Prefix = prefix
			return r.wrapNode(node)
		}
		if len(call.Arguments) > 2 {
			if options, ok := call.Arguments[2].(*goja.Object); ok && options != nil {
				if isValue := options.Get("is"); isValue != nil && !goja.IsUndefined(isValue) && !goja.IsNull(isValue) {
					node.CustomElementUndefined = true
					is := isValue.String()
					if is != "" {
						if r.customElementCandidates == nil {
							r.customElementCandidates = make(map[*dom.Node]string)
						}
						r.customElementCandidates[node] = is
						if def := r.customElementDefinitionIn(registry, is); def != nil && def.extends == localName {
							node.Data = localName
							node.Prefix = ""
							r.upgradeElementInRegistry(node, def, true, false, registry)
							node.Data = qualifiedName
							node.Prefix = prefix
						}
					}
				}
			}
		}
	}
	return r.wrapNode(node)
}

// jsCreateTextNode implements document.createTextNode(text).
func (r *Runtime) jsCreateTextNode(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil || len(call.Arguments) < 1 {
		return goja.Null()
	}
	text := call.Arguments[0].String()
	node := r.domDoc.NewText(text)
	return r.wrapNode(node)
}

// jsCreateComment implements document.createComment(data).
func (r *Runtime) jsCreateComment(call goja.FunctionCall) goja.Value {
	if r.domDoc == nil {
		return goja.Null()
	}
	data := ""
	if len(call.Arguments) > 0 {
		data = call.Arguments[0].String()
	}
	return r.wrapNode(r.domDoc.NewComment(data))
}

func (r *Runtime) jsCreateProcessingInstruction(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) < 2 {
		panic(r.vm.NewTypeError("Failed to execute 'createProcessingInstruction': 2 arguments required"))
	}
	if r.domDoc == nil || !validXMLQName(call.Arguments[0].String()) {
		panic(r.domError("InvalidCharacterError", "The processing instruction target is not a valid XML name"))
	}
	target, data := call.Arguments[0].String(), call.Arguments[1].String()
	if strings.Contains(data, "?>") {
		panic(r.domError("InvalidCharacterError", "Processing instruction data must not contain ?>"))
	}
	return r.wrapNode(r.domDoc.NewProcessingInstruction(target, data))
}

// ---------------------------------------------------------------------------
// HTML serialization (for innerHTML getter)
// ---------------------------------------------------------------------------

// voidElements lists the HTML elements that have no closing tag.
var voidElements = map[string]bool{
	"br": true, "hr": true, "img": true, "input": true,
	"meta": true, "link": true, "area": true, "base": true,
	"col": true, "embed": true, "param": true, "source": true,
	"track": true, "wbr": true,
}

// serializeChildren returns the HTML serialization of node's children.
func (r *Runtime) serializeChildren(node *dom.Node) string {
	var sb strings.Builder
	r.serializeChildrenSeen(&sb, node, make(map[*dom.Node]struct{}))
	return sb.String()
}

func (r *Runtime) serializeChildrenSeen(sb *strings.Builder, node *dom.Node, seen map[*dom.Node]struct{}) {
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		if _, exists := seen[child]; exists {
			break
		}
		if child.Type == dom.NodeText && rawTextSerializationElement(node) {
			sb.WriteString(child.DataContent)
		} else {
			r.serializeNodeSeen(sb, child, seen)
		}
		child = next
	}
}

func rawTextSerializationElement(node *dom.Node) bool {
	if node == nil || !node.Element() || node.Namespace != dom.NSHTML {
		return false
	}
	switch strings.ToLower(node.Data) {
	case "noscript":
		return !insideTemplateContents(node)
	case "iframe", "noembed", "noframes", "plaintext", "script", "style", "xmp":
		return true
	default:
		return false
	}
}

func insideTemplateContents(node *dom.Node) bool {
	for current := node; current != nil; current = current.Parent {
		if current.Type == dom.NodeDocumentFragment && current.TemplateHost != nil {
			return true
		}
	}
	return false
}

// serializeNode writes the HTML serialization of a single node to sb.
func (r *Runtime) serializeNode(sb *strings.Builder, n *dom.Node) {
	r.serializeNodeSeen(sb, n, make(map[*dom.Node]struct{}))
}

func (r *Runtime) serializeNodeSeen(sb *strings.Builder, n *dom.Node, seen map[*dom.Node]struct{}) {
	if n == nil {
		return
	}
	if _, exists := seen[n]; exists {
		return
	}
	seen[n] = struct{}{}
	switch n.Type {
	case dom.NodeText:
		sb.WriteString(escapeHTML(n.DataContent))
	case dom.NodeElement:
		tag := strings.ToLower(n.Data)
		sb.WriteString("<")
		sb.WriteString(tag)
		if !n.HasAttribute("is") && r.customElementCandidates != nil {
			if is := r.customElementCandidates[n]; is != "" {
				sb.WriteString(` is="`)
				sb.WriteString(escapeAttr(is))
				sb.WriteString(`"`)
			}
		}
		for _, a := range n.Attr {
			sb.WriteString(" ")
			sb.WriteString(a.Name)
			sb.WriteString(`="`)
			sb.WriteString(escapeAttr(a.Value))
			sb.WriteString(`"`)
		}
		if voidElements[tag] {
			sb.WriteString(">")
			return
		}
		sb.WriteString(">")
		children := n
		if n.IsHTMLTemplate() {
			children = n.TemplateContent()
		}
		r.serializeChildrenSeen(sb, children, seen)
		sb.WriteString("</")
		sb.WriteString(tag)
		sb.WriteString(">")
	case dom.NodeComment:
		sb.WriteString("<!--")
		sb.WriteString(n.DataContent)
		sb.WriteString("-->")
	}
}

// escapeHTML escapes special HTML characters in text content.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\u00a0", "&nbsp;")
	return s
}

// escapeAttr escapes special characters in attribute values.
func escapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// ---------------------------------------------------------------------------
// Mutation notification
// ---------------------------------------------------------------------------

// notifyMutation calls the OnMutation callback if one was provided. It is
// invoked after every DOM mutation from JS so the engine can re-style and
// re-layout.
func (r *Runtime) notifyMutation() {
	r.reconcileDirtyRadioGroups()
	// DOM mutation primitives synchronize clean textarea values before they
	// notify the runtime. Clamp any script-observed selections now so live
	// offsets stay valid across child-text and CharacterData changes too.
	r.clampTrackedTextControlSelections()
	r.mu.Lock()
	r.mutations++
	r.mu.Unlock()
	if r.opts.OnMutation != nil {
		r.opts.OnMutation()
	}
}

// ChainOnMutation appends fn to the mutation callback chain instead of
// replacing it: the engine uses this to observe DOM writes on a runtime the
// caller constructed (and may have given its own OnMutation). A nil fn is
// ignored. Safe for concurrent use; callbacks fire on the mutating thread.
func (r *Runtime) ChainOnMutation(fn func()) {
	if fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if prev := r.opts.OnMutation; prev != nil {
		r.opts.OnMutation = func() { prev(); fn() }
	} else {
		r.opts.OnMutation = fn
	}
}

// ChainOnFocusChange appends fn to the focus callback chain. The engine
// installs its Session binding after parsing the document but before scripts
// run, while callers can still supply their own observer in Options.
func (r *Runtime) ChainOnFocusChange(fn func(*dom.Node, bool)) {
	if r == nil || fn == nil {
		return
	}
	if prev := r.opts.OnFocusChange; prev != nil {
		r.opts.OnFocusChange = func(node *dom.Node, focused bool) {
			prev(node, focused)
			fn(node, focused)
		}
	} else {
		r.opts.OnFocusChange = fn
	}
}

// ChainOnTestDriverKeyDefault installs the host keyboard default used by
// testdriver actions after their key events have been dispatched.
func (r *Runtime) ChainOnTestDriverKeyDefault(fn func(key string, shift, ctrl, alt, meta bool)) {
	if r == nil || fn == nil {
		return
	}
	previous := r.opts.OnTestDriverKeyDefault
	if previous != nil {
		r.opts.OnTestDriverKeyDefault = func(key string, shift, ctrl, alt, meta bool) {
			previous(key, shift, ctrl, alt, meta)
			fn(key, shift, ctrl, alt, meta)
		}
	} else {
		r.opts.OnTestDriverKeyDefault = fn
	}
}

// ChainOnFocusRestore appends fn to the callback chain used when a dialog
// restores focus. The host callback must preserve the current scroll offset.
func (r *Runtime) ChainOnFocusRestore(fn func(*dom.Node)) {
	if r == nil || fn == nil {
		return
	}
	previous := r.opts.OnFocusRestore
	if previous != nil {
		r.opts.OnFocusRestore = func(node *dom.Node) {
			previous(node)
			fn(node)
		}
	} else {
		r.opts.OnFocusRestore = fn
	}
}

// ChainOnUserInput appends fn to the host callback chain for an input adapter
// that simulates a user edit. A nil callback is ignored.
func (r *Runtime) ChainOnUserInput(fn func(*dom.Node) bool) {
	if r == nil || fn == nil {
		return
	}
	previous := r.opts.OnUserInput
	if previous != nil {
		r.opts.OnUserInput = func(node *dom.Node) bool {
			previousResult := previous(node)
			return fn(node) || previousResult
		}
	} else {
		r.opts.OnUserInput = fn
	}
}

// ChainActiveElementProvider installs a host-owned source of truth for
// document.activeElement while retaining any caller provider as a fallback.
func (r *Runtime) ChainActiveElementProvider(fn func() *dom.Node) {
	if r == nil || fn == nil {
		return
	}
	previous := r.opts.ActiveElement
	r.opts.ActiveElement = func() *dom.Node {
		if active := fn(); active != nil {
			return active
		}
		if previous != nil {
			return previous()
		}
		return nil
	}
}
