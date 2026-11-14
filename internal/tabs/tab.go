package tabs

import (
	"context"
	"image"
	"sync"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/manifest"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

type NavController struct {
	Mu      sync.Mutex
	Cancel  context.CancelFunc
	Serial  uint64
	Loading bool
}

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
	favicon         image.Image

	Nav NavController
}

func newTab(id uint64) *Tab {
	return &Tab{
		id:      id,
		title:   "New Tab",
		history: toolbar.NewHistory(),
	}
}

func (t *Tab) ShieldsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return !t.shieldsDisabled
}

func (t *Tab) SetShieldsEnabled(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.shieldsDisabled = !enabled
}

func (t *Tab) BlockedTrackers() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.blockedTrackers
}

func (t *Tab) IncrementBlockedTrackers() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blockedTrackers++
}

func (t *Tab) ResetBlockedTrackers() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blockedTrackers = 0
}

func (t *Tab) Incognito() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.incognito
}

func (t *Tab) SetIncognito(incognito bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.incognito = incognito
}

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

func (t *Tab) Pinned() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.pinned
}

func (t *Tab) SetPinned(pinned bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pinned = pinned
}

func (t *Tab) Manifest() *manifest.Manifest {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.manifest
}

func (t *Tab) SetManifest(m *manifest.Manifest) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.manifest = m
}

func (t *Tab) IsStandaloneApp() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.standaloneApp
}

func (t *Tab) SetStandaloneApp(standalone bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.standaloneApp = standalone
}

func (t *Tab) Favicon() image.Image {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.favicon
}

func (t *Tab) SetFavicon(icon image.Image) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.favicon = icon
}
