package engine_test

import (
	"math"
	"testing"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/paint"
)

func TestDocumentStyleApplied(t *testing.T) {
	sess, err := engine.NewSession(`<html><head>
<style>
div { background-color: #336699; }
</style>
</head><body style="margin: 0;">
<div style="width: 50px; height: 50px;">x</div>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	list := sess.Paint(1)
	dl := list.Build(1)
	fills := 0
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color.R() == 0x33 && c.Color.G() == 0x66 && c.Color.B() == 0x99 {
			fills++
		}
	}
	if fills == 0 {
		t.Fatal("no fill command with the stylesheet color found in display list")
	}
}

func TestLinkPseudoClassApplied(t *testing.T) {
	sess, err := engine.NewSession(`<html><head>
<style>
a:link, a:visited { color: #38488f; }
</style>
</head><body>
<a href="https://example.com">More information...</a>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	list := sess.Paint(1)
	dl := list.Build(1)
	foundLinkColor := false
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color.R() == 0x38 && c.Text.Color.G() == 0x48 && c.Text.Color.B() == 0x8f {
			foundLinkColor = true
			break
		}
	}
	if !foundLinkColor {
		t.Fatal("no text command with a:link/#38488f color found in display list")
	}
}

func TestEmMarginResolution(t *testing.T) {
	sess, err := engine.NewSession(`<html><body>
<h1>Title</h1>
</body></html>`, nil, 800)
	if err != nil {
		t.Fatal(err)
	}

	for _, obj := range sess.Arena.Objects {
		if obj.Node != nil && obj.Node.Data == "h1" && obj.Style != nil {
			if obj.Style.FontSize != 32 {
				t.Fatalf("h1 font size = %v, want 32", obj.Style.FontSize)
			}
			if obj.MarginTop < 20 || obj.MarginTop > 23 {
				t.Fatalf("h1 MarginTop = %v, want ~21.44", obj.MarginTop)
			}
			return
		}
	}
	t.Fatal("h1 object not found")
}

func TestExampleDotComRender(t *testing.T) {
	exampleHTML := `<!doctype html>
<html>
<head>
    <title>Example Domain</title>
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    a:link, a:visited {
        color: #38488f;
        text-decoration: none;
    }
    </style>
</head>
<body>
<div>
    <h1>Example Domain</h1>
    <p>This domain is for use in illustrative examples in documents. You may use this
    domain in literature without prior coordination or asking for permission.</p>
    <p><a href="https://www.iana.org/domains/reserved">More information...</a></p>
</div>
</body>
</html>`

	viewportW := float32(1440)
	sess, err := engine.NewSession(exampleHTML, nil, viewportW)
	if err != nil {
		t.Fatal(err)
	}

	bg := sess.BackgroundColor()
	if bg.R() != 0xf0 || bg.G() != 0xf0 || bg.B() != 0xf2 {
		t.Errorf("sess.BackgroundColor = %v, want #f0f0f2", bg)
	}

	var divObj *layout.Object
	for i := range sess.Arena.Objects {
		obj := &sess.Arena.Objects[i]
		if obj.Node != nil && obj.Node.Data == "div" && obj.Style != nil {
			divObj = obj
			break
		}
	}
	if divObj == nil {
		t.Fatal("div not found")
	}

	if divObj.W != 600 {
		t.Errorf("div.W = %v, want 600", divObj.W)
	}
	if divObj.PaddingLeft != 32 || divObj.PaddingRight != 32 {
		t.Errorf("div padding = %v, %v, want 32, 32", divObj.PaddingLeft, divObj.PaddingRight)
	}
	if divObj.MarginTop != 80 {
		t.Errorf("div.MarginTop = %v, want 80", divObj.MarginTop)
	}
	wantMargin := float32(388)
	if math.Abs(float64(divObj.MarginLeft-wantMargin)) > 1 {
		t.Errorf("div.MarginLeft = %v, want %v", divObj.MarginLeft, wantMargin)
	}
	if math.Abs(float64(divObj.X-wantMargin)) > 1 {
		t.Errorf("div.X = %v, want %v", divObj.X, wantMargin)
	}

	list := sess.Paint(1)
	dl := list.Build(1)

	foundDivFill := false
	for _, c := range dl.All() {
		if c.Kind == paint.CmdFill && c.Color.R() == 0xfd && c.Color.G() == 0xfd && c.Color.B() == 0xff {
			foundDivFill = true
			if c.Rect.X0 < int32(wantMargin-2) || c.Rect.X0 > int32(wantMargin+2) {
				t.Errorf("div fill rect X0 = %v, want %v", c.Rect.X0, wantMargin)
			}
			break
		}
	}
	if !foundDivFill {
		t.Error("no div fill command with #fdfdff found")
	}

	foundLinkColor := false
	for _, c := range dl.All() {
		if c.Kind == paint.CmdText && c.Text.Color.R() == 0x38 && c.Text.Color.G() == 0x48 && c.Text.Color.B() == 0x8f {
			foundLinkColor = true
			break
		}
	}
	if !foundLinkColor {
		t.Error("no text command with link color #38488f found")
	}
}
