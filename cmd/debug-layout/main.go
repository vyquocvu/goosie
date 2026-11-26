package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/vyquocvu/goosie/internal/dom"
	"github.com/vyquocvu/goosie/internal/engine"
	"github.com/vyquocvu/goosie/internal/layout"
	"github.com/vyquocvu/goosie/internal/style"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: debug-layout <html-file> [width] [height]")
		os.Exit(1)
	}
	width := float32(1440)
	height := float32(1024)
	if len(os.Args) > 2 {
		if v, err := strconv.ParseFloat(os.Args[2], 32); err == nil && v > 0 {
			width = float32(v)
		}
	}
	if len(os.Args) > 3 {
		if v, err := strconv.ParseFloat(os.Args[3], 32); err == nil && v > 0 {
			height = float32(v)
		}
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sess, err := engine.NewSession(string(data), nil, width, engine.WithViewportH(height))
	if err != nil {
		fmt.Fprintln(os.Stderr, "session:", err)
		os.Exit(1)
	}
	arena := sess.Arena
	sheets := sess.Stylesheets()
	for i := layout.ObjectID(1); i < layout.ObjectID(len(arena.Objects)); i++ {
		obj := arena.Get(i)
		tag, classes, parent := "", "", ""
		if obj.Node != nil {
			tag = nodeLabel(obj.Node)
			classes = strings.Join(obj.Node.ClassList(), ".")
			if obj.Node.Parent != nil {
				parent = nodeLabel(obj.Node.Parent)
			}
		}
		display := ""
		if obj.Style != nil {
			display = fmt.Sprintf("%d", obj.Style.Display)
		}
		fmt.Printf("  [%d] tag=%-20s class=%-44s parent=%-20s display=%-10s X=%.0f Y=%.0f W=%.0f H=%.0f kids=%d next=%d\n",
			i, clip(tag, 20), clip(classes, 44), clip(parent, 20), display, obj.X, obj.Y, obj.W, obj.H, obj.FirstKid, obj.NextSibling)
		if obj.Node == nil {
			continue
		}
		matches := style.DebugMatches(sheets, obj.Node)
		for j, match := range matches {
			if j >= 24 {
				fmt.Printf("        selectors=+%d\n", len(matches)-j)
				break
			}
			fmt.Printf("        selector=%s specificity=%s rank=%d decls=%s\n",
				clip(match.Text, 96), match.Specificity, match.Rank, clip(strings.Join(match.Declarations, ";"), 160))
		}
	}
}

func nodeLabel(n *dom.Node) string {
	if n == nil {
		return ""
	}
	if n.Text() {
		return "TEXT:" + clip(strings.TrimSpace(n.DataContent), 16)
	}
	if id := n.GetAttribute("id"); id != "" {
		return n.Data + "#" + id
	}
	return n.Data
}

func clip(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
