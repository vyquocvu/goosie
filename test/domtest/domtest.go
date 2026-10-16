// Package domtest builds fixture documents under the same resource caps the engine
// applies to a loaded one.
//
// It exists because internal/dom's exported Parse entry point - which this round deleted -
// skipped the accounting entirely: `allowNode` returned early when the node cap was zero, so
// every fixture built through it was a tree no document path could produce, and the drift
// between the two was invisible. A fixture parsed under the caps is the shape the engine
// retains.
//
// The caps are spelled as literals here and pinned against internal/engine's by
// domtest_test.go rather than imported: engine depends on style, so importing engine from
// this package would make internal/style's own tests an import cycle.
package domtest

import (
	"github.com/vyquocvu/goosie/internal/dom"
)

// fixtureLimits mirrors the engine's document caps. Read through a function rather than a
// package var so this package holds no state a test can rewrite out from under another.
func fixtureLimits() dom.ParseLimits {
	return dom.ParseLimits{
		Nodes:          50000,
		Depth:          128,
		Attributes:     64,
		AttributeBytes: 65536,
	}
}

// Parse builds html under the engine's caps. A fixture that crosses one is a test bug
// rather than a runtime condition, so it panics instead of handing every call site an
// error to ignore. A test that wants a document past a cap is asking about the cap and
// should call dom.ParseBounded with the limits it means.
func Parse(html string) *dom.Document {
	doc, err := dom.ParseBounded(html, fixtureLimits())
	if err != nil {
		panic("domtest.Parse: fixture refused by the engine's document caps: " + err.Error())
	}
	return doc
}
