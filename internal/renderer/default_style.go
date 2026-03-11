package renderer

import (
	"github.com/vyquocvu/goosie/internal/css"
)

const defaultUAStyle = `
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
li { display: list-item; }
table { display: table; border-collapse: separate; border-spacing: 2px; border-color: gray; }
tr { display: table-row; }
td, th { display: table-cell; vertical-align: inherit; }
th { font-weight: bold; text-align: center; }
thead { display: table-header-group; }
tbody { display: table-row-group; }
tfoot { display: table-footer-group; }
`

var defaultStyleSheet *css.StyleSheet

// GetDefaultStyleSheet returns the default user-agent stylesheet.
func GetDefaultStyleSheet() *css.StyleSheet {
	if defaultStyleSheet != nil {
		return defaultStyleSheet
	}

	parser := css.NewParser(defaultUAStyle)
	sheet, err := parser.Parse()
	if err != nil {
		// Should not happen with hardcoded CSS
		return &css.StyleSheet{}
	}
	defaultStyleSheet = sheet
	return defaultStyleSheet
}
