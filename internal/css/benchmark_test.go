package css

import "testing"

func BenchmarkParseSmall(b *testing.B) {
	benchmarkParse(b, smallCSS)
}

func BenchmarkParseMedium(b *testing.B) {
	benchmarkParse(b, mediumCSS)
}

func BenchmarkParseLarge(b *testing.B) {
	benchmarkParse(b, largeCSS)
}

func BenchmarkParseSelectorComplex(b *testing.B) {
	benchmarkParse(b, selectorComplexCSS)
}

func BenchmarkParseSelectorHeavy(b *testing.B) {
	benchmarkParse(b, selectorHeavyCSS)
}

func BenchmarkParseAtRules(b *testing.B) {
	benchmarkParse(b, atRulesCSS)
}

func BenchmarkParseMalformed(b *testing.B) {
	benchmarkParse(b, malformedCSS)
}

func BenchmarkParseStyleAttribute(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ParseStyleAttribute("color: red; font-size: 16px; margin: 10px; padding: 5px; border: 1px solid black;")
	}
}

func benchmarkParse(b *testing.B, css string) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := NewParser(css)
		_, err := p.Parse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

const smallCSS = `
h1 { color: red; font-size: 32px; }
p { margin: 10px 0; line-height: 1.5; }
a { color: blue; text-decoration: underline; }
`

const mediumCSS = `
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: Arial, sans-serif; font-size: 16px; line-height: 1.6; color: #333; background: #fff; }
h1 { font-size: 2em; font-weight: 700; margin-bottom: 0.5em; color: #111; }
h2 { font-size: 1.5em; font-weight: 600; margin-bottom: 0.4em; color: #222; }
h3 { font-size: 1.25em; font-weight: 600; margin-bottom: 0.3em; color: #333; }
p { margin-bottom: 1em; }
a { color: #0066cc; text-decoration: none; }
a:hover { text-decoration: underline; }
ul, ol { margin-bottom: 1em; padding-left: 2em; }
li { margin-bottom: 0.5em; }
img { max-width: 100%; height: auto; }
blockquote { margin: 1em 0; padding: 0.5em 1em; border-left: 4px solid #ccc; color: #666; }
pre { background: #f5f5f5; padding: 1em; overflow-x: auto; border-radius: 4px; }
code { font-family: monospace; background: #f0f0f0; padding: 0.2em 0.4em; border-radius: 3px; }
pre code { background: transparent; padding: 0; }
table { width: 100%; border-collapse: collapse; margin-bottom: 1em; }
th, td { padding: 0.5em; border: 1px solid #ddd; text-align: left; }
th { background: #f5f5f5; font-weight: 600; }
.container { max-width: 1200px; margin: 0 auto; padding: 0 1em; }
.header { background: #333; color: #fff; padding: 1em 0; }
.footer { background: #333; color: #ccc; padding: 2em 0; margin-top: 2em; text-align: center; }
`

