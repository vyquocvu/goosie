package net_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"
	"github.com/vyquocvu/goosie/internal/net"
)

func TestCookieRecordsFromJar(t *testing.T) {
	jar := net.NewCookieJar()
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

	records := net.CookieRecordsForURL(jar, u)
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
	jar := net.NewCookieJar()
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

	records := net.CookieRecordsForURL(jar, target)

	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Name != "root" {
		t.Fatalf("record = %#v", records[0])
	}
}

func TestCookieRecordsHostOnlyCookieDoesNotMatchSubdomain(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://example.test/account")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{
		Name:   "hostonly",
		Value:  "1",
		Path:   "/",
		MaxAge: 300,
	}})

	sameHost, err := url.Parse("https://example.test/settings")
	if err != nil {
		t.Fatal(err)
	}
	if records := net.CookieRecordsForURL(jar, sameHost); len(records) != 1 {
		t.Fatalf("same-host records = %d, want 1", len(records))
	} else if records[0].MaxAge != 300 {
		t.Fatalf("MaxAge = %d, want 300", records[0].MaxAge)
	}

	subdomain, err := url.Parse("https://sub.example.test/settings")
	if err != nil {
		t.Fatal(err)
	}
	if records := net.CookieRecordsForURL(jar, subdomain); len(records) != 0 {
		t.Fatalf("subdomain records = %d, want 0", len(records))
	}
}

func TestCookieRecordsPathBoundaryMatching(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://example.test/foo/login")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{
		Name:   "scoped",
		Value:  "1",
		Path:   "/foo",
		Domain: "example.test",
	}})

	for _, rawURL := range []string{"https://example.test/foo", "https://example.test/foo/bar"} {
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		if records := net.CookieRecordsForURL(jar, u); len(records) != 1 {
			t.Fatalf("%s records = %d, want 1", rawURL, len(records))
		}
	}

	u, err := url.Parse("https://example.test/foobar")
	if err != nil {
		t.Fatal(err)
	}
	if records := net.CookieRecordsForURL(jar, u); len(records) != 0 {
		t.Fatalf("/foobar records = %d, want 0", len(records))
	}
}

func TestCookieRecordsDefaultPathUsesRequestDirectory(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://example.test/foo/bar/page")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{
		Name:   "implicit-path",
		Value:  "1",
		Domain: "example.test",
	}})

	matching, err := url.Parse("https://example.test/foo/bar/other")
	if err != nil {
		t.Fatal(err)
	}
	if records := net.CookieRecordsForURL(jar, matching); len(records) != 1 {
		t.Fatalf("directory records = %d, want 1", len(records))
	}

	nonMatching, err := url.Parse("https://example.test/foo/other")
	if err != nil {
		t.Fatal(err)
	}
	if records := net.CookieRecordsForURL(jar, nonMatching); len(records) != 0 {
		t.Fatalf("parent directory records = %d, want 0", len(records))
	}
}

func TestCookieRecordsRejectUnrelatedDomain(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://example.test/login")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{
		Name:   "session",
		Value:  "bad",
		Domain: "other.test",
		Path:   "/",
	}})

	other, err := url.Parse("https://other.test/")
	if err != nil {
		t.Fatal(err)
	}
	if records := net.CookieRecordsForURL(jar, other); len(records) != 0 {
		t.Fatalf("unrelated-domain records = %d, want 0", len(records))
	}
}

func TestCookieRecordsAcceptParentDomain(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://foo.example.com/login")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{
		Name:   "shared",
		Value:  "1",
		Domain: "example.com",
		Path:   "/",
	}})

	sibling, err := url.Parse("https://bar.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if records := net.CookieRecordsForURL(jar, sibling); len(records) != 1 {
		t.Fatalf("parent-domain records = %d, want 1", len(records))
	}
}

func TestCookieRecordsRejectPublicSuffixDomain(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://foo.example.com/login")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{
		Name:   "too-broad",
		Value:  "1",
		Domain: "com",
		Path:   "/",
	}})

	target, err := url.Parse("https://bar.com/")
	if err != nil {
		t.Fatal(err)
	}
	if records := net.CookieRecordsForURL(jar, target); len(records) != 0 {
		t.Fatalf("public-suffix records = %d, want 0", len(records))
	}
}

func TestCookieRecordsDeleteCookieWithNegativeMaxAge(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://example.test/")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{Name: "session", Value: "live", Path: "/"}})
	jar.SetCookies(origin, []*http.Cookie{{Name: "session", Value: "", Path: "/", MaxAge: -1}})

	if records := net.CookieRecordsForURL(jar, origin); len(records) != 0 {
		t.Fatalf("records after deletion = %d, want 0", len(records))
	}
	if len(jar.CookieRecords(origin)) != 0 {
		t.Fatalf("stored records after deletion = %d, want 0", len(jar.CookieRecords(origin)))
	}
}

