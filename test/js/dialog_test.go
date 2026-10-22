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
	if res[2] != true {
		t.Errorf("closedFired expected true, got %v", res[2])
	}
	if res[3] != "confirmed" {
		t.Errorf("returnValue expected 'confirmed', got %v", res[3])
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
