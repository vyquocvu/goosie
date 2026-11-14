package net_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	transport "github.com/vyquocvu/goosie/internal/net"
)

type rebindServer struct {
	srv    *httptest.Server
	hits   atomic.Int64
	hosts  atomic.Value
	target string
}

func newRebindServer(t *testing.T, body string) *rebindServer {
	t.Helper()
	r := &rebindServer{}
	r.hosts.Store("")
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits.Add(1)
		r.hosts.Store(req.Host)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(r.srv.Close)
	u, err := url.Parse(r.srv.URL)
	if err != nil {
		t.Fatalf("httptest URL %q: %v", r.srv.URL, err)
	}
	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("httptest host %q has no port: %v", u.Host, err)
	}
	r.target = "http://rebind.example:" + port
	return r
}

func (r *rebindServer) loopbackNames() []net.IP {
	return []net.IP{net.ParseIP("127.0.0.1")}
}

func lookupFor(hosts map[string][]net.IP) (transport.AddressLookup, *atomic.Int64) {
	var lookups atomic.Int64
	return func(ctx context.Context, host string) ([]net.IP, error) {
		lookups.Add(1)
		addrs, ok := hosts[host]
		if !ok {
			return nil, fmt.Errorf("unexpected lookup of %q", host)
		}
		return addrs, nil
	}, &lookups
}

func TestAddressGuardRefusesAHostThatResolvesToABlockedAddress(t *testing.T) {
	r := newRebindServer(t, "private")
	lookup, lookups := lookupFor(map[string][]net.IP{"rebind.example": r.loopbackNames()})
	ctx := transport.WithAddressGuard(context.Background(), transport.AddressGuard{
		Blocked: func(ip net.IP) bool { return ip.IsLoopback() },
		Lookup:  lookup,
	})
	client := transport.DefaultClient()
	defer client.Close()

	if _, err := client.Get(ctx, r.target); err == nil {
		t.Fatalf("got a response from %s, want a refusal: the name is public but the address is loopback", r.target)
	} else if !errors.Is(err, transport.ErrBlockedAddress) {
		t.Errorf("err = %v, want it to report a blocked address, because that is the difference between this browser refusing and this browser failing", err)
	}
	if got := r.hits.Load(); got != 0 {
		t.Errorf("the private endpoint served %d requests, want 0", got)
	}
	if got := lookups.Load(); got != 1 {
		t.Errorf("resolved %d times, want 1", got)
	}
}

func TestAddressGuardDialsTheAddressItValidated(t *testing.T) {
	r := newRebindServer(t, "private")
	lookup, lookups := lookupFor(map[string][]net.IP{"rebind.example": r.loopbackNames()})
	ctx := transport.WithAddressGuard(context.Background(), transport.AddressGuard{
		Blocked: func(ip net.IP) bool { return ip.Equal(net.ParseIP("1.2.3.4")) },
		Lookup:  lookup,
	})
	client := transport.DefaultClient()
	defer client.Close()

	resp, err := client.Get(ctx, r.target)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(resp.Body) != "private" {
		t.Errorf("body = %q, want the body the loopback server serves", resp.Body)
	}
	if got := lookups.Load(); got != 1 {
		t.Errorf("resolved %d times, want 1: the address checked has to be the address dialed", got)
	}
	host, _, err := net.SplitHostPort(r.hosts.Load().(string))
	if err != nil || host != "rebind.example" {
		t.Errorf("the server saw Host %q, want rebind.example: pinning the address must not rewrite the name (%v)", r.hosts.Load(), err)
	}
}

func TestAddressGuardSkipsABlockedAddressAndDialsTheNextOne(t *testing.T) {
	r := newRebindServer(t, "v4")
	lookup, lookups := lookupFor(map[string][]net.IP{
		"rebind.example": {net.ParseIP("::1"), net.ParseIP("127.0.0.1")},
	})
	ctx := transport.WithAddressGuard(context.Background(), transport.AddressGuard{
		Blocked: func(ip net.IP) bool { return ip.To4() == nil },
		Lookup:  lookup,
	})
	client := transport.DefaultClient()
	defer client.Close()

	resp, err := client.Get(ctx, r.target)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(resp.Body) != "v4" {
		t.Errorf("body = %q", resp.Body)
	}
	if got := r.hits.Load(); got != 1 {
		t.Errorf("the endpoint served %d requests, want 1", got)
	}
	if got := lookups.Load(); got != 1 {
		t.Errorf("resolved %d times, want 1", got)
	}
}

