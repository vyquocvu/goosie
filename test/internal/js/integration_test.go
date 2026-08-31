package js_test

import (
	"strings"
	"testing"

	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/js"
)

// TestJSIntegration validates that a script running in the JS runtime
// can query and modify a DOM built from an HTML string.
func TestJSIntegration(t *testing.T) {
	htmlStr := `<!DOCTYPE html>
<html>
<head><title>Test</title></head>
<body>
	<div id="target">Initial</div>
</body>
</html>`

	// Create JS runtime
	rt := js.NewRuntime()

	// Set the parsed DOM directly via HTML content
	rt.SetHTMLContent(htmlStr)

	// The script modifies the text content of the element
	script := `
		var target = document.getElementById("target");
		target.textContent = "Modified by JS";

		// Create a new element
		var newDiv = document.createElement("div");
		newDiv.id = "new-div";
		newDiv.textContent = "I am new";
		document.body.appendChild(newDiv);
	`

	_, err := rt.RunScript(script)
	if err != nil {
		t.Fatalf("Failed to run script: %v", err)
	}

	val, err := rt.VM().RunString("document.body.innerHTML")
	if err != nil {
		t.Fatalf("Failed to get innerHTML: %v", err)
	}
	updatedHTML := val.String()

	if !strings.Contains(updatedHTML, "Modified by JS") {
		t.Errorf("Expected DOM to be mutated with 'Modified by JS', got HTML: %s", updatedHTML)
	}
	if !strings.Contains(updatedHTML, "I am new") {
		t.Errorf("Expected DOM to contain new element 'I am new', got HTML: %s", updatedHTML)
	}
	if !strings.Contains(updatedHTML, `id="new-div"`) {
		t.Errorf("Expected DOM to contain new element id 'new-div', got HTML: %s", updatedHTML)
	}
}

// TestFetchIntegration validates that fetch() returns a promise. We mock the HTTP fetching process since
// relying on actual networking via the enqueueTask is asynchronous and proven in runtime_test.go.
func TestFetchIntegration(t *testing.T) {
	rt := js.NewRuntime()

	fetcher := net.NewFetcher()
	rt.SetFetcher(fetcher)

	// Since we don't simulate a full async loop here (already covered by runtime_test.go),
	// we just test the JS execution integrates fetch properly without throwing synchronous errors.
	_, err := rt.RunScript(`
		var fetchResult = fetch("https://api.github.com/");
	`)
	if err != nil {
		t.Fatalf("Failed to run script: %v", err)
	}

	val, _ := rt.VM().RunString("typeof fetchResult.then")
	if val.String() != "function" {
		t.Errorf("Expected fetch to return a promise, got typeof then: %s", val.String())
	}
}
