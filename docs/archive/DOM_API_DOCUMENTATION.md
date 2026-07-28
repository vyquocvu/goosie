# DOM API Extensions Documentation

This document describes the comprehensive DOM API extensions added to Goosie's JavaScript runtime.

## Overview

Goosie now supports a wide range of standard DOM APIs that enable dynamic manipulation and querying of HTML elements, mirroring browser capabilities. These APIs allow developers to write familiar JavaScript code that works within the Goosie environment.

## Query Methods

### document.getElementById(id)

Retrieves an element by its unique ID attribute.

**Parameters:**
- `id` (string): The ID of the element to find

**Returns:** Element object or `null` if not found

**Example:**
```javascript
var elem = document.getElementById("main-content");
if (elem) {
    console.log(elem.textContent);
}
```

### document.getElementsByClassName(className)

Returns all elements with the specified class name.

**Parameters:**
- `className` (string): The class name to search for

**Returns:** Array of element objects

**Example:**
```javascript
var items = document.getElementsByClassName("menu-item");
console.log("Found " + items.length + " menu items");
for (var i = 0; i < items.length; i++) {
    console.log(items[i].textContent);
}
```

### document.getElementsByTagName(tagName)

Returns all elements with the specified tag name.

**Parameters:**
- `tagName` (string): The HTML tag name (e.g., "div", "p", "span")

**Returns:** Array of element objects

**Example:**
```javascript
var paragraphs = document.getElementsByTagName("p");
console.log("Found " + paragraphs.length + " paragraphs");
```

### document.querySelector(selector)

Returns the first element that matches the specified CSS selector.

**Parameters:**
- `selector` (string): CSS selector string

**Supported Selectors:**
- ID selector: `#elementId`
- Class selector: `.className`
- Tag selector: `tagName`
- Attribute selector: `[attribute=value]`

**Returns:** Element object or `null` if not found

**Example:**
```javascript
// ID selector
var header = document.querySelector("#page-header");

// Class selector
var firstItem = document.querySelector(".list-item");

// Tag selector
var firstParagraph = document.querySelector("p");

// Attribute selector
var dataElement = document.querySelector("[data-type=special]");
```

### document.querySelectorAll(selector)

Returns all elements that match the specified CSS selector.

**Parameters:**
- `selector` (string): CSS selector string

**Returns:** Array of element objects

**Example:**
```javascript
// Get all elements with a specific class
var items = document.querySelectorAll(".list-item");

// Get all divs
var divs = document.querySelectorAll("div");

// Iterate through results
for (var i = 0; i < items.length; i++) {
    console.log(items[i].textContent);
}
```

## Element Creation

### document.createElement(tagName)

Creates a new HTML element with the specified tag name.

**Parameters:**
- `tagName` (string): The type of element to create (e.g., "div", "span", "p")

**Returns:** New element object

**Example:**
```javascript
var newDiv = document.createElement("div");
newDiv.textContent = "Hello, World!";

var newParagraph = document.createElement("p");
newParagraph.textContent = "This is a new paragraph";
```

## DOM Manipulation Methods

All element objects support the following manipulation methods:

### element.appendChild(child)

Appends a child element to the end of the element's children list.

**Parameters:**
- `child` (Element): The element to append

**Returns:** The appended child element

**Example:**
```javascript
var parent = document.getElementById("container");
var child = document.createElement("div");
child.textContent = "New child element";
parent.appendChild(child);
```

### element.removeChild(child)

Removes a child element from the element.

**Parameters:**
- `child` (Element): The child element to remove

**Returns:** The removed child element

**Example:**
```javascript
var parent = document.getElementById("container");
var child = document.querySelector(".unwanted-item");
if (child) {
    parent.removeChild(child);
}
```

### element.replaceChild(newChild, oldChild)

Replaces an existing child element with a new one.

**Parameters:**
- `newChild` (Element): The new element to insert
- `oldChild` (Element): The existing element to replace

**Returns:** The replaced (old) child element

**Example:**
```javascript
var parent = document.getElementById("container");
var oldElement = document.querySelector(".old-item");
var newElement = document.createElement("div");
newElement.textContent = "Replacement content";
parent.replaceChild(newElement, oldElement);
```

### element.insertBefore(newChild, referenceChild)

Inserts a new child element before an existing reference child.

