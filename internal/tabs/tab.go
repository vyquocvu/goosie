package tabs

import (
	"context"
	"sync"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/manifest"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

// NavController tracks per-tab navigation state: cancellation, serial, and loading.
type NavController struct {
	Mu      sync.Mutex
	Cancel  context.CancelFunc
	Serial  uint64
	Loading bool
}

// Tab is one browser tab with fully independent state.
//
// All mutable fields are private to the package: tabs are shared between the
// event pump, the navigation drain and the presentation loop, and bare fields
// let every one of those threads read half-written state. Accessors take the
// mutex; Nav keeps its own (it is driven independently of the rest), History
// keeps its (it is a locked type already), and the ID never changes after
// creation.
type Tab struct {
	mu              sync.RWMutex
	id              uint64
	url             string
	title           string
	history         *toolbar.History
	scrollY         int32
	layer           *frame.Layer
	session         *engine.Session
	loading         bool
	err             string
	bgColor         frame.Color
	incognito       bool
	pinned          bool
	shieldsDisabled bool
	blockedTrackers int
	manifest        *manifest.Manifest
	standaloneApp   bool

	Nav NavController
}

func newTab(id uint64) *Tab {
	return &Tab{
		id:      id,
		title:   "New Tab",
		history: toolbar.NewHistory(),
	}
}

// ShieldsEnabled reports whether ad and tracker blocking shields are active for this tab.
func (t *Tab) ShieldsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return !t.shieldsDisabled
}

// SetShieldsEnabled toggles ad and tracker blocking shields for this tab.
func (t *Tab) SetShieldsEnabled(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.shieldsDisabled = !enabled
}

// BlockedTrackers returns the number of requests blocked by shields on this tab.
func (t *Tab) BlockedTrackers() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.blockedTrackers
}

// IncrementBlockedTrackers increments the blocked request counter.
func (t *Tab) IncrementBlockedTrackers() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blockedTrackers++
}

// ResetBlockedTrackers resets the blocked request counter.
func (t *Tab) ResetBlockedTrackers() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blockedTrackers = 0
}

// Incognito reports whether the tab is running in private incognito mode.
func (t *Tab) Incognito() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.incognito
}

// SetIncognito configures the tab's private incognito mode.
func (t *Tab) SetIncognito(incognito bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.incognito = incognito
}

// ID identifies the tab; it never changes after creation.
func (t *Tab) ID() uint64 { return t.id }

func (t *Tab) URL() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.url
}

func (t *Tab) SetURL(url string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.url = url
}

func (t *Tab) Title() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.title
}

func (t *Tab) SetTitle(title string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.title = title
}

func (t *Tab) History() *toolbar.History {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.history
}

func (t *Tab) SetHistory(h *toolbar.History) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.history = h
}

func (t *Tab) ScrollY() int32 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.scrollY
}

func (t *Tab) SetScrollY(y int32) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scrollY = y
}

func (t *Tab) Layer() *frame.Layer {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.layer
}

func (t *Tab) SetLayer(layer *frame.Layer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.layer = layer
}

func (t *Tab) Session() *engine.Session {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.session
}

func (t *Tab) SetSession(sess *engine.Session) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.session = sess
}

func (t *Tab) Loading() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.loading
}

func (t *Tab) SetLoading(loading bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.loading = loading
}

func (t *Tab) Error() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.err
}

func (t *Tab) SetError(err string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.err = err
}

func (t *Tab) BGColor() frame.Color {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.bgColor
}

func (t *Tab) SetBGColor(c frame.Color) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.bgColor = c
}

// Pinned reports whether the tab is pinned.
func (t *Tab) Pinned() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.pinned
}

// SetPinned sets whether the tab is pinned.
func (t *Tab) SetPinned(pinned bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pinned = pinned
}

// Manifest returns the parsed PWA web app manifest for this tab, or nil if none.
func (t *Tab) Manifest() *manifest.Manifest {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.manifest
}

// SetManifest sets the parsed PWA web app manifest for this tab.
func (t *Tab) SetManifest(m *manifest.Manifest) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.manifest = m
}

// IsStandaloneApp reports whether this tab is presented in standalone PWA application mode.
func (t *Tab) IsStandaloneApp() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.standaloneApp
}

// SetStandaloneApp configures whether this tab is in standalone PWA application mode.
func (t *Tab) SetStandaloneApp(standalone bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.standaloneApp = standalone
}