const largeCSS = `
/* CSS Framework Reset */
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
html { font-size: 16px; -webkit-text-size-adjust: 100%; }
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif; font-size: 1rem; line-height: 1.5; color: #212529; background-color: #fff; }
h1, h2, h3, h4, h5, h6 { margin-top: 0; margin-bottom: 0.5rem; font-weight: 500; line-height: 1.2; color: inherit; }
h1 { font-size: 2.5rem; }
h2 { font-size: 2rem; }
h3 { font-size: 1.75rem; }
h4 { font-size: 1.5rem; }
h5 { font-size: 1.25rem; }
h6 { font-size: 1rem; }
p { margin-top: 0; margin-bottom: 1rem; }
a { color: #0d6efd; text-decoration: none; }
a:hover { color: #0a58ca; text-decoration: underline; }
img { vertical-align: middle; max-width: 100%; height: auto; }
figure { margin: 0 0 1rem; }
figcaption { font-size: 0.875em; color: #6c757d; }
table { caption-side: bottom; border-collapse: collapse; width: 100%; }
th { text-align: inherit; text-align: -webkit-match-parent; }
thead, tbody, tfoot, tr, td, th { border-color: inherit; border-style: solid; border-width: 0; }
table > :not(caption) > * > * { padding: 0.5rem; border-bottom-width: 1px; }
pre, code, kbd, samp { font-family: SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace; font-size: 1em; }
pre { display: block; margin-top: 0; margin-bottom: 1rem; overflow: auto; font-size: 0.875em; }
pre code { font-size: inherit; color: inherit; word-break: normal; }
code { font-size: 0.875em; color: #d63384; word-wrap: break-word; }
a > code { color: inherit; }
kbd { padding: 0.1875rem 0.375rem; font-size: 0.875em; color: #fff; background-color: #212529; border-radius: 0.25rem; }
kbd kbd { padding: 0; font-size: 1em; }
ul, ol { padding-left: 2rem; }
dl { margin-top: 0; margin-bottom: 1rem; }
dt { font-weight: 700; }
dd { margin-bottom: 0.5rem; padding-left: 1rem; }
blockquote { margin: 0 0 1rem; padding: 0.5rem 1rem; border-left: 0.25rem solid #dee2e6; }
hr { margin: 1rem 0; color: inherit; border: 0; border-top: 1px solid; opacity: 0.25; }
small { font-size: 0.875em; }
mark { padding: 0.1875em; background-color: #fcf8e3; }
sub, sup { position: relative; font-size: 0.75em; line-height: 0; vertical-align: baseline; }
sub { bottom: -0.25em; }
sup { top: -0.5em; }
.container { width: 100%; padding-right: 0.75rem; padding-left: 0.75rem; margin-right: auto; margin-left: auto; }
@media (min-width: 576px) { .container { max-width: 540px; } }
@media (min-width: 768px) { .container { max-width: 720px; } }
@media (min-width: 992px) { .container { max-width: 960px; } }
@media (min-width: 1200px) { .container { max-width: 1140px; } }
.row { display: flex; flex-wrap: wrap; margin-right: -0.75rem; margin-left: -0.75rem; }
.col { flex: 1 0 0%; }
.col-1 { flex: 0 0 8.333333%; max-width: 8.333333%; }
.col-2 { flex: 0 0 16.666667%; max-width: 16.666667%; }
.col-3 { flex: 0 0 25%; max-width: 25%; }
.col-4 { flex: 0 0 33.333333%; max-width: 33.333333%; }
.col-5 { flex: 0 0 41.666667%; max-width: 41.666667%; }
.col-6 { flex: 0 0 50%; max-width: 50%; }
.col-7 { flex: 0 0 58.333333%; max-width: 58.333333%; }
.col-8 { flex: 0 0 66.666667%; max-width: 66.666667%; }
.col-9 { flex: 0 0 75%; max-width: 75%; }
.col-10 { flex: 0 0 83.333333%; max-width: 83.333333%; }
.col-11 { flex: 0 0 91.666667%; max-width: 91.666667%; }
.col-12 { flex: 0 0 100%; max-width: 100%; }
.btn { display: inline-block; font-weight: 400; line-height: 1.5; color: #212529; text-align: center; text-decoration: none; vertical-align: middle; cursor: pointer; user-select: none; background-color: transparent; border: 1px solid transparent; padding: 0.375rem 0.75rem; font-size: 1rem; border-radius: 0.25rem; }
.btn-primary { color: #fff; background-color: #0d6efd; border-color: #0d6efd; }
.btn-secondary { color: #fff; background-color: #6c757d; border-color: #6c757d; }
.btn-success { color: #fff; background-color: #198754; border-color: #198754; }
.btn-danger { color: #fff; background-color: #dc3545; border-color: #dc3545; }
.btn-warning { color: #000; background-color: #ffc107; border-color: #ffc107; }
.btn-info { color: #000; background-color: #0dcaf0; border-color: #0dcaf0; }
.card { position: relative; display: flex; flex-direction: column; min-width: 0; word-wrap: break-word; background-color: #fff; background-clip: border-box; border: 1px solid rgba(0,0,0,0.125); border-radius: 0.25rem; }
.card-body { flex: 1 1 auto; padding: 1rem 1rem; }
.card-header { padding: 0.5rem 1rem; margin-bottom: 0; background-color: rgba(0,0,0,0.03); border-bottom: 1px solid rgba(0,0,0,0.125); }
.card-footer { padding: 0.5rem 1rem; background-color: rgba(0,0,0,0.03); border-top: 1px solid rgba(0,0,0,0.125); }
.form-control { display: block; width: 100%; padding: 0.375rem 0.75rem; font-size: 1rem; font-weight: 400; line-height: 1.5; color: #212529; background-color: #fff; background-clip: padding-box; border: 1px solid #ced4da; border-radius: 0.25rem; }
.form-label { margin-bottom: 0.5rem; }
.alert { position: relative; padding: 1rem; margin-bottom: 1rem; border: 1px solid transparent; border-radius: 0.25rem; }
.alert-primary { color: #084298; background-color: #cfe2ff; border-color: #b6d4fe; }
.alert-danger { color: #842029; background-color: #f8d7da; border-color: #f5c2c7; }
.nav { display: flex; flex-wrap: wrap; padding-left: 0; margin-bottom: 0; list-style: none; }
.nav-link { display: block; padding: 0.5rem 1rem; color: #0d6efd; text-decoration: none; }
.badge { display: inline-block; padding: 0.35em 0.65em; font-size: 0.75em; font-weight: 700; line-height: 1; color: #fff; text-align: center; white-space: nowrap; vertical-align: baseline; border-radius: 0.25rem; }
.text-center { text-align: center; }
.text-start { text-align: left; }
.text-end { text-align: right; }
.fw-bold { font-weight: 700; }
.fw-normal { font-weight: 400; }
.fst-italic { font-style: italic; }
.d-none { display: none; }
.d-block { display: block; }
.d-flex { display: flex; }
.d-grid { display: grid; }
.position-relative { position: relative; }
.position-absolute { position: absolute; }
.overflow-hidden { overflow: hidden; }
`

