package renderer

import (
	"sync"

	"github.com/vyquocvu/goosie/internal/css"
)

const defaultUAStyle = `
html { display: block; }
head, title, meta, link, style, script, noscript, template, base, iframe { display: none; }
body { display: block; margin: 8px; }
article, aside, details, figcaption, figure, footer, header, hgroup, main, nav, section, summary { display: block; }
address { display: block; font-style: italic; }
b, strong { font-weight: bold; }
i, em { font-style: italic; }
u { text-decoration: underline; }
strike, s { text-decoration: line-through; }
tt, code, kbd, samp { font-family: monospace; }
small { font-size: 0.83em; }
sub { vertical-align: sub; font-size: 0.83em; }
sup { vertical-align: super; font-size: 0.83em; }
h1 { font-size: 2em; font-weight: bold; display: block; margin: 0.67em 0; }
h2 { font-size: 1.5em; font-weight: bold; display: block; margin: 0.83em 0; }
h3 { font-size: 1.17em; font-weight: bold; display: block; margin: 1em 0; }
h4 { font-size: 1em; font-weight: bold; display: block; margin: 1.33em 0; }
h5 { font-size: 0.83em; font-weight: bold; display: block; margin: 1.67em 0; }
h6 { font-size: 0.67em; font-weight: bold; display: block; margin: 2.33em 0; }
p { display: block; margin: 1em 0; }
ul { display: block; list-style-type: disc; margin: 1em 0; padding-left: 40px; }
ol { display: block; list-style-type: decimal; margin: 1em 0; padding-left: 40px; }
li { display: list-item; margin: 0.1em 0; }
dl { display: block; margin: 1em 0; }
dt { display: block; font-weight: bold; }
dd { display: block; margin-left: 40px; }
blockquote { display: block; margin: 1em 40px; }
pre { display: block; margin: 1em 0; white-space: pre; }
hr { display: block; margin: 0.5em 0; border: 1px solid; }
table { display: table; border-collapse: separate; border-spacing: 2px; border-color: gray; }
tr { display: table-row; }
td, th { display: table-cell; vertical-align: inherit; }
th { font-weight: bold; text-align: center; }
thead { display: table-header-group; }
tbody { display: table-row-group; }
tfoot { display: table-footer-group; }
a { color: #0000ee; text-decoration: underline; }
`

// defaultStyleSheet caches the parsed UA stylesheet. It is immutable after
// parse (StyleManagers only read .Rules), so every manager can share one
// instance instead of re-parsing defaultUAStyle per construction.
var defaultStyleSheet = sync.OnceValue(func() *css.StyleSheet {
	parser := css.NewParser(defaultUAStyle)
	sheet, err := parser.Parse()
	if err != nil {
		return &css.StyleSheet{}
	}
	return sheet
})

// GetDefaultStyleSheet returns the default user-agent stylesheet.
func GetDefaultStyleSheet() *css.StyleSheet {
	return defaultStyleSheet()
}
