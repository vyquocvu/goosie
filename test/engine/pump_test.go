package engine_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/js"
)

func pumpSession(t *testing.T, html string, withJS bool) (*engine.Session, *js.Runtime, *engine.Invalidation) {
	t.Helper()
	var rt *js.Runtime
	var inv *engine.Invalidation
	opts := []engine.Option{engine.WithMetrics(&fixedMetrics{advance: 7})}
	if withJS {
		var err error
		rt, err = js.New(js.Options{URL: "https://example.com/page"})
		if err != nil {
			t.Fatalf("js.New: %v", err)
		}
		inv = &engine.Invalidation{}
		opts = append(opts, engine.WithJS(rt), engine.WithInvalidation(inv))
	}
	sess, err := engine.NewSession(html, nil, 800, opts...)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() { sess.Close() })
	return sess, rt, inv
}

func TestPumpJSFiresTimers(t *testing.T) {
	html := `<html><body><p id="t">waiting</p><script>setTimeout(function(){document.getElementById('t').textContent='fired';},0);</script></body></html>`
	sess, _, _ := pumpSession(t, html, true)
	if got := sess.Find("fired"); len(got) != 0 {
		t.Fatal("timer callback ran during load without any pump")
	}
	if !sess.PumpJS(800) {
		t.Fatal("PumpJS reported no work for a due timer")
	}
	if got := sess.Find("fired"); len(got) == 0 {
		t.Fatal("timer mutation not visible after PumpJS")
	}
	if sess.PumpJS(800) {
		t.Fatal("second PumpJS reported work on a quiescent document")
	}
}

// qc:BR-184
func TestPumpJSSameValueDOMWritesSkipPipeline(t *testing.T) {
	html := `<html><body>
<div id="same" class="stable">same</div>
<input id="check" type="checkbox" value="fixed">
<script>setTimeout(function() {
  var node = document.getElementById('same');
  node.setAttribute('class', 'stable');
  node.classList.remove('absent');
  var check = document.getElementById('check');
  check.checked = false;
  check.value = 'fixed';
}, 0);</script>
</body></html>`
	sess, _, _ := pumpSession(t, html, true)
	before := sess.WorkCounts()
	if sess.PumpJS(800) {
		t.Fatal("PumpJS reported visual work for same-value DOM writes")
	}
	if got := sess.WorkCounts(); got != before {
		t.Fatalf("work counts after no-op mutations = %+v, want unchanged %+v", got, before)
	}
}

// qc:BR-090
func TestPumpJSChangedAttributeStillRebuildsStylesAndLayout(t *testing.T) {
	html := `<html><head><style>.active { color: red; width: 200px; }</style></head><body>
<p id="target">text</p>
<script>setTimeout(function() { document.getElementById('target').setAttribute('class', 'active'); }, 0);</script>
</body></html>`
	sess, _, _ := pumpSession(t, html, true)
	before := sess.WorkCounts()
	if !sess.PumpJS(800) {
		t.Fatal("PumpJS missed a changed class attribute")
	}
	got := sess.WorkCounts()
	if got.StyleResolves != before.StyleResolves+1 || got.LayoutBuilds != before.LayoutBuilds+1 {
		t.Fatalf("work counts after changed attribute = %+v, want one style/layout pass after %+v", got, before)
	}
	target := sess.Doc.ElementByID("target")
	if target == nil || sess.Styles[target.ID].Color.R != 255 {
		t.Fatal("changed class did not update computed style")
	}
	var width float32
	for i := range sess.Arena.Objects {
		obj := &sess.Arena.Objects[i]
		if obj.Node == target {
			x0, _, x1, _ := obj.BorderRect()
			width = x1 - x0
		}
	}
	if width != 200 {
		t.Fatalf("laid-out width after the class change = %v, want 200", width)
	}
}

