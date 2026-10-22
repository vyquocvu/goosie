package js_test

import (
	"testing"
	"time"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/js"
)

func TestDragEventAndDataTransfer(t *testing.T) {
	doc := dom.NewDocument()
	html := doc.NewElement("html")
	doc.Node.AppendChild(html)
	body := doc.NewElement("body")
	html.AppendChild(body)
	div := doc.NewElement("div")
	div.SetAttribute("id", "dropzone")
	body.AppendChild(div)

	r, err := js.New(js.Options{
		Timeout: 5 * time.Second,
		URL:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	defer r.Close()

	r.SetDOM(doc)

	script := `
		var dt = new DataTransfer();
		dt.setData("text/plain", "Hello Goosie");
		dt.setData("application/json", '{"key": "val"}');

		if (dt.getData("text/plain") !== "Hello Goosie") {
			throw new Error("getData text/plain mismatch: " + dt.getData("text/plain"));
		}

		var types = dt.types;
		if (!types || types.length !== 2) {
			throw new Error("unexpected dt.types: " + JSON.stringify(types));
		}

		dt.clearData("application/json");
		if (dt.getData("application/json") !== "") {
			throw new Error("clearData failed to clear application/json");
		}

		var ev = new DragEvent("dragstart");
		if (!ev || ev.type !== "dragstart" || !ev.dataTransfer) {
			throw new Error("new DragEvent failed: " + JSON.stringify(ev));
		}

		ev.dataTransfer.setData("text/uri-list", "https://example.com");
		if (ev.dataTransfer.getData("text/uri-list") !== "https://example.com") {
			throw new Error("DragEvent dataTransfer getData failed");
		}
	`

	if err := r.Run(script, "test.js"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}
