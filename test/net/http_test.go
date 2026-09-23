package net_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	transport "github.com/vyquocvu/goosie/internal/net"
)

func TestGetFinalResponseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final?ok=1", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("final"))
	}))
	defer server.Close()
	client := transport.DefaultClient()
	defer client.Close()
	resp, err := client.Get(context.Background(), server.URL+"/start")
	if err != nil {
		t.Fatal(err)
	}
	if want := server.URL + "/final?ok=1"; resp.URL != want {
		t.Fatalf("response URL = %q, want %q", resp.URL, want)
	}
}

func TestGetRejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), (8<<20)+1))
	}))
	defer server.Close()
	client := transport.DefaultClient()
	defer client.Close()
	resp, err := client.Get(context.Background(), server.URL)
	if err == nil {
		t.Fatalf("accepted oversized body of %d bytes", len(resp.Body))
	}
}

func TestGetPercentEscapedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page with # and %.html")
	if err := os.WriteFile(path, []byte("local page"), 0600); err != nil {
		t.Fatal(err)
	}
	fileURL := (&url.URL{Scheme: "file", Path: path}).String()
	client := transport.DefaultClient()
	defer client.Close()
	resp, err := client.Get(context.Background(), fileURL)
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) != "local page" {
		t.Fatalf("body = %q", resp.Body)
	}
}

func TestNormalizeURL(t *testing.T) {
	valid := map[string]string{
		"  example.com/path?q=1#section  ": "https://example.com/path?q=1#section",
		"localhost:8080/path":              "https://localhost:8080/path",
		"example.com:8443":                 "https://example.com:8443",
		"127.0.0.1:8080":                   "https://127.0.0.1:8080",
		"[::1]:8080/path":                  "https://[::1]:8080/path",
		"https://example.com":              "https://example.com",
		"http://localhost:80/page":         "http://localhost:80/page",
		"HTTP://example.com":               "http://example.com",
		"file:///tmp/a%20b.html":           "file:///tmp/a%20b.html",
		"file://localhost/tmp/a%23b.html":  "file://localhost/tmp/a%23b.html",
	}
	for raw, want := range valid {
		t.Run(raw, func(t *testing.T) {
			got, err := transport.NormalizeURL(raw)
			if err != nil || got != want {
				t.Fatalf("NormalizeURL(%q) = %q, %v; want %q", raw, got, err, want)
			}
		})
	}
	invalid := []string{
		"", "   ", "/tmp/page.html", "//example.com/path", "https:///path", "http:example.com",
		"https://user:password@example.com", "https://user@example.com", "user@example.com",
		"https://example.com:", "localhost:", "localhost:abc", "example.com:abc",
		"http://example.com:0", "https://example.com:65536", "localhost:99999", "http://localhost:-1",
		"https://bad host/", "https://-bad.example", "https://bad-.example", "https://a..example",
		"https://example_.com", "https://999.999.999.999", "http://[not-an-ip]/", "http://::1/",
		"https://example.com/\npath", "\thttps://example.com", "https://example.com/\x7f",
		"https://example.com/%00", "https://example.com/%0a", "https://example.com/?q=%0d",
		"ftp://example.com", "mailto:person@example.com", "javascript:alert(1)", "data:text/html,hello",
		"file:relative.html", "file://remote/tmp/page.html", "file://localhost:80/tmp/page.html",
		"file://user@localhost/tmp/page.html", "file:///tmp/page.html?query", "file:///tmp/page.html?",
		"file://localhost", "file:///%00", "file:///tmp/%zz",
	}
	for _, raw := range invalid {
		t.Run("reject/"+raw, func(t *testing.T) {
			if got, err := transport.NormalizeURL(raw); err == nil {
				t.Fatalf("NormalizeURL(%q) accepted %q", raw, got)
			}
		})
	}
}

