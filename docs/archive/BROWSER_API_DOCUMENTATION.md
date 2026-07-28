# Browser API Documentation

This document describes the enhanced browser APIs available in Goosie's JavaScript runtime. These APIs provide core browser functionality for URL manipulation, history management, timers, network requests, and data storage.

## Table of Contents

1. [window.location Object](#windowlocation-object)
2. [window.history API](#windowhistory-api)
3. [setTimeout() and setInterval()](#settimeout-and-setinterval)
4. [fetch() API](#fetch-api)
5. [localStorage API](#localstorage-api)
6. [sessionStorage API](#sessionstorage-api)
7. [Best Practices](#best-practices)
8. [Security Considerations](#security-considerations)

---

## window.location Object

The `window.location` object provides access to and manipulation of the current page URL.

### Properties

- **`href`** - Full URL of the current page
- **`protocol`** - Protocol scheme (e.g., "https:")
- **`host`** - Hostname and port
- **`hostname`** - Hostname only
- **`port`** - Port number
- **`pathname`** - Path portion of the URL
- **`search`** - Query string (including the '?')
- **`hash`** - Fragment identifier (including the '#')

### Methods

#### `setURL(url)`

Parses and sets a new URL, updating all location properties.

**Parameters:**
- `url` (string): The complete URL to parse

**Example:**
```javascript
// Set a new URL
window.location.setURL("https://example.com:8080/path?key=value#section");

// Access parsed components
console.log(window.location.protocol);  // "https:"
console.log(window.location.hostname);  // "example.com"
console.log(window.location.pathname);  // "/path"
console.log(window.location.search);    // "?key=value"
console.log(window.location.hash);      // "#section"
```

#### `getQueryParam(name)`

Retrieves a specific query parameter value from the URL.

**Parameters:**
- `name` (string): The parameter name to retrieve

**Returns:** The parameter value as a string, or `null` if not found

**Example:**
```javascript
window.location.setURL("https://example.com?name=John&age=30");

var name = window.location.getQueryParam("name");
console.log(name);  // "John"

var city = window.location.getQueryParam("city");
console.log(city);  // null (not found)
```

#### `setQueryParam(name, value)`

Sets or updates a query parameter in the URL.

**Parameters:**
- `name` (string): The parameter name
- `value` (string): The parameter value

**Returns:** The updated URL string

**Example:**
```javascript
window.location.setURL("https://example.com?page=1");

// Add a new parameter
window.location.setQueryParam("sort", "desc");
// URL is now: https://example.com?page=1&sort=desc

// Update existing parameter
window.location.setQueryParam("page", "2");
// URL is now: https://example.com?page=2&sort=desc
```

#### `reload()`

Triggers a page reload (logs the action in the current implementation).

**Example:**
```javascript
window.location.reload();
```

### Best Practices

- Always validate URLs before setting them
- Use `getQueryParam()` and `setQueryParam()` for query string manipulation instead of manual parsing
- Handle null returns from `getQueryParam()` gracefully

---

## window.history API

The `window.history` API enables interaction with the browser's session history, allowing navigation between pages and state management.

### Methods

#### `length()`

Returns the number of entries in the history stack.

**Returns:** Number of history entries

**Example:**
```javascript
var historyLength = window.history.length();
console.log("History has " + historyLength + " entries");
```

#### `back()`

Navigates to the previous page in history (equivalent to the browser's back button).

**Example:**
```javascript
window.history.back();
```

#### `forward()`

Navigates to the next page in history (equivalent to the browser's forward button).

**Example:**
```javascript
window.history.forward();
```

#### `go(delta)`

Navigates to a specific point in history relative to the current page.

**Parameters:**
- `delta` (number): Relative position (-1 for back, 1 for forward, etc.)

**Example:**
```javascript
// Go back 2 pages
window.history.go(-2);

// Go forward 1 page
window.history.go(1);

// Reload current page
window.history.go(0);
```

#### `pushState(state, title, url)`

Adds a new entry to the history stack without reloading the page.

**Parameters:**
- `state` (object): State object associated with the history entry
- `title` (string): Title for the history entry
- `url` (string): URL for the new history entry

**Example:**
```javascript
// Add a new history entry
window.history.pushState(
    { page: 1 },
    "Page 1",
    "/page1"
);

// Add another
window.history.pushState(
    { page: 2 },
    "Page 2",
    "/page2"
);

// Navigate back
window.history.back();
```

#### `replaceState(state, title, url)`

Replaces the current history entry without adding a new one.

**Parameters:**
- `state` (object): State object for the current entry
- `title` (string): Title for the history entry
- `url` (string): URL for the current entry

**Example:**
```javascript
// Replace current history entry
window.history.replaceState(
    { updated: true },
    "Updated Title",
    "/updated-url"
);
```

### Best Practices

- Use `pushState()` for single-page application navigation
- Use `replaceState()` to update the current URL without adding to history
- Track history index for custom navigation logic
- Handle edge cases when navigating beyond history bounds

---

## setTimeout() and setInterval()

Timer functions for scheduling code execution.

### setTimeout(callback, delay)

Executes a function once after a specified delay.

**Parameters:**
- `callback` (function): The function to execute
- `delay` (number): Delay in milliseconds

**Returns:** Timer ID (number) that can be used with `clearTimeout()`

**Example:**
```javascript
var timerId = setTimeout(function() {
    console.log("Executed after 1 second");
}, 1000);

// Cancel if needed
clearTimeout(timerId);
```

### clearTimeout(timerId)

Cancels a timeout created by `setTimeout()`.

**Parameters:**
- `timerId` (number): The timer ID returned by `setTimeout()`

**Example:**
```javascript
var timerId = setTimeout(function() {
    console.log("This won't execute");
}, 5000);

// Cancel the timeout
clearTimeout(timerId);
```

### setInterval(callback, interval)

Executes a function repeatedly at specified intervals.

**Parameters:**
- `callback` (function): The function to execute
- `interval` (number): Interval in milliseconds

**Returns:** Timer ID (number) that can be used with `clearInterval()`

**Example:**
```javascript
var counter = 0;
var intervalId = setInterval(function() {
    counter++;
    console.log("Counter: " + counter);
    
    if (counter >= 5) {
        clearInterval(intervalId);
    }
}, 1000);
```

### clearInterval(timerId)

Cancels an interval created by `setInterval()`.

**Parameters:**
- `timerId` (number): The timer ID returned by `setInterval()`

**Example:**
```javascript
var intervalId = setInterval(function() {
    console.log("Repeating...");
}, 1000);

// Stop after 5 seconds
setTimeout(function() {
    clearInterval(intervalId);
}, 5000);
```

### Best Practices

- Always store timer IDs if you need to cancel them later
- Clear timers when they're no longer needed to prevent memory leaks
- Use `clearTimeout()` and `clearInterval()` in cleanup code
- Be mindful of timer accuracy - delays are minimum, not exact
- Avoid very short intervals (< 10ms) as they may impact performance

### Timer Management

The runtime provides automatic cleanup through the `Cleanup()` method:

```javascript
// Runtime cleanup (typically called when closing the application)
runtime.Cleanup();  // Stops all active timers
```

---

## fetch() API

The `fetch()` API provides an interface for making HTTP requests.

### fetch(url, options)

Initiates a network request to retrieve or send data.

**Parameters:**
- `url` (string): The URL to fetch
- `options` (object): Optional request configuration (reserved for future use)

**Returns:** A Promise-like object that resolves with a Response

**Example:**
```javascript
// Basic GET request
fetch("https://api.example.com/data")
    .then(function(response) {
        console.log("Status:", response.status);
        console.log("OK:", response.ok);
        return response.json();
    })
    .then(function(data) {
        console.log("Data:", data);
    })
    .catch(function(error) {
        console.log("Error:", error.message);
    });
```

### Response Object

The Response object returned by `fetch()` contains:

**Properties:**
- `ok` (boolean): True if status is 200-299
- `status` (number): HTTP status code
- `statusText` (string): Status message
- `url` (string): The requested URL

**Methods:**
- `json()`: Returns a promise that resolves with JSON data
- `text()`: Returns a promise that resolves with text data

**Example:**
```javascript
// Fetch and parse JSON
fetch("https://api.example.com/users/1")
    .then(function(response) {
        if (!response.ok) {
            throw new Error("HTTP error " + response.status);
        }
        return response.json();
    })
    .then(function(user) {
        console.log("User:", user.name);
    });

// Fetch text content
fetch("https://example.com/document.txt")
    .then(function(response) {
        return response.text();
    })
    .then(function(text) {
        console.log("Content:", text);
    });
```

### Error Handling

Always include error handling with `.catch()`:

```javascript
fetch("https://api.example.com/data")
    .then(function(response) {
        if (!response.ok) {
            throw new Error("Request failed: " + response.statusText);
        }
        return response.json();
    })
    .then(function(data) {
        // Process data
    })
    .catch(function(error) {
        console.log("Error occurred:", error.message);
        // Handle error appropriately
    });
```

### Best Practices

- Always check `response.ok` before processing data
- Include `.catch()` for error handling
- Validate data after receiving it
- Set appropriate timeouts for requests (future enhancement)
- Handle network failures gracefully

### Future Enhancements

The fetch API implementation is designed to support:
- Request/response interceptors
- Retry logic for failed requests
- Request cancellation
- Custom headers and request methods (POST, PUT, DELETE)
- Request timeout configuration

---

## localStorage API

The `localStorage` API provides persistent key-value storage that survives browser sessions.

### Methods

#### `setItem(key, value)`

Stores a key-value pair in localStorage with validation and versioning.

**Parameters:**
- `key` (string): Storage key (cannot be empty)
- `value` (string): Value to store

**Example:**
```javascript
localStorage.setItem("username", "JohnDoe");
localStorage.setItem("preferences", JSON.stringify({
    theme: "dark",
    language: "en"
}));
```

#### `getItem(key)`

Retrieves a value from localStorage.

**Parameters:**
- `key` (string): Storage key

**Returns:** The stored value as a string, or `null` if not found

**Example:**
```javascript
var username = localStorage.getItem("username");
console.log(username);  // "JohnDoe"

var prefs = localStorage.getItem("preferences");
var prefsObj = JSON.parse(prefs);
console.log(prefsObj.theme);  // "dark"
```

#### `removeItem(key)`

Removes a specific item from localStorage.

**Parameters:**
- `key` (string): Key to remove

**Example:**
```javascript
localStorage.removeItem("username");
```

#### `clear()`

Removes all items from localStorage.

**Example:**
```javascript
localStorage.clear();
```

#### `key(index)`

Returns the key name at the specified index.

**Parameters:**
- `index` (number): Index position

**Returns:** Key name or `null` if index is out of bounds

**Example:**
```javascript
localStorage.setItem("key1", "value1");
localStorage.setItem("key2", "value2");

var firstKey = localStorage.key(0);
console.log(firstKey);  // "key1" or "key2" (order not guaranteed)
```

#### `length()`

Returns the number of items stored.

**Returns:** Number of items in localStorage

**Example:**
```javascript
localStorage.setItem("a", "1");
localStorage.setItem("b", "2");
var count = localStorage.length();
console.log(count);  // 2
```

### Data Versioning

The implementation includes automatic versioning for stored values:

```javascript
// Values are stored with version prefix
localStorage.setItem("data", "myValue");
// Internally stored as: "v1:myValue"

// Retrieval automatically handles versioning
var data = localStorage.getItem("data");
console.log(data);  // "myValue" (version prefix removed)
```

### Best Practices

- Validate data before storing
- Use JSON.stringify() for objects
- Use JSON.parse() when retrieving objects
- Handle potential null returns
- Clear sensitive data when no longer needed
- Implement data migration for version changes

### Example: Complete Storage Pattern

```javascript
// Storage utility functions
var StorageUtil = {
    save: function(key, value) {
        try {
            var json = JSON.stringify(value);
            localStorage.setItem(key, json);
            return true;
        } catch (e) {
            console.log("Storage error:", e);
            return false;
        }
    },
    
    load: function(key, defaultValue) {
        try {
            var item = localStorage.getItem(key);
            if (item === null) {
                return defaultValue;
            }
            return JSON.parse(item);
        } catch (e) {
            console.log("Parse error:", e);
            return defaultValue;
        }
    },
    
    remove: function(key) {
        localStorage.removeItem(key);
    }
};

// Usage
StorageUtil.save("user", {
    id: 123,
    name: "John",
    preferences: { theme: "dark" }
});

var user = StorageUtil.load("user", {});
console.log(user.name);  // "John"
```

---

## sessionStorage API

The `sessionStorage` API provides temporary key-value storage that lasts only for the browser session.

### Methods

The `sessionStorage` API has the same interface as `localStorage`:

- `setItem(key, value)` - Store a value
- `getItem(key)` - Retrieve a value
- `removeItem(key)` - Remove a specific item
- `clear()` - Remove all items
- `key(index)` - Get key at index
- `length()` - Get number of items

### Session Schema

Values stored in sessionStorage include a schema prefix for validation:

```javascript
sessionStorage.setItem("tempData", "value");
// Internally stored as: "session:value"
```

### Differences from localStorage

| Feature | localStorage | sessionStorage |
|---------|-------------|----------------|
| **Persistence** | Survives browser restart | Cleared when session ends |
| **Scope** | Shared across tabs/windows | Tab/window specific |
| **Use Case** | Long-term data | Temporary session data |

### Example

```javascript
// Store session-specific data
sessionStorage.setItem("currentPage", "2");
sessionStorage.setItem("sortOrder", "desc");
sessionStorage.setItem("filters", JSON.stringify({
    category: "electronics",
    priceRange: "100-500"
}));

// Retrieve session data
var page = sessionStorage.getItem("currentPage");
var filters = JSON.parse(sessionStorage.getItem("filters"));

// Clear session data when done
sessionStorage.clear();
```

### Best Practices

- Use for temporary UI state
- Store shopping cart data
- Keep form data during multi-step processes
- Store authentication tokens (with caution)
- Clear sensitive data explicitly

---

## Best Practices

### General Guidelines

1. **Error Handling**: Always handle potential errors and null values
2. **Data Validation**: Validate data before storing or processing
3. **Resource Cleanup**: Clear timers and storage when no longer needed
4. **Performance**: Be mindful of storage size and timer frequency
5. **Security**: Never store sensitive data in plain text

### Example: Complete Application Pattern

```javascript
// Initialize application
var App = {
    init: function() {
        // Load stored preferences
        var prefs = this.loadPreferences();
        this.applyPreferences(prefs);
        
        // Setup history tracking
        this.setupHistoryTracking();
        
        // Start periodic tasks
        this.startPeriodicTasks();
    },
    
    loadPreferences: function() {
        var prefsJson = localStorage.getItem("preferences");
        if (prefsJson) {
            try {
                return JSON.parse(prefsJson);
            } catch (e) {
                console.log("Failed to parse preferences:", e);
            }
        }
        return { theme: "light", notifications: true };
    },
    
    applyPreferences: function(prefs) {
        console.log("Applying theme:", prefs.theme);
        // Apply preferences to UI
    },
    
    setupHistoryTracking: function() {
        // Track navigation
        var self = this;
        window.addEventListener("navigate", function(event) {
            window.history.pushState(
                { timestamp: Date.now() },
                event.title,
                event.url
            );
        });
    },
    
    startPeriodicTasks: function() {
        // Auto-save every 30 seconds
        this.autoSaveTimer = setInterval(function() {
            App.autoSave();
        }, 30000);
        
        // Cleanup old session data
        this.cleanupTimer = setInterval(function() {
            App.cleanupOldData();
        }, 300000);  // Every 5 minutes
    },
    
    autoSave: function() {
        var data = sessionStorage.getItem("draft");
        if (data) {
            console.log("Auto-saving draft...");
            localStorage.setItem("lastDraft", data);
        }
    },
    
    cleanupOldData: function() {
        // Remove old temporary data
        console.log("Cleaning up old data...");
    },
    
    cleanup: function() {
        // Clear timers
        if (this.autoSaveTimer) {
            clearInterval(this.autoSaveTimer);
        }
        if (this.cleanupTimer) {
            clearInterval(this.cleanupTimer);
        }
        
        // Clear session storage
        sessionStorage.clear();
    }
};

// Initialize the application
App.init();
```

---

## Security Considerations

### localStorage and sessionStorage

**Risks:**
- Data is stored in plain text
- Accessible to any JavaScript on the page
- Vulnerable to XSS attacks

**Recommendations:**
1. **Never store sensitive data** (passwords, tokens, personal info) in plain text
2. **Validate and sanitize** all data before storing
3. **Implement encryption** for sensitive data (future enhancement)
4. **Clear sensitive data** after use
5. **Use HTTPS** to prevent network interception

**Example: Safe Storage Pattern**

```javascript
// DO: Store non-sensitive preferences
localStorage.setItem("theme", "dark");
localStorage.setItem("language", "en");

// DON'T: Store sensitive data
// localStorage.setItem("password", "secret123");  // NEVER DO THIS
// localStorage.setItem("creditCard", "1234-5678");  // NEVER DO THIS

// DO: Clear sensitive session data
function logout() {
    sessionStorage.clear();
    // Clear any sensitive localStorage items
    localStorage.removeItem("authToken");
}
```

### fetch() API

**Risks:**
- CORS issues with cross-origin requests
- Potential for XSS if response data is not validated
- Man-in-the-middle attacks

**Recommendations:**
1. **Use HTTPS** for all requests
2. **Validate response data** before using it
3. **Handle errors** gracefully
4. **Implement timeouts** to prevent hanging requests
5. **Be cautious with credentials** and authentication

**Example: Safe Fetch Pattern**

```javascript
function fetchUserData(userId) {
    // Validate input
    if (!userId || isNaN(userId)) {
        console.log("Invalid user ID");
        return;
    }
    
    // Use HTTPS
    var url = "https://api.example.com/users/" + userId;
    
    fetch(url)
        .then(function(response) {
            // Check response status
            if (!response.ok) {
                throw new Error("HTTP " + response.status);
            }
            return response.json();
        })
        .then(function(data) {
            // Validate response data
            if (!data || !data.id) {
                throw new Error("Invalid response data");
            }
            
            // Use data safely
            console.log("User:", data.name);
        })
        .catch(function(error) {
            // Handle errors appropriately
            console.log("Failed to fetch user:", error.message);
            // Show user-friendly error message
        });
}
```

### Timer Functions

**Risks:**
- Memory leaks from uncancelled timers
- Performance degradation from too many timers

**Recommendations:**
1. **Always clear timers** when no longer needed
2. **Track timer IDs** for cleanup
3. **Avoid very short intervals** (< 10ms)
4. **Implement cleanup functions** for component unmounting

**Example: Safe Timer Pattern**

```javascript
var TimerManager = {
    timers: [],
    
    setTimeout: function(callback, delay) {
        var id = setTimeout(callback, delay);
        this.timers.push({ id: id, type: "timeout" });
        return id;
    },
    
    setInterval: function(callback, interval) {
        var id = setInterval(callback, interval);
        this.timers.push({ id: id, type: "interval" });
        return id;
    },
    
    clearAll: function() {
        for (var i = 0; i < this.timers.length; i++) {
            var timer = this.timers[i];
            if (timer.type === "timeout") {
                clearTimeout(timer.id);
            } else {
                clearInterval(timer.id);
            }
        }
        this.timers = [];
    }
};

// Usage
TimerManager.setTimeout(function() {
    console.log("Delayed execution");
}, 1000);

// Cleanup when done
TimerManager.clearAll();
```

---

## Summary

These browser APIs provide essential functionality for building web applications in Goosie:

- **window.location**: URL manipulation and query parameter handling
- **window.history**: Browser history management and navigation
- **setTimeout/setInterval**: Task scheduling and timing
- **fetch()**: HTTP requests for data retrieval
- **localStorage**: Persistent key-value storage
- **sessionStorage**: Temporary session storage

All APIs include:
- ✅ Error handling and validation
- ✅ Best practices and usage examples
- ✅ Security considerations
- ✅ Comprehensive test coverage
- ✅ Memory management and cleanup

For more information on DOM APIs, see [DOM_API_DOCUMENTATION.md](DOM_API_DOCUMENTATION.md).
