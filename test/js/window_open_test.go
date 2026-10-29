package js_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestWindowOpenNavigatesAndRestoresChildFormState(t *testing.T) {
	firstURL := "https://example.test/first.html"
	secondURL := "https://example.test/second.html"
	loadCountScript := `<script>window.onload = () => window.opener.loadCount = (window.opener.loadCount || 0) + 1;</script>`
	rt := newRuntime(t, js.Options{
		URL: "https://example.test/main.html",
		DOM: domtest.Parse("<!doctype html><html><head></head><body></body></html>"),
	})
	rt.SetIFrameLoaderWithBase(func(base, target string) (*dom.Document, error) {
		switch target {
		case firstURL:
			return domtest.Parse(`<!doctype html><html><head>` + loadCountScript + `</head><body><input id="value" type="range" min="0" max="10" value="5"></body></html>`), nil
		case secondURL:
			return domtest.Parse(`<!doctype html><html><head>` + loadCountScript + `</head><body></body></html>`), nil
		default:
			t.Errorf("unexpected iframe navigation base=%q target=%q", base, target)
			return nil, nil
		}
	})
	if err := rt.Run(`window.child = window.open("/first.html");`, "parent.js"); err != nil {
		t.Fatal(err)
	}
	runUntil(t, rt, `window.loadCount === 1`)

	if got := evalString(t, rt, `String(child === window.child && child.opener === window && child.parent === child && child.top === child && child.frameElement === null && child.document.querySelector("#value").value === "5")`); got != "true" {
		t.Fatalf("opened child proxy or initial document = %s", got)
	}
	if err := rt.Run(`child.document.querySelector("#value").value = "8"; window.selfOpenResult = child.open("second.html", "_self", "opener") === child;`, "parent.js"); err != nil {
		t.Fatal(err)
	}
	runUntil(t, rt, `window.loadCount === 2`)
	if got := evalString(t, rt, `String(window.selfOpenResult)`); got != "true" {
		t.Fatalf("window.open(..., _self) returned current context = %q, want true", got)
	}
	if got := evalString(t, rt, `String(child.location.href)`); got != secondURL {
		t.Fatalf("child URL after navigation = %q, want %q", got, secondURL)
	}
	if got := evalString(t, rt, `String(child.history.length)`); got != "2" {
		t.Fatalf("child history length = %s, want 2", got)
	}
	if err := rt.Run(`child.history.back();`, "parent.js"); err != nil {
		t.Fatal(err)
	}
	runUntil(t, rt, `window.loadCount === 3`)
	if got := evalString(t, rt, `String(child.location.href)`); got != firstURL {
		t.Fatalf("child URL after history.back() = %q, want %q", got, firstURL)
	}
	if got := evalString(t, rt, `String(child.document.querySelector("#value").value)`); got != "8" {
		t.Fatalf("restored range value = %s, want 8", got)
	}
}

func TestIframeWindowProxyOpenNavigatesItsOwnContext(t *testing.T) {
	rt := newRuntime(t, js.Options{
		URL: "https://example.test/main.html",
		DOM: domtest.Parse("<!doctype html><html><head></head><body></body></html>"),
	})
	rt.SetIFrameLoaderWithBase(func(base, target string) (*dom.Document, error) {
		if target != "https://example.test/destination.html" {
			t.Errorf("unexpected iframe navigation base=%q target=%q", base, target)
		}
		return domtest.Parse(`<!doctype html><html><body><script>parent.didNavigate = true;</script></body></html>`), nil
	})
	if err := rt.Run(`
		window.didNavigate = false;
		window.frame = document.createElement('iframe');
		frame.srcdoc = '';
		document.body.appendChild(frame);
	`, "iframe-window-open.js"); err != nil {
		t.Fatal(err)
	}
	rt.FlushIFrameLoads()
	if err := rt.Run(`window.frame.contentWindow.open('/destination.html', '_self');`, "iframe-window-open.js"); err != nil {
		t.Fatalf("iframe WindowProxy.open: %v", err)
	}
	runUntil(t, rt, `window.didNavigate`)
}

