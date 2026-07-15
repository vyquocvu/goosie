# Goosie Element Inspector

The Goosie Element Inspector is a comprehensive developer tool for debugging and modifying the DOM and styles of web pages rendered by the Goosie browser engine.

## Features

- **DOM Tree View**: hierarchical view of the DOM structure.
- **Element Selection**: Select elements by clicking in the tree, by clicking on the rendered page (Inspector mode), or via the right-click context menu ("Inspect Element").
- **Properties Editor**: View and edit tag names, IDs, classes, text content, and attributes.
- **Computed Styles**: View and edit computed CSS styles (e.g., color, font-size, dimensions).
- **Box Model Visualization**: Inspect layout metrics (position, size, margins, padding).
- **Search**: Find elements by tag name, ID (`#id`), or class (`.class`).
- **Performance Metrics**: View total node count and other metrics.
- **Right-Click Context Menu**: Right-click anywhere on a rendered page to get a developer-tools context menu with quick actions.
- **Live Editing**: Changes to properties and styles are reflected immediately in the renderer.

## Usage

1.  **Open Inspector**: Click the "🔍" (Search/Inspect) button in the browser toolbar.
2.  **Select Element**:
    -   Right-click an element and choose **Inspect Element**.
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

## Right-click context menu

Right-clicking anywhere on the rendered page opens a dev-tools context menu anchored at the cursor. The menu offers:

| Action | Description |
|--------|-------------|
| **Inspect Element** | Reveals the inspector panel (if hidden) and selects the element under the cursor. |
| **View Source** | Opens a read-only dialog containing the outer HTML of the right-clicked element. |
| **View Computed Style** | Opens a read-only dialog summarising the computed style of the element. |
| **Copy Selector** | Copies a CSS selector for the element (uses `#id` if available, otherwise the tag name) to the clipboard. |
| **Copy Outer HTML** | Copies the outer HTML of the element to the clipboard. |
| **Copy Inner HTML** | Copies the inner HTML of the element to the clipboard. |
| **Copy Inner Text** | Copies the plain-text content of the element to the clipboard. |

The context menu is implemented in `internal/ui/dev_tools_context_menu.go`. The right-click is detected by `InspectableContainer.TappedSecondary` in `internal/renderer/canvas.go`, which performs a hit test and forwards the result to the browser layer via the `SetContextMenuCallback` wiring on the `HTMLRenderer` interface.

## Development

The inspector is implemented in `internal/ui/inspect_panel.go`. It uses Fyne widgets for the UI and interacts with the `renderer` package via the `HTMLRenderer` interface.

### Key Components

-   `InspectPanel`: Main struct managing the UI and state.
-   `DevToolsContextMenu`: Builds and shows the right-click dev-tools popup. See `internal/ui/dev_tools_context_menu.go`.
-   `HTMLRenderer`: Interface for interacting with the browser's rendering engine.
-   `renderer.RenderNode`: Represents a DOM node.
-   `renderer.LayoutBox`: Represents the computed layout of a node.

### Architecture

The inspector follows a reactive pattern:
1.  User selects a node (via tree, hit-test, or context menu).
2.  `SetElement` is called, updating `selectedNode`.
3.  `updateDetails` refreshes the detail tabs based on the selected node.
4.  User edits a value (e.g., changes text).
5.  The `RenderNode` is updated directly.
6.  `renderer.Refresh()` is called, triggering a re-layout and repaint of the web page.

The right-click context menu follows the same reactive pattern:
1.  User right-clicks on the rendered page.
2.  `InspectableContainer.TappedSecondary` fires, performing a hit test.
3.  The hit-test result is forwarded to the browser layer.
4.  `DevToolsContextMenu.Show` rebuilds and pops the menu at the cursor.
5.  When the user picks an action, the matching callback runs on the UI goroutine.
