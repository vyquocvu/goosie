package integration

import (
	"context"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestJSDOMIntegration(t *testing.T) {
	testApp := test.NewApp()
	defer testApp.Quit()

	// 1. Initial HTML content
	initialHTML := `
		<html>
		<head>
			<style>
				.box { width: 100px; height: 100px; background-color: red; }
				#target { color: blue; }
			</style>
		</head>
		<body>
			<div id="target" class="box">Initial Content</div>
		</body>
		</html>
	`

	// 2. Setup Renderer and JS Runtime
	r := renderer.NewRenderer(800, 600)
	jsRuntime := js.NewRuntime()

	// Track rendered HTML updates
	var latestRenderedHTML string

	// Wire up mutation callback to update Goosie renderer
	jsRuntime.SetDOMMutationCallback(func(mutatedHTML string) {
		latestRenderedHTML = mutatedHTML
		_, err := r.RenderHTML(context.Background(), mutatedHTML)
		if err != nil {
			t.Errorf("Failed to render mutated HTML: %v", err)
		}
	})

	// Initial render
	_, err := r.RenderHTML(context.Background(), initialHTML)
	assert.NoError(t, err)
	jsRuntime.SetHTMLContent(initialHTML)

	// 3. Test DOM manipulation (jQuery style style/class/attribute/child manipulation)
	_, err = jsRuntime.RunScript(`
		// Query element
		var target = document.getElementById("target");
		
		// Update styles dynamically
		target.style.backgroundColor = "green";
		target.style.width = "200px";
		
		// Update classList
		target.classList.add("active");
		
		// Modify content
		target.textContent = "Modified Content";
		
		// Create and append a new element
		var child = document.createElement("p");
		child.id = "child-elem";
		child.textContent = "Dynamic Child";
		target.appendChild(child);
	`)
	assert.NoError(t, err)

	// Verify that the rendered HTML reflects the dynamic mutations
	assert.NotEmpty(t, latestRenderedHTML)
	assert.Contains(t, latestRenderedHTML, "green")
	assert.Contains(t, latestRenderedHTML, "200px")
	assert.Contains(t, latestRenderedHTML, "active")
	assert.Contains(t, latestRenderedHTML, "Modified Content")
	assert.Contains(t, latestRenderedHTML, "id=\"child-elem\"")
	assert.Contains(t, latestRenderedHTML, "Dynamic Child")

	// 4. Test Event handling & Bubbling
	// Let's add click listener and trigger it
	jsRuntime.SetDOMMutationCallback(func(mutatedHTML string) {
		latestRenderedHTML = mutatedHTML
	})

	_, err = jsRuntime.RunScript(`
		var btn = document.createElement("button");
		btn.id = "btn";
		btn.textContent = "Click Me";
		document.body.appendChild(btn);
		
		btn.addEventListener("click", function(event) {
			btn.textContent = "Clicked!";
			btn.style.color = "yellow";
		});
	`)
	assert.NoError(t, err)

	// Verify button was added
	assert.Contains(t, latestRenderedHTML, "Click Me")

	// Dispatch event
	_, err = jsRuntime.RunScript(`
		var btn = document.getElementById("btn");
		var clickEvent = new Event("click", { bubbles: true });
		btn.dispatchEvent(clickEvent);
	`)
	assert.NoError(t, err)

	// Verify button text and style were updated inside the event callback
	assert.Contains(t, latestRenderedHTML, "Clicked!")
	assert.Contains(t, latestRenderedHTML, "yellow")

	// 5. Test React-like innerHTML templating
	_, err = jsRuntime.RunScript(`
		var container = document.getElementById("target");
		container.innerHTML = "<div class='react-root'><h2 id='react-title'>React App</h2><p>State: active</p></div>";
	`)
	assert.NoError(t, err)

	// Verify innerHTML replacement rendered correctly
	assert.Contains(t, latestRenderedHTML, "react-root")
	assert.Contains(t, latestRenderedHTML, "react-title")
	assert.Contains(t, latestRenderedHTML, "React App")
	assert.Contains(t, latestRenderedHTML, "State: active")

	// Verify that the old target contents (like child-elem) are replaced and gone
	assert.NotContains(t, latestRenderedHTML, "child-elem")
	assert.NotContains(t, latestRenderedHTML, "Dynamic Child")
}
