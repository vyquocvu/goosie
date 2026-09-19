package main

import (
	"fmt"
	"os"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/raster"
)

func main() {
	html, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}

	fonts, err := raster.NewFonts()
	if err != nil {
		panic(err)
	}

	sess, err := engine.NewSession(string(html), nil, 800, engine.WithMetrics(fonts), engine.WithViewportH(600))
	if err != nil {
		panic(err)
	}

	dumpArena(sess.Arena, layout.ObjectID(1), 0)
}

func dumpArena(a *layout.Arena, id layout.ObjectID, depth int) {
	obj := a.Get(id)
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}

	tag := ""
	if obj.Node != nil {
		tag = obj.Node.Data
	}
	styleInfo := ""
	if obj.Style != nil {
		styleInfo = fmt.Sprintf(" display=%d pad=%v,%v,%v,%v",
			obj.Style.Display,
			obj.PaddingLeft, obj.PaddingTop, obj.PaddingRight, obj.PaddingBottom)
	}

	fmt.Printf("%sID=%d tag=%q X=%.0f Y=%.0f W=%.0f H=%.0f marg=%v,%v,%v,%v%s\n",
		indent, id, tag, obj.X, obj.Y, obj.W, obj.H,
		obj.MarginLeft, obj.MarginTop, obj.MarginRight, obj.MarginBottom,
		styleInfo)

	for kid := obj.FirstKid; kid != 0; kid = a.Get(kid).NextSibling {
		dumpArena(a, kid, depth+1)
	}
}
