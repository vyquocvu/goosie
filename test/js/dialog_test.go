package js_test

import (
	"encoding/json"
	"testing"

	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestDialog_OpenAttributeAndProperty(t *testing.T) {
	doc := domtest.Parse(`
		<html>
		<body>
			<dialog id="dlg">Hello</dialog>
		</body>
		</html>
	`)
	rt, err := js.New(js.Options{
		URL: "https://example.com",
		DOM: doc,
	})
	if err != nil {
		t.Fatalf("failed to create js runtime: %v", err)
	}
	defer rt.Close()

	val, err := rt.Evaluate(`
		var dlg = document.getElementById("dlg");
		var initial = dlg.open;
		dlg.open = true;
		var afterSet = dlg.open;
		dlg.open = false;
		var afterClear = dlg.open;
		JSON.stringify([initial, afterSet, afterClear]);
	`)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	var res []bool
	if err := json.Unmarshal([]byte(val), &res); err != nil {
		t.Fatalf("Unmarshal failed: %v, raw: %q", err, val)
	}
	if res[0] != false {
		t.Errorf("initial open expected false, got %v", res[0])
	}
	if res[1] != true {
		t.Errorf("afterSet open expected true, got %v", res[1])
	}
	if res[2] != false {
		t.Errorf("afterClear open expected false, got %v", res[2])
	}
}

func TestDialog_ShowAndClose(t *testing.T) {
	doc := domtest.Parse(`
		<html>
		<body>
			<dialog id="dlg">Content</dialog>
		</body>
		</html>
	`)
	rt, err := js.New(js.Options{
		URL: "https://example.com",
		DOM: doc,
	})
	if err != nil {
		t.Fatalf("failed to create js runtime: %v", err)
	}
	defer rt.Close()

	val, err := rt.Evaluate(`
		var dlg = document.getElementById("dlg");
		var closedFired = false;
		dlg.addEventListener("close", function() {
			closedFired = true;
		});

		dlg.show();
		var openAfterShow = dlg.open;

		dlg.close("confirmed");
		var openAfterClose = dlg.open;
		var retVal = dlg.returnValue;

		JSON.stringify([openAfterShow, openAfterClose, closedFired, retVal]);
	`)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	var res []any
	if err := json.Unmarshal([]byte(val), &res); err != nil {
		t.Fatalf("Unmarshal failed: %v, raw: %q", err, val)
	}
	if res[0] != true {
		t.Errorf("openAfterShow expected true, got %v", res[0])
	}
	if res[1] != false {
		t.Errorf("openAfterClose expected false, got %v", res[1])
	}
	if res[2] != false {
		t.Errorf("close event should be queued as a task, got immediate %v", res[2])
	}
	if res[3] != "confirmed" {
		t.Errorf("returnValue expected 'confirmed', got %v", res[3])
	}
	if !rt.Tick() {
		t.Fatal("queued close event task did not run")
	}
	val, err = rt.Evaluate(`JSON.stringify([closedFired])`)
	if err != nil {
		t.Fatalf("read queued close event: %v", err)
	}
	if val != `[true]` {
		t.Fatalf("close event after a task = %s, want [true]", val)
	}
}

func TestDialog_ShowModal(t *testing.T) {
	doc := domtest.Parse(`
		<html>
		<body>
			<dialog id="dlg">Modal Content</dialog>
		</body>
		</html>
	`)
	rt, err := js.New(js.Options{
		URL: "https://example.com",
		DOM: doc,
	})
	if err != nil {
		t.Fatalf("failed to create js runtime: %v", err)
	}
	defer rt.Close()

	val, err := rt.Evaluate(`
		var dlg = document.getElementById("dlg");
		dlg.showModal();
		var openAfterModal = dlg.open;

		var threw = false;
		try {
			dlg.showModal();
		} catch (e) {
			threw = true;
		}

		dlg.close();
		JSON.stringify([openAfterModal, threw, dlg.open]);
	`)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	var res []any
	if err := json.Unmarshal([]byte(val), &res); err != nil {
		t.Fatalf("Unmarshal failed: %v, raw: %q", err, val)
	}
	if res[0] != true {
		t.Errorf("openAfterModal expected true, got %v", res[0])
	}
	if res[1] != true {
		t.Errorf("second showModal should have thrown, got threw=%v", res[1])
	}
	if res[2] != false {
		t.Errorf("open after close expected false, got %v", res[2])
	}

	node := doc.ElementByID("dlg")
	if node.IsModal {
		t.Errorf("expected IsModal to be false after close")
	}
}

func TestDialogShowModalRejectsDisconnectedDialog(t *testing.T) {
	doc := domtest.Parse(`<html><body><dialog id="connected"></dialog></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	val, err := rt.Evaluate(`(() => {
	  try { document.createElement("dialog").showModal(); }
	  catch (error) { return error.name; }
	  return "no error";
	})()`)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if val != "InvalidStateError" {
		t.Fatalf("showModal on disconnected dialog = %q, want InvalidStateError", val)
	}
}

func TestDialogRequestCloseDispatchesCancelableTrustedEvents(t *testing.T) {
	doc := domtest.Parse(`<html><body><dialog id="dlg"></dialog></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	if _, err := rt.Evaluate(`
		var dlg = document.getElementById('dlg');
		var events = [];
		dlg.addEventListener('cancel', function(e) {
			events.push(['cancel', e.isTrusted]);
			e.preventDefault();
		}, { once: true });
		dlg.addEventListener('close', function(e) { events.push(['close', e.isTrusted]); });
		dlg.show();
		dlg.returnValue = 'kept';
		dlg.requestClose('discarded');
	`); err != nil {
		t.Fatalf("cancel requestClose: %v", err)
	}
	val, err := rt.Evaluate(`JSON.stringify([dlg.open, dlg.returnValue, events])`)
	if err != nil {
		t.Fatalf("read canceled requestClose state: %v", err)
	}
	if val != `[true,"kept",[["cancel",true]]]` {
		t.Fatalf("canceled requestClose state = %s", val)
	}
	if _, err := rt.Evaluate(`dlg.requestClose('accepted')`); err != nil {
		t.Fatalf("successful requestClose: %v", err)
	}
	if !rt.Tick() {
		t.Fatal("queued close task did not run")
	}
	val, err = rt.Evaluate(`JSON.stringify([dlg.open, dlg.returnValue, events])`)
	if err != nil {
		t.Fatalf("read successful requestClose state: %v", err)
	}
	if val != `[false,"accepted",[["cancel",true],["close",true]]]` {
		t.Fatalf("successful requestClose state = %s", val)
	}
}

func TestDialogClosedByReflectionAndDefaults(t *testing.T) {
	doc := domtest.Parse(`<html><body><dialog id="dlg"></dialog></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	val, err := rt.Evaluate(`
		var dlg = document.getElementById('dlg');
		var values = [dlg.closedBy];
		dlg.setAttribute('closedby', 'AnY');
		values.push(dlg.closedBy);
		dlg.setAttribute('closedby', 'invalid');
		values.push(dlg.closedBy);
		dlg.showModal();
		values.push(dlg.closedBy);
		dlg.setAttribute('closedby', 'none');
		values.push(dlg.closedBy);
		dlg.removeAttribute('closedby');
		values.push(dlg.closedBy);
		dlg.close();
		values.push(dlg.closedBy);
		dlg.closedBy = 'CLOSEREQUEST';
		values.push(dlg.closedBy, dlg.getAttribute('closedby'));
		JSON.stringify(values);
	`)
	if err != nil {
		t.Fatalf("evaluate closedBy reflection: %v", err)
	}
	const want = `["none","any","none","closerequest","none","closerequest","none","closerequest","CLOSEREQUEST"]`
	if val != want {
		t.Fatalf("closedBy values = %s, want %s", val, want)
	}
}

func TestDialogModalPseudoClassTracksDialogState(t *testing.T) {
	doc := domtest.Parse(`<html><body><dialog id="dlg"></dialog></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	val, err := rt.Evaluate(`
		var dlg = document.getElementById('dlg');
		var states = [dlg.matches(':modal')];
		dlg.show();
		states.push(dlg.matches(':modal'));
		dlg.close();
		dlg.showModal();
		states.push(dlg.matches(':modal'));
		dlg.close();
		states.push(dlg.matches(':modal'));
		dlg.open = true;
		states.push(dlg.matches(':modal'));
		JSON.stringify(states);
	`)
	if err != nil {
		t.Fatalf("evaluate :modal state: %v", err)
	}
	if val != `[false,false,true,false,false]` {
		t.Fatalf(":modal states = %s", val)
	}
}

func TestAddEventListenerSignalRemovesListenerOnAbort(t *testing.T) {
	doc := domtest.Parse(`<html><body><button id="target"></button></body></html>`)
	rt, err := js.New(js.Options{URL: "https://example.com", DOM: doc})
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	defer rt.Close()
	val, err := rt.Evaluate(`
		var target = document.getElementById('target');
		var controller = new AbortController();
		var calls = 0;
		function listener() { calls++; }
		target.addEventListener('probe', listener, { signal: controller.signal });
		target.dispatchEvent(new Event('probe'));
		controller.abort();
		target.dispatchEvent(new Event('probe'));
		var alreadyAborted = AbortSignal.abort();
		target.addEventListener('probe', listener, { signal: alreadyAborted });
		target.dispatchEvent(new Event('probe'));
		JSON.stringify([calls, alreadyAborted.aborted]);
	`)
	if err != nil {
		t.Fatalf("evaluate signal listener cleanup: %v", err)
	}
	if val != `[1,true]` {
		t.Fatalf("signal listener result = %s, want [1,true]", val)
	}
}
