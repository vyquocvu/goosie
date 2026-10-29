package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

// ---------------------------------------------------------------------------
// NodeFilter constants
// ---------------------------------------------------------------------------

func TestNodeFilterConstantsExist(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	tests := []struct {
		name string
		want string
	}{
		{"NodeFilter.SHOW_ALL", "4294967295"},
		{"NodeFilter.SHOW_ELEMENT", "1"},
		{"NodeFilter.SHOW_TEXT", "4"},
		{"NodeFilter.SHOW_COMMENT", "128"},
		{"NodeFilter.SHOW_DOCUMENT", "256"},
		{"NodeFilter.SHOW_DOCUMENT_FRAGMENT", "1024"},
		{"NodeFilter.FILTER_ACCEPT", "1"},
		{"NodeFilter.FILTER_REJECT", "2"},
		{"NodeFilter.FILTER_SKIP", "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Evaluate(tt.name)
			if err != nil {
				t.Fatalf("Evaluate(%q): %v", tt.name, err)
			}
			if got != tt.want {
				t.Errorf("%s = %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: creation
// ---------------------------------------------------------------------------

func TestCreateTreeWalker(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var walker = document.createTreeWalker(document.body, NodeFilter.SHOW_ALL);
	`, "test"); err != nil {
		t.Fatal(err)
	}

	// root should be body.
	if err := r.Run(`
		var rootIsBody = walker.root === document.body;
	`, "test"); err != nil {
		t.Fatal(err)
	}
	v, _ := r.Global("rootIsBody")
	if v.Kind != js.KindBool || !v.Bool {
		t.Error("walker.root should be document.body")
	}

	// currentNode should start at root.
	if err := r.Run(`
		var currentIsRoot = walker.currentNode === walker.root;
	`, "test"); err != nil {
		t.Fatal(err)
	}
	v, _ = r.Global("currentIsRoot")
	if v.Kind != js.KindBool || !v.Bool {
		t.Error("walker.currentNode should initially be walker.root")
	}

	// whatToShow should be SHOW_ALL.
	if err := r.Run(`
		var whatToShowVal = walker.whatToShow;
	`, "test"); err != nil {
		t.Fatal(err)
	}
	v, _ = r.Global("whatToShowVal")
	if v.Kind != js.KindNumber || v.Num != 0xFFFFFFFF {
		t.Errorf("walker.whatToShow = %v, want %v", v.Num, 0xFFFFFFFF)
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: nextNode in document order
// ---------------------------------------------------------------------------

func TestTreeWalkerNextNodeAll(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	// Collect all nodes under body with SHOW_ALL.
	if err := r.Run(`
		var walker = document.createTreeWalker(document.body, NodeFilter.SHOW_ALL);
		var twResults = [];
		var twN;
		while ((twN = walker.nextNode()) !== null) {
			if (twN.nodeType === 1) {
				twResults.push(twN.tagName.toLowerCase());
			} else if (twN.nodeType === 3) {
				twResults.push('#text');
			}
		}
		var twStr = twResults.join(',');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("twStr")
	// Document order under body: p, #text(Hello), p, #text(World), div, #text(!)
	want := "p,#text,p,#text,div,#text"
	if v.Str != want {
		t.Errorf("TreeWalker nextNode = %q, want %q", v.Str, want)
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: nextNode with SHOW_ELEMENT
// ---------------------------------------------------------------------------

func TestTreeWalkerShowElement(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var ew = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
		var eResults = [];
		var eN;
		while ((eN = ew.nextNode()) !== null) {
			eResults.push(eN.tagName.toLowerCase());
		}
		var eStr = eResults.join(',');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("eStr")
	want := "p,p,div"
	if v.Str != want {
		t.Errorf("SHOW_ELEMENT nextNode = %q, want %q", v.Str, want)
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: nextNode with SHOW_TEXT
// ---------------------------------------------------------------------------

func TestTreeWalkerShowText(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var tw2 = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
		var tResults = [];
		var tN;
		while ((tN = tw2.nextNode()) !== null) {
			tResults.push(tN.textContent);
		}
		var tStr = tResults.join('|');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("tStr")
	want := "Hello|World|!"
	if v.Str != want {
		t.Errorf("SHOW_TEXT nextNode = %q, want %q", v.Str, want)
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: custom filter function
// ---------------------------------------------------------------------------

func TestTreeWalkerWithFilter(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	// Filter that accepts only <p> elements.
	if err := r.Run(`
		var fw = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT, function(node) {
			return node.tagName.toLowerCase() === 'p' ? 1 : 3;
		});
		var fResults = [];
		var fN;
		while ((fN = fw.nextNode()) !== null) {
			fResults.push(fN.tagName.toLowerCase());
		}
		var fStr = fResults.join(',');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("fStr")
	want := "p,p"
	if v.Str != want {
		t.Errorf("filtered nextNode = %q, want %q", v.Str, want)
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: directional navigation
// ---------------------------------------------------------------------------

func TestTreeWalkerFirstChild(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var w1 = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
		var fc1 = w1.firstChild();
		var fc1Tag = fc1 ? fc1.tagName.toLowerCase() : 'null';
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("fc1Tag")
	if v.Str != "p" {
		t.Errorf("firstChild() = %q, want %q", v.Str, "p")
	}

	// From p, firstChild should be null (p has only text children, filtered out).
	if err := r.Run(`
		var fc2 = w1.firstChild();
		var fc2IsNull = fc2 === null;
	`, "test"); err != nil {
		t.Fatal(err)
	}
	v, _ = r.Global("fc2IsNull")
	if v.Kind != js.KindBool || !v.Bool {
		t.Error("firstChild() from <p> with SHOW_ELEMENT should be null")
	}
}

func TestTreeWalkerNextSibling(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var w2 = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
		w2.firstChild(); // move to <p> (first child)
		var ns1 = w2.nextSibling();
		var ns1Tag = ns1 ? ns1.tagName.toLowerCase() : 'null';
		var ns2 = w2.nextSibling();
		var ns2Tag = ns2 ? ns2.tagName.toLowerCase() : 'null';
		var ns3 = w2.nextSibling();
		var ns3IsNull = ns3 === null;
	`, "test"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		got  string
		want string
	}{
		{"first nextSibling", mustGlobalStr(r, "ns1Tag"), "p"},
		{"second nextSibling", mustGlobalStr(r, "ns2Tag"), "div"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	v, _ := r.Global("ns3IsNull")
	if v.Kind != js.KindBool || !v.Bool {
		t.Error("third nextSibling should be null (no more siblings)")
	}
}

func TestTreeWalkerParentNode(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var w3 = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
		w3.firstChild(); // move to <p>
		var par = w3.parentNode();
		var parIsBody = par === document.body;
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("parIsBody")
	if v.Kind != js.KindBool || v.Bool {
		t.Error("parentNode() from first child should not return the root")
	}
}

func TestTreeWalkerLastChild(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var w4 = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
		var lc = w4.lastChild();
		var lcTag = lc ? lc.tagName.toLowerCase() : 'null';
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("lcTag")
	if v.Str != "div" {
		t.Errorf("lastChild() = %q, want %q", v.Str, "div")
	}
}

func TestTreeWalkerPreviousSibling(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var w5 = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
		// Navigate to the last child (div) first.
		w5.lastChild();
		var ps = w5.previousSibling();
		var psTag = ps ? ps.tagName.toLowerCase() : 'null';
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("psTag")
	if v.Str != "p" {
		t.Errorf("previousSibling() from div = %q, want %q", v.Str, "p")
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: previousNode
// ---------------------------------------------------------------------------

func TestTreeWalkerPreviousNode(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	// Walk forward to the end, then walk backward.
	if err := r.Run(`
		var wp = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
		var fwd = [];
		var pn;
		while ((pn = wp.nextNode()) !== null) {
			fwd.push(pn.tagName.toLowerCase());
		}
		// currentNode is now null (past the end). Reset to root.
		wp.currentNode = wp.root;
		// Walk to the last node.
		var lastNode = null;
		while ((pn = wp.nextNode()) !== null) {
			lastNode = pn;
		}
		// wp.currentNode is null now; set it to last element to walk backwards.
		wp.currentNode = lastNode;
		var bwd = [];
		var bn;
		while ((bn = wp.previousNode()) !== null) {
			bwd.push(bn.tagName.toLowerCase());
		}
		var fwdStr = fwd.join(',');
		var bwdStr = bwd.join(',');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	fwdStr := mustGlobalStr(r, "fwdStr")
	bwdStr := mustGlobalStr(r, "bwdStr")

	if fwdStr != "p,p,div" {
		t.Errorf("forward = %q, want %q", fwdStr, "p,p,div")
	}
	// Backward should be the reverse of forward (body is root, excluded).
	if bwdStr != "p,p" {
		t.Errorf("backward = %q, want %q", bwdStr, "p,p")
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: currentNode setter
// ---------------------------------------------------------------------------

func TestTreeWalkerCurrentNodeSetter(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var ws = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT);
		var pEl = document.body.querySelector('p');
		ws.currentNode = pEl;
		var currentIsP = ws.currentNode === pEl;
		// nextNode from p should find the second p.
		var nx = ws.nextNode();
		var nxIsSecondP = nx !== null && nx !== pEl && nx.tagName.toLowerCase() === 'p';
	`, "test"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		key  string
		want bool
	}{
		{"currentNode set", "currentIsP", true},
		{"nextNode from set position", "nxIsSecondP", true},
	} {
		v, _ := r.Global(tc.key)
		if v.Kind != js.KindBool || v.Bool != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, v.Bool, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// NodeIterator: creation
// ---------------------------------------------------------------------------

func TestCreateNodeIterator(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var iter = document.createNodeIterator(document.body, NodeFilter.SHOW_ALL);
		var iterRootIsBody = iter.root === document.body;
		var iterCurrentIsRoot = iter.referenceNode === iter.root;
		var iterWhatToShow = iter.whatToShow;
	`, "test"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		key  string
		want interface{}
	}{
		{"root", "iterRootIsBody", true},
		{"currentNode", "iterCurrentIsRoot", true},
	} {
		v, _ := r.Global(tc.key)
		if v.Kind != js.KindBool || v.Bool != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, v.Bool, tc.want)
		}
	}
	v, _ := r.Global("iterWhatToShow")
	if v.Kind != js.KindNumber || v.Num != 0xFFFFFFFF {
		t.Errorf("whatToShow = %v, want %v", v.Num, 0xFFFFFFFF)
	}
}

// ---------------------------------------------------------------------------
// NodeIterator: nextNode in document order
// ---------------------------------------------------------------------------

func TestNodeIteratorNextNodeAll(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var it = document.createNodeIterator(document.body, NodeFilter.SHOW_ALL);
		var itResults = [];
		var itN;
		while ((itN = it.nextNode()) !== null) {
			if (itN.nodeType === 1) {
				itResults.push(itN.tagName.toLowerCase());
			} else if (itN.nodeType === 3) {
				itResults.push('#text');
			}
		}
		var itStr = itResults.join(',');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("itStr")
	want := "body,p,#text,p,#text,div,#text"
	if v.Str != want {
		t.Errorf("NodeIterator nextNode = %q, want %q", v.Str, want)
	}
}

func TestNodeIteratorShowElement(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var ie = document.createNodeIterator(document.body, NodeFilter.SHOW_ELEMENT);
		var ieResults = [];
		var ieN;
		while ((ieN = ie.nextNode()) !== null) {
			ieResults.push(ieN.tagName.toLowerCase());
		}
		var ieStr = ieResults.join(',');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("ieStr")
	want := "body,p,p,div"
	if v.Str != want {
		t.Errorf("NodeIterator SHOW_ELEMENT = %q, want %q", v.Str, want)
	}
}

func TestNodeIteratorShowText(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var it2 = document.createNodeIterator(document.body, NodeFilter.SHOW_TEXT);
		var it2Results = [];
		var it2N;
		while ((it2N = it2.nextNode()) !== null) {
			it2Results.push(it2N.textContent);
		}
		var it2Str = it2Results.join('|');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("it2Str")
	want := "Hello|World|!"
	if v.Str != want {
		t.Errorf("NodeIterator SHOW_TEXT = %q, want %q", v.Str, want)
	}
}

// ---------------------------------------------------------------------------
// NodeIterator: filter function
// ---------------------------------------------------------------------------

func TestNodeIteratorWithFilter(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var fi = document.createNodeIterator(document.body, NodeFilter.SHOW_ELEMENT, function(node) {
			return node.tagName.toLowerCase() === 'p' ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_SKIP;
		});
		var fiResults = [];
		var fiN;
		while ((fiN = fi.nextNode()) !== null) {
			fiResults.push(fiN.tagName.toLowerCase());
		}
		var fiStr = fiResults.join(',');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("fiStr")
	want := "p,p"
	if v.Str != want {
		t.Errorf("filtered NodeIterator = %q, want %q", v.Str, want)
	}
}

// ---------------------------------------------------------------------------
// NodeIterator: previousNode
// ---------------------------------------------------------------------------

func TestNodeIteratorPreviousNode(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	// Build a flat element tree for predictable backward iteration.
	doc := d
	body := doc.Body
	// Clear existing children.
	for body.FirstChild != nil {
		body.RemoveChild(body.FirstChild)
	}
	d1 := doc.NewElement("div")
	d1.SetAttribute("id", "d1")
	d2 := doc.NewElement("div")
	d2.SetAttribute("id", "d2")
	d3 := doc.NewElement("div")
	d3.SetAttribute("id", "d3")
	body.AppendChild(d1)
	body.AppendChild(d2)
	body.AppendChild(d3)

	if err := r.Run(`
		var ip = document.createNodeIterator(document.body, NodeFilter.SHOW_ELEMENT);
		// Walk forward past all nodes, then traverse backwards from d3.
		while (ip.nextNode() !== null) {}
		var prev1 = ip.previousNode();
		var prev2 = ip.previousNode();
		var prev3 = ip.previousNode();
		var p1Id = prev1 ? prev1.id : 'null';
		var p2Id = prev2 ? prev2.id : 'null';
		var p3Id = prev3 ? prev3.id : 'null';
	`, "test"); err != nil {
		t.Fatal(err)
	}

	if v, _ := r.Global("p1Id"); v.Str != "d3" {
		t.Errorf("previousNode() #1 = %q, want %q", v.Str, "d3")
	}
	if v, _ := r.Global("p2Id"); v.Str != "d2" {
		t.Errorf("previousNode() #2 = %q, want %q", v.Str, "d2")
	}
	if v, _ := r.Global("p3Id"); v.Str != "d1" {
		t.Errorf("previousNode() #3 = %q, want %q", v.Str, "d1")
	}
}

// ---------------------------------------------------------------------------
// NodeIterator: currentNode setter
// ---------------------------------------------------------------------------

func TestNodeIteratorCurrentNodeSetter(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var is = document.createNodeIterator(document.body, NodeFilter.SHOW_ELEMENT);
		var isInitialReference = is.referenceNode === document.body;
		var isInitialBefore = is.pointerBeforeReferenceNode === true;
		var isFirstIsRoot = is.nextNode() === document.body;
	`, "test"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		key  string
		want bool
	}{
		{"referenceNode starts at root", "isInitialReference", true},
		{"pointer starts before root", "isInitialBefore", true},
		{"first nextNode returns root", "isFirstIsRoot", true},
	} {
		v, _ := r.Global(tc.key)
		if v.Kind != js.KindBool || v.Bool != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, v.Bool, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// NodeIterator: detach (deprecated no-op)
// ---------------------------------------------------------------------------

func TestNodeIteratorDetach(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var id = document.createNodeIterator(document.body, NodeFilter.SHOW_ALL);
		id.detach();
		var detachOk = true;
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("detachOk")
	if v.Kind != js.KindBool || !v.Bool {
		t.Error("detach() should not throw")
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: root boundary
// ---------------------------------------------------------------------------

func TestTreeWalkerDoesNotEscapeRoot(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	// Create a walker rooted at the first <p>. It should not see body or html.
	if err := r.Run(`
		var pEl = document.body.querySelector('p');
		var wr = document.createTreeWalker(pEl, NodeFilter.SHOW_ELEMENT);
		var wrResults = [];
		var wrN;
		while ((wrN = wr.nextNode()) !== null) {
			wrResults.push(wrN.tagName.toLowerCase());
		}
		// Should be empty: <p> has no element children.
		var wrEmpty = wrResults.length === 0;
		// parentNode from root should be null.
		var wrParent = wr.parentNode();
		var wrParentNull = wrParent === null;
	`, "test"); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		key  string
		want bool
	}{
		{"no element children of <p>", "wrEmpty", true},
		{"parentNode from root is null", "wrParentNull", true},
	} {
		v, _ := r.Global(tc.key)
		if v.Kind != js.KindBool || v.Bool != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, v.Bool, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: filter with boolean return
// ---------------------------------------------------------------------------

func TestTreeWalkerFilterUsesNodeFilterConstants(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	// Filter results use FILTER_ACCEPT/FILTER_SKIP, not boolean truthiness.
	if err := r.Run(`
		var bf = document.createTreeWalker(document.body, NodeFilter.SHOW_ELEMENT, function(node) {
			return node.tagName.toLowerCase() !== 'div' ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_SKIP;
		});
		var bfResults = [];
		var bfN;
		while ((bfN = bf.nextNode()) !== null) {
			bfResults.push(bfN.tagName.toLowerCase());
		}
		var bfStr = bfResults.join(',');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("bfStr")
	want := "p,p"
	if v.Str != want {
		t.Errorf("filtered nodes = %q, want %q", v.Str, want)
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: default whatToShow
// ---------------------------------------------------------------------------

func TestTreeWalkerDefaultWhatToShow(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	// Omit whatToShow: should default to SHOW_ALL.
	if err := r.Run(`
		var dw = document.createTreeWalker(document.body);
		var dwWhat = dw.whatToShow;
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("dwWhat")
	if v.Kind != js.KindNumber || v.Num != 0xFFFFFFFF {
		t.Errorf("default whatToShow = %v, want %v", v.Num, 0xFFFFFFFF)
	}
}

// ---------------------------------------------------------------------------
// TreeWalker: comment nodes with SHOW_COMMENT
// ---------------------------------------------------------------------------

func TestTreeWalkerShowComment(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	doc.Body = body
	doc.HTML = html

	comment := doc.NewComment("a comment")
	body.AppendChild(comment)
	div := doc.NewElement("div")
	body.AppendChild(div)

	r := newDOMRuntime(t, doc)

	if err := r.Run(`
		var cw = document.createTreeWalker(document.body, NodeFilter.SHOW_COMMENT);
		var cResults = [];
		var cN;
		while ((cN = cw.nextNode()) !== null) {
			cResults.push(cN.nodeType);
		}
		var cLen = cResults.length;
		var cIsComment = cLen === 1 && cResults[0] === 8;
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("cIsComment")
	if v.Kind != js.KindBool || !v.Bool {
		t.Error("SHOW_COMMENT should find exactly one comment node (nodeType 8)")
	}
}

// ---------------------------------------------------------------------------
// NodeIterator: traverses entire subtree from root
// ---------------------------------------------------------------------------

func TestNodeIteratorFullSubtree(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	// Iterate from document (not body) with SHOW_ELEMENT.
	if err := r.Run(`
		var di = document.createNodeIterator(document, NodeFilter.SHOW_ELEMENT);
		var diResults = [];
		var diN;
		while ((diN = di.nextNode()) !== null) {
			diResults.push(diN.tagName.toLowerCase());
		}
		var diStr = diResults.join(',');
	`, "test"); err != nil {
		t.Fatal(err)
	}

	v, _ := r.Global("diStr")
	// Document order: html, head, title, body, p, p, div
	want := "html,head,title,body,p,p,div"
	if v.Str != want {
		t.Errorf("full subtree = %q, want %q", v.Str, want)
	}
}

func TestNodeIteratorRetargetsForTextContentAndInnerHTML(t *testing.T) {
	d := testDOM(t)
	r := newDOMRuntime(t, d)

	if err := r.Run(`
		var textRoot = document.createElement('div');
		textRoot.innerHTML = '<a><i></i></a>';
		document.body.appendChild(textRoot);
		var textIterator = document.createNodeIterator(textRoot, NodeFilter.SHOW_ALL);
		textIterator.nextNode();
		textIterator.nextNode();
		textIterator.nextNode();
		textRoot.textContent = 'replacement text';
		var textReferenceRetargeted = textIterator.referenceNode === textRoot && !textIterator.pointerBeforeReferenceNode;
		var textNext = textIterator.nextNode();
		var textReplacementFound = textNext !== null && textNext.nodeType === Node.TEXT_NODE && textNext.data === 'replacement text';

		var htmlRoot = document.createElement('div');
		htmlRoot.innerHTML = '<a><i></i></a>';
		document.body.appendChild(htmlRoot);
		var htmlIterator = document.createNodeIterator(htmlRoot, NodeFilter.SHOW_ALL);
		htmlIterator.nextNode();
		htmlIterator.nextNode();
		htmlIterator.nextNode();
		htmlRoot.innerHTML = '<b>replacement element</b>';
		var htmlReferenceRetargeted = htmlIterator.referenceNode === htmlRoot && !htmlIterator.pointerBeforeReferenceNode;
		var htmlNext = htmlIterator.nextNode();
		var htmlReplacementFound = htmlNext !== null && htmlNext.nodeType === Node.ELEMENT_NODE && htmlNext.tagName.toLowerCase() === 'b';
	`, "test"); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"textReferenceRetargeted", "textReplacementFound", "htmlReferenceRetargeted", "htmlReplacementFound"} {
		value, _ := r.Global(name)
		if value.Kind != js.KindBool || !value.Bool {
			t.Errorf("%s = %v, want true", name, value.Bool)
		}
	}
}

func TestTreeWalkerEveryNavigationMethodRejectsRecursiveFilter(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		var recursiveWalkerDepth = 0;
		var recursiveWalker = document.createTreeWalker(document, NodeFilter.SHOW_ALL, function() {
			if (recursiveWalkerDepth === 0) {
				recursiveWalkerDepth++;
				recursiveWalker.firstChild();
			}
			return NodeFilter.FILTER_ACCEPT;
		});
		recursiveWalker.currentNode = document.body;
		var recursiveMethods = ['parentNode', 'firstChild', 'lastChild', 'previousSibling', 'nextSibling', 'previousNode', 'nextNode'];
		var recursiveTargets = [document.body, document.body, document.body, document.body.childNodes[1], document.body.childNodes[0], document.body.childNodes[1], document.body.childNodes[0]];
		var recursiveResults = {};
		for (var recursiveIndex = 0; recursiveIndex < recursiveMethods.length; recursiveIndex++) {
			var recursiveMethod = recursiveMethods[recursiveIndex];
			recursiveWalker.currentNode = recursiveTargets[recursiveIndex];
			try {
				recursiveWalker[recursiveMethod]();
				recursiveResults[recursiveMethod] = 'no throw';
			} catch (error) {
				recursiveResults[recursiveMethod] = error.name;
			}
			recursiveWalkerDepth--;
		}
	`, "treewalker-recursive-methods"); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"parentNode", "firstChild", "lastChild", "previousSibling", "nextSibling", "previousNode", "nextNode"} {
		got, ok := r.Global("recursiveResults")
		if !ok || got.Kind != js.KindObject {
			t.Fatalf("recursiveResults missing or not object: %+v", got)
		}
		// Read each result through a small JS global because Runtime.Global only
		// exports top-level primitive values.
		if err := r.Run(`globalThis.recursiveResult = recursiveResults["`+method+`"]`, "treewalker-recursive-method-result"); err != nil {
			t.Fatal(err)
		}
		value, _ := r.Global("recursiveResult")
		if value.Str != "InvalidStateError" {
			t.Errorf("recursive %s returned %q, want InvalidStateError", method, value.Str)
		}
	}
}

func TestTreeWalkerFalseFilterResultDoesNotDescend(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		var twFalseRoot = document.createElement('section');
		var twFalseParent = document.createElement('div');
		var twFalseText = document.createTextNode('leaf');
		twFalseParent.appendChild(twFalseText);
		twFalseRoot.appendChild(twFalseParent);
		var twFalseWalker = document.createTreeWalker(twFalseRoot, NodeFilter.SHOW_ALL, function(node) {
			if (node === twFalseParent) return false;
			if (node === twFalseText) return NodeFilter.FILTER_ACCEPT;
			return NodeFilter.FILTER_REJECT;
		});
		var twFalseResult = twFalseWalker.firstChild();
	`, "treewalker-false-filter-result"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Evaluate(`twFalseResult === null`)
	if err != nil || got != "true" {
		t.Fatalf("firstChild with false filter result = %q, err=%v; want null", got, err)
	}
}

func TestTreeWalkerSiblingTraversalChecksAncestorFilter(t *testing.T) {
	r := newDOMRuntime(t, testDOM(t))
	if err := r.Run(`
		var twSiblingDepth = 0;
		var twSiblingWalker = document.createTreeWalker(document, NodeFilter.SHOW_ALL, function() {
			if (twSiblingDepth === 0) {
				twSiblingDepth++;
				twSiblingWalker.firstChild();
			}
			return NodeFilter.FILTER_ACCEPT;
		});
		twSiblingWalker.currentNode = document.body;
		var twSiblingError = '';
		try {
			twSiblingWalker.nextSibling();
		} catch (error) {
			twSiblingError = error.name;
		}
	`, "treewalker-sibling-ancestor-filter"); err != nil {
		t.Fatal(err)
	}
	got, err := r.Evaluate(`twSiblingError`)
	if err != nil || got != "InvalidStateError" {
		t.Fatalf("nextSibling ancestor filter error = %q, err=%v; want InvalidStateError", got, err)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustGlobalStr(r *js.Runtime, name string) string {
	v, ok := r.Global(name)
	if !ok {
		return ""
	}
	return v.Str
}

// Ensure testDOM is accessible (it's defined in dom_test.go in the same package).
var _ = strings.Contains
