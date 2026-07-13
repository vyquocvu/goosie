package navigation

import (
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// Origin represents a web origin (scheme + host + port) as defined by
// RFC 6454. The zero value is an invalid origin.
type Origin struct {
	scheme string
	host   string
	port   string
}

// ErrOpaqueOrigin is returned when a URL produces an opaque origin
// (data:, javascript:, about:blank, etc.) with no scheme and host.
var ErrOpaqueOrigin = errOpaqueOrigin()

func errOpaqueOrigin() error { return opaqueOriginError{} }

type opaqueOriginError struct{}

func (opaqueOriginError) Error() string { return "opaque origin" }

// ParseOrigin parses rawURL and returns its Origin. Opaque origins
// (data:, javascript:, about:blank, etc.) are returned as invalid
// origins without error. Returns an error only for malformed URLs.
func ParseOrigin(rawURL string) (Origin, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return Origin{}, err
	}
	return OriginFromURL(u), nil
}

// OriginFromURL extracts the Origin from a parsed URL. Returns an invalid
// Origin when u is nil or has no scheme or host.
func OriginFromURL(u *url.URL) Origin {
	if u == nil || u.Scheme == "" || u.Host == "" {
		return Origin{}
	}
	host, port := splitHostPort(u.Host)
	host = strings.ToLower(host)
	return Origin{scheme: u.Scheme, host: host, port: port}
}

// splitHostPort splits a host:port string into host and port. When there
// is no port, the entire string is returned as host.
func splitHostPort(hostport string) (host, port string) {
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		return h, p
	}
	return hostport, ""
}

// String returns the canonical serialization of the origin
// (e.g., "https://example.com" or "https://example.com:8443").
// Default ports (80 for http, 443 for https) are omitted.
// An invalid origin returns the empty string.
func (o Origin) String() string {
	if !o.IsValid() {
		return ""
	}
	if o.port == "" || isDefaultPort(o.scheme, o.port) {
		return o.scheme + "://" + formatHost(o.host)
	}
	return o.scheme + "://" + net.JoinHostPort(o.host, o.port)
}

// formatHost wraps IPv6 addresses in brackets for serialization.
func formatHost(host string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

// IsValid reports whether the origin has both scheme and host.
func (o Origin) IsValid() bool {
	return o.scheme != "" && o.host != ""
}

// IsOpaque reports whether the origin is opaque. Opaque origins
// (data:, javascript:, about:blank, etc.) cannot be serialized
// as scheme://host and are never same-origin with anything.
func (o Origin) IsOpaque() bool {
	return !o.IsValid()
}

// Host returns the host portion of the origin (without port).
func (o Origin) Host() string {
	return o.host
}

// Scheme returns the scheme portion of the origin.
func (o Origin) Scheme() string {
	return o.scheme
}

// Port returns the port portion of the origin, or the empty string
// when no explicit port was given.
func (o Origin) Port() string {
	return o.port
}

// IsSameOrigin reports whether o and other are the same origin per
// RFC 6454. Two opaque origins are never the same.
func (o Origin) IsSameOrigin(other Origin) bool {
	if o.IsOpaque() || other.IsOpaque() {
		return false
	}
	return o.scheme == other.scheme &&
		o.host == other.host &&
		normalizePort(o.scheme, o.port) == normalizePort(other.scheme, other.port)
}

// normalizePort returns the canonical port for comparison. Default ports
// are normalized to empty string so http://example.com:80 and
// http://example.com are the same origin.
func normalizePort(scheme, port string) string {
	if port == "" || isDefaultPort(scheme, port) {
		return ""
	}
	return port
}

func isDefaultPort(scheme, port string) bool {
	switch scheme {
	case "http":
		return port == "80"
	case "https":
		return port == "443"
	}
	return false
}

// isIPHost reports whether host is a literal IP address (IPv4 or IPv6).
func isIPHost(host string) bool {
	return net.ParseIP(host) != nil
}

// RegistrableDomain returns the registrable domain (eTLD+1) for the origin
// using the Public Suffix List. Returns ("", false) for opaque origins,
// IP addresses, localhost, and bare public suffixes (e.g., "com").
func (o Origin) RegistrableDomain() (string, bool) {
	if !o.IsValid() || isIPHost(o.host) {
		return "", false
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(o.host)
	if err != nil {
		return "", false
	}
	return domain, true
}

// IsSameSite reports whether o and other share the same registrable domain
// (eTLD+1). Unlike IsSameOrigin, it ignores scheme and port — two origins
// with different schemes but the same registrable domain are same-site.
// Opaque origins are never same-site with anything.
func (o Origin) IsSameSite(other Origin) bool {
	a, ok := o.RegistrableDomain()
	if !ok {
		return false
	}
	b, ok := other.RegistrableDomain()
	if !ok {
		return false
	}
	return a == b
}
