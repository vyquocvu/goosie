package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

// qc:BR-207
func TestLocationNavigationCoercesArgumentsAndUpdatesHref(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		wantURL    string
		wantKind   js.NavigationKind
	}{
		{"assign object", `location.assign({toString: function(){ return "/coerced?x=1"; }})`, "https://example.test/coerced?x=1", js.NavigationAssign},
		{"assign undefined", `location.assign(undefined)`, "https://example.test/a/undefined", js.NavigationAssign},
		{"assign null", `location.assign(null)`, "https://example.test/a/null", js.NavigationAssign},
		{"replace number", `location.replace(42)`, "https://example.test/a/42", js.NavigationReplace},
		{"href boolean", `location.href = true`, "https://example.test/a/true", js.NavigationAssign},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := locationRuntime(t, "https://example.test/a/start?old=1")
			var requests []js.NavigationRequest
			rt.SetNavigationSink(func(request js.NavigationRequest) { requests = append(requests, request) })
			if err := rt.Run(tt.expression, "inline"); err != nil {
				t.Fatal(err)
			}
			if got := evalString(t, rt, `location.href`); got != tt.wantURL {
				t.Fatalf("immediate location.href = %q, want %q", got, tt.wantURL)
			}
			if len(requests) != 1 || requests[0].Kind != tt.wantKind || requests[0].URL != tt.wantURL {
				t.Fatalf("navigation requests = %+v, want one %v to %s", requests, tt.wantKind, tt.wantURL)
			}
		})
	}
}

// qc:BR-207
func TestLocationNavigationRejectsMissingAndInvalidArguments(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		wantError  string
	}{
		{"assign missing", `location.assign()`, "TypeError"},
		{"replace missing", `location.replace()`, "TypeError"},
		{"assign symbol", `location.assign(Symbol("target"))`, "TypeError"},
		{"assign invalid", `location.assign("http://[")`, "SyntaxError"},
		{"assign invalid port", `location.assign("http://example.test:99999")`, "SyntaxError"},
		{"replace missing host", `location.replace("https://")`, "SyntaxError"},
		{"replace invalid", `location.replace("http://[")`, "SyntaxError"},
		{"href invalid", `location.href = "http://["`, "SyntaxError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := locationRuntime(t, "https://example.test/start")
			requests := 0
			rt.SetNavigationSink(func(js.NavigationRequest) { requests++ })
			if err := rt.Run(`
				var navigationError = "none";
				try { `+tt.expression+`; } catch (e) { navigationError = e.name; }`, "inline"); err != nil {
				t.Fatal(err)
			}
			if got := evalString(t, rt, `navigationError`); got != tt.wantError {
				t.Fatalf("error = %q, want %s", got, tt.wantError)
			}
			if got := evalString(t, rt, `location.href`); got != "https://example.test/start" {
				t.Fatalf("failed navigation changed href to %q", got)
			}
			if requests != 0 {
				t.Fatalf("failed navigation queued %d requests", requests)
			}
		})
	}
}

// qc:BR-207
func TestLocationNavigationResolvesRelativeURLs(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/a/b/start?old=1#old")
	var requests []js.NavigationRequest
	rt.SetNavigationSink(func(request js.NavigationRequest) { requests = append(requests, request) })
	if err := rt.Run(`location.assign("../next?x=1#new")`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `location.href`); got != "https://example.test/a/next?x=1#new" {
		t.Fatalf("relative assign href = %q", got)
	}
	if len(requests) != 1 || requests[0].URL != "https://example.test/a/next?x=1#new" {
		t.Fatalf("relative assign requests = %+v", requests)
	}
}

// qc:BR-207
func TestLocationAssignReplaceAndReloadHaveDistinctRequestKinds(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/start")
	var requests []js.NavigationRequest
	rt.SetNavigationSink(func(request js.NavigationRequest) { requests = append(requests, request) })
	if err := rt.Run(`
		location.assign("/assign");
		location.replace("/replace");
		location.reload(false);`, "inline"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("request count = %d, want 3", len(requests))
	}
	if requests[0].Kind != js.NavigationAssign || requests[0].URL != "https://example.test/assign" {
		t.Fatalf("first request = %+v, want assign", requests[0])
	}
	if requests[1].Kind != js.NavigationReplace || requests[1].URL != "https://example.test/replace" {
		t.Fatalf("second request = %+v, want replace", requests[1])
	}
	if requests[2].Kind != js.NavigationReload || requests[2].URL != "https://example.test/replace" {
		t.Fatalf("third request = %+v, want reload of immediate location", requests[2])
	}
}

