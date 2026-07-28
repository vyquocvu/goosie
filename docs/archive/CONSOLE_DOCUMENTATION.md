# Enhanced Console Features

This document describes the enhanced console features implemented in Goosie browser.

## Overview

The enhanced console provides comprehensive JavaScript debugging and logging capabilities, including:

- Multiple console log levels (log, info, warn, error)
- Structured data display with `console.table()`
- Console panel UI with filtering and error tracking
- Automatic JavaScript error reporting

## Console API

### Console Methods

#### `console.log(...args)`
Standard logging for general messages.

```javascript
console.log("Simple message");
console.log("Value:", 42, "Status:", "active");
```

#### `console.info(...args)`
Informational messages, displayed with [INFO] prefix.

```javascript
console.info("Application initialized");
console.info("Version:", "1.0.0");
```

#### `console.warn(...args)`
Warning messages, displayed with [WARN] prefix.

```javascript
console.warn("Deprecated API usage");
console.warn("Memory usage high:", "85%");
```

#### `console.error(...args)`
Error messages, displayed with [ERROR] prefix.

```javascript
console.error("Failed to load resource");
console.error("Connection error:", errorDetails);
```

#### `console.table(data)`
Display structured data as a formatted table. Supports arrays and objects.

```javascript
// Array table
var fruits = ["Apple", "Banana", "Cherry"];
console.table(fruits);
// Output:
// ┌─────┬─────────────────────────────────────────┐
// │ (i) │ Value                                   │
// ├─────┼─────────────────────────────────────────┤
// │ 0   │ Apple                                   │
// │ 1   │ Banana                                  │
// │ 2   │ Cherry                                  │
// └─────┴─────────────────────────────────────────┘

// Object table
var user = {
    name: "John Doe",
    age: 30,
    email: "john@example.com"
};
console.table(user);
// Output:
// ┌─────────────────────┬──────────────────────┐
// │ Key                 │ Value                │
// ├─────────────────────┼──────────────────────┤
// │ name                │ John Doe            │
// │ age                 │ 30                  │
// │ email               │ john@example.com    │
// └─────────────────────┴──────────────────────┘
```

## Console Panel UI

### Opening the Console Panel

Click the console button (⊞) in the browser toolbar to toggle the console panel.

- **⊞** - Console is hidden
- **⊟** - Console is visible

### Console Panel Features

1. **Message List**: Displays all console messages with timestamps and levels
2. **Filter Dropdown**: Filter messages by level (all/log/error/warn/info/table)
3. **Error Counter**: Shows total number of JavaScript errors
4. **Clear Button**: Clears all console messages

### Message Display

Each message shows:
- **Timestamp**: When the message was logged (HH:MM:SS format)
- **Level**: Message severity ([LOG], [INFO], [WARN], [ERROR], [TABLE])
- **Message**: The logged content

Messages are color-coded by importance:
- **Error**: High importance (red)
- **Warning**: Medium importance (yellow)
- **Info/Log/Table**: Low importance (default)

## JavaScript Error Reporting

JavaScript errors are automatically captured and logged to the console:

```javascript
// This will generate an error message in the console
undefinedFunction(); // JavaScript Error: ReferenceError: undefinedFunction is not defined
```

Errors appear in the console with:
- [ERROR] prefix
- Full error message with stack trace
- Timestamp
- Counted in the error counter

## Usage Examples

### Basic Logging

```javascript
console.log("Application started");
console.info("Loading configuration...");
console.warn("Using default settings");
console.error("Failed to connect to server");
```

### Debugging with Tables

```javascript
// Debug array data
var items = ["Item1", "Item2", "Item3"];
console.table(items);

// Debug object properties
var config = {
    apiUrl: "https://api.example.com",
    timeout: 5000,
    retries: 3
};
console.table(config);
```

### Error Tracking

```javascript
try {
    riskyOperation();
} catch (error) {
    console.error("Operation failed:", error);
}
```

## Demo

### Command Line Demo

Run the console features demo without GUI:

```bash
go run examples/console_demo.go
```

This demonstrates:
- All console methods
- Console table formatting
- Message tracking and retrieval
- Error tracking
- Console clearing

### Interactive Browser Demo

1. Start the browser:
   ```bash
   go run ./cmd/browser
   ```

2. Open the demo page:
   ```
   file:///absolute/path/to/examples/console_demo.html
   ```

3. Click the console button (⊞) to show the console panel

4. Click the demo buttons to test various features:
   - Console message levels
   - Console tables with different data types
   - JavaScript error generation
   - Mixed message types

## Implementation Details

### Message Storage

Console messages are stored per-tab with:
- Level (log, info, warn, error, table)
- Message text
- Timestamp
- Raw data (for table display)

### Thread Safety

Console operations are thread-safe using mutex locks:
- `consoleMu` for console messages
- `jsErrorsMu` for JavaScript errors

### Performance

- Console messages are stored in memory
- No limit on message count (clear manually if needed)
- Efficient filtering using level-based iteration

## API Reference (Go)

### ConsoleMessage Type

```go
type ConsoleMessage struct {
    Level     string        // "log", "error", "warn", "info", "table"
    Message   string        // Formatted message
    Timestamp time.Time     // When the message was logged
    Data      interface{}   // Raw data for table display
}
```

### Runtime Methods

```go
// Get all console messages
messages := runtime.GetConsoleMessages()

// Clear console messages
runtime.ClearConsoleMessages()

// Get JavaScript errors
errors := runtime.GetJavaScriptErrors()

// Clear JavaScript errors
runtime.ClearJavaScriptErrors()
```

## Best Practices

1. **Use appropriate log levels**:
   - `log` for general information
   - `info` for important milestones
   - `warn` for potential issues
   - `error` for failures

2. **Use console.table() for structured data**:
   - Arrays of values
   - Object properties
   - Configuration dumps

3. **Clear console when needed**:
   - Between test runs
   - When switching contexts
   - To reduce memory usage

4. **Monitor error counter**:
   - Check for JavaScript errors
   - Debug issues before they accumulate

## Future Enhancements

Potential improvements:
- Console command input for executing JavaScript
- Message search/filtering
- Export console logs
- Performance timing (`console.time()`, `console.timeEnd()`)
- Stack trace display for errors
- Object inspection in console
- Console history persistence

## See Also

- [DOM API Documentation](../DOM_API_DOCUMENTATION.md)
- [Browser API Documentation](../BROWSER_API_DOCUMENTATION.md)
- [Roadmap](ROADMAP_V2.md)
