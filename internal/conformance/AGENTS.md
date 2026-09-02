# internal/conformance — Agent Constraints & Architecture

## Core Responsibilities

The `internal/conformance` package provides the HTML5 element audit registry, layout test fixtures, automated specification compliance verification, and markdown report generation tools for Goosie.

## Element Audit Registry (`registry.go`)

- `registry.go` maintains the authoritative catalog of all standard HTML5 elements with their expected layout display categories, rendering behaviors, and conformance status:
  - `supported`: Fully parsed, generates expected layout box class, correctly handles text visibility.
  - `partial`: Parsed and placed in tree, but missing specific styling or box formatting nuances.
  - `missing`: Unrecognized tag or dropped during tree construction.
  - `n/a (structural)`: Metadata, script, or template elements that do not produce direct visual boxes.

## Automated Audit Engine (`audit.go`)

- `AuditElement(el Element)` renders isolated element fixtures through the real browser rendering pipeline (`renderer.LayoutHTML`) at 800x600 resolution.
- Verifies:
  1. `Parsed`: Tag is retained in the compact DOM and render tree (`findByConfAttr`).
  2. `DisplayMatch`: Normalized outer display class matches browser expectations.
  3. `HasBox`: Element generates non-zero layout box geometry.
  4. `TextOK`: Text node inclusion matches element rendering rules (e.g. `<script>` does not render visible text).

## Conformance Verification & Report Generation

Run the element audit test with verbose output to inspect individual tag statuses:
```bash
go test ./test/internal/conformance -run TestElementAudit -v
```

Run all conformance tests:
```bash
go test ./test/internal/conformance/...
```

Regenerate the repository-wide `HTML_CONFORMANCE.md` document from the audit registry:
```bash
make html-audit
# or directly:
go run ./cmd/html-audit
```
