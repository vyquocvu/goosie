package net_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	transport "github.com/vyquocvu/goosie/internal/net"
)

func redirectProbe(t *testing.T, setCookie, startPath, targetPath string) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var got atomic.Value
	got.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case startPath:
			if setCookie != "" {
				w.Header().Add("Set-Cookie", setCookie)
			}
			http.Redirect(w, r, targetPath, http.StatusFound)
		case targetPath:
			got.Store(r.Header.Get("Cookie"))
			_, _ = io.WriteString(w, "target")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

// qc:BR-157
func TestCookieSetOnRedirectReachesTheRedirectTarget(t *testing.T) {
	cases := []struct {
		name          string
		setCookie     string
		start, target string
		wantCookie    string
	}{
		{
			name:       "root path cookie",
			setCookie:  "flow=abc; Path=/",
			start:      "/start",
			target:     "/target",
			wantCookie: "flow=abc",
		},
		{
			name:       "cookie scoped to the target path",
			setCookie:  "deep=1; Path=/deep",
			start:      "/deep/start",
			target:     "/deep/target",
			wantCookie: "deep=1",
		},
		{
			name:       "cookie outside the target path",
			setCookie:  "deep=1; Path=/deep",
			start:      "/start",
			target:     "/other",
			wantCookie: "",
		},
		{
			name:       "expired cookie deleted by the redirect response",
			setCookie:  "gone=1; Max-Age=0; Path=/",
			start:      "/start",
			target:     "/target",
			wantCookie: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := redirectProbe(t, tc.setCookie, tc.start, tc.target)
			client := transport.DefaultClient()
			defer client.Close()

			resp, err := client.Get(context.Background(), srv.URL+tc.start)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if want := srv.URL + tc.target; resp.URL != want {
				t.Fatalf("final URL = %q, want %q", resp.URL, want)
			}
			if cookie := got.Load().(string); cookie != tc.wantCookie {
				t.Fatalf("redirect target Cookie = %q, want %q", cookie, tc.wantCookie)
			}
		})
	}
}

// qc:BR-157
func TestCookieAlreadyInTheJarReachesAnInScopeRedirectTarget(t *testing.T) {
	var targetCookie atomic.Value
	targetCookie.Store("")
	var bounceTarget string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/set":
			w.Header().Add("Set-Cookie", "scoped=1; Path=/app")
			_, _ = io.WriteString(w, "set")
		case "/bounce":
			http.Redirect(w, r, bounceTarget, http.StatusFound)
		case "/app/target":
			targetCookie.Store(r.Header.Get("Cookie"))
			_, _ = io.WriteString(w, "target")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	bounceTarget = "/app/target"

	client := transport.DefaultClient()
	defer client.Close()
	if _, err := client.Get(context.Background(), srv.URL+"/set"); err != nil {
		t.Fatalf("set cookie: %v", err)
	}
	if _, err := client.Get(context.Background(), srv.URL+"/bounce"); err != nil {
		t.Fatalf("bounce: %v", err)
	}
	if cookie := targetCookie.Load().(string); cookie != "scoped=1" {
		t.Fatalf("in-scope redirect target Cookie = %q, want scoped=1", cookie)
	}
}

// qc:BR-157
func TestSecureCookieSetOnTLSRedirectReachesTheTarget(t *testing.T) {
	var targetCookie atomic.Value
	targetCookie.Store("")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			w.Header().Add("Set-Cookie", "secret=1; Secure; Path=/")
			http.Redirect(w, r, "/target", http.StatusFound)
		case "/target":
			targetCookie.Store(r.Header.Get("Cookie"))
			_, _ = io.WriteString(w, "target")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	client := transport.DefaultClientWithTLS(&tls.Config{RootCAs: roots})
	defer client.Close()

	if _, err := client.Get(context.Background(), srv.URL+"/start"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cookie := targetCookie.Load().(string); cookie != "secret=1" {
		t.Fatalf("TLS redirect target Cookie = %q, want secret=1", cookie)
	}
}

// qc:BR-157 BR-158
func TestRedirectToAnotherOriginDoesNotCarryCookies(t *testing.T) {
	var foreignCookie atomic.Value
	foreignCookie.Store("")
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignCookie.Store(r.Header.Get("Cookie"))
		_, _ = io.WriteString(w, "foreign")
	}))
	defer foreign.Close()
	foreignURL, err := url.Parse(foreign.URL)
	if err != nil {
		t.Fatal(err)
	}

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "mine=1; Path=/")
		http.Redirect(w, r, "http://localhost:"+foreignURL.Port()+"/landing", http.StatusFound)
	}))
	defer origin.Close()

	client := transport.DefaultClient()
	defer client.Close()
	if _, err := client.Get(context.Background(), origin.URL+"/start"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cookie := foreignCookie.Load().(string); cookie != "" {
		t.Fatalf("cookies crossed origins on a redirect: %q", cookie)
	}
}
