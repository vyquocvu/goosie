# internal/css — Agent Constraints & Architecture

## Core Responsibilities

The `internal/css` package provides CSS parsing, stylesheet modeling, selector compilation, rule matching, cascade resolution, and computed style evaluation for Goosie.

## 16 Property Atom Enum Types (`atoms.go`)

- Hot-path CSS style property values are modeled as `uint8` enum atoms rather than raw strings:
  1. `DisplayAtom` (`DisplayAtomBlock`, `DisplayAtomInline`, `DisplayAtomFlex`, `DisplayAtomGrid`, `DisplayAtomNone`, etc.)
  2. `PositionAtom` (`PositionAtomStatic`, `PositionAtomRelative`, `PositionAtomAbsolute`, `PositionAtomFixed`, `PositionAtomSticky`)
  3. `FloatAtom` (`FloatAtomNone`, `FloatAtomLeft`, `FloatAtomRight`)
  4. `VisibilityAtom` (`VisibilityAtomVisible`, `VisibilityAtomHidden`, `VisibilityAtomCollapse`)
  5. `FontStyleAtom` (`FontStyleAtomNormal`, `FontStyleAtomItalic`, `FontStyleAtomOblique`)
  6. `TextAlignAtom` (`TextAlignAtomLeft`, `TextAlignAtomRight`, `TextAlignAtomCenter`, `TextAlignAtomJustify`)
  7. `TextDecorationAtom` (`TextDecorationAtomNone`, `TextDecorationAtomUnderline`, `TextDecorationAtomLineThrough`, `TextDecorationAtomOverline`)
  8. `TextTransformAtom` (`TextTransformAtomNone`, `TextTransformAtomUppercase`, `TextTransformAtomLowercase`, `TextTransformAtomCapitalize`)
  9. `WhiteSpaceAtom` (`WhiteSpaceAtomNormal`, `WhiteSpaceAtomNowrap`, `WhiteSpaceAtomPre`, `WhiteSpaceAtomPreWrap`, `WhiteSpaceAtomPreLine`)
  10. `BackgroundRepeatAtom` (`BackgroundRepeatAtomRepeat`, `BackgroundRepeatAtomNoRepeat`, `BackgroundRepeatAtomRepeatX`, `BackgroundRepeatAtomRepeatY`)
  11. `BackgroundPositionAtom` (`BackgroundPositionAtomTopLeft`, `BackgroundPositionAtomCenter`, etc.)
  12. `BackgroundSizeAtom` (`BackgroundSizeAtomAuto`, `BackgroundSizeAtomCover`, `BackgroundSizeAtomContain`)
  13. `BackgroundAttachmentAtom` (`BackgroundAttachmentAtomScroll`, `BackgroundAttachmentAtomFixed`)
  14. `ListStyleTypeAtom` (`ListStyleTypeAtomDisc`, `ListStyleTypeAtomCircle`, `ListStyleTypeAtomSquare`, `ListStyleTypeAtomDecimal`, `ListStyleTypeAtomNone`)
  15. `ListStylePositionAtom` (`ListStylePositionAtomInside`, `ListStylePositionAtomOutside`)
  16. `OverflowAtom` (`OverflowAtomVisible`, `OverflowAtomHidden`, `OverflowAtomScroll`, `OverflowAtomAuto`)
- Note that other properties (`BoxSizing`, `Clear`, `FlexDirection`, `FlexWrap`, `JustifyContent`, `AlignItems`, `AlignSelf`, `AlignContent`, `GridAutoFlow`, `VerticalAlign`, `WordBreak`) are `string` fields on `ComputedStyle` / `Style`.
- Conversion from strings occurs once at parse/declaration boundaries (`applyDeclaration`). Layout and rendering code strictly compares `uint8` atom values for these 16 atom types.

## Rule Bucketing by Right-Most Selector

- Stylesheet rules are partitioned into indexed buckets based on the key (right-most) compound selector:
  - Tag name bucket
  - ID bucket (`#id`)
  - Class name bucket (`.class`)
  - Universal / pseudo / attribute fallback bucket
- When matching candidate rules against a DOM node, only rules in the matching buckets are evaluated, drastically reducing the search space.

## Selector Matching & `MatchCache`

- `match_cache.go` maintains a per-document or per-pass `MatchCache` to memoize selector evaluation outcomes across repeated tree walks.
- Specificity calculation obeys CSS standard ordering: `Specificity{Inline: [0/1], IDs: int, Classes: int, Tags: int}`.
- Complex combinators (descendant ` `, child `>`, adjacent sibling `+`, general sibling `~`) evaluate right-to-left.

## Computed Style Resolution & Inheritance

- `computed.go` applies the standard CSS cascade order: User-Agent defaults → User styles → Author rules (by specificity & source order) → Inline styles → `!important` declarations.
- Inherited properties (color, font, line-height, text-align, etc.) automatically propagate down the render tree unless overridden.
- Relative units (`em`, `rem`, `vh`, `vw`, `%`) are resolved against parent computed metrics or viewport dimensions.

## Style Invalidation

- `invalidation.go` analyzes DOM mutation records (class changes, ID changes, attribute updates) to produce targeted dirty flags (`DirtyStyle`, `DirtyLayout`, `DirtySubtree`) instead of triggering full stylesheet re-computation.

## Testing & Verification

All CSS package tests reside in `test/internal/css/...`.

Run the CSS test suite:
```bash
go test -race -short ./test/internal/css/...
```
