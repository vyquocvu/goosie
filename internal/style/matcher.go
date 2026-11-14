package style

import (
	"sort"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

type ruleIndex struct {
	byID       map[string][]*indexedSelector
	byClass    map[string][]*indexedSelector
	byTag      map[string][]*indexedSelector
	unkeyed    []*indexedSelector
	part       []*indexedSelector
	declCount  int
	stamp      int
	properties map[string]dom.PropertyDefinition
	selectors  []css.Selector
}

type indexedSelector struct {
	rule       *css.Rule
	sel        css.Selector
	origin     int
	layerOrder int
	a, b, c    int
	rank       int
	mark       int
}

func buildRuleIndex(sheets []*css.Stylesheet) *ruleIndex {
	ix := &ruleIndex{
		byID:       make(map[string][]*indexedSelector),
		byClass:    make(map[string][]*indexedSelector),
		byTag:      make(map[string][]*indexedSelector),
		properties: make(map[string]dom.PropertyDefinition),
	}

	for _, sheet := range sheets {
		for _, prop := range sheet.Properties {
			ix.properties[prop.Name] = prop
		}
	}

	layerSeq := 0
	layerMap := make(map[string]int)

	for _, sheet := range sheets {
		for _, name := range sheet.Layers {
			if _, ok := layerMap[name]; !ok {
				layerSeq++
				layerMap[name] = layerSeq
			}
		}
	}

	for _, sheet := range sheets {
		origin := 1
		if sheet == uaSheet {
			origin = 0
		}
		for i := range sheet.Rules {
			rule := &sheet.Rules[i]
			layerOrder := 0
			if rule.Layer != "" {
				if order, ok := layerMap[rule.Layer]; ok {
					layerOrder = order
				} else {
					layerSeq++
					layerMap[rule.Layer] = layerSeq
					layerOrder = layerSeq
				}
			}
			for _, sel := range rule.Selectors {
				ix.selectors = append(ix.selectors, sel)
				a, b, c := sel.Specificity()
				e := &indexedSelector{
					rule:       rule,
					sel:        sel,
					origin:     origin,
					layerOrder: layerOrder,
					a:          a, b: b, c: c,
					rank: ix.declCount,
				}
				ix.declCount += len(rule.Declarations)
				ix.insert(e)
				if selectorHasPart(e.sel) {
					ix.part = append(ix.part, e)
				}
			}
		}
	}
	return ix
}

func selectorHasPart(sel css.Selector) bool {
	for _, part := range sel.Parts {
		for _, cond := range part.Conditions {
			if cond.Type == css.CondPseudoElement && cond.Value == "part" {
				return true
			}
		}
	}
	return false
}

func (ix *ruleIndex) insert(e *indexedSelector) {
	part := e.sel.Parts
	if len(part) == 0 {
		ix.unkeyed = append(ix.unkeyed, e)
		return
	}
	var (
		id      string
		classes []string
		tag     string
	)
	for _, cond := range part[len(part)-1].Conditions {
		switch cond.Type {
		case css.CondID:
			id = cond.Value
		case css.CondClass:
			classes = append(classes, cond.Value)
		case css.CondType:
			tag = cond.Value
		}
	}
	switch {
	case id != "":
		ix.byID[id] = append(ix.byID[id], e)
	case len(classes) > 0:
		for _, cls := range classes {
			ix.byClass[cls] = append(ix.byClass[cls], e)
		}
	case tag != "":
		ix.byTag[tag] = append(ix.byTag[tag], e)
	default:
		ix.unkeyed = append(ix.unkeyed, e)
	}
}

func (ix *ruleIndex) candidates(n *dom.Node) []*indexedSelector {
	ix.stamp++
	stamp := ix.stamp
	var out []*indexedSelector
	collect := func(list []*indexedSelector) {
		for _, e := range list {
			if e.mark == stamp {
				continue
			}
			e.mark = stamp
			out = append(out, e)
		}
	}
	if id := n.GetAttribute("id"); id != "" {
		collect(ix.byID[id])
	}
	for _, cls := range n.ClassList() {
		collect(ix.byClass[cls])
	}
	collect(ix.byTag[n.Data])
	collect(ix.unkeyed)
	collect(ix.part)
	switch len(out) {
	case 0, 1:
	case 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16:
		for i := 1; i < len(out); i++ {
			for j := i; j > 0 && out[j].rank < out[j-1].rank; j-- {
				out[j], out[j-1] = out[j-1], out[j]
			}
		}
	default:
		sort.Slice(out, func(i, j int) bool { return out[i].rank < out[j].rank })
	}
	return out
}

func (ix *ruleIndex) loadDocumentProperties(doc *dom.Document) {
	if doc == nil {
		return
	}
	if reg := doc.RegisteredProperties(); len(reg) > 0 {
		if ix.properties == nil {
			ix.properties = make(map[string]dom.PropertyDefinition, len(reg))
		}
		for k, v := range reg {
			ix.properties[k] = v
		}
	}
}
