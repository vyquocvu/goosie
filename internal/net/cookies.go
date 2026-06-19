package net

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
)

type CookieRecord struct {
	Name     string
	Value    string
	Domain   string
	Path     string
	Expires  time.Time
	MaxAge   int
	Secure   bool
	HttpOnly bool
	HostOnly bool
}

type CookieJar struct {
	mu      sync.Mutex
	records []CookieRecord
}

func NewCookieJar() *CookieJar {
	return &CookieJar{}
}

func (j *CookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if j == nil || u == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	now := time.Now()
	for _, cookie := range cookies {
		if cookie == nil {
			continue
		}
		if cookie.Domain != "" && !validCookieDomain(u.Hostname(), cookie.Domain) {
			continue
		}
		record := cookieRecordFromCookie(u, cookie)
		if cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && !cookie.Expires.After(now)) {
			j.remove(record)
			continue
		}
		replaced := false
		for i := range j.records {
			if j.records[i].Name == record.Name && j.records[i].Domain == record.Domain && j.records[i].Path == record.Path {
				j.records[i] = record
				replaced = true
				break
			}
		}
		if !replaced {
			j.records = append(j.records, record)
		}
	}
}

func (j *CookieJar) remove(record CookieRecord) {
	kept := j.records[:0]
	for _, existing := range j.records {
		if existing.Name == record.Name && existing.Domain == record.Domain && existing.Path == record.Path {
			continue
		}
		kept = append(kept, existing)
	}
	j.records = kept
}

func (j *CookieJar) Cookies(u *url.URL) []*http.Cookie {
	records := CookieRecordsForURL(j, u)
	cookies := make([]*http.Cookie, 0, len(records))
	for _, record := range records {
		cookies = append(cookies, &http.Cookie{Name: record.Name, Value: record.Value})
	}
	return cookies
}

func CookieRecordsForURL(jar http.CookieJar, u *url.URL) []CookieRecord {
	if jar == nil || u == nil {
		return nil
	}
	if recorder, ok := jar.(interface{ CookieRecords(*url.URL) []CookieRecord }); ok {
		return recorder.CookieRecords(u)
	}
	cookies := jar.Cookies(u)
	records := make([]CookieRecord, 0, len(cookies))
	for _, cookie := range cookies {
		records = append(records, cookieRecordFromCookie(u, cookie))
	}
	return records
}

func (j *CookieJar) CookieRecords(u *url.URL) []CookieRecord {
	if j == nil || u == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	now := time.Now()
	requestPath := cookieRequestPath(u)
	var records []CookieRecord
	for _, record := range j.records {
		if !record.Expires.IsZero() && now.After(record.Expires) {
			continue
		}
		if record.Secure && u.Scheme != "https" {
			continue
		}
		if !domainMatches(u.Hostname(), record.Domain, record.HostOnly) {
			continue
		}
		if !pathMatches(requestPath, record.Path) {
			continue
		}
		records = append(records, record)
	}
	return records
}

func cookieRecordFromCookie(u *url.URL, cookie *http.Cookie) CookieRecord {
	domain := cookie.Domain
	hostOnly := domain == ""
	if domain == "" && u != nil {
		domain = u.Hostname()
	}
	path := cookie.Path
	if path == "" {
		path = defaultCookiePath(u)
	}
	return CookieRecord{
		Name:     cookie.Name,
		Value:    cookie.Value,
		Domain:   strings.TrimPrefix(domain, "."),
		Path:     path,
		Expires:  cookie.Expires,
		MaxAge:   cookie.MaxAge,
		Secure:   cookie.Secure,
		HttpOnly: cookie.HttpOnly,
		HostOnly: hostOnly,
	}
}

func validCookieDomain(host, domain string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(domain, "."), "."))
	if host == "" || domain == "" {
		return false
	}
	if host != domain && !strings.HasSuffix(host, "."+domain) {
		return false
	}
	publicSuffix, _ := publicsuffix.PublicSuffix(domain)
	return publicSuffix != domain
}

func domainMatches(host, domain string, hostOnly bool) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(domain, "."), "."))
	if hostOnly {
		return host == domain
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func cookieRequestPath(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func defaultCookiePath(u *url.URL) string {
	if u == nil {
		return "/"
	}
	path := cookieRequestPath(u)
	if path == "/" {
		return "/"
	}
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash <= 0 {
		return "/"
	}
	return path[:lastSlash]
}

func pathMatches(requestPath, cookiePath string) bool {
	if cookiePath == "" {
		cookiePath = "/"
	}
	if requestPath == cookiePath {
		return true
	}
	if !strings.HasPrefix(requestPath, cookiePath) {
		return false
	}
	if strings.HasSuffix(cookiePath, "/") {
		return true
	}
	return len(requestPath) > len(cookiePath) && requestPath[len(cookiePath)] == '/'
}
