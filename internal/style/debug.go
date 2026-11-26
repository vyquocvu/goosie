package style

import (
	"fmt"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/dom"
)

type SelectorMatch struct {
	Text         string
	Specificity  string
	Rank         int
	Declarations []string
}

func DebugMatches(sheets []*css.Stylesheet, n *dom.Node) []SelectorMatch {
	if n == nil || !n.Element() {
		return nil
	}
	all := make([]*css.Stylesheet, 0, len(sheets)+1)
	all = append(all, UserAgentStylesheet())
	all = append(all, sheets...)
	index := buildRuleIndex(all)
	index.loadDocumentProperties(n.Doc)
	six := css.NewSiblingIndex()
	out := make([]SelectorMatch, 0, 8)
	for _, cand := range index.candidates(n) {
		if cand.origin == 0 || !cand.sel.MatchesIndexed(n, six) {
			continue
		}
		match := SelectorMatch{
			Text:        cand.sel.String(),
			Specificity: fmt.Sprintf("%d,%d,%d", cand.a, cand.b, cand.c),
			Rank:        cand.rank,
		}
		for _, decl := range cand.rule.Declarations {
			entry := decl.Property + ":" + decl.Value
			if decl.Important {
				entry += "!important"
			}
			match.Declarations = append(match.Declarations, entry)
		}
		out = append(out, match)
	}
	return out
}
