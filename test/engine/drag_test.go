package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
)

// ---------------------------------------------------------------------------
// DataTransfer data store (Go level)
// ---------------------------------------------------------------------------

func TestDataTransferStore(t *testing.T) {
	dt := dom.NewDataTransfer()

	if got := dt.Types(); len(got) != 0 {
		t.Errorf("fresh DataTransfer types = %v, want empty", got)
	}

	dt.SetData("text/plain", "hello")
	dt.SetData("text/html", "<b>hi</b>")
	if got := dt.GetData("text/plain"); got != "hello" {
		t.Errorf("GetData(text/plain) = %q, want %q", got, "hello")
	}
	if got := dt.GetData("TEXT/PLAIN"); got != "hello" {
		t.Errorf("format lookup is case-insensitive: got %q", got)
	}

	// Types keeps insertion order.
	types := dt.Types()
	if len(types) != 2 || types[0] != "text/plain" || types[1] != "text/html" {
		t.Errorf("Types() = %v, want [text/plain text/html]", types)
	}

	// Re-setting a format replaces the entry without changing its position.
	dt.SetData("text/plain", "replaced")
	if got := dt.GetData("text/plain"); got != "replaced" {
		t.Errorf("re-set data = %q, want %q", got, "replaced")
	}
	if types := dt.Types(); len(types) != 2 || types[0] != "text/html" || types[1] != "text/plain" {
		t.Errorf("re-set did not move the replaced format to the end: %v", types)
	}

	// The legacy "text" keyword aliases text/plain.
	if got := dt.GetData("text"); got != "replaced" {
		t.Errorf(`GetData("text") = %q, want the text/plain entry`, got)
	}
	dt.SetData("Text", "legacy")
	if got := dt.GetData("text/plain"); got != "legacy" {
		t.Errorf(`SetData("Text") did not alias text/plain: got %q`, got)
	}

	dt.ClearData("text/html")
	if got := dt.GetData("text/html"); got != "" {
		t.Errorf("ClearData(format) left data behind: %q", got)
	}
	if types := dt.Types(); len(types) != 1 || types[0] != "text/plain" {
		t.Errorf("Types() after ClearData(format) = %v, want [text/plain]", types)
	}

	dt.ClearData()
	if got := dt.Types(); len(got) != 0 {
		t.Errorf("ClearData() left formats behind: %v", got)
	}
}

func TestDataTransferURLFormatAlias(t *testing.T) {
	dt := dom.NewDataTransfer()
	value := "# first comment\r\n\r\nhttps://example.test/one\r\nhttps://example.test/two\r\n"
	dt.SetData("URL", value)
	if got := dt.GetData("url"); got != "https://example.test/one" {
		t.Errorf(`GetData("url") = %q, want first URI-list URL`, got)
	}
	if got := dt.GetData("text/uri-list"); got != value {
		t.Errorf("GetData(text/uri-list) = %q, want raw URI list %q", got, value)
	}
	if types := dt.Types(); len(types) != 1 || types[0] != "text/uri-list" {
		t.Errorf("types after SetData(url) = %v, want [text/uri-list]", types)
	}
	dt.ClearData("url")
	if got := dt.GetData("text/uri-list"); got != "" || len(dt.Types()) != 0 {
		t.Errorf("ClearData(url) left URI data behind: data=%q types=%v", got, dt.Types())
	}
}

func TestDataTransferEffectValues(t *testing.T) {
	dt := dom.NewDataTransfer()
	if got := dt.DropEffect(); got != "none" {
		t.Errorf("initial dropEffect = %q, want %q", got, "none")
	}
	if got := dt.EffectAllowed(); got != "none" {
		t.Errorf("initial effectAllowed = %q, want %q", got, "none")
	}

	for _, effect := range []string{"copy", "move", "link", "none"} {
		dt.SetDropEffect(effect)
		if dt.DropEffect() != effect {
			t.Errorf("SetDropEffect(%q) not stored", effect)
		}
	}
	dt.SetDropEffect("banana")
	if dt.DropEffect() != "none" {
		t.Errorf("invalid dropEffect %q was stored", "banana")
	}

	for _, allowed := range []string{"none", "copy", "copyLink", "copyMove", "link", "linkMove", "move", "all", "uninitialized"} {
		dt.SetEffectAllowed(allowed)
		if dt.EffectAllowed() != allowed {
			t.Errorf("SetEffectAllowed(%q) not stored", allowed)
		}
	}
	dt.SetEffectAllowed("everything")
	if dt.EffectAllowed() != "uninitialized" {
		t.Errorf("invalid effectAllowed was stored")
	}

	// setDragImage is a no-op that must not panic.
	dt.SetDragImage(nil, 10, 10)
}

