package main

import (
	"fmt"

	"github.com/vyquocvu/goosie/internal/js"
)

func main() {
	fmt.Println("=== Enhanced Console Features Demo ===")

	// Create a new JavaScript runtime
	runtime := js.NewRuntime()

	// Test 1: Basic console methods
	fmt.Println("Test 1: Basic Console Methods")
	fmt.Println("-------------------------------")
	runtime.RunScript(`
		console.log("This is a log message");
		console.info("This is an info message");
		console.warn("This is a warning message");
		console.error("This is an error message");
	`)
	fmt.Println()

	// Test 2: Console with multiple arguments
	fmt.Println("Test 2: Multiple Arguments")
	fmt.Println("---------------------------")
	runtime.RunScript(`
		console.log("Value:", 42, "Status:", "active");
		console.info("User:", "John Doe", "Age:", 30);
	`)
	fmt.Println()

	// Test 3: Console table with array
	fmt.Println("Test 3: Console Table - Array")
	fmt.Println("------------------------------")
	runtime.RunScript(`
		var fruits = ["Apple", "Banana", "Cherry", "Date", "Elderberry"];
		console.table(fruits);
	`)
	fmt.Println()

	// Test 4: Console table with object
	fmt.Println("Test 4: Console Table - Object")
	fmt.Println("-------------------------------")
	runtime.RunScript(`
		var user = {
			name: "Alice Johnson",
			age: 28,
			email: "alice@example.com",
			role: "Software Engineer"
		};
		console.table(user);
	`)
	fmt.Println()

	// Test 5: JavaScript error tracking
	fmt.Println("Test 5: JavaScript Error Tracking")
	fmt.Println("----------------------------------")
	runtime.RunScript(`var x = ;`) // Intentional syntax error
	errors := runtime.GetJavaScriptErrors()
	fmt.Printf("Tracked errors: %d\n", len(errors))
	fmt.Println()

	// Test 6: Get console messages
	fmt.Println("Test 6: Retrieved Console Messages")
	fmt.Println("-----------------------------------")
	messages := runtime.GetConsoleMessages()
	fmt.Printf("Total console messages: %d\n", len(messages))

	// Display messages by level
	levelCount := make(map[string]int)
	for _, msg := range messages {
		levelCount[msg.Level]++
	}

	fmt.Println("\nMessages by level:")
	for level, count := range levelCount {
		fmt.Printf("  %s: %d\n", level, count)
	}
	fmt.Println()

	// Test 7: Filter messages by level
	fmt.Println("Test 7: Error Messages Only")
	fmt.Println("----------------------------")
	for _, msg := range messages {
		if msg.Level == "error" {
			fmt.Printf("[%s] %s - %s\n",
				msg.Timestamp.Format("15:04:05"),
				msg.Level,
				msg.Message)
		}
	}
	fmt.Println()

	// Test 8: Clear and verify
	fmt.Println("Test 8: Clear Console")
	fmt.Println("----------------------")
	fmt.Printf("Messages before clear: %d\n", len(runtime.GetConsoleMessages()))
	runtime.ClearConsoleMessages()
	fmt.Printf("Messages after clear: %d\n", len(runtime.GetConsoleMessages()))
	fmt.Println()

	// Test 9: New messages after clear
	fmt.Println("Test 9: Messages After Clear")
	fmt.Println("-----------------------------")
	runtime.RunScript(`
		console.log("New log after clear");
		console.info("New info after clear");
	`)
	messages = runtime.GetConsoleMessages()
	fmt.Printf("New messages: %d\n", len(messages))
	fmt.Println()

	// Test 10: Complex table data
	fmt.Println("Test 10: Complex Data Table")
	fmt.Println("----------------------------")
	runtime.RunScript(`
		var numbers = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10];
		console.table(numbers);
	`)
	fmt.Println()

	// Summary
	fmt.Println("=== Demo Complete ===")
	fmt.Println("\nFeatures Demonstrated:")
	fmt.Println("✓ console.log(), console.info(), console.warn(), console.error()")
	fmt.Println("✓ console.table() for arrays and objects")
	fmt.Println("✓ JavaScript error tracking")
	fmt.Println("✓ Message retrieval and filtering")
	fmt.Println("✓ Console clearing")
	fmt.Println("\nTo test the UI console panel:")
	fmt.Println("1. Run: go run ./cmd/browser")
	fmt.Println("2. Navigate to: file:///path/to/examples/console_demo.html")
	fmt.Println("3. Click the console button (⊞) to show the panel")
	fmt.Println("4. Click demo buttons to test various console features")

	// Cleanup
	runtime.Cleanup()
}