func TestWindowOpenParentAndTopReuseOpenedContext(t *testing.T) {
	for _, target := range []string{"_parent", "_top"} {
		t.Run(target, func(t *testing.T) {
			const firstURL = "https://example.test/first.html"
			const nextURL = "https://example.test/next.html"
			loader := func(_, target string) (*dom.Document, error) {
				switch target {
				case firstURL, nextURL:
					return domtest.Parse(`<!doctype html><html><body><script>
						window.onload = () => window.opener.loadCount = (window.opener.loadCount || 0) + 1;
					</script></body></html>`), nil
				default:
					t.Errorf("unexpected window.open target %q", target)
					return nil, nil
				}
			}
			rt := newRuntime(t, js.Options{
				URL: "https://example.test/main.html",
				DOM: domtest.Parse(`<!doctype html><html><body></body></html>`),
			})
			rt.SetIFrameLoaderWithBase(loader)
			if err := rt.Run(`window.popup = window.open("/first.html")`, "open-parent-top.js"); err != nil {
				t.Fatal(err)
			}
			runUntil(t, rt, `window.loadCount === 1`)
			if err := rt.Run(`window.openTargetResult = popup.open("/next.html", "`+target+`") === popup`, "open-parent-top.js"); err != nil {
				t.Fatal(err)
			}
			runUntil(t, rt, `window.loadCount === 2`)
			if got := evalString(t, rt, `String(window.openTargetResult) + '|' + popup.location.href`); got != "true|"+nextURL {
				t.Fatalf("window.open target %s result = %q, want true|%s", target, got, nextURL)
			}
		})
	}
}

func TestWindowOpenParentAndTopFromNestedIframeNavigatePopup(t *testing.T) {
	for _, target := range []string{"_parent", "_top"} {
		t.Run(target, func(t *testing.T) {
			const popupURL = "https://example.test/popup.html"
			const nextURL = "https://example.test/next.html"
			rt := newRuntime(t, js.Options{
				URL: "https://example.test/main.html",
				DOM: domtest.Parse(`<!doctype html><html><body></body></html>`),
			})
			rt.SetIFrameLoaderWithBase(func(_, destination string) (*dom.Document, error) {
				switch destination {
				case popupURL:
					return domtest.Parse(`<!doctype html><html><body><iframe src="/inner.html"></iframe></body></html>`), nil
				case "/inner.html":
					return domtest.Parse(`<!doctype html><html><body><script>window.open('/next.html', '` + target + `');</script></body></html>`), nil
				case nextURL:
					return domtest.Parse(`<!doctype html><html><body>arrived</body></html>`), nil
				default:
					t.Errorf("unexpected window.open target %q", destination)
					return nil, nil
				}
			})
			if err := rt.Run(`
				window.popupReachedTarget = false;
				window.popup = window.open("/popup.html");
				popup.onload = function() {
					if (popup.location.href === "`+nextURL+`") window.popupReachedTarget = true;
				};
			`, "nested-window-open-target.js"); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 20 && evalString(t, rt, `String(window.popupReachedTarget)`) != "true"; i++ {
				rt.Tick()
			}
			if got := evalString(t, rt, `String(window.popupReachedTarget) + '|' + popup.location.href`); got != "true|"+nextURL {
				t.Fatalf("nested window.open target %s result = %q, want true|%s", target, got, nextURL)
			}
		})
	}
}

func TestAnchorTargetBlankOpenerRules(t *testing.T) {
	rt := newRuntime(t, js.Options{
		URL: "https://example.test/main.html",
		DOM: domtest.Parse(`<!doctype html><html><body>
			<a id="implicit" target="_blank" href="/implicit.html?implicit">implicit</a>
			<a id="explicit" target="_blank" rel="opener" href="/explicit.html?explicit">explicit</a>
		</body></html>`),
	})
	rt.SetIFrameLoaderWithBase(func(_, _ string) (*dom.Document, error) {
		return domtest.Parse(`<!doctype html><html><body><script>
			window.observedOpener = window.opener !== null;
			new BroadcastChannel(window.location.search.substring(1)).postMessage({hasOpener: window.observedOpener});
			window.close();
		</script>opened</body></html>`), nil
	})
	if err := rt.Run(`
		window.receivedOpeners = {};
		for (const name of ['implicit', 'explicit']) {
			const channel = new BroadcastChannel(name);
			channel.onmessage = event => receivedOpeners[name] = event.data.hasOpener;
		}
		document.getElementById('implicit').click();
		document.getElementById('explicit').click();
	`, "anchor-target.js"); err != nil {
		t.Fatal(err)
	}
	runUntil(t, rt, `receivedOpeners.implicit === false && receivedOpeners.explicit === true`)
	if got := evalString(t, rt, `String(receivedOpeners.implicit) + '|' + String(receivedOpeners.explicit)`); got != "false|true" {
		t.Fatalf("target=_blank opener results = %q, want false|true", got)
	}
}