func TestRequestsValidateURL(t *testing.T) {
	client := transport.DefaultClient()
	defer client.Close()
	for _, raw := range []string{"ftp://localhost/file", "http://user@localhost/", "http://localhost:65536", "file://remote/tmp/page"} {
		if _, err := client.Get(context.Background(), raw); err == nil {
			t.Errorf("Get accepted %q", raw)
		}
		if _, err := client.Post(context.Background(), raw, "text/plain", strings.NewReader("body")); err == nil {
			t.Errorf("Post accepted %q", raw)
		}
	}
}

func TestHTTPStatusHeadersAndPost(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					t.Errorf("method = %s, want %s", r.Method, method)
				}
				if method == http.MethodPost {
					body, err := io.ReadAll(r.Body)
					if err != nil || string(body) != "request body" || r.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
						t.Errorf("Post lost body or content type: %q, %v, %q", body, err, r.Header.Get("Content-Type"))
					}
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Add("X-Trace", "one")
				w.Header().Add("X-Trace", "two")
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = io.WriteString(w, "<p>error page</p>")
			}))
			defer server.Close()
			client := transport.DefaultClient()
			defer client.Close()
			var resp *transport.Response
			var err error
			if method == http.MethodGet {
				resp, err = client.Get(context.Background(), server.URL)
			} else {
				resp, err = client.Post(context.Background(), server.URL, "text/plain; charset=utf-8", strings.NewReader("request body"))
			}
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusUnprocessableEntity || string(resp.Body) != "<p>error page</p>" {
				t.Fatalf("status/body = %d / %q", resp.StatusCode, resp.Body)
			}
			if resp.Headers["Content-Type"] != "text/html; charset=utf-8" || resp.Headers["X-Trace"] != "one, two" {
				t.Fatalf("headers = %v", resp.Headers)
			}
		})
	}
}

func TestHTTPCancellation(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, stage := range []string{"request", "body"} {
			t.Run(method+"/"+stage, func(t *testing.T) {
				ready, disconnected, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Drain the upload so net/http can observe client disconnection
					// even while the handler is blocked before response headers.
					_, _ = io.Copy(io.Discard, r.Body)
					if stage == "body" {
						_, _ = io.WriteString(w, "partial")
						w.(http.Flusher).Flush()
					}
					close(ready)
					select {
					case <-r.Context().Done():
						close(disconnected)
					case <-release:
					}
				}))
				defer server.Close()
				defer close(release)
				client := transport.DefaultClient()
				defer client.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				result := make(chan error, 1)
				go func() {
					var err error
					if method == http.MethodGet {
						_, err = client.Get(ctx, server.URL)
					} else {
						_, err = client.Post(ctx, server.URL, "text/plain", strings.NewReader("body"))
					}
					result <- err
				}()
				await(t, ready, "request reached server")
				cancel()
				select {
				case err := <-result:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("error = %v, want context.Canceled", err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("request did not cancel")
				}
				await(t, disconnected, "response body closed after cancellation")
			})
		}
	}
}

func TestHTTPResponseByteLimit(t *testing.T) {
	if transport.MaxResponseBytes != 8<<20 {
		t.Fatalf("MaxResponseBytes = %d", transport.MaxResponseBytes)
	}
	for _, mode := range []string{"declared", "chunked", "gzip"} {
		for _, extra := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/extra=%d", mode, extra), func(t *testing.T) {
				data := bytes.Repeat([]byte("x"), transport.MaxResponseBytes+extra)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch mode {
					case "declared":
						w.Header().Set("Content-Length", strconv.Itoa(len(data)))
					case "chunked":
						w.(http.Flusher).Flush()
					case "gzip":
						w.Header().Set("Content-Encoding", "gzip")
						writer := gzip.NewWriter(w)
						_, _ = writer.Write(data)
						_ = writer.Close()
						return
					}
					_, _ = w.Write(data)
				}))
				defer server.Close()
				client := transport.DefaultClient()
				defer client.Close()
				resp, err := client.Get(context.Background(), server.URL)
				if extra != 0 {
					if !errors.Is(err, transport.ErrResponseTooLarge) {
						t.Fatalf("error = %v, want ErrResponseTooLarge", err)
					}
				} else if err != nil || resp == nil || !bytes.Equal(resp.Body, data) {
					t.Fatalf("exact-limit response rejected or changed: %v", err)
				}
			})
		}
	}
}

