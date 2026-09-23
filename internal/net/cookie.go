package net

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cookie limits are explicit units, not convenience constants: a hostile page
// can set thousands of cookies per response, and an unbounded jar grows with
// every navigation.
const (
	MaxCookiesPerHost = 50
	MaxTotalCookies   = 3000
)

// Cookie is one stored cookie. HttpOnly and SameSite are parsed and kept for a
// future scripting/cross-site boundary; nothing today can read a cookie except
// this client's own requests.
type Cookie struct {
	Name     string
	Value    string
	Domain   string // empty means host-only
	Path     string
	Secure   bool
	HttpOnly bool
	Expires  time.Time // zero means session cookie
	hostOnly bool
}

// CookieJar stores cookies between requests with policy enforcement: a Domain
// attribute must match the host that set it, Secure cookies travel only over
// HTTPS, and jar size is capped per host and in total.
type CookieJar struct {
	mu      sync.Mutex
	entries map[string]*Cookie
}

func NewCookieJar() *CookieJar {
	return &CookieJar{entries: make(map[string]*Cookie)}
}

func cookieKey(host, name, domain, path string) string {
	return host + "\x00" + name + "\x00" + domain + "\x00" + path
}

// Store parses raw Set-Cookie header values from a response served by reqURL
// and keeps every cookie that passes policy. Invalid attributes reject only
// that cookie, never the rest of the response.
func (j *CookieJar) Store(reqURL *url.URL, setCookies []string) {
	if j == nil {
		return
	}
	host := strings.ToLower(reqURL.Hostname())
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, raw := range setCookies {
		c := parseSetCookie(raw)
		if c == nil {
			continue
		}
		if c.Domain != "" {
			// Cookie tossing guard: a Domain attribute must be this host or a
			// parent of it, and a bare TLD ("Domain=com") is refused so one
			// site cannot claim cookies for every .com host.
			if !hostMatchesDomain(host, c.Domain) || !strings.Contains(c.Domain, ".") {
				continue
			}
			c.hostOnly = false
		} else {
			c.Domain = host
			c.hostOnly = true
		}
		if c.Path == "" {
			c.Path = defaultCookiePath(reqURL.Path)
		} else if !strings.HasPrefix(c.Path, "/") {
			continue
		}
		if c.Expires.Before(time.Now()) && !c.Expires.IsZero() {
			delete(j.entries, cookieKey(host, c.Name, c.Domain, c.Path))
			continue
		}
		// (name, domain, path) identifies a cookie whatever host first stored
		// it; a cookie loaded from disk under its Domain must be replaced, not
		// duplicated, when a later response re-Sets it.
		for k, old := range j.entries {
			if old.Name == c.Name && old.Domain == c.Domain && old.Path == c.Path {
				delete(j.entries, k)
			}
		}
		j.entries[cookieKey(host, c.Name, c.Domain, c.Path)] = c
		j.evict(host)
	}
}

// HeaderFor returns the Cookie header value for a request to reqURL, or ""
// when no stored cookie applies.
func (j *CookieJar) HeaderFor(reqURL *url.URL) string {
	if j == nil {
		return ""
	}
	host := strings.ToLower(reqURL.Hostname())
	secure := reqURL.Scheme == "https"
	path := reqURL.Path
	if path == "" {
		path = "/"
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	var matches []*Cookie
	for _, c := range j.entries {
		if c.Secure && !secure {
			continue
		}
		if c.hostOnly {
			if c.Domain != host {
				continue
			}
		} else if !hostMatchesDomain(host, c.Domain) {
			continue
		}
		if !cookiePathMatch(path, c.Path) {
			continue
		}
		if !c.Expires.IsZero() && c.Expires.Before(time.Now()) {
			continue
		}
		matches = append(matches, c)
	}
	if len(matches) == 0 {
		return ""
	}
	sort.SliceStable(matches, func(i, k int) bool {
		if len(matches[i].Path) != len(matches[k].Path) {
			return len(matches[i].Path) > len(matches[k].Path)
		}
		return matches[i].Expires.Before(matches[k].Expires)
	})
	pairs := make([]string, len(matches))
	for i, c := range matches {
		pairs[i] = c.Name + "=" + c.Value
	}
	return strings.Join(pairs, "; ")
}

// Clear drops every stored cookie, including session cookies.
func (j *CookieJar) Clear() {
	if j == nil {
		return
	}
	j.mu.Lock()
	j.entries = make(map[string]*Cookie)
	j.mu.Unlock()
}

// persistedCookie is the on-disk form of a stored cookie.
type persistedCookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Secure   bool      `json:"secure,omitempty"`
	HttpOnly bool      `json:"httpOnly,omitempty"`
	Expires  time.Time `json:"expires"`
	HostOnly bool      `json:"hostOnly,omitempty"`
}

