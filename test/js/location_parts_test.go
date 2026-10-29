package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// window.location, as the parts pages read.
//
// This is the file that fixes the error which survived every other repair on
// the saved vnexpress home page. Its inline boot script ends with
//
//	if (window.location.hostname.indexOf(...) < 0) { ... }
//
// and before this cluster `location` carried exactly one property - href - so
// the read was `Cannot read property 'indexOf' of undefined or null`: a
// missing property, not a missing method, throwing a TypeError that took the
// rest of the boot script with it.
//
// The measured reads on that 201,234-byte saved page (counted with grep over
// the file, `.host` minus the `.hostname` it also matches):
//
//	location.href 7, location.search 6, location.hash 5, location.pathname 4,
//	location.hostname 4, location.origin 3, location.host 2, location.protocol 2
//	location.port 0, location.searchParams 0, location.assign 0, location.replace 0
//
// Each part is an accessor over one parsed *url.URL the runtime holds, not a
// snapshot string set at boot, because history.pushState/replaceState move the
// document's URL and a page that reads location.pathname afterwards has to see
// the new one.

func locationRuntime(t *testing.T, url string) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(`<html><head></head><body></body></html>`)
	return newRuntime(t, js.Options{URL: url, DOM: doc})
}

// Every part of a full URL, with the spellings the URL Standard uses: protocol
// and hash and search carry their own punctuation, port is the empty string
// when the URL does not write one, and origin is scheme://host with no path.
// qc:BR-207
func TestLocationPartsOfAFullURL(t *testing.T) {
	rt := locationRuntime(t, "https://news.example.test:8443/muc-duc/tin-moi.html?tag=1&lang=vi#top")
	if err := rt.Run(`
		var out = [
			location.href, location.protocol, location.host, location.hostname,
			location.port, location.pathname, location.search, location.hash,
			location.origin
		].join("|");`, "inline"); err != nil {
		t.Fatal(err)
	}
	want := "https://news.example.test:8443/muc-duc/tin-moi.html?tag=1&lang=vi#top" +
		"|https:|news.example.test:8443|news.example.test|8443" +
		"|/muc-duc/tin-moi.html|?tag=1&lang=vi|#top|https://news.example.test:8443"
	if got := evalString(t, rt, `out`); got != want {
		t.Fatalf("location parts = %q, want %q", got, want)
	}
}

// An origin-only URL still reports a path, because that is what the URL
// serializer produces and pages build URLs by concatenating location.pathname
// with a route: an empty string here would turn "https://host" + "login" into
// "https://hostlogin". A port the URL does not write reads as "".
// qc:BR-207
func TestLocationPartsOfAnOriginOnlyURL(t *testing.T) {
	rt := locationRuntime(t, "https://example.test")
	if err := rt.Run(`
		var out = [
			location.pathname, location.search, location.hash, location.port,
			location.origin, location.host
		].join("|");`, "inline"); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{"/", "", "", "", "https://example.test", "example.test"}, "|")
	if got := evalString(t, rt, `out`); got != want {
		t.Fatalf("origin-only parts = %q, want %q", got, want)
	}
}

// The exact read that threw on the real page: a method call on hostname. A
// location whose hostname is undefined fails here as a TypeError the page
// cannot catch its way out of.
// qc:BR-207
func TestLocationHostnameSupportsARealPagePattern(t *testing.T) {
	rt := locationRuntime(t, "https://vnexpress.net/tin-tuc-24h?utm_source=rss")
	if err := rt.Run(`
		window.pageResult = "unset";
		try {
			var host = window.location.hostname;
			if (host.indexOf("vnexpress") >= 0 && location.pathname.indexOf("/tin-tuc") >= 0) {
				window.pageResult = "matched:" + host + location.pathname + location.search;
			} else {
				window.pageResult = "no-match";
			}
		} catch (e) {
			window.pageResult = "THREW:" + e.message;
		}`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `String(window.pageResult)`); got != "matched:vnexpress.net/tin-tuc-24h?utm_source=rss" {
		t.Fatalf("page pattern result = %q, want the matched branch", got)
	}
}

// location is the same object as a string: coercing it gives href, which is
// what `window.location + ""` and `String(location)` do in a page.
// qc:BR-207
func TestLocationCoercesToHref(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/a/b?c=d#e")
	if got := evalString(t, rt, `String(location)`); got != "https://example.test/a/b?c=d#e" {
		t.Fatalf("String(location) = %q", got)
	}
	if got := evalString(t, rt, `"" + location`); got != "https://example.test/a/b?c=d#e" {
		t.Fatalf("location + \"\" = %q", got)
	}
	if got := evalString(t, rt, `location.toString()`); got != "https://example.test/a/b?c=d#e" {
		t.Fatalf("location.toString() = %q", got)
	}
}

