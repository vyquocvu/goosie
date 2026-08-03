//go:build e2e && online

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/vyquocvu/goosie/internal/renderer"
)

func TestReproIANADumpLayout(t *testing.T) {
	url := "https://www.iana.org/help/example-domains"
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	require.NoError(t, err)

	r := renderer.NewRenderer(1280, 800)
	r.SetTestingMode(true)
	r.SetCurrentURL(url)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = r.RenderHTML(ctx, buf.String())
	require.NoError(t, err)

	root := r.GetRoot()
	require.NotNil(t, root)
	dumpNodes(r, root, 0)
}

func dumpNodes(r *renderer.Renderer, node *renderer.RenderNode, depth int) {
	if node == nil {
		return
	}
	if depth > 14 {
		return
	}
	typ := "?"
	if node.Type == renderer.NodeTypeElement {
		typ = node.TagName
	} else if node.Type == renderer.NodeTypeText {
		typ = "text"
	}
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}
	cs := node.ComputedStyle
	interesting := map[string]bool{
		"html": true, "article": true, "main": true, "nav": true, "header": true,
		"div": true, "body": true, "footer": true, "table": true,
		"ol": true, "ul": true, "p": true,
	}
	if !interesting[typ] {
		return
	}
	fmt.Printf("%s%s", indent, typ)
	if node.TagName != "" {
		if id := node.Attrs["id"]; id != "" {
			fmt.Printf(" #%s", id)
		}
	}
	lb := r.GetLayoutBox(node)
	if cs != nil {
		fmt.Printf(" display=%s flexBasis=%s flexGrow=%.0f width=%s float=%s", cs.Display, cs.FlexBasis, cs.FlexGrow, cs.Width, cs.Float)
		if cs.Color != nil {
			r2, g, b, _ := cs.Color.RGBA()
			fmt.Printf(" color=rgb(%d,%d,%d)", r2>>8&0xff, g>>8&0xff, b>>8&0xff)
		}
		if cs.BackgroundColor != nil {
			r2, g, b, _ := cs.BackgroundColor.RGBA()
			fmt.Printf(" bg=rgb(%d,%d,%d)", r2>>8&0xff, g>>8&0xff, b>>8&0xff)
		}
	}
	if lb != nil {
		fmt.Printf(" box=(%.0f,%.0f %.0fx%.0f) pad=(%.0f,%.0f,%.0f,%.0f) margin=(%.0f,%.0f,%.0f,%.0f)",
			lb.Box.X, lb.Box.Y, lb.Box.Width, lb.Box.Height,
			lb.PaddingLeft, lb.PaddingRight, lb.PaddingTop, lb.PaddingBottom,
			lb.MarginLeft, lb.MarginRight, lb.MarginTop, lb.MarginBottom)
	}
	fmt.Println()
	for _, c := range node.Children {
		dumpNodes(r, c, depth+1)
	}
}
