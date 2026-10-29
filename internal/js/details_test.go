package js

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/dom"
)

func TestDetailsOpenQueuesCoalescedTrustedToggleAndEnforcesNameGroup(t *testing.T) {
	doc, err := dom.ParseBounded(`<html><body><details id="one" name="group"></details><details id="two" name="group"></details></body></html>`, dom.ParseLimits{Nodes: 32, Depth: 8, Attributes: 16, AttributeBytes: 256})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(Options{DOM: doc, URL: "https://example.test/details.html"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`
		var one = document.getElementById('one');
		var two = document.getElementById('two');
		window.detailsToggleCount = 0;
		window.detailsToggleState = '';
		one.addEventListener('toggle', function(e) {
			window.detailsToggleCount++;
			window.detailsToggleState = e.oldState + '/' + e.newState + '/' + e.isTrusted + '/' + (e.source === null);
		});
		one.open = true;
		one.open = false;
		window.detailsToggleBeforeTask = window.detailsToggleCount;
	`, "details-toggle-coalescing"); err != nil {
		t.Fatal(err)
	}
	if got, _ := runtime.Global("detailsToggleBeforeTask"); got.Num != 0 {
		t.Fatalf("toggle event fired synchronously: %v", got.Num)
	}
	if !runtime.Tick() {
		t.Fatal("expected queued details toggle task")
	}
	if got, _ := runtime.Global("detailsToggleState"); got.Str != "closed/closed/true/true" {
		t.Fatalf("coalesced ToggleEvent state = %q, want closed/closed/true/true", got.Str)
	}
	if got, _ := runtime.Global("detailsToggleCount"); got.Num != 1 {
		t.Fatalf("toggle event count = %v, want one coalesced event", got.Num)
	}
	if err := runtime.Run(`
		one.open = true; two.open = true;
		var box = document.createElement('div'); document.body.appendChild(box);
		var fragment = document.createDocumentFragment();
		var first = document.createElement('details'); first.name = 'inserted'; first.setAttribute('open', '');
		var second = document.createElement('details'); second.name = 'inserted'; second.setAttribute('open', '');
		fragment.appendChild(first); fragment.appendChild(second); box.appendChild(fragment);
		window.fragmentDetailsState = String(first.open) + '/' + String(second.open);
		window.reflectedDetailsName = first.name;
		var documentGroup = document.createElement('details');
		documentGroup.name = 'scope'; documentGroup.open = true; box.appendChild(documentGroup);
		var host = document.createElement('div'); document.body.appendChild(host);
		var shadow = host.attachShadow({mode: 'open'});
		var shadowGroup = document.createElement('details');
		shadowGroup.name = 'scope'; shadow.appendChild(shadowGroup); shadowGroup.open = true;
		window.scopedDetailsState = String(documentGroup.open) + '/' + String(shadowGroup.open);
		var existing = document.createElement('details'); existing.name = 'existing'; existing.setAttribute('open', '');
		box.appendChild(existing);
		window.existingDetails = existing;
		window.insertedDetails = document.createElement('details');
		insertedDetails.name = 'existing'; insertedDetails.setAttribute('open', '');
		window.detachedGroupState = String(existingDetails.open) + '/' + String(insertedDetails.open);
	`, "details-name-group"); err != nil {
		t.Fatal(err)
	}
	if one, two := doc.ElementByID("one"), doc.ElementByID("two"); one.HasAttribute("open") || !two.HasAttribute("open") {
		t.Fatalf("exclusive details states = %t/%t, want false/true", one.HasAttribute("open"), two.HasAttribute("open"))
	}
	if got, _ := runtime.Global("fragmentDetailsState"); got.Str != "true/false" {
		t.Fatalf("inserted fragment details states = %q, want true/false", got.Str)
	}
	if got, _ := runtime.Global("reflectedDetailsName"); got.Str != "inserted" {
		t.Fatalf("details.name = %q, want reflected name", got.Str)
	}
	if got, _ := runtime.Global("scopedDetailsState"); got.Str != "true/true" {
		t.Fatalf("document/shadow group states = %q, want true/true", got.Str)
	}
	if got, _ := runtime.Global("detachedGroupState"); got.Str != "true/true" {
		t.Fatalf("detached member group states = %q, want true/true", got.Str)
	}
	if err := runtime.Run(`box.insertBefore(insertedDetails, existingDetails); window.attachedGroupState = String(existingDetails.open) + '/' + String(insertedDetails.open);`, "details-group-insert-before"); err != nil {
		t.Fatal(err)
	}
	if got, _ := runtime.Global("attachedGroupState"); got.Str != "true/false" {
		t.Fatalf("inserted-before member group states = %q, want existing member open and inserted member closed", got.Str)
	}
}

func TestDOMParserOpenDetailsQueuesParserToggleEvent(t *testing.T) {
	doc, err := dom.ParseBounded("<html><body></body></html>", dom.ParseLimits{Nodes: 8, Depth: 4, Attributes: 8, AttributeBytes: 128})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(Options{DOM: doc, URL: "https://example.test/details.html"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`
		var parsedDetails = new DOMParser().parseFromString('<details id="parsed" open></details>', 'text/html').getElementById('parsed');
		window.parserToggle = 'pending';
		parsedDetails.addEventListener('toggle', function(event) {
			window.parserToggle = event.oldState + '/' + event.newState + '/' + event.isTrusted;
		});
	`, "domparser-details-toggle"); err != nil {
		t.Fatal(err)
	}
	if got, _ := runtime.Global("parserToggle"); got.Str != "pending" {
		t.Fatalf("parser toggle fired synchronously: %q", got.Str)
	}
	if !runtime.Tick() {
		t.Fatal("expected queued parser toggle task")
	}
	if got, _ := runtime.Global("parserToggle"); got.Str != "closed/open/true" {
		t.Fatalf("DOMParser details toggle = %q, want closed/open/true", got.Str)
	}
}

func TestDetailsToggleCoalescingMovesDisplacedMemberToTaskTail(t *testing.T) {
	doc, err := dom.ParseBounded(`<html><body><details id="first" name="group" open></details><details id="second" name="group"></details></body></html>`, dom.ParseLimits{Nodes: 16, Depth: 8, Attributes: 16, AttributeBytes: 256})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := New(Options{DOM: doc, URL: "https://example.test/details.html"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Run(`
		var first = document.getElementById('first');
		var second = document.getElementById('second');
		window.detailsToggleOrder = '';
		first.addEventListener('toggle', function() { detailsToggleOrder += 'first,'; });
		second.addEventListener('toggle', function() { detailsToggleOrder += 'second,'; });
		second.open = true;
	`, "details-toggle-task-order"); err != nil {
		t.Fatal(err)
	}
	if !runtime.Tick() {
		t.Fatal("expected queued details toggle task")
	}
	if !runtime.Tick() {
		t.Fatal("expected displaced member toggle task")
	}
	if got, _ := runtime.Global("detailsToggleOrder"); got.Str != "second,first," {
		t.Fatalf("details toggle order = %q, want second,first", got.Str)
	}
}
