# CSS Parser Documentation

The CSS parser provides comprehensive support for CSS selectors, combinators, pseudo-classes, pseudo-elements, and at-rules.

## Features

### Selectors
- **Basic**: Tag (`h1`), class (`.class`), ID (`#id`), universal (`*`)
- **Combinators**: Descendant (` `), child (`>`), adjacent sibling (`+`), general sibling (`~`)
- **Attributes**: All operators (`=`, `~=`, `|=`, `^=`, `$=`, `*=`)
- **Pseudo-classes**: `:link`, `:visited`, `:hover`, `:focus`, `:active`, `:first-child`, `:last-child`, `:nth-child(n)`
- **Pseudo-elements**: `::before`, `::after`

### Advanced Features
- **Multiple selectors**: Comma-separated (`h1, h2, h3`)
- **CSS comments**: `/* */`
- **At-rules**: `@media`, `@import`, `@keyframes`, `@supports` (parsed, not fully evaluated)
- **Important flag**: `!important`
- **Complex values**: Functions (`url()`, `calc()`, `rgb()`, `rgba()`)

## Examples

```css
/* Complex selector */
div.container > p#intro.highlight:first-child {
    color: blue;
    font-size: 18px;
}

/* Attribute selectors */
a[href^="https"] { color: green; }
img[src$=".png"] { border: 1px solid gray; }

/* Sibling selectors */
h2 + p { font-size: 18px; }
h2 ~ p { color: #555; }
```

## Architecture

- **Parser** (`internal/css/parser.go`): Tokenizes and parses CSS
- **Stylesheet** (`internal/css/stylesheet.go`): Data structures
- **Matcher** (`internal/renderer/style.go`): Right-to-left matching algorithm

## Testing

```bash
go test ./internal/css/... -v
```

## Limitations

- Pseudo-class state tracking (`:hover`, `:focus`) not fully implemented
- Media query conditions not evaluated
- Pseudo-element content generation not implemented
- Specificity calculation not yet implemented

## Demo

See `examples/html/full_css_demo.html` for comprehensive examples.
