package engine_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

const frameTargetPageURL = "https://example.test/frames/page.html"

func parseFrameTargetDocument(t *testing.T, markup string) *dom.Document {
	t.Helper()
	doc, err := dom.ParseBounded(markup, dom.ParseLimits{Nodes: 128, Depth: 32, Attributes: 128, AttributeBytes: 1 << 14})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestSubmitFormNavigatesNamedFrameTargetWithGET(t *testing.T) {
	rt, err := js.New(js.Options{URL: frameTargetPageURL})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	var requested []string
	loader := func(base, src string) (*dom.Document, error) {
		requested = append(requested, src)
		u, parseErr := url.Parse(src)
		if parseErr != nil {
			return nil, parseErr
		}
		if u.Path == "/result" {
			return parseFrameTargetDocument(t, `<!doctype html><body><p>initial</p></body>`), nil
		}
		return parseFrameTargetDocument(t, `<!doctype html><body><p>submitted</p></body>`), nil
	}
	html := `<!doctype html><html><body>
		<iframe name="result" src="/result"></iframe>
		<form id="f" action="/submit" method="get" target="result">
			<input name="q" value="hello">
		</form>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS(frameTargetPageURL, nil),
		engine.WithIFrameDocumentLoader(loader),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	sess.SubmitForm(sess.Doc.ElementByID("f"), nil)
	if sub, ok := sess.PopSubmittedForm(); ok {
		t.Fatalf("named-frame submission reached the host: %+v", sub)
	}
	sess.PumpJS(800)

	if got, err := sess.EvaluateJS(`document.querySelector('iframe').contentDocument.body.textContent`); err != nil || got != "submitted" {
		t.Fatalf("iframe content after GET submission = %q (err %v)", got, err)
	}
	if len(requested) != 2 || requested[0] != "/result" || requested[1] != "https://example.test/submit?q=hello" {
		t.Fatalf("iframe requests = %v, want initial /result then the encoded action URL", requested)
	}
}

func TestSubmitFormPostsNamedFrameTargetThroughHostTransport(t *testing.T) {
	rt, err := js.New(js.Options{URL: frameTargetPageURL})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	var postedContentType, postedBody string
	html := `<!doctype html><html><body>
		<iframe name="result" src="/result"></iframe>
		<form id="f" action="/upload" method="post" target="result">
			<input name="q" value="hello">
		</form>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS(frameTargetPageURL, nil),
		engine.WithIFrameDocumentLoader(func(base, src string) (*dom.Document, error) {
			return parseFrameTargetDocument(t, `<!doctype html><body><p>initial</p></body>`), nil
		}),
		engine.WithIFrameFormSubmitter(func(base, target, contentType string, body []byte) (*dom.Document, error) {
			postedContentType, postedBody = contentType, string(body)
			if target != "https://example.test/upload" {
				t.Errorf("POST target = %q", target)
			}
			return parseFrameTargetDocument(t, `<!doctype html><body><p>uploaded</p></body>`), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	sess.SubmitForm(sess.Doc.ElementByID("f"), nil)
	if sub, ok := sess.PopSubmittedForm(); ok {
		t.Fatalf("named-frame submission reached the host: %+v", sub)
	}
	sess.PumpJS(800)

	if postedContentType != "application/x-www-form-urlencoded" || postedBody != "q=hello" {
		t.Fatalf("POST transport = %q %q, want URL-encoded form body", postedContentType, postedBody)
	}
	if got, err := sess.EvaluateJS(`document.querySelector('iframe').contentDocument.body.textContent`); err != nil || got != "uploaded" {
		t.Fatalf("iframe content after POST submission = %q (err %v)", got, err)
	}
}

func TestSubmitFormUsesSubmitterFormTargetOverride(t *testing.T) {
	rt, err := js.New(js.Options{URL: frameTargetPageURL})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><html><body>
		<iframe name="result" src="/result"></iframe>
		<form id="f" action="/submit">
			<input name="q" value="hello">
			<button id="send" type="submit" formtarget="result">Go</button>
		</form>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS(frameTargetPageURL, nil),
		engine.WithIFrameDocumentLoader(func(base, src string) (*dom.Document, error) {
			return parseFrameTargetDocument(t, `<!doctype html><body><p>frame body</p></body>`), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	sess.SubmitForm(sess.Doc.ElementByID("f"), sess.Doc.ElementByID("send"))
	if sub, ok := sess.PopSubmittedForm(); ok {
		t.Fatalf("formtarget submission reached the host: %+v", sub)
	}
	sess.PumpJS(800)
	if got, err := sess.EvaluateJS(`document.querySelector('iframe').contentDocument.body.textContent`); err != nil || got != "frame body" {
		t.Fatalf("iframe content after formtarget submission = %q (err %v)", got, err)
	}
}

func TestSubmitFormWithUnknownTargetFrameFallsBackToHost(t *testing.T) {
	rt, err := js.New(js.Options{URL: frameTargetPageURL})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><html><body>
		<iframe name="result" src="/result"></iframe>
		<form id="f" action="/submit" method="get" target="missing">
			<input name="q" value="hello">
		</form>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS(frameTargetPageURL, nil),
		engine.WithIFrameDocumentLoader(func(base, src string) (*dom.Document, error) {
			return parseFrameTargetDocument(t, `<!doctype html><body><p>frame body</p></body>`), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	sess.SubmitForm(sess.Doc.ElementByID("f"), nil)
	sub, ok := sess.PopSubmittedForm()
	if !ok {
		t.Fatal("unknown named target did not fall back to a host submission")
	}
	if sub.Target != "missing" || sub.Method != "GET" || !strings.Contains(sub.Encoded, "q=hello") {
		t.Fatalf("fallback submission = %+v", sub)
	}
}

func TestSubmitFormWithBlankTargetStaysWithTheHost(t *testing.T) {
	rt, err := js.New(js.Options{URL: frameTargetPageURL})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	html := `<!doctype html><html><body>
		<iframe name="result" src="/result"></iframe>
		<form id="f" action="/submit" method="get" target="_blank">
			<input name="q" value="hello">
		</form>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800,
		engine.WithJS(rt),
		engine.WithLinkedCSS(frameTargetPageURL, nil),
		engine.WithIFrameDocumentLoader(func(base, src string) (*dom.Document, error) {
			return parseFrameTargetDocument(t, `<!doctype html><body><p>frame body</p></body>`), nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	sess.SubmitForm(sess.Doc.ElementByID("f"), nil)
	sub, ok := sess.PopSubmittedForm()
	if !ok || sub.Target != "_blank" {
		t.Fatalf("blank-target submission = %+v (present %v), want host transport", sub, ok)
	}
}
