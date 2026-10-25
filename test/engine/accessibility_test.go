package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/engine"
)

// axFind returns the first node in the tree with the given role and label.
func axFind(nodes []ax.Node, role ax.Role, label string) *ax.Node {
	for i := range nodes {
		if nodes[i].Role == role && nodes[i].Label == label {
			return &nodes[i]
		}
		if got := axFind(nodes[i].Children, role, label); got != nil {
			return got
		}
	}
	return nil
}

func axFlatten(nodes []ax.Node) []ax.Node {
	var out []ax.Node
	for _, n := range nodes {
		out = append(out, n)
		out = append(out, axFlatten(n.Children)...)
	}
	return out
}

func TestAccessibilityDocumentRoot(t *testing.T) {
	s := newFocusSession(t, `<html><body><p>hi</p></body></html>`)
	tree := s.AccessibilityTree()
	if len(tree) != 1 {
		t.Fatalf("tree roots = %d, want 1 document", len(tree))
	}
	if tree[0].Role != ax.RoleDocument {
		t.Fatalf("root role = %d, want document", tree[0].Role)
	}
	if tree[0].X1 <= tree[0].X0 || tree[0].Y1 <= tree[0].Y0 {
		t.Fatalf("document bounds empty: %+v", tree[0])
	}
}

const axRichHTML = `<html><body style="margin: 0;">` +
	`<p>Hello <a href="https://example.com/docs" id="docs">goosie docs</a> world</p>` +
	`<img src="missing.png" alt="Go logo" width="40" height="40">` +
	`<button id="btn">Click me</button>` +
	`<input id="field" type="text" value="typed" placeholder="Your name">` +
	`<textarea id="area">line one</textarea>` +
	`</body></html>`

func TestAccessibilityLinkRoleAndHref(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	link := axFind(s.AccessibilityTree(), ax.RoleLink, "goosie docs")
	if link == nil {
		t.Fatal("link node not found")
	}
	if link.Href != "https://example.com/docs" {
		t.Fatalf("href = %q, want https://example.com/docs", link.Href)
	}
	if link.X1 <= link.X0 || link.Y1 <= link.Y0 {
		t.Fatalf("link bounds empty: %+v", link)
	}
}

func TestAccessibilityImageAlt(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	img := axFind(s.AccessibilityTree(), ax.RoleImage, "Go logo")
	if img == nil {
		t.Fatal("image node not found")
	}
	if img.X1 <= img.X0 || img.Y1 <= img.Y0 {
		t.Fatalf("image bounds empty: %+v", img)
	}
}

func TestAccessibilityButtonLabel(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	btn := axFind(s.AccessibilityTree(), ax.RoleButton, "Click me")
	if btn == nil {
		t.Fatal("button node not found")
	}
}

func TestAccessibilityTextFieldValue(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	field := axFind(s.AccessibilityTree(), ax.RoleTextField, "Your name")
	if field == nil {
		t.Fatal("text field node not found")
	}
	if field.Value != "typed" {
		t.Fatalf("value = %q, want typed", field.Value)
	}
}

func TestAccessibilityTextareaValue(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	for _, n := range axFlatten(s.AccessibilityTree()) {
		if n.Role == ax.RoleTextField && n.Value == "line one" {
			return
		}
	}
	t.Fatalf("textarea node with value %q not found", "line one")
}

func TestAccessibilityStaticTextParagraphs(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;"><p>First</p><p>Second</p></body></html>`)
	flat := axFlatten(s.AccessibilityTree())
	first := -1
	second := -1
	groups := 0
	for i, n := range flat {
		switch {
		case n.Role == ax.RoleStaticText && n.Label == "First":
			first = i
		case n.Role == ax.RoleStaticText && n.Label == "Second":
			second = i
		case n.Role == ax.RoleGroup:
			groups++
		}
	}
	if first < 0 || second < 0 {
		t.Fatalf("static text nodes missing: First=%d Second=%d in %+v", first, second, flat)
	}
	if first >= second {
		t.Fatalf("paragraph order wrong: First at %d, Second at %d", first, second)
	}
	if groups == 0 {
		t.Fatal("no group nodes around the blocks")
	}
}

