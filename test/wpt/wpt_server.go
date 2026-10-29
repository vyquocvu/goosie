package wpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var wptGetSubstitution = regexp.MustCompile(`\{\{GET\[([^\]]+)\]\}\}`)
var wptPortSubstitution = regexp.MustCompile(`\{\{ports\[([a-zA-Z]+)\]\[([0-9]+)\]\}\}`)

// Server serves WPT test files over HTTP. WPT tests use absolute paths like
// /resources/testharness.js and /css/support/... which only resolve correctly
// when served from the repository root. The server maps HTTP requests to files
// in the WPT checkout directory.
type Server struct {
	mu             sync.Mutex
	listener       net.Listener
	server         *http.Server
	port           int
	root           string
	dispatchMu     sync.Mutex
	dispatchQueues map[string][][]byte
	dispatchGETs   int
	dispatchPOSTs  int
}

// NewServer creates an HTTP server rooted at the WPT checkout directory.
// The server is not started until Start is called.
func NewServer(wptRoot string) *Server {
	return &Server{
		root:           wptRoot,
		dispatchQueues: make(map[string][][]byte),
	}
}

// Start binds to a random available port and begins serving. The server URL
// is available via URL() after Start returns. The wpt.localhost origin is a
// loopback alias and gives substituted sibling hosts a shared domain suffix
// for cross-origin and document.domain coverage.
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

