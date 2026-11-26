package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func locationRuntime(t *testing.T, url string) *js.Runtime {
	t.Helper()
	doc := domtest.Parse(`<html><head></head><body></body></html>`)
	return newRuntime(t, js.Options{URL: url, DOM: doc})
}

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

// qc:BR-207
func TestDocumentLocationIsTheSameObjectAsWindowLocation(t *testing.T) {
	rt := locationRuntime(t, "https://vietnamnet.vn/vn/thoi-su?a=1")
	var requests []js.NavigationRequest
	rt.SetNavigationSink(func(request js.NavigationRequest) { requests = append(requests, request) })
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
	if got := evalString(t, rt, `stillSame`); got != "true,other.test" {
		t.Fatalf("after a document.location write = %q, want the object kept and URL updated", got)
	}
	if len(requests) != 1 || requests[0].Kind != js.NavigationAssign || requests[0].URL != "https://other.test/" {
		t.Fatalf("document.location navigation requests = %+v, want one assign to https://other.test/", requests)
	}
}

// qc:BR-207
func TestLocationPartsFollowHistory(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/start?old=1")
	if err := rt.Run(`
	history.pushState({n: 1}, "", "/route/2?step=3#deep");
	var pushed = [location.href, location.pathname, location.search, location.hash].join("|");
	history.replaceState({n: 2}, "", "https://example.test/final?x=1");
	var replaced = [location.host, location.hostname, location.pathname, location.search].join("|");
	history.back();
	var back = [location.href, location.pathname].join("|");`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `pushed`); got != "https://example.test/route/2?step=3#deep|/route/2|?step=3|#deep" {
		t.Fatalf("after pushState = %q", got)
	}
	if got := evalString(t, rt, `replaced`); got != "example.test|example.test|/final|?x=1" {
		t.Fatalf("after replaceState = %q", got)
	}
	if got := evalString(t, rt, `back`); got != "https://example.test/start?old=1|/start" {
		t.Fatalf("after history.back() = %q, want the URL the history walk restored", got)
	}
}

// qc:BR-207
func TestLocationHashWriteUpdatesSameDocumentURL(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/page?a=1")
	requests := 0
	rt.SetNavigationSink(func(js.NavigationRequest) { requests++ })
	if err := rt.Run(`
		var before = location.href;
		var sameDocument = String(document.querySelector("body") === document.body);
		location.hash = "#fragment";
		var after = location.href;
		var parts = [location.hash, location.pathname].join("|");
		var stayed = String(before === after);`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `stayed`); got != "false" {
		t.Fatalf("location.hash did not update the URL: %q", got)
	}
	if got := evalString(t, rt, `sameDocument`); got != "true" {
		t.Fatalf("location.hash replaced the document: %q", got)
	}
	if got := evalString(t, rt, `parts`); got != "#fragment|/page" {
		t.Fatalf("parts after hash write = %q, want hash update and unchanged pathname", got)
	}
	if got := evalString(t, rt, `String(location)`); got != "https://example.test/page?a=1#fragment" {
		t.Fatalf("href after hash write = %q", got)
	}
	if requests != 0 {
		t.Fatalf("hash write queued %d navigations, want same-document update", requests)
	}
}

// qc:BR-207
func TestLocationNavigationMembersAreAbsent(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/page")
	var requests []js.NavigationRequest
	rt.SetNavigationSink(func(request js.NavigationRequest) { requests = append(requests, request) })
	if err := rt.Run(`
		var out = [
			typeof location.assign, typeof location.replace, typeof location.reload,
			typeof location.searchParams, typeof location.ancestorOrigins
		].join("|");
		var reloadResult = "no-error";
		try { reloadResult = String(location.reload(true)); } catch (e) { reloadResult = e.name + ":" + e.message; }
		var assignError = "no-error";
		try { location.assign(); } catch (e) { assignError = e.name; }`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `out`); got != "function|function|function|undefined|undefined" {
		t.Fatalf("navigation members = %q, want Chromium's Location API", got)
	}
	if got := evalString(t, rt, `reloadResult`); got != "undefined" {
		t.Fatalf("location.reload(true) = %q, want undefined", got)
	}
	if got := evalString(t, rt, `assignError`); got != "TypeError" {
		t.Fatalf("location.assign() error = %q, want TypeError", got)
	}
	if len(requests) != 1 || requests[0].Kind != js.NavigationReload || requests[0].URL != "https://example.test/page" {
		t.Fatalf("reload requests = %+v, want one reload of the current URL", requests)
	}
}

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
	if got := evalString(t, rt, `String(location)`); got != "://not-a-url" {
		t.Fatalf("String(location) = %q, want the raw host string", got)
	}
}
