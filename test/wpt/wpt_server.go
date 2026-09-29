package wpt

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

// Server serves WPT test files over HTTP. WPT tests use absolute paths like
// /resources/testharness.js and /css/support/... which only resolve correctly
// when served from the repository root. The server maps HTTP requests to files
// in the WPT checkout directory.
type Server struct {
	mu       sync.Mutex
	listener net.Listener
	server   *http.Server
	port     int
	root     string
}

// NewServer creates an HTTP server rooted at the WPT checkout directory.
// The server is not started until Start is called.
func NewServer(wptRoot string) *Server {
	return &Server{
		root: wptRoot,
	}
}

// Start binds to a random available port and begins serving. The server URL
// is available via URL() after Start returns.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	var err error
	s.listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("wpt: listen: %w", err)
	}
	s.port = s.listener.Addr().(*net.TCPAddr).Port

	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		_ = s.server.Serve(s.listener)
	}()
	return nil
}

// URL returns the base URL of the running server, e.g. "http://127.0.0.1:12345".
func (s *Server) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("http://127.0.0.1:%d", s.port)
}

// Port returns the port the server is listening on.
func (s *Server) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// Stop shuts down the server gracefully.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// handleRequest serves files from the WPT checkout. Path traversal is prevented
// by cleaning the path and verifying it stays within the root directory.
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Clean the path to prevent directory traversal.
	cleanPath := filepath.Clean(r.URL.Path)
	if cleanPath == "." || cleanPath == "/" {
		cleanPath = "/"
	}

	// Build the full filesystem path and verify it stays within root.
	fullPath := filepath.Join(s.root, cleanPath)
	absRoot, err := filepath.Abs(s.root)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Verify the resolved path is within the root directory.
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil || len(rel) >= 2 && rel[:2] == ".." {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Set appropriate content types for WPT resources.
	switch {
	case filepath.Ext(absPath) == ".html" || filepath.Ext(absPath) == ".htm":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case filepath.Ext(absPath) == ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case filepath.Ext(absPath) == ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case filepath.Ext(absPath) == ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case filepath.Ext(absPath) == ".png":
		w.Header().Set("Content-Type", "image/png")
	case filepath.Ext(absPath) == ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case filepath.Ext(absPath) == ".woff" || filepath.Ext(absPath) == ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	}

	http.ServeFile(w, r, absPath)
}
