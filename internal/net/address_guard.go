package net

import (
	"context"
	"errors"
	"fmt"
	stdnet "net"
	"strings"
	"time"
)

// ErrBlockedAddress reports that a connection was refused because the address it
// would have gone to is one the request's guard refuses. It is a distinct error
// rather than a dial failure so a caller can tell "this browser said no" apart
// from "the server did not answer".
var ErrBlockedAddress = errors.New("net: address blocked by policy")

// AddressBlocked classifies one resolved address. It is a predicate over an
// address and not over a URL because the whole point of checking at dial time is
// that the name has already been seen and found innocent: internal/engine
// refuses a public page's subresource that *names* 169.254.169.254, and this is
// the half that catches the name that resolves to it.
type AddressBlocked func(ip stdnet.IP) bool

// AddressLookup resolves a host name to the addresses a connection may be made
// to. nil means the system resolver. Supplying one is how a test names an
// address without a DNS record it does not control, and it is the same hook a
// future proxy or --host-resolver-rules style setting would use.
type AddressLookup func(ctx context.Context, host string) ([]stdnet.IP, error)

// AddressGuard is the dial-time policy a request carries.
type AddressGuard struct {
	Blocked AddressBlocked
	Lookup  AddressLookup
}

// guardKey is private so no other package can read or forge a guard.
type guardKey struct{}

// WithAddressGuard returns ctx with g applied to every connection its requests
// make, including the connections made for each redirect hop, since those reuse
// the same context. A guard whose predicate is nil is dropped rather than
// installed: refusing nothing while still resolving and pinning would pay for
// the mechanism and buy none of the policy.
//
// One limit is worth stating: when a proxy is configured the transport dials the
// proxy, so addr names the proxy and not the origin, and this guard judges the
// proxy's address. Browsers with the same arrangement inherit the same caveat.
func WithAddressGuard(ctx context.Context, g AddressGuard) context.Context {
	if g.Blocked == nil {
		return ctx
	}
	return context.WithValue(ctx, guardKey{}, g)
}

// AddressGuardFromContext reads back what WithAddressGuard put there, which is
// how the wiring's owner (cmd/goosie) can be tested for having guarded a fetch at
// all rather than merely hoping it did.
func AddressGuardFromContext(ctx context.Context) (AddressGuard, bool) {
	g, ok := ctx.Value(guardKey{}).(AddressGuard)
	return g, ok
}

// guardDialer mirrors the dialer settings the cloned transport was built with, so
// that replacing its DialContext does not quietly change the connect timeout or
// drop TCP keepalives from the guarded path.
var guardDialer = &stdnet.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}

// dialAddressGuarded is the transport's DialContext. With no guard on the context
// it dials exactly as the standard transport would; with one, it resolves first,
// asks the guard about every answer, and then dials the address it validated
// rather than the name.
//
// Dialing the validated address is not an optimisation. Resolving, checking, and
// then handing the *name* to the dialer would resolve a second time, and the
// second answer is free to differ from the first - which is the same rebinding
// this exists to prevent, arriving as a time-of-check gap instead of as a
// DNS record.
func dialAddressGuarded(ctx context.Context, network, addr string) (stdnet.Conn, error) {
	g, ok := AddressGuardFromContext(ctx)
	if !ok {
		return guardDialer.DialContext(ctx, network, addr)
	}
	host, port, err := stdnet.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if ip := stdnet.ParseIP(host); ip != nil {
		if g.Blocked(ip) {
			return nil, fmt.Errorf("%w: %s", ErrBlockedAddress, addr)
		}
		return guardDialer.DialContext(ctx, network, addr)
	}
	lookup := g.Lookup
	if lookup == nil {
		lookup = func(ctx context.Context, host string) ([]stdnet.IP, error) {
			return stdnet.DefaultResolver.LookupIP(ctx, "ip", host)
		}
	}
	addrs, err := lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("net: resolving %s: %w", host, err)
	}
	if len(addrs) == 0 {
		// Not a refusal: nothing was judged. Falling through to a plain dial of
		// the name would be, though, since that resolves again and dials whatever
		// it gets.
		return nil, fmt.Errorf("net: %s resolved to no addresses to dial", host)
	}
	for _, ip := range addrs {
		if !g.Blocked(ip) {
			return guardDialer.DialContext(ctx, network, stdnet.JoinHostPort(ip.String(), port))
		}
	}
	return nil, fmt.Errorf("%w: %s resolves to %s", ErrBlockedAddress, host, joinIPs(addrs))
}

func joinIPs(addrs []stdnet.IP) string {
	out := make([]string, 0, len(addrs))
	for _, ip := range addrs {
		out = append(out, ip.String())
	}
	return strings.Join(out, ", ")
}