func TestHTTPStopsAndClosesOversizedBody(t *testing.T) {
	for _, mode := range []string{"declared", "chunked", "post"} {
		t.Run(mode, func(t *testing.T) {
			disconnected, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "declared" {
					w.Header().Set("Content-Length", strconv.Itoa(transport.MaxResponseBytes+1))
					w.(http.Flusher).Flush()
				} else {
					w.(http.Flusher).Flush()
					_, _ = w.Write(bytes.Repeat([]byte("x"), transport.MaxResponseBytes+1))
					w.(http.Flusher).Flush()
				}
				select {
				case <-r.Context().Done():
					close(disconnected)
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			client := transport.DefaultClient()
			defer client.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var err error
			if mode == "post" {
				_, err = client.Post(ctx, server.URL, "text/plain", strings.NewReader("request"))
			} else {
				_, err = client.Get(ctx, server.URL)
			}
			if !errors.Is(err, transport.ErrResponseTooLarge) {
				t.Fatalf("error = %v, want early ErrResponseTooLarge", err)
			}
			await(t, disconnected, "oversized response body closed")
		})
	}
}

func TestHTTPRedirectValidation(t *testing.T) {
	for _, target := range []string{"file:///etc/hosts", "ftp://localhost/file", "http://user@localhost/", "http://localhost:65536", "http://bad-.example/"} {
		t.Run(target, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, target, http.StatusFound)
			}))
			defer server.Close()
			client := transport.DefaultClient()
			defer client.Close()
			if _, err := client.Get(context.Background(), server.URL); err == nil {
				t.Fatalf("followed forbidden redirect to %q", target)
			}
		})
	}
	for _, count := range []int{10, 11} {
		t.Run(fmt.Sprintf("hops=%d", count), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/"))
				if n < count {
					http.Redirect(w, r, fmt.Sprintf("/%d", n+1), http.StatusFound)
					return
				}
				_, _ = io.WriteString(w, "done")
			}))
			defer server.Close()
			client := transport.DefaultClient()
			defer client.Close()
			resp, err := client.Get(context.Background(), server.URL+"/0")
			if count == 10 {
				if err != nil || resp.URL != server.URL+"/10" {
					t.Fatalf("ten redirects should succeed: %v", err)
				}
			} else if err == nil {
				t.Fatal("followed more than ten redirects")
			}
		})
	}
	t.Run("loop", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/loop", http.StatusFound)
		}))
		defer server.Close()
		client := transport.DefaultClient()
		defer client.Close()
		if _, err := client.Get(context.Background(), server.URL); err == nil {
			t.Fatal("redirect loop succeeded")
		}
	})
}

func TestHTTPTimeoutAndTLSVerification(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer server.Close()
		client := transport.DefaultClient()
		defer client.Close()
		client.SetTimeout(50 * time.Millisecond)
		if _, err := client.Get(context.Background(), server.URL); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("timeout error = %v", err)
		}
	})
	t.Run("untrusted TLS", func(t *testing.T) {
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		server.Config.ErrorLog = log.New(io.Discard, "", 0)
		server.StartTLS()
		defer server.Close()
		client := transport.DefaultClient()
		defer client.Close()
		if _, err := client.Get(context.Background(), server.URL); err == nil {
			t.Fatal("accepted untrusted TLS certificate")
		}
	})
}

