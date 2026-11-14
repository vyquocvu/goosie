package style

import (
	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

type PseudoKey struct {
	NodeID dom.NodeID
	Pseudo string
}

func ResolvePseudoElements(doc *dom.Document, sheets []*css.Stylesheet, elementStyles map[dom.NodeID]*ComputedStyle, fonts *CustomFonts) map[PseudoKey]*ComputedStyle {
	return NewResolver(sheets).ResolvePseudoElements(doc, elementStyles, fonts)
}

func resolvePseudoElements(doc *dom.Document, index *ruleIndex, elementStyles map[dom.NodeID]*ComputedStyle, fonts *CustomFonts) map[PseudoKey]*ComputedStyle {
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
				for _, pseudo := range []string{"before", "after", "first-letter", "first-line"} {
					cs := computePseudoStyle(n, pseudo, index, parentStyle, fonts, six)
					if cs == nil {
						continue
					}
					if pseudo == "first-line" {
						filterFirstLineProperties(cs)
						result[PseudoKey{NodeID: n.ID, Pseudo: pseudo}] = cs
					} else if pseudo == "first-letter" {
						if cs.Content != "" && cs.Content != "none" {
							result[PseudoKey{NodeID: n.ID, Pseudo: pseudo}] = cs
						}
					} else {
						if len(cs.ContentParts) > 0 || (cs.Content != "normal" && cs.Content != "none") {
							result[PseudoKey{NodeID: n.ID, Pseudo: pseudo}] = cs
						}
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

func reResolvePseudoSubtree(roots []*dom.Node, index *ruleIndex, elementStyles map[dom.NodeID]*ComputedStyle, fonts *CustomFonts, affected map[dom.NodeID]struct{}, result map[PseudoKey]*ComputedStyle) {
	if result == nil {
		return
	}
	for key := range result {
		if _, ok := affected[key.NodeID]; ok {
			delete(result, key)
		}
	}
	six := css.NewSiblingIndex()
	var walk func(*dom.Node)
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
				for _, pseudo := range []string{"before", "after", "first-letter", "first-line"} {
					cs := computePseudoStyle(n, pseudo, index, parentStyle, fonts, six)
					if cs == nil {
						continue
					}
					if pseudo == "first-line" {
						filterFirstLineProperties(cs)
						result[PseudoKey{NodeID: n.ID, Pseudo: pseudo}] = cs
					} else if pseudo == "first-letter" {
						if cs.Content != "" && cs.Content != "none" {
							result[PseudoKey{NodeID: n.ID, Pseudo: pseudo}] = cs
						}
					} else {
						if len(cs.ContentParts) > 0 || (cs.Content != "normal" && cs.Content != "none") {
							result[PseudoKey{NodeID: n.ID, Pseudo: pseudo}] = cs
						}
					}
				}
			}
		}
		if n.ShadowRoot != nil {
			for c := n.ShadowRoot.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, root := range roots {
		walk(root)
	}
}

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
