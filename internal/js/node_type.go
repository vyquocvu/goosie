package js

import "github.com/vyquocvu/goosie/internal/dom"

const (
	NodeTypeElement          = 1
	NodeTypeText             = 3
	NodeTypeComment          = 8
	NodeTypeDocument         = 9
	NodeTypeDocumentType     = 10
	NodeTypeDocumentFragment = 11
)

func domNodeType(t dom.NodeType) int {
	switch t {
	case dom.NodeElement:
		return NodeTypeElement
	case dom.NodeText:
		return NodeTypeText
	case dom.NodeCDATASection:
		return 4
	case dom.NodeComment:
		return NodeTypeComment
	case dom.NodeProcessingInstruction:
		return 7
	case dom.NodeDocument:
		return NodeTypeDocument
	case dom.NodeDoctype:
		return NodeTypeDocumentType
	case dom.NodeDocumentFragment:
		return NodeTypeDocumentFragment
	}
	return 0
}