func TestFileResponse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page with # and %.html")
	if err := os.WriteFile(path, []byte("<p>local</p>"), 0600); err != nil {
		t.Fatal(err)
	}
	client := transport.DefaultClient()
	defer client.Close()
	for _, host := range []string{"", "localhost"} {
		fileURL := (&url.URL{Scheme: "file", Host: host, Path: path}).String()
		resp, err := client.Get(context.Background(), fileURL)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 || resp.URL != fileURL || string(resp.Body) != "<p>local</p>" || !strings.HasPrefix(resp.Headers["Content-Type"], "text/html") {
			t.Fatalf("file response = %+v", resp)
		}
		if _, err := client.Post(context.Background(), fileURL, "text/plain", strings.NewReader("overwrite")); err == nil {
			t.Fatal("Post accepted file URL")
		}
	}
}

func TestFileBoundaries(t *testing.T) {
	client := transport.DefaultClient()
	defer client.Close()
	for _, path := range []string{t.TempDir(), os.DevNull} {
		fileURL := (&url.URL{Scheme: "file", Path: path}).String()
		if _, err := client.Get(context.Background(), fileURL); err == nil {
			t.Errorf("accepted nonregular file %q", path)
		}
	}
	path := filepath.Join(t.TempDir(), "body.html")
	for _, size := range []int{transport.MaxResponseBytes, transport.MaxResponseBytes + 1} {
		if err := os.WriteFile(path, bytes.Repeat([]byte("x"), size), 0600); err != nil {
			t.Fatal(err)
		}
		fileURL := (&url.URL{Scheme: "file", Path: path}).String()
		resp, err := client.Get(context.Background(), fileURL)
		if size > transport.MaxResponseBytes {
			if !errors.Is(err, transport.ErrResponseTooLarge) {
				t.Fatalf("file size error = %v", err)
			}
		} else if err != nil || len(resp.Body) != size {
			t.Fatalf("exact-limit file rejected: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := client.Get(ctx, fileURL); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled file read = %v", err)
		}
	}
}

func TestFilePathTraversal(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("sensitive"), 0600); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal(err)
	}
	client := transport.DefaultClient()
	defer client.Close()
	traversal := filepath.Join(subdir, "..", "secret.txt")
	fileURL := (&url.URL{Scheme: "file", Path: traversal}).String()
	resp, err := client.Get(context.Background(), fileURL)
	if err != nil {
		t.Fatalf("normalized path should resolve: %v", err)
	}
	if string(resp.Body) != "sensitive" {
		t.Fatalf("body = %q, want %q", resp.Body, "sensitive")
	}
	cleanURL := (&url.URL{Scheme: "file", Path: secret}).String()
	if resp.URL != cleanURL {
		t.Fatalf("URL = %q, want cleaned %q", resp.URL, cleanURL)
	}
}

func TestHTTPSDowngradeRedirectBlocked(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "downgraded")
	}))
	defer plain.Close()
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer tls.Close()
	client := transport.DefaultClientWithTLS(tls.TLS)
	defer client.Close()
	if _, err := client.Get(context.Background(), tls.URL); err == nil {
		t.Fatal("HTTPS-to-HTTP redirect should be blocked")
	}
}

func TestSetCookiePreservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "a=1; Expires=Wed, 09 Jun 2026 10:00:00 GMT")
		w.Header().Add("Set-Cookie", "b=2; Expires=Thu, 10 Jun 2026 10:00:00 GMT")
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	client := transport.DefaultClient()
	defer client.Close()
	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	cookies := resp.Headers["Set-Cookie"]
	if !strings.Contains(cookies, "\n") {
		t.Fatalf("Set-Cookie should use newline separator, got %q", cookies)
	}
	parts := strings.Split(cookies, "\n")
	if len(parts) != 2 {
		t.Fatalf("expected 2 cookies, got %d: %q", len(parts), cookies)
	}
	if !strings.Contains(parts[0], "a=1") || !strings.Contains(parts[1], "b=2") {
		t.Fatalf("cookies = %q", cookies)
	}
}

func await(t *testing.T, ch <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}
