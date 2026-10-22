package style

import (
	"sort"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

// ruleIndex buckets selectors by what their key (rightmost) compound requires, so
// resolving one element tests only the selectors that could match it.
//
// Without bucketing, resolution is O(selectors x elements): a site with ~2,000
// selectors and ~13,000 elements asks 26M membership questions per document,
// which is slow enough that the engine's style budget has to clip most of the
// author stylesheet away. Every element already knows its own id, classes and
// tag, and a selector that requires a class the element lacks cannot match it
// regardless of its ancestors, so those selectors need never be tested.
type ruleIndex struct {
	byID    map[string][]*indexedSelector
	byClass map[string][]*indexedSelector
	byTag   map[string][]*indexedSelector
	// unkeyed holds selectors whose key compound tests none of id, class or tag —
	// `*`, `[type=text]`, `:hover` — which every element must still try.
	unkeyed []*indexedSelector
	// declCount is the total declaration slots across all sheets, used to rank
	// inline styles after every authored declaration.
	declCount  int
	stamp      int
	properties map[string]dom.PropertyDefinition
}

// indexedSelector is one selector of one rule plus the cascade facts derived
// from it, so a resolution pass never revisits the sheet structure.
type indexedSelector struct {
	rule       *css.Rule
	sel        css.Selector
	origin     int
	layerOrder int
	a, b, c    int
	// rank counts the declarations that precede this selector's own block in
	// sheet, rule and selector order. Walking matches in rank order reproduces
	// exactly the plain traversal the cascade is defined on, which is what keeps
	// the index invisible to the result.
	rank int
	// mark is the stamp of the query that last collected this selector, used to
	// drop duplicates when a key compound names several classes.
	mark int
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

	// Step 1: Record all explicitly ordered layers across sheets (@layer name1, name2;)
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
			}
		}
	}
	return ix
}

// insert files a selector under the tests its key compound requires. An id is
// the narrowest possible key, then any class the compound demands (the selector
// goes under each, since a node needs all of them), then the tag.
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

// candidates returns every selector that could match n, in cascade traversal
// order. The caller still has to run the full match: the key compound is a
// necessary condition, not a sufficient one.
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
	// Each bucket is already in rank order (buildRuleIndex appends in sheet
	// order), so the merged slice is nearly sorted and almost always tiny:
	// skip the sort for 0-1 candidates and insertion-sort the common small
	// case instead of paying sort.Slice's interface dispatch per element.
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
