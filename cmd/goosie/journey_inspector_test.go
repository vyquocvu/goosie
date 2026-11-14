package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/frame"
	"github.com/vyquocvu/goosie/internal/paint"
	"github.com/vyquocvu/goosie/internal/surface"
)

const inspectorSubjectHTML = `<!doctype html><html><head><title>Subject</title>` +
	`<style>html,body{margin:0}#target{position:absolute;left:10px;top:20px;width:100px;height:40px}</style>` +
	`</head><body><p id="caption">hello</p><div id="target"><span>inner</span></div></body></html>`

func inspectDOMFingerprint(s *engine.Session) string {
	var b strings.Builder
	s.Doc.Node.Walk(func(n *dom.Node) bool {
		fmt.Fprintf(&b, "%d|%d|%s|%s", n.ID, n.Type, n.Data, n.DataContent)
		for _, a := range n.Attr {
			fmt.Fprintf(&b, "|%s=%s", a.Name, a.Value)
		}
		b.WriteByte('\n')
		return true
	})
	return b.String()
}

func sessionHasInspectOverlay(t *testing.T, s *engine.Session) bool {
	t.Helper()
	dl, err := s.PaintChecked(1.0)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	for _, cmd := range dl.Build(1).All() {
		if cmd.Kind == paint.CmdFill && cmd.Color == paint.InspectHighlightColor {
			return true
		}
	}
	return false
}

func layerHasInspectOverlay(lyr *frame.Layer) bool {
	if lyr == nil {
		return false
	}
	dl, ok := lyr.Content.(*paint.LayerDL)
	if !ok {
		return false
	}
	for _, cmd := range dl.All() {
		if cmd.Kind == paint.CmdFill && cmd.Color == paint.InspectHighlightColor {
			return true
		}
	}
	return false
}

// qc:BR-142
func TestInspectorSelectDefersRepaintAndPaintsOverlay(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}

	tab := f.tabMgr.Active()
	tab.SetURL("https://example.com/subject")
	tab.SetTitle("Subject")
	sess, err := engine.NewSession(inspectorSubjectHTML, nil, 800)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	tab.SetSession(sess)
	target := sess.Doc.ElementByID("target")
	if target == nil {
		t.Fatal("element #target not found")
	}

	beforeFP := inspectDOMFingerprint(sess)
	beforeCount, beforeDepth := sess.DOMStats()

	_, htmlContent := f.buildInternalPageHTML(fmt.Sprintf("goosie://inspect?tab=%d&select=%d", tab.ID(), target.ID))
	if !strings.Contains(htmlContent, fmt.Sprintf("goosie://inspect?tab=%d&select=%d", tab.ID(), target.ID)) {
		t.Error("the DOM tree did not render a select link for the target node")
	}
	if got := sess.InspectHighlight(); got != target {
		t.Fatalf("InspectHighlight() = %v, want the target node %v", got, target)
	}
	if layerHasInspectOverlay(tab.Layer()) {
		t.Error("the page builder painted the overlay synchronously; it must defer to the chrome thread")
	}
	f.inspectRepaintMu.Lock()
	queued := len(f.inspectRepaintPending)
	f.inspectRepaintMu.Unlock()
	if queued != 1 {
		t.Fatalf("deferred repaint queue holds %d tabs, want 1", queued)
	}

	f.drainInspectRepaints()
	f.inspectRepaintMu.Lock()
	queued = len(f.inspectRepaintPending)
	f.inspectRepaintMu.Unlock()
	if queued != 0 {
		t.Fatalf("after drain the queue holds %d tabs, want 0", queued)
	}
	if !layerHasInspectOverlay(tab.Layer()) {
		t.Error("after the deferred repaint the tab's layer carries no highlight overlay")
	}

	afterFP := inspectDOMFingerprint(sess)
	afterCount, afterDepth := sess.DOMStats()
	if beforeFP != afterFP {
		t.Errorf("the DOM changed under inspection:\nbefore:\n%s\nafter:\n%s", beforeFP, afterFP)
	}
	if beforeCount != afterCount || beforeDepth != afterDepth {
		t.Errorf("DOMStats changed: (%d,%d) -> (%d,%d)", beforeCount, beforeDepth, afterCount, afterDepth)
	}

	f.buildInternalPageHTML(fmt.Sprintf("goosie://inspect?tab=%d&select=clear", tab.ID()))
	if got := sess.InspectHighlight(); got != nil {
		t.Fatalf("after clear InspectHighlight() = %v, want nil", got)
	}
	f.drainInspectRepaints()
	if layerHasInspectOverlay(tab.Layer()) {
		t.Error("after clear the tab's layer still carries a highlight overlay")
	}
}