func TestDragOperationRequiresAllowedEffect(t *testing.T) {
	tests := []struct {
		allowed string
		drop    string
		want    string
	}{
		{allowed: "uninitialized", drop: "copy", want: "copy"},
		{allowed: "copy", drop: "copy", want: "copy"},
		{allowed: "copyLink", drop: "link", want: "link"},
		{allowed: "copyMove", drop: "move", want: "move"},
		{allowed: "linkMove", drop: "copy", want: "none"},
		{allowed: "none", drop: "copy", want: "none"},
		{allowed: "move", drop: "link", want: "none"},
		{allowed: "all", drop: "move", want: "move"},
	}
	for _, tt := range tests {
		t.Run(tt.allowed+"/"+tt.drop, func(t *testing.T) {
			if got := dom.DragOperation(tt.allowed, tt.drop); got != tt.want {
				t.Errorf("DragOperation(%q, %q) = %q, want %q", tt.allowed, tt.drop, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// draggable reflection
// ---------------------------------------------------------------------------

func TestDraggableReflection(t *testing.T) {
	doc, err := dom.ParseBounded(`<html><body>
		<div id="plain">x</div>
		<div id="opt" draggable="true">x</div>
		<img id="pic" src="a.png">
		<img id="picOff" src="a.png" draggable="false">
		<a id="link" href="https://example.test/">l</a>
		<a id="nolink">l</a>
	</body></html>`, dom.ParseLimits{
		Nodes: 100000, Depth: 128, Attributes: 64, AttributeBytes: 4096,
	})
	if err != nil {
		t.Fatalf("ParseBounded: %v", err)
	}

	cases := []struct {
		id   string
		want bool
	}{
		{"plain", false},
		{"opt", true},
		{"pic", true},     // img default
		{"picOff", false}, // explicit attribute wins over the img default
		{"link", true},    // a[href] default
		{"nolink", false},
	}
	for _, c := range cases {
		n := doc.ElementByID(c.id)
		if n == nil {
			t.Fatalf("element %s missing", c.id)
		}
		if got := dom.IsDraggable(n); got != c.want {
			t.Errorf("IsDraggable(%s) = %v, want %v", c.id, got, c.want)
		}
	}

	// The attribute setter path: set true, then back to false.
	n := doc.ElementByID("plain")
	n.SetAttribute("draggable", "true")
	if !dom.IsDraggable(n) {
		t.Error(`draggable="true" did not make the element draggable`)
	}
	n.SetAttribute("draggable", "false")
	if dom.IsDraggable(n) {
		t.Error(`draggable="false" did not make the element non-draggable`)
	}
	if dom.IsDraggable(nil) {
		t.Error("nil node reported draggable")
	}
}

func TestDraggableJSProperty(t *testing.T) {
	html := `<html><body style="margin:0">
		<div id="d" style="width:50px;height:50px;">x</div>
		<img id="im" src="a.png" style="width:20px;height:20px;">
		<a id="lnk" href="https://example.test/">l</a>
	</body></html>`
	sess, _ := newJSSession(t, html)

	jsEval := func(expr string) string {
		t.Helper()
		v, err := sess.EvaluateJS(expr)
		if err != nil {
			t.Fatalf("EvaluateJS(%s): %v", expr, err)
		}
		return v
	}

	if got := jsEval(`document.getElementById('d').draggable`); got != "false" {
		t.Errorf("div draggable default = %q, want false", got)
	}
	if got := jsEval(`document.getElementById('im').draggable`); got != "true" {
		t.Errorf("img draggable default = %q, want true", got)
	}
	if got := jsEval(`document.getElementById('lnk').draggable`); got != "true" {
		t.Errorf("a[href] draggable default = %q, want true", got)
	}
	if got := jsEval(`JSON.stringify([HTMLElement.prototype.hasOwnProperty('draggable'), Object.prototype.hasOwnProperty.call(document.getElementById('d'), 'draggable')])`); got != `[true,false]` {
		t.Errorf("draggable IDL placement = %s, want [true,false]", got)
	}

	// el.draggable = true sets the attribute.
	if got := jsEval(`var d = document.getElementById('d'); d.draggable = true; d.getAttribute('draggable')`); got != "true" {
		t.Errorf("el.draggable = true gave attribute %q, want %q", got, "true")
	}
	if got := jsEval(`document.getElementById('d').draggable`); got != "true" {
		t.Errorf("el.draggable after set = %q, want true", got)
	}
	// el.draggable = false writes draggable="false" (removing the attribute
	// would silently restore the img/link auto defaults).
	if got := jsEval(`var d = document.getElementById('d'); d.draggable = false; d.getAttribute('draggable')`); got != "false" {
		t.Errorf("el.draggable = false gave attribute %q, want %q", got, "false")
	}
}

// ---------------------------------------------------------------------------
// Drag event sequence (engine level)
// ---------------------------------------------------------------------------

// dragBoxCenter returns the center of the laid-out box for the element id.
func dragBoxCenter(t *testing.T, s *engine.Session, id string) (float32, float32) {
	t.Helper()
	if s.Arena == nil {
		t.Fatal("layout arena is nil")
	}
	for _, obj := range s.Arena.Objects {
		if obj.Node != nil && obj.Node.GetAttribute("id") == id {
			x0, y0, x1, y1 := obj.BorderRect()
			return (x0 + x1) / 2, (y0 + y1) / 2
		}
	}
	t.Fatalf("element %s not found in the layout arena", id)
	return 0, 0
}

// dragHTML lays out a draggable source box above a drop target box.
const dragHTML = `<html><body style="margin:0">
	<div id="src" draggable="true" style="width:100px;height:100px;">src</div>
	<div id="dst" style="width:100px;height:100px;">dst</div>
</body></html>`

func TestDragEventSequence(t *testing.T) {
	sess, err := engine.NewSession(dragHTML, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	var seq []string
	record := func(elem, typ string) func(*dom.Event) {
		return func(*dom.Event) { seq = append(seq, elem+":"+typ) }
	}
	src := sess.Doc.ElementByID("src")
	dst := sess.Doc.ElementByID("dst")
	for _, typ := range []string{"dragstart", "drag", "dragend", "dragleave"} {
		dom.AddEventListener(src, typ, record("src", typ), false)
	}
	for _, typ := range []string{"dragenter", "dragover", "drop"} {
		dom.AddEventListener(dst, typ, record("dst", typ), false)
	}
	// A drop is allowed only when dragover is canceled.
	dom.AddEventListener(dst, "dragover", func(e *dom.Event) { e.PreventDefault() }, false)

	sx, sy := dragBoxCenter(t, sess, "src")
	dx, dy := dragBoxCenter(t, sess, "dst")

	if !sess.StartDrag(sx, sy) {
		t.Fatal("StartDrag on a draggable element returned false")
	}
	sess.DragTo(dx, dy)
	sess.EndDrag(dx, dy)

	want := []string{
		"src:dragstart",
		"dst:dragenter",
		"dst:dragover",
		"src:drag",
		"dst:drop",
		"src:dragend",
	}
	if len(seq) != len(want) {
		t.Fatalf("event sequence = %v, want %v", seq, want)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("event sequence = %v, want %v", seq, want)
		}
	}
	if sess.Dragging() {
		t.Error("drag state survived EndDrag")
	}
}

func TestDropRequiresCanceledDragover(t *testing.T) {
	sess, err := engine.NewSession(dragHTML, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	dropped := false
	dragEnded := false
	dst := sess.Doc.ElementByID("dst")
	src := sess.Doc.ElementByID("src")
	dom.AddEventListener(dst, "drop", func(*dom.Event) { dropped = true }, false)
	dom.AddEventListener(src, "dragend", func(*dom.Event) { dragEnded = true }, false)
	// No dragover listener: the target never cancels dragover, so it is not a
	// valid drop target.

	sx, sy := dragBoxCenter(t, sess, "src")
	dx, dy := dragBoxCenter(t, sess, "dst")

	if !sess.StartDrag(sx, sy) {
		t.Fatal("StartDrag returned false")
	}
	sess.DragTo(dx, dy)
	sess.EndDrag(dx, dy)

	if dropped {
		t.Error("drop fired on a target whose dragover was never canceled")
	}
	if !dragEnded {
		t.Error("dragend did not fire after a rejected drop")
	}
}

func TestDraggableElementDoesNotImplicitlyAcceptDrop(t *testing.T) {
	// Being draggable does not select a non-none drag operation; dragover still
	// has to be canceled and choose an operation allowed by the source.
	html := `<html><body style="margin:0">
		<div id="src" draggable="true" style="width:100px;height:100px;">src</div>
		<div id="dst" draggable="true" style="width:100px;height:100px;">dst</div>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	dropped := false
	dst := sess.Doc.ElementByID("dst")
	dom.AddEventListener(dst, "drop", func(*dom.Event) { dropped = true }, false)

	sx, sy := dragBoxCenter(t, sess, "src")
	dx, dy := dragBoxCenter(t, sess, "dst")

	if !sess.StartDrag(sx, sy) {
		t.Fatal("StartDrag returned false")
	}
	sess.DragTo(dx, dy)
	sess.EndDrag(dx, dy)

	if dropped {
		t.Error("drop fired on a draggable target without a selected operation")
	}
}

func TestDragStartPreventedCancelsDrag(t *testing.T) {
	sess, err := engine.NewSession(dragHTML, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	src := sess.Doc.ElementByID("src")
	dom.AddEventListener(src, "dragstart", func(e *dom.Event) { e.PreventDefault() }, false)

	sx, sy := dragBoxCenter(t, sess, "src")
	if sess.StartDrag(sx, sy) {
		t.Fatal("StartDrag succeeded although dragstart was canceled")
	}
	if sess.Dragging() {
		t.Error("drag state exists after a canceled dragstart")
	}
	// EndDrag must be a no-op and must not fire dragend for a drag that
	// never started.
	ended := false
	dom.AddEventListener(src, "dragend", func(*dom.Event) { ended = true }, false)
	sess.EndDrag(sx, sy)
	if ended {
		t.Error("dragend fired for a canceled drag")
	}
}

func TestStartDragRequiresDraggable(t *testing.T) {
	html := `<html><body style="margin:0">
		<div id="plain" style="width:100px;height:100px;">x</div>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	x, y := dragBoxCenter(t, sess, "plain")
	if sess.StartDrag(x, y) {
		t.Fatal("StartDrag succeeded on a non-draggable element")
	}
	if sess.Dragging() {
		t.Error("drag state exists after a rejected StartDrag")
	}
	// DragTo/EndDrag with no active drag are no-ops.
	sess.DragTo(x+10, y+10)
	sess.EndDrag(x+10, y+10)
}

func TestCancelDrag(t *testing.T) {
	sess, err := engine.NewSession(dragHTML, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	var seq []string
	src := sess.Doc.ElementByID("src")
	dst := sess.Doc.ElementByID("dst")
	dom.AddEventListener(dst, "dragleave", func(*dom.Event) { seq = append(seq, "dst:dragleave") }, false)
	dom.AddEventListener(src, "dragend", func(*dom.Event) { seq = append(seq, "src:dragend") }, false)
	dom.AddEventListener(dst, "drop", func(*dom.Event) { seq = append(seq, "dst:drop") }, false)
	dom.AddEventListener(dst, "dragover", func(e *dom.Event) { e.PreventDefault() }, false)

	sx, sy := dragBoxCenter(t, sess, "src")
	dx, dy := dragBoxCenter(t, sess, "dst")

	if !sess.StartDrag(sx, sy) {
		t.Fatal("StartDrag returned false")
	}
	sess.DragTo(dx, dy)
	sess.CancelDrag()

	want := []string{"dst:dragleave", "src:dragend"}
	if len(seq) != len(want) {
		t.Fatalf("cancel sequence = %v, want %v", seq, want)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Errorf("cancel sequence = %v, want %v", seq, want)
		}
	}
	if sess.Dragging() {
		t.Error("drag state survived CancelDrag")
	}
}

func TestDragEntersAndLeavesMultipleTargets(t *testing.T) {
	html := `<html><body style="margin:0">
		<div id="src" draggable="true" style="width:100px;height:100px;">src</div>
		<div id="mid" style="width:100px;height:100px;">mid</div>
		<div id="dst" style="width:100px;height:100px;">dst</div>
	</body></html>`
	sess, err := engine.NewSession(html, nil, 800, engine.WithViewportH(600))
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Close()

	var seq []string
	dst := sess.Doc.ElementByID("dst")
	mid := sess.Doc.ElementByID("mid")
	dom.AddEventListener(mid, "dragenter", func(*dom.Event) { seq = append(seq, "mid:dragenter") }, false)
	dom.AddEventListener(mid, "dragleave", func(*dom.Event) { seq = append(seq, "mid:dragleave") }, false)
	dom.AddEventListener(dst, "dragenter", func(*dom.Event) { seq = append(seq, "dst:dragenter") }, false)
	dom.AddEventListener(dst, "dragover", func(e *dom.Event) { e.PreventDefault() }, false)
	dom.AddEventListener(dst, "drop", func(*dom.Event) { seq = append(seq, "dst:drop") }, false)

	sx, sy := dragBoxCenter(t, sess, "src")
	mx, my := dragBoxCenter(t, sess, "mid")
	dx, dy := dragBoxCenter(t, sess, "dst")

	if !sess.StartDrag(sx, sy) {
		t.Fatal("StartDrag returned false")
	}
	sess.DragTo(mx, my)
	sess.DragTo(dx, dy)
	sess.EndDrag(dx, dy)

	want := []string{"mid:dragenter", "mid:dragleave", "dst:dragenter", "dst:drop"}
	if len(seq) != len(want) {
		t.Fatalf("multi-target sequence = %v, want %v", seq, want)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Errorf("multi-target sequence = %v, want %v", seq, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Drag through the JS bridge
// ---------------------------------------------------------------------------

func TestDragDataTransferThroughJS(t *testing.T) {
	html := `<html><body style="margin:0">
		<div id="src" draggable="true" style="width:100px;height:100px;">src</div>
		<div id="dst" style="width:100px;height:100px;">dst</div>
		<script>
			var got = '';
			var effects = '';
			var coords = 0;
			var src = document.getElementById('src');
			var dst = document.getElementById('dst');
			src.addEventListener('dragstart', function(e) {
				e.dataTransfer.setData('text/plain', 'hello');
				e.dataTransfer.effectAllowed = 'copy';
			});
			dst.addEventListener('dragover', function(e) {
				e.preventDefault();
				e.dataTransfer.dropEffect = 'copy';
				coords = e.clientX;
			});
			dst.addEventListener('drop', function(e) {
				got = e.dataTransfer.getData('text/plain');
				effects += e.dataTransfer.effectAllowed + '/' + e.dataTransfer.dropEffect;
			});
		</script>
	</body></html>`
	sess, rt := newJSSession(t, html)

	sx, sy := dragBoxCenter(t, sess, "src")
	dx, dy := dragBoxCenter(t, sess, "dst")

	if !sess.StartDrag(sx, sy) {
		t.Fatal("StartDrag returned false")
	}
	sess.DragTo(dx, dy)
	sess.EndDrag(dx, dy)

	if v, ok := rt.Global("got"); !ok || v.Str != "hello" {
		t.Errorf("drop read dataTransfer %v (present %v), want %q", v, ok, "hello")
	}
	if v, ok := rt.Global("effects"); !ok || v.Str != "copy/copy" {
		t.Errorf("effectAllowed/dropEffect across the drag = %v (present %v), want %q", v, ok, "copy/copy")
	}
	// dragover fires at the target's center.
	if v, ok := rt.Global("coords"); !ok || int(v.Num) != int(dx) {
		t.Errorf("dragover clientX = %v (present %v), want %v", v, ok, dx)
	}
}

func TestDragOperationControlsDropAndDragEnd(t *testing.T) {
	run := func(t *testing.T, allowed, requested string) (string, string) {
		t.Helper()
		html := `<html><body style="margin:0">
			<div id="src" draggable="true" style="width:100px;height:100px;">src</div>
			<div id="dst" style="width:100px;height:100px;">dst</div>
			<script>
				var dropped = '';
				var ended = '';
				document.getElementById('src').addEventListener('dragstart', function(e) {
					e.dataTransfer.effectAllowed = '` + allowed + `';
				});
				document.getElementById('dst').addEventListener('dragover', function(e) {
					e.dataTransfer.dropEffect = '` + requested + `';
					e.preventDefault();
				});
				document.getElementById('dst').addEventListener('drop', function(e) {
					dropped = e.dataTransfer.dropEffect;
					e.preventDefault();
				});
				document.getElementById('src').addEventListener('dragend', function(e) {
					ended = e.dataTransfer.dropEffect;
				});
			</script>
		</body></html>`
		sess, rt := newJSSession(t, html)
		sx, sy := dragBoxCenter(t, sess, "src")
		dx, dy := dragBoxCenter(t, sess, "dst")
		if !sess.StartDrag(sx, sy) {
			t.Fatal("StartDrag returned false")
		}
		sess.DragTo(dx, dy)
		sess.EndDrag(dx, dy)
		droppedValue, _ := rt.Global("dropped")
		endedValue, _ := rt.Global("ended")
		return droppedValue.Str, endedValue.Str
	}

	t.Run("disallowed operation rejects drop", func(t *testing.T) {
		dropped, ended := run(t, "move", "copy")
		if dropped != "" || ended != "none" {
			t.Errorf("disallowed copy: dropped=%q dragend dropEffect=%q, want empty/none", dropped, ended)
		}
	})
	t.Run("allowed operation reaches canceled drop and dragend", func(t *testing.T) {
		dropped, ended := run(t, "copyMove", "move")
		if dropped != "move" || ended != "move" {
			t.Errorf("allowed move: dropped=%q dragend dropEffect=%q, want move/move", dropped, ended)
		}
	})
}

func TestDragDataTransferAccessModesThroughJS(t *testing.T) {
	html := `<html><body style="margin:0">
		<div id="src" draggable="true" style="width:100px;height:100px;">src</div>
		<div id="dst" style="width:100px;height:100px;">dst</div>
		<script>
			var observations = [];
			var itemErrors = [];
			var itemCallback = '';
			var transferIdentity = [];
			var src = document.getElementById('src');
			var dst = document.getElementById('dst');
			var startTransfer, overTransfer, dropTransfer, startItem, startFileItem;
			src.addEventListener('dragstart', function(e) {
				startTransfer = e.dataTransfer;
				e.dataTransfer.setData('text/plain', 'payload');
				e.dataTransfer.items.add(new File(['bytes'], 'drag.txt'));
				startItem = e.dataTransfer.items[0];
				startFileItem = e.dataTransfer.items[1];
				e.dataTransfer.effectAllowed = 'copy';
				observations.push('start:' + e.dataTransfer.getData('text/plain') + ':' + e.dataTransfer.effectAllowed + ':' + e.dataTransfer.files.length);
			});
			dst.addEventListener('dragover', function(e) {
				transferIdentity.push(e.dataTransfer !== startTransfer);
				overTransfer = e.dataTransfer;
				observations.push('over:' + e.dataTransfer.getData('text/plain') + ':' + e.dataTransfer.types.join(',') + ':' + e.dataTransfer.effectAllowed + ':' + e.dataTransfer.files.length);
				e.dataTransfer.setData('text/html', 'blocked');
				e.dataTransfer.clearData();
				e.dataTransfer.effectAllowed = 'move';
				e.dataTransfer.dropEffect = 'copy';
				e.dataTransfer.items.add('blocked', 'text/html');
				try { e.dataTransfer.items.remove(0); } catch (error) { itemErrors.push(error.name); }
				e.dataTransfer.items[1].getAsString(function(data) { itemCallback = 'protected:' + data; });
				e.preventDefault();
			});
			dst.addEventListener('drop', function(e) {
				transferIdentity.push(e.dataTransfer !== overTransfer);
				dropTransfer = e.dataTransfer;
				observations.push('drop:' + e.dataTransfer.getData('text/plain') + ':' + e.dataTransfer.types.join(',') + ':' + e.dataTransfer.effectAllowed + ':' + e.dataTransfer.files.length);
				e.dataTransfer.setData('text/plain', 'blocked');
				e.dataTransfer.clearData();
				e.dataTransfer.effectAllowed = 'move';
				e.dataTransfer.items.clear();
				try { e.dataTransfer.items.remove(0); } catch (error) { itemErrors.push(error.name); }
				itemErrors.push(e.dataTransfer.items.add('blocked', 'text/html') === null ? 'readonly-add-null' : 'readonly-add-value');
				e.dataTransfer.items[0].getAsString(function(data) { itemCallback = data; });
			});
			src.addEventListener('dragend', function(e) {
				observations.push('end:' + e.dataTransfer.getData('text/plain') + ':' + e.dataTransfer.types.join(',') + ':' + e.dataTransfer.files.length);
			});
		</script>
	</body></html>`
	sess, _ := newJSSession(t, html)
	sx, sy := dragBoxCenter(t, sess, "src")
	dx, dy := dragBoxCenter(t, sess, "dst")
	if !sess.StartDrag(sx, sy) {
		t.Fatal("StartDrag returned false")
	}
	sess.DragTo(dx, dy)
	sess.EndDrag(dx, dy)
	sess.PumpJS(800)

	got, err := sess.EvaluateJS(`observations.join('|')`)
	if err != nil {
		t.Fatalf("read observations: %v", err)
	}
	want := "start:payload:copy:1|over::text/plain,Files:copy:0|drop:payload:text/plain,Files:copy:1|end::text/plain,Files:0"
	if got != want {
		t.Errorf("DataTransfer access modes observed %q, want %q", got, want)
	}
	itemErrors, err := sess.EvaluateJS(`itemErrors.join('|')`)
	if err != nil {
		t.Fatalf("read item errors: %v", err)
	}
	if itemErrors != "InvalidStateError|InvalidStateError|readonly-add-null" {
		t.Errorf("DataTransferItemList access modes observed %q", itemErrors)
	}
	itemCallback, err := sess.EvaluateJS(`itemCallback`)
	if err != nil {
		t.Fatalf("read item callback value: %v", err)
	}
	if itemCallback != "payload" {
		t.Errorf("DataTransferItem.getAsString callback received %q, want payload", itemCallback)
	}
	association, err := sess.EvaluateJS(`[
		transferIdentity.join(','),
		startTransfer.getData('text/plain'), startTransfer.types.length,
		startTransfer.items.length, startTransfer.files.length,
		startItem.kind, startItem.type, startFileItem.kind,
		startFileItem.getAsFile() === null,
		dropTransfer.getData('text/plain'), dropTransfer.types.length,
		dropTransfer.items.length, dropTransfer.files.length
	].join('|')`)
	if err != nil {
		t.Fatalf("read post-event DataTransfer association: %v", err)
	}
	if want := "true,true||0|0|0||||true||0|0|0"; association != want {
		t.Errorf("per-event DataTransfer association result %q, want %q", association, want)
	}
}

func TestDataTransferJSAPI(t *testing.T) {
	sess, _ := newJSSession(t, `<html><body></body></html>`)

	jsEval := func(expr string) string {
		t.Helper()
		v, err := sess.EvaluateJS(expr)
		if err != nil {
			t.Fatalf("EvaluateJS(%s): %v", expr, err)
		}
		return v
	}

	// setData/getData/clearData round trip and types ordering.
	if got := jsEval(`var dt = new DataTransfer();
		dt.setData('text/html', '<b>x</b>');
		dt.setData('text/plain', 'x');
		dt.types.join(',');`); got != "text/html,text/plain" {
		t.Errorf("dt.types = %q, want %q", got, "text/html,text/plain")
	}
	if got := jsEval(`Object.isFrozen(dt.types)`); got != "true" {
		t.Errorf("dt.types is not a frozen array: %v", got)
	}
	if got := jsEval(`dt.getData('TEXT/PLAIN')`); got != "x" {
		t.Errorf("getData is case-insensitive: %q", got)
	}
	if got := jsEval(`dt.getData('text')`); got != "x" {
		t.Errorf(`getData("text") did not alias text/plain: %q`, got)
	}
	if got := jsEval(`dt.clearData('text/html'); dt.types.join(',')`); got != "text/plain" {
		t.Errorf("types after clearData(format) = %q, want %q", got, "text/plain")
	}
	if got := jsEval(`dt.clearData(); dt.types.length`); got != "0" {
		t.Errorf("types after clearData() = %q, want 0", got)
	}

	// dropEffect/effectAllowed round trip with validation.
	if got := jsEval(`dt.dropEffect = 'copy'; dt.dropEffect`); got != "copy" {
		t.Errorf("dropEffect round trip = %q, want %q", got, "copy")
	}
	if got := jsEval(`dt.dropEffect = 'banana'; dt.dropEffect`); got != "copy" {
		t.Errorf("invalid dropEffect stored: %q", got)
	}
	if got := jsEval(`dt.effectAllowed = 'copyMove'; dt.effectAllowed`); got != "copyMove" {
		t.Errorf("effectAllowed round trip = %q, want %q", got, "copyMove")
	}

	// setDragImage is a no-op that must not throw; files is empty; items works.
	if got := jsEval(`dt.setDragImage(null, 1, 2); 'ok'`); got != "ok" {
		t.Errorf("setDragImage threw or returned %q", got)
	}
	if got := jsEval(`dt.files.length`); got != "0" {
		t.Errorf("dt.files.length = %q, want 0", got)
	}
	if got := jsEval(`var addedItem = dt.items.add('itemdata', 'text/plain'); addedItem === dt.items[0] && addedItem === dt.items[0]`); got != "true" {
		t.Errorf("DataTransferItemList.add/item did not return a stable item object: %q", got)
	}
	if got := jsEval(`dt.items.length`); got != "1" {
		t.Errorf("dt.items.length after add = %q, want 1", got)
	}
	if got := jsEval(`dt.items[0].type`); got != "text/plain" {
		t.Errorf("dt.items[0].type = %q, want %q", got, "text/plain")
	}
	if got := jsEval(`(function(){try { dt.items.add('duplicate', 'text/plain'); } catch (error) { return error.name; } return 'no-error';})()`); got != "NotSupportedError" {
		t.Errorf("duplicate DataTransferItemList.add error = %q, want NotSupportedError", got)
	}
	if got := jsEval(`(function(){try { dt.items.add('missing type'); } catch (error) { return error.name; } return 'no-error';})()`); got != "TypeError" {
		t.Errorf("DataTransferItemList.add missing type error = %q, want TypeError", got)
	}
	if got := jsEval(`(function(){try { dt.items.remove(); } catch (error) { return error.name; } return 'no-error';})()`); got != "TypeError" {
		t.Errorf("DataTransferItemList.remove missing index error = %q, want TypeError", got)
	}
	if got := jsEval(`(function(){var old=dt.items[0];dt.items.remove(0);var disabled=old.kind===''&&old.type==='';var fresh=dt.items.add('replacement','text/plain');return [disabled,fresh!==old,dt.items[0]===fresh].join('|')})()`); got != "true|true|true" {
		t.Errorf("removed/re-added DataTransferItem state = %q, want true|true|true", got)
	}
	if got := jsEval(`dt.items.clear(); dt.items.add('literal URL item', 'URL'); dt.types.join(',')`); got != "url" {
		t.Errorf("DataTransferItemList.add aliased its literal type: %q, want url", got)
	}
	if got := jsEval(`dt.getData('url')`); got != "" {
		t.Errorf("getData(url) exposed a literal DataTransferItemList type: %q", got)
	}
	if got := jsEval(`dt.items.clear(); dt.items.length`); got != "0" {
		t.Errorf("dt.items.length after clear = %q, want 0", got)
	}

	// DragEvent cancelability follows the event type.
	if got := jsEval(`new DragEvent('dragover').cancelable`); got != "true" {
		t.Errorf("dragover cancelable = %q, want true", got)
	}
	if got := jsEval(`new DragEvent('dragleave').cancelable`); got != "false" {
		t.Errorf("dragleave cancelable = %q, want false", got)
	}
	if got := jsEval(`new DragEvent('dragend').cancelable`); got != "false" {
		t.Errorf("dragend cancelable = %q, want false", got)
	}
}

// TestDragSequenceThroughJSUsesSharedStore verifies each event receives a
// DataTransfer view over the operation's shared store: data written at
// dragstart is readable at drop, and event ordering holds for JS listeners.
func TestDragSequenceThroughJS(t *testing.T) {
	html := `<html><body style="margin:0">
		<div id="src" draggable="true" style="width:100px;height:100px;">src</div>
		<div id="dst" style="width:100px;height:100px;">dst</div>
		<script>
			var order = '';
			var src = document.getElementById('src');
			var dst = document.getElementById('dst');
			function mark(name) { return function() { order += name + ';'; }; }
			src.addEventListener('dragstart', function(e) {
				order += 'start;';
				e.dataTransfer.setData('text/plain', 'payload');
			});
			src.addEventListener('drag', mark('drag'));
			src.addEventListener('dragend', mark('end'));
			dst.addEventListener('dragenter', mark('enter'));
			dst.addEventListener('dragover', function(e) { e.preventDefault(); });
			dst.addEventListener('drop', function(e) {
				order += 'drop:' + e.dataTransfer.getData('text/plain') + ';';
			});
		</script>
	</body></html>`
	sess, rt := newJSSession(t, html)

	sx, sy := dragBoxCenter(t, sess, "src")
	dx, dy := dragBoxCenter(t, sess, "dst")

	if !sess.StartDrag(sx, sy) {
		t.Fatal("StartDrag returned false")
	}
	sess.DragTo(dx, dy)
	sess.EndDrag(dx, dy)

	v, ok := rt.Global("order")
	if !ok {
		t.Fatal("order global missing")
	}
	// drag fires on the source during the move; drop happens at release.
	want := "start;enter;drag;drop:payload;end;"
	if v.Str != want {
		t.Errorf("JS event order = %q, want %q", v.Str, want)
	}
}

func TestDragEnterLeaveRelatedTargetThroughJS(t *testing.T) {
	html := `<html><body style="margin:0">
		<div id="src" draggable="true" style="width:50px;height:50px">source</div>
		<div id="first" style="width:50px;height:50px">first</div>
		<div id="second" style="width:50px;height:50px">second</div>
		<script>
			var related = [];
			var first = document.getElementById('first');
			var second = document.getElementById('second');
			first.addEventListener('dragenter', e => related.push(e.relatedTarget === null));
			first.addEventListener('dragleave', e => related.push(e.relatedTarget === second));
			second.addEventListener('dragenter', e => related.push(e.relatedTarget === first));
			first.addEventListener('dragover', e => e.preventDefault());
			second.addEventListener('dragover', e => e.preventDefault());
		</script>
	</body></html>`
	sess, _ := newJSSession(t, html)

	sx, sy := dragBoxCenter(t, sess, "src")
	fx, fy := dragBoxCenter(t, sess, "first")
	dx, dy := dragBoxCenter(t, sess, "second")
	if !sess.StartDrag(sx, sy) {
		t.Fatal("StartDrag returned false")
	}
	sess.DragTo(fx, fy)
	sess.DragTo(dx, dy)
	sess.EndDrag(dx, dy)
	got, err := sess.EvaluateJS(`JSON.stringify(related)`)
	if err != nil {
		t.Fatalf("read relatedTarget results: %v", err)
	}
	if got != `[true,true,true]` {
		t.Fatalf("dragenter/dragleave relatedTarget = %s, want [true,true,true]", got)
	}
}
