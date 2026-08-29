package renderer

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// ============================================================================
// 1. Text Fragmentation Across Interleaved Inline Elements
// ============================================================================

func TestAdversarialTextFragmentationNestedInlines(t *testing.T) {
	// Deep nested inline elements with alternating formatting
	content := `<!DOCTYPE html><html><body>
		<p id="target">
			Alpha <b>Beta <i>Gamma <u>Delta</u> Epsilon</i> Zeta</b> Eta <a href="https://example.com/test">Theta <b>Iota</b></a> Kappa
		</p>
	</body></html>`

	r := NewRenderer(800, 600)
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	renderRoot := BuildRenderTree(findBodyNode(doc))
	layoutRoot := r.layoutEngine.ComputeLayout(renderRoot)

	dlb := NewDisplayListBuilder()
	displayList := dlb.Build(layoutRoot, renderRoot)

	var textRuns []string
	var linkRuns []string

	for _, cmd := range displayList.Commands {
		switch cmd.Type {
		case PaintText:
			textRuns = append(textRuns, cmd.Text)
		case PaintLink:
			if cmd.LinkText != "" {
				linkRuns = append(linkRuns, cmd.LinkText)
			}
		}
	}

	// Verify no text runs got accidentally merged across boundaries
	for _, run := range textRuns {
		if strings.Contains(run, "Alpha") && strings.Contains(run, "Beta") {
			t.Errorf("Alpha and Beta merged in text command: %s", run)
		}
		if strings.Contains(run, "Beta") && strings.Contains(run, "Gamma") {
			t.Errorf("Beta and Gamma merged in text command: %s", run)
		}
		if strings.Contains(run, "Gamma") && strings.Contains(run, "Delta") {
			t.Errorf("Gamma and Delta merged in text command: %s", run)
		}
		if strings.Contains(run, "Eta") && strings.Contains(run, "Theta") {
			t.Errorf("Eta and Theta merged in text command: %s", run)
		}
	}

	// Verify link runs are captured
	foundLinkText := false
	for _, l := range linkRuns {
		if strings.Contains(l, "Theta") || strings.Contains(l, "Iota") {
			foundLinkText = true
		}
	}
	if !foundLinkText && len(linkRuns) == 0 {
		t.Errorf("Expected link runs for Theta/Iota, got %v", linkRuns)
	}
}

func TestAdversarialTextFragmentationTightAdjacentSpans(t *testing.T) {
	// Adjacent inline spans with no intervening whitespace
	content := `<!DOCTYPE html><html><body>
		<p id="target"><span>Foo</span><span>Bar</span><span>Baz</span></p>
	</body></html>`

	r := NewRenderer(800, 600)
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	renderRoot := BuildRenderTree(findBodyNode(doc))
	layoutRoot := r.layoutEngine.ComputeLayout(renderRoot)

	dlb := NewDisplayListBuilder()
	displayList := dlb.Build(layoutRoot, renderRoot)

	var texts []string
	for _, cmd := range displayList.Commands {
		if cmd.Type == PaintText {
			texts = append(texts, cmd.Text)
		}
	}

	// Each span should have its own distinct text command
	hasFoo, hasBar, hasBaz := false, false, false
	for _, txt := range texts {
		if txt == "Foo" {
			hasFoo = true
		}
		if txt == "Bar" {
			hasBar = true
		}
		if txt == "Baz" {
			hasBaz = true
		}
	}
	if !hasFoo || !hasBar || !hasBaz {
		t.Errorf("Expected distinct text commands for Foo, Bar, Baz. Got: %v", texts)
	}
}

// ============================================================================
// 2. Cascade Specificity Ordering with Complex Selectors & !important
// ============================================================================