const selectorComplexCSS = `
div > p { color: red; }
h1 + p { margin-top: 0; }
h2 ~ p { color: gray; }
div.container > p#intro.highlight:first-child { font-weight: bold; }
ul li:first-child { font-weight: bold; }
ul li:last-child { border-bottom: none; }
tr:nth-child(2n) { background: #f5f5f5; }
a[href^="https"] { color: green; }
a[href$=".pdf"]::after { content: " (PDF)"; }
input[type="text"] { border: 1px solid #ccc; }
input[type="checkbox"]:checked { outline: 2px solid blue; }
p[class~="highlight"] { background: yellow; }
a[href*="example.com"] { font-weight: bold; }
blockquote::before { content: "> "; color: #999; }
p::first-line { font-weight: bold; }
div:not(.hidden) { display: block; }
`

const selectorHeavyCSS = `
.container .row .col { padding: 10px; }
#header .nav .nav-item .nav-link { color: #333; }
div > ul > li > a { text-decoration: none; }
body.main-page div.content p.text { line-height: 1.8; }
#sidebar .widget:first-child .widget-title { font-size: 1.2em; }
div[data-type="article"] h2.title { margin-bottom: 0.5em; }
ul.list li.item:nth-child(odd) { background: #f9f9f9; }
div.container > div.row > div.col-6 > p { margin: 0; }
#page .section:nth-child(2n+1) .card .card-body { padding: 1.5em; }
form[method="post"] .form-group input[type="text"] { width: 100%; }
footer .footer-links a[href^="/"] { color: #666; }
nav.sidebar ul li a[target="_blank"]::after { content: " ext"; }
div.modal .modal-header h5.modal-title { font-size: 1.25rem; }
main#main-content article.post header h1.post-title { color: #111; }
section.blog-posts article.post:not(.draft) .post-body { display: block; }
div[data-theme="dark"] .card .card-body p { color: #eee; }
#header div.container nav ul.nav li.nav-item a.nav-link { padding: 10px 15px; }
div.wrapper main.content aside.sidebar .widget:last-child { border-bottom: none; }
`

const atRulesCSS = `
@media screen and (max-width: 600px) {
	body { font-size: 14px; }
	.container { padding: 0 10px; }
	.sidebar { display: none; }
	.nav { flex-direction: column; }
}
@media screen and (min-width: 601px) and (max-width: 1024px) {
	.container { max-width: 960px; }
	.col-md-6 { flex: 0 0 50%; max-width: 50%; }
	.col-md-4 { flex: 0 0 33.333%; max-width: 33.333%; }
}
@media print {
	body { font-size: 12pt; color: #000; }
	.nav, .sidebar, .footer { display: none; }
	a[href]::after { content: " (" attr(href) ")"; }
}
@import url("reset.css");
@import url("theme.css") screen;
@import "print.css" print;
@keyframes fadeIn {
	from { opacity: 0; }
	to { opacity: 1; }
}
@keyframes slideIn {
	0% { transform: translateX(-100%); opacity: 0; }
	50% { transform: translateX(10%); }
	100% { transform: translateX(0); opacity: 1; }
}
@keyframes pulse {
	0%, 100% { opacity: 1; }
	50% { opacity: 0.5; }
}
@media (prefers-color-scheme: dark) {
	body { background: #1a1a2e; color: #e0e0e0; }
	a { color: #6cb4ee; }
	.card { background: #16213e; border-color: #0f3460; }
}
`

const malformedCSS = `
p { color: red; /* missing close comment
div { }
h1 { color: ; font-size: 16px; }
span { font-weight bold; }
a { color: blue; } /* another unclosed comment
.class { property: value
#id { background: url("http://example.com/image.png); }
[type=text] { border: 1px; }
`
