package engine_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/ax"
	"github.com/vyquocvu/goosie/internal/engine"
)

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

// qc:BR-139
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

// qc:BR-139
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

// qc:BR-139
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

// qc:BR-139
func TestAccessibilityButtonLabel(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	btn := axFind(s.AccessibilityTree(), ax.RoleButton, "Click me")
	if btn == nil {
		t.Fatal("button node not found")
	}
}

// qc:BR-139
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

func TestAccessibilityElementInternalsRoleAndLabelledByElements(t *testing.T) {
	s, _ := newJSSession(t, `<html><body><span id="label">Open details</span><span id="other-label">Host label</span><x-control id="control" style="display:block;width:100px;height:20px">Visible fallback</x-control><div id="shadow-host"></div><x-shadow-control id="shadow-control" style="display:block;width:100px;height:20px"></x-shadow-control><script>
		class XControl extends HTMLElement {
			constructor() {
				super();
				const internals = this.attachInternals();
				internals.role = "button";
				internals.ariaLabelledByElements = [document.getElementById("label")];
			}
		}
		customElements.define("x-control", XControl);
		const root = document.getElementById("shadow-host").attachShadow({mode: "open"});
		root.innerHTML = '<span id="shadow-label">Invalid cross-root name</span>';
		const shadowLabel = root.getElementById("shadow-label");
		customElements.define("x-shadow-control", class extends HTMLElement {
			constructor() {
				super();
				const internals = this.attachInternals();
				internals.role = "button";
				internals.ariaLabelledByElements = [shadowLabel];
			}
		});
	</script></body></html>`)
	control := axFind(s.AccessibilityTree(), ax.RoleButton, "Open details")
	if control == nil || control.ID != "control" {
		state, err := s.EvaluateJS(`document.getElementById("control").constructor.name + "|" + document.getElementById("control").getAttribute("style")`)
		t.Fatalf("ElementInternals button node not found: state=%q err=%v tree=%+v", state, err, s.AccessibilityTree())
	}
	if _, err := s.EvaluateJS(`document.getElementById("control").ariaLabelledByElements = [document.getElementById("other-label")]`); err != nil {
		t.Fatalf("set host ariaLabelledByElements: %v", err)
	}
	if control := axFind(s.AccessibilityTree(), ax.RoleButton, "Host label"); control == nil {
		t.Fatalf("host ariaLabelledByElements did not override internals: %+v", s.AccessibilityTree())
	}
	if _, err := s.EvaluateJS(`document.getElementById("control").setAttribute("aria-labelledby", "missing-reference")`); err != nil {
		t.Fatalf("set invalid host aria-labelledby: %v", err)
	}
	if control := axFind(s.AccessibilityTree(), ax.RoleButton, ""); control == nil {
		t.Fatalf("invalid host aria-labelledby did not suppress internals name: %+v", s.AccessibilityTree())
	}
	for _, node := range axFlatten(s.AccessibilityTree()) {
		if node.ID == "shadow-control" {
			if node.Role != ax.RoleButton || node.Label != "" {
				t.Fatalf("cross-root ElementInternals reference resolved as a name: %+v", node)
			}
			return
		}
	}
	t.Fatalf("cross-root ElementInternals control not found: %+v", s.AccessibilityTree())
}

func TestAccessibilityEmptyCustomElementWithoutLayout(t *testing.T) {
	s, _ := newJSSession(t, `<html><body><x-empty id="empty"></x-empty><script>
		customElements.define("x-empty", class extends HTMLElement {
			constructor() {
				super();
				const internals = this.attachInternals();
				internals.role = "button";
				internals.ariaLabel = "Empty action";
			}
		});
	</script></body></html>`)
	for _, node := range axFlatten(s.AccessibilityTree()) {
		if node.DOMNodeID == 0 || node.ID != "empty" {
			continue
		}
		if node.RoleName != "button" || node.Label != "Empty action" {
			t.Fatalf("empty custom element accessibility info = %+v", node)
		}
		if node.X0 != node.X1 || node.Y0 != node.Y1 {
			t.Fatalf("empty custom element unexpectedly has layout bounds: %+v", node)
		}
		return
	}
	t.Fatalf("empty custom element missing from accessibility tree: %+v", s.AccessibilityTree())
}

// qc:BR-139
func TestAccessibilityContentRoleTokensAndARIAName(t *testing.T) {
	s, _ := newJSSession(t, `<html><body>
		<span id="name">Referenced name</span>
		<button id="override" role="unsupported link" aria-labelledby="name">Native name</button>
		<button id="aria-label" aria-label="Explicit name">Fallback name</button>
	</body></html>`)
	link := axFind(s.AccessibilityTree(), ax.RoleLink, "Referenced name")
	if link == nil || link.ID != "override" {
		t.Fatalf("content role or aria-labelledby not applied: %+v", s.AccessibilityTree())
	}
	button := axFind(s.AccessibilityTree(), ax.RoleButton, "Explicit name")
	if button == nil || button.ID != "aria-label" {
		t.Fatalf("aria-label did not override native button text: %+v", s.AccessibilityTree())
	}
}

// qc:BR-139
func TestAccessibilityTextareaValue(t *testing.T) {
	s := newFocusSession(t, axRichHTML)
	for _, n := range axFlatten(s.AccessibilityTree()) {
		if n.Role == ax.RoleTextField && n.Value == "line one" {
			return
		}
	}
	t.Fatalf("textarea node with value %q not found", "line one")
}

