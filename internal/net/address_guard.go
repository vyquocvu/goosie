package net

import (
	"context"
	"errors"
	"fmt"
	stdnet "net"
	"strings"
	"time"
)

var ErrBlockedAddress = errors.New("net: address blocked by policy")

type AddressBlocked func(ip stdnet.IP) bool

type AddressLookup func(ctx context.Context, host string) ([]stdnet.IP, error)

type AddressGuard struct {
	Blocked AddressBlocked
	Lookup  AddressLookup
}

type guardKey struct{}

func WithAddressGuard(ctx context.Context, g AddressGuard) context.Context {
	if g.Blocked == nil {
		return ctx
	}
	return context.WithValue(ctx, guardKey{}, g)
}

func AddressGuardFromContext(ctx context.Context) (AddressGuard, bool) {
	g, ok := ctx.Value(guardKey{}).(AddressGuard)
	return g, ok
}

var guardDialer = &stdnet.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}

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
