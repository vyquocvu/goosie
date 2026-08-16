// Package conformance tracks Goosie's HTML element coverage against the
// MDN HTML elements reference
// (https://developer.mozilla.org/en-US/docs/Web/HTML/Reference/Elements).
//
// The registry below enumerates every element MDN documents, grouped by
// MDN's own categories, with a fixture that exercises the element through
// the real pipeline and the browser default display the audit expects.
// The generated tracker document (HTML_CONFORMANCE.md at the repo root)
// records the observed state; fixing elements one by one means updating the
// renderer, then regenerating the tracker with `make html-audit`.
package conformance

// MDNStatus classifies an element the way MDN's reference does.
type MDNStatus string

const (
	MDNStandard    MDNStatus = "standard"
	MDNDeprecated  MDNStatus = "deprecated"
	MDNExperimental MDNStatus = "experimental"
)

// ExpectedDisplay is the browser-default outer display class the audit
// compares against. Goosie's engine has a coarser display model
// (block/inline/flex/grid/table/none), so table-internal roles are
// pragmatically expected as "block"; the e2e conformance test compares the
// full computed style against Chromium separately.
type ExpectedDisplay string

const (
	DisplayBlock       ExpectedDisplay = "block"
	DisplayInline      ExpectedDisplay = "inline"
	DisplayInlineBlock ExpectedDisplay = "inline-block" // replaced/form controls in browsers
	DisplayNone        ExpectedDisplay = "none"
	// Ancestor-owned roles: rendering is handled by a container (table
	// rows, ruby annotations). Correct when the content actually flowed.
	DisplayInternalContent ExpectedDisplay = "internal-content"
	// Supporting roles (col, source, track...). Correct when their content
	// does not leak into layout.
	DisplayInternalHidden ExpectedDisplay = "internal-hidden"
	// Void inline elements (br, wbr): no box or text of their own; being
	// present in the tree is the check.
	DisplayVoidInline ExpectedDisplay = "void-inline"
)

// Element describes one MDN HTML element for the conformance program.
type Element struct {
	Name     string          // lowercase tag name, e.g. "p"
	Category string          // MDN category, e.g. "Text content"
	MDN      MDNStatus       // standard | deprecated | experimental
	Display  ExpectedDisplay // browser-default display class
	// Fixture is body-inner HTML that exercises the element. The element
	// under test carries data-conf="<name>" so both the unit audit and the
	// e2e Chromium comparison can locate it.
	Fixture string
	// RendersText is false for elements whose content must NOT become
	// visible text (head metadata, script, template, ...).
	RendersText bool
	// Audit controls whether the element participates in pass/fail scoring.
	// Deprecated elements are tracked informationally only.
	Audit bool
}

// defaultFixture wraps generic content in the element under test.
func defaultFixture(name string) string {
	return "<" + name + ` data-conf="` + name + `">Goosie conformance text</` + name + ">"
}

func el(name, category string, mdn MDNStatus, display ExpectedDisplay, fixture string, rendersText bool) Element {
	if fixture == "" {
		fixture = defaultFixture(name)
	}
	return Element{
		Name: name, Category: category, MDN: mdn, Display: display,
		Fixture: fixture, RendersText: rendersText,
		Audit: mdn != MDNDeprecated,
	}
}

const (
	catRoot      = "Main root"
	catMeta      = "Document metadata"
	catBody      = "Sectioning root"
	catSection   = "Content sectioning"
	catText      = "Text content"
	catInline    = "Inline text semantics"
	catMedia     = "Image and multimedia"
	catEmbedded  = "Embedded content"
	catScripting = "Scripting"
	catEdits     = "Demarcating edits"
	catTable     = "Table content"
	catForms     = "Forms"
	catInteract  = "Interactive elements"
	catWebComp   = "Web Components"
	catObsolete  = "Obsolete and deprecated elements"
)

// Elements is the full MDN HTML element registry in MDN reference order.
var Elements = buildRegistry()

