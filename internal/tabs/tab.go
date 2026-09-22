package tabs

import (
	"context"
	"sync"

	"github.com/vyquocvu/goosie/internal/frame"
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
type Tab struct {
	ID      uint64
	URL     string
	Title   string
	History *toolbar.History
	ScrollY int32
	Layer   *frame.Layer
	Loading bool
	Error   string
	BGColor frame.Color

	Nav NavController
}

func newTab(id uint64) *Tab {
	return &Tab{
		ID:      id,
		Title:   "New Tab",
		History: toolbar.NewHistory(),
	}
}
