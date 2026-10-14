package style

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

// PseudoKey identifies a pseudo-element of a specific node.
type PseudoKey struct {
	NodeID dom.NodeID
	Pseudo string
}

// ResolvePseudoElements computes styles for ::before and ::after pseudo-elements.
// It scans the stylesheet for selectors matching pseudo-elements of each element,
// computes their styles (inheriting from the originating element), and returns
// a map keyed by (NodeID, pseudo name).
func ResolvePseudoElements(doc *dom.Document, sheets []*css.Stylesheet, elementStyles map[dom.NodeID]*ComputedStyle, fonts *CustomFonts) map[PseudoKey]*ComputedStyle {
	allSheets := []*css.Stylesheet{UserAgentStylesheet()}
	allSheets = append(allSheets, sheets...)

	index := buildRuleIndex(allSheets)
	result := make(map[PseudoKey]*ComputedStyle)
	six := css.NewSiblingIndex()

	var walk func(n *dom.Node)
	walk = func(n *dom.Node) {
		if n == nil {
			return
		}
		if n.Element() {
			parentStyle := elementStyles[n.ID]
			if parentStyle == nil {
				parentStyle = findParentStyle(n, elementStyles)
			}
			if parentStyle != nil {
				for _, pseudo := range []string{"before", "after"} {
					cs := computePseudoStyle(n, pseudo, index, parentStyle, fonts, six)
					// An explicitly set empty string (content:"") is generated
					// content - an empty box - not no content: float-clearing
					// clearfixes depend on that box existing. Only the unset
					// default ("normal", which computes to none on pseudos) and
					// an explicit none suppress generation.
					if cs != nil && (len(cs.ContentParts) > 0 || (cs.Content != "normal" && cs.Content != "none")) {
						result[PseudoKey{NodeID: n.ID, Pseudo: pseudo}] = cs
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}

	for c := doc.Node.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	return result
}

// computePseudoStyle computes the style for a pseudo-element of the given node.
// It returns nil if no rules match.
func computePseudoStyle(n *dom.Node, pseudo string, index *ruleIndex, parent *ComputedStyle, fonts *CustomFonts, six *css.SiblingIndex) *ComputedStyle {
	candidates := index.candidates(n)
	var matchingDecls []css.Declaration
	var matchingOrigins []int
	var matchingABC [][3]int
	var matchingOrders []int

	for _, entry := range candidates {
		if !entry.sel.MatchesPseudoElementIndexed(n, pseudo, six) {
			continue
		}
		a, b, c := entry.sel.Specificity()
		for _, decl := range entry.rule.Declarations {
			matchingDecls = append(matchingDecls, decl)
			matchingOrigins = append(matchingOrigins, entry.origin)
			matchingABC = append(matchingABC, [3]int{a, b, c})
			matchingOrders = append(matchingOrders, entry.rank)
		}
	}

	if len(matchingDecls) == 0 {
		return nil
	}

	cs := DefaultStyle()
	inheritFromParent(&cs, parent)

	for i, decl := range matchingDecls {
		applyProperty(&cs, decl.Property, decl.Value, decl.Parsed, parent.FontSize, fonts)
		_ = matchingOrigins[i]
		_ = matchingABC[i]
		_ = matchingOrders[i]
	}
	resolveCustomFont(&cs, fonts)

	return &cs
}
