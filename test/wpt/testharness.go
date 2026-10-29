package wpt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	htmlstd "html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
	internalnet "github.com/vyquocvu/goosie/internal/net"
)

const maxReportedWPTFailures = 20

const wptTestDriverAccessibilityShim = `(function () {
  const install = internal => {
    if (!internal || internal.__goosieAccessibilityMethods) return;
    Object.defineProperty(internal, "__goosieAccessibilityMethods", {value: true});
    const waitForLayout = async () => {
      await new Promise(resolve => setTimeout(resolve, 0));
      await new Promise(resolve => setTimeout(resolve, 0));
    };
    internal.get_computed_role = async element => {
      await waitForLayout();
      const info = window.__goosieAccessibilityInfo ? window.__goosieAccessibilityInfo(element) : null;
      return info ? info.role : null;
    };
    internal.get_computed_label = async element => {
      await waitForLayout();
      const info = window.__goosieAccessibilityInfo ? window.__goosieAccessibilityInfo(element) : null;
      return info ? info.label : "";
    };
  };
  let current = window.test_driver_internal;
  install(current);
  const descriptor = Object.getOwnPropertyDescriptor(window, "test_driver_internal");
  if (!descriptor || descriptor.configurable) {
    Object.defineProperty(window, "test_driver_internal", {
      configurable: true,
      enumerable: true,
      get: () => current,
      set: value => { current = value; install(value); }
    });
  }
})();`

type wptHarnessReport struct {
	Complete      bool     `json:"complete"`
	Status        int      `json:"status"`
	StatusMessage string   `json:"statusMessage"`
	StatusStack   string   `json:"statusStack"`
	Total         int      `json:"total"`
	Failed        int      `json:"failed"`
	Skipped       int      `json:"skipped"`
	TimedOut      int      `json:"timedOut"`
	Failures      []string `json:"failures"`
	Skips         []string `json:"skips"`
}

// runTestHarness executes a WPT testharness.js file in Goosie's real document
// engine and collects the completion callback result. Page scripts and harness
// resources are restricted to the active WPT server origin.
func (r *Runner) runTestHarness(dt DiscoveredTest) TestResult {
	queries := r.cfg.TestVariants[dt.Path]
	if len(queries) == 0 {
		return r.runTestHarnessVariant(dt, "")
	}
	combined := TestResult{Path: dt.Path, Type: dt.Type}
	allPassed, allSkipped := true, true
	var statusDetails, failures []string
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if _, err := testVariantURL("http://wpt.invalid/test.html", query); err != nil {
			combined.Status = "error"
			combined.Message = fmt.Sprintf("invalid configured variant %q: %v", query, err)
			combined.Variants = append(combined.Variants, TestVariantResult{Query: query, Status: "error", Message: err.Error()})
			return combined
		}
		result := r.runTestHarnessVariant(dt, query)
		combined.Variants = append(combined.Variants, TestVariantResult{
			Query: query, Status: result.Status, Message: result.Message, DurationMS: result.DurationMS,
		})
		statusDetails = append(statusDetails, query+"="+result.Status)
		combined.DurationMS += result.DurationMS
		allPassed = allPassed && result.Status == "pass"
		allSkipped = allSkipped && result.Status == "skip"
		if result.Status != "pass" && result.Status != "skip" {
			failures = append(failures, fmt.Sprintf("%s: %s %s", query, result.Status, result.Message))
		}
	}
	switch {
	case allPassed:
		combined.Status = "pass"
	case allSkipped:
		combined.Status = "skip"
	default:
		combined.Status = "fail"
		for _, variant := range combined.Variants {
			if variant.Status == "error" {
				combined.Status = "error"
				break
			}
			if variant.Status == "timeout" {
				combined.Status = "timeout"
			}
		}
	}
	combined.Message = "variants: " + strings.Join(statusDetails, ", ")
	if len(failures) > 0 {
		combined.Message += " | " + strings.Join(failures, " | ")
	}
	return combined
}

func testVariantURL(testURL, query string) (string, error) {
	if query == "" {
		return testURL, nil
	}
	query = strings.TrimPrefix(query, "?")
	if query == "" || strings.ContainsAny(query, "?#") {
		return "", fmt.Errorf("variant must be a non-empty query without a fragment")
	}
	if _, err := url.ParseQuery(query); err != nil {
		return "", fmt.Errorf("invalid variant query: %w", err)
	}
	parsed, err := url.Parse(testURL)
	if err != nil {
		return "", fmt.Errorf("invalid test URL: %w", err)
	}
	parsed.RawQuery = query
	return parsed.String(), nil
}

// wptDocumentURL gives secure-suffixed WPT files their specified HTTPS origin
// while the harness transports the document bytes through its local HTTP
// listener. Subresource adapters apply the same alias transport mapping.
func wptDocumentURL(transportURL, testPath string) string {
	if !strings.Contains(strings.ToLower(testPath), ".https.") {
		return transportURL
	}
	parsed, err := url.Parse(transportURL)
	if err != nil || parsed.Scheme != "http" {
		return transportURL
	}
	parsed.Scheme = "https"
	return parsed.String()
}

