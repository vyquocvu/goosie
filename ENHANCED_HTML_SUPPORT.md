# Enhanced HTML Support

Goosie supports comprehensive HTML rendering with CSS styling, form elements, tables, and multiple image formats.

## Features

### CSS Styling
- **Properties**: `color`, `font-size`, `font-weight`, `font-family`
- **Selectors**: Element, class, ID, pseudo-classes (`:link`, `:visited`)
- **Values**: Named colors, hex codes, pixel/em units

### Image Rendering
- **Formats**: PNG, JPEG, GIF, WebP
- **Features**: Async loading, LRU caching, relative/absolute URL resolution

### Interactive Links
- Clickable anchor tags with navigation integration
- Relative and absolute URL support

### Form Elements
- `<input>` (text, email, etc.)
- `<button>`
- `<textarea>`

### Tables
- Full table support: `<table>`, `<tbody>`, `<thead>`, `<tfoot>`, `<tr>`, `<td>`, `<th>`
- Automatic column width management

## Examples

See `examples/enhanced_html_demo.html` for comprehensive examples.

## Limitations

- No Flexbox/Grid layouts
- Background colors parsed but not rendered
- Form submission not implemented
- Advanced CSS features (animations, transitions, media queries) not supported

## Testing

```bash
go test ./internal/renderer -v
go test ./internal/css -v
go test ./internal/image -v
```
