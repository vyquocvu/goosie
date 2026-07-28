package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TestCase represents a single test file to be generated
type TestCase struct {
	ID          int
	Category    string
	Description string
	HTML        string
}

func main() {
	outputDir := flag.String("output", "testdata", "Output directory")
	flag.Parse()

	// Ensure output directory exists
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	testCases := generateTestCases()
	fmt.Printf("Generating %d test cases...\n", len(testCases))

	var wg sync.WaitGroup
	for _, tc := range testCases {
		wg.Add(1)
		go func(tc TestCase) {
			defer wg.Done()
			filename := fmt.Sprintf("test_%03d_%s.html", tc.ID, tc.Category)
			path := filepath.Join(*outputDir, filename)

			// Add a standard header/footer wrapper if not present in the specific case content
			// But most cases below will provide full HTML to be precise

			if err := os.WriteFile(path, []byte(tc.HTML), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", filename, err)
			}
		}(tc)
	}
	wg.Wait()

	// Also generate the index file for easy navigation
	generateIndexFile(*outputDir, testCases)
	generateOutputHTMLFile(*outputDir)

	fmt.Println("All test cases generated successfully.")
}

func generateOutputHTMLFile(dir string) {
	content := `<!DOCTYPE html>
<html>
<head>
	<title>Goosie Test Output</title>
	<style>
		body { font-family: sans-serif; margin: 20px; }
		.container { display: flex; gap: 10px; }
		.box { width: 100px; height: 100px; border: 1px solid black; display: flex; align-items: center; justify-content: center; }
		.red { background-color: red; color: white; }
		.green { background-color: green; color: white; }
		.blue { background-color: blue; color: white; }
	</style>
</head>
<body>
	<h1>Goosie Generated Output</h1>
	<p>This file is generated for visual regression testing.</p>
	<div class="container" id="boxes">
		<div class="box red">Red</div>
		<div class="box green">Green</div>
		<div class="box blue">Blue</div>
	</div>
</body>
</html>`
	os.WriteFile(filepath.Join(dir, "output.html"), []byte(content), 0644)
}

func generateIndexFile(dir string, cases []TestCase) {
	content := `<!DOCTYPE html><html><head><title>Goosie Test Suite</title>
	<style>body{font-family:sans-serif;margin:20px;} table{border-collapse:collapse;width:100%;} th,td{border:1px solid #ddd;padding:8px;text-align:left;} th{background-color:#f2f2f2;} tr:nth-child(even){background-color:#f9f9f9;} a{text-decoration:none;color:#0066cc;} a:hover{text-decoration:underline;}</style>
	</head><body><h1>Goosie Test Suite Index</h1>
	<table><tr><th>ID</th><th>Category</th><th>Description</th><th>Link</th></tr>`

	for _, tc := range cases {
		filename := fmt.Sprintf("test_%03d_%s.html", tc.ID, tc.Category)
		content += fmt.Sprintf("<tr><td>%03d</td><td>%s</td><td>%s</td><td><a href='%s'>View</a></td></tr>",
			tc.ID, tc.Category, tc.Description, filename)
	}

	content += "</table></body></html>"
	os.WriteFile(filepath.Join(dir, "index.html"), []byte(content), 0644)
}

