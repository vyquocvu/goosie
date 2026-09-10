// Package archtest enforces v2's import boundaries mechanically. It contains no
// production code: its tests shell out to `go list` and walk the tree, so the
// architecture rules hold against a change nobody reviewed rather than against
// the review that did not happen.
package archtest
