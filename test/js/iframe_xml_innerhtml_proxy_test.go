package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestIframeWindowProxyPropagatesXMLInnerHTMLErrors(t *testing.T) {
	rt := newRuntime(t, js.Options{
		URL:     "https://example.test/main.xhtml",
		DOM:     domtest.Parse(`<!doctype html><html><head></head><body></body></html>`),
		Timeout: 5 * time.Second,
	})
	rt.SetIFrameLoaderWithBase(func(_, target string) (*dom.Document, error) {
		if target != "/child.xml" {
			t.Fatalf("unexpected iframe URL %q", target)
		}
		return js.ParseXMLDocumentBounded(`<html xmlns="http://www.w3.org/1999/xhtml"><foo--/></html>`, "application/xhtml+xml", dom.ParseLimits{Nodes: 64, Depth: 16, Attributes: 32, AttributeBytes: 1024})
	})
	if err := rt.Run(`
		var frame = document.createElement('iframe');
		frame.src = '/child.xml';
		document.body.appendChild(frame);
	`, "iframe-xml-innerhtml-setup.js"); err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(`
		var foo = window[0].document.documentElement.firstChild;
		try {
			foo.innerHTML = 'x</foo--><\!--y';
			document.body.setAttribute('data-iframe-xml-innerhtml-caught', 'no throw');
		} catch (error) {
			document.body.setAttribute('data-iframe-xml-innerhtml-caught', error.name);
		}
	`, "iframe-xml-innerhtml-assignment.js"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, rt, `document.body.getAttribute('data-iframe-xml-innerhtml-caught')`); got != "SyntaxError" {
		t.Fatalf("cross-realm XML innerHTML error = %q, want SyntaxError", got)
	}
}