// document.location is the same object as window.location, and the pattern that
// proves it is worth pinning: the Google Analytics async snippet, quoted from
// the saved vietnamnet home page, reads the protocol off document -
//
//	ga.src = ('https:' == document.location.protocol ? 'https://ssl' : 'http://www') + ...
//
// and with no document.location the whole statement was that page's first
// surviving error, `Cannot read property 'protocol' of undefined`.
// qc:BR-207
func TestDocumentLocationIsTheSameObjectAsWindowLocation(t *testing.T) {
	rt := locationRuntime(t, "https://vietnamnet.vn/vn/thoi-su?a=1")
	if err := rt.Run(`
		var same = String(document.location === window.location);
		// The library's own line, with the URL it is reading from.
		var gaPrefix = ('https:' == document.location.protocol ? 'https://ssl' : 'http://www');
		var gaSrc = gaPrefix + '.google-analytics.com/ga.js';
		// A write through the document alias must not replace the object either.
		document.location = "https://other.test/";
		var stillSame = String(document.location === window.location) + "," + location.hostname;`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `same`); got != "true" {
		t.Fatalf("document.location === window.location read %q", got)
	}
	if got := evalString(t, rt, `gaSrc`); got != "https://ssl.google-analytics.com/ga.js" {
		t.Fatalf("analytics snippet built %q, want the https branch", got)
	}
	if got := evalString(t, rt, `stillSame`); got != "true,vietnamnet.vn" {
		t.Fatalf("after a document.location write = %q, want the object kept", got)
	}
}

// history.pushState and replaceState move the document's URL, and the parts are
// accessors over that URL, so a route change is visible in every part
// afterwards - including a relative argument, which the standard resolves
// against the document's own URL rather than storing as-is.
// qc:BR-207
func TestLocationPartsFollowHistory(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/start?old=1")
	if err := rt.Run(`
	history.pushState({n: 1}, "", "/route/2?step=3#deep");
	var pushed = [location.href, location.pathname, location.search, location.hash].join("|");
	history.replaceState({n: 2}, "", "https://m.example.test/final");
	var replaced = [location.host, location.hostname, location.pathname, location.search].join("|");
	history.back();
	var back = [location.href, location.pathname].join("|");`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `pushed`); got != "https://example.test/route/2?step=3#deep|/route/2|?step=3|#deep" {
		t.Fatalf("after pushState = %q", got)
	}
	if got := evalString(t, rt, `replaced`); got != "m.example.test|m.example.test|/final|" {
		t.Fatalf("after replaceState = %q", got)
	}
	if got := evalString(t, rt, `back`); got != "https://example.test/start?old=1|/start" {
		t.Fatalf("after history.back() = %q, want the URL the history walk restored", got)
	}
}

// Fragment writes perform same-document navigation while other URL component
// writes remain unsupported because they require a full document navigation.
// qc:BR-207
func TestLocationHashWriteUpdatesSameDocumentURL(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/page?a=1")
	if err := rt.Run(`
		var before = location.href;
		location.href = "https://other.test/elsewhere";
		location.hash = "#fragment";
		location.pathname = "/else";
		var after = location.href;
		var parts = [location.hash, location.pathname].join("|");
		var stayed = String(before === after);`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `stayed`); got != "false" {
		t.Fatalf("location.hash did not update the URL: %q", got)
	}
	if got := evalString(t, rt, `parts`); got != "#fragment|/page" {
		t.Fatalf("parts after writes = %q, want hash update and unchanged pathname", got)
	}
	if got := evalString(t, rt, `String(location)`); got != "https://example.test/page?a=1#fragment" {
		t.Fatalf("href after writes = %q", got)
	}
}

// The navigation members are deliberately absent, and a call on one is a loud
// TypeError naming the member rather than a silent no-op. Measured reason for
// keeping them absent: the saved vnexpress page calls `location.reload()` once,
// inside `validateCachePage`, after `new Image(1,1).src = ...` - and that Image
// constructor is the error the page actually stops on (see the task doc), so
// reload is not reached offline. A no-op reload would also be the one API here
// that could turn a snapshot render into a loop.
// qc:BR-207
func TestLocationNavigationMembersAreAbsent(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/page")
	if err := rt.Run(`
		var out = [
			typeof location.assign, typeof location.replace, typeof location.reload,
			typeof location.searchParams, typeof location.ancestorOrigins
		].join("|");
		var reloadError = "no-error";
		try { location.reload(); } catch (e) { reloadError = e.name + ":" + e.message; }`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `out`); got != "undefined|undefined|undefined|undefined|undefined" {
		t.Fatalf("navigation members = %q, want all undefined", got)
	}
	got := evalString(t, rt, `reloadError`)
	if !strings.Contains(got, "TypeError") || !strings.Contains(got, "reload") {
		t.Fatalf("location.reload() failed with %q, want a TypeError naming reload", got)
	}
}

// A URL this host gave that Go cannot parse still yields an object: href reads
// back exactly what the host passed, and the derived parts are empty strings
// rather than a partial parse. This is the file://-and-scheme-less-host case.
// qc:BR-207
func TestLocationOfAnUnparseableURL(t *testing.T) {
	rt := locationRuntime(t, "://not-a-url")
	if err := rt.Run(`
		var out = [
			location.href, location.protocol, location.host, location.hostname,
			location.pathname, location.search, location.hash, location.origin,
			location.port
		].join("|");`, "inline"); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{"://not-a-url", "", "", "", "", "", "", "null", ""}, "|")
	if got := evalString(t, rt, `out`); got != want {
		t.Fatalf("unparseable URL parts = %q, want %q", got, want)
	}
	// String coercion keeps the raw value too.
	if got := evalString(t, rt, `String(location)`); got != "://not-a-url" {
		t.Fatalf("String(location) = %q, want the raw host string", got)
	}
}
