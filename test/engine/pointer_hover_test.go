package engine_test

import (
	"strings"
	"testing"
)

const pointerHoverPage = `<html><body style="margin:0">
<div id="outer" style="position:absolute;left:50px;top:50px;width:200px;height:200px;background:#eee">
  <div id="inner" style="position:absolute;left:50px;top:50px;width:100px;height:100px;background:#ccc">in</div>
</div>
<div id="other" style="position:absolute;left:300px;top:50px;width:100px;height:100px;background:#dfd">other</div>
<button id="off" disabled style="position:absolute;left:300px;top:200px;width:100px;height:40px">off</button>
<div id="ghost" style="position:absolute;left:80px;top:80px;width:40px;height:40px;pointer-events:none;background:rgba(0,0,0,0.3)">g</div>
<div id="cap" style="position:absolute;left:300px;top:300px;width:100px;height:100px;background:#ccf">cap</div>
<div id="far" style="position:absolute;left:450px;top:300px;width:100px;height:100px;background:#fcc">far</div>
</body></html>`

func installPointerHoverRecorder(s interface {
	EvaluateJS(string) (string, error)
}) error {
	_, err := s.EvaluateJS(`(function(){
		window.evts = [];
		var TYPES = ['pointerover','pointerenter','pointermove','pointerout','pointerleave',
			'mouseover','mouseenter','mousemove','mouseout','mouseleave',
			'pointerdown','mousedown','pointerup','mouseup','click','dblclick',
			'gotpointercapture','lostpointercapture','pointercancel'];
		function label(e) {
			if (e.target === document) { return 'doc'; }
			if (e.target === document.body) { return 'body'; }
			return (e.target && e.target.id) || String((e.target && e.target.nodeName) || '?').toLowerCase();
		}
		TYPES.forEach(function(t) {
			document.addEventListener(t, function(e){ window.evts.push(t + ':c:' + label(e)); }, true);
			document.addEventListener(t, function(e){ window.evts.push(t + ':b:' + label(e)); }, false);
		});
		window.clearEvts = function() { window.evts = []; };
	})()`)
	return err
}

func hoverEvents(s interface {
	EvaluateJS(string) (string, error)
}) string {
	got, err := s.EvaluateJS(`window.evts.join(",")`)
	if err != nil {
		return "evaluate-error: " + err.Error()
	}
	return got
}

func TestHoverEnterRunsPointerBoundaryBeforeMouseCompat(t *testing.T) {
	s, _ := newJSSession(t, pointerHoverPage)
	if err := installPointerHoverRecorder(s); err != nil {
		t.Fatalf("install recorder: %v", err)
	}
	s.SetHover(150, 150)
	want := strings.Join([]string{
		"pointerover:c:inner", "pointerover:b:inner",
		"pointerenter:c:doc", "pointerenter:b:doc",
		"pointerenter:c:html", "pointerenter:c:body", "pointerenter:c:outer", "pointerenter:c:inner",
		"mouseover:c:inner", "mouseover:b:inner",
		"mouseenter:c:doc", "mouseenter:b:doc",
		"mouseenter:c:html", "mouseenter:c:body", "mouseenter:c:outer", "mouseenter:c:inner",
		"pointermove:c:inner", "pointermove:b:inner",
		"mousemove:c:inner", "mousemove:b:inner",
	}, ",")
	if got := hoverEvents(s); got != want {
		t.Fatalf("enter events = %q\nwant %q", got, want)
	}
}

func TestHoverMoveFiresPointerMoveAndMouseMoveOnEveryMotion(t *testing.T) {
	s, _ := newJSSession(t, pointerHoverPage)
	if err := installPointerHoverRecorder(s); err != nil {
		t.Fatalf("install recorder: %v", err)
	}
	s.SetHover(150, 150)
	if _, err := s.EvaluateJS(`window.clearEvts()`); err != nil {
		t.Fatalf("clear events: %v", err)
	}
	s.SetHover(151, 151)
	want := "pointermove:c:inner,pointermove:b:inner,mousemove:c:inner,mousemove:b:inner"
	if got := hoverEvents(s); got != want {
		t.Fatalf("move events = %q, want %q", got, want)
	}
	if _, err := s.EvaluateJS(`window.clearEvts()`); err != nil {
		t.Fatalf("clear events: %v", err)
	}
	s.SetHover(152, 152)
	if got := hoverEvents(s); got != want {
		t.Fatalf("second move events = %q, want %q", got, want)
	}
}

