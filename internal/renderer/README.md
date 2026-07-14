# HTML Renderer Module

The HTML renderer module provides rendering capabilities for the Goosie project. It parses HTML content, builds a render tree, performs layout calculations, and produces pixel output via Go's own raster backends (CPU or CoreGraphics) — not via platform WebViews (WKWebView, WebView2) or Fyne canvas primitives. Fyne is used only as the window shell that presents the final pixel buffer.

**Render model:** Pure Go CPU raster backend (`internal/renderer/frame/raster`). No platform WebViews. Core engine packages never import Fyne types.

## Architecture

The renderer module consists of four main components:

### 1. Render Tree (`node.go`)

The render tree is a simplified representation of the HTML DOM optimized for rendering.

#### Key Components:

- **RenderNode**: Represents a node in the render tree
  - `NodeType`: Either `NodeTypeElement` or `NodeTypeText`
  - `TagName`: HTML tag name (e.g., "div", "p", "h1")
  - `Text`: Text content for text nodes
  - `Attrs`: HTML attributes as a key-value map
  - `Children`: Child nodes
  - `Box`: Layout properties (position, size)

#### Features:

- **Node Management**: Add/remove children, set/get attributes
- **Tree Building**: `BuildRenderTree()` converts HTML nodes to render nodes
- **Block Detection**: `IsBlock()` determines if an element is block-level

### 2. Layout Engine (`layout.go`)

The layout engine calculates the position and size of each element in the render tree.

#### Key Components:

- **LayoutEngine**: Handles layout calculations
  - Configurable canvas dimensions
  - Default font sizes and line heights
  - Vertical spacing for elements
  - **InlineLayoutEngine**: Dedicated inline layout handling

#### Layout Algorithm:

1. **Top-Down Traversal**: Starts from the root and processes children
2. **Block Layout**: Block elements stack vertically
3. **Inline Layout**: True inline layout with line boxes (see `inline_layout.go`)
   - Proper word wrapping and line breaking
   - White space handling (all CSS modes)
   - Vertical alignment support
   - Character-level breaking for long words
4. **Text Layout**: Accurate text measurement using font metrics
5. **Spacing**: Applies element-specific vertical spacing

#### Supported Layout Rules:

- Heading font sizes (h1-h6)
- Paragraph spacing
- List spacing
- Block vs. inline element flow
- **Line box model for inline content**
- **All CSS white-space modes**
- **All CSS vertical-align modes**

For detailed information about inline layout, see [INLINE_LAYOUT_IMPLEMENTATION.md](../../INLINE_LAYOUT_IMPLEMENTATION.md).

### 3. Canvas Renderer (`canvas.go`)

The canvas renderer converts the laid-out render tree into Fyne canvas objects.

#### Key Components:

- **CanvasRenderer**: Renders nodes to Fyne widgets
  - Converts render nodes to visual elements
  - Applies text styles (bold, italic)
  - Handles different element types

#### Supported Elements:

- **Headings** (h1-h6): Bold text with visual distinction
- **Paragraphs** (p): Standard text with word wrapping
- **Divs** (div): Container elements
- **Links** (a): Hyperlink widgets (href support)
- **Lists** (ul, ol): Unordered and ordered lists
- **List Items** (li): Bullet points with indentation
- **Images** (img): Placeholder rendering with alt text
- **Inline Elements** (span, strong, em, b, i): Styled text
- **Code** (code): Inline code with monospace font
- **Preformatted Text** (pre): Preformatted text blocks with monospace font and preserved whitespace
- **Blockquotes** (blockquote): Quote blocks with italic styling
- **Line Breaks** (br): Spacing elements

### 4. Main Renderer (`renderer.go`)

The main renderer coordinates all components to provide a simple API.

#### Key Methods:

- `NewRenderer(width, height)`: Creates a new renderer with specified dimensions
- `RenderHTML(htmlContent)`: Renders complete HTML documents
- `RenderHTMLBody(htmlContent)`: Renders just the body content
- `SetSize(width, height)`: Updates renderer dimensions

#### Rendering Pipeline:

1. **Parse**: HTML string → HTML nodes (using `golang.org/x/net/html`)
2. **Build**: HTML nodes → Render tree
3. **Layout**: Calculate positions and sizes
4. **Render**: Render tree → Fyne canvas objects

## Usage

### Basic Example

```go
import "github.com/vyquocvu/goosie/internal/renderer"

// Create renderer with canvas size
htmlRenderer := renderer.NewRenderer(800, 600)

// Render HTML content
canvasObject, err := htmlRenderer.RenderHTML(htmlContent)
if err != nil {
    log.Fatal(err)
}

// Use the canvas object in your Fyne UI
container.Add(canvasObject)
```

