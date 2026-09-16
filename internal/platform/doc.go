// Package platform chooses and constructs the window backend for a v2 binary.
//
// The dependency arrow points down from here: platform imports surface (for the
// Window contract it returns) and the concrete backends, and each backend imports
// surface but never this package. That is what makes the cycle-free claim in the
// architecture spec checkable rather than aspirational.
//
// Only internal/platform/darwin may contain cgo. Everything else, including
// this package's non-darwin build, has to compile on a machine with no display
// server, which is the property CI depends on.
package platform
