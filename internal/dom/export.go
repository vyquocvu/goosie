package dom

import "github.com/vyquocvu/goosie/internal/dom/atom"

// RareData is the exported type alias for rareData for use by external test packages.
type RareData = rareData

// NewRareData creates a new rareData for use by external test packages.
func NewRareData(namespace atom.Atom) RareData {
	return rareData{Namespace: namespace}
}
