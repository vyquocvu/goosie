package style

import (
	"strings"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

type PseudoKey struct {
	NodeID dom.NodeID
	Pseudo string
}

func PseudoStyleApplies(pseudo string, cs *ComputedStyle) bool {
	if cs == nil {
		return false
	}
	switch pseudo {
	case "first-line", "placeholder":
		return true
	case "first-letter":
		return cs.Content != "" && cs.Content != "none"
	case "before", "after":
		return len(cs.ContentParts) > 0 || (cs.Content != "normal" && cs.Content != "none")
	default:
		return false
	}
}

func storePseudoStyle(result map[PseudoKey]*ComputedStyle, nodeID dom.NodeID, pseudo string, cs *ComputedStyle) {
	if !PseudoStyleApplies(pseudo, cs) {
		return
	}
	if pseudo == "first-line" {
		filterFirstLineProperties(cs)
	}
	result[PseudoKey{NodeID: nodeID, Pseudo: pseudo}] = cs
}

const (
	generatedBefore uint8 = 1 << iota
	generatedAfter
	generatedFirstLetter
	generatedFirstLine
	generatedPlaceholder
)

func generatedPseudoMask(candidates []*indexedSelector) uint8 {
	var mask uint8
	for _, entry := range candidates {
		for _, part := range entry.sel.Parts {
			for _, cond := range part.Conditions {
				if cond.Type != css.CondPseudoElement {
					continue
				}
				switch cond.Value {
				case "before":
					mask |= generatedBefore
				case "after":
					mask |= generatedAfter
				case "first-letter":
					mask |= generatedFirstLetter
				case "first-line":
					mask |= generatedFirstLine
				case "placeholder":
					mask |= generatedPlaceholder
				}
			}
		}
	}
	return mask
}

func resolveGeneratedPseudoStyles(n *dom.Node, candidates []*indexedSelector, mask uint8, parent *ComputedStyle, fonts *CustomFonts, six *css.SiblingIndex, result map[PseudoKey]*ComputedStyle) {
	if mask == 0 {
		return
	}
	names := [...]string{"before", "after", "first-letter", "first-line", "placeholder"}
	for i, pseudo := range names {
		if mask&(1<<i) == 0 {
			continue
		}
		cs := computePseudoStyle(n, pseudo, candidates, parent, fonts, six)
		if cs != nil {
			storePseudoStyle(result, n.ID, pseudo, cs)
		}
	}
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
				candidates := index.candidates(n)
				resolveGeneratedPseudoStyles(n, candidates, generatedPseudoMask(candidates), parentStyle, fonts, six, result)
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
				candidates := index.candidates(n)
				resolveGeneratedPseudoStyles(n, candidates, generatedPseudoMask(candidates), parentStyle, fonts, six, result)
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

func computePseudoStyle(n *dom.Node, pseudo string, candidates []*indexedSelector, parent *ComputedStyle, fonts *CustomFonts, six *css.SiblingIndex) *ComputedStyle {
	var entries []cascadeEntry
	for _, entry := range candidates {
		if !entry.sel.MatchesPseudoElementIndexed(n, pseudo, six) {
			continue
		}
		dist := computeScopeDist(n, entry.rule.ScopeRoot)
		for _, decl := range entry.rule.Declarations {
			entries = append(entries, cascadeEntry{decl: decl, origin: entry.origin, layerOrder: entry.layerOrder, scopeDist: dist, a: entry.a, b: entry.b, c: entry.c, order: entry.rank})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	sortCascade(entries)

	cs := DefaultStyle()
	inheritFromParent(&cs, parent)
	cs.inheritedFont = readFontGroup(&cs)

	if pseudo == "placeholder" {
		cs.Color = css.Color{R: 117, G: 117, B: 117, A: 255}
	}

	for _, e := range entries {
		decl := e.decl
		applyPropertySpecified(&cs, decl.Property, decl.Value, decl.Parsed, parent.FontSize, fonts, e.origin > 0)
		if pseudo == "placeholder" && strings.EqualFold(strings.TrimSpace(decl.Property), "color") &&
			(strings.EqualFold(strings.TrimSpace(decl.Value), "inherit") || strings.EqualFold(strings.TrimSpace(decl.Value), "unset")) {
			cs.Color = parent.Color
		}
	}
	resolveBorderColors(&cs, parent, entries, nil, nil)
	resolveCustomFont(&cs, fonts)
	if pseudo == "first-line" {
		if cs.LineHeightRatio > 0 {
			cs.LineHeight = cs.LineHeightRatio * cs.FontSize
		}
		if cs.LineHeightEm > 0 {
			cs.LineHeight = cs.LineHeightEm * cs.FontSize
		}
	}

	return &cs
}

var pseudoInitialDisplays = map[string]Display{
	"marker":          DisplayInlineBlock,
	"backdrop":        DisplayBlock,
	"cue":             DisplayBlock,
	"view-transition": DisplayBlock,
}

var pseudoReadUnresolved = map[string]bool{
	"part":    true,
	"slotted": true,
}

func (r *Resolver) PseudoStyleForRead(n *dom.Node, pseudo string, elementStyles map[dom.NodeID]*ComputedStyle, fonts *CustomFonts) (*ComputedStyle, bool) {
	if r == nil || r.index == nil || n == nil || !n.Element() || pseudoReadUnresolved[pseudo] {
		return nil, false
	}
	parent := elementStyles[n.ID]
	if parent == nil {
		parent = findParentStyle(n, elementStyles)
	}
	if parent == nil {
		return nil, false
	}
	cs := computePseudoStyle(n, pseudo, r.index.candidates(n), parent, fonts, css.NewSiblingIndex())
	if cs == nil {
		defaulted := DefaultStyle()
		inheritFromParent(&defaulted, parent)
		if pseudo == "placeholder" && css.IsPlaceholderShown(n) {
			defaulted.Color = css.Color{R: 117, G: 117, B: 117, A: 255}
		}
		cs = &defaulted
	} else if pseudo == "first-line" {
		filterFirstLineProperties(cs)
	}
	if cs.Display == DisplayInline {
		if initial, ok := pseudoInitialDisplays[pseudo]; ok {
			cs.Display = initial
		}
	}
	return cs, true
}

func PseudoContentValue(pseudo string, cs *ComputedStyle, attr func(string) (string, bool), resolveURL func(string) string) string {
	if cs == nil {
		return ""
	}
	if pseudo == "first-line" || pseudo == "first-letter" {
		return "normal"
	}
	if text, ok := readContentValue(cs.ContentRaw, attr, resolveURL); ok {
		if text == "normal" && (pseudo == "before" || pseudo == "after") {
			return "none"
		}
		return text
	}
	if len(cs.ContentParts) > 0 {
		return ""
	}
	switch cs.Content {
	case "":
		if cs.ContentIsString {
			return `""`
		}
		return ""
	case "normal":
		if pseudo == "before" || pseudo == "after" {
			return "none"
		}
		return "normal"
	}
	if cs.ContentIsString {
		return `"` + cs.Content + `"`
	}
	return cs.Content
}
