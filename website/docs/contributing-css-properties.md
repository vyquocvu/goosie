# Adding a CSS Property

This guide explains how to add a new CSS property to the Goosie engine.

## Overview

CSS properties flow through four stages: parsing → style resolution → layout → display. Depending on the property type, you may need to add it to one or more stages.

## Step 1: Classify the Property

Determine if the property is **hot** (commonly used, stored in typed fields) or **cold** (rare, stored in rare-property map).

Hot properties include layout (display, position, margins), borders, typography (font-size, color), backgrounds, and flexbox. If your property is used frequently and affects layout or paint, make it hot. Otherwise, make it cold.

## Step 2: Add to Parser

**Hot property:** Add it to the `hotProperties` map in `internal/css/properties.go`. This ensures it's available as a typed field:

```go
var hotProperties = map[string]bool{
    "your-property": true,
    // ... existing properties
}
```

**Cold property:** No parser change needed. Cold properties are parsed generically and stored in the `RareProperties` map.

## Step 3: Add Typed Fields

### For hot properties only

1. **If inherited** (affects children): Add a field to `InheritedStyle` in `internal/css/computed.go`:

```go
type InheritedStyle struct {
    // ... existing fields
    YourProperty float32  // or string, Color, etc.
}
```

2. **If non-inherited** (affects only the element): Add a field to `NonInheritedStyle` in the same file.

3. **Update `Fingerprint()` and `Equal()`** methods to include the new field.

4. **Add resolution logic** in `ApplyDeclaration()` or the style resolution function — convert the parsed CSS value string to the typed field.

### For cold properties

Cold properties are stored generically; no field addition is needed.

## Step 4: Add Layout Support (if layout-affecting)

If the property affects layout (box model, display type, position), update the layout engine in `internal/renderer/`:

1. Read the computed style value in the appropriate layout function.
2. Apply the value to the layout box calculations.
3. Add dirty-reason flags if the change should trigger reflow.

## Step 5: Add Display/Raster Support (if paint-affecting)

If the property affects rendering (color, background, border), update the display list builder or raster backend:

1. Transfer the computed value to a `DisplayCommand` field in `internal/renderer/displaycmd.go`.
2. The raster backend (`internal/renderer/frame/raster/`) will pick up the value from the display command.

## Step 6: Add to Supported Platform Doc

Update `supported-web-platform.md` — add the new property to the CSS properties table with its status (`supported`, `partial`, or `planned`).

## Step 7: Tests

- **Parser test:** Add a test fixture in `internal/css/` that parses a stylesheet with the new property.
- **Style resolution test:** Verify the computed value matches expectations.
- **Layout test:** If layout-affecting, add a layout fixture in `internal/renderer/layoutgolden/`.
- **Golden image test:** If paint-affecting, add a rendering fixture in `internal/renderer/frame/golden/`.

## Checklist

- [ ] Added to `hotProperties` map (hot) or left as cold
- [ ] Added typed field to `InheritedStyle`/`NonInheritedStyle` (hot only)
- [ ] Updated `Fingerprint()` and `Equal()` (hot only)
- [ ] Added resolution logic in `ApplyDeclaration()`
- [ ] Updated layout engine (if layout-affecting)
- [ ] Updated display commands or raster (if paint-affecting)
- [ ] Updated `supported-web-platform.md`
- [ ] Added parser and resolution tests
- [ ] Added layout or golden tests (if applicable)
- [ ] Ran `go test -short ./internal/css/...`