func TestHoverLeaveChainRunsInnermostFirstBeforeEnterChain(t *testing.T) {
	s, _ := newJSSession(t, pointerHoverPage)
	if err := installPointerHoverRecorder(s); err != nil {
		t.Fatalf("install recorder: %v", err)
	}
	s.SetHover(150, 150)
	if _, err := s.EvaluateJS(`window.clearEvts()`); err != nil {
		t.Fatalf("clear events: %v", err)
	}
	s.SetHover(350, 100)
	want := strings.Join([]string{
		"pointerout:c:inner", "pointerout:b:inner",
		"pointerleave:c:inner", "pointerleave:c:outer",
		"pointerover:c:other", "pointerover:b:other",
		"pointerenter:c:other",
		"mouseout:c:inner", "mouseout:b:inner",
		"mouseleave:c:inner", "mouseleave:c:outer",
		"mouseover:c:other", "mouseover:b:other",
		"mouseenter:c:other",
		"pointermove:c:other", "pointermove:b:other",
		"mousemove:c:other", "mousemove:b:other",
	}, ",")
	if got := hoverEvents(s); got != want {
		t.Fatalf("transition events = %q\nwant %q", got, want)
	}
}

func TestHoverSkipsPointerEventsNoneElement(t *testing.T) {
	s, _ := newJSSession(t, pointerHoverPage)
	if err := installPointerHoverRecorder(s); err != nil {
		t.Fatalf("install recorder: %v", err)
	}
	s.SetHover(100, 100)
	got := hoverEvents(s)
	want := strings.Join([]string{
		"pointerover:c:inner", "pointerover:b:inner",
		"pointerenter:c:doc", "pointerenter:b:doc",
		"pointerenter:c:html", "pointerenter:c:body", "pointerenter:c:outer", "pointerenter:c:inner",
		"mouseover:c:inner", "mouseover:b:inner",
		"mouseenter:c:doc", "mouseenter:b:doc",
		"mouseenter:c:html", "mouseenter:c:body", "mouseenter:c:outer", "mouseenter:c:inner",
		"pointermove:c:inner", "pointermove:b:inner",
		"mousemove:c:inner", "mousemove:b:inner",
	}, ",")
	if got != want {
		t.Fatalf("ghost overlap events = %q\nwant %q", got, want)
	}
	if strings.Contains(got, "ghost") {
		t.Fatalf("pointer-events:none element received events: %q", got)
	}
}

func TestDisabledControlReceivesHoverFamily(t *testing.T) {
	s, _ := newJSSession(t, pointerHoverPage)
	if err := installPointerHoverRecorder(s); err != nil {
		t.Fatalf("install recorder: %v", err)
	}
	s.SetHover(350, 220)
	want := strings.Join([]string{
		"pointerover:c:off", "pointerover:b:off",
		"pointerenter:c:doc", "pointerenter:b:doc",
		"pointerenter:c:html", "pointerenter:c:body", "pointerenter:c:off",
		"mouseover:c:off", "mouseover:b:off",
		"mouseenter:c:doc", "mouseenter:b:doc",
		"mouseenter:c:html", "mouseenter:c:body", "mouseenter:c:off",
		"pointermove:c:off", "pointermove:b:off",
		"mousemove:c:off", "mousemove:b:off",
	}, ",")
	if got := hoverEvents(s); got != want {
		t.Fatalf("disabled hover events = %q\nwant %q", got, want)
	}
}

