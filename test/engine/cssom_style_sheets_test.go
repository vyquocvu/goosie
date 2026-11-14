package engine_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

func colorOfClass(t *testing.T, sess *engine.Session, class string) (int, int, int) {
	t.Helper()
	var found []*dom.Node
	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n.Element() {
			for _, tok := range strings.Fields(n.GetAttribute("class")) {
				if tok == class {
					found = append(found, n)
					break
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(&sess.Doc.Node)
	if len(found) != 1 {
		t.Fatalf("found %d .%s elements, want 1", len(found), class)
	}
	st, ok := sess.Styles[found[0].ID]
	if !ok {
		t.Fatalf(".%s has no resolved style", class)
	}
	return int(st.Color.R), int(st.Color.G), int(st.Color.B)
}

type cssomServer struct {
	mu       sync.Mutex
	requests map[string]int
	text     map[string]string
}

func newCSSOMServer() *cssomServer {
	return &cssomServer{
		requests: map[string]int{},
		text: map[string]string{
			"https://example.test/a.css":     "p.a { color: #100000; }",
			"https://example.test/print.css": "p.prt { color: #010101; }",
			"https://example.test/dup.css":   "p.dup { color: #030303; }",
			"https://example.test/dis.css":   "p.dis { color: #060606; }",
			"https://example.test/late.css":  "p.late { color: #050505; }",
		},
	}
}

func (s *cssomServer) link(base, href string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[href]++
	text, ok := s.text[href]
	if !ok {
		return "", fmt.Errorf("no sheet at %s", href)
	}
	return text, nil
}

func (s *cssomServer) count(href string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests[href]
}

func cssomJS(t *testing.T, sess *engine.Session, expr, want string) string {
	t.Helper()
	got, err := sess.EvaluateJS(expr)
	if err != nil {
		t.Fatalf("EvaluateJS(%s): %v", expr, err)
	}
	got = strings.TrimSpace(got)
	if want != "" && got != want {
		t.Fatalf("CSSOM probe = %q, want %q", got, want)
	}
	return got
}

// qc:BR-284
func TestCSSOMListsLinkedSheetsWithFetchedRules(t *testing.T) {
	srv := newCSSOMServer()
	rt := newCSSRuntime(t)
	html := `<html><head>
<link id="L1" rel="stylesheet" href="/a.css">
<link id="L2" rel="stylesheet" href="/print.css" media="print">
<link id="L3" rel="stylesheet" href="/missing.css">
<style id="S1">p.style1 { color: #070707; }</style>
</head><body><p class="a">x</p></body></html>`
	sess := newCSSSession(t, rt, html, srv.link)

	cssomJS(t, sess, `
		var out = [];
		for (var i = 0; i < document.styleSheets.length; i++) {
			var s = document.styleSheets[i];
			out.push(s.ownerNode.nodeName + '#' + s.ownerNode.id + '@' + s.href +
			         '[' + s.cssRules.length + ':' + (s.cssRules[0] ? s.cssRules[0].selectorText : '-') + ']');
		}
		out.join(',')
	`, "LINK#L1@https://example.test/a.css[1:p.a],"+
		"LINK#L2@https://example.test/print.css[1:p.prt],"+
		"LINK#L3@https://example.test/missing.css[0:-],"+
		"STYLE#S1@null[1:p.style1]")

	cssomJS(t, sess, `
		var link = document.getElementById('L3');
		[String(link.sheet), link.sheet === document.styleSheets[2],
		 document.getElementById('L1').sheet === document.styleSheets[0],
		 document.getElementById('S1').sheet === document.styleSheets[3],
		 document.getElementById('L2').media].join(',')
	`, "[object CSSStyleSheet],true,true,true,print")
}

// qc:BR-284
func TestDisabledLinkedSheetIsNeitherListedNorApplied(t *testing.T) {
	srv := newCSSOMServer()
	rt := newCSSRuntime(t)
	html := `<html><head>
<link id="L1" rel="stylesheet" href="/a.css">
<link id="L2" rel="stylesheet" href="/dis.css" disabled>
<style id="S1" disabled>p.styledis { color: #0e0e0e; }</style>
</head><body><p class="a">a</p><p class="dis">d</p><p class="styledis">s</p></body></html>`
	sess := newCSSSession(t, rt, html, srv.link)

	cssomJS(t, sess, `
		var owners = [];
		for (var i = 0; i < document.styleSheets.length; i++) {
			owners.push(document.styleSheets[i].ownerNode.id);
		}
		[owners.join('+'), String(document.getElementById('L2').sheet)].join('|')
	`, "L1+S1|null")

	if r, g, b := colorOfClass(t, sess, "dis"); r != 0 || g != 0 || b != 0 {
		t.Errorf("p.dis color = (%d,%d,%d), want (0,0,0) as Chromium measures it", r, g, b)
	}
	if r, g, b := colorOfClass(t, sess, "styledis"); r != 14 || g != 14 || b != 14 {
		t.Errorf("p.styledis color = (%d,%d,%d), want (14,14,14) from the still-applied style", r, g, b)
	}
	if r, g, b := colorOfClass(t, sess, "a"); r != 16 || g != 0 || b != 0 {
		t.Errorf("p.a color = (%d,%d,%d), want (16,0,0) from the enabled sheet", r, g, b)
	}
}

// qc:BR-284
func TestDuplicateLinkedHrefListsTwoSheetsFromOneRequest(t *testing.T) {
	srv := newCSSOMServer()
	rt := newCSSRuntime(t)
	html := `<html><head>
<link id="L1" rel="stylesheet" href="/dup.css">
<link id="L2" rel="stylesheet" href="/dup.css">
</head><body><p class="dup">x</p></body></html>`
	sess := newCSSSession(t, rt, html, srv.link)

	cssomJS(t, sess, `
		var a = document.styleSheets[0], b = document.styleSheets[1];
		[document.styleSheets.length, a.href === b.href, a !== b,
		 a.ownerNode.id + '+' + b.ownerNode.id,
		 a.cssRules.length + '+' + b.cssRules.length,
		 a.cssRules[0].selectorText].join(',')
	`, "2,true,true,L1+L2,1+1,p.dup")

	if got := srv.count("https://example.test/dup.css"); got != 1 {
		t.Errorf("dup.css requested %d times, want 1 for two links to one URL", got)
	}
}

// qc:BR-284
func TestMediaMismatchedDuplicateHrefKeepsTheCascadeSlot(t *testing.T) {
	srv := newCSSOMServer()
	srv.text["https://example.test/s.css"] = "p.dup { color: #0b1621; }"
	rt := newCSSRuntime(t)
	html := `<html><head>
<link id="L1" rel="stylesheet" href="/s.css" media="print">
<link id="L2" rel="stylesheet" href="/s.css">
</head><body><p class="dup">x</p></body></html>`
	sess := newCSSSession(t, rt, html, srv.link)

	if r, g, b := colorOfClass(t, sess, "dup"); r != 11 || g != 22 || b != 33 {
		t.Errorf("p.dup color = (%d,%d,%d), want (11,22,33) from the matching link", r, g, b)
	}
	if got := srv.count("https://example.test/s.css"); got != 1 {
		t.Errorf("s.css requested %d times, want 1 for two links to one URL", got)
	}
	cssomJS(t, sess, `
		var out = [];
		for (var i = 0; i < document.styleSheets.length; i++) {
			var s = document.styleSheets[i];
			out.push((s.href === null ? 'null' : s.href.replace('https://example.test', '')) +
			         '|' + s.media.mediaText + '|' + s.cssRules.length);
		}
		out.join(',')
	`, "/s.css|print|1,/s.css||1")
}

// qc:BR-284
func TestScriptInsertedLinkIsListedAtItsDocumentPosition(t *testing.T) {
	srv := newCSSOMServer()
	rt := newCSSRuntime(t)
	html := `<html><head>
<link id="L1" rel="stylesheet" href="/a.css">
<style id="S1">p.style1 { color: #070707; }</style>
<style id="S2">p.style2 { color: #080808; }</style>
<script>
var l = document.createElement('link');
l.id = 'LATE'; l.rel = 'stylesheet'; l.href = 'late.css';
document.head.appendChild(l);
</script>
</head><body><style id="S3">p.body { color: #0b0b0b; }</style><p class="a">x</p></body></html>`
	sess := newCSSSession(t, rt, html, srv.link)

	cssomJS(t, sess, `
		var owners = [];
		for (var i = 0; i < document.styleSheets.length; i++) {
			var s = document.styleSheets[i];
			owners.push(s.ownerNode.id + '@' + (s.href === null ? 'null' : s.href.replace('https://example.test', '')));
		}
		owners.join(',')
	`, "L1@/a.css,S1@null,S2@null,LATE@/late.css,S3@null")
}