func TestAdversarialCascadeComplexRulesAndImportant(t *testing.T) {
	// 5 conflicting rules on #main .item[data-status="active"]
	// Rule 1: .item { color: black; font-size: 10px; }                         (spec: 0,1,0)
	// Rule 2: div.item[data-status="active"] { color: blue; font-size: 14px; } (spec: 0,2,1)
	// Rule 3: #main .item { color: green; font-size: 18px; }                   (spec: 1,1,0)
	// Rule 4: #main div.item[data-status="active"] { color: orange; font-size: 22px; } (spec: 1,2,1)
	// Rule 5: .item { font-size: 30px !important; }                            (spec: 0,1,0 with !important)
	// Expected result for target:
	// - color: orange (Rule 4 has highest specificity 1,2,1 among normal rules)
	// - font-size: 30px (!important in Rule 5 overrides higher specificity Rule 4)
	content := `<!DOCTYPE html><html><head><style>
		.item { color: black; font-size: 10px; }
		div.item[data-status="active"] { color: blue; font-size: 14px; }
		#main .item { color: green; font-size: 18px; }
		#main div.item[data-status="active"] { color: orange; font-size: 22px; }
		.item { font-size: 30px !important; }
	</style></head><body>
		<div id="main">
			<div class="item" data-status="active" id="target">Content</div>
		</div>
	</body></html>`

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	target := findNodeByID(renderTree, "target")
	if target == nil {
		t.Fatal("target node not found")
	}

	expectedColor := color.RGBA{R: 0xff, G: 0xa5, B: 0x00, A: 0xff} // orange
	if target.ComputedStyle.Color != expectedColor {
		t.Errorf("Expected color orange %v, got %v", expectedColor, target.ComputedStyle.Color)
	}

	if target.ComputedStyle.FontSize != 30.0 {
		t.Errorf("Expected font-size 30.0 (!important override), got %f", target.ComputedStyle.FontSize)
	}
}

func TestAdversarialCascadeMultipleImportantSpecificityTieBreak(t *testing.T) {
	// When multiple rules have !important for the same property:
	// Higher specificity !important should beat lower specificity !important.
	// Rule 1: .item { color: red !important; }                     (spec: 0,1,0)
	// Rule 2: #main .item { color: purple !important; }            (spec: 1,1,0)
	// Expected: purple (higher specificity !important wins)
	content := `<!DOCTYPE html><html><head><style>
		.item { color: red !important; }
		#main .item { color: purple !important; }
	</style></head><body>
		<div id="main">
			<div class="item" id="target">Content</div>
		</div>
	</body></html>`

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	target := findNodeByID(renderTree, "target")
	if target == nil {
		t.Fatal("target node not found")
	}

	expectedColor := color.RGBA{R: 0x80, G: 0x00, B: 0x80, A: 0xff} // purple
	if target.ComputedStyle.Color != expectedColor {
		t.Errorf("Expected higher specificity !important purple %v, got %v", expectedColor, target.ComputedStyle.Color)
	}
}

func TestAdversarialCascadeSourceOrderTieBreak(t *testing.T) {
	// When specificity is identical and both are normal:
	// Later rule in source order wins.
	content := `<!DOCTYPE html><html><head><style>
		.classA.classB { color: blue; }
		.classB.classA { color: green; }
	</style></head><body>
		<div class="classA classB" id="target">Content</div>
	</body></html>`

	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("html.Parse failed: %v", err)
	}

	stylesheet := extractAndParseCSS(doc)
	renderTree := BuildRenderTree(findBodyNode(doc))
	sm := NewStyleManager(stylesheet)
	sm.ApplyStyles(renderTree)

	target := findNodeByID(renderTree, "target")
	if target == nil {
		t.Fatal("target node not found")
	}

	expectedColor := color.RGBA{G: 0x80, A: 0xff} // green
	if target.ComputedStyle.Color != expectedColor {
		t.Errorf("Expected later rule green %v, got %v", expectedColor, target.ComputedStyle.Color)
	}
}

// ============================================================================
// 3. Table Layouts with Mixed Column Constraints
// ============================================================================