// qc:BR-089
func TestPumpJSMutationReachesDOMAccessibilityTreeAndPaint(t *testing.T) {
	html := `<html><body style="margin: 0;">
<p id="msg">before</p>
<a id="link" href="/one">docs</a>
<script>setTimeout(function() {
  document.getElementById('msg').textContent = 'after';
  document.getElementById('link').setAttribute('href', '/two');
}, 0);</script>
</body></html>`
	sess, _, _ := pumpSession(t, html, true)

	initial, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked before pump: %v", err)
	}
	if got := paintedText(initial); !strings.Contains(got, "before") {
		t.Fatalf("load-time paint text = %q, want it to draw the initial text", got)
	}
	if axFind(sess.AccessibilityTree(), ax.RoleStaticText, "before") == nil {
		t.Fatal("load-time accessibility tree lacks the initial text")
	}
	if link := axFind(sess.AccessibilityTree(), ax.RoleLink, "docs"); link == nil || link.Href != "/one" {
		t.Fatalf("load-time link node = %+v, want href /one", link)
	}

	if !sess.PumpJS(800) {
		t.Fatal("PumpJS missed the text and attribute mutations")
	}

	node := sess.Doc.ElementByID("msg")
	if node == nil || strings.TrimSpace(node.TextContent()) != "after" {
		t.Fatalf("DOM text after pump = %q, want after", node.TextContent())
	}
	if axFind(sess.AccessibilityTree(), ax.RoleStaticText, "after") == nil {
		t.Fatal("accessibility tree still shows the pre-mutation text")
	}
	if link := axFind(sess.AccessibilityTree(), ax.RoleLink, "docs"); link == nil || link.Href != "/two" {
		t.Fatalf("link node after pump = %+v, want href /two", link)
	}
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked after pump: %v", err)
	}
	if got := paintedText(list); !strings.Contains(got, "after") || strings.Contains(got, "before") {
		t.Fatalf("painted text after pump = %q, want the mutated text only", got)
	}
}

// qc:BR-113
func TestPumpJSRunsRAF(t *testing.T) {
	html := `<html><body><p id="t">waiting</p><script>
window.__frames = 0;
requestAnimationFrame(function(){
  window.__frames++;
  document.getElementById('t').textContent = 'painted';
});
</script></body></html>`
	sess, rt, _ := pumpSession(t, html, true)
	if !sess.PumpJS(800) {
		t.Fatal("PumpJS reported no work for a pending RAF callback")
	}
	if got := sess.Find("painted"); len(got) == 0 {
		t.Fatal("RAF mutation not visible after PumpJS")
	}
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked after RAF: %v", err)
	}
	if got := paintedText(list); !strings.Contains(got, "painted") {
		t.Fatalf("painted text after RAF = %q, want the callback's write", got)
	}
	if v, ok := rt.Global("__frames"); !ok || v.Num != 1 {
		t.Fatalf("frame callbacks = %+v, want exactly 1", v)
	}
	if sess.PumpJS(800) {
		t.Fatal("a one-shot RAF callback ran again on the next turn")
	}
	if v, _ := rt.Global("__frames"); v.Num != 1 {
		t.Fatalf("frame callbacks after a second turn = %v, want 1", v.Num)
	}
}

// qc:BR-114
func TestPumpJSCancelledTimersAndFramesDoNotTouchTheDocument(t *testing.T) {
	html := `<html><body style="margin: 0;"><p id="t">still</p><script>
var timer = setTimeout(function(){
  window.__timerRan = true;
  document.getElementById('t').textContent = 'timer';
}, 0);
clearTimeout(timer);
var frame = requestAnimationFrame(function(){
  window.__frameRan = true;
  document.getElementById('t').textContent = 'frame';
});
cancelAnimationFrame(frame);
</script></body></html>`
	sess, rt, _ := pumpSession(t, html, true)
	if sess.PumpJS(800) {
		t.Fatal("PumpJS reported work for a cancelled timer and frame")
	}
	for _, name := range []string{"__timerRan", "__frameRan"} {
		if v, ok := rt.Global(name); ok && v.Bool {
			t.Fatalf("cancelled callback set %s", name)
		}
	}
	if got := sess.Find("timer"); len(got) != 0 {
		t.Fatal("cancelled timer mutated the document")
	}
	if got := sess.Find("frame"); len(got) != 0 {
		t.Fatal("cancelled animation frame mutated the document")
	}
	list, err := sess.PaintChecked(1)
	if err != nil {
		t.Fatalf("PaintChecked: %v", err)
	}
	if got := paintedText(list); !strings.Contains(got, "still") {
		t.Fatalf("painted text = %q, want the unchanged load-time text", got)
	}
}

