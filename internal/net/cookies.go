package net

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type CookieRecord struct {
	Name     string
	Value    string
	Domain   string
	Path     string
	Expires  time.Time
	Secure   bool
	HttpOnly bool
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
	for _, cookie := range cookies {
		if cookie == nil {
			continue
		}
		record := cookieRecordFromCookie(u, cookie)
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
		if !domainMatches(u.Hostname(), record.Domain) {
			continue
		}
		if !strings.HasPrefix(requestPath, record.Path) {
			continue
		}
		records = append(records, record)
	}
	return records
}

func cookieRecordFromCookie(u *url.URL, cookie *http.Cookie) CookieRecord {
	domain := cookie.Domain
	if domain == "" && u != nil {
		domain = u.Hostname()
	}
	path := cookie.Path
	if path == "" {
		path = "/"
	}
	return CookieRecord{
		Name:     cookie.Name,
		Value:    cookie.Value,
		Domain:   strings.TrimPrefix(domain, "."),
		Path:     path,
		Expires:  cookie.Expires,
		Secure:   cookie.Secure,
		HttpOnly: cookie.HttpOnly,
	}
}

func domainMatches(host, domain string) bool {
	domain = strings.TrimPrefix(domain, ".")
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func cookieRequestPath(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}
