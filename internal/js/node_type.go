package js

import "github.com/vyquocvu/goosie/internal/dom"

// nodeType ordinals are the DOM Standard's, not internal/dom's. Page scripts
// compare the literal numbers (`1 === e.nodeType`, `9 === doc.nodeType`), so
// the binding translates and must never pass the internal kind through:
// internal/dom counts its kinds from 0 with the document first, which would
// hand a page `document.nodeType === 0` and text nodes `2`. The document value
// is the fatal one - it is Sizzle's setup gate, and jQuery 3 (the most-deployed
// library on the live corpus) quietly un-configures itself when the gate fails,
// taking every `$(document).ready` reveal with it (GOOSIE-LIVE-001).
const (
	NodeTypeElement          = 1
	NodeTypeText             = 3
	NodeTypeComment          = 8
	NodeTypeDocument         = 9
	NodeTypeDocumentType     = 10
	NodeTypeDocumentFragment = 11
)

// domNodeType translates one internal/dom kind onto its spec ordinal. An
// unknown kind reports 0, a number no spec node produces, so a future kind
// shows up as "matched nothing" instead of masquerading as a document.
func domNodeType(t dom.NodeType) int {
	switch t {
	case dom.NodeElement:
		return NodeTypeElement
	case dom.NodeText:
		return NodeTypeText
	case dom.NodeComment:
		return NodeTypeComment
	case dom.NodeDocument:
		return NodeTypeDocument
	case dom.NodeDoctype:
		return NodeTypeDocumentType
	case dom.NodeDocumentFragment:
		return NodeTypeDocumentFragment
	}
	return 0
}