// qc:BR-112
func TestPumpJSDOMNodeReplacementDropsStaleReferences(t *testing.T) {
	html := `<html><body style="margin: 0;">
<div style="height: 40px"><a id="gone" href="/gone" style="display:block;height:40px">gone</a></div>
<div id="slot" style="height: 80px"><a id="stay" href="/stay" style="display:block;height:20px;width:200px">stay</a></div>
<script>setTimeout(function() {
  var gone = document.getElementById('gone');
  gone.parentNode.removeChild(gone);
  var added = document.createElement('a');
  added.id = 'added';
  added.setAttribute('href', '/added');
  added.setAttribute('style', 'display:block;height:20px;width:200px');
  added.textContent = 'added';
  document.getElementById('slot').appendChild(added);
}, 0);</script>
</body></html>`
	sess, _, _ := pumpSession(t, html, true)
	removed := sess.Doc.ElementByID("gone")
	if removed == nil {
		t.Fatal("fixture did not parse the node that is about to be removed")
	}
	if href := sess.HitTestLink(10, 20); href != "/gone" {
		t.Fatalf("load-time hit test = %q, want /gone", href)
	}
	if axFind(sess.AccessibilityTree(), ax.RoleLink, "gone") == nil {
		t.Fatal("load-time accessibility tree lacks the removable link")
	}

	if !sess.PumpJS(800) {
		t.Fatal("PumpJS missed the node replacement")
	}

	if sess.Doc.ElementByID("gone") != nil {
		t.Fatal("removed node is still reachable from the document")
	}
	for i := range sess.Arena.Objects {
		if obj := &sess.Arena.Objects[i]; obj.Node == removed {
			t.Fatalf("layout arena still holds a box for the removed node at index %d", i)
		}
	}
	if href := sess.HitTestLink(10, 20); href != "" {
		t.Fatalf("hit test at the removed link's old box = %q, want no link", href)
	}
	if axFind(sess.AccessibilityTree(), ax.RoleLink, "gone") != nil {
		t.Fatal("accessibility tree still exposes the removed link")
	}

	added := sess.Doc.ElementByID("added")
	if added == nil {
		t.Fatal("appended node is missing from the document")
	}
	var addedRect [4]float32
	found := false
	for i := range sess.Arena.Objects {
		obj := &sess.Arena.Objects[i]
		if obj.Node == added {
			x0, y0, x1, y1 := obj.BorderRect()
			addedRect = [4]float32{x0, y0, x1, y1}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("appended node has no layout box")
	}
	cx, cy := (addedRect[0]+addedRect[2])/2, (addedRect[1]+addedRect[3])/2
	if href := sess.HitTestLink(cx, cy); href != "/added" {
		t.Fatalf("hit test on the appended link = %q, want /added", href)
	}
	if link := axFind(sess.AccessibilityTree(), ax.RoleLink, "added"); link == nil || link.Href != "/added" {
		t.Fatalf("appended link node = %+v, want href /added", link)
	}
}

func TestPumpJSCanvasRAFRepaintsOnlyWhenDrawingChanges(t *testing.T) {
	html := `<html><body><canvas id="c" width="10" height="10"></canvas><script>
		var ctx = document.getElementById('c').getContext('2d');
		function tick(){requestAnimationFrame(tick);}
		requestAnimationFrame(function(){ctx.fillRect(0, 0, 5, 5); tick();});
	</script></body></html>`
	sess, _, _ := pumpSession(t, html, true)
	if !sess.PumpJS(800) {
		t.Fatal("PumpJS reported no work for a canvas drawing callback")
	}
	if sess.PumpJS(800) {
		t.Fatal("PumpJS reported repaint work for a subsequent no-op RAF callback")
	}
}

func TestPumpJSWithoutRuntime(t *testing.T) {
	sess, _, _ := pumpSession(t, `<html><body><p>x</p></body></html>`, false)
	if sess.PumpJS(800) {
		t.Fatal("PumpJS reported work with no runtime")
	}
}

func TestPumpJSClosedSession(t *testing.T) {
	html := `<html><body><script>setTimeout(function(){},0);</script></body></html>`
	sess, _, _ := pumpSession(t, html, true)
	sess.Close()
	if sess.PumpJS(800) {
		t.Fatal("PumpJS reported work on a closed session")
	}
}
