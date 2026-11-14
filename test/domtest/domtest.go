package domtest

import (
	"github.com/vyquocvu/goosie/internal/dom"
)

func fixtureLimits() dom.ParseLimits {
	return dom.ParseLimits{
		Nodes:          1000000,
		Depth:          128,
		Attributes:     64,
		AttributeBytes: 1048576,
	}
}

func Parse(html string) *dom.Document {
	doc, err := dom.ParseBounded(html, fixtureLimits())
	if err != nil {
		panic("domtest.Parse: fixture refused by the engine's document caps: " + err.Error())
	}
	return doc
}
