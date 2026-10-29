package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/js"
)

func TestMutationObserverAttributeFilterAndEmptyOldValue(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com", DOM: observerTestDocument()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var target = document.createElement("div");
		document.body.appendChild(target);
		var observer = new MutationObserver(function() {});
		observer.observe(target, { attributeOldValue: true, attributeFilter: ["id"] });
		target.setAttribute("title", "ignored");
		target.setAttribute("id", "");
		target.setAttribute("id", "next");
		var records = observer.takeRecords();
		if (records.length !== 2) throw new Error("unexpected record count: " + records.length);
		if (records[0].oldValue !== null || records[0].attributeName !== "id" || records[0].attributeNamespace !== null) throw new Error("wrong initial record");
		if (records[1].oldValue !== "" || records[1].attributeName !== "id") throw new Error("empty oldValue was lost");
	`, "mutation-observer.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestMutationObserverOptionsValidation(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com", DOM: observerTestDocument()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var observer = new MutationObserver(function() {});
		var target = document.body;
		function throwsTypeError(options) {
			try { observer.observe(target, options); } catch (e) { return e instanceof TypeError; }
			return false;
		}
		if (!throwsTypeError({})) throw new Error("empty options did not throw");
		if (!throwsTypeError({ attributes: false, attributeOldValue: true })) throw new Error("invalid oldValue did not throw");
		if (!throwsTypeError({ characterData: false, characterDataOldValue: true })) throw new Error("invalid characterDataOldValue did not throw");
		observer.observe(target, { attributeOldValue: true });
		observer.observe(target, { characterDataOldValue: true });
	`, "mutation-observer.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestMutationObserverAppendQueuesOneChildListRecord(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com", DOM: observerTestDocument()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	err = r.Run(`
		var target = document.createElement("div");
		document.body.appendChild(target);
		var observer = new MutationObserver(function() {});
		observer.observe(target, {childList: true});
		target.append("first", "second");
		var records = observer.takeRecords();
		if (records.length !== 1) throw new Error("expected one append record, got " + records.length);
		if (records[0].type !== "childList" || records[0].target !== target) throw new Error("append record has the wrong type or target");
		if (records[0].addedNodes.length !== 2 || records[0].addedNodes[0].data !== "first" || records[0].addedNodes[1].data !== "second") throw new Error("append record has the wrong added nodes");
	`, "mutation-observer-append.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestMutationObserverCallbackArguments(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com", DOM: observerTestDocument()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	err = r.Run(`
		var target = document.createElement("div"); document.body.appendChild(target);
		var observer = new MutationObserver(function(records, receivedObserver) {
			globalThis.callbackThis = this === observer;
				globalThis.argumentCount = arguments.length;
			var testObject = { step: function(callback) { callback(); } };
			testObject.step(() => { globalThis.arrowArgumentCount = arguments.length; });
			globalThis.arrayCheck = Array.isArray(records);
			globalThis.recordCount = records.length;
			globalThis.receivedSameObserver = receivedObserver === observer;
		globalThis.recordBrand = records[0] instanceof MutationRecord;
		});
		observer.observe(target, {attributes:true}); target.className = "changed";
	`, "mutation-observer-callback.js")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for name, want := range map[string]float64{"argumentCount": 2, "arrowArgumentCount": 2, "recordCount": 1} {
		value, ok := r.Global(name)
		if !ok || value.Num != want {
			t.Errorf("%s = %+v, want %v", name, value, want)
		}
	}
	for _, name := range []string{"callbackThis", "receivedSameObserver", "recordBrand", "arrayCheck"} {
		value, ok := r.Global(name)
		if !ok || !value.Bool {
			t.Errorf("%s = %+v, want true", name, value)
		}
	}
}

func TestMutationObserverCallbackArrowArgumentsViaApply(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com", DOM: observerTestDocument()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		var testObject = { step: function(callback, thisArg) {
			if (arguments.length === 1) thisArg = this;
			return callback.apply(thisArg, []);
		} };
		function callbackProbe(records, observer) {
			testObject.step(() => { globalThis.arrowLength = arguments.length; });
		}
		callbackProbe([], {});
	`, "arrow-arguments-apply.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	value, ok := r.Global("arrowLength")
	if !ok || value.Num != 2 {
		t.Fatalf("arrowLength = %+v, want 2", value)
	}
}

func TestArrowCapturesOuterArgumentsObject(t *testing.T) {
	r, err := js.New(js.Options{Timeout: 5 * time.Second, URL: "https://example.com", DOM: observerTestDocument()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()
	if err := r.Run(`
		function directCapture(a, b) { return (() => arguments.length)(); }
		function appliedCapture(a, b) { return (() => arguments.length).apply({}, []); }
		globalThis.directCount = directCapture(1, 2);
		globalThis.appliedCount = appliedCapture(1, 2);
	`, "arrow-arguments-capture.js"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for name, want := range map[string]float64{"directCount": 2, "appliedCount": 2} {
		value, ok := r.Global(name)
		if !ok || value.Num != want {
			t.Errorf("%s = %+v, want %v", name, value, want)
		}
	}
}