// URL returns the base URL of the running server, e.g. "http://wpt.localhost:12345".
func (s *Server) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fmt.Sprintf("http://wpt.localhost:%d", s.port)
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
	if cleanPath == "/common/dispatcher/dispatcher.py" {
		s.serveDispatcher(w, r)
		return
	}
	if r.Method == http.MethodPost && (strings.HasSuffix(cleanPath, "/echo-content.py") || strings.HasSuffix(cleanPath, "/echo-content-escaped.py")) {
		s.serveEchoContent(w, r, strings.HasSuffix(cleanPath, "/echo-content-escaped.py"))
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(cleanPath, "/html/semantics/forms/form-submission-0/form-echo.py") {
		s.serveFormEcho(w, r)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(cleanPath, "/html/semantics/forms/form-submission-0/resources/form-submission.py") {
		s.serveFormSubmissionCheck(w, r)
		return
	}
	if r.Method == http.MethodPost && strings.HasSuffix(cleanPath, "/html/semantics/forms/form-submission-0/resources/file-submission.py") {
		s.serveFormFileSubmission(w, r)
		return
	}
	// A few pinned upstream tests still request the old generated bundle name.
	// Current WPT checkouts place the same WebIDL2 global at this path.
	if strings.EqualFold(cleanPath, "/resources/WebIDLParser.js") {
		cleanPath = "/resources/webidl2/lib/webidl2.js"
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
	case filepath.Ext(absPath) == ".xhtml" || filepath.Ext(absPath) == ".xht":
		w.Header().Set("Content-Type", "application/xhtml+xml")
	case filepath.Ext(absPath) == ".html" || filepath.Ext(absPath) == ".htm":
		// Do not invent a transport charset: parser and meta-encoding WPTs
		// rely on the response matching the test server's actual metadata.
		w.Header().Set("Content-Type", "text/html")
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
	if data, readErr := os.ReadFile(absPath + ".headers"); readErr == nil && len(data) <= 16<<10 {
		for _, line := range strings.Split(string(data), "\n") {
			name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
			if !ok {
				continue
			}
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			if name != "" {
				if strings.EqualFold(name, "Content-Type") {
					w.Header().Set(name, value)
				} else {
					w.Header().Add(name, value)
				}
			}
		}
	}

	if strings.Contains(filepath.Base(absPath), ".sub.") {
		data, readErr := os.ReadFile(absPath)
		if readErr != nil {
			http.NotFound(w, r)
			return
		}
		data = wptGetSubstitution.ReplaceAllFunc(data, func(match []byte) []byte {
			parts := wptGetSubstitution.FindSubmatch(match)
			if len(parts) != 2 {
				return match
			}
			return []byte(html.EscapeString(r.URL.Query().Get(string(parts[1]))))
		})
		// WPT .sub. resources use distinct loopback hostnames so their runtime
		// origins remain distinct even though the single listener serves them.
		data = bytes.ReplaceAll(data, []byte("{{host}}"), []byte("wpt.localhost"))
		data = bytes.ReplaceAll(data, []byte("{{domains[www1]}}"), []byte("www1.wpt.localhost"))
		data = bytes.ReplaceAll(data, []byte("{{domains[www2]}}"), []byte("www2.wpt.localhost"))
		data = bytes.ReplaceAll(data, []byte("{{hosts[alt][]}}"), []byte("wpt-other.localhost"))
		data = bytes.ReplaceAll(data, []byte("{{hosts[alt][www2]}}"), []byte("www2.wpt-other.localhost"))
		data = bytes.ReplaceAll(data, []byte("{{location[port]}}"), []byte(fmt.Sprint(s.Port())))
		data = wptPortSubstitution.ReplaceAllFunc(data, func(match []byte) []byte {
			parts := wptPortSubstitution.FindSubmatch(match)
			if len(parts) != 3 || (!strings.EqualFold(string(parts[1]), "http") && !strings.EqualFold(string(parts[1]), "https")) || string(parts[2]) != "0" {
				return match
			}
			return []byte(fmt.Sprintf("%d", s.Port()))
		})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	http.ServeFile(w, r, absPath)
}

// serveDispatcher implements the small stateful HTTP queue used by upstream
// WPT helpers to pass messages among windows, workers, and service workers.
func (s *Server) serveDispatcher(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Add("Vary", "Origin")
	if _, cacheable := r.URL.Query()["cacheable"]; cacheable {
		w.Header().Set("Cache-Control", "max-age=31536000")
	} else {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "OPTIONS, GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uuid := r.URL.Query().Get("uuid")
	if uuid == "" {
		http.Error(w, "uuid is required", http.StatusBadRequest)
		return
	}
	if _, showHeaders := r.URL.Query()["show-headers"]; showHeaders {
		if r.Method != http.MethodGet {
			http.Error(w, "show-headers requires GET", http.StatusBadRequest)
			return
		}
		headers := make(map[string]string, len(r.Header))
		for name, values := range r.Header {
			headers[name] = strings.Join(values, ", ")
		}
		if err := json.NewEncoder(w).Encode(headers); err != nil {
			return
		}
		return
	}

	s.dispatchMu.Lock()
	defer s.dispatchMu.Unlock()
	if r.Method == http.MethodPost {
		s.dispatchPOSTs++
		const maxDispatchMessageBytes = 1 << 20
		body, err := io.ReadAll(io.LimitReader(r.Body, maxDispatchMessageBytes+1))
		if err != nil {
			http.Error(w, "unable to read dispatcher message", http.StatusBadRequest)
			return
		}
		if len(body) > maxDispatchMessageBytes {
			http.Error(w, "dispatcher message exceeds size limit", http.StatusRequestEntityTooLarge)
			return
		}
		s.dispatchQueues[uuid] = append(s.dispatchQueues[uuid], body)
		_, _ = io.WriteString(w, "done")
		return
	}
	s.dispatchGETs++
	queue := s.dispatchQueues[uuid]
	if len(queue) == 0 {
		_, _ = io.WriteString(w, "not ready")
		return
	}
	message := queue[0]
	if len(queue) == 1 {
		delete(s.dispatchQueues, uuid)
	} else {
		s.dispatchQueues[uuid] = queue[1:]
	}
	_, _ = w.Write(message)
}

type dispatcherStats struct {
	GETs            int `json:"gets"`
	POSTs           int `json:"posts"`
	Queues          int `json:"queues"`
	PendingMessages int `json:"pendingMessages"`
}

func (s *Server) dispatcherStats() dispatcherStats {
	s.dispatchMu.Lock()
	defer s.dispatchMu.Unlock()
	stats := dispatcherStats{GETs: s.dispatchGETs, POSTs: s.dispatchPOSTs, Queues: len(s.dispatchQueues)}
	for _, queue := range s.dispatchQueues {
		stats.PendingMessages += len(queue)
	}
	return stats
}

func (s *Server) serveFormFileSubmission(w http.ResponseWriter, r *http.Request) {
	const maxBody = 4 << 20
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "request body exceeds WPT file submission limit", http.StatusRequestEntityTooLarge)
		return
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		http.Error(w, "invalid WPT multipart file submission", http.StatusBadRequest)
		return
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var fileData []byte
	found := false
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			http.Error(w, "invalid WPT multipart body", http.StatusBadRequest)
			return
		}
		if part.FormName() == "testinput" {
			fileData, err = io.ReadAll(io.LimitReader(part, maxBody+1))
			found = err == nil && len(fileData) <= maxBody
		}
		_ = part.Close()
		if err != nil || len(fileData) > maxBody {
			http.Error(w, "WPT uploaded file exceeds limit", http.StatusRequestEntityTooLarge)
			return
		}
	}
	quoted, _ := json.Marshal(string(fileData))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !found {
		_, _ = io.WriteString(w, "<!doctype html><script>parent.postMessage(null, '*');</script>")
		return
	}
	_, _ = fmt.Fprintf(w, "<!doctype html><script>parent.postMessage(%s, '*');</script>", quoted)
}

func (s *Server) serveEchoContent(w http.ResponseWriter, r *http.Request, escaped bool) {
	const maxEchoBody = 4 << 20
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxEchoBody))
	if err != nil {
		http.Error(w, "request body exceeds WPT echo limit", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Request-Method", r.Method)
	w.Header().Set("X-Request-Content-Length", fmt.Sprint(len(body)))
	w.Header().Set("X-Request-Content-Type", r.Header.Get("Content-Type"))
	if escaped {
		var output bytes.Buffer
		for _, b := range body {
			switch {
			case b < 0x20 || b >= 0x7f:
				fmt.Fprintf(&output, "\\x%02x", b)
			case b == '\\':
				output.WriteString("\\\\")
			default:
				output.WriteByte(b)
			}
		}
		body = bytes.ReplaceAll(output.Bytes(), []byte("\\x0d\\x0a"), []byte("\r\n"))
	}
	_, _ = w.Write(body)
}

func (s *Server) serveFormEcho(w http.ResponseWriter, r *http.Request) {
	const maxEchoBody = 4 << 20
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxEchoBody))
	if err != nil {
		http.Error(w, "request body exceeds WPT form echo limit", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("X-Request-Method", r.Method)
	w.Header().Set("X-Request-Content-Type", r.Header.Get("Content-Type"))
	var encoded strings.Builder
	encoded.Grow(len(body) * 3)
	for i, value := range body {
		if i != 0 {
			encoded.WriteByte(' ')
		}
		const digits = "0123456789abcdef"
		encoded.WriteByte(digits[value>>4])
		encoded.WriteByte(digits[value&0x0f])
	}
	_, _ = io.WriteString(w, encoded.String())
}

// serveFormSubmissionCheck implements the small pinned WPT handler used by
// submit-entity-body.html. It checks encoded form bodies without running the
// Python WPT server or exposing a general-purpose script handler.
func (s *Server) serveFormSubmissionCheck(w http.ResponseWriter, r *http.Request) {
	const maxBody = 4 << 20
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "request body exceeds WPT form submission limit", http.StatusRequestEntityTooLarge)
		return
	}
	ok := false
	query := r.URL.Query()
	if query.Get("query") == "1" {
		mediaType, params, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if parseErr == nil {
			switch mediaType {
			case "application/x-www-form-urlencoded":
				ok = bytes.Equal(body, []byte("foo=bara"))
			case "text/plain":
				ok = bytes.Equal(body, []byte("qux=baz\r\n"))
			case "multipart/form-data":
				if boundary := params["boundary"]; boundary != "" {
					reader := multipart.NewReader(bytes.NewReader(body), boundary)
					for {
						part, nextErr := reader.NextPart()
						if nextErr == io.EOF {
							break
						}
						if nextErr != nil {
							break
						}
						value, readErr := io.ReadAll(io.LimitReader(part, maxBody+1))
						name := part.FormName()
						_ = part.Close()
						if readErr != nil || len(value) > maxBody {
							break
						}
						if name == "foo" && bytes.Equal(value, []byte("bar")) {
							ok = true
						}
					}
				}
			}
		}
	} else if expected, present := query["expected_body"]; present && len(expected) > 0 {
		ok = bytes.Equal(body, []byte(expected[0]))
	}
	w.Header().Set("Content-Type", "text/plain")
	if ok {
		_, _ = io.WriteString(w, "OK")
	} else {
		_, _ = io.WriteString(w, "FAIL")
	}
}
