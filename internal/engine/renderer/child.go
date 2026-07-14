package renderer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/message"
	"github.com/vyquocvu/goosie/internal/engine/session"
)

// Child runs an engine session, reading commands from an io.Reader
// and writing events to an io.Writer. All I/O uses newline-delimited JSON.
// This abstraction works over os.Pipe, net.Pipe, or any other streams.
type Child struct {
	r       io.Reader
	w       io.Writer
	session *session.Session
}

// NewChild creates a Child that reads commands from r and writes events to w.
func NewChild(r io.Reader, w io.Writer) *Child {
	return &Child{
		r:       r,
		w:       w,
		session: session.New(),
	}
}

// Run enters the main loop: reads commands, executes them, and writes
// events back. It returns when a Close command is received or r is closed.
func (c *Child) Run(ctx context.Context) error {
	c.session.SetEventCallback(func(ev session.Event) {
		msg := message.Event(ev, time.Now())
		c.writeEvent(msg)
	})

	scanner := bufio.NewScanner(c.r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		cmd, err := DecodeCommand(line)
		if err != nil {
			c.writeError(fmt.Sprintf("decode error: %v", err))
			continue
		}

		if c.handleCommand(ctx, cmd) {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("renderer: read: %w", err)
	}
	return nil
}

func (c *Child) handleCommand(ctx context.Context, cmd *Command) bool {
	switch {
	case cmd.Navigate != nil:
		c.handleNavigate(ctx, cmd.Navigate)
	case cmd.SetViewport != nil:
		c.writeEvent(&message.Message{
			Version: message.Version,
			Time:    time.Now(),
			Viewport: &message.Viewport{
				Width:  cmd.SetViewport.Width,
				Height: cmd.SetViewport.Height,
			},
		})
	case cmd.Input != nil:
		c.writeEvent(&message.Message{
			Version: message.Version,
			Time:    time.Now(),
			Log: &message.Log{
				Level:   "debug",
				Message: fmt.Sprintf("input: kind=%d", cmd.Input.Kind),
			},
		})
	case cmd.Close != nil:
		c.session.Close()
		return true
	case cmd.Ping != nil:
		c.writeEvent(&message.Message{Version: message.Version, Time: time.Now()})
	}
	return false
}

func (c *Child) handleNavigate(ctx context.Context, cmd *NavigateCmd) {
	_, navCtx := c.session.Navigate(ctx, cmd.URL)
	// Prototype: mark complete after a tick so lifecycle events flow.
	go func() {
		t := time.NewTimer(50 * time.Millisecond)
		defer t.Stop()
		select {
		case <-t.C:
			if c.session.State() == session.StateNavigating {
				c.session.Complete()
			}
		case <-navCtx.Done():
		}
	}()
}

func (c *Child) writeEvent(msg *message.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	data = append(data, '\n')
	c.w.Write(data)
}

func (c *Child) writeError(text string) {
	c.writeEvent(&message.Message{
		Version: message.Version,
		Time:    time.Now(),
		Error:   &message.Error{Message: text},
	})
}

// Parent reads events from a child process and sends commands to it.
// It operates on io.ReadCloser / io.WriteCloser so it can be used with
// exec.Cmd pipes or in-memory io.Pipe pairs for testing.
type Parent struct {
	w   io.WriteCloser
	r   io.ReadCloser
	mu  sync.Mutex
	done chan struct{}

	// OnEvent is called for each event from the child.
	// Runs in the reader goroutine — must not block.
	OnEvent func(*message.Message)

	// OnExit is called when the child stream ends.
	OnExit func(error)
}

// NewParent creates a Parent that reads events from r and sends commands to w.
func NewParent(w io.WriteCloser, r io.ReadCloser) *Parent {
	return &Parent{
		w:    w,
		r:    r,
		done: make(chan struct{}),
	}
}

// Start begins reading events in a background goroutine.
func (p *Parent) Start() {
	go p.readLoop()
}

// Send writes a command to the child.
func (p *Parent) Send(cmd *Command) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := cmd.Encode()
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = p.w.Write(data)
	return err
}

// Close sends a Close command and tears down the pipes.
func (p *Parent) Close() error {
	p.Send(NewCloseCommand())
	p.mu.Lock()
	defer p.mu.Unlock()
	p.w.Close()
	p.r.Close()
	<-p.done
	return nil
}

func (p *Parent) readLoop() {
	defer close(p.done)

	scanner := bufio.NewScanner(p.r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg message.Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		if p.OnEvent != nil {
			p.OnEvent(&msg)
		}
	}

	err := scanner.Err()
	if p.OnExit != nil {
		if err != nil {
			p.OnExit(err)
		} else {
			p.OnExit(nil)
		}
	}
}
