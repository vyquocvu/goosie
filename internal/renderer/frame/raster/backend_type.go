package raster

import "fmt"

// BackendType identifies a raster backend implementation.
type BackendType int

const (
	BackendUnspecified BackendType = iota
	BackendCPU
	BackendCoreGraphics
)

func (t BackendType) String() string {
	switch t {
	case BackendCPU:
		return "cpu"
	case BackendCoreGraphics:
		return "core-graphics"
	default:
		return "unspecified"
	}
}

// ---------------------------------------------------------------------------
// BackendOption
// ---------------------------------------------------------------------------

// BackendOption configures how NewBackend creates a backend.
type BackendOption interface {
	apply(*backendConfig)
}

type backendFuncOption func(*backendConfig)

func (f backendFuncOption) apply(cfg *backendConfig) { f(cfg) }

type backendConfig struct {
	forceType    BackendType
	crashRecover bool
}

func defaultConfig() backendConfig {
	return backendConfig{
		forceType: BackendUnspecified,
	}
}

// WithBackend forces NewBackend to use a specific backend type. When the
// forced backend fails, NewBackend returns the error instead of falling
// back to CPU.
func WithBackend(t BackendType) BackendOption {
	return backendFuncOption(func(cfg *backendConfig) {
		cfg.forceType = t
	})
}

// WithCrashRecover enables panic recovery during backend construction.
// If the preferred backend panics during creation, NewBackend recovers
// and falls back to the CPU backend.
func WithCrashRecover() BackendOption {
	return backendFuncOption(func(cfg *backendConfig) {
		cfg.crashRecover = true
	})
}

// ---------------------------------------------------------------------------
// NewBackend
// ---------------------------------------------------------------------------

// NewBackend creates a raster Backend with automatic type selection.
// It returns the backend, the type actually created, and any error.
//
// Selection rules:
//  1. If WithBackend(t) is provided, that type is forced.
//  2. Otherwise SelectBackend() chooses the best backend for the platform.
//  3. If the chosen backend fails to construct and no type was forced, CPU
//     is used as a fallback.
//  4. If WithCrashRecover is set, constructor panics are caught and CPU is
//     used as a fallback.
func NewBackend(w, h int, opts ...BackendOption) (Backend, BackendType, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o.apply(&cfg)
	}

	bt := cfg.forceType
	if bt == BackendUnspecified {
		bt = SelectBackend()
	}

	b, actual, err := createBackendWithFallback(bt, w, h, cfg)
	return b, actual, err
}

func createBackendWithFallback(bt BackendType, w, h int, cfg backendConfig) (b Backend, actual BackendType, err error) {
	if cfg.crashRecover {
		defer func() {
			if r := recover(); r != nil {
				b = NewCPUBackend(w, h)
				actual = BackendCPU
				err = nil
			}
		}()
	}

	switch bt {
	case BackendCoreGraphics:
		b, err = NewCGBackend(w, h)
		if err == nil {
			return b, bt, nil
		}
		if cfg.forceType == BackendUnspecified {
			return NewCPUBackend(w, h), BackendCPU, nil
		}
		return nil, bt, fmt.Errorf("%s: %w", bt, err)

	case BackendCPU:
		return NewCPUBackend(w, h), BackendCPU, nil

	default:
		return NewCPUBackend(w, h), BackendCPU, nil
	}
}