// qc:BR-161
func TestAddressGuardChecksALiteralAddressWithoutResolving(t *testing.T) {
	r := newRebindServer(t, "private")
	lookup, lookups := lookupFor(map[string][]net.IP{})
	ctx := transport.WithAddressGuard(context.Background(), transport.AddressGuard{
		Blocked: func(ip net.IP) bool { return ip.IsLoopback() },
		Lookup:  lookup,
	})
	client := transport.DefaultClient()
	defer client.Close()

	_, err := client.Get(ctx, r.srv.URL)
	if err == nil {
		t.Fatal("a literal loopback address passed a guard that refuses loopback")
	}
	if !errors.Is(err, transport.ErrBlockedAddress) {
		t.Errorf("err = %v, want a blocked-address error", err)
	}
	if got := lookups.Load(); got != 0 {
		t.Errorf("resolved %d times, want 0: an address literal is not a name", got)
	}
}

// qc:BR-161
func TestAddressGuardRefusesIPv6LiteralsAndEncodedIPv4(t *testing.T) {
	r := newRebindServer(t, "private")
	u, err := url.Parse(r.srv.URL)
	if err != nil {
		t.Fatalf("httptest URL %q: %v", r.srv.URL, err)
	}
	_, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("httptest host %q has no port: %v", u.Host, err)
	}

	literals := []struct {
		name string
		url  string
	}{
		{"ipv6 loopback", "http://[::1]:" + port + "/"},
		{"ipv4-mapped ipv6", "http://[::ffff:127.0.0.1]:" + port + "/"},
		{"unique local ipv6", "http://[fd00::1]:" + port + "/"},
		{"link-local ipv6", "http://[fe80::1]:" + port + "/"},
	}
	for _, tc := range literals {
		t.Run(tc.name, func(t *testing.T) {
			r.hits.Store(0)
			lookup, lookups := lookupFor(map[string][]net.IP{})
			ctx := transport.WithAddressGuard(context.Background(), transport.AddressGuard{
				Blocked: engine.AddressBlocked,
				Lookup:  lookup,
			})
			client := transport.DefaultClient()
			defer client.Close()

			_, err := client.Get(ctx, tc.url)
			if err == nil {
				t.Fatalf("%s passed a guard that refuses every non-public address", tc.url)
			}
			if !errors.Is(err, transport.ErrBlockedAddress) {
				t.Errorf("err = %v, want a blocked-address error", err)
			}
			if got := lookups.Load(); got != 0 {
				t.Errorf("resolved %d times, want 0: an address literal is not a name", got)
			}
			if got := r.hits.Load(); got != 0 {
				t.Errorf("the private endpoint served %d requests, want 0", got)
			}
		})
	}

	for _, host := range []string{"2130706433", "0x7f000001", "0x7f.0.0.1", "017700000001"} {
		t.Run("encoded "+host, func(t *testing.T) {
			r.hits.Store(0)
			lookup, lookups := lookupFor(map[string][]net.IP{host: {net.ParseIP("127.0.0.1")}})
			ctx := transport.WithAddressGuard(context.Background(), transport.AddressGuard{
				Blocked: engine.AddressBlocked,
				Lookup:  lookup,
			})
			client := transport.DefaultClient()
			defer client.Close()

			target := "http://" + host + ":" + port + "/"
			_, err := client.Get(ctx, target)
			if err == nil {
				t.Fatalf("%s passed a guard that refuses the loopback address it resolves to", target)
			}
			if !errors.Is(err, transport.ErrBlockedAddress) {
				t.Errorf("err = %v, want a blocked-address error", err)
			}
			if got := lookups.Load(); got != 1 {
				t.Errorf("resolved %d times, want 1", got)
			}
			if got := r.hits.Load(); got != 0 {
				t.Errorf("the private endpoint served %d requests, want 0", got)
			}
		})
	}

	t.Run("dotted shorthand rejected before dial", func(t *testing.T) {
		r.hits.Store(0)
		lookup, lookups := lookupFor(map[string][]net.IP{})
		ctx := transport.WithAddressGuard(context.Background(), transport.AddressGuard{
			Blocked: engine.AddressBlocked,
			Lookup:  lookup,
		})
		client := transport.DefaultClient()
		defer client.Close()

		_, err := client.Get(ctx, "http://127.1:"+port+"/")
		if err == nil {
			t.Fatal("the legacy two-part loopback spelling was accepted")
		}
		if !strings.Contains(err.Error(), "invalid IPv4 hostname") {
			t.Errorf("err = %v, want the validator's invalid-IPv4 refusal", err)
		}
		if got := lookups.Load(); got != 0 {
			t.Errorf("resolved %d times, want 0: the host never reached the resolver", got)
		}
		if got := r.hits.Load(); got != 0 {
			t.Errorf("the private endpoint served %d requests, want 0", got)
		}
	})
}

