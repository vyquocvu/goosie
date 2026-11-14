package engine_test

import (
	"net"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
)

func addressOf(authority string) net.IP {
	host := authority
	if h, _, err := net.SplitHostPort(authority); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	return net.ParseIP(host)
}

func TestAddressBlockedAgreesWithTheResolutionRule(t *testing.T) {
	var checked int
	for _, authority := range localAuthorities {
		ip := addressOf(authority)
		if ip == nil {
			continue
		}
		checked++
		if !engine.AddressBlocked(ip) {
			t.Errorf("AddressBlocked(%s) = false, want true: %s is refused at resolution time, so the same address reached by DNS has to be refused at dial time too", ip, authority)
		}
	}
	if checked < 10 {
		t.Fatalf("only %d of the %d localAuthorities are addresses, so this compares the two rules against each other on a handful of cases and proves nothing", checked, len(localAuthorities))
	}
}

func TestAddressBlockedAllowsPublicAddresses(t *testing.T) {
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111", "2001:4860:4860::8888"} {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("net.ParseIP(%q) = nil", s)
		}
		if engine.AddressBlocked(ip) {
			t.Errorf("AddressBlocked(%s) = true, want false", s)
		}
	}
}

func TestInitiatorIsLocal(t *testing.T) {
	cases := []struct {
		base string
		want bool
	}{
		{"http://127.0.0.1:8080/page", true},
		{"https://localhost:3000/", true},
		{"http://192.168.1.10/index.html", true},
		{"http://[::1]:8080/", true},
		{"http://printer.local/", true},
		{"file:///Users/me/notes/index.html", true},
		{"http://public.example/", false},
		{"https://8.8.8.8/", false},
		{"", false},
		{"%not a url", false},
	}
	for _, tc := range cases {
		if got := engine.InitiatorIsLocal(tc.base); got != tc.want {
			t.Errorf("InitiatorIsLocal(%q) = %v, want %v", tc.base, got, tc.want)
		}
	}
}