// Save writes the jar's persistent cookies to path as JSON, atomically.
// Session cookies are dropped by design: they end with the browsing session,
// and an exit save is that end.
func (j *CookieJar) Save(path string) error {
	if j == nil {
		return nil
	}
	j.mu.Lock()
	out := make([]persistedCookie, 0, len(j.entries))
	for _, c := range j.entries {
		if c.Expires.IsZero() || c.Expires.Before(time.Now()) {
			continue
		}
		out = append(out, persistedCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HttpOnly: c.HttpOnly,
			Expires:  c.Expires,
			HostOnly: c.hostOnly,
		})
	}
	j.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cookies-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Load merges cookies written by Save into the jar. A missing file is the
// normal first-run case, not an error; expired and nameless entries are
// dropped rather than stored. Domain-keyed entries loaded here are replaced,
// never duplicated, when a later response re-Sets the same cookie.
func (j *CookieJar) Load(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var in []persistedCookie
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, pc := range in {
		if pc.Name == "" || pc.Domain == "" {
			continue
		}
		if !pc.Expires.IsZero() && pc.Expires.Before(time.Now()) {
			continue
		}
		if pc.Path == "" {
			pc.Path = "/"
		}
		j.entries[cookieKey(pc.Domain, pc.Name, pc.Domain, pc.Path)] = &Cookie{
			Name:     pc.Name,
			Value:    pc.Value,
			Domain:   pc.Domain,
			Path:     pc.Path,
			Secure:   pc.Secure,
			HttpOnly: pc.HttpOnly,
			Expires:  pc.Expires,
			hostOnly: pc.HostOnly,
		}
	}
	return nil
}

// evict enforces the jar limits, called with the lock held. Expired entries go
// first, then the soonest-to-expire, then the longest idle ones.
func (j *CookieJar) evict(host string) {
	total := len(j.entries)
	var perHost int
	var candidates []*Cookie
	for k, c := range j.entries {
		if strings.HasPrefix(k, host+"\x00") {
			perHost++
		}
		if !c.Expires.IsZero() && c.Expires.Before(time.Now()) {
			delete(j.entries, k)
			total--
			if strings.HasPrefix(k, host+"\x00") {
				perHost--
			}
			continue
		}
		candidates = append(candidates, c)
	}
	for perHost > MaxCookiesPerHost || total > MaxTotalCookies {
		if len(candidates) == 0 {
			return
		}
		victim := candidates[0]
		victimKey := ""
		for k, c := range j.entries {
			if c == victim {
				victimKey = k
				break
			}
		}
		if victimKey == "" {
			return
		}
		delete(j.entries, victimKey)
		total--
		if strings.HasPrefix(victimKey, host+"\x00") {
			perHost--
		}
		candidates = candidates[1:]
	}
}

func hostMatchesDomain(host, domain string) bool {
	domain = strings.TrimPrefix(domain, ".")
	if domain == host {
		return true
	}
	return strings.HasSuffix(host, "."+domain)
}

// defaultCookiePath is the directory portion of the request path (RFC 6265
// 5.1.4): "/a/b" sets "/a/", "/" and paths without a slash set "/".
func defaultCookiePath(reqPath string) string {
	if i := strings.LastIndex(reqPath, "/"); i >= 0 {
		return reqPath[:i+1]
	}
	return "/"
}

// cookiePathMatch implements RFC 6265 5.1.4 path comparison.
func cookiePathMatch(reqPath, cookiePath string) bool {
	if reqPath == cookiePath {
		return true
	}
	if !strings.HasPrefix(reqPath, cookiePath) {
		return false
	}
	if strings.HasSuffix(cookiePath, "/") {
		return true
	}
	return len(reqPath) > len(cookiePath) && reqPath[len(cookiePath)] == '/'
}

// parseSetCookie parses one Set-Cookie value. nil means the value is invalid
// or carries nothing usable.
func parseSetCookie(raw string) *Cookie {
	pair, attrs, _ := strings.Cut(raw, ";")
	name, value, _ := strings.Cut(pair, "=")
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t") {
		return nil
	}
	c := &Cookie{
		Name:  name,
		Value: strings.TrimSpace(value),
		Path:  "",
	}
	for _, attr := range strings.Split(attrs, ";") {
		key, val, _ := strings.Cut(attr, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "domain":
			c.Domain = strings.ToLower(strings.TrimPrefix(val, "."))
		case "path":
			c.Path = val
		case "secure":
			c.Secure = true
		case "httponly":
			c.HttpOnly = true
		case "max-age":
			secs, err := strconv.Atoi(val)
			if err != nil || secs < 0 {
				continue
			}
			if secs == 0 {
				c.Expires = time.Unix(1, 0)
			} else {
				c.Expires = time.Now().Add(time.Duration(secs) * time.Second)
			}
		case "expires":
			if t, err := http.ParseTime(val); err == nil {
				c.Expires = t
			}
		}
	}
	return c
}
