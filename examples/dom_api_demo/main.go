package main

import (
	"fmt"
	"github.com/vyquocvu/goosie/internal/js"
)

func main() {
	fmt.Println("=== Goosie DOM API Examples ===")

	// Create a new JavaScript runtime
	runtime := js.NewRuntime()

	// Set up HTML content for testing
	html := `
		<html>
			<body>
				<div id="container" class="main-container">
					<h1 id="title">Welcome to Goosie</h1>
					<p class="text">This is a demonstration of DOM APIs.</p>
					<p class="text">Multiple query methods are supported.</p>
					<div class="item">Item 1</div>
					<div class="item">Item 2</div>
					<div class="item">Item 3</div>
					<button id="submit-btn">Submit</button>
				</div>
			</body>
		</html>
	`
	runtime.SetHTMLContent(html)

	// Example 1: Query by ID
	fmt.Println("Example 1: document.getElementById()")
	_, err := runtime.RunScript(`
		var title = document.getElementById("title");
		if (title) {
			console.log("Found element: " + title.tagName);
			console.log("Text content: " + title.textContent);
		}
	`)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Println()

	// Example 2: Query by class name
	fmt.Println("Example 2: document.getElementsByClassName()")
	_, err = runtime.RunScript(`
		var items = document.getElementsByClassName("item");
		console.log("Found " + items.length + " items");
		for (var i = 0; i < items.length; i++) {
			console.log("Item " + (i + 1) + ": " + items[i].textContent);
		}
	`)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Println()

	// Example 3: Query by tag name
	fmt.Println("Example 3: document.getElementsByTagName()")
	_, err = runtime.RunScript(`
		var paragraphs = document.getElementsByTagName("p");
		console.log("Found " + paragraphs.length + " paragraphs");
	`)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Println()

	// Example 4: CSS Selector queries
	fmt.Println("Example 4: document.querySelector() and querySelectorAll()")
	_, err = runtime.RunScript(`
		// Query by ID selector
		var container = document.querySelector("#container");
		console.log("Container ID: " + container.id);
		
		// Query by class selector
		var firstText = document.querySelector(".text");
		console.log("First text paragraph: " + firstText.textContent);
		
		// Query all with class
		var allText = document.querySelectorAll(".text");
		console.log("Total text paragraphs: " + allText.length);
	`)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Println()

	// Example 5: Create and manipulate elements
	fmt.Println("Example 5: createElement() and appendChild()")
	_, err = runtime.RunScript(`
		// Create a new element
		var newDiv = document.createElement("div");
		newDiv.textContent = "This is a dynamically created element";
		
		// Get the container
		var container = document.getElementById("container");
		
		// Append the new element
		container.appendChild(newDiv);
		console.log("Created and appended new element");
		console.log("Container now has " + container.children.length + " children");
	`)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Println()

	// Example 6: Replace and remove children
	fmt.Println("Example 6: replaceChild() and removeChild()")
	_, err = runtime.RunScript(`
		var parent = document.getElementById("container");
		
		// Create a replacement element
		var replacement = document.createElement("p");
		replacement.textContent = "This replaces the submit button";
		
		// Get the button to replace
		var button = document.querySelector("#submit-btn");
		
		if (button && parent) {
			parent.replaceChild(replacement, button);
			console.log("Replaced button with new paragraph");
		}
	`)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Println()

	// Example 7: Event listeners
	fmt.Println("Example 7: addEventListener()")
	_, err = runtime.RunScript(`
		var container = document.getElementById("container");
		
		if (container) {
			container.addEventListener("click", function() {
				console.log("Container clicked!");
			});
			console.log("Event listener added successfully");
		}
	`)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Println()

	// Example 8: Access element properties
	fmt.Println("Example 8: Element properties")
	_, err = runtime.RunScript(`
		var container = document.querySelector("#container");
		
		console.log("Tag name: " + container.tagName);
		console.log("ID: " + container.id);
		
		if (container.classList) {
			console.log("Classes: " + container.classList.join(", "));
		}
		
		if (container.attributes) {
			console.log("Has attributes object");
		}
	`)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
	fmt.Println()

	fmt.Println("=== Examples Complete ===")
}
