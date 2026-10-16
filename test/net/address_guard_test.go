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
	"sync/atomic"
	"testing"

	transport "github.com/vyquocvu/goosie/internal/net"
)

// The tests here are about *where a connection goes*, not about what a URL says.
//
// internal/engine refuses a public page's subresource when the href *names* a
// local address (see test/engine/private_network_test.go). That check can only
// read the name. A host that reads as public and resolves as private - the
// attacker registering rebinding.example with an A record of 127.0.0.1, or one
// that simply changes its record between the check and the connect - walks past
// it and reaches the viewer's machine, which is where cloud metadata services
// answer. The only place that can see the difference is the dial itself.
//
// So the client takes a guard on the request context: a predicate over resolved
// addresses plus, because a test cannot otherwise name an address without
// touching DNS, the lookup to run it against.

// rebindServer is the private endpoint a public-looking name points at: a real
// HTTP server on loopback, counted, plus the URL to ask for it by name.
type rebindServer struct {
	srv   *httptest.Server
	hits  atomic.Int64
	hosts atomic.Value // string: the Host header of the last request
	// target is the same endpoint addressed by a name no local-address rule
	// would refuse: http://rebind.example:<port>.
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

// lookupFor returns the addresses for every name the tests ask for, and counts
// how many times the guard resolved anything at all. The count is the point:
// validating one answer and dialing a second, separately resolved one is the
// time-of-check gap this is meant to close, so a guard that resolves twice has
// not closed it however correctly it refuses.
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

// TestAddressGuardDialsTheAddressItValidated is the non-vacuity half: the same
// machinery with a predicate that refuses nothing has to keep working, and do it
// in one resolution. A guard that only ever refuses proves nothing about the
// connect, and one that resolves twice describes an address it then ignores.
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
	// The name has to survive the pinning, or virtual hosts and TLS server names
	// would break: the request goes to 127.0.0.1 while still asking for
	// rebind.example.
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
		// Every IPv6 address is refused, so the only way this request can
		// succeed is by going on to the second answer.
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

// TestAddressGuardChecksALiteralAddressWithoutResolving: 127.0.0.1 written into
// the URL needs no lookup, and a guard that performed one anyway would be
// reporting a resolution error for a name that is already an address.
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

// TestAddressGuardWithoutAGuardDialsNormally is the control for every case
// above: the refusals have to be the guard's doing and not something the client
// already did to a loopback address.
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

// TestAddressGuardRefusesANameThatResolvesToNothing: an empty answer must not
// fall through to an unvalidated dial of the name, which is the one path on
// which the guard would be bypassed rather than merely unused. It is also not a
// blocked address, because nothing was refused on policy grounds and a log that
// conflates the two reports a NXDOMAIN as an attack.
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

// TestAddressGuardKeepsTheNameForTLS: pinning the validated address moves the
// connection, and it must move nothing else. HTTPS needs the name twice - as the
// server name indication and as the certificate's expected identity - so a guard
// that handed the dialed IP to TLS instead would make every guarded subresource
// behind a name-verifying host fail. InsecureSkipVerify here is only about not
// bundling httptest's CA; the thing under test is the name the server was
// introduced to, not the trust decision.
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
