package main

import (
	"fmt"
	"os"

	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: debug-layout <html-file>")
		os.Exit(1)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sess, err := engine.NewSession(string(data), nil, 1440)
	if err != nil {
		fmt.Fprintln(os.Stderr, "session:", err)
		os.Exit(1)
	}
	arena := sess.Arena
	for i := layout.ObjectID(1); i < layout.ObjectID(len(arena.Objects)); i++ {
		obj := arena.Get(i)
		tag := ""
		if obj.Node != nil {
			tag = obj.Node.Data
			if obj.Node.Type == 2 {
				tag = "TEXT:" + obj.Node.DataContent
			}
		}
		display := ""
		if obj.Style != nil {
			display = fmt.Sprintf("%d", obj.Style.Display)
		}
		fmt.Printf("  [%d] tag=%-20s display=%-10s X=%.0f Y=%.0f W=%.0f H=%.0f kids=%d next=%d\n",
			i, tag, display, obj.X, obj.Y, obj.W, obj.H, obj.FirstKid, obj.NextSibling)
	}
}