func TestAccessibilityTextRunsAroundLink(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	flat := axFlatten(s.AccessibilityTree())
	hello := -1
	link := -1
	world := -1
	for i, n := range flat {
		switch {
		case n.Role == ax.RoleStaticText && n.Label == "Hello":
			hello = i
		case n.Role == ax.RoleLink && n.Label == "goosie docs":
			link = i
		case n.Role == ax.RoleStaticText && n.Label == "world":
			world = i
		}
	}
	if hello < 0 || link < 0 || world < 0 {
		t.Fatalf("missing runs: Hello=%d link=%d world=%d in %+v", hello, link, world, flat)
	}
	if !(hello < link && link < world) {
		t.Fatalf("runs out of document order: Hello=%d link=%d world=%d", hello, link, world)
	}
}

func TestAccessibilitySearchLandmark(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">
<search>
  <input type="text" placeholder="Search the web">
  <button>Find</button>
</search>
<div role="search">
  <input type="text" placeholder="Site search">
</div>
</body></html>`)

	tree := s.AccessibilityTree()
	searchTagNode := axFind(tree, ax.RoleSearch, "Search the web Find")
	if searchTagNode == nil {
		// Or label could be without button text or composite
		searchTagNode = axFind(tree, ax.RoleSearch, "")
	}
	// Check all search role nodes in flattened tree
	flat := axFlatten(tree)
	var searchNodes []ax.Node
	for _, n := range flat {
		if n.Role == ax.RoleSearch {
			searchNodes = append(searchNodes, n)
		}
	}
	if len(searchNodes) != 2 {
		t.Fatalf("found %d RoleSearch nodes, want 2 (one <search> and one role=search); tree: %+v", len(searchNodes), flat)
	}
}

func TestProgressBarAndMeterAccessibility(t *testing.T) {
	s := newFocusSession(t, `<html><body>
<label for="p">Loading</label>
<progress id="p" value="70" max="100"></progress>
<label for="m">Disk</label>
<meter id="m" value="0.6"></meter>
</body></html>`)

	tree := s.AccessibilityTree()
	flat := axFlatten(tree)

	var progNode, meterNode *ax.Node
	for i := range flat {
		if flat[i].Role == ax.RoleProgressBar {
			progNode = &flat[i]
		}
		if flat[i].Role == ax.RoleMeter {
			meterNode = &flat[i]
		}
	}

	if progNode == nil {
		t.Fatal("RoleProgressBar node not found in accessibility tree")
	}
	if progNode.Value != "70/100" {
		t.Errorf("progNode.Value = %q, want 70/100", progNode.Value)
	}

	if meterNode == nil {
		t.Fatal("RoleMeter node not found in accessibility tree")
	}
	if meterNode.Value != "0.6" {
		t.Errorf("meterNode.Value = %q, want 0.6", meterNode.Value)
	}
}

// qc:BR-139
func TestAccessibilitySelectComboBoxAndOptions(t *testing.T) {
	s := newFocusSession(t, `<html><body style="margin: 0;">
<label for="sel">Choose a value</label>
<select id="sel" name="choice">
  <option value="first">First choice</option>
  <option value="second" selected>Second choice</option>
  <option>Third choice</option>
</select></body></html>`)
	tree := s.AccessibilityTree()
	combo := axFind(tree, ax.RoleComboBox, "Choose a value")
	if combo == nil || combo.ID != "sel" {
		t.Fatalf("select combo box missing or has wrong ID: %+v", combo)
	}
	if combo.Value != "second" {
		t.Fatalf("combo value = %q, want selected form value second", combo.Value)
	}
	if len(combo.Children) != 3 {
		t.Fatalf("combo has %d option children, want 3: %+v", len(combo.Children), combo.Children)
	}
	selected := 0
	for _, label := range []string{"First choice", "Second choice", "Third choice"} {
		if option := axFind(combo.Children, ax.RoleOption, label); option == nil {
			t.Errorf("option %q not found in combo children: %+v", label, combo.Children)
		} else if label == "Second choice" {
			if option.Value != "second" || !option.Selected {
				t.Errorf("selected option = %+v, want value=second selected=true", option)
			}
		} else if label == "Third choice" && option.Value != "Third choice" {
			t.Errorf("option without value attribute has value %q, want its text", option.Value)
		}
	}
	for _, option := range combo.Children {
		if option.Role == ax.RoleOption && option.Selected {
			selected++
		}
	}
	if selected != 1 {
		t.Fatalf("selected options = %d, want exactly 1", selected)
	}
}

func TestAccessibilitySelectDefaultsToFirstOption(t *testing.T) {
	s := newFocusSession(t, `<html><body><select id="sel"><option value="alpha">Alpha</option><option value="beta">Beta</option></select></body></html>`)
	tree := s.AccessibilityTree()
	combo := axFind(tree, ax.RoleComboBox, "Alpha")
	if combo == nil {
		t.Fatalf("select combo box not found: %+v", tree)
	}
	if combo.Value != "alpha" {
		t.Fatalf("combo value = %q, want first option alpha", combo.Value)
	}
	first := axFind(combo.Children, ax.RoleOption, "Alpha")
	second := axFind(combo.Children, ax.RoleOption, "Beta")
	if first == nil || !first.Selected || second == nil || second.Selected {
		t.Fatalf("default selected state wrong: first=%+v second=%+v", first, second)
	}
}

func TestAccessibilityDisabledSelectAndOption(t *testing.T) {
	s := newFocusSession(t, `<html><body><select id="sel" disabled><option value="x" disabled>Unavailable</option></select></body></html>`)
	combo := axFind(s.AccessibilityTree(), ax.RoleComboBox, "Unavailable")
	if combo == nil || !combo.Disabled {
		t.Fatalf("disabled select node = %+v, want present and disabled", combo)
	}
	option := axFind(combo.Children, ax.RoleOption, "Unavailable")
	if option == nil || !option.Disabled {
		t.Fatalf("disabled option node = %+v, want present and disabled", option)
	}
}

// qc:BR-139
func TestAccessibilitySelectTracksMoveSelect(t *testing.T) {
	s := newFocusSession(t, `<html><body><select id="sel"><option value="one" selected>One</option><option value="two">Two</option><option value="three">Three</option></select></body></html>`)
	sel := s.Doc.ElementByID("sel")
	if !s.FocusNode(sel) || !s.MoveSelect(engine.EditDown) {
		t.Fatal("could not focus select and move to the next option")
	}
	combo := axFind(s.AccessibilityTree(), ax.RoleComboBox, "Two")
	if combo == nil {
		t.Fatalf("select combo box not found after MoveSelect: %+v", s.AccessibilityTree())
	}
	if combo.Value != "two" {
		t.Fatalf("combo value after MoveSelect = %q, want two", combo.Value)
	}
	selected := 0
	for _, option := range combo.Children {
		if option.Selected {
			selected++
			if option.Label != "Two" || option.Value != "two" {
				t.Fatalf("selected option after MoveSelect = %+v, want Two/two", option)
			}
		}
	}
	if selected != 1 {
		t.Fatalf("selected option count after MoveSelect = %d, want 1", selected)
	}
}

func TestAccessibilityEmptySelectHasNoOptions(t *testing.T) {
	s := newFocusSession(t, `<html><body><select id="sel"></select></body></html>`)
	flat := axFlatten(s.AccessibilityTree())
	var combo *ax.Node
	for i := range flat {
		if flat[i].Role == ax.RoleComboBox && flat[i].ID == "sel" {
			combo = &flat[i]
			break
		}
	}
	if combo == nil {
		t.Fatal("empty select combo box missing")
	}
	if len(combo.Children) != 0 {
		t.Fatalf("empty select has %d children, want none", len(combo.Children))
	}
}