func TestAdversarialTableMixedColumnConstraints(t *testing.T) {
	// Table width: 1000px, 4 columns:
	// Col 1: Fixed px width="100px"
	// Col 2: Percentage width="20%" (20% of 1000 = 200px)
	// Col 3: Empty spacer cell width="50"
	// Col 4: Auto content column (remaining space: 1000 - 100 - 200 - 50 = 650px)
	content := `<!DOCTYPE html><html><body>
		<table width="1000" cellspacing="0" cellpadding="0">
			<tr>
				<td width="100px" id="col1">Fixed 100</td>
				<td width="20%" id="col2">Pct 200</td>
				<td width="50" id="col3"></td>
				<td id="col4">Auto expanding main article content</td>
			</tr>
		</table>
	</body></html>`

	r := NewRenderer(1000, 600)
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	renderRoot := BuildRenderTree(findBodyNode(doc))
	layoutRoot := r.layoutEngine.ComputeLayout(renderRoot)

	// Find the table layout box
	var tableBox *LayoutBox
	var findTable func(b *LayoutBox)
	findTable = func(b *LayoutBox) {
		if b == nil {
			return
		}
		if b.Display == DisplayGrid && len(b.Children) == 4 {
			tableBox = b
			return
		}
		for _, c := range b.Children {
			findTable(c)
		}
	}
	findTable(layoutRoot)

	if tableBox == nil {
		t.Fatal("table layout box not found")
	}

	cell1 := tableBox.Children[0]
	cell2 := tableBox.Children[1]
	cell3 := tableBox.Children[2]
	cell4 := tableBox.Children[3]

	if cell1.Box.Width != 100.0 {
		t.Errorf("Expected cell1 width 100.0, got %f", cell1.Box.Width)
	}
	if cell2.Box.Width != 200.0 {
		t.Errorf("Expected cell2 width 200.0 (20%% of 1000), got %f", cell2.Box.Width)
	}
	if cell3.Box.Width != 50.0 {
		t.Errorf("Expected cell3 width 50.0, got %f", cell3.Box.Width)
	}
	if cell4.Box.Width != 650.0 {
		t.Errorf("Expected cell4 auto width 650.0, got %f", cell4.Box.Width)
	}

	// Verify total columns width equals table width
	totalWidth := cell1.Box.Width + cell2.Box.Width + cell3.Box.Width + cell4.Box.Width
	if totalWidth != 1000.0 {
		t.Errorf("Expected sum of cell widths 1000.0, got %f", totalWidth)
	}
}

func TestAdversarialTableColspanWithSpacers(t *testing.T) {
	// Table with 2 rows:
	// Row 1: Header spanning 3 columns (colspan="3")
	// Row 2: Spacer 30px, Main 740px, Spacer 30px (total 800px)
	content := `<!DOCTYPE html><html><body>
		<table width="800" cellspacing="0" cellpadding="0">
			<tr>
				<td colspan="3" id="header-cell">Header banner spanning entire width</td>
			</tr>
			<tr>
				<td width="30">L</td>
				<td>Body Center</td>
				<td width="30">R</td>
			</tr>
		</table>
	</body></html>`

	r := NewRenderer(800, 600)
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	renderRoot := BuildRenderTree(findBodyNode(doc))
	layoutRoot := r.layoutEngine.ComputeLayout(renderRoot)

	var tableBox *LayoutBox
	var findTable func(b *LayoutBox)
	findTable = func(b *LayoutBox) {
		if b == nil {
			return
		}
		if b.Display == DisplayGrid && len(b.Children) == 4 {
			tableBox = b
			return
		}
		for _, c := range b.Children {
			findTable(c)
		}
	}
	findTable(layoutRoot)

	if tableBox == nil {
		t.Fatal("table layout box not found")
	}

	headerCell := tableBox.Children[0]
	leftSpacer := tableBox.Children[1]
	centerCell := tableBox.Children[2]
	rightSpacer := tableBox.Children[3]

	if headerCell.Box.Width != 800.0 {
		t.Errorf("Expected header cell width 800.0 (colspan=3), got %f", headerCell.Box.Width)
	}
	if leftSpacer.Box.Width != 30.0 {
		t.Errorf("Expected left spacer width 30.0, got %f", leftSpacer.Box.Width)
	}
	if rightSpacer.Box.Width != 30.0 {
		t.Errorf("Expected right spacer width 30.0, got %f", rightSpacer.Box.Width)
	}
	if centerCell.Box.Width != 740.0 {
		t.Errorf("Expected center cell width 740.0, got %f", centerCell.Box.Width)
	}
}