// qc:BR-207
func TestLocationHashOnlyAssignAndReplaceFireHashChange(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/page?x=1")
	var requests []js.NavigationRequest
	rt.SetNavigationSink(func(request js.NavigationRequest) { requests = append(requests, request) })
	if err := rt.Run(`
		window.hashEvents = [];
		window.addEventListener("hashchange", function(event) {
			window.hashEvents.push(event.oldURL + "|" + event.newURL);
		});
		location.assign("#first");
		location.replace("#second");`, "inline"); err != nil {
		t.Fatal(err)
	}
	want := "https://example.test/page?x=1|https://example.test/page?x=1#first," +
		"https://example.test/page?x=1#first|https://example.test/page?x=1#second"
	if got := evalString(t, rt, `window.hashEvents.join(",")`); got != want {
		t.Fatalf("hashchange events = %q, want %q", got, want)
	}
	if len(requests) != 2 || requests[0].Kind != js.NavigationAssign || requests[1].Kind != js.NavigationReplace {
		t.Fatalf("hash-only requests = %+v, want assign then replace", requests)
	}
	if got := evalString(t, rt, `location.href`); got != "https://example.test/page?x=1#second" {
		t.Fatalf("href after hash-only replace = %q", got)
	}
}

// qc:BR-207
func TestLocationNavigationSinkCanBeCleared(t *testing.T) {
	rt := locationRuntime(t, "https://example.test/start")
	requests := 0
	rt.SetNavigationSink(func(js.NavigationRequest) { requests++ })
	rt.SetNavigationSink(nil)
	if err := rt.Run(`location.assign("/next")`, "inline"); err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatalf("cleared sink received %d requests", requests)
	}
	if got := evalString(t, rt, `location.href`); got != "https://example.test/next" {
		t.Fatalf("href after cleared sink = %q", got)
	}
}

// qc:BR-207
func TestIFrameLocationNavigationDoesNotReachTopLevelSink(t *testing.T) {
	parentDoc := domtest.Parse(`<!doctype html><html><body><iframe id="frame" src="/child"></iframe></body></html>`)
	parent, err := js.New(js.Options{URL: "https://example.test/parent", DOM: parentDoc})
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	requests := 0
	parent.SetNavigationSink(func(js.NavigationRequest) { requests++ })
	parent.SetIFrameLoaderWithBase(func(base, src string) (*dom.Document, error) {
		return dom.ParseBounded(`<!doctype html><html><body><script>
			location.assign("/child-assign");
			location.replace("/child-replace");
			location.reload();
		</script></body></html>`, dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 32, AttributeBytes: 1024})
	})
	if err := parent.Run(`window.frameWindow = document.getElementById("frame").contentWindow;`, "inline"); err != nil {
		t.Fatal(err)
	}
	parent.QueueExistingIFrames()
	parent.FlushIFrameLoads()
	if got := evalString(t, parent, `frameWindow.location.href`); got != "https://example.test/child-replace" {
		t.Fatalf("iframe href = %q, want iframe navigation target", got)
	}
	if got := evalString(t, parent, `location.href`); got != "https://example.test/parent" {
		t.Fatalf("top-level href changed to %q", got)
	}
	if requests != 0 {
		t.Fatalf("iframe navigation reached top-level sink %d times", requests)
	}
	if err := parent.Run(`frameWindow.location.assign("/frame-assign")`, "inline"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, parent, `frameWindow.location.href`); got != "https://example.test/frame-assign" {
		t.Fatalf("parent-driven iframe href = %q", got)
	}
	if got := evalString(t, parent, `frameWindow.eval("location.href")`); got != "https://example.test/frame-assign" {
		t.Fatalf("old iframe runtime href = %q, want immediate update", got)
	}
	if requests != 0 {
		t.Fatalf("parent-driven iframe navigation reached top-level sink %d times", requests)
	}
}
