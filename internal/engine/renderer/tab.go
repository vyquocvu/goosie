package renderer

import (
	"io"
	"sync"

	"github.com/vyquocvu/goosie/internal/engine/message"
)

// Tab manages one renderer child process with crash recovery.
// On unexpected child exit, Tab emits a Crash event via OnCrash.
// Calling Restart with fresh pipes resumes the session.
type Tab struct {
	mu      sync.Mutex
	parent  *Parent
	nextURL string

	// OnEvent is forwarded from the parent for each child event.
	OnEvent func(*message.Message)

	// OnCrash is called when the child exits unexpectedly.
	OnCrash func(error)

	// OnRecover is called after a successful Restart.
	OnRecover func()
}

// NewTab creates a Tab from an already-connected child and parent.
func NewTab(child *Child, parent *Parent) *Tab {
	t := &Tab{parent: parent}
	t.parent.OnEvent = func(msg *message.Message) {
		if t.OnEvent != nil {
			t.OnEvent(msg)
		}
	}
	t.parent.OnExit = func(err error) {
		if err != nil && t.OnCrash != nil {
			t.OnCrash(err)
		}
	}
	t.parent.Start()
	return t
}

// Navigate sends a Navigate command. The URL is saved for replay on restart.
func (t *Tab) Navigate(url string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextURL = url
	return t.parent.Send(NewNavigateCommand(url))
}

// SetViewport sends a SetViewport command.
func (t *Tab) SetViewport(w, h float64) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.parent.Send(NewSetViewportCommand(w, h))
}

// Close sends a Close command and tears down the tab.
func (t *Tab) Close() error {
	return t.parent.Close()
}

// Restart replaces the underlying pipes with fresh ones.
// It does NOT send commands — the caller must start the child first,
// then call Navigate to replay the last URL. This avoids blocking on
// an unread pipe.
func (t *Tab) Restart(newChild io.Reader, newChildW io.Writer, newParentW io.WriteCloser, newParentR io.ReadCloser) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.parent.w.Close()
	t.parent.r.Close()

	t.parent = NewParent(newParentW, newParentR)
	t.parent.OnEvent = func(msg *message.Message) {
		if t.OnEvent != nil {
			t.OnEvent(msg)
		}
	}
	t.parent.OnExit = func(err error) {
		if err != nil && t.OnCrash != nil {
			t.OnCrash(err)
		}
	}
	t.parent.Start()

	if t.OnRecover != nil {
		t.OnRecover()
	}
}