func TestPointerCaptureDuringPressRetargetsUpAndClick(t *testing.T) {
	s, _ := newJSSession(t, pointerHoverPage)
	if err := installPointerHoverRecorder(s); err != nil {
		t.Fatalf("install recorder: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){
		var cap = document.getElementById('cap');
		cap.addEventListener('pointerdown', function(e){ cap.setPointerCapture(e.pointerId); });
		cap.addEventListener('pointerup', function(e){
			window.evts.push('PROBE:' + cap.hasPointerCapture(e.pointerId));
		});
	})()`); err != nil {
		t.Fatalf("install capture listeners: %v", err)
	}
	s.SetHover(350, 350)
	if _, err := s.EvaluateJS(`window.clearEvts()`); err != nil {
		t.Fatalf("clear events: %v", err)
	}
	s.ClickAt(350, 350)
	want := strings.Join([]string{
		"pointerdown:c:cap", "pointerdown:b:cap",
		"mousedown:c:cap", "mousedown:b:cap",
		"gotpointercapture:c:cap", "gotpointercapture:b:cap",
		"pointerup:c:cap", "PROBE:true", "pointerup:b:cap",
		"mouseup:c:cap", "mouseup:b:cap",
		"lostpointercapture:c:cap", "lostpointercapture:b:cap",
		"click:c:cap", "click:b:cap",
	}, ",")
	if got := hoverEvents(s); got != want {
		t.Fatalf("capture press events = %q\nwant %q", got, want)
	}
}

func TestPointerCaptureRetargetsToCapturedElement(t *testing.T) {
	s, _ := newJSSession(t, pointerHoverPage)
	if err := installPointerHoverRecorder(s); err != nil {
		t.Fatalf("install recorder: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){
		document.getElementById('cap').addEventListener('pointerdown', function(e){
			document.getElementById('far').setPointerCapture(e.pointerId);
		});
	})()`); err != nil {
		t.Fatalf("install capture listener: %v", err)
	}
	s.SetHover(350, 350)
	if _, err := s.EvaluateJS(`window.clearEvts()`); err != nil {
		t.Fatalf("clear events: %v", err)
	}
	s.ClickAt(350, 350)
	want := strings.Join([]string{
		"pointerdown:c:cap", "pointerdown:b:cap",
		"mousedown:c:cap", "mousedown:b:cap",
		"gotpointercapture:c:far", "gotpointercapture:b:far",
		"pointerup:c:far", "pointerup:b:far",
		"mouseup:c:far", "mouseup:b:far",
		"lostpointercapture:c:far", "lostpointercapture:b:far",
		"click:c:far", "click:b:far",
	}, ",")
	if got := hoverEvents(s); got != want {
		t.Fatalf("cross-element capture events = %q\nwant %q", got, want)
	}
}

func TestSetPointerCaptureWhileHoveringDoesNotCapture(t *testing.T) {
	s, _ := newJSSession(t, pointerHoverPage)
	if err := installPointerHoverRecorder(s); err != nil {
		t.Fatalf("install recorder: %v", err)
	}
	s.SetHover(350, 350)
	if _, err := s.EvaluateJS(`window.clearEvts()`); err != nil {
		t.Fatalf("clear events: %v", err)
	}
	if _, err := s.EvaluateJS(`(function(){
		document.getElementById('cap').setPointerCapture(1);
		window.evts.push('PROBE:' + document.getElementById('cap').hasPointerCapture(1));
	})()`); err != nil {
		t.Fatalf("attempt hover capture: %v", err)
	}
	s.SetHover(150, 150)
	got := hoverEvents(s)
	want := strings.Join([]string{
		"PROBE:false",
		"pointerout:c:cap", "pointerout:b:cap", "pointerleave:c:cap",
		"pointerover:c:inner", "pointerover:b:inner",
		"pointerenter:c:outer", "pointerenter:c:inner",
		"mouseout:c:cap", "mouseout:b:cap", "mouseleave:c:cap",
		"mouseover:c:inner", "mouseover:b:inner",
		"mouseenter:c:outer", "mouseenter:c:inner",
		"pointermove:c:inner", "pointermove:b:inner",
		"mousemove:c:inner", "mousemove:b:inner",
	}, ",")
	if got != want {
		t.Fatalf("hover capture attempt events = %q\nwant %q", got, want)
	}
}