**Parameters:**
- `newChild` (Element): The new element to insert
- `referenceChild` (Element): The existing child before which to insert

**Returns:** The inserted child element

**Example:**
```javascript
var parent = document.getElementById("list");
var newItem = document.createElement("li");
newItem.textContent = "New list item";
var referenceItem = document.querySelector(".reference-item");
parent.insertBefore(newItem, referenceItem);
```

## Event Handling

### element.addEventListener(eventType, callback)

Adds an event listener to the element.

**Parameters:**
- `eventType` (string): The type of event (e.g., "click", "change", "submit")
- `callback` (function): The function to call when the event occurs

**Returns:** `undefined`

**Example:**
```javascript
var button = document.getElementById("submit-btn");
button.addEventListener("click", function() {
    console.log("Button clicked!");
});

var input = document.getElementById("username");
input.addEventListener("change", function() {
    console.log("Input value changed");
});
```

### element.removeEventListener(eventType, callback)

Removes an event listener from the element.

**Parameters:**
- `eventType` (string): The type of event
- `callback` (function): The callback function to remove

**Returns:** `undefined`

**Note:** This implementation removes all listeners for the specified event type. For more precise control, store a reference to your callback function.

**Example:**
```javascript
var button = document.getElementById("submit-btn");

function handleClick() {
    console.log("Clicked!");
}

button.addEventListener("click", handleClick);
// Later...
button.removeEventListener("click", handleClick);
```

## Element Properties

All element objects have the following properties:

### element.textContent

Gets or sets the text content of the element and its descendants.

**Type:** string

**Example:**
```javascript
var elem = document.getElementById("message");
console.log(elem.textContent); // Read text content
elem.textContent = "New message"; // Set text content
```

### element.tagName

Gets the tag name of the element (e.g., "DIV", "P", "SPAN").

**Type:** string (read-only)

**Example:**
```javascript
var elem = document.querySelector(".my-element");
console.log(elem.tagName); // "div", "p", etc.
```

### element.id

Gets the ID attribute of the element.

**Type:** string (read-only)

**Example:**
```javascript
var elem = document.querySelector("#main-content");
console.log(elem.id); // "main-content"
```

### element.classList

Gets an array of class names applied to the element.

**Type:** array of strings (read-only)

**Example:**
```javascript
var elem = document.querySelector(".my-element");
console.log(elem.classList); // ["class1", "class2", "class3"]
```

### element.attributes

Gets an object containing all attributes of the element.

**Type:** object (read-only)

**Example:**
```javascript
var elem = document.querySelector("[data-type=special]");
console.log(elem.attributes); // { "data-type": "special", "class": "item", ... }
```

### element.children

Gets an array of child elements.

**Type:** array of elements

**Example:**
```javascript
var parent = document.getElementById("container");
console.log("Number of children: " + parent.children.length);
for (var i = 0; i < parent.children.length; i++) {
    console.log(parent.children[i].tagName);
}
```

## Complete Examples

### Example 1: Building a Dynamic List

```javascript
// Create a list container
var list = document.createElement("ul");
list.id = "dynamic-list";

// Add items to the list
var items = ["Apple", "Banana", "Cherry", "Date"];
for (var i = 0; i < items.length; i++) {
    var listItem = document.createElement("li");
    listItem.textContent = items[i];
    listItem.classList = ["fruit-item"];
    list.appendChild(listItem);
}

// Add the list to the page
var container = document.getElementById("main-content");
container.appendChild(list);

console.log("Created list with " + list.children.length + " items");
```

### Example 2: Querying and Manipulating Elements

```javascript
// Find all elements with a specific class
var cards = document.querySelectorAll(".card");
console.log("Found " + cards.length + " cards");

// Update each card
for (var i = 0; i < cards.length; i++) {
    var card = cards[i];
    console.log("Card " + (i + 1) + ": " + card.textContent);
    
    // Add a timestamp
    var timestamp = document.createElement("span");
    timestamp.textContent = " [Updated]";
    card.appendChild(timestamp);
}
```

### Example 3: Filtering and Removing Elements