func TestAddressGuardWithoutAGuardDialsNormally(t *testing.T) {
	r := newRebindServer(t, "control")
	client := transport.DefaultClient()
	defer client.Close()

	resp, err := client.Get(context.Background(), r.srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(resp.Body) != "control" {
		t.Errorf("body = %q", resp.Body)
	}
	if got := r.hits.Load(); got != 1 {
		t.Errorf("the endpoint served %d requests, want 1", got)
	}
}

func TestAddressGuardRefusesANameThatResolvesToNothing(t *testing.T) {
	r := newRebindServer(t, "private")
	lookup, lookups := lookupFor(map[string][]net.IP{"rebind.example": {}})
	ctx := transport.WithAddressGuard(context.Background(), transport.AddressGuard{
		Blocked: func(ip net.IP) bool { return ip.IsLoopback() },
		Lookup:  lookup,
	})
	client := transport.DefaultClient()
	defer client.Close()

	_, err := client.Get(ctx, r.target)
	if err == nil {
		t.Fatal("a name with no addresses was served from somewhere the guard never looked")
	}
	if errors.Is(err, transport.ErrBlockedAddress) {
		t.Errorf("err = %v, want a resolution failure rather than a policy refusal: no address was refused here", err)
	}
	if got := r.hits.Load(); got != 0 {
		t.Errorf("the private endpoint served %d requests, want 0", got)
	}
	if got := lookups.Load(); got != 1 {
		t.Errorf("resolved %d times, want 1", got)
	}
}

// qc:BR-160
func TestAddressGuardChecksTheRedirectDestination(t *testing.T) {
	guard := transport.AddressGuard{
		Blocked: func(ip net.IP) bool { return ip.IsPrivate() },
	}

	t.Run("destination blocked", func(t *testing.T) {
		var publicHits atomic.Int64
		public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			publicHits.Add(1)
			http.Redirect(w, r, "http://private.example:8080/secret", http.StatusFound)
		}))
		defer public.Close()

		lookup, lookups := lookupFor(map[string][]net.IP{
			"public.example":  {net.ParseIP("127.0.0.1")},
			"private.example": {net.ParseIP("10.0.0.5")},
		})
		guard.Lookup = lookup
		ctx := transport.WithAddressGuard(context.Background(), guard)
		client := transport.DefaultClient()
		defer client.Close()

		u, err := url.Parse(public.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Get(ctx, "http://public.example:"+u.Port()+"/start")
		if err == nil {
			t.Fatal("a redirect to a private address was followed")
		}
		if !errors.Is(err, transport.ErrBlockedAddress) {
			t.Errorf("err = %v, want a blocked-address error at the destination", err)
		}
		if got := publicHits.Load(); got != 1 {
			t.Errorf("the public server served %d requests, want 1", got)
		}
		if got := lookups.Load(); got != 2 {
			t.Errorf("resolved %d names, want 2: both hops have to be judged", got)
		}
	})

	t.Run("allowed destination", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("allowed"))
		}))
		defer target.Close()
		tu, err := url.Parse(target.URL)
		if err != nil {
			t.Fatal(err)
		}

		public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://allowed.example:"+tu.Port()+"/secret", http.StatusFound)
		}))
		defer public.Close()
		pu, err := url.Parse(public.URL)
		if err != nil {
			t.Fatal(err)
		}

		lookup, lookups := lookupFor(map[string][]net.IP{
			"public.example":  {net.ParseIP("127.0.0.1")},
			"allowed.example": {net.ParseIP("127.0.0.1")},
		})
		guard.Lookup = lookup
		ctx := transport.WithAddressGuard(context.Background(), guard)
		client := transport.DefaultClient()
		defer client.Close()

		resp, err := client.Get(ctx, "http://public.example:"+pu.Port()+"/start")
		if err != nil {
			t.Fatalf("an allowed redirect was refused: %v", err)
		}
		if string(resp.Body) != "allowed" {
			t.Errorf("body = %q, want the destination's body", resp.Body)
		}
		if got := lookups.Load(); got != 2 {
			t.Errorf("resolved %d names, want 2", got)
		}
	})
}

func TestAddressGuardKeepsTheNameForTLS(t *testing.T) {
	var sni atomic.Value
	sni.Store("")
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.TLS != nil {
			sni.Store(req.TLS.ServerName)
		}
		_, _ = w.Write([]byte("secure"))
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("httptest URL %q: %v", srv.URL, err)
	}

	lookup, lookups := lookupFor(map[string][]net.IP{"rebind.example": {net.ParseIP("127.0.0.1")}})
	ctx := transport.WithAddressGuard(context.Background(), transport.AddressGuard{
		Blocked: func(ip net.IP) bool { return ip.Equal(net.ParseIP("1.2.3.4")) },
		Lookup:  lookup,
	})
	client := transport.DefaultClientWithTLS(&tls.Config{InsecureSkipVerify: true})
	defer client.Close()

	resp, err := client.Get(ctx, "https://rebind.example:"+u.Port())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(resp.Body) != "secure" {
		t.Errorf("body = %q, want the body the loopback TLS server serves", resp.Body)
	}
	if got := lookups.Load(); got != 1 {
		t.Errorf("resolved %d times, want 1", got)
	}
	if got := sni.Load().(string); got != "rebind.example" {
		t.Errorf("the TLS server was introduced to %q, want rebind.example: the guard dials an address, but the page still names a host", got)
	}
}