func TestAnchorTargetBlankConsumesTransientUserActivation(t *testing.T) {
	rt := newRuntime(t, js.Options{
		URL: "https://example.test/main.html",
		DOM: domtest.Parse(`<!doctype html><html><body><a id="open" target="_blank" href="/child.html">open</a></body></html>`),
	})
	rt.SetTransientUserActivation(true)
	if !rt.HasTransientUserActivation() {
		t.Fatal("transient activation was not set before opening the link")
	}
	if err := rt.Run(`document.getElementById("open").click()`, "activation-consumption.js"); err != nil {
		t.Fatal(err)
	}
	if rt.HasTransientUserActivation() {
		t.Fatal("target=_blank left transient activation active")
	}
	if got := evalString(t, rt, `String(navigator.userActivation.hasBeenActive)`); got != "true" {
		t.Fatalf("sticky activation after opening popup = %q, want true", got)
	}
}

func TestWindowOpenNoopenerFeatureForcesUnnamedContext(t *testing.T) {
	const targetURL = "https://example.test/child.html"
	rt := newRuntime(t, js.Options{
		URL: "https://example.test/main.html",
		DOM: domtest.Parse(`<!doctype html><html><body></body></html>`),
	})
	rt.SetIFrameLoaderWithBase(func(_, target string) (*dom.Document, error) {
		if target != targetURL {
			t.Errorf("unexpected window.open destination %q", target)
		}
		return domtest.Parse(`<!doctype html><html><body><script>
			new BroadcastChannel("window-open-noopener").postMessage({
				hasOpener: window.opener !== null,
				name: window.name
			});
		</script></body></html>`), nil
	})
	if err := rt.Run(`
		window.openNoopenerResult = null;
		const channel = new BroadcastChannel("window-open-noopener");
		channel.onmessage = event => window.openNoopenerResult = event.data;
		window.openReturnWasNull = window.open("/child.html", "reusable", "width=100, NoOpener=yes") === null;
	`, "window-open-noopener.js"); err != nil {
		t.Fatal(err)
	}
	runUntil(t, rt, `window.openNoopenerResult !== null`)
	if got := evalString(t, rt, `String(window.openReturnWasNull) + '|' + String(window.openNoopenerResult.hasOpener) + '|' + window.openNoopenerResult.name`); got != "true|false|" {
		t.Fatalf("window.open noopener feature result = %q, want true|false|", got)
	}
}

func TestAnchorRelNoopenerSelfTargetNavigatesOpenedContext(t *testing.T) {
	const popupURL = "https://example.test/support/noopener-popup.html"
	const targetURL = "https://example.test/support/noopener-target-1.html"
	rt := newRuntime(t, js.Options{
		URL: "https://example.test/main.html",
		DOM: domtest.Parse(`<!doctype html><html><body></body></html>`),
	})
	rt.SetIFrameLoaderWithBase(func(_, target string) (*dom.Document, error) {
		switch target {
		case popupURL:
			return domtest.Parse(`<!doctype html><html><body><a id="selftarget" target="_self" rel="noopener" href="noopener-target-1.html">go</a></body></html>`), nil
		case targetURL:
			return domtest.Parse(`<!doctype html><html><body><script>opener.target1Loaded(window)</script></body></html>`), nil
		default:
			t.Errorf("unexpected frame navigation %q", target)
			return nil, nil
		}
	})
	if err := rt.Run(`
		window.didLoadTarget = false;
		window.target1Loaded = function(win) {
			window.didLoadTarget = true;
			window.targetOpenerMatches = win.opener === window;
		};
		window.popup = window.open('/support/noopener-popup.html');
		popup.onload = function() { popup.document.getElementById('selftarget').click(); };
	`, "noopener-self-target.js"); err != nil {
		t.Fatal(err)
	}
	runUntil(t, rt, `window.didLoadTarget`)
	if got := evalString(t, rt, `String(window.didLoadTarget) + '|' + String(window.targetOpenerMatches)`); got != "true|true" {
		t.Fatalf("same-context target navigation opener = %q, want true|true", got)
	}
}

func runUntil(t *testing.T, rt *js.Runtime, condition string) {
	t.Helper()
	for i := 0; i < 20; i++ {
		if got := evalString(t, rt, condition); got == "true" {
			return
		}
		if !rt.Tick() {
			continue
		}
	}
	got := evalString(t, rt, "String(window.loadCount)")
	details := evalString(t, rt, `String(child && child.location.href) + ";" + String(child && child.document && child.document.readyState) + ";" + String(child && child.onload) + ";opener=" + String(child && child.opener && child.opener.loadCount)`)
	t.Fatalf("condition %q did not become true after event loop turns (loadCount=%s child=%s)", condition, got, details)
}
