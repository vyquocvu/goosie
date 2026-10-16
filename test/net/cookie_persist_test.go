package net_test

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/net"
)

func TestCookieJarSaveLoadRoundTrip(t *testing.T) {
	u, _ := url.Parse("https://example.com/")
	jar := net.NewCookieJar()
	jar.Store(u, []string{
		"persistent=yes; Expires=Wed, 21 Oct 2099 07:28:00 GMT; Path=/",
		"bearer=token; Secure; Max-Age=3600",
	})
	path := filepath.Join(t.TempDir(), "state", "cookies.json")
	if err := jar.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded := net.NewCookieJar()
	if err := reloaded.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	header := reloaded.HeaderFor(u)
	for _, want := range []string{"persistent=yes", "bearer=token"} {
		if !strings.Contains(header, want) {
			t.Errorf("reloaded header %q missing %q", header, want)
		}
	}
}

func TestCookieSaveDropsSessionCookies(t *testing.T) {
	u, _ := url.Parse("https://example.com/")
	jar := net.NewCookieJar()
	jar.Store(u, []string{"session=only"})
	path := filepath.Join(t.TempDir(), "cookies.json")
	if err := jar.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "session=only") || strings.Contains(string(data), "\"session\"") {
		t.Errorf("session cookie must not survive an exit save: %s", data)
	}

	reloaded := net.NewCookieJar()
	if err := reloaded.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := reloaded.HeaderFor(u); got != "" {
		t.Errorf("session cookie came back from disk: %q", got)
	}
}

func TestCookieLoadMissingFileIsEmptyJar(t *testing.T) {
	jar := net.NewCookieJar()
	if err := jar.Load(filepath.Join(t.TempDir(), "absent.json")); err != nil {
		t.Fatalf("Load on missing file should be a no-op, got %v", err)
	}
	u, _ := url.Parse("https://example.com/")
	if got := jar.HeaderFor(u); got != "" {
		t.Errorf("empty jar produced a header: %q", got)
	}
}

func TestCookieLoadDropsExpiredEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	file := `[
		{"name":"stale","value":"x","domain":"example.com","path":"/","hostOnly":true,"expires":"2000-01-01T00:00:00Z"},
		{"name":"fresh","value":"y","domain":"example.com","path":"/","hostOnly":true,"expires":"2099-01-01T00:00:00Z"}
	]`
	if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	jar := net.NewCookieJar()
	if err := jar.Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	u, _ := url.Parse("https://example.com/")
	if got := jar.HeaderFor(u); got != "fresh=y" {
		t.Errorf("HeaderFor = %q, want only the unexpired cookie", got)
	}
}

func TestCookieLoadCorruptFileIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	jar := net.NewCookieJar()
	if err := jar.Load(path); err == nil {
		t.Fatal("Load on a corrupt file should report the error")
	}
}

func TestCookieStoreReplacesEarlierHostEntry(t *testing.T) {
	// A cookie loaded under its Domain and then re-Set by a subdomain must
	// not appear twice in the jar: (name, domain, path) is the identity.
	path := filepath.Join(t.TempDir(), "cookies.json")
	file := `[{"name":"k","value":"old","domain":"example.com","path":"/","hostOnly":false,"expires":"2099-01-01T00:00:00Z"}]`
	if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	jar := net.NewCookieJar()
	if err := jar.Load(path); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("https://www.example.com/")
	jar.Store(u, []string{"k=new; Domain=example.com; Path=/; Max-Age=3600"})

	if got := jar.HeaderFor(u); got != "k=new" {
		t.Errorf("HeaderFor = %q, want the re-Set cookie exactly once", got)
	}
}

func TestDefaultClientWithCookiesLoadsAndSavesOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	seed := `[{"name":"seeded","value":"1","domain":"example.com","path":"/","hostOnly":true,"expires":"2099-01-01T00:00:00Z"}]`
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := net.DefaultClientWithCookies(path)
	if err != nil {
		t.Fatalf("DefaultClientWithCookies: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	jar := net.NewCookieJar()
	if err := jar.Load(path); err != nil {
		t.Fatalf("file left by Close does not parse: %v", err)
	}
	u, _ := url.Parse("https://example.com/")
	if got := jar.HeaderFor(u); got != "seeded=1" {
		t.Errorf("cookie lost across a client Close: %q", got)
	}
}

func TestDefaultClientWithCookiesCorruptFileIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cookies.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := net.DefaultClientWithCookies(path); err == nil {
		t.Fatal("a corrupt cookie file should surface an error, not start an empty jar that saves over it")
	}
}

func TestDefaultClientWithCookiesMissingFileIsFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "cookies.json")
	c, err := net.DefaultClientWithCookies(path)
	if err != nil {
		t.Fatalf("first run should not fail on a missing file: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Close should create the cookie file: %v", err)
	}
}