// ============================================================================
// 4. Golden Layout Determinism & Stress Reproducibility
// ============================================================================

func TestAdversarialLayoutDeterminismStress(t *testing.T) {
	// A complex composite layout combining flex, inline text runs, nested boxes, and styled tables
	htmlContent := `<!DOCTYPE html>
	<html>
	<head>
		<style>
			body { font-size: 16px; margin: 0; padding: 10px; }
			.nav { display: flex; flex-direction: row; width: 600px; }
			.nav-item { width: 100px; height: 30px; margin: 5px; }
			.main-table { width: 600px; }
			.main-table td { padding: 8px; }
			.text-block { width: 500px; line-height: 24px; }
		</style>
	</head>
	<body>
		<div class="nav">
			<div class="nav-item">Home</div>
			<div class="nav-item">Articles</div>
			<div class="nav-item">About</div>
		</div>
		<table class="main-table" width="600">
			<tr>
				<td width="100">Sidebar</td>
				<td>
					<div class="text-block">
						Hello <b>bold world</b> with <a href="https://example.com">links</a> and <i>italics</i>.
					</div>
				</td>
			</tr>
		</table>
	</body>
	</html>`

	r := NewRenderer(800, 600)
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	// Compute reference layout
	renderRoot1 := BuildRenderTree(findBodyNode(doc))
	sm1 := NewStyleManager(extractAndParseCSS(doc))
	sm1.ApplyStyles(renderRoot1)
	layoutRoot1 := r.layoutEngine.ComputeLayout(renderRoot1)
	dlb1 := NewDisplayListBuilder()
	dl1 := dlb1.Build(layoutRoot1, renderRoot1)

	var refCommands []string
	for _, cmd := range dl1.Commands {
		refCommands = append(refCommands, fmt.Sprintf("%v: %s at (%.1f, %.1f, %.1f, %.1f)", cmd.Type, cmd.Text, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height))
	}

	// Run 10 iterations and verify 100% byte-for-byte determinism
	for iter := 1; iter <= 10; iter++ {
		docIter, _ := html.Parse(strings.NewReader(htmlContent))
		renderRootIter := BuildRenderTree(findBodyNode(docIter))
		smIter := NewStyleManager(extractAndParseCSS(docIter))
		smIter.ApplyStyles(renderRootIter)
		layoutRootIter := r.layoutEngine.ComputeLayout(renderRootIter)
		dlbIter := NewDisplayListBuilder()
		dlIter := dlbIter.Build(layoutRootIter, renderRootIter)

		if len(dlIter.Commands) != len(dl1.Commands) {
			t.Fatalf("Iteration %d command count mismatch: got %d, want %d", iter, len(dlIter.Commands), len(dl1.Commands))
		}

		for idx, cmd := range dlIter.Commands {
			cmdStr := fmt.Sprintf("%v: %s at (%.1f, %.1f, %.1f, %.1f)", cmd.Type, cmd.Text, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height)
			if cmdStr != refCommands[idx] {
				t.Fatalf("Iteration %d non-deterministic command at [%d]: got '%s', want '%s'", iter, idx, cmdStr, refCommands[idx])
			}
		}
	}
}
