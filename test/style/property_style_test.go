package style_test

import (
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
	"github.com/vyquocvu/goosie/internal/js"
	"github.com/vyquocvu/goosie/internal/style"
	"github.com/vyquocvu/goosie/test/domtest"
)

func TestStyle_PropertyInheritsFalse(t *testing.T) {
	html := `<!DOCTYPE html>
	<html>
	<head>
		<style>
			@property --not-inherited {
				syntax: '<color>';
				inherits: false;
				initial-value: red;
			}
			@property --do-inherit {
				syntax: '<color>';
				inherits: true;
				initial-value: green;
			}
			.parent {
				--not-inherited: blue;
				--do-inherit: blue;
				--unregistered: blue;
			}
		</style>
	</head>
	<body>
		<div id="parent" class="parent">
			<div id="child"></div>
		</div>
	</body>
	</html>`

	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	for _, node := range doc.ElementsByTagName("style") {
		sheets = append(sheets, css.Parse(node.TextContent()))
	}

	styles := style.ResolveViewport(doc, sheets, style.Viewport{}, nil)

	parent := doc.ElementByID("parent")
	child := doc.ElementByID("child")

	parentStyle := styles[parent.ID]
	childStyle := styles[child.ID]

	// On parent, all three are blue
	if parentStyle.Custom["--not-inherited"] != "blue" {
		t.Errorf("parent --not-inherited = %q, want 'blue'", parentStyle.Custom["--not-inherited"])
	}
	if parentStyle.Custom["--do-inherit"] != "blue" {
		t.Errorf("parent --do-inherit = %q, want 'blue'", parentStyle.Custom["--do-inherit"])
	}
	if parentStyle.Custom["--unregistered"] != "blue" {
		t.Errorf("parent --unregistered = %q, want 'blue'", parentStyle.Custom["--unregistered"])
	}

	// On child:
	// --not-inherited has inherits: false, so it must NOT inherit blue! It should be initial-value red!
	if childStyle.Custom["--not-inherited"] != "red" {
		t.Errorf("child --not-inherited = %q, want 'red' (initial-value because inherits: false)", childStyle.Custom["--not-inherited"])
	}
	// --do-inherit has inherits: true, so it inherits blue from parent
	if childStyle.Custom["--do-inherit"] != "blue" {
		t.Errorf("child --do-inherit = %q, want 'blue'", childStyle.Custom["--do-inherit"])
	}
	// --unregistered inherits by standard CSS default
	if childStyle.Custom["--unregistered"] != "blue" {
		t.Errorf("child --unregistered = %q, want 'blue'", childStyle.Custom["--unregistered"])
	}
}

func TestStyle_PropertySyntaxValidationAndFallback(t *testing.T) {
	html := `<!DOCTYPE html>
	<html>
	<head>
		<style>
			@property --size {
				syntax: '<length>';
				inherits: true;
				initial-value: 20px;
			}
			.valid {
				--size: 50px;
				width: var(--size);
			}
			.invalid {
				--size: red; /* invalid for <length>, must fallback to initial 20px */
				width: var(--size);
			}
			.undeclared {
				width: var(--size); /* undeclared must resolve to initial 20px */
			}
		</style>
	</head>
	<body>
		<div id="v" class="valid"></div>
		<div id="inv" class="invalid"></div>
		<div id="und" class="undeclared"></div>
	</body>
	</html>`

	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	for _, node := range doc.ElementsByTagName("style") {
		sheets = append(sheets, css.Parse(node.TextContent()))
	}

	styles := style.ResolveViewport(doc, sheets, style.Viewport{}, nil)

	v := doc.ElementByID("v")
	inv := doc.ElementByID("inv")
	und := doc.ElementByID("und")

	if styles[v.ID].Width != 50 {
		t.Errorf("valid width = %v, want 50", styles[v.ID].Width)
	}

	if styles[inv.ID].Width != 20 {
		t.Errorf("invalid width = %v, want 20 (fallback to initial-value)", styles[inv.ID].Width)
	}

	if styles[und.ID].Width != 20 {
		t.Errorf("undeclared width = %v, want 20 (initial-value)", styles[und.ID].Width)
	}
}