// qc:BR-142
func TestInspectorSelectIgnoresUnknownNode(t *testing.T) {
	f := newNavTestPath(t)
	f.config = config{width: 800, height: 600, dpr: 1}
	tab := f.tabMgr.Active()
	sess, err := engine.NewSession(inspectorSubjectHTML, nil, 800)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	tab.SetSession(sess)

	f.buildInternalPageHTML(fmt.Sprintf("goosie://inspect?tab=%d&select=999999", tab.ID()))
	if got := sess.InspectHighlight(); got != nil {
		t.Errorf("an unknown node id set InspectHighlight() = %v, want nil", got)
	}
	f.inspectRepaintMu.Lock()
	queued := len(f.inspectRepaintPending)
	f.inspectRepaintMu.Unlock()
	if queued != 0 {
		t.Errorf("an unknown node id queued %d repaints, want 0", queued)
	}
}

// qc:BR-142
func TestJourneyInspectorHighlightsBackgroundTab(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(inspectorSubjectHTML))
	}))
	defer srv.Close()

	b := newBrowserE2E(t)
	pageURL := srv.URL + "/subject"
	b.navigateViaAddressBar(t, pageURL)
	b.waitFor(t, "subject page load", func() bool {
		tab := b.f.tabMgr.Active()
		return b.activeURL() == pageURL && tab != nil && tab.Session() != nil &&
			tab.Session().Doc.ElementByID("target") != nil
	})

	tabA := b.f.tabMgr.Active()
	sessA := tabA.Session()
	target := sessA.Doc.ElementByID("target")
	tabAID := tabA.ID()
	beforeFP := inspectDOMFingerprint(sessA)
	beforeCount, beforeDepth := sessA.DOMStats()

	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 't', Mods: surface.ModCommand})
	b.inject(t, surface.Event{Kind: surface.EvKey, Key: 't', Mods: surface.ModCommand, KeyAction: surface.KeyRelease})
	b.waitFor(t, "inspector tab opened", func() bool {
		return b.f.tabMgr.Count() == 2 && b.f.tabMgr.Active().ID() != tabAID
	})

	b.navigateViaAddressBar(t, fmt.Sprintf("goosie://inspect?tab=%d&select=%d", tabAID, target.ID))
	b.waitFor(t, "inspector committed", func() bool {
		return strings.HasPrefix(b.activeURL(), "goosie://inspect")
	})
	b.waitFor(t, "background tab highlighted", func() bool {
		return sessA.InspectHighlight() == target && sessionHasInspectOverlay(t, sessA)
	})

	afterFP := inspectDOMFingerprint(sessA)
	afterCount, afterDepth := sessA.DOMStats()
	if beforeFP != afterFP {
		t.Errorf("the inspected DOM changed:\nbefore:\n%s\nafter:\n%s", beforeFP, afterFP)
	}
	if beforeCount != afterCount || beforeDepth != afterDepth {
		t.Errorf("DOMStats changed: (%d,%d) -> (%d,%d)", beforeCount, beforeDepth, afterCount, afterDepth)
	}

	b.navigateViaAddressBar(t, fmt.Sprintf("goosie://inspect?tab=%d&select=clear", tabAID))
	b.waitFor(t, "highlight cleared", func() bool {
		return sessA.InspectHighlight() == nil && !sessionHasInspectOverlay(t, sessA)
	})
}
