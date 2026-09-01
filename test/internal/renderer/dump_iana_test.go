package renderer_test

import (
	"github.com/vyquocvu/goosie/internal/renderer"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/vyquocvu/goosie/internal/css"
)

func TestDumpIANA(t *testing.T) {
	// 1. Fetch IANA html
	resp, err := http.Get("https://www.iana.org/help/example-domains")
	if err != nil {
		t.Fatalf("Failed to fetch IANA HTML: %v", err)
	}
	defer resp.Body.Close()
	htmlBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read html: %v", err)
	}
	htmlStr := string(htmlBytes)

	// 2. Fetch stylesheet
	respCSS, err := http.Get("https://www.iana.org/static/css/iana_website.968be078325a.css")
	if err != nil {
		t.Fatalf("Failed to fetch CSS: %v", err)
	}
	defer respCSS.Body.Close()
	cssBytes, err := io.ReadAll(respCSS.Body)
	if err != nil {
		t.Fatalf("Failed to read CSS: %v", err)
	}
	cssStr := string(cssBytes)

	// 3. Build render tree
	renderTree, err := parseHTMLToRenderTree(htmlStr)
	if err != nil {
		t.Fatalf("parseHTMLToRenderTree: %v", err)
	}

	// 4. Parse & apply styles
	parser := css.NewParser(cssStr)
	stylesheet, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse CSS failed: %v", err)
	}
	styleManager := renderer.NewStyleManagerWithViewport(stylesheet, 800, 600)
	styleManager.ApplyStyles(renderTree)

	// 5. Compute layout
	layoutEngine := renderer.NewLayoutEngine(800, 600)
	layoutRoot := layoutEngine.ComputeLayout(renderTree)

	// 6. Build display list
	dlb := renderer.NewDisplayListBuilder()
	dl := dlb.Build(layoutRoot, renderTree)
	renderer.SortByZIndex(dl)

	fmt.Println("--- START PAINT COMMANDS ---")
	for i, cmd := range dl.Commands {
		tagName := "text"
		if cmd.Node != nil {
			tagName = cmd.Node.TagName
		}
		var extra string
		if cmd.Type == renderer.PaintText {
			extra = fmt.Sprintf("text=%q color=%v", cmd.Text, cmd.Color)
		} else if cmd.Type == renderer.PaintRect {
			extra = fmt.Sprintf("fill=%v", cmd.FillColor)
		}
		fmt.Printf("cmd[%d] Type=%v node=%q box={x:%.1f y:%.1f w:%.1f h:%.1f} %s\n",
			i, cmd.Type, tagName, cmd.Box.X, cmd.Box.Y, cmd.Box.Width, cmd.Box.Height, extra)
	}
	fmt.Println("--- END PAINT COMMANDS ---")
}
