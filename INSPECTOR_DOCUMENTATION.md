# Goosie Element Inspector

The Goosie Element Inspector is a comprehensive developer tool for debugging and modifying the DOM and styles of web pages rendered by the Goosie browser engine.

## Features

- **DOM Tree View**: hierarchical view of the DOM structure.
- **Element Selection**: Select elements by clicking in the tree or on the rendered page (Inspector mode).
- **Properties Editor**: View and edit tag names, IDs, classes, text content, and attributes.
- **Computed Styles**: View and edit computed CSS styles (e.g., color, font-size, dimensions).
- **Box Model Visualization**: Inspect layout metrics (position, size, margins, padding).
- **Search**: Find elements by tag name, ID (`#id`), or class (`.class`).
- **Performance Metrics**: View total node count and other metrics.
- **Live Editing**: Changes to properties and styles are reflected immediately in the renderer.

## Usage

1.  **Open Inspector**: Click the "🔍" (Search/Inspect) button in the browser toolbar.
2.  **Select Element**:
    -   Click on an element in the web page view.
    -   Or, browse the DOM tree on the left side of the inspector panel.
3.  **Inspect Details**: Use the tabs on the right side:
    -   **Properties**: Edit HTML attributes and text.
    -   **Styles**: Edit CSS properties.
    -   **Layout**: View box model dimensions.
    -   **Performance**: View stats.
4.  **Search**: Enter a query in the search bar above the tree:
    -   `div`: Find tags named "div".
    -   `#main`: Find element with ID "main".
    -   `.button`: Find elements with class "button".
    -   Click "Find" or press Enter.

## Development

The inspector is implemented in `internal/ui/inspect_panel.go`. It uses Fyne widgets for the UI and interacts with the `renderer` package via the `HTMLRenderer` interface.

### Key Components

-   `InspectPanel`: Main struct managing the UI and state.
-   `HTMLRenderer`: Interface for interacting with the browser's rendering engine.
-   `renderer.RenderNode`: Represents a DOM node.
-   `renderer.LayoutBox`: Represents the computed layout of a node.

### Architecture

The inspector follows a reactive pattern:
1.  User selects a node (via tree or hit-test).
2.  `SetElement` is called, updating `selectedNode`.
3.  `updateDetails` refreshes the detail tabs based on the selected node.
4.  User edits a value (e.g., changes text).
5.  The `RenderNode` is updated directly.
6.  `renderer.Refresh()` is called, triggering a re-layout and repaint of the web page.

