# ADR 0004: MCP Uses a UI-Independent Browser-Control Boundary

**Status:** Proposed

**Decision date:** 2026-07-15

**Deciders:** Goosie maintainers

## Context

Goosie has reusable navigation, network, DOM, JavaScript, and raster packages, but the complete page-load workflow is currently coordinated by the GUI command and `internal/ui`. MCP requires a long-lived, cancellable, multi-context automation service. Directly wrapping UI methods would couple protocol behavior to Fyne, UI-thread scheduling, active-tab globals, and renderer pointers.

MCP also has an independent lifecycle and transport model. Its JSON-RPC session must not be confused with Goosie's navigation session or existing child-renderer IPC protocol.

## Decision

Introduce two boundaries:

1. `internal/browsercontrol`: protocol-neutral orchestration and stable browser automation types.
2. `internal/mcpserver`: a thin adapter from MCP schemas/handlers to browser-control.

Add `cmd/mcp-server` as the stdio composition root. It owns configuration, process lifecycle, and logs but no browser behavior.

The first transport is stdio. Streamable HTTP is deferred behind a Go 1.25+, SDK, authorization, and security review gate.

## Detailed rules

- Browser-control and MCP server packages do not import `internal/ui` or Fyne.
- One browser context owns its engine session, network, DOM, JS, renderer, event buffers, and mutation queue.
- Semantic element references are opaque and bound to context plus page revision.
- MCP types do not leak below the adapter.
- Existing engine IPC types may inspire internal commands but are not the public MCP contract.
- The GUI should migrate toward the same browser-control service so navigation behavior has one owner.
- Stdio stdout is protocol-only; logs use stderr.

## Alternatives considered

### Wrap `internal/ui.Browser`

Rejected. It makes headless correctness depend on Fyne and the selected active tab, preserves sleep/callback readiness, and violates engine/UI boundaries.

### Map MCP directly to `internal/engine/renderer` IPC

Rejected. That protocol is an internal process-isolation schema with different versioning, capabilities, errors, and trust assumptions. Exposing it would freeze implementation details and still leave DOM/query/security orchestration unresolved.

### Put MCP logic in `cmd/mcp-server`

Rejected. Command packages should compose dependencies; embedding handlers and browser orchestration there prevents focused tests and reuse.

### Implement MCP framing manually

Rejected. Use the official Go SDK for lifecycle, schema, transport, and compatibility behavior. Custom code remains limited to browser contracts and policy.

### Start with Streamable HTTP

Rejected for v1. It adds Origin/Host validation, authentication, secure session management, proxy, SSRF, and operational requirements before browser-control is stable. It also conflicts with the present Go 1.24.9 toolchain if using current reviewed SDK releases.

## Consequences

### Positive

- Deterministic headless tests without GUI setup.
- One public automation model can support MCP and future adapters.
- Security/policy is enforced before protocol mapping.
- MCP SDK upgrades do not ripple into engine packages.
- GUI and MCP can converge on one navigation implementation.

### Costs

- Requires extraction/refactoring before visible MCP tools.
- Needs stable semantic snapshot/reference design.
- Context ownership and cancellation add lifecycle complexity.
- During migration, GUI and browser-control paths must be guarded against behavioral drift.

## Acceptance criteria

- The dependency graph matches `docs/MCP_ARCHITECTURE.md`.
- Browser-control integration tests load and act on local fixtures with no Fyne initialization.
- MCP handler tests run entirely against a fake browser-control implementation.
- No package below the shell imports the MCP SDK.
- A real stdio test proves protocol-only stdout and graceful disconnect cleanup.

## Revisit triggers

- Goosie adopts a different stable public automation API.
- Renderer process isolation becomes the sole engine execution model.
- MCP adds a new released transport or incompatible lifecycle revision.
- Remote hosting becomes an approved product requirement.
