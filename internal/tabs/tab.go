package tabs

import (
	"context"
	"sync"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/toolbar"
)

type navController struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	serial  uint64
	loading bool
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

	nav navController
}

func newTab(id uint64) *Tab {
	return &Tab{
		ID:      id,
		Title:   "New Tab",
		History: toolbar.NewHistory(),
	}
}