func TestStyle_JSRegisterProperty(t *testing.T) {
	html := `<!DOCTYPE html>
	<html>
	<head>
		<style>
			.box {
				color: var(--js-color);
			}
		</style>
	</head>
	<body>
		<div id="target" class="box"></div>
	</body>
	</html>`

	doc := domtest.Parse(html)
	rt, err := js.New(js.Options{DOM: doc})
	if err != nil {
		t.Fatalf("js.New failed: %v", err)
	}
	defer rt.Close()

	// Register property via JS CSS.registerProperty
	err = rt.Run(`
		CSS.registerProperty({
			name: '--js-color',
			syntax: '<color>',
			inherits: false,
			initialValue: '#00ff00'
		});
	`, "test.js")
	if err != nil {
		t.Fatalf("CSS.registerProperty failed: %v", err)
	}

	var sheets []*css.Stylesheet
	for _, node := range doc.ElementsByTagName("style") {
		sheets = append(sheets, css.Parse(node.TextContent()))
	}

	styles := style.ResolveViewport(doc, sheets, style.Viewport{}, nil)
	target := doc.ElementByID("target")
	targetStyle := styles[target.ID]

	// Color should have resolved to #00ff00 (green: R=0, G=255, B=0, A=255)
	wantColor := css.Color{R: 0, G: 255, B: 0, A: 255}
	if targetStyle.Color != wantColor {
		t.Errorf("target color = %+v, want %+v (#00ff00 from JS registered property)", targetStyle.Color, wantColor)
	}
}

func TestStyle_StartingStyle(t *testing.T) {
	html := `<!DOCTYPE html>
	<html>
	<head>
		<style>
			dialog {
				opacity: 1;
				display: block;
			}
			@starting-style {
				dialog {
					opacity: 0;
					display: none;
				}
			}

			.card {
				opacity: 1;
				@starting-style {
					opacity: 0.2;
				}
			}
		</style>
	</head>
	<body>
		<dialog id="dlg"></dialog>
		<div id="c" class="card"></div>
	</body>
	</html>`

	doc := domtest.Parse(html)
	var sheets []*css.Stylesheet
	for _, node := range doc.ElementsByTagName("style") {
		sheets = append(sheets, css.Parse(node.TextContent()))
	}

	styles := style.ResolveViewport(doc, sheets, style.Viewport{}, nil)

	dlg := doc.ElementByID("dlg")
	dlgStyle := styles[dlg.ID]

	// Normal style has opacity 1 and display block
	if dlgStyle.Opacity != 1 {
		t.Errorf("dialog normal opacity = %v, want 1", dlgStyle.Opacity)
	}
	if dlgStyle.Display != style.DisplayBlock {
		t.Errorf("dialog normal display = %v, want DisplayBlock", dlgStyle.Display)
	}

	// Starting style has opacity 0 and display none
	if dlgStyle.Starting == nil {
		t.Fatalf("dialog Starting is nil, want populated StartingStyle")
	}
	if !dlgStyle.Starting.HasOpacity || dlgStyle.Starting.Opacity != 0 {
		t.Errorf("dialog starting opacity = %v (has=%v), want 0 (has=true)", dlgStyle.Starting.Opacity, dlgStyle.Starting.HasOpacity)
	}
	if dlgStyle.Starting.Display != style.DisplayNone {
		t.Errorf("dialog starting display = %v, want DisplayNone", dlgStyle.Starting.Display)
	}

	// Nested starting-style on .card
	c := doc.ElementByID("c")
	cStyle := styles[c.ID]

	if cStyle.Opacity != 1 {
		t.Errorf("card normal opacity = %v, want 1", cStyle.Opacity)
	}
	if cStyle.Starting == nil {
		t.Fatalf("card Starting is nil, want populated StartingStyle")
	}
	if !cStyle.Starting.HasOpacity || cStyle.Starting.Opacity < 0.19 || cStyle.Starting.Opacity > 0.21 {
		t.Errorf("card starting opacity = %v, want ~0.2", cStyle.Starting.Opacity)
	}
}
