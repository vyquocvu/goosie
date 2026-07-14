// Package renderer implements renderer process isolation for the Goosie
// browser engine. It provides a protocol for communicating between a parent
// process (shell/UI) and a child process (engine session) over stdin/stdout
// using newline-delimited JSON messages.
//
// Architecture:
//   - Parent sends Command messages to child's stdin
//   - Child sends message.Message events to parent's stdout
//   - Both sides use a simple length-prefixed framing for reliability
package renderer

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/vyquocvu/goosie/internal/engine/message"
)

// Protocol version for compatibility checking.
const ProtocolVersion = 1

// Command is a message sent from the parent process to the child process.
// Exactly one payload field should be set per command.
type Command struct {
	Version int       `json:"v"`
	Time    time.Time `json:"ts,omitempty"`

	// Engine lifecycle
	Navigate    *NavigateCmd   `json:"navigate,omitempty"`
	SetViewport *ViewportCmd   `json:"setViewport,omitempty"`
	Input       *message.Input `json:"input,omitempty"`
	Close       *CloseCmd      `json:"close,omitempty"`

	// Synchronization
	Ping *PingCmd `json:"ping,omitempty"`
}

// NavigateCmd instructs the child to start a navigation.
type NavigateCmd struct {
	URL string `json:"url"`
}

// ViewportCmd sets the viewport dimensions.
type ViewportCmd struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// CloseCmd instructs the child to shut down gracefully.
type CloseCmd struct{}

// PingCmd is a keepalive/synchronization message.
type PingCmd struct{}

// Encode serializes a Command to JSON.
func (c *Command) Encode() ([]byte, error) {
	return json.Marshal(c)
}

// DecodeCommand deserializes a Command from JSON.
func DecodeCommand(data []byte) (*Command, error) {
	var c Command
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("renderer: decode command: %w", err)
	}
	if c.Version != ProtocolVersion {
		return nil, fmt.Errorf("renderer: unsupported protocol version %d", c.Version)
	}
	return &c, nil
}

// NewNavigateCommand creates a Navigate command.
func NewNavigateCommand(url string) *Command {
	return &Command{
		Version:  ProtocolVersion,
		Time:     time.Now(),
		Navigate: &NavigateCmd{URL: url},
	}
}

// NewSetViewportCommand creates a SetViewport command.
func NewSetViewportCommand(width, height float64) *Command {
	return &Command{
		Version:     ProtocolVersion,
		Time:        time.Now(),
		SetViewport: &ViewportCmd{Width: width, Height: height},
	}
}

// NewInputCommand creates an Input command.
func NewInputCommand(input message.Input) *Command {
	return &Command{
		Version: ProtocolVersion,
		Time:    time.Now(),
		Input:   &input,
	}
}

// NewCloseCommand creates a Close command.
func NewCloseCommand() *Command {
	return &Command{
		Version: ProtocolVersion,
		Time:    time.Now(),
		Close:   &CloseCmd{},
	}
}

// NewPingCommand creates a Ping command.
func NewPingCommand() *Command {
	return &Command{
		Version: ProtocolVersion,
		Time:    time.Now(),
		Ping:    &PingCmd{},
	}
}
