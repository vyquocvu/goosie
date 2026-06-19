package net

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestCookieRecordsFromJar(t *testing.T) {
	jar := NewCookieJar()
	u, err := url.Parse("https://example.test/path")
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour).UTC()
	jar.SetCookies(u, []*http.Cookie{{
		Name:     "session",
		Value:    "abc123",
		Path:     "/",
		Domain:   "example.test",
		Expires:  expires,
		Secure:   true,
		HttpOnly: true,
	}})

	records := CookieRecordsForURL(jar, u)
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Name != "session" || record.Value != "abc123" {
		t.Fatalf("record = %#v", record)
	}
	if !record.Secure {
		t.Error("Secure = false, want true")
	}
	if !record.Expires.Equal(expires) {
		t.Errorf("Expires = %v, want %v", record.Expires, expires)
	}
}

func TestCookieRecordsRootPathMatchesEmptyURLPath(t *testing.T) {
	jar := NewCookieJar()
	origin, err := url.Parse("https://example.test/login")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{
		Name:   "root",
		Value:  "1",
		Path:   "/",
		Domain: "example.test",
		Secure: true,
	}})
	target, err := url.Parse("https://example.test")
	if err != nil {
		t.Fatal(err)
	}

	records := CookieRecordsForURL(jar, target)

	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Name != "root" {
		t.Fatalf("record = %#v", records[0])
	}
}