```javascript
// Get all paragraphs
var paragraphs = document.getElementsByTagName("p");
console.log("Total paragraphs: " + paragraphs.length);

// Find paragraphs with specific class
var outdatedItems = document.querySelectorAll(".outdated");
console.log("Found " + outdatedItems.length + " outdated items");

// Remove outdated items
var parent = document.getElementById("content");
for (var i = 0; i < outdatedItems.length; i++) {
    parent.removeChild(outdatedItems[i]);
}
console.log("Removed outdated items");
```

### Example 4: Event Handling

```javascript
// Set up event listeners on buttons
var buttons = document.querySelectorAll("button");
for (var i = 0; i < buttons.length; i++) {
    buttons[i].addEventListener("click", function() {
        console.log("Button clicked: " + this.textContent);
    });
}

// Set up form submission
var form = document.querySelector("#contact-form");
if (form) {
    form.addEventListener("submit", function() {
        console.log("Form submitted");
        // Handle form submission
    });
}
```

### Example 5: Replacing Content

```javascript
// Find the old content
var oldContent = document.querySelector(".legacy-content");

if (oldContent) {
    // Create new content
    var newContent = document.createElement("div");
    newContent.classList = ["modern-content"];
    newContent.textContent = "Updated content with modern styling";
    
    // Replace old with new
    var parent = oldContent.parentNode;
    if (parent) {
        parent.replaceChild(newContent, oldContent);
        console.log("Content updated successfully");
    }
}
```

## CSS Selector Support

The `querySelector` and `querySelectorAll` methods support the following CSS selectors:

| Selector Type | Syntax | Example | Description |
|--------------|--------|---------|-------------|
| ID | `#id` | `#main-header` | Selects element with id="main-header" |
| Class | `.class` | `.menu-item` | Selects elements with class="menu-item" |
| Tag | `tag` | `div` | Selects all `<div>` elements |
| Attribute | `[attr=value]` | `[data-type=special]` | Selects elements with matching attribute |

**Note:** The current implementation supports basic selectors. Complex selectors (e.g., descendant selectors, pseudo-classes) may be added in future versions.

## Performance Considerations

- **Query Operations**: Query methods traverse the DOM tree. For frequently accessed elements, consider caching the results.
- **Manipulation Operations**: DOM manipulations update the internal structure. Batch multiple changes when possible.
- **Event Listeners**: Store references to callback functions if you need to remove them later.

## Best Practices

1. **Check for null**: Always check if query methods return null before accessing properties
   ```javascript
   var elem = document.getElementById("optional-element");
   if (elem) {
       console.log(elem.textContent);
   }
   ```

2. **Use specific selectors**: More specific selectors are generally more efficient
   ```javascript
   // Good: Specific selector
   var elem = document.getElementById("unique-id");
   
   // Less efficient: Generic selector
   var elems = document.querySelectorAll("div");
   ```

3. **Cache DOM queries**: If you access the same element multiple times
   ```javascript
   // Cache the element
   var container = document.getElementById("container");
   
   // Use the cached reference
   container.appendChild(child1);
   container.appendChild(child2);
   container.appendChild(child3);
   ```

4. **Use appropriate query methods**:
   - Use `getElementById` when you have an ID (fastest)
   - Use `getElementsByClassName` or `getElementsByTagName` for simple queries
   - Use `querySelector` or `querySelectorAll` for complex selectors

## Migration from Browser Code

Code written for standard browsers should work with minimal changes in Goosie:

```javascript
// Standard browser code
var elem = document.getElementById("my-element");
elem.textContent = "Hello, World!";

var items = document.querySelectorAll(".item");
for (var i = 0; i < items.length; i++) {
    console.log(items[i].textContent);
}

// This same code works in Goosie!
```

## Limitations

Current limitations to be aware of:

1. **CSS Selector Support**: Only basic selectors are supported. Complex selectors like `:hover`, `>`, `+`, etc. are not yet implemented.
2. **Event Bubbling**: Event propagation and bubbling are not yet fully implemented.
3. **Synchronous Operations**: All DOM operations are synchronous. Async operations may be added in future versions.

## Future Enhancements

Planned additions for future versions:

- More complex CSS selector support (descendant selectors, pseudo-classes)
- Full event propagation and bubbling
- More element properties (innerHTML, outerHTML, etc.)
- Form manipulation APIs
- Animation and transition support

## Support

For questions, issues, or feature requests, please refer to the main Goosie documentation or open an issue on the GitHub repository.