// qc:BR-139
func TestAccessibilityHeadingRolesAndLabels(t *testing.T) {
	s, _ := newJSSession(t, `<html><body style="margin: 0;">
<h1 id="h1">Main title</h1>
<h2 id="h2">Section <a href="/details">details</a></h2>
<h3 id="h3" aria-label="Custom heading">Ignored text</h3>
<div id="h4" role="heading" aria-level="4">ARIA heading</div>
</body></html>`)
	tree := s.AccessibilityTree()
	main := axFind(tree, ax.RoleHeading, "Main title")
	if main == nil || main.ID != "h1" || main.RoleName != "heading" {
		t.Fatalf("h1 heading missing: %+v", axFlatten(tree))
	}
	section := axFind(tree, ax.RoleHeading, "Section details")
	if section == nil || section.ID != "h2" {
		t.Fatalf("h2 heading missing: %+v", axFlatten(tree))
	}
	if link := axFind(section.Children, ax.RoleLink, "details"); link == nil || link.Href != "/details" {
		t.Fatalf("heading link child = %+v, want details -> /details", section.Children)
	}
	custom := axFind(tree, ax.RoleHeading, "Custom heading")
	if custom == nil || custom.ID != "h3" {
		t.Fatalf("aria-label did not rename the heading: %+v", axFlatten(tree))
	}
	aria := axFind(tree, ax.RoleHeading, "ARIA heading")
	if aria == nil || aria.ID != "h4" {
		t.Fatalf("role=heading element missing: %+v", axFlatten(tree))
	}
	headings := 0
	for _, n := range axFlatten(tree) {
		if n.Role == ax.RoleHeading {
			headings++
		}
	}
	if headings != 4 {
		t.Fatalf("heading nodes = %d, want 4: %+v", headings, axFlatten(tree))
	}
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
		searchTagNode = axFind(tree, ax.RoleSearch, "")
	}
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
	if combo.Value != "Second choice" {
		t.Fatalf("combo value = %q, want the selected option's display string", combo.Value)
	}
	if len(combo.Children) != 3 {
		t.Fatalf("combo has %d option children, want 3: %+v", len(combo.Children), combo.Children)
	}
	selected := 0
	for _, label := range []string{"First choice", "Second choice", "Third choice"} {
		if option := axFind(combo.Children, ax.RoleOption, label); option == nil {
			t.Errorf("option %q not found in combo children: %+v", label, combo.Children)
		} else if label == "Second choice" {
			if option.Value != "" || !option.Selected {
				t.Errorf("selected option = %+v, want the name and selected=true with no value", option)
			}
		} else if option.Value != "" {
			t.Errorf("option %q announces value %q, want none at all", label, option.Value)
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
	combo := comboByID(tree, "sel")
	if combo == nil {
		t.Fatalf("select combo box not found: %+v", tree)
	}
	if combo.Label != "" {
		t.Fatalf("combo name = %q, want the empty name Chromium gives a select nothing labels", combo.Label)
	}
	if combo.Value != "Alpha" {
		t.Fatalf("combo value = %q, want the first option's display string", combo.Value)
	}
	first := axFind(combo.Children, ax.RoleOption, "Alpha")
	second := axFind(combo.Children, ax.RoleOption, "Beta")
	if first == nil || !first.Selected || second == nil || second.Selected {
		t.Fatalf("default selected state wrong: first=%+v second=%+v", first, second)
	}
}

func TestAccessibilityDisabledSelectAndOption(t *testing.T) {
	s := newFocusSession(t, `<html><body><select id="sel" disabled><option value="x" disabled>Unavailable</option></select></body></html>`)
	combo := comboByID(s.AccessibilityTree(), "sel")
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
	combo := comboByID(s.AccessibilityTree(), "sel")
	if combo == nil {
		t.Fatalf("select combo box not found after MoveSelect: %+v", s.AccessibilityTree())
	}
	if combo.Value != "Two" {
		t.Fatalf("combo value after MoveSelect = %q, want the display string Two", combo.Value)
	}
	selected := 0
	for _, option := range combo.Children {
		if option.Selected {
			selected++
			if option.Label != "Two" || option.Value != "" {
				t.Fatalf("selected option after MoveSelect = %+v, want the name Two and no value", option)
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

// qc:BR-139
func TestAccessibilitySelectAnnouncesTheDisplayWhileTheFormSubmitsTheValue(t *testing.T) {
	cases := []struct{ name, options, announce, want string }{
		{"value attribute", `<option value="red">Red</option>
<option value="blue" selected>Blue</option>
<option>Charcoal</option>`, "Blue", "blue"},
		{"text content fallback", `<option>Scarlet</option>
<option selected>Cobalt</option>
<option value="ink">Ink</option>`, "Cobalt", "Cobalt"},
		{"label attribute spelling", `<option selected label="  My   Label  " value="sub">WWWWW</option>`,
			"My   Label", "sub"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newFocusSession(t, `<html><body><form id="f"><select id="sel" name="color">
`+tc.options+`
</select></form></body></html>`)
			combo := comboByID(axFlatten(s.AccessibilityTree()), "sel")
			if combo == nil {
				t.Fatal("select combo box missing from the accessibility tree")
			}
			if combo.Value != tc.announce {
				t.Fatalf("accessibility announces %q, want the display string %q", combo.Value, tc.announce)
			}
			submitted := ""
			for _, d := range engine.CollectFormData(s.Doc.ElementByID("f"), nil) {
				if d.Name == "color" {
					submitted = d.Value
				}
			}
			if submitted != tc.want {
				t.Errorf("submitted value = %q, want %q", submitted, tc.want)
			}
		})
	}
}