func buildRegistry() []Element {
	var e []Element

	// --- Main root / metadata / sectioning root ---
	e = append(e,
		Element{Name: "html", Category: catRoot, MDN: MDNStandard, Display: DisplayBlock,
			Fixture: `<div data-conf="html-proxy">shell</div>`, RendersText: true, Audit: false}, // structural shell; audited via document fixture
		Element{Name: "head", Category: catMeta, MDN: MDNStandard, Display: DisplayNone,
			Fixture: `<head data-conf="head"><title>t</title></head>`, RendersText: false, Audit: false},
		el("title", catMeta, MDNStandard, DisplayNone, `<title data-conf="title">must not render</title>`, false),
		el("base", catMeta, MDNStandard, DisplayNone, `<base data-conf="base" href="/">`, false),
		el("link", catMeta, MDNStandard, DisplayNone, `<link data-conf="link" rel="stylesheet" href="data:,">`, false),
		el("meta", catMeta, MDNStandard, DisplayNone, `<meta data-conf="meta" charset="utf-8">`, false),
		el("style", catMeta, MDNStandard, DisplayNone, `<style data-conf="style">p{color:red}</style>`, false),
		Element{Name: "body", Category: catBody, MDN: MDNStandard, Display: DisplayBlock,
			Fixture: `<div data-conf="body-proxy">body shell</div>`, RendersText: true, Audit: false},
	)

	// --- Content sectioning ---
	e = append(e,
		el("address", catSection, MDNStandard, DisplayBlock, "", true),
		el("article", catSection, MDNStandard, DisplayBlock, "", true),
		el("aside", catSection, MDNStandard, DisplayBlock, "", true),
		el("footer", catSection, MDNStandard, DisplayBlock, "", true),
		el("header", catSection, MDNStandard, DisplayBlock, "", true),
		el("hgroup", catSection, MDNStandard, DisplayBlock, `<hgroup data-conf="hgroup"><h2>h</h2><p>p</p></hgroup>`, true),
		el("main", catSection, MDNStandard, DisplayBlock, "", true),
		el("nav", catSection, MDNStandard, DisplayBlock, "", true),
		el("section", catSection, MDNStandard, DisplayBlock, "", true),
		el("search", catSection, MDNStandard, DisplayBlock, "", true),
	)
	for _, h := range []string{"h1", "h2", "h3", "h4", "h5", "h6"} {
		e = append(e, el(h, catSection, MDNStandard, DisplayBlock, "", true))
	}

	// --- Text content ---
	e = append(e,
		el("blockquote", catText, MDNStandard, DisplayBlock, `<blockquote data-conf="blockquote">quoted words</blockquote>`, true),
		el("dd", catText, MDNStandard, DisplayBlock, `<dl><dt>term</dt><dd data-conf="dd">definition</dd></dl>`, true),
		el("dt", catText, MDNStandard, DisplayBlock, `<dl><dt data-conf="dt">term</dt><dd>definition</dd></dl>`, true),
		el("dl", catText, MDNStandard, DisplayBlock, `<dl data-conf="dl"><dt>term</dt><dd>definition</dd></dl>`, true),
		el("div", catText, MDNStandard, DisplayBlock, "", true),
		el("figcaption", catText, MDNStandard, DisplayBlock, `<figure><figcaption data-conf="figcaption">caption</figcaption><p>content</p></figure>`, true),
		el("figure", catText, MDNStandard, DisplayBlock, `<figure data-conf="figure"><figcaption>cap</figcaption><p>content</p></figure>`, true),
		el("hr", catText, MDNStandard, DisplayBlock, `<hr data-conf="hr">`, false),
		el("li", catText, MDNStandard, DisplayBlock, `<ul><li data-conf="li">item</li></ul>`, true),
		el("ol", catText, MDNStandard, DisplayBlock, `<ol data-conf="ol"><li>one</li><li>two</li></ol>`, true),
		el("ul", catText, MDNStandard, DisplayBlock, `<ul data-conf="ul"><li>one</li><li>two</li></ul>`, true),
		el("menu", catText, MDNStandard, DisplayBlock, `<menu data-conf="menu"><li>one</li><li>two</li></menu>`, true),
		el("p", catText, MDNStandard, DisplayBlock, "", true),
		el("pre", catText, MDNStandard, DisplayBlock, `<pre data-conf="pre">pre text
  indented</pre>`, true),
	)

	// --- Inline text semantics ---
	e = append(e,
		el("a", catInline, MDNStandard, DisplayInline, `<a data-conf="a" href="https://example.com">link text</a>`, true),
		el("abbr", catInline, MDNStandard, DisplayInline, `<abbr data-conf="abbr" title="HyperText">HT</abbr>`, true),
		el("b", catInline, MDNStandard, DisplayInline, "", true),
		el("bdi", catInline, MDNStandard, DisplayInline, "", true),
		el("bdo", catInline, MDNStandard, DisplayInline, `<bdo data-conf="bdo" dir="rtl">rtl</bdo>`, true),
		el("br", catInline, MDNStandard, DisplayVoidInline, `line one<br data-conf="br">line two`, false),
		el("cite", catInline, MDNStandard, DisplayInline, "", true),
		el("code", catInline, MDNStandard, DisplayInline, `<code data-conf="code">code_sample()</code>`, true),
		el("data", catInline, MDNStandard, DisplayInline, `<data data-conf="data" value="1">one</data>`, true),
		el("dfn", catInline, MDNStandard, DisplayInline, "", true),
		el("em", catInline, MDNStandard, DisplayInline, "", true),
		el("i", catInline, MDNStandard, DisplayInline, "", true),
		el("kbd", catInline, MDNStandard, DisplayInline, "", true),
		el("mark", catInline, MDNStandard, DisplayInline, `<mark data-conf="mark">marked</mark>`, true),
		el("q", catInline, MDNStandard, DisplayInline, `<q data-conf="q">quoted</q>`, true),
		el("rp", catInline, MDNStandard, DisplayNone, `<ruby>漢<rp data-conf="rp">(</rp><rt>kan</rt><rp>)</rp></ruby>`, false),
		el("rt", catInline, MDNStandard, DisplayInternalContent, `<ruby>漢<rt data-conf="rt">kan</rt></ruby>`, true), // browsers: ruby-internal; goosie pragmatic none
		el("ruby", catInline, MDNStandard, DisplayInline, "", true),
		el("s", catInline, MDNStandard, DisplayInline, "", true),
		el("samp", catInline, MDNStandard, DisplayInline, "", true),
		el("small", catInline, MDNStandard, DisplayInline, "", true),
		el("span", catInline, MDNStandard, DisplayInline, "", true),
		el("strong", catInline, MDNStandard, DisplayInline, "", true),
		el("sub", catInline, MDNStandard, DisplayInline, `H<sub data-conf="sub">2</sub>O`, true),
		el("sup", catInline, MDNStandard, DisplayInline, `x<sup data-conf="sup">2</sup>`, true),
		el("time", catInline, MDNStandard, DisplayInline, `<time data-conf="time" datetime="2026-01-01">Jan 1</time>`, true),
		el("u", catInline, MDNStandard, DisplayInline, "", true),
		el("var", catInline, MDNStandard, DisplayInline, "", true),
		el("wbr", catInline, MDNStandard, DisplayVoidInline, `long<wbr data-conf="wbr">word`, false),
	)

	// --- Image and multimedia ---
	e = append(e,
		el("area", catMedia, MDNStandard, DisplayInternalHidden, `<map name="m"><area data-conf="area" href="#" shape="rect" coords="0,0,10,10"></map>`, false),
		el("audio", catMedia, MDNStandard, DisplayNone, `<audio data-conf="audio" controls></audio>`, false),
		el("img", catMedia, MDNStandard, DisplayInlineBlock, `<img data-conf="img" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA=" width="40" height="20" alt="px">`, false),
		el("map", catMedia, MDNStandard, DisplayInternalHidden, `<map data-conf="map" name="m"><area href="#" shape="rect" coords="0,0,10,10"></map>`, false),
		el("track", catMedia, MDNStandard, DisplayInternalHidden, `<video><track data-conf="track" kind="captions"></video>`, false),
		el("video", catMedia, MDNStandard, DisplayInline, `<video data-conf="video" controls></video>`, false),
	)

	// --- Embedded content ---
	e = append(e,
		el("embed", catEmbedded, MDNStandard, DisplayInline, `<embed data-conf="embed" type="image/gif" src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA=">`, false),
		el("fencedframe", catEmbedded, MDNExperimental, DisplayInline, `<fencedframe data-conf="fencedframe"></fencedframe>`, false),
		el("iframe", catEmbedded, MDNStandard, DisplayInline, `<iframe data-conf="iframe" src="about:blank" title="t"></iframe>`, false),
		el("object", catEmbedded, MDNStandard, DisplayInline, `<object data-conf="object" type="image/gif" data="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA="><p>fallback</p></object>`, false),
		el("picture", catEmbedded, MDNStandard, DisplayInline, `<picture data-conf="picture"><source media="(min-width:1px)" srcset="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA="><img src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA=" width="40" height="20" alt="px"></picture>`, false),
		el("source", catEmbedded, MDNStandard, DisplayInternalHidden, `<video><source data-conf="source" type="video/mp4"></video>`, false),
	)

	// --- Scripting ---
	e = append(e,
		el("canvas", catScripting, MDNStandard, DisplayInline, `<canvas data-conf="canvas" width="60" height="30">canvas fallback</canvas>`, false),
		el("noscript", catScripting, MDNStandard, DisplayNone, `<noscript data-conf="noscript">no scripts</noscript>`, false),
		el("script", catScripting, MDNStandard, DisplayNone, `<script data-conf="script">1+1</script>`, false),
	)

	// --- Demarcating edits ---
	e = append(e,
		el("del", catEdits, MDNStandard, DisplayInline, `<del data-conf="del">removed</del>`, true),
		el("ins", catEdits, MDNStandard, DisplayInline, `<ins data-conf="ins">added</ins>`, true),
	)

	// --- Table content ---
	e = append(e,
		el("caption", catTable, MDNStandard, DisplayInternalContent, `<table><caption data-conf="caption">caption</caption><tr><td>cell</td></tr></table>`, true),
		el("col", catTable, MDNStandard, DisplayInternalHidden, `<table><colgroup><col data-conf="col" span="1"></colgroup><tr><td>cell</td></tr></table>`, false),
		el("colgroup", catTable, MDNStandard, DisplayInternalHidden, `<table><colgroup data-conf="colgroup"><col></colgroup><tr><td>cell</td></tr></table>`, false),
		el("table", catTable, MDNStandard, DisplayBlock, `<table data-conf="table"><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table>`, true),
		el("tbody", catTable, MDNStandard, DisplayInternalContent, `<table><tbody data-conf="tbody"><tr><td>cell</td></tr></tbody></table>`, true),
		el("tr", catTable, MDNStandard, DisplayInternalContent, `<table><tr data-conf="tr"><td>cell</td></tr></table>`, true),
		el("td", catTable, MDNStandard, DisplayInternalContent, `<table><tr><td data-conf="td">cell</td></tr></table>`, true),
		el("tfoot", catTable, MDNStandard, DisplayInternalContent, `<table><tr><td>c</td></tr><tfoot data-conf="tfoot"><tr><td>f</td></tr></tfoot></table>`, true),
		el("th", catTable, MDNStandard, DisplayInternalContent, `<table><tr><th data-conf="th">head</th></tr></table>`, true),
		el("thead", catTable, MDNStandard, DisplayInternalContent, `<table><thead data-conf="thead"><tr><th>h</th></tr></thead></table>`, true),
	)

	// --- Forms ---
	formCSS := `<style>[data-conf]{outline:1px solid #ccc;}</style>`
	_ = formCSS
	e = append(e,
		el("button", catForms, MDNStandard, DisplayInlineBlock, `<button data-conf="button" type="button">Click</button>`, true),
		el("datalist", catForms, MDNStandard, DisplayInternalHidden, `<datalist data-conf="datalist"><option value="a"></datalist>`, false),
		el("fieldset", catForms, MDNStandard, DisplayBlock, `<fieldset data-conf="fieldset"><legend>lg</legend><input name="q"></fieldset>`, false),
		el("form", catForms, MDNStandard, DisplayBlock, `<form data-conf="form"><input name="q"><button>b</button></form>`, false),
		el("label", catForms, MDNStandard, DisplayInline, `<label data-conf="label" for="i">Label</label><input id="i">`, true),
		el("legend", catForms, MDNStandard, DisplayBlock, `<fieldset><legend data-conf="legend">Legend</legend><input></fieldset>`, true),
		el("meter", catForms, MDNStandard, DisplayInlineBlock, `<meter data-conf="meter" value="0.6">60</meter>`, false),
		el("optgroup", catForms, MDNStandard, DisplayInternalHidden, `<select><optgroup data-conf="optgroup" label="g"><option>a</option></optgroup></select>`, false),
		el("option", catForms, MDNStandard, DisplayInternalHidden, `<select><option data-conf="option" selected>opt</option></select>`, false),
		el("output", catForms, MDNStandard, DisplayInline, `<output data-conf="output" for="a">42</output>`, true),
		el("progress", catForms, MDNStandard, DisplayInlineBlock, `<progress data-conf="progress" value="0.5"></progress>`, false),
		el("select", catForms, MDNStandard, DisplayInlineBlock, `<select data-conf="select"><option selected>a</option><option>b</option></select>`, false),
		el("textarea", catForms, MDNStandard, DisplayInlineBlock, `<textarea data-conf="textarea" rows="2" cols="12">text</textarea>`, false),
	)
	// input types
	for _, t := range []string{
		"text", "password", "email", "search", "url", "tel",
		"number", "range", "checkbox", "radio", "file", "hidden",
		"submit", "reset", "button", "image", "color",
		"date", "datetime-local", "month", "time", "week",
	} {
		expected := DisplayInlineBlock
		if t == "hidden" {
			expected = DisplayNone
		}
		e = append(e, Element{
			Name: "input[type=" + t + "]", Category: catForms, MDN: MDNStandard, Display: expected,
			Fixture:    `<input data-conf="input[type=` + t + `]" type="` + t + `" value="v">`,
			RendersText: false,
			Audit:      true,
		})
	}

	// --- Interactive elements ---
	e = append(e,
		el("details", catInteract, MDNStandard, DisplayBlock, `<details data-conf="details" open><summary>s</summary><p>body</p></details>`, true),
		el("summary", catInteract, MDNStandard, DisplayBlock, `<details open><summary data-conf="summary">Summary</summary><p>body</p></details>`, true),
		el("dialog", catInteract, MDNStandard, DisplayNone, `<dialog data-conf="dialog" open><p>dialog body</p></dialog>`, true),
		el("selectedcontent", catInteract, MDNExperimental, DisplayNone, `<selectedcontent data-conf="selectedcontent"></selectedcontent>`, false),
	)

	// --- Web Components ---
	e = append(e,
		el("slot", catWebComp, MDNStandard, DisplayNone, `<slot data-conf="slot" name="s"></slot>`, false),
		el("template", catWebComp, MDNStandard, DisplayNone, `<template data-conf="template"><p>hidden</p></template>`, false),
	)

	// --- Obsolete and deprecated elements (informational) ---
	e = append(e,
		el("acronym", catObsolete, MDNDeprecated, DisplayInline, "", true),
		el("big", catObsolete, MDNDeprecated, DisplayInline, "", true),
		el("center", catObsolete, MDNDeprecated, DisplayBlock, "", true),
		el("dir", catObsolete, MDNDeprecated, DisplayBlock, `<dir data-conf="dir"><li>a</li></dir>`, true),
		el("font", catObsolete, MDNDeprecated, DisplayInline, `<font data-conf="font" size="4" color="red">f</font>`, true),
		el("frame", catObsolete, MDNDeprecated, DisplayNone, `<frameset><frame data-conf="frame"></frameset>`, false),
		el("frameset", catObsolete, MDNDeprecated, DisplayNone, `<frameset data-conf="frameset"></frameset>`, false),
		el("marquee", catObsolete, MDNDeprecated, DisplayInline, "", true),
		el("nobr", catObsolete, MDNDeprecated, DisplayInline, "", true),
		el("noembed", catObsolete, MDNDeprecated, DisplayNone, `<noembed data-conf="noembed">x</noembed>`, false),
		el("noframes", catObsolete, MDNDeprecated, DisplayNone, `<noframes data-conf="noframes">x</noframes>`, false),
		el("param", catObsolete, MDNDeprecated, DisplayNone, `<object><param data-conf="param" name="a" value="b"></object>`, false),
		el("plaintext", catObsolete, MDNDeprecated, DisplayBlock, "", true),
		el("rb", catObsolete, MDNDeprecated, DisplayNone, `<ruby><rb data-conf="rb">漢</rb><rt>kan</rt></ruby>`, false),
		el("rtc", catObsolete, MDNDeprecated, DisplayNone, `<ruby>漢<rtc data-conf="rtc">kan</rtc></ruby>`, false),
		el("strike", catObsolete, MDNDeprecated, DisplayInline, "", true),
		el("tt", catObsolete, MDNDeprecated, DisplayInline, "", true),
		el("xmp", catObsolete, MDNDeprecated, DisplayBlock, "", true),
	)

	return e
}

// WrapFixture builds a full HTML document around a body-inner fixture.
func WrapFixture(fixture string) string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:0}` +
		`</style></head><body>` + fixture + `</body></html>`
}

// CountAuditElements returns how many elements participate in scoring.
func CountAuditElements() int {
	n := 0
	for _, e := range Elements {
		if e.Audit {
			n++
		}
	}
	return n
}