func generateTestCases() []TestCase {
	cases := []TestCase{}
	id := 1

	// Helper to add cases
	add := func(cat, desc, html string) {
		cases = append(cases, TestCase{ID: id, Category: cat, Description: desc, HTML: html})
		id++
	}

	// 1-10: Basic Typography & Text
	add("typography", "Headings and Paragraphs", `<!DOCTYPE html><html><body><h1>H1 Heading</h1><h2>H2 Heading</h2><h3>H3 Heading</h3><h4>H4 Heading</h4><h5>H5 Heading</h5><h6>H6 Heading</h6><p>Standard paragraph text.</p></body></html>`)
	add("typography", "Inline Text Styles", `<!DOCTYPE html><html><body><p><b>Bold</b>, <strong>Strong</strong>, <i>Italic</i>, <em>Emphasized</em>, <u>Underline</u>, <strike>Strike</strike>, <s>Strikethrough</s>, <tt>Teletype</tt>, <code>Code</code>, <small>Small</small>, <sub>Sub</sub>, <sup>Sup</sup>.</p></body></html>`)
	add("typography", "Lists", `<!DOCTYPE html><html><body><ul><li>Unordered 1</li><li>Unordered 2</li></ul><ol><li>Ordered 1</li><li>Ordered 2</li></ol><dl><dt>Term</dt><dd>Definition</dd></dl></body></html>`)
	add("typography", "Preformatted Text", `<!DOCTYPE html><html><body><pre>  Indented\n    Text\n      Preserved</pre></body></html>`)
	add("typography", "Blockquotes", `<!DOCTYPE html><html><body><blockquote><p>This is a blockquote.</p></blockquote></body></html>`)
	add("typography", "Text Alignment", `<!DOCTYPE html><html><body><p style="text-align:left">Left</p><p style="text-align:center">Center</p><p style="text-align:right">Right</p><p style="text-align:justify">Justify verify that the text is justified properly across the line width.</p></body></html>`)
	add("typography", "Font Families", `<!DOCTYPE html><html><body><p style="font-family:serif">Serif</p><p style="font-family:sans-serif">Sans-serif</p><p style="font-family:monospace">Monospace</p><p style="font-family:cursive">Cursive</p><p style="font-family:fantasy">Fantasy</p></body></html>`)
	add("typography", "Font Weights", `<!DOCTYPE html><html><body><p style="font-weight:100">100</p><p style="font-weight:400">400 (Normal)</p><p style="font-weight:700">700 (Bold)</p><p style="font-weight:900">900</p></body></html>`)
	add("typography", "Line Height", `<!DOCTYPE html><html><body><p style="line-height:1.0">Single line height<br>Second line</p><p style="line-height:2.0">Double line height<br>Second line</p></body></html>`)
	add("typography", "Letter Spacing", `<!DOCTYPE html><html><body><p style="letter-spacing:normal">Normal</p><p style="letter-spacing:2px">Wide</p><p style="letter-spacing:-1px">Tight</p></body></html>`)

	// 11-25: CSS Box Model & Layouts
	add("layout", "Box Model Margins", `<!DOCTYPE html><html><style>.box{border:1px solid black;margin:20px;}</style><body><div class="box">Box 1</div><div class="box">Box 2</div></body></html>`)
	add("layout", "Box Model Padding", `<!DOCTYPE html><html><style>.box{border:1px solid black;padding:20px;}</style><body><div class="box">Box with Padding</div></body></html>`)
	add("layout", "Box Model Borders", `<!DOCTYPE html><html><style>.b1{border:1px solid black;}.b2{border:5px dashed red;}.b3{border:10px dotted blue;}</style><body><div class="b1">Solid</div><div class="b2">Dashed</div><div class="b3">Dotted</div></body></html>`)
	add("layout", "Display Block vs Inline", `<!DOCTYPE html><html><style>.block{display:block;border:1px solid red;}.inline{display:inline;border:1px solid blue;}</style><body><span class="block">Span as Block</span><div class="inline">Div as Inline</div></body></html>`)
	add("layout", "Display Inline-Block", `<!DOCTYPE html><html><style>.ib{display:inline-block;width:100px;height:100px;border:1px solid black;}</style><body><div class="ib">1</div><div class="ib">2</div><div class="ib">3</div></body></html>`)
	add("layout", "Float Left/Right", `<!DOCTYPE html><html><style>.left{float:left;width:50px;height:50px;background:red;}.right{float:right;width:50px;height:50px;background:blue;}</style><body><div class="left">L</div><div class="right">R</div><p>Text wrapping around floats.</p></body></html>`)
	add("layout", "Clear Floats", `<!DOCTYPE html><html><style>.float{float:left;width:50px;height:50px;background:red;}.clear{clear:both;border-top:1px solid black;}</style><body><div class="float"></div><div class="clear">Cleared</div></body></html>`)
	add("layout", "Position Relative", `<!DOCTYPE html><html><style>.box{position:relative;left:20px;top:20px;border:1px solid black;}</style><body><div class="box">Relative Offset</div></body></html>`)
	add("layout", "Position Absolute", `<!DOCTYPE html><html><style>.container{position:relative;height:100px;border:1px solid black;}.abs{position:absolute;top:10px;right:10px;background:yellow;}</style><body><div class="container"><div class="abs">Absolute</div></div></body></html>`)
	add("layout", "Position Fixed", `<!DOCTYPE html><html><style>.fixed{position:fixed;bottom:10px;right:10px;background:lime;}</style><body><div class="fixed">Fixed</div><p>Scroll me (content needed)</p></body></html>`)
	add("layout", "Z-Index", `<!DOCTYPE html><html><style>.box{position:absolute;width:100px;height:100px;}.red{background:red;z-index:1;top:10px;left:10px;}.blue{background:blue;z-index:2;top:30px;left:30px;}</style><body><div class="red"></div><div class="blue"></div></body></html>`)
	add("layout", "Overflow Hidden", `<!DOCTYPE html><html><style>.box{width:100px;height:50px;overflow:hidden;border:1px solid black;}</style><body><div class="box">Content that exceeds the height of the box should be clipped.</div></body></html>`)
	add("layout", "Overflow Scroll", `<!DOCTYPE html><html><style>.box{width:100px;height:50px;overflow:scroll;border:1px solid black;}</style><body><div class="box">Content that exceeds the height of the box should scroll.</div></body></html>`)
	add("layout", "Visibility Hidden", `<!DOCTYPE html><html><body><div>Visible</div><div style="visibility:hidden">Hidden</div><div>Visible</div></body></html>`)
	add("layout", "Display None", `<!DOCTYPE html><html><body><div>Visible</div><div style="display:none">Gone</div><div>Visible</div></body></html>`)

	// 26-40: Flexbox
	add("flexbox", "Flex Row", `<!DOCTYPE html><html><style>.flex{display:flex;}.item{width:50px;height:50px;border:1px solid black;}</style><body><div class="flex"><div class="item">1</div><div class="item">2</div></div></body></html>`)
	add("flexbox", "Flex Column", `<!DOCTYPE html><html><style>.flex{display:flex;flex-direction:column;}.item{width:50px;height:50px;border:1px solid black;}</style><body><div class="flex"><div class="item">1</div><div class="item">2</div></div></body></html>`)
	add("flexbox", "Justify Content Center", `<!DOCTYPE html><html><style>.flex{display:flex;justify-content:center;border:1px solid blue;}.item{width:50px;height:50px;border:1px solid black;}</style><body><div class="flex"><div class="item">1</div></div></body></html>`)
	add("flexbox", "Justify Content Space-Between", `<!DOCTYPE html><html><style>.flex{display:flex;justify-content:space-between;border:1px solid blue;}.item{width:50px;height:50px;border:1px solid black;}</style><body><div class="flex"><div class="item">1</div><div class="item">2</div></div></body></html>`)
	add("flexbox", "Align Items Center", `<!DOCTYPE html><html><style>.flex{display:flex;align-items:center;height:100px;border:1px solid blue;}.item{width:50px;height:50px;border:1px solid black;}</style><body><div class="flex"><div class="item">1</div></div></body></html>`)
	add("flexbox", "Flex Wrap", `<!DOCTYPE html><html><style>.flex{display:flex;flex-wrap:wrap;width:110px;}.item{width:50px;height:50px;border:1px solid black;}</style><body><div class="flex"><div class="item">1</div><div class="item">2</div><div class="item">3</div></div></body></html>`)
	add("flexbox", "Flex Grow", `<!DOCTYPE html><html><style>.flex{display:flex;width:200px;}.item{border:1px solid black;}.grow{flex-grow:1;}</style><body><div class="flex"><div class="item">Fixed</div><div class="item grow">Grows</div></div></body></html>`)
	add("flexbox", "Flex Shrink", `<!DOCTYPE html><html><style>.flex{display:flex;width:200px;}.item{width:150px;border:1px solid black;flex-shrink:0;}.shrink{flex-shrink:1;}</style><body><div class="flex"><div class="item">Fixed</div><div class="item shrink">Shrinks</div></div></body></html>`)
	add("flexbox", "Flex Basis", `<!DOCTYPE html><html><style>.flex{display:flex;}.item{flex-basis:100px;border:1px solid black;}</style><body><div class="flex"><div class="item">Basis 100</div></div></body></html>`)
	add("flexbox", "Align Self", `<!DOCTYPE html><html><style>.flex{display:flex;height:100px;align-items:flex-start;}.item{height:50px;border:1px solid black;}.end{align-self:flex-end;}</style><body><div class="flex"><div class="item">Start</div><div class="item end">End</div></div></body></html>`)
	add("flexbox", "Order", `<!DOCTYPE html><html><style>.flex{display:flex;}.item{width:50px;height:50px;border:1px solid black;}.first{order:1;}.second{order:2;}</style><body><div class="flex"><div class="second">2</div><div class="first">1</div></div></body></html>`)
	add("flexbox", "Nested Flex", `<!DOCTYPE html><html><style>.outer{display:flex;justify-content:center;}.inner{display:flex;flex-direction:column;border:1px solid black;}</style><body><div class="outer"><div class="inner"><div>A</div><div>B</div></div></div></body></html>`)
	add("flexbox", "Flex Gap", `<!DOCTYPE html><html><style>.flex{display:flex;gap:20px;}.item{border:1px solid black;}</style><body><div class="flex"><div class="item">1</div><div class="item">2</div></div></body></html>`)
	add("flexbox", "Flex Row Reverse", `<!DOCTYPE html><html><style>.flex{display:flex;flex-direction:row-reverse;}.item{width:50px;border:1px solid black;}</style><body><div class="flex"><div class="item">1</div><div class="item">2</div></div></body></html>`)
	add("flexbox", "Flex Column Reverse", `<!DOCTYPE html><html><style>.flex{display:flex;flex-direction:column-reverse;}.item{height:50px;border:1px solid black;}</style><body><div class="flex"><div class="item">1</div><div class="item">2</div></div></body></html>`)

	// 41-50: Grid Layout
	add("grid", "Grid Basic", `<!DOCTYPE html><html><style>.grid{display:grid;grid-template-columns:100px 100px;}.item{border:1px solid black;}</style><body><div class="grid"><div class="item">1</div><div class="item">2</div><div class="item">3</div><div class="item">4</div></div></body></html>`)
	add("grid", "Grid Gap", `<!DOCTYPE html><html><style>.grid{display:grid;grid-template-columns:100px 100px;gap:10px;}.item{border:1px solid black;}</style><body><div class="grid"><div class="item">1</div><div class="item">2</div></div></body></html>`)
	add("grid", "Grid Areas", `<!DOCTYPE html><html><style>.grid{display:grid;grid-template-areas:"a b" "c c";}.a{grid-area:a;background:red;}.b{grid-area:b;background:blue;}.c{grid-area:c;background:green;}</style><body><div class="grid"><div class="a">A</div><div class="b">B</div><div class="c">C</div></div></body></html>`)
	add("grid", "Grid Column Span", `<!DOCTYPE html><html><style>.grid{display:grid;grid-template-columns:repeat(3, 1fr);}.span{grid-column:span 2;background:red;}</style><body><div class="grid"><div class="span">Span 2</div><div>1</div></div></body></html>`)
	add("grid", "Grid Row Span", `<!DOCTYPE html><html><style>.grid{display:grid;grid-template-columns:1fr 1fr;grid-auto-rows:50px;}.span{grid-row:span 2;background:red;}</style><body><div class="grid"><div class="span">Span 2</div><div>1</div><div>2</div></div></body></html>`)
	add("grid", "Grid Auto Flow Column", `<!DOCTYPE html><html><style>.grid{display:grid;grid-auto-flow:column;grid-template-rows:50px 50px;}.item{border:1px solid black;}</style><body><div class="grid"><div class="item">1</div><div class="item">2</div><div class="item">3</div></div></body></html>`)
	add("grid", "Grid Justify Items", `<!DOCTYPE html><html><style>.grid{display:grid;justify-items:center;}.item{border:1px solid black;width:50px;}</style><body><div class="grid"><div class="item">Center</div></div></body></html>`)
	add("grid", "Grid Align Items", `<!DOCTYPE html><html><style>.grid{display:grid;align-items:center;height:100px;}.item{border:1px solid black;height:50px;}</style><body><div class="grid"><div class="item">Center</div></div></body></html>`)
	add("grid", "Grid Template Fractions", `<!DOCTYPE html><html><style>.grid{display:grid;grid-template-columns:1fr 2fr;}.item{border:1px solid black;}</style><body><div class="grid"><div class="item">1fr</div><div class="item">2fr</div></div></body></html>`)
	add("grid", "Grid MinMax", `<!DOCTYPE html><html><style>.grid{display:grid;grid-template-columns:minmax(100px, 1fr);}.item{border:1px solid black;}</style><body><div class="grid"><div class="item">Min 100px</div></div></body></html>`)

	// 51-60: Forms
	add("forms", "Input Types", `<!DOCTYPE html><html><body><input type="text" value="Text"><input type="password" value="Pass"><input type="checkbox" checked><input type="radio" checked><input type="submit"></body></html>`)
	add("forms", "Textarea", `<!DOCTYPE html><html><body><textarea>Multi-line text area</textarea></body></html>`)
	add("forms", "Select Box", `<!DOCTYPE html><html><body><select><option>Option 1</option><option selected>Option 2</option></select></body></html>`)
	add("forms", "Button Styles", `<!DOCTYPE html><html><body><button>Default</button><button style="background:red;color:white;border:none;padding:10px;">Styled</button></body></html>`)
	add("forms", "Label Association", `<!DOCTYPE html><html><body><label for="inp">Label</label><input id="inp" type="text"></body></html>`)
	add("forms", "Fieldset Legend", `<!DOCTYPE html><html><body><fieldset><legend>Group</legend><input type="text"></fieldset></body></html>`)
	add("forms", "Form Layout", `<!DOCTYPE html><html><style>.row{margin-bottom:10px;}</style><body><div class="row"><label>Name:</label><input type="text"></div><div class="row"><label>Email:</label><input type="email"></div></body></html>`)
	add("forms", "Input Attributes", `<!DOCTYPE html><html><body><input type="text" placeholder="Placeholder" disabled readonly required></body></html>`)
	add("forms", "Color Input", `<!DOCTYPE html><html><body><input type="color" value="#ff0000"></body></html>`)
	add("forms", "Date Input", `<!DOCTYPE html><html><body><input type="date" value="2023-01-01"></body></html>`)

	// 61-70: Multimedia (Using inline SVGs or colors instead of external URLs to avoid timeout/network issues)
	add("media", "Image Basic", `<!DOCTYPE html><html><body><div style="width:150px;height:150px;background:red;">Placeholder</div></body></html>`)
	add("media", "Image Scaling", `<!DOCTYPE html><html><body><div style="width:50px;height:50px;background:blue;">Scaled</div></body></html>`)
	add("media", "Image Object Fit", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;border:1px solid black;}.img{width:100%;height:100%;background:green;}</style><body><div class="box"><div class="img">Fit</div></div></body></html>`)
	add("media", "SVG Inline", `<!DOCTYPE html><html><body><svg width="100" height="100"><circle cx="50" cy="50" r="40" stroke="black" stroke-width="3" fill="red" /></svg></body></html>`)
	add("media", "Canvas Basic", `<!DOCTYPE html><html><body><canvas id="c" width="200" height="100" style="border:1px solid #000;"></canvas><script>var c=document.getElementById("c");var ctx=c.getContext("2d");ctx.fillStyle="#FF0000";ctx.fillRect(0,0,150,75);</script></body></html>`)
	add("media", "Video Tag", `<!DOCTYPE html><html><body><div style="width:320px;height:240px;background:gray;display:flex;align-items:center;justify-content:center;">Video Placeholder</div></body></html>`)
	add("media", "Audio Tag", `<!DOCTYPE html><html><body><div style="padding:10px;border:1px solid black;">Audio Controls Placeholder</div></body></html>`)
	add("media", "Iframe", `<!DOCTYPE html><html><body><iframe srcdoc="<p>Inside iframe</p>" width="200" height="200"></iframe></body></html>`)
	add("media", "Figure Caption", `<!DOCTYPE html><html><body><figure><div style="width:150px;height:150px;background:purple;"></div><figcaption>Fig.1 - Placeholder</figcaption></figure></body></html>`)
	add("media", "Map Area", `<!DOCTYPE html><html><body><div style="width:100px;height:100px;background:orange;">Map Area</div></body></html>`)

	// 71-80: Tables
	add("tables", "Table Basic", `<!DOCTYPE html><html><body><table border="1"><tr><th>H1</th><th>H2</th></tr><tr><td>D1</td><td>D2</td></tr></table></body></html>`)
	add("tables", "Table Colspan", `<!DOCTYPE html><html><body><table border="1"><tr><td colspan="2">Span 2</td></tr><tr><td>1</td><td>2</td></tr></table></body></html>`)
	add("tables", "Table Rowspan", `<!DOCTYPE html><html><body><table border="1"><tr><td rowspan="2">Span 2</td><td>1</td></tr><tr><td>2</td></tr></table></body></html>`)
	add("tables", "Table Styling", `<!DOCTYPE html><html><style>table{border-collapse:collapse;width:100%;}th,td{padding:8px;border:1px solid #ddd;}tr:nth-child(even){background:#f2f2f2;}</style><body><table><tr><th>Head</th></tr><tr><td>Row 1</td></tr><tr><td>Row 2</td></tr></table></body></html>`)
	add("tables", "Table Caption", `<!DOCTYPE html><html><body><table border="1"><caption>Monthly Savings</caption><tr><th>Month</th><th>Savings</th></tr></table></body></html>`)
	add("tables", "Table Colgroup", `<!DOCTYPE html><html><body><table border="1"><colgroup><col style="background-color:yellow"></colgroup><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>2</td></tr></table></body></html>`)
	add("tables", "Table Nested", `<!DOCTYPE html><html><body><table border="1"><tr><td>Outer</td><td><table border="1"><tr><td>Inner</td></tr></table></td></tr></table></body></html>`)
	add("tables", "Table Layout Fixed", `<!DOCTYPE html><html><style>table{table-layout:fixed;width:200px;}td{overflow:hidden;}</style><body><table border="1"><tr><td>VeryLongContentThatMightOverflow</td></tr></table></body></html>`)
	add("tables", "Table Vertical Align", `<!DOCTYPE html><html><style>td{height:100px;vertical-align:bottom;}</style><body><table border="1"><tr><td>Bottom</td></tr></table></body></html>`)
	add("tables", "Table Empty Cells", `<!DOCTYPE html><html><style>table{empty-cells:hide;}</style><body><table border="1"><tr><td>A</td><td></td></tr><tr><td>C</td><td>D</td></tr></table></body></html>`)

	// 81-90: CSS Pseudo-classes & Elements
	add("css_advanced", "Hover State", `<!DOCTYPE html><html><style>button:hover{background:red;}</style><body><button>Hover Me</button></body></html>`)
	add("css_advanced", "First Child", `<!DOCTYPE html><html><style>li:first-child{color:red;}</style><body><ul><li>Red</li><li>Black</li></ul></body></html>`)
	add("css_advanced", "Last Child", `<!DOCTYPE html><html><style>li:last-child{color:red;}</style><body><ul><li>Black</li><li>Red</li></ul></body></html>`)
	add("css_advanced", "Nth Child", `<!DOCTYPE html><html><style>li:nth-child(2n){color:red;}</style><body><ul><li>1</li><li>2 (Red)</li><li>3</li><li>4 (Red)</li></ul></body></html>`)
	add("css_advanced", "Pseudo Elements Before", `<!DOCTYPE html><html><style>p::before{content:"Note: ";color:red;}</style><body><p>This is important.</p></body></html>`)
	add("css_advanced", "Pseudo Elements After", `<!DOCTYPE html><html><style>p::after{content:" (End)";color:blue;}</style><body><p>Sentence.</p></body></html>`)
	add("css_advanced", "Attribute Selector", `<!DOCTYPE html><html><style>input[type="text"]{background:yellow;}</style><body><input type="text"><input type="password"></body></html>`)
	add("css_advanced", "Combinator Child", `<!DOCTYPE html><html><style>div > p{color:red;}</style><body><div><p>Direct Child (Red)</p><span><p>Grandchild (Black)</p></span></div></body></html>`)
	add("css_advanced", "Combinator Adjacent", `<!DOCTYPE html><html><style>h1 + p{color:red;}</style><body><h1>Head</h1><p>Adjacent (Red)</p><p>Next (Black)</p></body></html>`)
	add("css_advanced", "Combinator Sibling", `<!DOCTYPE html><html><style>h1 ~ p{color:red;}</style><body><h1>Head</h1><p>Sibling (Red)</p><p>Sibling (Red)</p></body></html>`)

	// 91-100: Complex & Edge Cases
	add("edge_cases", "Deep Nesting", `<!DOCTYPE html><html><body><div><div><div><div><div><div><div><div><p>Deep</p></div></div></div></div></div></div></div></div></body></html>`)
	add("edge_cases", "Long Text No Space", `<!DOCTYPE html><html><body><div style="width:100px;border:1px solid black;word-wrap:break-word;">Supercalifragilisticexpialidocious</div></body></html>`)
	add("edge_cases", "Mixed Direction RTL", `<!DOCTYPE html><html><body><p>LTR text <span dir="rtl">טקסט בעברית</span> LTR text.</p></body></html>`)
	add("edge_cases", "Transparency Opacity", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background:red;}.trans{opacity:0.5;}</style><body><div class="box">Normal</div><div class="box trans">50%</div></body></html>`)
	add("edge_cases", "Transform Rotate", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background:red;transform:rotate(45deg);margin:50px;}</style><body><div class="box">Rotated</div></body></html>`)
	add("edge_cases", "Transform Scale", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background:red;transform:scale(0.5);}</style><body><div class="box">Scaled</div></body></html>`)
	add("edge_cases", "CSS Variables", `<!DOCTYPE html><html><style>:root{--main-color:blue;}p{color:var(--main-color);}</style><body><p>Blue via Var</p></body></html>`)
	add("edge_cases", "Calc Function", `<!DOCTYPE html><html><style>.box{width:calc(100% - 50px);border:1px solid black;}</style><body><div class="box">Calc Width</div></body></html>`)
	add("edge_cases", "Viewport Units", `<!DOCTYPE html><html><style>.box{width:50vw;height:50vh;background:pink;}</style><body><div class="box">Viewport Units</div></body></html>`)
	add("edge_cases", "Gradient Background", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background:linear-gradient(to right, red, yellow);}</style><body><div class="box"></div></body></html>`)

	// 101-105: Background Styling
	add("background", "Background Color Named", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background-color:tomato;border:1px solid black;}</style><body><div class="box">Tomato</div></body></html>`)
	add("background", "Background Color Hex", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background-color:#3366cc;border:1px solid black;}</style><body><div class="box">Hex</div></body></html>`)
	add("background", "Background Shorthand Color", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background:rgb(120,200,80);border:1px solid black;}</style><body><div class="box">RGB</div></body></html>`)
	add("background", "Transparent Background", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background:transparent;border:1px solid black;}</style><body><div class="box">Transparent</div></body></html>`)
	add("background", "Background Color Inheritance", `<!DOCTYPE html><html><style>.parent{background-color:yellow;}.child{background-color:inherit;}</style><body><div class="parent">Parent<div class="child">Child</div></div></body></html>`)

	// 106-110: Border Radius
	add("border_radius", "All Corners Rounded", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background:blue;border-radius:10px;}</style><body><div class="box">10px</div></body></html>`)
	add("border_radius", "Circle 50 Percent", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background:red;border-radius:50%;}</style><body><div class="box">Circle</div></body></html>`)
	add("border_radius", "Large Radius", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background:green;border-radius:50px;}</style><body><div class="box">50px</div></body></html>`)
	add("border_radius", "Mixed Values", `<!DOCTYPE html><html><style>.box{width:120px;height:80px;background:orange;border-radius:10px 30px;}</style><body><div class="box">Mixed</div></body></html>`)
	add("border_radius", "Rounded with Border", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;background:white;border:3px solid black;border-radius:15px;}</style><body><div class="box">With Border</div></body></html>`)

	// 111-115: Text Decoration & Transform
	add("text_style", "Text Decoration Underline", `<!DOCTYPE html><html><body><p style="text-decoration:underline">Underlined text</p></body></html>`)
	add("text_style", "Text Decoration Line-Through", `<!DOCTYPE html><html><body><p style="text-decoration:line-through">Strikethrough text</p></body></html>`)
	add("text_style", "Text Transform Uppercase", `<!DOCTYPE html><html><body><p style="text-transform:uppercase">lowercase becomes upper</p></body></html>`)
	add("text_style", "Text Transform Capitalize", `<!DOCTYPE html><html><body><p style="text-transform:capitalize">capitalize each word</p></body></html>`)
	add("text_style", "Vertical Alignment", `<!DOCTYPE html><html><body><p>Baseline <span style="vertical-align:super">super</span> and <span style="vertical-align:sub">sub</span> text</p></body></html>`)

	// 116-120: HTML5 Semantic Elements
	add("semantic", "Article Element", `<!DOCTYPE html><html><body><article style="border:1px solid black;padding:10px;"><h2>Article Title</h2><p>Article content goes here.</p></article></body></html>`)
	add("semantic", "Section Element", `<!DOCTYPE html><html><body><section style="background:#eee;padding:10px;"><h2>Section Heading</h2><p>Section content.</p></section></body></html>`)
	add("semantic", "Nav Element", `<!DOCTYPE html><html><body><nav style="background:#333;color:white;padding:10px;"><a href="#" style="color:white;">Home</a> <a href="#" style="color:white;">About</a></nav></body></html>`)
	add("semantic", "Header Footer Main", `<!DOCTYPE html><html><body><header style="background:#333;color:white;padding:10px;">Site Header</header><main style="padding:20px;">Main Content</main><footer style="background:#666;color:white;padding:10px;">Site Footer</footer></body></html>`)
	add("semantic", "Aside and Address", `<!DOCTYPE html><html><body><main><p>Article text</p><aside style="background:#ffe;padding:10px;">Sidebar note</aside></main><address>Contact: test@example.com</address></body></html>`)

	// 121-125: Color Values
	add("colors", "Color Inheritance", `<!DOCTYPE html><html><style>.parent{color:red;}.child{color:inherit;}</style><body><div class="parent">Red parent<span class="child">Red child</span></div></body></html>`)
	add("colors", "Color Keywords", `<!DOCTYPE html><html><body><p style="color:red">Red</p><p style="color:blue">Blue</p><p style="color:green">Green</p></body></html>`)
	add("colors", "Hex Short Color", `<!DOCTYPE html><html><body><p style="color:#f00">#f00 Red</p><p style="color:#0f0">#0f0 Green</p><p style="color:#00f">#00f Blue</p></body></html>`)
	add("colors", "RGB Color", `<!DOCTYPE html><html><body><p style="color:rgb(255,128,0)">Orange RGB</p><p style="color:rgb(0,128,128)">Teal RGB</p></body></html>`)
	add("colors", "Opacity Layered", `<!DOCTYPE html><html><style>.red{width:100px;height:100px;background:red;}.blue{width:100px;height:100px;background:blue;opacity:0.5;}</style><body><div class="red"></div><div class="blue"></div></body></html>`)

	// 126-130: Box Sizing, Cursor, Outline
	add("misc", "Box Sizing Border-Box", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;padding:10px;border:5px solid black;box-sizing:border-box;}</style><body><div class="box">Border-Box</div></body></html>`)
	add("misc", "Box Sizing Content-Box", `<!DOCTYPE html><html><style>.box{width:100px;height:100px;padding:10px;border:5px solid black;box-sizing:content-box;}</style><body><div class="box">Content-Box</div></body></html>`)
	add("misc", "Cursor Pointer", `<!DOCTYPE html><html><body><button style="cursor:pointer;padding:10px;">Pointer</button><button style="cursor:text;padding:10px;">Text</button></body></html>`)
	add("misc", "Outline Style", `<!DOCTYPE html><html><body><button style="outline:2px solid red;padding:10px;">Outlined</button></body></html>`)
	add("misc", "Text Overflow Ellipsis", `<!DOCTYPE html><html><style>.box{width:100px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;border:1px solid black;}</style><body><div class="box">Long text that should be truncated with ellipsis</div></body></html>`)

	return cases
}