func (r *Runner) runTestHarnessVariant(dt DiscoveredTest, query string) TestResult {
	tr := TestResult{Path: dt.Path, Type: dt.Type}
	if r.server == nil {
		tr.Status = "error"
		tr.Message = "WPT server is not running"
		return tr
	}

	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.cfg.Timeout)*time.Second)
	defer cancel()

	baseURL := r.server.URL()
	transportTestURL, err := testVariantURL(baseURL+"/"+strings.ReplaceAll(dt.Path, "\\", "/"), query)
	if err != nil {
		tr.Status = "error"
		tr.Message = "build test URL: " + err.Error()
		return tr
	}
	testURL := wptDocumentURL(transportTestURL, dt.Path)
	client := &http.Client{
		Timeout: time.Duration(r.cfg.Timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !sameOrigin(req.URL, baseURL) {
				return fmt.Errorf("WPT resource redirect left the local test origin")
			}
			return nil
		},
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, transportTestURL, nil)
	if err != nil {
		tr.Status = "error"
		tr.Message = "build test request: " + err.Error()
		return tr
	}
	response, err := client.Do(request)
	if err != nil {
		tr.Status = "error"
		tr.Message = "fetch test: " + err.Error()
		return tr
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		tr.Status = "error"
		tr.Message = fmt.Sprintf("fetch test: HTTP %d", response.StatusCode)
		return tr
	}
	html, err := io.ReadAll(io.LimitReader(response.Body, engine.MaxDocumentBytes+1))
	if err != nil {
		tr.Status = "error"
		tr.Message = "read test: " + err.Error()
		return tr
	}
	if len(html) > engine.MaxDocumentBytes {
		tr.Status = "error"
		tr.Message = "WPT HTML exceeds the engine document byte limit"
		return tr
	}
	if strings.HasSuffix(strings.ToLower(dt.Path), ".any.js") {
		html = anyJSTestDocument(dt.Path, html)
	}

	var fetchRequests atomic.Int32
	var fetchSuccesses atomic.Int32
	var fetchFailures atomic.Int32
	var iframeRequests atomic.Int32
	var scriptRequests atomic.Int32
	var scriptFailures atomic.Int32
	var lastScriptURL atomic.Value
	var workerScriptRequests atomic.Int32
	var lastWorkerScriptURL atomic.Value
	var session *engine.Session
	var harnessActiveNode *dom.Node
	setHarnessActive := func(node *dom.Node, active bool) {
		if node == nil {
			return
		}
		if active {
			harnessActiveNode = node
		} else if harnessActiveNode != node {
			return
		} else {
			harnessActiveNode = nil
		}
		for cur := node; cur != nil && cur.Element(); cur = cur.Parent {
			css.SetActive(cur, active)
		}
	}
	fetchWorkerScript := func(resourceURL string) ([]byte, error) {
		workerScriptRequests.Add(1)
		lastWorkerScriptURL.Store(resourceURL)
		parsed, err := url.Parse(resourceURL)
		if err != nil {
			return nil, fmt.Errorf("WPT worker script URL is outside the local test origin")
		}
		transportURL, ok := wptLocalTransportURL(parsed, baseURL)
		if !ok {
			return nil, fmt.Errorf("WPT worker script URL is outside the local WPT server aliases")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, transportURL.String(), nil)
		if err != nil {
			return nil, err
		}
		request.Host = parsed.Host
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("WPT worker script fetch: HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, js.MaxScriptBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > js.MaxScriptBytes {
			return nil, fmt.Errorf("WPT worker script exceeds the engine script byte limit")
		}
		return data, nil
	}
	runtime, err := js.New(js.Options{
		URL:                 testURL,
		WorkerScriptFetcher: fetchWorkerScript,
		OnUserInput: func(*dom.Node) bool {
			return false
		},
		OnNumberInputEdit: func(*dom.Node, bool) {},
		OnEmailInputEdit:  func(*dom.Node, string) {},
		OnActiveState: func(node *dom.Node, active bool) {
			setHarnessActive(node, active)
			if session != nil {
				session.SetKeyboardActive(node, active)
			}
		},
		OnTabNavigation: func(backwards bool) {
			setHarnessActive(harnessActiveNode, false)
			if session == nil {
				return
			}
			if backwards {
				session.FocusPrev()
			} else {
				session.FocusNext()
			}
		},
		OnTabNavigationFrom: func(from *dom.Node, backwards bool) {
			setHarnessActive(harnessActiveNode, false)
			if session == nil {
				return
			}
			if from != nil {
				session.FocusFrom(from, backwards)
			} else if backwards {
				session.FocusPrev()
			} else {
				session.FocusNext()
			}
		},
		HTTPClient: wptFetchAdapter{
			ctx: ctx, client: client, baseURL: baseURL,
			requests: &fetchRequests, successes: &fetchSuccesses, failures: &fetchFailures,
		},
	})
	if err != nil {
		tr.Status = "error"
		tr.Message = "create JavaScript runtime: " + err.Error()
		return tr
	}
	runtime.EnableTestUserActivationHook()
	fetchScriptImpl := func(_ string, resourceURL string) ([]byte, error) {
		parsed, err := url.Parse(resourceURL)
		if err != nil {
			return nil, fmt.Errorf("WPT script URL is outside the local WPT server aliases")
		}
		transportURL, ok := wptLocalTransportURL(parsed, baseURL)
		if !ok {
			return nil, fmt.Errorf("WPT script URL is outside the local WPT server aliases")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, transportURL.String(), nil)
		if err != nil {
			return nil, err
		}
		request.Host = parsed.Host
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("WPT script fetch: HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, js.MaxScriptBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > js.MaxScriptBytes {
			return nil, fmt.Errorf("WPT script exceeds the engine script byte limit")
		}
		if strings.HasSuffix(strings.ToLower(parsed.Path), "/resources/testharness.js") {
			marker := []byte("var tests = new Tests();")
			if at := bytes.Index(data, marker); at >= 0 {
				at += len(marker)
				data = append(data[:at], append([]byte("\n"+wptResultCollectorScript+"\n"), data[at:]...)...)
			} else {
				// Small WPT-compatible harnesses used by runner tests may not expose
				// the upstream implementation's Tests constructor.
				data = append(data, []byte("\n"+wptResultCollectorScript)...)
			}
			if len(data) > js.MaxScriptBytes {
				return nil, fmt.Errorf("WPT testharness with result collector exceeds the engine script byte limit")
			}
		}
		if strings.HasSuffix(strings.ToLower(parsed.Path), "/resources/testharnessreport.js") {
			data = disableWPTHarnessOutput(data)
			if len(data) > js.MaxScriptBytes {
				return nil, fmt.Errorf("WPT testharness report exceeds the engine script byte limit")
			}
		}
		if strings.HasSuffix(strings.ToLower(parsed.Path), "/resources/testdriver-vendor.js") {
			data = append(data, []byte("\n"+wptTestDriverAccessibilityShim+"\n"+wptTestDriverInputShim)...)
			if len(data) > js.MaxScriptBytes {
				return nil, fmt.Errorf("WPT testdriver with input shim exceeds the engine script byte limit")
			}
		}
		return data, nil
	}
	fetchScript := func(base, resourceURL string) ([]byte, error) {
		scriptRequests.Add(1)
		lastScriptURL.Store(resourceURL)
		data, err := fetchScriptImpl(base, resourceURL)
		if err != nil {
			scriptFailures.Add(1)
		}
		return data, err
	}
	fetchImage := func(_ string, resourceURL string) ([]byte, error) {
		parsed, err := url.Parse(resourceURL)
		transportURL, ok := wptLocalTransportURL(parsed, baseURL)
		if !ok {
			return nil, fmt.Errorf("WPT image URL is outside the local WPT server aliases")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, transportURL.String(), nil)
		if err != nil {
			return nil, err
		}
		request.Host = parsed.Host
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("WPT image fetch: HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, internalnet.MaxResponseBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > internalnet.MaxResponseBytes {
			return nil, fmt.Errorf("WPT image exceeds the response byte limit")
		}
		return data, nil
	}
	fetchCSS := func(_ string, resourceURL string) (string, error) {
		parsed, err := url.Parse(resourceURL)
		transportURL, ok := wptLocalTransportURL(parsed, baseURL)
		if !ok {
			return "", fmt.Errorf("WPT stylesheet URL is outside the local WPT server aliases")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, transportURL.String(), nil)
		if err != nil {
			return "", err
		}
		request.Host = parsed.Host
		response, err := client.Do(request)
		if err != nil {
			return "", err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return "", fmt.Errorf("WPT stylesheet fetch: HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, engine.MaxCSSBytes+1))
		if err != nil {
			return "", err
		}
		if len(data) > engine.MaxCSSBytes {
			return "", fmt.Errorf("WPT stylesheet exceeds the engine CSS byte limit")
		}
		return string(data), nil
	}
	loadIFrame := func(base, source string) (*dom.Document, error) {
		iframeRequests.Add(1)
		if strings.HasPrefix(strings.ToLower(source), "data:") {
			return parseWPTDataDocument(source)
		}
		documentURL, err := url.Parse(base)
		if err != nil {
			return nil, err
		}
		reference, err := url.Parse(source)
		if err != nil {
			return nil, err
		}
		frameURL := documentURL.ResolveReference(reference)
		transportURL, ok := wptLocalTransportURL(frameURL, baseURL)
		if !ok {
			return nil, fmt.Errorf("WPT iframe URL is outside the local WPT server aliases")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, transportURL.String(), nil)
		if err != nil {
			return nil, err
		}
		request.Host = frameURL.Host
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("WPT iframe fetch: HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, engine.MaxDocumentBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > engine.MaxDocumentBytes {
			return nil, fmt.Errorf("WPT iframe document exceeds the engine byte limit")
		}
		text, encoding, _ := internalnet.DecodeHTML(data, response.Header.Get("Content-Type"))
		if len(text) > engine.MaxDocumentBytes {
			return nil, fmt.Errorf("WPT iframe exceeds the engine byte limit after decoding")
		}
		limits := dom.ParseLimits{
			Nodes: engine.MaxDocumentNodes, Depth: engine.MaxDocumentDepth,
			Attributes: engine.MaxAttributes, AttributeBytes: engine.MaxAttributeBytes,
		}
		contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		var doc *dom.Document
		if isWPTXMLDocumentType(contentType) {
			doc, err = js.ParseXMLDocumentBounded(text, contentType, limits)
			if doc != nil {
				doc.ParserInput = text
			}
		} else {
			doc, err = dom.ParseBounded(text, limits)
		}
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(encoding, "utf-8") {
			encoding = "UTF-8"
		} else if strings.HasPrefix(strings.ToLower(encoding), "iso-8859-") {
			encoding = strings.ToUpper(encoding)
		}
		doc.CharacterSet = encoding
		doc.ContentLanguage = firstContentLanguage(response.Header.Values("Content-Language"))
		return doc, nil
	}
	submitIFrame := func(base, target, contentType string, body []byte) (*dom.Document, error) {
		if len(body) > engine.MaxDocumentBytes {
			return nil, fmt.Errorf("WPT form body exceeds the engine byte limit")
		}
		baseURL, err := url.Parse(base)
		if err != nil {
			return nil, err
		}
		reference, err := url.Parse(target)
		if err != nil {
			return nil, err
		}
		formURL := baseURL.ResolveReference(reference)
		transportURL, ok := wptLocalTransportURL(formURL, r.server.URL())
		if !ok {
			return nil, fmt.Errorf("WPT form target is outside the local WPT server aliases")
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, transportURL.String(), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Host = formURL.Host
		request.Header.Set("Content-Type", contentType)
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 400 {
			return nil, fmt.Errorf("WPT form target response: HTTP %d", response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, engine.MaxDocumentBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > engine.MaxDocumentBytes {
			return nil, fmt.Errorf("WPT form response exceeds the engine byte limit")
		}
		text, encoding, _ := internalnet.DecodeHTML(data, response.Header.Get("Content-Type"))
		if len(text) > engine.MaxDocumentBytes {
			return nil, fmt.Errorf("WPT form response exceeds the engine byte limit after decoding")
		}
		limits := dom.ParseLimits{
			Nodes: engine.MaxDocumentNodes, Depth: engine.MaxDocumentDepth,
			Attributes: engine.MaxAttributes, AttributeBytes: engine.MaxAttributeBytes,
		}
		responseType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
		var doc *dom.Document
		if isWPTXMLDocumentType(responseType) {
			doc, err = js.ParseXMLDocumentBounded(text, responseType, limits)
		} else if responseType == "text/plain" {
			doc = makeTextDocument(text)
		} else {
			doc, err = dom.ParseBounded(text, limits)
		}
		if err != nil {
			return nil, err
		}
		doc.CharacterSet = encoding
		doc.ContentLanguage = firstContentLanguage(response.Header.Values("Content-Language"))
		return doc, nil
	}

	documentText, documentEncoding, _ := internalnet.DecodeHTML(html, response.Header.Get("Content-Type"))
	contentType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	var parsedDocument *dom.Document
	if isWPTXMLDocumentType(contentType) {
		parsedDocument, err = js.ParseXMLDocumentBounded(documentText, contentType, dom.ParseLimits{
			Nodes: engine.MaxDocumentNodes, Depth: engine.MaxDocumentDepth,
			Attributes: engine.MaxAttributes, AttributeBytes: engine.MaxAttributeBytes,
		})
		if err != nil {
			tr.Status = "error"
			tr.Message = "parse XML WPT document: " + err.Error()
			return tr
		}
		parsedDocument.CharacterSet = documentEncoding
		parsedDocument.ContentLanguage = firstContentLanguage(response.Header.Values("Content-Language"))
	}
	options := []engine.Option{
		engine.WithViewportH(float32(r.cfg.ViewportHeight)),
		engine.WithDocumentEncoding(documentEncoding),
		engine.WithContentLanguage(firstContentLanguage(response.Header.Values("Content-Language"))),
		engine.WithJS(runtime),
		engine.WithScripts(testURL, fetchScript),
		engine.WithLinkedCSS(testURL, fetchCSS),
		engine.WithImages(testURL, fetchImage),
		engine.WithIFrameDocumentLoader(loadIFrame),
		engine.WithIFrameFormSubmitter(submitIFrame),
	}
	if parsedDocument != nil {
		options = append(options, engine.WithDocumentTree(parsedDocument))
	}
	session, err = engine.NewSession(documentText, nil, float32(r.cfg.ViewportWidth), options...)
	if err != nil {
		tr.Status = "error"
		tr.Message = "execute WPT document: " + err.Error()
		return tr
	}
	defer session.Close()
	if err := runtime.SetAccessibilityProvider(session.AccessibilityInfoForNode); err != nil {
		tr.Status = "error"
		tr.Message = "install accessibility provider: " + err.Error()
		return tr
	}

	for {
		raw, evalErr := session.EvaluateJS("window.__goosieWPTReport || ''")
		if evalErr != nil {
			tr.Status = "error"
			tr.Message = "read WPT completion: " + evalErr.Error()
			return tr
		}
		if raw != "" {
			var report wptHarnessReport
			if err := json.Unmarshal([]byte(raw), &report); err != nil {
				tr.Status = "error"
				tr.Message = "decode WPT completion: " + err.Error()
				return tr
			}
			tr.DurationMS = time.Since(started).Milliseconds()
			if !report.Complete {
				tr.Status = "error"
				tr.Message = "testharness completion callback did not report completion"
				return tr
			}
			if report.Status == 3 {
				tr.Status = "skip"
				tr.Message = "optional feature unsupported: " + report.StatusMessage
				return tr
			}
			if report.Total == 0 {
				tr.Status = "error"
				tr.Message = "testharness completed without reporting any tests"
				if state, stateErr := session.EvaluateJS(`JSON.stringify({readyState: document.readyState, started: window.__goosieWPTStarted || false, results: window.__goosieWPTResultCount || 0, fetch: typeof fetch, promise: typeof Promise, webIDL2: typeof WebIDL2, parse: typeof WebIDL2 !== 'undefined' ? typeof WebIDL2.parse : 'missing'})`); stateErr == nil && state != "" {
					tr.Message += " (page state: " + state + ", fetch requests: " + fmt.Sprint(fetchRequests.Load()) + ", fetch successes: " + fmt.Sprint(fetchSuccesses.Load()) + ", fetch failures: " + fmt.Sprint(fetchFailures.Load()) + ", pending fetches: " + fmt.Sprint(runtime.PendingFetches()) + ", script errors: " + fmt.Sprint(session.ScriptErrors()) + ")"
				}
				return tr
			}
			if report.Status == 2 || report.TimedOut != 0 {
				tr.Status = "timeout"
			} else if report.Status != 0 || report.Failed != 0 {
				tr.Status = "fail"
			} else if report.Skipped == report.Total {
				tr.Status = "skip"
			} else {
				tr.Status = "pass"
			}
			if report.Failed != 0 || report.Status != 0 {
				tr.Message = fmt.Sprintf("%d/%d testharness tests failed", report.Failed, report.Total)
				if report.StatusMessage != "" {
					tr.Message += ": testharness status: " + report.StatusMessage
				}
				if report.StatusStack != "" {
					tr.Message += " (status stack: " + strings.ReplaceAll(report.StatusStack, "\n", " <- ") + ")"
				}
				if report.Status != 0 || report.TimedOut != 0 {
					if lastError, stateErr := session.EvaluateJS("window.__goosieWPTLastError || ''"); stateErr == nil && lastError != "" {
						tr.Message += " (window error: " + lastError + ")"
					}
					if scriptErrors := session.ScriptErrors(); len(scriptErrors) > 0 {
						tr.Message += " (script errors: " + fmt.Sprint(scriptErrors) + ")"
					}
					if report.TimedOut != 0 {
						tr.Message += " (fetch requests: " + fmt.Sprint(fetchRequests.Load()) + ", successes: " + fmt.Sprint(fetchSuccesses.Load()) + ", failures: " + fmt.Sprint(fetchFailures.Load()) + ", pending: " + fmt.Sprint(runtime.PendingFetches()) + ", iframe requests: " + fmt.Sprint(iframeRequests.Load()) + ")"
					}
				}
				if len(report.Failures) > 0 {
					separator := ": "
					if report.StatusMessage != "" {
						separator = " | "
					}
					tr.Message += separator + strings.Join(report.Failures, " | ")
				}
			}
			if report.Skipped > 0 {
				if tr.Message != "" {
					tr.Message += "; "
				}
				tr.Message += fmt.Sprintf("%d/%d optional subtests skipped", report.Skipped, report.Total)
				if len(report.Skips) > 0 {
					tr.Message += ": " + strings.Join(report.Skips, " | ")
				}
			}
			return tr
		}
		if time.Now().After(started.Add(time.Duration(r.cfg.Timeout)*time.Second)) || ctx.Err() != nil {
			tr.Status = "timeout"
			tr.DurationMS = time.Since(started).Milliseconds()
			tr.Message = "testharness completion callback did not run before the timeout"
			lastScript := ""
			if value := lastScriptURL.Load(); value != nil {
				lastScript, _ = value.(string)
			}
			lastWorkerScript := ""
			if value := lastWorkerScriptURL.Load(); value != nil {
				lastWorkerScript, _ = value.(string)
			}
			if state, stateErr := session.EvaluateJS(`JSON.stringify({test: typeof test, completion: typeof add_completion_callback, readyState: document.readyState, started: window.__goosieWPTStarted || false, results: window.__goosieWPTResultCount || 0, observed: window.__goosieWPTObservedResults || [], fetchRequests: ` + fmt.Sprint(fetchRequests.Load()) + `, fetchSuccesses: ` + fmt.Sprint(fetchSuccesses.Load()) + `, fetchFailures: ` + fmt.Sprint(fetchFailures.Load()) + `, pendingFetches: ` + fmt.Sprint(runtime.PendingFetches()) + `, iframeRequests: ` + fmt.Sprint(iframeRequests.Load()) + `, lastError: window.__goosieWPTLastError || ''})`); stateErr == nil && state != "" {
				tr.Message += " (page state: " + state + ", dispatcher: " + fmt.Sprint(r.server.dispatcherStats()) + ", scripts: " + fmt.Sprint(scriptRequests.Load()) + ", script failures: " + fmt.Sprint(scriptFailures.Load()) + ", last script: " + lastScript + ", worker scripts: " + fmt.Sprint(workerScriptRequests.Load()) + ", last worker script: " + lastWorkerScript + ", dynamic script errors: " + fmt.Sprint(runtime.DynamicScriptErrors()) + ", script errors: " + fmt.Sprint(session.ScriptErrors()) + ")"
			}
			return tr
		}
		session.PumpJS(float32(r.cfg.ViewportWidth))
		time.Sleep(time.Millisecond)
	}
}

// disableWPTHarnessOutput keeps the harness's progress UI from mutating the
// document under test. Browser WPT runners normally render this output in a
// parent document; Goosie's in-document runner has no opener document.
func disableWPTHarnessOutput(source []byte) []byte {
	return append([]byte("setup({output:false});\n"), source...)
}

func makeTextDocument(text string) *dom.Document {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	head := doc.NewElement("head")
	body := doc.NewElement("body")
	pre := doc.NewElement("pre")
	pre.AppendChild(doc.NewText(text))
	body.AppendChild(pre)
	html.AppendChild(head)
	html.AppendChild(body)
	doc.Node.AppendChild(html)
	doc.HTML, doc.Head, doc.Body = html, head, body
	doc.ContentType = "text/plain"
	return doc
}

func parseWPTDataDocument(raw string) (*dom.Document, error) {
	if len(raw) > engine.MaxDocumentBytes*2 {
		return nil, fmt.Errorf("WPT data URL exceeds the encoded byte limit")
	}
	dataURL := strings.TrimPrefix(raw, "data:")
	if hash := strings.IndexByte(dataURL, '#'); hash >= 0 {
		dataURL = dataURL[:hash]
	}
	metadata, payload, ok := strings.Cut(dataURL, ",")
	if !ok {
		return nil, fmt.Errorf("malformed WPT data URL")
	}
	parts := strings.Split(metadata, ";")
	contentType := "text/plain"
	start := 0
	if len(parts) > 0 && strings.Contains(parts[0], "/") {
		contentType = strings.ToLower(parts[0])
		start = 1
	}
	isBase64 := false
	for _, part := range parts[start:] {
		if strings.EqualFold(part, "base64") {
			isBase64 = true
		}
	}
	var data []byte
	var err error
	if isBase64 {
		data, err = base64.StdEncoding.DecodeString(payload)
	} else {
		var decoded string
		decoded, err = url.PathUnescape(payload)
		data = []byte(decoded)
	}
	if err != nil {
		return nil, fmt.Errorf("decode WPT data URL: %w", err)
	}
	if len(data) > engine.MaxDocumentBytes {
		return nil, fmt.Errorf("WPT data document exceeds the engine byte limit")
	}
	text, encoding, _ := internalnet.DecodeHTML(data, contentType)
	limits := dom.ParseLimits{
		Nodes: engine.MaxDocumentNodes, Depth: engine.MaxDocumentDepth,
		Attributes: engine.MaxAttributes, AttributeBytes: engine.MaxAttributeBytes,
	}
	var doc *dom.Document
	if isWPTXMLDocumentType(contentType) {
		doc, err = js.ParseXMLDocumentBounded(text, contentType, limits)
		if doc != nil {
			doc.ParserInput = text
		}
	} else {
		doc, err = dom.ParseBounded(text, limits)
	}
	if err != nil {
		return nil, err
	}
	doc.CharacterSet = encoding
	doc.ContentType = contentType
	return doc, nil
}

// anyJSTestDocument adapts the Window variant of a WPT .any.js test to the
// existing document-based testharness runner. META script dependencies remain
// relative to the original WPT path because the generated document uses that
// path as its URL.
func anyJSTestDocument(testPath string, source []byte) []byte {
	var dependencies []string
	for _, line := range strings.Split(string(source), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "// META:") {
			continue
		}
		meta := strings.TrimSpace(strings.TrimPrefix(line, "// META:"))
		key, value, ok := strings.Cut(meta, "=")
		if !ok || strings.TrimSpace(key) != "script" {
			continue
		}
		dependency := strings.TrimSpace(value)
		if dependency == "" || strings.HasSuffix(dependency, "/resources/testharness.js") || strings.HasSuffix(dependency, "/resources/testharnessreport.js") {
			continue
		}
		dependencies = append(dependencies, dependency)
	}

	var document strings.Builder
	// Match WPT's AnyHtmlHandler window wrapper. The per-global helpers must
	// exist before the testharness and the .any.js source execute.
	document.WriteString("<!doctype html><html><head><meta charset=utf-8><script>self.GLOBAL = { isWindow: function() { return true; }, isWorker: function() { return false; }, isShadowRealm: function() { return false; } };</script><script src='/resources/testharness.js'></script><script src='/resources/testharnessreport.js'></script>")
	for _, dependency := range dependencies {
		document.WriteString("<script src='")
		document.WriteString(htmlstd.EscapeString(dependency))
		document.WriteString("'></script>")
	}
	document.WriteString("</head><body><script src='/")
	document.WriteString(htmlstd.EscapeString(strings.ReplaceAll(testPath, "\\", "/")))
	document.WriteString("'></script></body></html>")
	return []byte(document.String())
}

func firstContentLanguage(values []string) string {
	if len(values) == 0 {
		return ""
	}
	value := strings.TrimSpace(values[0])
	if i := strings.IndexByte(value, ','); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	return value
}

func sameOrigin(candidate *url.URL, base string) bool {
	if candidate == nil {
		return false
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return false
	}
	if !strings.EqualFold(candidate.Scheme, baseURL.Scheme) || candidate.Port() != baseURL.Port() {
		return false
	}
	if strings.EqualFold(candidate.Hostname(), baseURL.Hostname()) {
		return true
	}
	// The WPT server's *.localhost names all route to loopback. Permit those
	// hostnames for transport while the runtime still enforces their distinct
	// document origins.
	return isLocalhostHost(candidate.Hostname()) && isLocalhostHost(baseURL.Hostname())
}

// wptLocalTransportURL maps a WPT loopback origin to the one local HTTP
// listener. The document keeps candidate's original scheme and host in its
// Runtime URL; this mapping only chooses where the harness fetches its bytes.
func wptLocalTransportURL(candidate *url.URL, serverBase string) (*url.URL, bool) {
	if candidate == nil || candidate.User != nil || (candidate.Scheme != "http" && candidate.Scheme != "https") {
		return nil, false
	}
	base, err := url.Parse(serverBase)
	if err != nil || base.Scheme != "http" || base.User != nil || base.Port() == "" ||
		!isWPTLocalHostAlias(base.Hostname()) || candidate.Port() != base.Port() || !isWPTLocalHostAlias(candidate.Hostname()) {
		return nil, false
	}
	transport := *base
	transport.Path = candidate.Path
	transport.RawPath = candidate.RawPath
	transport.RawQuery = candidate.RawQuery
	transport.ForceQuery = candidate.ForceQuery
	transport.Fragment = ""
	transport.RawFragment = ""
	return &transport, true
}

func isWPTLocalHostAlias(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "wpt.localhost" || host == "wpt-other.localhost" ||
		strings.HasSuffix(host, ".wpt.localhost") || strings.HasSuffix(host, ".wpt-other.localhost")
}

func isLocalhostHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || strings.HasSuffix(host, ".localhost")
}

func isWPTXMLDocumentType(mediaType string) bool {
	switch mediaType {
	case "application/xhtml+xml", "application/xml", "image/svg+xml", "text/xml":
		return true
	default:
		return false
	}
}

const wptResultCollectorScript = `(function () {
  if (typeof add_completion_callback !== "function") return;
  if (typeof window.addEventListener === "function") {
    window.addEventListener("error", function (event) {
      window.__goosieWPTLastError = String(event.message || "error") + " at " + String(event.filename || "") + ":" + String(event.lineno || 0) + ":" + String(event.colno || 0);
    }, true);
  }
  if (typeof add_start_callback === "function") {
    add_start_callback(function () { window.__goosieWPTStarted = true; });
  }
  if (typeof add_result_callback === "function") {
    add_result_callback(function (test) {
      window.__goosieWPTResultCount = (window.__goosieWPTResultCount || 0) + 1;
      var observed = window.__goosieWPTObservedResults || (window.__goosieWPTObservedResults = []);
      if (observed.length < 20) {
        observed.push({
          name: String(test && test.name || "unnamed test"),
          status: Number(test && test.status || 0),
          message: String(test && test.message || "")
        });
      }
    });
  }
  add_completion_callback(function (tests, status) {
    var failures = [];
    var skips = [];
    var failed = 0;
    var skipped = 0;
    var timedOut = 0;
    for (var i = 0; i < tests.length; i++) {
      var test = tests[i];
      if (test.status === 4) {
        skipped++;
        if (skips.length < 20) {
          var reason = String(test.name || "unnamed test") + ": " + String(test.message || "optional feature unsupported");
          skips.push(reason.slice(0, 512));
        }
        continue;
      }
      if (test.status !== 0) {
        failed++;
        if (test.status === 2) timedOut++;
        if (failures.length < 20) {
          var detail = test.message;
          if (!detail && test.stack) detail = test.stack;
          if (!detail && test.error) detail = test.error.message || test.error;
          if (!detail) detail = "failed (status " + String(test.status) + ")";
          var message = String(test.name || "unnamed test") + ": " + String(detail);
          failures.push(message.slice(0, 512));
        }
      }
    }
    window.__goosieWPTReport = JSON.stringify({
      complete: true,
      status: Number(status.status || 0),
      statusMessage: String(status.message || ""),
      statusStack: String(status.stack || ""),
      total: tests.length,
      failed: failed,
      skipped: skipped,
      timedOut: timedOut,
      failures: failures,
      skips: skips
    });
  });
})();`

const wptTestDriverInputShim = `(function () {
  if (!window.test_driver) return;
  const heldKeys = new Set();
  const parentWindow = window;
  const activeSpaceKeys = new Map();
  const pointerPositions = new Map();
  const pointerCoordinates = new Map();
  const activePointers = new Map();
  const suppressedPointers = new Set();
  let navigationAnchor = null;
  const numberEdits = new WeakMap();
  const emailEdits = new WeakMap();
  function inConnectedDocument(element) {
    let doc = element && element.ownerDocument;
    while (doc && doc.defaultView && doc.defaultView.frameElement) {
      const frame = doc.defaultView.frameElement;
      if (!frame.parentNode) return false;
      doc = frame.ownerDocument;
    }
    return !!doc;
  }
  function spinNumberInput(element, direction) {
    if (!element || element.localName !== "input" || String(element.type || "").toLowerCase() !== "number" ||
        (element.matches && element.matches(":disabled")) || element.readOnly) return false;
    const beforeinput = new InputEvent("beforeinput", {bubbles: true, cancelable: true, inputType: "insertReplacementText", data: null});
    element.dispatchEvent(beforeinput);
    if (beforeinput.defaultPrevented || !inConnectedDocument(element)) return true;
    const current = Number(element.value || 0);
    const stepAttr = String(element.getAttribute("step") || "1");
    const step = stepAttr === "any" ? 1 : (Number(stepAttr) || 1);
    let next = current + direction * step;
    const min = Number(element.getAttribute("min"));
    const max = Number(element.getAttribute("max"));
    if (Number.isFinite(min)) next = Math.max(min, next);
    if (Number.isFinite(max)) next = Math.min(max, next);
    element.value = String(next);
    const targetWindow = element.ownerDocument && element.ownerDocument.defaultView || window;
    if (typeof targetWindow.__goosieNotifyUserInput === "function") targetWindow.__goosieNotifyUserInput(element);
    element.dispatchEvent(new InputEvent("input", {bubbles: true, inputType: "insertReplacementText", data: null}));
    element.dispatchEvent(new Event("change", {bubbles: true}));
    return true;
  }
  let heldPointer = null;
  const isSpaceActivatable = el => {
    if (!el || el.disabled || (el.matches && el.matches(":disabled"))) return false;
    if (el.localName === "button" || el.localName === "summary") return true;
    if (el.localName !== "input") return false;
    return ["button", "submit", "reset", "checkbox", "radio", "image"].includes(String(el.type).toLowerCase());
  };
  const setTestActive = (target, active) => {
    if (target && typeof window.__goosieSetActiveState === "function") window.__goosieSetActiveState(target, active);
  };
  window.test_driver.action_sequence = async function (sources, context) {
    const window = context || parentWindow;
    const document = window.document || parentWindow.document;
    const keyNames = {"\uE003": "Backspace", "\uE004": "Tab", "\uE006": "Enter", "\uE007": "Enter", "\uE008": "Shift", "\uE009": "Control", "\uE00A": "Alt", "\uE03D": "Meta", "\u2318": "Meta", "\uE00C": "Escape", "\uE00D": "Space", "\uE00E": "PageUp", "\uE00F": "PageDown", "\uE010": "End", "\uE011": "Home", "\uE012": "ArrowLeft", "\uE013": "ArrowUp", "\uE014": "ArrowRight", "\uE015": "ArrowDown", "\uE016": "Insert", "\uE017": "Delete", "\uE018": ";", "\uE019": "="};
    const keyCode = key => {
      if (/^[a-z]$/i.test(key)) return "Key" + key.toUpperCase();
      if (/^[0-9]$/.test(key)) return "Digit" + key;
      return ({" ": "Space", "Space": "Space", "Enter": "Enter", "Tab": "Tab", "ArrowLeft": "ArrowLeft", "ArrowRight": "ArrowRight", "ArrowUp": "ArrowUp", "ArrowDown": "ArrowDown", "Shift": "ShiftLeft", "Control": "ControlLeft", "Alt": "AltLeft", "Meta": "MetaLeft"})[key] || "";
    };
    const focusables = () => Array.from(document.querySelectorAll("a[href],area[href],button,input,select,textarea,iframe,[tabindex]"))
      .filter(el => !el.disabled && el.tabIndex >= 0 && el.getClientRects().length);
    const radioArrow = (current, key) => {
      if (!current || current.localName !== "input" || String(current.type).toLowerCase() !== "radio" || !current.name) return;
      const radios = Array.from(document.querySelectorAll("input[type=radio]"))
        .filter(el => el.name === current.name && el.form === current.form && !el.disabled && el.getClientRects().length);
      const index = radios.indexOf(current);
      if (index < 0 || radios.length < 2) return;
      const forward = key === "ArrowRight" || key === "ArrowDown";
      const target = radios[(index + (forward ? 1 : radios.length - 1)) % radios.length];
      target.focus();
      target.click();
    };
    const spaceActivatable = el => {
      if (!el || el.disabled || (el.matches && el.matches(":disabled"))) return false;
      if (el.localName === "button" || el.localName === "summary") return true;
      if (el.localName !== "input") return false;
      return ["button", "submit", "reset", "checkbox", "radio", "image"].includes(String(el.type).toLowerCase());
    };
    const sequence = Array.isArray(sources) ? sources : [];
    const setActive = (target, active) => {
      if (target && typeof window.__goosieSetActiveState === "function") window.__goosieSetActiveState(target, active);
    };
    const dispatchPointer = (target, type, button, id) => {
      if (!target || typeof target.dispatchEvent !== "function") return;
      const eventType = type === "pointerDown" ? "pointerdown" : type === "pointerUp" ? "pointerup" : "pointermove";
      const mouseType = type === "pointerDown" ? "mousedown" : type === "pointerUp" ? "mouseup" : "mousemove";
      if (!suppressedPointers.has(id)) target.dispatchEvent(new Event(eventType, {bubbles: true, cancelable: true}));
      target.dispatchEvent(new MouseEvent(mouseType, {bubbles: true, cancelable: true, button: button || 0, buttons: type === "pointerDown" ? 1 : 0}));
    };
    const actionPoint = (action, target, id) => {
      const previous = pointerCoordinates.get(id);
      const hasX = Number.isFinite(Number(action.x));
      const hasY = Number.isFinite(Number(action.y));
      const x = hasX ? Number(action.x) : 0;
      const y = hasY ? Number(action.y) : 0;
      if (action.origin === "pointer") {
        return {x: (previous ? previous.x : 0) + x, y: (previous ? previous.y : 0) + y};
      }
      if (action.origin && typeof action.origin.dispatchEvent === "function") {
        const rect = action.origin.getBoundingClientRect ? action.origin.getBoundingClientRect() : null;
        return rect ? {x: rect.left + rect.width / 2 + x, y: rect.top + rect.height / 2 + y} : {x, y};
      }
      if (hasX && hasY) return {x, y};
      if (previous) return previous;
      const rect = target && target.getBoundingClientRect ? target.getBoundingClientRect() : null;
      return rect ? {x: rect.left + rect.width / 2, y: rect.top + rect.height / 2} : {x: 0, y: 0};
    };
    const imageMapAreaAtPoint = (target, point) => {
      if (!target || target.localName !== "img" || !target.useMap || !target.useMap.startsWith("#")) return null;
      const mapName = target.useMap.slice(1);
      const map = Array.from(document.querySelectorAll("map[name],map[id]")).find(candidate => candidate.name === mapName || candidate.id === mapName);
      const rect = target.getBoundingClientRect && target.getBoundingClientRect();
      if (!map || !rect || rect.width <= 0 || rect.height <= 0) return null;
      const width = target.naturalWidth || rect.width;
      const height = target.naturalHeight || rect.height;
      const x = (point.x - rect.left) * width / rect.width;
      const y = (point.y - rect.top) * height / rect.height;
      let defaultArea = null;
      for (const area of map.querySelectorAll("area")) {
        const shape = String(area.shape || "rect").toLowerCase();
        const coords = String(area.coords || "").split(/[\\s,]+/).filter(Boolean).map(Number);
        if (shape === "default") { if (!defaultArea) defaultArea = area; continue; }
        let hit = false;
        if ((shape === "rect" || shape === "rectangle") && coords.length >= 4) {
          const left = Math.min(coords[0], coords[2]), right = Math.max(coords[0], coords[2]);
          const top = Math.min(coords[1], coords[3]), bottom = Math.max(coords[1], coords[3]);
          hit = x >= left && x <= right && y >= top && y <= bottom;
        } else if ((shape === "circle" || shape === "circ") && coords.length >= 3) {
          const dx = x - coords[0], dy = y - coords[1];
          hit = dx * dx + dy * dy <= coords[2] * coords[2];
        } else if ((shape === "poly" || shape === "polygon") && coords.length >= 6) {
          let inside = false;
          for (let i = 0, j = coords.length - 2; i < coords.length; j = i, i += 2) {
            const xi = coords[i], yi = coords[i + 1], xj = coords[j], yj = coords[j + 1];
            if ((yi > y) !== (yj > y) && x < (xj - xi) * (y - yi) / (yj - yi) + xi) inside = !inside;
          }
          hit = inside;
        }
        if (hit) return area;
      }
      return defaultArea;
    };
    const routePointerAction = (phase, point, button) => {
      if (typeof window.__goosieTestDriverPointer !== "function") return false;
      return window.__goosieTestDriverPointer(phase, point.x, point.y, button || 0);
    };
    // Native WebDriver pointer input performs text selection. The headless
    // adapter dispatches synthetic events, so model the selection endpoints
    // for element-origin actions that land on direct Text children.
    const updatePointerTextSelection = (target, phase) => {
      if (!target || !target.firstChild || target.firstChild.nodeType !== 3) return;
      const selection = window.getSelection();
      if (!selection) return;
      if (phase === "start") selection.collapse(target.firstChild, 0);
      else if (phase === "extend" && selection.extend) selection.extend(target.firstChild, target.firstChild.length);
    };
    const placeEditableCaretAtPoint = (target, point) => {
      if (!target || !target.isContentEditable || !point) return;
      let host = target;
      while (host && host.isContentEditable && !host.hasAttribute("contenteditable")) host = host.parentElement;
      if (!host || !host.isContentEditable) return;
      let lastText = null;
      const findLastText = node => {
        for (let child = node && node.lastChild; child; child = child.previousSibling) {
          if (child.nodeType === 3) { lastText = child; return true; }
          if (findLastText(child)) return true;
        }
        return false;
      };
      if (!findLastText(target)) return;
      const selection = window.getSelection();
      if (!selection) return;
      const rect = host.getBoundingClientRect && host.getBoundingClientRect();
      const textWidth = Math.max(1, String(target.textContent || "").length) * 8;
      const offset = rect && point.x - rect.left >= textWidth ? lastText.length : 0;
      selection.collapse(lastText, offset);
    };
    const maxTicks = sequence.reduce((max, source) => Math.max(max, (source.actions || []).length), 0);
    for (let tick = 0; tick < maxTicks; tick++) {
      for (const source of sequence) {
        const action = (source.actions || [])[tick];
        if (!action || action.type === "pause" || source.type === "none") continue;
        if (source.type === "key") {
          const raw = String(action.value || "");
          const key = keyNames[raw] || raw;
          const isSpace = key === "Space" || key === " " || raw === "\uE00D";
          const target = document.activeElement || document.body;
          if (action.type !== "keyDown" && action.type !== "keyUp") throw new Error("Goosie testdriver action_sequence does not support key action " + action.type);
          const down = action.type === "keyDown";
          const shift = heldKeys.has("Shift") || key === "Shift";
          const ctrl = heldKeys.has("Control") || key === "Control";
          const alt = heldKeys.has("Alt") || key === "Alt";
          const meta = heldKeys.has("Meta") || key === "Meta";
          const eventKey = shift && /^[a-z]$/.test(key) ? key.toUpperCase() : key;
          const ev = new KeyboardEvent(down ? "keydown" : "keyup", {
            key: eventKey, code: keyCode(key), bubbles: true, cancelable: true,
            shiftKey: shift, ctrlKey: ctrl, altKey: alt, metaKey: meta
          });
          target.dispatchEvent(ev);
          let defaultPrevented = ev.defaultPrevented;
          const printable = eventKey.length === 1 && eventKey.charCodeAt(0) >= 0x20 && eventKey.charCodeAt(0) < 0xE000;
          if (down && !defaultPrevented && printable && !ctrl && !alt && !meta) {
            const keypress = new KeyboardEvent("keypress", {
              key: eventKey, code: keyCode(key), bubbles: true, cancelable: true,
              shiftKey: shift, ctrlKey: ctrl, altKey: alt, metaKey: meta
            });
            target.dispatchEvent(keypress);
            defaultPrevented = keypress.defaultPrevented;
          }
          if (down) heldKeys.add(key); else heldKeys.delete(key);
          if (down && key === "Escape" && !defaultPrevented && typeof window.__goosieEscapeKey === "function") {
            window.__goosieEscapeKey();
          }
          if (down && isSpace && !defaultPrevented && spaceActivatable(target)) {
            activeSpaceKeys.set(key, target);
            setActive(target, true);
          }
          if (down && !defaultPrevented) {
            if (key === "Tab") {
              if (navigationAnchor && typeof window.__goosieTabNavigationFrom === "function") {
                window.__goosieTabNavigationFrom(navigationAnchor, shift);
                navigationAnchor = null;
              } else if (typeof window.__goosieTabNavigation === "function") window.__goosieTabNavigation(shift);
              else {
                const list = focusables();
                if (list.length) {
                  const at = list.indexOf(document.activeElement);
                  const delta = shift ? -1 : 1;
                  list[(at + delta + list.length) % list.length].focus();
                }
              }
            } else if (["ArrowLeft", "ArrowRight", "ArrowUp", "ArrowDown"].includes(key)) {
              radioArrow(document.activeElement, key);
            } else if (key === "Enter" && target && typeof target.click === "function") {
              if (target.localName === "button" || target.localName === "a" || target.localName === "input") target.click();
            }
            const isEditingKey = key === "Backspace" || key === "Delete" || key === "Enter";
            const isWordDeletion = alt && (key === "Backspace" || key === "Delete");
            const isEditableCaretKey = (key === "ArrowLeft" || key === "ArrowRight") &&
              (target.isContentEditable || target.localName === "input" || target.localName === "textarea");
            if ((isWordDeletion || !alt && ((ctrl || meta) || isEditingKey || isEditableCaretKey || (key.length === 1 && key.charCodeAt(0) >= 0x20))) &&
                typeof window.__goosieTestDriverKeyDefault === "function") {
              window.__goosieTestDriverKeyDefault(eventKey, shift, ctrl, alt, meta);
            }
          }
          if (!down && isSpace) {
            const pressed = activeSpaceKeys.get(key);
            if (pressed) {
              activeSpaceKeys.delete(key);
              setActive(pressed, false);
              if (!ev.defaultPrevented && document.activeElement === pressed && typeof pressed.click === "function") pressed.click();
            }
          }
          continue;
        }
        if (source.type === "pointer") {
          const id = source.id;
          if (action.type === "pointerMove") {
            let target = action.origin && typeof action.origin.dispatchEvent === "function" ? action.origin : null;
            if (!target && action.origin === "pointer") target = pointerPositions.get(id);
            if (!target && typeof document.elementFromPoint === "function") target = document.elementFromPoint(Number(action.x) || 0, Number(action.y) || 0);
            if (!target || target === document.body || target === document.documentElement) {
              const x = Number(action.x) || 0, y = Number(action.y) || 0;
              const numberInputs = Array.from(document.querySelectorAll("input[type=number]"));
              const numberInput = numberInputs.find(el => {
                const rect = el.getBoundingClientRect();
                return (x >= rect.left && x <= rect.left + rect.width && y >= rect.top && y <= rect.top + rect.height) ||
                  (numberInputs.length === 1 && rect.width === 0 && rect.height === 0 && x >= rect.left - 20 && x <= rect.left + 20 && y === rect.top);
              });
              if (numberInput) target = numberInput;
            }
            if (!target || typeof target.dispatchEvent !== "function") target = document.body;
            pointerPositions.set(id, target);
            const point = actionPoint(action, target, id);
            pointerCoordinates.set(id, point);
            dispatchPointer(target, "pointerMove", 0, id);
            if (heldPointer) updatePointerTextSelection(target, "extend");
            const wasDragging = activePointers.get(id) && activePointers.get(id).dragging;
            const dragging = routePointerAction("move", point, activePointers.has(id) ? 0 : -1);
            const pressedPointer = activePointers.get(id);
            if (dragging && pressedPointer) pressedPointer.dragging = true;
            if (dragging && !wasDragging && pressedPointer && !suppressedPointers.has(id)) {
              const pressedTarget = pressedPointer.target;
              pressedTarget.dispatchEvent(new Event("pointercancel", {bubbles: true, cancelable: false}));
              pressedTarget.dispatchEvent(new Event("pointerout", {bubbles: true, cancelable: true}));
              pressedTarget.dispatchEvent(new Event("pointerleave", {bubbles: false, cancelable: false}));
              suppressedPointers.add(id);
            }
          } else if (action.type === "pointerDown") {
            const target = pointerPositions.get(id) || document.body;
            const activeTarget = target.control || target;
            const point = actionPoint(action, target, id);
            pointerCoordinates.set(id, point);
            heldPointer = {target, activeTarget};
            activePointers.set(id, heldPointer);
            setActive(activeTarget, true);
            dispatchPointer(target, "pointerDown", action.button, id);
            if (typeof window.__goosiePointerDown === "function") window.__goosiePointerDown(target, id, action.button);
            routePointerAction("down", point, action.button);
            // A primary pointer press focuses a focusable editing host in a
            // browser. The headless adapter synthesizes the click itself, so
            // apply that focus default before subsequent testdriver keys.
            if (action.button === 0 && activeTarget && typeof activeTarget.focus === "function") activeTarget.focus();
            updatePointerTextSelection(target, "start");
            placeEditableCaretAtPoint(target, point);
          } else if (action.type === "pointerUp") {
            const pressed = activePointers.get(id) || heldPointer;
            const target = pointerPositions.get(id) || (heldPointer && heldPointer.target) || document.body;
            const point = actionPoint(action, target, id);
            const numberTarget = target.control || target;
            const numberRect = numberTarget && numberTarget.getBoundingClientRect && numberTarget.getBoundingClientRect();
            if (numberRect && numberTarget.localName === "input" && String(numberTarget.type || "").toLowerCase() === "number" &&
                point.x >= numberRect.left + numberRect.width - 20 && point.y >= numberRect.top && point.y <= numberRect.top + numberRect.height) {
              spinNumberInput(numberTarget, point.y < numberRect.top + numberRect.height / 2 ? 1 : -1);
            }
            // Popover light dismiss is a pointer default action that runs
            // after mousedown and before pointerup/mouseup are dispatched.
            if (typeof window.__goosiePointerUp === "function") window.__goosiePointerUp(target, id, action.button);
            const dragFinished = routePointerAction("up", point, action.button);
            dispatchPointer(target, "pointerUp", action.button, id);
            if (action.button === 2) {
              target.dispatchEvent(new MouseEvent("contextmenu", {bubbles: true, cancelable: true, button: 2}));
            }
            if (pressed) {
              activePointers.delete(id);
              heldPointer = null;
              setActive(pressed.activeTarget, false);
              const hadDrag = dragFinished || pressed.dragging;
              if (!hadDrag && action.button === 0 && pressed.target === target && typeof target.click === "function") {
                const area = imageMapAreaAtPoint(target, point);
                (area || target).click();
              }
            }
            suppressedPointers.delete(id);
            navigationAnchor = target;
          } else {
            throw new Error("Goosie testdriver action_sequence does not support pointer action " + action.type);
          }
          continue;
        }
        throw new Error("Goosie testdriver action_sequence does not support source " + source.type);
      }
    }
  };
  window.test_driver.click = function (element) {
    return Promise.resolve().then(function () {
      if (!element || typeof element.click !== "function") throw new TypeError("test_driver.click requires an Element");
      if (element.localName === "input" && String(element.type || "").toLowerCase() === "number") numberEdits.delete(element);
      if (element.localName === "input" && String(element.type || "").toLowerCase() === "email") emailEdits.delete(element);
      if (element.isContentEditable) {
        let focusTarget = element;
        while (focusTarget && focusTarget.isContentEditable && !focusTarget.hasAttribute("contenteditable")) focusTarget = focusTarget.parentElement;
        if (focusTarget && focusTarget.isContentEditable && typeof focusTarget.focus === "function") focusTarget.focus();
        let textNode = null;
        const findLastText = node => {
          for (let child = node && node.lastChild; child; child = child.previousSibling) {
            if (child.nodeType === 3) { textNode = child; return true; }
            if (findLastText(child)) return true;
          }
          return false;
        };
        if (findLastText(element)) {
          const selection = element.ownerDocument && element.ownerDocument.getSelection();
          if (selection) selection.collapse(textNode, textNode.length);
        }
      } else if (typeof element.focus === "function") element.focus();
      const targetWindow = element.ownerDocument && element.ownerDocument.defaultView || window;
      if (typeof targetWindow.__goosieGrantUserActivation === "function") targetWindow.__goosieGrantUserActivation();
      if (typeof targetWindow.__goosieTestDriverClick === "function") targetWindow.__goosieTestDriverClick(element);
      else element.click();
    });
  };
  window.test_driver.send_keys = function (element, keys) {
    const activeEditableAncestor = document.activeElement && document.activeElement !== element &&
      document.activeElement.isContentEditable && document.activeElement.contains && document.activeElement.contains(element);
    if (element && document.activeElement !== element && !activeEditableAncestor && typeof element.focus === "function") {
      const selection = element.ownerDocument && element.ownerDocument.getSelection && element.ownerDocument.getSelection();
      let hadSelectionInTarget = false;
      if (selection && selection.rangeCount) {
        const anchor = selection.anchorNode;
        hadSelectionInTarget = !!anchor && (anchor === element || element.contains(anchor));
      }
      element.focus();
      if (!hadSelectionInTarget && element.isContentEditable) {
        let textNode = null;
        const findLastText = node => {
          for (let child = node && node.lastChild; child; child = child.previousSibling) {
            if (child.nodeType === 3) { textNode = child; return true; }
            if (findLastText(child)) return true;
          }
          return false;
        };
        if (findLastText(element) && selection) selection.collapse(textNode, textNode.length);
      } else if (!hadSelectionInTarget && (element.localName === "input" || element.localName === "textarea") && typeof element.setSelectionRange === "function") {
        const end = String(element.value || "").length;
        element.setSelectionRange(end, end);
      }
    }
    for (const key of String(keys)) {
          const keyNames = {"\uE003": "Backspace", "\uE004": "Tab", "\uE006": "Enter", "\uE007": "Enter", "\uE00C": "Escape", "\uE012": "ArrowLeft", "\uE013": "ArrowUp", "\uE014": "ArrowRight", "\uE015": "ArrowDown", "\uE017": "Delete"};
      const keyName = keyNames[key] || key;
      const code = ["Backspace", "Delete", "Enter", "Escape"].includes(keyName) ? keyName : (/^[a-z]$/i.test(key) ? "Key" + key.toUpperCase() : "");
      const keydown = new KeyboardEvent("keydown", {key: keyName, code, bubbles: true, cancelable: true});
      element.dispatchEvent(keydown);
      let keyCanceled = keydown.defaultPrevented;
      if (!keyCanceled && key.length === 1 && key.charCodeAt(0) >= 0x20 && key.charCodeAt(0) < 0xE000) {
        const keypress = new KeyboardEvent("keypress", {key: keyName, code, bubbles: true, cancelable: true});
        element.dispatchEvent(keypress);
        keyCanceled = keypress.defaultPrevented;
      }
      const activateSpace = keyName === " " && !keydown.defaultPrevented && isSpaceActivatable(element);
      if (activateSpace) setTestActive(element, true);
      const finishKey = () => {
        const keyup = new KeyboardEvent("keyup", {key: keyName, code, bubbles: true, cancelable: true});
        element.dispatchEvent(keyup);
        if (activateSpace) {
          setTestActive(element, false);
          if (!keyup.defaultPrevented && document.activeElement === element) {
            if (typeof window.__goosieTestDriverClick === "function") window.__goosieTestDriverClick(element);
            else if (typeof element.click === "function") element.click();
          }
        }
      };
      if (keyName === "Escape" && !keydown.defaultPrevented && typeof window.__goosieEscapeKey === "function") {
        window.__goosieEscapeKey();
      }
      if (keyName === "Tab" && !keydown.defaultPrevented) {
        const backwards = !!(keydown.shiftKey || heldKeys.has("Shift"));
        if (navigationAnchor && typeof window.__goosieTabNavigationFrom === "function") {
          window.__goosieTabNavigationFrom(navigationAnchor, backwards);
          navigationAnchor = null;
        } else if (typeof window.__goosieTabNavigation === "function") {
          window.__goosieTabNavigation(backwards);
        }
      }
      const inputTypeForImplicitSubmission = element.localName === "input" ? String(element.type || "text").toLowerCase() : "";
      const supportsImplicitSubmission = element.localName === "input" &&
        !["button", "submit", "reset", "image", "checkbox", "radio", "file", "hidden", "color", "range"].includes(inputTypeForImplicitSubmission);
      if (keyName === "Enter" && element.form && supportsImplicitSubmission) {
        if (!keyCanceled) {
        let hasSubmitButton = false;
        let submitButton = null;
        for (let i = 0; i < element.form.elements.length; i++) {
          const control = element.form.elements[i];
          const type = String(control.type || "").toLowerCase();
          const isSubmit = (control.localName === "button" && type !== "button" && type !== "reset") ||
            (control.localName === "input" && (type === "submit" || type === "image"));
          if (isSubmit) {
            hasSubmitButton = true;
            if (!control.disabled) submitButton = control;
            break;
          }
        }
        if (submitButton) submitButton.click();
        else if (!hasSubmitButton) element.form.requestSubmit();
        }
        finishKey();
        continue;
      }
      if (!keyCanceled && (element.localName === "input" || element.localName === "textarea")) {
        const type = String(element.type || "text").toLowerCase();
        const readonlyApplies = element.localName === "textarea" || ["text", "search", "url", "tel", "email", "password", "date", "month", "week", "time", "datetime-local", "number"].includes(type);
        const mutable = !(element.matches && element.matches(":disabled")) && !(element.readOnly && readonlyApplies);
        if (!mutable) {
          finishKey();
          continue;
        }

        if (element.localName === "input" && type === "number") {
          if (keyName === "ArrowUp" || keyName === "ArrowDown") {
            spinNumberInput(element, keyName === "ArrowUp" ? 1 : -1);
            finishKey();
            continue;
          }
          let edit = numberEdits.get(element);
          const visible = String(element.value || "");
          if (!edit || edit.visible !== visible) edit = {raw: visible, visible, caret: visible.length};
          let next = edit.raw;
          let caret = edit.caret;
          let inputType = "insertText";
          if (keyName === "ArrowLeft") {
            edit.caret = Math.max(0, caret - 1);
            numberEdits.set(element, edit);
          } else if (keyName === "ArrowRight") {
            edit.caret = Math.min(edit.raw.length, caret + 1);
            numberEdits.set(element, edit);
          } else {
            if (keyName === "Backspace") {
              if (caret > 0) { next = edit.raw.slice(0, caret - 1) + edit.raw.slice(caret); caret--; }
              inputType = "deleteContentBackward";
            } else if (keyName === "Delete") {
              if (caret < edit.raw.length) next = edit.raw.slice(0, caret) + edit.raw.slice(caret + 1);
              inputType = "deleteContentForward";
            } else if (key.length === 1 && key.charCodeAt(0) < 0xE000) {
              next = edit.raw.slice(0, caret) + key + edit.raw.slice(caret);
              caret++;
            }
            if (next !== edit.raw) {
              const validNumberEdit = /^-?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$/.test(next);
              const partialNumberEdit = /^-?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]*)?(?:[eE][+-]?[0-9]*)?$/.test(next) && next !== "";
              if (validNumberEdit || partialNumberEdit) {
                const beforeinput = new InputEvent("beforeinput", {bubbles: true, cancelable: true, inputType, data: inputType.startsWith("delete") ? null : key});
                element.dispatchEvent(beforeinput);
                if (beforeinput.defaultPrevented) { finishKey(); continue; }
                const normalized = validNumberEdit ? next.replace(/\.$/, "") : "";
                element.value = normalized;
                edit = {raw: next, visible: String(element.value || ""), caret};
                numberEdits.set(element, edit);
                if (typeof window.__goosieSetNumberBadInput === "function") window.__goosieSetNumberBadInput(element, !validNumberEdit);
                if (typeof window.__goosieNotifyUserInput === "function") window.__goosieNotifyUserInput(element);
                element.dispatchEvent(new InputEvent("input", {bubbles: true, inputType, data: inputType.startsWith("delete") ? null : key}));
              }
            }
          }
          finishKey();
          continue;
        }

        if (element.localName === "input" && type === "email") {
          let edit = emailEdits.get(element);
          const visible = String(element.value || "");
          if (!edit || edit.visible !== visible) edit = {raw: visible, visible};
          let next = edit.raw;
          let inputType = "insertText";
          if (keyName === "Backspace") {
            next = next.slice(0, -1);
            inputType = "deleteContentBackward";
          } else if (keyName === "Delete") {
            inputType = "deleteContentForward";
          } else if (key.length === 1 && key.charCodeAt(0) < 0xE000) {
            next += key;
          }
          if (next !== edit.raw) {
            const beforeinput = new InputEvent("beforeinput", {bubbles: true, cancelable: true, inputType, data: inputType.startsWith("delete") ? null : key});
            element.dispatchEvent(beforeinput);
            if (beforeinput.defaultPrevented) { finishKey(); continue; }
            element.value = next;
            edit = {raw: next, visible: String(element.value || "")};
            emailEdits.set(element, edit);
            if (typeof window.__goosieSetEmailEditValue === "function") window.__goosieSetEmailEditValue(element, next);
            if (typeof window.__goosieNotifyUserInput === "function") window.__goosieNotifyUserInput(element);
            element.dispatchEvent(new InputEvent("input", {bubbles: true, inputType, data: inputType.startsWith("delete") ? null : key}));
          }
          finishKey();
          continue;
        }

        const targetWindow = element.ownerDocument && element.ownerDocument.defaultView || window;
        const keyDefault = targetWindow.__goosieTestDriverKeyDefault;
        const isInsertKey = key.length === 1 && key.charCodeAt(0) >= 0x20 && key.charCodeAt(0) < 0xE000;
        if ((keyName === "Backspace" || keyName === "Delete" || keyName === "Enter" || isInsertKey) && typeof keyDefault === "function") {
          keyDefault(keyName, false, false, false, false);
          finishKey();
          continue;
        }

        let start = element.selectionStart;
        let end = element.selectionEnd;
        let replacement = null;
        let inputType = "insertText";
        if (keyName === "Backspace") {
          inputType = "deleteContentBackward";
          if (start === end && start > 0) start--;
          replacement = "";
        } else if (keyName === "Delete") {
          inputType = "deleteContentForward";
          if (start === end && end < String(element.value || "").length) end++;
          replacement = "";
        } else if (keyName === "Enter" && element.localName === "textarea") {
          replacement = "\n";
        } else if (key.length === 1 && key.charCodeAt(0) < 0xE000) {
          replacement = key;
        }

        let changed = false;
        const willChange = replacement !== null && (start !== end || replacement !== "" || keyName === "Backspace" || keyName === "Delete");
        if (willChange) {
          const beforeinput = new InputEvent("beforeinput", {bubbles: true, cancelable: true, inputType, data: inputType.startsWith("delete") ? null : replacement});
          element.dispatchEvent(beforeinput);
          if (beforeinput.defaultPrevented) { finishKey(); continue; }
        }
        if (replacement !== null && typeof element.setRangeText === "function" && start !== null && end !== null && (start !== end || replacement !== "")) {
          element.setRangeText(replacement, start, end, "end");
          changed = true;
        } else if (replacement !== null && start === null && end === null) {
          const current = String(element.value || "");
          let next = current;
          if (keyName === "Backspace") next = current.slice(0, -1);
          else if (keyName === "Delete") next = current;
          else next = current + replacement;
          if (next !== current) {
            element.value = next;
            changed = true;
          }
        }
        if (changed) {
          if (typeof window.__goosieNotifyUserInput === "function") window.__goosieNotifyUserInput(element);
          element.dispatchEvent(new InputEvent("input", {bubbles: true, inputType, data: inputType.startsWith("delete") ? null : replacement}));
        }
      }
      if (!keyCanceled) {
        const targetWindow = element.ownerDocument && element.ownerDocument.defaultView || window;
        const keyDefault = targetWindow.__goosieTestDriverKeyDefault;
        const isInsertKey = key.length === 1 && key.charCodeAt(0) >= 0x20 && key.charCodeAt(0) < 0xE000;
        if ((keyName === "Backspace" || keyName === "Delete" || keyName === "Enter" || isInsertKey) && typeof keyDefault === "function") {
          keyDefault(keyName, false, false, false, false);
          finishKey();
          continue;
        }
      }
      finishKey();
    }
    return Promise.resolve();
  };
})();`
