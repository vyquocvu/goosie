// Package darwin is v2's macOS window: an NSWindow paced by CVDisplayLink whose
// content layer shows the frame path's composed buffer.
//
// It is the only package in v2 that links a system GUI framework, and the only place
// cgo is allowed, which is what the archtest's cgo rule enforces. Everything on the
// other side of shim.h is Objective-C; everything on this side is Go implementing
// surface.Window.
//
// On a machine or a build where the shim cannot be used - Linux, Windows, or
// CGO_ENABLED=0 - this package still compiles, from stub.go, and every entry point
// reports ErrPlatformUnavailable. That is why the errors below live in an untagged
// file: a caller on any platform has to be able to name the failure it got.
package darwin
