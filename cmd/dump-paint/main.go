package main

import (
	"fmt"
	"os"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/paint"
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

	list := sess.Paint(1)
	dl := list.Build(1)
	fmt.Printf("EXTENT %v\n", dl.Extent())

	for _, cmd := range dl.All() {
		switch cmd.Kind {
		case paint.CmdFill:
			fmt.Printf("FILL rect=(%d,%d,%d,%d) color=%v\n",
				cmd.Rect.X0, cmd.Rect.Y0, cmd.Rect.X1, cmd.Rect.Y1, cmd.Color)
		case paint.CmdBorder:
			fmt.Printf("BORDER rect=(%d,%d,%d,%d)\n",
				cmd.Rect.X0, cmd.Rect.Y0, cmd.Rect.X1, cmd.Rect.Y1)
		case paint.CmdText:
			if len(cmd.Text.Glyphs) > 0 {
				g := cmd.Text.Glyphs[0]
				fmt.Printf("TEXT x=%d y=%d size=%d color=%v text=%q\n",
					g.X, g.Y, g.Size, cmd.Text.Color, string(g.Rune))
			}
		}
	}
}
