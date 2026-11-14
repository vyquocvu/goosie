//go:build !darwin || !cgo

package darwin

import (
	"time"

	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/media"
)

type Config struct {
	Title string
	Size  frame.Size
	Scale float32
}

type Window struct{}

func Available() bool { return false }

func Open(Config) (*Window, error) { return nil, ErrPlatformUnavailable }

func NewMediaBackend() media.Backend { return nil }

func PumpMainRunLoopForTest(time.Duration) {}