func TestCookieRecordsDeleteCookieWithPastExpiry(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://example.test/")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{Name: "session", Value: "live", Path: "/"}})
	jar.SetCookies(origin, []*http.Cookie{{
		Name:    "session",
		Value:   "",
		Path:    "/",
		Expires: time.Now().Add(-time.Minute),
	}})

	if records := net.CookieRecordsForURL(jar, origin); len(records) != 0 {
		t.Fatalf("records after expiry = %d, want 0", len(records))
	}
	if len(jar.CookieRecords(origin)) != 0 {
		t.Fatalf("stored records after expiry = %d, want 0", len(jar.CookieRecords(origin)))
	}
}

func TestCookieRecordsPositiveMaxAgeExpiresAndIsRemoved(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://example.test/")
	if err != nil {
		t.Fatal(err)
	}
	// Set a cookie that's already expired via Expires in the past
	jar.SetCookies(origin, []*http.Cookie{{
		Name:    "short-lived",
		Value:   "1",
		Path:    "/",
		Expires: time.Now().Add(-time.Second),
	}})

	// Expired cookies should not be returned
	if cookies := jar.Cookies(origin); len(cookies) != 0 {
		t.Fatalf("cookies after expiry = %d, want 0", len(cookies))
	}
	if len(jar.CookieRecords(origin)) != 0 {
		t.Fatalf("stored records after expiry = %d, want 0", len(jar.CookieRecords(origin)))
	}
}

func TestCookieRecordsCanonicalDomainReplacementAndDeletion(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://Example.COM./account/login")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{Name: "session", Value: "old", Domain: "EXAMPLE.COM.", Path: "/"}})
	jar.SetCookies(origin, []*http.Cookie{{Name: "session", Value: "new", Domain: ".example.com", Path: "/"}})

	target, err := url.Parse("https://example.com/account")
	if err != nil {
		t.Fatal(err)
	}
	records := net.CookieRecordsForURL(jar, target)
	if len(records) != 1 {
		t.Fatalf("records after replacement = %d, want 1", len(records))
	}
	if records[0].Value != "new" || records[0].Domain != "example.com" {
		t.Fatalf("canonical replacement record = %#v", records[0])
	}

	jar.SetCookies(origin, []*http.Cookie{{Name: "session", Domain: "Example.Com.", Path: "/", MaxAge: -1}})
	if records := net.CookieRecordsForURL(jar, target); len(records) != 0 {
		t.Fatalf("records after canonical deletion = %d, want 0", len(records))
	}
}

func TestCookieRecordsMalformedPathUsesDefaultForReplacementAndDeletion(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("https://example.test/foo/bar/page")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{{Name: "scoped", Value: "old", Path: "foo"}})
	jar.SetCookies(origin, []*http.Cookie{{Name: "scoped", Value: "new"}})

	target, err := url.Parse("https://example.test/foo/bar/next")
	if err != nil {
		t.Fatal(err)
	}
	records := net.CookieRecordsForURL(jar, target)
	if len(records) != 1 {
		t.Fatalf("records after default-path replacement = %d, want 1", len(records))
	}
	if records[0].Value != "new" || records[0].Path != "/foo/bar" {
		t.Fatalf("default-path replacement record = %#v", records[0])
	}

	jar.SetCookies(origin, []*http.Cookie{{Name: "scoped", Path: "bar", MaxAge: -1}})
	if records := net.CookieRecordsForURL(jar, target); len(records) != 0 {
		t.Fatalf("records after default-path deletion = %d, want 0", len(records))
	}
}

func TestCookieRecordsIPHostRejectsDomainAttribute(t *testing.T) {
	jar := net.NewCookieJar()
	origin, err := url.Parse("http://127.0.0.1/account")
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(origin, []*http.Cookie{
		{Name: "domain", Value: "bad", Domain: "127.0.0.1", Path: "/"},
		{Name: "host-only", Value: "good", Path: "/"},
	})

	records := net.CookieRecordsForURL(jar, origin)
	if len(records) != 1 {
		t.Fatalf("IP-host records = %d, want 1 host-only cookie", len(records))
	}
	if records[0].Name != "host-only" || !records[0].HostOnly {
		t.Fatalf("IP-host record = %#v, want host-only cookie", records[0])
	}
}