### Integration with Browser UI

```go
// In Browser initialization
browser := &Browser{
    htmlRenderer: renderer.NewRenderer(1000, 700),
    // ... other fields
}

// Render HTML content
func (b *Browser) RenderHTMLContent(htmlContent string) error {
    canvasObject, err := b.htmlRenderer.RenderHTML(htmlContent)
    if err != nil {
        return err
    }
    
    b.contentScroll.Content = canvasObject
    b.contentScroll.Refresh()
    
    return nil
}
```

### Updating Canvas Size

```go
// When window is resized
renderer.SetSize(newWidth, newHeight)
```

## HTML Support

### Fully Supported Elements

- **Document Structure**: `html`, `head`, `body`
- **Headings**: `h1`, `h2`, `h3`, `h4`, `h5`, `h6`
- **Text Blocks**: `p`, `div`, `span`
- **Lists**: `ul`, `ol`, `li`
- **Links**: `a` (with href attribute)
- **Images**: `img` (with image loading support)
- **Text Styling**: `strong`, `b`, `em`, `i`
- **Code**: `code` (inline code with monospace font)
- **Preformatted Text**: `pre` (preformatted blocks with monospace font)
- **Blockquotes**: `blockquote` (quote blocks with italic styling)
- **Line Breaks**: `br`

### Partially Supported Elements

- **Links**: Clickable links with navigation callback support
- **Inline Elements**: Simplified inline layout (mostly vertical stacking)

### Not Yet Supported

- **CSS Styling**: The renderer does not parse or apply CSS
- **Tables**: `table`, `tr`, `td`, `th`
- **Forms**: `form`, `input`, `button`, `textarea`, `select`
- **Media**: `video`, `audio`
- **Canvas/SVG**: `canvas`, `svg`
- **Semantic Elements**: Full support for `article`, `section`, `nav`, etc.

## Design Considerations

### Extensibility

The module is designed for easy extension:

1. **CSS Support**: The `Box` structure includes fields for padding, margin, and border
2. **New Elements**: Add rendering logic in `canvas.go` for new element types
3. **Layout Algorithms**: Extend `layout.go` to support more complex layouts (flexbox, grid)
4. **Styling**: Add style computation between parsing and layout phases

### Performance

- **Efficient Tree Building**: Single-pass HTML node traversal
- **Lazy Rendering**: Only visible content is rendered (via Fyne's scrolling)
- **Minimal Memory**: Render tree is lightweight compared to full DOM

### Limitations

1. **Approximate Text Layout**: Character-based width calculation is approximate
2. **Simplified Inline Layout**: Inline elements mostly stack vertically
3. **No CSS Cascade**: No style inheritance or specificity resolution
4. **Static Rendering**: No support for dynamic content updates (yet)

## Testing

The module includes comprehensive tests covering:

- **Node Management**: Creating nodes, adding children, setting attributes
- **Tree Building**: Converting HTML to render tree with various structures
- **Layout Calculations**: Font sizes, spacing, positioning
- **Rendering**: All supported element types
- **Integration**: Complex HTML documents

Run tests:

```bash
go test -v ./internal/renderer/...
```

View coverage:

```bash
go test -cover ./internal/renderer/...
```

## Future Enhancements

### Phase 1: Improved Rendering

- [x] Accurate text measurement using font metrics (completed)
- [x] True inline layout for inline elements (completed)
- [ ] Image loading and caching
- [ ] Clickable links with navigation integration

### Phase 2: CSS Support

- [ ] CSS parser
- [ ] Style computation and cascade
- [ ] Box model implementation (padding, margin, border)
- [ ] Color and background support
- [ ] Basic selectors (class, id, element)

### Phase 3: Advanced Layout

- [ ] Flexbox layout
- [ ] Grid layout
- [ ] Absolute/relative positioning
- [ ] Float layout
- [ ] Multi-column layout

### Phase 4: Interactive Elements

- [ ] Form input handling
- [ ] Button click events
- [ ] Scrolling within elements
- [ ] Hover effects
- [ ] Focus management

## Contributing

When extending the renderer:

1. **Add Tests**: Write comprehensive tests for new features
2. **Document**: Update this README with new capabilities
3. **Follow Patterns**: Use existing patterns for consistency
4. **Consider CSS**: Design with future CSS support in mind
5. **Performance**: Profile and optimize critical paths

## References

- [MDN HTML Documentation](https://developer.mozilla.org/en-US/docs/Web/HTML)
- [Fyne Documentation](https://docs.fyne.io/)
- [Go HTML Parser](https://pkg.go.dev/golang.org/x/net/html)

## License

This module is part of the Goosie project and follows the same license.
