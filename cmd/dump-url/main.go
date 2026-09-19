package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/net"
	"github.com/vyquocvu/goosie/internal/raster"
)

// dump-url prints the layout tree goosie builds for a live URL, for diagnosing
// parity failures against Chromium.
func main() {
	url := flag.String("url", "", "URL to load")
	w := flag.Float64("width", 1280, "viewport width")
	h := flag.Float64("height", 800, "viewport height")
	tag := flag.String("tag", "", "only dump subtrees whose tag matches")
	depth := flag.Int("max-depth", 40, "maximum depth to print")
	flag.Parse()

	fonts, err := raster.NewFonts()
	if err != nil {
		panic(err)
	}
	client := net.DefaultClient()
	ctx := context.Background()
	resp, err := client.Get(ctx, *url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch:", err)
		os.Exit(1)
	}
	linker := func(base, href string) (string, error) {
		r, err := client.Get(ctx, href)
		if err != nil {
			return "", err
		}
		if refused := r.Headers["Content-Type"]; net.StyleSheetRefused(refused) {
			return "", fmt.Errorf("%s is served as %q", href, refused)
		}
		return r.Text(), nil
	}
	imageFetcher := func(base, u string) ([]byte, error) {
		r, err := client.Get(ctx, u)
		if err != nil {
			return nil, err
		}
		return r.Body, nil
	}
	sess, err := engine.NewSession(resp.Text(), nil, float32(*w),
		engine.WithMetrics(fonts),
		engine.WithViewportH(float32(*h)),
		engine.WithLinkedCSS(resp.URL, linker),
		engine.WithImages(resp.URL, imageFetcher))
	if err != nil {
		fmt.Fprintln(os.Stderr, "session:", err)
		os.Exit(1)
	}
	a := sess.Arena
	dump(a, layout.ObjectID(1), 0, *tag, *depth)
}

func dump(a *layout.Arena, id layout.ObjectID, depth int, want string, maxDepth int) {
	if depth > maxDepth {
		return
	}
	obj := a.Get(id)
	tag := ""
	if obj.Node != nil {
		tag = obj.Node.Data
	}
	cls := ""
	if obj.Node != nil {
		cls = obj.Node.GetAttribute("class")
	}
	match := want == "" || tag == want || cls == want
	if match {
		indent := ""
		for i := 0; i < depth; i++ {
			indent += "  "
		}
		extra := ""
		if obj.Style != nil {
			extra = fmt.Sprintf(" disp=%d pos=%d fs=%.1f fam=%v w=%.0f h=%.0f minw=%.0f maxw=%.0f",
				obj.Style.Display, obj.Style.Position, obj.Style.FontSize,
				obj.Style.FontFamily, obj.Style.Width, obj.Style.Height,
				obj.Style.MinWidth, obj.Style.MaxWidth)
		}
		fmt.Printf("%s%s.%s X=%.0f Y=%.0f W=%.0f H=%.0f pad=%.0f/%.0f/%.0f/%.0f mar=%.0f/%.0f/%.0f/%.0f%s\n",
			indent, tag, cls, obj.X, obj.Y, obj.W, obj.H,
			obj.PaddingLeft, obj.PaddingTop, obj.PaddingRight, obj.PaddingBottom,
			obj.MarginLeft, obj.MarginTop, obj.MarginRight, obj.MarginBottom, extra)
	}
	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		dump(a, kid, depth+1, want, maxDepth)
	}
}
