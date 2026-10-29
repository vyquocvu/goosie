# Goosie Goja fork

This directory is a tracked local fork of `github.com/dop251/goja` at
`v0.0.0-20260926152631-39ec2650adc9`, matching the original version pinned in
the root `go.mod`. The root module's `replace` directive keeps Goja's module
identity unchanged while allowing the engine changes here to be reviewed and
tested with Goosie.

The local changes add resizable `ArrayBuffer` and growable `SharedArrayBuffer`
construction and resizing, dynamic typed-array/DataView tracking and
out-of-bounds state, and a narrow host API for structured-clone metadata. This
fork does not implement cross-agent shared-memory/atomic semantics for
`SharedArrayBuffer`.

Run Goja's package tests from the repository root with:

```sh
go test github.com/dop251/goja -count=1
```

When updating Goja, compare this directory against the chosen upstream revision
and reapply or upstream the local changes before changing the pinned version.
