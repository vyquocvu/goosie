// Package archtest enforces v2's import boundaries mechanically. No shipped binary
// imports it: the rules shell out to `go list` and walk the tree, so the architecture
// holds against a change nobody reviewed rather than against the review that did not
// happen.
//
// The rules are ordinary exported functions rather than test-local helpers so that
// more than one suite can enforce them - boundary_test.go reports them, and the M1
// gate suite reports the same ones, which is what lets "M1 CI green" stay one command
// without a table maintained twice.
package archtest
